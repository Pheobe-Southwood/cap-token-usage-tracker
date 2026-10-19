# Technical Architecture & API Reference

This document contains the low-level architecture, build guidelines, and complete API specifications for **CAP Token Usage Tracker**.

---

## Table of Contents

- [System Architecture](#system-architecture)
- [Building from Source](#building-from-source)
- [Configuration Reference](#configuration-reference)
- [API Reference](#api-reference)
  - [Dashboard & Resource Endpoints](#dashboard--resource-endpoints)
  - [Management API Endpoints](#management-api-endpoints)
- [Security & Cryptography](#security--cryptography)
- [TTFT Linkage Architecture](#ttft-linkage-architecture)

---

## System Architecture

CAP Token Usage Tracker integrates with [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) through its `usage_plugin` interface and registers administration endpoints via `management_api`.

```
┌─────────────────────────────────────────────────────────────┐
│                         CLIProxyAPI                         │
│                                                             │
│   ┌────────────────────┐          ┌──────────────────────┐  │
│   │    usage_plugin    │          │    management_api    │  │
│   └─────────┬──────────┘          └──────────▲───────────┘  │
└─────────────┼────────────────────────────────┼──────────────┘
              │ Usage Records                  │ HTTP / RPC
              ▼                                │
┌──────────────────────────────────────────────┴──────────────┐
│                  cap-token-usage-tracker                    │
│                                                             │
│  ┌───────────────────────┐      ┌────────────────────────┐  │
│  │   Minute Aggregator   │      │   Dashboard & Query    │  │
│  │   & Per-Request Log   │      │   Engine (Zero-fill)   │  │
│  └──────────┬────────────┘      └────────────▲───────────┘  │
│             │                                │              │
│             ▼                                │              │
│  ┌───────────────────────────────────────────┴───────────┐  │
│  │             Embedded bbolt Storage Engine             │  │
│  │  - Bucket: minute_aggregates   - Bucket: request_logs │  │
│  │  - Bucket: preferences         - Bucket: pricing      │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### Storage Model
- **Storage Engine**: Embedded `bbolt` key-value database.
- **Aggregation**: Persists UTC-minute buckets alongside per-request logs.
- **Bucket Zero-filling**: Range queries zero-fill sparse intervals so the frontend axis always spans the requested range cleanly without gaps.

---

## Building from Source

### Prerequisites
- Go 1.26+
- `CGO_ENABLED=1`
- Cross-compilation toolchains:
  - Windows amd64: `x86_64-w64-mingw32-gcc`
  - Linux arm64: `aarch64-linux-gnu-gcc`
  - macOS: Xcode CLI or target-specific Darwin SDK

### Target Commands

```bash
# Linux amd64
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X github.com/AITNR/cap-token-usage-tracker/internal/plugin.version=1.0.0" \
  -o cap-token-usage-tracker.so .

# Linux arm64
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X github.com/AITNR/cap-token-usage-tracker/internal/plugin.version=1.0.0" \
  -o cap-token-usage-tracker.so .

# macOS amd64 (Intel)
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X github.com/AITNR/cap-token-usage-tracker/internal/plugin.version=1.0.0" \
  -o cap-token-usage-tracker.dylib .

# macOS arm64 (Apple Silicon)
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
  go build -buildmode=c-shared -trimpath -buildvcs=false \
  -ldflags="-s -w -X github.com/AITNR/cap-token-usage-tracker/internal/plugin.version=1.0.0" \
  -o cap-token-usage-tracker.dylib .
```

Windows PowerShell:
```powershell
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "1"
go build -buildmode=c-shared -trimpath -buildvcs=false `
  -ldflags="-s -w -X github.com/AITNR/cap-token-usage-tracker/internal/plugin.version=1.0.0" `
  -o dist/cap-token-usage-tracker.dll .
```

### Local Testing

```bash
gofmt -w *.go
go test -count=1 ./...
go vet ./...

# Run browser tests with Chromium / Google Chrome
REQUIRE_BROWSER_TESTS=1 CHROME_PATH=/path/to/google-chrome go test -count=1 ./...
```

---

## Configuration Reference

Add to your `config.yaml` in CLIProxyAPI:

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      priority: 0
      retention_days: 365
      flush_interval: 5s
      flush_max_records: 100
      sync_on_record: true
      api_key_secret: "replace-with-a-random-secret-at-least-32-bytes"
      response_compression: true
      response_compression_min_bytes: 1024
      session_ttl_minutes: 720
      pricing_catalog_url: "https://models.dev/api.json"
      pricing_auto_refresh: true
      pricing_stale_after: 24h
      peak_pricing: true
      ttft_supplement: true
      # ttft_supplement_dir: /path/to/cap-opencode-go
```

| Field | Default | Description |
|---|---|---|
| `data_path` | `CLIProxyAPI/data/token-usage-tracker.db` | Path to the bbolt database. |
| `retention_days` | `365` | Data retention in days (1-3650). |
| `flush_interval` | `5s` | Max batch-mode disk flush interval (1s - 1h). |
| `flush_max_records` | `100` | Flush buffer limit in batch mode. |
| `sync_on_record` | `true` | When true, commits each usage record synchronously before acknowledging. |
| `api_key_secret` | `123456` | Secret key for HMAC fingerprinting and encryption (must be >= 32 bytes in production). |
| `response_compression` | `true` | Gzip compression for dashboard and statistics responses. |
| `response_compression_min_bytes` | `1024` | Minimum payload size for compression. |
| `session_ttl_minutes` | `720` | Duration of unlocked dashboard session. |
| `pricing_catalog_url` | `https://models.dev/api.json` | Remote models catalog URL. |
| `pricing_auto_refresh` | `true` | Silently refreshes catalog in background when stale. |
| `pricing_stale_after` | `24h` | Catalog staleness threshold. |
| `peak_pricing` | `true` | Enables built-in peak/off-peak tier rules. |
| `ttft_supplement` | `true` | Enables read-only TTFT linkage from sibling `cap-opencode-go` plugin. |
| `ttft_supplement_dir` | empty | Path to sibling state directory; defaults to `../cap-opencode-go`. |

---

## API Reference

### Dashboard & Resource Endpoints

Prefix: `/v0/resource/plugins/cap-token-usage-tracker`

| Method | Path | Description | Required Header |
|---|---|---|---|
| `GET` | `/dashboard` | HTML page shell (contains no data) | None |
| `GET` | `/stats` | Full aggregates for compatible clients | `X-Plugin-Session` |
| `GET` | `/stats/initial` | First-screen summary, series, cost series, model roll-up | `X-Plugin-Session` |
| `GET` | `/stats/trends` | Downsampled model trends | `X-Plugin-Session` |
| `GET` | `/stats/groups` | Dimension-grouped statistics with server pagination | `X-Plugin-Session` |
| `GET` | `/stats/models` | Per-model metrics (TPS, TTFT, cost, tokens) | `X-Plugin-Session` |
| `GET` | `/requests` | Paginated per-request log details | `X-Plugin-Session` |
| `GET` | `/preferences` | User UI preferences | `X-Plugin-Session` |
| `GET` | `/model-prices` | Read manual prices and catalog status | `X-Plugin-Session` |

Query Parameters for `/stats/initial`:
- `range`: `today`, `yesterday`, `7d`, `30d`, or custom timestamps.
- `scale`: `auto`, `minute`, `hour`, `day`.
- `tz_offset`: Local timezone offset in minutes (-840 to 840).
- `model`: Model filter (repeatable).
- `api_key_ref`: Key fingerprint filter (repeatable).

### Management API Endpoints

Prefix: `/v0/management/plugins/cap-token-usage-tracker`

Authentication: Requires `Authorization: Bearer <CPA_MANAGEMENT_KEY>`.

| Method | Path | Description |
|---|---|---|
| `POST` | `/session` | Exchange management key for an unlocked session token |
| `POST` | `/session/revoke` | Invalidate the active session token |
| `PUT` | `/api-key-labels` | Update custom alias for an API Key fingerprint |
| `PUT` | `/model-prices` | Save custom pricing and peak hour rules |
| `POST` | `/preferences` | Save dashboard display preferences |
| `GET` | `/backup` | Download complete database backup |
| `POST` | `/restore` | Restore database (requires `X-Confirm-Restore: replace`) |
| `POST` | `/reset` | Clear all recorded statistics (requires `{"confirm":"reset"}`) |

---

## Security & Cryptography

1. **Zero Raw Key Storage**:
   - API keys are never stored in plaintext.
   - Keys are hashed with HMAC-SHA256 (`api_key_secret`) and truncated into a non-reversible reference fingerprint (`api_key_ref`).
   - Ciphertexts are encrypted using AES-GCM under cryptographic generations.
2. **Zero Prompt/Response Retention**:
   - Prompts, completions, streaming chunks, and error response bodies are dropped immediately. Only token counts, latency, and status codes are preserved.
3. **Session Authentication**:
   - Management keys are checked once by the host. A cryptographically random session token is issued and stored in `sessionStorage` on the client.
   - Unauthorized external requests to `/stats` or `/requests` receive `401 {"code":"session_required"}`.

---

## TTFT Linkage Architecture

For plugin executors where the host runtime does not record TTFT (`ttft_ns = 0`), this plugin can optionally inspect the sibling `cap-opencode-go` plugin directory in read-only mode:

- **Files Read**: `shared.json` (model mappings) and `stats-{chat,gemini,responses,messages}.json`.
- **Field Whitelist**: Only model ID, hour, TTFT nanoseconds, and sample count are parsed. Secret fields (such as `settings.keys`) are ignored and never loaded.
- **Graceful Degradation**: If the sibling plugin is not found or files are corrupted, the linkage safely falls back with no impact on core token tracking.
