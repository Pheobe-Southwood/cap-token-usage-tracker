package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDashboardTokenCostTabInBrowser drives the split Token/花销 tabs against a
// mock host: the shared filter bar, the time-axis scale control, the cost tab
// mirroring the token layout, and the widened request log columns.
func TestDashboardTokenCostTabInBrowser(t *testing.T) {
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

	command := exec.Command(node, filepath.Join("..", "..", "test", "dashboard_token_cost_tab.mjs"), htmlPath, chrome)
	command.Env = append(os.Environ(), "TZ=UTC")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("token/cost tab browser regression failed: %v\n%s\nInstall the pinned browser-test dependency with `npm ci`; this test uses the installed Google Chrome and does not download a browser.", err, output)
	}
}
