package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func resolveBrowserTestChrome() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CHROME_PATH")); configured != "" {
		if filepath.IsAbs(configured) || strings.ContainsAny(configured, `/\\`) {
			if _, err := os.Stat(configured); err != nil {
				return "", err
			}
			return configured, nil
		}
		chrome, err := exec.LookPath(configured)
		if err != nil {
			return "", err
		}
		return chrome, nil
	}

	if runtime.GOOS == "darwin" {
		const chrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(chrome); err != nil {
			return "", err
		}
		return chrome, nil
	}
	if runtime.GOOS == "linux" {
		for _, candidate := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
			if chrome, err := exec.LookPath(candidate); err == nil {
				return chrome, nil
			}
		}
	}

	return "", os.ErrNotExist
}

func browserTestUnavailable(t *testing.T, prerequisite string, err error) {
	t.Helper()
	if os.Getenv("REQUIRE_BROWSER_TESTS") == "1" {
		t.Fatalf("browser tests are required but %s is unavailable: %v", prerequisite, err)
	}
	t.Skipf("browser tests skipped because %s is unavailable: %v", prerequisite, err)
}

func runDashboardDateRangeBrowserTest(t *testing.T, scenario, description string) {
	t.Helper()
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
	if err := os.WriteFile(htmlPath, dashboardResponse().Body, 0o600); err != nil {
		t.Fatalf("write dashboard response body: %v", err)
	}

	command := exec.Command(node, filepath.Join("..", "..", "test", "dashboard_date_range.mjs"), htmlPath, chrome, scenario)
	command.Env = append(os.Environ(), "TZ=UTC")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s\nInstall the pinned browser-test dependency with `npm ci`; this test uses the installed Google Chrome and does not download a browser.", description, err, output)
	}
}

func TestDashboardRangePresetsInBrowser(t *testing.T) {
	runDashboardDateRangeBrowserTest(t, "presets", "dashboard range preset browser regression failed")
}

func TestDashboardCustomRangeInBrowser(t *testing.T) {
	runDashboardDateRangeBrowserTest(t, "custom", "dashboard custom range browser regression failed")
}
