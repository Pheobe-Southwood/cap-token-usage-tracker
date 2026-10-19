package plugin

import (
	"embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

//go:embed locales/*.json
var localeFS embed.FS

var dashboardHTML string

func init() {
	localeData := make(map[string]map[string]string)
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		data, err := localeFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			panic("locale missing: " + code)
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			panic("locale invalid JSON: " + code)
		}
		localeData[code] = m
	}
	localeJSONBytes, err := json.Marshal(localeData)
	if err != nil {
		panic("locale marshal failed: " + err.Error())
	}
	rendered := strings.Replace(dashboardHTMLTemplate, "/*LOCALE_PLACEHOLDER*/", string(localeJSONBytes), 1)
	dashboardHTML = rendered
}

func dashboardResponse() pluginapi.ManagementResponse {
	return dashboardHTMLResponse(dashboardHTML)
}

func dashboardHTMLResponse(html string) pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type":           []string{"text/html; charset=utf-8"},
			"Cache-Control":          []string{"no-store"},
			"X-Content-Type-Options": []string{"nosniff"},
			"Referrer-Policy":        []string{"no-referrer"},
			"Content-Security-Policy": []string{
				"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src data: blob:; base-uri 'none'; form-action 'none'; frame-ancestors 'self'",
			},
		},
		Body: []byte(html),
	}
}
