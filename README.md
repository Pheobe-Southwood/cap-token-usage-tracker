# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CLIProxyAPI](https://img.shields.io/badge/CLIProxyAPI-Plugin-success.svg)](https://github.com/router-for-me/CLIProxyAPI)

**[English](README.md)** | **[简体中文](README_zh.md)** | **[日本語](README_ja.md)** | **[Français](README_fr.md)** | **[한국어](README_ko.md)**

---

**CAP Token Usage Tracker** is a persistent token and cost analytics plugin designed for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). Hooking directly into the official usage pipeline, it captures request telemetry, stores minute-level aggregations and detailed audit logs in a local lightweight database, and renders an interactive dashboard.

Gain total visibility into token throughput, prompt cache hit efficiency, estimated financial spend, and real-world model latency without running heavy monitoring infrastructure.

---

## Key Features & Visual Dashboard

### 1. Token Analytics & Prompt Cache Efficiency

Monitor traffic scale and token distribution in real time, quantifying speed and cost gains from prompt caching.

- **Token Composition Breakdown**: Inspect prompt tokens, completion tokens, and cache-hit tokens at a glance.
- **Cache Hit Rate Metering**: Track your overall prompt cache hit percentage to evaluate context-caching efficiency.
- **Dynamic Time Scales**: Switch between minute, hour, and day granularities with zero-filled axes that prevent timeline gaps.

![Token Analytics & Cache Hit Rate](assets/screenshots/01-token-overview.png)

---

### 2. Attribution Rankings by Model & API Key

Attribute usage across models, internal services, teams, or client applications with ease.

- **Horizontal Share Rankings**: View proportional token consumption across active models and API keys.
- **Operator-Friendly Labels**: Map irreversible API key fingerprints to human-readable aliases (such as `Production-Agent-Cluster` or `Dev-Workspace-IDE`).
- **Smooth Traffic Curves**: Observe API invocation volume and error rates over time with smooth cubic interpolation.

![Token Rankings & Request Volume](assets/screenshots/02-token-rankings.png)

---

### 3. Real-Time Cost Metering & Attribution

Avoid end-of-month cloud billing surprises with live USD cost tracking powered by the `models.dev` catalog.

- **Built-in Price Catalog**: Ships with list rates for over 8,000 models, supporting background catalog synchronization.
- **One-Click Dimension Switch**: Toggle the stacked cost histogram between **By Model** and **By API Key** instantly with zero extra server round-trips.
- **Peak / Off-Peak Scheduling**: Supports time-of-use pricing rules (such as DeepSeek peak schedules), displaying exact cost deltas.
- **On-the-Fly Calculation**: Costs are computed dynamically on query; changing rates immediately updates historical views without rewriting the database.

#### Cost Trend by Model
![Cost Analysis by Model](assets/screenshots/03-cost-analysis.png)

#### Cost Trend by API Key
![Cost Analysis by API Key](assets/screenshots/04-cost-by-api-key.png)

---

### 4. Model Performance Benchmarks & Custom Pricing

Benchmark real model throughput and configure custom pricing rules tailored to your vendor agreements.

- **Dual TPS Metrics**: Track both **Generation TPS** (generation latency denominator) and **End-to-End TPS** (full round-trip denominator).
- **Time to First Token (TTFT)**: Measure millisecond-accurate TTFT latency, with optional read-only linkage to executor sibling plugins.
- **Rate Overrides & Time Tiers**: Customize input, output, cache-read, and cache-write rates per model or configure time-tier multipliers.

![Model Performance Matrix](assets/screenshots/05-model-performance.png)

![Custom Pricing Dialog](assets/screenshots/06-pricing-modal.png)

---

### 5. Per-Request Audit Logs & Troubleshooting

Inspect the full lifecycle of every API call to diagnose latency bottlenecks and upstream anomalies.

- **Granular Log Columns**: Inspect timestamps, models, providers, API key labels, full token breakdowns, TTFT, and total duration.
- **Quick Status Filtering**: Filter for successful or failed calls to inspect HTTP 500 errors, upstream timeouts (504), or quota limits.

![Per-Request Audit Logs](assets/screenshots/07-request-logs.png)

---

## Quick Start

### 1. Download and Install

Download the pre-compiled shared library for your platform from [Releases](https://github.com/AITNR/cap-token-usage-tracker/releases) and place it in your CLIProxyAPI `plugins` folder:

| Platform & Architecture | Target Path |
|---|---|
| Linux amd64 | `plugins/linux/amd64/cap-token-usage-tracker.so` |
| Linux arm64 | `plugins/linux/arm64/cap-token-usage-tracker.so` |
| Windows amd64 | `plugins/windows/amd64/cap-token-usage-tracker.dll` |
| macOS (Intel) | `plugins/darwin/amd64/cap-token-usage-tracker.dylib` |
| macOS (Apple Silicon) | `plugins/darwin/arm64/cap-token-usage-tracker.dylib` |

### 2. Configure CLIProxyAPI

Enable the plugin in your `config.yaml`:

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

### 3. Open the Dashboard

Launch CLIProxyAPI and open the dashboard endpoint in your browser:

```text
http://127.0.0.1:8317/v0/resource/plugins/cap-token-usage-tracker/dashboard
```

Unlock with your CLIProxyAPI **Management Key** to view all metrics and charts.

---

## Security & Privacy Boundary

- **Zero Content Logging**: The plugin never records prompts, input context, model completions, streaming chunks, or request headers.
- **Key Anonymization**: Raw API keys are never stored in plaintext. They are indexed via keyed HMAC-SHA256 non-reversible fingerprints and encrypted at rest under cryptographic generations.
- **100% Local Storage**: All telemetry stays strictly in your embedded local `bbolt` database file without phoning home.
- **Session Authentication**: External requests directly hitting metrics endpoints without an authenticated session receive `401 Unauthorized`.

---

## Technical Architecture & API Reference

For CGO compilation commands, cross-compilation instructions, database zero-filling algorithms, and complete RESTful management API documentation, please refer to the technical reference document:

👉 **[Technical Architecture & API Reference (docs/TECHNICAL.md)](docs/TECHNICAL.md)**

---

## License

This project is licensed under the [MIT License](LICENSE).
