package plugin

import (
	"math"
	"testing"
	"time"
)

// TestChargeForAppliesTimeTier pins the cost contract: a matched tier replaces
// the base rates field by field, and the reported surcharge is the signed
// difference against the flat base price.
func TestChargeForAppliesTimeTier(t *testing.T) {
	table := testPricingTable(t, pricingUpstream, nil)
	dimensions := Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"}
	counters := Counters{Requests: 2, InputTokens: 1_000_000, OutputTokens: 1_000_000, TotalTokens: 2_000_000}
	resolved := table.PriceFor(dimensions)
	if resolved.TimePlanSource != priceSourceBuiltin || len(resolved.TimeTiers) != 2 || resolved.TimeZone != "UTC" {
		t.Fatalf("bundled schedule not resolved: %+v", resolved)
	}
	if resolved.Price.TimeTiers != nil || resolved.Price.TimeZone != "" {
		t.Fatalf("schedule leaked into the base rates: %+v", resolved.Price)
	}

	// Catalog base 0.15/0.60; the bundled peak tariff is 0.30/1.20.
	amount, surcharge, priced := chargeFor(resolved, dimensions, counters, testPeak)
	if !priced || math.Abs(amount-1.5) > 1e-9 || math.Abs(surcharge-0.75) > 1e-9 {
		t.Fatalf("peak charge = %v surcharge %v priced %v", amount, surcharge, priced)
	}
	amount, surcharge, priced = chargeFor(resolved, dimensions, counters, testOffPeak)
	if !priced || math.Abs(amount-0.75) > 1e-9 || surcharge != 0 {
		t.Fatalf("off-peak charge = %v surcharge %v priced %v", amount, surcharge, priced)
	}

	// The concrete Cost helper agrees with chargeFor.
	if cost, priced := table.Cost(dimensions, counters, testPeak); !priced || math.Abs(cost-1.5) > 1e-9 {
		t.Fatalf("Cost at peak = %v, %v", cost, priced)
	}

	// An unpriced model stays unpriced even inside a peak window.
	if _, _, priced := chargeFor(resolvedPrice{Source: priceSourceNone}, Dimensions{Model: "Beam"}, counters, testPeak); priced {
		t.Fatal("unpriced model reported a cost")
	}
}

// TestBuiltinTimePlanGating pins when the bundled DeepSeek schedule applies.
func TestBuiltinTimePlanGating(t *testing.T) {
	const resellerUpstream = `{
  "opencode-go": {"api": "https://opencode.ai/zen/go/v1", "models": {
    "deepseek-v4.1-flash": {"cost": {"input": 0.15, "output": 0.6}}
  }},
  "openrouter": {"models": {
    "deepseek-v4.1-flash": {"cost": {"input": 0.5, "output": 1.0}}
  }}
}`
	table := testPricingTable(t, resellerUpstream, nil)
	opencode := Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"}
	if resolved := table.PriceFor(opencode); resolved.TimePlanSource != priceSourceBuiltin {
		t.Fatalf("opencode-go model lost the bundled plan: %+v", resolved)
	}
	reseller := Dimensions{Provider: "openrouter", Model: "deepseek-v4.1-flash"}
	resolved := table.PriceFor(reseller)
	if resolved.CatalogProvider != "openrouter" || resolved.TimePlanSource != "" || len(resolved.TimeTiers) != 0 {
		t.Fatalf("reseller inherited the OpenCode Go tariff: %+v", resolved)
	}
	if amount, surcharge, _ := chargeFor(resolved, reseller, Counters{InputTokens: 1_000_000}, testPeak); math.Abs(amount-0.5) > 1e-9 || surcharge != 0 {
		t.Fatalf("reseller charge = %v surcharge %v", amount, surcharge)
	}

	// A model outside the bundled table is never affected.
	gemini := Dimensions{Provider: "antigravity", Model: "gemini-3.8-flash-high"}
	if resolved := table.PriceFor(gemini); resolved.TimePlanSource != "" {
		t.Fatalf("unrelated model gained a schedule: %+v", resolved)
	}

	// The global switch disables the bundled schedule only.
	table.SetBuiltinTimePlans(false)
	disabled := table.PriceFor(opencode)
	if disabled.TimePlanSource != "" || len(disabled.TimeTiers) != 0 {
		t.Fatalf("bundled plan survived the switch: %+v", disabled)
	}
	if amount, surcharge, _ := chargeFor(disabled, opencode, Counters{InputTokens: 1_000_000}, testPeak); math.Abs(amount-0.15) > 1e-9 || surcharge != 0 {
		t.Fatalf("disabled charge = %v surcharge %v", amount, surcharge)
	}
	table.SetBuiltinTimePlans(true)
	if resolved := table.PriceFor(opencode); resolved.TimePlanSource != priceSourceBuiltin {
		t.Fatalf("re-enabling did not clear the memo: %+v", resolved)
	}
}

