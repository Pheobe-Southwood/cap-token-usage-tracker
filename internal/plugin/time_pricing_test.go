package plugin

import (
	"math"
	"testing"
	"time"
)

func at(t *testing.T, value string) time.Time {
	t.Helper()
	stamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %s: %v", value, err)
	}
	return stamp
}

func weekdayTier(name, start, end string, days ...int) TimePriceTier {
	return TimePriceTier{
		Name:      name,
		Days:      days,
		Start:     start,
		End:       end,
		Input:     timeRate(0.30),
		Output:    timeRate(1.20),
		CacheRead: timeRate(0.006),
	}
}

func TestSelectTimeTierBoundaries(t *testing.T) {
	tiers := []TimePriceTier{
		weekdayTier("peak-morning", "01:00", "04:00", 1, 2, 3, 4, 5),
		weekdayTier("peak-day", "06:00", "10:00", 1, 2, 3, 4, 5),
	}
	for _, testCase := range []struct {
		stamp string
		hit   bool
	}{
		{"2026-10-19T00:59:00Z", false}, // Monday, just before the window
		{"2026-10-19T01:00:00Z", true},
		{"2026-10-19T03:59:59Z", true},
		{"2026-10-19T04:00:00Z", false},
		{"2026-10-19T05:59:00Z", false},
		{"2026-10-19T06:00:00Z", true},
		{"2026-10-19T09:59:59Z", true},
		{"2026-10-19T10:00:00Z", false},
		{"2026-10-17T02:00:00Z", false}, // Saturday
		{"2026-10-18T02:00:00Z", false}, // Sunday
		{"2026-10-16T02:00:00Z", true},  // Friday
	} {
		_, hit := selectTimeTier("UTC", tiers, at(t, testCase.stamp))
		if hit != testCase.hit {
			t.Errorf("%s hit = %v, want %v", testCase.stamp, hit, testCase.hit)
		}
	}
	if _, hit := selectTimeTier("", tiers, at(t, "2026-10-19T02:00:00Z")); !hit {
		t.Fatal("an empty zone must fall back to UTC")
	}
	if _, hit := selectTimeTier("UTC", nil, at(t, "2026-10-19T02:00:00Z")); hit {
		t.Fatal("no tiers must never match")
	}
	if _, hit := selectTimeTier("UTC", tiers, time.Time{}); hit {
		t.Fatal("a zero instant must never match")
	}
	if _, hit := selectTimeTier("Local", tiers, at(t, "2026-10-19T02:00:00Z")); hit {
		t.Fatal("the machine zone must never match")
	}
	if _, hit := selectTimeTier("bad/zone", tiers, at(t, "2026-10-19T02:00:00Z")); hit {
		t.Fatal("an unknown zone must never match")
	}
	if _, hit := selectTimeTier("UTC", []TimePriceTier{{Name: "bad", Start: "24:00", End: "25:00"}}, at(t, "2026-10-19T02:00:00Z")); hit {
		t.Fatal("a malformed stored tier must never match")
	}
}

func TestSelectTimeTierOvernightBelongsToStartDay(t *testing.T) {
	tiers := []TimePriceTier{weekdayTier("night", "23:00", "08:00", 7)}
	for _, testCase := range []struct {
		stamp string
		hit   bool
	}{
		{"2026-10-18T22:59:00+08:00", false}, // Sunday before the window
		{"2026-10-18T23:00:00+08:00", true},
		{"2026-10-19T07:59:59+08:00", true}, // Monday morning belongs to Sunday
		{"2026-10-19T08:00:00+08:00", false},
		{"2026-10-17T23:30:00+08:00", false}, // Saturday night is not Sunday
	} {
		_, hit := selectTimeTier("Asia/Shanghai", tiers, at(t, testCase.stamp))
		if hit != testCase.hit {
			t.Errorf("%s hit = %v, want %v", testCase.stamp, hit, testCase.hit)
		}
	}
}

func TestSelectTimeTierAcrossDaylightSaving(t *testing.T) {
	// 2026-11-01 in New York repeats 01:00-02:00; both passes must match.
	repeated := []TimePriceTier{weekdayTier("repeat", "01:00", "02:00")}
	for _, stamp := range []string{"2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z"} {
		if _, hit := selectTimeTier("America/New_York", repeated, at(t, stamp)); !hit {
			t.Fatalf("repeated DST hour %s did not match", stamp)
		}
	}
	// 2026-03-08 skips 02:00-03:00; the surrounding instants must not match.
	skipped := []TimePriceTier{weekdayTier("skipped", "02:00", "03:00")}
	for _, stamp := range []string{"2026-03-08T06:59:00Z", "2026-03-08T07:00:00Z"} {
		if _, hit := selectTimeTier("America/New_York", skipped, at(t, stamp)); hit {
			t.Fatalf("skipped DST hour %s matched", stamp)
		}
	}
}

