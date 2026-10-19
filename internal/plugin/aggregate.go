package plugin

import (
	"cmp"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// legacyUntitledModelName is retained only to accept old API filters; it is
// not emitted in new statistics responses.
const legacyUntitledModelName = "未标记模型"

type Dimensions struct {
	Provider         string `json:"provider"`
	ExecutorType     string `json:"executor_type"`
	Model            string `json:"model"`
	Alias            string `json:"alias"`
	Source           string `json:"source"`
	APIKey           string `json:"api_key,omitempty"`
	APIKeyHash       string `json:"api_key_hash,omitempty"`
	APIKeyGeneration uint64 `json:"api_key_generation,omitempty"`
	APIKeyRef        string `json:"api_key_ref,omitempty"`
	APIKeyStatus     string `json:"api_key_status,omitempty"`
	AuthType         string `json:"auth_type"`
	ServiceTier      string `json:"service_tier"`
	ReasoningEffort  string `json:"reasoning_effort"`
	Failed           bool   `json:"failed"`
	FailureStatus    int    `json:"failure_status"`
}

// groupIdentity strips failure-only fields so rows that differ only by
// Failed/FailureStatus merge into one dimension row. Failure counts stay
// visible through Counters.FailedRequests, and per-request failure details
// remain available on the /requests endpoint.
func groupIdentity(dimensions Dimensions) Dimensions {
	dimensions.Failed = false
	dimensions.FailureStatus = 0
	return dimensions
}

// usageFilter scopes every analytics surface to the same persisted dimensions.
// Empty fields intentionally mean no restriction, which preserves legacy callers.
type usageFilter struct {
	Source           string
	APIKeyHash       string
	APIKeyGeneration uint64
	APIKeyRefSet     string
	Model            string
	// Models is the multi-select form of Model used by the dashboard. An empty
	// slice leaves every model included; otherwise a row matches when any entry
	// matches. Model and Models are both honoured so legacy single-model callers
	// keep working.
	Models []string
}

func newUsageFilter(source, apiKeyIdentity string) usageFilter {
	if apiKeyIdentity == "" {
		return usageFilter{Source: normalizeDimension(source)}
	}
	return newUsageFilterFromIdentities(source, []string{apiKeyIdentity})
}

func newUsageFilterFromIdentities(source string, identities []string) usageFilter {
	filter := usageFilter{Source: normalizeDimension(source)}
	seen := make(map[string]struct{})
	refs := make([]string, 0, len(identities))
	for _, identity := range identities {
		identity = normalizeDimension(identity)
		if identity == "" {
			continue
		}
		if generation, hash, ok := parseAPIKeyRef(identity); ok {
			ref := apiKeyRef(generation, hash)
			if _, exists := seen[ref]; exists {
				continue
			}
			seen[ref] = struct{}{}
			refs = append(refs, ref)
			continue
		}
		filter.APIKeyHash = identity
	}
	if len(refs) > 0 {
		sort.Strings(refs)
		filter.APIKeyRefSet = strings.Join(refs, "\n")
	}
	return filter
}

func (f usageFilter) matches(dimensions Dimensions) bool {
	if f.Source != "" && dimensions.Source != f.Source {
		return false
	}
	if f.Model != "" && !modelFilterMatches(f.Model, dimensions.Model) {
		return false
	}
	if len(f.Models) > 0 && !modelsFilterMatch(f.Models, dimensions.Model) {
		return false
	}
	if f.APIKeyRefSet != "" {
		ref := apiKeyRef(dimensions.APIKeyGeneration, dimensions.APIKeyHash)
		if ref == "" {
			return false
		}
		for _, candidate := range strings.Split(f.APIKeyRefSet, "\n") {
			if candidate == ref {
				return true
			}
		}
		return false
	}
	return (f.APIKeyHash == "" || dimensions.APIKeyHash == f.APIKeyHash) &&
		(f.APIKeyGeneration == 0 || dimensions.APIKeyGeneration == f.APIKeyGeneration)
}

// Redact clears key material while keeping the opaque fingerprint. The
// dashboard needs the ref to label request rows and CSV exports; the fingerprint
// is already exposed by the API-key filter list, and hashes, crypto generations
// and plaintext stay behind full mode.
func (d *Dimensions) Redact() {
	d.APIKey = ""
	d.APIKeyHash = ""
	d.APIKeyGeneration = 0
	d.APIKeyStatus = ""
}

func (d *Dimensions) Reveal(decrypt DecryptFunc) {
	if d.APIKeyHash == "" || d.APIKeyGeneration == 0 {
		return
	}
	d.APIKeyRef = apiKeyRef(d.APIKeyGeneration, d.APIKeyHash)
	plaintext, status := decrypt(d.APIKey, d.APIKeyHash, d.APIKeyGeneration)
	d.APIKey = plaintext
	d.APIKeyStatus = status
}

type Counters struct {
	Requests            uint64 `json:"requests"`
	FailedRequests      uint64 `json:"failed_requests"`
	InputTokens         uint64 `json:"input_tokens"`
	OutputTokens        uint64 `json:"output_tokens"`
	ReasoningTokens     uint64 `json:"reasoning_tokens"`
	CachedTokens        uint64 `json:"cached_tokens"`
	CacheReadTokens     uint64 `json:"cache_read_tokens"`
	CacheCreationTokens uint64 `json:"cache_creation_tokens"`
	TotalTokens         uint64 `json:"total_tokens"`
	TotalLatencyNS      uint64 `json:"total_latency_ns"`
	TotalTTFTNS         uint64 `json:"total_ttft_ns"`
	LatencySamples      uint64 `json:"latency_samples"`
	TTFTSamples         uint64 `json:"ttft_samples"`
}

func (c *Counters) add(other Counters) {
	c.Requests = saturatingAdd(c.Requests, other.Requests)
	c.FailedRequests = saturatingAdd(c.FailedRequests, other.FailedRequests)
	c.InputTokens = saturatingAdd(c.InputTokens, other.InputTokens)
	c.OutputTokens = saturatingAdd(c.OutputTokens, other.OutputTokens)
	c.ReasoningTokens = saturatingAdd(c.ReasoningTokens, other.ReasoningTokens)
	c.CachedTokens = saturatingAdd(c.CachedTokens, other.CachedTokens)
	c.CacheReadTokens = saturatingAdd(c.CacheReadTokens, other.CacheReadTokens)
	c.CacheCreationTokens = saturatingAdd(c.CacheCreationTokens, other.CacheCreationTokens)
	c.TotalTokens = saturatingAdd(c.TotalTokens, other.TotalTokens)
	c.TotalLatencyNS = saturatingAdd(c.TotalLatencyNS, other.TotalLatencyNS)
	c.TotalTTFTNS = saturatingAdd(c.TotalTTFTNS, other.TotalTTFTNS)
	c.LatencySamples = saturatingAdd(c.LatencySamples, other.LatencySamples)
	c.TTFTSamples = saturatingAdd(c.TTFTSamples, other.TTFTSamples)
}

func (c Counters) averageLatencyNS() uint64 {
	if c.LatencySamples == 0 {
		return 0
	}
	return c.TotalLatencyNS / c.LatencySamples
}

func (c Counters) averageTTFTNS() uint64 {
	if c.TTFTSamples == 0 {
		return 0
	}
	return c.TotalTTFTNS / c.TTFTSamples
}

func countersForUsage(usage normalizedUsage) Counters {
	result := usage.Counters
	if usage.LatencyNS > 0 {
		result.TotalLatencyNS = usage.LatencyNS
		result.LatencySamples = 1
	}
	if usage.TTFTNS > 0 {
		result.TotalTTFTNS = usage.TTFTNS
		result.TTFTSamples = 1
	}
	return result
}

func saturatingAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

type aggregateKey struct {
	Hour       int64
	Dimensions Dimensions
}

type GroupStats struct {
	Dimensions
	Counters
	AverageLatencyNS uint64 `json:"average_latency_ns"`
	AverageTTFTNS    uint64 `json:"average_ttft_ns"`
}

// CostSummary is the derived list-price cost of one response window. It is
// computed on every response and never persisted. PricedRequests counts the
// requests that had a usable price and therefore contributed to CostUSD;
// UnpricedRequests counts the rest of the window. TimeTierCostUSD is the signed
// delta between the time-of-use schedule and the flat base price, so the
// dashboard can explain why a total differs from a naive flat estimate.
type CostSummary struct {
	CostUSD          float64 `json:"cost_usd"`
	PricedModels     int     `json:"priced_models"`
	FreeModels       int     `json:"free_models"`
	UnpricedModels   int     `json:"unpriced_models"`
	PricedRequests   uint64  `json:"priced_requests"`
	UnpricedRequests uint64  `json:"unpriced_requests"`
	TimeTierCostUSD  float64 `json:"time_tier_cost_usd"`
	TimeTierRequests uint64  `json:"time_tier_requests"`
}

// costAccumulator folds priced and unpriced groups into per-model cost tables
// plus the window summary.
type costAccumulator struct {
	costs            map[string]float64
	priced           map[string]bool
	free             map[string]bool
	unpriced         map[string]bool
	pricedRequests   map[string]uint64
	unpricedRequests map[string]uint64
	timeTierCost     float64
	timeTierRequests uint64
}

// add folds one group into the accumulator and reports the derived amount plus
// whether the group had a usable price. Callers use the flag to build the
// per-bucket and per-key cost splits.
func (a *costAccumulator) add(dimensions Dimensions, counters Counters, pricer modelPricing, at time.Time) (float64, bool) {
	model := compactModelName(dimensions.Model)
	if model == "" {
		return 0, false
	}
	if a.unpriced == nil {
		a.costs = make(map[string]float64)
		a.priced = make(map[string]bool)
		a.free = make(map[string]bool)
		a.unpriced = make(map[string]bool)
		a.pricedRequests = make(map[string]uint64)
		a.unpricedRequests = make(map[string]uint64)
	}
	if pricer == nil {
		a.unpriced[model] = true
		a.unpricedRequests[model] = saturatingAdd(a.unpricedRequests[model], counters.Requests)
		return 0, false
	}
	resolved := pricer.PriceFor(dimensions)
	amount, surcharge, priced := chargeFor(resolved, dimensions, counters, at)
	if !priced {
		a.unpriced[model] = true
		a.unpricedRequests[model] = saturatingAdd(a.unpricedRequests[model], counters.Requests)
		return 0, false
	}
	a.costs[model] += amount
	a.priced[model] = true
	a.pricedRequests[model] = saturatingAdd(a.pricedRequests[model], counters.Requests)
	if surcharge != 0 {
		a.timeTierCost += surcharge
		a.timeTierRequests = saturatingAdd(a.timeTierRequests, counters.Requests)
	}
	if resolved.Free {
		a.free[model] = true
	}
	return amount, true
}

// summary reports the window totals. Unpriced models only contribute their
// request count: they have no rate to multiply.
func (a *costAccumulator) summary() CostSummary {
	summary := CostSummary{}
	if a == nil {
		return summary
	}
	for model, amount := range a.costs {
		summary.CostUSD += amount
		if a.free[model] {
			summary.FreeModels++
		} else {
			summary.PricedModels++
		}
		summary.PricedRequests = saturatingAdd(summary.PricedRequests, a.pricedRequests[model])
	}
	for model := range a.unpriced {
		summary.UnpricedModels++
		summary.UnpricedRequests = saturatingAdd(summary.UnpricedRequests, a.unpricedRequests[model])
	}
	summary.TimeTierCostUSD = a.timeTierCost
	summary.TimeTierRequests = a.timeTierRequests
	return summary
}

// ModelStats is the compact per-model aggregate used by the first dashboard
// response. It intentionally omits the unused dimension fields in GroupStats.
// CostUSD is the list-price estimate of the aggregated usage and is derived on
// every response (never persisted), so editing a price changes history.
type ModelStats struct {
	Model string `json:"model"`
	Counters
	AverageLatencyNS uint64  `json:"average_latency_ns"`
	AverageTTFTNS    uint64  `json:"average_ttft_ns"`
	CostUSD          float64 `json:"cost_usd"`
	Priced           bool    `json:"priced"`
}

type SeriesPoint struct {
	Hour string `json:"hour"`
	Counters
}

// CostSeriesPoint is one bucket of the derived cost trend. CostUSD is computed
// on every response from the current price table and never persisted, so a
// price edit immediately rewrites history.
type CostSeriesPoint struct {
	Hour             string  `json:"hour"`
	CostUSD          float64 `json:"cost_usd"`
	Requests         uint64  `json:"requests"`
	UnpricedRequests uint64  `json:"unpriced_requests"`
}

// CostSeriesEntry is one stacked series of the cost histogram: the per-bucket
// cost of a single model or API key. Values is aligned index-by-index with
// InitialStatsResponse.CostSeries, so the browser can stack the series without
// any regrouping. Entries beyond the series limit are folded into a single
// Others entry whose Key is empty.
type CostSeriesEntry struct {
	Key      string    `json:"key"`
	TotalUSD float64   `json:"total_cost_usd"`
	Others   bool      `json:"others,omitempty"`
	Values   []float64 `json:"values"`
}

// ModelSeriesPoint preserves the time-bucketed model split needed by the dashboard for
// stacked trends, model drill-down, and cost calculations without retaining
// individual prompt contents.
type ModelSeriesPoint struct {
	Hour  string `json:"hour"`
	Model string `json:"model"`
	Counters
}

type StatsResponse struct {
	SchemaVersion uint32             `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Range         string             `json:"range"`
	RetainedSince time.Time          `json:"retained_since"`
	LastUsed      time.Time          `json:"last_used"`
	Summary       Counters           `json:"summary"`
	Groups        []GroupStats       `json:"groups"`
	Series        []SeriesPoint      `json:"series"`
	ModelSeries   []ModelSeriesPoint `json:"model_series"`
	Sources       []string           `json:"sources"`
	APIKeys       []APIKeyOption     `json:"api_keys,omitempty"`
}

// InitialStatsResponse excludes dimension rows and per-model time points so the
// dashboard can render its first screen without parsing the largest payloads.
// Series and CostSeries are zero-filled at the resolved scale: every bucket of
// the selected window is present, evenly spaced, so the chart axis can span the
// whole range even when most buckets are empty.
type InitialStatsResponse struct {
	SchemaVersion uint32    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Range         string    `json:"range"`
	RetainedSince time.Time `json:"retained_since"`
	LastUsed      time.Time `json:"last_used"`
	Scale         string    `json:"scale"`
	BucketSeconds uint64    `json:"bucket_seconds"`
	// RequestedScale echoes the explicit ?scale= value when the window was too
	// wide for it and the axis had to be widened. The dashboard uses it to
	// explain the substitution instead of silently showing another unit.
	RequestedScale string            `json:"requested_scale,omitempty"`
	Summary        Counters          `json:"summary"`
	Models         []ModelStats      `json:"models"`
	Series         []SeriesPoint     `json:"series"`
	CostSeries     []CostSeriesPoint `json:"cost_series"`
	CostByModel    []CostSeriesEntry `json:"cost_by_model,omitempty"`
	CostByKey      []CostSeriesEntry `json:"cost_by_key,omitempty"`
	Sources        []string          `json:"sources"`
	APIKeys        []APIKeyOption    `json:"api_keys,omitempty"`
	Pricing        pricingStatus     `json:"pricing"`
	Cost           CostSummary       `json:"cost"`
}

type StatsTrendResponse struct {
	SchemaVersion uint32             `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Range         string             `json:"range"`
	BucketSeconds uint64             `json:"bucket_seconds"`
	ModelSeries   []ModelSeriesPoint `json:"model_series"`
}

type GroupStatsPage struct {
	SchemaVersion uint32       `json:"schema_version"`
	GeneratedAt   time.Time    `json:"generated_at"`
	Range         string       `json:"range"`
	Items         []GroupStats `json:"items"`
	Total         int          `json:"total"`
}

// APIKeyOption is one API-key filter entry. Ref is a stable fingerprint
// ("g<generation>:<hash>") that never contains key material. Key/Hash/
// Generation/Status are cleared on every response; Label is the
// operator-chosen display name from the database. CostUSD is the derived
// list-price estimate of the key's usage inside the window.
type APIKeyOption struct {
	Ref              string  `json:"ref"`
	Hash             string  `json:"hash,omitempty"`
	Generation       uint64  `json:"generation,omitempty"`
	Key              string  `json:"key,omitempty"`
	Status           string  `json:"status,omitempty"`
	Label            string  `json:"label,omitempty"`
	CostUSD          float64 `json:"cost_usd"`
	PricedRequests   uint64  `json:"priced_requests"`
	UnpricedRequests uint64  `json:"unpriced_requests"`
	Counters
}

// redactIdentity clears everything except the opaque fingerprint, the display
// label and the aggregate counters. Filtering by API key needs the ref; the
// key itself, its hash and its crypto generation stay behind full mode.
func (o *APIKeyOption) redactIdentity() {
	o.Hash = ""
	o.Generation = 0
	o.Key = ""
	o.Status = ""
}

func (s *StatsResponse) Redact() {
	for i := range s.Groups {
		s.Groups[i].Dimensions.Redact()
	}
	for i := range s.APIKeys {
		s.APIKeys[i].redactIdentity()
	}
}

func (s *StatsResponse) Reveal(decrypt DecryptFunc) {
	for i := range s.Groups {
		s.Groups[i].Dimensions.Reveal(decrypt)
	}
	for i := range s.APIKeys {
		plaintext, status := decrypt(s.APIKeys[i].Key, s.APIKeys[i].Hash, s.APIKeys[i].Generation)
		s.APIKeys[i].Key = plaintext
		s.APIKeys[i].Status = status
	}
}

func (s *InitialStatsResponse) Redact() {
	for i := range s.APIKeys {
		s.APIKeys[i].redactIdentity()
	}
}

func (s *InitialStatsResponse) Reveal(decrypt DecryptFunc) {
	for i := range s.APIKeys {
		plaintext, status := decrypt(s.APIKeys[i].Key, s.APIKeys[i].Hash, s.APIKeys[i].Generation)
		s.APIKeys[i].Key = plaintext
		s.APIKeys[i].Status = status
	}
}

func (p *GroupStatsPage) Redact() {
	for i := range p.Items {
		p.Items[i].Dimensions.Redact()
	}
}

func (p *GroupStatsPage) Reveal(decrypt DecryptFunc) {
	for i := range p.Items {
		p.Items[i].Dimensions.Reveal(decrypt)
	}
}

type usageRange struct {
	Name  string
	Start time.Time
	End   time.Time
}

func buildStats(data map[aggregateKey]Counters, since, lastUsed time.Time, requestedRange string, now time.Time) (StatsResponse, error) {
	queryRange, err := presetUsageRange(requestedRange, now)
	if err != nil {
		return StatsResponse{}, err
	}
	return buildStatsForRange(data, since, lastUsed, queryRange, "", now), nil
}

func buildStatsForRange(data map[aggregateKey]Counters, since, lastUsed time.Time, queryRange usageRange, source string, now time.Time) StatsResponse {
	return buildStatsForRangeWithFilter(data, since, lastUsed, queryRange, newUsageFilter(source, ""), now, nil)
}

func buildStatsForRangeWithFilter(data map[aggregateKey]Counters, since, lastUsed time.Time, queryRange usageRange, filter usageFilter, now time.Time, apiKeyCiphertexts map[string]string) StatsResponse {
	groups := make(map[Dimensions]Counters)
	series := make(map[int64]Counters)
	modelSeries := make(map[struct {
		Hour  int64
		Model string
	}]Counters)
	summary := Counters{}
	sources := make(map[string]struct{})
	apiKeyRefs := make(map[string]struct{})
	apiKeyCounters := make(map[string]Counters)
	for key, counters := range data {
		bucketTime := time.Unix(key.Hour, 0).UTC()
		if !queryRange.Start.IsZero() && bucketTime.Before(queryRange.Start) {
			continue
		}
		if !queryRange.End.IsZero() && !bucketTime.Before(queryRange.End) {
			continue
		}
		dimensions := sanitizeDimensionsSource(key.Dimensions)
		if dimensions.Source != "" {
			sources[dimensions.Source] = struct{}{}
		}
		if ref := apiKeyRef(dimensions.APIKeyGeneration, dimensions.APIKeyHash); ref != "" {
			apiKeyRefs[ref] = struct{}{}
		}
		if !filter.matches(dimensions) {
			continue
		}
		identity := groupIdentity(dimensions)
		group := groups[identity]
		group.add(counters)
		groups[identity] = group

		point := series[key.Hour]
		point.add(counters)
		series[key.Hour] = point

		model := compactModelName(key.Dimensions.Model)
		modelKey := struct {
			Hour  int64
			Model string
		}{Hour: key.Hour, Model: model}
		modelPoint := modelSeries[modelKey]
		modelPoint.add(counters)
		modelSeries[modelKey] = modelPoint

		if ref := apiKeyRef(dimensions.APIKeyGeneration, dimensions.APIKeyHash); ref != "" {
			refCounters := apiKeyCounters[ref]
			refCounters.add(counters)
			apiKeyCounters[ref] = refCounters
		}

		summary.add(counters)
	}

	groupRows := make([]GroupStats, 0, len(groups))
	for dimensions, counters := range groups {
		if ref := apiKeyRef(dimensions.APIKeyGeneration, dimensions.APIKeyHash); ref != "" {
			dimensions.APIKey = apiKeyCiphertexts[ref]
			dimensions.APIKeyRef = ref
		}
		groupRows = append(groupRows, GroupStats{
			Dimensions:       dimensions,
			Counters:         counters,
			AverageLatencyNS: counters.averageLatencyNS(),
			AverageTTFTNS:    counters.averageTTFTNS(),
		})
	}
	sort.Slice(groupRows, func(i, j int) bool {
		if groupRows[i].TotalTokens != groupRows[j].TotalTokens {
			return groupRows[i].TotalTokens > groupRows[j].TotalTokens
		}
		return compareDimensions(groupRows[i].Dimensions, groupRows[j].Dimensions) < 0
	})

	hours := make([]int64, 0, len(series))
	for hour := range series {
		hours = append(hours, hour)
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i] < hours[j] })
	points := make([]SeriesPoint, 0, len(hours))
	for _, hour := range hours {
		points = append(points, SeriesPoint{
			Hour:     time.Unix(hour, 0).UTC().Format(time.RFC3339),
			Counters: series[hour],
		})
	}

	modelKeys := make([]struct {
		Hour  int64
		Model string
	}, 0, len(modelSeries))
	for key := range modelSeries {
		modelKeys = append(modelKeys, key)
	}
	sort.Slice(modelKeys, func(i, j int) bool {
		if modelKeys[i].Hour != modelKeys[j].Hour {
			return modelKeys[i].Hour < modelKeys[j].Hour
		}
		return modelKeys[i].Model < modelKeys[j].Model
	})
	modelPoints := make([]ModelSeriesPoint, 0, len(modelKeys))
	for _, key := range modelKeys {
		modelPoints = append(modelPoints, ModelSeriesPoint{
			Hour:     time.Unix(key.Hour, 0).UTC().Format(time.RFC3339),
			Model:    key.Model,
			Counters: modelSeries[key],
		})
	}
	sourceValues := make([]string, 0, len(sources))
	for value := range sources {
		sourceValues = append(sourceValues, value)
	}
	sort.Strings(sourceValues)
	apiKeys := sortedStatsAPIKeys(apiKeyRefs, apiKeyCounters, apiKeyCiphertexts, nil, apiKeyCosts{})

	return StatsResponse{
		SchemaVersion: 1,
		GeneratedAt:   now.UTC(),
		Range:         queryRange.Name,
		RetainedSince: since.UTC(),
		LastUsed:      lastUsed.UTC(),
		Summary:       summary,
		Groups:        groupRows,
		Series:        points,
		ModelSeries:   modelPoints,
		Sources:       sourceValues,
		APIKeys:       apiKeys,
	}
}

