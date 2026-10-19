import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_token_cost_tab.mjs <dashboard-html-path> <google-chrome-path>');
}

const dashboardHTML = await readFile(htmlPath);
const resourceBase = '/v0/resource/plugins/cap-token-usage-tracker';
const managementBase = '/v0/management/plugins/cap-token-usage-tracker';
const SESSION = 'test-dashboard-session-token-cost-0001';
const MANAGEMENT_KEY = 'test-management-key';
const now = Date.UTC(2026, 9, 17, 12, 0, 0);
const HOUR = 3600 * 1000;

// Every /stats/initial request is recorded so the test can assert which axis
// scale the browser asked the server for.
const initialRequests = [];
let unauthorizedHits = 0;

const SCALES = {
  day: { scale: 'day', bucket_seconds: 86400, count: 30, step: 24 * HOUR },
  hour: { scale: 'hour', bucket_seconds: 3600, count: 48, step: HOUR },
  minute: { scale: 'minute', bucket_seconds: 60, count: 90, step: 60 * 1000 },
};

function resolveScale(requested) {
  if (requested === 'minute' || requested === 'hour' || requested === 'day') return SCALES[requested];
  // Auto over a 30 day window resolves to day buckets.
  return SCALES.day;
}

function buildPayload(requested, tzOffset) {
  const spec = resolveScale(requested);
  const series = [];
  const costSeries = [];
  // The stacked histogram split must stay aligned with cost_series: every
  // series carries one value per bucket, and the values add up to the bucket
  // total. The last model entry exercises the folded "others" label.
  const breakdown = {
    model: [
      { key: 'gemini-3.8-flash-high', total: 1.0, share: 1.0 },
      { key: 'gpt-6-luna', total: 0.1, share: 0.1 },
      { key: '', total: 0.16, share: 0.16, others: true },
    ],
    key: [
      { key: 'g1:aaaaaa', total: 1.1, share: 1.1 },
      { key: 'g1:bbbbbb', total: 0.16, share: 0.16 },
    ],
  };
  const first = now - (spec.count - 1) * spec.step;
  for (let index = 0; index < spec.count; index += 1) {
    const at = new Date(first + index * spec.step).toISOString();
    const active = index >= spec.count - 3;
    const requests = active ? 120 + index : 0;
    const input = active ? 240000 + index : 0;
    const cacheRead = active ? 60000 : 0;
    const output = active ? 40000 : 0;
    series.push({
      hour: at,
      requests,
      failed_requests: active ? 1 : 0,
      input_tokens: input,
      output_tokens: output,
      reasoning_tokens: active ? 1200 : 0,
      cache_read_tokens: cacheRead,
      cache_creation_tokens: active ? 300 : 0,
      total_tokens: input + output,
    });
    costSeries.push({
      hour: at,
      cost_usd: active ? 0.42 : 0,
      requests,
      unpriced_requests: active ? 5 : 0,
    });
    for (const dimension of Object.keys(breakdown)) {
      for (const entry of breakdown[dimension]) {
        entry.values = entry.values || [];
        entry.values.push(active ? entry.share / 3 : 0);
      }
    }
  }
  const costByModel = breakdown.model.map((entry) => ({
    key: entry.key,
    total_cost_usd: entry.total,
    values: entry.values,
    ...(entry.others ? { others: true } : {}),
  }));
  const costByKey = breakdown.key.map((entry) => ({
    key: entry.key,
    total_cost_usd: entry.total,
    values: entry.values,
  }));
  const summary = {
    requests: 726,
    failed_requests: 3,
    input_tokens: 720600,
    output_tokens: 120000,
    cache_read_tokens: 180000,
    total_tokens: 840600,
  };
  return {
    schema_version: 3,
    generated_at: new Date(now).toISOString(),
    range: requested === 'auto' ? '30d' : '30d',
    retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
    last_used: new Date(now - 4 * 60 * 1000).toISOString(),
    scale: spec.scale,
    bucket_seconds: spec.bucket_seconds,
    tz_offset_minutes: tzOffset,
    summary,
    cost: {
      cost_usd: 1.26,
      priced_models: 3,
      free_models: 1,
      unpriced_models: 1,
      priced_requests: 720,
      unpriced_requests: 6,
      time_tier_cost_usd: 0.18,
      time_tier_requests: 120,
    },
    models: [
      { model: 'gemini-3.8-flash-high', requests: 600, total_tokens: 700000, cost_usd: 1.1, priced: true, in_range: true },
      { model: 'Beam', requests: 120, total_tokens: 140600, cost_usd: 0, priced: false, in_range: true },
    ],
    series,
    cost_series: costSeries,
    cost_by_model: costByModel,
    cost_by_key: costByKey,
    sources: ['cli'],
    // Four keys on purpose: the two keys that own the cost breakdown sit at
    // index 2 and 3, past the first two entries. A label lookup that scanned
    // only the first two keys (or the wrong list) throws here and leaves the
    // model histogram on screen after switching the grouping.
    api_keys: [
      { ref: 'g1:old001', label: '归档 Key', requests: 0, total_tokens: 0, cost_usd: 0, priced_requests: 0, unpriced_requests: 0 },
      { ref: 'g1:new002', label: '测试 Key', requests: 0, total_tokens: 0, cost_usd: 0, priced_requests: 0, unpriced_requests: 0 },
      { ref: 'g1:aaaaaa', label: '主 Key', requests: 600, total_tokens: 700000, cost_usd: 1.1, priced_requests: 600, unpriced_requests: 0 },
      { ref: 'g1:bbbbbb', label: '备用 Key', requests: 126, total_tokens: 140600, cost_usd: 0.16, priced_requests: 120, unpriced_requests: 6 },
    ],
    pricing: {
      source: 'embedded',
      generated_at: new Date(now - 2 * HOUR).toISOString(),
      stale: false,
      status: 'ok',
      providers: 232,
      models: 8991,
      priced_models: 8538,
      free_models: 677,
      auto_refresh: true,
      remote_enabled: true,
      last_error: '',
      stale_after_seconds: 86400,
      overrides: 0,
    },
  };
}

