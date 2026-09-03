# 🌮 Nacho Flow VS Code Companion Extension

A lightweight, high-visibility UI companion for the **Nacho Flow** AI routing gateway. Provides real-time financial telemetry, route inspection, auto-tuning advisor, spot deal discovery, and lifecycle management directly inside VS Code and Cursor.

---

## ✨ Features

### 📊 Real-Time Financial Telemetry & Status Bar HUD
- **Zero-Polling SSE Updates**: Live streaming token metrics, monthly cost savings, and request velocity without polling overhead.
- **Interactive Status Bar**: View instant local vs cloud split and dollar savings directly in your editor status bar.

### 🧭 Route History & Traffic Inspection
- **Per-Turn Inspector**: View every recent routing decision with token counts, latency breakdown, matching tier, and retry counts.
- **Modal Classification & In-Chat Directives**: Inspect whether turns were routed via adaptive token bounds, code keywords, active tool calls, or in-prompt HotSauce directives (`@nacho:*`). Control session switches (`@nacho:kickstart-off`, `@nacho:toggles`, `@nacho:reset`) directly from chat.

### 🎛️ 1-Click Auto-Tuning Optimizer
- **Empirical AST Synthesizer**: Run statistical odds-ratio optimization on your local `traffic.jsonl` logs directly from the webview dashboard.
- **Diff & Apply**: Review recommended token boundary thresholds and keyword exclusion rules, then apply them atomically to `config.yaml` with automatic backups.

### 🔥 Spot Market & Heat Seeker Deals
- **Live Deal Alerts**: Real-time discount and promotional pricing discovery across cloud model providers.
- **1-Click Tier Adoption**: Adopt subsidized model deals directly into your active routing tiers with a single click.

### 🚦 Circuit Breaker & Health Management
- **Provider Status Monitoring**: Real-time health tracking for local engines (Ollama, vLLM) and cloud APIs (OpenRouter, DeepSeek, Anthropic).
- **Manual Circuit Reset**: Instantly reset tripped circuit breakers after resolving local VRAM or endpoint outages.

### 🛠️ Visual Configuration Editor & Pre-Flight Validation
- **Interactive Rule Editor**: Edit routing tiers, token bounds, and model fallbacks with instant syntax validation.
- **Actionable Diagnostics**: Clear, structured error messages with 1-click `[📝 Open config.yaml]` resolution toasts on port collisions or YAML syntax errors.

---

## 📋 Requirements

- **Nacho Flow Daemon**: Version `v0.8.0` or later (can be auto-spawned locally or run remotely).
- **VS Code**: Version `1.80.0` or later (also fully compatible with Cursor and VSCodium).

---

## 🚀 Getting Started

### 1. Install Extension
Install from the Visual Studio Code Marketplace or via command line:
```bash
code --install-extension dixieflatline76.nacho-flow
```

### 2. Launch or Connect Daemon
- If `nacho-flow` binary is in your system `PATH`, the extension automatically supervises and launches the local embedded daemon.
- To connect to an existing local or remote gateway, configure `nachoFlow.daemonUrl` in VS Code Settings (default: `http://127.0.0.1:8000`).

### 3. Open Dashboard
Press `Cmd+Shift+P` (macOS) or `Ctrl+Shift+P` (Windows/Linux) and select:
```text
Nacho Flow: Show Dashboard
```

---

## ⚙️ Configuration Settings

| Setting | Default | Description |
| :--- | :--- | :--- |
| `nachoFlow.daemonUrl` | `http://127.0.0.1:8000` | HTTP endpoint URL of the active Nacho Flow routing gateway. |
| `nachoFlow.authToken` | `""` | Optional Bearer authentication token if required by your gateway daemon. |
| `nachoFlow.autoStartDaemon` | `true` | Automatically spawn and supervise local `nacho-flow` binary if detected. |
| `nachoFlow.showStatusBar` | `true` | Display real-time cost savings and local routing percentage in the status bar. |

---

## ⌨️ Command Palette Reference

| Command | Identifier | Description |
| :--- | :--- | :--- |
| **Show Dashboard** | `nacho-flow.showDashboard` | Opens the full visual telemetry and route history dashboard. |
| **Open Config Editor** | `nacho-flow.openConfig` | Opens active `config.yaml` for interactive editing. |
| **Run Auto-Tuner** | `nacho-flow.runOptimizer` | Analyzes historical traffic logs and generates optimal rule recommendations. |
| **Reset Circuit Breaker** | `nacho-flow.resetCircuit` | Clears tripped provider circuit breakers and resumes traffic. |
| **Refresh Deals** | `nacho-flow.refreshDeals` | Queries upstream pricing for spot market promotional discounts. |
| **Refresh Statistics** | `nacho-flow.refreshStats` | Forces an immediate refresh of local and cloud token telemetry. |

---

## 🏛️ Architecture: Thin-Client Doctrine

This extension strictly adheres to the **Thin-Client Doctrine**:
1. **Single Source of Truth**: All configuration resides exclusively in `config.yaml`—zero duplicated settings in VS Code workspace state.
2. **Zero Core Logic in TypeScript**: All routing evaluations, token estimations, normalizers, and cost optimizations execute in the high-performance Go daemon.
3. **Reactive SSE Transport**: Consumes server-sent events with zero polling loops, preserving editor battery and CPU cycles.

---

## 📄 License

The VS Code Companion Extension is open source under the **MIT License**.
The core Nacho Flow daemon is licensed under **GNU AGPL-3.0**.