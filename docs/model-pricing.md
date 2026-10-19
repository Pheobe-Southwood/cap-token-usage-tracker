# 模型价格与费用（v2.6.0）

本文说明「模型」与「花销」页签、预估费用的取价链路与配置项。实现背景与 kelivo 对照研究见
[model-pricing-chain.md](model-pricing-chain.md)。

## 数据来源

插件内置一份构建时生成的 models.dev 精简快照（`internal/plugin/modelcatalog/baseline.json`，约 630 KB，
200+ 厂商、8900+ 模型、8500+ 有价模型），并可选地从远端刷新：

| 来源 | 说明 |
|---|---|
| `embedded` | 编译进 `.so` 的快照，永远可用，是最终兜底 |
| `cache` | `data_path` 同目录的 `token-usage-tracker-model-catalog.json`，比内置快照新时优先 |
| `remote` | 通过 `pricing_catalog_url` 下载并裁剪后的快照 |

启动时选择「缓存与内置中 `generated_at` 较新者」；远端刷新成功后原子写入缓存（临时文件 + `fsync` +
`rename`，权限 `0600`），失败则保留旧快照并在状态里记录 `last_error`。

## 取价顺序

1. **手工覆盖**：数据库中按模型名保存的单价，优先级最高；与刊例价**逐字段合并**，未填字段沿用刊例价。
2. **刊例价**：在目录中按匹配阶梯找到的模型单价。
3. **无价格**：`price_source = "none"`，界面显示 `无刊例价`，费用不计入合计。

时段价（峰谷价）在基础价之上再选一层，见下节。

匹配阶梯（`internal/plugin/modelcatalog/match.go`）：

1. **渠道提示**：从请求记录的渠道名（`Dimensions.Provider`，如 `openai-compatible-opencode go`、
   `cap-opencode-go-gemini`）中提取厂商 token，去掉 `openai-compatible-` 前缀后按最长匹配。
2. **来源提示**：从来源地址（`Dimensions.Source`，如 `https://opencode.ai/zen/go/v1`）匹配目录中
   `api` 字段的主机名，同主机多厂商时按路径重合度、ID 长度排序。
3. **厂商前缀**：`openai/gpt-6-luna` 这类 `vendor/model` 写法。
4. **全局匹配**：精确 ID → 小写 ID → 归一化 ID（去 `:free`、去 `@<日期>`）。
5. **变体回退**：`gemini-3.8-flash-high` → `gemini-3.8-flash`（19 个推理强度/变体后缀，最长优先）。

同一模型出现在多个厂商时，比较器依次看：提示位次 → 是否有价（全 0 视为无价）→ 是否第一方厂商 →
ID 前缀亲和度 → 厂商 ID（保证确定性）。

## 计费口径

费用 = 单价 × Token 数 / 1,000,000，缓存 Token 的处理与 CLIProxyAPI 自身的 token 记账一致：

| 协议族 | 语义 | 处理 |
|---|---|---|
| `openai-compatible-*`、openai、codex、xai、grok、kimi、qwen、deepseek、openrouter | 输入**包含**缓存 Token | 未缓存输入 = 输入 − 缓存读取 − 缓存写入 |
| claude、anthropic | 缓存 Token **独立**于输入 | 未缓存输入 = 输入，缓存单独计费 |
| gemini、aistudio、antigravity、vertex、interaction | 输入包含缓存，推理独立 | 同第一行；推理 Token 已含在输出侧统计 |

目录未提供缓存读取/写入单价时按输入单价计算。未知厂商先用 Token 总数做算术推断
（`输入+输出` 与 `输入+缓存+输出` 哪个与总数相符），推断不出则按「输入包含缓存」处理。

## 时段价（峰谷）

某些厂商按一天中的时段给出不同单价（DeepSeek 的高峰时段为 2 倍价）。时段价是**基础价之上的第二层**：
先用上面的取价顺序得到基础单价，再按该请求所在的时间桶选出时段，用时段费率**逐字段覆盖**基础费率，
未填写的费率字段继续沿用基础价。

优先级：**手工时段价**（手工覆盖里带 `time_tiers`，非空即整体替换）> **内置峰谷计划**（`peak_pricing`，
见下表）> 无时段价（全部按基础价计费）。

