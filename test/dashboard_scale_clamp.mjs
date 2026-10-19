import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_scale_clamp.mjs <dashboard-html-path> <google-chrome-path>');
}

const dashboardHTML = await readFile(htmlPath);
const resourceBase = '/v0/resource/plugins/cap-token-usage-tracker';
const managementBase = '/v0/management/plugins/cap-token-usage-tracker';
const SESSION = 'test-dashboard-session-scale-clamp-0001';
const MANAGEMENT_KEY = 'test-management-key';
const now = Date.UTC(2026, 9, 17, 12, 0, 0);
const HOUR = 3600 * 1000;

let currentScenario = 'auto'; // 'auto' | 'clamp-minute' | 'explicit-hour' | 'large-minute-1440'

function buildPayload() {
  if (currentScenario === 'clamp-minute') {
    // 30d range requesting minute scale: server clamps to hour and reports requested_scale: 'minute'.
    const count = 48;
    const series = [];
    const costSeries = [];
    const first = now - (count - 1) * HOUR;
    for (let i = 0; i < count; i += 1) {
      const at = new Date(first + i * HOUR).toISOString();
      series.push({
        hour: at,
        requests: 10,
        failed_requests: 0,
        input_tokens: 1000,
        output_tokens: 500,
        reasoning_tokens: 0,
        cache_read_tokens: 200,
        cache_creation_tokens: 0,
        total_tokens: 1500,
      });
      costSeries.push({
        hour: at,
        cost_usd: 0.05,
        requests: 10,
        unpriced_requests: 0,
      });
    }
    return {
      schema_version: 3,
      generated_at: new Date(now).toISOString(),
      range: '30d',
      retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
      last_used: new Date(now - 60000).toISOString(),
      scale: 'hour',
      bucket_seconds: 3600,
      requested_scale: 'minute',
      tz_offset_minutes: 0,
      summary: { requests: 480, failed_requests: 0, input_tokens: 48000, output_tokens: 24000, cache_read_tokens: 9600, total_tokens: 72000 },
      cost: { cost_usd: 2.4, priced_models: 1, free_models: 0, unpriced_models: 0, priced_requests: 480, unpriced_requests: 0, time_tier_cost_usd: 0, time_tier_requests: 0 },
      models: [{ model: 'gemini-3.8-flash-high', requests: 480, total_tokens: 72000, cost_usd: 2.4, priced: true, in_range: true }],
      series,
      cost_series: costSeries,
      cost_by_model: [{ key: 'gemini-3.8-flash-high', total_cost_usd: 2.4, values: new Array(count).fill(2.4 / count) }],
      sources: ['cli'],
      api_keys: [{ ref: 'g1:aaaaaa', label: '主 Key', requests: 480, total_tokens: 72000, cost_usd: 2.4, priced_requests: 480, unpriced_requests: 0 }],
      pricing: { source: 'embedded', generated_at: new Date(now - HOUR).toISOString(), stale: false, status: 'ok', providers: 1, models: 1, priced_models: 1, free_models: 0, auto_refresh: true, remote_enabled: true, last_error: '', stale_after_seconds: 86400, overrides: 0 },
    };
  }

  if (currentScenario === 'explicit-hour') {
    // 30d range requesting hour scale: fits cap 1440, no requested_scale.
    const count = 48;
    const series = [];
    const costSeries = [];
    const first = now - (count - 1) * HOUR;
    for (let i = 0; i < count; i += 1) {
      const at = new Date(first + i * HOUR).toISOString();
      series.push({
        hour: at,
        requests: 10,
        failed_requests: 0,
        input_tokens: 1000,
        output_tokens: 500,
        reasoning_tokens: 0,
        cache_read_tokens: 200,
        cache_creation_tokens: 0,
        total_tokens: 1500,
      });
      costSeries.push({
        hour: at,
        cost_usd: 0.05,
        requests: 10,
        unpriced_requests: 0,
      });
    }
    return {
      schema_version: 3,
      generated_at: new Date(now).toISOString(),
      range: '30d',
      retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
      last_used: new Date(now - 60000).toISOString(),
      scale: 'hour',
      bucket_seconds: 3600,
      tz_offset_minutes: 0,
      summary: { requests: 480, failed_requests: 0, input_tokens: 48000, output_tokens: 24000, cache_read_tokens: 9600, total_tokens: 72000 },
      cost: { cost_usd: 2.4, priced_models: 1, free_models: 0, unpriced_models: 0, priced_requests: 480, unpriced_requests: 0, time_tier_cost_usd: 0, time_tier_requests: 0 },
      models: [{ model: 'gemini-3.8-flash-high', requests: 480, total_tokens: 72000, cost_usd: 2.4, priced: true, in_range: true }],
      series,
      cost_series: costSeries,
      cost_by_model: [{ key: 'gemini-3.8-flash-high', total_cost_usd: 2.4, values: new Array(count).fill(2.4 / count) }],
      sources: ['cli'],
      api_keys: [{ ref: 'g1:aaaaaa', label: '主 Key', requests: 480, total_tokens: 72000, cost_usd: 2.4, priced_requests: 480, unpriced_requests: 0 }],
      pricing: { source: 'embedded', generated_at: new Date(now - HOUR).toISOString(), stale: false, status: 'ok', providers: 1, models: 1, priced_models: 1, free_models: 0, auto_refresh: true, remote_enabled: true, last_error: '', stale_after_seconds: 86400, overrides: 0 },
    };
  }

  if (currentScenario === 'large-minute-1440') {
    // 24h range requesting minute scale: 1440 buckets, fits cap 1440, no requested_scale.
    const count = 1440;
    const series = [];
    const costSeries = [];
    const STEP = 60 * 1000;
    const first = now - (count - 1) * STEP;
    for (let i = 0; i < count; i += 1) {
      const at = new Date(first + i * STEP).toISOString();
      const active = i % 5 === 0;
      series.push({
        hour: at,
        requests: active ? 2 : 0,
        failed_requests: 0,
        input_tokens: active ? 200 : 0,
        output_tokens: active ? 100 : 0,
        reasoning_tokens: 0,
        cache_read_tokens: active ? 50 : 0,
        cache_creation_tokens: 0,
        total_tokens: active ? 300 : 0,
      });
      costSeries.push({
        hour: at,
        cost_usd: active ? 0.005 : 0,
        requests: active ? 2 : 0,
        unpriced_requests: 0,
      });
    }
    return {
      schema_version: 3,
      generated_at: new Date(now).toISOString(),
      range: '24h',
      retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
      last_used: new Date(now - 60000).toISOString(),
      scale: 'minute',
      bucket_seconds: 60,
      tz_offset_minutes: 0,
      summary: { requests: 576, failed_requests: 0, input_tokens: 57600, output_tokens: 28800, cache_read_tokens: 14400, total_tokens: 86400 },
      cost: { cost_usd: 1.44, priced_models: 1, free_models: 0, unpriced_models: 0, priced_requests: 576, unpriced_requests: 0, time_tier_cost_usd: 0, time_tier_requests: 0 },
      models: [{ model: 'gemini-3.8-flash-high', requests: 576, total_tokens: 86400, cost_usd: 1.44, priced: true, in_range: true }],
      series,
      cost_series: costSeries,
      cost_by_model: [{ key: 'gemini-3.8-flash-high', total_cost_usd: 1.44, values: costSeries.map((c) => c.cost_usd) }],
      sources: ['cli'],
      api_keys: [{ ref: 'g1:aaaaaa', label: '主 Key', requests: 576, total_tokens: 86400, cost_usd: 1.44, priced_requests: 576, unpriced_requests: 0 }],
      pricing: { source: 'embedded', generated_at: new Date(now - HOUR).toISOString(), stale: false, status: 'ok', providers: 1, models: 1, priced_models: 1, free_models: 0, auto_refresh: true, remote_enabled: true, last_error: '', stale_after_seconds: 86400, overrides: 0 },
    };
  }

  // default 'auto' (30d, day buckets)
  const count = 30;
  const series = [];
  const costSeries = [];
  const DAY = 24 * HOUR;
  const first = now - (count - 1) * DAY;
  for (let i = 0; i < count; i += 1) {
    const at = new Date(first + i * DAY).toISOString();
    series.push({
      hour: at,
      requests: 50,
      failed_requests: 0,
      input_tokens: 5000,
      output_tokens: 2500,
      reasoning_tokens: 0,
      cache_read_tokens: 1000,
      cache_creation_tokens: 0,
      total_tokens: 7500,
    });
    costSeries.push({
      hour: at,
      cost_usd: 0.25,
      requests: 50,
      unpriced_requests: 0,
    });
  }
  return {
    schema_version: 3,
    generated_at: new Date(now).toISOString(),
    range: '30d',
    retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
    last_used: new Date(now - 60000).toISOString(),
    scale: 'day',
    bucket_seconds: 86400,
    tz_offset_minutes: 0,
    summary: { requests: 1500, failed_requests: 0, input_tokens: 150000, output_tokens: 75000, cache_read_tokens: 30000, total_tokens: 225000 },
    cost: { cost_usd: 7.5, priced_models: 1, free_models: 0, unpriced_models: 0, priced_requests: 1500, unpriced_requests: 0, time_tier_cost_usd: 0, time_tier_requests: 0 },
    models: [{ model: 'gemini-3.8-flash-high', requests: 1500, total_tokens: 225000, cost_usd: 7.5, priced: true, in_range: true }],
    series,
    cost_series: costSeries,
    cost_by_model: [{ key: 'gemini-3.8-flash-high', total_cost_usd: 7.5, values: new Array(count).fill(7.5 / count) }],
    sources: ['cli'],
    api_keys: [{ ref: 'g1:aaaaaa', label: '主 Key', requests: 1500, total_tokens: 225000, cost_usd: 7.5, priced_requests: 1500, unpriced_requests: 0 }],
    pricing: { source: 'embedded', generated_at: new Date(now - HOUR).toISOString(), stale: false, status: 'ok', providers: 1, models: 1, priced_models: 1, free_models: 0, auto_refresh: true, remote_enabled: true, last_error: '', stale_after_seconds: 86400, overrides: 0 },
  };
}

