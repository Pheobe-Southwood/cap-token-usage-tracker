package plugin

import (
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestPluginIDFromResourceBase(t *testing.T) {
	got, err := pluginIDFromResourceBase("/v0/resource/plugins/cap-token-usage-tracker")
	if err != nil || got != "cap-token-usage-tracker" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, invalid := range []string{"/wrong/id", "/v0/resource/plugins/a/b", "/v0/resource/plugins/bad id"} {
		if _, err := pluginIDFromResourceBase(invalid); err == nil {
			t.Fatalf("accepted invalid base %q", invalid)
		}
	}
}

func TestManagementRegistrationUsesDynamicPluginID(t *testing.T) {
	runtime := &pluginRuntime{}
	raw, _ := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/custom-id"})
	registration, err := runtime.registerManagement(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(registration.Routes) != 9 || registration.Routes[0].Method != http.MethodPost || registration.Routes[0].Path != "/plugins/custom-id/session" || registration.Routes[1].Path != "/plugins/custom-id/session/revoke" || registration.Routes[2].Method != http.MethodPut || registration.Routes[2].Path != "/plugins/custom-id/api-key-labels" || registration.Routes[3].Path != "/plugins/custom-id/stats" || registration.Routes[4].Method != http.MethodPost || registration.Routes[4].Path != "/plugins/custom-id/preferences" || registration.Routes[5].Method != http.MethodPut || registration.Routes[5].Path != "/plugins/custom-id/model-prices" || registration.Routes[6].Method != http.MethodPost || registration.Routes[6].Path != "/plugins/custom-id/reset" || registration.Routes[7].Method != http.MethodGet || registration.Routes[7].Path != "/plugins/custom-id/backup" || registration.Routes[8].Method != http.MethodPost || registration.Routes[8].Path != "/plugins/custom-id/restore" || len(registration.Resources) != 9 {
		t.Fatalf("unexpected registration: %+v", registration)
	}
	resourcePaths := make(map[string]bool, len(registration.Resources))
	for _, resource := range registration.Resources {
		resourcePaths[resource.Path] = true
	}
	for _, path := range []string{"/dashboard", "/stats", "/stats/initial", "/stats/trends", "/stats/groups", "/requests", "/preferences", "/stats/models", "/model-prices"} {
		if !resourcePaths[path] {
			t.Fatalf("registration missing resource %q: %+v", path, registration.Resources)
		}
	}
	if registration.Routes[0].Menu != "" {
		t.Fatal("authenticated stats route must not declare a legacy menu")
	}
}

func TestCompactStatsResourcesShapePagingAndMethods(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	config.PricingCatalogURL = ""
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	// A real pricing table plus explicit overrides keeps the cost assertions
	// independent of the embedded catalog.
	pricing, err := newPricingTableFor(config, store)
	if err != nil {
		t.Fatal(err)
	}
	runtime.pricing = pricing
	defer runtime.shutdown()
	registration, err := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.registerManagement(registration); err != nil {
		t.Fatal(err)
	}
	now := nowUTC().Truncate(5 * time.Minute)
	for _, usage := range []normalizedUsage{
		{Dimensions: Dimensions{Provider: "openai", Model: "alpha", Source: "cli"}, RequestedAt: now.Add(-4 * time.Minute), Counters: Counters{Requests: 2, InputTokens: 20, TotalTokens: 20}},
		{Dimensions: Dimensions{Provider: "anthropic", Model: "beta", Source: "cli"}, RequestedAt: now.Add(-3 * time.Minute), Counters: Counters{Requests: 1, InputTokens: 10, TotalTokens: 10}},
		{Dimensions: Dimensions{Provider: "openai", Model: "alpha", Source: "web"}, RequestedAt: now.Add(-2 * time.Minute), Counters: Counters{Requests: 3, InputTokens: 30, TotalTokens: 30}},
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
	query := url.Values{"range": {"24h"}}
	initialResponse := call(http.MethodGet, runtime.routes.resourceStatsInitialPath, query)
	if initialResponse.StatusCode != http.StatusOK || strings.Contains(string(initialResponse.Body), `"groups"`) || strings.Contains(string(initialResponse.Body), `"model_series"`) {
		t.Fatalf("initial response = %+v", initialResponse)
	}
	var initial InitialStatsResponse
	if err := json.Unmarshal(initialResponse.Body, &initial); err != nil || initial.SchemaVersion != 3 || initial.Scale != seriesScaleHour || initial.BucketSeconds != 3600 || initial.Summary.TotalTokens != 60 || len(initial.Models) != 2 || len(initial.Series) < 24 || len(initial.Series) > 26 || len(initial.CostSeries) != len(initial.Series) {
		t.Fatalf("initial payload = %+v, %v", initial, err)
	}
	// Pricing the fixture models through the compatibility route must add the
	// stacked histogram split to the same response.
	for _, price := range []url.Values{
		{"save": {"1"}, "model": {"alpha"}, "input": {"2"}, "output": {"2"}},
		{"save": {"1"}, "model": {"beta"}, "input": {"1"}, "output": {"1"}},
	} {
		if response := call(http.MethodGet, runtime.routes.resourceModelPricesPath, price); response.StatusCode != http.StatusOK {
			t.Fatalf("price save response = %+v", response)
		}
	}
	initialResponse = call(http.MethodGet, runtime.routes.resourceStatsInitialPath, query)
	if err := json.Unmarshal(initialResponse.Body, &initial); err != nil {
		t.Fatal(err)
	}
	if len(initial.CostByModel) != 2 || len(initial.CostByKey) != 0 {
		t.Fatalf("cost breakdown = %d models / %d keys, want 2/0", len(initial.CostByModel), len(initial.CostByKey))
	}
	if initial.CostByModel[0].Key != "alpha" || initial.CostByModel[0].Others {
		t.Fatalf("cost breakdown order = %+v", initial.CostByModel)
	}
	for _, entry := range initial.CostByModel {
		if len(entry.Values) != len(initial.CostSeries) {
			t.Fatalf("breakdown %q values = %d, want %d", entry.Key, len(entry.Values), len(initial.CostSeries))
		}
		var sum float64
		for _, value := range entry.Values {
			sum += value
		}
		if math.Abs(sum-entry.TotalUSD) > 1e-9 {
			t.Fatalf("breakdown %q total %v != sum %v", entry.Key, entry.TotalUSD, sum)
		}
	}
	// Bucket by bucket the stacked series must add up to the cost series.
	for index, point := range initial.CostSeries {
		var stacked float64
		for _, entry := range initial.CostByModel {
			stacked += entry.Values[index]
		}
		if math.Abs(stacked-point.CostUSD) > 1e-9 {
			t.Fatalf("bucket %d stacked %v != cost series %v", index, stacked, point.CostUSD)
		}
	}
	var breakdownTotal float64
	for _, entry := range initial.CostByModel {
		breakdownTotal += entry.TotalUSD
	}
	var seriesTotal float64
	for _, point := range initial.CostSeries {
		seriesTotal += point.CostUSD
	}
	if math.Abs(breakdownTotal-seriesTotal) > 1e-9 || math.Abs(breakdownTotal-initial.Cost.CostUSD) > 1e-9 {
		t.Fatalf("breakdown %v != series %v != summary %v", breakdownTotal, seriesTotal, initial.Cost.CostUSD)
	}
	trendResponse := call(http.MethodGet, runtime.routes.resourceStatsTrendPath, query)
	var trend StatsTrendResponse
	if trendResponse.StatusCode != http.StatusOK || json.Unmarshal(trendResponse.Body, &trend) != nil || trend.BucketSeconds != 300 || len(trend.ModelSeries) != 2 {
		t.Fatalf("trend response = %+v, payload=%+v", trendResponse, trend)
	}
	groupQuery := url.Values{"range": {"24h"}, "offset": {"0"}, "limit": {"1"}, "sort": {"model"}, "direction": {"asc"}}
	groupsResponse := call(http.MethodGet, runtime.routes.resourceStatsGroupsPath, groupQuery)
	var groups GroupStatsPage
	if groupsResponse.StatusCode != http.StatusOK || json.Unmarshal(groupsResponse.Body, &groups) != nil || groups.Total != 3 || len(groups.Items) != 1 || groups.Items[0].Model != "alpha" {
		t.Fatalf("groups response = %+v, payload=%+v", groupsResponse, groups)
	}
	groupQuery.Set("model", "alpha")
	groupQuery.Set("exclude_model", "alpha")
	groupsResponse = call(http.MethodGet, runtime.routes.resourceStatsGroupsPath, groupQuery)
	if groupsResponse.StatusCode != http.StatusOK || json.Unmarshal(groupsResponse.Body, &groups) != nil || groups.Total != 0 || len(groups.Items) != 0 {
		t.Fatalf("filtered groups response = %+v, payload=%+v", groupsResponse, groups)
	}
	for _, path := range []string{runtime.routes.resourceStatsInitialPath, runtime.routes.resourceStatsTrendPath, runtime.routes.resourceStatsGroupsPath} {
		response := call(http.MethodPost, path, nil)
		if response.StatusCode != http.StatusMethodNotAllowed || response.Headers.Get("Allow") != http.MethodGet {
			t.Fatalf("method restriction for %s = %+v", path, response)
		}
	}
}

func TestManagementStatsAndReset(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID: "test", statsPath: "/v0/management/plugins/test/stats", resetPath: "/v0/management/plugins/test/reset", dashboardPath: "/v0/resource/plugins/test/dashboard", resourceStatsPath: "/v0/resource/plugins/test/stats", resourceRequestsPath: "/v0/resource/plugins/test/requests", resourcePreferencesPath: "/v0/resource/plugins/test/preferences",
	}}
	defer runtime.shutdown()
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "m"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 3}}); err != nil {
		t.Fatal(err)
	}

	statsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.statsPath, Query: url.Values{"range": []string{"24h"}}})
	response, err := authedCall(t, runtime, statsRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("stats response: %+v, %v", response, err)
	}
	if strings.Contains(string(response.Body), `"ok"`) {
		t.Fatal("HTTP response body was incorrectly wrapped in RPC envelope")
	}
	if response.Headers.Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store header")
	}

	resourceStatsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsPath, Query: url.Values{"range": []string{"24h"}}})
	response, err = authedCall(t, runtime, resourceStatsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"total_tokens":3`) {
		t.Fatalf("resource stats response: %+v, %v", response, err)
	}
	customQuery := url.Values{
		"range": []string{"custom"},
		"start": []string{time.Now().Add(-time.Hour).Format(time.RFC3339)},
		"end":   []string{time.Now().Add(time.Hour).Format(time.RFC3339)},
	}
	customStatsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsPath, Query: customQuery})
	response, err = authedCall(t, runtime, customStatsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"range":"custom"`) || !strings.Contains(string(response.Body), `"total_tokens":3`) {
		t.Fatalf("custom stats response: %+v, %v", response, err)
	}
	invalidRangeRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsPath, Query: url.Values{"range": []string{"custom"}, "start": []string{time.Now().Format(time.RFC3339)}}})
	response, err = authedCall(t, runtime, invalidRangeRequest)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid custom range response: %+v, %v", response, err)
	}

	requestsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: url.Values{"range": []string{"24h"}, "offset": []string{"0"}, "limit": []string{"20"}, "model": []string{"m"}}})
	response, err = authedCall(t, runtime, requestsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"total":1`) || !strings.Contains(string(response.Body), `"model":"m"`) {
		t.Fatalf("resource requests response: %+v, %v", response, err)
	}
	requestsResultQuery := url.Values{"range": []string{"24h"}, "result": []string{"failed"}}
	requestsResultRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: requestsResultQuery})
	response, err = authedCall(t, runtime, requestsResultRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"total":0`) {
		t.Fatalf("filtered resource requests response: %+v, %v", response, err)
	}
	requestsResultQuery.Set("result", "unknown")
	requestsResultRequest, _ = json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: requestsResultQuery})
	response, err = authedCall(t, runtime, requestsResultRequest)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid request result response: %+v, %v", response, err)
	}
	customRequestsQuery := url.Values{}
	for key, values := range customQuery {
		customRequestsQuery[key] = append([]string(nil), values...)
	}
	customRequestsQuery.Set("offset", "0")
	customRequestsQuery.Set("limit", "20")
	customRequestsQuery.Set("model", "m")
	customRequestsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: customRequestsQuery})
	response, err = authedCall(t, runtime, customRequestsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"range":"custom"`) || !strings.Contains(string(response.Body), `"total":1`) {
		t.Fatalf("custom requests response: %+v, %v", response, err)
	}

	preferencesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourcePreferencesPath})
	response, err = authedCall(t, runtime, preferencesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"request_page_size":100`) || !strings.Contains(string(response.Body), `"hidden_request_columns":[]`) || !strings.Contains(string(response.Body), `"time_range_mode":"custom"`) {
		t.Fatalf("default preferences response: %+v, %v", response, err)
	}
	savePreferencesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourcePreferencesPath, Query: url.Values{"save": []string{"1"}, "request_page_size": []string{"50"}, "dimension_page_size": []string{"200"}, "hidden_request_column": []string{"source"}, "hidden_dimension_column": []string{"provider"}, "time_range_mode": []string{"last_7_days"}}})
	response, err = authedCall(t, runtime, savePreferencesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"request_page_size":50`) || !strings.Contains(string(response.Body), `"hidden_dimension_columns":["provider"]`) {
		t.Fatalf("save preferences response: %+v, %v", response, err)
	}
	response, err = authedCall(t, runtime, preferencesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"dimension_page_size":200`) || !strings.Contains(string(response.Body), `"hidden_request_columns":["source"]`) || !strings.Contains(string(response.Body), `"time_range_mode":"last_7_days"`) {
		t.Fatalf("persisted preferences response: %+v, %v", response, err)
	}

	badRequestsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: url.Values{"offset": []string{"bad"}}})
	response, _ = authedCall(t, runtime, badRequestsRequest)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad requests query status = %d", response.StatusCode)
	}

	looseContentType, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.resetPath, Headers: http.Header{"Content-Type": []string{"application/jsonp"}}, Body: []byte(`{"confirm":"reset"}`)})
	response, _ = authedCall(t, runtime, looseContentType)
	if response.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("loose content type status = %d", response.StatusCode)
	}

	badReset, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.resetPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"confirm":"no"}`)})
	response, _ = authedCall(t, runtime, badReset)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad reset status = %d", response.StatusCode)
	}
	goodReset, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.resetPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"confirm":"reset"}`)})
	response, _ = authedCall(t, runtime, goodReset)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("good reset status = %d body=%s", response.StatusCode, response.Body)
	}
}

