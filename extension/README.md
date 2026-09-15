<p align="center">
  <img src="https://raw.githubusercontent.com/dixieflatline76/nacho-flow/main/images/hero-mascot.png" alt="Nacho Flow" width="700" />
</p>

# 🌮 Nacho Flow: Active Execution Runtime & Autonomous Agent Supervisor

<p align="center">
  <a href="https://marketplace.visualstudio.com/items?itemName=dixieflatline76.nacho-flow"><img src="https://img.shields.io/visual-studio-marketplace/v/dixieflatline76.nacho-flow?color=007ACC&label=VS%20Code%20Marketplace&logo=visualstudiocode&logoColor=white" alt="VS Code Marketplace"></a>
  <a href="https://github.com/dixieflatline76/nacho-flow"><img src="https://img.shields.io/badge/Platform-VS%20Code%20%7C%20Cursor-blue" alt="Platform: VS Code | Cursor"></a>
  <a href="https://github.com/dixieflatline76/nacho-flow"><img src="https://img.shields.io/badge/Bundled%20Runtime-Pure%20Go%20(Zero%20Setup)-success" alt="Bundled Pure Go Binary"></a>
  <a href="https://github.com/dixieflatline76/nacho-flow"><img src="https://img.shields.io/github/stars/dixieflatline76/nacho-flow?style=social" alt="GitHub Stars"></a>
  <a href="https://github.com/dixieflatline76/nacho-flow/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
</p>

> ### Coding agents were supposed to be fire-and-forget.
>
> *They promised you 10x efficiency. Instead, they turned you into an anxious babysitter sitting in an idling cab with the meter running, watching a ticking time bomb.*
>
> **Nacho Flow unites local mini-models, budget cloud workers, and frontier brains into a single cohesive, OpenAI-compatible hybrid model that any coding agent understands.** It seamlessly stabilizes small models so they stop crashing your harness, crushes the token snowball with in-flight context compression, and routes to frontier models only when deep reasoning is genuinely required—zero babysitting needed.
>
> 🌮 🐱 **So grab some nachos and pet your cat.**  
> **And let Nacho Flow get your coding groove back!**

<p align="center">
  <img src="https://raw.githubusercontent.com/dixieflatline76/nacho-flow/main/images/vscode-extension-showcase.png" alt="Nacho Flow VS Code Extension - Live Analytics Dashboard, Sidebar Control Hub, and Cline Pairing" width="900" />
</p>

