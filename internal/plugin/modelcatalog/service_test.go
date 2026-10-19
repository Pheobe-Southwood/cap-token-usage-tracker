package modelcatalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const upstreamFixture = `{
  "openai": {"id": "openai", "name": "OpenAI", "models": {
    "gpt-6-luna": {"id": "gpt-6-luna", "name": "GPT-6 Luna", "cost": {"input": 0.1, "output": 0.5}}
  }},
  "google": {"id": "google", "name": "Google", "models": {
    "gemini-3.8-flash": {"id": "gemini-3.8-flash", "name": "Gemini 3.8 Flash", "cost": {"input": 0.75, "output": 3.75}}
  }}
}`

func cacheDocument(t *testing.T, generatedAt time.Time) []byte {
	t.Helper()
	raw, _, err := Trim([]byte(upstreamFixture), generatedAt)
	if err != nil {
		t.Fatalf("Trim fixture: %v", err)
	}
	return raw
}

func TestNewPrefersNewerCacheAndDropsCorruptCache(t *testing.T) {
	embedded, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded: %v", err)
	}
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "model-catalog.json")
	newer := embedded.GeneratedAt.Add(2 * time.Hour)
	if err := os.WriteFile(cachePath, cacheDocument(t, newer), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	service, err := New(Options{CachePath: cachePath, URL: DefaultURL, AutoRefresh: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	status := service.Status()
	if status.Source != SourceCache {
		t.Fatalf("source = %s, want %s", status.Source, SourceCache)
	}
	if status.Providers != 2 || status.Models != 2 {
		t.Fatalf("cached catalog not loaded: %+v", status)
	}

	// An older cache must lose against the embedded snapshot.
	older := embedded.GeneratedAt.Add(-48 * time.Hour)
	if err := os.WriteFile(cachePath, cacheDocument(t, older), 0o600); err != nil {
		t.Fatalf("write old cache: %v", err)
	}
	service, err = New(Options{CachePath: cachePath, URL: DefaultURL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if source := service.Status().Source; source != SourceEmbedded {
		t.Fatalf("source = %s, want %s", source, SourceEmbedded)
	}

	// A corrupt cache is deleted and the embedded snapshot is used.
	if err := os.WriteFile(cachePath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}
	service, err = New(Options{CachePath: cachePath, URL: DefaultURL})
	if err != nil {
		t.Fatalf("New with corrupt cache: %v", err)
	}
	if source := service.Status().Source; source != SourceEmbedded {
		t.Fatalf("source = %s, want %s", source, SourceEmbedded)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt cache was not removed: %v", err)
	}
}

func TestRefreshWritesCacheAndUpdatesStatus(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(upstreamFixture))
	}))
	defer server.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "model-catalog.json")
	embedded, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded: %v", err)
	}
	now := embedded.GeneratedAt.Add(48 * time.Hour)
	service, err := New(Options{CachePath: cachePath, URL: server.URL, Client: server.Client(), AutoRefresh: true, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !service.NeedsRefresh() {
		t.Fatalf("freshly built catalog should be stale until refreshed")
	}
	status, err := service.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if status.Source != SourceRemote || status.Stale || status.Status != StatusOK {
		t.Fatalf("unexpected status after refresh: %+v", status)
	}
	if status.Providers != 2 || status.PricedModels != 2 {
		t.Fatalf("refreshed catalog stats wrong: %+v", status)
	}
	if service.NeedsRefresh() {
		t.Fatalf("catalog should be fresh after refresh")
	}
	raw, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("cache not written: %v", err)
	}
	if _, err := Parse(raw); err != nil {
		t.Fatalf("cache is not a valid catalog: %v", err)
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".tmp") {
				t.Fatalf("temporary cache file left behind: %s", entry.Name())
			}
		}
	}

	// A second refresh inside the cooldown window is refused.
	if _, err := service.Refresh(context.Background()); !errors.Is(err, ErrRefreshTooSoon) {
		t.Fatalf("second refresh error = %v, want ErrRefreshTooSoon", err)
	}
}

