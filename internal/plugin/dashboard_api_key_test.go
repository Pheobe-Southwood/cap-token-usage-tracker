package plugin

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestAPIKeyDashboardFilters(t *testing.T) {
	for name, html := range map[string]string{"ordinary": dashboardHTML, "full": dashboardHTML} {
		t.Run(name, func(t *testing.T) {
			for _, required := range []string{
				`id="apiKeyButton"`,
				`id="apiKeyMenu"`,
				`function buildApiKeyMenu()`,
				`function keyLabel(entry)`,
				`params.append('api_key_ref',ref)`,
				`state.keys.indexOf(entry.ref)`,
				`entry.label`,
				`state.catalog.api_keys`,
			} {
				if !strings.Contains(html, required) {
					t.Fatalf("%s dashboard missing API-key element %q", name, required)
				}
			}
			for _, forbidden := range []string{
				`full-mode/api-key-labels`,
				`X-Full-Mode-Session`,
				`initializeAPIKeyFullMode`,
				`id="apiKeyLabelDialog"`,
			} {
				if strings.Contains(html, forbidden) {
					t.Fatalf("%s dashboard still depends on the removed full-mode flow: %q", name, forbidden)
				}
			}
		})
	}
}

func TestAPIKeyLocaleCatalog(t *testing.T) {
	required := []string{
		"tab.usage",
		"apiKey.filterAll",
		"apiKey.filterHint",
		"apiKey.count",
	}
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		data, err := localeFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			t.Fatalf("locale %s: %v", code, err)
		}
		for _, key := range required {
			if catalog[key] == "" {
				t.Fatalf("locale %s missing %q", code, key)
			}
		}
	}
}

func TestGeneratedDashboardJavaScriptSyntax(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	for name, html := range map[string]string{"ordinary": dashboardHTML, "full": dashboardHTML} {
		t.Run(name, func(t *testing.T) {
			remaining := html
			for index := 0; ; index++ {
				start := strings.Index(remaining, "<script")
				if start < 0 {
					break
				}
				openEnd := strings.Index(remaining[start:], ">")
				if openEnd < 0 {
					t.Fatal("unterminated script tag")
				}
				openEnd += start
				closeAt := strings.Index(remaining[openEnd+1:], "</script>")
				if closeAt < 0 {
					t.Fatal("unterminated script body")
				}
				closeAt += openEnd + 1
				script := remaining[openEnd+1 : closeAt]
				command := exec.Command(node, "--check", "-")
				command.Stdin = strings.NewReader(script)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("script %d syntax: %v\n%s", index, err, output)
				}
				remaining = remaining[closeAt+len("</script>"):]
			}
		})
	}
}