const server = createServer((request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1');
  const sendJSON = (value, status = 200) => {
    response.writeHead(status, { 'content-type': 'application/json', 'cache-control': 'no-store' });
    response.end(JSON.stringify(value));
  };
  if (url.pathname === `${resourceBase}/dashboard`) {
    response.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
    response.end(dashboardHTML);
    return;
  }
  if (url.pathname === `${managementBase}/session`) {
    if (request.method !== 'POST' || request.headers.authorization !== `Bearer ${MANAGEMENT_KEY}`) {
      sendJSON({ error: 'invalid management key' }, 401);
      return;
    }
    sendJSON({ session: SESSION, expires_at: new Date(now + 8 * HOUR).toISOString() });
    return;
  }
  if (request.headers['x-plugin-session'] !== SESSION) {
    sendJSON({ error: 'session required' }, 401);
    return;
  }
  if (url.pathname === `${resourceBase}/preferences`) {
    sendJSON({});
    return;
  }
  if (url.pathname === `${resourceBase}/stats/initial`) {
    sendJSON(buildPayload());
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

async function unlock(page) {
  await page.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
  await page.locator('#lockScreen').waitFor({ state: 'visible' });
  await page.locator('#unlockKey').fill(MANAGEMENT_KEY);
  await page.locator('#unlockButton').click();
  await page.locator('#lockScreen').waitFor({ state: 'hidden' });
  await page.locator('#totalTokens').waitFor({ state: 'visible' });
}

try {
  const context = await browser.newContext({ locale: 'zh-CN', timezoneId: 'UTC', viewport: { width: 1360, height: 1000 } });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (err) => errors.push(String(err)));
  await unlock(page);

  // 1. Initial load (auto scale): scaleHint must be hidden, requestScaleNote shows "尺度：天"
  assert(await page.locator('#scaleHint').isHidden(), 'scaleHint should be hidden on initial auto load');
  const initialNote = await page.locator('#requestScaleNote').innerText();
  assert(initialNote.includes('尺度：天'), `initial note expected 尺度：天, got: ${initialNote}`);

  // 2. Select "minute" on 30d range: clamped scenario
  currentScenario = 'clamp-minute';
  await page.locator('#scaleControl button[data-scale="minute"]').click();
  await page.locator('#scaleHint').waitFor({ state: 'visible' });

  const hintText = await page.locator('#scaleHint').innerText();
  assert(hintText.includes('分钟超出该范围上限，已按小时显示'), `scaleHint text expected clamp message, got: ${hintText}`);
  const hintClasses = await page.locator('#scaleHint').getAttribute('class');
  assert(hintClasses.includes('scale-clamped'), `scaleHint class expected scale-clamped, got: ${hintClasses}`);

  const reqNoteClamped = await page.locator('#requestScaleNote').innerText();
  assert(reqNoteClamped.includes('分钟超出该范围上限，已按小时显示'), `requestScaleNote expected clamp message, got: ${reqNoteClamped}`);

  // Switch to Cost tab: costScaleNote must also show the clamp message
  await page.locator('.tab[data-tab="cost"]').click();
  await page.locator('#panelCost').waitFor({ state: 'visible' });
  const costNoteClamped = await page.locator('#costScaleNote').innerText();
  assert(costNoteClamped.includes('分钟超出该范围上限，已按小时显示'), `costScaleNote expected clamp message, got: ${costNoteClamped}`);

  // Switch back to Token tab
  await page.locator('.tab[data-tab="usage"]').click();
  await page.locator('#panelUsage').waitFor({ state: 'visible' });
  const minuteActive = await page.locator('#scaleControl button[data-scale="minute"]').getAttribute('class');
  assert(minuteActive.includes('is-active'), `minute button should stay active after click, got: ${minuteActive}`);

  // 3. Select "hour": fits cap, clamp note disappears
  currentScenario = 'explicit-hour';
  await page.locator('#scaleControl button[data-scale="hour"]').click();
  await page.locator('#scaleHint').waitFor({ state: 'hidden' });
  const reqNoteHour = await page.locator('#requestScaleNote').innerText();
  assert(reqNoteHour.includes('尺度：小时'), `requestScaleNote expected 尺度：小时, got: ${reqNoteHour}`);
  assert(!reqNoteHour.includes('超出'), `requestScaleNote should not contain clamp message: ${reqNoteHour}`);

  // 4. Test 1440 minute buckets rendering without page errors or overflow
  currentScenario = 'large-minute-1440';
  await page.locator('#scaleControl button[data-scale="minute"]').click();
  await page.waitForFunction(() => document.querySelectorAll('#tokenChart rect, #tokenChart path').length > 100);
  assert(await page.locator('#scaleHint').isHidden(), 'scaleHint should be hidden when 1440 buckets fit');
  const reqNoteMinute = await page.locator('#requestScaleNote').innerText();
  assert(reqNoteMinute.includes('尺度：分钟'), `requestScaleNote expected 尺度：分钟, got: ${reqNoteMinute}`);

  // Check mobile 360px viewport
  await page.setViewportSize({ width: 360, height: 780 });
  await page.waitForTimeout(300);
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1);
  assert(overflow, 'page should not horizontally overflow at 360px with 1440 bars');

  assert(errors.length === 0, `unexpected page errors: ${errors.join('\n')}`);
  await context.close();
} finally {
  await browser.close();
  server.close();
}
