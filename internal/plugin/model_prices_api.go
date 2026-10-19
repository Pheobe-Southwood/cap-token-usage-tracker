package plugin

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"

	"github.com/AITNR/cap-token-usage-tracker/internal/plugin/modelcatalog"
)

const (
	// maxModelPriceBodyBytes bounds a management price request body.
	maxModelPriceBodyBytes = 1 << 20
	// modelPriceSaveQueryParam is the resource-route save flag (resource routes
	// are GET-only, mirroring the preferences compatibility path).
	modelPriceSaveQueryParam = "save"
)

// modelsResponse serves the models tab: per-model usage, both TPS variants,
// average TTFT, the list-price cost and the catalog status. The sibling
// opencode-go plugin's hourly TTFT statistics are linked in because the host
// reports no TTFT for plugin executors.
func (r *pluginRuntime) modelsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	r.mu.RLock()
	pricing := r.pricing
	store := r.store
	config := r.config
	linkageCache := r.ttftLinkage
	r.mu.RUnlock()
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	queryRange, filter, err := r.statsFilter(request)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	var pricer modelPricing
	if pricing != nil {
		pricer = pricing
	}
	var linkage *ttftLinkage
	if linkageCache != nil {
		linkage = linkageCache.snapshot(config)
	}
	response, err := store.queryModelsByFilter(queryRange, filter, pricer, linkage)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, response.withPricing(r.pricingStatus())), nil
}

// modelPricesResponse implements the GET-only resource route:
//
//	GET /model-prices                              → current overrides and status
//	GET /model-prices?save=1&model=…&input=…       → upsert one override
//	GET /model-prices?reset=1&model=…              → drop one override
//	GET /model-prices?refresh=1                    → refresh the catalog
func (r *pluginRuntime) modelPricesResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	switch {
	case len(request.Query) == 0:
		return r.modelPricesSnapshotResponse(), nil
	case request.Query.Get("refresh") != "":
		return r.refreshModelCatalogResponse(), nil
	case request.Query.Get("reset") != "":
		return r.resetModelPriceResponse(request)
	case request.Query.Get(modelPriceSaveQueryParam) != "":
		price, model, err := modelPriceFromQuery(request.Query)
		if err != nil {
			return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
		}
		// The legacy GET form only carries rates; keep a stored weekly schedule
		// so a script that edits a price cannot silently drop peak pricing.
		r.preserveTimePlan(model, &price)
		return r.persistModelPrice(model, &price)
	default:
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "unsupported model price query"}), nil
	}
}

// saveModelPriceResponse implements the management PUT route used by scripts.
func (r *pluginRuntime) saveModelPriceResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	contentType, _, err := mime.ParseMediaType(request.Headers.Get("Content-Type"))
	if err != nil || !strings.EqualFold(contentType, "application/json") {
		return jsonResponse(http.StatusUnsupportedMediaType, map[string]any{"error": "Content-Type must be application/json"}), nil
	}
	if len(request.Body) > maxModelPriceBodyBytes {
		return jsonResponse(http.StatusRequestEntityTooLarge, map[string]any{"error": "model price JSON is too large"}), nil
	}
	var body struct {
		Model string      `json:"model"`
		Price *ModelPrice `json:"price"`
	}
	if err := decodeStrictJSON(request.Body, &body); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid model price JSON: " + err.Error()}), nil
	}
	model, err := normalizeOverrideModel(body.Model)
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
	}
	return r.persistModelPrice(model, body.Price)
}

func (r *pluginRuntime) modelPricesSnapshotResponse() pluginapi.ManagementResponse {
	table := r.pricingTableRef()
	overrides := map[string]ModelPrice{}
	if table != nil {
		if stored := table.Overrides(); len(stored) > 0 {
			overrides = stored
		}
	}
	return jsonResponse(http.StatusOK, map[string]any{
		"pricing":   r.pricingStatus(),
		"overrides": overrides,
	})
}

