# 🌮 Nacho Flow v0.6.0: Official VS Code Extension, Zero-Crash Tool Normalizer & AGPL-3.0 Dual-Licensing

Nacho Flow **v0.6.0** is a major platform milestone delivering the official VS Code Companion Extension, universal 8-format tool call normalization for local models, in-prompt HotSauce directives, empirical A/B benchmark verification, and transition to a **GNU AGPL-3.0 + Commercial Dual-Licensing Model**.

---

### 🧩 Official VS Code Companion Extension (Zero-Friction Control)
* **Zero Terminal Management**: Start, stop, and restart the Nacho Flow daemon directly from your Activity Bar sidebar with streaming live logs.
* **Instant 1-Click Agent Setup**: Quick-copy buttons for Base URL, API Key, and Model ID (`nacho-hybrid`) pre-configured for Roo Code, Cline, and Cursor.
* **Full-Page Financial Dashboard**: Real-time interactive glassmorphic webview displaying live cost savings graphs, active route inspection, circuit breaker health, and live YAML configuration editing.
* **Inference Runtime Auto-Discovery**: Automatic status chips and health checks for local Ollama, vLLM, SGLang, and remote OpenRouter endpoints.
* **Separate Local & Remote Settings**: Independent configuration state preserving remote host URLs and auth tokens when toggling between workstation and remote GPU servers.

---

### 🛠️ Universal 8-Format Tool Normalizer (Zero Agent Crashes on Cheap Models)
* **Eliminates Tool Parsing Failures**: Open-weight and cheaper models (Qwen, DeepSeek, Mistral, Llama 3) frequently emit raw XML or markdown instead of standard tool calls, causing agent harnesses (Roo Code, Cline) to crash or waste expensive retry turns.
* **Transparent Real-Time Translation**: Automatically converts 8 distinct LLM tool variations into strict OpenAI JSON schemas before the agent harness receives them, unlocking 100% reliable tool calling on $0.00 local models.

---

### 🌶️ HotSauce Directives & In-Prompt Steering (Zero-Config Overrides)
* **Turn-by-Turn Dynamic Routing**: Developers can override routing tiers directly inside their prompt (e.g. `@nacho:tier=cloud_flagship` or `@nacho:force=anthropic`) for complex refactors without modifying configuration files.
* **$0.00 In-Prompt Telemetry**: Diagnostic tags (`@nacho:status`, `@nacho:stats`, `@nacho:deals`) return live proxy telemetry, uptime, and pricing drops instantly with zero upstream API calls.

---

### 🔥 Heat Seeker: Live Model Deals & 1-Click Adoption
* **Automatic Price Drop Scout**: Continuously monitors 300+ frontier and open-weight models to surface flash discounts, subsidized endpoints, and free models.
* **1-Click Substitution**: Adopt newly discounted models into your active `config.yaml` tiers directly from the VS Code QuickPick with full comment preservation.

---

### ⚡ Real-Time IDE Control Plane & Core Engine
* **High-Throughput IPC Backbone**: Zero-polling Server-Sent Events (`/api/v1/events`) stream live cost metrics, route transitions, and error states directly to the official VS Code extension.
* **Code-Aware Adaptive Token Estimator**: Continuously calibrates character-to-token ratios for dense code diffs and markdown, eliminating premature tier escalations and unexpected context length overflow errors.
* **Historical Financial Recalculation**: Background audit capability recalculating historical `traffic.jsonl` logs to guarantee accurate financial totals.

---

### 🔬 Empirical A/B Benchmark Whitepaper & TCO Model
* **Proven 87%–92% Cost Reduction**: Comprehensive evaluation of real-world multi-file coding tasks comparing pure cloud baselines ($6.86) against hybrid local routing ($0.57).
* **Failure Recovery Mathematical Model**: Incorporates developer recovery friction cost into TCO calculations, proving hybrid routing preserves first-pass task completion.
* **5 Executive ROI Charts**: Publication-grade data visualizations covering per-turn token snowball curves, hardware absorption, and 10-engineer team fleet savings ($167k/year).

---

### ⚖️ Licensing & Commercial Dual-License
* **GNU AGPL-3.0 + API Interoperability Exception**: The core Go daemon is licensed under AGPL-3.0 with an explicit **Section 7 Exception** ensuring calling applications and private prompts are not considered derivative works. The official VS Code Extension is licensed under **MIT** for zero-friction developer adoption.
* **Commercial & OEM Licensing**: Dedicated commercial licenses available for enterprise fleet management, corporate AGPL exemptions, priority support SLAs, and closed-source software embedding ([COMMERCIAL_LICENSE.md](COMMERCIAL_LICENSE.md) / `karl@spicebox.dev`).

---

### 📊 Verification, Stability & Performance
* **100% Test Suite Coverage**: All 150/150 TypeScript extension unit tests and Go test suites passing with race detector (`-race`).
* **Sub-Millisecond Speed**: Sustains **30,000+ req/s** with $< 0.2\text{ms}$ routing overhead.
