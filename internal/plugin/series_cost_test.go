package plugin

import (
	"math"
	"testing"
	"time"
)

// TestBuildInitialStatsCostSeriesAndKeyCosts pins the schema 3 cost contract:
// the cost series mirrors the token series bucket for bucket, carries the
// unpriced request count, and the per-key and summary blocks expose the priced
// request split the 花销 tab renders.
func TestBuildInitialStatsCostSeriesAndKeyCosts(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	first := now.Add(-3 * time.Hour).Truncate(time.Hour)
	hashA := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	refA := apiKeyRef(1, hashA)
	refB := apiKeyRef(1, hashB)
	data := map[aggregateKey]Counters{
		{Hour: first.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "priced-model", APIKeyHash: hashA, APIKeyGeneration: 1}}: {
			Requests: 4, InputTokens: 1_000_000, OutputTokens: 1_000_000, TotalTokens: 2_000_000,
		},
		{Hour: first.Add(time.Hour).Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "unpriced-model", APIKeyHash: hashB, APIKeyGeneration: 1}}: {
			Requests: 6, InputTokens: 500_000, OutputTokens: 500_000, TotalTokens: 1_000_000,
		},
		{Hour: first.Add(2 * time.Hour).Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "priced-model", APIKeyHash: hashB, APIKeyGeneration: 1}}: {
			Requests: 2, InputTokens: 100_000, OutputTokens: 100_000, TotalTokens: 200_000,
		},
	}
	pricer := stubPricer{prices: map[string]ModelPrice{
		// 1 USD per million input tokens, 2 USD per million output tokens.
		"priced-model": priceOf(1, 2, mathNaN(), mathNaN()),
	}}
	queryRange := usageRange{Name: "custom", Start: now.Add(-4 * time.Hour), End: now}
	// The fill window reaches one hour past the data window so the trailing
	// bucket proves the zero fill.
	spec := seriesSpec{Scale: seriesScaleHour, BucketSeconds: seriesHourSeconds, WindowStart: first, WindowEnd: now.Add(time.Hour)}
	initial := buildInitialStatsForRange(data, now.Add(-30*24*time.Hour), now, queryRange, usageFilter{}, now, nil, pricer, spec)

	if initial.SchemaVersion != 3 || initial.Scale != seriesScaleHour || initial.BucketSeconds != 3600 {
		t.Fatalf("initial shape = schema %d scale %q bucket %d", initial.SchemaVersion, initial.Scale, initial.BucketSeconds)
	}
	if len(initial.Series) != 4 || len(initial.CostSeries) != 4 {
		t.Fatalf("series/cost series = %d/%d, want 4/4", len(initial.Series), len(initial.CostSeries))
	}
	for index := range initial.Series {
		if initial.Series[index].Hour != initial.CostSeries[index].Hour {
			t.Fatalf("bucket %d hour mismatch: %s vs %s", index, initial.Series[index].Hour, initial.CostSeries[index].Hour)
		}
	}
	// 1M input + 1M output on the priced model = 1 + 2 = 3 USD; the later
	// priced hour adds 0.1 + 0.2 = 0.3 USD.
	wantCost := 3.3
	if math.Abs(initial.CostSeries[0].CostUSD-3) > 1e-9 || initial.CostSeries[0].Requests != 4 || initial.CostSeries[0].UnpricedRequests != 0 {
		t.Fatalf("first cost bucket = %+v", initial.CostSeries[0])
	}
	if initial.CostSeries[1].CostUSD != 0 || initial.CostSeries[1].Requests != 6 || initial.CostSeries[1].UnpricedRequests != 6 {
		t.Fatalf("unpriced cost bucket = %+v", initial.CostSeries[1])
	}
	if math.Abs(initial.CostSeries[2].CostUSD-0.3) > 1e-9 || initial.CostSeries[2].UnpricedRequests != 0 {
		t.Fatalf("third cost bucket = %+v", initial.CostSeries[2])
	}
	if initial.CostSeries[3].CostUSD != 0 || initial.CostSeries[3].Requests != 0 {
		t.Fatalf("trailing cost bucket must stay zero-filled: %+v", initial.CostSeries[3])
	}

	if math.Abs(initial.Cost.CostUSD-wantCost) > 1e-9 || initial.Cost.PricedRequests != 6 || initial.Cost.UnpricedRequests != 6 {
		t.Fatalf("cost summary = %+v", initial.Cost)
	}
	if initial.Cost.PricedModels != 1 || initial.Cost.UnpricedModels != 1 {
		t.Fatalf("cost model split = %+v", initial.Cost)
	}
	if initial.Summary.Requests != 12 || initial.Summary.TotalTokens != 3_200_000 {
		t.Fatalf("summary counters = %+v", initial.Summary)
	}

	keys := map[string]APIKeyOption{}
	for _, entry := range initial.APIKeys {
		keys[entry.Ref] = entry
	}
	if len(keys) != 2 {
		t.Fatalf("api keys = %+v", initial.APIKeys)
	}
	if math.Abs(keys[refA].CostUSD-3) > 1e-9 || keys[refA].PricedRequests != 4 || keys[refA].UnpricedRequests != 0 {
		t.Fatalf("key A cost = %+v", keys[refA])
	}
	if math.Abs(keys[refB].CostUSD-0.3) > 1e-9 || keys[refB].PricedRequests != 2 || keys[refB].UnpricedRequests != 6 {
		t.Fatalf("key B cost = %+v", keys[refB])
	}

	// The stacked histogram splits ride along with the cost series: one entry
	// per priced model, one per key, each aligned bucket for bucket.
	if len(initial.CostByModel) != 1 || len(initial.CostByKey) != 2 {
		t.Fatalf("cost breakdown = %d models / %d keys, want 1/2", len(initial.CostByModel), len(initial.CostByKey))
	}
	if entry := initial.CostByModel[0]; entry.Key != "priced-model" || entry.Others || math.Abs(entry.TotalUSD-3.3) > 1e-9 {
		t.Fatalf("model breakdown entry = %+v", entry)
	}
	if got := initial.CostByModel[0].Values; len(got) != 4 || math.Abs(got[0]-3) > 1e-9 || got[1] != 0 || math.Abs(got[2]-0.3) > 1e-9 || got[3] != 0 {
		t.Fatalf("model breakdown values = %v", got)
	}
	// Highest total first, and the per-key series mirror the cost buckets.
	if initial.CostByKey[0].Key != refA || initial.CostByKey[1].Key != refB {
		t.Fatalf("key breakdown order = %q/%q", initial.CostByKey[0].Key, initial.CostByKey[1].Key)
	}
	for _, entry := range initial.CostByKey {
		if len(entry.Values) != len(initial.CostSeries) {
			t.Fatalf("key breakdown %q has %d values, want %d", entry.Key, len(entry.Values), len(initial.CostSeries))
		}
	}
	if got := initial.CostByKey[1].Values; math.Abs(got[2]-0.3) > 1e-9 || got[0] != 0 {
		t.Fatalf("key B breakdown values = %v", got)
	}
	// Every breakdown series must add up to the cost series it stacks into.
	for _, entry := range append(append([]CostSeriesEntry{}, initial.CostByModel...), initial.CostByKey...) {
		var sum float64
		for _, value := range entry.Values {
			sum += value
		}
		if math.Abs(sum-entry.TotalUSD) > 1e-9 {
			t.Fatalf("breakdown %q total %v != sum %v", entry.Key, entry.TotalUSD, sum)
		}
	}
}

