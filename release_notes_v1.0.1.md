### 🌮 Nacho Flow v1.0.1: The Agent Supervisor & Model Dispatcher

Nacho Flow is an open-source, high-performance agent supervisor and model dispatcher written in pure Go. It sits between autonomous coding agents (Cline, Zoo Code, Cursor, OpenCode, Aider, Continue) and LLM providers to monitor token streams in real time, terminate runaway loops, resuscitate stalled agents, and dynamically route prompts across local GPU models ($0.00) and cloud reasoning APIs.

---

### 🚀 What's New in v1.0.1

* **VS Code Extension Optimization & Lean Packaging:**
  * Configured `vscode:prepublish` to automatically compile and bundle extension code with `esbuild`, enabling aggressive tree-shaking.
  * Audited and removed unused `eventsource` external dependency; the extension now operates with **zero external runtime npm dependencies**, relying exclusively on native Node HTTP transport for streaming telemetry.
* **Automated Multi-Platform Marketplace Publishing:**
  * Upgraded GitHub Actions CI workflow to automatically publish pre-built packages for all 5 target platforms (**Windows x64, macOS Apple Silicon, macOS Intel, Linux x64, Linux ARM64**) directly to the Visual Studio Code Marketplace upon release promotion.
  * Hands-free distribution pipeline: builds are verified across all matrix jobs before public distribution occurs.
* **Strict Local vs. Remote Engine Isolation & Persistent Auto-Resume:**
  * Strict separation between local embedded daemon and remote server modes: suppressed background health checks, route polling, and SSE reconnect loops when the local daemon is offline.
  * Cross-session daemon auto-resume via VS Code `globalState` (honoring `nachoFlow.autoStartDaemon`).
  * Mode-switch lifecycle management: stopping local processes on remote switch to free ports and GPU resources while preserving user intent to run locally upon return.
  * Resolved safe working directory for daemon spawning to prevent running in the VS Code installation folder.
  * Added transparent JSON error payload extraction in API client.
