package plugin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// stubPricer is a deterministic price table for aggregation tests.
type stubPricer struct {
	prices map[string]ModelPrice
	// plan optionally attaches a time-of-use schedule to every resolved price.
	plan *ModelPrice
}

func (p stubPricer) PriceFor(dimensions Dimensions) resolvedPrice {
	price, ok := p.prices[dimensions.Model]
	resolved := resolvedPrice{Source: priceSourceNone}
	if !ok {
		return resolved
	}
	resolved.Price = price
	resolved.Priced = price.Priced()
	resolved.Free = price.Free()
	resolved.Source = priceSourceOverride
	resolved.CatalogProvider = "stub"
	resolved.CatalogModel = dimensions.Model
	resolved.CatalogMatch = "global"
	if p.plan != nil {
		resolved.TimeZone = p.plan.TimeZone
		resolved.TimeTiers = cloneTimeTiers(p.plan.TimeTiers)
		resolved.TimePlanSource = priceSourceOverride
	}
	return resolved
}

func TestBuildModelsForRangeListsUsageObservedAndOverrides(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	hour := now.Add(-time.Hour).Truncate(time.Hour).Unix()
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Provider: "antigravity", Model: "gemini-3.8-flash-high"}}: {
			Requests: 3, InputTokens: 1_000_000, OutputTokens: 500_000, TotalTokens: 1_500_000,
			TotalLatencyNS: 20_000_000_000, TotalTTFTNS: 4_000_000_000,
		},
		{Hour: hour, Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "Beam"}}: {
			Requests: 2, InputTokens: 100, OutputTokens: 50, TotalTokens: 150, TotalLatencyNS: 2_000_000_000,
		},
		{Hour: now.Add(-48 * time.Hour).Truncate(time.Hour).Unix(), Dimensions: Dimensions{Model: "old-model"}}: {
			Requests: 9, TotalTokens: 90,
		},
	}
	pricer := stubPricer{prices: map[string]ModelPrice{
		"gemini-3.8-flash-high": priceOf(0.75, 3.75, mathNaN(), mathNaN()),
		"Beam":                  priceOf(1, 2, mathNaN(), mathNaN()),
		"priced-unused":         priceOf(5, 5, mathNaN(), mathNaN()),
		"override-only":         priceOf(3, 3, mathNaN(), mathNaN()),
		"free-model":            priceOf(0, 0, mathNaN(), mathNaN()),
	}}
	queryRange := usageRange{Name: "24h", Start: now.Add(-24 * time.Hour), End: now}
	response := buildModelsForRange(data, now.Add(-30*24*time.Hour), now, queryRange, usageFilter{}, now, pricer,
		[]string{"observed-only", "Beam"},
		map[string]ModelPrice{"override-only": priceOf(3, 3, mathNaN(), mathNaN()), "free-model": priceOf(0, 0, mathNaN(), mathNaN())}, nil)

	byModel := map[string]ModelPricingInfo{}
	for _, row := range response.Models {
		byModel[row.Model] = row
	}
	for _, model := range []string{"gemini-3.8-flash-high", "Beam", "observed-only", "override-only", "free-model"} {
		if _, ok := byModel[model]; !ok {
			t.Fatalf("model %s missing from %+v", model, response.Models)
		}
	}
	if _, ok := byModel["old-model"]; ok {
		t.Fatalf("out-of-range model leaked into the response")
	}
	if row := byModel["gemini-3.8-flash-high"]; !row.Priced || !row.InRange || row.CostUSD <= 0 || row.Requests != 3 {
		t.Fatalf("priced row = %+v", row)
	}
	if row := byModel["Beam"]; !row.Priced || row.Pricing.Source != priceSourceOverride || row.CostUSD != (100*1+50*2)/1e6 {
		t.Fatalf("override row = %+v", row)
	}
	if row := byModel["observed-only"]; row.InRange || row.Requests != 0 || row.Priced {
		t.Fatalf("observed-only row = %+v", row)
	}
	if row := byModel["override-only"]; row.InRange || !row.Priced || row.CostUSD != 0 {
		t.Fatalf("override-only row = %+v", row)
	}
	if row := byModel["free-model"]; !row.Pricing.Free {
		t.Fatalf("free row = %+v", row)
	}
	if response.Summary.Requests != 5 || response.Summary.PricedModels != 2 || response.Summary.UnpricedModels != 0 || response.Summary.UnpricedRequests != 0 {
		t.Fatalf("summary = %+v", response.Summary)
	}
	if response.Summary.CostUSD <= 0 {
		t.Fatalf("summary cost = %v", response.Summary.CostUSD)
	}
	if response.SchemaVersion != 1 || response.Range != "24h" || response.Truncated {
		t.Fatalf("response envelope = %+v", response)
	}
	if response.Models[0].Model != "gemini-3.8-flash-high" {
		t.Fatalf("rows are not sorted by total tokens: %+v", response.Models)
	}
}