// TestBuildCostBreakdownFoldsAndAligns pins the histogram contract: series are
// ordered by total, capped, and everything past the cap is folded into a single
// trailing Others entry whose per-bucket values are the exact sum of the folded
// series.
func TestBuildCostBreakdownFoldsAndAligns(t *testing.T) {
	buckets := []int64{100, 200, 300}
	costs := map[string]map[int64]float64{}
	for index := 0; index < 9; index++ {
		key := string(rune('a' + index))
		amount := float64(9 - index) // a=9 .. i=1, so the order is unambiguous
		costs[key] = map[int64]float64{buckets[0]: amount, buckets[2]: amount / 2}
	}
	entries := buildCostBreakdown(costs, buckets, costBreakdownSeriesLimit)
	if len(entries) != costBreakdownSeriesLimit+1 {
		t.Fatalf("entries = %d, want %d", len(entries), costBreakdownSeriesLimit+1)
	}
	if entries[0].Key != "a" || math.Abs(entries[0].TotalUSD-13.5) > 1e-9 {
		t.Fatalf("first entry = %+v", entries[0])
	}
	last := entries[len(entries)-1]
	if !last.Others || last.Key != "" {
		t.Fatalf("trailing entry must be the folded remainder: %+v", last)
	}
	// Totals run 13.5, 12, ... 1.5 (a..i), so a..g are kept and h+i fold into
	// one entry: 3 + 1.5 = 4.5 USD.
	if math.Abs(last.TotalUSD-4.5) > 1e-9 {
		t.Fatalf("others total = %v, want 4.5", last.TotalUSD)
	}
	if len(last.Values) != 3 || math.Abs(last.Values[0]-3) > 1e-9 || last.Values[1] != 0 || math.Abs(last.Values[2]-1.5) > 1e-9 {
		t.Fatalf("others values = %v", last.Values)
	}
	for _, entry := range entries {
		if len(entry.Values) != len(buckets) {
			t.Fatalf("entry %q has %d values, want %d", entry.Key, len(entry.Values), len(buckets))
		}
	}
	// Ties are broken by key so the payload never reshuffles between requests.
	ties := buildCostBreakdown(map[string]map[int64]float64{
		"zeta":  {buckets[0]: 2},
		"alpha": {buckets[0]: 2},
	}, buckets, costBreakdownSeriesLimit)
	if len(ties) != 2 || ties[0].Key != "alpha" || ties[1].Key != "zeta" {
		t.Fatalf("tie order = %+v", ties)
	}
	// A window with nothing priced yields no series at all.
	if got := buildCostBreakdown(map[string]map[int64]float64{"free": {buckets[0]: 0}}, buckets, costBreakdownSeriesLimit); got != nil {
		t.Fatalf("zero-cost breakdown = %+v, want nil", got)
	}
	if got := buildCostBreakdown(nil, buckets, costBreakdownSeriesLimit); got != nil {
		t.Fatalf("empty breakdown = %+v, want nil", got)
	}
	// A fine axis keeps fewer series so the bars stay distinguishable.
	fineBuckets := make([]int64, costBreakdownFineBuckets+1)
	for index := range fineBuckets {
		fineBuckets[index] = int64(index)
	}
	fineCosts := map[string]map[int64]float64{}
	for index := 0; index < 9; index++ {
		fineCosts[string(rune('a'+index))] = map[int64]float64{0: float64(9 - index)}
	}
	if got := buildCostBreakdown(fineCosts, fineBuckets, costBreakdownFineSeriesLimit); len(got) != costBreakdownFineSeriesLimit+1 {
		t.Fatalf("fine breakdown = %d entries, want %d", len(got), costBreakdownFineSeriesLimit+1)
	}
}