func TestRefreshFailureKeepsCatalogAndRecordsError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	embedded, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded: %v", err)
	}
	now := embedded.GeneratedAt.Add(48 * time.Hour)
	service, err := New(Options{CachePath: filepath.Join(t.TempDir(), "catalog.json"), URL: server.URL, Client: server.Client(), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	before, _ := service.Snapshot()
	if _, err := service.Refresh(context.Background()); err == nil {
		t.Fatalf("refresh against HTTP 500 must fail")
	}
	after, _ := service.Snapshot()
	if before != after {
		t.Fatalf("failed refresh replaced the snapshot")
	}
	status := service.Status()
	if status.LastError == "" || status.Status != StatusError {
		t.Fatalf("failure not reported: %+v", status)
	}
	if status.Providers == 0 {
		t.Fatalf("catalog became empty after failure: %+v", status)
	}
}

func TestRefreshRejectsOversizedAndInvalidPayloads(t *testing.T) {
	originalLimit := MaxPayloadBytes
	MaxPayloadBytes = 64
	defer func() { MaxPayloadBytes = originalLimit }()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", 256)))
	}))
	defer server.Close()
	service, err := New(Options{CachePath: filepath.Join(t.TempDir(), "catalog.json"), URL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := service.Refresh(context.Background()); err == nil {
		t.Fatalf("oversized payload must be rejected")
	}

	MaxPayloadBytes = originalLimit
	invalid := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"openai": 5}`))
	}))
	defer invalid.Close()
	service, err = New(Options{CachePath: filepath.Join(t.TempDir(), "catalog.json"), URL: invalid.URL, Client: invalid.Client()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := service.Refresh(context.Background()); err == nil {
		t.Fatalf("payload without providers must be rejected")
	}
}

func TestRefreshDisabledWithoutURL(t *testing.T) {
	service, err := New(Options{CachePath: filepath.Join(t.TempDir(), "catalog.json")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if service.NeedsRefresh() {
		t.Fatalf("refresh must stay disabled without a URL")
	}
	if _, err := service.Refresh(context.Background()); err == nil {
		t.Fatalf("refresh without a URL must fail")
	}
	if status := service.Status(); status.RemoteEnabled || status.Source != SourceEmbedded {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRefreshRejectsNonHTTPSURL(t *testing.T) {
	service, err := New(Options{CachePath: filepath.Join(t.TempDir(), "catalog.json"), URL: "http://models.dev/api.json"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := service.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("plain http URL must be rejected, got %v", err)
	}
}

func TestStalenessUsesInjectedClock(t *testing.T) {
	embedded, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded: %v", err)
	}
	now := embedded.GeneratedAt.Add(2 * time.Hour)
	service, err := New(Options{CachePath: filepath.Join(t.TempDir(), "catalog.json"), URL: DefaultURL, StaleAfter: 24 * time.Hour, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if status := service.Status(); status.Stale || status.Status != StatusOK {
		t.Fatalf("catalog should be fresh: %+v", status)
	}
	now = embedded.GeneratedAt.Add(48 * time.Hour)
	if status := service.Status(); !status.Stale || status.Status != StatusStale {
		t.Fatalf("catalog should be stale: %+v", status)
	}
}

func TestRedirectPolicyKeepsHostAndBoundsChain(t *testing.T) {
	base, _ := http.NewRequest(http.MethodGet, "https://models.dev/api.json", nil)
	sameHost, _ := http.NewRequest(http.MethodGet, "https://models.dev/other.json", nil)
	otherHost, _ := http.NewRequest(http.MethodGet, "https://evil.example/api.json", nil)
	if err := redirectPolicy(sameHost, []*http.Request{base}); err != nil {
		t.Fatalf("same-host redirect rejected: %v", err)
	}
	if err := redirectPolicy(otherHost, []*http.Request{base}); err == nil {
		t.Fatalf("cross-host redirect must be rejected")
	}
	chain := []*http.Request{base, base, base}
	if err := redirectPolicy(sameHost, chain); err == nil {
		t.Fatalf("redirect chain over the limit must be rejected")
	}
}
