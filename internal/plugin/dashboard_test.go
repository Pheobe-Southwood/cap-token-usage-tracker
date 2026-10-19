package plugin

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestDashboardUsesBoundedSafeRendering(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"document.createDocumentFragment()",
		"AbortController",
		"setTimeout(function(){controller.abort();},timeout||25000)",
		"function bucketScale(){",
		"svg.replaceChildren",
		"svgFragment.appendChild",
		"var resourceBase=publicPathPrefix+'/v0/resource/plugins/'",
		"var statsInitialURL=resourceBase+'/stats/initial'",
		"showError(error)",
		"window.parent.document.documentElement",
		"new MutationObserver",
		"attributeFilter:['data-theme','style','class','lang']",
		"initializeThemeSync()",
		"window.matchMedia",
		"supportedLocales=['en','zh-CN','zh-TW','ru']",
		"function detectLocale()",
		"function normalizeLocale(value)",
		"navigator.languages",
		"window.addEventListener('languagechange'",
		"document.documentElement.lang=locale",
		"formatterLocale=locale==='zh-CN'?'zh-CN':locale==='zh-TW'?'zh-TW':locale==='ru'?'ru-RU':'en-US'",
		"function translateStatic()",
		"function localeNumber(value,options)",
		"function localeDate(value,options)",
		"<html lang=\"zh-CN\" data-theme=\"dark\" style=\"background:#151412;color-scheme:dark\">",
		"<meta name=\"color-scheme\" content=\"dark light\">",
		"<style id=\"initial-theme\">",
		"html{background:#151412;color-scheme:dark}",
		"html:not([data-theme]){background:#faf9f5;color-scheme:light}",
		"html[data-theme='white']{background:#fff;color-scheme:light}",
		"html[data-theme='dark']{background:#151412;color-scheme:dark}",
		"var theme='dark',background='#151412';",
		"getComputedStyle(parentRoot).getPropertyValue('--bg-secondary')",
		"window.frameElement.style.backgroundColor=background",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"innerHTML",
		"localStorage",
		"document.cookie",
		"connectButton",
		"logoutButton",
		"fetch('stats')",
		`fetch("stats")`,
		`costFor(`,
		`fetch('https://models.dev`,
		`fetch("https://models.dev`,
		`fetch('https://open.er-api.com`,
		`fetch("https://open.er-api.com`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard contains unsafe pattern %q", forbidden)
		}
	}
}

func TestDashboardAnimatesEveryDropdownMenu(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`.dropdown-surface{position:fixed;z-index:900;border:1px solid var(--line);border-radius:16px;background:var(--card);box-shadow:var(--shadow-pop);opacity:0;pointer-events:none;transform:translateY(-6px) scale(.985)`,
		`.dropdown-surface.is-open{opacity:1;pointer-events:auto;transform:translateY(0) scale(1)}`,
		`.dropdown-surface.is-closing{transition-duration:120ms}`,
		`function openDropdownSurface(menu,button,position)`,
		`function closeDropdownSurface(menu,button,restoreFocus)`,
		`event.propertyName==='opacity'`,
		`menu.classList.add('is-opening')`,
		`menu.classList.add('is-closing')`,
		`@media(prefers-reduced-motion:reduce)`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing dropdown animation contract %q", required)
		}
	}
}