func TestDashboardPreferencesResourceValidation(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{pluginID: "test", resourcePreferencesPath: "/v0/resource/plugins/test/preferences"}}
	defer runtime.shutdown()

	request := func(method string, query url.Values) pluginapi.ManagementResponse {
		raw, _ := json.Marshal(pluginapi.ManagementRequest{Method: method, Path: runtime.routes.resourcePreferencesPath, Query: query})
		response, handleErr := authedCall(t, runtime, raw)
		if handleErr != nil {
			t.Fatal(handleErr)
		}
		return response
	}
	if response := request(http.MethodGet, url.Values{"request_page_size": []string{"100"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("save flag omission status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"0"}, "dimension_page_size": []string{"100"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid page size status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"100"}, "dimension_page_size": []string{"100"}, "hidden_request_column": []string{"api_key"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid column status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"100"}, "dimension_page_size": []string{"100"}, "unknown": []string{"value"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown query status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"100"}, "dimension_page_size": []string{"100"}, "time_range_mode": []string{"fortnight"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid range mode status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"100"}, "dimension_page_size": []string{"100"}, "token_display_mode": []string{"billion"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid token display mode status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"100"}, "dimension_page_size": []string{"100"}, "time_range_mode": []string{"custom"}, "time_range_start": []string{"2026-08-05"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("incomplete custom range status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodPost, nil); response.StatusCode != http.StatusMethodNotAllowed || response.Headers.Get("Allow") != "GET" {
		t.Fatalf("wrong method response = %+v", response)
	}
}

func TestManagementSourceFilterAppliesToStatsAndRequests(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID: "test", resourceStatsPath: "/v0/resource/plugins/test/stats", resourceRequestsPath: "/v0/resource/plugins/test/requests",
	}}
	defer runtime.shutdown()
	for _, usage := range []normalizedUsage{
		{Dimensions: Dimensions{Model: "alpha", Source: "cli"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 3}},
		{Dimensions: Dimensions{Model: "beta", Source: "web"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 5}},
	} {
		if err := store.Record(usage); err != nil {
			t.Fatal(err)
		}
	}
	query := url.Values{"range": []string{"24h"}, "source": []string{"cli"}}
	for _, path := range []string{runtime.routes.resourceStatsPath, runtime.routes.resourceRequestsPath} {
		if path == runtime.routes.resourceRequestsPath {
			query.Set("offset", "0")
			query.Set("limit", "100")
		}
		request, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: path, Query: query})
		response, err := authedCall(t, runtime, request)
		if err != nil || response.StatusCode != http.StatusOK || strings.Contains(string(response.Body), `"source":"web"`) {
			t.Fatalf("source-filtered %s response: %+v, %v", path, response, err)
		}
		if path == runtime.routes.resourceStatsPath && (!strings.Contains(string(response.Body), `"total_tokens":3`) || !strings.Contains(string(response.Body), `"sources":["cli","web"]`)) {
			t.Fatalf("source-filtered stats response: %+v", response)
		}
		if path == runtime.routes.resourceRequestsPath && !strings.Contains(string(response.Body), `"total":1`) {
			t.Fatalf("source-filtered request response: %+v", response)
		}
	}
}

