import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_models_tab.mjs <dashboard-html-path> <google-chrome-path>');
}

const dashboardHTML = await readFile(htmlPath);
const resourceBase = '/v0/resource/plugins/cap-token-usage-tracker';
const managementBase = '/v0/management/plugins/cap-token-usage-tracker';
const SESSION = 'test-dashboard-session-models-000000';
const MANAGEMENT_KEY = 'test-management-key';

const HOUR = 3600 * 1000;
const now = Date.UTC(2026, 9, 17, 12, 0, 0);

let stale = false;
let refreshRequests = 0;
let unauthorizedHits = 0;
const savedPrices = new Map();

const pricingBlock = () => ({
  source: 'embedded',
  generated_at: new Date(now - (stale ? 72 : 2) * HOUR).toISOString(),
  stale,
  status: stale ? 'stale' : 'ok',
  providers: 232,
  models: 8991,
  priced_models: 8538,
  free_models: 677,
  auto_refresh: true,
  remote_enabled: true,
  last_error: '',
  stale_after_seconds: 86400,
  overrides: savedPrices.size,
});

function modelRows() {
  const rows = [
    {
      model: 'gemini-3.8-flash-high',
      requests: 4000,
      input_tokens: 900000000,
      output_tokens: 300000000,
      total_tokens: 1200000000,
      total_latency_ns: 1.5e12,
      total_ttft_ns: 6.0e12,
      average_latency_ns: 375000000,
      average_ttft_ns: 1500000000,
      generation_tps: 812.5,
      end_to_end_tps: 640.25,
      cost_usd: 12.3456,
      priced: true,
      priced_requests: 4000,
      unpriced_requests: 0,
      in_range: true,
      pricing: {
        price: { input: 0.75, output: 3.75, cache_read: 0.075 },
        priced: true,
        free: false,
        price_source: 'catalog',
        catalog_provider: 'google',
        catalog_model: 'gemini-3.8-flash',
        catalog_match: 'variant',
        time_zone: 'UTC',
        time_plan_source: 'builtin',
        time_tiers: [
          { name: 'peak', days: [1, 2, 3, 4, 5], start: '01:00', end: '04:00', input: 1.5, output: 7.5, cache_read: 0.15 },
        ],
      },
    },
    {
      model: 'cat-jam-free',
      requests: 10,
      input_tokens: 400000,
      output_tokens: 100000,
      total_tokens: 500000,
      total_latency_ns: 12400000,
      total_ttft_ns: 3200000,
      average_latency_ns: 1240000,
      average_ttft_ns: 320000,
      generation_tps: 44.5,
      end_to_end_tps: 40.1,
      cost_usd: 0,
      priced: true,
      priced_requests: 10,
      unpriced_requests: 0,
      in_range: true,
      pricing: {
        price: { input: 0, output: 0 },
        priced: true,
        free: true,
        price_source: 'catalog',
        catalog_provider: 'opencode-go',
        catalog_model: 'cat-jam-free',
        catalog_match: 'provider',
      },
    },
    {
      model: 'Beam',
      requests: 120,
      input_tokens: 3000000,
      output_tokens: 400000,
      total_tokens: 3400000,
      total_latency_ns: 9.6e10,
      total_ttft_ns: 1.2e11,
      ttft_samples: 120,
      supplemented_ttft_samples: 120,
      average_latency_ns: 800000000,
      average_ttft_ns: 1000000000,
      generation_tps: 0,
      end_to_end_tps: 0,
      cost_usd: 0,
      priced: false,
      priced_requests: 0,
      unpriced_requests: 120,
      in_range: true,
      pricing: { price: {}, priced: false, free: false, price_source: 'none' },
    },
    {
      model: 'mimo-v2.6-flash',
      requests: 0,
      total_tokens: 0,
      cost_usd: 0,
      priced: true,
      in_range: false,
      pricing: {
        price: { input: 0.14, output: 0.28 },
        priced: true,
        free: false,
        price_source: 'catalog',
        catalog_provider: 'xiaomi',
        catalog_model: 'mimo-v2.6-flash',
        catalog_match: 'global',
      },
    },
  ];
  for (const [model, override] of savedPrices) {
    const row = rows.find((item) => item.model === model);
    if (!row) continue;
    const base = row.pricing.price || {};
    const input = override.input !== undefined ? override.input : base.input;
    const output = override.output !== undefined ? override.output : base.output;
    row.priced = true;
    row.priced_requests = row.requests;
    row.unpriced_requests = 0;
    if (input !== undefined && output !== undefined) {
      row.cost_usd = (row.input_tokens * input + row.output_tokens * output) / 1e6;
    }
    row.pricing = Object.assign({}, row.pricing, {
      price: Object.assign({}, base, override),
      override,
      priced: true,
      free: false,
      price_source: 'override',
    });
    if (Array.isArray(override.time_tiers) && override.time_tiers.length) {
      row.pricing.time_zone = override.time_zone || 'UTC';
      row.pricing.time_tiers = override.time_tiers;
      row.pricing.time_plan_source = 'override';
    }
  }
  return rows;
}

