package plugin

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/AITNR/cap-token-usage-tracker/internal/plugin/modelcatalog"
)

// testOffPeak is a Saturday noon, so no bundled weekday peak window applies.
var testOffPeak = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// testPeak is a Monday inside the DeepSeek 01:00-04:00 UTC peak window.
var testPeak = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)

// testCatalogTime pins the fixture snapshot timestamp.
var testCatalogTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func priceOf(input, output, cacheRead, cacheWrite float64) ModelPrice {
	price := ModelPrice{}
	if !math.IsNaN(input) {
		value := input
		price.Input = &value
	}
	if !math.IsNaN(output) {
		value := output
		price.Output = &value
	}
	if !math.IsNaN(cacheRead) {
		value := cacheRead
		price.CacheRead = &value
	}
	if !math.IsNaN(cacheWrite) {
		value := cacheWrite
		price.CacheWrite = &value
	}
	return price
}

func testPricingTable(t *testing.T, upstream string, overrides map[string]ModelPrice) *pricingTable {
	t.Helper()
	raw, _, err := modelcatalog.Trim([]byte(upstream), testCatalogTime)
	if err != nil {
		t.Fatalf("trim upstream fixture: %v", err)
	}
	snapshot, err := modelcatalog.Parse(raw)
	if err != nil {
		t.Fatalf("parse trimmed fixture: %v", err)
	}
	service, err := modelcatalog.New(modelcatalog.Options{
		CachePath: filepath.Join(t.TempDir(), "catalog.json"),
		Snapshot:  snapshot,
	})
	if err != nil {
		t.Fatalf("catalog service: %v", err)
	}
	table := newPricingTable(service)
	table.SetOverrides(overrides)
	return table
}

const pricingUpstream = `{
  "google": {"models": {
    "gemini-3.8-flash": {"cost": {"input": 0.75, "output": 3.75, "cache_read": 0.075}},
    "gemini-3.8-flash-free": {"cost": {"input": 0, "output": 0}}
  }},
  "anthropic": {"models": {
    "claude-haiku-5-5": {"cost": {"input": 1, "output": 5, "cache_read": 0.1, "cache_write": 1.25}}
  }},
  "opencode-go": {"api": "https://opencode.ai/zen/go/v1", "models": {
    "deepseek-v4.1-flash": {"cost": {"input": 0.15, "output": 0.6}}
  }}
}`

func TestResolvedPricePrecedenceAndMerge(t *testing.T) {
	table := testPricingTable(t, pricingUpstream, map[string]ModelPrice{
		"deepseek-v4.1-flash": priceOf(math.NaN(), 1.5, math.NaN(), math.NaN()),
		"unlisted-model":      priceOf(0.2, 0.8, math.NaN(), math.NaN()),
	})
	cases := []struct {
		name        string
		dimensions  Dimensions
		wantSource  string
		wantPriced  bool
		wantInput   float64
		wantOutput  float64
		wantCatalog string
	}{
		{
			name:        "override output wins and input is inherited",
			dimensions:  Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"},
			wantSource:  priceSourceOverride,
			wantPriced:  true,
			wantInput:   0.15,
			wantOutput:  1.5,
			wantCatalog: "opencode-go",
		},
		{
			name:       "override only model",
			dimensions: Dimensions{Model: "unlisted-model"},
			wantSource: priceSourceOverride,
			wantPriced: true,
			wantInput:  0.2,
			wantOutput: 0.8,
		},
		{
			name:        "catalog only",
			dimensions:  Dimensions{Provider: "antigravity", Model: "gemini-3.8-flash-high"},
			wantSource:  priceSourceCatalog,
			wantPriced:  true,
			wantInput:   0.75,
			wantOutput:  3.75,
			wantCatalog: "google",
		},
		{
			name:       "unknown model stays unpriced",
			dimensions: Dimensions{Model: "Beam"},
			wantSource: priceSourceNone,
			wantPriced: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resolved := table.PriceFor(testCase.dimensions)
			if resolved.Source != testCase.wantSource || resolved.Priced != testCase.wantPriced {
				t.Fatalf("resolved = %+v", resolved)
			}
			if testCase.wantPriced {
				if priceValue(resolved.Price.Input) != testCase.wantInput || priceValue(resolved.Price.Output) != testCase.wantOutput {
					t.Fatalf("price = %+v, want input %v output %v", resolved.Price, testCase.wantInput, testCase.wantOutput)
				}
			}
			if testCase.wantCatalog != "" && resolved.CatalogProvider != testCase.wantCatalog {
				t.Fatalf("catalog provider = %q, want %q", resolved.CatalogProvider, testCase.wantCatalog)
			}
		})
	}
	if free := table.PriceFor(Dimensions{Model: "gemini-3.8-flash-free"}); !free.Free || free.Source != priceSourceCatalog {
		t.Fatalf("free model = %+v", free)
	}
}