func compactBucketSeconds(queryRange usageRange, since, now time.Time) uint64 {
	switch queryRange.Name {
	case "24h":
		return uint64((5 * time.Minute) / time.Second)
	case "7d":
		return uint64(time.Hour / time.Second)
	case "30d":
		return uint64((6 * time.Hour) / time.Second)
	}
	start := queryRange.Start
	if start.IsZero() {
		start = since
	}
	if start.IsZero() || !start.Before(now) {
		return uint64(time.Hour / time.Second)
	}
	for _, width := range []uint64{300, 900, 3600, 21600, 86400, 604800} {
		if uint64(now.Sub(start)/time.Second)/width <= 360 {
			return width
		}
	}
	return 604800
}

func compactBucket(unix int64, bucketSeconds uint64) int64 {
	return unix / int64(bucketSeconds) * int64(bucketSeconds)
}

// seriesScale is the dashboard time-axis unit. "auto" is resolved from the
// requested window so the browser receives ready-to-draw, evenly spaced
// buckets that cover the whole selected range.
const (
	seriesScaleAuto   = "auto"
	seriesScaleMinute = "minute"
	seriesScaleHour   = "hour"
	seriesScaleDay    = "day"
)

const (
	seriesMinuteSeconds = int64(60)
	seriesHourSeconds   = int64(60 * 60)
	seriesDaySeconds    = int64(24 * 60 * 60)

	// maxFineSeriesBuckets bounds one chart at 1440 points, i.e. a full day of
	// minutes or sixty days of hours. Minute and hour scales are clamped to it
	// so a huge window can never inflate the first payload or render unreadable
	// sub-pixel bars.
	maxFineSeriesBuckets = 1440
	// maxDaySeriesBuckets leaves room for a multi-year retention window.
	maxDaySeriesBuckets = 800

	maxSeriesTZOffsetMinutes     = 840
	seriesAutoDayThreshold       = 48 * time.Hour
	seriesAutoHourThreshold      = 2 * time.Hour
	maxSeriesBucketFillHardLimit = 4000

	// costBreakdownSeriesLimit bounds the stacked cost histogram. Everything
	// past the limit is folded into one "others" series so the legend, the
	// tooltip and the SVG stay readable.
	costBreakdownSeriesLimit = 7
	// costBreakdownFineSeriesLimit is used when the axis has so many buckets
	// that individual bars are only a couple of pixels wide.
	costBreakdownFineSeriesLimit = 5
	costBreakdownFineBuckets     = 240
)