func TestDashboardSingleUsageTabLayout(t *testing.T) {
	for name, html := range map[string]string{"ordinary": dashboardHTML, "full": dashboardHTML} {
		t.Run(name, func(t *testing.T) {
			// Exactly five tabs, in a fixed order:
			// Token → 花销 → 模型 → 请求日志 → 数据管理.
			order := []string{`data-tab="usage"`, `data-tab="cost"`, `data-tab="models"`, `data-tab="requests"`, `data-tab="data"`}
			position := -1
			for _, marker := range order {
				index := strings.Index(html, marker)
				if index < 0 || index < position {
					t.Fatalf("%s dashboard tab order is wrong at %q", name, marker)
				}
				position = index
			}
			for _, required := range []string{
				`id="panelUsage"`,
				`id="panelCost"`,
				`id="panelModels"`,
				`id="panelRequests"`,
				`id="panelData"`,

				// One shared filter bar for every data tab.
				`id="globalFilterBar"`,
				`id="rangeButton"`,
				`id="modelButton"`,
				`id="apiKeyButton"`,
				`id="modelMenu"`,
				`id="apiKeyMenu"`,
				`data-i18n="filter.timeDimension"`,
				`data-i18n="filter.model"`,
				`params.append('model',model)`,
				`params.append('api_key_ref',ref)`,

				// Hero + stat cards.
				`id="totalTokens"`,
				`id="tokenStack"`,
				`id="tokenLegend"`,
				`id="totalRequests"`,
				`id="cacheHitRate"`,

				// Token tab: one stacked bar chart, one request trend chart and
				// two rankings. Cost lives on its own tab.
				`id="tokenChart"`,
				`id="requestChart"`,
				`id="modelRank"`,
				`id="apiKeyRank"`,
				`data-rank="tokens"`,
				`data-rank="requests"`,

				// Time-axis scale control.
				`id="scaleControl"`,
				`id="scaleHint"`,
				`data-scale="auto"`,
				`data-scale="minute"`,
				`data-scale="hour"`,
				`data-scale="day"`,
				`function bucketScale(){`,
				`function bucketLabel(date){`,
				`function scaleClampText(data){`,
				`function scaleNoteText(data){`,
				`requested_scale`,
				`params.set('tz_offset',String(-new Date().getTimezoneOffset()))`,

				// Responsive behaviour.
				`@media(max-width:768px)`,
				`@media(min-width:769px)`,
			} {
				if !strings.Contains(html, required) {
					t.Fatalf("%s dashboard missing layout element %q", name, required)
				}
			}
			for _, forbidden := range []string{
				`id="donut"`,
				`modelShare.chart.aria`,
				`id="statDimensionButton"`,
				`id="panelOverview"`,
				`id="panelDimensions"`,
				`id="panelApiKeys"`,
				// The metric toggle and the duplicated request trend are gone:
				// the token chart is always the stacked bar chart.
				`id="metricControl"`,
				`data-metric="tokens"`,
			} {
				if strings.Contains(html, forbidden) {
					t.Fatalf("%s dashboard still contains removed element %q", name, forbidden)
				}
			}
		})
	}
}

func TestDashboardModelsTabPricingSurface(t *testing.T) {
	for name, html := range map[string]string{"ordinary": dashboardHTML, "full": dashboardHTML} {
		t.Run(name, func(t *testing.T) {
			for _, required := range []string{
				// Models tab surface.
				`id="panelModels"`,
				`id="modelSearch"`,
				`id="modelsTotalCost"`,
				`id="modelsCount"`,
				`id="modelList"`,
				`id="catalogNote"`,
				`id="refreshCatalog"`,
				`data-i18n="models.generationTps"`,
				`data-i18n="models.endToEndTps"`,
				`data-i18n="models.avgTtft"`,

				// Manual price dialog.
				`id="priceDialog"`,
				`id="priceInput"`,
				`id="priceOutput"`,
				`id="priceCacheRead"`,
				`id="priceCacheWrite"`,
				`id="priceReset"`,
				`id="priceSave"`,

				// Time-of-use schedule editor inside the price dialog.
				`id="timePlan"`,
				`id="timePlanZone"`,
				`id="timePlanSource"`,
				`id="timeTierList"`,
				`id="timeTierAdd"`,
				`function renderTimePlan(pricing){`,
				`function collectTimePlan(){`,
				`function makeTimeTierRow(tier){`,
				`function timePlanSummary(plan){`,
				`time_plan_source`,
				`time_tiers`,
				// The schedule travels as a JSON PUT so tiers are not truncated
				// into query parameters.
				`method:'PUT'`,
				`id="costTimeTierNote"`,
				`time_tier_cost_usd`,
				`function formatSignedMoney(value){`,

				// TTFT linkage surface: the models table says where the
				// first-token latency came from.
				`id="ttftNote"`,
				`function renderTTFTNote(data)`,
				`supplemented_ttft_samples`,
				`models.ttftLinked`,

				// Cost tab surface.
				`id="panelCost"`,
				`id="costTotal"`,
				`id="costStack"`,
				`id="costLegend"`,
				`id="costPricedModels"`,
				`id="costUnpricedModels"`,
				`id="costCoverage"`,
				`id="costChart"`,
				`id="costChartLegend"`,
				`id="costDimensionControl"`,
				`data-cost-dimension="model"`,
				`data-cost-dimension="key"`,
				`id="costModelRank"`,
				`id="costApiKeyRank"`,
				`function renderCost(){`,
				`function renderCostChart(){`,
				`function costChartSeries(points,data){`,
				`function renderSeriesLegend(hostId,rows){`,
				`function renderCostRankings(){`,
				// The key grouping resolves each breakdown ref against the
				// payload key list before drawing.
				`function breakdownKeyLabel(ref,data){`,
				`for(var j=0;j<list.length;j++){`,
				`cost_series`,
				`cost_by_model`,
				`cost_by_key`,

				// Wire format.
				`var statsModelsURL=resourceBase+'/stats/models';`,
				`var modelPricesURL=resourceBase+'/model-prices';`,
				`function formatMoney(value)`,
				`function loadModels()`,
				`function openPriceDialog(model)`,
			} {
				if !strings.Contains(html, required) {
					t.Fatalf("%s dashboard missing models surface %q", name, required)
				}
			}
			// The browser must never contact the price catalog itself: the
			// plugin performs the download behind its own session-gated route.
			for _, forbidden := range []string{
				"models.dev",
				"https://open.er-api.com",
				"currencyButton",
				"displayCurrency",
				"accounting_mode",
				// Regression: the key-label lookup used to index the outer list
				// with the inner cursor, so any breakdown ref past the second
				// key threw and left the previous (model) histogram on screen.
				`lists[j]`,
			} {
				if strings.Contains(html, forbidden) {
					t.Fatalf("%s dashboard still contains %q", name, forbidden)
				}
			}
			// The cost trend is a stacked histogram: the cost chart call must
			// stay on bars, otherwise the tab silently regresses to a line.
			costChartCall := regexp.MustCompile(`(?s)svgId:'costChart',.*?mode:'([a-z]+)'`).FindStringSubmatch(html)
			if len(costChartCall) != 2 || costChartCall[1] != "bars" {
				t.Fatalf("%s cost chart mode = %v, want bars", name, costChartCall)
			}
		})
	}
}
func TestDashboardExposesEveryFeatureBehindOneAccessGate(t *testing.T) {
	// Full mode is unified away: the ordinary dashboard exposes every feature.
	if strings.Contains(dashboardHTML, `id="fullModeDialog"`) || strings.Contains(dashboardHTML, `X-Full-Mode-Session`) {
		t.Fatal("dashboard still references the removed full-mode flow")
	}
	for _, required := range []string{
		`id="apiKeyMenu"`,
		`id="exportBackup"`,
		`id="restoreBackup"`,
		`id="exportButton"`,
		`id="lockScreen"`,
		`id="unlockKey"`,
		`id="keyDialog"`,
		`type="password"`,
		`Authorization`,
	} {
		if !strings.Contains(dashboardHTML, required) {
			t.Fatalf("unified dashboard missing direct feature %q", required)
		}
	}
}

