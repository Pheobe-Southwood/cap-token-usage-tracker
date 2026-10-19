import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_api_key_layout.mjs <dashboard-html-path> <google-chrome-path>');
}

const dashboardHTML = await readFile(htmlPath);
const resourceBase = '/v0/resource/plugins/cap-token-usage-tracker';
const managementBase = '/v0/management/plugins/cap-token-usage-tracker';
// Every data route is gated behind a session minted by the management API.
// The mock host enforces the same contract as the real plugin.
const SESSION = 'test-dashboard-session-0000000000000000';
const MANAGEMENT_KEY = 'test-management-key';
let unauthorizedHits = 0;

const HOUR = 3600 * 1000;
const now = Date.UTC(2026, 9, 17, 12, 0, 0);
const DAYS = 30;

function buildSeries() {
  const series = [];
  for (let day = DAYS - 1; day >= 0; day -= 1) {
    const at = new Date(now - day * 24 * HOUR);
    const requests = 900 + ((day * 137) % 700);
    const input = requests * (18000 + ((day * 977) % 9000));
    const cacheRead = Math.round(input * 0.86);
    const output = requests * 320;
    series.push({
      hour: at.toISOString(),
      requests,
      failed_requests: day % 7 === 0 ? 12 : 0,
      input_tokens: input,
      output_tokens: output,
      reasoning_tokens: 0,
      cached_tokens: cacheRead,
      cache_read_tokens: cacheRead,
      cache_creation_tokens: 0,
      total_tokens: input + output,
      total_latency_ns: 0,
      total_ttft_ns: 0,
      latency_samples: 0,
      ttft_samples: 0,
    });
  }
  return series;
}

function summarize(series) {
  const summary = {
    requests: 0,
    failed_requests: 0,
    input_tokens: 0,
    output_tokens: 0,
    reasoning_tokens: 0,
    cached_tokens: 0,
    cache_read_tokens: 0,
    cache_creation_tokens: 0,
    total_tokens: 0,
    total_latency_ns: 0,
    total_ttft_ns: 0,
    latency_samples: 0,
    ttft_samples: 0,
  };
  for (const point of series) {
    for (const key of Object.keys(summary)) summary[key] += point[key] || 0;
  }
  return summary;
}

const series = buildSeries();
const summary = summarize(series);
const models = [
  { model: 'deepseek-v4-pro', requests: 12000, total_tokens: Math.round(summary.total_tokens * 0.52), input_tokens: 0, output_tokens: 0, cache_read_tokens: 0 },
  { model: 'deepseek-flash', requests: 9000, total_tokens: Math.round(summary.total_tokens * 0.31), input_tokens: 0, output_tokens: 0, cache_read_tokens: 0 },
  { model: 'gemini-3.8-flash-high', requests: 4000, total_tokens: Math.round(summary.total_tokens * 0.14), input_tokens: 0, output_tokens: 0, cache_read_tokens: 0 },
  { model: '', requests: 200, total_tokens: Math.round(summary.total_tokens * 0.03), input_tokens: 0, output_tokens: 0, cache_read_tokens: 0 },
];
const apiKeys = [
  { ref: 'g1:11111111111111111111111111111111', label: 'cup', requests: 9000, total_tokens: Math.round(summary.total_tokens * 0.5) },
  { ref: 'g1:22222222222222222222222222222222', label: 'dsh', requests: 8000, total_tokens: Math.round(summary.total_tokens * 0.3) },
  { ref: 'g1:33333333333333333333333333333333', requests: 4000, total_tokens: Math.round(summary.total_tokens * 0.2) },
];

const requests = [];
for (let index = 0; index < 60; index += 1) {
  requests.push({
    sequence: index + 1,
    time: new Date(now - index * 5 * 60 * 1000).toISOString(),
    model: models[index % 3].model || 'deepseek-flash',
    api_key_ref: apiKeys[index % 3].ref,
    source: 'cli',
    result: index % 11 === 0 ? 'failed' : 'success',
    input_tokens: 12000 + index * 37,
    output_tokens: 320 + index,
    cache_read_tokens: 9000 + index * 31,
    total_tokens: 12320 + index * 38,
  });
}

