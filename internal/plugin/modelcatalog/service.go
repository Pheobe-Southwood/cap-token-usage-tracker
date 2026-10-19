package modelcatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Source values reported to the dashboard.
const (
	SourceEmbedded = "embedded"
	SourceCache    = "cache"
	SourceRemote   = "remote"
)

// Status values reported to the dashboard.
const (
	StatusOK      = "ok"
	StatusStale   = "stale"
	StatusMissing = "missing"
	StatusError   = "error"
)

const (
	// DefaultURL is the upstream catalog endpoint.
	DefaultURL = "https://models.dev/api.json"
	// DefaultStaleAfter is how long a downloaded catalog stays fresh.
	DefaultStaleAfter = 24 * time.Hour
	// refreshTimeout bounds one upstream download.
	refreshTimeout = 20 * time.Second
	// refreshCooldown prevents hammering the upstream.
	refreshCooldown = 30 * time.Second
	// maxRedirects bounds redirect chains; every hop must keep the host.
	maxRedirects = 3
)

// ErrRefreshTooSoon is returned when a refresh is requested inside the
// cooldown window.
var ErrRefreshTooSoon = errors.New("model catalog was refreshed moments ago")

// MaxPayloadBytes caps the accepted upstream response size. It is a variable so
// tests can lower the bound.
var MaxPayloadBytes = 16 << 20

// redirectPolicy keeps catalog redirects on the configured host and bounds the
// chain length.
func redirectPolicy(request *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("model catalog redirect limit reached")
	}
	if !strings.EqualFold(request.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("model catalog redirect left %s", via[0].URL.Host)
	}
	return nil
}

// Options configures a Service.
type Options struct {
	// CachePath is the trimmed catalog cache file. Empty disables caching.
	CachePath string
	// URL is the upstream endpoint. Empty disables remote refresh.
	URL string
	// AutoRefresh records whether the operator allows automatic refreshes.
	AutoRefresh bool
	// StaleAfter is the freshness window; zero uses DefaultStaleAfter.
	StaleAfter time.Duration
	// Snapshot overrides the embedded baseline (tests).
	Snapshot *Snapshot
	// Client overrides the HTTP client (tests).
	Client *http.Client
	// Now overrides the clock (tests).
	Now func() time.Time
}

// Status is the catalog state exposed to the dashboard.
type Status struct {
	Source            string    `json:"source"`
	GeneratedAt       time.Time `json:"generated_at"`
	Stale             bool      `json:"stale"`
	Status            string    `json:"status"`
	Providers         int       `json:"providers"`
	Models            int       `json:"models"`
	PricedModels      int       `json:"priced_models"`
	FreeModels        int       `json:"free_models"`
	AutoRefresh       bool      `json:"auto_refresh"`
	RemoteEnabled     bool      `json:"remote_enabled"`
	LastError         string    `json:"last_error"`
	StaleAfterSeconds int64     `json:"stale_after_seconds"`
}

// Service owns the active catalog snapshot.
type Service struct {
	options Options

	mu        sync.RWMutex
	snapshot  *Snapshot
	index     *Index
	source    string
	fetchedAt time.Time
	lastError string

	refreshMu   sync.Mutex
	lastAttempt time.Time
}

