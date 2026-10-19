package modelcatalog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func floatPtr(value float64) *float64 { return &value }

func TestTrimRemapsAndSkipsMalformedEntries(t *testing.T) {
	upstream := []byte(`{
	  "openai": {
	    "id": "openai",
	    "name": "OpenAI",
	    "api": "https://api.openai.com/v1",
	    "models": {
	      "gpt-6-luna": {"id": "gpt-6-luna", "name": "GPT-6 Luna", "family": "gpt", "cost": {"input": 0.1, "output": 0.5, "cache_read": 0.01, "cache_write": 0.125, "tiers": [{"input": 1}]}},
	      "no-cost": {"id": "no-cost", "name": "No Cost"},
	      "broken": 7,
	      "unnamed": {"name": "Unnamed"}
	    }
	  },
	  "broken-provider": 12,
	  "empty-provider": {"id": "empty-provider", "models": {}},
	  "nan": {"id": "nan", "name": "NaN", "models": {"mimo": {"id": "mimo", "cost": {"input": 0, "output": 0}}}}
	}`)
	trimmed, snapshot, err := Trim(upstream, time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Trim returned error: %v", err)
	}
	if _, ok := snapshot.Providers["broken-provider"]; ok {
		t.Fatalf("malformed provider survived trimming")
	}
	if _, ok := snapshot.Providers["empty-provider"]; ok {
		t.Fatalf("provider without models survived trimming")
	}
	openai := snapshot.Providers["openai"]
	if openai.Name != "OpenAI" || openai.API != "https://api.openai.com/v1" {
		t.Fatalf("provider metadata lost: %+v", openai)
	}
	if _, ok := openai.Models["unnamed"]; !ok {
		t.Fatalf("model without id should fall back to its map key")
	}
	if _, ok := openai.Models["broken"]; ok {
		t.Fatalf("malformed model survived trimming")
	}
	luna := openai.Models["gpt-6-luna"]
	if luna.Family != "gpt" || luna.Cost.Input == nil || *luna.Cost.Input != 0.1 || luna.Cost.CacheWrite == nil || *luna.Cost.CacheWrite != 0.125 {
		t.Fatalf("cost mapping lost: %+v", luna)
	}
	if noCost := openai.Models["no-cost"]; noCost.Cost.Priced() {
		t.Fatalf("model without cost must stay unpriced: %+v", noCost)
	}
	if !snapshot.Providers["nan"].Models["mimo"].Cost.Free() {
		t.Fatalf("zero cost must be detected as free")
	}

	// The trimmed document must round-trip through Parse, and must not carry
	// the dropped upstream fields.
	if strings.Contains(string(trimmed), "tiers") || strings.Contains(string(trimmed), "GPT-6 Luna") {
		t.Fatalf("trimmed document kept unused upstream fields: %s", string(trimmed))
	}
	parsed, err := Parse(trimmed)
	if err != nil {
		t.Fatalf("Parse(trimmed) failed: %v", err)
	}
	if parsed.GeneratedAt.IsZero() || !parsed.GeneratedAt.Equal(snapshot.GeneratedAt) {
		t.Fatalf("generatedAt lost: %v", parsed.GeneratedAt)
	}
	stats := parsed.ComputeStats()
	if stats.Providers != 2 || stats.Models != 4 || stats.Priced != 2 || stats.Free != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestParseRejectsUnsupportedSchemaAndEmptyCatalogs(t *testing.T) {
	cases := map[string]string{
		"schema":    `{"schemaVersion":2,"providers":{"a":{"models":{"m":{}}}}}`,
		"missing":   `{"providers":{}}`,
		"no-models": `{"schemaVersion":1,"providers":{"a":{"models":{}}}}`,
		"garbage":   `not json`,
		"unknown":   `{"schemaVersion":1,"providers":{},"extra":true}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(raw)); err == nil {
				t.Fatalf("Parse(%s) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestParseToleratesMissingGeneratedAt(t *testing.T) {
	snapshot, err := Parse([]byte(`{"schemaVersion":1,"providers":{"a":{"models":{"m":{"c":{"i":1}}}}}}`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !snapshot.GeneratedAt.IsZero() {
		t.Fatalf("missing generatedAt should decode to the zero time, got %v", snapshot.GeneratedAt)
	}
	if !snapshot.Valid() {
		t.Fatalf("snapshot with one model must be valid")
	}
}

func TestEmbeddedBaselineCoversTheCatalog(t *testing.T) {
	snapshot, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded failed: %v", err)
	}
	stats := snapshot.ComputeStats()
	if stats.Providers < 200 || stats.Models < 8000 || stats.Priced < 7000 {
		t.Fatalf("embedded snapshot looks truncated: %+v", stats)
	}
	if snapshot.GeneratedAt.IsZero() {
		t.Fatalf("embedded snapshot has no generatedAt")
	}
	encoded, err := json.Marshal(snapshot.Providers["openai"])
	if err != nil {
		t.Fatalf("marshal provider: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatalf("openai provider missing from embedded snapshot")
	}
}

func TestPriceHelpers(t *testing.T) {
	full := Price{Input: floatPtr(1), Output: floatPtr(2), CacheRead: floatPtr(0.1), CacheWrite: floatPtr(1.25)}
	if !full.Priced() || !full.Complete() || full.Free() {
		t.Fatalf("unexpected helpers for %+v", full)
	}
	cacheOnly := Price{CacheRead: floatPtr(0.1)}
	if cacheOnly.Priced() {
		t.Fatalf("cache-only price must not count as priced")
	}
	zero := Price{Input: floatPtr(0), Output: floatPtr(0)}
	if !zero.Priced() || !zero.Free() {
		t.Fatalf("explicit zero price must be free and priced")
	}
}
