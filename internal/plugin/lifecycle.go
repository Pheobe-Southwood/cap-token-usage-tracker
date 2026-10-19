package plugin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"

	"github.com/AITNR/cap-token-usage-tracker/internal/plugin/modelcatalog"
)

// version is set at build time with:
// -X github.com/AITNR/cap-token-usage-tracker/internal/plugin.version=<version>
var version = "dev"

// maxSupportedRPCSchema is intentionally independent from the SDK's latest
// schema. Future hosts may negotiate down to this verified contract without
// the plugin claiming support for semantics it has not implemented.
const maxSupportedRPCSchema uint32 = 3

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationCapabilities struct {
	UsagePlugin   bool `json:"usage_plugin"`
	ManagementAPI bool `json:"management_api"`
}

type pluginRuntime struct {
	lifecycleMu       sync.Mutex
	mu                sync.RWMutex
	store             *Store
	pricing           *pricingTable
	config            Config
	crypto            cryptoContext
	apiKeyGeneration  uint64
	apiKeyGenerations map[uint64]APIKeyCryptoGeneration
	routes            registeredRoutes
	authResolver      *authIdentityResolver
	sessionMu         sync.Mutex
	sessions          map[[32]byte]time.Time
	// ttftLinkage memoizes the sibling opencode-go plugin's published TTFT
	// statistics so the models endpoint can link them in without re-parsing
	// unchanged files.
	ttftLinkage *ttftLinkageCache
}

var runtimeState = &pluginRuntime{ttftLinkage: newTTFTLinkageCache()}