func TestDashboardCacheHitRateUsesInputTokenDenominator(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`function cacheHitRate(input,cacheRead){`,
		`var inputTokens=Number(input)||0;`,
		`return Math.min(100,cacheTokens/inputTokens*100);`,
		`var cacheRate=cacheHitRate(input,hit);`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboardHTML missing cache hit rate helper %q", required)
		}
	}
}

func TestDashboardPaintsDarkBeforeRunningThemeSync(t *testing.T) {
	html := dashboardHTML
	rootStart := strings.Index(html, `<html lang="zh-CN" data-theme="dark" style="background:#151412;color-scheme:dark">`)
	initialStyle := strings.Index(html, `<style id="initial-theme">`)
	initialScript := strings.Index(html, `<script>`)
	if rootStart < 0 || initialStyle < 0 || initialScript < 0 || rootStart > initialStyle || initialStyle > initialScript {
		t.Fatal("dark root background and initial stylesheet must be available before theme sync script runs")
	}
}

func TestDashboardSynchronizesHostFrameBackground(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"getComputedStyle(parentRoot).getPropertyValue('--bg-secondary')",
		"root.style.backgroundColor=bg",
		"window.frameElement.style.backgroundColor=background",
		"window.frameElement.parentElement.style.backgroundColor=background",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing host background sync %q", required)
		}
	}
}

func TestDashboardResponseHeaders(t *testing.T) {
	response := dashboardResponse()
	if response.Headers.Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
	if response.Headers.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("missing referrer policy")
	}
	csp := response.Headers.Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'none'", "connect-src 'self'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("CSP missing %q: %s", directive, csp)
		}
	}
}

func TestDashboardDoesNotServerRenderUsageValues(t *testing.T) {
	malicious := `</td><script>alert(1)</script>`
	if strings.Contains(dashboardHTML, malicious) {
		t.Fatal("dashboard unexpectedly embeds usage fixture")
	}
	if strings.Contains(dashboardHTML, `innerHTML`) {
		t.Fatal("dashboard unexpectedly uses innerHTML")
	}
}

func TestDashboardPreservesReverseProxyPathPrefix(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`function readDashboardRoute()`,
		`marker='/v0/resource/plugins/'`,
		`var resourceBase=publicPathPrefix+'/v0/resource/plugins/'+pluginID;`,
		`var managementBase=publicPathPrefix+'/v0/management/plugins/'+pluginID;`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing reverse-proxy path-prefix contract %q", required)
		}
	}
}

func TestDashboardTokenSummaryShowsUpstreamComponents(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`text('totalTokens',formatTokens(total))`,
		`{key:'hit',label:t('chart.inputHit'),value:hit,color:seriesColors.hit}`,
		`{key:'miss',label:t('chart.inputMiss'),value:miss,color:seriesColors.miss}`,
		`{key:'out',label:t('chart.output'),value:out,color:seriesColors.out}`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing total-token component contract %q", required)
		}
	}
}