func (r *pluginRuntime) refreshModelCatalogResponse() pluginapi.ManagementResponse {
	table := r.pricingTableRef()
	if table == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "model catalog is unavailable"})
	}
	status, err := table.RefreshCatalog(context.Background())
	payload := map[string]any{
		"ok":      err == nil,
		"pricing": pricingStatus{Status: status, Overrides: table.OverrideCount()},
	}
	if err != nil {
		if errors.Is(err, modelcatalog.ErrRefreshTooSoon) {
			payload["reason"] = "too_soon"
		}
		payload["error"] = err.Error()
	}
	return jsonResponse(http.StatusOK, payload)
}

func (r *pluginRuntime) resetModelPriceResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if request.Query.Get("reset") != "1" {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "reset must be 1"}), nil
	}
	model, err := normalizeOverrideModel(request.Query.Get("model"))
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()}), nil
	}
	return r.persistModelPrice(model, nil)
}

// preserveTimePlan copies a stored weekly schedule onto a rate-only save.
func (r *pluginRuntime) preserveTimePlan(model string, price *ModelPrice) {
	table := r.pricingTableRef()
	if table == nil {
		return
	}
	stored := table.Overrides()[model]
	price.TimeZone = stored.TimeZone
	price.TimeTiers = cloneTimeTiers(stored.TimeTiers)
}

// persistModelPrice writes one override (nil clears it) and returns the fresh
// override table plus the resolved price of that model.
func (r *pluginRuntime) persistModelPrice(model string, price *ModelPrice) (pluginapi.ManagementResponse, error) {
	r.mu.RLock()
	store := r.store
	table := r.pricing
	r.mu.RUnlock()
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	overrides, err := store.SetModelPriceOverride(model, price)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	if table != nil {
		table.SetOverrides(overrides)
	}
	resolved := resolvedPrice{Source: priceSourceNone}
	if table != nil {
		resolved = table.PriceFor(Dimensions{Model: model})
	}
	payload := map[string]any{
		"pricing":   r.pricingStatus(),
		"overrides": overrides,
		"model":     model,
		"price":     resolved,
	}
	return jsonResponse(http.StatusOK, payload), nil
}

// modelPriceFromQuery parses the legacy GET save form. Omitted fields inherit
// the catalog rate; an empty price clears the override.
func modelPriceFromQuery(query map[string][]string) (ModelPrice, string, error) {
	allowed := map[string]struct{}{
		modelPriceSaveQueryParam: {}, "model": {},
		"input": {}, "output": {}, "cache_read": {}, "cache_write": {},
	}
	for key := range query {
		if _, ok := allowed[key]; !ok {
			return ModelPrice{}, "", withStatus(http.StatusBadRequest, "unsupported model price query parameter %q", key)
		}
	}
	if values := query[modelPriceSaveQueryParam]; len(values) != 1 || values[0] != "1" {
		return ModelPrice{}, "", withStatus(http.StatusBadRequest, "save must be 1")
	}
	model, err := normalizeOverrideModel(singleQueryValue(query, "model"))
	if err != nil {
		return ModelPrice{}, "", withStatus(http.StatusBadRequest, "%v", err)
	}
	price := ModelPrice{}
	for _, field := range []struct {
		name   string
		target **float64
	}{
		{name: "input", target: &price.Input},
		{name: "output", target: &price.Output},
		{name: "cache_read", target: &price.CacheRead},
		{name: "cache_write", target: &price.CacheWrite},
	} {
		raw, present := query[field.name]
		if !present {
			continue
		}
		if len(raw) != 1 {
			return ModelPrice{}, "", withStatus(http.StatusBadRequest, "%s must be given at most once", field.name)
		}
		trimmed := strings.TrimSpace(raw[0])
		if trimmed == "" {
			continue
		}
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return ModelPrice{}, "", withStatus(http.StatusBadRequest, "%s must be a decimal number", field.name)
		}
		pointer := value
		*field.target = &pointer
	}
	return price, model, nil
}

func singleQueryValue(query map[string][]string, name string) string {
	values := query[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