function modelsSummary(rows) {
  const inRange = rows.filter((row) => row.in_range);
  return {
    requests: inRange.reduce((sum, row) => sum + row.requests, 0),
    cost_usd: inRange.reduce((sum, row) => sum + row.cost_usd, 0),
    priced_models: inRange.filter((row) => row.priced && !row.pricing.free).length,
    free_models: inRange.filter((row) => row.pricing.free).length,
    unpriced_models: inRange.filter((row) => !row.priced).length,
    unpriced_requests: inRange.reduce((sum, row) => sum + row.unpriced_requests, 0),
  };
}

const initialModels = [
  { model: 'gemini-3.8-flash-high', requests: 4000, total_tokens: 1200000000, cost_usd: 12.3456, priced: true },
  { model: 'Beam', requests: 120, total_tokens: 3400000, cost_usd: 0, priced: false },
];

async function readBody(request) {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  return Buffer.concat(chunks).toString('utf8');
}

const server = createServer(async (request, response) => {
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
    const rows = modelRows();
    const summary = modelsSummary(rows);
    const buckets = [];
    const costBuckets = [];
    for (let index = 29; index >= 0; index -= 1) {
      const at = new Date(now - index * 24 * HOUR).toISOString();
      const active = index < 3;
      buckets.push({
        hour: at,
        requests: active ? 1400 : 0,
        failed_requests: active ? 2 : 0,
        input_tokens: active ? 301133333 : 0,
        output_tokens: active ? 100166666 : 0,
        cache_read_tokens: active ? 260000000 : 0,
        total_tokens: active ? 401300000 : 0,
      });
      costBuckets.push({
        hour: at,
        cost_usd: active ? summary.cost_usd / 3 : 0,
        requests: active ? 1400 : 0,
        unpriced_requests: active ? 40 : 0,
      });
    }
    sendJSON({
      schema_version: 3,
      generated_at: new Date(now).toISOString(),
      range: '30d',
      retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
      last_used: new Date(now - 5 * 60 * 1000).toISOString(),
      scale: 'day',
      bucket_seconds: 86400,
      summary: {
        requests: 4130,
        failed_requests: 4,
        input_tokens: 903400000,
        output_tokens: 300500000,
        cache_read_tokens: 780000000,
        total_tokens: 1203900000,
      },
      // The real payload carries the derived cost in its own block.
      cost: {
        cost_usd: summary.cost_usd,
        priced_models: summary.priced_models,
        free_models: summary.free_models,
        unpriced_models: summary.unpriced_models,
        priced_requests: 4010,
        unpriced_requests: summary.unpriced_requests,
      },
      models: initialModels.map((row) => {
        const live = rows.find((item) => item.model === row.model) || row;
        return Object.assign({}, row, { cost_usd: live.cost_usd, priced: live.priced });
      }),
      series: buckets,
      cost_series: costBuckets,
      sources: ['cli'],
      api_keys: [
        { ref: 'g1:aaaaaa', label: '主 Key', requests: 3000, total_tokens: 900000000, cost_usd: 10.5, priced_requests: 3000, unpriced_requests: 0 },
        { ref: 'g1:bbbbbb', label: '备用 Key', requests: 1130, total_tokens: 303900000, cost_usd: 1.8456, priced_requests: 1010, unpriced_requests: 120 },
      ],
      pricing: pricingBlock(),
    });
    return;
  }
  if (url.pathname === `${resourceBase}/stats/models`) {
    const rows = modelRows();
    sendJSON({
      schema_version: 1,
      generated_at: new Date(now).toISOString(),
      range: '30d',
      retained_since: new Date(now - 30 * 24 * HOUR).toISOString(),
      pricing: pricingBlock(),
      ttft_supplement: {
        enabled: true,
        available: true,
        sources: ['shared.json', 'stats-chat.json'],
        written_at: new Date(now - 4 * 60 * 1000).toISOString(),
        models: 1,
        samples: 120,
      },
      summary: modelsSummary(rows),
      truncated: false,
      models: rows,
    });
    return;
  }
  if (url.pathname === `${resourceBase}/model-prices`) {
    if (request.method === 'PUT') {
      let payload = {};
      try {
        payload = JSON.parse((await readBody(request)) || '{}');
      } catch (error) {
        payload = {};
      }
      const model = payload.model || '';
      savedPrices.set(model, payload.price || {});
      sendJSON({
        pricing: pricingBlock(),
        overrides: Object.fromEntries(savedPrices),
        model,
        price: { price: payload.price || {}, priced: true, free: false, price_source: 'override' },
      });
      return;
    }
    if (url.searchParams.get('refresh') !== null) {
      refreshRequests += 1;
      stale = false;
      sendJSON({ ok: true, pricing: pricingBlock() });
      return;
    }
    if (url.searchParams.get('reset') !== null) {
      savedPrices.delete(url.searchParams.get('model') || '');
      sendJSON({ pricing: pricingBlock(), overrides: Object.fromEntries(savedPrices), model: url.searchParams.get('model') || '' });
      return;
    }
    if (url.searchParams.get('save') === '1') {
      const model = url.searchParams.get('model') || '';
      const price = {};
      for (const field of ['input', 'output', 'cache_read', 'cache_write']) {
        const raw = url.searchParams.get(field);
        if (raw !== null && raw !== '') price[field] = Number(raw);
      }
      savedPrices.set(model, price);
      sendJSON({
        pricing: pricingBlock(),
        overrides: Object.fromEntries(savedPrices),
        model,
        price: { price, priced: true, free: false, price_source: 'override' },
      });
      return;
    }
    sendJSON({ pricing: pricingBlock(), overrides: Object.fromEntries(savedPrices) });
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

function fail(message) {
  throw new Error(message);
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
  /* ------------------- tab order, table and search ------------------- */
  const context = await browser.newContext({ locale: 'zh-CN', timezoneId: 'UTC', viewport: { width: 1300, height: 1000 } });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(String(error)));
  // Count refreshes from page events instead of server state: the server
  // counter is only incremented once the request has been handled, which races
  // with the assertion.
  let manualRefreshes = 0;
  page.on('request', (request) => {
    const candidate = new URL(request.url());
    if (candidate.pathname === `${resourceBase}/model-prices` && candidate.searchParams.get('refresh') === '1') manualRefreshes += 1;
  });
  await unlock(page);

  const refused = await page.evaluate(async (base) => {
    const response = await fetch(`${base}/stats/models?range=30d`, { credentials: 'same-origin' });
    return response.status;
  }, resourceBase);
  if (refused !== 401) fail(`models route without a session returned ${refused}, want 401`);

  const tabLabels = await page.locator('.tabs .tab').allInnerTexts();
  if (tabLabels.join('|') !== 'Token|花销|模型|请求日志|数据管理') {
    fail(`tab order changed: ${JSON.stringify(tabLabels)}`);
  }

  // The token tab keeps a single stacked bar chart and no cost card.
  if (await page.locator('#totalCost').count()) fail('token tab still renders the cost card');
  if (await page.locator('#metricControl').count()) fail('token tab still renders the metric toggle');
  const requestScale = await page.locator('#requestScaleNote').innerText();
  if (!requestScale.includes('尺度') || !requestScale.includes('天')) fail(`request scale note = ${requestScale}`);

  // The cost tab mirrors the token layout with USD units.
  await page.locator('.tab[data-tab="cost"]').click();
  await page.locator('#panelCost').waitFor({ state: 'visible' });
  await page.waitForFunction(() => document.querySelector('#costTotal').textContent.trim() !== '$0');
  const costTab = await page.evaluate(() => ({
    total: document.querySelector('#costTotal').textContent.trim(),
    note: document.querySelector('#costRangeNote').textContent.trim(),
    coverage: document.querySelector('#costCoverage').textContent.trim(),
    unpriced: document.querySelector('#costUnpricedModels').textContent.trim(),
    rank: document.querySelector('#costModelRank').textContent.trim(),
    keyRank: document.querySelector('#costApiKeyRank').textContent.trim(),
    bars: document.querySelectorAll('#costChart rect, #costChart path').length,
    legend: document.querySelector('#costChartLegend').innerText,
  }));
  if (!costTab.total.startsWith('$') || costTab.total === '$0') fail(`cost tab total = ${costTab.total}`);
  if (!costTab.note.includes('刊例价') || !costTab.note.includes('待补价')) fail(`cost tab subtitle = ${costTab.note}`);
  if (!costTab.coverage.endsWith('%')) fail(`cost coverage = ${costTab.coverage}`);
  if (costTab.unpriced !== '1') fail(`unpriced model card = ${costTab.unpriced}`);
  if (!costTab.rank.includes('gemini-3.8-flash-high')) fail(`model cost ranking = ${costTab.rank}`);
  if (!costTab.keyRank.includes('主 Key')) fail(`key cost ranking = ${costTab.keyRank}`);
  if (costTab.bars < 1) fail('cost chart drew nothing');
  // This payload carries no cost breakdown arrays, so the histogram must fall
  // back to the aggregate cost series instead of rendering an empty chart.
  if (!costTab.legend.includes('花销趋势')) fail(`fallback cost legend = ${costTab.legend}`);
  if (costTab.legend.includes('其他')) fail(`fallback cost legend invented an others entry: ${costTab.legend}`);
  await page.locator('.tab[data-tab="usage"]').click();
  await page.locator('#panelUsage').waitFor({ state: 'visible' });

  await page.locator('.tab[data-tab="models"]').click();
  await page.locator('#panelModels').waitFor({ state: 'visible' });
  await page.waitForFunction(() => document.querySelectorAll('#modelList tr').length > 0);

  const rendered = await page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll('#modelList tr')).map((row) => {
      const cells = Array.from(row.children).map((cell) => cell.textContent.trim());
      return { cells, muted: row.classList.contains('row-muted') };
    });
    return {
      rows,
      count: document.querySelector('#modelsCount').textContent.trim(),
      totalCost: document.querySelector('#modelsTotalCost').textContent.trim(),
      note: document.querySelector('#catalogNote').textContent.trim(),
      sub: document.querySelector('#modelsSub').textContent.trim(),
      ttftNote: document.querySelector('#ttftNote').textContent.trim(),
    };
  });
  if (rendered.rows.length !== 4) fail(`models table rendered ${rendered.rows.length} rows: ${JSON.stringify(rendered.rows)}`);
  if (rendered.count !== '4') fail(`models count card = ${rendered.count}`);
  if (!rendered.totalCost.startsWith('$') || rendered.totalCost === '$0') fail(`models cost card = ${rendered.totalCost}`);
  if (!rendered.note.includes('内置快照') || !rendered.note.includes('8,991')) fail(`catalog note = ${rendered.note}`);
  if (!rendered.sub.includes('4')) fail(`models subtitle = ${rendered.sub}`);

  const gemini = rendered.rows[0];
  if (gemini.cells[0].indexOf('gemini-3.8-flash-high') !== 0) fail(`first row is not the biggest model: ${JSON.stringify(gemini)}`);
  if (!gemini.cells[0].includes('刊例价')) fail(`catalog badge missing: ${JSON.stringify(gemini.cells)}`);
  if (gemini.cells[1] !== 'google') fail(`provider tag = ${gemini.cells[1]}`);
  if (gemini.cells[2] !== '813' && gemini.cells[2] !== '812.5') fail(`generation TPS = ${gemini.cells[2]}`);
  if (!gemini.cells[3].startsWith('640')) fail(`end-to-end TPS = ${gemini.cells[3]}`);
  if (gemini.cells[4] !== '1.5 s') fail(`average TTFT = ${gemini.cells[4]}`);
  if (gemini.cells[5] !== '1,200,000,000') fail(`token total = ${gemini.cells[5]}`);
  if (gemini.cells[6] !== '$12.3456') fail(`cost = ${gemini.cells[6]}`);
  if (gemini.cells[7].indexOf('$0.75') !== 0 || !gemini.cells[7].includes('$3.75')) fail(`list price = ${gemini.cells[7]}`);
  if (!gemini.cells[7].includes('峰谷')) fail(`peak badge missing: ${JSON.stringify(gemini.cells[7])}`);
  const peakTitle = await page.locator('#modelList tr').first().locator('.badge-peak').getAttribute('title');
  if (!peakTitle.includes('UTC') || !peakTitle.includes('1,2,3,4,5') || !peakTitle.includes('01:00-04:00')) {
    fail(`peak badge tooltip = ${peakTitle}`);
  }

  const beam = rendered.rows.find((row) => row.cells[0].indexOf('Beam') === 0);
  if (!beam || !beam.cells[0].includes('无刊例价')) fail(`unpriced badge missing: ${JSON.stringify(beam)}`);
  if (beam.cells[6] !== '—') fail(`unpriced row shows a cost: ${JSON.stringify(beam.cells)}`);
  // The linked TTFT carries a marker and the footnote names the source plugin.
  if (!beam.cells[4].includes('1 s') || !beam.cells[4].includes('*')) fail(`linked TTFT marker missing: ${JSON.stringify(beam.cells)}`);
  if (!rendered.ttftNote.includes('cap-opencode-go') || !rendered.ttftNote.includes('120')) {
    fail(`TTFT linkage footnote = ${rendered.ttftNote}`);
  }
  const free = rendered.rows.find((row) => row.cells[0].indexOf('cat-jam-free') === 0);
  if (!free || !free.cells[0].includes('免费')) fail(`free badge missing: ${JSON.stringify(free)}`);
  const observed = rendered.rows.find((row) => row.cells[0].indexOf('mimo-v2.6-flash') === 0);
  if (!observed || !observed.muted || !observed.cells[5].includes('本期无用量')) {
    fail(`observed-only row is not marked: ${JSON.stringify(observed)}`);
  }

  // Search narrows the table without a new request.
  await page.locator('#modelSearch').fill('mimo');
  await page.waitForFunction(() => document.querySelectorAll('#modelList tr').length === 1);
  await page.locator('#modelSearch').fill('google');
  await page.waitForFunction(() => document.querySelectorAll('#modelList tr').length === 1);
  const searched = await page.locator('#modelList tr').first().innerText();
  if (!searched.includes('gemini-3.8-flash-high')) fail(`search by provider failed: ${searched}`);
  await page.locator('#modelSearch').fill('zzz-no-model');
  await page.waitForFunction(() => document.querySelector('#modelList').textContent.includes('没有匹配的模型'));
  await page.locator('#modelSearch').fill('');
  await page.waitForFunction(() => document.querySelectorAll('#modelList tr').length === 4);

  /* ---------------------------- price dialog ---------------------------- */
  // fillPriceField verifies that the value landed in the intended field:
  // Playwright focuses a number input before inserting text, and a dialog that
  // is still being laid out can otherwise send the keystrokes to the field that
  // still holds focus.
  async function fillPriceField(selector, value) {
    for (let attempt = 0; attempt < 4; attempt += 1) {
      await page.locator(selector).fill(value);
      if ((await page.locator(selector).inputValue()) === value) return;
    }
    return fail(`could not fill ${selector} with ${value}`);
  }

  // submitPrice retries the click once so a slow re-render can never turn a
  // correct save into a spurious timeout.
  async function submitPrice(predicate) {
    for (let attempt = 0; attempt < 3; attempt += 1) {
      const waiter = page.waitForRequest(predicate, { timeout: 6000 }).catch(() => null);
      await page.locator('#priceSave').click();
      const matched = await waiter;
      if (matched) return matched;
      if (!(await page.locator('#priceDialog').isVisible())) break;
      const message = await page.locator('#priceError').innerText();
      if (message.trim()) fail(`price dialog rejected the input: ${message}`);
    }
    const diagnostics = await page.evaluate(() => ({
      open: document.querySelector('#priceDialog').open,
      error: document.querySelector('#priceError').textContent.trim(),
      model: window.__dashboardTest.state.priceModel,
      input: document.querySelector('#priceInput').value,
      output: document.querySelector('#priceOutput').value,
      rows: document.querySelectorAll('#modelList tr').length,
    }));
    return fail(`price save did not issue a request: ${JSON.stringify(diagnostics)}`);
  }

  const beamRow = page.locator('#modelList tr', { hasText: 'Beam' }).first();
  await beamRow.locator('.gear-btn').click();
  await page.locator('#priceDialog').waitFor({ state: 'visible' });
  const dialogSub = await page.locator('#priceDialogSub').innerText();
  if (!dialogSub.includes('Beam') || !dialogSub.includes('没有找到刊例价')) fail(`price dialog subtitle = ${dialogSub}`);
  if (await page.locator('#priceReset').isVisible()) fail('reset button is visible without an override');
  await fillPriceField('#priceInput', '1.5');
  await fillPriceField('#priceOutput', '3');
  if ((await page.locator('#priceInput').inputValue()) !== '1.5' || (await page.locator('#priceOutput').inputValue()) !== '3') {
    fail('price fields lost their values before saving');
  }
  await submitPrice((request) => {
    if (request.method() !== 'PUT') return false;
    const candidate = new URL(request.url());
    if (candidate.pathname !== `${resourceBase}/model-prices`) return false;
    let payload;
    try {
      payload = JSON.parse(request.postData() || '{}');
    } catch (error) {
      return false;
    }
    return payload.model === 'Beam' && payload.price
      && payload.price.input === 1.5 && payload.price.output === 3
      && payload.price.time_tiers === undefined;
  });
  await page.locator('#priceDialog').waitFor({ state: 'hidden' });
  await page.waitForFunction(() => {
    const row = Array.from(document.querySelectorAll('#modelList tr')).find((item) => item.textContent.includes('Beam'));
    return !!row && row.textContent.includes('自定义');
  });
  const savedRow = await page.locator('#modelList tr', { hasText: 'Beam' }).first().innerText();
  if (!savedRow.includes('自定义')) fail(`saved row lost the override badge: ${savedRow}`);
  if (!savedRow.includes('$5.7')) fail(`override cost was not recomputed: ${savedRow}`);

  // Reopening the dialog prefills the stored override and offers a reset.
  await page.locator('#modelList tr', { hasText: 'Beam' }).first().locator('.gear-btn').click();
  await page.locator('#priceDialog').waitFor({ state: 'visible' });
  if ((await page.locator('#priceInput').inputValue()) !== '1.5') fail('override input was not prefilled');
  if ((await page.locator('#priceOutput').inputValue()) !== '3') fail('override output was not prefilled');
  if (!(await page.locator('#priceReset').isVisible())) fail('reset button missing for an override');
  const resetRequest = page.waitForRequest((request) => {
    const candidate = new URL(request.url());
    return candidate.pathname === `${resourceBase}/model-prices` && candidate.searchParams.get('reset') === '1';
  });
  await page.locator('#priceReset').click();
  await resetRequest;
  await page.locator('#priceDialog').waitFor({ state: 'hidden' });
  await page.waitForFunction(() => {
    const row = Array.from(document.querySelectorAll('#modelList tr')).find((item) => item.textContent.includes('Beam'));
    return !!row && row.textContent.includes('无刊例价');
  });

  // A negative price is refused client-side without a request.
  await page.locator('#modelList tr', { hasText: 'Beam' }).first().locator('.gear-btn').click();
  await page.locator('#priceDialog').waitFor({ state: 'visible' });
  await fillPriceField('#priceInput', '-4');
  await page.locator('#priceSave').click();
  await page.waitForFunction(() => document.querySelector('#priceError').textContent.trim().length > 0);
  await page.locator('#priceCancel').click();
  await page.locator('#priceDialog').waitFor({ state: 'hidden' });

  /* ------------------------- time-of-use schedule ------------------------- */
  const geminiRow = page.locator('#modelList tr', { hasText: 'gemini-3.8-flash-high' }).first();
  await geminiRow.locator('.gear-btn').click();
  await page.locator('#priceDialog').waitFor({ state: 'visible' });
  const prefilled = await page.evaluate(() => ({
    open: document.querySelector('#timePlan').open,
    source: document.querySelector('#timePlanSource').textContent.trim(),
    zone: document.querySelector('#timePlanZone').value,
    rows: document.querySelectorAll('#timeTierList .time-tier-row').length,
    name: document.querySelector('#timeTierList .time-name').value,
    days: document.querySelector('#timeTierList .time-days').value,
    start: document.querySelector('#timeTierList .time-start').value,
    end: document.querySelector('#timeTierList .time-end').value,
    input: document.querySelector('#timeTierList .time-input').value,
  }));
  if (!prefilled.open) fail('the schedule section stayed collapsed for a peak-priced model');
  if (prefilled.rows !== 1 || prefilled.name !== 'peak' || prefilled.days !== '1,2,3,4,5') {
    fail(`bundled schedule was not prefilled: ${JSON.stringify(prefilled)}`);
  }
  if (prefilled.start !== '01:00' || prefilled.end !== '04:00' || prefilled.input !== '1.5') {
    fail(`bundled schedule rates were not prefilled: ${JSON.stringify(prefilled)}`);
  }
  if (!prefilled.source.includes('内置峰谷计划')) fail(`schedule source note = ${prefilled.source}`);

  // A tier-only save keeps the catalog base rates and stores the schedule.
  await page.locator('#timeTierAdd').click();
  await page.waitForFunction(() => document.querySelectorAll('#timeTierList .time-tier-row').length === 2);
  const secondTier = page.locator('#timeTierList .time-tier-row').nth(1);
  await secondTier.locator('.time-name').fill('night');
  await secondTier.locator('.time-days').fill('1,2,3,4,5');
  await secondTier.locator('.time-start').fill('22:00');
  await secondTier.locator('.time-end').fill('23:00');
  await secondTier.locator('.time-input').fill('0.5');
  await submitPrice((request) => {
    if (request.method() !== 'PUT') return false;
    const candidate = new URL(request.url());
    if (candidate.pathname !== `${resourceBase}/model-prices`) return false;
    let payload;
    try {
      payload = JSON.parse(request.postData() || '{}');
    } catch (error) {
      return false;
    }
    const tiers = payload.price && payload.price.time_tiers;
    return payload.model === 'gemini-3.8-flash-high' && Array.isArray(tiers) && tiers.length === 2
      && tiers[1].name === 'night' && tiers[1].start === '22:00' && tiers[1].end === '23:00'
      && tiers[1].input === 0.5 && payload.price.time_zone === 'UTC';
  });
  await page.locator('#priceDialog').waitFor({ state: 'hidden' });
  await page.waitForFunction(() => {
    const row = Array.from(document.querySelectorAll('#modelList tr')).find((item) => item.textContent.includes('gemini-3.8-flash-high'));
    return !!row && row.textContent.includes('自定义');
  });

  // An overlapping schedule is refused in the browser and never reaches the host.
  await geminiRow.locator('.gear-btn').click();
  await page.locator('#priceDialog').waitFor({ state: 'visible' });
  await page.waitForFunction(() => document.querySelectorAll('#timeTierList .time-tier-row').length === 2);
  const overlapTier = page.locator('#timeTierList .time-tier-row').nth(1);
  await overlapTier.locator('.time-start').fill('02:00');
  await overlapTier.locator('.time-end').fill('05:00');
  const unexpectedPut = page.waitForRequest((request) => {
    if (request.method() !== 'PUT') return false;
    return new URL(request.url()).pathname === `${resourceBase}/model-prices`;
  }, { timeout: 1200 }).catch(() => null);
  await page.locator('#priceSave').click();
  await page.waitForFunction(() => document.querySelector('#priceError').textContent.trim().length > 0);
  const overlapError = await page.locator('#priceError').innerText();
  if (!overlapError.includes('重叠')) fail(`overlapping schedule was not refused: ${overlapError}`);
  if (await unexpectedPut) fail('an overlapping schedule was sent to the host');
  await page.locator('#priceCancel').click();
  await page.locator('#priceDialog').waitFor({ state: 'hidden' });

  /* --------------------------- manual refresh --------------------------- */
  const refreshCall = page.waitForRequest((request) => {
    const candidate = new URL(request.url());
    return candidate.pathname === `${resourceBase}/model-prices` && candidate.searchParams.get('refresh') === '1';
  });
  await page.locator('#refreshCatalog').click();
  await refreshCall;
  await page.waitForFunction(() => document.querySelector('#catalogNote').textContent.includes('内置快照'));
  await page.waitForTimeout(600);
  if (manualRefreshes !== 1) fail(`manual refresh requested ${manualRefreshes} times`);

  const overflow = await page.evaluate(() => ({ width: window.innerWidth, scroll: document.documentElement.scrollWidth }));
  if (overflow.scroll > overflow.width + 1) fail(`models tab scrolls horizontally: ${JSON.stringify(overflow)}`);
  if (errors.length) fail(`desktop page errors: ${errors.join('\n')}`);
  await context.close();

  /* ------------------- automatic refresh once per page ------------------- */
  stale = true;
  const staleContext = await browser.newContext({ locale: 'zh-CN', timezoneId: 'UTC', viewport: { width: 1300, height: 1000 } });
  await staleContext.addInitScript(({ token }) => {
    window.sessionStorage.setItem('cap-token-usage-tracker.session', JSON.stringify({
      token, expiresAt: new Date(Date.now() + 3600 * 1000).toISOString(), pluginID: 'cap-token-usage-tracker',
    }));
  }, { token: SESSION });
  const stalePage = await staleContext.newPage();
  const staleErrors = [];
  stalePage.on('pageerror', (error) => staleErrors.push(String(error)));
  let automaticRefreshes = 0;
  stalePage.on('request', (request) => {
    const candidate = new URL(request.url());
    if (candidate.pathname === `${resourceBase}/model-prices` && candidate.searchParams.get('refresh') === '1') automaticRefreshes += 1;
  });
  const automatic = stalePage.waitForRequest((request) => {
    const candidate = new URL(request.url());
    return candidate.pathname === `${resourceBase}/model-prices` && candidate.searchParams.get('refresh') === '1';
  });
  await stalePage.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
  await stalePage.locator('#totalTokens').waitFor({ state: 'visible' });
  await stalePage.locator('.tab[data-tab="models"]').click();
  await automatic;
  await stalePage.waitForTimeout(1000);
  if (automaticRefreshes !== 1) fail(`stale catalog triggered ${automaticRefreshes} refreshes`);
  if (staleErrors.length) fail(`stale page errors: ${staleErrors.join('\n')}`);
  await staleContext.close();
  stale = false;

  /* ------------------------------ mobile ------------------------------ */
  for (const width of [390, 700]) {
    const mobileContext = await browser.newContext({ locale: 'zh-CN', timezoneId: 'UTC', viewport: { width, height: 900 } });
    await mobileContext.addInitScript(({ token }) => {
      window.sessionStorage.setItem('cap-token-usage-tracker.session', JSON.stringify({
        token, expiresAt: new Date(Date.now() + 3600 * 1000).toISOString(), pluginID: 'cap-token-usage-tracker',
      }));
    }, { token: SESSION });
    const mobile = await mobileContext.newPage();
    const mobileErrors = [];
    mobile.on('pageerror', (error) => mobileErrors.push(String(error)));
    await mobile.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
    await mobile.locator('#totalTokens').waitFor({ state: 'visible' });
    await mobile.locator('.tab[data-tab="models"]').click();
    await mobile.locator('#panelModels').waitFor({ state: 'visible' });
    await mobile.waitForTimeout(250);
    const layout = await mobile.evaluate(() => ({
      innerWidth: window.innerWidth,
      scrollWidth: document.documentElement.scrollWidth,
      search: document.querySelector('#modelSearch').getBoundingClientRect().width,
      cost: document.querySelector('#modelsTotalCost').getBoundingClientRect().width,
    }));
    if (layout.scrollWidth > layout.innerWidth + 1) fail(`models tab scrolls horizontally at ${width}px: ${JSON.stringify(layout)}`);
    if (!(layout.search > 0) || !(layout.cost > 0)) fail(`models tab collapsed at ${width}px: ${JSON.stringify(layout)}`);
    if (mobileErrors.length) fail(`page errors at ${width}px: ${mobileErrors.join('\n')}`);
    await mobileContext.close();
  }

  if (unauthorizedHits < 1) fail('mock host never refused an unauthenticated data request');
} finally {
  await browser.close();
  server.close();
}