const server = createServer((request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1');
  const sendJSON = (value) => {
    response.writeHead(200, { 'content-type': 'application/json', 'cache-control': 'no-store' });
    response.end(JSON.stringify(value));
  };
  if (url.pathname === `${resourceBase}/dashboard`) {
    response.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
    response.end(dashboardHTML);
    return;
  }
  if (url.pathname === `${managementBase}/session`) {
    if (request.method !== 'POST' || request.headers.authorization !== `Bearer ${MANAGEMENT_KEY}`) {
      response.writeHead(401, { 'content-type': 'application/json' });
      response.end(JSON.stringify({ error: 'invalid management key' }));
      return;
    }
    sendJSON({ session: SESSION, expires_at: new Date(now + 8 * HOUR).toISOString() });
    return;
  }
  if (url.pathname === `${managementBase}/session/revoke`) {
    sendJSON({ revoked: true });
    return;
  }
  if (request.headers['x-plugin-session'] !== SESSION) {
    unauthorizedHits += 1;
    response.writeHead(401, { 'content-type': 'application/json', 'cache-control': 'no-store' });
    response.end(JSON.stringify({ error: 'dashboard session is missing or expired', code: 'session_required' }));
    return;
  }
  if (url.pathname === `${resourceBase}/preferences`) {
    sendJSON({});
    return;
  }
  if (url.pathname === `${resourceBase}/stats/initial`) {
    sendJSON({
      schema_version: 2,
      generated_at: new Date(now).toISOString(),
      range: url.searchParams.get('range') || '30d',
      retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
      last_used: new Date(now - 5 * 60 * 1000).toISOString(),
      bucket_seconds: 86400,
      summary,
      models,
      series,
      sources: ['cli'],
      api_keys: apiKeys,
    });
    return;
  }
  if (url.pathname === `${resourceBase}/stats/groups`) {
    sendJSON({ items: [], total: 0 });
    return;
  }
  if (url.pathname === `${resourceBase}/requests`) {
    sendJSON({
      generated_at: new Date(now).toISOString(),
      range: '30d',
      total: requests.length,
      offset: 0,
      limit: 30,
      items: requests.slice(0, 30),
    });
    return;
  }
  sendJSON({});
});

await new Promise((resolve, reject) => {
  server.once('error', reject);
  server.listen(0, '127.0.0.1', resolve);
});

const address = server.address();
const dashboardURL = `http://127.0.0.1:${address.port}${resourceBase}/dashboard`;
const browser = await chromium.launch({ executablePath: chromePath, headless: true });

function fail(message) {
  throw new Error(message);
}

