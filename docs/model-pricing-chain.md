# 模型价格确定链路（语言无关规格）

> 来源：对 [kelivo](https://github.com/) Flutter/Dart 实现的只读源码分析。
> 用途：跨语言移植参考。本文刻意剔除 Dart 细节，只保留语义、数据形状、判定规则与常量。
> 第 13 节是必须保住的行为契约，第 14 节是原实现的债——照抄会一并继承。

## 0. 一句话总结

价格是一个**派生量**，不是事实源。系统里只有两份持久事实：

1. 每个模型的价格表（外部目录 + 用户覆盖）
2. 每次调用的 token 计数

费用 = 当前价格 × 已存 token 数，**永远现算，从不落库**。

价格来源恰好两个：外部目录（models.dev）与用户逐模型覆盖。没有硬编码价格表，没有从余额接口反推的单价，没有按模型名猜测的价格。

---

## 1. 数据结构

### 1.1 价格记录 Pricing

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `input` | 数值或空 | 单价，每 1,000,000 token |
| `output` | 数值或空 | 单价，每 1,000,000 token |
| `cacheRead` | 数值或空 | 缓存读取单价 |
| `cacheWrite` | 数值或空 | 缓存写入单价 |
| `currency` | 字符串 | 默认 `"USD"` |

四个单价字段都可为空。货币字段在目录路径下**恒为 `"USD"`**，只有用户覆盖能引入其他币种。

### 1.2 用量记录 Usage

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `promptTokens` | 整数 | 输入总量，**包含**缓存读取与缓存写入 |
| `completionTokens` | 整数 | 输出总量，**包含**推理 token |
| `cachedTokens` | 整数 | 缓存读取量 |
| `cacheWriteTokens` | 整数 | 缓存写入量 |
| `reasoningTokens` | 整数 | 推理 token |
| `totalTokens` | 整数 | 总量 |

每个字段都区分「缺失」与「零」。缺失表示该次上报未提供该计数，零表示明确上报为零。合并两次上报时，**缺失不覆盖已有值，零会覆盖**。

### 1.3 价格来源标记 PriceSource

| 取值 | 含义 | 是否产生价格 |
| --- | --- | --- |
| `override` | 用户覆盖 | 是 |
| `catalog` | 外部目录 | 是 |
| `guess` | 名称猜测器 | **否** |
| `vendor` | 厂商默认 | **否** |
| `fallback` | 兜底 | **否** |

---

## 2. 管线总览

```text
  外部目录 JSON ──┐
                  ├─→ [1] 精简 ─→ 目录数据（内存）
  内置快照 JSON ──┘                  │
                                     ▼
                    [2] 求候选 provider 集合
                                     │
                                     ▼
                    [3] 模型 id 匹配阶梯 ─→ 命中记录 或 无命中
                                     │
                                     ▼
                    [4] 逐字段定源 ─→ Pricing 或 空
                                     │
                                     ▼
                    [5] 施加用户覆盖（整体替换）
                                     │
                    ┌────────────────┴─────────────────┐
                    ▼                                  ▼
        [6] 费用计算                         [7] 按币种聚合
                    │                                  │
                    ▼                                  ▼
             单条消息展示                        统计面板展示
```

---

## 3. 阶段 1 — 目录摄取

### 3.1 上游

| 项 | 值 |
| --- | --- |
| 地址 | `https://models.dev/api.json` |
| 请求超时 | 60 秒 |
| 自动更新 | 默认开启（偏好键 `model_catalog_auto_update_v1`） |
| 陈旧阈值 | 24 小时 |

### 3.2 加载顺序（启动时）

1. 读本地缓存文件 `<应用支持目录>/model_catalog/models_dev.json`
   - 成功 → 使用，标记 `bundled = false`
   - 失败 → 删除该文件，继续
2. 读内置快照 `assets/model_catalog/models_dev.json`，标记 `bundled = true`

### 3.3 刷新流程（自动或手动）

```text
若 非强制 且 不陈旧 → 直接返回
下载 → 精简 → 校验（至少一个 provider 含至少一个模型）
     → 原子写缓存（写临时文件，删旧文件，重命名）
     → 替换内存数据，版本号 +1
```

### 3.4 精简规则（上游 → 内部）

保留全部 provider 与模型，**不做任何过滤**。键名重映射，空值丢弃。价格部分的映射：

| 上游键 | 内部字段 |
| --- | --- |
| `cost.input` | `costInput` |
| `cost.output` | `costOutput` |
| `cost.cache_read` | `costCacheRead` |
| `cost.cache_write` | `costCacheWrite` |

价格**不做舍入**，数值原样保留（整数也提升为浮点）。顶层含 `schemaVersion`（当前恒为 `1`）与 `generatedAt`（ISO-8601 UTC）。`schemaVersion` 不等于 1 时解析必须抛错。单个畸形 provider 或模型被跳过，不导致整体失败。

### 3.5 版本与失效

目录数据每次替换，版本号 +1。依赖目录的解析结果按 `(providerId, modelKey)` 缓存；失效条件三者任一：

- 覆盖对象身份变化
- 目录版本变化
- 配置指纹变化

### 3.6 实测内置快照规模（供量级参考）

```text
provider          217 个
模型              7808 个，其中带价格 7388 个（94.6%）
价格键出现次数     input 7388、output 7388、cache_read 4847、cache_write 1543
价格值域           0.0 – 600.0
「免费」模型       input 与 output 同时为 0 的有 619 个
```

---

## 4. 阶段 2 — 求候选 provider 集合

输入：provider 配置（含 `baseUrl`、`providerType`、`vertexAI`）。输出：有序的目录 provider id 列表。

按顺序求值：

| 规则 | 条件 | 结果 |
| --- | --- | --- |
| R1 | `vertexAI` 为真 | `["google-vertex", "google-vertex-anthropic"]` |
| R2 | 否则 `baseUrl` 去空白后为空 | 按 `providerType` 单元素：claude 型 → `anthropic`；google 型 → `google`；其他 → `openai` |
| R3 | 否则 | 见下方推导 |
| R4 | 始终 | 排序：先按字符串长度升序，长度相同按字典序升序 |

R3 推导：

```text
host = baseUrl 的小写主机名（解析失败则尝试补 "https://" 再解析）
ids  = 精确主机映射表[host]  ∪  {所有 api 主机名等于 host 的目录 provider}
若 ids 仍为空 → 按 R2 的 providerType 规则补一个
```

### 4.1 精确主机映射表（20 条，用于上游未提供 `api` 字段的 provider）

| host | provider id |
| --- | --- |
| `api.openai.com` | `openai` |
| `api.anthropic.com` | `anthropic` |
| `generativelanguage.googleapis.com` | `google` |
| `api.x.ai` | `xai` |
| `api.mistral.ai` | `mistral` |
| `api.groq.com` | `groq` |
| `api.cerebras.ai` | `cerebras` |
| `api.cohere.com` | `cohere` |
| `api.cohere.ai` | `cohere` |
| `api.deepinfra.com` | `deepinfra` |
| `api.together.xyz` | `togetherai` |
| `api.perplexity.ai` | `perplexity` |
| `aihubmix.com` | `aihubmix` |
| `api.venice.ai` | `venice` |
| `ai-gateway.vercel.sh` | `vercel` |
| `gateway.ai.cloudflare.com` | `cloudflare-ai-gateway` |
| `api.v0.dev` | `v0` |
| `gitlab.com` | `gitlab` |
| `cloud.gitlab.com` | `gitlab` |
| `ai.salad.cloud` | `salad-cloud` |

### 4.2 主机后缀规则（精确表未命中时）

| 模式 | provider id |
| --- | --- |
| `aiplatform.googleapis.com` 或其 `.aiplatform.googleapis.com` / `-aiplatform.googleapis.com` 结尾 | `google-vertex`, `google-vertex-anthropic` |
| `*.openai.azure.com` | `azure` |
| `*.cognitiveservices.azure.com` | `azure-cognitive-services` |
| `bedrock-runtime.*.amazonaws.com` | `amazon-bedrock` |
| `*.aihubmix.com` | `aihubmix` |
| `ai.salad.cloud` 或 `*.salad.cloud` | `salad-cloud` |
| `*.gitlab.com` | `gitlab` |
| `*.ml.cloud.ibm.com` | `watsonx` |

---

## 5. 阶段 3 — 模型 id 匹配阶梯

### 5.1 归一化函数

```text
FUNCTION normalize(id)
    s = id 去首尾空白，转小写
    若 s 以 ":free" 结尾        → 去掉该后缀
    若 s 含 "@"，取最后一个 "@" 之后的部分
        且该部分非空且全为数字   → 去掉 "@数字" 后缀
    RETURN s
END
```

### 5.2 索引结构

每个 provider 三张表，另加一张全局表：

| 索引 | 键 | 值 | 冲突处理 |
| --- | --- | --- | --- |
| `exactByProvider` | 原始 id | 模型 | — |
| `lowerByProvider` | 小写 id | 模型 | 先到先得，不覆盖 |
| `normalizedByProvider` | `normalize(id)` | 模型 | 先到先得，不覆盖 |
| `globalByNormalized` | `normalize(id)` | 模型列表（含所属 provider） | 追加 |

另记录每个 provider 在目录中的插入序号。

### 5.3 匹配顺序

设 `target` = 输入模型 id（去空白），`candidates` = 阶段 2 的结果。

```text
L1  按 candidates 顺序，查 exactByProvider[provider][target]
        → 命中，类型 = providerExact

L2  按 candidates 顺序，查 lowerByProvider[provider][小写 target]
        → 命中，类型 = providerNormalized

L3  按 candidates 顺序，查 normalizedByProvider[provider][normalize(target)]
        → 命中，类型 = providerNormalized

L4  全局兜底，类型 = global：

    L4a 若 target 含 "/" 且位置 > 0
            prefix = "/" 前部分的小写
            suffix = "/" 后部分
            若 prefix 命中厂商前缀映射表
                在该 provider 内依次尝试 suffix 的
                    exact → lower → normalize 三级
                命中即返回

    L4b 取 globalByNormalized[normalize(target)] 的候选列表
            空 → 返回无命中
            否则按下列比较器取最优：
                键 1：第一方优先级排名，升序（排名小者优先，未列入者排最后）
                键 2：provider 前缀匹配，命中者优先
                      （模型 id 小写后以 provider id 小写开头，
                        或模型 family 小写后以 provider id 小写开头）
                键 3：provider 在目录中的插入序号，升序

全部落空 → 无命中
```

### 5.4 厂商前缀映射表（10 条）

| 斜杠前缀 | 第一方 provider |
| --- | --- |
| `openai` | `openai` |
| `anthropic` | `anthropic` |
| `google` | `google` |
| `deepseek` | `deepseek` |
| `moonshotai` | `moonshotai` |
| `qwen` | `alibaba` |
| `x-ai` | `xai` |
| `z-ai` | `zai` |
| `mistralai` | `mistral` |
| `meta-llama` | `meta` |

### 5.5 第一方优先级表（24 项，靠前者优先）

```text
openai, anthropic, google, google-vertex, deepseek, moonshotai,
alibaba, alibaba-cn, zhipuai, zai, xai, mistral, minimax, minimax-cn,
volcengine, stepfun, meta, cohere, groq, cerebras, togetherai,
fireworks-ai, openrouter
```

---

## 6. 阶段 4 — 逐字段定源

解析器对每个字段独立按优先级取值：

```text
用户覆盖  >  目录  >  名称猜测器  >  厂商默认  >  兜底
```

并为每个字段记录最终来源标记。

价格字段的规则是**特例，只有三态**：

```text
若 命中记录存在 且 (costInput 非空 或 costOutput 非空)
    → 来源 = catalog
    → 返回 Pricing{ input:      costInput,
                    output:     costOutput,
                    cacheRead:  costCacheRead,
                    cacheWrite: costCacheWrite,
                    currency:   "USD" }        // 币种硬编码
否则
    → 来源 = fallback
    → 返回 空
```

**关键点**：猜测器与厂商默认**永不**产生价格。命中不到目录就是没有价格，不回退到任何默认单价，也不从模型名推断。

判定条件**只检查 `input` 与 `output`**，不检查 cache 字段：

- 只填了 `cacheRead` / `cacheWrite` 的价格记录会被判为「无价格」。
- `input` 与 `output` 均为 0 的免费模型**会**被判为有价格，费用显示为 0。

---

## 7. 阶段 5 — 施加用户覆盖

```text
最终价格 = 覆盖价格 若存在，否则 基准价格
```

这是**整体替换**，不是逐字段合并。后果：

- 手工写入 `{"pricing": {"input": 1}}` 会使 `output` / `cacheRead` / `cacheWrite` 全部变为空，而不是继承目录值。
- 编辑表单通过「以基准价格预填」规避了这一点：草稿价格为空时，用基准价格填充各输入框，因此 UI 路径下实际是带值覆盖。

覆盖存在时，该字段来源标记改写为 `override`。

---

## 8. 阶段 6 — 费用计算

```text
FUNCTION estimate_cost(usage, pricing) -> Cost 或 空

    IF pricing 为空 THEN RETURN 空
    IF pricing.input 为空 AND pricing.output 为空 THEN RETURN 空

    inputPrice      = pricing.input  ?? 0
    outputPrice     = pricing.output ?? 0
    cacheReadPrice  = pricing.cacheRead  ?? inputPrice     // 缺价回退输入价
    cacheWritePrice = pricing.cacheWrite ?? inputPrice

    cachedInput  = usage.cachedTokens + usage.cacheWriteTokens
    billedPrompt = usage.promptTokens - cachedInput
    IF billedPrompt < 0 THEN billedPrompt = 0              // 钳到 0

    amount = ( billedPrompt           * inputPrice
             + usage.cachedTokens     * cacheReadPrice
             + usage.cacheWriteTokens * cacheWritePrice
             + usage.completionTokens * outputPrice
             ) / 1000000

    RETURN Cost{ amount, currency: pricing.currency }

END
```

### 语义要点

- 除数是 `1,000,000`，即单价单位是「每百万 token」。
- 减法成立的前提是 `promptTokens` 已包含缓存量（见第 10 节契约）。
- 缺失的单个费率按 0 计；缺失的缓存费率回退到输入价。
- 三个费率全空时返回空，**不返回 0**。

### 金额格式化规则

```text
0                → "0"（带符号则 "$0"）
绝对值 < 0.0001   → "<0.0001"（符号前置为 "<$0.0001"）
其他              → 保留 4 位小数，去尾随零与尾随小数点
```

货币符号：`USD`→`$`、`CNY`→`¥`、`JPY`→`¥`、`EUR`→`€`、`GBP`→`£`，其他 → 后缀 `" 币种码"`。

---

## 9. 阶段 7 — 聚合

分组键：`(模型 id, provider id)`。每组先按第 8 节算出该组的 `Cost`，再按币种分别累加。

```text
costByCurrency       : 币种 → 金额合计
modelsWithoutPricing : 无价格模型计数
```

规则：

- **不做汇率换算**。不同币种并列展示，各成一项。
- 价格为空（含 provider id 为空、模型 id 为空的情况）的组计入 `modelsWithoutPricing`，其 token 量仍计入总量，但不计入任何金额。
- 分组行的 token 量由存储层聚合而来，价格在聚合时现查。

### 重算触发

统计页的缓存签名包含：会话列表、时间范围、启动次数、provider 名称表、助手名称表、provider 覆盖表、统计修订号。

> **注意**：该签名**不包含**目录版本或目录生成时间。因此运行中目录刷新后，已算出的金额不会立即重算，需等待其他签名变化。

---

## 10. 用量采集契约（费用计算的输入前提）

**核心契约**：`promptTokens` = 总输入量，**包含**缓存读取量与缓存写入量。各上游必须在入口处归一化到此契约。

### Anthropic 系

```text
cachedTokens     = usage.cache_read_input_tokens
cacheWriteTokens = usage.cache_creation_input_tokens
promptTokens     = usage.input_tokens
                 + cache_read_input_tokens
                 + cache_creation_input_tokens      // 显式相加
三者全缺失时 promptTokens 记为缺失。
```

### OpenAI 兼容系

```text
promptTokens = usage.prompt_tokens  或  usage.input_tokens
               // 该值本身已包含缓存，无需相加
cachedTokens = usage.prompt_tokens_details.cached_tokens
               或  usage.input_tokens_details.cached_tokens
cacheWriteTokens：不读取（OpenAI 不计缓存写入费）
completionTokens = usage.completion_tokens 或 usage.output_tokens
reasoningTokens  = completion_tokens_details.reasoning_tokens
                   或 output_tokens_details.reasoning_tokens
```

### Google 系

```text
promptTokens = usageMetadata.promptTokenCount
               // 该值本身已包含 cachedContentTokenCount
cachedTokens = usageMetadata.cachedContentTokenCount
```

### 未覆盖的上游字段（移植时需注意）

DeepSeek 的 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` **未被读取**。若上游不额外提供 `prompt_tokens_details.cached_tokens`，DeepSeek 的缓存读取会被按全价输入计费，导致**高估**。

---

## 11. 阶段 8 — 价格的编辑与持久化

### 11.1 编辑界面

- 四个数值输入框：输入单价、输出单价、缓存读单价、缓存写单价
- 另有货币字段（覆盖态下默认 `"USD"`）
- 校验：非空时必须能解析为十进制数，否则整体拒绝保存
- 每个字段旁显示来源标记（`catalog` / `override`）与「重置」入口

### 11.2 重置语义

重置 = 清除覆盖，回到「用空覆盖重新解析出的基准值」，即目录值（或空）。**不是**回到上一次手工输入。

### 11.3 序列化形状

```json
{
  "pricing": {
    "input": 1.5,
    "output": 6.0,
    "cacheRead": 0.15,
    "cacheWrite": 1.875,
    "currency": "USD"
  }
}
```

写入位置：provider 配置中的 `modelOverrides[模型键]`。数值为空则整个键省略；`currency` 恒写出。整个 provider 配置表序列化为 JSON，存于单一偏好项中。

### 11.4 持久化位置

键值存储（`shared_preferences` 类），**非关系库**。删除模型时，其 `modelOverrides` 条目一并移除。

### 11.5 备份

provider 配置属于备份范围，因此价格覆盖随备份与恢复一起迁移。

---

## 12. 用量与费用的存储布局

### 12.1 每条消息行存储的列

```text
model_id, provider_id
prompt_tokens, completion_tokens, cached_tokens, total_tokens
extras_json                      // 无模式的扩展字段
```

### 12.2 缓存写入量的特殊处理

**没有**独立的 `cache_write_tokens` 列。它被写入 `extras_json` 的 `"tokens.cacheWrite"` 键，统计查询时用 JSON 取值函数（如 `json_extract`）提取。

### 12.3 没有费用列

全库**不存在**任何 `cost` / `price` / `amount` 列。这是本链路最重要的架构事实：历史费用无法复现，改价或目录刷新会追溯性地改变所有历史统计金额。

---

## 13. 必须成立的不变量

| 编号 | 不变量 |
| --- | --- |
| I1 | 价格来源只有目录与用户覆盖两种，其余层级不产生价格 |
| I2 | 目录路径的币种恒为 `"USD"` |
| I3 | `promptTokens` 包含缓存读取与缓存写入（减法依赖此契约） |
| I4 | 用量字段区分「缺失」与「零」；合并时缺失不覆盖，零覆盖 |
| I5 | 费用 = 现算，不落库；金额永远反映**当前**价格 |
| I6 | 价格覆盖是整体替换，不做逐字段合并 |
| I7 | 只有 `input` 或 `output` 非空才算得出费用；纯缓存费率返回空 |
| I8 | 多币种分别累加，不做汇率换算 |
| I9 | 匹配失败（provider 认不出或模型 id 非标准）导致价格为空，该模型全部历史费用归入「无价格」计数 |

---

## 14. 已知缺陷与陷阱

| 编号 | 缺陷 | 影响 |
| --- | --- | --- |
| P1 | 费用不落库 | 历史账单不可复现、不可审计。同一批 token，改价前后算出不同金额 |
| P2 | 缓存写入量藏在 `extras_json` 而非独立列 | 查询需 JSON 取值，索引与聚合效率差；趋势类查询只汇总了 `cached_tokens`，未汇总 `cache_write` |
| P3 | DeepSeek 缓存命中未被识别 | 高估（详见第 10 节） |
| P4 | 零价模型（619 个）显示为 `$0` 而非「无价格」 | 不计入 `modelsWithoutPricing`，掩盖了「其实没配价」的情况 |
| P5 | provider 匹配失败时价格静默变为空 | 用户只能在统计页看到「N 个模型无价格」的计数，无法得知是哪个模型、为什么 |
| P6 | 价格覆盖整体替换 | 手工或导入的、只含部分字段的覆盖会意外清空其余字段 |
| P7 | 统计重算签名不含目录版本 | 运行中目录刷新后金额不立即更新 |
| P8 | 缺 cache 费率的模型（`cache_read` 缺 2541 个，`cache_write` 缺 1543 个） | 命中缓存时按全价输入计费 |
| P9 | 内置快照为手工生成、无 CI 定时任务 | 容易陈旧。实测快照 `generatedAt` 2026-09-15，仓库 HEAD 2026-10-16，差约一个月。离线首启用户拿到的是一个月前的价目表 |
| P10 | 缓存写入量在同一请求的多轮工具调用之间需要正确的快照语义 | 否则上一轮的计数会泄漏到下一轮 |

---

## 15. 移植检查清单

- [ ] 定义 `Pricing` 与 `Usage` 数据结构，`Usage` 各字段必须可空以区分缺失与零
- [ ] 定义来源标记枚举，覆盖 `catalog` / `override` / `fallback` 三态
- [ ] 实现目录精简与解析，校验 `schemaVersion == 1`
- [ ] 实现本地缓存 + 内置兜底的加载顺序，以及原子写
- [ ] 实现陈旧判定（24 小时）与自动刷新开关
- [ ] 实现归一化函数（去 `:free` 后缀、去 `@数字` 后缀、转小写）
- [ ] 实现候选 provider 推导（主机映射 + 后缀规则 + 类型回退 + 长度排序）
- [ ] 实现四级匹配阶梯与全局兜底的比较器
- [ ] 实现价格字段的三态判定（只查 `input` / `output`）
- [ ] 实现费用公式，含钳零与费率回退
- [ ] 决定是否沿用「整体替换」的覆盖语义（**建议改为逐字段合并**）
- [ ] 决定是否落盘价格快照（**强烈建议落盘**，见 P1）
- [ ] 在各上游入口归一化 `promptTokens` 为「含缓存」语义
- [ ] 覆盖 DeepSeek 的 `prompt_cache_hit_tokens`（见 P3）
- [ ] 聚合时按币种分组，不做汇率换算
- [ ] 让重算签名包含价格版本（见 P7）

---

## 16. 常量速查

| 常量 | 值 |
| --- | --- |
| 目录地址 | `https://models.dev/api.json` |
| 内置快照路径 | `assets/model_catalog/models_dev.json` |
| 缓存文件名 | `models_dev.json`（位于 `<应用支持目录>/model_catalog/`） |
| 自动更新偏好键 | `model_catalog_auto_update_v1` |
| 自动更新默认值 | 开启 |
| 陈旧阈值 | 24 小时 |
| 刷新超时 | 60 秒 |
| 目录模式版本 | `1` |
| 价格单位 | 每 1,000,000 token |
| 价格键（上游 → 内部） | `input`→`input`、`output`→`output`、`cache_read`→`cacheRead`、`cache_write`→`cacheWrite` |
| 货币符号表 | `USD`→`$`、`CNY`→`¥`、`JPY`→`¥`、`EUR`→`€`、`GBP`→`£` |
| 最小可显示金额 | `0.0001` |
| 金额小数位 | 4 |

---

## 17. 源码对照表（如需回溯原实现）

| 关注点 | 源文件 |
| --- | --- |
| 费用公式 | `lib/core/utils/model_cost.dart` |
| 价格结构定义、覆盖结构与整体替换 | `lib/core/models/model_spec.dart` |
| 逐字段定源与价格三态 | `lib/core/services/model_spec/model_spec_resolver.dart` |
| 目录加载 / 刷新 / 匹配 | `lib/core/services/model_catalog/model_catalog_service.dart` |
| 目录精简规则 | `lib/core/services/model_catalog/model_catalog_trim.dart` |
| 目录 JSON 结构与解析 | `lib/core/services/model_catalog/catalog_entry.dart` |
| 目录生成脚本 | `tool/update_model_catalog.dart` |
| 用量结构契约 | `lib/core/models/token_usage.dart` |
| Anthropic 用量归一化 | `lib/core/services/api/providers/claude/claude_decoder.dart` |
| OpenAI 用量归一化 | `lib/core/services/api/providers/openai/openai_request_shaping.dart` |
| Google 用量归一化 | `lib/core/services/api/providers/google/google_decoder.dart` |
| 存储布局与聚合 SQL | `lib/core/database/app_database.dart`、`lib/core/database/chat_database_repository.dart` |
| 统计聚合与多币种 | `lib/features/stats/services/stats_aggregation_service.dart` |
| 统计重算签名 | `lib/features/stats/pages/stats_page.dart` |
| 单条消息展示 | `lib/features/chat/widgets/token_detail_popup.dart` |
| 编辑表单 | `lib/features/model/widgets/model_spec_form/` |
| 持久化与默认配置 | `lib/core/providers/settings_provider.dart` |
| 费用语义测试（可当规格读） | `test/core/utils/model_cost_test.dart` |
