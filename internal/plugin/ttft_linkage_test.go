package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSiblingFixture materializes a sibling opencode-go state directory next
// to a fake database path and returns the resolved Config.
func writeSiblingFixture(t *testing.T, shared map[string]any, stats map[string]any) Config {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, defaultSiblingStateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if shared != nil {
		raw, err := json.Marshal(shared)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, siblingSharedFileName), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, payload := range stats {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, siblingStatsPrefix+name+siblingStatsSuffix), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := defaultConfig()
	config.DataPath = filepath.Join(root, "token-usage-tracker.db")
	config.TTFTSupplement = true
	return config
}

func siblingShared(models map[string]any) map[string]any {
	return map[string]any{
		"version":    "1.11.0",
		"written_at": "2026-10-17T14:03:05Z",
		"settings":   map[string]any{"models": models},
	}
}

func siblingStats(role string, writtenAt string, models map[string]any) map[string]any {
	return map[string]any{
		"version":    siblingStatsVersion,
		"role":       role,
		"written_at": writtenAt,
		"models":     models,
	}
}

func siblingModel(hours map[string]any) map[string]any {
	return map[string]any{"hours": hours}
}

func TestTTFTLinkageSupplementsPluginExecutorSamples(t *testing.T) {
	hour := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	nextHour := hour.Add(time.Hour)
	config := writeSiblingFixture(t,
		siblingShared(map[string]any{
			"beam-free":     map[string]any{"aliases": []string{"Beam"}},
			"qwen3.8-flash": map[string]any{"aliases": []string{"Qwen3.8-Flash"}},
		}),
		map[string]any{"chat": siblingStats("chat", "2026-10-17T14:03:05Z", map[string]any{
			"beam-free": siblingModel(map[string]any{
				"2026-10-17T12": map[string]any{"requests": 5, "ttft_samples": 5, "ttft_nano": int64(5 * time.Second)},
				"2026-10-17T13": map[string]any{"requests": 2, "ttft_samples": 2, "ttft_nano": int64(2 * time.Second)},
			}),
			"qwen3.8-flash": siblingModel(map[string]any{
				"2026-10-17T12": map[string]any{"requests": 4, "ttft_samples": 4, "ttft_nano": int64(4 * time.Second)},
			}),
		})},
	)
	linkage := newTTFTLinkageCache().snapshot(config)
	if !linkage.usable() {
		t.Fatalf("linkage should be usable: %+v", linkage)
	}
	if linkage.Provider != defaultSiblingStateDirName {
		t.Fatalf("provider prefix = %q", linkage.Provider)
	}

	data := map[aggregateKey]Counters{
		// Plugin executor rows: the host reports no TTFT for them.
		{Hour: hour.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "Beam"}}:                 {Requests: 5, TotalTokens: 50},
		{Hour: nextHour.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "Beam"}}:             {Requests: 2, TotalTTFTNS: uint64(2 * time.Second), TTFTSamples: 2},
		{Hour: hour.Unix(), Dimensions: Dimensions{Provider: "openai-compatible-基元律动", Model: "Qwen3.8-Flash"}}: {Requests: 4, TotalTokens: 40},
	}
	response := buildModelsForRange(data, hour.Add(-24*time.Hour), hour, usageRange{Name: "24h", Start: hour.Add(-24 * time.Hour)}, usageFilter{}, hour.Add(time.Hour), nil, nil, nil, linkage)

	byModel := map[string]ModelPricingInfo{}
	for _, row := range response.Models {
		byModel[row.Model] = row
	}
	beam := byModel["Beam"]
	if beam.SupplementedTTFTSamples != 5 || beam.TTFTSamples != 7 || beam.TotalTTFTNS != uint64(7*time.Second) {
		t.Fatalf("beam linkage = %+v", beam)
	}
	if beam.AverageTTFTNS != uint64(time.Second) {
		t.Fatalf("beam average ttft = %d", beam.AverageTTFTNS)
	}
	// The second hour already had matching samples: no double counting.
	if response.TTFTSupplement.Models != 1 || response.TTFTSupplement.Samples != 5 {
		t.Fatalf("supplement status = %+v", response.TTFTSupplement)
	}
	if !response.TTFTSupplement.Available || len(response.TTFTSupplement.Sources) == 0 {
		t.Fatalf("supplement availability = %+v", response.TTFTSupplement)
	}
	// A same-named model on a different channel must never borrow the sibling's
	// samples, even though the alias table maps it.
	if row := byModel["Qwen3.8-Flash"]; row.SupplementedTTFTSamples != 0 || row.TTFTSamples != 0 {
		t.Fatalf("foreign provider linkage = %+v", row)
	}
}