const requestItems = [
  {
    time: new Date(now - 5 * 60 * 1000).toISOString(),
    model: 'gemini-3.8-flash-high',
    provider: 'cap-opencode-go',
    source: 'cli',
    api_key: '主 Key',
    api_key_ref: 'g1:aaaaaa',
    input_tokens: 12000,
    output_tokens: 3400,
    reasoning_tokens: 800,
    cache_read_tokens: 9000,
    cache_creation_tokens: 400,
    total_tokens: 15400,
    ttft_ns: 1250000000,
    latency_ns: 8400000000,
    tps: 404.76,
    end_to_end_tps: 1833.33,
    result: '成功',
    failed: false,
    failure_status: 0,
  },
  {
    time: new Date(now - 9 * 60 * 1000).toISOString(),
    model: 'Beam',
    provider: 'cap-opencode-go',
    source: 'cli',
    api_key: '备用 Key',
    api_key_ref: 'g1:bbbbbb',
    input_tokens: 800,
    output_tokens: 0,
    reasoning_tokens: 0,
    cache_read_tokens: 0,
    cache_creation_tokens: 0,
    total_tokens: 800,
    ttft_ns: 0,
    latency_ns: 120000000,
    tps: 0,
    end_to_end_tps: 0,
    result: '失败 (HTTP 500)',
    failed: true,
    failure_status: 500,
  },
];

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
    unauthorizedHits += 1;
    sendJSON({ error: 'dashboard session is missing or expired', code: 'session_required' }, 401);
    return;
  }
  if (url.pathname === `${resourceBase}/preferences`) {
    sendJSON({});
    return;
  }
  if (url.pathname === `${resourceBase}/stats/initial`) {
    const requested = url.searchParams.get('scale') || 'auto';
    const tzOffset = Number(url.searchParams.get('tz_offset') || 0);
    initialRequests.push({ scale: requested, tzOffset });
    sendJSON(buildPayload(requested, tzOffset));
    return;
  }
  if (url.pathname === `${resourceBase}/stats/models`) {
    sendJSON({
      schema_version: 1,
      generated_at: new Date(now).toISOString(),
      range: '30d',
      retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
      pricing: buildPayload('auto', 0).pricing,
      ttft_supplement: { enabled: true, available: false, sources: [], written_at: '', models: 0, samples: 0, error: 'no sibling state directory' },
      summary: { requests: 726, cost_usd: 1.26, priced_models: 3, free_models: 1, unpriced_models: 1, unpriced_requests: 6 },
      truncated: false,
      models: [
        { model: 'gemini-3.8-flash-high', requests: 600, total_tokens: 700000, cost_usd: 1.1, priced: true, in_range: true, pricing: { price: { input: 0.75, output: 3.75 }, priced: true, free: false, price_source: 'catalog', catalog_provider: 'google', catalog_model: 'gemini-3.8-flash' } },
        { model: 'Beam', requests: 126, total_tokens: 140600, cost_usd: 0, priced: false, in_range: true, pricing: { price: {}, priced: false, free: false, price_source: 'none' } },
      ],
    });
    return;
  }
  if (url.pathname === `${resourceBase}/requests`) {
    sendJSON({
      generated_at: new Date(now).toISOString(),
      range: '30d',
      total: requestItems.length,
      offset: 0,
      limit: 30,
      items: requestItems,
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

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

// zh-CN axis labels for the bucket timestamps the mock returns.
function axisLabel(iso, scale, timeZone = 'UTC') {
  const date = new Date(iso);
  const options = scale === 'minute'
    ? { hour: '2-digit', minute: '2-digit', hour12: false, timeZone }
    : scale === 'day'
      ? { month: 'numeric', day: 'numeric', timeZone }
      : { month: 'numeric', day: 'numeric', hour: '2-digit', hour12: false, timeZone };
  return new Intl.DateTimeFormat('zh-CN', options).format(date);
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
  page.on('pageerror', (error) => errors.push(String(error)));
  await unlock(page);

  /* ---------------------------- tabs and shared filter bar ---------------------------- */
  const tabLabels = await page.locator('.tabs .tab').allInnerTexts();
  assert(tabLabels.join('|') === 'Token|花销|模型|请求日志|数据管理', `tab order changed: ${JSON.stringify(tabLabels)}`);
  assert(await page.locator('#globalFilterBar').count() === 1, 'the filter bar must exist exactly once');
  assert(await page.locator('#panelUsage #rangeButton').count() === 0, 'the filter bar must not live inside a single panel');

  /* ---------------------------- token tab: one chart of each kind ---------------------------- */
  assert(await page.locator('#metricControl').count() === 0, 'the token/request metric toggle must be gone');
  assert(await page.locator('#tokenChart').count() === 1, 'token chart missing');
  assert(await page.locator('#requestChart').count() === 1, 'request trend chart missing');
  const tokenBars = await page.locator('#tokenChart path').count();
  assert(tokenBars > 0, 'the token chart drew no stacked bars');

  const defaultScale = initialRequests[initialRequests.length - 1];
  assert(defaultScale.scale === 'auto', `the first load must ask for the automatic scale: ${JSON.stringify(defaultScale)}`);
  assert(defaultScale.tzOffset === 0, `the browser timezone offset was not sent: ${JSON.stringify(defaultScale)}`);
  const scaleNote = await page.locator('#requestScaleNote').innerText();
  assert(scaleNote.includes('尺度') && scaleNote.includes('天'), `auto scale note = ${scaleNote}`);

  // The request trend axis must cover the whole window and label the last bucket.
  const requestAxis = await page.evaluate(() => {
    const texts = Array.from(document.querySelectorAll('#requestChart text'));
    const positions = texts.map((node) => ({ text: node.textContent, x: Number(node.getAttribute('x')), anchor: node.getAttribute('text-anchor') }));
    return positions;
  });
  const lastBucket = new Date(now).toISOString();
  const expectedLast = axisLabel(lastBucket, 'day');
  const axisTexts = requestAxis.map((entry) => entry.text);
  assert(axisTexts.includes(expectedLast), `request axis is missing the last bucket label ${expectedLast}: ${JSON.stringify(axisTexts)}`);
  const lastEntry = requestAxis.find((entry) => entry.text === expectedLast);
  assert(lastEntry.anchor === 'end', `the last axis label is not right-anchored: ${JSON.stringify(lastEntry)}`);

  /* ---------------------------- scale control round trip ---------------------------- */
  await page.locator('#scaleControl button[data-scale="minute"]').click();
  await page.waitForFunction(() => document.querySelector('#requestScaleNote').textContent.includes('分钟'));
  const minuteRequest = initialRequests[initialRequests.length - 1];
  assert(minuteRequest.scale === 'minute', `the minute scale was not requested: ${JSON.stringify(minuteRequest)}`);
  await page.waitForFunction(() => document.querySelectorAll('#requestChart text').length > 4);
  const minuteAxis = await page.evaluate(() => Array.from(document.querySelectorAll('#requestChart text')).map((node) => node.textContent));
  assert(minuteAxis.includes(axisLabel(new Date(now).toISOString(), 'minute')), `minute axis has no HH:mm labels: ${JSON.stringify(minuteAxis)}`);

  await page.locator('#scaleControl button[data-scale="day"]').click();
  await page.waitForFunction(() => document.querySelector('#requestScaleNote').textContent.includes('尺度：天'));
  assert(initialRequests[initialRequests.length - 1].scale === 'day', 'the explicit day scale was not requested');

  /* ---------------------------- cost tab mirrors the token tab ---------------------------- */
  await page.locator('.tab[data-tab="cost"]').click();
  await page.locator('#panelCost').waitFor({ state: 'visible' });
  assert(await page.locator('#globalFilterBar').isVisible(), 'the shared filter bar is hidden on the cost tab');
  await page.waitForFunction(() => document.querySelector('#costTotal').textContent.trim() !== '$0');
  const cost = await page.evaluate(() => ({
    total: document.querySelector('#costTotal').textContent.trim(),
    note: document.querySelector('#costRangeNote').textContent.trim(),
    priced: document.querySelector('#costPricedModels').textContent.trim(),
    unpriced: document.querySelector('#costUnpricedModels').textContent.trim(),
    coverage: document.querySelector('#costCoverage').textContent.trim(),
    scaleNote: document.querySelector('#costScaleNote').textContent.trim(),
    trend: document.querySelector('#costTrendTotal').textContent.trim(),
    modelRank: document.querySelector('#costModelRank').innerText,
    keyRank: document.querySelector('#costApiKeyRank').innerText,
    chartSeries: document.querySelectorAll('#costChart path').length,
    chartBars: document.querySelectorAll('#costChart rect, #costChart path').length,
    chartColors: Array.from(new Set(Array.from(document.querySelectorAll('#costChart rect, #costChart path'))
      .map((node) => node.getAttribute('fill'))
      .filter((fill) => fill && fill !== 'transparent' && fill !== 'none'))).length,
    legend: document.querySelector('#costChartLegend').innerText,
    legendHidden: document.querySelector('#costChartLegend').hidden,
    dimensionActive: document.querySelector('#costDimensionControl button.is-active').getAttribute('data-cost-dimension'),
    dimensionVisible: !!document.querySelector('#costDimensionControl') && document.querySelector('#costDimensionControl').getBoundingClientRect().height > 0,
    tierNote: document.querySelector('#costTimeTierNote').textContent.trim(),
    tierNoteHidden: document.querySelector('#costTimeTierNote').hidden,
  }));
  assert(cost.total.startsWith('$1.26'), `cost hero = ${cost.total}`);
  assert(cost.note.includes('3') && cost.note.includes('1'), `cost subtitle = ${cost.note}`);
  assert(cost.priced === '3', `priced model card = ${cost.priced}`);
  assert(cost.unpriced === '1', `unpriced model card = ${cost.unpriced}`);
  assert(cost.coverage.endsWith('%'), `coverage card = ${cost.coverage}`);
  assert(cost.scaleNote.includes('尺度'), `cost scale note = ${cost.scaleNote}`);
  assert(cost.trend.startsWith('$'), `cost trend total = ${cost.trend}`);
  assert(cost.modelRank.includes('gemini-3.8-flash-high') && cost.modelRank.includes('$1.1'), `model cost ranking = ${cost.modelRank}`);
  assert(cost.keyRank.includes('主 Key'), `key cost ranking = ${cost.keyRank}`);
  assert(cost.chartSeries > 0, 'the cost chart drew nothing');
  // The cost trend is a stacked histogram: several segments per bucket, one
  // colour per series, and a legend that names them with their totals.
  assert(cost.chartBars > 3, `the cost histogram drew ${cost.chartBars} segments`);
  assert(cost.chartColors >= 3, `the cost histogram has ${cost.chartColors} colours, want at least 3`);
  assert(cost.dimensionVisible, 'the cost grouping control is missing');
  assert(cost.dimensionActive === 'model', `default cost grouping = ${cost.dimensionActive}`);
  assert(!cost.legendHidden, 'the cost legend is hidden');
  assert(cost.legend.includes('gemini-3.8-flash-high'), `cost legend misses the model: ${cost.legend}`);
  assert(cost.legend.includes('其他'), `cost legend misses the folded remainder: ${cost.legend}`);
  // The peak/off-peak delta is explained next to the hero total.
  assert(!cost.tierNoteHidden, 'the time-of-use note is hidden even though the payload carries a delta');
  assert(cost.tierNote.includes('+$0.18') && cost.tierNote.includes('120'), `time-of-use note = ${cost.tierNote}`);
  // formatMoney trims trailing zeros above one dollar, so the totals read
  // $1 / $0.1000 / $0.1600.
  assert(cost.legend.includes('$1') && cost.legend.includes('$0.1000') && cost.legend.includes('$0.1600'), `cost legend totals = ${cost.legend}`);
  // The cost tab must not duplicate the request trend chart.
  assert(await page.locator('#panelCost #requestChart').count() === 0, 'the cost tab duplicates the request trend chart');

  // Switching the grouping is a pure redraw: both breakdowns ship with the
  // initial payload, so no further /stats/initial request may be issued.
  const requestsBeforeGrouping = initialRequests.length;
  const modelHistogram = await page.evaluate(() => document.querySelector('#costChart').innerHTML);
  await page.locator('#costDimensionControl button[data-cost-dimension="key"]').click();
  await page.waitForFunction(() => document.querySelector('#costChartLegend').innerText.includes('主 Key'));
  const keyGrouping = await page.evaluate(() => ({
    legend: document.querySelector('#costChartLegend').innerText,
    active: document.querySelector('#costDimensionControl button.is-active').getAttribute('data-cost-dimension'),
    svg: document.querySelector('#costChart').innerHTML,
    colors: Array.from(new Set(Array.from(document.querySelectorAll('#costChart rect, #costChart path'))
      .map((node) => node.getAttribute('fill'))
      .filter((fill) => fill && fill !== 'transparent' && fill !== 'none'))).length,
  }));
  assert(keyGrouping.active === 'key', `grouping switch did not stick: ${keyGrouping.active}`);
  // The bars themselves must be repainted, not just the legend: a thrown error
  // while resolving labels used to leave the model histogram on screen.
  assert(keyGrouping.svg !== modelHistogram, 'the histogram was not redrawn for the key grouping');
  assert(keyGrouping.legend.includes('主 Key') && keyGrouping.legend.includes('备用 Key'), `key legend = ${keyGrouping.legend}`);
  assert(!keyGrouping.legend.includes('gemini-3.8-flash-high'), `key legend still lists models: ${keyGrouping.legend}`);
  assert(keyGrouping.colors >= 2, `key histogram has ${keyGrouping.colors} colours`);
  assert(initialRequests.length === requestsBeforeGrouping, 'switching the cost grouping refetched the payload');

  // Hovering a bucket lists one row per non-empty series plus the bucket total.
  await page.locator('#costChartHost').scrollIntoViewIfNeeded();
  const box = await page.locator('#costChart').boundingBox();
  await page.mouse.move(box.x + box.width * 0.92, box.y + box.height * 0.6);
  await page.waitForFunction(() => document.querySelector('#costTip').classList.contains('is-visible'));
  const tip = await page.evaluate(() => ({
    head: document.querySelector('#costTip .tip-head').innerText,
    rows: document.querySelectorAll('#costTip .tip-row').length,
  }));
  assert(tip.head.includes('$'), `cost tooltip total = ${tip.head}`);
  assert(tip.rows === 2, `cost tooltip rows = ${tip.rows}, want one per non-empty series`);

  await page.locator('#costDimensionControl button[data-cost-dimension="model"]').click();
  await page.waitForFunction(() => document.querySelector('#costChartLegend').innerText.includes('gemini-3.8-flash-high'));

  // Clicking a cost ranking row reuses the shared model filter.
  await page.locator('#costModelRank .rank-row').first().click();
  await page.waitForFunction(() => document.querySelector('#modelValue').textContent.includes('1'));
  const modelChip = await page.locator('#modelValue').innerText();
  assert(modelChip.includes('1'), `cost ranking did not apply the model filter: ${modelChip}`);

  /* ---------------------------- request log columns ---------------------------- */
  await page.locator('.tab[data-tab="requests"]').click();
  await page.locator('#panelRequests').waitFor({ state: 'visible' });
  await page.waitForFunction(() => document.querySelectorAll('#requestRows tr').length === 2);
  const headers = await page.locator('#panelRequests thead th').allInnerTexts();
  const wanted = ['时间', '模型名称', 'API Key', '输入', '输出', '推理', '缓存读取', '缓存写入', '总 Token 数', '首字延迟', '持续时长', '生成 TPS', '端到端 TPS', '结果'];
  assert(headers.join('|') === wanted.join('|'), `request log headers = ${JSON.stringify(headers)}`);
  const rows = await page.evaluate(() => Array.from(document.querySelectorAll('#requestRows tr')).map((row) => ({
    cells: Array.from(row.children).map((cell) => cell.textContent.trim()),
    status: row.lastElementChild.className,
  })));
  const ok = rows[0];
  assert(ok.cells[3] === '12,000' && ok.cells[4] === '3,400' && ok.cells[5] === '800', `token columns = ${JSON.stringify(ok.cells)}`);
  assert(ok.cells[6] === '9,000' && ok.cells[7] === '400' && ok.cells[8] === '15,400', `cache/total columns = ${JSON.stringify(ok.cells)}`);
  assert(ok.cells[9] === '1.25 s', `TTFT column = ${ok.cells[9]}`);
  assert(ok.cells[10] === '8.4 s', `duration column = ${ok.cells[10]}`);
  assert(ok.cells[11] === '405', `generation TPS column = ${ok.cells[11]}`);
  assert(ok.cells[12] === '1833', `end-to-end TPS column = ${ok.cells[12]}`);
  assert(ok.cells[13] === '成功', `success label = ${ok.cells[13]}`);
  const failed = rows[1];
  assert(failed.cells[9] === '—' && failed.cells[11] === '—', `missing metrics must render as a dash: ${JSON.stringify(failed.cells)}`);
  assert(failed.cells[13] === '失败 (HTTP 500)', `failure label = ${failed.cells[13]}`);
  assert(failed.status.includes('bad'), `failure row is not marked as failed: ${failed.status}`);
  const modelTitle = await page.locator('#requestRows tr').first().locator('td').nth(1).getAttribute('title');
  assert(modelTitle.includes('cap-opencode-go'), `model cell does not name the provider: ${modelTitle}`);

  assert(errors.length === 0, `page errors: ${errors.join('\n')}`);
  await context.close();

  /* ---------------------------- mobile chips keep their labels ---------------------------- */
  for (const width of [360, 390]) {
    const mobileContext = await browser.newContext({ locale: 'zh-CN', timezoneId: 'UTC', viewport: { width, height: 780 } });
    const mobile = await mobileContext.newPage();
    const mobileErrors = [];
    mobile.on('pageerror', (error) => mobileErrors.push(String(error)));
    await unlock(mobile);
    const layout = await mobile.evaluate(() => {
      const tag = document.querySelector('#modelButton .chip-tag');
      const style = getComputedStyle(tag);
      const keyTag = document.querySelector('#apiKeyButton .chip-tag');
      return {
        innerWidth: window.innerWidth,
        scrollWidth: document.documentElement.scrollWidth,
        modelTagDisplay: style.display,
        modelTagText: tag.textContent.trim(),
        keyTagText: keyTag.textContent.trim(),
        value: document.querySelector('#modelValue').textContent.trim(),
        tabs: Array.from(document.querySelectorAll('.tabs .tab')).map((tab) => tab.textContent.trim()),
      };
    });
    assert(layout.modelTagDisplay !== 'none', `chip label hidden at ${width}px: ${JSON.stringify(layout)}`);
    assert(layout.modelTagText.length > 0 && layout.keyTagText.length > 0, `chip tags lost their text at ${width}px: ${JSON.stringify(layout)}`);
    assert(layout.value.length > 0, `chip value empty at ${width}px: ${JSON.stringify(layout)}`);
    assert(layout.tabs.join('|') === 'Token|花销|模型|请求日志|数据管理', `mobile tab labels = ${JSON.stringify(layout.tabs)}`);
    assert(layout.scrollWidth <= layout.innerWidth + 1, `page scrolls horizontally at ${width}px: ${JSON.stringify(layout)}`);
    assert(mobileErrors.length === 0, `page errors at ${width}px: ${mobileErrors.join('\n')}`);
    await mobileContext.close();
  }

  assert(unauthorizedHits >= 1, 'the mock host never refused an unauthenticated data request');
} finally {
  await browser.close();
  server.close();
}
