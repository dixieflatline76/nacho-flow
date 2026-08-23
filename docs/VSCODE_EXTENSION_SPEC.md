# 🌮 Nacho Flow VS Code Extension — Developer Specification & API Guide (Draft)

> **Document Version:** `0.1.0-draft`  
> **Target Daemon Version:** `nacho-flow v0.6.0+`  
> **Audience:** Roo Code / Extension Developer  
> **Status:** Working Draft — Subject to ongoing alignment with the backend development.

---

## 1. Executive Summary & Architectural Principles

The **Nacho Flow VS Code Extension** is a lightweight, high-visibility UI companion for the `nacho-flow` AI routing gateway. It provides real-time cost visibility, route inspection, circuit breaker management, and interactive configuration editing directly inside the VS Code editor.

### The Thin-Client Doctrine
1. **Zero Core Logic in TypeScript:** The extension does **not** parse prompt tokens, evaluate `expr` AST routing rules, estimate costs, or normalize tool calls. All domain logic resides in the Go daemon.
2. **Pure Presentation & Control Layer:** The extension communicates with the daemon over local or remote HTTP (`http://127.0.0.1:8000` or a Tailscale IP) using **REST endpoints** for control actions and **Server-Sent Events (SSE)** for zero-polling real-time updates.
3. **No Mandatory Local Binary Bundling:** The extension connects to a running `nacho-flow` daemon. It can detect a locally installed binary in `PATH`, but gracefully supports remote daemons (e.g. running on a dedicated Linux GPU server).

```
┌────────────────────────────────────────────────────────┐
│                   VS Code Extension                    │
│                                                        │
│  ┌────────────────────┐      ┌──────────────────────┐  │
│  │ Status Bar Item    │      │ Webview Dashboard    │  │
│  │ $(pulse) $0.12 svd │      │ (Charts, Routes, CB) │  │
│  └─────────▲──────────┘      └──────────▲───────────┘  │
│            │ (IPC)                      │ (IPC)        │
│  ┌─────────┴────────────────────────────┴───────────┐  │
│  │         Extension Host Service (TypeScript)       │  │
│  │  - SSE Event Stream Client (EventSource/HTTP)    │  │
│  │  - REST Client with Bearer Auth                  │  │
│  │  - Connection Health State Machine               │  │
│  └──────────────────────▲───────────────────────────┘  │
└─────────────────────────┼──────────────────────────────┘
                          │ HTTP REST & SSE
                          │ (localhost:8000 or remote)
┌─────────────────────────▼──────────────────────────────┐
│             Nacho Flow Go Daemon (v0.6.0+)             │
│                                                        │
│  • In-Memory Ring Buffer (500 turns, 0ms disk I/O)     │
│  • Pub/Sub Event Broker (Broadcasts SSE events)        │
│  • RCU Atomic Config Manager + Memento Watchdog        │
│  • Circuit Breaker Registry & Pricing Oracle           │
└────────────────────────────────────────────────────────┘
```

### 1.1 Domain Context & Reference Architecture (Where to Look)

To maintain clean **Separation of Concerns**, the extension must treat `nacho-flow` as an external service strictly through the REST and SSE contracts defined in this specification. However, to design intuitive UI elements, tooltips, and sensible defaults, the developer/agent should consult the following documents in the repository for architectural mental models:

| Document / Source | What Context It Provides | Why It Matters for the UI |
| :--- | :--- | :--- |
| [`docs/ARCHITECTURE.md`](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/docs/ARCHITECTURE.md) | Comprehensive overview of Tier cascades, AST evaluation, fallback failovers, and streaming normalization. | Helps model route status badges (e.g. Local vs Cloud, Fallback, Quality Defect failover). |
| [`docs/USER_GUIDE.md`](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/docs/USER_GUIDE.md) | Standard configuration examples, supported providers (`ollama`, `vllm`, `openrouter`, `anthropic`, `openai`), and CLI usage. | Informs the Config Editor validation, autocomplete presets, and provider type badges. |
| [`docs/TUNING_GUIDE.md`](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/docs/TUNING_GUIDE.md) | Explanation of the cost-penalty optimization algorithm, safety margins, and traffic log distillation. | Informs the Tuning Dialog UI, diff preview, and savings estimation displays. |
| [`pkg/contract/config.go`](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/contract/config.go) | Canonical Go struct definitions for `Config`, `Tier`, `ProviderConfig`, and `RequestContext`. | Provides exact schema reference for rule expressions (`Tokens`, `HasImages`, `HasTools`, `Keywords`, `Retries`). |

