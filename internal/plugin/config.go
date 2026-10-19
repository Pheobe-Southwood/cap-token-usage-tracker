package plugin

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/AITNR/cap-token-usage-tracker/internal/plugin/modelcatalog"
)

const (
	defaultRetentionDays               = 365
	defaultSessionTTLMinutes           = 720
	minSessionTTLMinutes               = 1
	maxSessionTTLMinutes               = 43200
	defaultFlushInterval               = 5 * time.Second
	defaultFlushMaxRecords             = 100
	defaultResponseCompression         = true
	defaultResponseCompressionMinBytes = 1024
	maxResponseCompressionMinBytes     = 16 << 20
	defaultPricingCatalogURL           = modelcatalog.DefaultURL
	defaultPricingAutoRefresh          = true
	defaultPricingStaleAfter           = modelcatalog.DefaultStaleAfter
	minPricingStaleAfter               = time.Hour
	maxPricingStaleAfter               = 720 * time.Hour
)

type Config struct {
	DataPath                    string
	RetentionDays               int
	FlushInterval               time.Duration
	FlushMaxRecords             int
	SyncOnRecord                bool
	APIKeySecret                string
	ResponseCompression         bool
	ResponseCompressionMinBytes int
	SessionTTLMinutes           int
	PricingCatalogURL           string
	PricingAutoRefresh          bool
	PricingStaleAfter           time.Duration
	// TTFTSupplement links the sibling opencode-go plugin's published TTFT
	// statistics into the models view. The host reports no TTFT for plugin
	// executors, so without the link those models would show no first-token
	// latency at all.
	TTFTSupplement    bool
	TTFTSupplementDir string
	// PeakPricing enables the bundled vendor time-of-use schedules (currently
	// the DeepSeek peak hours). Manual per-model schedules are unaffected.
	PeakPricing bool
}

type configYAML struct {
	DataPath                    string  `yaml:"data_path"`
	RetentionDays               *int    `yaml:"retention_days"`
	FlushInterval               string  `yaml:"flush_interval"`
	FlushMaxRecords             *int    `yaml:"flush_max_records"`
	SyncOnRecord                *bool   `yaml:"sync_on_record"`
	APIKeySecret                *string `yaml:"api_key_secret"`
	ResponseCompression         *bool   `yaml:"response_compression"`
	ResponseCompressionMinBytes *int    `yaml:"response_compression_min_bytes"`
	SessionTTLMinutes           *int    `yaml:"session_ttl_minutes"`
	PricingCatalogURL           *string `yaml:"pricing_catalog_url"`
	PricingAutoRefresh          *bool   `yaml:"pricing_auto_refresh"`
	PricingStaleAfter           string  `yaml:"pricing_stale_after"`
	TTFTSupplement              *bool   `yaml:"ttft_supplement"`
	TTFTSupplementDir           string  `yaml:"ttft_supplement_dir"`
	PeakPricing                 *bool   `yaml:"peak_pricing"`
}

func defaultConfig() Config {
	return Config{
		DataPath:                    resolvedDefaultDataPath(),
		RetentionDays:               defaultRetentionDays,
		FlushInterval:               defaultFlushInterval,
		FlushMaxRecords:             defaultFlushMaxRecords,
		SyncOnRecord:                true,
		APIKeySecret:                defaultAPIKeySecret,
		ResponseCompression:         defaultResponseCompression,
		ResponseCompressionMinBytes: defaultResponseCompressionMinBytes,
		SessionTTLMinutes:           defaultSessionTTLMinutes,
		PricingCatalogURL:           defaultPricingCatalogURL,
		PricingAutoRefresh:          defaultPricingAutoRefresh,
		PricingStaleAfter:           defaultPricingStaleAfter,
		TTFTSupplement:              true,
		PeakPricing:                 true,
	}
}

