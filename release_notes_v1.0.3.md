### 🌮 Nacho Flow v1.0.3: In-Flight Tool Cycle Breaking & Router Resilience

Nacho Flow is an open-source, high-performance agent supervisor and model dispatcher written in pure Go. It sits between autonomous coding agents (Cline, Zoo Code, Cursor, OpenCode, Aider, Continue) and LLM providers to monitor token streams in real time, terminate runaway loops, resuscitate stalled agents, and dynamically route prompts across local GPU models ($0.00) and cloud reasoning APIs.

---

### 🚀 What's New in v1.0.3

#### 🛡️ 1. In-Flight Tool Loop Killer: Catch Runaway Commands Before They Run
* **The Problem:** When an agent gets stuck in a reasoning loop, local and frontier models sometimes emit zero conversational text and instead stream endless repetitive tool arguments (for example, chaining identical shell commands like `sed -i ... && sed -i ...` dozens of times in a single payload). Previously, loop breakers only watched conversational prose text, allowing pure tool-call repetition loops to burn thousands of tokens and freeze editor harnesses before anyone noticed.
* **The Solution:** Nacho Flow's **Cycle Killer** now inspects streaming tool arguments chunk-by-chunk in real time. If an agent starts repeating the same command or parameter pattern, Nacho Flow severs the stream mid-flight and injects a clean system recovery notice **before** the runaway command ever reaches your terminal or files.
* **Zero-Latency Fast Path:** Operates directly on raw streaming SSE bytes with zero additional memory allocations ($0\text{ B/op}$), keeping proxy pass-through latency sub-millisecond.

---

#### 🔍 2. Smart Shell Write Detection & Failing-Test Escalation
* **The Problem:** When an autonomous coding agent encounters a failing unit test, it often spirals into an inspection loop—running `cat`, `grep`, or `read_file` over and over without ever editing code. Under previous write-only guardrails, running any terminal command was mistakenly counted as "forward progress", resetting the retry counter and trapping the agent in an endless local failure loop.
* **The Solution:** Nacho Flow now inspects shell commands at the syntax level to cleanly differentiate mutating commands (`sed -i`, `>`, `>>`, `| tee`, `patch`, `git restore`) from read-only inspections (`cat`, `ls`, `grep`, `head`). 
* **Automatic Cloud Escalation:** Merely reading files while tests fail no longer counts as forward progress. Retries accumulate accurately, auto-escalating the task to a high-reasoning cloud model to solve the roadblock.

---

#### 📈 3. Router Resilience & Test Coverage Boost
* Added comprehensive edge-case test suites for token estimators, custom rule AST cache invalidation, and tier fallback handlers.
* Increased `pkg/router` statement coverage from **94.1%** to **97.9%**, lifting global repository coverage to **96.7%** (with all 16 Go packages $\ge 95.1\%$).

---

#### 📋 4. Repository Governance & Roadmap Alignment
* Configured an official repository label taxonomy (`area/*`, `type/*`, `priority/*`).
* Established and linked active milestones to the **Nacho Flow Roadmap** GitHub Project.

---

### ⚡ Architectural Highlights (v1.0 Foundation)

#### 🚀 1. Wire-Speed Pass-Through & Streaming Routing ($30,000+\text{ req/s}$)
**Engineered in pure Go for zero-latency reverse proxying.**
* **Zero Allocations on the Fast Path:** Hand-tuned SSE streaming pipeline eliminates unnecessary buffer copies and allocations during in-flight token dispatch.
* **SIMD & Bitwise Acceleration:** Fast ASCII case-insensitive scanning using branchless bit manipulation ($0\text{ B/op}$).
* **Single-Pass Struct Parsing:** Replaced generic dynamic JSON unmarshaling with fast concrete struct decoding in `Classify()`.
* **Fast Byte Stream Scanner:** Dedicated `extractContentFast` scanner to extract streaming deltas directly from byte slices without intermediate copies.
* **The Result:** Sustains **30,284+ req/s** with negligible overhead: **$0.184\text{ ms}$** raw pass-through proxy latency and **$0.205\text{ ms}$** full deep-inspection latency.

