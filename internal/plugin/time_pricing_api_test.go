package plugin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// newTimePlanRuntime builds a runtime with a live price table so the model
// price routes can be driven end to end.
func newTimePlanRuntime(t *testing.T) *pluginRuntime {
	t.Helper()
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	t.Cleanup(func() { runtime.shutdown() })
	pricing, err := newPricingTableFor(config, store)
	if err != nil {
		t.Fatal(err)
	}
	runtime.pricing = pricing
	registration, err := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.registerManagement(registration); err != nil {
		t.Fatal(err)
	}
	return runtime
}

func timePlanCall(t *testing.T, runtime *pluginRuntime, method, path string, query url.Values, body []byte) pluginapi.ManagementResponse {
	t.Helper()
	request := pluginapi.ManagementRequest{Method: method, Path: path, Query: query}
	if body != nil {
		request.Body = body
		request.Headers = http.Header{"Content-Type": []string{"application/json"}}
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := authedCall(t, runtime, raw)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

// TestTimePlanSaveAndRateOnlyPreserve pins the API contract: the management PUT
// stores a whole schedule, and the legacy GET save keeps it while editing rates.
func TestTimePlanSaveAndRateOnlyPreserve(t *testing.T) {
	runtime := newTimePlanRuntime(t)
	schedule := map[string]any{
		"model": "deepseek-v4.1-flash",
		"price": map[string]any{
			"input":     0.2,
			"output":    0.8,
			"time_zone": "Asia/Shanghai",
			"time_tiers": []any{map[string]any{
				"name": "night", "days": []int{1, 7}, "start": "23:00", "end": "08:00", "input": 0.1,
			}},
		},
	}
	body, err := json.Marshal(schedule)
	if err != nil {
		t.Fatal(err)
	}
	response := timePlanCall(t, runtime, http.MethodPut, runtime.routes.modelPricesSavePath, nil, body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"time_tiers"`) {
		t.Fatalf("schedule PUT = %+v", response)
	}
	var saved struct {
		Price resolvedPrice `json:"price"`
	}
	if err := json.Unmarshal(response.Body, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Price.TimePlanSource != priceSourceOverride || saved.Price.TimeZone != "Asia/Shanghai" || len(saved.Price.TimeTiers) != 1 {
		t.Fatalf("resolved schedule = %+v", saved.Price)
	}

	// A rate-only GET save must not drop the stored schedule.
	response = timePlanCall(t, runtime, http.MethodGet, runtime.routes.resourceModelPricesPath,
		url.Values{"save": {"1"}, "model": {"deepseek-v4.1-flash"}, "input": {"0.3"}}, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("rate-only save = %+v", response)
	}
	stored := runtime.pricing.Overrides()["deepseek-v4.1-flash"]
	if priceValue(stored.Input) != 0.3 || stored.TimeZone != "Asia/Shanghai" || len(stored.TimeTiers) != 1 || len(stored.TimeTiers[0].Days) != 2 {
		t.Fatalf("rate-only save lost the schedule: %+v", stored)
	}

	// An invalid schedule is refused without touching the stored one.
	bad := []byte(`{"model":"deepseek-v4.1-flash","price":{"input":0.2,"time_tiers":[{"name":"a","start":"01:00","end":"04:00"},{"name":"b","start":"03:00","end":"05:00"}]}}`)
	response = timePlanCall(t, runtime, http.MethodPut, runtime.routes.modelPricesSavePath, nil, bad)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("overlapping schedule accepted: %+v", response)
	}
	if len(runtime.pricing.Overrides()["deepseek-v4.1-flash"].TimeTiers) != 1 {
		t.Fatal("a rejected save changed the stored schedule")
	}

	// Resetting the override restores the bundled DeepSeek plan.
	response = timePlanCall(t, runtime, http.MethodGet, runtime.routes.resourceModelPricesPath,
		url.Values{"reset": {"1"}, "model": {"deepseek-v4.1-flash"}}, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("reset = %+v", response)
	}
	resolved := runtime.pricing.PriceFor(Dimensions{Provider: "cap-opencode-go", Model: "deepseek-v4.1-flash"})
	if resolved.TimePlanSource != priceSourceBuiltin || len(resolved.TimeTiers) != 2 {
		t.Fatalf("bundled plan did not return after reset: %+v", resolved)
	}
}
