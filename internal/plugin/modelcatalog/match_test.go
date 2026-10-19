package modelcatalog

import (
	"testing"
)

func fixtureSnapshot() *Snapshot {
	providers := map[string]Provider{
		"openai": {
			Models: map[string]Model{
				"gpt-6-luna": {Family: "gpt", Cost: Price{Input: floatPtr(0.1), Output: floatPtr(0.5), CacheRead: floatPtr(0.01), CacheWrite: floatPtr(0.125)}},
			},
		},
		"google": {
			Models: map[string]Model{
				"gemini-3.8-flash":       {Family: "gemini-flash", Cost: Price{Input: floatPtr(0.75), Output: floatPtr(3.75), CacheRead: floatPtr(0.075)}},
				"gemini-4-argon":         {Family: "gemini", Cost: Price{Input: floatPtr(2), Output: floatPtr(10)}},
				"gemini-3.1-flash-image": {Family: "gemini", Cost: Price{Input: floatPtr(0.5), Output: floatPtr(60)}},
			},
		},
		"opencode": {
			API: "https://opencode.ai/inference/openai/v1",
			Models: map[string]Model{
				"deepseek-v4.1-flash": {Cost: Price{Input: floatPtr(0.3), Output: floatPtr(1.2)}},
				"gemini-4-argon":      {Cost: Price{Input: floatPtr(2), Output: floatPtr(10)}},
				"cat-jam-free":        {Cost: Price{Input: floatPtr(0), Output: floatPtr(0), CacheRead: floatPtr(0)}},
			},
		},
		"opencode-go": {
			API: "https://opencode.ai/inference/go/openai/v1",
			Models: map[string]Model{
				"deepseek-v4.1-flash":        {Cost: Price{Input: floatPtr(0.15), Output: floatPtr(0.6), CacheRead: floatPtr(0.003)}},
				"gpt-6-luna":                 {Cost: Price{Input: floatPtr(0.1), Output: floatPtr(0.5)}},
				"cat-jam-free":               {Cost: Price{Input: floatPtr(0), Output: floatPtr(0), CacheRead: floatPtr(0)}},
				"gemini-4-argon-contributor": {Cost: Price{Input: floatPtr(4), Output: floatPtr(20)}},
			},
		},
		"alibaba-cn": {
			Models: map[string]Model{
				"deepseek-v4.1-flash": {Cost: Price{Input: floatPtr(0.29754), Output: floatPtr(1.19015)}},
			},
		},
		"nan": {
			API: "https://api.nan.builders/v1",
			Models: map[string]Model{
				"mimo-v2.6-flash": {Cost: Price{Input: floatPtr(0), Output: floatPtr(0)}},
			},
		},
		"xiaomi": {
			API: "https://api.xiaomimimo.com/v1",
			Models: map[string]Model{
				"mimo-v2.6-flash": {Family: "mimo", Cost: Price{Input: floatPtr(0.14), Output: floatPtr(0.28), CacheRead: floatPtr(0.0028)}},
			},
		},
		"deepseek": {
			API: "https://api.deepseek.com",
			Models: map[string]Model{
				"deepseek-flash": {Cost: Price{Input: floatPtr(0.15), Output: floatPtr(0.6)}},
			},
		},
	}
	return &Snapshot{SchemaVersion: SchemaVersion, Providers: providers}
}

