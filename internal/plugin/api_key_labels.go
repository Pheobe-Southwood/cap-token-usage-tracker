package plugin

import (
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// setAPIKeyLabelResponse handles PUT /v0/management/plugins/<id>/api-key-labels.
// The route lives on the management API, so the host has already verified the
// CLIProxyAPI management key before the plugin sees the request. Labels are
// display names for keyed fingerprints; they never contain key material.
func (r *pluginRuntime) setAPIKeyLabelResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if contentType := strings.TrimSpace(request.Headers.Get("Content-Type")); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || !strings.EqualFold(mediaType, "application/json") {
			return jsonResponse(http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"}), nil
		}
	}
	if len(request.Body) > 16<<10 {
		return jsonResponse(http.StatusRequestEntityTooLarge, map[string]string{"error": "API key label JSON is too large"}), nil
	}
	if !utf8.Valid(request.Body) {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "API key label JSON must be valid UTF-8"}), nil
	}
	var input struct {
		Ref   string `json:"ref"`
		Label string `json:"label"`
	}
	if err := decodeStrictJSON(request.Body, &input); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid API key label JSON"}), nil
	}
	if err := validateAPIKeyLabel(input.Ref, input.Label); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()}), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]string{"error": "storage is not initialized"}), nil
	}
	if err := r.store.SetAPIKeyLabel(input.Ref, input.Label); err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]string{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{"saved": true, "ref": input.Ref, "label": input.Label}), nil
}
