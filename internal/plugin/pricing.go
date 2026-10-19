package plugin

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/AITNR/cap-token-usage-tracker/internal/plugin/modelcatalog"
)

const (
	// maxModelPriceValue bounds one manually entered rate (USD per 1M tokens).
	maxModelPriceValue = 1_000_000
	// maxModelPriceOverrides bounds the stored override table.
	maxModelPriceOverrides = 10_000
	// maxModelPriceModelBytes bounds one overridden model id.
	maxModelPriceModelBytes = 200
	// priceDecimals is the stored precision of a manual rate.
	priceDecimals = 6
)

// ModelPrice is a list price in USD per 1,000,000 tokens. Missing fields mean
// "inherit": for a stored override they fall back to the catalog rate, and for
// an effective price they mean the rate is unknown.
//
// TimeZone and TimeTiers optionally add a weekly time-of-use schedule. A
// matched tier overrides the four base rates field by field, so a tier can be
// a peak surcharge or an off-peak discount.
type ModelPrice struct {
	Input      *float64        `json:"input,omitempty"`
	Output     *float64        `json:"output,omitempty"`
	CacheRead  *float64        `json:"cache_read,omitempty"`
	CacheWrite *float64        `json:"cache_write,omitempty"`
	TimeZone   string          `json:"time_zone,omitempty"`
	TimeTiers  []TimePriceTier `json:"time_tiers,omitempty"`
}

// Priced reports whether the price carries a usable rate. Only input or output
// count, mirroring the upstream chain: a record with cache rates alone cannot
// produce a cost.
func (p ModelPrice) Priced() bool {
	return p.Input != nil || p.Output != nil
}

// Free reports whether the price is an explicit zero price.
func (p ModelPrice) Free() bool {
	return p.Priced() && priceValue(p.Input) == 0 && priceValue(p.Output) == 0
}

// Empty reports whether no rate is set.
func (p ModelPrice) Empty() bool {
	return p.Input == nil && p.Output == nil && p.CacheRead == nil && p.CacheWrite == nil && len(p.TimeTiers) == 0
}

// merge applies the non-nil fields of other on top of p. A time schedule is
// replaced as a whole because a partial schedule has no meaning.
func (p ModelPrice) merge(other ModelPrice) ModelPrice {
	merged := p
	if other.Input != nil {
		merged.Input = other.Input
	}
	if other.Output != nil {
		merged.Output = other.Output
	}
	if other.CacheRead != nil {
		merged.CacheRead = other.CacheRead
	}
	if other.CacheWrite != nil {
		merged.CacheWrite = other.CacheWrite
	}
	if len(other.TimeTiers) > 0 {
		merged.TimeZone = other.TimeZone
		merged.TimeTiers = cloneTimeTiers(other.TimeTiers)
	}
	return merged
}

func priceValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func priceFromCatalog(price modelcatalog.Price) ModelPrice {
	return ModelPrice{Input: price.Input, Output: price.Output, CacheRead: price.CacheRead, CacheWrite: price.CacheWrite}
}

// normalizeModelPrice validates and rounds one manually entered price.
func normalizeModelPrice(model string, raw ModelPrice) (ModelPrice, error) {
	normalized := ModelPrice{}
	for _, field := range []struct {
		name   string
		value  *float64
		target **float64
	}{
		{name: "input", value: raw.Input, target: &normalized.Input},
		{name: "output", value: raw.Output, target: &normalized.Output},
		{name: "cache_read", value: raw.CacheRead, target: &normalized.CacheRead},
		{name: "cache_write", value: raw.CacheWrite, target: &normalized.CacheWrite},
	} {
		if field.value == nil {
			continue
		}
		rounded, err := normalizePriceValue(*field.value)
		if err != nil {
			return ModelPrice{}, fmt.Errorf("%s: %w", field.name, err)
		}
		pointer := rounded
		*field.target = &pointer
	}
	normalized.TimeZone = raw.TimeZone
	normalized.TimeTiers = raw.TimeTiers
	if err := normalizeTimePricing(model, &normalized); err != nil {
		return ModelPrice{}, err
	}
	return normalized, nil
}

func normalizePriceValue(value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("price must be a finite number")
	}
	if value < 0 {
		return 0, fmt.Errorf("price must not be negative")
	}
	if value > maxModelPriceValue {
		return 0, fmt.Errorf("price must not exceed %d", maxModelPriceValue)
	}
	scale := math.Pow(10, priceDecimals)
	return math.Round(value*scale) / scale, nil
}