// seriesSpec is the resolved bucket geometry of one statistics response.
// WindowStart/WindowEnd carry the fill window independently of the data filter:
// request-row queries are already range-filtered, so their builders must not
// re-apply second-precision bounds to minute-aligned buckets.
type seriesSpec struct {
	Scale           string
	BucketSeconds   int64
	TZOffsetMinutes int
	WindowStart     time.Time
	WindowEnd       time.Time
	// RequestedScale carries the explicit scale the caller asked for when the
	// window was too wide for it and the axis had to be widened. It stays empty
	// for the automatic scale and whenever the request was honoured verbatim, so
	// the dashboard only explains a real, silent-looking substitution.
	RequestedScale string
}

// parseSeriesScale validates the ?scale= query value.
func parseSeriesScale(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", seriesScaleAuto:
		return seriesScaleAuto, nil
	case seriesScaleMinute, seriesScaleHour, seriesScaleDay:
		return strings.ToLower(strings.TrimSpace(value)), nil
	default:
		return "", withStatus(400, "scale must be auto, minute, hour or day")
	}
}

// parseSeriesTZOffset validates ?tz_offset= (minutes east of UTC).
func parseSeriesTZOffset(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	minutes, err := strconv.Atoi(trimmed)
	if err != nil || minutes < -maxSeriesTZOffsetMinutes || minutes > maxSeriesTZOffsetMinutes {
		return 0, withStatus(400, "tz_offset must be an integer between -%d and %d", maxSeriesTZOffsetMinutes, maxSeriesTZOffsetMinutes)
	}
	return minutes, nil
}