---

#### 🛡️ 2. Delimiter Tag Defense & Loop Notification
**Active prompt-injection resilience and clear feedback.**
* **Streaming Delimiter Defense:** Prevents `<channel|>` and unicode-escaped delimiter leakage across streaming SSE chunk boundaries, safeguarding against malformed model token emissions desynchronizing editor buffers.
* **Clear Loop Severing Banner:** When the Cycle Killer detects an infinite reasoning loop, it cleanly severs the stream and injects a formatted Markdown **"Loop Detected"** notice into the agent context with actionable suggestions (e.g. escalating to `@nacho:cloud` or `@nacho:reasoning`).
* **Expanded Tool Registry:** Full recognition for `editor` and `run_commands` interactive tools across modern agent harnesses.

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

#### 🔌 7. Engine Isolation & Persistent Auto-Resume
* **Dedicated Engine Modes:** Configure `nachoFlow.engineMode` (`local` vs `remote`) to cleanly separate your local GPU daemon from remote team endpoints. Switching modes automatically stops the local process to free ports and VRAM.
* **Persistent Auto-Resume:** The extension remembers your engine state across editor sessions and restarts automatically on VS Code launch when enabled (`nachoFlow.autoStartDaemon`).
* **Loopback & Win32 PE Security:** Embeds authentic Win32 PE metadata (`Publisher: Spicebox`) and binds daemon to `127.0.0.1` by default to eliminate firewall prompts.

---

#### 🛰️ 8. VS Code Companion Extension: Telemetry Auto-Refresh
**Real-time status bar synchronization that never sleeps.**
* **Decoupled Background Polling:** The status bar chip (`🌮 $0.00 / 65.5%`) and detailed hover tooltip refresh continuously in the background at your chosen cadence (15s, 30s, or 60s), even when the dashboard webview is hidden.
* **Zero-Lag Status Bar:** Metric totals, savings percentages, and active preset badges stay synchronized across both the status bar and the dashboard webview.
* **Visibility-Gated Route Polling:** Heavy route tables and per-model token distributions only poll when the dashboard panel is open, maximizing IDE responsiveness.

---

### 🧪 Verification Matrix

| Check / Metric | Scope | Result | Status |
| :--- | :--- | :--- | :---: |
| **Go Test Coverage (`test-cover`)** | All 16 Go packages | **96.7%** statement coverage (all $\ge 95.1\%$) | ✅ Passed |
| **Go Static Analysis (`vet`)** | Full repository | 0 warnings, 0 errors (`go vet ./...`) | ✅ Passed |
| **Go Race Detector (`test-race`)** | Full test suite | 0 data races (`go test -race ./...`) | ✅ Passed |
| **VS Code Extension Suite** | Extension core & webview | **14 / 14 suites, 223 / 223 tests passed (100%)** | ✅ Passed |
| **Security Audit (`sec`)** | Static AST analysis | 65 files, 15,300+ lines: **0 issues** (`gosec`) | ✅ Passed |
| **Benchmark Stress Test** | 350k live requests @ 1k workers | **30,284 req/s peak**, 0 dropped connections | ✅ Passed |
| **Documentation Link Audit** | All Markdown documents & SPA links | **0 broken links, 0 errors** | ✅ Passed |
| **Empirical Agent Validation** | Multi-turn sessions (Cline / Zoo Code) | Caught runaway in-flight command loop | ✅ Validated |

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
winget install dixieflatline76.NachoFlow
```

#### Homebrew (macOS)
```bash
brew install dixieflatline76/tap/nacho-flow
```

#### Docker / Podman Container
```bash
docker run -d -p 8000:8000 \
  -v $(pwd)/config.yaml:/config/config.yaml \
  ghcr.io/dixieflatline76/nacho-flow:v1.0.3
```