func TestManagementIgnoresLegacyAuthenticationIdentityParameters(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID: "test", resourceStatsPath: "/v0/resource/plugins/test/stats", resourceRequestsPath: "/v0/resource/plugins/test/requests",
	}}
	defer runtime.shutdown()
	for _, usage := range []normalizedUsage{
		{Dimensions: Dimensions{Model: "alpha", Source: "Codex-user@example.com"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 3}},
		{Dimensions: Dimensions{Model: "beta", Source: "Antigravity-user@example.com"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 5}},
	} {
		if err := store.Record(usage); err != nil {
			t.Fatal(err)
		}
	}
	query := url.Values{"range": []string{"24h"}, "source": []string{"Codex-user@example.com"}, "auth_provider": []string{"ignored"}, "auth_account": []string{"ignored"}}
	for _, path := range []string{runtime.routes.resourceStatsPath, runtime.routes.resourceRequestsPath} {
		if path == runtime.routes.resourceRequestsPath {
			query.Set("offset", "0")
			query.Set("limit", "100")
		}
		request, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: path, Query: query})
		response, err := authedCall(t, runtime, request)
		if err != nil || response.StatusCode != http.StatusOK || strings.Contains(string(response.Body), "\"auth_provider\"") || strings.Contains(string(response.Body), "\"auth_account\"") {
			t.Fatalf("identity-filtered %s response: %+v, %v", path, response, err)
		}
	}
}