try {
  /* ---------------------------- desktop ---------------------------- */
  const desktop = await browser.newContext({ locale: 'zh-CN', timezoneId: 'UTC', viewport: { width: 1300, height: 900 } });
  const page = await desktop.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(String(error)));
  await page.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
  await page.locator('#lockScreen').waitFor({ state: 'visible' });
  if (!(await page.evaluate(() => window.__dashboardTest.isLocked()))) fail('dashboard started unlocked without a session');
  const refused = await page.evaluate(async (base) => (await fetch(`${base}/stats/initial?range=30d`, { credentials: 'same-origin' })).status, resourceBase);
  if (refused !== 401) fail(`data route without a session returned ${refused}, want 401`);
  await page.locator('#unlockKey').fill(MANAGEMENT_KEY);
  await page.locator('#unlockButton').click();
  await page.locator('#lockScreen').waitFor({ state: 'hidden' });
  if (await page.evaluate(() => window.__dashboardTest.isLocked())) fail('dashboard stayed locked after unlocking');
  await page.locator('#totalTokens').waitFor({ state: 'visible' });
  await page.waitForFunction(() => document.querySelectorAll('#tokenChart path, #tokenChart rect').length > 5);

  const hero = await page.locator('#totalTokens').innerText();
  if (!/[\d,]+/.test(hero) || hero === '0') fail(`hero token total did not render: ${hero}`);

  const barShape = await page.evaluate(() => {
    const marks = Array.from(document.querySelectorAll('#tokenChart path, #tokenChart rect'));
    const rounded = marks.filter((mark) => mark.tagName.toLowerCase() === 'path' && /Q/.test(mark.getAttribute('d') || '')).length;
    const colors = Array.from(new Set(marks.map((mark) => mark.getAttribute('fill'))));
    const buckets = window.__dashboardTest.currentBuckets().filter((bucket) => bucket.total > 0).length;
    const styles = getComputedStyle(document.documentElement);
    const vars = {
      hit: styles.getPropertyValue('--hit').trim().toLowerCase(),
      miss: styles.getPropertyValue('--miss').trim().toLowerCase(),
      out: styles.getPropertyValue('--out').trim().toLowerCase(),
    };
    // Group the segments of the tallest bar and report them top-down: cache
    // hits must be the top slice (and therefore own the rounded cap).
    const segments = marks.map((mark) => {
      const box = mark.getBBox();
      return { x: Math.round(box.x), fill: (mark.getAttribute('fill') || '').toLowerCase(), top: box.y, height: box.height, rounded: mark.tagName.toLowerCase() === 'path' };
    }).filter((segment) => segment.fill && segment.fill !== 'transparent' && segment.fill !== 'none');
    const byColumn = new Map();
    segments.forEach((segment) => {
      const column = byColumn.get(segment.x) || [];
      column.push(segment);
      byColumn.set(segment.x, column);
    });
    let tallest = [];
    byColumn.forEach((column) => {
      const total = column.reduce((sum, segment) => sum + segment.height, 0);
      const best = tallest.reduce((sum, segment) => sum + segment.height, 0);
      if (total > best) tallest = column;
    });
    const stack = tallest.slice().sort((a, b) => a.top - b.top).map((segment) => segment.fill);
    const capFill = (tallest.find((segment) => segment.rounded) || {}).fill || '';
    return { marks: marks.length, rounded, colors, buckets, vars, stack, capFill };
  });
  if (barShape.marks < 10) fail(`token chart rendered only ${barShape.marks} marks`);
  if (barShape.rounded < barShape.buckets) fail(`not every bar has a rounded cap: ${JSON.stringify(barShape)}`);
  if (barShape.colors.length < 3) fail(`stacked bars lost their token classes: ${JSON.stringify(barShape.colors)}`);
  if (barShape.stack.length < 3) fail(`tallest bar is not stacked: ${JSON.stringify(barShape.stack)}`);
  if (barShape.stack[0] !== barShape.vars.hit) fail(`cache hits are not the top slice: ${JSON.stringify(barShape)}`);
  if (barShape.stack[barShape.stack.length - 1] !== barShape.vars.out) fail(`output is not the bottom slice: ${JSON.stringify(barShape)}`);
  if (barShape.capFill !== barShape.vars.hit) fail(`rounded cap is not the hit colour: ${JSON.stringify(barShape)}`);
  const smoothCurve = await page.evaluate(() => {
    const path = document.querySelector('#requestChart path[stroke]');
    const d = path ? path.getAttribute('d') || '' : '';
    return (d.match(/C/g) || []).length >= 8;
  });
  if (!smoothCurve) fail('request chart is not drawn with a smooth cubic spline');

  const overflow = await page.evaluate(() => ({ width: window.innerWidth, scroll: document.documentElement.scrollWidth }));
  if (overflow.scroll > overflow.width + 1) fail(`desktop page scrolls horizontally: ${JSON.stringify(overflow)}`);

  // Chart tooltip on hover. The host is scrolled into view first because the
  // usage panel grows as cards are added above the charts.
  const host = await page.locator('#tokenChartHost').boundingBox();
  await page.locator('#tokenChartHost').hover({ position: { x: host.width * 0.6, y: host.height * 0.6 } });
  await page.locator('#tokenTip.is-visible').waitFor({ state: 'visible', timeout: 4000 });
  const tipText = await page.locator('#tokenTip').innerText();
  if (!tipText.trim()) fail('token tooltip rendered without content');

  // Model filter: selecting a model must scope the next stats request.
  const modelMenu = page.locator('#modelMenu');
  await page.locator('#modelButton').click();
  await modelMenu.waitFor({ state: 'visible' });
  const modelRequest = page.waitForRequest((request) => {
    const candidate = new URL(request.url());
    return candidate.pathname === `${resourceBase}/stats/initial` && candidate.searchParams.getAll('model').length === 1;
  });
  await modelMenu.locator('.menu-option:not(.is-all) input').first().check();
  await modelRequest;
  const modelChip = await page.locator('#modelValue').innerText();
  if (!modelChip.includes('1')) fail(`model chip did not report the selection: ${modelChip}`);
  await page.keyboard.press('Escape');

  // API key filter: same contract with api_key_ref.
  const apiKeyMenu = page.locator('#apiKeyMenu');
  await page.locator('#apiKeyButton').click();
  await apiKeyMenu.waitFor({ state: 'visible' });
  const keyRequest = page.waitForRequest((request) => {
    const candidate = new URL(request.url());
    return candidate.pathname === `${resourceBase}/stats/initial` && candidate.searchParams.getAll('api_key_ref').length === 1;
  });
  await apiKeyMenu.locator('.menu-option:not(.is-all) input').first().check();
  await keyRequest;
  const apiKeyChip = await page.locator('#apiKeyValue').innerText();
  if (!apiKeyChip.includes('1')) fail(`API key chip did not report the selection: ${apiKeyChip}`);
  await page.keyboard.press('Escape');

  if (errors.length) fail(`desktop page errors: ${errors.join('\n')}`);
  await desktop.close();

  /* ----------------------------- mobile ---------------------------- */
  for (const width of [390, 430, 574, 700, 820, 1000]) {
    const context = await browser.newContext({ locale: 'zh-CN', timezoneId: 'UTC', viewport: { width, height: 900 } });
    await context.addInitScript(({ token }) => {
      window.sessionStorage.setItem('cap-token-usage-tracker.session', JSON.stringify({
        token,
        expiresAt: new Date(Date.now() + 3600 * 1000).toISOString(),
        pluginID: 'cap-token-usage-tracker',
      }));
    }, { token: SESSION });
    const mobile = await context.newPage();
    const mobileErrors = [];
    mobile.on('pageerror', (error) => mobileErrors.push(String(error)));
    await mobile.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
    await mobile.locator('#totalTokens').waitFor({ state: 'visible' });
    await mobile.waitForTimeout(250);
    const layout = await mobile.evaluate(() => {
      const rect = (selector) => {
        const element = document.querySelector(selector);
        if (!element) return null;
        const box = element.getBoundingClientRect();
        return { left: box.left, right: box.right, width: box.width };
      };
      return {
        innerWidth: window.innerWidth,
        scrollWidth: document.documentElement.scrollWidth,
        exportButton: rect('#exportButton'),
        apiKeyChip: rect('#apiKeyButton'),
        hero: rect('#totalTokens'),
      };
    });
    if (layout.scrollWidth > layout.innerWidth + 1) {
      fail(`dashboard scrolls horizontally at ${width}px: ${JSON.stringify(layout)}`);
    }
    if (!layout.exportButton || !layout.apiKeyChip) fail(`missing chips at ${width}px: ${JSON.stringify(layout)}`);
    if (layout.apiKeyChip.right > layout.innerWidth + 1) {
      fail(`API key chip escapes the viewport at ${width}px: ${JSON.stringify(layout)}`);
    }
    if (layout.exportButton.right > layout.innerWidth + 1) {
      fail(`export button escapes the viewport at ${width}px: ${JSON.stringify(layout)}`);
    }
    if (!(layout.hero.width > 0)) fail(`hero number collapsed at ${width}px`);
    if (mobileErrors.length) fail(`page errors at ${width}px: ${mobileErrors.join('\n')}`);
    await context.close();
  }

  /* ------------------------------ touch ---------------------------- */
  // A single tap must pin the tooltip on mobile; the pointerleave that follows
  // every touch used to dismiss it, which is why the charts needed a long press.
  const touchContext = await browser.newContext({
    locale: 'zh-CN', timezoneId: 'UTC', viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
  });
  await touchContext.addInitScript(({ token }) => {
    window.sessionStorage.setItem('cap-token-usage-tracker.session', JSON.stringify({
      token, expiresAt: new Date(Date.now() + 3600 * 1000).toISOString(), pluginID: 'cap-token-usage-tracker',
    }));
  }, { token: SESSION });
  const touchPage = await touchContext.newPage();
  const touchErrors = [];
  touchPage.on('pageerror', (error) => touchErrors.push(String(error)));
  await touchPage.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
  await touchPage.locator('#totalTokens').waitFor({ state: 'visible' });
  const touchHost = await touchPage.locator('#tokenChartHost').boundingBox();
  await touchPage.locator('#tokenChartHost').tap({ position: { x: touchHost.width * 0.62, y: touchHost.height * 0.55 } });
  await touchPage.locator('#tokenTip.is-visible').waitFor({ state: 'visible', timeout: 4000 });
  await touchPage.waitForTimeout(1500);
  if (!(await touchPage.locator('#tokenTip').isVisible())) fail('touch tooltip disappeared after a single tap');
  if (!(await touchPage.locator('#tokenTip').innerText()).trim()) fail('touch tooltip rendered without content');
  await touchPage.locator('#tokenTrendSub').tap();
  await touchPage.waitForTimeout(300);
  if (await touchPage.locator('#tokenTip').isVisible()) fail('touch tooltip did not dismiss when tapping outside the chart');
  if (touchErrors.length) fail(`touch page errors: ${touchErrors.join('\n')}`);
  await touchContext.close();

  // The second chart shares the same interaction code; tapping it must pin too.
  if (unauthorizedHits < 1) fail('mock host never refused an unauthenticated data request');
} finally {
  await browser.close();
  server.close();
}
