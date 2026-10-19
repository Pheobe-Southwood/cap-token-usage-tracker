package plugin

import (
	"sort"
	"time"

	"github.com/AITNR/cap-token-usage-tracker/internal/plugin/modelcatalog"
)

// maxModelRows bounds the model list returned to the dashboard.
const maxModelRows = 1000

// modelPricing resolves list prices for aggregated usage groups.
// Implementations must be safe to call from the store actor goroutine and must
// not touch the store. The cost itself is derived by chargeFor so the
// time-of-use schedule stays in one place.
type modelPricing interface {
	PriceFor(dimensions Dimensions) resolvedPrice
}

// pricingStatus is the catalog state embedded in statistics responses.
type pricingStatus struct {
	modelcatalog.Status
	Overrides int `json:"overrides"`
}

// ModelCostSummary aggregates the priced/unpriced split for one range.
type ModelCostSummary struct {
	Requests         uint64  `json:"requests"`
	CostUSD          float64 `json:"cost_usd"`
	PricedModels     int     `json:"priced_models"`
	FreeModels       int     `json:"free_models"`
	UnpricedModels   int     `json:"unpriced_models"`
	UnpricedRequests uint64  `json:"unpriced_requests"`
}

// ModelPricingInfo is one row of the models tab: usage, both TPS variants,
// average TTFT, the estimated list-price cost and the pricing evidence.
// SupplementedTTFTSamples counts the TTFT samples that came from the sibling
// opencode-go plugin because the host did not report them for plugin executors.
type ModelPricingInfo struct {
	Model string `json:"model"`
	Counters
	AverageLatencyNS        uint64        `json:"average_latency_ns"`
	AverageTTFTNS           uint64        `json:"average_ttft_ns"`
	SupplementedTTFTSamples uint64        `json:"supplemented_ttft_samples,omitempty"`
	GenerationTPS           float64       `json:"generation_tps"`
	EndToEndTPS             float64       `json:"end_to_end_tps"`
	CostUSD                 float64       `json:"cost_usd"`
	Priced                  bool          `json:"priced"`
	PricedRequests          uint64        `json:"priced_requests"`
	UnpricedRequests        uint64        `json:"unpriced_requests"`
	InRange                 bool          `json:"in_range"`
	Pricing                 resolvedPrice `json:"pricing"`
}

// ModelsResponse is the payload of the models endpoint.
type ModelsResponse struct {
	SchemaVersion  uint32               `json:"schema_version"`
	GeneratedAt    time.Time            `json:"generated_at"`
	Range          string               `json:"range"`
	RetainedSince  time.Time            `json:"retained_since"`
	Pricing        pricingStatus        `json:"pricing"`
	TTFTSupplement ttftSupplementStatus `json:"ttft_supplement"`
	Summary        ModelCostSummary     `json:"summary"`
	Truncated      bool                 `json:"truncated"`
	Models         []ModelPricingInfo   `json:"models"`
}

type modelAccumulator struct {
	counters               Counters
	cost                   float64
	pricedRequests         uint64
	unpricedRequests       uint64
	representative         Dimensions
	representativeRequests uint64
	inRange                bool
}

// linkageHourTotals is our own TTFT coverage of one sibling model and UTC hour,
// summed over every provider. It is compared against the sibling's own sample
// count to derive the missing samples.
type linkageHourTotals struct {
	samples uint64
	nano    uint64
}

// linkageRowHour tracks one of our model rows inside a sibling model/hour so
// the missing samples can be attributed to the row that actually lacks them.
type linkageRowHour struct {
	requests uint64
	samples  uint64
}