func TestNormalizeTimePricing(t *testing.T) {
	valid := ModelPrice{TimeZone: "Asia/Shanghai", TimeTiers: []TimePriceTier{weekdayTier("night", "23:00", "08:00", 1)}}
	if _, err := normalizeModelPrice("m", valid); err != nil {
		t.Fatalf("valid schedule rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ModelPrice){
		"zone":           func(p *ModelPrice) { p.TimeZone = "bad/zone" },
		"local":          func(p *ModelPrice) { p.TimeZone = "Local" },
		"clock":          func(p *ModelPrice) { p.TimeTiers[0].Start = "24:00" },
		"short clock":    func(p *ModelPrice) { p.TimeTiers[0].End = "8:00" },
		"equal":          func(p *ModelPrice) { p.TimeTiers[0].End = "23:00" },
		"day zero":       func(p *ModelPrice) { p.TimeTiers[0].Days = []int{0} },
		"day eight":      func(p *ModelPrice) { p.TimeTiers[0].Days = []int{8} },
		"duplicate day":  func(p *ModelPrice) { p.TimeTiers[0].Days = []int{1, 1} },
		"empty name":     func(p *ModelPrice) { p.TimeTiers[0].Name = "  " },
		"duplicate name": func(p *ModelPrice) { p.TimeTiers = append(p.TimeTiers, weekdayTier("night", "09:00", "10:00")) },
		"newline name":   func(p *ModelPrice) { p.TimeTiers[0].Name = "bad\nname" },
		"nan rate":       func(p *ModelPrice) { p.TimeTiers[0].Input = timeRate(math.NaN()) },
		"negative rate":  func(p *ModelPrice) { p.TimeTiers[0].Output = timeRate(-1) },
		"huge rate":      func(p *ModelPrice) { p.TimeTiers[0].Input = timeRate(maxModelPriceValue + 1) },
		"overlap": func(p *ModelPrice) {
			p.TimeTiers = append(p.TimeTiers, weekdayTier("next", "23:30", "23:45", 1))
		},
		"overnight overlap": func(p *ModelPrice) {
			p.TimeTiers = []TimePriceTier{
				weekdayTier("night", "23:00", "08:00", 1),
				weekdayTier("next", "07:00", "09:00", 2),
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			price := ModelPrice{TimeZone: "Asia/Shanghai", TimeTiers: []TimePriceTier{weekdayTier("night", "23:00", "08:00", 1)}}
			mutate(&price)
			if _, err := normalizeModelPrice("m", price); err == nil {
				t.Fatal("accepted an invalid schedule")
			}
		})
	}
	if _, err := normalizeModelPrice("m", ModelPrice{TimeTiers: make([]TimePriceTier, maxTimePriceTiers+1)}); err == nil {
		t.Fatal("accepted too many tiers")
	}

	// A schedule without a zone defaults to UTC; a zone without a schedule is
	// dropped so an empty override stays empty.
	normalized, err := normalizeModelPrice("m", ModelPrice{TimeTiers: []TimePriceTier{weekdayTier("night", "23:00", "08:00")}})
	if err != nil || normalized.TimeZone != "UTC" {
		t.Fatalf("zone default = %+v, %v", normalized, err)
	}
	normalized, err = normalizeModelPrice("m", ModelPrice{TimeZone: "Asia/Shanghai"})
	if err != nil || normalized.TimeZone != "" || !normalized.Empty() {
		t.Fatalf("zone without schedule = %+v, %v", normalized, err)
	}

	// Tiers are deep copied so callers cannot alias the stored weekday slices.
	original := ModelPrice{TimeZone: "UTC", TimeTiers: []TimePriceTier{weekdayTier("night", "23:00", "08:00", 1)}}
	cloned := cloneModelPriceOverrides(map[string]ModelPrice{"m": original})
	mutated := cloned["m"]
	mutated.TimeTiers[0].Days[0] = 5
	if original.TimeTiers[0].Days[0] != 1 {
		t.Fatal("cloneModelPriceOverrides shares weekday slices")
	}
}

func TestBuiltinTimePlansAreValid(t *testing.T) {
	for model, plan := range builtinTimePlans {
		if _, err := normalizeModelPrice(model, plan); err != nil {
			t.Fatalf("builtin plan %s is invalid: %v", model, err)
		}
	}
}

func TestNormalizeTimePlanModelID(t *testing.T) {
	for input, want := range map[string]string{
		"DeepSeek-V4.1-Flash":          "deepseek-v4.1-flash",
		"deepseek/deepseek-v4.1-flash": "deepseek-v4.1-flash",
		"deepseek-v4.1-flash:free":     "deepseek-v4.1-flash",
		"openai/gpt-6-luna@20261017":   "gpt-6-luna",
		"deepseek-v4-pro":              "deepseek-v4-pro",
		"  DeepSeek-V4-Pro  ":          "deepseek-v4-pro",
		"a/b/c":                        "c",
		"":                             "",
	} {
		if got := normalizeTimePlanModelID(input); got != want {
			t.Errorf("normalizeTimePlanModelID(%q) = %q, want %q", input, got, want)
		}
	}
}
