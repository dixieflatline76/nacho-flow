# 🌮 Nacho Flow: VS Code & Cursor Companion Extension

<p align="center">
  <img src="https://raw.githubusercontent.com/dixieflatline76/nacho-flow/main/images/vscode-extension-showcase.png" alt="Nacho Flow VS Code Extension - Live Analytics Dashboard, Sidebar Control Hub, and Cline Pairing" width="800" />
</p>

<p align="center">
  <a href="https://marketplace.visualstudio.com/items?itemName=dixieflatline76.nacho-flow"><img src="https://img.shields.io/visual-studio-marketplace/v/dixieflatline76.nacho-flow?color=blue&label=VS%20Code%20Marketplace" alt="VS Code Marketplace"></a>
  <a href="https://marketplace.visualstudio.com/items?itemName=dixieflatline76.nacho-flow"><img src="https://img.shields.io/visual-studio-marketplace/i/dixieflatline76.nacho-flow?color=green" alt="Installs"></a>
  <a href="https://github.com/dixieflatline76/nacho-flow"><img src="https://img.shields.io/github/stars/dixieflatline76/nacho-flow?style=social" alt="GitHub Stars"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
</p>

> **You just paid $2.00 to ask your AI agent to check a log file. There's a better way.**
>
> Route routine prompt turns (log inspections, file searches, syntax fixes) to your **local workstation GPU ($0.00)** and automatically burst complex multi-file reasoning to **frontier cloud APIs** with 100% reasoning fidelity and up to **94.7% cost reduction**.