// autoSeriesScale implements the documented default: day for windows of two
// days or longer, hour between two hours and two days, minute below that.
func autoSeriesScale(span time.Duration) string {
	switch {
	case span >= seriesAutoDayThreshold:
		return seriesScaleDay
	case span >= seriesAutoHourThreshold:
		return seriesScaleHour
	default:
		return seriesScaleMinute
	}
}

func seriesScaleBucketSeconds(scale string) int64 {
	switch scale {
	case seriesScaleMinute:
		return seriesMinuteSeconds
	case seriesScaleDay:
		return seriesDaySeconds
	default:
		return seriesHourSeconds
	}
}

func seriesScaleAvailable(scale string, span time.Duration) bool {
	// A one-bucket tolerance accommodates partial boundary minutes or hours
	// created when a preset start was truncated to the beginning of the unit
	// but the window runs up to the current wall-clock second.
	switch scale {
	case seriesScaleMinute:
		return span <= time.Duration(maxFineSeriesBuckets)*time.Minute+time.Minute
	case seriesScaleHour:
		return span <= time.Duration(maxFineSeriesBuckets)*time.Hour+time.Hour
	case seriesScaleDay:
		return span <= time.Duration(maxDaySeriesBuckets)*24*time.Hour+24*time.Hour
	default:
		return true
	}
}