// New builds a service and loads the embedded snapshot plus any cache file.
func New(options Options) (*Service, error) {
	if options.StaleAfter <= 0 {
		options.StaleAfter = DefaultStaleAfter
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.Client == nil {
		options.Client = &http.Client{Timeout: refreshTimeout, CheckRedirect: redirectPolicy}
	}
	service := &Service{options: options}
	if err := service.load(); err != nil {
		return nil, err
	}
	return service, nil
}

// load resolves the startup snapshot: the newer of the cache file and the
// embedded baseline. A corrupt cache is removed so the next start is clean.
func (s *Service) load() error {
	embedded := s.options.Snapshot
	if embedded == nil {
		var err error
		embedded, err = Embedded()
		if err != nil {
			return err
		}
	}
	snapshot := embedded
	source := SourceEmbedded
	if cache, ok := s.readCache(); ok && cache.GeneratedAt.After(embedded.GeneratedAt) {
		snapshot = cache
		source = SourceCache
	}
	s.mu.Lock()
	s.snapshot = snapshot
	s.index = NewIndex(snapshot)
	s.source = source
	s.fetchedAt = snapshot.GeneratedAt
	s.mu.Unlock()
	return nil
}

func (s *Service) readCache() (*Snapshot, bool) {
	path := strings.TrimSpace(s.options.CachePath)
	if path == "" {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	snapshot, err := Parse(raw)
	if err != nil || !snapshot.Valid() {
		_ = os.Remove(path)
		return nil, false
	}
	return snapshot, true
}

// Snapshot returns the active catalog and its index.
func (s *Service) Snapshot() (*Snapshot, *Index) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot, s.index
}

// Status reports the current catalog state.
func (s *Service) Status() Status {
	if s == nil {
		return Status{Status: StatusMissing, Source: SourceEmbedded}
	}
	s.mu.RLock()
	snapshot := s.snapshot
	source := s.source
	lastError := s.lastError
	s.mu.RUnlock()

	status := Status{
		Source:            source,
		AutoRefresh:       s.options.AutoRefresh,
		RemoteEnabled:     strings.TrimSpace(s.options.URL) != "",
		LastError:         lastError,
		StaleAfterSeconds: int64(s.options.StaleAfter / time.Second),
	}
	if snapshot == nil {
		status.Status = StatusMissing
		return status
	}
	stats := snapshot.ComputeStats()
	status.GeneratedAt = snapshot.GeneratedAt
	status.Providers = stats.Providers
	status.Models = stats.Models
	status.PricedModels = stats.Priced
	status.FreeModels = stats.Free
	status.Stale = s.isStale(snapshot)
	switch {
	case status.Stale:
		status.Status = StatusStale
	default:
		status.Status = StatusOK
	}
	if lastError != "" && status.Stale {
		status.Status = StatusError
	}
	return status
}

func (s *Service) isStale(snapshot *Snapshot) bool {
	if snapshot == nil {
		return true
	}
	generated := snapshot.GeneratedAt
	if generated.IsZero() {
		return true
	}
	return s.options.Now().Sub(generated) > s.options.StaleAfter
}

// NeedsRefresh reports whether the catalog is stale and remote refresh is
// enabled.
func (s *Service) NeedsRefresh() bool {
	if s == nil || strings.TrimSpace(s.options.URL) == "" {
		return false
	}
	s.mu.RLock()
	snapshot := s.snapshot
	s.mu.RUnlock()
	return s.isStale(snapshot)
}

// Refresh downloads, trims, validates and atomically caches a new catalog. A
// failure keeps the current snapshot and records the error.
func (s *Service) Refresh(ctx context.Context) (Status, error) {
	if s == nil {
		return Status{Status: StatusMissing}, errors.New("model catalog is unavailable")
	}
	if strings.TrimSpace(s.options.URL) == "" {
		return s.Status(), errors.New("model catalog refresh is disabled")
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if !s.lastAttempt.IsZero() && s.options.Now().Sub(s.lastAttempt) < refreshCooldown {
		return s.Status(), ErrRefreshTooSoon
	}
	s.lastAttempt = s.options.Now()

	raw, err := s.download(ctx)
	if err != nil {
		s.recordError(err)
		return s.Status(), err
	}
	generatedAt := s.options.Now()
	trimmed, snapshot, err := Trim(raw, generatedAt)
	if err != nil {
		s.recordError(err)
		return s.Status(), err
	}
	if !snapshot.Valid() {
		err = errors.New("model catalog has no providers or models")
		s.recordError(err)
		return s.Status(), err
	}
	// A read-only data directory must not discard a usable catalog; the write
	// failure is reported through last_error while the refresh still succeeds.
	cacheErr := s.writeCache(trimmed)
	s.mu.Lock()
	s.snapshot = snapshot
	s.index = NewIndex(snapshot)
	s.source = SourceRemote
	s.fetchedAt = generatedAt
	s.lastError = ""
	if cacheErr != nil {
		s.lastError = cacheErr.Error()
	}
	s.mu.Unlock()
	return s.Status(), nil
}

func (s *Service) download(ctx context.Context) ([]byte, error) {
	endpoint := strings.TrimSpace(s.options.URL)
	parsed, err := url.Parse(endpoint)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return nil, fmt.Errorf("model catalog URL must be an absolute https URL")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build model catalog request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.options.Client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download model catalog: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("download model catalog: HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(MaxPayloadBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("read model catalog: %w", err)
	}
	if len(raw) > MaxPayloadBytes {
		return nil, fmt.Errorf("model catalog exceeds %d bytes", MaxPayloadBytes)
	}
	return raw, nil
}

// writeCache persists the trimmed catalog with a same-directory temp file so a
// crash cannot publish a partial document.
func (s *Service) writeCache(raw []byte) error {
	path := strings.TrimSpace(s.options.CachePath)
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create model catalog directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, "model-catalog-*.tmp")
	if err != nil {
		return fmt.Errorf("create model catalog temp file: %w", err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write model catalog cache: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync model catalog cache: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close model catalog cache: %w", err)
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return fmt.Errorf("chmod model catalog cache: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace model catalog cache: %w", err)
	}
	cleanup = false
	return nil
}

func (s *Service) recordError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.lastError = err.Error()
	s.mu.Unlock()
}