func (r *pluginRuntime) register(raw []byte) (registration, error) {
	request, config, err := decodeLifecycle(raw)
	if err != nil {
		return registration{}, err
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if err := r.applyConfig(config); err != nil {
		return registration{}, err
	}
	return pluginRegistration(negotiateRPCSchema(request.SchemaVersion)), nil
}

func (r *pluginRuntime) reconfigure(raw []byte) (registration, error) {
	request, config, err := decodeLifecycle(raw)
	if err != nil {
		return registration{}, err
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if err := r.applyConfig(config); err != nil {
		return registration{}, err
	}
	return pluginRegistration(negotiateRPCSchema(request.SchemaVersion)), nil
}

func negotiateRPCSchema(hostSchema uint32) uint32 {
	if hostSchema == 0 {
		return 1
	}
	if hostSchema < maxSupportedRPCSchema {
		return hostSchema
	}
	return maxSupportedRPCSchema
}

func (r *pluginRuntime) applyConfig(config Config) error {
	crypto, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		return err
	}
	if r.ttftLinkage == nil {
		r.ttftLinkage = newTTFTLinkageCache()
	} else {
		r.ttftLinkage.reset()
	}
	r.mu.RLock()
	current := r.store
	currentConfig := r.config
	r.mu.RUnlock()

	if current != nil && currentConfig.DataPath == config.DataPath {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.store != current || r.config.DataPath != config.DataPath {
			return errors.New("plugin storage changed during reconfiguration")
		}
		if err := current.ReconfigureWithCrypto(config, crypto); err != nil {
			return err
		}
		pricing, err := newPricingTableFor(config, current)
		if err != nil {
			return err
		}
		generation, generations := current.APIKeyCryptoState()
		r.config = config
		r.pricing = pricing
		r.crypto = crypto
		r.apiKeyGeneration = generation
		r.apiKeyGenerations = generations
		return nil
	}

	next, err := openStoreWithCrypto(config, crypto)
	if err != nil {
		return err
	}
	pricing, err := newPricingTableFor(config, next)
	if err != nil {
		_ = next.Close()
		return err
	}
	r.mu.Lock()
	old := r.store
	r.store = next
	r.pricing = pricing
	r.config = config
	r.crypto = crypto
	r.apiKeyGeneration, r.apiKeyGenerations = next.APIKeyCryptoState()
	r.mu.Unlock()
	r.sessionMu.Lock()
	r.sessions = nil
	r.sessionMu.Unlock()
	if old != nil {
		if err := old.Close(); err != nil {
			return fmt.Errorf("close previous store: %w", err)
		}
	}
	return nil
}

// newPricingTableFor builds the catalog service and loads the manual price
// overrides for a store. A catalog that cannot even be parsed degrades to an
// empty price table instead of failing registration.
func newPricingTableFor(config Config, store *Store) (*pricingTable, error) {
	catalog, err := modelcatalog.New(modelcatalog.Options{
		CachePath:   modelCatalogCachePath(config.DataPath),
		URL:         config.PricingCatalogURL,
		AutoRefresh: config.PricingAutoRefresh,
		StaleAfter:  config.PricingStaleAfter,
	})
	if err != nil {
		catalog = nil
	}
	table := newPricingTable(catalog)
	table.SetBuiltinTimePlans(config.PeakPricing)
	if store == nil {
		return table, nil
	}
	overrides, err := store.ModelPriceOverrides()
	if err != nil {
		return nil, err
	}
	table.SetOverrides(overrides)
	return table, nil
}

// pricingTableRef returns the active price table without holding the runtime
// lock while it is used.
func (r *pluginRuntime) pricingTableRef() *pricingTable {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pricing
}

// pricingStatus reports the catalog state for API responses.
func (r *pluginRuntime) pricingStatus() pricingStatus {
	table := r.pricingTableRef()
	if table == nil {
		return pricingStatus{Status: modelcatalog.Status{Status: modelcatalog.StatusMissing, Source: modelcatalog.SourceEmbedded}}
	}
	return pricingStatus{Status: table.Status(), Overrides: table.OverrideCount()}
}

func (r *pluginRuntime) handleUsage(raw []byte) (map[string]any, error) {
	usage, err := decodeUsage(raw, nowUTC())
	if err != nil {
		return nil, withStatus(400, "%v", err)
	}
	r.resolveUsageIdentity(&usage)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return nil, withStatus(503, "plugin storage is not initialized")
	}
	crypto := r.crypto
	generation := r.apiKeyGeneration
	if generation == 0 && crypto.enabled {
		generation, _ = r.store.APIKeyCryptoState()
	}
	plainKey := usage.Dimensions.APIKey
	if plainKey != "" && crypto.enabled {
		if generation == 0 {
			return nil, errors.New("API key tracking has no active crypto generation")
		}
		fingerprint := apiKeyFingerprint(plainKey, crypto.indexKey)
		ciphertext, err := encryptAPIKeyForGeneration(crypto, plainKey, fingerprint, generation)
		if err != nil {
			return nil, fmt.Errorf("encrypt api key: %w", err)
		}
		usage.Dimensions.APIKeyHash = fingerprint
		usage.Dimensions.APIKeyGeneration = generation
		usage.Dimensions.APIKey = ciphertext
		usage.Dimensions.APIKeyStatus = ""
	} else {
		usage.Dimensions.APIKey = ""
		usage.Dimensions.APIKeyHash = ""
		usage.Dimensions.APIKeyGeneration = 0
		usage.Dimensions.APIKeyStatus = ""
		if crypto.enabled {
			usage.Dimensions.APIKeyStatus = apiKeyStatusSourceMissing
		}
	}
	if err := r.store.Record(usage); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

func (r *pluginRuntime) shutdown() error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.mu.Lock()
	store := r.store
	r.store = nil
	r.pricing = nil
	r.config = Config{}
	r.crypto = cryptoContext{}
	r.apiKeyGeneration = 0
	r.apiKeyGenerations = nil
	r.routes = registeredRoutes{}
	r.authResolver = nil
	r.mu.Unlock()
	r.sessionMu.Lock()
	r.sessions = nil
	r.sessionMu.Unlock()
	if store == nil {
		return nil
	}
	return store.Close()
}