// TestCostBreakdownHonoursFilters keeps the histogram consistent with the rest
// of the cost tab: the split is built from the filtered rows only.
func TestCostBreakdownHonoursFilters(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	hour := now.Add(-2 * time.Hour).Truncate(time.Hour)
	hashA := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	data := map[aggregateKey]Counters{
		{Hour: hour.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "alpha", APIKeyHash: hashA, APIKeyGeneration: 1}}: {
			Requests: 1, InputTokens: 1_000_000, TotalTokens: 1_000_000,
		},
		{Hour: hour.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "beta", APIKeyHash: hashB, APIKeyGeneration: 1}}: {
			Requests: 1, InputTokens: 2_000_000, TotalTokens: 2_000_000,
		},
	}
	pricer := stubPricer{prices: map[string]ModelPrice{
		"alpha": priceOf(1, mathNaN(), mathNaN(), mathNaN()),
		"beta":  priceOf(2, mathNaN(), mathNaN(), mathNaN()),
	}}
	queryRange := usageRange{Name: "custom", Start: hour, End: now}
	spec := seriesSpec{Scale: seriesScaleHour, BucketSeconds: seriesHourSeconds, WindowStart: hour, WindowEnd: now}

	all := buildInitialStatsForRange(data, now.Add(-time.Hour), now, queryRange, usageFilter{}, now, nil, pricer, spec)
	if len(all.CostByModel) != 2 || len(all.CostByKey) != 2 {
		t.Fatalf("unfiltered breakdown = %d models / %d keys", len(all.CostByModel), len(all.CostByKey))
	}
	if all.CostByModel[0].Key != "beta" || math.Abs(all.CostByModel[0].TotalUSD-4) > 1e-9 {
		t.Fatalf("unfiltered first model = %+v", all.CostByModel[0])
	}

	filtered := buildInitialStatsForRange(data, now.Add(-time.Hour), now, queryRange, usageFilter{Models: []string{"alpha"}}, now, nil, pricer, spec)
	if len(filtered.CostByModel) != 1 || filtered.CostByModel[0].Key != "alpha" {
		t.Fatalf("filtered models = %+v", filtered.CostByModel)
	}
	if len(filtered.CostByKey) != 1 || filtered.CostByKey[0].Key != apiKeyRef(1, hashA) {
		t.Fatalf("filtered keys = %+v", filtered.CostByKey)
	}
	if math.Abs(filtered.CostByModel[0].TotalUSD-1) > 1e-9 {
		t.Fatalf("filtered total = %v, want 1", filtered.CostByModel[0].TotalUSD)
	}
	// The cost series itself must agree with the folded breakdown.
	var seriesTotal float64
	for _, point := range filtered.CostSeries {
		seriesTotal += point.CostUSD
	}
	if math.Abs(seriesTotal-filtered.CostByModel[0].TotalUSD) > 1e-9 {
		t.Fatalf("filtered series %v != breakdown %v", seriesTotal, filtered.CostByModel[0].TotalUSD)
	}
}

