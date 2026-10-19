package plugin

// dashboardHTMLTemplate is the single-page analytics dashboard served to the
// CLIProxyAPI management UI. It is embedded as one Go string so the binary ships
// without static assets; init() injects the locale catalog into the marker below.
//
// Design goals (DeepSeek mobile-web inspired):
//   - one usage tab, one request tab, one data tab
//   - large rounded cards with a single oversized number per metric
//   - in-card charts (stacked bars + gradient area) instead of a pie/donut
//   - model and API-key filters that scope every chart at once
const dashboardHTMLTemplate = `<!doctype html>
<html lang="zh-CN" data-theme="dark" style="background:#151412;color-scheme:dark">
<head>
<meta charset="utf-8">
<meta name="color-scheme" content="dark light">
<style id="initial-theme">
html{background:#151412;color-scheme:dark}
html:not([data-theme]){background:#faf9f5;color-scheme:light}
html[data-theme='white']{background:#fff;color-scheme:light}
html[data-theme='dark']{background:#151412;color-scheme:dark}
</style>
<script>
(function(){
'use strict';
var theme='dark',background='#151412';
try{if(window.parent!==window){var parentRoot=window.parent.document.documentElement,parentTheme=parentRoot.getAttribute('data-theme'),parentBackground=window.parent.getComputedStyle(parentRoot).getPropertyValue('--bg-secondary').trim();theme=parentTheme==='dark'||parentTheme==='white'?parentTheme:'light';if(parentBackground)background=parentBackground;}else{theme=window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'white';background=theme==='dark'?'#151412':'#fff';}}catch(_error){}
var root=document.documentElement;if(theme==='light')root.removeAttribute('data-theme');else root.setAttribute('data-theme',theme);root.style.backgroundColor=background;root.style.colorScheme=theme==='dark'?'dark':'light';
try{if(window.frameElement){window.frameElement.style.backgroundColor=background;if(window.frameElement.parentElement)window.frameElement.parentElement.style.backgroundColor=background;}}catch(_error){}
})();
</script>
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>Token Usage Analytics</title>
<style>
*{box-sizing:border-box}
[hidden]{display:none!important}
:root{
  color-scheme:light;
  --bg-secondary:#faf9f5;
  --page:#faf9f5;
  --card:#ffffff;
  --inset:#f2f0ea;
  --inset-strong:#e9e6df;
  --line:#e7e4dd;
  --line-strong:#d8d4cc;
  --t1:#26231f;
  --t2:#6d6760;
  --t3:#9a938b;
  --track:#e9e6df;
  --grid:rgba(38,35,31,.10);
  --seg-active:#26231f;
  --seg-active-text:#ffffff;
  --accent:#e8871f;
  --accent-soft:rgba(232,135,31,.13);
  --hit:#6ecdf6;
  --miss:#1ba6f2;
  --out:#0068e6;
  --req:#3f7fe0;
  --ok:#12a171;
  --bad:#d8503c;
  --shadow:0 1px 2px rgba(28,25,21,.05);
  --shadow-pop:0 18px 44px rgba(28,25,21,.18);
  --focus:rgba(63,127,224,.35);
  --tip-bg:#26231f;
  --tip-text:#f8f6f2;
  --tip-muted:#b6afa7;
}
[data-theme='white']{
  color-scheme:light;
  --bg-secondary:#fff;
  --page:#fff;
  --card:#ffffff;
  --inset:#f6f6f6;
  --inset-strong:#efefef;
  --line:#e8e8e8;
  --line-strong:#d9d9d9;
  --t1:#1d1b18;
  --t2:#6b6660;
  --t3:#9b958d;
  --track:#ebebeb;
  --grid:rgba(29,27,24,.10);
}
[data-theme='dark']{
  color-scheme:dark;
  --bg-secondary:#151412;
  --page:#151412;
  --card:#1e1c19;
  --inset:#272421;
  --inset-strong:#322e2a;
  --line:rgba(245,242,235,.075);
  --line-strong:rgba(245,242,235,.16);
  --t1:#f5f2eb;
  --t2:#a8a199;
  --t3:#7d766e;
  --track:rgba(245,242,235,.10);
  --grid:rgba(245,242,235,.08);
  --seg-active:#0f0e0c;
  --seg-active-text:#f7f4ee;
  --accent:#f4a23f;
  --accent-soft:rgba(244,162,63,.16);
  --hit:#8edeff;
  --miss:#3ab5ff;
  --out:#0072fb;
  --req:#5b9bf5;
  --ok:#34d399;
  --bad:#f87171;
  --shadow:none;
  --shadow-pop:0 20px 48px rgba(0,0,0,.55);
  --focus:rgba(124,192,248,.35);
  --tip-bg:#2b2824;
  --tip-text:#faf8f4;
  --tip-muted:#b8b1a8;
}
html,body{margin:0;padding:0}
body{background:var(--page);color:var(--t1);font-family:system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif;font-size:14px;line-height:1.5;-webkit-font-smoothing:antialiased;text-rendering:optimizeLegibility}
.page{max-width:1120px;margin:0 auto;padding:22px 22px 64px}
.topbar{display:flex;align-items:flex-start;justify-content:space-between;gap:14px;margin-bottom:16px}
.titles{min-width:0}
.title{margin:0;font-size:17px;font-weight:650;letter-spacing:-.01em}
.subtitle{margin:3px 0 0;display:flex;align-items:center;gap:6px;font-size:12px;color:var(--t3)}
.live-dot{width:6px;height:6px;border-radius:50%;background:var(--ok);box-shadow:0 0 0 3px color-mix(in srgb,var(--ok) 16%,transparent)}
.topbar-actions{display:flex;align-items:center;gap:8px;flex:0 0 auto}
.ghost-btn{height:36px;padding:0 16px;border-radius:999px;border:1px solid transparent;background:var(--inset);color:var(--t1);font:600 13px/1 inherit;cursor:pointer;transition:background .15s,border-color .15s}
.ghost-btn:hover{background:var(--inset-strong)}
.icon-btn{width:36px;height:36px;display:inline-flex;align-items:center;justify-content:center;border-radius:999px;border:1px solid transparent;background:var(--inset);color:var(--t1);cursor:pointer;transition:background .15s}
.icon-btn:hover{background:var(--inset-strong)}
.icon-btn svg{width:16px;height:16px;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}
.icon-btn.is-busy svg{animation:spin 1s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
@media(prefers-reduced-motion:reduce){.icon-btn.is-busy svg{animation:none}}

/* Tabs */
.tabs{display:inline-flex;gap:2px;padding:4px;border-radius:14px;background:var(--inset);margin-bottom:16px;max-width:100%;overflow-x:auto;scrollbar-width:none}
.tabs::-webkit-scrollbar{display:none}
.tab{border:0;background:transparent;color:var(--t2);font:600 13px/1 inherit;padding:9px 18px;border-radius:11px;cursor:pointer;white-space:nowrap;transition:background .15s,color .15s}
.tab:hover{color:var(--t1)}
.tab.is-active{background:var(--card);color:var(--t1);box-shadow:var(--shadow)}
.panel{display:none}
.panel.is-active{display:block;animation:fade .18s ease}
@keyframes fade{from{opacity:0;transform:translateY(2px)}to{opacity:1;transform:none}}
@media(prefers-reduced-motion:reduce){.panel.is-active{animation:none}}

/* Filter chips */
.filterbar{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:14px}
.chip{display:inline-flex;align-items:center;gap:9px;height:38px;padding:0 14px;border-radius:999px;border:1px solid transparent;background:var(--inset);color:var(--t1);font:500 13px/1 inherit;cursor:pointer;transition:background .15s,border-color .15s;max-width:100%}
.chip:hover{background:var(--inset-strong)}
.chip.is-active{border-color:color-mix(in srgb,var(--accent) 55%,transparent);background:var(--accent-soft)}
.chip-tag{color:var(--t2);font-weight:500}
.chip-sep{width:1px;height:11px;background:var(--t3);opacity:.45}
.chip-value{font-weight:600;max-width:190px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.chip-caret{width:11px;height:11px;flex:0 0 auto;color:var(--t3)}
.chip-count{min-width:18px;height:18px;padding:0 5px;border-radius:999px;background:var(--accent);color:#1b1409;font:700 10px/18px inherit;text-align:center}
.chip-note{font-size:11.5px;color:var(--t3);white-space:nowrap;align-self:center}
.chip-note.scale-clamped{white-space:normal;max-width:240px;text-align:right}
.filterbar-tight{margin-bottom:12px}

/* Cards */
.card{background:var(--card);border:1px solid var(--line);border-radius:20px;padding:18px;margin-bottom:12px;box-shadow:var(--shadow)}
.card-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}
.card-label{font-size:13px;font-weight:500;color:var(--t2)}
.card-title{font-size:15px;font-weight:650;letter-spacing:-.01em}
.card-sub{margin-top:2px;font-size:12px;color:var(--t3)}
.hero-value{margin:8px 0 16px;font-size:38px;line-height:1.05;font-weight:700;letter-spacing:-.035em;font-variant-numeric:tabular-nums;word-break:break-word}
.stack{display:flex;height:10px;border-radius:999px;background:var(--track);overflow:hidden;margin-bottom:14px}
.stack i{display:block;height:100%;transition:width .45s ease}
.legend{display:flex;flex-wrap:wrap;gap:10px 18px}
.legend-item{display:inline-flex;align-items:center;gap:7px;font-size:12px;color:var(--t2);min-width:0}
.legend-dot{width:9px;height:9px;border-radius:3px;flex:0 0 auto}
.legend-item b{color:var(--t1);font-weight:600;font-variant-numeric:tabular-nums}

.stat-grid{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px}
.stat{padding:18px;margin:0}
.stat-value{margin:8px 0 2px;font-size:30px;line-height:1.1;font-weight:700;letter-spacing:-.03em;font-variant-numeric:tabular-nums}
.stat-sub{font-size:12px;color:var(--t3);min-height:18px}
.mini-bar{height:4px;border-radius:999px;background:var(--track);overflow:hidden;margin-top:12px}
.mini-bar i{display:block;height:100%;border-radius:999px;transition:width .45s ease}
.cost-card{grid-column:1/-1}

/* Models tab */
.search-field{display:inline-flex;align-items:center;gap:8px;height:38px;padding:0 13px;border-radius:999px;background:var(--inset);border:1px solid transparent;min-width:220px;flex:1 1 220px;max-width:340px}
.search-field:focus-within{border-color:var(--accent)}
.search-icon{width:14px;height:14px;flex:0 0 auto;color:var(--t3);fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round}
.search-field input{flex:1;min-width:0;border:0;background:transparent;color:var(--t1);font:inherit;font-size:13px}
.search-field input:focus{outline:none}
.model-cell{display:flex;align-items:center;gap:8px;min-width:0}
.model-cell .model-text{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.badge{display:inline-flex;align-items:center;height:18px;padding:0 7px;border-radius:999px;font:600 10.5px/1 inherit;white-space:nowrap;flex:0 0 auto}
.badge-catalog{background:var(--inset-strong);color:var(--t2)}
.badge-override{background:var(--accent-soft);color:var(--accent)}
.badge-free{background:color-mix(in srgb,var(--ok) 18%,transparent);color:var(--ok)}
.badge-none{background:color-mix(in srgb,var(--bad) 15%,transparent);color:var(--bad)}
.provider-tag{display:inline-block;max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;color:var(--t3)}
.model-price{font-size:11.5px;color:var(--t3);font-variant-numeric:tabular-nums;white-space:nowrap}
.badge-peak{background:var(--accent-soft);color:var(--accent)}
.price-cell{display:inline-flex;align-items:center;gap:6px;justify-content:flex-end}
.cost-note{margin:10px 0 0;font-size:12px;color:var(--t3)}
#priceDialog{width:min(720px,calc(100vw - 28px));max-height:calc(100vh - 40px);overflow:auto}
.time-plan{margin:0 0 16px;padding:10px 12px;border:1px solid var(--line);border-radius:12px;background:var(--inset)}
.time-plan>summary{cursor:pointer;font-size:12.5px;font-weight:600;color:var(--t2)}
.time-plan .field{margin-top:10px}
.time-plan .field input{max-width:220px}
.time-tier-list{display:flex;flex-direction:column;gap:10px;margin:10px 0}
.time-tier-row{padding:9px 10px;border:1px solid var(--line);border-radius:10px;background:var(--card)}
.time-tier-head{display:flex;align-items:center;gap:8px;margin-bottom:8px}
.time-tier-head .time-name{flex:1;height:32px;padding:0 9px;border-radius:9px;border:1px solid var(--line);background:var(--card);color:var(--t1);font:inherit;font-size:12.5px}
.time-tier-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px}
.time-tier-grid label{display:block;font-size:11px;color:var(--t3)}
.time-tier-grid input{width:100%;height:32px;padding:0 8px;border-radius:9px;border:1px solid var(--line);background:var(--card);color:var(--t1);font:inherit;font-size:12.5px;color-scheme:inherit}
.time-tier-grid input:focus{outline:none;border-color:var(--accent)}
.time-remove{height:28px;width:28px;border-radius:8px;border:1px solid var(--line);background:transparent;color:var(--t3);cursor:pointer;font:600 14px/1 inherit}
.time-remove:hover{border-color:var(--line-strong);color:var(--t1)}
.model-price b{color:var(--t2);font-weight:600}
.gear-btn{width:28px;height:28px;padding:0;border-radius:9px;border:1px solid var(--line);background:transparent;color:var(--t2);cursor:pointer;display:inline-flex;align-items:center;justify-content:center;transition:background .15s,color .15s}
.gear-btn:hover{background:var(--inset);color:var(--t1)}
.gear-btn:focus-visible{outline:2px solid var(--focus);outline-offset:1px}
.gear-btn svg{width:14px;height:14px;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}
.row-muted{opacity:.62}
.catalog-note{margin:12px 0 0;font-size:11.5px;color:var(--t3)}
.field-hint{margin:-6px 0 12px;font-size:11.5px;color:var(--t3)}
.rank-cost{margin-left:7px;font-size:11.5px;font-weight:600;color:var(--accent);font-variant-numeric:tabular-nums;white-space:nowrap}

/* Charts */
.chart-card{padding:18px}
.chart-card .card-head{margin-bottom:10px}
.chart-host{position:relative;height:210px;margin-top:2px}
.chart-host{user-select:none;-webkit-user-select:none;-webkit-touch-callout:none}
.chart-host svg{display:block;width:100%;height:100%;overflow:visible;touch-action:pan-y}
.chart-legend{display:flex;flex-wrap:wrap;gap:14px;margin-top:10px;font-size:11.5px;color:var(--t3)}
.chart-legend span{display:inline-flex;align-items:center;gap:6px}
.chart-legend i{width:8px;height:8px;border-radius:2px;flex:0 0 auto}
.chart-legend span b{color:var(--t1);font-weight:600;font-variant-numeric:tabular-nums}
.chart-legend span em{font-style:normal;max-width:170px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.chart-meta{display:flex;align-items:center;gap:10px;flex-wrap:wrap;justify-content:flex-end}
.chart-total{margin-left:8px;font-size:12.5px;font-weight:600;color:var(--t2);font-variant-numeric:tabular-nums;letter-spacing:0}
.chart-empty{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;color:var(--t3);font-size:13px}
.segmented{display:inline-flex;padding:3px;border-radius:999px;background:var(--inset);flex:0 0 auto}
.segmented button{border:0;background:transparent;color:var(--t2);font:600 12px/1 inherit;padding:7px 13px;border-radius:999px;cursor:pointer;transition:background .15s,color .15s}
.segmented button.is-active{background:var(--seg-active);color:var(--seg-active-text)}
.tip{position:absolute;z-index:6;min-width:206px;max-width:min(300px,88vw);padding:13px 15px;border-radius:16px;background:var(--tip-bg);color:var(--tip-text);box-shadow:0 16px 40px rgba(0,0,0,.35);pointer-events:none;opacity:0;transform:translateY(3px);transition:opacity .12s ease,transform .12s ease}
.tip.is-visible{opacity:1;transform:none}
.tip-head{display:flex;align-items:baseline;justify-content:space-between;gap:18px;font-size:15px;font-weight:700;font-variant-numeric:tabular-nums;margin-bottom:7px}
.tip-head span:first-child{font-weight:600;color:var(--tip-muted);font-size:13px}
.tip-row{display:flex;align-items:center;gap:9px;margin-top:5px;font-size:12.5px}
.tip-row i{width:9px;height:9px;border-radius:2px;flex:0 0 auto}
.tip-row span{flex:1;min-width:0;color:var(--tip-muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.tip-row b{font-weight:650;font-variant-numeric:tabular-nums}

/* Rankings */
.rank-list{display:flex;flex-direction:column;gap:7px;margin-top:14px}
.rank-row{position:relative;display:flex;align-items:center;gap:10px;width:100%;padding:11px 13px;border:1px solid transparent;border-radius:13px;background:var(--inset);color:inherit;font:inherit;text-align:left;cursor:pointer;overflow:hidden;transition:border-color .15s}
.rank-row:hover{border-color:var(--line-strong)}
.rank-row:focus-visible{outline:2px solid var(--focus);outline-offset:2px}
.rank-fill{position:absolute;left:0;top:0;bottom:0;pointer-events:none;transition:width .45s ease}
.rank-main{position:relative;display:flex;align-items:center;gap:9px;flex:1;min-width:0}
.rank-name{font-size:13px;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.rank-meta{font-size:11.5px;color:var(--t3);white-space:nowrap}
.rank-val{position:relative;font-size:13px;font-weight:650;font-variant-numeric:tabular-nums;white-space:nowrap}
.rank-key-dot{width:9px;height:9px;border-radius:3px;flex:0 0 auto}
.rank-empty{padding:22px 4px;color:var(--t3);font-size:13px;text-align:center}

/* Request log */
.log-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:12px}
.table-wrap{overflow-x:auto;margin:0 -4px}
table{border-collapse:collapse;width:100%;font-size:12.5px}
th{text-align:left;padding:8px 10px;color:var(--t2);font-weight:600;white-space:nowrap;border-bottom:1px solid var(--line)}
td{padding:10px;border-bottom:1px solid var(--line);white-space:nowrap;font-variant-numeric:tabular-nums}
tbody tr:last-child td{border-bottom:0}
th.num,td.num{text-align:right}
.log-model{max-width:220px;overflow:hidden;text-overflow:ellipsis}
.status{display:inline-flex;align-items:center;gap:6px;font-size:12px}
.status i{width:7px;height:7px;border-radius:50%;flex:0 0 auto}
.status.ok{color:var(--ok)}
.status.bad{color:var(--bad)}
/* The request log carries fourteen columns; tighten it and pin the result
   cell so the status never scrolls out of view. */
#panelRequests table{font-size:12.5px}
#panelRequests th{padding:8px 6px}
#panelRequests td{padding:9px 6px}
#panelRequests .log-model{max-width:150px}
#panelRequests th:last-child,#panelRequests td:last-child{position:sticky;right:0;background:var(--card)}
#panelRequests th:last-child{box-shadow:-9px 0 9px -9px rgba(0,0,0,.5)}
#panelRequests td:last-child{box-shadow:-9px 0 9px -9px rgba(0,0,0,.5)}
.pager{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-top:14px;font-size:12px;color:var(--t3)}
.pager-actions{display:flex;gap:8px}
.pager button{height:32px;padding:0 13px;border-radius:999px;border:1px solid var(--line);background:var(--card);color:var(--t1);font:600 12px/1 inherit;cursor:pointer}
.pager button:hover:not(:disabled){border-color:var(--line-strong)}
.pager button:disabled{opacity:.4;cursor:not-allowed}

/* Data tab */
.data-card .card-title{margin-bottom:4px}
.data-hint{font-size:12px;color:var(--t3);margin:0 0 14px;line-height:1.6}
.btn-row{display:flex;flex-wrap:wrap;gap:8px}
.solid-btn{height:36px;padding:0 16px;border-radius:999px;border:0;background:var(--t1);color:var(--card);font:600 13px/1 inherit;cursor:pointer}
.solid-btn:hover{opacity:.9}
.danger-btn{height:36px;padding:0 16px;border-radius:999px;border:1px solid color-mix(in srgb,var(--bad) 45%,transparent);background:color-mix(in srgb,var(--bad) 12%,transparent);color:var(--bad);font:600 13px/1 inherit;cursor:pointer}
.key-input{width:100%;max-width:420px;height:38px;padding:0 13px;border-radius:11px;border:1px solid var(--line);background:var(--card);color:var(--t1);font:inherit;font-size:13px}
.key-input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 3px var(--focus)}

/* Dropdown surfaces */
.dropdown-surface{position:fixed;z-index:900;border:1px solid var(--line);border-radius:16px;background:var(--card);box-shadow:var(--shadow-pop);opacity:0;pointer-events:none;transform:translateY(-6px) scale(.985);transition:opacity 150ms ease,transform 150ms ease}
.dropdown-surface.is-open{opacity:1;pointer-events:auto;transform:translateY(0) scale(1)}
.dropdown-surface.is-closing{transition-duration:120ms}
.menu-list{width:min(320px,calc(100vw - 24px));max-height:min(420px,60vh);overflow:auto;padding:8px}
.menu-search{width:100%;height:34px;padding:0 11px;margin-bottom:6px;border-radius:9px;border:1px solid var(--line);background:var(--inset);color:var(--t1);font:inherit;font-size:12.5px}
.menu-search:focus{outline:none;border-color:var(--accent)}
.menu-option{display:flex;align-items:center;gap:9px;padding:9px 10px;border-radius:10px;cursor:pointer;font-size:12.5px;color:var(--t1)}
.menu-option:hover{background:var(--inset)}
.menu-option.is-all{border-bottom:1px solid var(--line);border-radius:10px 10px 0 0;margin-bottom:4px;font-weight:600}
.menu-option input{margin:0;accent-color:var(--accent);flex:0 0 auto}
.menu-option span{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.menu-option em{font-style:normal;color:var(--t3);font-size:11.5px;font-variant-numeric:tabular-nums}
.menu-empty{padding:14px 10px;color:var(--t3);font-size:12.5px}

/* Dialogs */
dialog{border:1px solid var(--line);border-radius:20px;background:var(--card);color:var(--t1);padding:20px;box-shadow:var(--shadow-pop);width:min(440px,calc(100vw - 28px))}
dialog::backdrop{background:rgba(0,0,0,.45);backdrop-filter:blur(3px)}
.dialog-title{font-size:16px;font-weight:650;margin:0 0 4px}
.dialog-sub{font-size:12px;color:var(--t3);margin:0 0 16px}
.preset-list{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-bottom:16px}
.preset{height:40px;border-radius:11px;border:1px solid var(--line);background:var(--inset);color:var(--t1);font:600 13px/1 inherit;cursor:pointer}
.preset:hover{border-color:var(--line-strong)}
.preset.is-active{background:var(--accent-soft);border-color:color-mix(in srgb,var(--accent) 55%,transparent)}
.field-row{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:16px}
.field label{display:block;font-size:11.5px;color:var(--t3);margin-bottom:5px}
.field input{width:100%;height:38px;padding:0 10px;border-radius:10px;border:1px solid var(--line);background:var(--card);color:var(--t1);font:inherit;font-size:12.5px;color-scheme:inherit}
.field input:focus{outline:none;border-color:var(--accent)}
.dialog-foot{display:flex;justify-content:flex-end;gap:8px}
.primary-btn{height:36px;padding:0 18px;border-radius:999px;border:0;background:var(--t1);color:var(--card);font:600 13px/1 inherit;cursor:pointer}
.soft-btn{height:36px;padding:0 16px;border-radius:999px;border:1px solid var(--line);background:transparent;color:var(--t1);font:600 13px/1 inherit;cursor:pointer}
.error-text{color:var(--bad);font-size:12.5px;margin:0 0 12px;min-height:0}
.toast{position:fixed;left:50%;bottom:24px;transform:translate(-50%,8px);z-index:1000;background:var(--tip-bg);color:var(--tip-text);padding:11px 18px;border-radius:999px;font-size:13px;box-shadow:0 16px 40px rgba(0,0,0,.35);opacity:0;pointer-events:none;transition:opacity .18s,transform .18s}
.toast.is-visible{opacity:1;transform:translate(-50%,0)}

/* Access gate */
body.is-locked .page{display:none}
.lock-screen{position:fixed;inset:0;z-index:960;display:flex;align-items:center;justify-content:center;padding:20px;background:var(--page)}
.lock-card{width:min(420px,100%);padding:30px 26px 24px;border:1px solid var(--line);border-radius:24px;background:var(--card);box-shadow:var(--shadow-pop)}
.lock-badge{width:44px;height:44px;display:flex;align-items:center;justify-content:center;border-radius:14px;background:var(--accent-soft);color:var(--accent);margin-bottom:16px}
.lock-badge svg{width:22px;height:22px;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}
.lock-title{margin:0 0 6px;font-size:18px;font-weight:650;letter-spacing:-.01em}
.lock-sub{margin:0 0 18px;font-size:12.5px;line-height:1.65;color:var(--t2)}
.lock-input{width:100%;height:42px;padding:0 14px;border-radius:12px;border:1px solid var(--line-strong);background:var(--inset);color:var(--t1);font:inherit;font-size:13.5px}
.lock-input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 3px var(--focus);background:var(--card)}
.lock-error{margin:10px 0 0;font-size:12px;color:var(--bad);min-height:16px}
.lock-submit{width:100%;height:42px;margin-top:8px;border-radius:999px;font-size:13.5px}
.lock-foot{margin:16px 0 0;font-size:11.5px;line-height:1.6;color:var(--t3)}
.topbar-actions .lock-btn{display:none}
body.is-unlocked .topbar-actions .lock-btn{display:inline-flex}

@media(min-width:769px){
  .page{padding:26px 26px 72px}
  .card{padding:22px}
  .chart-card{padding:22px}
  .hero-value{font-size:46px}
  .chart-host{height:260px}
  .stat-value{font-size:34px}
  #panelCost .stat-grid{grid-template-columns:repeat(3,minmax(0,1fr))}
  .rank-grid{display:grid;grid-template-columns:1fr 1fr;gap:12px;align-items:start}
  .rank-grid .card{margin-bottom:0}
}
@media(max-width:768px){
  .page{padding:14px 14px 48px}
  .title{font-size:15px}
  .tabs{display:flex;width:100%}
  .tab{flex:1;text-align:center;padding:9px 6px}
  .filterbar{gap:7px;margin-bottom:12px}
  .chip{height:36px;padding:0 12px;gap:7px;font-size:12.5px;flex:0 1 auto;min-width:0}
  .chip-value{max-width:132px}
  .hero-value{font-size:34px}
  .chart-host{height:190px}
  .stat-value{font-size:26px}
  .stat-grid{gap:10px}
  #panelCost .stat-grid .stat:last-child{grid-column:1/-1}
  table{font-size:12px}
  th,td{padding:9px 7px;white-space:nowrap}
  th:first-child,td:first-child{padding-left:2px}
  th:last-child,td:last-child{padding-right:2px}
  .log-model{max-width:118px}
  .status{gap:5px}
  .table-wrap{-webkit-overflow-scrolling:touch}
  .chip-tag{font-size:11.5px}
  .chip-value{max-width:104px}
}
@media(max-width:520px){
  .chart-card .card-head{flex-wrap:wrap;gap:8px}
  #scaleControl,#costDimensionControl{width:100%;justify-content:space-between}
  #scaleControl button,#costDimensionControl button{flex:1;padding:7px 8px}
  .chart-meta{width:100%}
  .chip-note.scale-clamped{max-width:none;text-align:left}
  .time-tier-grid{grid-template-columns:repeat(2,minmax(0,1fr))}
}
@media(max-width:380px){
  .search-field{min-width:0;max-width:none}
  .provider-tag{max-width:104px}
  .chip{padding:0 10px;gap:5px}
  .chip-value{max-width:86px}
  .chip-sep{display:none}
}
</style>
</head>
<body class="is-locked">
<div id="lockScreen" class="lock-screen">
  <form class="lock-card" id="unlockForm" autocomplete="off">
    <div class="lock-badge"><svg viewBox="0 0 24 24"><rect x="4" y="10" width="16" height="11" rx="2.5"></rect><path d="M8 10V7a4 4 0 0 1 8 0v3"></path></svg></div>
    <h2 class="lock-title" data-i18n="lock.title">需要管理密钥</h2>
    <p class="lock-sub" data-i18n="lock.subtitle">该仪表盘只对已登录 CLIProxyAPI 的用户开放。请输入管理密钥解锁；密钥仅用于换取本次会话，不会被插件保存。</p>
    <input id="unlockKey" class="lock-input" type="password" autocomplete="off" spellcheck="false" data-i18n-placeholder="lock.placeholder" placeholder="Management Key">
    <p class="lock-error" id="unlockError" role="alert"></p>
    <button id="unlockButton" class="primary-btn lock-submit" type="submit" data-i18n="lock.unlock">解锁</button>
    <p class="lock-foot" data-i18n="lock.footnote">插件只保存令牌的 SHA-256 摘要；解锁状态仅存于本浏览器，点击右上角可随时锁定。</p>
  </form>
</div>
<div class="page">
  <header class="topbar">
    <div class="titles">
      <h1 class="title" data-i18n="app.title">Token 用量统计</h1>
      <p class="subtitle"><span class="live-dot"></span><span id="updatedAt">—</span></p>
    </div>
    <div class="topbar-actions">
      <button id="lockButton" class="icon-btn lock-btn" type="button" title="锁定" data-i18n-title="lock.action" data-i18n-aria="lock.action">
        <svg viewBox="0 0 24 24"><rect x="4" y="10" width="16" height="11" rx="2.5"></rect><path d="M8 10V7a4 4 0 0 1 8 0v3"></path></svg>
      </button>
      <button id="exportButton" class="ghost-btn" type="button" data-i18n="button.export">导出</button>
      <button id="refreshButton" class="icon-btn" type="button" title="刷新" data-i18n-title="button.refresh" data-i18n-aria="button.refresh">
        <svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"></path><path d="M3 3v5h5"></path><path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"></path><path d="M16 16h5v5"></path></svg>
      </button>
    </div>
  </header>

  <nav class="tabs" role="tablist">
    <button class="tab is-active" type="button" role="tab" data-tab="usage" data-i18n="tab.usage">Token</button>
    <button class="tab" type="button" role="tab" data-tab="cost" data-i18n="tab.cost">花销</button>
    <button class="tab" type="button" role="tab" data-tab="models" data-i18n="tab.models">模型</button>
    <button class="tab" type="button" role="tab" data-tab="requests" data-i18n="tab.requests">请求日志</button>
    <button class="tab" type="button" role="tab" data-tab="data" data-i18n="tab.data">数据管理</button>
  </nav>

  <div id="error" class="error-text" role="alert"></div>

  <div class="filterbar" id="globalFilterBar">
      <button id="rangeButton" class="chip" type="button" aria-haspopup="dialog">
        <span class="chip-tag" data-i18n="filter.timeDimension">时间维度</span>
        <span class="chip-sep"></span>
        <span class="chip-value" id="rangeValue">近30天</span>
        <svg class="chip-caret" viewBox="0 0 12 12"><path d="M2.5 4.5 6 8l3.5-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </button>
      <button id="modelButton" class="chip" type="button" aria-haspopup="menu">
        <span class="chip-tag" data-i18n="filter.model">模型</span>
        <span class="chip-sep"></span>
        <span class="chip-value" id="modelValue" data-i18n="filter.all">全部</span>
        <svg class="chip-caret" viewBox="0 0 12 12"><path d="M2.5 4.5 6 8l3.5-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </button>
      <button id="apiKeyButton" class="chip" type="button" aria-haspopup="menu">
        <span class="chip-tag">API Key</span>
        <span class="chip-sep"></span>
        <span class="chip-value" id="apiKeyValue" data-i18n="filter.all">全部</span>
        <svg class="chip-caret" viewBox="0 0 12 12"><path d="M2.5 4.5 6 8l3.5-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </button>
      <div id="modelMenu" class="dropdown-surface menu-list" role="menu" hidden></div>
      <div id="apiKeyMenu" class="dropdown-surface menu-list" role="menu" hidden></div>
    </div>

  <section class="panel is-active" id="panelUsage" role="tabpanel">
    <article class="card">
      <div class="card-head">
        <span class="card-label" data-i18n="card.totalTokens">总消耗 Token</span>
        <span class="card-sub" id="rangeNote"></span>
      </div>
      <div class="hero-value" id="totalTokens">0</div>
      <div class="stack" id="tokenStack"></div>
      <div class="legend" id="tokenLegend"></div>
    </article>

    <div class="stat-grid">
      <article class="card stat">
        <div class="card-label" data-i18n="card.requests">API 请求次数</div>
        <div class="stat-value" id="totalRequests">0</div>
        <div class="stat-sub" id="requestSub"></div>
        <div class="mini-bar"><i id="successFill" style="width:0;background:var(--ok)"></i></div>
      </article>
      <article class="card stat">
        <div class="card-label" data-i18n="trend.cacheHitRate">缓存命中率</div>
        <div class="stat-value" id="cacheHitRate">0%</div>
        <div class="stat-sub" id="cacheSub"></div>
        <div class="mini-bar"><i id="cacheFill" style="width:0;background:var(--hit)"></i></div>
      </article>
    </div>

    <article class="card chart-card">
      <div class="card-head">
        <div>
          <div class="card-title"><span data-i18n="chart.tokensTrend">Tokens 消耗</span><span class="chart-total" id="tokenTrendTotal"></span></div>
          <div class="card-sub" id="tokenTrendSub"></div>
        </div>
        <div class="chart-meta">
          <div class="segmented" id="scaleControl" role="group">
            <button type="button" data-scale="auto" class="is-active" data-i18n="scale.auto">自动</button>
            <button type="button" data-scale="minute" data-i18n="scale.minute">分钟</button>
            <button type="button" data-scale="hour" data-i18n="scale.hour">小时</button>
            <button type="button" data-scale="day" data-i18n="scale.day">天</button>
          </div>
          <span class="chip-note" id="scaleHint" hidden></span>
        </div>
      </div>
      <div class="chart-host" id="tokenChartHost">
        <svg id="tokenChart" role="img" aria-label="Token trend"></svg>
        <div class="tip" id="tokenTip" hidden></div>
      </div>
      <div class="chart-legend" id="tokenChartLegend" hidden></div>
    </article>

    <article class="card chart-card">
      <div class="card-head">
        <div>
          <div class="card-title"><span data-i18n="chart.requestsTrend">API 请求次数</span><span class="chart-total" id="requestTrendTotal"></span></div>
          <div class="card-sub" id="requestTrendSub"></div>
        </div>
        <span class="chip-note" id="requestScaleNote"></span>
      </div>
      <div class="chart-host" id="requestChartHost">
        <svg id="requestChart" role="img" aria-label="Request trend"></svg>
        <div class="tip" id="requestTip" hidden></div>
      </div>
    </article>

    <div class="rank-grid">
      <article class="card">
        <div class="card-head">
          <div>
            <div class="card-title" data-i18n="ranking.models">模型用量</div>
            <div class="card-sub" id="modelRankSub"></div>
          </div>
          <div class="segmented" id="rankControl" role="group">
            <button type="button" data-rank="tokens" class="is-active">Tokens</button>
            <button type="button" data-rank="requests" data-i18n="chart.requests">请求</button>
          </div>
        </div>
        <div class="rank-list" id="modelRank"></div>
      </article>
      <article class="card">
        <div class="card-head">
          <div>
            <div class="card-title" data-i18n="ranking.apiKeys">API Key 用量</div>
            <div class="card-sub" id="apiKeyRankSub"></div>
          </div>
        </div>
        <div class="rank-list" id="apiKeyRank"></div>
      </article>
    </div>
  </section>

  <section class="panel" id="panelCost" role="tabpanel">
    <article class="card">
      <div class="card-head">
        <span class="card-label" data-i18n="cost.total">预估费用</span>
        <span class="card-sub" id="costRangeNote"></span>
      </div>
      <div class="hero-value" id="costTotal">$0</div>
      <div class="stack" id="costStack"></div>
      <div class="legend" id="costLegend"></div>
      <p class="cost-note" id="costTimeTierNote" hidden></p>
    </article>

    <div class="stat-grid">
      <article class="card stat">
        <div class="card-label" data-i18n="cost.pricedModels">已定价模型</div>
        <div class="stat-value" id="costPricedModels">0</div>
        <div class="stat-sub" id="costPricedSub"></div>
        <div class="mini-bar"><i id="costPricedFill" style="width:0;background:var(--ok)"></i></div>
      </article>
      <article class="card stat">
        <div class="card-label" data-i18n="cost.unpricedModels">待补价模型</div>
        <div class="stat-value" id="costUnpricedModels">0</div>
        <div class="stat-sub" id="costUnpricedSub"></div>
        <div class="mini-bar"><i id="costUnpricedFill" style="width:0;background:var(--miss)"></i></div>
      </article>
      <article class="card stat">
        <div class="card-label" data-i18n="cost.coverage">请求计价覆盖率</div>
        <div class="stat-value" id="costCoverage">0%</div>
        <div class="stat-sub" id="costCoverageSub"></div>
        <div class="mini-bar"><i id="costCoverageFill" style="width:0;background:var(--accent)"></i></div>
      </article>
    </div>

    <article class="card chart-card">
      <div class="card-head">
        <div>
          <div class="card-title"><span data-i18n="cost.trend">花销趋势</span><span class="chart-total" id="costTrendTotal"></span></div>
          <div class="card-sub" id="costTrendSub"></div>
        </div>
        <div class="chart-meta">
          <div class="segmented" id="costDimensionControl" role="group">
            <button type="button" data-cost-dimension="model" class="is-active" data-i18n="cost.groupByModel">按模型</button>
            <button type="button" data-cost-dimension="key" data-i18n="cost.groupByKey">按 API Key</button>
          </div>
          <span class="chip-note" id="costScaleNote"></span>
        </div>
      </div>
      <div class="chart-host" id="costChartHost">
        <svg id="costChart" role="img" aria-label="Cost trend"></svg>
        <div class="tip" id="costTip" hidden></div>
      </div>
      <div class="chart-legend" id="costChartLegend" hidden></div>
    </article>

    <div class="rank-grid">
      <article class="card">
        <div class="card-head">
          <div>
            <div class="card-title" data-i18n="cost.byModel">模型花销</div>
            <div class="card-sub" id="costModelRankSub"></div>
          </div>
        </div>
        <div class="rank-list" id="costModelRank"></div>
      </article>
      <article class="card">
        <div class="card-head">
          <div>
            <div class="card-title" data-i18n="cost.byKey">API Key 花销</div>
            <div class="card-sub" id="costApiKeyRankSub"></div>
          </div>
        </div>
        <div class="rank-list" id="costApiKeyRank"></div>
      </article>
    </div>
  </section>

  <section class="panel" id="panelModels" role="tabpanel">
    <div class="filterbar filterbar-tight">
      <label class="search-field" for="modelSearch">
        <svg class="search-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="11" cy="11" r="7"></circle><path d="m20 20-3.5-3.5"></path></svg>
        <input id="modelSearch" type="search" autocomplete="off" spellcheck="false" placeholder="搜索模型" data-i18n-placeholder="models.search" data-i18n-aria="models.search" aria-label="搜索模型">
      </label>
    </div>

    <div class="stat-grid">
      <article class="card stat">
        <div class="card-label" data-i18n="models.totalCost">预估费用</div>
        <div class="stat-value" id="modelsTotalCost">$0</div>
        <div class="stat-sub" id="modelsCostSub"></div>
        <div class="mini-bar"><i id="modelsCostFill" style="width:0;background:var(--accent)"></i></div>
      </article>
      <article class="card stat">
        <div class="card-label" data-i18n="models.count">模型数量</div>
        <div class="stat-value" id="modelsCount">0</div>
        <div class="stat-sub" id="modelsCountSub"></div>
        <div class="mini-bar"><i id="modelsCountFill" style="width:0;background:var(--miss)"></i></div>
      </article>
    </div>

    <article class="card">
      <div class="log-head">
        <div>
          <div class="card-title" data-i18n="models.listTitle">模型速度与价格</div>
          <div class="card-sub" id="modelsSub"></div>
        </div>
        <button id="refreshCatalog" class="soft-btn" type="button" data-i18n="models.refreshCatalog">刷新价格</button>
      </div>
      <div class="table-wrap">
        <table>
          <thead><tr>
            <th data-i18n="table.model">模型名称</th>
            <th class="col-key" data-i18n="models.provider">渠道</th>
            <th class="num" data-i18n="models.generationTps">生成 TPS</th>
            <th class="num" data-i18n="models.endToEndTps">端到端 TPS</th>
            <th class="num" data-i18n="models.avgTtft">平均 TTFT</th>
            <th class="num" data-i18n="models.tokens">Token 用量</th>
            <th class="num" data-i18n="models.cost">费用</th>
            <th data-i18n="models.price">价格</th>
            <th class="col-action" data-i18n="models.action">操作</th>
          </tr></thead>
          <tbody id="modelList"></tbody>
        </table>
      </div>
      <p class="catalog-note" id="catalogNote"></p>
      <p class="catalog-note" id="ttftNote" hidden></p>
    </article>
  </section>

  <section class="panel" id="panelRequests" role="tabpanel">
    <article class="card">
      <div class="log-head">
        <div>
          <div class="card-title" data-i18n="tab.requests">请求日志</div>
          <div class="card-sub" id="requestLogSub"></div>
        </div>
        <div class="segmented" id="resultControl" role="group">
          <button type="button" data-result="" class="is-active" data-i18n="filter.all">全部</button>
          <button type="button" data-result="success" data-i18n="result.success">成功</button>
          <button type="button" data-result="failed" data-i18n="result.failed">失败</button>
        </div>
      </div>
      <div class="table-wrap">
        <table>
          <thead><tr>
            <th data-i18n="table.time">时间</th>
            <th data-i18n="table.model">模型名称</th>
            <th class="col-key" data-i18n="table.apiKey">API Key</th>
            <th class="num" data-i18n="table.input">输入</th>
            <th class="num" data-i18n="table.output">输出</th>
            <th class="num" data-i18n="table.reasoning">推理</th>
            <th class="num" data-i18n="table.cacheRead">缓存读取</th>
            <th class="num" data-i18n="table.cacheWrite">缓存写入</th>
            <th class="num" data-i18n="table.totalTokens">总 Token 数</th>
            <th class="num" data-i18n="table.ttft">首字延迟</th>
            <th class="num" data-i18n="table.duration">持续时长</th>
            <th class="num" data-i18n="table.generationTps">生成 TPS</th>
            <th class="num" data-i18n="table.endToEndTps">端到端 TPS</th>
            <th data-i18n="table.result">结果</th>
          </tr></thead>
          <tbody id="requestRows"></tbody>
        </table>
      </div>
      <div class="pager">
        <span id="requestPageInfo"></span>
        <div class="pager-actions">
          <button id="prevPage" type="button" data-i18n="pagination.prev">上一页</button>
          <button id="nextPage" type="button" data-i18n="pagination.next">下一页</button>
        </div>
      </div>
    </article>
  </section>

  <section class="panel" id="panelData" role="tabpanel">
    <article class="card data-card">
      <div class="card-title" data-i18n="data.keyTitle">管理密钥</div>
      <p class="data-hint" data-i18n="data.keyHint">备份、恢复与清空需要 CLIProxyAPI 管理密钥。执行这类操作时会弹窗索取密钥，用完立即从页面清除，不会保存到插件或浏览器存储。</p>
    </article>
    <article class="card data-card">
      <div class="card-title" data-i18n="data.backupTitle">备份与恢复</div>
      <p class="data-hint" data-i18n="data.backupHint">下载完整数据库备份，或用备份文件替换当前数据（使用历史、偏好设置和 API Key 备注）。</p>
      <div class="btn-row">
        <button id="exportBackup" class="solid-btn" type="button" data-i18n="button.downloadBackup">下载备份</button>
        <button id="restoreBackup" class="soft-btn" type="button" data-i18n="button.restoreBackup">恢复备份</button>
      </div>
    </article>
    <article class="card data-card">
      <div class="card-title" data-i18n="data.resetTitle">清空统计</div>
      <p class="data-hint" data-i18n="data.resetHint">永久删除全部 Token 统计记录，此操作无法撤销。建议先下载备份。</p>
      <div class="btn-row">
        <button id="resetButtonPanel" class="danger-btn" type="button" data-i18n="button.confirmReset">确认清空</button>
      </div>
    </article>
  </section>
</div>

<dialog id="rangeDialog">
  <h2 class="dialog-title" data-i18n="range.label">时间范围</h2>
  <p class="dialog-sub" data-i18n="range.quickRanges">快捷时间范围</p>
  <div class="preset-list" id="presetList">
    <button class="preset" type="button" data-mode="today" data-i18n="range.today">今日</button>
    <button class="preset" type="button" data-mode="yesterday" data-i18n="range.yesterday">昨日</button>
    <button class="preset" type="button" data-mode="last_7_days" data-i18n="range.lastSevenDays">最近 7 天</button>
    <button class="preset" type="button" data-mode="last_30_days" data-i18n="range.lastThirtyDays">最近 30 天</button>
  </div>
  <div class="field-row">
    <div class="field">
      <label for="customStart" data-i18n="range.startTime">开始时间</label>
      <input id="customStart" type="datetime-local">
    </div>
    <div class="field">
      <label for="customEnd" data-i18n="range.endTime">结束时间</label>
      <input id="customEnd" type="datetime-local">
    </div>
  </div>
  <p class="error-text" id="rangeError"></p>
  <div class="dialog-foot">
    <button id="rangeCancel" class="soft-btn" type="button" data-i18n="dialog.cancel">取消</button>
    <button id="rangeApply" class="primary-btn" type="button" data-i18n="dialog.apply">应用</button>
  </div>
</dialog>

<dialog id="resetDialog">
  <h2 class="dialog-title" data-i18n="reset.title">管理密钥确认</h2>
  <p class="dialog-sub" data-i18n="confirm.reset">确定永久删除全部 Token 统计吗？此操作不可撤销。</p>
  <div class="dialog-foot">
    <button id="resetCancel" class="soft-btn" type="button" data-i18n="dialog.cancel">取消</button>
    <button id="resetConfirm" class="danger-btn" type="button" data-i18n="button.confirmReset">确认清空</button>
  </div>
</dialog>

<dialog id="keyDialog">
  <h2 class="dialog-title" id="keyDialogTitle" data-i18n="data.keyTitle">管理密钥</h2>
  <p class="dialog-sub" id="keyDialogSub"></p>
  <input id="keyDialogInput" class="lock-input" type="password" autocomplete="off" spellcheck="false" data-i18n-placeholder="lock.placeholder" placeholder="Management Key">
  <p class="error-text" id="keyDialogError"></p>
  <div class="dialog-foot">
    <button id="keyDialogCancel" class="soft-btn" type="button" data-i18n="dialog.cancel">取消</button>
    <button id="keyDialogConfirm" class="primary-btn" type="button" data-i18n="dialog.continue">继续</button>
  </div>
</dialog>

<dialog id="priceDialog">
  <h2 class="dialog-title" data-i18n="models.priceTitle">模型价格</h2>
  <p class="dialog-sub" id="priceDialogSub"></p>
  <div class="field-row">
    <div class="field">
      <label for="priceInput" data-i18n="models.priceInput">输入价格</label>
      <input id="priceInput" type="number" min="0" step="0.000001" inputmode="decimal" autocomplete="off">
    </div>
    <div class="field">
      <label for="priceOutput" data-i18n="models.priceOutput">输出价格</label>
      <input id="priceOutput" type="number" min="0" step="0.000001" inputmode="decimal" autocomplete="off">
    </div>
  </div>
  <div class="field-row">
    <div class="field">
      <label for="priceCacheRead" data-i18n="models.priceCacheRead">缓存读取价格</label>
      <input id="priceCacheRead" type="number" min="0" step="0.000001" inputmode="decimal" autocomplete="off">
    </div>
    <div class="field">
      <label for="priceCacheWrite" data-i18n="models.priceCacheWrite">缓存写入价格</label>
      <input id="priceCacheWrite" type="number" min="0" step="0.000001" inputmode="decimal" autocomplete="off">
    </div>
  </div>
  <p class="field-hint" data-i18n="models.priceHint">单位：美元 / 百万 Token。留空的字段沿用刊例价；清空全部字段并保存即恢复刊例价。</p>
  <details class="time-plan" id="timePlan">
    <summary data-i18n="models.timePlanTitle">峰谷 / 时段价</summary>
    <p class="field-hint" data-i18n="models.timePlanHelp">按所选时区的墙钟时间生效；星期用 1-7（周一至周日），留空表示每天；跨午夜时段归属起始日；费率留空继承基础价。</p>
    <div class="field">
      <label for="timePlanZone" data-i18n="models.timePlanZone">时区</label>
      <input id="timePlanZone" type="text" autocomplete="off" spellcheck="false" placeholder="UTC">
    </div>
    <p class="field-hint" id="timePlanSource"></p>
    <div class="time-tier-list" id="timeTierList"></div>
    <button id="timeTierAdd" class="soft-btn" type="button" data-i18n="models.timePlanAdd">添加时段</button>
  </details>
  <p class="error-text" id="priceError"></p>
  <div class="dialog-foot">
    <button id="priceReset" class="soft-btn" type="button" data-i18n="models.resetPrice">恢复刊例价</button>
    <button id="priceCancel" class="soft-btn" type="button" data-i18n="dialog.cancel">取消</button>
    <button id="priceSave" class="primary-btn" type="button" data-i18n="dialog.save">保存</button>
  </div>
</dialog>

<div id="toast" class="toast" role="status"></div>

<script>
(function(){
'use strict';

/* ------------------------------------------------------------------ route */
function readDashboardRoute(){
  var path=window.location.pathname;
  var marker='/v0/resource/plugins/';
  var index=path.lastIndexOf(marker);
  var prefix=index>=0?path.slice(0,index):'';
  var pluginID='';
  if(index>=0){
    var sub=path.slice(index+marker.length);
    var slash=sub.indexOf('/');
    pluginID=slash>=0?sub.slice(0,slash):sub;
  }
  return {publicPathPrefix:prefix,pluginID:pluginID};
}
var dashboardRoute=readDashboardRoute();
var publicPathPrefix=dashboardRoute.publicPathPrefix;
var pluginID=dashboardRoute.pluginID;
var resourceBase=publicPathPrefix+'/v0/resource/plugins/'+pluginID;
var managementBase=publicPathPrefix+'/v0/management/plugins/'+pluginID;
var statsInitialURL=resourceBase+'/stats/initial';
var statsGroupsURL=resourceBase+'/stats/groups';
var requestsURL=resourceBase+'/requests';
var statsModelsURL=resourceBase+'/stats/models';
var modelPricesURL=resourceBase+'/model-prices';
var preferencesURL=resourceBase+'/preferences';
var sessionURL=managementBase+'/session';
var sessionRevokeURL=managementBase+'/session/revoke';
var backupURL=managementBase+'/backup';
var restoreURL=managementBase+'/restore';
var resetURL=managementBase+'/reset';

/* ----------------------------------------------------------------- locale */
var locales=/*LOCALE_PLACEHOLDER*/;
var supportedLocales=['en','zh-CN','zh-TW','ru'];
var locale='zh-CN';
var formatterLocale='zh-CN';

function detectLocale(){
  var nav=navigator.languages||[navigator.language||''];
  for(var i=0;i<nav.length;i++){
    var norm=normalizeLocale(nav[i]);
    if(norm)return norm;
  }
  return 'zh-CN';
}
function normalizeLocale(value){
  if(!value)return '';
  var v=value.toLowerCase();
  if(v.indexOf('zh-tw')===0||v.indexOf('zh-hk')===0)return 'zh-TW';
  if(v.indexOf('zh')===0)return 'zh-CN';
  if(v.indexOf('ru')===0)return 'ru';
  if(v.indexOf('en')===0)return 'en';
  return '';
}
function t(key,params){
  var dict=locales[locale]||locales['zh-CN']||{};
  var label=dict[key]||key;
  if(params&&typeof params==='object'){
    Object.keys(params).forEach(function(name){
      label=label.replace(new RegExp('\\{'+name+'\\}','g'),params[name]);
    });
  }
  return label;
}
function text(id,content){
  var el=document.getElementById(id);
  if(el)el.textContent=content;
}

/* ----------------------------------------------------------------- theme */
function initializeThemeSync(){
  if(!window.parent||window.parent===window)return;
  try{
    var parentDoc=window.parent.document;
    var observer=new MutationObserver(function(){
      var parentRoot=parentDoc.documentElement;
      var parentTheme=parentRoot.getAttribute('data-theme');
      var theme=parentTheme==='dark'||parentTheme==='white'?parentTheme:'light';
      var bg=window.parent.getComputedStyle(parentRoot).getPropertyValue('--bg-secondary').trim();
      var root=document.documentElement;
      if(theme==='light')root.removeAttribute('data-theme');else root.setAttribute('data-theme',theme);
      root.style.backgroundColor=bg;
      root.style.colorScheme=theme==='dark'?'dark':'light';
    });
    observer.observe(parentDoc.documentElement,{attributes:true,attributeFilter:['data-theme','style','class','lang']});
  }catch(_e){}
}

/* ---------------------------------------------------------------- format */
function localeNumber(value,options){
  return new Intl.NumberFormat(formatterLocale,options).format(Number(value)||0);
}
function localeDate(value,options){
  var date=value instanceof Date?value:new Date(value);
  return new Intl.DateTimeFormat(formatterLocale,options).format(date);
}
function formatTokens(value){
  return localeNumber(Math.round(Number(value)||0));
}
function trimFixed(value,digits){
  var fixed=value.toFixed(digits);
  return fixed.replace(/\.0+$/,'').replace(/(\.\d)0$/,'$1');
}
function compactNumber(value){
  var num=Number(value)||0;
  var abs=Math.abs(num);
  if(abs>=1e9)return trimFixed(num/1e9,abs>=1e10?0:1)+'B';
  if(abs>=1e6)return trimFixed(num/1e6,abs>=1e7?0:1)+'M';
  if(abs>=1e3)return trimFixed(num/1e3,abs>=1e4?0:1)+'K';
  return String(Math.round(num));
}
// cacheHitRate keeps cache hit rate anchored to upstream input tokens so
// cached_tokens can never inflate it above 100%.
function cacheHitRate(input,cacheRead){
  var inputTokens=Number(input)||0;
  var cacheTokens=Number(cacheRead)||0;
  if(!inputTokens)return 0;
  return Math.min(100,cacheTokens/inputTokens*100);
}
function formatPercent(value,digits){
  var num=Number(value)||0;
  return (digits?num.toFixed(digits):String(Math.round(num)))+'%';
}
// formatMoney renders USD list prices without inventing precision: cents above
// $1000, four decimals below, and a floor marker for non-zero pocket change.
function formatMoney(value){
  var num=Number(value)||0;
  if(!(num>0))return '$0';
  if(num<0.0001)return '<$0.0001';
  if(num>=1000)return '$'+localeNumber(num,{minimumFractionDigits:2,maximumFractionDigits:2});
  return '$'+trimFixed(num,4);
}
function formatRate(value){
  if(value===null||value===undefined||value==='')return '';
  return formatMoney(value);
}
// formatSignedMoney keeps the sign visible for a delta that may be a discount.
function formatSignedMoney(value){
  var num=Number(value)||0;
  if(num>0)return '+'+formatMoney(num);
  if(num<0)return '−'+formatMoney(Math.abs(num));
  return formatMoney(0);
}
function formatRatePair(price){
  if(!price)return '';
  var input=formatRate(price.input);
  var output=formatRate(price.output);
  if(!input&&!output)return '';
  return (input||'—')+' / '+(output||'—');
}
function formatTPS(value){
  var num=Number(value)||0;
  if(!(num>0))return '—';
  if(num>=100)return String(Math.round(num));
  if(num>=10)return num.toFixed(1);
  return num.toFixed(2);
}
function formatTTFT(ns){
  var value=Number(ns)||0;
  if(!(value>0))return '—';
  var ms=value/1e6;
  if(ms>=1000)return trimFixed(ms/1000,2)+' s';
  if(ms>=100)return String(Math.round(ms))+' ms';
  return trimFixed(ms,ms>=10?1:2)+' ms';
}
// formatDuration renders a whole-request latency, so multi-second values stay
// readable without inventing sub-millisecond precision.
function formatDuration(ns){
  var value=Number(ns)||0;
  if(!(value>0))return '—';
  var ms=value/1e6;
  if(ms>=60000)return trimFixed(ms/60000,1)+' min';
  if(ms>=1000)return trimFixed(ms/1000,2)+' s';
  return String(Math.round(ms))+' ms';
}
function startOfDay(date){
  return new Date(date.getFullYear(),date.getMonth(),date.getDate());
}
function formatClock(value){
  return localeDate(value,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false});
}

/* ----------------------------------------------------------------- state */
var state={
  range:{mode:'last_30_days',start:null,end:null},
  models:[],
  keys:[],
  scale:'auto',
  costDimension:'model',
  rankMetric:'tokens',
  data:null,
  catalog:null,
  catalogKey:'',
  tab:'usage',
  seq:0,
  requests:{offset:0,limit:30,total:0,result:'',loading:false},
  modelsData:null,
  modelsSeq:0,
  modelsLoading:false,
  modelSearch:'',
  priceModel:'',
  catalogRefreshRequested:false
};
var palette=['#e8871f','#3f7fe0','#12a171','#9b6ede','#d8503c','#2aa3a8','#c9a227','#7b8aa0'];
var seriesColors={hit:'',miss:'',out:'',req:''};
var chartState={};
// pinnedChart remembers which chart a touch tap selected, so the tooltip stays
// on screen until the operator taps somewhere else.
var pinnedChart=null;

/* ---------------------------------------------------------- access gate */
// The plugin serves this page from an unauthenticated host route, so every
// data request has to present a session token. The token is minted by the
// management API (which the host guards with the CLIProxyAPI management key)
// and only its SHA-256 digest is kept in plugin memory. The token lives in
// sessionStorage (this tab only) and never together with the management key.
var sessionStoreKey='cap-token-usage-tracker.session';
var session={token:'',expiresAt:''};
var sessionStartPending=false;
var sessionExpiredShown=false;

function readStoredSession(){
  try{
    var raw=window.sessionStorage.getItem(sessionStoreKey);
    if(!raw)return null;
    var record=JSON.parse(raw);
    if(!record||typeof record.token!=='string'||!record.token)return null;
    if(record.pluginID&&pluginID&&record.pluginID!==pluginID)return null;
    if(record.expiresAt&&Date.parse(record.expiresAt)<=Date.now())return null;
    return record;
  }catch(_error){return null;}
}
function persistStoredSession(){
  try{
    window.sessionStorage.setItem(sessionStoreKey,JSON.stringify({
      token:session.token,expiresAt:session.expiresAt,pluginID:pluginID
    }));
  }catch(_error){}
}
function clearStoredSession(){
  try{window.sessionStorage.removeItem(sessionStoreKey);}catch(_error){}
}
function sessionHeaders(headers){
  var merged={};
  Object.keys(headers||{}).forEach(function(name){merged[name]=headers[name];});
  if(session.token)merged['X-Plugin-Session']=session.token;
  return merged;
}
function showLock(message){
  document.body.classList.remove('is-unlocked');
  document.body.classList.add('is-locked');
  var screen=document.getElementById('lockScreen');
  if(screen)screen.hidden=false;
  text('unlockError',message||'');
  var input=document.getElementById('unlockKey');
  if(input){
    input.value='';
    setTimeout(function(){input.focus();},40);
  }
}
function hideLock(){
  document.body.classList.remove('is-locked');
  document.body.classList.add('is-unlocked');
  var screen=document.getElementById('lockScreen');
  if(screen)screen.hidden=true;
}
function clearDashboardView(){
  ['totalTokens','tokenTrendTotal','requestTrendTotal','totalRequests','cacheHitRate','costTotal','costPricedModels','costUnpricedModels','costCoverage','costTrendTotal'].forEach(function(id){
    text(id,'0');
  });
  ['tokenStack','tokenLegend','modelRank','apiKeyRank','requestRows','tokenChart','requestChart','costStack','costLegend','costModelRank','costApiKeyRank','costChart','costChartLegend'].forEach(function(id){
    var node=document.getElementById(id);
    if(node)node.replaceChildren();
  });
  var hint=document.getElementById('scaleHint');
  if(hint){hint.textContent='';hint.hidden=true;hint.classList.remove('scale-clamped');}
  state.data=null;
  state.catalog=null;
  state.catalogKey='';
}
function handleSessionRejected(){
  session={token:'',expiresAt:''};
  clearStoredSession();
  clearDashboardView();
  if(sessionExpiredShown)return;
  sessionExpiredShown=true;
  showLock(t('lock.expired'));
}
function unlockWithKey(key){
  return fetch(sessionURL,{
    method:'POST',
    credentials:'same-origin',
    cache:'no-store',
    headers:{Authorization:'Bearer '+key}
  }).then(function(res){
    if(res.status===401||res.status===403)throw new Error(t('lock.invalidKey'));
    if(!res.ok)throw new Error(t('lock.failed'));
    return res.json();
  }).then(function(body){
    if(!body||!body.session)throw new Error(t('lock.failed'));
    session={token:body.session,expiresAt:body.expires_at||''};
    persistStoredSession();
    sessionExpiredShown=false;
    hideLock();
    startDashboard();
  });
}
function beginSession(){
  if(sessionStartPending)return;
  sessionStartPending=true;
  loadDashboardPreferences().then(function(){
    updateChips();
    return load();
  }).catch(function(){
    sessionStartPending=false;
  }).then(function(){
    sessionStartPending=false;
  });
}
function startDashboard(){
  sessionStartPending=false;
  beginSession();
}

function cssVar(name){
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}
function refreshSeriesColors(){
  seriesColors.hit=cssVar('--hit')||'#8edeff';
  seriesColors.miss=cssVar('--miss')||'#3ab5ff';
  seriesColors.out=cssVar('--out')||'#0072fb';
  seriesColors.req=cssVar('--req')||'#5b9bf5';
}

/* ------------------------------------------------------------ data access */
function api(url,options,timeout){
  var controller=new AbortController();
  var timer=setTimeout(function(){controller.abort();},timeout||25000);
  options=options||{};
  options.signal=controller.signal;
  if(!options.credentials)options.credentials='same-origin';
  options.headers=sessionHeaders(options.headers);
  return fetch(url,options).then(function(res){
    clearTimeout(timer);
    if(res.status===401){
      var expired=new Error(t('lock.expired'));
      expired.sessionRejected=true;
      handleSessionRejected();
      throw expired;
    }
    if(!res.ok){
      return res.json().catch(function(){return {};}).then(function(body){
        throw new Error(body.error||('HTTP '+res.status));
      });
    }
    return res.json();
  }).catch(function(error){
    clearTimeout(timer);
    throw error;
  });
}

function rangeKey(){
  if(state.range.mode==='last_7_days'||state.range.mode==='last_30_days')return state.range.mode;
  return state.range.mode+'|'+state.range.start+'|'+state.range.end;
}
function currentWindow(){
  var now=new Date();
  if(state.range.mode==='today')return {start:startOfDay(now),end:now,name:'today'};
  if(state.range.mode==='yesterday'){
    var today=startOfDay(now);
    var previous=new Date(today.getFullYear(),today.getMonth(),today.getDate()-1);
    return {start:previous,end:today,name:'yesterday'};
  }
  if(state.range.mode==='custom'){
    var start=state.range.start?new Date(state.range.start):new Date(now.getTime()-7*864e5);
    var end=state.range.end?new Date(state.range.end):now;
    return {start:start,end:end,name:'custom'};
  }
  return {start:null,end:null,name:state.range.mode};
}
function queryString(withFilters){
  var params=new URLSearchParams();
  var range=currentWindow();
  if(range.start&&range.end){
    params.set('range','custom');
    params.set('start',range.start.toISOString());
    params.set('end',range.end.toISOString());
  }else{
    params.set('range',state.range.mode==='last_7_days'?'7d':'30d');
  }
  if(withFilters){
    state.models.forEach(function(model){params.append('model',model);});
    state.keys.forEach(function(ref){params.append('api_key_ref',ref);});
  }
  if(state.scale&&state.scale!=='auto')params.set('scale',state.scale);
  params.set('tz_offset',String(-new Date().getTimezoneOffset()));
  return params.toString();
}
function hasFilters(){
  return state.models.length>0||state.keys.length>0;
}

/* ------------------------------------------------------------- rendering */
function summaryOf(data){
  return (data&&data.summary)||{};
}
function currentBuckets(){
  var rows=(state.data&&state.data.series)||[];
  var points=[];
  for(var i=0;i<rows.length;i++){
    var row=rows[i];
    var hit=Number(row.cache_read_tokens||0)||Number(row.cached_tokens||0)||0;
    var input=Number(row.input_tokens||0);
    if(hit>input)hit=input;
    var miss=Math.max(0,input-hit);
    var out=Number(row.output_tokens||0);
    points.push({
      at:new Date(row.hour),
      hit:hit,
      miss:miss,
      out:out,
      requests:Number(row.requests||0),
      failed:Number(row.failed_requests||0),
      total:hit+miss+out
    });
  }
  // The server already returns zero-filled, evenly spaced buckets at the
  // resolved scale, so the browser never regroups the axis.
  return points;
}
// bucketScale reports the resolved axis unit of the current payload.
function bucketScale(){
  var scale=state.data&&state.data.scale;
  if(scale==='minute'||scale==='hour'||scale==='day')return scale;
  var seconds=Number((state.data&&state.data.bucket_seconds)||0);
  if(seconds&&seconds<3600)return 'minute';
  if(seconds>=86400)return 'day';
  return 'hour';
}
function scaleLabel(){
  return t('scale.'+bucketScale());
}
function scaleClampText(data){
  var requested=data&&data.requested_scale;
  if(!requested||requested===data.scale)return '';
  return t('scale.clamped',{requested:t('scale.'+requested),unit:scaleLabel()});
}
function scaleNoteText(data){
  var clamp=scaleClampText(data);
  if(clamp)return clamp;
  return t('scale.note',{unit:scaleLabel()});
}
// bucketLabel formats one axis tick for the resolved scale.
function bucketLabel(date){
  var scale=bucketScale();
  if(scale==='minute')return localeDate(date,{hour:'2-digit',minute:'2-digit',hour12:false});
  if(scale==='day')return localeDate(date,{month:'numeric',day:'numeric'});
  return localeDate(date,{month:'numeric',day:'numeric',hour:'2-digit',hour12:false});
}
function renderHero(){
  var data=state.data;
  var summary=summaryOf(data);
  var total=Number(summary.total_tokens||0);
  text('totalTokens',formatTokens(total));
  var hit=Number(summary.cache_read_tokens||summary.cached_tokens||0);
  var input=Number(summary.input_tokens||0);
  if(hit>input)hit=input;
  var miss=Math.max(0,input-hit);
  var out=Number(summary.output_tokens||0);
  var parts=[
    {key:'hit',label:t('chart.inputHit'),value:hit,color:seriesColors.hit},
    {key:'miss',label:t('chart.inputMiss'),value:miss,color:seriesColors.miss},
    {key:'out',label:t('chart.output'),value:out,color:seriesColors.out}
  ];
  var sum=hit+miss+out;
  var stack=document.getElementById('tokenStack');
  var legend=document.getElementById('tokenLegend');
  var stackFragment=document.createDocumentFragment();
  var legendFragment=document.createDocumentFragment();
  parts.forEach(function(part){
    var seg=document.createElement('i');
    seg.style.width=(sum>0?part.value/sum*100:0)+'%';
    seg.style.background=part.color;
    stackFragment.appendChild(seg);
    var item=document.createElement('span');
    item.className='legend-item';
    var dot=document.createElement('i');
    dot.className='legend-dot';
    dot.style.background=part.color;
    item.appendChild(dot);
    var label=document.createElement('span');
    label.textContent=part.label;
    item.appendChild(label);
    var value=document.createElement('b');
    value.textContent=formatTokens(part.value);
    item.appendChild(value);
    legendFragment.appendChild(item);
  });
  stack.replaceChildren(stackFragment);
  legend.replaceChildren(legendFragment);

  var requests=Number(summary.requests||0);
  var failed=Number(summary.failed_requests||0);
  var succeeded=Math.max(0,requests-failed);
  var successRate=requests?Math.round(succeeded/requests*1000)/10:0;
  text('totalRequests',formatTokens(requests));
  text('requestSub',t('card.successRate',{value:successRate})+' · '+t('result.failed')+' '+formatTokens(failed));
  document.getElementById('successFill').style.width=(requests?successRate:0)+'%';

  var cacheRate=cacheHitRate(input,hit);
  text('cacheHitRate',formatPercent(cacheRate));
  text('cacheSub',t('chart.inputHit')+' '+formatTokens(hit));

  var note=state.data&&state.data.generated_at?formatClock(state.data.generated_at):'';
  var rangeNode=document.getElementById('rangeValue');
  text('rangeNote',rangeNode?rangeNode.textContent:'');
  text('updatedAt',t('status.updated',{value:note||'—'}));
  document.getElementById('refreshButton').classList.remove('is-busy');
}

/* charts */
function niceMax(value){
  if(!(value>0))return 1;
  var exp=Math.pow(10,Math.floor(Math.log10(value)));
  var fraction=value/exp;
  var steps=[1,1.2,1.5,2,2.5,3,4,5,6,8,10];
  var nice=steps[steps.length-1];
  for(var i=0;i<steps.length;i++){
    if(fraction<=steps[i]){nice=steps[i];break;}
  }
  return nice*exp;
}
function svgEl(name,attrs){
  var node=document.createElementNS('http://www.w3.org/2000/svg',name);
  if(attrs){
    Object.keys(attrs).forEach(function(key){node.setAttribute(key,attrs[key]);});
  }
  return node;
}
function chartGeometry(svg){
  var width=svg.clientWidth||svg.parentNode.clientWidth||320;
  var height=svg.clientHeight||svg.parentNode.clientHeight||200;
  var pad={left:width<380?38:46,right:8,top:12,bottom:24};
  return {width:width,height:height,pad:pad,plotWidth:Math.max(10,width-pad.left-pad.right),plotHeight:Math.max(10,height-pad.top-pad.bottom)};
}
function axisLabel(example){
  return compactNumber(example);
}
function emptyChart(svg,message){
  svg.replaceChildren();
  var parent=svg.parentNode;
  var existing=parent.querySelector('.chart-empty');
  if(existing)existing.remove();
  var note=document.createElement('div');
  note.className='chart-empty';
  note.textContent=message;
  parent.appendChild(note);
}
function clearEmptyChart(svg){
  var parent=svg.parentNode;
  var existing=parent.querySelector('.chart-empty');
  if(existing)existing.remove();
}

// monotonePath interpolates a smooth cubic curve that never overshoots the
// sampled values, so a jagged series reads like the reference app without
// dipping below zero between buckets.
function monotonePath(points){
  var count=points.length;
  if(!count)return '';
  if(count===1)return 'M'+points[0].x+' '+points[0].y;
  var dx=[],slopes=[],tangents=[];
  for(var i=0;i<count-1;i++){
    dx[i]=points[i+1].x-points[i].x;
    slopes[i]=dx[i]?(points[i+1].y-points[i].y)/dx[i]:0;
  }
  tangents[0]=slopes[0];
  tangents[count-1]=slopes[count-2];
  for(var j=1;j<count-1;j++){
    if(slopes[j-1]*slopes[j]<=0){tangents[j]=0;continue;}
    var w1=2*dx[j]+dx[j-1];
    var w2=dx[j]+2*dx[j-1];
    tangents[j]=(w1+w2)/(w1/slopes[j-1]+w2/slopes[j]);
  }
  var d='M'+points[0].x+' '+points[0].y;
  for(var k=0;k<count-1;k++){
    var third=dx[k]/3;
    d+=' C'+(points[k].x+third)+' '+(points[k].y+tangents[k]*third)+' '+
      (points[k+1].x-third)+' '+(points[k+1].y-tangents[k+1]*third)+' '+
      points[k+1].x+' '+points[k+1].y;
  }
  return d;
}

function drawChart(config){
  var svg=document.getElementById(config.svgId);
  var tip=document.getElementById(config.tipId);
  if(!svg||!tip)return;
  var points=config.points||[];
  if(!points.length){
    emptyChart(svg,config.empty||t('chart.noCalls'));
    tip.hidden=true;
    chartState[config.key]=null;
    return;
  }
  clearEmptyChart(svg);
  tip.classList.remove('is-visible');
  tip.hidden=true;
  if(pinnedChart===config.key)pinnedChart=null;
  var geom=chartGeometry(svg);
  var asBars=config.mode!=='area';
  var count=points.length;
  var series=config.series;
  var totals=new Array(count);
  var maxValue=0;
  for(var i=0;i<count;i++){
    var sum=0;
    for(var s=0;s<series.length;s++)sum+=Number(series[s].values[i])||0;
    totals[i]=sum;
    if(sum>maxValue)maxValue=sum;
  }
  var top=niceMax(maxValue||1);
  var ticks=[0,top/2,top];
  var axisFormat=config.axisFormat||compactNumber;
  var slot=geom.plotWidth/Math.max(1,count);
  var base=geom.pad.top+geom.plotHeight;
  function areaX(index){return geom.pad.left+(count===1?geom.plotWidth/2:geom.plotWidth*index/(count-1));}
  function barX(index){return geom.pad.left+slot*index+slot/2;}
  function xAt(index){return asBars?barX(index):areaX(index);}
  var svgFragment=document.createDocumentFragment();

  ticks.forEach(function(tick,index){
    var y=base-(tick/top)*geom.plotHeight;
    var line=svgEl('line',{x1:geom.pad.left,x2:geom.width-geom.pad.right,y1:y,y2:y,stroke:cssVar('--grid'),'stroke-width':1});
    svgFragment.appendChild(line);
    var label=svgEl('text',{x:geom.pad.left-8,y:y+4,'text-anchor':'end',fill:cssVar('--t3'),'font-size':11,'font-family':'inherit'});
    label.textContent=(index===0?'0':axisFormat(tick));
    svgFragment.appendChild(label);
  });

  if(asBars){
    // Reference look: cache hits own the top of the bar and the two smaller
    // classes stack underneath them, so the rounded cap is always the hit
    // colour. The series list stays in reading order (hit, miss, out) for the
    // tooltip and the legend; the bars are drawn from the reversed list.
    var stack=series.slice().reverse();
    var barWidth=Math.min(Math.max(6,slot*0.6),slot*0.92,26);
    var radius=Math.min(4,barWidth/2);
    for(var index=0;index<count;index++){
      var topSegment=-1;
      for(var ti=stack.length-1;ti>=0;ti--){
        if((Number(stack[ti].values[index])||0)>0){topSegment=ti;break;}
      }
      var exactTotal=(totals[index]/top)*geom.plotHeight;
      var minSegment=exactTotal>=10?1.5:0.75;
      var cursor=base;
      for(var si=0;si<stack.length;si++){
        var value=Number(stack[si].values[index])||0;
        if(value<=0)continue;
        var height=Math.max(minSegment,(value/top)*geom.plotHeight);
        var x=barX(index)-barWidth/2;
        var y=cursor-height;
        var segment;
        if(si===topSegment){
          var r=Math.min(radius,height,barWidth/2);
          segment=svgEl('path',{
            d:'M'+x+' '+cursor+' L'+x+' '+(y+r)+' Q'+x+' '+y+' '+(x+r)+' '+y+
              ' L'+(x+barWidth-r)+' '+y+' Q'+(x+barWidth)+' '+y+' '+(x+barWidth)+' '+(y+r)+
              ' L'+(x+barWidth)+' '+cursor+' Z',
            fill:stack[si].color
          });
        }else{
          segment=svgEl('rect',{x:x,y:y,width:barWidth,height:height,fill:stack[si].color});
        }
        svgFragment.appendChild(segment);
        cursor=y;
      }
    }
  }else{
    var single=series[0];
    var coords=[];
    for(var pi=0;pi<count;pi++){
      coords.push({x:areaX(pi),y:base-((Number(single.values[pi])||0)/top)*geom.plotHeight});
    }
    var linePath=monotonePath(coords);
    var gradientId='gradient-'+config.key;
    var defs=svgEl('defs');
    var gradient=svgEl('linearGradient',{id:gradientId,x1:'0',y1:'0',x2:'0',y2:'1'});
    gradient.appendChild(svgEl('stop',{offset:'0%','stop-color':single.color,'stop-opacity':'.42'}));
    gradient.appendChild(svgEl('stop',{offset:'68%','stop-color':single.color,'stop-opacity':'.13'}));
    gradient.appendChild(svgEl('stop',{offset:'100%','stop-color':single.color,'stop-opacity':'0'}));
    defs.appendChild(gradient);
    svgFragment.appendChild(defs);
    var areaPath=linePath+' L'+coords[coords.length-1].x+' '+base+' L'+coords[0].x+' '+base+' Z';
    svgFragment.appendChild(svgEl('path',{d:areaPath,fill:'url(#'+gradientId+')',stroke:'none'}));
    svgFragment.appendChild(svgEl('path',{d:linePath,fill:'none',stroke:single.color,'stroke-width':2.6,'stroke-linejoin':'round','stroke-linecap':'round'}));
  }

  // Ticks use a uniform stride and always include the last bucket, so the
  // axis covers the whole window even when the window is sparsely populated.
  var labelCount=geom.width<420?3:5;
  var stride=Math.max(1,Math.ceil((count-1)/Math.max(1,labelCount-1)));
  var lastIndex=count-1;
  var labelIndexes=[];
  for(var li=0;li<count;li+=stride)labelIndexes.push(li);
  if(labelIndexes[labelIndexes.length-1]!==lastIndex){
    if(labelIndexes.length>1&&lastIndex-labelIndexes[labelIndexes.length-1]<stride/2)labelIndexes.pop();
    labelIndexes.push(lastIndex);
  }
  labelIndexes.forEach(function(index,position){
    var point=points[index];
    if(!point)return;
    var textNode=svgEl('text',{
      x:xAt(index),y:geom.height-6,'text-anchor':position===0?'start':(index===lastIndex?'end':'middle'),
      fill:cssVar('--t3'),'font-size':11,'font-family':'inherit'
    });
    textNode.textContent=config.label(point.at);
    svgFragment.appendChild(textNode);
  });

  var guide=svgEl('g',{opacity:'0'});
  var guideLine=svgEl('line',{y1:geom.pad.top,y2:base,stroke:cssVar('--t3'),'stroke-width':1,'stroke-dasharray':'3 4'});
  var guideDot=svgEl('circle',{r:4,fill:series[0].color,stroke:cssVar('--card'),'stroke-width':2});
  guide.appendChild(guideLine);
  guide.appendChild(guideDot);
  svgFragment.appendChild(guide);

  var overlay=svgEl('rect',{x:geom.pad.left,y:geom.pad.top,width:geom.plotWidth,height:geom.plotHeight,fill:'transparent'});
  svgFragment.appendChild(overlay);
  svg.replaceChildren(svgFragment);

  chartState[config.key]={config:config,geometry:geom,base:base,top:top,totals:totals,guide:guide,guideLine:guideLine,guideDot:guideDot,tip:tip};

  function indexAt(clientX){
    var rect=svg.getBoundingClientRect();
    var scale=(geom.width||rect.width||1)/(rect.width||geom.width||1);
    var relative=(clientX-rect.left)*scale-geom.pad.left;
    if(asBars)return Math.max(0,Math.min(count-1,Math.floor(relative/slot)));
    if(count===1)return 0;
    var ratio=relative/geom.plotWidth;
    return Math.max(0,Math.min(count-1,Math.round(ratio*(count-1))));
  }
  function showAt(index,pin){
    var record=chartState[config.key];
    if(!record)return;
    var point=points[index];
    if(!point){
      hideTip();
      return;
    }
    var x=xAt(index);
    var value=record.totals[index];
    var y=base-(value/top)*geom.plotHeight;
    guide.setAttribute('opacity','1');
    record.guideLine.setAttribute('x1',x);
    record.guideLine.setAttribute('x2',x);
    record.guideDot.setAttribute('cx',x);
    record.guideDot.setAttribute('cy',Math.max(geom.pad.top,y));
    fillTip(record.tip,config.label(point.at),config.totalFormat(value),series.filter(function(entry){
      // A stacked histogram can carry many series; an empty class is noise.
      return (Number(entry.values[index])||0)>0;
    }).map(function(entry){
      return {color:entry.color,label:entry.label,value:config.totalFormat(entry.values[index]||0)};
    }));
    positionTip(record.tip,svg,(x/(geom.width||1))*100,Math.max(geom.pad.top,y),geom);
    if(pin)pinnedChart=config.key;
  }
  function hideTip(){
    if(pinnedChart===config.key)pinnedChart=null;
    var record=chartState[config.key];
    if(!record)return;
    record.guide.setAttribute('opacity','0');
    record.tip.classList.remove('is-visible');
    record.tip.hidden=true;
  }
  // Mouse: hover previews. Touch: a single tap pins the tooltip, and dragging
  // keeps scrubbing without releasing it; the native pointerleave that follows
  // every touch must not dismiss it (that is what forced a long press before).
  overlay.addEventListener('pointerdown',function(event){
    if(event.pointerType==='touch'){
      if(!event.isPrimary)return;
      showAt(indexAt(event.clientX),true);
      try{overlay.setPointerCapture(event.pointerId);}catch(_error){}
      return;
    }
    showAt(indexAt(event.clientX),false);
  });
  overlay.addEventListener('pointermove',function(event){
    if(event.pointerType==='touch'){
      if(!event.isPrimary)return;
      if(!overlay.hasPointerCapture||!overlay.hasPointerCapture(event.pointerId))return;
      showAt(indexAt(event.clientX),true);
      return;
    }
    showAt(indexAt(event.clientX),false);
  });
  overlay.addEventListener('pointerup',function(event){
    if(event.pointerType==='touch'&&event.isPrimary)showAt(indexAt(event.clientX),true);
  });
  overlay.addEventListener('pointerleave',function(event){
    if(event.pointerType==='touch')return;
    hideTip();
  });
  overlay.addEventListener('pointercancel',function(event){
    if(event.pointerType==='touch')return;
    hideTip();
  });
  svg.addEventListener('pointerleave',function(event){
    if(event.pointerType==='touch')return;
    hideTip();
  });
  chartState[config.key].hide=hideTip;
}

function fillTip(tip,head,value,rows){
  var fragment=document.createDocumentFragment();
  var headNode=document.createElement('div');
  headNode.className='tip-head';
  var headLabel=document.createElement('span');
  headLabel.textContent=head;
  var headValue=document.createElement('span');
  headValue.textContent=value;
  headNode.appendChild(headLabel);
  headNode.appendChild(headValue);
  fragment.appendChild(headNode);
  rows.forEach(function(row){
    var line=document.createElement('div');
    line.className='tip-row';
    var dot=document.createElement('i');
    dot.style.background=row.color;
    line.appendChild(dot);
    var label=document.createElement('span');
    label.textContent=row.label;
    line.appendChild(label);
    var amount=document.createElement('b');
    amount.textContent=row.value;
    line.appendChild(amount);
    fragment.appendChild(line);
  });
  tip.replaceChildren(fragment);
}
function positionTip(tip,svg,x,y,geom){
  tip.hidden=false;
  tip.classList.add('is-visible');
  var host=svg.parentNode;
  var hostWidth=host.clientWidth||geom.width;
  var width=tip.offsetWidth||220;
  var height=tip.offsetHeight||90;
  var left=x-width/2;
  left=Math.max(4,Math.min(hostWidth-width-4,left));
  var top=y-height-14;
  if(top<4)top=Math.min(host.clientHeight-height-4,y+18);
  if(top<4)top=4;
  tip.style.left=left+'px';
  tip.style.top=top+'px';
}

// renderSeriesLegend draws the shared colour legend used by the token chart and
// the cost histogram. Entries may carry a value, which is rendered as money.
function renderSeriesLegend(hostId,rows){
  var host=document.getElementById(hostId);
  if(!host)return;
  if(!rows.length){
    host.replaceChildren();
    host.hidden=true;
    return;
  }
  var fragment=document.createDocumentFragment();
  rows.forEach(function(row){
    var item=document.createElement('span');
    var dot=document.createElement('i');
    dot.style.background=row.color;
    item.appendChild(dot);
    if(row.label){
      var label=document.createElement('em');
      label.textContent=row.label;
      if(row.title)label.title=row.title;
      item.appendChild(label);
    }
    if(row.value){
      var value=document.createElement('b');
      value.textContent=row.value;
      item.appendChild(value);
    }
    fragment.appendChild(item);
  });
  host.replaceChildren(fragment);
  host.hidden=false;
}
function renderChartLegend(){
  renderSeriesLegend('tokenChartLegend',[
    {color:seriesColors.hit,label:t('chart.inputHit')},
    {color:seriesColors.miss,label:t('chart.inputMiss')},
    {color:seriesColors.out,label:t('chart.output')}
  ]);
}

function renderCharts(){
  if(!state.data)return;
  var points=currentBuckets();
  var range=currentWindow();
  var label=bucketLabel;
  var tokenSeries=[
    {key:'hit',label:t('chart.inputHit'),color:seriesColors.hit,values:points.map(function(point){return point.hit;})},
    {key:'miss',label:t('chart.inputMiss'),color:seriesColors.miss,values:points.map(function(point){return point.miss;})},
    {key:'out',label:t('chart.output'),color:seriesColors.out,values:points.map(function(point){return point.out;})}
  ];
  drawChart({
    key:'tokens',
    svgId:'tokenChart',
    tipId:'tokenTip',
    points:points,
    series:tokenSeries,
    label:label,
    mode:'bars',
    totalFormat:formatTokens,
    empty:t('chart.noCalls')
  });
  renderChartLegend();
  var summary=summaryOf(state.data);
  var tokenTotal=points.reduce(function(acc,point){return acc+point.total;},0);
  var requestTotal=points.reduce(function(acc,point){return acc+point.requests;},0);
  var peak=points.reduce(function(best,point){
    return point.total>(best.value||0)?{value:point.total,at:point.at}:best;
  },{value:0,at:null});
  text('tokenTrendTotal',compactNumber(tokenTotal));
  var tokenSub=peak.at?t('chart.peak',{value:compactNumber(peak.value),date:localeDate(peak.at,{month:'numeric',day:'numeric'})}):'';
  if(!tokenTotal)tokenSub=t('chart.rangeEmpty');
  text('tokenTrendSub',tokenSub);
  text('requestTrendTotal',compactNumber(requestTotal));
  var requestSub=t('model.calls',{count:formatTokens(requestTotal)})+' · '+t('result.failed')+' '+formatTokens(Number(summary.failed_requests||0));
  if(!requestTotal)requestSub=t('chart.rangeEmpty');
  text('requestTrendSub',requestSub);
  var clamp=scaleClampText(state.data);
  var hint=document.getElementById('scaleHint');
  if(hint){
    if(clamp){
      hint.textContent=clamp;
      hint.title=clamp;
      hint.classList.add('scale-clamped');
      hint.hidden=false;
    }else{
      hint.textContent='';
      hint.title='';
      hint.classList.remove('scale-clamped');
      hint.hidden=true;
    }
  }
  text('requestScaleNote',scaleNoteText(state.data));

  drawChart({
    key:'requests',
    svgId:'requestChart',
    tipId:'requestTip',
    points:points,
    series:[{key:'requests',label:t('chart.requests'),color:seriesColors.req,values:points.map(function(point){return point.requests;})}],
    label:label,
    mode:'area',
    totalFormat:formatTokens,
    empty:t('chart.noRequests')
  });
  renderCostChart();
  if(range.mode==='today')text('rangeNote',t('range.today'));
}

/* ------------------------------------------------------------- cost tab */
function costBlockOf(data){
  return (data&&data.cost)||{};
}
function costPoints(){
  var rows=(state.data&&state.data.cost_series)||[];
  return rows.map(function(row){
    return {
      at:new Date(row.hour),
      cost:Number(row.cost_usd||0),
      requests:Number(row.requests||0),
      unpriced:Number(row.unpriced_requests||0)
    };
  });
}
function renderCostHero(){
  var summary=summaryOf(state.data);
  var block=costBlockOf(state.data);
  var cost=Number(block.cost_usd||summary.cost_usd||0);
  text('costTotal',formatMoney(cost));
  var pricedModels=Number(block.priced_models||0);
  var freeModels=Number(block.free_models||0);
  var unpricedModels=Number(block.unpriced_models||0);
  var pricedRequests=Number(block.priced_requests||0);
  var unpricedRequests=Number(block.unpriced_requests||0);
  var billable=Math.max(0,pricedModels-freeModels);
  var note=t('cost.sub',{priced:formatTokens(pricedModels),unpriced:formatTokens(unpricedModels)});
  text('costRangeNote',note);

  var stack=document.getElementById('costStack');
  var legend=document.getElementById('costLegend');
  var total=Math.max(1,pricedModels+unpricedModels);
  var segments=[
    {label:t('cost.pricedLegend'),value:billable,color:'var(--ok)'},
    {label:t('cost.freeLegend'),value:freeModels,color:'var(--hit)'},
    {label:t('cost.unpricedLegend'),value:unpricedModels,color:'var(--miss)'}
  ];
  var stackFragment=document.createDocumentFragment();
  var legendFragment=document.createDocumentFragment();
  segments.forEach(function(segment){
    var seg=document.createElement('i');
    seg.style.width=(segment.value/total*100)+'%';
    seg.style.background=segment.color;
    stackFragment.appendChild(seg);
    var item=document.createElement('span');
    item.className='legend-item';
    var dot=document.createElement('i');
    dot.className='legend-dot';
    dot.style.background=segment.color;
    item.appendChild(dot);
    var label=document.createElement('span');
    label.textContent=segment.label;
    item.appendChild(label);
    var value=document.createElement('b');
    value.textContent=formatTokens(segment.value);
    item.appendChild(value);
    legendFragment.appendChild(item);
  });
  stack.replaceChildren(stackFragment);
  legend.replaceChildren(legendFragment);

  text('costPricedModels',formatTokens(pricedModels));
  text('costPricedSub',t('cost.pricedSub',{free:formatTokens(freeModels),requests:formatTokens(pricedRequests)}));
  document.getElementById('costPricedFill').style.width=(pricedModels?Math.min(100,billable/Math.max(1,pricedModels)*100):0)+'%';
  text('costUnpricedModels',formatTokens(unpricedModels));
  text('costUnpricedSub',t('cost.unpriced',{count:formatTokens(unpricedRequests)}));
  document.getElementById('costUnpricedFill').style.width=(unpricedModels?Math.min(100,unpricedModels/Math.max(1,pricedModels+unpricedModels)*100):0)+'%';
  var requests=Number(summary.requests||0);
  var coverage=requests?pricedRequests/requests*100:0;
  text('costCoverage',formatPercent(coverage));
  text('costCoverageSub',t('cost.coverageSub',{priced:formatTokens(pricedRequests),total:formatTokens(requests)}));
  document.getElementById('costCoverageFill').style.width=Math.min(100,coverage)+'%';

  // The time-of-use delta explains why the total differs from a flat estimate.
  var tierCost=Number(block.time_tier_cost_usd||0);
  var tierRequests=Number(block.time_tier_requests||0);
  var tierNote=document.getElementById('costTimeTierNote');
  if(tierNote){
    if(tierCost||tierRequests){
      tierNote.hidden=false;
      tierNote.textContent=t('cost.timeTierNote',{value:formatSignedMoney(tierCost),requests:formatTokens(tierRequests)});
    }else{
      tierNote.hidden=true;
      tierNote.textContent='';
    }
  }
}
// breakdownKeyLabel resolves an API key reference to its operator label. The
// current payload is preferred, the unfiltered catalog is the fallback, and the
// reference tail is the last resort so a key is never rendered as blank.
function breakdownKeyLabel(ref,data){
  var lists=[(data&&data.api_keys)||[],(state.catalog&&state.catalog.api_keys)||[]];
  for(var i=0;i<lists.length;i++){
    var list=lists[i];
    if(!list)continue;
    for(var j=0;j<list.length;j++){
      var entry=list[j];
      if(entry&&entry.ref===ref)return keyLabel(entry);
    }
  }
  return keyLabel({ref:ref});
}
// costChartSeries turns the server-side per-bucket breakdown into stacked chart
// series. The breakdown arrays are aligned with cost_series, so index i of every
// series is the same bucket.
function costChartSeries(points,data){
  var byKey=state.costDimension==='key';
  var rows=((byKey?(data&&data.cost_by_key):(data&&data.cost_by_model))||[]);
  if(!rows.length){
    if(byKey)return {series:[],note:t('cost.noKeys')};
    // Older payloads (or a window with no priced usage at all) still get the
    // aggregate total so the histogram never disappears.
    return {series:[{
      key:'cost',
      label:t('cost.trend'),
      color:seriesColors.hit,
      total:points.reduce(function(acc,point){return acc+point.cost;},0),
      values:points.map(function(point){return point.cost;})
    }]};
  }
  var series=[];
  rows.forEach(function(row,index){
    var raw=row.values||[];
    var values=points.map(function(_point,bucket){return Number(raw[bucket]||0);});
    var total=Number(row.total_cost_usd||0);
    if(total<=0)return;
    var others=!!row.others;
    series.push({
      key:row.key||(others?'others':''),
      label:others?t('cost.others'):(byKey?breakdownKeyLabel(row.key,data):(row.key||t('model.untitled'))),
      color:others?cssVar('--t3'):palette[index%palette.length],
      total:total,
      others:others,
      values:values
    });
  });
  return {series:series};
}
function renderCostChart(){
  if(!state.data)return;
  var points=costPoints();
  var total=points.reduce(function(acc,point){return acc+point.cost;},0);
  text('costTrendTotal',formatMoney(total));
  var peak=points.reduce(function(best,point){return point.cost>(best.value||0)?{value:point.cost,at:point.at}:best;},{value:0,at:null});
  var sub=peak.at?t('cost.trendSub',{value:formatMoney(peak.value),date:localeDate(peak.at,{month:'numeric',day:'numeric'})}):'';
  if(!total)sub=t('chart.rangeEmpty');
  text('costScaleNote',scaleNoteText(state.data));

  var resolved=costChartSeries(points,state.data);
  var series=resolved.series;
  var chartTotal=series.reduce(function(acc,entry){return acc+entry.total;},0);
  var notes=[];
  if(resolved.note)notes.push(resolved.note);
  if(series.length&&chartTotal>0){
    var folded=series.reduce(function(acc,entry){return acc+(entry.others?1:0);},0);
    if(folded>0)notes.push(t('cost.breakdownNote',{count:formatTokens(series.length-1)}));
  }
  var unpriced=points.reduce(function(acc,point){return acc+point.unpriced;},0);
  if(unpriced>0)notes.push(t('cost.unpriced',{count:formatTokens(unpriced)}));
  if(notes.length)sub+=(sub?' · ':'')+notes.join(' · ');
  text('costTrendSub',sub);

  // Nothing priced in the window: show the empty state instead of a flat axis.
  var drawPoints=chartTotal>0?points:[];
  drawChart({
    key:'cost',
    svgId:'costChart',
    tipId:'costTip',
    points:drawPoints,
    series:series,
    label:bucketLabel,
    mode:'bars',
    totalFormat:formatMoney,
    axisFormat:function(value){return formatMoney(value);},
    empty:t('cost.empty')
  });
  renderSeriesLegend('costChartLegend',chartTotal>0?series.map(function(entry){
    return {color:entry.color,label:entry.label,value:formatMoney(entry.total),title:entry.label};
  }):[]);
}
function renderCostRankings(){
  if(!state.data)return;
  var models=(state.data.models||[]).slice().filter(function(entry){return Number(entry.cost_usd||0)>0;});
  models.sort(function(a,b){return Number(b.cost_usd||0)-Number(a.cost_usd||0);});
  renderCostRankList('costModelRank','costModelRankSub',models.map(function(entry){
    return {name:entry.model||t('model.untitled'),value:Number(entry.cost_usd||0),meta:t('model.calls',{count:formatTokens(Number(entry.requests||0))}),filterModel:entry.model||''};
  }),'rgba(232,135,31,0.16)');

  var keys=(state.data.api_keys||[]).slice().filter(function(entry){return Number(entry.cost_usd||0)>0;});
  keys.sort(function(a,b){return Number(b.cost_usd||0)-Number(a.cost_usd||0);});
  renderCostRankList('costApiKeyRank','costApiKeyRankSub',keys.map(function(entry){
    return {name:keyLabel(entry),value:Number(entry.cost_usd||0),meta:t('cost.keyCoverage',{priced:formatTokens(Number(entry.priced_requests||0)),unpriced:formatTokens(Number(entry.unpriced_requests||0))}),filterKey:entry.ref,color:keyColor(entry.ref)};
  }),'rgba(63,127,224,0.14)');
}
function renderCostRankList(listId,subId,rows,fillColor){
  var list=document.getElementById(listId);
  var fragment=document.createDocumentFragment();
  if(!rows.length){
    var emptyNode=document.createElement('div');
    emptyNode.className='rank-empty';
    emptyNode.textContent=t('ranking.empty');
    fragment.appendChild(emptyNode);
  }
  var max=rows.length?rows[0].value:0;
  rows.slice(0,8).forEach(function(entry,index){
    var row=document.createElement('button');
    row.type='button';
    row.className='rank-row';
    row.title=t('ranking.filterHint',{name:entry.name});
    var fill=document.createElement('span');
    fill.className='rank-fill';
    fill.style.width=(max?Math.max(2,entry.value/max*100):2)+'%';
    fill.style.background=fillColor;
    row.appendChild(fill);
    var main=document.createElement('span');
    main.className='rank-main';
    var dot=document.createElement('i');
    dot.className='rank-key-dot';
    dot.style.background=entry.color||palette[index%palette.length];
    main.appendChild(dot);
    var label=document.createElement('span');
    label.className='rank-name';
    label.textContent=entry.name;
    main.appendChild(label);
    var meta=document.createElement('span');
    meta.className='rank-meta';
    meta.textContent=entry.meta;
    main.appendChild(meta);
    row.appendChild(main);
    var amount=document.createElement('span');
    amount.className='rank-val';
    amount.textContent=formatMoney(entry.value);
    row.appendChild(amount);
    row.addEventListener('click',function(){
      if(entry.filterModel!==undefined)toggleModelFilter(entry.filterModel);
      else if(entry.filterKey)toggleKeyFilter(entry.filterKey);
    });
    fragment.appendChild(row);
  });
  list.replaceChildren(fragment);
  text(subId,t('cost.rankSub',{count:formatTokens(rows.length)}));
}
function renderCost(){
  renderCostHero();
  renderCostChart();
  renderCostRankings();
}

/* rankings */
function rankValue(entry,metric){
  return metric==='requests'?Number(entry.requests||0):Number(entry.total_tokens||0);
}
function renderRankings(){
  if(!state.data)return;
  var models=(state.data.models||[]).slice();
  models.sort(function(a,b){return rankValue(b,state.rankMetric)-rankValue(a,state.rankMetric);});
  var top=models.slice(0,8);
  var max=top.length?rankValue(top[0],state.rankMetric):0;
  var total=models.reduce(function(acc,entry){return acc+rankValue(entry,state.rankMetric);},0);
  var list=document.getElementById('modelRank');
  var fragment=document.createDocumentFragment();
  if(!top.length){
    var emptyNode=document.createElement('div');
    emptyNode.className='rank-empty';
    emptyNode.textContent=t('ranking.empty');
    fragment.appendChild(emptyNode);
  }
  top.forEach(function(entry,index){
    var name=entry.model||t('model.untitled');
    var value=rankValue(entry,state.rankMetric);
    var share=total?value/total*100:0;
    var row=document.createElement('button');
    row.type='button';
    row.className='rank-row';
    row.title=t('ranking.filterHint',{name:name});
    var fill=document.createElement('span');
    fill.className='rank-fill';
    fill.style.width=(max?Math.max(2,value/max*100):2)+'%';
    fill.style.background='rgba(232,135,31,0.16)';
    row.appendChild(fill);
    var main=document.createElement('span');
    main.className='rank-main';
    var dot=document.createElement('i');
    dot.className='rank-key-dot';
    dot.style.background=palette[index%palette.length];
    main.appendChild(dot);
    var label=document.createElement('span');
    label.className='rank-name';
    label.textContent=name;
    main.appendChild(label);
    var meta=document.createElement('span');
    meta.className='rank-meta';
    meta.textContent=formatPercent(share,share<10?1:0);
    main.appendChild(meta);
    row.appendChild(main);
    var amount=document.createElement('span');
    amount.className='rank-val';
    amount.textContent=state.rankMetric==='requests'?formatTokens(value):compactNumber(value);
    row.appendChild(amount);
    row.addEventListener('click',function(){
      toggleModelFilter(entry.model||'');
    });
    fragment.appendChild(row);
  });
  list.replaceChildren(fragment);
  text('modelRankSub',t('model.count',{count:models.length}));

  var keys=(state.data.api_keys||[]).slice();
  keys.sort(function(a,b){return Number(b.total_tokens||0)-Number(a.total_tokens||0);});
  var keyList=document.getElementById('apiKeyRank');
  var keyFragment=document.createDocumentFragment();
  var activeKeys=keys.filter(function(entry){return Number(entry.total_tokens||0)>0||Number(entry.requests||0)>0;});
  if(!activeKeys.length){
    var keyEmpty=document.createElement('div');
    keyEmpty.className='rank-empty';
    keyEmpty.textContent=t('ranking.empty');
    keyFragment.appendChild(keyEmpty);
  }
  var keyMax=activeKeys.length?Number(activeKeys[0].total_tokens||0):0;
  activeKeys.slice(0,8).forEach(function(entry,index){
    var name=keyLabel(entry);
    var value=Number(entry.total_tokens||0);
    var row=document.createElement('button');
    row.type='button';
    row.className='rank-row';
    row.title=t('ranking.filterHint',{name:name});
    var fill=document.createElement('span');
    fill.className='rank-fill';
    fill.style.width=(keyMax?Math.max(2,value/keyMax*100):2)+'%';
    fill.style.background='rgba(63,127,224,0.14)';
    row.appendChild(fill);
    var main=document.createElement('span');
    main.className='rank-main';
    var dot=document.createElement('i');
    dot.className='rank-key-dot';
    dot.style.background=keyColor(entry.ref);
    main.appendChild(dot);
    var label=document.createElement('span');
    label.className='rank-name';
    label.textContent=name;
    main.appendChild(label);
    var meta=document.createElement('span');
    meta.className='rank-meta';
    meta.textContent=t('model.calls',{count:formatTokens(Number(entry.requests||0))});
    main.appendChild(meta);
    row.appendChild(main);
    var amount=document.createElement('span');
    amount.className='rank-val';
    amount.textContent=compactNumber(value);
    row.appendChild(amount);
    row.addEventListener('click',function(){
      toggleKeyFilter(entry.ref);
    });
    keyFragment.appendChild(row);
  });
  keyList.replaceChildren(keyFragment);
  text('apiKeyRankSub',t('apiKey.count',{count:activeKeys.length}));
}
function keyColor(ref){
  var hash=String(ref||'');
  var total=0;
  for(var i=0;i<hash.length;i++)total=(total*31+hash.charCodeAt(i))>>>0;
  return palette[total%palette.length];
}
function apiKeyName(ref){
  if(!ref)return '';
  var list=(state.catalog&&state.catalog.api_keys)||[];
  for(var i=0;i<list.length;i++){
    if(list[i].ref===ref)return keyLabel(list[i]);
  }
  return keyLabel({ref:ref});
}
function keyLabel(entry){
  if(entry.label)return entry.label;
  var ref=String(entry.ref||'');
  var parts=ref.split(':');
  var tail=parts.length>1?parts[1].slice(0,6):ref.slice(0,6);
  return 'Key #'+tail;
}

/* -------------------------------------------------------------- models tab */
// The models tab lists every model the plugin has metered (plus previously
// observed models and models that only carry a manual price) with both TPS
// variants, average TTFT and the estimated list-price cost. Costs are derived
// on every request, so editing a price immediately rewrites history.
function modelBadge(pricing){
  var source=pricing&&pricing.price_source?pricing.price_source:'none';
  if(source==='override')return {label:t('models.badgeOverride'),className:'badge badge-override'};
  if(source==='catalog'){
    if(pricing.free)return {label:t('models.badgeFree'),className:'badge badge-free'};
    return {label:t('models.badgeCatalog'),className:'badge badge-catalog'};
  }
  return {label:t('models.badgeNone'),className:'badge badge-none'};
}
function catalogSourceLabel(source){
  if(source==='remote')return t('models.sourceRemote');
  if(source==='cache')return t('models.sourceCache');
  return t('models.sourceEmbedded');
}
function renderCatalogNote(data){
  var pricing=(data&&data.pricing)||{};
  var note=t('models.catalogNote',{
    source:catalogSourceLabel(pricing.source),
    date:pricing.generated_at?formatClock(pricing.generated_at):'—',
    models:formatTokens(pricing.models||0),
    priced:formatTokens(pricing.priced_models||0),
    overrides:formatTokens(pricing.overrides||0)
  });
  if(pricing.remote_enabled===false)note+=' · '+t('models.catalogDisabled');
  else if(pricing.stale)note+=' · '+t('models.catalogStale');
  if(pricing.last_error)note+=' · '+t('models.catalogError',{message:pricing.last_error});
  text('catalogNote',note);
}
// renderTTFTNote explains where first-token latency comes from. The host does
// not report TTFT for plugin executors, so the models view links the sibling
// opencode-go plugin's hourly statistics in and says so explicitly.
function renderTTFTNote(data){
  var node=document.getElementById('ttftNote');
  if(!node)return;
  var supplement=(data&&data.ttft_supplement)||{};
  var parts=[];
  if(Number(supplement.samples||0)>0){
    parts.push(t('models.ttftLinkedNote',{
      models:formatTokens(Number(supplement.models||0)),
      samples:formatTokens(Number(supplement.samples||0)),
      date:supplement.written_at?formatClock(supplement.written_at):'—'
    }));
  }else if(supplement.enabled&&!supplement.available){
    parts.push(t('models.ttftLinkUnavailable'));
  }
  var missingModels=0;
  var missingRequests=0;
  (data&&data.models||[]).forEach(function(row){
    if(!row.in_range)return;
    var samples=Number(row.ttft_samples||0);
    var requests=Number(row.requests||0);
    if(samples>0||requests<=0)return;
    missingModels++;
    missingRequests+=requests;
  });
  if(missingModels>0){
    parts.push(t('models.ttftMissingNote',{models:formatTokens(missingModels),requests:formatTokens(missingRequests)}));
  }
  if(!parts.length){
    node.hidden=true;
    node.textContent='';
    return;
  }
  node.textContent=parts.join(' · ');
  node.hidden=false;
}
function matchesModelSearch(row){
  if(!state.modelSearch)return true;
  var needle=state.modelSearch.toLowerCase();
  var haystack=[row.model,row.pricing&&row.pricing.catalog_provider,row.pricing&&row.pricing.catalog_model]
    .filter(function(value){return !!value;}).join(' ').toLowerCase();
  return haystack.indexOf(needle)>=0;
}
// gearIcon draws the per-row price button with DOM nodes only.
function gearIcon(){
  var svg=svgEl('svg',{viewBox:'0 0 24 24','aria-hidden':'true'});
  svg.appendChild(svgEl('circle',{cx:'12',cy:'12',r:'3.2'}));
  svg.appendChild(svgEl('path',{d:'M4 7h3.2l1.2-2h7.2l1.2 2H20v10h-3.2l-1.2 2H8.4l-1.2-2H4z'}));
  return svg;
}
function renderModels(){
  var data=state.modelsData;
  if(!data)return;
  var summary=data.summary||{};
  var rows=data.models||[];
  var visible=rows.filter(matchesModelSearch);
  text('modelsTotalCost',formatMoney(summary.cost_usd));
  var costNote=t('cost.sub',{priced:formatTokens(summary.priced_models||0),unpriced:formatTokens(summary.unpriced_models||0)});
  if(Number(summary.unpriced_requests||0)>0)costNote+=' · '+t('cost.unpriced',{count:formatTokens(summary.unpriced_requests)});
  text('modelsCostSub',costNote);
  var cost=Number(summary.cost_usd||0);
  document.getElementById('modelsCostFill').style.width=(cost>0?Math.min(100,Math.max(4,Math.log10(cost+1)*34)):0)+'%';
  var inRange=rows.filter(function(row){return row.in_range;}).length;
  text('modelsCount',formatTokens(rows.length));
  text('modelsCountSub',t('models.countSub',{count:formatTokens(inRange)}));
  document.getElementById('modelsCountFill').style.width=(rows.length?Math.min(100,inRange/rows.length*100):0)+'%';
  text('modelsSub',t('models.listSub',{count:formatTokens(visible.length),total:formatTokens(rows.length)}));
  renderCatalogNote(data);
  renderTTFTNote(data);

  var body=document.getElementById('modelList');
  var fragment=document.createDocumentFragment();
  if(!visible.length){
    var emptyRow=document.createElement('tr');
    var emptyCell=document.createElement('td');
    emptyCell.colSpan=9;
    emptyCell.className='rank-empty';
    emptyCell.textContent=t('models.searchEmpty');
    emptyRow.appendChild(emptyCell);
    fragment.appendChild(emptyRow);
  }
  visible.forEach(function(row){
    var badge=modelBadge(row.pricing);
    var tr=document.createElement('tr');
    if(!row.in_range)tr.className='row-muted';

    var modelCell=document.createElement('td');
    var modelWrap=document.createElement('div');
    modelWrap.className='model-cell';
    var modelText=document.createElement('span');
    modelText.className='model-text';
    modelText.textContent=row.model||t('model.untitled');
    modelText.title=row.model||'';
    modelWrap.appendChild(modelText);
    var badgeNode=document.createElement('span');
    badgeNode.className=badge.className;
    badgeNode.textContent=badge.label;
    modelWrap.appendChild(badgeNode);
    modelCell.appendChild(modelWrap);
    tr.appendChild(modelCell);

    var providerCell=document.createElement('td');
    providerCell.className='col-key';
    var provider=document.createElement('span');
    provider.className='provider-tag';
    provider.textContent=(row.pricing&&row.pricing.catalog_provider)||t('models.unknownProvider');
    if(row.pricing&&row.pricing.catalog_model)provider.title=row.pricing.catalog_model;
    providerCell.appendChild(provider);
    tr.appendChild(providerCell);

    [['num',formatTPS(row.generation_tps)],['num',formatTPS(row.end_to_end_tps)]].forEach(function(pair){
      var td=document.createElement('td');
      td.className=pair[0];
      td.textContent=pair[1];
      tr.appendChild(td);
    });
    var ttftCell=document.createElement('td');
    ttftCell.className='num';
    var supplemented=Number(row.supplemented_ttft_samples||0);
    ttftCell.textContent=formatTTFT(row.average_ttft_ns);
    if(supplemented>0){
      var linkedMark=document.createElement('sup');
      linkedMark.className='ttft-linked';
      linkedMark.textContent='*';
      linkedMark.title=t('models.ttftLinked',{count:formatTokens(supplemented)});
      ttftCell.appendChild(linkedMark);
      ttftCell.title=linkedMark.title;
    }
    if(Number(row.ttft_samples||0)>0){
      var sampleNote=t('models.ttftSamples',{samples:formatTokens(Number(row.ttft_samples||0)),requests:formatTokens(Number(row.requests||0))});
      ttftCell.title=ttftCell.title?ttftCell.title+' · '+sampleNote:sampleNote;
    }
    tr.appendChild(ttftCell);

    var tokensCell=document.createElement('td');
    tokensCell.className='num';
    if(row.in_range){
      tokensCell.textContent=formatTokens(row.total_tokens);
      tokensCell.title=t('model.calls',{count:formatTokens(row.requests||0)});
    }else{
      tokensCell.textContent=t('models.noUsage');
      tokensCell.title=t('models.noUsageHint');
    }
    tr.appendChild(tokensCell);

    var costCell=document.createElement('td');
    costCell.className='num';
    costCell.textContent=row.priced?formatMoney(row.cost_usd):'—';
    tr.appendChild(costCell);

    var priceCell=document.createElement('td');
    var priceWrap=document.createElement('span');
    priceWrap.className='price-cell';
    var priceNode=document.createElement('span');
    priceNode.className='model-price';
    var pair=formatRatePair(row.pricing&&row.pricing.price);
    priceNode.textContent=pair||t('models.noPrice');
    if(row.pricing&&row.pricing.override){
      var overridePair=formatRatePair(row.pricing.override);
      if(overridePair)priceNode.title=t('models.overrideHint',{value:overridePair});
    }
    priceWrap.appendChild(priceNode);
    var plan=timePlanOf(row.pricing);
    if(plan.tiers.length){
      var peakBadge=document.createElement('span');
      peakBadge.className='badge badge-peak';
      peakBadge.textContent=t('models.timePlanBadge');
      peakBadge.title=timePlanSummary(plan);
      priceWrap.appendChild(peakBadge);
    }
    priceCell.appendChild(priceWrap);
    tr.appendChild(priceCell);

    var actionCell=document.createElement('td');
    actionCell.className='col-action';
    var gear=document.createElement('button');
    gear.type='button';
    gear.className='gear-btn';
    gear.title=t('models.editPrice',{name:row.model||''});
    gear.setAttribute('aria-label',gear.title);
    gear.appendChild(gearIcon());
    gear.addEventListener('click',function(){openPriceDialog(row.model||'');});
    actionCell.appendChild(gear);
    tr.appendChild(actionCell);

    fragment.appendChild(tr);
  });
  body.replaceChildren(fragment);
}
function loadModels(){
  var seq=++state.modelsSeq;
  state.modelsLoading=true;
  var params=new URLSearchParams(queryString(false));
  return api(statsModelsURL+'?'+params.toString()).then(function(data){
    if(seq!==state.modelsSeq)return;
    state.modelsData=data;
    renderModels();
    maybeRefreshCatalog(data);
  }).catch(function(error){
    if(seq===state.modelsSeq)showError(error);
  }).finally(function(){
    if(seq===state.modelsSeq)state.modelsLoading=false;
  });
}
// maybeRefreshCatalog asks the plugin for a fresh catalog once per page load
// when the embedded/cached snapshot has gone stale. The plugin performs the
// download synchronously; the browser never contacts the catalog host itself.
function maybeRefreshCatalog(data){
  var pricing=(data&&data.pricing)||{};
  if(state.catalogRefreshRequested||!pricing.stale||pricing.auto_refresh===false)return;
  state.catalogRefreshRequested=true;
  refreshPricing(true).then(function(){
    if(state.tab==='models')loadModels();
  });
}
function refreshPricing(silent){
  var button=document.getElementById('refreshCatalog');
  if(button)button.classList.add('is-busy');
  return api(modelPricesURL+'?refresh=1',{},40000).then(function(result){
    if(result&&result.ok===false){
      if(!silent)toast(t('models.refreshFailed',{message:(result.pricing&&result.pricing.last_error)||''}));
      return result;
    }
    if(!silent)toast(t('models.refreshDone'));
    return result;
  }).catch(function(error){
    if(!silent)toast(error.message||t('models.refreshFailed',{message:''}));
    return null;
  }).finally(function(){
    if(button)button.classList.remove('is-busy');
  });
}
document.getElementById('refreshCatalog').addEventListener('click',function(){
  toast(t('models.refreshing'));
  refreshPricing(false).then(function(result){
    if(!result)return;
    state.catalogKey='';
    loadModels();
    load();
  });
});
document.getElementById('modelSearch').addEventListener('input',function(){
  state.modelSearch=this.value.trim();
  renderModels();
});
/* --------------------------------------------------------- price dialog */
var priceDialog=document.getElementById('priceDialog');
function findModelRow(model){
  var rows=(state.modelsData&&state.modelsData.models)||[];
  for(var i=0;i<rows.length;i++){
    if(rows[i].model===model)return rows[i];
  }
  return null;
}
function priceFieldValue(value){
  if(value===null||value===undefined)return '';
  return String(value);
}
/* ---------------------------------------------- time-of-use price schedule */
var TIME_TIER_FIELDS=[
  {key:'days',className:'time-days',type:'text'},
  {key:'start',className:'time-start',type:'time'},
  {key:'end',className:'time-end',type:'time'},
  {key:'input',className:'time-input',type:'number'},
  {key:'output',className:'time-output',type:'number'},
  {key:'cache_read',className:'time-cache-read',type:'number'},
  {key:'cache_write',className:'time-cache-write',type:'number'}
];
// timeTierFieldLabel spells every key out so the template/catalog parity test
// can see them; the field table itself stays locale independent.
function timeTierFieldLabel(field){
  if(field.key==='days')return t('models.timePlanDays');
  if(field.key==='start')return t('models.timePlanStart');
  if(field.key==='end')return t('models.timePlanEnd');
  if(field.key==='input')return t('models.priceInput');
  if(field.key==='output')return t('models.priceOutput');
  if(field.key==='cache_read')return t('models.priceCacheRead');
  return t('models.priceCacheWrite');
}
function timePlanOf(pricing){
  return {
    zone:(pricing&&pricing.time_zone)||'',
    tiers:(pricing&&pricing.time_tiers)||[],
    source:(pricing&&pricing.time_plan_source)||''
  };
}
function timePlanSummary(plan){
  var windows=(plan.tiers||[]).map(function(tier){
    var days=(tier.days&&tier.days.length)?tier.days.join(','):'1-7';
    return days+' '+tier.start+'-'+tier.end;
  }).join(' · ');
  var first=(plan.tiers||[])[0]||{};
  var rates=['input','output','cache_read','cache_write'].map(function(key){
    return formatRate(first[key]);
  }).filter(function(value){return !!value;});
  return (plan.zone||'UTC')+' '+windows+(rates.length?' → '+rates.join(' / '):'');
}
function parseClock(value){
  if(!/^\d{2}:\d{2}$/.test(value))return NaN;
  var parts=value.split(':');
  var hour=Number(parts[0]);
  var minute=Number(parts[1]);
  if(hour>23||minute>59)return NaN;
  return hour*60+minute;
}
function clockText(minutes){
  var hour=Math.floor(minutes/60);
  var minute=minutes%60;
  return (hour<10?'0':'')+hour+':'+(minute<10?'0':'')+minute;
}
function makeTimeTierRow(tier){
  var row=document.createElement('div');
  row.className='time-tier-row';
  var head=document.createElement('div');
  head.className='time-tier-head';
  var name=document.createElement('input');
  name.type='text';
  name.className='time-name';
  name.autocomplete='off';
  name.spellcheck=false;
  name.placeholder=t('models.timePlanName');
  name.value=(tier&&tier.name)||'';
  head.appendChild(name);
  var remove=document.createElement('button');
  remove.type='button';
  remove.className='time-remove';
  remove.title=t('models.timePlanRemove');
  remove.setAttribute('aria-label',remove.title);
  remove.textContent='×';
  remove.addEventListener('click',function(){row.remove();});
  head.appendChild(remove);
  row.appendChild(head);
  var grid=document.createElement('div');
  grid.className='time-tier-grid';
  TIME_TIER_FIELDS.forEach(function(field){
    var label=document.createElement('label');
    label.appendChild(document.createTextNode(timeTierFieldLabel(field)));
    var input=document.createElement('input');
    input.type=field.type;
    input.className=field.className;
    input.autocomplete='off';
    if(field.type==='number'){
      input.min='0';
      input.step='0.000001';
      input.inputMode='decimal';
    }
    var value=(tier&&tier[field.key]!==undefined&&tier[field.key]!==null)?tier[field.key]:'';
    if(field.key==='days')value=Array.isArray(value)?value.join(','):value;
    input.value=priceFieldValue(value);
    label.appendChild(input);
    grid.appendChild(label);
  });
  row.appendChild(grid);
  return row;
}
function renderTimePlan(pricing){
  var plan=timePlanOf(pricing);
  document.getElementById('timePlanZone').value=plan.zone||'UTC';
  var list=document.getElementById('timeTierList');
  var fragment=document.createDocumentFragment();
  plan.tiers.forEach(function(tier){fragment.appendChild(makeTimeTierRow(tier));});
  list.replaceChildren(fragment);
  text('timePlanSource',plan.source==='builtin'?t('models.timePlanBuiltin'):(plan.source==='override'?t('models.timePlanOverride'):''));
}
// collectTimePlan mirrors the server validation so an invalid schedule never
// leaves the browser.
function collectTimePlan(){
  var zone=document.getElementById('timePlanZone').value.trim()||'UTC';
  var rows=document.querySelectorAll('#timeTierList .time-tier-row');
  if(!rows.length)return {zone:'',tiers:[]};
  if(zone==='Local')throw new Error(t('models.timePlanInvalid'));
  try{
    new Intl.DateTimeFormat('en',{timeZone:zone});
  }catch(error){
    throw new Error(t('models.timePlanInvalid'));
  }
  var tiers=[];
  var names={};
  var occupied={};
  Array.prototype.forEach.call(rows,function(row){
    var name=row.querySelector('.time-name').value.trim();
    var rawDays=row.querySelector('.time-days').value.trim();
    var days=rawDays?rawDays.split(',').map(function(part){return Number(part.trim());}):[];
    var start=parseClock(row.querySelector('.time-start').value);
    var end=parseClock(row.querySelector('.time-end').value);
    if(!name||names[name]||!isFinite(start)||!isFinite(end)||start===end){
      throw new Error(t('models.timePlanInvalid'));
    }
    if(days.some(function(day){return !Number.isInteger(day)||day<1||day>7;})||new Set(days).size!==days.length){
      throw new Error(t('models.timePlanInvalid'));
    }
    names[name]=true;
    days=days.slice().sort(function(left,right){return left-right;});
    var duration=(end-start+1440)%1440;
    (days.length?days:[1,2,3,4,5,6,7]).forEach(function(day){
      for(var offset=0;offset<duration;offset++){
        var key=((day-1)*1440+start+offset)%10080;
        if(occupied[key])throw new Error(t('models.timePlanOverlap'));
        occupied[key]=true;
      }
    });
    var tier={name:name,days:days,start:clockText(start),end:clockText(end)};
    TIME_TIER_FIELDS.forEach(function(field){
      if(field.type!=='number')return;
      var raw=row.querySelector('.'+field.className).value.trim();
      if(!raw)return;
      var value=Number(raw);
      if(!isFinite(value)||value<0||value>1000000)throw new Error(t('models.priceInvalid'));
      tier[field.key]=value;
    });
    tiers.push(tier);
  });
  if(tiers.length>32)throw new Error(t('models.timePlanInvalid'));
  return {zone:zone,tiers:tiers};
}
function openPriceDialog(model){
  var row=findModelRow(model);
  if(!row)return;
  state.priceModel=model;
  var override=(row.pricing&&row.pricing.override)||{};
  document.getElementById('priceInput').value=priceFieldValue(override.input);
  document.getElementById('priceOutput').value=priceFieldValue(override.output);
  document.getElementById('priceCacheRead').value=priceFieldValue(override.cache_read);
  document.getElementById('priceCacheWrite').value=priceFieldValue(override.cache_write);
  // The effective schedule (manual or bundled) is prefilled so saving rates
  // never silently drops peak pricing.
  renderTimePlan(row.pricing);
  // An existing schedule opens the section so it is visible without a click.
  document.getElementById('timePlan').open=timePlanOf(row.pricing).tiers.length>0;
  var catalog=(row.pricing&&row.pricing.catalog_provider)?t('models.priceCatalogRef',{provider:row.pricing.catalog_provider,model:row.pricing.catalog_model||'',value:formatRatePair(row.pricing.price)||t('models.noPrice')}):t('models.priceNoCatalog');
  text('priceDialogSub',(row.model||t('model.untitled'))+' · '+catalog);
  text('priceError','');
  document.getElementById('priceReset').hidden=!(row.pricing&&row.pricing.price_source==='override');
  if(typeof priceDialog.showModal==='function')priceDialog.showModal();
  // Focus the first field, but never steal focus from an operator who already
  // moved into another field of this dialog.
  setTimeout(function(){
    var active=document.activeElement;
    if(active&&priceDialog.contains(active))return;
    document.getElementById('priceInput').focus();
  },40);
}
function closePriceDialog(){
  if(priceDialog&&priceDialog.open)priceDialog.close();
  state.priceModel='';
}
var PRICE_FIELDS=[['input','priceInput'],['output','priceOutput'],['cache_read','priceCacheRead'],['cache_write','priceCacheWrite']];
function readPriceFields(){
  var values={};
  var hasValue=false;
  var invalid=false;
  PRICE_FIELDS.forEach(function(field){
    var raw=document.getElementById(field[1]).value.trim();
    if(!raw)return;
    var value=Number(raw);
    if(!isFinite(value)||value<0){invalid=true;return;}
    hasValue=true;
    values[field[0]]=value;
  });
  return {values:values,hasValue:hasValue,invalid:invalid};
}
// applyPriceResult refreshes both tabs because a price change rewrites every
// derived cost, including the ones already on screen.
function applyPriceResult(){
  state.catalogKey='';
  loadModels();
  load();
}
function savePrice(){
  var model=state.priceModel;
  if(!model)return;
  var parsed=readPriceFields();
  if(parsed.invalid){
    text('priceError',t('models.priceInvalid'));
    return;
  }
  var schedule;
  try{
    schedule=collectTimePlan();
  }catch(error){
    text('priceError',error.message||t('models.timePlanInvalid'));
    return;
  }
  // Clearing every rate and every tier restores the list price.
  if(!parsed.hasValue&&!schedule.tiers.length){
    api(modelPricesURL+'?'+new URLSearchParams({reset:'1',model:model}).toString()).then(function(payload){
      if(payload&&payload.error)throw new Error(payload.error);
      closePriceDialog();
      toast(t('models.resetDone'));
      applyPriceResult();
    }).catch(function(error){
      text('priceError',error.message||t('models.saveFailed'));
    });
    return;
  }
  var price=parsed.values;
  if(schedule.tiers.length){
    price.time_zone=schedule.zone;
    price.time_tiers=schedule.tiers;
  }
  api(modelPricesURL,{
    method:'PUT',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({model:model,price:price})
  }).then(function(payload){
    if(payload&&payload.error)throw new Error(payload.error);
    closePriceDialog();
    toast(t('models.saved'));
    applyPriceResult();
  }).catch(function(error){
    text('priceError',error.message||t('models.saveFailed'));
  });
}
document.getElementById('priceSave').addEventListener('click',savePrice);
document.getElementById('timeTierAdd').addEventListener('click',function(){
  document.getElementById('timeTierList').appendChild(makeTimeTierRow(null));
});
document.getElementById('priceCancel').addEventListener('click',closePriceDialog);
priceDialog.addEventListener('cancel',function(event){event.preventDefault();closePriceDialog();});
document.getElementById('priceReset').addEventListener('click',function(){
  var model=state.priceModel;
  if(!model)return;
  api(modelPricesURL+'?'+new URLSearchParams({reset:'1',model:model}).toString()).then(function(){
    closePriceDialog();
    toast(t('models.resetDone'));
    state.catalogKey='';
    loadModels();
    load();
  }).catch(function(error){
    text('priceError',error.message||t('models.saveFailed'));
  });
});

/* ------------------------------------------------------------------ tabs */
function activateTab(name){
  state.tab=name;
  clearError();
  document.querySelectorAll('.tab').forEach(function(tab){
    tab.classList.toggle('is-active',tab.getAttribute('data-tab')===name);
  });
  document.querySelectorAll('.panel').forEach(function(panel){
    panel.classList.toggle('is-active',panel.id==='panel'+(name.charAt(0).toUpperCase()+name.slice(1)));
  });
  if(name==='requests')loadRequests();
  if(name==='models')loadModels();
  if(name==='usage'||name==='cost'){
    // Re-render charts so they pick up the width that was hidden while inactive.
    requestAnimationFrame(renderCharts);
  }
}
document.querySelectorAll('.tab').forEach(function(tab){
  tab.addEventListener('click',function(){activateTab(this.getAttribute('data-tab'));});
});

/* --------------------------------------------------------------- filters */
// customRangeLabel renders a half-open window as inclusive local dates when it
// covers whole days, so "10/17 00:00 → 10/18 00:00" reads as "10/17".
function customRangeLabel(startValue,endValue){
  var start=new Date(startValue);
  var end=new Date(endValue);
  var dayMs=864e5;
  var midnight=start.getHours()===0&&start.getMinutes()===0&&start.getSeconds()===0&&start.getMilliseconds()===0;
  if(midnight&&end.getTime()>start.getTime()&&(end.getTime()-start.getTime())%dayMs===0){
    var last=new Date(end.getTime()-1);
    var short={month:'numeric',day:'numeric'};
    if(last.getFullYear()===start.getFullYear()&&last.getMonth()===start.getMonth()&&last.getDate()===start.getDate()){
      return localeDate(start,short);
    }
    return localeDate(start,short)+' - '+localeDate(last,short);
  }
  var withTime={month:'numeric',day:'numeric',hour:'2-digit',minute:'2-digit',hour12:false};
  return localeDate(start,withTime)+' - '+localeDate(end,withTime);
}
function updateChips(){
  var rangeChip=document.getElementById('rangeButton');
  var rangeLabel=t('range.lastThirtyDays');
  if(state.range.mode==='today')rangeLabel=t('range.today');
  else if(state.range.mode==='yesterday')rangeLabel=t('range.yesterday');
  else if(state.range.mode==='last_7_days')rangeLabel=t('range.lastSevenDays');
  else if(state.range.mode==='last_30_days')rangeLabel=t('range.lastThirtyDays');
  else if(state.range.start&&state.range.end){
    rangeLabel=customRangeLabel(state.range.start,state.range.end);
  }
  text('rangeValue',rangeLabel);
  var customRange=state.range.mode==='custom'||state.range.mode==='today'||state.range.mode==='yesterday';
  rangeChip.classList.toggle('is-active',customRange);

  var modelChip=document.getElementById('modelButton');
  text('modelValue',state.models.length?t('filter.modelsSelected',{count:state.models.length}):t('filter.all'));
  modelChip.classList.toggle('is-active',state.models.length>0);

  var keyChip=document.getElementById('apiKeyButton');
  text('apiKeyValue',state.keys.length?t('filter.keysSelected',{count:state.keys.length}):t('filter.all'));
  keyChip.classList.toggle('is-active',state.keys.length>0);
}

function toggleModelFilter(model){
  var index=state.models.indexOf(model);
  if(index>=0)state.models.splice(index,1);
  else state.models.push(model);
  applyFilters();
}
function toggleKeyFilter(ref){
  var index=state.keys.indexOf(ref);
  if(index>=0)state.keys.splice(index,1);
  else state.keys.push(ref);
  applyFilters();
}
function applyFilters(){
  updateChips();
  state.requests.offset=0;
  load();
}

/* menus */
function initializeDropdownSurface(menu){
  if(!menu||menu._dropdownInitialized)return;
  menu._dropdownInitialized=true;
  menu.addEventListener('transitionend',function(event){
    if(event.target===menu&&event.propertyName==='opacity'){
      if(menu.classList.contains('is-closing')){
        menu.classList.remove('is-closing');
        menu.hidden=true;
        var callback=menu._afterClose;
        if(typeof callback==='function'){menu._afterClose=null;callback();}
      }else if(menu.classList.contains('is-opening')){
        menu.classList.remove('is-opening');
      }
    }
  });
}
var activeDropdown=null;
var dropdownTimers=new WeakMap();
function openDropdownSurface(menu,button,position){
  initializeDropdownSurface(menu);
  if(activeDropdown&&activeDropdown!==menu)closeDropdownSurface(activeDropdown,null,false);
  var timer=dropdownTimers.get(menu);
  if(timer){clearTimeout(timer);dropdownTimers.delete(menu);}
  menu.hidden=false;
  menu.classList.remove('is-closing');
  menu.classList.add('is-opening');
  activeDropdown=menu;
  menu._dropdownButton=button;
  button.setAttribute('aria-expanded','true');
  if(position)position();
  requestAnimationFrame(function(){
    menu.classList.remove('is-opening');
    menu.classList.add('is-open');
  });
  function onDocPointer(event){
    if(menu.contains(event.target)||button.contains(event.target))return;
    closeDropdownSurface(menu,button,false);
    document.removeEventListener('pointerdown',onDocPointer,true);
  }
  setTimeout(function(){document.addEventListener('pointerdown',onDocPointer,true);},0);
}
function closeDropdownSurface(menu,button,restoreFocus){
  if(!menu||menu.hidden)return;
  if(menu.classList.contains('is-closing'))return;
  menu.classList.remove('is-open');
  menu.classList.add('is-closing');
  if(button)button.setAttribute('aria-expanded','false');
  if(activeDropdown===menu)activeDropdown=null;
  var reducer=window.matchMedia&&window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  menu._afterClose=function(){
    if(restoreFocus&&button&&button.isConnected)button.focus();
  };
  if(reducer){
    menu.classList.remove('is-closing');
    menu.hidden=true;
    if(menu._afterClose){menu._afterClose();menu._afterClose=null;}
    return;
  }
  var timer=setTimeout(function(){
    menu.classList.remove('is-closing');
    menu.hidden=true;
    if(menu._afterClose){menu._afterClose();menu._afterClose=null;}
    dropdownTimers.delete(menu);
  },200);
  dropdownTimers.set(menu,timer);
}
function positionMenu(menu,button){
  var rect=button.getBoundingClientRect();
  var margin=10;
  var width=Math.min(320,window.innerWidth-margin*2);
  menu.style.width=width+'px';
  menu.style.left=Math.max(margin,Math.min(rect.left,window.innerWidth-width-margin))+'px';
  menu.style.maxHeight=Math.min(420,window.innerHeight-margin*2)+'px';
  var below=window.innerHeight-rect.bottom-margin;
  menu.style.top=(rect.bottom+6)+'px';
  if(below<220){
    var height=menu.offsetHeight||320;
    menu.style.top=Math.max(margin,rect.top-6-Math.min(height,window.innerHeight-margin*2))+'px';
  }
}

function buildModelMenu(){
  ensureCatalog().then(function(){
    var menu=document.getElementById('modelMenu');
    var fragment=document.createDocumentFragment();
    var search=document.createElement('input');
    search.className='menu-search';
    search.type='search';
    search.placeholder=t('filter.searchModels');
    search.setAttribute('aria-label',t('filter.searchModels'));
    fragment.appendChild(search);
    var all=document.createElement('label');
    all.className='menu-option is-all';
    var allBox=document.createElement('input');
    allBox.type='checkbox';
    allBox.checked=state.models.length===0;
    allBox.addEventListener('change',function(){
      state.models=[];
      applyFilters();
      buildModelMenu();
    });
    all.appendChild(allBox);
    var allLabel=document.createElement('span');
    allLabel.textContent=t('filter.allModels');
    all.appendChild(allLabel);
    fragment.appendChild(all);
    var list=document.createElement('div');
    fragment.appendChild(list);
    var catalog=(state.catalog&&state.catalog.models)||[];
    var names=catalog.map(function(entry){return entry.model||'';});
    renderModelOptions(list,names,'');
    search.addEventListener('input',function(){
      renderModelOptions(list,names,this.value.trim().toLowerCase());
    });
    menu.replaceChildren(fragment);
    function renderModelOptions(host,items,needle){
      var rows=document.createDocumentFragment();
      var shown=0;
      items.forEach(function(model){
        var name=model||t('model.untitled');
        if(needle&&name.toLowerCase().indexOf(needle)<0)return;
        shown+=1;
        var option=document.createElement('label');
        option.className='menu-option';
        var box=document.createElement('input');
        box.type='checkbox';
        box.checked=state.models.indexOf(model)>=0;
        box.addEventListener('change',function(){
          var index=state.models.indexOf(model);
          if(this.checked&&index<0)state.models.push(model);
          else if(!this.checked&&index>=0)state.models.splice(index,1);
          applyFilters();
        });
        option.appendChild(box);
        var label=document.createElement('span');
        label.textContent=name;
        option.appendChild(label);
        rows.appendChild(option);
      });
      if(!shown){
        var empty=document.createElement('div');
        empty.className='menu-empty';
        empty.textContent=t('filter.noMatches');
        rows.appendChild(empty);
      }
      host.replaceChildren(rows);
    }
  }).catch(function(error){
    showError(error);
  });
}

function buildApiKeyMenu(){
  ensureCatalog().then(function(){
    var menu=document.getElementById('apiKeyMenu');
    var fragment=document.createDocumentFragment();
    var all=document.createElement('label');
    all.className='menu-option is-all';
    var allBox=document.createElement('input');
    allBox.type='checkbox';
    allBox.checked=state.keys.length===0;
    allBox.addEventListener('change',function(){
      state.keys=[];
      applyFilters();
      buildApiKeyMenu();
    });
    all.appendChild(allBox);
    var allLabel=document.createElement('span');
    allLabel.textContent=t('apiKey.filterAll');
    all.appendChild(allLabel);
    fragment.appendChild(all);
    var keys=(state.catalog&&state.catalog.api_keys)||[];
    if(!keys.length){
      var empty=document.createElement('div');
      empty.className='menu-empty';
      empty.textContent=t('apiKey.filterHint');
      fragment.appendChild(empty);
    }
    keys.forEach(function(entry){
      var option=document.createElement('label');
      option.className='menu-option';
      var box=document.createElement('input');
      box.type='checkbox';
      box.checked=state.keys.indexOf(entry.ref)>=0;
      box.addEventListener('change',function(){
        var index=state.keys.indexOf(entry.ref);
        if(this.checked&&index<0)state.keys.push(entry.ref);
        else if(!this.checked&&index>=0)state.keys.splice(index,1);
        applyFilters();
      });
      option.appendChild(box);
      var dot=document.createElement('i');
      dot.className='rank-key-dot';
      dot.style.background=keyColor(entry.ref);
      option.appendChild(dot);
      var label=document.createElement('span');
      label.textContent=keyLabel(entry);
      option.appendChild(label);
      var amount=document.createElement('em');
      amount.textContent=compactNumber(entry.total_tokens||0);
      option.appendChild(amount);
      fragment.appendChild(option);
    });
    menu.replaceChildren(fragment);
  }).catch(function(error){
    showError(error);
  });
}

var modelButton=document.getElementById('modelButton');
var modelMenu=document.getElementById('modelMenu');
modelButton.addEventListener('click',function(){
  if(modelMenu.hidden||modelMenu.classList.contains('is-closing')){
    buildModelMenu();
    openDropdownSurface(modelMenu,modelButton,function(){positionMenu(modelMenu,modelButton);});
  }else{
    closeDropdownSurface(modelMenu,modelButton,true);
  }
});
var apiKeyButton=document.getElementById('apiKeyButton');
var apiKeyMenu=document.getElementById('apiKeyMenu');
apiKeyButton.addEventListener('click',function(){
  if(apiKeyMenu.hidden||apiKeyMenu.classList.contains('is-closing')){
    buildApiKeyMenu();
    openDropdownSurface(apiKeyMenu,apiKeyButton,function(){positionMenu(apiKeyMenu,apiKeyButton);});
  }else{
    closeDropdownSurface(apiKeyMenu,apiKeyButton,true);
  }
});
window.addEventListener('resize',function(){
  if(activeDropdown&&activeDropdown._dropdownButton){
    positionMenu(activeDropdown,activeDropdown._dropdownButton);
  }
});

/* ------------------------------------------------------------ range dialog */
var rangeDialog=document.getElementById('rangeDialog');
var customStart=document.getElementById('customStart');
var customEnd=document.getElementById('customEnd');
function toLocalInput(date){
  if(!date)return '';
  var pad=function(value){return String(value).padStart(2,'0');};
  return date.getFullYear()+'-'+pad(date.getMonth()+1)+'-'+pad(date.getDate())+'T'+pad(date.getHours())+':'+pad(date.getMinutes());
}
function loadRangeDialog(){
  var window_=currentWindow();
  var start=window_.start||new Date(Date.now()-30*864e5);
  var end=window_.end||new Date();
  customStart.value=toLocalInput(start);
  customEnd.value=toLocalInput(end);
  text('rangeError','');
  document.querySelectorAll('#presetList .preset').forEach(function(button){
    button.classList.toggle('is-active',button.getAttribute('data-mode')===state.range.mode);
  });
}
document.getElementById('rangeButton').addEventListener('click',function(){
  loadRangeDialog();
  if(typeof rangeDialog.showModal==='function')rangeDialog.showModal();
});
document.getElementById('rangeCancel').addEventListener('click',function(){rangeDialog.close();});
document.querySelectorAll('#presetList .preset').forEach(function(button){
  button.addEventListener('click',function(){
    var mode=this.getAttribute('data-mode');
    state.range={mode:mode,start:null,end:null};
    rangeDialog.close();
    updateChips();
    state.catalogKey='';
    state.requests.offset=0;
    scheduleDashboardPreferencesSave();
    load();
  });
});
document.getElementById('rangeApply').addEventListener('click',function(){
  var start=customStart.value?new Date(customStart.value):null;
  var end=customEnd.value?new Date(customEnd.value):null;
  if(!start||!end||start.getTime()>=end.getTime()){
    text('rangeError',t('range.invalidTimeRange'));
    return;
  }
  state.range={mode:'custom',start:start.toISOString(),end:end.toISOString()};
  rangeDialog.close();
  updateChips();
  state.catalogKey='';
  state.requests.offset=0;
  scheduleDashboardPreferencesSave();
  load();
});

/* ---------------------------------------------------------------- catalog */
function ensureCatalog(){
  var key=rangeKey();
  if(state.catalog&&state.catalogKey===key)return Promise.resolve(state.catalog);
  return api(statsInitialURL+'?'+queryString(false)).then(function(data){
    state.catalog=data;
    state.catalogKey=key;
    return data;
  });
}

/* -------------------------------------------------------------------- load */
function showError(error){
  if(error&&error.sessionRejected)return;
  text('error',error&&error.message?error.message:'');
  document.getElementById('refreshButton').classList.remove('is-busy');
}
function clearError(){
  text('error','');
}
function load(){
  var seq=++state.seq;
  var button=document.getElementById('refreshButton');
  button.classList.add('is-busy');
  clearError();
  return api(statsInitialURL+'?'+queryString(true)).then(function(data){
    if(seq!==state.seq)return;
    state.data=data;
    if(!hasFilters()){
      // An unfiltered response already carries every option the menus need.
      state.catalog=data;
      state.catalogKey=rangeKey();
    }
    refreshSeriesColors();
    updateChips();
    renderHero();
    renderCharts();
    renderRankings();
    renderCost();
    if(state.tab==='requests')loadRequests();
    if(state.tab==='models')loadModels();
  }).catch(function(error){
    if(seq===state.seq)showError(error);
  }).finally(function(){
    if(seq===state.seq)button.classList.remove('is-busy');
  });
}

/* --------------------------------------------------------- request log tab */
function loadRequests(){
  var params=new URLSearchParams(queryString(true));
  params.set('offset',String(state.requests.offset));
  params.set('limit',String(state.requests.limit));
  if(state.requests.result)params.set('result',state.requests.result);
  state.requests.loading=true;
  return api(requestsURL+'?'+params.toString()).then(function(page){
    state.requests.total=Number(page.total||0);
    renderRequests(page);
  }).catch(function(error){
    showError(error);
  }).finally(function(){
    state.requests.loading=false;
  });
}
function renderRequests(page){
  var rows=document.getElementById('requestRows');
  var fragment=document.createDocumentFragment();
  (page.items||[]).forEach(function(item){
    var tr=document.createElement('tr');
    var time=document.createElement('td');
    time.textContent=localeDate(item.time,{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false});
    tr.appendChild(time);
    var model=document.createElement('td');
    model.className='log-model';
    model.textContent=item.model||t('model.untitled');
    model.title=[item.model||'',item.provider||'',item.source||''].filter(function(value){return !!value;}).join(' · ');
    tr.appendChild(model);
    var key=document.createElement('td');
    key.className='col-key';
    key.textContent=item.api_key||apiKeyName(item.api_key_ref||'');
    tr.appendChild(key);
    [
      ['num',formatTokens(item.input_tokens)],
      ['num',formatTokens(item.output_tokens)],
      ['num',formatTokens(item.reasoning_tokens)],
      ['num',formatTokens(item.cache_read_tokens)],
      ['num',formatTokens(item.cache_creation_tokens)],
      ['num',formatTokens(item.total_tokens)]
    ].forEach(function(pair){
      var td=document.createElement('td');
      td.className=pair[0];
      td.textContent=pair[1];
      tr.appendChild(td);
    });
    [
      ['num',formatTTFT(item.ttft_ns)],
      ['num',formatDuration(item.latency_ns)],
      ['num',formatTPS(item.tps)],
      ['num',formatTPS(item.end_to_end_tps)]
    ].forEach(function(pair){
      var td=document.createElement('td');
      td.className=pair[0];
      td.textContent=pair[1];
      tr.appendChild(td);
    });
    // The persisted Result string is localized on the server; Failed and
    // FailureStatus are the stable fields the dashboard must branch on.
    var failed=!!item.failed;
    var result=document.createElement('td');
    result.className='status '+(failed?'bad':'ok');
    var dot=document.createElement('i');
    dot.style.background=failed?'var(--bad)':'var(--ok)';
    result.appendChild(dot);
    var label=document.createElement('span');
    var status=Number(item.failure_status||0);
    label.textContent=failed?(status>0?t('result.failedStatus',{status:formatTokens(status)}):t('result.failed')):t('result.success');
    result.appendChild(label);
    tr.appendChild(result);
    fragment.appendChild(tr);
  });
  rows.replaceChildren(fragment);
  var total=state.requests.total;
  var start=total?state.requests.offset+1:0;
  var end=Math.min(total,state.requests.offset+state.requests.limit);
  text('requestPageInfo',t('pagination.page',{start:start,end:end,total:total}));
  var rangeNode=document.getElementById('rangeValue');
  text('requestLogSub',t('status.range',{range:rangeNode?rangeNode.textContent:''}));
  document.getElementById('prevPage').disabled=state.requests.offset<=0;
  document.getElementById('nextPage').disabled=end>=total;
}
document.getElementById('prevPage').addEventListener('click',function(){
  state.requests.offset=Math.max(0,state.requests.offset-state.requests.limit);
  loadRequests();
});
document.getElementById('nextPage').addEventListener('click',function(){
  if(state.requests.offset+state.requests.limit>=state.requests.total)return;
  state.requests.offset+=state.requests.limit;
  loadRequests();
});
document.querySelectorAll('#resultControl button').forEach(function(button){
  button.addEventListener('click',function(){
    state.requests.result=this.getAttribute('data-result')||'';
    state.requests.offset=0;
    document.querySelectorAll('#resultControl button').forEach(function(other){
      other.classList.toggle('is-active',other===button);
    });
    loadRequests();
  });
});

/* ------------------------------------------------------------- segmented */
document.querySelectorAll('#scaleControl button').forEach(function(button){
  button.addEventListener('click',function(){
    state.scale=this.getAttribute('data-scale')||'auto';
    document.querySelectorAll('#scaleControl button').forEach(function(other){
      other.classList.toggle('is-active',other===button);
    });
    // The axis unit is a server-side aggregation decision, so switching the
    // scale reloads the window instead of regrouping in the browser.
    load();
  });
});
document.querySelectorAll('#costDimensionControl button').forEach(function(button){
  button.addEventListener('click',function(){
    state.costDimension=this.getAttribute('data-cost-dimension')||'model';
    document.querySelectorAll('#costDimensionControl button').forEach(function(other){
      other.classList.toggle('is-active',other===button);
    });
    // Both breakdowns ship with the initial payload, so switching the grouping
    // is a pure redraw with no extra request.
    renderCostChart();
  });
});
document.querySelectorAll('#rankControl button').forEach(function(button){
  button.addEventListener('click',function(){
    state.rankMetric=this.getAttribute('data-rank');
    document.querySelectorAll('#rankControl button').forEach(function(other){
      other.classList.toggle('is-active',other===button);
    });
    renderRankings();
  });
});

/* ----------------------------------------------------------------- export */
function exportCSV(){
  var params=new URLSearchParams(queryString(true));
  params.set('limit','500');
  ensureCatalog().catch(function(){return null;}).then(function(){
    return api(statsGroupsURL+'?'+params.toString());
  }).then(function(page){
    var labels={};
    ((state.catalog&&state.catalog.api_keys)||[]).forEach(function(entry){labels[entry.ref]=keyLabel(entry);});
    var rows=[['Model','Provider','Source','API Key','Requests','Input Tokens','Output Tokens','Cache Read Tokens','Total Tokens']];
    (page.items||[]).forEach(function(item){
      // GroupStats embeds its dimensions flat; tolerate the nested legacy shape too.
      var dimensions=item.dimensions||item;
      var key=dimensions.api_key_ref?labels[dimensions.api_key_ref]||dimensions.api_key_ref:(dimensions.api_key||'');
      rows.push([dimensions.model||'',dimensions.provider||'',dimensions.source||'',key,item.requests,item.input_tokens,item.output_tokens,item.cache_read_tokens,item.total_tokens]);
    });
    var csv=rows.map(function(row){
      return row.map(function(cellValue){return '"'+String(cellValue).replace(/"/g,'""')+'"';}).join(',');
    }).join('\n');
    var blob=new Blob([csv],{type:'text/csv;charset=utf-8'});
    var link=document.createElement('a');
    link.href=URL.createObjectURL(blob);
    link.download='token-usage.csv';
    link.click();
    setTimeout(function(){URL.revokeObjectURL(link.href);},2000);
    toast(t('backup.success'));
  }).catch(function(error){
    showError(error);
  });
}
document.getElementById('exportButton').addEventListener('click',exportCSV);

/* ------------------------------------------------------------- management */
// Destructive operations run on the host-authenticated management API, so they
// need the CLIProxyAPI management key. The key is requested in a dialog for the
// single operation, handed to the request, and then dropped: it is never kept in
// a variable, the DOM, or browser storage.
var keyDialog=document.getElementById('keyDialog');
var keyDialogRequest=null;

function keyDialogCanceled(){return new Error('key-dialog-canceled');}
function isKeyDialogCanceled(error){return error&&error.message==='key-dialog-canceled';}

function closeKeyDialog(){
  if(keyDialog&&keyDialog.open)keyDialog.close();
  if(keyDialogRequest){
    var pending=keyDialogRequest;
    keyDialogRequest=null;
    pending.reject(keyDialogCanceled());
  }
  var input=document.getElementById('keyDialogInput');
  if(input)input.value='';
}

function requestManagementKey(title,description){
  return new Promise(function(resolve,reject){
    if(keyDialogRequest)closeKeyDialog();
    text('keyDialogTitle',title||t('data.keyTitle'));
    text('keyDialogSub',description||t('data.keyPrompt'));
    text('keyDialogError','');
    var input=document.getElementById('keyDialogInput');
    input.value='';
    keyDialogRequest={resolve:resolve,reject:reject};
    if(typeof keyDialog.showModal==='function')keyDialog.showModal();
    setTimeout(function(){input.focus();},40);
  });
}

document.getElementById('keyDialogCancel').addEventListener('click',closeKeyDialog);
keyDialog.addEventListener('cancel',function(event){event.preventDefault();closeKeyDialog();});
keyDialog.addEventListener('close',function(){
  var input=document.getElementById('keyDialogInput');
  if(input)input.value='';
});
document.getElementById('keyDialogConfirm').addEventListener('click',function(){
  var input=document.getElementById('keyDialogInput');
  var key=input.value.trim();
  if(!key){
    text('keyDialogError',t('error.missingManagementKey'));
    input.focus();
    return;
  }
  input.value='';
  var pending=keyDialogRequest;
  keyDialogRequest=null;
  if(keyDialog&&keyDialog.open)keyDialog.close();
  if(pending)pending.resolve(key);
});

function managementFetch(url,options,key){
  options=options||{};
  options.headers=options.headers||{};
  options.headers.Authorization='Bearer '+key;
  return fetch(url,options).then(function(res){
    if(!res.ok){
      if(res.status===401||res.status===403)throw new Error(t('error.unauthorized'));
      return res.json().catch(function(){return {};}).then(function(body){
        throw new Error(body.error||('HTTP '+res.status));
      });
    }
    return res;
  });
}
document.getElementById('exportBackup').addEventListener('click',function(){
  requestManagementKey(t('data.keyTitle'),t('data.backupPrompt')).then(function(key){
    toast(t('backup.preparing'));
    return managementFetch(backupURL,{credentials:'same-origin'},key).then(function(res){return res.blob();}).then(function(blob){
      var link=document.createElement('a');
      link.href=URL.createObjectURL(blob);
      link.download='token-usage-backup.db';
      link.click();
      setTimeout(function(){URL.revokeObjectURL(link.href);},2000);
    });
  }).catch(function(error){
    if(!isKeyDialogCanceled(error))toast(error.message);
  });
});
var restoreInput=document.createElement('input');
restoreInput.type='file';
restoreInput.accept='.db,application/octet-stream';
restoreInput.style.display='none';
document.body.appendChild(restoreInput);
document.getElementById('restoreBackup').addEventListener('click',function(){
  restoreInput.click();
});
restoreInput.addEventListener('change',function(){
  var file=this.files&&this.files[0];
  this.value='';
  if(!file)return;
  if(!window.confirm(t('backup.restoreWarning')))return;
  requestManagementKey(t('data.keyTitle'),t('data.restorePrompt')).then(function(key){
    toast(t('backup.restoring'));
    return file.arrayBuffer().then(function(buffer){
      return managementFetch(restoreURL,{
        method:'POST',
        headers:{'Content-Type':'application/octet-stream','X-Confirm-Restore':'replace'},
        body:buffer,
        credentials:'same-origin'
      },key);
    });
  }).then(function(){
    toast(t('backup.restored'));
    state.catalogKey='';
    load();
  }).catch(function(error){
    if(!isKeyDialogCanceled(error))toast(error.message);
  });
});
var resetDialog=document.getElementById('resetDialog');
document.getElementById('resetButtonPanel').addEventListener('click',function(){
  if(typeof resetDialog.showModal==='function')resetDialog.showModal();
});
document.getElementById('resetCancel').addEventListener('click',function(){resetDialog.close();});
document.getElementById('resetConfirm').addEventListener('click',function(){
  resetDialog.close();
  requestManagementKey(t('data.keyTitle'),t('data.resetPrompt')).then(function(key){
    return managementFetch(resetURL,{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({confirm:'reset'}),
      credentials:'same-origin'
    },key);
  }).then(function(){
    toast(t('reset.done'));
    state.catalogKey='';
    load();
  }).catch(function(error){
    if(!isKeyDialogCanceled(error))toast(error.message);
  });
});

/* ------------------------------------------------------------------ toast */
var toastTimer=null;
function toast(message){
  var node=document.getElementById('toast');
  node.textContent=message;
  node.classList.add('is-visible');
  if(toastTimer)clearTimeout(toastTimer);
  toastTimer=setTimeout(function(){node.classList.remove('is-visible');},2600);
}

/* --------------------------------------------------------------- prefs */
var preferencesSaveInFlight=false;
var preferencesSavePending=false;
function scheduleDashboardPreferencesSave(){
  if(preferencesSaveInFlight){preferencesSavePending=true;return;}
  preferencesSaveInFlight=true;
  var params=new URLSearchParams();
  params.set('save','1');
  params.set('request_page_size','100');
  params.set('dimension_page_size','100');
  params.set('time_range_mode',state.range.mode);
  params.set('token_display_mode','full');
  if(state.range.mode==='custom'&&state.range.start&&state.range.end){
    params.set('time_range_start',state.range.start);
    params.set('time_range_end',state.range.end);
  }
  fetch(preferencesURL+'?'+params.toString(),{credentials:'same-origin',keepalive:true,headers:sessionHeaders({})})
    .catch(function(){})
    .finally(function(){
      preferencesSaveInFlight=false;
      if(preferencesSavePending){preferencesSavePending=false;scheduleDashboardPreferencesSave();}
    });
}
function loadDashboardPreferences(){
  return fetch(preferencesURL,{credentials:'same-origin',headers:sessionHeaders({})}).then(function(res){
    if(res.status===401){handleSessionRejected();throw new Error(t('lock.expired'));}
    if(!res.ok)throw new Error('preferences unavailable');
    return res.json();
  }).then(function(prefs){
    var mode=prefs.time_range_mode;
    if(mode==='custom'&&prefs.time_range_start&&prefs.time_range_end){
      state.range={mode:'custom',start:prefs.time_range_start,end:prefs.time_range_end};
    }else if(mode==='today'||mode==='yesterday'||mode==='last_7_days'||mode==='last_30_days'){
      state.range={mode:mode,start:null,end:null};
    }else if(mode){
      // A stored custom range without bounds cannot be honoured; fall back and
      // rewrite the preference so the next visit starts clean.
      state.range={mode:'last_30_days',start:null,end:null};
      scheduleDashboardPreferencesSave();
    }
  }).catch(function(){});
}

/* ----------------------------------------------------------- resize etc. */
function dismissPinnedChart(event){
  if(!pinnedChart)return;
  var key=pinnedChart;
  var svg=document.getElementById(key==='tokens'?'tokenChart':'requestChart');
  if(svg&&(svg===event.target||svg.contains(event.target)))return;
  var record=chartState[key];
  pinnedChart=null;
  if(record&&record.hide)record.hide();
}
document.addEventListener('pointerdown',function(event){
  if(event.pointerType==='touch'||event.pointerType==='mouse'||event.pointerType==='pen')dismissPinnedChart(event);
},true);

var resizeFrame=0;
function initializeChartResize(){
  if(!window.ResizeObserver)return;
  var observer=new ResizeObserver(function(){
    if(resizeFrame)cancelAnimationFrame(resizeFrame);
    resizeFrame=requestAnimationFrame(function(){
      resizeFrame=0;
      if(state.data&&state.tab==='usage')renderCharts();
    });
  });
  ['tokenChartHost','requestChartHost'].forEach(function(id){
    var host=document.getElementById(id);
    if(host)observer.observe(host);
  });
}
var colorSchemeQuery=window.matchMedia?window.matchMedia('(prefers-color-scheme: dark)'):null;
if(colorSchemeQuery&&colorSchemeQuery.addEventListener){
  colorSchemeQuery.addEventListener('change',function(){
    refreshSeriesColors();
    if(state.data){renderHero();renderCharts();}
  });
}

/* ---------------------------------------------------------------- i18n */
function translateStatic(){
  document.title=t('app.title');
  document.querySelectorAll('[data-i18n]').forEach(function(el){
    el.textContent=t(el.getAttribute('data-i18n'));
  });
  document.querySelectorAll('[data-i18n-title]').forEach(function(el){
    el.title=t(el.getAttribute('data-i18n-title'));
  });
  document.querySelectorAll('[data-i18n-aria]').forEach(function(el){
    el.setAttribute('aria-label',t(el.getAttribute('data-i18n-aria')));
  });
  document.querySelectorAll('[data-i18n-placeholder]').forEach(function(el){
    el.placeholder=t(el.getAttribute('data-i18n-placeholder'));
  });
  updateChips();
}

/* ------------------------------------------------------------ test hooks */
window.__dashboardTest={
  sessionKey:sessionStoreKey,
  sessionState:function(){return {token:session.token,expiresAt:session.expiresAt};},
  isLocked:function(){return document.body.classList.contains('is-locked');},
  formatTokens:formatTokens,
  compactNumber:compactNumber,
  formatPercent:formatPercent,
  currentBuckets:currentBuckets,
  queryString:queryString,
  state:state
};

/* ------------------------------------------------------------------ init */
function start(){
  locale=detectLocale();
  formatterLocale=locale==='zh-CN'?'zh-CN':locale==='zh-TW'?'zh-TW':locale==='ru'?'ru-RU':'en-US';
  document.documentElement.lang=locale;
  refreshSeriesColors();
  translateStatic();
  initializeThemeSync();
  initializeChartResize();
  window.addEventListener('languagechange',function(){
    locale=detectLocale();
    formatterLocale=locale==='zh-CN'?'zh-CN':locale==='zh-TW'?'zh-TW':locale==='ru'?'ru-RU':'en-US';
    document.documentElement.lang=locale;
    translateStatic();
    if(state.data){renderHero();renderCharts();renderRankings();}
  });
  document.getElementById('refreshButton').addEventListener('click',function(){load();});
  document.addEventListener('visibilitychange',function(){
    if(document.visibilityState==='visible'&&state.data)load();
  });
  document.getElementById('unlockForm').addEventListener('submit',function(event){
    event.preventDefault();
    var input=document.getElementById('unlockKey');
    var key=input.value.trim();
    if(!key){
      text('unlockError',t('error.missingManagementKey'));
      input.focus();
      return;
    }
    // Drop the key from the DOM before the request so it never lingers.
    input.value='';
    var button=document.getElementById('unlockButton');
    button.disabled=true;
    text('unlockError','');
    unlockWithKey(key).catch(function(error){
      button.disabled=false;
      text('unlockError',error&&error.message?error.message:t('lock.failed'));
      var again=document.getElementById('unlockKey');
      if(again){again.value='';again.focus();}
    }).then(function(){
      button.disabled=false;
    });
  });
  document.getElementById('lockButton').addEventListener('click',lockNow);
  var stored=readStoredSession();
  if(!stored){
    showLock('');
    return;
  }
  session={token:stored.token,expiresAt:stored.expiresAt||''};
  hideLock();
  beginSession();
}
function lockNow(){
  var token=session.token;
  session={token:'',expiresAt:''};
  clearStoredSession();
  if(token){
    fetch(sessionRevokeURL,{
      method:'POST',
      credentials:'same-origin',
      cache:'no-store',
      headers:{'X-Plugin-Session':token}
    }).catch(function(){});
  }
  state.seq++;
  clearDashboardView();
  showLock('');
}
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',start);
else start();
})();
</script>
</body>
</html>`
