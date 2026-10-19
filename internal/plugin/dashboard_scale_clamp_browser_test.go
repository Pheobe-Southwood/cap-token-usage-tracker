package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDashboardScaleClampInBrowser tests that choosing an overly fine scale
// (e.g. minute on a 30d range) triggers an explicit localized clamp note,
// redraws the axis at the widened scale, and does not silently substitute it.
func TestDashboardScaleClampInBrowser(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		browserTestUnavailable(t, "Node.js", err)
	}
	chrome, err := resolveBrowserTestChrome()
	if err != nil {
		browserTestUnavailable(t, "Google Chrome (set CHROME_PATH to its executable)", err)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "node_modules", "playwright-core", "package.json")); err != nil {
		browserTestUnavailable(t, "playwright-core (run npm ci)", err)
	}

	htmlPath := filepath.Join(t.TempDir(), "dashboard.html")
	if err := os.WriteFile(htmlPath, []byte(dashboardHTML), 0o600); err != nil {
		t.Fatalf("write dashboard response body: %v", err)
	}

	command := exec.Command(node, filepath.Join("..", "..", "test", "dashboard_scale_clamp.mjs"), htmlPath, chrome)
	command.Env = append(os.Environ(), "TZ=UTC")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("scale clamp browser regression failed: %v\n%s", err, output)
	}
}