func TestDashboardSecurityContract(t *testing.T) {
	response := dashboardResponse()
	html := string(response.Body)
	for _, required := range []string{"/v0/resource/plugins/", "/v0/management/plugins/", "Authorization", "type=\"password\"", "textContent", "replaceChildren", "showError(error)", "X-Plugin-Session", "window.sessionStorage.setItem(sessionStoreKey"} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing %q", required)
		}
	}
	for _, forbidden := range []string{"localStorage", "document.cookie", "connectButton", "logoutButton", "fetch('stats')", `fetch("stats")`} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard contains forbidden pattern %q", forbidden)
		}
	}
	if response.Headers.Get("Content-Security-Policy") == "" {
		t.Fatal("missing content security policy")
	}
}

func TestManagementBackupAndRestore(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID: "test", backupPath: "/v0/management/plugins/test/backup", restorePath: "/v0/management/plugins/test/restore",
	}}
	defer runtime.shutdown()
	if err := store.Record(normalizedUsage{
		Dimensions:  Dimensions{Model: "backup-endpoint", Source: "cli"},
		RequestedAt: nowUTC(),
		Counters:    Counters{Requests: 1, InputTokens: 4, OutputTokens: 5, TotalTokens: 9},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveDashboardPreferences(DashboardPreferences{
		RequestPageSize:   25,
		DimensionPageSize: 50,
	}); err != nil {
		t.Fatal(err)
	}

	backupRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.backupPath})
	response, err := authedCall(t, runtime, backupRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("backup response: %+v, %v", response, err)
	}
	if response.Headers.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("backup content type = %q", response.Headers.Get("Content-Type"))
	}
	if !strings.Contains(response.Headers.Get("Content-Disposition"), "attachment;") {
		t.Fatalf("backup disposition = %q", response.Headers.Get("Content-Disposition"))
	}
	if response.Headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("backup cache control = %q", response.Headers.Get("Cache-Control"))
	}
	if response.Headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("backup nosniff header = %q", response.Headers.Get("X-Content-Type-Options"))
	}
	if len(response.Body) == 0 {
		t.Fatal("expected non-empty backup body")
	}

	backupPath := filepath.Join(t.TempDir(), "endpoint-backup.db")
	if err := os.WriteFile(backupPath, response.Body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreDatabase(backupPath); err != nil {
		t.Fatalf("backup body is not a valid bolt database: %v", err)
	}
	backupBody := append([]byte(nil), response.Body...)

	emptyRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   runtime.routes.restorePath,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
	})
	response, err = authedCall(t, runtime, emptyRestore)
	if err != nil || response.StatusCode != http.StatusBadRequest || !strings.Contains(string(response.Body), "backup body must not be empty") {
		t.Fatalf("empty restore response: %+v, %v", response, err)
	}

	invalidTypeRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    runtime.routes.restorePath,
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte("not-a-database"),
	})
	response, err = authedCall(t, runtime, invalidTypeRestore)
	if err != nil || response.StatusCode != http.StatusUnsupportedMediaType || !strings.Contains(string(response.Body), "application/octet-stream") {
		t.Fatalf("invalid content type restore response: %+v, %v", response, err)
	}

	missingConfirmRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    runtime.routes.restorePath,
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
		Body:    backupBody,
	})
	response, err = authedCall(t, runtime, missingConfirmRestore)
	if err != nil || response.StatusCode != http.StatusBadRequest || !strings.Contains(string(response.Body), "missing X-Confirm-Restore: replace header") {
		t.Fatalf("missing confirm restore response: %+v, %v", response, err)
	}

	invalidBodyRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   runtime.routes.restorePath,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
		Body: []byte("not-a-database"),
	})
	response, err = authedCall(t, runtime, invalidBodyRestore)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid body restore response: %+v, %v", response, err)
	}

	// Oversized restore body is rejected at the management layer before touching storage.
	// Call restoreResponse directly to avoid base64-encoding a 64 MiB payload through JSON.
	oversizedBody := make([]byte, maxDatabaseBackupBytes+1)
	response, err = runtime.restoreResponse(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   runtime.routes.restorePath,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
		Body: oversizedBody,
	})
	if err != nil || response.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(string(response.Body), "backup body is too large") {
		t.Fatalf("oversized restore response: %+v, %v", response, err)
	}

	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	// Mutate preferences after backup so restore must reintroduce the originals.
	// Reset only clears usage counters, not preferences.
	if _, err := store.SaveDashboardPreferences(DashboardPreferences{
		RequestPageSize:   100,
		DimensionPageSize: 100,
	}); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 0 {
		t.Fatalf("expected empty stats after reset, got %+v", stats.Summary)
	}

	roundTripRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   runtime.routes.restorePath,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
		Body: backupBody,
	})
	response, err = authedCall(t, runtime, roundTripRestore)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"restored":true`) {
		t.Fatalf("round-trip restore response: %+v, %v", response, err)
	}
	stats, err = store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 9 {
		t.Fatalf("restored total tokens = %d, want 9", stats.Summary.TotalTokens)
	}
	preferences, err := store.QueryDashboardPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if preferences.RequestPageSize != 25 || preferences.DimensionPageSize != 50 {
		t.Fatalf("restored preferences = %+v, want request=25 dimension=50", preferences)
	}
}

func TestAPIKeyIdentitiesFromRequestUnionsRepeatedRefs(t *testing.T) {
	hashA := strings.Repeat("a", 32)
	hashB := strings.Repeat("b", 32)
	refA := apiKeyRef(1, hashA)
	refB := apiKeyRef(2, hashB)
	request := pluginapi.ManagementRequest{Query: url.Values{"api_key_ref": {refA, "", refB, refA}}}
	got, err := apiKeyIdentitiesFromRequest(request, nil)
	if err != nil {
		t.Fatalf("parse repeated refs: %v", err)
	}
	if len(got) != 2 || got[0] != refA || got[1] != refB {
		t.Fatalf("identities = %#v, want first-seen unique [%q %q]", got, refA, refB)
	}
}

func TestAPIKeyIdentitiesFromRequestUnionsRepeatedRefsWithoutPlaintext(t *testing.T) {
	refA := apiKeyRef(1, strings.Repeat("a", 32))
	refB := apiKeyRef(1, strings.Repeat("b", 32))
	got, err := apiKeyIdentitiesFromRequest(pluginapi.ManagementRequest{Query: url.Values{"api_key_ref": {refA, refB}}}, nil)
	if err != nil {
		t.Fatalf("repeated refs should not require key material: %v", err)
	}
	if len(got) != 2 || got[0] != refA || got[1] != refB {
		t.Fatalf("identities = %#v, want [%q %q]", got, refA, refB)
	}
}

func TestAPIKeyIdentitiesFromRequestRejectsRefAndHashTogether(t *testing.T) {
	refA := apiKeyRef(1, strings.Repeat("a", 32))
	hashB := strings.Repeat("b", 32)
	_, err := apiKeyIdentitiesFromRequest(pluginapi.ManagementRequest{Query: url.Values{"api_key_ref": {refA}, "api_key_hash": {hashB}}}, nil)
	if err == nil || errorHTTPStatus(err) != http.StatusBadRequest || err.Error() != "api_key_ref and api_key_hash cannot be used together" {
		t.Fatalf("mixed filter error = %v", err)
	}
}

func TestAPIKeyIdentitiesFromRequestRejectsRepeatedHashes(t *testing.T) {
	hashA := strings.Repeat("a", 32)
	hashB := strings.Repeat("b", 32)
	_, err := apiKeyIdentitiesFromRequest(pluginapi.ManagementRequest{Query: url.Values{"api_key_hash": {hashA, hashB}}}, nil)
	if err == nil || errorHTTPStatus(err) != http.StatusBadRequest || err.Error() != "api_key_hash cannot be repeated; use api_key_ref" {
		t.Fatalf("repeated hash error = %v", err)
	}
}

func TestDashboardPreferencesManagementSaveRoute(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{
		store:  store,
		config: config,
		routes: registeredRoutes{
			pluginID:                "test",
			resourcePreferencesPath: "/v0/resource/plugins/test/preferences",
			preferencesSavePath:     "/v0/management/plugins/test/preferences",
		},
	}
	defer runtime.shutdown()

	call := func(method, path string, query url.Values, headers http.Header, body []byte) pluginapi.ManagementResponse {
		if headers == nil {
			headers = http.Header{}
		}
		raw, err := json.Marshal(pluginapi.ManagementRequest{Method: method, Path: path, Query: query, Headers: headers, Body: body})
		if err != nil {
			t.Fatal(err)
		}
		response, err := authedCall(t, runtime, raw)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}

	wrongMethod := call(http.MethodGet, runtime.routes.preferencesSavePath, nil, nil, nil)
	if wrongMethod.StatusCode != http.StatusMethodNotAllowed || wrongMethod.Headers.Get("Allow") != http.MethodPost {
		t.Fatalf("wrong management save method response = %+v", wrongMethod)
	}

	body, err := json.Marshal(DashboardPreferences{
		RequestPageSize:      25,
		DimensionPageSize:    50,
		HiddenRequestColumns: []string{"model", "source"},
		TimeRangeMode:        "last_7_days",
		TokenDisplayMode:     "B",
		LastDashboardOpenAt:  "2026-08-23T04:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := call(http.MethodPost, runtime.routes.preferencesSavePath, nil, http.Header{"Content-Type": []string{"application/json"}}, body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("management preferences save response = %+v body=%s", response, response.Body)
	}
	var saved DashboardPreferences
	if err := json.Unmarshal(response.Body, &saved); err != nil || saved.RequestPageSize != 25 || saved.DimensionPageSize != 50 || len(saved.HiddenRequestColumns) != 2 || saved.TimeRangeMode != "last_7_days" || saved.TokenDisplayMode != "B" || saved.LastDashboardOpenAt != "2026-08-23T04:00:00Z" {
		t.Fatalf("saved preferences payload = %s, err = %v", response.Body, err)
	}

	invalidBody := []byte(`{"request_page_size":0,"dimension_page_size":50}`)
	response = call(http.MethodPost, runtime.routes.preferencesSavePath, nil, http.Header{"Content-Type": []string{"application/json"}}, invalidBody)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid management preferences save response = %+v body=%s", response, response.Body)
	}

	response = call(http.MethodGet, runtime.routes.resourcePreferencesPath, nil, nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("resource preferences read response = %+v body=%s", response, response.Body)
	}
	saved = DashboardPreferences{}
	if err := json.Unmarshal(response.Body, &saved); err != nil || saved.RequestPageSize != 25 || saved.DimensionPageSize != 50 || saved.TokenDisplayMode != "B" || saved.LastDashboardOpenAt != "2026-08-23T04:00:00Z" {
		t.Fatalf("stored preferences payload = %s, err = %v", response.Body, err)
	}

	legacyQuery := url.Values{"save": {"1"}, "request_page_size": {"75"}, "dimension_page_size": {"100"}, "token_display_mode": {"m"}}
	response = call(http.MethodGet, runtime.routes.resourcePreferencesPath, legacyQuery, nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("legacy resource preferences save response = %+v body=%s", response, response.Body)
	}
	response = call(http.MethodGet, runtime.routes.resourcePreferencesPath, nil, nil, nil)
	saved = DashboardPreferences{}
	if err := json.Unmarshal(response.Body, &saved); err != nil || saved.RequestPageSize != 75 || saved.DimensionPageSize != 100 || saved.TokenDisplayMode != "m" {
		t.Fatalf("legacy stored preferences payload = %s, err = %v", response.Body, err)
	}
}

func TestManagementMultiModelFilterScopesDashboardRoutes(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID:                 "test",
		resourceStatsInitialPath: "/v0/resource/plugins/test/stats/initial",
		resourceStatsGroupsPath:  "/v0/resource/plugins/test/stats/groups",
		resourceRequestsPath:     "/v0/resource/plugins/test/requests",
	}}
	defer runtime.shutdown()
	for model, tokens := range map[string]uint64{"alpha": 3, "beta": 5, "gamma": 7} {
		if err := store.Record(normalizedUsage{
			Dimensions:  Dimensions{Model: model, Source: "cli"},
			RequestedAt: nowUTC(),
			Counters:    Counters{Requests: 1, TotalTokens: tokens},
		}); err != nil {
			t.Fatal(err)
		}
	}
	call := func(path string, query url.Values) pluginapi.ManagementResponse {
		t.Helper()
		raw, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: path, Query: query})
		response, callErr := authedCall(t, runtime, raw)
		if callErr != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("%s response = %+v, %v", path, response, callErr)
		}
		return response
	}

	initial := call(runtime.routes.resourceStatsInitialPath, url.Values{"range": {"24h"}, "model": {"alpha", "beta", "alpha"}})
	var stats InitialStatsResponse
	if err := json.Unmarshal(initial.Body, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != 2 || stats.Summary.TotalTokens != 8 {
		t.Fatalf("multi-model summary = %+v", stats.Summary)
	}
	if len(stats.Models) != 2 {
		t.Fatalf("multi-model rows = %+v", stats.Models)
	}
	for _, row := range stats.Models {
		if row.Model != "alpha" && row.Model != "beta" {
			t.Fatalf("unexpected model row %+v", row)
		}
	}

	groups := call(runtime.routes.resourceStatsGroupsPath, url.Values{"range": {"24h"}, "model": {"gamma"}, "offset": {"0"}, "limit": {"50"}})
	var page GroupStatsPage
	if err := json.Unmarshal(groups.Body, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].Model != "gamma" {
		t.Fatalf("group model filter = %+v", page)
	}

	requests := call(runtime.routes.resourceRequestsPath, url.Values{"range": {"24h"}, "model": {"alpha", "gamma"}, "offset": {"0"}, "limit": {"50"}})
	var requestPage RequestPage
	if err := json.Unmarshal(requests.Body, &requestPage); err != nil {
		t.Fatal(err)
	}
	if requestPage.Total != 2 {
		t.Fatalf("request model filter total = %d", requestPage.Total)
	}
	for _, item := range requestPage.Items {
		if item.Model == "beta" {
			t.Fatalf("request model filter leaked beta: %+v", item)
		}
	}
}

func TestManagementInitialStatsExposeAPIKeyFilterOptions(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = defaultAPIKeySecret
	config.SyncOnRecord = true
	crypto, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openStoreWithCrypto(config, crypto)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{
		store:  store,
		config: config,
		crypto: crypto,
		routes: registeredRoutes{
			pluginID:                 "test",
			resourceStatsInitialPath: "/v0/resource/plugins/test/stats/initial",
		},
	}
	defer runtime.shutdown()
	keyA := "test-client-key-alpha"
	keyB := "test-client-key-beta"
	for index, key := range []string{keyA, keyA, keyB} {
		record := pluginapi.UsageRecord{
			Provider:    "test",
			Model:       "model-alpha",
			Source:      "cli",
			APIKey:      key,
			RequestedAt: time.Now().UTC().Add(time.Duration(index-3) * time.Second),
			Detail:      pluginapi.UsageDetail{InputTokens: int64(index + 1), OutputTokens: 1, TotalTokens: int64(index + 2)},
		}
		raw, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, err := runtime.handleUsage(raw); err != nil {
			t.Fatal(err)
		}
	}
	hashA := apiKeyFingerprint(keyA, crypto.indexKey)
	refA := apiKeyRef(1, hashA)
	if err := store.SetAPIKeyLabel(refA, "cup"); err != nil {
		t.Fatal(err)
	}
	call := func(query url.Values) InitialStatsResponse {
		t.Helper()
		raw, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsInitialPath, Query: query})
		response, callErr := authedCall(t, runtime, raw)
		if callErr != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("initial stats = %+v, %v", response, callErr)
		}
		if strings.Contains(string(response.Body), `"key":`) || strings.Contains(string(response.Body), `"api_key_hash"`) {
			t.Fatalf("ordinary response leaked key material: %s", response.Body)
		}
		var stats InitialStatsResponse
		if err := json.Unmarshal(response.Body, &stats); err != nil {
			t.Fatal(err)
		}
		return stats
	}

	stats := call(url.Values{"range": {"24h"}})
	if len(stats.APIKeys) != 2 {
		t.Fatalf("api key options = %+v", stats.APIKeys)
	}
	byRef := map[string]APIKeyOption{}
	for _, option := range stats.APIKeys {
		byRef[option.Ref] = option
	}
	optionA, ok := byRef[refA]
	if !ok {
		t.Fatalf("missing ref for key A: %+v", stats.APIKeys)
	}
	if optionA.Label != "cup" || optionA.Requests != 2 || optionA.TotalTokens == 0 {
		t.Fatalf("labeled option = %+v", optionA)
	}
	if optionA.Key != "" || optionA.Hash != "" || optionA.Generation != 0 {
		t.Fatalf("ordinary response leaked key identity: %+v", optionA)
	}

	// Filtering by the fingerprint works with any dashboard session.
	filtered := call(url.Values{"range": {"24h"}, "api_key_ref": {refA}})
	if filtered.Summary.Requests != 2 {
		t.Fatalf("filtered summary = %+v", filtered.Summary)
	}
	if len(filtered.APIKeys) != 2 {
		t.Fatalf("filtered options should still list every key: %+v", filtered.APIKeys)
	}
	for _, option := range filtered.APIKeys {
		if option.Ref != refA && option.TotalTokens != 0 {
			t.Fatalf("unselected key should report zero counters: %+v", option)
		}
	}
}

func TestInitialStatsScaleAndRequestedScale(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID:                 "test",
		resourceStatsInitialPath: "/v0/resource/plugins/test/stats/initial",
	}}
	defer runtime.shutdown()

	if err := store.Record(normalizedUsage{
		Dimensions:  Dimensions{Model: "gpt-test", Source: "cli"},
		RequestedAt: nowUTC(),
		Counters:    Counters{Requests: 1, TotalTokens: 10},
	}); err != nil {
		t.Fatal(err)
	}

	call := func(query url.Values) (InitialStatsResponse, int, string) {
		t.Helper()
		raw, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsInitialPath, Query: query})
		response, callErr := authedCall(t, runtime, raw)
		if callErr != nil {
			t.Fatalf("call error: %v", callErr)
		}
		var stats InitialStatsResponse
		_ = json.Unmarshal(response.Body, &stats)
		return stats, response.StatusCode, string(response.Body)
	}

	// 1. 30d with scale=minute -> clamped to hour, requested_scale="minute"
	stats, code, body := call(url.Values{"range": {"30d"}, "scale": {"minute"}})
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if stats.Scale != "hour" || stats.RequestedScale != "minute" {
		t.Fatalf("expected hour / minute, got scale=%q requested_scale=%q", stats.Scale, stats.RequestedScale)
	}
	if !strings.Contains(body, `"requested_scale":"minute"`) {
		t.Fatalf("body missing requested_scale JSON: %s", body)
	}

	// 2. 30d with scale=hour -> fits cap 1440, scale="hour", requested_scale omitted
	stats, code, body = call(url.Values{"range": {"30d"}, "scale": {"hour"}})
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if stats.Scale != "hour" || stats.RequestedScale != "" {
		t.Fatalf("expected hour / empty, got scale=%q requested_scale=%q", stats.Scale, stats.RequestedScale)
	}
	if strings.Contains(body, `"requested_scale"`) {
		t.Fatalf("body should omit requested_scale when not clamped: %s", body)
	}

	// 3. 24h with scale=minute -> fits cap 1440, scale="minute", requested_scale omitted
	stats, code, body = call(url.Values{"range": {"24h"}, "scale": {"minute"}})
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if stats.Scale != "minute" || stats.RequestedScale != "" {
		t.Fatalf("expected minute / empty, got scale=%q requested_scale=%q", stats.Scale, stats.RequestedScale)
	}
	if strings.Contains(body, `"requested_scale"`) {
		t.Fatalf("body should omit requested_scale when not clamped: %s", body)
	}

	// 4. Invalid scale returns 400
	_, code, _ = call(url.Values{"range": {"30d"}, "scale": {"invalid"}})
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid scale, got %d", code)
	}
}