The **Nacho Flow VS Code Companion Extension** is the official in-editor control center for autonomous coding agents ([Cline](https://github.com/cline/cline), [Zoo Code](https://www.zoocode.dev), [Cursor](https://cursor.com), [OpenCode](https://github.com/anomalyco/opencode), [Aider](https://github.com/paul-gauthier/aider), [Continue](https://continue.dev)). It bundles the compiled Go execution runtime directly inside the extension—giving you instant loop defense, in-flight context compaction, real-time financial telemetry, and zero-downtime preset switching with **zero CLI setup**.

🌐 **Website & Documentation**: [spicebox.dev/nacho-flow](https://spicebox.dev/nacho-flow/)  
Part of the **[spicebox.dev](https://spicebox.dev)** developer tool suite by [@dixieflatline76](https://github.com/dixieflatline76).

---

## ⚡ 60-Second Quickstart (Zero CLI or Go Toolchain Required)

The extension **bundles the native high-performance Go dispatch binary directly**. You do not need Go, Node servers, or Python runtimes installed:

1. **Open the Nacho Flow Sidebar**: Click the **🌮 Nacho Flow** icon in the VS Code Activity Bar (left sidebar).
2. **Launch the Engine**: Under **1. Model Dispatcher**, click **`▶ Start`**. The status chip turns `🟢 Engine Online`.
3. **Configure Your Agent**: Under **3. Coding Agents**, click **`📋 Copy`** next to:
   - **Base URL**: `http://127.0.0.1:8000/v1`
   - **Model ID**: `nacho-hybrid`
   - *(Optional API Key: `sk-nacho-secret-key`)*
4. **Paste into Your Agent**: Open **Cline**, **Zoo Code**, or **Cursor** settings → set Provider to **OpenAI Compatible** → paste the copied values.

Routine turns now run on your GPU for **$0.00**, while complex reasoning automatically escalates to Claude or DeepSeek-R1!

---

## 🥊 Why Autonomous Agents Need an Active Runtime (Not Just a Proxy)

Passive proxies like LiteLLM just blindly forward prompts. Nacho Flow actively supervises token streams, compacts context in-flight, and sanitizes tool calls so agents don't crash or burn your credit card:

| Agent Failure Mode | What Happens Without Nacho Flow | How Nacho Flow Solves It |
| :--- | :--- | :--- |
| **Runaway Loops** | Agent repeats identical broken edits 12 times, burning hours and dollars. | **Cycle Killer**: Detects n-gram loops in < 3s and severs the stream with a protocol-safe override. |
| **Context Snowball** | Every turn re-transmits 60k+ tokens of repetitive ANSI spinner logs & stale files. | **Nacho Token Saver (NTS)**: In-flight ANSI de-noising & stale read deduplication (30%–60% bloat cut). |
| **Open-Weight Crashes** | Small local models output malformed XML or bare JSON, causing 3-strike harness deadlocks. | **Strategy-Pipeline Normalizer**: Converts 8 format families into valid OpenAI `tool_calls` JSON on the fly. |
| **Planning Stalls** | Agent procrastinates in 10-turn read-only analysis without editing code. | **Kickstart**: Injects authoritative resuscitation prompts when implementation stalls. |
| **Meter Panic** | Paying $3.00/M tokens to Claude just to check `git status` or inspect a 10-line file. | **Hybrid Tier Dispatch**: Routine work runs on local GPUs for **$0.00**; bursts to Claude only when needed. |

---

## ✨ Features

### 🗜️ 1. Nacho Token Saver (NTS — In-Flight Context Compaction)

Autonomous coding agents re-send 50k–120k+ tokens of conversation history, file dumps, and terminal noise on *every single turn*. NTS compacts the bloat in-flight before the model ever sees it:

- **Zero-Alloc In-Place ANSI De-Noising**: Terminal test executions spit out thousands of raw ANSI escape sequences, spinner animations, and carriage returns (`\r`). The NTS de-noising fast path (`pkg/nts`) purges them cleanly in-flight with zero heap allocation using mutable byte slices.
- **Redundant File-Read Compaction**: When an agent inspects the same 1,000-line file four times across 20 turns, re-transmitting it burns 8,000 wasted tokens. NTS compacts stale read outputs into structural digests while keeping the active turn fresh.
- **Attention Defense for Open Weights**: Smaller open-weight models (8B–14B) suffer sharp reasoning degradation when prompt context exceeds 32k tokens. By stripping noise and deduplicating reads, NTS keeps smaller models working inside their high-accuracy attention zone.
- **Direct Cloud Invoice Protection**: When turns escalate to frontier models like Claude or DeepSeek-R1, you aren't paying $3.00/M tokens for repetitive linter logs already sent five turns ago. NTS stops the context bill from snowballing.
- **Empirical Impact**: **30%–60% context bloat eliminated** at **< 0.1ms zero-alloc Go overhead** with **zero loss** in semantic or code fidelity.

---

### 🛡️ 2. Proactive Agent Supervision (Runtime Stream & Loop Defense)

Nacho Flow supervises local and cloud open-weight models in real time, eliminating common agent failure loops:

- 🎸 **Cycle Killer (In-Flight Stream Breaker)**: Monitors live token streams across prose, thinking, and tool lanes in real time (*"Qu'est-ce que c'est?"*). Kills repetitive N-gram loops and runaway prose in < 3s, injecting a protocol-safe local $0.00 system override before escalating to cloud. Grants file writes full immunity so table-driven unit tests, repetitive structs, and boilerplate code are never falsely interrupted.
- ⚡ **Kickstart (Stall Resuscitation Engine)**: Monitors consecutive non-write turns. Auto-suspends during exploration via extensible schema detection (`HasWriteCapability`), and jolts agents out of passive read/plan procrastination when implementation stalls.
- 🧚 **Fairy Dust (Programmable Milestone Checkpoints)**: A cadenced intervention engine. You control the trigger interval (every N writes), the model, the audit prompt, and the spend cap—deploying frontier reasoning models precisely when and where quality verification matters.
- 🛡️ **Agentic Tool Fallback Shield**: Sub-nanosecond sliding tail-buffer analysis (4.67 ns/op, 0 B/op) intercepting conversational plans or questions from local models (Gemma 2, DeepSeek-R1, Qwen) in agentic IDEs (Zoo Code, Cline) and auto-synthesizing schema-compliant `ask_followup_question` tool calls to eliminate 3-strike deadlocks.

---

### 🎛️ 3. Sidebar Control Hub (Activity Bar)

Manage your agent supervisor and model dispatcher directly from your editor sidebar without obscuring your code:

- **Local vs. Remote Gateway**:
  - **This Machine**: 1-click `▶ Start`, `⏹ Stop`, `🔄 Restart`, and interactive streaming `📄 Logs` for the bundled native Go engine.
  - **Remote Server**: Connect across LAN or Tailscale (e.g. `http://192.168.1.100:8000` or `http://gpu-box.internal:8000`) with optional Bearer Auth Token and instant `⚡ Test` ping. When switching to Remote Server, the local engine is cleanly stopped to free GPU memory, and automatically resumed when you switch back to This Machine.
- **Configurable Routing Profiles with 1-Click Switching (`⚡ Switch`)**:
  - Switch on the fly between three independent, fully customizable configuration profiles (`profile1.yaml`, `profile2.yaml`, `profile3.yaml`) with native process isolation.
  - In local mode, switching cleanly restarts the native engine with the `--config <path>` flag. In remote mode, local profile mutation is safely disabled.
  - Click **`📝 Edit YAML`** (or the dashboard **`[📝 Profile X (YAML)]`** button) to open the active profile in the editor with auto-reload on save.
- **Provider Status Monitoring**: Real-time discovery and health chips for local engines (Ollama, vLLM, llama.cpp) and cloud APIs (OpenRouter, DeepSeek, Anthropic).
- **1-Click Agent Setup**: Instant copy buttons for Base URL, API Key, and Model ID, plus marketplace install buttons for Zoo Code and Cline.
- **Maintenance & Recovery**: 1-click buttons to recalculate stats from logs, reset tripped circuit breakers, or zero accumulators.

---

### 📊 4. Real-Time Analytics Dashboard (`Ctrl+Shift+P` → `Nacho Flow: Show Dashboard`)

A mission-control flight instrument webview built on a **Unified Top-Down State Snapshot Architecture**:

- **Unified State Snapshot & Monotonic Rendering**: Atomic snapshot delivery (`DashboardSnapshot`) with monotonic timestamp sequencing, eliminating visual race conditions, ghost cards, or out-of-order deliveries during rapid switches.
- **Financial Telemetry & Time Windows**: Filter metrics by **All Time**, **Today**, **Yesterday**, **This Week**, or **This Month**. Displays Total Spend, Total Savings ($ and %), Local GPU Turns ($0.00), Cloud Turns, and Billed vs. Avoided Token volume.
- **Counterfactual Savings Engine**: Calculates true mathematical cost savings comparing local turns against frontier cloud pricing, including prompt cache discounts.
- **🗜️ Nacho Token Saver (NTS) Live Telemetry Panel**: Displays real-time context compaction: **Total Tokens Saved** (1.4M+), **Payload Reduced** (5.4 MB+), and **Compacted Turns**. Visualizes active in-flight de-noising guardrails (*Dual-Lane Immunity Guard* & *Alphanumeric ASCII Protection*).
- **Live Route Inspector**: Inspect the last 500 LLM requests processed by the gateway in real time. View exact token estimates, round-trip latency, matching tier rule, provider, model ID, compaction savings, and retry recovery steps.
- **Auto-Refresh Controls**: Set background route updates to `15s`, `30s`, `60s`, or `Off`, or click `Refresh Now`.

---

### 🔥 5. Heat Seeker: Live Model Deals & 1-Click Tier Adoption

Heat Seeker continuously scans 300+ cloud models on OpenRouter, discovering flash discounts, subsidized capacity, and 100% free endpoints:

- **Deal Cards**: Displays discount percentage (up to `99% OFF` or `100% FREE`), prompt and completion pricing per 1M tokens, `🔧 Tools` support indicator, SWE-bench coding capability score (`🧠 Index XX.X`), and provider badge.
- **1-Click Tier Adoption (`⚡ Adopt`)**:
  1. Click **`⚡ Adopt`** on any discovered deal card.
  2. A VS Code QuickPick modal appears with your active tiers; recommended target tiers are marked with a **`⭐`**.
  3. Select the tier to replace. The extension creates an automatic timestamped backup (`config.yaml.bak_<timestamp>`), updates the YAML while preserving all comments, and hot-swaps the new model into the running gateway with **zero downtime**!
- **1-Click Copy Model ID**: Copy model IDs to your clipboard for instant prompt turn overrides (`@nacho:model="..."`).

---

### 🎛️ 6. 1-Click Auto-Tuning Optimizer

Click **`Run Auto-Tuner`** in the dashboard toolbar to analyze historical turns from `traffic.jsonl`:
- Statistical odds-ratio analysis calculates the optimal context boundary where local model error rates rise.
- Recommends calibrated token thresholds and keyword exclusion rules.
- Review the visual diff banner in the dashboard and click **`Apply Recommendation`** to atomically update `config.yaml` with an automatic backup.

---

### 🚦 7. Status Bar HUD & Live Telemetry Widget

A lightweight widget in your VS Code Status Bar (bottom right):
```text
🌮 Nacho Flow ● Online | $193.74 Saved All Time (5% Local)
```
- **Rich Hover Card**: Interactive tooltip displaying:
  - Active profile & server URL (`Profile 1 · http://127.0.0.1:8000`)
  - Cost metrics: Est. Cost Saved ($193.74 / 81% saved), Cloud Spend ($44.79), Local GPU share
  - **Nacho Token Saver Telemetry**: 1,426,821 tokens saved (5.4 MB saved across 852 turns)
  - **Supervisor Defense Telemetry**: 22 loops healed, 24 kickstarts, 87 fairy dust checkpoints
- **Click QuickPick**: Instant access to open the dashboard, switch presets, start/stop/restart the engine, open `config.yaml`, or reset circuit breakers.

---

### 🌶️ 8. Direct In-Chat Control Directives (`@nacho:`)

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

## 🏛️ Architecture: Thin-Client Doctrine & Zero-Alloc Systems Core

This extension strictly adheres to the **Thin-Client Doctrine**:

1. **Bundled Native Go Core (Zero Go Setup Required)**: The extension ships and manages a pre-compiled native Go static binary with wire-speed zero-alloc fast paths (`pkg/zeroalloc`). You do not need Go, Node servers, or Python runtimes installed.
2. **Sub-Millisecond Routing & Lock-Free RCU**: All routing evaluations (< 0.19 ms), token estimations, format normalizations, and cost calculations execute in compiled Go with lock-free atomic RCU (Read-Copy-Update) state synchronization.
3. **Single Source of Truth**: All configuration resides in `config.yaml`—zero duplicate settings in VS Code workspace state.
4. **Reactive SSE Transport**: Consumes server-sent events with zero polling loops, preserving editor battery and CPU cycles (**0.0% CPU when idle**).

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

Configure extension behaviors in VS Code Settings (`Ctrl+,` → search `Nacho Flow`):

| Setting | Default | Description |
| :--- | :--- | :--- |
| `nachoFlow.engineMode` | `"local"` | Operating mode: `'local'` runs the embedded daemon; `'remote'` connects to an external gateway. |
| `nachoFlow.daemonUrl` | `http://127.0.0.1:8000` | HTTP endpoint URL of the active gateway daemon (supports LAN / Tailscale). |
| `nachoFlow.authToken` | `""` | Optional Bearer Auth Token if connecting to a protected remote gateway. |
| `nachoFlow.autoStartDaemon` | `true` | Automatically resume the bundled local binary on VS Code launch if previously running. |
| `nachoFlow.showStatusBar` | `true` | Display real-time cost savings and local routing percentage in the status bar. |

---

## 📚 Documentation & Community

- **Official Website**: [spicebox.dev/nacho-flow](https://spicebox.dev/nacho-flow/)
- **Extension User Guide**: [spicebox.dev/nacho-flow/docs.html?doc=extension_guide](https://spicebox.dev/nacho-flow/docs.html?doc=extension_guide)
- **Core Engine & CLI Guide**: [spicebox.dev/nacho-flow/docs.html?doc=user_guide](https://spicebox.dev/nacho-flow/docs.html?doc=user_guide)
- **GitHub Repository**: [github.com/dixieflatline76/nacho-flow](https://github.com/dixieflatline76/nacho-flow)
- **Support & Community**: [spicebox.dev/nacho-flow/support.html](https://spicebox.dev/nacho-flow/support.html)

## 🌮 Support Nacho Flow

Nacho Flow is 100% free and open source for individual developers and open-source projects. It was born out of sheer engineering frustration while building [**Spice**](https://spicebox.dev/), when autonomous coding agents kept burning hundreds of dollars in API credits on runaway loops and planning stalls.

If Nacho Flow saved your sanity, your workflow, or your API bill:
* **Don't buy me a coffee.** Instead, consider grabbing a copy of **Spice** on the [**Mac App Store**](https://apps.apple.com/us/app/spice-wallpaper-manager/id6760980759?mt=12) or [**Microsoft Store**](https://apps.microsoft.com/detail/9NPBQ3C91WPF) (or leaving it a 5-star review). You get a beautiful, native desktop utility in return, and it directly funds continued open-source development.
* **On Linux or dev containers?** Give [**Nacho Flow a star on GitHub**](https://github.com/dixieflatline76/nacho-flow). It boosts repository visibility and helps fellow developers discover local agent routing!

---

## 📄 License

- The VS Code Companion Extension is open source under the **[MIT License](https://github.com/dixieflatline76/nacho-flow/blob/main/LICENSE)**.
- The core Nacho Flow gateway daemon is licensed under **GNU AGPL-3.0** with API Interoperability Exception.