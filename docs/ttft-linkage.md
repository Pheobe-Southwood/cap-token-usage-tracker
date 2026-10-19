# TTFT 联动：读取 cap-opencode-go 的逐小时首字延迟（v2.5.0）

本文说明为什么插件执行器的平均 TTFT 会缺失、本插件如何只读联动同级 `cap-opencode-go` 插件的统计
补全它，以及联动的精度边界。实现位于 `internal/plugin/ttft_linkage.go`。

## 问题

宿主的用量记录对插件执行器（`provider` 形如 `cap-opencode-go`、`cap-opencode-go-gemini`）只给出
延迟与 Token，不给首字延迟：这些请求的 `ttft_ns` 恒为 0（生产库中 3337 行全部如此），于是
「模型」页签的平均 TTFT 显示 `—`，生成 TPS 也退化成用总延迟做分母。

宿主不会把兄弟插件的配置块交给本插件（`internal/pluginhost/config.go` 的
`runtimeConfigYAML(item, enabled)` 只传自己的 `plugins.configs.<id>`），所以无法从配置里读
`cap-opencode-go` 的模型清单；但兄弟插件会把统计写到磁盘，本插件可以直接读那些文件。

## 数据来源

`cap-opencode-go` 插件把状态写在 `<CLIProxyAPI>/data/cap-opencode-go/`：

| 文件 | 用途 | 本插件读取的字段 |
|---|---|---|
| `shared.json` | 模型别名映射（`settings.models[*].aliases`） | 模型 ID 与别名 |
| `stats-chat.json` | chat 端点逐小时统计 | `version`、`role`、`written_at`、`models[*].hours["YYYY-MM-DDTHH"].ttft_nano` / `ttft_samples` |
| `stats-gemini.json` | gemini 端点逐小时统计 | 同上 |
| `stats-responses.json` | responses 端点逐小时统计 | 同上 |
| `stats-messages.json` | messages 端点逐小时统计 | 同上 |

这些文件由兄弟插件以「临时文件 + rename」原子替换写入，`version` 恒为 `1`。兄弟插件的保留期是
`40 * 24h`，所以联动只覆盖最近约 40 天。

目录解析顺序：`ttft_supplement_dir`（若配置）→ `data_path` 同目录下的 `cap-opencode-go`。两者都不存在
时联动降级，不影响任何其他统计。

## 安全边界

- **只读**：本插件从不写入、创建或修改兄弟插件目录中的任何文件。
- **白名单解析**：只解码上表中的字段。`shared.json` 里的 `settings.keys`（明文 API Key）等字段
  **从不解码**，也不会出现在任何响应或日志中。
- **不引入新路由**：联动结果只并入既有的 `/stats/models` 响应，没有新增数据接口。
- 读取失败（文件缺失、JSON 损坏、`version` 不是 1）只会让 `ttft_supplement.available` 为 `false`
  并带上 `error`，统计照常返回。

## 合并算法

对每个「小时 × 模型」：

1. 兄弟插件给的是该小时的 `ttft_nano` 总和与 `ttft_samples` 样本数；本插件同时算出自己在该小时的
   `total_ttft_ns` 与 `ttft_samples`（用于校验，不参与合并）。
2. 记 `Δn = 兄弟样本数 − 本插件样本数`、`Δs = 兄弟纳秒和 − 本插件纳秒和`。生产库实测：两边的
   `requests` 逐小时一致，重叠小时的 `ttft_nano` 与本插件 `total_ttft_ns` 比值为 1.0000，因此这个
   差值就是「本插件没记到的那部分样本」。
3. `Δn ≤ 0` 或 `Δs ≤ 0` 时跳过（本插件已经更全）。
4. 平均首字延迟按 `Δs / Δn` 计算；若 `Δs` 不可用（兄弟插件只写了样本数），退化为兄弟插件该小时的
   每样本均值。
5. 归属：把补入的样本记到**该小时缺失样本最多**的那一行（同一小时同一模型可能因 API Key、来源等
   维度拆成多行），补入量以该行缺失的请求数为上限；纳秒按补入比例缩放。
6. 只对提供商名匹配兄弟目录名（`cap-opencode-go` 或 `cap-opencode-go-*`）的行生效，避免同名模型
   挂在别的渠道时借到别人的样本。

缓存：按目录内各文件的 `size + mtime` 做指纹，指纹不变则复用上次解析结果；配置热重载时清空。

## 精度与边界

- **逐小时、非逐请求**：兄弟插件没有逐请求记录，所以 `/requests` 里这些行的 TTFT 仍是 `—`，
  只有模型级平均值被补全。
- **按行归属是估算**：同一小时同一模型拆成多行时，补入样本归到缺失最多的那一行；行级平均值因此
  可能有偏差，小时级合计与模型级平均值是准确的。
- **40 天窗口**：更早的区间没有兄弟数据，仍显示 `—`。
- **同名模型保护**：提供商不匹配时绝不借用样本。
- **筛选后仍受上限约束**：筛选后的行按该行缺失的请求数封顶，不会凭空放大样本数。

## 验证记录（生产库副本）

30 天区间按小时逐模型合并的结果（`Δs/Δn` 为补入的平均首字延迟）：

| 模型 | 补入样本 | 合并后平均 TTFT |
|---|---:|---:|
| `DeepSeek-V4.1-Flash` | +2005 | 4.43 s |
| `gemini-4-argon` | +14 | 13.41 s |
| `gpt-6-luna` | +13（自身 3） | 1.60 s |
| `Beam` | +14 | 752 ms |

其余模型（`gemini-4-argon-contributor`、`cat-jam-free`、`CatJam`、`grok*` 等）在窗口内没有兄弟样本，
仍显示 `—`；两个小时本插件有记录而兄弟插件缺失，保持原值不补。

## 配置

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `ttft_supplement` | `true` | 关闭后完全不读兄弟插件目录 |
| `ttft_supplement_dir` | 空 | 同级插件状态目录；留空时取 `data_path` 同目录下的 `cap-opencode-go` |

## 响应字段

`/stats/models` 顶层：

```json
{
  "ttft_supplement": {
    "enabled": true,
    "available": true,
    "sources": ["shared.json", "stats-chat.json"],
    "written_at": "2026-10-17T12:04:00Z",
    "models": 6,
    "samples": 2046,
    "error": ""
  }
}
```

每个模型行新增 `supplemented_ttft_samples`（该行补入的样本数），界面在平均 TTFT 后显示 `*`，页脚
说明来源、样本数与更新时间。