> [!TIP]
> **Separation of Concerns Principle:** Use the reference documents above for domain semantics and terminology (e.g. knowing what a "Tier" or "Circuit Breaker" represents). **Do not** attempt to replicate backend routing evaluation, token counting, or cost estimation in TypeScript—always delegate data and actions to the `/api/v1/*` endpoints.

---

## 2. Authentication & Base URL

### Base URL
* **Default:** `http://127.0.0.1:8000`
* **Configurable Setting:** `nachoFlow.daemonUrl` (allows remote Tailscale/LAN URLs such as `http://100.103.131.43:8000`).

### Authentication Headers
All `/api/v1/*` endpoints (except `/api/v1/info` and `/health`) are protected by perimeter authentication. The extension must send the configured token in one of the following formats:
```http
Authorization: Bearer <auth_token>
```
*or*
```http
X-API-Key: <auth_token>
```

---

## 3. REST API Endpoint Reference

### 3.1 `GET /api/v1/info` — Daemon Discovery & Version Negotiation
* **Auth Required:** No (Public)
* **Purpose:** Called on extension startup to verify connectivity and negotiate capability flags.

#### Response (`200 OK`):
```json
{
  "service": "nacho-flow",
  "version": "0.6.0",
  "features": [
    "sse_telemetry",
    "ring_buffer_routes",
    "config_hot_reload",
    "circuit_breakers",
    "pricing_oracle",
    "async_tuner"
  ],
  "uptime_seconds": 3600
}
```

---

### 3.2 `GET /api/v1/events` — Real-Time SSE Telemetry Stream
* **Auth Required:** Yes (`Bearer <token>`)
* **Headers:** `Accept: text/event-stream`
* **Purpose:** Establishes a long-lived Server-Sent Events stream. The extension connects to this on startup to receive real-time UI updates without polling.
* **Keep-Alive:** The daemon sends a comment line (`: ping\n\n`) every 15 seconds to prevent proxy/socket disconnects.

#### Event Stream Format:
```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
Access-Control-Allow-Origin: *

event: route_completed
data: {"timestamp":"2026-08-22T18:00:00Z","request_id":"req-178733","tokens":1450,"selected_tier":"Tier 1: Local Free","target_model":"qwen2.5-coder:7b","provider":"ollama","is_local":true,"is_fallback":false,"latency_ms":420.5,"status_code":200,"is_retry":false,"cost_saved_usd":0.00435}

event: circuit_state_changed
data: {"provider":"ollama","state":"open","failures":2,"last_failure_unix":1755885600000000000}

event: config_updated
data: {"timestamp":"2026-08-22T18:05:00Z","version":"0.6.0","applied_rule_change":true}
```

---

### 3.3 `GET /api/v1/routes` — Circular Ring Buffer Route History
* **Auth Required:** Yes
* **Query Parameters:**
  * `limit` *(optional, default: 50, max: 500)*: Number of recent turns to retrieve.
* **Purpose:** Fetches the last $N$ prompt turns from the daemon's in-memory circular ring buffer (zero disk I/O).

#### Response (`200 OK`):
```json
{
  "total_tracked": 1420,
  "buffer_capacity": 500,
  "routes": [
    {
      "timestamp": "2026-08-22T18:10:00Z",
      "request_id": "req-178733599",
      "tokens": 42100,
      "has_images": false,
      "has_tools": true,
      "keywords": ["refactor", "test"],
      "selected_tier": "Tier 2: Cloud Coder (Qwen 3 Coder 480B)",
      "target_model": "qwen/qwen3-coder",
      "provider": "openrouter",
      "is_local": false,
      "is_fallback": false,
      "latency_ms": 1820.0,
      "status_code": 200,
      "is_retry": false,
      "cost_saved_usd": 0.0
    }
  ]
}
```

---

### 3.4 `GET /api/v1/circuits` — Circuit Breaker Status
* **Auth Required:** Yes
* **Purpose:** Returns the current state of all upstream provider circuit breakers.