内置计划（`internal/plugin/peak_plans.go`）只在目录厂商为 `opencode-go` 或 `deepseek` 时生效，且
仅覆盖以下模型（模型名先做归一化：转小写、取最后一个 `/` 之后、去掉 `:free` 与 `@<日期>`）：

| 模型 | 高峰时段（UTC，周一至周五） | 平时价（输入/输出/缓存读） | 高峰价（输入/输出/缓存读） |
|---|---|---|---|
| `deepseek-v4.1-flash` | `01:00-04:00`、`06:00-10:00` | `0.15 / 0.60 / 0.003` | `0.30 / 1.20 / 0.006` |
| `deepseek-v4-pro` | 同上 | `0.66 / 1.98 / 0.022` | `1.32 / 3.96 / 0.044` |

费率为显式常量而不是倍数，与兄弟插件 `cap-opencode-go` 的 `isPeak` 表一致。`deepseek-flash` 等兄弟插件
没有峰谷价的模型保持平价。

时段字段（`ModelPrice`）：

| 字段 | 说明 |
|---|---|
| `time_zone` | IANA 时区名，默认 `UTC`；不接受 `Local`（服务器本地时区不可复现） |
| `time_tiers[].name` | 时段名，非空、唯一、≤ 200 字节 |
| `time_tiers[].days` | 星期 1-7（周一至周日），升序去重；空数组表示每天 |
| `time_tiers[].start` / `end` | `HH:MM` 墙钟时间；`end <= start` 视为跨午夜，归属**起始日** |
| `time_tiers[].input` / `output` / `cache_read` / `cache_write` | 可选的时段费率，留空（`null`）继承基础价 |

校验（保存时执行，任何一条不满足都返回 400）：时段数 ≤ 32；时区可解析；星期 1-7 且不重复；起止时间合法且
不相等；费率 0-1000000 且非 NaN/Inf；同一时区内任意两个时段的分钟格不重叠（含跨午夜溢出到次日、含夏令时
重复小时）。选择时段时按分钟桶的 UTC 时刻换算到目标时区：跨午夜时段算作起始日，夏令时重复的小时两次都命中，
被跳过的小时不存在。

时段选择用的是**请求时间桶**（`hours` 桶键，分钟粒度），与界面上的 `tz_offset` 显示口径无关；改价或改时段会
立即改写历史区间的显示费用（费用不落库）。

## 配置

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `pricing_catalog_url` | `https://models.dev/api.json` | 必须是 https 绝对地址；空字符串禁用远程刷新 |
| `pricing_auto_refresh` | `true` | 快照过期时允许页面触发一次静默刷新 |
| `pricing_stale_after` | `24h` | 过期阈值，1h-720h |
| `peak_pricing` | `true` | 是否启用内置峰谷计划；`false` 只关闭内置计划，手工时段价不受影响 |

远端下载限制：仅 https、最多 3 次同源重定向、响应体上限 16 MB、超时 20 秒、成功后 30 秒内不再重复刷新
（`ErrRefreshTooSoon`）。插件不启动常驻 goroutine，刷新只在请求处理过程中同步完成。

