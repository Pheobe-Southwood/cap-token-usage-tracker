import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath, scenario = 'presets'] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_date_range.mjs <dashboard-html-path> <google-chrome-path> <presets|custom>');
}

const dashboardHTML = await readFile(htmlPath);
const resourceBase = '/v0/resource/plugins/cap-token-usage-tracker';
const managementBase = '/v0/management/plugins/cap-token-usage-tracker';
const SESSION = 'test-dashboard-session-1111111111111111';
const now = Date.UTC(2026, 9, 17, 12, 0, 0);

const series = [];
for (let day = 29; day >= 0; day -= 1) {
  const at = new Date(now - day * 24 * 3600 * 1000);
  series.push({
    hour: at.toISOString(),
    requests: 100 + day,
    failed_requests: 0,
    input_tokens: 1000 + day,
    output_tokens: 100,
    reasoning_tokens: 0,
    cached_tokens: 0,
    cache_read_tokens: 0,
    cache_creation_tokens: 0,
    total_tokens: 1100 + day,
    total_latency_ns: 0,
    total_ttft_ns: 0,
    latency_samples: 0,
    ttft_samples: 0,
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
    sendJSON({ session: SESSION, expires_at: new Date(now + 8 * 3600 * 1000).toISOString() });
    return;
  }
  if (request.headers['x-plugin-session'] !== SESSION) {
    response.writeHead(401, { 'content-type': 'application/json' });
    response.end(JSON.stringify({ error: 'dashboard session is missing or expired', code: 'session_required' }));
    return;
  }
  if (url.pathname === `${resourceBase}/preferences`) {
    sendJSON({ time_range_mode: 'last_30_days' });
    return;
  }
  if (url.pathname === `${resourceBase}/stats/initial`) {
    sendJSON({
      schema_version: 2,
      generated_at: new Date(now).toISOString(),
      range: url.searchParams.get('range') || '30d',
      retained_since: new Date(now - 30 * 24 * 3600 * 1000).toISOString(),
      last_used: new Date(now).toISOString(),
      bucket_seconds: 3600,
      summary: { requests: 3000, input_tokens: 30000, output_tokens: 3000, cache_read_tokens: 0, total_tokens: 33000 },
      models: [{ model: 'deepseek-flash', requests: 3000, total_tokens: 33000 }],
      series,
      sources: ['cli'],
      api_keys: [],
    });
    return;
  }
  if (url.pathname === `${resourceBase}/requests`) {
    sendJSON({ generated_at: new Date(now).toISOString(), range: '30d', total: 0, offset: 0, limit: 30, items: [] });
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

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

try {
  const context = await browser.newContext({ locale: 'zh-CN', timezoneId: 'UTC', viewport: { width: 1280, height: 900 } });
  await context.addInitScript(({ token }) => {
    window.sessionStorage.setItem('cap-token-usage-tracker.session', JSON.stringify({
      token, expiresAt: new Date(Date.now() + 3600 * 1000).toISOString(), pluginID: 'cap-token-usage-tracker',
    }));
  }, { token: SESSION });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(String(error)));
  await page.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
  await page.locator('#totalTokens').waitFor({ state: 'visible' });
  await page.locator('#rangeButton').click();
  await page.locator('#rangeDialog').waitFor({ state: 'visible' });

  const captures = [];
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.pathname === `${resourceBase}/stats/initial` && url.searchParams.get('range') === 'custom') {
      captures.push({ start: url.searchParams.get('start'), end: url.searchParams.get('end') });
    }
  });

  if (scenario === 'presets') {
    // Yesterday must resolve to a half-open local-day window.
    await page.locator('#presetList .preset[data-mode="yesterday"]').click();
    await page.waitForTimeout(600);
    assert(captures.length > 0, 'selecting yesterday issued no custom-range request');
    const first = captures[captures.length - 1];
    const todayMidnight = new Date();
    todayMidnight.setUTCHours(0, 0, 0, 0);
    const todayStart = todayMidnight.getTime();
    const yesterdayStart = todayStart - 24 * 3600 * 1000;
    assert(new Date(first.start).getTime() === yesterdayStart, `unexpected start for yesterday: ${first.start}`);
    assert(new Date(first.end).getTime() === todayStart, `unexpected end for yesterday: ${first.end}`);
    const chip = await page.locator('#rangeValue').innerText();
    assert(chip.trim().length > 0 && chip !== '最近 30 天', `range chip did not update: ${chip}`);

    // Today starts at local midnight and ends now.
    await page.locator('#rangeButton').click();
    await page.locator('#presetList .preset[data-mode="today"]').click();
    await page.waitForTimeout(600);
    const today = captures[captures.length - 1];
    assert(new Date(today.start).getTime() === todayStart, `unexpected start for today: ${today.start}`);
    assert(new Date(today.end).getTime() > todayStart, `today end must be after local midnight: ${today.end}`);
  } else {
    // Invalid ranges are rejected without a request.
    await page.locator('#customStart').fill('2026-10-12T08:30');
    await page.locator('#customEnd').fill('2026-10-11T09:15');
    await page.locator('#rangeApply').click();
    await page.waitForTimeout(300);
    const error = await page.locator('#rangeError').innerText();
    assert(error.trim().length > 0, 'invalid custom range did not surface an error');
    assert(captures.length === 0, 'invalid custom range still issued a request');

    // A valid custom range keeps the exact instants the operator picked.
    await page.locator('#customStart').fill('2026-10-10T08:30');
    await page.locator('#customEnd').fill('2026-10-12T09:15');
    await page.locator('#rangeApply').click();
    await page.waitForTimeout(600);
    assert(captures.length === 1, `valid custom range issued ${captures.length} requests`);
    const applied = captures[0];
    assert(new Date(applied.start).getTime() === Date.UTC(2026, 9, 10, 8, 30, 0), `unexpected custom start: ${applied.start}`);
    assert(new Date(applied.end).getTime() === Date.UTC(2026, 9, 12, 9, 15, 0), `unexpected custom end: ${applied.end}`);
    const chip = await page.locator('#rangeValue').innerText();
    assert(chip.includes('10/10') && chip.includes('10/12'), `custom range chip did not update: ${chip}`);
  }

  assert(errors.length === 0, `page errors: ${errors.join('\n')}`);
  await context.close();
} finally {
  await browser.close();
  server.close();
}