* **Documentation & Commercial Dual-Licensing Clarity:**
  * Clarified the open-source scope for individual developers and OSS contributors under GNU AGPL-3.0 with API Interoperability Exception, while maintaining clear boundaries for enterprise OEM and fleet licensing.
  * Added the authentic origin story and native desktop utility backing links for [Spice](https://spicebox.dev) on the Mac App Store and Microsoft Store.
  * Replaced static hardcoded version badges in extension documentation with dynamic Shields.io release endpoints.

---

### ⚡ Architectural Highlights (v1.0 Foundation)

#### 🚀 1. Wire-Speed Pass-Through & Streaming Routing ($28,900+\text{ req/s}$)
**Engineered in pure Go for zero-latency reverse proxying.**
* **Zero Allocations on the Fast Path:** Hand-tuned SSE streaming pipeline eliminates unnecessary buffer copies and allocations during in-flight token dispatch.
* **SIMD & Bitwise Acceleration:** Fast ASCII case-insensitive scanning using branchless bit manipulation ($0\text{ B/op}$).
* **Single-Pass Struct Parsing:** Replaced generic dynamic JSON unmarshaling with fast concrete struct decoding in `Classify()`.
* **Fast Byte Stream Scanner:** Dedicated `extractContentFast` scanner to extract streaming deltas directly from byte slices without intermediate copies.
* **The Result:** Sustains **28,966+ req/s** with negligible overhead: **$0.184\text{ ms}$** raw pass-through proxy latency and **$0.205\text{ ms}$** full deep-inspection latency.

---

#### 🛡️ 2. Delimiter Tag Defense & Loop Notification
**Active prompt-injection resilience and clear feedback.**
* **Streaming Delimiter Defense:** Prevents `<channel|>` and unicode-escaped delimiter leakage across streaming SSE chunk boundaries, safeguarding against malformed model token emissions desynchronizing editor buffers.
* **Clear Loop Severing Banner:** When the Cycle Killer detects an infinite reasoning loop, it cleanly severs the stream and injects a formatted Markdown **"Loop Detected"** notice into the agent context with actionable suggestions (e.g. escalating to `@nacho:cloud` or `@nacho:reasoning`).
* **Expanded Tool Registry:** Added full recognition for `editor` and `run_commands` interactive tools across modern agent harnesses.

---

#### 👁️ 3. Automatic Multimodal Vision Routing
**Never let text-only local models choke on screenshots.**
* **The Problem:** When you drop screenshots, UI mockups, or diagrams into an agent chat, dispatching to a local text-only model produces immediate errors or hallucinations.
* **The Solution:** Nacho Flow inspects incoming payload structures for image data (e.g., `image_url` or base64 multimodal blocks). If detected, it deterministically routes the prompt to the configured vision-capable model tier, preserving visual reasoning without requiring you to switch configurations manually.

---

#### 🧠 4. Plan-Mode Guard: Intelligent Schema Awareness
**Auto-suspends stall detection during legitimate exploration turns.**
* **The Problem:** In write-only kickstart mode, previous versions treated turns spent exploring codebases, reading files, searching symbols, or asking user questions as "idle" turns. If an agent entered an intentional planning or review phase, false-positive stall overrides would break the agent's train of thought.
* **The Solution:** Nacho Flow inspects the active tool definitions passed in the request payload:
  * If the model *only* has access to exploration/planning tools (`read_file`, `list_dir`, `grep_search`, `ask_question`), Kickstart stall counting is **automatically suspended**.
  * Stall accumulation only ticks when the model actively possesses code modification capabilities (`write_to_file`, `replace_file_content`, `apply_diff`) yet repeatedly evades executing them.

---

#### 🌶️ 5. HotSauce In-Band Chat Directives (`@nacho:...`)
**Steer model dispatching and safeguards directly from your chat prompt.**
* Type runtime directives directly into user prompt messages. Nacho Flow intercepts the directive, applies the modification instantly, and scrubs the tag from the payload before forwarding to the model:
  * `@nacho:local` — Force dispatch to local GPU workhorse tier ($0.00).
  * `@nacho:cloud` — Force dispatch to cloud utility model.
  * `@nacho:reasoning` — Escalate current turn to frontier reasoning tier.
  * `@nacho:kickstart-off` / `@nacho:kickstart-on` — Temporarily toggle stall resuscitation.
  * `@nacho:cyclekiller-off` / `@nacho:cyclekiller-on` — Toggle repetition loop termination.
  * `@nacho:shield-off` / `@nacho:shield-on` — Toggle active circuit breaker guardrails.
  * `@nacho:status` — Inspect current session telemetry, tier distribution, and active presets.
  * `@nacho:reset` — Reset session circuit breaker counters and idle accumulators.

---

#### 🕹️ 6. Safe Cold Startup Maintenance & Log Rotation
**Clean log rotation without daemon crashes.**
* **Unified Management API (`POST /api/v1/directive`):** Programmatic control-plane endpoint for runtime maintenance (`PURGE_ALL_LOGS`, `RESET_CIRCUITS`, `RECALCULATE_STATS`).
* **Safe Log Rotation (`cmd/nacho-flow/directive.go`):** On log purge, an atomic command envelope is staged. Upon daemon restart, `executeStartupDirectives()` safely rotates `traffic.jsonl` and `router.log` to timestamped backups (`.bak.YYYYMMDD-HHMMSS`) before loggers lock files, unlinks `stats.json`, and cleans up cleanly.
* **VS Code Extension Integration:** The sidebar action is upgraded to **Rotate Logs & Reset Stats**, backed by an automatic health polling recovery loop in `controller.ts` that gracefully reconnects to the daemon post-restart.

---

#### 🛑 7. Smarter Test-Loop Breaker
**Breaks autonomous agent death spirals on failing test loops.**
* If a test suite fails (e.g. `FAIL: TestQueue`, `1 failed`, `build failed`), the agent **must** perform concrete file write modifications to count as making progress. Analyzing or reading test files without modifying code no longer grants write immunity.

---

#### 🛰️ 8. VS Code Companion Extension: Telemetry Auto-Refresh
**Real-time status bar synchronization that never sleeps.**
* **Decoupled Background Polling:** The status bar chip (`🌮 $0.00 / 65.5%`) and detailed hover tooltip refresh continuously in the background at your chosen cadence (15s, 30s, or 60s), even when the dashboard webview is hidden.
* **Zero-Lag Status Bar:** Metric totals, savings percentages, and active preset badges stay synchronized across both the status bar and the dashboard webview.
* **Visibility-Gated Route Polling:** Heavy route tables and per-model token distributions only poll when the dashboard panel is open, maximizing IDE responsiveness.

---

### 🧪 9. Verification Matrix

| Check / Metric | Scope | Result | Status |
| :--- | :--- | :--- | :---: |
| **Go Test Coverage (`test-cover`)** | All 18 Go packages | **96.3%** statement coverage | ✅ Passed |
| **Go Static Analysis (`vet`)** | Full repository | 0 issues (`go vet ./...`) | ✅ Passed |
| **Go Race Detector (`test-race`)** | Full test suite | 0 data races (`go test -race ./...`) | ✅ Passed |
| **VS Code Extension Suite** | Extension core & webview | **14 / 14 suites, 212 / 212 tests passed (100%)** | ✅ Passed |
| **Security Audit (`sec`)** | Static AST analysis | 65 files, 14,600+ lines: **0 issues** (`gosec`) | ✅ Passed |
| **Benchmark Stress Test** | 350k live requests @ 1k workers | **28,966 req/s peak**, 0 dropped connections | ✅ Passed |
| **Empirical Agent Validation** | Multi-turn sessions (Cline / Zoo Code) | Caught 2 runaway loops, 100% test pass rates | ✅ Validated |

---

### 📦 Installation & Quickstart

#### VS Code Companion Extension
Install directly from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=dixieflatline76.nacho-flow) or via CLI:
```bash
code --install-extension dixieflatline76.nacho-flow
```

#### Universal Shell Installer (Linux & macOS)
```bash
curl -fsSL https://raw.githubusercontent.com/dixieflatline76/nacho-flow/main/scripts/install.sh | bash
```

#### Windows (Winget)
```powershell
winget install dixieflatline76.nacho-flow
```

#### Homebrew (macOS)
```bash
brew install dixieflatline76/tap/nacho-flow
```

#### Docker / Podman Container
```bash
docker run -d -p 8000:8000 \
  -v $(pwd)/config.yaml:/config/config.yaml \
  ghcr.io/dixieflatline76/nacho-flow:v1.0.1
```