func TestNormalizeModelID(t *testing.T) {
	cases := map[string]string{
		"  GPT-6-Luna ":             "gpt-6-luna",
		"deepseek-v4.1-flash:free":  "deepseek-v4.1-flash",
		"claude-haiku-5-5@20260101": "claude-haiku-5-5",
		"qwen3.8-flash@latest":      "qwen3.8-flash@latest",
		"glm-5.3":                   "glm-5.3",
	}
	for input, want := range cases {
		if got := NormalizeModelID(input); got != want {
			t.Fatalf("NormalizeModelID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestChannelHintsPreferLongestTokenRun(t *testing.T) {
	index := NewIndex(fixtureSnapshot())
	cases := map[string][]string{
		"openai-compatible-opencode go": {"opencode-go", "opencode"},
		"cap-opencode-go-gemini":        {"opencode-go", "opencode"},
		"antigravity":                   nil,
		"openai-compatible-基元律动":        nil,
		"cap-opencode-go-responses":     {"opencode-go", "opencode"},
	}
	for channel, want := range cases {
		got := index.channelHints(channel)
		if len(got) != len(want) {
			t.Fatalf("channelHints(%q) = %v, want %v", channel, got, want)
		}
		for position := range want {
			if got[position] != want[position] {
				t.Fatalf("channelHints(%q) = %v, want %v", channel, got, want)
			}
		}
	}
}

func TestSourceHintsPreferLongestProviderOnSharedHost(t *testing.T) {
	index := NewIndex(fixtureSnapshot())
	got := index.sourceHints("https://opencode.ai/zen/go/v1")
	if len(got) < 2 || got[0] != "opencode-go" || got[1] != "opencode" {
		t.Fatalf("sourceHints shared host = %v, want [opencode-go opencode ...]", got)
	}
	if hints := index.sourceHints("https://api.openai.com/v1"); len(hints) != 1 || hints[0] != "openai" {
		t.Fatalf("exact host table not applied: %v", hints)
	}
	if hints := index.sourceHints("https://api.xiaomimimo.com/v1"); len(hints) != 1 || hints[0] != "xiaomi" {
		t.Fatalf("catalog api host not applied: %v", hints)
	}
	if hints := index.sourceHints("https://tierflow.cn/v1"); len(hints) != 0 {
		t.Fatalf("unknown host produced hints: %v", hints)
	}
}

func TestLookupLadderAndComparator(t *testing.T) {
	index := NewIndex(fixtureSnapshot())
	cases := []struct {
		name     string
		source   string
		provider string
		model    string
		wantID   string
		wantKind string
	}{
		{name: "channel hint wins over priority list", provider: "openai-compatible-opencode go", model: "deepseek-v4.1-flash", wantID: "opencode-go", wantKind: MatchHint},
		{name: "channel hint with cap prefix", provider: "cap-opencode-go", model: "DeepSeek-V4.1-Flash", wantID: "opencode-go", wantKind: MatchHint},
		{name: "hint falls through to shorter provider", provider: "cap-opencode-go-gemini", model: "gemini-4-argon", wantID: "opencode", wantKind: MatchHint},
		{name: "source url hint", source: "https://opencode.ai/zen/go/v1", provider: "unlabelled", model: "gpt-6-luna", wantID: "opencode-go", wantKind: MatchHint},
		{name: "zero price candidates are filtered", provider: "openai-compatible-mimo", model: "mimo-v2.6-flash", wantID: "xiaomi", wantKind: MatchGlobal},
		{name: "effort suffix fallback", provider: "antigravity", model: "gemini-3.8-flash-high", wantID: "google", wantKind: MatchVariant},
		{name: "vendor slash prefix", provider: "", model: "openai/gpt-6-luna", wantID: "openai", wantKind: MatchVendor},
		{name: "global fallback by priority", provider: "codex", model: "gpt-6-luna", wantID: "openai", wantKind: MatchGlobal},
		{name: "free model stays free", provider: "cap-opencode-go", model: "cat-jam-free", wantID: "opencode-go", wantKind: MatchHint},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			match, ok := index.Lookup(testCase.source, testCase.provider, testCase.model)
			if !ok {
				t.Fatalf("Lookup(%q, %q) found nothing", testCase.provider, testCase.model)
			}
			if match.ProviderID != testCase.wantID || match.Kind != testCase.wantKind {
				t.Fatalf("Lookup(%q, %q) = %s/%s (%s), want %s/%s",
					testCase.provider, testCase.model, match.ProviderID, match.ModelID, match.Kind, testCase.wantID, testCase.wantKind)
			}
		})
	}
	if _, ok := index.Lookup("", "cap-opencode-go", "Beam"); ok {
		t.Fatalf("unknown model must not resolve")
	}
	if _, ok := index.Lookup("", "", ""); ok {
		t.Fatalf("empty model must not resolve")
	}
}

func TestLookupIsDeterministic(t *testing.T) {
	index := NewIndex(fixtureSnapshot())
	first, ok := index.Lookup("https://opencode.ai/zen/go/v1", "openai-compatible-opencode go", "deepseek-v4.1-flash")
	if !ok {
		t.Fatalf("lookup failed")
	}
	for attempt := 0; attempt < 50; attempt++ {
		again, ok := index.Lookup("https://opencode.ai/zen/go/v1", "openai-compatible-opencode go", "deepseek-v4.1-flash")
		if !ok || again.ProviderID != first.ProviderID || again.ModelID != first.ModelID {
			t.Fatalf("lookup is not deterministic: %+v vs %+v", first, again)
		}
	}
}

// TestEmbeddedCatalogResolvesProductionModels documents the resolution of the
// models actually metered in production. Prices are not asserted (the upstream
// catalog changes); the resolved provider is.
func TestEmbeddedCatalogResolvesProductionModels(t *testing.T) {
	snapshot, err := Embedded()
	if err != nil {
		t.Fatalf("Embedded failed: %v", err)
	}
	index := NewIndex(snapshot)
	cases := []struct {
		channel string
		source  string
		model   string
		want    string
	}{
		{channel: "antigravity", model: "gemini-3.8-flash-high", want: "google"},
		{channel: "openai-compatible-opencode go", source: "https://opencode.ai/zen/go/v1", model: "deepseek-v4.1-flash", want: "opencode-go"},
		{channel: "cap-opencode-go", model: "DeepSeek-V4.1-Flash", want: "opencode-go"},
		{channel: "cap-opencode-go-responses", model: "gpt-6-luna", want: "opencode-go"},
		{channel: "cap-opencode-go-gemini", model: "gemini-4-argon", want: "opencode"},
		{channel: "cap-opencode-go", model: "glm-5.3", want: "opencode-go"},
		{channel: "cap-opencode-go", model: "cat-jam-free", want: "opencode-go"},
		{channel: "codex", model: "gpt-6-luna", want: "openai"},
		{channel: "openai-compatible-tierflow", model: "Qwen3.8-Flash", want: "alibaba"},
	}
	for _, testCase := range cases {
		match, ok := index.Lookup(testCase.source, testCase.channel, testCase.model)
		if !ok {
			t.Fatalf("embedded catalog cannot resolve %s (%s)", testCase.model, testCase.channel)
		}
		if match.ProviderID != testCase.want {
			t.Fatalf("embedded catalog resolved %s (%s) to %s, want %s", testCase.model, testCase.channel, match.ProviderID, testCase.want)
		}
		if !match.Model.Cost.Priced() {
			t.Fatalf("resolved %s without a price", testCase.model)
		}
	}
	// Channel-specific aliases have no catalog entry: the dashboard must show
	// them as unpriced so the operator can set a manual price.
	for _, model := range []string{"Beam", "CatJam", "tierflow_pro", "grok", "gemini-pro-agent"} {
		if match, ok := index.Lookup("", "cap-opencode-go", model); ok {
			t.Fatalf("alias %s unexpectedly resolved to %s/%s", model, match.ProviderID, match.ModelID)
		}
	}
}