#### Response (`200 OK`):
```json
{
  "circuits": [
    {
      "provider": "ollama",
      "name": "Ollama Local GPU",
      "state": "closed",
      "failures": 0,
      "failure_threshold": 2,
      "cooldown_seconds": 20,
      "is_available": true
    },
    {
      "provider": "openrouter",
      "name": "OpenRouter AI Gateway",
      "state": "closed",
      "failures": 0,
      "failure_threshold": 2,
      "cooldown_seconds": 20,
      "is_available": true
    }
  ]
}
```

---

### 3.5 `POST /api/v1/circuits/reset` — Manual Circuit Breaker Reset
* **Auth Required:** Yes
* **Payload (optional):**
```json
{
  "provider": "ollama"
}
```
*(If payload is omitted or `"provider": "all"`, all circuit breakers are reset to `closed`).*

#### Response (`200 OK`):
```json
{
  "status": "ok",
  "message": "Circuit breaker for 'ollama' reset to closed"
}
```

---

### 3.6 `GET /api/v1/pricing` — Active Pricing Oracle Rates
* **Auth Required:** Yes
* **Purpose:** Returns cached model pricing per million tokens used by the daemon for cost calculations.

#### Response (`200 OK`):
```json
{
  "benchmark_model": "anthropic/claude-3.5-sonnet",
  "benchmark_price_per_million": 3.00,
  "pricing": {
    "openrouter:qwen/qwen3-coder": {
      "prompt_cost_per_million": 0.20,
      "completion_cost_per_million": 0.60
    },
    "openrouter:anthropic/claude-3.7-sonnet": {
      "prompt_cost_per_million": 3.00,
      "completion_cost_per_million": 15.00
    },
    "ollama:qwen2.5-coder:7b": {
      "prompt_cost_per_million": 0.0,
      "completion_cost_per_million": 0.0
    }
  }
}
```

---

### 3.7 `GET /api/v1/config` — Sanitized Active Configuration
* **Auth Required:** Yes
* **Purpose:** Returns the current running configuration. Sensitive API keys and auth tokens are masked with `***` placeholders for safe UI display.

#### Response (`200 OK`):
```json
{
  "port": 8000,
  "auth_token": "sk-nacho-***",
  "providers": {
    "ollama": {
      "base_url": "http://127.0.0.1:11434"
    },
    "openrouter": {
      "base_url": "https://openrouter.ai/api/v1",
      "api_key": "sk-or-v1-***"
    }
  },
  "tiers": [
    {
      "name": "Tier 1: Local Free",
      "provider": "ollama",
      "model": "qwen2.5-coder:7b",
      "when": "Tokens < 8000 && !HasImages && !HasTools"
    },
    {
      "name": "Tier 2: Cloud Coder",
      "provider": "openrouter",
      "model": "qwen/qwen3-coder",
      "when": "Retries < 2"
    }
  ],
  "default_tier": {
    "name": "Tier 3: Cloud Reasoning Fallback",
    "provider": "openrouter",
    "model": "anthropic/claude-3.7-sonnet"
  }
}
```

---

### 3.8 `PUT /api/v1/config` — Atomic Hot-Reload with Secret Merging & Dry Run
* **Auth Required:** Yes
* **Query Parameters:**
  * `dry_run=true` *(optional)*: Validates syntax and test-compiles AST expressions without modifying runtime state or saving to disk.
* **Payload:** Complete or partial JSON config matching the schema above.

#### Behavior:
1. **Secret Deep-Merge:** Any fields containing `***` (e.g. `"api_key": "sk-or-v1-***"`) automatically inherit the existing active secret from daemon memory.
2. **AST Pre-Compilation:** Compiles all `when` expressions against `contract.RequestContext`. If any syntax error occurs, immediately rejects with `400 Bad Request`.
3. **Atomic Swap & Rollback Watchdog:** Creates a timestamped `.bak` on disk, swaps memory pointers with zero dropped requests, and arms a 30s auto-rollback watchdog.

#### Success Response (`200 OK`):
```json
{
  "status": "ok",
  "message": "Configuration validated and atomically hot-reloaded",
  "backup_file": "config.yaml.bak.20260822T180500"
}
```

#### Error Response (`400 Bad Request`):
```json
{
  "error": {
    "type": "invalid_config",
    "message": "failed to compile expr for tier 'Tier 1' (Tokens < 8000 && invalid_token): unknown name invalid_token"
  }
}
```