func TestCostFormulaUsesTokenSemantics(t *testing.T) {
	table := testPricingTable(t, pricingUpstream, nil)
	// Anthropic reports cache tokens outside the input total.
	anthropic := Counters{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheCreationTokens: 1_000_000, TotalTokens: 4_000_000}
	cost, priced := table.Cost(Dimensions{Provider: "claude", Model: "claude-haiku-5-5"}, anthropic, testOffPeak)
	if !priced {
		t.Fatalf("anthropic model must be priced")
	}
	// 1M input @1 + 1M output @5 + 1M cache read @0.1 + 1M cache write @1.25
	if math.Abs(cost-7.35) > 1e-9 {
		t.Fatalf("anthropic cost = %v, want 7.35", cost)
	}

	// Google reports cache tokens inside the prompt total and reasoning
	// separately; the cached share must not be billed twice.
	google := Counters{InputTokens: 1_000_000, OutputTokens: 1_000_000, CachedTokens: 400_000, ReasoningTokens: 100_000, TotalTokens: 2_100_000}
	cost, priced = table.Cost(Dimensions{Provider: "antigravity", Model: "gemini-3.8-flash-high"}, google, testOffPeak)
	if !priced {
		t.Fatalf("google model must be priced")
	}
	// 600k uncached @0.75 + 400k cached @0.075 + 1M output @3.75
	want := 600_000*0.75/1e6 + 400_000*0.075/1e6 + 1_000_000*3.75/1e6
	if math.Abs(cost-want) > 1e-9 {
		t.Fatalf("google cost = %v, want %v", cost, want)
	}

	// Cache rates fall back to the input rate when the catalog omits them.
	fallback := testPricingTable(t, pricingUpstream, map[string]ModelPrice{"deepseek-v4.1-flash": priceOf(2, 4, math.NaN(), math.NaN())})
	cost, _ = fallback.Cost(Dimensions{Provider: "openai-compatible-opencode go", Model: "deepseek-v4.1-flash"},
		Counters{InputTokens: 1_000_000, OutputTokens: 0, CachedTokens: 1_000_000}, testOffPeak)
	if math.Abs(cost-2) > 1e-9 {
		t.Fatalf("cache fallback cost = %v, want 2", cost)
	}

	// An unpriced model never reports a cost.
	if amount, priced := table.Cost(Dimensions{Model: "Beam"}, Counters{InputTokens: 1_000_000}, testOffPeak); priced || amount != 0 {
		t.Fatalf("unpriced model returned %v, %v", amount, priced)
	}
}

func TestTokenSemanticsClassification(t *testing.T) {
	cases := []struct {
		provider string
		executor string
		want     tokenSemantics
		known    bool
	}{
		{provider: "openai-compatible-opencode go", want: semanticsSubset, known: true},
		{provider: "openai-compatible-tierflow", want: semanticsSubset, known: true},
		// A channel name that merely contains "gemini" is classified by the
		// host's own rule, which only short-circuits names starting with
		// "openai-compatible-".
		{provider: "cap-opencode-go-gemini", want: semanticsSeparateReasoning, known: true},
		{provider: "codex", want: semanticsSubset, known: true},
		{provider: "claude", want: semanticsIndependent, known: true},
		{provider: "anthropic", want: semanticsIndependent, known: true},
		{provider: "antigravity", want: semanticsSeparateReasoning, known: true},
		{provider: "gemini", want: semanticsSeparateReasoning, known: true},
		{provider: "vertex", want: semanticsSeparateReasoning, known: true},
		{provider: "", executor: "OpenAICompatExecutor", want: semanticsSubset, known: true},
		{provider: "mystery", want: semanticsSubset, known: false},
		{provider: "", want: semanticsSubset, known: false},
	}
	for _, testCase := range cases {
		got, known := tokenSemanticsFor(testCase.provider, testCase.executor)
		if got != testCase.want || known != testCase.known {
			t.Fatalf("tokenSemanticsFor(%q, %q) = %v, %v; want %v, %v", testCase.provider, testCase.executor, got, known, testCase.want, testCase.known)
		}
	}

	// Unknown providers fall back to arithmetic evidence.
	subset := Counters{InputTokens: 100, OutputTokens: 20, CachedTokens: 40, TotalTokens: 120}
	if got := inferTokenSemantics(Dimensions{Provider: "mystery"}, subset); got != semanticsSubset {
		t.Fatalf("arithmetic inference = %v, want subset", got)
	}
	independent := Counters{InputTokens: 100, OutputTokens: 20, CachedTokens: 40, TotalTokens: 160}
	if got := inferTokenSemantics(Dimensions{Provider: "mystery"}, independent); got != semanticsIndependent {
		t.Fatalf("arithmetic inference = %v, want independent", got)
	}
	if got := inferTokenSemantics(Dimensions{Provider: "claude"}, independent); got != semanticsIndependent {
		t.Fatalf("known provider must win over arithmetic evidence")
	}
}