func parseConfig(raw []byte) (Config, error) {
	cfg := defaultConfig()
	if len(raw) == 0 {
		return normalizeConfig(cfg)
	}

	var input configYAML
	if err := yaml.Unmarshal(raw, &input); err != nil {
		return Config{}, fmt.Errorf("parse config YAML: %w", err)
	}
	if strings.TrimSpace(input.DataPath) != "" {
		cfg.DataPath = strings.TrimSpace(input.DataPath)
	}
	if input.RetentionDays != nil {
		cfg.RetentionDays = *input.RetentionDays
	}
	if strings.TrimSpace(input.FlushInterval) != "" {
		interval, err := time.ParseDuration(strings.TrimSpace(input.FlushInterval))
		if err != nil {
			return Config{}, fmt.Errorf("parse flush_interval: %w", err)
		}
		cfg.FlushInterval = interval
	}
	if input.FlushMaxRecords != nil {
		cfg.FlushMaxRecords = *input.FlushMaxRecords
	}
	if input.SyncOnRecord != nil {
		cfg.SyncOnRecord = *input.SyncOnRecord
	}
	if input.APIKeySecret != nil {
		cfg.APIKeySecret = *input.APIKeySecret
	}
	if input.ResponseCompression != nil {
		cfg.ResponseCompression = *input.ResponseCompression
	}
	if input.ResponseCompressionMinBytes != nil {
		cfg.ResponseCompressionMinBytes = *input.ResponseCompressionMinBytes
	}
	if input.SessionTTLMinutes != nil {
		cfg.SessionTTLMinutes = *input.SessionTTLMinutes
	}
	if input.PricingCatalogURL != nil {
		cfg.PricingCatalogURL = strings.TrimSpace(*input.PricingCatalogURL)
	}
	if input.PricingAutoRefresh != nil {
		cfg.PricingAutoRefresh = *input.PricingAutoRefresh
	}
	if strings.TrimSpace(input.PricingStaleAfter) != "" {
		interval, err := time.ParseDuration(strings.TrimSpace(input.PricingStaleAfter))
		if err != nil {
			return Config{}, fmt.Errorf("parse pricing_stale_after: %w", err)
		}
		cfg.PricingStaleAfter = interval
	}
	if input.TTFTSupplement != nil {
		cfg.TTFTSupplement = *input.TTFTSupplement
	}
	if strings.TrimSpace(input.TTFTSupplementDir) != "" {
		cfg.TTFTSupplementDir = strings.TrimSpace(input.TTFTSupplementDir)
	}
	if input.PeakPricing != nil {
		cfg.PeakPricing = *input.PeakPricing
	}
	return normalizeConfig(cfg)
}

func normalizeConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.DataPath) == "" {
		return Config{}, fmt.Errorf("data_path must not be empty")
	}
	if cfg.RetentionDays < 1 || cfg.RetentionDays > 3650 {
		return Config{}, fmt.Errorf("retention_days must be between 1 and 3650")
	}
	if cfg.FlushInterval < time.Second || cfg.FlushInterval > time.Hour {
		return Config{}, fmt.Errorf("flush_interval must be between 1s and 1h")
	}
	if cfg.FlushMaxRecords < 1 || cfg.FlushMaxRecords > 1_000_000 {
		return Config{}, fmt.Errorf("flush_max_records must be between 1 and 1000000")
	}
	if cfg.APIKeySecret != "" && cfg.APIKeySecret != defaultAPIKeySecret && len([]byte(cfg.APIKeySecret)) < 32 {
		return Config{}, fmt.Errorf("api_key_secret must be empty, 123456, or at least 32 bytes")
	}
	if cfg.ResponseCompressionMinBytes < 0 || cfg.ResponseCompressionMinBytes > maxResponseCompressionMinBytes {
		return Config{}, fmt.Errorf("response_compression_min_bytes must be between 0 and %d", maxResponseCompressionMinBytes)
	}
	if cfg.SessionTTLMinutes < minSessionTTLMinutes || cfg.SessionTTLMinutes > maxSessionTTLMinutes {
		return Config{}, fmt.Errorf("session_ttl_minutes must be between %d and %d", minSessionTTLMinutes, maxSessionTTLMinutes)
	}
	if cfg.PricingStaleAfter < minPricingStaleAfter || cfg.PricingStaleAfter > maxPricingStaleAfter {
		return Config{}, fmt.Errorf("pricing_stale_after must be between %s and %s", minPricingStaleAfter, maxPricingStaleAfter)
	}
	if endpoint := strings.TrimSpace(cfg.PricingCatalogURL); endpoint != "" {
		parsed, err := url.Parse(endpoint)
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
			return Config{}, fmt.Errorf("pricing_catalog_url must be an absolute https URL or empty")
		}
		cfg.PricingCatalogURL = endpoint
	}
	absolute, err := filepath.Abs(filepath.Clean(cfg.DataPath))
	if err != nil {
		return Config{}, fmt.Errorf("resolve data_path: %w", err)
	}
	cfg.DataPath = absolute
	if cfg.TTFTSupplementDir != "" {
		resolved, err := filepath.Abs(filepath.Clean(cfg.TTFTSupplementDir))
		if err != nil {
			return Config{}, fmt.Errorf("resolve ttft_supplement_dir: %w", err)
		}
		cfg.TTFTSupplementDir = resolved
	}
	return cfg, nil
}

// modelCatalogCachePath keeps the trimmed catalog cache next to the database so
// it follows the operator's data_path.
func modelCatalogCachePath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), "token-usage-tracker-model-catalog.json")
}