func TestDashboardLocalesCatalog(t *testing.T) {
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		if !strings.Contains(dashboardHTML, `"`+code+`"`) {
			t.Fatalf("dashboardHTML missing locale code %q", code)
		}
	}

	requiredKeys := []string{
		"app.title",
		"button.refresh",
		"button.export",
		"button.downloadBackup",
		"button.restoreBackup",
		"tab.usage",
		"tab.requests",
		"tab.data",
		"filter.timeDimension",
		"filter.model",
		"filter.all",
		"filter.allModels",
		"filter.modelsSelected",
		"filter.keysSelected",
		"card.totalTokens",
		"card.requests",
		"card.successRate",
		"trend.cacheHitRate",
		"chart.inputHit",
		"chart.inputMiss",
		"chart.output",
		"chart.requests",
		"chart.peak",
		"ranking.models",
		"ranking.apiKeys",
		"ranking.empty",
		"range.today",
		"range.yesterday",
		"data.keyTitle",
		"data.backupTitle",
		"data.resetTitle",
		"apiKey.filterAll",
		"apiKey.count",
		"error.unauthorized",
	}
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		data, err := localeFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			t.Fatalf("locale %s: %v", code, err)
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("locale %s invalid JSON: %v", code, err)
		}
		for _, key := range requiredKeys {
			if strings.TrimSpace(m[key]) == "" {
				t.Fatalf("locale %s missing required key %q", code, key)
			}
		}
	}
}

func TestDashboardTestHooksExposed(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	for _, hook := range []string{
		"window.__dashboardTest={",
		"formatTokens:formatTokens",
		"compactNumber:compactNumber",
		"currentBuckets:currentBuckets",
		"queryString:queryString",
		"state:state",
	} {
		if !strings.Contains(dashboardHTML, hook) {
			t.Fatalf("dashboard missing test hook %q", hook)
		}
	}
	_ = node
}

// TestDashboardTokenChartPalette pins the reference app's three-colour palette
// and the stacking contract that goes with it: cache hits fill the top of every
// bar (so the rounded cap is always the hit colour) and the two smaller classes
// stack underneath.
func TestDashboardTokenChartPalette(t *testing.T) {
	dark := regexp.MustCompile(`(?s)\n\[data-theme='dark'\]\{.*?\n\}`).FindString(dashboardHTMLTemplate)
	if dark == "" {
		t.Fatal("dashboard template has no dark theme block")
	}
	for _, declaration := range []string{"--hit:#8edeff;", "--miss:#3ab5ff;", "--out:#0072fb;"} {
		if !strings.Contains(dark, declaration) {
			t.Fatalf("dark theme is missing the reference chart colour %q", declaration)
		}
	}
	if !strings.Contains(dashboardHTMLTemplate, "var stack=series.slice().reverse();") {
		t.Fatal("token bars no longer draw from the reversed series: cache hits must sit on top")
	}
}

// TestDashboardTranslationKeysAreComplete couples the template and the locale
// catalogs: a key that the page renders but no catalog defines would surface as
// a raw key string in the UI, and an unused key is dead weight in the binary.
func TestDashboardTranslationKeysAreComplete(t *testing.T) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`data-i18n="([^"]+)"`),
		regexp.MustCompile(`data-i18n-title="([^"]+)"`),
		regexp.MustCompile(`data-i18n-aria="([^"]+)"`),
		regexp.MustCompile(`data-i18n-placeholder="([^"]+)"`),
		regexp.MustCompile(`(?:^|[^A-Za-z0-9_$.])t\('([A-Za-z0-9_]+\.[A-Za-z0-9_.]+)'`),
	}
	used := map[string]bool{}
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllStringSubmatch(dashboardHTMLTemplate, -1) {
			used[match[1]] = true
		}
	}
	keys := make([]string, 0, len(used))
	for key := range used {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Fatal("no translation keys detected in the dashboard template")
	}
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		data, err := localeFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			t.Fatalf("locale %s: %v", code, err)
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			t.Fatalf("locale %s invalid JSON: %v", code, err)
		}
		var missing, unused []string
		for _, key := range keys {
			if strings.TrimSpace(catalog[key]) == "" {
				missing = append(missing, key)
			}
		}
		for key := range catalog {
			if !used[key] {
				unused = append(unused, key)
			}
		}
		sort.Strings(unused)
		if len(missing) > 0 {
			t.Fatalf("locale %s is missing %d keys: %v", code, len(missing), missing)
		}
		if len(unused) > 0 {
			t.Fatalf("locale %s has %d unused keys: %v", code, len(unused), unused)
		}
	}
}
