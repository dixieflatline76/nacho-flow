# 🌮 Nacho Flow v0.6.0: Official VS Code Extension, Universal Tool Normalizer & AGPL-3.0 Dual-Licensing

Nacho Flow **v0.6.0** is a major platform milestone delivering the official VS Code Companion Extension, universal 8-format tool call normalization for local models, in-prompt HotSauce directives, empirical A/B benchmark verification, and transition to a **GNU AGPL-3.0 + Commercial Dual-Licensing Model**.

---

### 🧩 1. Official VS Code Companion Extension (Zero-Friction Control)
* **Zero Terminal Management**: Start, stop, and restart the Nacho Flow daemon directly from your Activity Bar sidebar with streaming live logs.
* **Instant 1-Click Agent Setup**: Quick-copy buttons for Base URL, API Key, and Model ID (`nacho-hybrid`) pre-configured for Roo Code, Cline, and Cursor.
* **Full-Page Financial Dashboard**: Real-time interactive glassmorphic webview displaying live cost savings graphs, active route inspection, circuit breaker health, and live YAML configuration editing.
* **Separate Local & Remote Settings**: Independent configuration state preserving remote host URLs and auth tokens when toggling between workstation and remote GPU servers.
* 💡 **Why It Matters (The "So What?")**:
  * **For Developers**: You never have to switch to a terminal or edit JSON configs. One click connects Roo Code, Cursor, or Cline to your local or remote GPU gateway.
  * **For IT Admins**: Engineers on managed laptops can connect to shared on-premise GPU rigs over LAN/VPN with encrypted credential storage in the OS keychain.

---

### 🛠️ 2. Universal 8-Format Tool Normalizer (Zero Broken Tool Calls)
* **Eliminates Tool Parsing Failures**: Open-weight and cheaper models (Qwen, DeepSeek, Mistral, Llama 3) frequently emit raw XML, markdown code fences, or unescaped JSON instead of standard OpenAI tool call payloads.
* **Transparent Real-Time Translation**: Automatically converts 8 distinct LLM tool variations into strict OpenAI JSON schemas before the agent harness receives them.
* 💡 **Why It Matters (The "So What?")**:
  * **For Developers**: Unlocks reliable agentic workflows on $0.00 local models. Autonomous agents won't crash, hallucinate missing tools, or burn expensive retry loops on cheap models.
  * **For IT Admins**: Enables development teams to safely route 80%+ of tool-heavy agent steps to self-hosted GPUs without sacrificing agent reliability.

---

### 🌶️ 3. HotSauce Directives & In-Prompt Steering (Zero-Config Overrides)
* **Turn-by-Turn Dynamic Routing**: Developers can override routing tiers directly inside their prompt (e.g. `@nacho:tier=frontier` or `@nacho:force=anthropic`) for complex refactors without touching config files.
* **$0.00 In-Prompt Telemetry**: Diagnostic tags (`@nacho:status`, `@nacho:stats`, `@nacho:deals`) return live proxy telemetry, uptime, and pricing drops instantly with zero upstream API calls.
* 💡 **Why It Matters (The "So What?")**:
  * **For Developers**: Instant surgical control. When tackling a brutal architecture task, you can summon frontier models for a single prompt turn without breaking your flow.

---

### 🔥 4. Heat Seeker: Live Model Deals & 1-Click Adoption
* **Automatic Price Drop Scout**: Continuously monitors 300+ frontier and open-weight models to surface flash discounts, subsidized endpoints, and free models.
* **1-Click Substitution**: Adopt newly discounted models into your active `config.yaml` tiers directly from the VS Code QuickPick with full comment preservation.
* 💡 **Why It Matters (The "So What?")**:
  * **For Developers & Teams**: Automatically capitalizes on provider price wars and subsidized inference without manual benchmark hunting.

---

### ⚡ 5. Real-Time Control Plane & Code-Aware Token Estimator
* **Code-Aware Adaptive Token Estimator**: Continuously calibrates character-to-token ratios for dense code diffs and markdown, eliminating premature tier escalations and context overflows.
* **High-Throughput IPC Backbone**: Zero-polling Server-Sent Events (`/api/v1/events`) stream live cost metrics, route transitions, and error states directly to the official VS Code extension.
* **Historical Financial Recalculation**: Background audit capability recalculating historical `traffic.jsonl` logs to guarantee accurate financial totals.
* 💡 **Why It Matters (The "So What?")**:
  * **For Developers**: Prevents agent harnesses from unexpectedly failing due to context length mismatches on large diffs.
  * **For IT Admins**: Provides auditable, tamper-resistant financial metrics and historical log reconciliation.

---

### 🔬 6. Empirical A/B Benchmark Whitepaper & Mathematical TCO Model
* **Proven 87%–92% Cost Reduction**: Comprehensive evaluation of real-world multi-file coding tasks comparing pure cloud baselines ($6.86) against hybrid local routing ($0.57).
* **Failure Recovery Mathematical Model**: Incorporates developer recovery friction cost into TCO calculations ($\text{TCO} = \text{CloudTokenSpend} + \text{FailureRecoveryCost}$), proving hybrid routing preserves first-pass task completion.
* **5 Executive ROI Charts**: Publication-grade data visualizations covering per-turn token snowball curves, hardware absorption, and 10-engineer team fleet savings ($167k/year).
* 💡 **Why It Matters (The "So What?")**:
  * **For Engineering Leaders**: Delivers peer-reviewed data and ROI formulas needed to justify local GPU hardware investments to finance and executive teams.

---

### ⚖️ 7. Licensing & Commercial Dual-License Model
* **GNU AGPL-3.0 Core Engine + API Interoperability Exception**: The core Go daemon is licensed under AGPL-3.0 with an explicit **Section 7 Exception** ensuring calling applications (`/v1/*`), prompt inputs, and private models are not considered derivative works.
* **MIT-Licensed VS Code Extension**: Frictionless installation for developers on corporate-managed laptops with automated compliance scanners.
* **Commercial & OEM Licensing**: Dedicated commercial licenses for enterprise fleet management, corporate AGPL exemptions, priority support SLAs, and closed-source embedding ([COMMERCIAL_LICENSE.md](COMMERCIAL_LICENSE.md) / `karl@spicebox.dev`).
* 💡 **Why It Matters (The "So What?")**:
  * **For Developers**: Free and open-source forever for individual development and OSS projects.
  * **For Enterprise Legal / IT**: Clean compliance boundaries with zero copyleft risk to internal proprietary codebases, plus an indemnified commercial path for enterprise fleet deployments.

---

### 📊 8. Verification, Stability & Performance
* **100% Test Suite Coverage**: All 150/150 TypeScript extension unit tests and Go test suites passing with race detector (`-race`).
* **Sub-Millisecond Speed**: Sustains **30,000+ req/s** with $< 0.2\text{ms}$ routing overhead.