## 接口

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/v0/resource/plugins/<id>/stats/models` | 逐模型用量、生成/端到端 TPS、平均 TTFT、费用、价格证据 |
| `GET` | `/v0/resource/plugins/<id>/model-prices` | 读取手工价格与目录状态 |
| `GET` | `/v0/resource/plugins/<id>/model-prices?save=1&model=…&input=…` | 设置或覆盖单价（GET 兼容写法，保留已存的时段价） |
| `GET` | `/v0/resource/plugins/<id>/model-prices?reset=1&model=…` | 恢复刊例价 |
| `GET` | `/v0/resource/plugins/<id>/model-prices?refresh=1` | 立即刷新价格目录 |
| `PUT` | `/v0/management/plugins/<id>/model-prices` | 以 JSON 请求体设置或清除（`price: null` 清除；可带 `time_zone`/`time_tiers`，**整体替换**已存时段价） |

资源路由要求 `X-Plugin-Session` 会话令牌；管理路由由宿主校验管理密钥。

`/stats/models` 返回 `schema_version: 1`，字段：

```json
{
  "schema_version": 1,
  "range": "30d",
  "pricing": {"source": "embedded", "stale": false, "models": 8991, "priced_models": 8538, "overrides": 1},
  "summary": {"requests": 29783, "cost_usd": 412.55, "priced_models": 21, "free_models": 2, "unpriced_models": 5, "unpriced_requests": 116},
  "truncated": false,
  "models": [
    {
      "model": "gemini-3.8-flash-high",
      "requests": 28018, "total_tokens": 1234567,
      "generation_tps": 812.5, "end_to_end_tps": 640.25, "average_ttft_ns": 1500000000,
      "cost_usd": 12.3456, "priced": true, "priced_requests": 28018, "unpriced_requests": 0, "in_range": true,
      "pricing": {
        "price": {"input": 0.15, "output": 0.60, "cache_read": 0.003},
        "priced": true, "free": false, "price_source": "catalog",
        "catalog_provider": "opencode-go", "catalog_model": "deepseek-v4.1-flash", "catalog_match": "provider",
        "time_zone": "UTC", "time_plan_source": "builtin",
        "time_tiers": [
          {"name": "peak-0100-0400", "days": [1, 2, 3, 4, 5], "start": "01:00", "end": "04:00",
           "input": 0.30, "output": 1.20, "cache_read": 0.006}
        ]
      }
    }
  ]
}
```

`/stats/models` 的每个模型行还带 `ttft_samples` 与 `supplemented_ttft_samples`：后者是只读联动
`cap-opencode-go` 插件补入的样本数（见 [ttft-linkage.md](ttft-linkage.md)），`/stats/models` 顶层另有
`ttft_supplement` 状态块。

`/stats/initial` 自 v2.5.0 起为 `schema_version: 3`，新增：

| 字段 | 说明 |
|---|---|
| `scale` / `bucket_seconds` | 本次聚合使用的时间轴尺度（`minute`/`hour`/`day`）与桶秒数 |
| `series` | 与时间轴等长的零填充 Token 序列（`hour` 为桶起点） |
| `cost_series` | 与 `series` 逐桶对齐的费用序列：`cost_usd`、`requests`、`unpriced_requests` |
| `cost` | 顶层费用块（`cost_usd`、`priced_models`、`free_models`、`unpriced_models`、`priced_requests`、`unpriced_requests`） |
| `cost.time_tier_cost_usd` / `cost.time_tier_requests` | 本区间因时段价相对基础价多出（或省下）的金额，以及命中时段的请求数；无时段价时为 0 |
| `api_keys[].cost_usd` / `priced_requests` / `unpriced_requests` | 每个 Key 的费用与计价覆盖 |
| `cost_by_model` / `cost_by_key` | 花销直方图的堆叠拆分，见下 |

两条查询路径（聚合热路径与逐请求精确路径）都会填充 `cost` 与 `cost_series`。

`cost_by_model` 与 `cost_by_key` 供「花销」页的堆叠直方图使用，结构相同：

```json
"cost_by_model": [
  { "key": "gemini-3.8-flash-high", "total_cost_usd": 1.0, "values": [0, 0.333, 0.333, 0.334] },
  { "key": "gpt-6-luna", "total_cost_usd": 0.1, "values": [0, 0.033, 0.033, 0.034] },
  { "key": "", "total_cost_usd": 0.16, "others": true, "values": [0, 0.053, 0.053, 0.054] }
]
```

- `values` 与 `cost_series` **同序等长**：下标 i 对应同一个桶，逐桶相加等于该桶的 `cost_usd`。
- 按区间合计降序排列（同值按键升序，保证输出稳定），最多 7 项；桶数超过 240（分钟尺度）时降为 5 项。
- 被截断的尾部模型/Key 合并成 `others: true` 的一项，`key` 为空，永远排在最后；图例把它显示为「其他」。
- 只收录区间合计 > 0 的模型/Key（免费或待补价的模型花销为 0，由 `cost.unpriced_*` 与缺价请求数体现）。
- 两项都按当前模型/API Key 筛选计算；区间内没有可计价用量时字段被省略（`omitempty`），前端回退为单序列总量柱。
- Key 维度的 `key` 是 API Key 指纹引用，标签由 `api_keys[].label` 解析，接口不返回明文。

## 存储

- 手工价格保存在 bbolt 元数据键 `model_price_overrides`（JSON，最多 10000 条，模型名 ≤ 200 字节，
  单价 0-1000000，保留 6 位小数），随备份/恢复一起迁移。
- 价格是配置而非用量数据：**清空统计不会删除手工价格**。
- 费用从不写库，每次响应现算，因此改价会立即改变历史区间的显示费用。

## 界面

- 「Token」页签（第一位）：只保留 Token 堆叠柱状图与请求次数折线图，不再显示费用卡片或每行费用。
- 「花销」页签（第二位）：与 Token 页签同构的卡片、图表与排行，口径为美元费用；总花销卡片带
  已定价/免费/待补价构成条，另有三张统计卡（已定价模型、待补价模型、请求计价覆盖率）、花销趋势
  **堆叠直方图**，以及「模型花销」「API Key 花销」两张排行。
- 花销直方图：每根柱子是一个时间桶（与 Token 页共用同一时间轴尺度），柱内按维度分段；图表右上角的
  分段控件在「按模型 / 按 API Key」之间切换，两份拆分随首屏一起下发，切换只重绘、不重新请求；图例给出
  每段颜色、名称与区间合计，合并项显示为「其他」，缺价请求数与合并说明写在副标题。
- 「模型」页签（第三位）：搜索框、预估费用与模型数量两张卡片、逐模型表格（徽标 / 渠道 / 生成 TPS /
  端到端 TPS / 平均 TTFT / Token 用量 / 费用 / 单价 / 齿轮）、价格目录说明行；联动补入 TTFT 的行在
  平均首字延迟后带 `*`。
- 价格对话框：输入、输出、缓存读取、缓存写入四个单价；留空沿用刊例价，全部清空（且无时段）并保存即恢复
  刊例价；对话框副标题显示匹配到的厂商、目录模型与当前刊例价。保存走管理路由的 JSON `PUT`。
- 价格对话框下半部是「峰谷 / 时段价」折叠区：时区输入框、可增删的时段行（名称 / 星期 / 开始 / 结束 /
  四个费率）、以及当前生效来源（内置峰谷计划或手工时段价）。已有生效时段时自动展开并预填，保存费率不会
  丢掉时段价；重叠、非法星期或非法时区在浏览器里就被拦下，不会发出请求。
- 「模型」页签的单价列在生效时段价时显示 `峰谷` 徽标，悬停显示时区、各时段窗口与首段费率。
- 「花销」页签总花销下方在存在时段价差时显示一行说明（如 `峰谷价差 +$1.88 · 1200 次命中时段`）。

## 已知边界

- 目录只有模型单价，没有 CPA 侧模型清单：表格行 = 本区间有用量 ∪ 历史记录过 ∪ 手工定价过。
- 只支持美元，不做汇率换算。
- 忽略 `cost.tiers` 与 `context_over_200k` 分档价，超长上下文请求可能低估。
- 「花销」页签的口径与「模型」页签的预估费用一致：只统计有价格的模型，缺价请求在覆盖率卡片中单列。
- 列表最多 1000 行，超出时 `truncated: true`。
- 内置快照使 `.so` 体积增加约 630 KB。
- 时段价按分钟桶选择：同一分钟内跨过时段边界的请求按桶起点归类。
- 时段价是**当前**价目表的函数：升级或改配置后，历史区间的显示费用会随之内变（与改单价一致）。
- 用 v2.6.0 之前的版本重新保存某个模型的价格会丢掉它的时段价（旧版不认识 `time_tiers` 字段）。
- 在价格对话框里保存会把**当前生效**的时段价（包括内置峰谷计划）固化成该模型的手工时段价：费率与当前一致，
  但此后不再跟随 `peak_pricing` 开关或内置表的更新。只想改基础单价而不固化时段价时，用 GET 兼容写法
  `?save=1`（它保留已存时段价），或直接清空时段行后再保存。