// buildModelsForRange lists every metered model in the requested window plus
// models the plugin has metered before and models that only carry a manual
// price, so the operator can price a model that has no usage yet.
//
// When the sibling opencode-go plugin is available, its hourly TTFT samples are
// linked in: the host does not report TTFT for plugin executors, so without the
// link the heaviest models would show no first-token latency at all.
func buildModelsForRange(data map[aggregateKey]Counters, since, lastUsed time.Time, queryRange usageRange, filter usageFilter, now time.Time, pricer modelPricing, observed []string, overrides map[string]ModelPrice, linkage *ttftLinkage) ModelsResponse {
	models := make(map[string]*modelAccumulator)
	linkageTotals := make(map[string]map[int64]*linkageHourTotals)
	linkageRows := make(map[string]map[int64]map[string]*linkageRowHour)
	linkageActive := linkage.usable()
	for key, counters := range data {
		bucketTime := time.Unix(key.Hour, 0).UTC()
		if !queryRange.includes(bucketTime) {
			continue
		}
		dimensions := sanitizeDimensionsSource(key.Dimensions)
		model := compactModelName(dimensions.Model)
		hour := bucketTime.Truncate(time.Hour).Unix()
		var siblingID string
		if linkageActive && model != "" {
			siblingID = linkage.lookup(model)
		}
		if siblingID != "" {
			// Totals are collected before filtering: the sibling's sample count
			// covers the whole hour, so subtracting every sample we recorded
			// keeps the delta free of double counting.
			hours := linkageTotals[siblingID]
			if hours == nil {
				hours = make(map[int64]*linkageHourTotals)
				linkageTotals[siblingID] = hours
			}
			total := hours[hour]
			if total == nil {
				total = &linkageHourTotals{}
				hours[hour] = total
			}
			total.samples = saturatingAdd(total.samples, counters.TTFTSamples)
			total.nano = saturatingAdd(total.nano, counters.TotalTTFTNS)
		}
		if !filter.matches(dimensions) {
			continue
		}
		if model == "" {
			continue
		}
		entry := models[model]
		if entry == nil {
			entry = &modelAccumulator{}
			models[model] = entry
		}
		entry.counters.add(counters)
		entry.inRange = true
		if pricer != nil {
			if amount, _, priced := chargeFor(pricer.PriceFor(dimensions), dimensions, counters, bucketTime); priced {
				entry.cost += amount
				entry.pricedRequests = saturatingAdd(entry.pricedRequests, counters.Requests)
			} else {
				entry.unpricedRequests = saturatingAdd(entry.unpricedRequests, counters.Requests)
			}
		} else {
			entry.unpricedRequests = saturatingAdd(entry.unpricedRequests, counters.Requests)
		}
		if counters.Requests > entry.representativeRequests ||
			(counters.Requests == entry.representativeRequests && dimensions.Provider < entry.representative.Provider) {
			entry.representative = dimensions
			entry.representativeRequests = counters.Requests
		}
		if siblingID != "" && linkage.ownsProvider(dimensions.Provider) {
			hours := linkageRows[siblingID]
			if hours == nil {
				hours = make(map[int64]map[string]*linkageRowHour)
				linkageRows[siblingID] = hours
			}
			rows := hours[hour]
			if rows == nil {
				rows = make(map[string]*linkageRowHour)
				hours[hour] = rows
			}
			row := rows[model]
			if row == nil {
				row = &linkageRowHour{}
				rows[model] = row
			}
			row.requests = saturatingAdd(row.requests, counters.Requests)
			row.samples = saturatingAdd(row.samples, counters.TTFTSamples)
		}
	}
	supplements, supplementStatus := linkSiblingTTFT(linkage, linkageTotals, linkageRows)
	for _, model := range observed {
		model = compactModelName(model)
		if model == "" {
			continue
		}
		if _, exists := models[model]; !exists {
			models[model] = &modelAccumulator{}
		}
	}
	for model := range overrides {
		model = compactModelName(model)
		if model == "" {
			continue
		}
		if _, exists := models[model]; !exists {
			models[model] = &modelAccumulator{}
		}
	}

	summary := ModelCostSummary{}
	rows := make([]ModelPricingInfo, 0, len(models))
	for model, entry := range models {
		dimensions := entry.representative
		if dimensions.Model == "" {
			dimensions = Dimensions{Model: model}
		}
		resolved := resolvedPrice{Source: priceSourceNone}
		if pricer != nil {
			resolved = pricer.PriceFor(dimensions)
		}
		supplemented := uint64(0)
		if add, ok := supplements[model]; ok && add.Samples > 0 {
			entry.counters.TotalTTFTNS = saturatingAdd(entry.counters.TotalTTFTNS, add.Nano)
			entry.counters.TTFTSamples = saturatingAdd(entry.counters.TTFTSamples, add.Samples)
			supplemented = add.Samples
		}
		generationTPS, endToEndTPS := modelThroughput(dimensions, entry.counters)
		row := ModelPricingInfo{
			Model:                   model,
			Counters:                entry.counters,
			AverageLatencyNS:        entry.counters.averageLatencyNS(),
			AverageTTFTNS:           entry.counters.averageTTFTNS(),
			SupplementedTTFTSamples: supplemented,
			GenerationTPS:           generationTPS,
			EndToEndTPS:             endToEndTPS,
			CostUSD:                 entry.cost,
			Priced:                  resolved.Priced,
			PricedRequests:          entry.pricedRequests,
			UnpricedRequests:        entry.unpricedRequests,
			InRange:                 entry.inRange,
			Pricing:                 resolved,
		}
		if entry.inRange {
			summary.Requests = saturatingAdd(summary.Requests, entry.counters.Requests)
			summary.CostUSD += entry.cost
			switch {
			case !resolved.Priced:
				summary.UnpricedModels++
				summary.UnpricedRequests = saturatingAdd(summary.UnpricedRequests, entry.unpricedRequests)
			case resolved.Free:
				summary.FreeModels++
			default:
				summary.PricedModels++
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens != rows[j].TotalTokens {
			return rows[i].TotalTokens > rows[j].TotalTokens
		}
		return rows[i].Model < rows[j].Model
	})
	truncated := false
	if len(rows) > maxModelRows {
		rows = rows[:maxModelRows]
		truncated = true
	}
	return ModelsResponse{
		SchemaVersion:  1,
		GeneratedAt:    now.UTC(),
		Range:          queryRange.Name,
		RetainedSince:  since.UTC(),
		TTFTSupplement: supplementStatus,
		Summary:        summary,
		Truncated:      truncated,
		Models:         rows,
	}
}

// linkSiblingTTFT derives the missing TTFT samples per (sibling model, hour)
// and attributes them to the model rows that actually lack samples. Only rows
// served by the sibling plugin are eligible, so a same-named model on another
// channel never borrows the sibling's measurements.
func linkSiblingTTFT(linkage *ttftLinkage, totals map[string]map[int64]*linkageHourTotals, rows map[string]map[int64]map[string]*linkageRowHour) (map[string]ttftLinkageSample, ttftSupplementStatus) {
	status := linkage.status()
	supplements := make(map[string]ttftLinkageSample)
	if !linkage.usable() {
		return supplements, status
	}
	for modelID, hours := range totals {
		for hour, ours := range hours {
			sibling, ok := linkage.sample(modelID, hour)
			if !ok || sibling.Samples <= ours.samples {
				continue
			}
			deltaSamples := sibling.Samples - ours.samples
			deltaNano := uint64(0)
			switch {
			case sibling.Nano > ours.nano:
				deltaNano = sibling.Nano - ours.nano
			case sibling.Samples > 0:
				deltaNano = uint64(float64(sibling.Nano) * float64(deltaSamples) / float64(sibling.Samples))
			}
			candidates := rows[modelID][hour]
			bestModel := ""
			var bestMissing uint64
			for model, row := range candidates {
				if row.requests <= row.samples {
					continue
				}
				missing := row.requests - row.samples
				if missing > bestMissing || (missing == bestMissing && model < bestModel) {
					bestModel = model
					bestMissing = missing
				}
			}
			if bestModel == "" || bestMissing == 0 {
				continue
			}
			if deltaSamples > bestMissing {
				deltaNano = uint64(float64(deltaNano) * float64(bestMissing) / float64(deltaSamples))
				deltaSamples = bestMissing
			}
			add := supplements[bestModel]
			add.Samples = saturatingAdd(add.Samples, deltaSamples)
			add.Nano = saturatingAdd(add.Nano, deltaNano)
			supplements[bestModel] = add
		}
	}
	for _, add := range supplements {
		status.Models++
		status.Samples = saturatingAdd(status.Samples, add.Samples)
	}
	return supplements, status
}

// applyPricingStatus fills the catalog block of a models response.
func (r ModelsResponse) withPricing(status pricingStatus) ModelsResponse {
	r.Pricing = status
	return r
}
