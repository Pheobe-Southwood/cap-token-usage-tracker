# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CLIProxyAPI](https://img.shields.io/badge/CLIProxyAPI-Plugin-success.svg)](https://github.com/router-for-me/CLIProxyAPI)

**[English](README.md)** | **[简体中文](README_zh.md)** | **[日本語](README_ja.md)** | **[Français](README_fr.md)** | **[한국어](README_ko.md)**

---

CAP Token Usage Tracker 是专为 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 设计的持久化用量与成本分析插件。插件通过官方插件接口无缝捕获请求元数据，将分钟级聚合数据与请求日志安全存储在本地轻量级数据库中，并提供美观、即时的可视化仪表盘。

无需复杂的监控基础设施，即可清晰掌握 Token 消耗结构、Prompt 缓存命中率、实际开销分布及各模型实测性能。

---

## 核心功能与直观看板

### 1. 全景 Token 统计与缓存命中率监控

实时掌握整体流量规模与 Token 构成，精确量化 Prompt Cache（上下文缓存）带来的提速与降本收益。

- **用量结构透视**：直观拆解输入 Token、输出 Token 及命中缓存的 Token 数量。
- **缓存效率量化**：显示整体 Prompt 缓存命中率（Cache Hit Rate），评估提示词复用效果。
- **颗粒度趋势图**：支持分钟、小时、天等多尺度聚合直方图，横轴自动补齐区间，高峰低谷一目了然。

![Token 用量与缓存分析](assets/screenshots/01-token-overview.png)

---

### 2. 模型与 API Key 消耗排行

清晰归因团队内部各业务线、客户端或开发者的用量占比。

- **多维度占比排行**：横向柱状图实时展示各模型、各 API Key 的消耗比例与调用总量。
- **业务友好标签**：支持为 API Key 指纹绑定可读名称（如「生产集群」、「开发环境」），保护密钥隐私的同时明确归属。
- **平滑请求曲线**：直观监控每分钟/每小时的实际 API 调用量走势与失败率。

![模型与 API Key 消耗排行](assets/screenshots/02-token-rankings.png)

---

### 3. 实时花销核算与多维成本归因

集成官方 models.dev 价格目录并支持动态测算，告别月末账单盲盒。

- **开箱即用刊例价**：内置超 8,000 款主流模型价格表，支持全自动后台增量更新。
- **灵活切换分组**：花销趋势堆叠图可在「按模型」与「按 API Key」之间一秒切换，无需重新请求数据。
- **峰谷差异核算**：支持高峰/低谷费率规则（如 DeepSeek 峰谷计划），实时展示时段差价带来的费用变动。
- **纯动态计算**：费用随价格定义即时计算，修改单价后无需重写数据库，历史区间自动刷新。

#### 按模型查看花销趋势
![模型花销分析](assets/screenshots/03-cost-analysis.png)

#### 按 API Key 查看花销分布
![API Key 花销分布](assets/screenshots/04-cost-by-api-key.png)

---

### 4. 模型性能指标与灵活调价

评估不同模型的真实调用速度，并根据实际采购成本自主设定单价。

- **双口径 TPS 测速**：提供生成 TPS（Token 每秒生成速度）与端到端 TPS（总延迟口径），客观衡量模型响应能力。
- **首字延迟 (TTFT)**：毫秒级记录首字响应延迟，支持联动同级执行器插件补齐统计。
- **价格矩阵与时段规则**：支持为任意模型单独设置输入/输出/缓存单价，或配置自定义峰谷时段规则。

![模型性能与单价管理](assets/screenshots/05-model-performance.png)

![模型调价与时段配置弹窗](assets/screenshots/06-pricing-modal.png)

---

### 5. 逐请求调用明细与故障排查

每一笔 API 调用的全生命周期记录，快速定位慢请求与异常调用。

- **详尽字段追踪**：记录请求时间戳、模型名称、来源渠道、API Key 标签、各类 Token 计数、TTFT 及持续总时长。
- **状态过滤与排障**：一键筛选成功或失败请求，快速定位 HTTP 500、504 超时等异常详情。

![逐请求调用追踪日志](assets/screenshots/07-request-logs.png)

---

## 快速上手

### 第一步：获取与安装插件

从 [Releases 页面](https://github.com/AITNR/cap-token-usage-tracker/releases) 下载对应操作系统与架构的共享库文件，放入 CLIProxyAPI 的 `plugins` 目录中：

| 操作系统与架构 | 安装路径 |
|---|---|
| Linux amd64 | `plugins/linux/amd64/cap-token-usage-tracker.so` |
| Linux arm64 | `plugins/linux/arm64/cap-token-usage-tracker.so` |
| Windows amd64 | `plugins/windows/amd64/cap-token-usage-tracker.dll` |
| macOS (Intel) | `plugins/darwin/amd64/cap-token-usage-tracker.dylib` |
| macOS (Apple Silicon) | `plugins/darwin/arm64/cap-token-usage-tracker.dylib` |

### 第二步：添加配置

在 CLIProxyAPI 的 `config.yaml` 中启用插件：

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      retention_days: 365
      api_key_secret: "your-strong-random-secret-key-at-least-32-bytes"
      pricing_auto_refresh: true
      peak_pricing: true
```

### 第三步：访问仪表盘

启动 CLIProxyAPI 后，在浏览器中打开管理中心仪表盘地址：

```text
http://127.0.0.1:8317/v0/resource/plugins/cap-token-usage-tracker/dashboard
```

输入 CLIProxyAPI 的管理密钥（Management Key）即可解锁查看全部图表与数据。

---

## 安全与隐私承诺

- **绝不收集核心内容**：插件严禁记录用户的 Prompt 内容、上下文请求正文、模型生成的文本回复及请求头敏感信息。
- **API Key 脱敏安全**：不保存 API Key 明文。所有统计使用带盐 HMAC-SHA256 不可逆指纹与加密代际存储，仪表盘只暴露指纹与用户自拟标签，杜绝密钥泄露风险。
- **数据完全本地化**：所有统计数据直接保存在本地嵌入式 `bbolt` 文件数据库中，绝不上报给任何第三方服务器。
- **会话鉴权隔离**：数据接口均要求经过校验的短期会话令牌，未授权的外部直接访问一律返回 `401 Unauthorized`。

---

## 技术架构与 API

关于底层编译命令（CGO/交叉编译）、嵌入式存储引擎设计、详细 RESTful API 规范及 TTFT 联动机制，请参阅专门的技术文档：

👉 **[技术架构与 API 详细文档 (docs/TECHNICAL.md)](docs/TECHNICAL.md)**

---

## 开源协议

本项目采用 [MIT License](LICENSE) 开源协议。