// resolveSeriesSpec maps the requested scale onto an available bucket width.
// An explicit scale that does not fit the window is widened to the nearest
// coarser unit that does (minute -> hour -> day) instead of failing the
// dashboard, and the original request travels in RequestedScale so the browser
// can say why the axis looks different from the highlighted button.
func resolveSeriesSpec(scale string, tzMinutes int, queryRange usageRange, since, now time.Time) seriesSpec {
	// The automatic scale follows the window the operator selected, not the
	// retained slice of it, so a 24h view always reports hour buckets.
	requestedStart, requestedEnd := queryRange.requestedWindow(since, now)
	span := time.Duration(0)
	if !requestedStart.IsZero() && requestedStart.Before(requestedEnd) {
		span = requestedEnd.Sub(requestedStart)
	}
	start, end := queryRange.window(since, now)
	requested := ""
	if scale != "" && scale != seriesScaleAuto {
		requested = scale
	}
	effective := autoSeriesScale(span)
	if requested != "" {
		effective = requested
		if !seriesScaleAvailable(effective, span) {
			switch effective {
			case seriesScaleMinute:
				effective = seriesScaleHour
			case seriesScaleHour:
				effective = seriesScaleDay
			}
		}
	}
	// Safety net for the automatic choice and for a window longer than the
	// coarsest cap.
	if !seriesScaleAvailable(effective, span) {
		switch effective {
		case seriesScaleMinute:
			effective = seriesScaleHour
		case seriesScaleHour:
			effective = seriesScaleDay
		}
	}
	requestedScale := ""
	if requested != "" && requested != effective {
		requestedScale = requested
	}
	tzMinutes = clampSeriesTZOffset(tzMinutes)
	return seriesSpec{
		Scale:           effective,
		BucketSeconds:   seriesScaleBucketSeconds(effective),
		TZOffsetMinutes: tzMinutes,
		WindowStart:     start,
		WindowEnd:       end,
		RequestedScale:  requestedScale,
	}
}

func clampSeriesTZOffset(minutes int) int {
	if minutes < -maxSeriesTZOffsetMinutes {
		return -maxSeriesTZOffsetMinutes
	}
	if minutes > maxSeriesTZOffsetMinutes {
		return maxSeriesTZOffsetMinutes
	}
	return minutes
}

// requestedWindow is the window the caller asked for: an explicit start/end
// when supplied, otherwise the retained horizon up to now.
func (r usageRange) requestedWindow(since, now time.Time) (time.Time, time.Time) {
	end := r.End
	if end.IsZero() {
		end = now.UTC()
	}
	start := r.Start
	if start.IsZero() {
		start = since
	}
	if start.IsZero() || !start.Before(end) {
		return time.Time{}, time.Time{}
	}
	return start.UTC(), end.UTC()
}

// window returns the fill window of the range: the requested window itself, so
// the chart axis always spans what the operator selected. Pathologically long
// windows are trimmed to the per-scale bucket cap by bucketStarts.
func (r usageRange) window(since, now time.Time) (time.Time, time.Time) {
	return r.requestedWindow(since, now)
}

// seriesBucketLimit is the number of buckets a scale may fill.
func seriesBucketLimit(scale string) int {
	if scale == seriesScaleDay {
		return maxDaySeriesBuckets
	}
	return maxFineSeriesBuckets
}

func floorDiv(value, divisor int64) int64 {
	if divisor == 0 {
		return 0
	}
	result := value / divisor
	if value%divisor != 0 && (value < 0) != (divisor < 0) {
		result--
	}
	return result
}

// bucketStartOf snaps a unix timestamp to the bucket that contains it. Day
// buckets honour the browser timezone so a local-day chart starts at local
// midnight; hour and minute buckets stay UTC aligned.
func bucketStartOf(unix int64, spec seriesSpec) int64 {
	width := spec.BucketSeconds
	if width <= 0 {
		width = seriesHourSeconds
	}
	if width == seriesDaySeconds {
		offset := int64(spec.TZOffsetMinutes) * 60
		return floorDiv(unix+offset, width)*width - offset
	}
	return floorDiv(unix, width) * width
}

// bucketStarts fills every bucket between start and end, including the empty
// ones, so the chart axis always spans the whole selected window.
func bucketStarts(start, end time.Time, spec seriesSpec) []int64 {
	if start.IsZero() || !start.Before(end) {
		return nil
	}
	width := spec.BucketSeconds
	if width <= 0 {
		width = seriesHourSeconds
	}
	first := bucketStartOf(start.Unix(), spec)
	last := bucketStartOf(end.Add(-time.Nanosecond).Unix(), spec)
	if last < first {
		return nil
	}
	count := int((last-first)/width) + 1
	if count <= 0 || count > maxSeriesBucketFillHardLimit {
		return nil
	}
	starts := make([]int64, 0, count)
	for value := first; value <= last; value += width {
		starts = append(starts, value)
	}
	return starts
}