func TestModelThroughputSeparatesGenerationAndEndToEnd(t *testing.T) {
	counters := Counters{OutputTokens: 1000, TotalLatencyNS: 10_000_000_000, TotalTTFTNS: 2_000_000_000, Requests: 1}
	generation, endToEnd := modelThroughput(Dimensions{Model: "gpt-6-luna"}, counters)
	if math.Abs(generation-125) > 1e-9 {
		t.Fatalf("generation TPS = %v, want 125", generation)
	}
	if math.Abs(endToEnd-100) > 1e-9 {
		t.Fatalf("end-to-end TPS = %v, want 100", endToEnd)
	}
	if generation, endToEnd := modelThroughput(Dimensions{}, Counters{}); generation != 0 || endToEnd != 0 {
		t.Fatalf("empty counters must not produce TPS: %v %v", generation, endToEnd)
	}
	// A TTFT larger than the total latency must not produce a negative rate.
	broken := Counters{OutputTokens: 500, TotalLatencyNS: 1_000_000_000, TotalTTFTNS: 5_000_000_000}
	if generation, _ := modelThroughput(Dimensions{}, broken); generation != 0 {
		t.Fatalf("generation TPS = %v, want 0", generation)
	}
}

func TestModelPriceValidation(t *testing.T) {
	if _, err := normalizeModelPrice("m", priceOf(-1, math.NaN(), math.NaN(), math.NaN())); err == nil {
		t.Fatalf("negative price accepted")
	}
	if _, err := normalizeModelPrice("m", priceOf(maxModelPriceValue+1, math.NaN(), math.NaN(), math.NaN())); err == nil {
		t.Fatalf("oversized price accepted")
	}
	if _, err := normalizeModelPrice("m", priceOf(math.Inf(1), math.NaN(), math.NaN(), math.NaN())); err == nil {
		t.Fatalf("infinite price accepted")
	}
	rounded, err := normalizeModelPrice("m", priceOf(0.12345678, math.NaN(), math.NaN(), math.NaN()))
	if err != nil || priceValue(rounded.Input) != 0.123457 {
		t.Fatalf("rounding = %+v, %v", rounded, err)
	}
	if _, err := normalizeOverrideModel("  "); err == nil {
		t.Fatalf("empty model accepted")
	}
	if _, err := normalizeOverrideModel("bad\nmodel"); err == nil {
		t.Fatalf("control characters accepted")
	}
	if _, err := normalizeOverrideModel(string(make([]byte, maxModelPriceModelBytes+1))); err == nil {
		t.Fatalf("oversized model accepted")
	}
	normalized, err := normalizeModelPriceOverrides(map[string]ModelPrice{" empty ": {}, "kept": priceOf(1, 1, math.NaN(), math.NaN())})
	if err != nil || len(normalized) != 1 {
		t.Fatalf("override table normalization = %+v, %v", normalized, err)
	}
}

func TestPricingTableOverrideMutationsClearMemo(t *testing.T) {
	table := testPricingTable(t, pricingUpstream, nil)
	dimensions := Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"}
	before := table.PriceFor(dimensions)
	if before.Source != priceSourceCatalog {
		t.Fatalf("initial source = %s", before.Source)
	}
	table.SetOverrides(map[string]ModelPrice{"deepseek-v4.1-flash": priceOf(9, 9, math.NaN(), math.NaN())})
	after := table.PriceFor(dimensions)
	if after.Source != priceSourceOverride || priceValue(after.Price.Input) != 9 {
		t.Fatalf("override not applied: %+v", after)
	}
	if table.OverrideCount() != 1 {
		t.Fatalf("override count = %d", table.OverrideCount())
	}
	table.SetOverrides(nil)
	if table.OverrideCount() != 0 || table.PriceFor(dimensions).Source != priceSourceCatalog {
		t.Fatalf("overrides not cleared")
	}
}