---

### 3.9 `POST /api/v1/tune` — Asynchronous Auto-Tuning Optimization
* **Auth Required:** Yes
* **Purpose:** Triggers heuristic cost-penalty analysis over recent traffic logs to compute optimal token thresholds and friction keywords.
* **Execution:** Non-blocking with a strict 5-second context timeout.

#### Response (`200 OK`):
```json
{
  "recommended_threshold": 6500,
  "synthesized_rule": "Tokens < 6500 && !HasImages && !HasTools && !contains_any(Keywords, ['refactor', 'architect'])",
  "estimated_monthly_savings_usd": 42.50,
  "local_offload_percentage": 68.4,
  "confidence_score": 0.94
}
```

---

### 3.10 `GET /v1/stats` — High-Level Aggregated Metrics
* **Auth Required:** Yes
* **Purpose:** Cumulative session telemetry and cost counters.

#### Response (`200 OK`):
```json
{
  "started_at": "2026-08-22T12:00:00Z",
  "total_requests": 342,
  "total_tokens_routed_locally": 1284500,
  "estimated_cost_saved_usd": 3.85,
  "tier_breakdown": {
    "tier1_local_free": 240,
    "tier2_cloud_coder": 92,
    "tier3_cloud_reasoning": 8,
    "tier4_cloud_vision": 2,
    "explicit_override": 0,
    "fallbacks": 0
  }
}
```

---

## 4. UI Component Architecture for Roo Code

Roo Code can build the extension progressively across 3 tiers:

### Tier 1: Core MVP (Status Bar & Health Indicator)
1. **Status Bar Item (`StatusBarItem`):**
   - Text: `$(pulse) Nacho: $3.85 saved (70% local)`
   - Tooltip: Total requests, current active tier, daemon connection state.
   - Click Action: Opens the Nacho Flow Dashboard Webview.
2. **Connection Health Watchdog:**
   - Connects to `/api/v1/events` SSE.
   - If SSE disconnects, shows `$(alert) Nacho: Offline` and attempts exponential backoff reconnection.
3. **Roo Code Integration Validator:**
   - Detects if VS Code `settings.json` or Roo Code provider config points to `http://localhost:8000/v1`.
   - 1-Click "Fix Roo Code Config" command.

### Tier 2: Route History & Circuit Breakers Panel (Webview)
1. **Route History Table:**
   - Rendered using VS Code Webview Toolkit or sleek modern CSS.
   - Live appends incoming `route_completed` events from SSE.
   - Badges: `Local (0ms/Free)`, `Cloud Coder ($)`, `Fallback (!)`.
2. **Circuit Breaker Status Cards:**
   - Displays real-time status of Ollama & OpenRouter.
   - Red badge if `Open` (tripped).
   - "Reset Circuit" button firing `POST /api/v1/circuits/reset`.

### Tier 3: Visual Config Editor & Auto-Tuning Panel
1. **Interactive Rule Editor:**
   - Sliders for token thresholds (`tokens < 8000`).
   - Toggles for `has_images`, `has_tools`.
   - "Test Rule" button triggering `PUT /api/v1/config?dry_run=true`.
   - "Apply Changes" button triggering live hot-reload.
2. **Tuning Advisor Card:**
   - "Run Optimizer" button calling `POST /api/v1/tune`.
   - Displays projected savings and recommended `when` expression with a 1-click "Apply Optimization" button.

---

## 5. Summary Checklist for Roo Code

| Phase | Feature | Endpoints Used | Status |
| :--- | :--- | :--- | :---: |
| **Phase 1** | Daemon Discovery & Status Bar | `GET /api/v1/info`, `GET /api/v1/events`, `GET /v1/stats` | Specification Ready |
| **Phase 2** | Route History & Circuit Breaker Webview | `GET /api/v1/routes`, `GET /api/v1/circuits`, `POST /api/v1/circuits/reset` | Specification Ready |
| **Phase 3** | Config Editor & Tuning Advisor | `GET /api/v1/config`, `PUT /api/v1/config`, `POST /api/v1/tune`, `GET /api/v1/pricing` | Specification Ready |

---

*This document is maintained directly inside the repository under [`docs/VSCODE_EXTENSION_SPEC.md`](./docs/VSCODE_EXTENSION_SPEC.md).*