// TestManualScheduleOverridesBuiltinPlan pins the precedence between a stored
// schedule and the bundled one, including a rate-only override.
func TestManualScheduleOverridesBuiltinPlan(t *testing.T) {
	table := testPricingTable(t, pricingUpstream, nil)
	dimensions := Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"}

	// A rate-only override keeps the bundled schedule.
	table.SetOverrides(map[string]ModelPrice{"deepseek-v4.1-flash": priceOf(0.2, 0.8, math.NaN(), math.NaN())})
	resolved := table.PriceFor(dimensions)
	if resolved.TimePlanSource != priceSourceBuiltin || priceValue(resolved.Price.Input) != 0.2 {
		t.Fatalf("rate-only override dropped the bundled plan: %+v", resolved)
	}

	// A stored schedule replaces it entirely.
	table.SetOverrides(map[string]ModelPrice{"deepseek-v4.1-flash": {
		Input:     timeRate(0.2),
		Output:    timeRate(0.8),
		TimeZone:  "Asia/Shanghai",
		TimeTiers: []TimePriceTier{weekdayTier("night", "23:00", "08:00", 1)},
	}})
	resolved = table.PriceFor(dimensions)
	if resolved.TimePlanSource != priceSourceOverride || resolved.TimeZone != "Asia/Shanghai" || len(resolved.TimeTiers) != 1 {
		t.Fatalf("manual schedule did not win: %+v", resolved)
	}
	if amount, _, _ := chargeFor(resolved, dimensions, Counters{InputTokens: 1_000_000}, testPeak); math.Abs(amount-0.2) > 1e-9 {
		t.Fatalf("manual schedule charge = %v, want the base rate outside its window", amount)
	}
	night := at(t, "2026-10-19T23:30:00+08:00") // Monday night in Shanghai
	if amount, surcharge, _ := chargeFor(resolved, dimensions, Counters{InputTokens: 1_000_000}, night); math.Abs(amount-0.30) > 1e-9 || math.Abs(surcharge-0.10) > 1e-9 {
		t.Fatalf("manual schedule night charge = %v surcharge %v", amount, surcharge)
	}
}

// TestBuildInitialStatsTimeTierSummary pins the summary fields the cost tab
// renders: the window total includes the schedule and the delta is reported
// separately, bucket by bucket.
func TestBuildInitialStatsTimeTierSummary(t *testing.T) {
	now := time.Date(2026, 10, 19, 5, 30, 0, 0, time.UTC)
	peak := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	off := time.Date(2026, 10, 19, 5, 0, 0, 0, time.UTC)
	data := map[aggregateKey]Counters{
		{Hour: peak.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"}}: {
			Requests: 2, InputTokens: 1_000_000, OutputTokens: 1_000_000, TotalTokens: 2_000_000,
		},
		{Hour: off.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"}}: {
			Requests: 3, InputTokens: 1_000_000, OutputTokens: 1_000_000, TotalTokens: 2_000_000,
		},
	}
	pricer := stubPricer{
		prices: map[string]ModelPrice{"deepseek-v4.1-flash": priceOf(0.15, 0.6, math.NaN(), math.NaN())},
		plan:   &ModelPrice{TimeZone: "UTC", TimeTiers: []TimePriceTier{weekdayTier("peak", "01:00", "04:00", 1, 2, 3, 4, 5)}},
	}
	queryRange := usageRange{Name: "custom", Start: peak.Add(-time.Hour), End: now}
	spec := seriesSpec{Scale: seriesScaleHour, BucketSeconds: seriesHourSeconds, WindowStart: peak.Truncate(time.Hour), WindowEnd: now.Truncate(time.Hour).Add(time.Hour)}
	initial := buildInitialStatsForRange(data, now.Add(-30*24*time.Hour), now, queryRange, usageFilter{}, now, nil, pricer, spec)

	// 0.75 flat per bucket; the peak bucket doubles to 1.50.
	if math.Abs(initial.Cost.CostUSD-2.25) > 1e-9 {
		t.Fatalf("window cost = %v, want 2.25", initial.Cost.CostUSD)
	}
	if math.Abs(initial.Cost.TimeTierCostUSD-0.75) > 1e-9 || initial.Cost.TimeTierRequests != 2 {
		t.Fatalf("time tier delta = %+v", initial.Cost)
	}
	var seriesTotal float64
	for _, point := range initial.CostSeries {
		seriesTotal += point.CostUSD
	}
	if math.Abs(seriesTotal-2.25) > 1e-9 {
		t.Fatalf("cost series total = %v, want 2.25", seriesTotal)
	}
	if len(initial.CostByModel) != 1 || math.Abs(initial.CostByModel[0].TotalUSD-2.25) > 1e-9 {
		t.Fatalf("model breakdown = %+v", initial.CostByModel)
	}
}