// normalizeOverrideModel validates an override key.
func normalizeOverrideModel(model string) (string, error) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", fmt.Errorf("model must not be empty")
	}
	if len(trimmed) > maxModelPriceModelBytes {
		return "", fmt.Errorf("model must not exceed %d bytes", maxModelPriceModelBytes)
	}
	if strings.ContainsAny(trimmed, "\x00\n\r") {
		return "", fmt.Errorf("model must not contain control characters")
	}
	return trimmed, nil
}

// normalizeModelPriceOverrides validates a whole override table, typically read
// back from storage.
func normalizeModelPriceOverrides(raw map[string]ModelPrice) (map[string]ModelPrice, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxModelPriceOverrides {
		return nil, fmt.Errorf("model price overrides exceed %d entries", maxModelPriceOverrides)
	}
	normalized := make(map[string]ModelPrice, len(raw))
	for model, price := range raw {
		key, err := normalizeOverrideModel(model)
		if err != nil {
			return nil, err
		}
		value, err := normalizeModelPrice(key, price)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		if value.Empty() {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

func cloneModelPriceOverrides(raw map[string]ModelPrice) map[string]ModelPrice {
	if len(raw) == 0 {
		return nil
	}
	cloned := make(map[string]ModelPrice, len(raw))
	for model, price := range raw {
		price.TimeTiers = cloneTimeTiers(price.TimeTiers)
		cloned[model] = price
	}
	return cloned
}

// resolvedPrice is the effective price of one model together with the evidence
// behind it. It is embedded in API responses. TimeZone/TimeTiers describe the
// schedule the cost formula applies; TimePlanSource says where it came from so
// the dashboard can label a bundled plan.
type resolvedPrice struct {
	Price           ModelPrice      `json:"price,omitzero"`
	Priced          bool            `json:"priced"`
	Free            bool            `json:"free"`
	Source          string          `json:"price_source"`
	Override        ModelPrice      `json:"override,omitzero"`
	CatalogProvider string          `json:"catalog_provider,omitempty"`
	CatalogModel    string          `json:"catalog_model,omitempty"`
	CatalogMatch    string          `json:"catalog_match,omitempty"`
	TimeZone        string          `json:"time_zone,omitempty"`
	TimeTiers       []TimePriceTier `json:"time_tiers,omitempty"`
	TimePlanSource  string          `json:"time_plan_source,omitempty"`
}

// Price source labels.
const (
	priceSourceNone     = "none"
	priceSourceCatalog  = "catalog"
	priceSourceOverride = "override"
	priceSourceBuiltin  = "builtin"
)

// tokenSemantics describes how an upstream protocol reports cache tokens
// relative to the prompt total.
type tokenSemantics uint8

const (
	// semanticsSubset: cache tokens are included in the input total.
	semanticsSubset tokenSemantics = iota
	// semanticsIndependent: cache tokens are separate from the input total.
	semanticsIndependent
	// semanticsSeparateReasoning: cache included in input, reasoning separate
	// from output.
	semanticsSeparateReasoning
)

// tokenSemanticsFor mirrors CLIProxyAPI's provider/executor classification so
// the cost formula reads the same buckets the host normalized.
func tokenSemanticsFor(provider, executorType string) (tokenSemantics, bool) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	normalizedExecutor := strings.ToLower(strings.TrimSpace(executorType))
	value := strings.TrimSpace(normalizedProvider + " " + normalizedExecutor)
	if value == "" || value == "unknown" || value == "unknown unknown" {
		return semanticsSubset, false
	}
	if normalizedExecutor == "openaicompatexecutor" || normalizedProvider == "openai-compatibility" || strings.HasPrefix(normalizedProvider, "openai-compatible-") {
		return semanticsSubset, true
	}
	if strings.Contains(value, "claude") || strings.Contains(value, "anthropic") {
		return semanticsIndependent, true
	}
	for _, marker := range []string{"gemini", "aistudio", "antigravity", "vertex", "interaction"} {
		if strings.Contains(value, marker) {
			return semanticsSeparateReasoning, true
		}
	}
	for _, marker := range []string{"openai", "codex", "xai", "grok", "kimi", "qwen", "deepseek", "openrouter"} {
		if strings.Contains(value, marker) {
			return semanticsSubset, true
		}
	}
	return semanticsSubset, false
}

// inferTokenSemantics classifies a group from its provider/executor, falling
// back to arithmetic evidence from the token totals when the provider is not
// recognized.
func inferTokenSemantics(dimensions Dimensions, counters Counters) tokenSemantics {
	if semantics, known := tokenSemanticsFor(dimensions.Provider, dimensions.ExecutorType); known {
		return semantics
	}
	total := counters.TotalTokens
	if total == 0 {
		return semanticsSubset
	}
	subset := saturatingAdd(saturatingAdd(counters.InputTokens, counters.OutputTokens), counters.ReasoningTokens)
	independent := saturatingAdd(saturatingAdd(counters.InputTokens, cacheReadTokens(counters)), counters.CacheCreationTokens)
	independent = saturatingAdd(saturatingAdd(independent, counters.OutputTokens), counters.ReasoningTokens)
	switch total {
	case subset:
		return semanticsSubset
	case independent:
		return semanticsIndependent
	}
	return semanticsSubset
}

// cacheReadTokens prefers the dedicated cache-read bucket and falls back to the
// legacy total-cached counter.
func cacheReadTokens(counters Counters) uint64 {
	if counters.CacheReadTokens > counters.CachedTokens {
		return counters.CacheReadTokens
	}
	return counters.CachedTokens
}

// costForCounters applies the list price to one aggregated group.
func costForCounters(price ModelPrice, semantics tokenSemantics, counters Counters) float64 {
	inputPrice := priceValue(price.Input)
	outputPrice := priceValue(price.Output)
	cacheReadPrice := priceValue(price.CacheRead)
	if price.CacheRead == nil {
		cacheReadPrice = inputPrice
	}
	cacheWritePrice := priceValue(price.CacheWrite)
	if price.CacheWrite == nil {
		cacheWritePrice = inputPrice
	}
	cacheRead := cacheReadTokens(counters)
	cacheWrite := counters.CacheCreationTokens
	uncached := counters.InputTokens
	if semantics != semanticsIndependent {
		cached := saturatingAdd(cacheRead, cacheWrite)
		if cached >= uncached {
			uncached = 0
		} else {
			uncached -= cached
		}
	}
	amount := (float64(uncached)*inputPrice +
		float64(cacheRead)*cacheReadPrice +
		float64(cacheWrite)*cacheWritePrice +
		float64(counters.OutputTokens)*outputPrice) / 1_000_000
	if math.IsNaN(amount) || amount < 0 {
		return 0
	}
	return amount
}

// modelThroughput derives both TPS variants from aggregated counters. The
// generation variant excludes time to first token; the end-to-end variant does
// not.
func modelThroughput(dimensions Dimensions, counters Counters) (float64, float64) {
	numerator := effectiveOutputTokensForTPS(dimensions, counters, false)
	if numerator == 0 {
		return 0, 0
	}
	generationNS := counters.TotalLatencyNS
	if counters.TotalTTFTNS < generationNS {
		generationNS -= counters.TotalTTFTNS
	} else {
		generationNS = 0
	}
	generationTPS := 0.0
	if generationNS > 0 {
		generationTPS = float64(numerator) / (float64(generationNS) / float64(time.Second))
	}
	endToEndTPS := 0.0
	if counters.TotalLatencyNS > 0 {
		endToEndTPS = float64(numerator) / (float64(counters.TotalLatencyNS) / float64(time.Second))
	}
	return generationTPS, endToEndTPS
}

type priceMemoKey struct {
	source   string
	provider string
	model    string
}

// pricingTable resolves effective prices for aggregated groups. It owns the
// in-memory override table so aggregation never re-enters the store actor.
type pricingTable struct {
	catalog *modelcatalog.Service

	mu        sync.RWMutex
	overrides map[string]ModelPrice
	memo      map[priceMemoKey]resolvedPrice
	// builtinTimePlans enables the bundled vendor time-of-use schedules for
	// models that carry no manual schedule of their own.
	builtinTimePlans bool
}

func newPricingTable(catalog *modelcatalog.Service) *pricingTable {
	return &pricingTable{catalog: catalog, memo: map[priceMemoKey]resolvedPrice{}, builtinTimePlans: true}
}

// SetOverrides replaces the whole override table.
func (t *pricingTable) SetOverrides(overrides map[string]ModelPrice) {
	t.mu.Lock()
	t.overrides = cloneModelPriceOverrides(overrides)
	t.memo = map[priceMemoKey]resolvedPrice{}
	t.mu.Unlock()
}

// SetBuiltinTimePlans toggles the bundled schedules. The effective schedule is
// part of a resolved price, so the memo has to be dropped.
func (t *pricingTable) SetBuiltinTimePlans(enabled bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.builtinTimePlans = enabled
	t.memo = map[priceMemoKey]resolvedPrice{}
	t.mu.Unlock()
}

// Overrides returns a copy of the override table.
func (t *pricingTable) Overrides() map[string]ModelPrice {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return cloneModelPriceOverrides(t.overrides)
}

// OverrideCount returns the number of stored overrides.
func (t *pricingTable) OverrideCount() int {
	if t == nil {
		return 0
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.overrides)
}

// PriceFor resolves the effective price of one model.
func (t *pricingTable) PriceFor(dimensions Dimensions) resolvedPrice {
	if t == nil {
		return resolvedPrice{Source: priceSourceNone}
	}
	key := priceMemoKey{source: dimensions.Source, provider: dimensions.Provider, model: dimensions.Model}
	t.mu.RLock()
	if cached, ok := t.memo[key]; ok {
		t.mu.RUnlock()
		return cached
	}
	override, hasOverride := t.overrides[dimensions.Model]
	t.mu.RUnlock()

	resolved := resolvedPrice{Source: priceSourceNone}
	if hasOverride {
		resolved.Override = override
	}
	if t.catalog != nil {
		if _, index := t.catalog.Snapshot(); index != nil {
			if match, ok := index.Lookup(dimensions.Source, dimensions.Provider, dimensions.Model); ok {
				resolved.CatalogProvider = match.ProviderID
				resolved.CatalogModel = match.ModelID
				resolved.CatalogMatch = match.Kind
				resolved.Price = priceFromCatalog(match.Model.Cost)
			}
		}
	}
	if hasOverride {
		resolved.Price = resolved.Price.merge(override)
		resolved.Source = priceSourceOverride
	} else if resolved.Price.Priced() {
		resolved.Source = priceSourceCatalog
	}
	resolved.Priced = resolved.Price.Priced()
	resolved.Free = resolved.Price.Free()
	if !resolved.Priced {
		resolved.Source = priceSourceNone
	}

	// The schedule lives beside the rates, never inside them: a manual schedule
	// wins, otherwise the bundled vendor plan applies. A manual price that only
	// edits rates therefore keeps the bundled schedule.
	resolved.Price.TimeZone = ""
	resolved.Price.TimeTiers = nil
	if hasOverride && len(override.TimeTiers) > 0 {
		resolved.TimeZone = override.TimeZone
		resolved.TimeTiers = cloneTimeTiers(override.TimeTiers)
		resolved.TimePlanSource = priceSourceOverride
	} else {
		t.mu.RLock()
		builtinEnabled := t.builtinTimePlans
		t.mu.RUnlock()
		if builtinEnabled {
			if plan, ok := builtinTimePlanFor(resolved.CatalogProvider, dimensions.Model); ok {
				resolved.TimeZone = plan.TimeZone
				resolved.TimeTiers = cloneTimeTiers(plan.TimeTiers)
				resolved.TimePlanSource = priceSourceBuiltin
			}
		}
	}

	t.mu.Lock()
	if len(t.memo) >= 100_000 {
		t.memo = map[priceMemoKey]resolvedPrice{}
	}
	t.memo[key] = resolved
	t.mu.Unlock()
	return resolved
}

// chargeFor applies a resolved price to one aggregated group at the request
// time and reports the signed delta contributed by an active time tier.
// Amounts are USD; surcharge is negative for an off-peak discount.
func chargeFor(resolved resolvedPrice, dimensions Dimensions, counters Counters, at time.Time) (amount, surcharge float64, priced bool) {
	if !resolved.Priced {
		return 0, 0, false
	}
	semantics := inferTokenSemantics(dimensions, counters)
	base := costForCounters(resolved.Price, semantics, counters)
	tier, ok := selectTimeTier(resolved.TimeZone, resolved.TimeTiers, at)
	if !ok {
		return base, 0, true
	}
	amount = costForCounters(resolved.Price.merge(timeTierRates(tier)), semantics, counters)
	return amount, amount - base, true
}

// Cost returns the estimated cost of one aggregated group at the given request
// time.
func (t *pricingTable) Cost(dimensions Dimensions, counters Counters, at time.Time) (float64, bool) {
	amount, _, priced := chargeFor(t.PriceFor(dimensions), dimensions, counters, at)
	return amount, priced
}

// Status exposes the catalog status for API responses.
func (t *pricingTable) Status() modelcatalog.Status {
	if t == nil || t.catalog == nil {
		return modelcatalog.Status{Status: modelcatalog.StatusMissing, Source: modelcatalog.SourceEmbedded}
	}
	return t.catalog.Status()
}

// NeedsRefresh reports whether the catalog should be refreshed.
func (t *pricingTable) NeedsRefresh() bool {
	if t == nil || t.catalog == nil {
		return false
	}
	return t.catalog.NeedsRefresh()
}

// RefreshCatalog downloads a new catalog snapshot.
func (t *pricingTable) RefreshCatalog(ctx context.Context) (modelcatalog.Status, error) {
	if t == nil || t.catalog == nil {
		return modelcatalog.Status{Status: modelcatalog.StatusMissing}, fmt.Errorf("model catalog is unavailable")
	}
	return t.catalog.Refresh(ctx)
}