func TestTTFTLinkageCapsSamplesAtMissingRequests(t *testing.T) {
	hour := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	config := writeSiblingFixture(t,
		siblingShared(map[string]any{"gpt-6-luna": map[string]any{}}),
		map[string]any{"responses": siblingStats("responses", "2026-10-17T14:03:05Z", map[string]any{
			"gpt-6-luna": siblingModel(map[string]any{
				"2026-10-17T12": map[string]any{"requests": 10, "ttft_samples": 10, "ttft_nano": int64(10 * time.Second)},
			}),
		})},
	)
	linkage := newTTFTLinkageCache().snapshot(config)
	data := map[aggregateKey]Counters{
		{Hour: hour.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go-responses", Model: "gpt-6-luna"}}: {Requests: 3, TotalTokens: 30},
	}
	response := buildModelsForRange(data, hour.Add(-24*time.Hour), hour, usageRange{Name: "24h", Start: hour.Add(-24 * time.Hour)}, usageFilter{}, hour.Add(time.Hour), nil, nil, nil, linkage)
	if len(response.Models) != 1 {
		t.Fatalf("models = %+v", response.Models)
	}
	row := response.Models[0]
	if row.SupplementedTTFTSamples != 3 || row.TTFTSamples != 3 || row.TotalTTFTNS != uint64(3*time.Second) {
		t.Fatalf("capped linkage = %+v", row)
	}
}

func TestTTFTLinkageDegradesWithoutSiblingFiles(t *testing.T) {
	config := writeSiblingFixture(t, nil, nil)
	linkage := newTTFTLinkageCache().snapshot(config)
	if linkage.usable() || linkage.Err == "" {
		t.Fatalf("missing sibling files = %+v", linkage)
	}
	if linkage.status().Available {
		t.Fatalf("status should report unavailable: %+v", linkage.status())
	}

	// Version drift is refused instead of guessing the schema.
	config = writeSiblingFixture(t,
		siblingShared(map[string]any{"beam-free": map[string]any{"aliases": []string{"Beam"}}}),
		map[string]any{"chat": map[string]any{
			"version": 99,
			"role":    "chat",
			"models":  map[string]any{"beam-free": siblingModel(map[string]any{"2026-10-17T12": map[string]any{"ttft_samples": 5}})},
		}},
	)
	linkage = newTTFTLinkageCache().snapshot(config)
	if linkage.usable() {
		t.Fatalf("unsupported version must not be linked: %+v", linkage)
	}
	if linkage.Err == "" {
		t.Fatal("version mismatch should be reported")
	}

	// The feature can be switched off entirely.
	disabled := config
	disabled.TTFTSupplement = false
	if linkage := newTTFTLinkageCache().snapshot(disabled); linkage.Enabled || linkage.usable() {
		t.Fatalf("disabled linkage = %+v", linkage)
	}
}

func TestTTFTLinkageCacheReusesUnchangedFiles(t *testing.T) {
	config := writeSiblingFixture(t,
		siblingShared(map[string]any{"beam-free": map[string]any{"aliases": []string{"Beam"}}}),
		map[string]any{"chat": siblingStats("chat", "2026-10-17T14:03:05Z", map[string]any{
			"beam-free": siblingModel(map[string]any{"2026-10-17T12": map[string]any{"ttft_samples": 5, "ttft_nano": int64(5 * time.Second)}}),
		})},
	)
	cache := newTTFTLinkageCache()
	first := cache.snapshot(config)
	second := cache.snapshot(config)
	if first != second {
		t.Fatal("unchanged files should reuse the memoized snapshot")
	}
	// Touching the stats file invalidates the memo.
	path := filepath.Join(config.resolvedTTFTSupplementDir(), siblingStatsPrefix+"chat"+siblingStatsSuffix)
	if err := os.WriteFile(path, []byte(`{"version":1,"role":"chat","models":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	third := cache.snapshot(config)
	if third == first {
		t.Fatal("changed files should rebuild the snapshot")
	}
	if third.usable() {
		t.Fatalf("empty statistics must not be usable: %+v", third)
	}
}

func TestResolveSeriesSpecAutoRules(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	since := now.Add(-365 * 24 * time.Hour)
	cases := []struct {
		name               string
		queryRange         usageRange
		requested          string
		tz                 int
		wantScale          string
		wantRequestedScale string
		wantBuckets        int
		wantWindowEnd      time.Time
	}{
		{name: "30d auto is day", queryRange: usageRange{Name: "30d", Start: now.Add(-30 * 24 * time.Hour)}, wantScale: seriesScaleDay, wantBuckets: 31},
		{name: "24h auto is hour", queryRange: usageRange{Name: "24h", Start: now.Add(-24 * time.Hour)}, wantScale: seriesScaleHour, wantBuckets: 24},
		{name: "90m auto is minute", queryRange: usageRange{Name: "custom", Start: now.Add(-90 * time.Minute), End: now}, wantScale: seriesScaleMinute, wantBuckets: 90},
		{name: "48h boundary is day", queryRange: usageRange{Name: "custom", Start: now.Add(-48 * time.Hour), End: now}, wantScale: seriesScaleDay, wantBuckets: 3},
		{name: "24h explicit minute fits cap", queryRange: usageRange{Name: "24h", Start: now.Add(-24 * time.Hour)}, requested: seriesScaleMinute, wantScale: seriesScaleMinute, wantBuckets: 1440},
		{name: "12h explicit minute fits cap", queryRange: usageRange{Name: "custom", Start: now.Add(-12 * time.Hour), End: now}, requested: seriesScaleMinute, wantScale: seriesScaleMinute, wantBuckets: 720},
		{name: "7d explicit minute clamped to hour", queryRange: usageRange{Name: "7d", Start: now.Add(-7 * 24 * time.Hour)}, requested: seriesScaleMinute, wantScale: seriesScaleHour, wantRequestedScale: seriesScaleMinute, wantBuckets: 168},
		{name: "30d explicit minute clamped to hour", queryRange: usageRange{Name: "30d", Start: now.Add(-30 * 24 * time.Hour)}, requested: seriesScaleMinute, wantScale: seriesScaleHour, wantRequestedScale: seriesScaleMinute, wantBuckets: 720},
		{name: "30d explicit hour fits cap", queryRange: usageRange{Name: "30d", Start: now.Add(-30 * 24 * time.Hour)}, requested: seriesScaleHour, wantScale: seriesScaleHour, wantBuckets: 720},
		{name: "90d explicit hour clamped to day", queryRange: usageRange{Name: "custom", Start: now.Add(-90 * 24 * time.Hour), End: now}, requested: seriesScaleHour, wantScale: seriesScaleDay, wantRequestedScale: seriesScaleHour, wantBuckets: 91},
		{name: "90d explicit minute clamped to day", queryRange: usageRange{Name: "custom", Start: now.Add(-90 * 24 * time.Hour), End: now}, requested: seriesScaleMinute, wantScale: seriesScaleDay, wantRequestedScale: seriesScaleMinute, wantBuckets: 91},
		{name: "explicit day kept", queryRange: usageRange{Name: "24h", Start: now.Add(-24 * time.Hour)}, requested: seriesScaleDay, wantScale: seriesScaleDay, wantBuckets: 2},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			spec := resolveSeriesSpec(testCase.requested, testCase.tz, testCase.queryRange, since, now)
			if spec.Scale != testCase.wantScale {
				t.Fatalf("scale = %q, want %q", spec.Scale, testCase.wantScale)
			}
			if spec.RequestedScale != testCase.wantRequestedScale {
				t.Fatalf("requested_scale = %q, want %q", spec.RequestedScale, testCase.wantRequestedScale)
			}
			starts := bucketStarts(spec.WindowStart, spec.WindowEnd, spec)
			if len(starts) != testCase.wantBuckets {
				t.Fatalf("buckets = %d, want %d (%v..%v)", len(starts), testCase.wantBuckets, spec.WindowStart, spec.WindowEnd)
			}
			if len(starts) > 0 {
				last := time.Unix(starts[len(starts)-1], 0).UTC().Add(time.Duration(spec.BucketSeconds) * time.Second)
				if last.Before(spec.WindowEnd) || last.Sub(spec.WindowEnd) >= time.Duration(spec.BucketSeconds)*time.Second {
					t.Fatalf("axis does not cover the window: last=%v end=%v", last, spec.WindowEnd)
				}
			}
		})
	}
}

func TestSeriesDayBucketsHonourTimezoneOffset(t *testing.T) {
	// 2026-10-17T18:30Z is 2026-10-18 02:30 in UTC+8, so the day bucket must
	// start at 2026-10-17T16:00Z (local midnight).
	unix := time.Date(2026, 10, 17, 18, 30, 0, 0, time.UTC).Unix()
	spec := seriesSpec{Scale: seriesScaleDay, BucketSeconds: seriesDaySeconds, TZOffsetMinutes: 480}
	got := bucketStartOf(unix, spec)
	want := time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC).Unix()
	if got != want {
		t.Fatalf("day bucket = %v, want %v", time.Unix(got, 0).UTC(), time.Unix(want, 0).UTC())
	}
	if bucketStartOf(unix, seriesSpec{Scale: seriesScaleHour, BucketSeconds: seriesHourSeconds}) != time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC).Unix() {
		t.Fatal("hour buckets must stay UTC aligned")
	}
}

func TestParseSeriesQueryParameters(t *testing.T) {
	if scale, err := parseSeriesScale(" Day "); err != nil || scale != seriesScaleDay {
		t.Fatalf("scale = %q, %v", scale, err)
	}
	if scale, err := parseSeriesScale(""); err != nil || scale != seriesScaleAuto {
		t.Fatalf("empty scale = %q, %v", scale, err)
	}
	if _, err := parseSeriesScale("week"); err == nil {
		t.Fatal("unknown scale must be rejected")
	}
	if offset, err := parseSeriesTZOffset("480"); err != nil || offset != 480 {
		t.Fatalf("tz = %d, %v", offset, err)
	}
	if _, err := parseSeriesTZOffset("900"); err == nil {
		t.Fatal("out-of-range tz must be rejected")
	}
	if _, err := parseSeriesTZOffset("abc"); err == nil {
		t.Fatal("non-numeric tz must be rejected")
	}
}