// TestBuildModelsTimePlanExposure pins what the models tab needs to render the
// peak badge and to prefill the price dialog.
func TestBuildModelsTimePlanExposure(t *testing.T) {
	now := time.Date(2026, 10, 19, 5, 30, 0, 0, time.UTC)
	peak := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	data := map[aggregateKey]Counters{
		{Hour: peak.Unix(), Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"}}: {
			Requests: 2, InputTokens: 1_000_000, OutputTokens: 1_000_000, TotalTokens: 2_000_000,
		},
	}
	table := testPricingTable(t, pricingUpstream, nil)
	queryRange := usageRange{Name: "custom", Start: peak.Add(-time.Hour), End: now}
	response := buildModelsForRange(data, now.Add(-30*24*time.Hour), now, queryRange, usageFilter{}, now, table, nil, nil, nil)
	if len(response.Models) != 1 {
		t.Fatalf("models = %+v", response.Models)
	}
	row := response.Models[0]
	if math.Abs(row.CostUSD-1.5) > 1e-9 {
		t.Fatalf("model cost = %v, want the peak charge 1.5", row.CostUSD)
	}
	if row.Pricing.TimePlanSource != priceSourceBuiltin || len(row.Pricing.TimeTiers) != 2 || row.Pricing.TimeZone != "UTC" {
		t.Fatalf("model schedule = %+v", row.Pricing)
	}
	if math.Abs(response.Summary.CostUSD-1.5) > 1e-9 {
		t.Fatalf("summary cost = %v", response.Summary.CostUSD)
	}
}

// TestProductionCatalogResolvesBundledPlans pins the parity contract against the
// real embedded models.dev snapshot: the DeepSeek rows the host reports must
// resolve to the OpenCode Go catalog entry and pick up the bundled peak plan,
// while a model the sibling prices flat stays flat.
func TestProductionCatalogResolvesBundledPlans(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	table, err := newPricingTableFor(config, store)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		model     string
		provider  string
		catalog   string
		plan      string
		tiers     int
		peakInput float64
	}{
		{model: "DeepSeek-V4.1-Flash", provider: "cap-opencode-go", catalog: "opencode-go", plan: priceSourceBuiltin, tiers: 2, peakInput: 0.30},
		{model: "deepseek-v4.1-flash", provider: "openai-compatible-opencode go", catalog: "opencode-go", plan: priceSourceBuiltin, tiers: 2, peakInput: 0.30},
		{model: "deepseek-v4-pro", provider: "cap-opencode-go", catalog: "opencode-go", plan: priceSourceBuiltin, tiers: 2, peakInput: 1.32},
		{model: "deepseek-flash", provider: "cap-opencode-go", catalog: "deepseek", plan: "", tiers: 0},
	} {
		dimensions := Dimensions{Provider: testCase.provider, Model: testCase.model}
		resolved := table.PriceFor(dimensions)
		if resolved.CatalogProvider != testCase.catalog {
			t.Fatalf("%s: catalog provider = %q, want %q", testCase.model, resolved.CatalogProvider, testCase.catalog)
		}
		if resolved.TimePlanSource != testCase.plan || len(resolved.TimeTiers) != testCase.tiers {
			t.Fatalf("%s: plan = %q with %d tiers, want %q with %d", testCase.model, resolved.TimePlanSource, len(resolved.TimeTiers), testCase.plan, testCase.tiers)
		}
		if testCase.tiers == 0 {
			continue
		}
		_, surcharge, priced := chargeFor(resolved, dimensions, Counters{InputTokens: 1_000_000}, testPeak)
		if !priced || math.Abs(surcharge-(testCase.peakInput-priceValue(resolved.Price.Input))) > 1e-9 {
			t.Fatalf("%s: peak surcharge = %v, want %v", testCase.model, surcharge, testCase.peakInput-priceValue(resolved.Price.Input))
		}
	}
}