The **Nacho Flow VS Code Companion Extension** delivers a high-visibility control hub, real-time financial telemetry dashboard, preset hot-swapper, and market deal scout for autonomous coding agents ([Zoo Code](https://github.com/zoocodeorganization/zoo-code), [Cline](https://github.com/cline/cline), [Cursor](https://www.cursor.com), [OpenCode](https://github.com/anomalyco/opencode), [Aider](https://github.com/paul-gauthier/aider), [Continue](https://continue.dev)).

---

## ⚡ 60-Second Quickstart (Zero CLI Required)

The extension **bundles the native high-performance Go routing binary directly**. You do not need Go installed or any command-line setup:

1. **Open the Nacho Flow Sidebar**: Click the **🌮 Nacho Flow** icon in the VS Code Activity Bar (left sidebar).
2. **Launch the Engine**: Under **1. Routing Engine**, click **`▶ Start`**. The status chip turns `🟢 Engine Online`.
3. **Configure Your Agent**: Under **3. Coding Agents**, click **`📋 Copy`** next to:
   - **Base URL**: `http://127.0.0.1:8000/v1`
   - **Model ID**: `nacho-hybrid`
   - *(Optional API Key: `sk-nacho-secret-key`)*
4. **Paste into Your Agent**: Open **Zoo Code**, **Cline**, or **Cursor** settings $\rightarrow$ set Provider to **OpenAI Compatible** $\rightarrow$ paste the copied values.

Routine turns now run on your GPU for **$0.00**, while complex reasoning automatically escalates to Claude or DeepSeek-R1!

---

## 🌟 Key Features

### 🎛️ 1. Sidebar Control Hub (Activity Bar)

Manage your hybrid routing gateway directly from your editor sidebar without obscuring your code:

- **Local vs. Remote Gateway**:
  - **This Machine**: 1-click `▶ Start`, `⏹ Stop`, `🔄 Restart`, and interactive streaming `📄 Logs` for the bundled native Go engine.
  - **Remote Server**: Connect across LAN or Tailscale (e.g. `http://192.168.1.100:8000` or `http://gpu-box.internal:8000`) with optional Bearer Auth Token and instant `⚡ Test` ping.
- **Routing Presets with 1-Click Hot-Swap (`⚡ Hot-Swap`)**:
  - Switch between tailored agent configurations with **zero daemon restarts and zero dropped streams**:
    - **🌮 Standard** (`config.yaml`): Balanced context bounds (16k local), general-purpose coding rules for Cursor, Aider, and Continue.
    - **🤖 Zoo Code** (`config.zoo.yaml`): Calibrated for strict OpenAI JSON tool calling, tighter prose limits (800 words), and aggressive Cycle Killer loop murder.
    - **🛠️ Cline XML-Native** (`config.cline.yaml`): Relaxed prose token ceilings (`max_prose_tokens: 6144`) and XML tool extraction (`<write_to_file>`, `<replace_in_file>`), preventing false stream severing during conversational XML drafting.
  - Click **`📝 Edit YAML`** to open the active preset in the editor with auto-reload on save.
- **Provider Status Monitoring**: Real-time discovery and health chips for local engines (Ollama, vLLM, llama.cpp) and cloud APIs (OpenRouter, DeepSeek, Anthropic).
- **1-Click Agent Setup**: Instant copy buttons for Base URL, API Key, and Model ID, plus marketplace install buttons for Zoo Code and Cline.
- **Maintenance & Recovery**: 1-click buttons to recalculate stats from logs, reset tripped circuit breakers, or zero accumulators.

---

### 📊 2. Real-Time Analytics Dashboard (`Ctrl+Shift+P` → `Nacho Flow: Show Dashboard`)

A mission-control flight instrument webview providing total visibility into your AI agent economics:

- **Financial Telemetry & Time Windows**:
  - Filter metrics by **All Time**, **Today**, **Yesterday**, **This Week**, or **This Month**.
  - Displays Total Spend, Total Savings ($ and %), Local GPU Turns ($0.00), Cloud Turns, and Billed vs. Avoided Token volume.
  - **Counterfactual Savings Engine**: Calculates true mathematical cost savings comparing local turns against frontier cloud pricing, including prompt cache discounts.
- **Live Route Inspector**:
  - Inspect the last 500 LLM requests processed by the gateway in real time.
  - View exact token estimates, round-trip latency, matching tier rule, provider, model ID, and retry recovery steps.
- **Auto-Refresh Controls**: Set background route updates to `15s`, `30s`, `60s`, or `Off`, or click `Refresh Now`.

---

### 🛡️ 3. "The Three Fixes" Live Defense Telemetry

Nacho Flow wraps open-weight and non-frontier models in active runtime guardrails, eliminating the common agent failure loops:

- 🎸 **Cycle Killer (In-Flight Loop Defense)**:
  - Murders circular deliberation loops and runaway monologues in $<3$ seconds before they burn GPU compute or drain your wallet.
  - Visualizes murdied loops, avoided runaway GPU minutes, and local self-healing rate ($0.00 recovery via `[SYSTEM OVERRIDE]` prompts).
- ⚡ **Kickstart Resuscitation**:
  - Shocks agents out of lazy read-only planning spirals back into active file edits and terminal commands.
  - **Automatic Plan-Mode Protection**: Intelligently suspends stall escalation when the agent enters Plan Mode (when declared tools lack write capabilities), allowing uninterrupted exploration.
- 🧚 **Fairy Dusting (Frontier Quality Checkpoints)**:
  - Tracks productive file writes and periodically routes turns to frontier models (Claude Sonnet 5, Claude Opus 5) to audit syntax and architectural integrity before errors cascade.

---

### 🔥 4. Heat Seeker: Live Model Deals & 1-Click Tier Adoption

Heat Seeker continuously scans 300+ cloud models on OpenRouter, discovering flash discounts, subsidized capacity, and 100% free endpoints:

- **Deal Cards**: Displays discount percentage (up to `99% OFF` or `100% FREE`), prompt and completion pricing per 1M tokens, `🔧 Tools` support indicator, SWE-bench coding capability score (`🧠 Index XX.X`), and provider badge.
- **1-Click Tier Adoption (`⚡ Adopt`)**:
  1. Click **`⚡ Adopt`** on any discovered deal card.
  2. A VS Code QuickPick modal appears with your active tiers; recommended target tiers are marked with a **`⭐`**.
  3. Select the tier to replace. The extension creates an automatic timestamped backup (`config.yaml.bak_<timestamp>`), updates the YAML while preserving all comments, and hot-swaps the new model into the running gateway with **0ms interruption**!
- **1-Click Copy Model ID**: Copy model IDs to your clipboard for instant prompt turn overrides (`@nacho:model="..."`).

---

### 🎛️ 5. 1-Click Auto-Tuning Optimizer

Click **`Run Auto-Tuner`** in the dashboard toolbar to analyze historical turns from `traffic.jsonl`:
- Statistical odds-ratio analysis calculates the optimal context boundary where local model error rates rise.
- Recommends calibrated token thresholds and keyword exclusion rules.
- Review the visual diff banner in the dashboard and click **`Apply Recommendation`** to atomically update `config.yaml` with an automatic backup.

---

### 🚦 6. Status Bar HUD & Hover Card

A lightweight widget in your VS Code Status Bar (bottom right):
```text
🌮 $45.81 Saved Today (2% Local)
```
- **Hover Card**: Rich Markdown tooltip displaying active daemon status, active preset (`Cline`), today's spend/savings (`+$45.81 / 75% saved`), token volume, and quick links.
- **Click QuickPick**: Opens a quick menu to open the dashboard, switch presets, start/stop/restart the engine, open `config.yaml`, or reset circuit breakers.

---

### 🌶️ 7. Direct In-Chat Control Directives (`@nacho:`)

Steer routing and toggle session guardrails directly from your prompt in **Zoo Code**, **Cline**, or **Cursor** without opening settings:

| Directive | Type | Action / Effect |
| :--- | :--- | :--- |
| `@nacho:toggles` | Inspection | Displays live session switches ($0.00 cost / 0 tokens) |
| `@nacho:status` | Inspection | Displays live daemon uptime, spend, savings, and circuits |
| `@nacho:reset` | Management | Hard resets session turns and restores default guardrails |
| `@nacho:kickstart-off` / `on` | Session Switch | Suspend / resume Kickstart idle stall escalation |
| `@nacho:cyclekiller-off` / `on` | Session Switch | Suspend / resume Cycle Killer stream loop breaker |
| `@nacho:shield-off` / `on` | Session Switch | Suspend / resume synthetic `ask_followup_question` tool calls |
| `@nacho:raw-on` / `off` | Session Switch | Enable / disable raw unadulterated upstream SSE stream |
| `@nacho:fairydust-off` / `on` | Session Switch | Suspend / resume periodic frontier checkpoints |
| `@nacho:local` | Single Turn | Force current turn to Local GPU ($0.00) |
| `@nacho:cloud` | Single Turn | Force current turn to Cloud Fallback tier |
| `@nacho:reasoning` | Single Turn | Force current turn to DeepSeek-R1 / o1 |

---

## 🏛️ Architecture: Thin-Client Doctrine

This extension strictly adheres to the **Thin-Client Doctrine**:

1. **Single Source of Truth**: All configuration resides in `config.yaml`—zero duplicate settings in VS Code workspace state.
2. **Zero Core Logic in TypeScript**: All routing evaluations, token estimations, normalizers, and cost calculations execute in compiled Go inside the daemon.
3. **Reactive SSE Transport**: Consumes server-sent events with zero polling loops, preserving editor battery and CPU cycles (**0.0% CPU when idle**).

---

## ⌨️ Command Palette Reference

All features can be triggered via `Ctrl+Shift+P` / `Cmd+Shift+P`:

| Command | Identifier | Description |
| :--- | :--- | :--- |
| **Show Dashboard** | `nacho-flow.showDashboard` | Opens the full visual telemetry and route history dashboard. |
| **Open Controls in Sidebar** | `nacho-flow.openSettings` | Focuses the Nacho Flow control panel in the Activity Bar. |
| **Open Config Editor** | `nacho-flow.openConfig` | Opens the active preset YAML file in the editor. |
| **Run Auto-Tuner & Optimize** | `nacho-flow.runOptimizer` | Analyzes historical logs and recommends optimized context thresholds. |
| **Refresh Heatseeker Deals** | `nacho-flow.refreshDeals` | Scans upstream pricing oracles for model discounts and subsidized endpoints. |
| **Reset Circuit Breaker** | `nacho-flow.resetCircuit` | Clears tripped provider circuit breakers and restores traffic. |
| **Refresh Statistics** | `nacho-flow.refreshStats` | Forces an immediate refresh of local and cloud token telemetry. |
| **Set Auth Token** | `nacho-flow.setAuthToken` | Prompts for Bearer Auth Token for connecting to authenticated gateways. |
| **Set Timeframe (Today / Week / Month / All Time)** | `nacho-flow.setTimeWindow*` | Switches the active analytics reporting horizon. |
| **Open User Guide & Documentation** | `nacho-flow.openDocs` | Opens the online User Guide at spicebox.dev. |
| **Open Support & Community** | `nacho-flow.openSupport` | Opens the Nacho Flow Support & Community portal. |

---

## ⚙️ Extension Settings

Configure extension behaviors in VS Code Settings (`Ctrl+,` $\rightarrow$ search `Nacho Flow`):

| Setting | Default | Description |
| :--- | :--- | :--- |
| `nachoFlow.daemonUrl` | `http://127.0.0.1:8000` | HTTP endpoint URL of the active gateway daemon (supports LAN / Tailscale). |
| `nachoFlow.authToken` | `""` | Optional Bearer Auth Token if connecting to a protected remote gateway. |
| `nachoFlow.autoStartDaemon` | `true` | Automatically spawn and supervise the bundled local binary on VS Code launch. |
| `nachoFlow.showStatusBar` | `true` | Display real-time cost savings and local routing percentage in the status bar. |

---

## 📚 Documentation & Community

- **Official Website & Docs Hub**: [spicebox.dev/nacho-flow](https://spicebox.dev/nacho-flow/)
- **Interactive Documentation**: [spicebox.dev/nacho-flow/docs.html](https://spicebox.dev/nacho-flow/docs.html)
- **Full Extension User Guide**: [docs/EXTENSION_USER_GUIDE.md](https://github.com/dixieflatline76/nacho-flow/blob/main/docs/EXTENSION_USER_GUIDE.md)
- **GitHub Repository**: [github.com/dixieflatline76/nacho-flow](https://github.com/dixieflatline76/nacho-flow)
- **Support & Community**: [spicebox.dev/nacho-flow/support.html](https://spicebox.dev/nacho-flow/support.html)

---

## 📄 License

- The VS Code Companion Extension is open source under the **[MIT License](LICENSE)**.
- The core Nacho Flow gateway daemon is licensed under **GNU AGPL-3.0** with API Interoperability Exception.