// TestBuildInitialStatsMinuteScaleKeepsRequestTotals guards the fine-grained
// axis: the minute path must still report the same window totals as the coarse
// axis, only spread over more buckets.
func TestBuildInitialStatsMinuteScaleKeepsRequestTotals(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC)
	first := now.Add(-90 * time.Minute)
	data := map[aggregateKey]Counters{
		{Hour: first.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "m"}}:                       {Requests: 2, TotalTokens: 20, TotalLatencyNS: uint64(2 * time.Second), LatencySamples: 2, TotalTTFTNS: uint64(400 * time.Millisecond), TTFTSamples: 2},
		{Hour: first.Add(30 * time.Minute).Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "m"}}: {Requests: 3, TotalTokens: 30, TotalLatencyNS: uint64(3 * time.Second), LatencySamples: 3, TotalTTFTNS: uint64(600 * time.Millisecond), TTFTSamples: 3},
	}
	queryRange := usageRange{Name: "custom", Start: first, End: now}
	spec := seriesSpec{Scale: seriesScaleMinute, BucketSeconds: seriesMinuteSeconds, WindowStart: first, WindowEnd: now}
	initial := buildInitialStatsForRange(data, first, now, queryRange, usageFilter{}, now, nil, nil, spec)

	if initial.Scale != seriesScaleMinute || initial.BucketSeconds != 60 {
		t.Fatalf("minute spec not honoured: %q/%d", initial.Scale, initial.BucketSeconds)
	}
	if len(initial.Series) != 90 || len(initial.CostSeries) != 90 {
		t.Fatalf("minute buckets = %d/%d, want 90", len(initial.Series), len(initial.CostSeries))
	}
	var requests, tokens uint64
	for _, point := range initial.Series {
		requests += point.Requests
		tokens += point.TotalTokens
	}
	if requests != 5 || tokens != 50 {
		t.Fatalf("minute series totals = %d requests / %d tokens, want 5/50", requests, tokens)
	}
	if initial.Summary.Requests != 5 || initial.Summary.TotalTokens != 50 {
		t.Fatalf("minute summary = %+v", initial.Summary)
	}
	if initial.Models[0].AverageTTFTNS != uint64(200*time.Millisecond) || initial.Models[0].TTFTSamples != 5 {
		t.Fatalf("minute model averages = %+v", initial.Models[0])
	}
}

// TestSeriesBucketLimitCapsFineScales keeps the axis bounded when an operator
// asks for a minute scale over a very long window.
func TestSeriesBucketLimitCapsFineScales(t *testing.T) {
	if seriesBucketLimit(seriesScaleMinute) != 1440 || seriesBucketLimit(seriesScaleHour) != 1440 || seriesBucketLimit(seriesScaleDay) != 800 {
		t.Fatalf("bucket limits = %d/%d/%d", seriesBucketLimit(seriesScaleMinute), seriesBucketLimit(seriesScaleHour), seriesBucketLimit(seriesScaleDay))
	}
}