func formatBucketTime(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

// includes reports whether t falls inside the half-open query window.
func (r usageRange) includes(t time.Time) bool {
	if !r.Start.IsZero() && t.Before(r.Start) {
		return false
	}
	if !r.End.IsZero() && !t.Before(r.End) {
		return false
	}
	return true
}

func compactModelName(model string) string {
	return model
}

// modelFilterMatches keeps the old Chinese "unnamed model" filter value
// working while API payloads remain backend-neutral and let the dashboard
// localize presentation.
func modelFilterMatches(requested, model string) bool {
	model = compactModelName(model)
	return model == requested || (requested == legacyUntitledModelName && model == "")
}

// modelsFilterMatch reports whether model matches any requested model value.
func modelsFilterMatch(requested []string, model string) bool {
	for _, candidate := range requested {
		if modelFilterMatches(candidate, model) {
			return true
		}
	}
	return false
}

// normalizeModelFilter canonicalizes repeated model query values, dropping
// blanks and duplicates while preserving the caller's order.
func normalizeModelFilter(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	models := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeDimension(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		models = append(models, value)
	}
	return models
}

func sortedCompactSeries(series map[int64]Counters) []SeriesPoint {
	keys := make([]int64, 0, len(series))
	for key := range series {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	points := make([]SeriesPoint, 0, len(keys))
	for _, key := range keys {
		points = append(points, SeriesPoint{Hour: time.Unix(key, 0).UTC().Format(time.RFC3339), Counters: series[key]})
	}
	return points
}

func sortedCompactModels(models map[string]Counters, costs map[string]float64, priced map[string]bool) []ModelStats {
	rows := make([]ModelStats, 0, len(models))
	for model, counters := range models {
		rows = append(rows, ModelStats{
			Model:            model,
			Counters:         counters,
			AverageLatencyNS: counters.averageLatencyNS(),
			AverageTTFTNS:    counters.averageTTFTNS(),
			CostUSD:          costs[model],
			Priced:           priced[model],
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens != rows[j].TotalTokens {
			return rows[i].TotalTokens > rows[j].TotalTokens
		}
		return rows[i].Model < rows[j].Model
	})
	return rows
}

func trendStatsFromFull(stats StatsResponse, queryRange usageRange) StatsTrendResponse {
	bucketSeconds := compactBucketSeconds(queryRange, stats.RetainedSince, stats.GeneratedAt)
	series := make(map[struct {
		bucket int64
		model  string
	}]Counters)
	for _, point := range stats.ModelSeries {
		timestamp, err := time.Parse(time.RFC3339, point.Hour)
		if err != nil {
			continue
		}
		key := struct {
			bucket int64
			model  string
		}{bucket: compactBucket(timestamp.Unix(), bucketSeconds), model: compactModelName(point.Model)}
		value := series[key]
		value.add(point.Counters)
		series[key] = value
	}
	keys := make([]struct {
		bucket int64
		model  string
	}, 0, len(series))
	for key := range series {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].bucket != keys[j].bucket {
			return keys[i].bucket < keys[j].bucket
		}
		return keys[i].model < keys[j].model
	})
	points := make([]ModelSeriesPoint, 0, len(keys))
	for _, key := range keys {
		points = append(points, ModelSeriesPoint{Hour: time.Unix(key.bucket, 0).UTC().Format(time.RFC3339), Model: key.model, Counters: series[key]})
	}
	return StatsTrendResponse{SchemaVersion: 2, GeneratedAt: stats.GeneratedAt, Range: stats.Range, BucketSeconds: bucketSeconds, ModelSeries: points}
}

// apiKeyCosts carries the derived per-key cost split into the key list.
type apiKeyCosts struct {
	costUSD          map[string]float64
	pricedRequests   map[string]uint64
	unpricedRequests map[string]uint64
}

// sortedStatsAPIKeys builds the API-key filter list. refs holds every key
// observed inside the requested window, while counters holds the filtered
// totals so a key that is currently excluded still appears as a zero-value
// option the operator can re-enable.
func sortedStatsAPIKeys(refs map[string]struct{}, counters map[string]Counters, ciphertexts, labels map[string]string, costs apiKeyCosts) []APIKeyOption {
	values := make([]APIKeyOption, 0, len(refs))
	for ref := range refs {
		generation, hash, _ := parseAPIKeyRef(ref)
		values = append(values, APIKeyOption{
			Ref:              ref,
			Hash:             hash,
			Generation:       generation,
			Key:              ciphertexts[ref],
			Label:            labels[ref],
			CostUSD:          costs.costUSD[ref],
			PricedRequests:   costs.pricedRequests[ref],
			UnpricedRequests: costs.unpricedRequests[ref],
			Counters:         counters[ref],
		})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].TotalTokens != values[j].TotalTokens {
			return values[i].TotalTokens > values[j].TotalTokens
		}
		if values[i].Requests != values[j].Requests {
			return values[i].Requests > values[j].Requests
		}
		return values[i].Ref < values[j].Ref
	})
	return values
}

// buildCostBreakdown turns per-key per-bucket costs into the stacked series of
// the cost histogram. Series are ordered by total cost (ties broken by key so
// the payload is deterministic), capped at limit, and everything past the cap
// is folded into one Others entry that always sits last. Values are aligned
// with the caller's bucket list, which is the same list CostSeries uses.
func buildCostBreakdown(costs map[string]map[int64]float64, buckets []int64, limit int) []CostSeriesEntry {
	if len(costs) == 0 || len(buckets) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = costBreakdownSeriesLimit
	}
	type rankedCost struct {
		key   string
		total float64
	}
	ranked := make([]rankedCost, 0, len(costs))
	for key, perBucket := range costs {
		var total float64
		for _, amount := range perBucket {
			total += amount
		}
		// Unpriced models cost nothing; they are reported through the unpriced
		// request counters instead of an all-zero chart series.
		if total <= 0 {
			continue
		}
		ranked = append(ranked, rankedCost{key: key, total: total})
	}
	if len(ranked) == 0 {
		return nil
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].total != ranked[j].total {
			return ranked[i].total > ranked[j].total
		}
		return ranked[i].key < ranked[j].key
	})
	valuesFor := func(perBucket map[int64]float64) []float64 {
		values := make([]float64, len(buckets))
		for index, bucket := range buckets {
			values[index] = perBucket[bucket]
		}
		return values
	}
	entries := make([]CostSeriesEntry, 0, limit+1)
	for _, item := range ranked {
		if len(entries) >= limit {
			break
		}
		entries = append(entries, CostSeriesEntry{
			Key:      item.key,
			TotalUSD: item.total,
			Values:   valuesFor(costs[item.key]),
		})
	}
	if len(ranked) <= limit {
		return entries
	}
	others := CostSeriesEntry{Others: true, Values: make([]float64, len(buckets))}
	for _, item := range ranked[limit:] {
		others.TotalUSD += item.total
		for index, bucket := range buckets {
			others.Values[index] += costs[item.key][bucket]
		}
	}
	return append(entries, others)
}

