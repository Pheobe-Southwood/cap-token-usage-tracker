package plugin

import "strings"

// builtinTimePlanProviders lists the catalog providers whose models may use a
// bundled schedule. It keeps a reseller that happens to publish the same model
// id from inheriting OpenCode Go's peak tariff.
var builtinTimePlanProviders = map[string]bool{
	"opencode-go": true,
	"deepseek":    true,
}

// builtinTimePlans are the vendor time-of-use schedules that ship with the
// plugin, keyed by normalizeTimePlanModelID. They mirror the cap-opencode-go
// plugin's DeepSeek peak rates (weekdays 01:00-04:00 and 06:00-10:00 UTC, twice
// the off-peak rate) so both plugins bill the same tokens the same way. The
// rates are explicit tariffs, not a multiplier: if the vendor changes them,
// update this table and the documentation.
//
// The plans are only consulted when a model has no manual schedule of its own,
// and only while Config.PeakPricing is enabled.
var builtinTimePlans = map[string]ModelPrice{
	"deepseek-v4.1-flash": {
		TimeZone: "UTC",
		TimeTiers: []TimePriceTier{
			{
				Name:      "peak-0100-0400",
				Days:      []int{1, 2, 3, 4, 5},
				Start:     "01:00",
				End:       "04:00",
				Input:     timeRate(0.30),
				Output:    timeRate(1.20),
				CacheRead: timeRate(0.006),
			},
			{
				Name:      "peak-0600-1000",
				Days:      []int{1, 2, 3, 4, 5},
				Start:     "06:00",
				End:       "10:00",
				Input:     timeRate(0.30),
				Output:    timeRate(1.20),
				CacheRead: timeRate(0.006),
			},
		},
	},
	"deepseek-v4-pro": {
		TimeZone: "UTC",
		TimeTiers: []TimePriceTier{
			{
				Name:      "peak-0100-0400",
				Days:      []int{1, 2, 3, 4, 5},
				Start:     "01:00",
				End:       "04:00",
				Input:     timeRate(1.32),
				Output:    timeRate(3.96),
				CacheRead: timeRate(0.044),
			},
			{
				Name:      "peak-0600-1000",
				Days:      []int{1, 2, 3, 4, 5},
				Start:     "06:00",
				End:       "10:00",
				Input:     timeRate(1.32),
				Output:    timeRate(3.96),
				CacheRead: timeRate(0.044),
			},
		},
	},
}

// timeRate returns a pointer to a copy of one rate.
func timeRate(value float64) *float64 {
	return &value
}

// builtinTimePlanFor resolves the bundled schedule of one model. The caller
// owns the returned value and may mutate it.
func builtinTimePlanFor(catalogProvider, model string) (ModelPrice, bool) {
	if !builtinTimePlanProviders[strings.ToLower(strings.TrimSpace(catalogProvider))] {
		return ModelPrice{}, false
	}
	plan, ok := builtinTimePlans[normalizeTimePlanModelID(model)]
	if !ok || len(plan.TimeTiers) == 0 {
		return ModelPrice{}, false
	}
	return ModelPrice{TimeZone: plan.TimeZone, TimeTiers: cloneTimeTiers(plan.TimeTiers)}, true
}