func TestBuildModelsForRangeCountsUnpricedRequests(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	hour := now.Add(-time.Hour).Truncate(time.Hour).Unix()
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Model: "Beam"}}:   {Requests: 4, TotalTokens: 10},
		{Hour: hour, Dimensions: Dimensions{Model: "CatJam"}}: {Requests: 1, TotalTokens: 5},
	}
	queryRange := usageRange{Name: "24h", Start: now.Add(-24 * time.Hour), End: now}
	response := buildModelsForRange(data, now, now, queryRange, usageFilter{}, now, stubPricer{}, nil, nil, nil)
	if response.Summary.UnpricedModels != 2 || response.Summary.UnpricedRequests != 5 || response.Summary.CostUSD != 0 {
		t.Fatalf("summary = %+v", response.Summary)
	}
	for _, row := range response.Models {
		if row.Priced || row.Pricing.Source != priceSourceNone {
			t.Fatalf("row %s should be unpriced: %+v", row.Model, row)
		}
	}
}

func TestBuildModelsForRangeTruncatesLongLists(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	observed := make([]string, 0, maxModelRows+10)
	for index := 0; index < maxModelRows+10; index++ {
		observed = append(observed, "model-"+string(rune('a'+index%26))+strings.Repeat("x", index%7)+"-"+itoa(index))
	}
	response := buildModelsForRange(nil, now, now, usageRange{Name: "24h", Start: now.Add(-time.Hour), End: now}, usageFilter{}, now, stubPricer{}, observed, nil, nil)
	if len(response.Models) != maxModelRows || !response.Truncated {
		t.Fatalf("truncation = %d rows, truncated=%v", len(response.Models), response.Truncated)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

func mathNaN() float64 {
	var zero float64
	return zero / zero
}

func TestModelsAndPriceEndpoints(t *testing.T) {
	config := testConfig(t)
	config.PricingCatalogURL = ""
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()
	pricing, err := newPricingTableFor(config, store)
	if err != nil {
		t.Fatal(err)
	}
	runtime.pricing = pricing
	registration, _ := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if _, err := runtime.registerManagement(registration); err != nil {
		t.Fatal(err)
	}
	now := nowUTC().Truncate(5 * time.Minute)
	for _, usage := range []normalizedUsage{
		{Dimensions: Dimensions{Provider: "antigravity", Model: "gemini-3.8-flash-high"}, RequestedAt: now.Add(-4 * time.Minute), Counters: Counters{Requests: 2, InputTokens: 1_000_000, OutputTokens: 200_000, TotalTokens: 1_200_000, TotalLatencyNS: 10_000_000_000, TotalTTFTNS: 2_000_000_000, LatencySamples: 2, TTFTSamples: 2}},
		{Dimensions: Dimensions{Provider: "cap-opencode-go", Model: "Beam"}, RequestedAt: now.Add(-3 * time.Minute), Counters: Counters{Requests: 1, InputTokens: 10, OutputTokens: 5, TotalTokens: 15}},
	} {
		if err := store.Record(usage); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path string, query url.Values) pluginapi.ManagementResponse {
		t.Helper()
		raw, err := json.Marshal(pluginapi.ManagementRequest{Method: method, Path: path, Query: query})
		if err != nil {
			t.Fatal(err)
		}
		response, err := authedCall(t, runtime, raw)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}

	// The usage tab carries the cost of every model.
	initialResponse := call(http.MethodGet, runtime.routes.resourceStatsInitialPath, url.Values{"range": {"24h"}})
	var initial InitialStatsResponse
	if err := json.Unmarshal(initialResponse.Body, &initial); err != nil {
		t.Fatalf("initial payload: %v", err)
	}
	if initial.Pricing.Source != "embedded" || initial.Pricing.Models == 0 {
		t.Fatalf("initial pricing block = %+v", initial.Pricing)
	}
	if initial.Cost.CostUSD <= 0 || initial.Cost.PricedModels != 1 || initial.Cost.UnpricedModels != 1 || initial.Cost.UnpricedRequests != 1 {
		t.Fatalf("initial cost block = %+v", initial.Cost)
	}
	var geminiCost float64
	for _, model := range initial.Models {
		if model.Model == "gemini-3.8-flash-high" {
			geminiCost = model.CostUSD
			if !model.Priced {
				t.Fatalf("model row is not priced: %+v", model)
			}
		}
		if model.Model == "Beam" && model.Priced {
			t.Fatalf("unpriced model reported as priced: %+v", model)
		}
	}
	if geminiCost <= 0 {
		t.Fatalf("usage tab cost = %v", geminiCost)
	}

	// A custom range with sub-minute bounds takes the exact per-request path;
	// the derived cost must survive that branch too.
	exactStart := now.Add(-30 * time.Minute).Add(17 * time.Second)
	exactQuery := url.Values{"range": {"custom"}, "start": {exactStart.Format(time.RFC3339)}, "end": {now.Format(time.RFC3339)}}
	exactResponse := call(http.MethodGet, runtime.routes.resourceStatsInitialPath, exactQuery)
	var exactInitial InitialStatsResponse
	if exactResponse.StatusCode != http.StatusOK || json.Unmarshal(exactResponse.Body, &exactInitial) != nil {
		t.Fatalf("exact initial response = %+v", exactResponse)
	}
	if exactInitial.Cost.CostUSD <= 0 || exactInitial.Cost.PricedModels == 0 {
		t.Fatalf("exact-path cost block = %+v", exactInitial.Cost)
	}
	var exactGeminiCost float64
	for _, model := range exactInitial.Models {
		if model.Model == "gemini-3.8-flash-high" {
			exactGeminiCost = model.CostUSD
		}
	}
	if exactGeminiCost <= 0 {
		t.Fatalf("exact-path model cost = %v", exactGeminiCost)
	}

	// The models tab lists every metered model with both TPS variants.
	modelsResponse := call(http.MethodGet, runtime.routes.resourceStatsModelsPath, url.Values{"range": {"24h"}})
	var models ModelsResponse
	if modelsResponse.StatusCode != http.StatusOK || json.Unmarshal(modelsResponse.Body, &models) != nil {
		t.Fatalf("models response = %+v", modelsResponse)
	}
	if models.SchemaVersion != 1 || len(models.Models) < 2 || models.Summary.Requests != 3 {
		t.Fatalf("models payload = %+v", models)
	}
	var sawGemini bool
	for _, row := range models.Models {
		if row.Model != "gemini-3.8-flash-high" {
			continue
		}
		sawGemini = true
		if row.GenerationTPS <= 0 || row.EndToEndTPS <= 0 || row.GenerationTPS <= row.EndToEndTPS {
			t.Fatalf("TPS values = %+v", row)
		}
		if row.AverageTTFTNS == 0 || row.Pricing.CatalogProvider != "google" {
			t.Fatalf("row = %+v", row)
		}
	}
	if !sawGemini {
		t.Fatalf("gemini row missing: %+v", models.Models)
	}
	if models.Summary.UnpricedModels != 1 || models.Summary.UnpricedRequests != 1 {
		t.Fatalf("summary = %+v", models.Summary)
	}

	// Saving a manual price through the GET compatibility route prices it.
	saveQuery := url.Values{"save": {"1"}, "model": {"Beam"}, "input": {"1.5"}, "output": {"3"}}
	saveResponse := call(http.MethodGet, runtime.routes.resourceModelPricesPath, saveQuery)
	if saveResponse.StatusCode != http.StatusOK {
		t.Fatalf("save response = %+v", saveResponse)
	}
	modelsResponse = call(http.MethodGet, runtime.routes.resourceStatsModelsPath, url.Values{"range": {"24h"}})
	if err := json.Unmarshal(modelsResponse.Body, &models); err != nil {
		t.Fatal(err)
	}
	for _, row := range models.Models {
		if row.Model == "Beam" && (!row.Priced || row.Pricing.Source != priceSourceOverride || row.CostUSD != (10*1.5+5*3)/1e6) {
			t.Fatalf("override row = %+v", row)
		}
	}
	if models.Summary.UnpricedModels != 0 || models.Summary.UnpricedRequests != 0 {
		t.Fatalf("summary after override = %+v", models.Summary)
	}

	// The management PUT route accepts the same update and returns the table.
	body, _ := json.Marshal(map[string]any{"model": "Beam", "price": map[string]any{"input": 2, "output": 4}})
	putRequest, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodPut, Path: runtime.routes.modelPricesSavePath,
		Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: body,
	})
	putResponse, err := authedCall(t, runtime, putRequest)
	if err != nil || putResponse.StatusCode != http.StatusOK || !strings.Contains(string(putResponse.Body), `"overrides"`) {
		t.Fatalf("PUT response = %+v, %v", putResponse, err)
	}

	// Snapshot route reports the stored override table.
	snapshot := call(http.MethodGet, runtime.routes.resourceModelPricesPath, nil)
	if snapshot.StatusCode != http.StatusOK || !strings.Contains(string(snapshot.Body), `"Beam"`) || !strings.Contains(string(snapshot.Body), `"overrides"`) {
		t.Fatalf("snapshot = %+v", snapshot)
	}

	// Reset drops the override so the model is unpriced again.
	resetResponse := call(http.MethodGet, runtime.routes.resourceModelPricesPath, url.Values{"reset": {"1"}, "model": {"Beam"}})
	if resetResponse.StatusCode != http.StatusOK {
		t.Fatalf("reset response = %+v", resetResponse)
	}
	modelsResponse = call(http.MethodGet, runtime.routes.resourceStatsModelsPath, url.Values{"range": {"24h"}})
	if err := json.Unmarshal(modelsResponse.Body, &models); err != nil {
		t.Fatal(err)
	}
	if models.Summary.UnpricedModels != 1 {
		t.Fatalf("summary after reset = %+v", models.Summary)
	}

	// Refresh without a configured URL fails softly.
	refresh := call(http.MethodGet, runtime.routes.resourceModelPricesPath, url.Values{"refresh": {"1"}})
	if refresh.StatusCode != http.StatusOK || !strings.Contains(string(refresh.Body), `"ok":false`) {
		t.Fatalf("refresh response = %+v", refresh)
	}

	// Unknown query parameters and methods are rejected.
	if response := call(http.MethodGet, runtime.routes.resourceModelPricesPath, url.Values{"bogus": {"1"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("bogus query = %+v", response)
	}
	if response := call(http.MethodPost, runtime.routes.resourceStatsModelsPath, nil); response.StatusCode != http.StatusMethodNotAllowed || response.Headers.Get("Allow") != http.MethodGet {
		t.Fatalf("method restriction = %+v", response)
	}
	if response := call(http.MethodPost, runtime.routes.modelPricesSavePath, nil); response.StatusCode != http.StatusMethodNotAllowed || response.Headers.Get("Allow") != http.MethodPut {
		t.Fatalf("management method restriction = %+v", response)
	}
	// A negative manual price is rejected.
	if response := call(http.MethodGet, runtime.routes.resourceModelPricesPath, url.Values{"save": {"1"}, "model": {"Beam"}, "input": {"-1"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("negative price = %+v", response)
	}
}