// bumpBucketCost adds one priced row to a per-key per-bucket cost table.
func bumpBucketCost(table map[string]map[int64]float64, key string, bucket int64, amount float64) {
	perBucket := table[key]
	if perBucket == nil {
		perBucket = make(map[int64]float64)
		table[key] = perBucket
	}
	perBucket[bucket] += amount
}

// buildInitialStatsForRange renders the first dashboard payload. Series and
// CostSeries are zero-filled across the resolved window so the browser can draw
// a full-width axis without regrouping anything.
func buildInitialStatsForRange(data map[aggregateKey]Counters, since, lastUsed time.Time, queryRange usageRange, filter usageFilter, now time.Time, apiKeyCiphertexts map[string]string, pricer modelPricing, spec seriesSpec) InitialStatsResponse {
	if spec.BucketSeconds <= 0 {
		spec = resolveSeriesSpec(seriesScaleAuto, 0, queryRange, since, now)
	}
	series := make(map[int64]Counters)
	bucketCosts := make(map[int64]float64)
	bucketRequests := make(map[int64]uint64)
	bucketUnpriced := make(map[int64]uint64)
	modelBucketCosts := make(map[string]map[int64]float64)
	keyBucketCosts := make(map[string]map[int64]float64)
	models := make(map[string]Counters)
	cost := &costAccumulator{}
	sources := make(map[string]struct{})
	apiKeyRefs := make(map[string]struct{})
	apiKeyCounters := make(map[string]Counters)
	apiKeyCostsByRef := make(map[string]float64)
	apiKeyPriced := make(map[string]uint64)
	apiKeyUnpriced := make(map[string]uint64)
	summary := Counters{}
	for key, counters := range data {
		bucketTime := time.Unix(key.Hour, 0).UTC()
		if !queryRange.includes(bucketTime) {
			continue
		}
		dimensions := sanitizeDimensionsSource(key.Dimensions)
		if dimensions.Source != "" {
			sources[dimensions.Source] = struct{}{}
		}
		ref := apiKeyRef(dimensions.APIKeyGeneration, dimensions.APIKeyHash)
		if ref != "" {
			apiKeyRefs[ref] = struct{}{}
		}
		if !filter.matches(dimensions) {
			continue
		}
		seriesKey := bucketStartOf(key.Hour, spec)
		point := series[seriesKey]
		point.add(counters)
		series[seriesKey] = point
		model := compactModelName(dimensions.Model)
		modelCounters := models[model]
		modelCounters.add(counters)
		models[model] = modelCounters
		amount, priced := cost.add(dimensions, counters, pricer, bucketTime)
		bucketRequests[seriesKey] = saturatingAdd(bucketRequests[seriesKey], counters.Requests)
		if priced {
			bucketCosts[seriesKey] += amount
			if amount > 0 {
				bumpBucketCost(modelBucketCosts, model, seriesKey, amount)
				if ref != "" {
					bumpBucketCost(keyBucketCosts, ref, seriesKey, amount)
				}
			}
		} else {
			bucketUnpriced[seriesKey] = saturatingAdd(bucketUnpriced[seriesKey], counters.Requests)
		}
		if ref != "" {
			refCounters := apiKeyCounters[ref]
			refCounters.add(counters)
			apiKeyCounters[ref] = refCounters
			if priced {
				apiKeyCostsByRef[ref] += amount
				apiKeyPriced[ref] = saturatingAdd(apiKeyPriced[ref], counters.Requests)
			} else {
				apiKeyUnpriced[ref] = saturatingAdd(apiKeyUnpriced[ref], counters.Requests)
			}
		}
		summary.add(counters)
	}
	sourceValues := make([]string, 0, len(sources))
	for source := range sources {
		sourceValues = append(sourceValues, source)
	}
	sort.Strings(sourceValues)

	start, end := spec.WindowStart, spec.WindowEnd
	if start.IsZero() || end.IsZero() {
		start, end = queryRange.window(since, now)
	}
	starts := bucketStarts(start, end, spec)
	if len(starts) == 0 && !start.IsZero() && start.Before(end) {
		// The selected window is longer than the per-scale cap: keep the newest
		// buckets instead of degrading to a sparse axis.
		limit := seriesBucketLimit(spec.Scale)
		clampedStart := end.Add(-time.Duration(limit) * time.Duration(spec.BucketSeconds) * time.Second)
		if clampedStart.After(start) {
			starts = bucketStarts(clampedStart, end, spec)
		}
	}
	points := make([]SeriesPoint, 0, len(starts))
	costPoints := make([]CostSeriesPoint, 0, len(starts))
	// bucketUnix mirrors costPoints one-to-one so the stacked breakdown stays
	// aligned with the cost series in both the aligned and sparse branches.
	bucketUnix := make([]int64, 0, len(starts))
	for _, bucket := range starts {
		points = append(points, SeriesPoint{Hour: formatBucketTime(bucket), Counters: series[bucket]})
		costPoints = append(costPoints, CostSeriesPoint{
			Hour:             formatBucketTime(bucket),
			CostUSD:          bucketCosts[bucket],
			Requests:         bucketRequests[bucket],
			UnpricedRequests: bucketUnpriced[bucket],
		})
		bucketUnix = append(bucketUnix, bucket)
	}
	if len(starts) == 0 {
		// Degenerate window (no retained data yet): keep the sparse buckets so
		// the payload shape stays stable.
		points = sortedCompactSeries(series)
		for _, point := range points {
			timestamp, err := time.Parse(time.RFC3339, point.Hour)
			if err != nil {
				continue
			}
			bucket := timestamp.Unix()
			costPoints = append(costPoints, CostSeriesPoint{
				Hour:             point.Hour,
				CostUSD:          bucketCosts[bucket],
				Requests:         bucketRequests[bucket],
				UnpricedRequests: bucketUnpriced[bucket],
			})
			bucketUnix = append(bucketUnix, bucket)
		}
	}
	breakdownLimit := costBreakdownSeriesLimit
	if len(bucketUnix) > costBreakdownFineBuckets {
		breakdownLimit = costBreakdownFineSeriesLimit
	}

	return InitialStatsResponse{
		SchemaVersion:  3,
		GeneratedAt:    now.UTC(),
		Range:          queryRange.Name,
		RetainedSince:  since.UTC(),
		LastUsed:       lastUsed.UTC(),
		Scale:          spec.Scale,
		BucketSeconds:  uint64(spec.BucketSeconds),
		RequestedScale: spec.RequestedScale,
		Summary:        summary,
		Models:         sortedCompactModels(models, cost.costs, cost.priced),
		Series:         points,
		CostSeries:     costPoints,
		CostByModel:    buildCostBreakdown(modelBucketCosts, bucketUnix, breakdownLimit),
		CostByKey:      buildCostBreakdown(keyBucketCosts, bucketUnix, breakdownLimit),
		Sources:        sourceValues,
		APIKeys: sortedStatsAPIKeys(apiKeyRefs, apiKeyCounters, apiKeyCiphertexts, nil, apiKeyCosts{
			costUSD:          apiKeyCostsByRef,
			pricedRequests:   apiKeyPriced,
			unpricedRequests: apiKeyUnpriced,
		}),
		Cost: cost.summary(),
	}
}

