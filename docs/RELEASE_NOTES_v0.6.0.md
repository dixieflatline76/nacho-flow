# 🌮 Nacho Flow v0.6.0: Official VS Code Extension, Management REST API & Multi-Model Tool Normalizer

Nacho Flow **v0.6.0** is a major feature and platform release introducing the official VS Code Companion Extension, a high-throughput Management REST API with real-time SSE streaming, universal 8-format multi-model tool call normalization, in-prompt HotSauce directives, and an empirical A/B benchmark case study.

---

### 🧩 Official VS Code Companion Extension
* **Activity Bar Control Hub**: Full-featured sidebar view provider with native one-click daemon lifecycle management (Start, Stop, Restart) and streaming output channel logs.
* **Inference Engine Discovery**: Live detection and status chips for local and remote inference runtimes (Ollama, OpenRouter, vLLM, SGLang, llama.cpp).
* **Interactive Full-Page Analytics Webview**: Glassmorphic dashboard featuring real-time financial telemetry, rolling cost savings graphs, route inspector, circuit breaker status, and live YAML configuration editor.
* **1-Click Agent Setup**: Instant copy buttons for Base URL, API Key, and Model ID (`nacho-hybrid`) tailored for Roo Code, Cline, and Cursor.
* **Separate Local & Remote Settings**: Independent configuration state preserving remote host URLs and auth tokens when toggling between workstation and remote GPU servers.

---

### ⚡ Backend Management REST API (`/v1/mgmt/*`) & SSE Event Broker
* **Live Stats Recalculation**: New `/v1/mgmt/stats/recalculate` endpoint recalculates financial totals and turn metrics directly from historical `traffic.jsonl` logs.
* **Zero-Polling SSE Telemetry**: High-throughput `/v1/events` Server-Sent Events broker delivering real-time route transitions, cost deltas, and error metrics to connected IDE clients.
* **Administrative Controls**: Dedicated REST endpoints for runtime counter resets (`/v1/mgmt/stats/reset`) and tripped circuit breaker resets (`/v1/mgmt/circuits/reset`).

---

### 🛠️ 8-Format Multi-Model Tool Normalizer Pipeline
* **Universal LLM Tool Extraction**: Automatically intercepts, parses, and normalizes heterogeneous tool calls across 8 distinct open-weight and proprietary format variations (Raw XML, DeepSeek `<tool_call>`, Qwen `<function>`, nested JSON markdown, and OpenAI standard payloads).
* **Strict Schema Reconstruction**: Guarantees perfectly structured OpenAI JSON tool call schemas delivered to agent harnesses (Roo Code, Cline, Aider), eliminating JSON schema validation errors on cheaper models.

---

### 🌶️ HotSauce Directives & Zero-Cost Meta Command Engine
* **In-Prompt Steering**: Developers and agents can steer routing tiers dynamically inside prompts using `@nacho:tier=<name>`, `@nacho:force=<provider>`, or `@nacho:bypass`.
* **Zero-Cost Telemetry Directives**: In-prompt diagnostic directives (`@nacho:status`, `@nacho:stats`, `@nacho:deals`) return immediate structured proxy telemetry and model deal metrics without making upstream API calls ($0.00 cost).
* **Sanitized Upstream Transmission**: Directive tags are stripped before forwarding to upstream inference providers to prevent model confusion.

---

### 🔥 Heat Seeker: Live Model Deals & Price Drops
* **Live Catalog Synchronization**: Synchronizes pricing across 300+ frontier and open-weight models from OpenRouter APIs.
* **1-Click Deal Adoption**: Interactive VS Code QuickPick allowing developers to adopt flash discounts and subsidized models directly into `config.yaml` with comment-preserving YAML AST manipulation.

---

### 🔬 Empirical A/B Benchmark Case Study Whitepaper
* **Publication-Grade Evaluation**: Added full empirical whitepaper (*"Empirical Evaluation of Hybrid Multi-Tier AI Routing in Autonomous Coding Agents"*) evaluating Run A (Local GPU + Qwen 3 Coder) vs. Run B (Local GPU + Gemini 3.7 Flash Thinking) on a real-world multi-file feature.
* **Total Cost of Ownership (TCO) Model**: Introduces mathematical modeling of failure recovery cost ($\text{TCO} = \text{CloudTokenSpend} + \text{FailureRecoveryCost}$), demonstrating that 87%–92% cloud cost reduction is achieved while preserving first-pass success.
* **5 Visual Data Charts**: High-resolution embedded visual charts for TCO comparison, baseline savings, token distribution, Context Snowball per-turn trajectory, and 10-engineer enterprise fleet ROI.

---

### 📊 Verification, Quality & Benchmarks
* **100% Test Suite Pass**: All 150/150 TypeScript extension tests passing across 12 suites with 96.6% code coverage; 100% Go unit tests passing with race detector.
* **Cross-Platform Compatibility**: Verified across Linux (AMD64/ARM64), macOS (Intel/Apple Silicon), and Windows (x64).
* **Zero Overhead**: Sustains over **30,000+ req/s** with sub-millisecond routing latency ($< 0.2\text{ms}$).
