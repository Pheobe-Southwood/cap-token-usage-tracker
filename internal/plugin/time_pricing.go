package plugin

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Keep named zones available on Windows and in minimal containers.
)

const (
	// maxTimePriceTiers bounds the weekly schedule stored for one model.
	maxTimePriceTiers = 32
	// maxTimeTierNameBytes bounds one tier label.
	maxTimeTierNameBytes = 200
)

// TimePriceTier is one weekly recurring time window of a model's price
// schedule. Days use ISO weekdays (1=Monday, 7=Sunday); an empty list means
// every day. Windows are half open ([start, end)) and an overnight window
// belongs to its starting day, so a Sunday 23:00-08:00 window also covers
// Monday until 08:00. Rates are USD per 1,000,000 tokens; a missing rate
// inherits the model's base rate, mirroring the override merge semantics.
type TimePriceTier struct {
	Name       string   `json:"name"`
	Days       []int    `json:"days,omitempty"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
	Input      *float64 `json:"input,omitempty"`
	Output     *float64 `json:"output,omitempty"`
	CacheRead  *float64 `json:"cache_read,omitempty"`
	CacheWrite *float64 `json:"cache_write,omitempty"`
}

// clockMinute parses a strict HH:mm wall-clock value into minutes of day.
func clockMinute(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return 0, fmt.Errorf("invalid time %q: use HH:mm", value)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// timeTierDays expands the optional weekday list.
func timeTierDays(tier TimePriceTier) []int {
	if len(tier.Days) == 0 {
		return []int{1, 2, 3, 4, 5, 6, 7}
	}
	return tier.Days
}

// timeTierRates projects one tier onto the shared rate shape so the cost
// formula can merge it over the base price.
func timeTierRates(tier TimePriceTier) ModelPrice {
	return ModelPrice{Input: tier.Input, Output: tier.Output, CacheRead: tier.CacheRead, CacheWrite: tier.CacheWrite}
}

// normalizeTimePricing validates and canonicalises the optional weekly
// schedule of one price. It is shared by the save path and by the storage load
// path, so a hand-edited database cannot inject a schedule the cost formula
// would misread.
func normalizeTimePricing(model string, price *ModelPrice) error {
	price.TimeZone = strings.TrimSpace(price.TimeZone)
	if len(price.TimeTiers) > maxTimePriceTiers {
		return fmt.Errorf("model %q: at most %d time tiers", model, maxTimePriceTiers)
	}
	if len(price.TimeTiers) == 0 {
		// A zone without a schedule carries no meaning; drop it so an empty
		// override stays empty.
		price.TimeZone = ""
		price.TimeTiers = nil
		return nil
	}
	if price.TimeZone == "" {
		price.TimeZone = "UTC"
	}
	if price.TimeZone == "Local" {
		return fmt.Errorf("model %q: use an explicit time zone", model)
	}
	if _, err := time.LoadLocation(price.TimeZone); err != nil {
		return fmt.Errorf("model %q: invalid time zone %q", model, price.TimeZone)
	}
	price.TimeTiers = cloneTimeTiers(price.TimeTiers)
	var occupied [7 * 1440]bool
	names := make(map[string]bool, len(price.TimeTiers))
	for index := range price.TimeTiers {
		tier := &price.TimeTiers[index]
		tier.Name = strings.TrimSpace(tier.Name)
		if tier.Name == "" || len(tier.Name) > maxTimeTierNameBytes || strings.ContainsAny(tier.Name, "\x00\n\r") {
			return fmt.Errorf("model %q: time tier names must be nonempty and unique", model)
		}
		if names[tier.Name] {
			return fmt.Errorf("model %q: time tier names must be nonempty and unique", model)
		}
		names[tier.Name] = true
		start, err := clockMinute(tier.Start)
		if err != nil {
			return fmt.Errorf("model %q: time tier %s: %w", model, tier.Name, err)
		}
		end, err := clockMinute(tier.End)
		if err != nil {
			return fmt.Errorf("model %q: time tier %s: %w", model, tier.Name, err)
		}
		if start == end {
			return fmt.Errorf("model %q: time tier %s: start and end must differ", model, tier.Name)
		}
		for _, field := range []struct {
			name  string
			value *float64
		}{
			{name: "input", value: tier.Input},
			{name: "output", value: tier.Output},
			{name: "cache_read", value: tier.CacheRead},
			{name: "cache_write", value: tier.CacheWrite},
		} {
			if field.value == nil {
				continue
			}
			rounded, err := normalizePriceValue(*field.value)
			if err != nil {
				return fmt.Errorf("model %q: time tier %s: %s: %w", model, tier.Name, field.name, err)
			}
			pointer := rounded
			switch field.name {
			case "input":
				tier.Input = &pointer
			case "output":
				tier.Output = &pointer
			case "cache_read":
				tier.CacheRead = &pointer
			case "cache_write":
				tier.CacheWrite = &pointer
			}
		}
		sort.Ints(tier.Days)
		for position, day := range tier.Days {
			if day < 1 || day > 7 || (position > 0 && tier.Days[position-1] == day) {
				return fmt.Errorf("model %q: days must be unique integers from 1 to 7", model)
			}
		}
		duration := (end - start + 1440) % 1440
		for _, day := range timeTierDays(*tier) {
			for minute := 0; minute < duration; minute++ {
				index := ((day-1)*1440 + start + minute) % len(occupied)
				if occupied[index] {
					return fmt.Errorf("model %q: overlapping time tiers", model)
				}
				occupied[index] = true
			}
		}
	}
	return nil
}

// cloneTimeTiers deep copies a schedule so callers never share weekday slices.
func cloneTimeTiers(tiers []TimePriceTier) []TimePriceTier {
	if len(tiers) == 0 {
		return nil
	}
	result := append([]TimePriceTier(nil), tiers...)
	for index := range result {
		result[index].Days = append([]int(nil), result[index].Days...)
	}
	return result
}

// timeZoneCache memoises time.LoadLocation because the cost formula runs it
// for every aggregated group.
var timeZoneCache sync.Map

// loadTimeZone resolves one IANA zone name, caching successful lookups.
func loadTimeZone(zone string) (*time.Location, bool) {
	if zone == "" {
		zone = "UTC"
	}
	if cached, ok := timeZoneCache.Load(zone); ok {
		location, _ := cached.(*time.Location)
		return location, location != nil
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, false
	}
	timeZoneCache.Store(zone, location)
	return location, true
}

// selectTimeTier matches the wall-clock time in the configured zone and
// returns the single tier that covers it. Repeated DST hours match twice
// because the comparison uses local wall-clock minutes; a skipped hour has no
// matching request instant. Overlapping schedules are rejected at save time,
// so a second match means the stored schedule is malformed and the lookup
// refuses to guess.
func selectTimeTier(zone string, tiers []TimePriceTier, at time.Time) (TimePriceTier, bool) {
	if len(tiers) == 0 || at.IsZero() {
		return TimePriceTier{}, false
	}
	if zone == "" {
		zone = "UTC"
	}
	if zone == "Local" {
		return TimePriceTier{}, false
	}
	location, ok := loadTimeZone(zone)
	if !ok {
		return TimePriceTier{}, false
	}
	local := at.In(location)
	day := (int(local.Weekday())+6)%7 + 1
	minute := local.Hour()*60 + local.Minute()
	var matched TimePriceTier
	found := false
	for _, tier := range tiers {
		start, err := clockMinute(tier.Start)
		if err != nil {
			return TimePriceTier{}, false
		}
		end, err := clockMinute(tier.End)
		if err != nil || start == end {
			return TimePriceTier{}, false
		}
		matchDay := day
		hit := minute >= start && minute < end
		if start > end {
			hit = minute >= start || minute < end
			if minute < end {
				// The tail of an overnight window belongs to the previous day.
				matchDay = (day+5)%7 + 1
			}
		}
		if !hit {
			continue
		}
		for _, allowed := range timeTierDays(tier) {
			if allowed != matchDay {
				continue
			}
			if found {
				return TimePriceTier{}, false
			}
			matched = tier
			found = true
			break
		}
	}
	return matched, found
}

// normalizeTimePlanModelID reduces a reported model id to the key shape used by
// the built-in schedules: lowercase, without a vendor prefix, without the
// ":free" variant suffix and without an "@date" pin.
func normalizeTimePlanModelID(model string) string {
	value := strings.ToLower(strings.TrimSpace(model))
	if value == "" {
		return ""
	}
	if slash := strings.LastIndex(value, "/"); slash >= 0 {
		value = value[slash+1:]
	}
	if strings.HasSuffix(value, ":free") {
		value = strings.TrimSuffix(value, ":free")
	}
	if at := strings.LastIndex(value, "@"); at > 0 {
		suffix := value[at+1:]
		if suffix != "" && strings.IndexFunc(suffix, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
			value = value[:at]
		}
	}
	return strings.TrimSpace(value)
}