func buildStatsTrendForRange(data map[aggregateKey]Counters, since time.Time, queryRange usageRange, filter usageFilter, now time.Time) StatsTrendResponse {
	bucketSeconds := compactBucketSeconds(queryRange, since, now)
	series := make(map[struct {
		bucket int64
		model  string
	}]Counters)
	for key, counters := range data {
		bucketTime := time.Unix(key.Hour, 0).UTC()
		if (!queryRange.Start.IsZero() && bucketTime.Before(queryRange.Start)) || (!queryRange.End.IsZero() && !bucketTime.Before(queryRange.End)) {
			continue
		}
		dimensions := sanitizeDimensionsSource(key.Dimensions)
		if !filter.matches(dimensions) {
			continue
		}
		modelKey := struct {
			bucket int64
			model  string
		}{bucket: compactBucket(key.Hour, bucketSeconds), model: compactModelName(dimensions.Model)}
		point := series[modelKey]
		point.add(counters)
		series[modelKey] = point
	}
	keys := make([]struct {
		bucket int64
		model  string
	}, 0, len(series))
	for key := range series {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].bucket != keys[j].bucket {
			return keys[i].bucket < keys[j].bucket
		}
		return keys[i].model < keys[j].model
	})
	points := make([]ModelSeriesPoint, 0, len(keys))
	for _, key := range keys {
		points = append(points, ModelSeriesPoint{Hour: time.Unix(key.bucket, 0).UTC().Format(time.RFC3339), Model: key.model, Counters: series[key]})
	}
	return StatsTrendResponse{SchemaVersion: 2, GeneratedAt: now.UTC(), Range: queryRange.Name, BucketSeconds: bucketSeconds, ModelSeries: points}
}

func buildGroupsForRange(data map[aggregateKey]Counters, queryRange usageRange, filter usageFilter, now time.Time, apiKeyCiphertexts map[string]string) GroupStatsPage {
	groups := make(map[Dimensions]Counters)
	for key, counters := range data {
		bucketTime := time.Unix(key.Hour, 0).UTC()
		if (!queryRange.Start.IsZero() && bucketTime.Before(queryRange.Start)) || (!queryRange.End.IsZero() && !bucketTime.Before(queryRange.End)) {
			continue
		}
		dimensions := sanitizeDimensionsSource(key.Dimensions)
		if !filter.matches(dimensions) {
			continue
		}
		identity := groupIdentity(dimensions)
		group := groups[identity]
		group.add(counters)
		groups[identity] = group
	}
	items := make([]GroupStats, 0, len(groups))
	for dimensions, counters := range groups {
		if ref := apiKeyRef(dimensions.APIKeyGeneration, dimensions.APIKeyHash); ref != "" {
			dimensions.APIKey = apiKeyCiphertexts[ref]
			dimensions.APIKeyRef = ref
		}
		items = append(items, GroupStats{Dimensions: dimensions, Counters: counters, AverageLatencyNS: counters.averageLatencyNS(), AverageTTFTNS: counters.averageTTFTNS()})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TotalTokens != items[j].TotalTokens {
			return items[i].TotalTokens > items[j].TotalTokens
		}
		return compareDimensions(items[i].Dimensions, items[j].Dimensions) < 0
	})
	return GroupStatsPage{SchemaVersion: 2, GeneratedAt: now.UTC(), Range: queryRange.Name, Items: items, Total: len(items)}
}

func queryCutoff(value string, now time.Time) (string, time.Time, error) {
	queryRange, err := presetUsageRange(value, now)
	return queryRange.Name, queryRange.Start, err
}

func presetUsageRange(value string, now time.Time) (usageRange, error) {
	now = now.UTC()
	switch value {
	case "", "24h":
		return usageRange{Name: "24h", Start: now.Add(-24 * time.Hour).Truncate(time.Minute)}, nil
	case "7d":
		return usageRange{Name: "7d", Start: now.Add(-7 * 24 * time.Hour).Truncate(time.Minute)}, nil
	case "30d":
		return usageRange{Name: "30d", Start: now.Add(-30 * 24 * time.Hour).Truncate(time.Minute)}, nil
	case "retention":
		return usageRange{Name: "retention"}, nil
	default:
		return usageRange{}, withStatus(400, "unsupported range %q", value)
	}
}

func usageRangeFromQuery(rangeValue, startValue, endValue string, now time.Time) (usageRange, error) {
	if startValue == "" && endValue == "" {
		return presetUsageRange(rangeValue, now)
	}
	if rangeValue != "" && rangeValue != "custom" {
		return usageRange{}, withStatus(400, "range must be custom when start and end are provided")
	}
	if startValue == "" || endValue == "" {
		return usageRange{}, withStatus(400, "start and end must be provided together")
	}
	start, err := time.Parse(time.RFC3339, startValue)
	if err != nil {
		return usageRange{}, withStatus(400, "invalid start time: %v", err)
	}
	end, err := time.Parse(time.RFC3339, endValue)
	if err != nil {
		return usageRange{}, withStatus(400, "invalid end time: %v", err)
	}
	start = start.UTC()
	end = end.UTC()
	if !start.Before(end) {
		return usageRange{}, withStatus(400, "start must be before end")
	}
	return usageRange{Name: "custom", Start: start, End: end}, nil
}

func (r usageRange) validate() error {
	if r.Name == "" {
		return fmt.Errorf("range name is required")
	}
	if !r.End.IsZero() && (r.Start.IsZero() || !r.Start.Before(r.End)) {
		return fmt.Errorf("invalid time range")
	}
	return nil
}

func compareDimensions(left, right Dimensions) int {
	for _, comparison := range []int{
		cmp.Compare(left.Provider, right.Provider),
		cmp.Compare(left.ExecutorType, right.ExecutorType),
		cmp.Compare(left.Model, right.Model),
		cmp.Compare(left.Alias, right.Alias),
		cmp.Compare(left.Source, right.Source),
		cmp.Compare(left.APIKeyGeneration, right.APIKeyGeneration),
		cmp.Compare(left.APIKeyHash, right.APIKeyHash),
		cmp.Compare(left.AuthType, right.AuthType),
		cmp.Compare(left.ServiceTier, right.ServiceTier),
		cmp.Compare(left.ReasoningEffort, right.ReasoningEffort),
		cmp.Compare(boolInt(left.Failed), boolInt(right.Failed)),
		cmp.Compare(left.FailureStatus, right.FailureStatus),
	} {
		if comparison != 0 {
			return comparison
		}
	}
	return 0
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