func decodeLifecycle(raw []byte) (lifecycleRequest, Config, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var envelope struct {
		ConfigYAML    json.RawMessage `json:"config_yaml"`
		SchemaVersion uint32          `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return lifecycleRequest{}, Config{}, fmt.Errorf("decode lifecycle request: %w", err)
	}
	configYAML, err := decodeLifecycleConfigYAML(envelope.ConfigYAML)
	if err != nil {
		return lifecycleRequest{}, Config{}, err
	}
	request := lifecycleRequest{ConfigYAML: configYAML, SchemaVersion: envelope.SchemaVersion}
	config, err := parseConfig(request.ConfigYAML)
	if err != nil {
		return lifecycleRequest{}, Config{}, err
	}
	return request, config, nil
}

// decodeLifecycleConfigYAML accepts the host's standard base64 encoding of a
// []byte field, while remaining compatible with hosts/tools that send a plain
// YAML string or an explicit JSON byte array.
func decodeLifecycleConfigYAML(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if decoded, decodeErr := base64.StdEncoding.DecodeString(text); decodeErr == nil && strings.Contains(string(decoded), ":") {
			return decoded, nil
		}
		return []byte(text), nil
	}
	var bytes []byte
	if err := json.Unmarshal(raw, &bytes); err == nil {
		return bytes, nil
	}
	return nil, fmt.Errorf("config_yaml must be a base64/plain string or byte array")
}

func pluginRegistration(schemaVersion uint32) registration {
	return registration{
		SchemaVersion: schemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "CAP Token Usage Tracker",
			Version:          version,
			Author:           "AITNR",
			GitHubRepository: "https://github.com/AITNR/cap-token-usage-tracker",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "data_path", Type: pluginapi.ConfigFieldTypeString, Description: "bbolt database path; defaults to the data directory next to the discovered plugins directory, while explicit relative paths use the CLIProxyAPI working directory."},
				{Name: "retention_days", Type: pluginapi.ConfigFieldTypeInteger, Description: "Number of UTC days of minute-level statistics and request details to retain (1-3650)."},
				{Name: "flush_interval", Type: pluginapi.ConfigFieldTypeString, Description: "Maximum delay before batched statistics are flushed, for example 5s."},
				{Name: "flush_max_records", Type: pluginapi.ConfigFieldTypeInteger, Description: "Flush after this many accepted usage records."},
				{Name: "sync_on_record", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Commit every usage record before acknowledging it."},
				{Name: "api_key_secret", Type: pluginapi.ConfigFieldTypeString, Description: "Secret for API-key encryption and keyed fingerprints. Defaults to 123456; set a custom value of at least 32 bytes for protection against database disclosure. Empty disables API-key tracking. Changing the value starts a new crypto generation while preserving historical records."},
				{Name: "response_compression", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Compress eligible public dashboard HTML and JSON responses with gzip when supported by the client. Defaults to true."},
				{Name: "response_compression_min_bytes", Type: pluginapi.ConfigFieldTypeInteger, Description: "Minimum eligible public response size in bytes before gzip compression. Defaults to 1024; range 0-16777216."},
				{Name: "session_ttl_minutes", Type: pluginapi.ConfigFieldTypeInteger, Description: "Lifetime of a dashboard access session in minutes. Defaults to 720; range 1-43200. Sessions are minted only through the management API, stored as SHA-256 digests in plugin memory, and never persisted."},
				{Name: "ttft_supplement", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Link the sibling cap-opencode-go plugin's published hourly TTFT statistics into the models view. The host reports no TTFT for plugin executors, so without the link those models show no first-token latency. Defaults to true."},
				{Name: "ttft_supplement_dir", Type: pluginapi.ConfigFieldTypeString, Description: "Directory holding the sibling plugin's shared.json and stats-*.json files. Defaults to the cap-opencode-go directory next to data_path. Only model aliases and per-hour TTFT counters are read; credentials are never decoded."},
			},
		},
		Capabilities: registrationCapabilities{UsagePlugin: true, ManagementAPI: true},
	}
}

var nowUTC = func() time.Time { return time.Now().UTC() }
