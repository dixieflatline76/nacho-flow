### 🌮 Nacho Flow v1.1.0: 3-Profile Switcher, In-Flight XML Write Protection, Zero-Flicker Dashboard & High-Concurrency Scaling

Nacho Flow is an open-source, high-performance agent supervisor and model dispatcher written in pure Go. It sits between autonomous coding agents (Cline, Zoo Code, Cursor, OpenCode, Aider, Continue) and LLM providers to monitor token streams in real time, terminate runaway loops, unstuck frozen agents, and dynamically route prompts across local GPU models ($0.00) and cloud reasoning APIs.

---

### 🚀 What's New in v1.1.0

#### 🎛️ 1. Instant 3-Profile Switcher (Profiles 1, 2, & 3)
* **The Problem:** Developers switch between very different workflows, agent harnesses (Cline, Zoo Code, Cursor, Aider), and tasks throughout the day. Previously, switching between local GPU offloading, strict schema enforcement, and relaxed token bounds required manually editing YAML configuration files or restarting the daemon with custom command-line flags.
* **The Solution:** Nacho Flow introduces a dedicated **Profile Switcher** directly in the VS Code status bar and sidebar, offering three independent, fully user-customizable configuration slots:
  * **Profile 1** (`profile1.yaml` / `config.yaml`)
  * **Profile 2** (`profile2.yaml`)
  * **Profile 3** (`profile3.yaml`)
* **Total User Freedom:** Profiles 1, 2, and 3 are completely open slots. You can configure each profile for any combination of local models (Ollama, vLLM, LM Studio), cloud providers (OpenRouter, Anthropic, OpenAI, DeepSeek), token limits, and loop-killer rules.
* **Native `--config` Flag & Automatic Restart:** Selecting a profile in the VS Code UI immediately passes the native `--config <path>` flag to `nacho-flow.exe` and triggers a seamless, graceful daemon restart.
* **Workspace & Global Storage Resolution:** Nacho Flow resolves configuration hierarchically:
  `Workspace (.nacho/profile*.yaml) → Global Storage (~/.nacho-profiles/) → Bundled Presets`
* **1-Click Config Editing:** Click **"Edit Active Profile"** in the sidebar to open the active profile's YAML configuration directly in your editor.

---

#### 📊 2. Rock-Solid, Zero-Flicker Dashboard Sync
* **The Problem:** In high-speed workflows—such as rapidly toggling profiles, connecting to remote servers, or processing bursts of streaming agent turns—fragmented background updates previously caused race conditions, out-of-order message delivery, and "ghost" cards or frozen counters in the webview.
* **The Solution:** The extension webview and controller now operate on a unified state snapshot pipeline:
  * **Guaranteed In-Order Updates:** All dashboard events use monotonic timestamps. Delayed background updates are automatically dropped so older network packets can never overwrite fresh metrics.
  * **Zero-Flicker Single-Pass Render:** Replaced piecemeal DOM updates with a single, atomic render loop backed by VS Code state persistence, guaranteeing instant UI updates with zero flickering.
  * **Non-Blocking Telemetry Aggregation:** The extension controller gathers daemon health, status bar metrics, route history, and tier distributions concurrently without slowing down the editor.
  * **Instant Offline Cleanup:** Disconnecting or stopping the engine instantly clears telemetry cards, route tables, and charts, preventing stale or confusing data display.

---

#### 🛡️ 3. In-Flight XML File Write Protection (Cline & Claude Dev Immunity)
* **The Problem:** Autonomous agents like Cline, Claude Dev, and Hermes stream file writes wrapped inside prose XML tags (`<write_to_file path="...">`, `<replace_in_file>`, `<str_replace_editor>`) directly within regular chat text, rather than using structured JSON tool calls. Traditional prose cycle breakers treated large code diffs and repetitive test tables as loop text, prematurely killing the stream during legitimate file edits.
* **The Solution:** Nacho Flow's streaming normalizer now inspects content deltas in real time:
  * **In-Flight Write Detection:** Detects opening XML write tags as they stream across the wire, even when tags are split across network packet boundaries.
  * **Dedicated Write Runway:** The instant a file write tag is detected, the stream is dynamically routed to a dedicated write lane.
  * **Zero False-Positive Kills:** Legitimate file edits and unit test tables bypass sliding N-gram loop detection while remaining safely bounded by a generous **32,768-token** ceiling (`max_write_tokens: 32768`).
  * **Sub-Microsecond Overhead:** Operates at **173.8 ns/op**, **0 B/op**, and **0 memory allocations**.

---

#### ⚡ 4. Clean Stream Severing & Fix for "Position 515" JSON Crashes
* **The Problem:** When previous cycle breakers terminated a runaway stream mid-flight, cutting the connection mid-JSON argument string caused client-side V8 parsers (e.g., Zoo Code / Lumo Max) to crash with fatal unhandled syntax errors (`SyntaxError: Expected ':' at position 515`).
* **The Solution:** Re-engineered Cycle Breaker stream severance to emit standard OpenAI protocol-compliant SSE error frames, cleanly omitting `finish_reason: "stop"` to signal an aborted error turn rather than truncated JSON.
* **Regression-Verified:** Backed by live regression replay tests against the exact 102-turn Zoo Code N-Queens failure payload.

---

#### 🧠 5. Deep Reasoning Runway for Frontier Models (Claude, o1, Gemini)
* **The Problem:** Flagship reasoning and deep chain-of-thought models (Claude Sonnet 5, Gemini 3.7 Flash Extended Thinking, OpenAI o1/o3, DeepSeek R1) produce lengthy multi-step deliberations. Repetition thresholds calibrated for 7B-14B open-weight models prematurely clipped complex reasoning turns.
* **The Solution:** Built-in Frontier Immunity automatically detects flagship reasoning models (configured in `data/reasoning.json`) and turns escalated via Fairy Dust, bypassing sliding N-gram repetition loops while maintaining overall token budget guardrails.
* **Fresh Slate on Fallback:** Failed attempts on local models don't count against cloud fallback models, giving smart models a clean slate to solve the roadblock.

---

#### 📦 6. Universal Agent Catalog (Auto-Detection for Cline, Cursor, Zoo, Aider)
* **Decoupled Architecture:** Replaced fragile string pattern matching with a pre-compiled, extensible catalog in pure Go backed by embedded JSON schemas (`data/agents/` for Cline, Zoo Code, Cursor, Aider, Anthropic, Standard).
* **Instant Lookups:** Pre-compiled static hash maps provide instant classification in **26.7 ns/op** and **0 B/op**.
* **Pluggable Agent Schemas:** Agent manifests cleanly define tool calling formats, reasoning delimiters, and error signatures out-of-the-box.

---

#### 🚀 7. High-Concurrency Performance Boost (23,000+ req/s Under Heavy Load)
* **The Problem:** Under 1,000 concurrent worker stress testing, shared internal locks on logging and classification paths caused severe thread contention, dropping throughput to 8,916 req/s and spiking tail latency to 430 ms.
* **The Solution:** Eliminated internal lock bottlenecks in favor of lock-free atomic pointer swapping, non-blocking ring-buffer channel draining for traffic logs, and object recycling for cycle breaker instances (using Go 1.21 `clear()` for zero-allocation reuse).
* **Stress Test Benchmark Results (1,000 Concurrent Workers):**
  * **Throughput:** Jumped from 8,917 req/s to **23,663 req/s** (**+165.4% boost**).
  * **P99 Tail Latency:** Dropped from 431 ms to **83.8 ms** (**-80.5% reduction**).
  * **Peak Heap Memory:** Decreased from 474 MB to **267 MB** (**-43.7% reduction**).

---

#### 🌐 8. Remote Server Mode with Zero-VRAM Workstation Hibernation
* **Dedicated Engine Modes:** Configure `nachoFlow.engineMode` (`local` vs `remote`) to cleanly separate your local GPU workstation daemon from remote lab or team servers.
* **Zero-VRAM Workstation Hibernation:** Switching to Remote Mode automatically stops the local engine process, instantly freeing workstation TCP ports, RAM, and GPU VRAM.
* **Intent-Preserving Auto-Resume:** Switching back to Local Mode automatically restarts the local daemon without requiring manual intervention.
* **Secure Token Storage:** Remote server bearer tokens are securely isolated in VS Code's encrypted credential vault (`vscode.SecretStorage`).
* **Context-Aware Guardrails:** Local-only controls (Start/Stop Engine, Kill Daemon, Profile Switching) are safely disabled with clear feedback when connected to a remote server.

---

#### 🔬 9. Multi-Agent Benchmarks: Real-World Cost Savings & Quality Insights
* **Head-to-Head Multi-Agent Challenge:** Documented Runs 6 & 7 in the Empirical A/B Case Study:
  * **Cline:** Completed the full Go N-Queens implementation in **50 turns**, achieving **94.8% statement coverage** for **$0.87**.
  * **Zoo Code:** Completed in 102 turns for **$5.50** with zero crashes.
* **Fleet Economics:** Across 2,068 production API requests and 78.2M tokens, Nacho Flow delivered **65.5% net fleet savings** ($86.28 spend vs. $250.37 unrouted baseline).
* **Catching "Test Weakening":** Documented real-world cases where autonomous agents modified unit test assertions (`t.Logf("Known bug: expected 92. Test passes if it doesn't crash")`) to force green CI builds, proving why Nacho Flow's runtime oversight and frontier verification are non-negotiable.

---

#### 🛑 10. Clean Process Management (Zero Zombie Background Processes)
* **Tree-Kill Process Cleanup:** Upgraded daemon process lifecycle management with Windows tree-killing (`taskkill /PID <pid> /T /F`) and POSIX process group termination (`process.kill(-pid, 'SIGKILL')`).
* **Zero Orphaned Daemons:** Eliminates orphaned background processes across rapid editor restarts or profile switches.

---

#### 📚 11. Comprehensive Documentation & Architecture Sync
* Synchronized all documentation across both the repository and the documentation website (`docs/` and `site/docs/`):
  * **Architecture Guide:** Documented the Profile Switcher, In-Flight XML Write Protection, and Unified Snapshot pipeline.
  * **Extension User Guide:** Added complete walkthroughs for Profiles 1, 2, and 3, Remote Server connection, and secure credential storage.
  * **Core User Guide:** Updated YAML configuration references and Cycle Breaker settings.
  * **Extension README:** Updated feature matrices and getting-started workflows.

---

#### 🧪 12. Test-Driven Development & Global Coverage (≥ 95%)
* **VS Code Extension Suite:** **263 / 263 passed tests** across 14 test suites with **97.35% statement coverage**, **98.02% line coverage**, **95.47% function coverage**, and **82.08% branch coverage**.
* **Go Core Suite:** **100% test pass rate** across all 16 packages with ≥ 96.6% coverage. Zero data races (`go test -race ./...`), zero security vulnerabilities (`gosec`).

---

### ⚡ Architectural Highlights (v1.0 & v1.1 Foundation)

#### 🚀 1. Wire-Speed Pass-Through & Streaming Routing (30,000+ req/s)
**Engineered in pure Go for zero-latency reverse proxying.**
* **Zero Allocations on the Fast Path:** Hand-tuned SSE streaming pipeline eliminates unnecessary buffer copies and allocations during in-flight token dispatch.
* **SIMD & Bitwise Acceleration:** Fast ASCII case-insensitive scanning using branchless bit manipulation (0 B/op).
* **Single-Pass Struct Parsing:** Replaced generic dynamic JSON unmarshaling with fast concrete struct decoding in `Classify()`.
* **Fast Byte Stream Scanner:** Dedicated `extractContentFast` scanner to extract streaming deltas directly from byte slices without intermediate copies.
* **The Result:** Sustains **30,284+ req/s** with negligible overhead: **0.18 ms** raw pass-through proxy latency and **0.20 ms** full deep-inspection latency.

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

#### 📡 7. SSE Data Prefix Calibration & Token Usage Normalization (Lumo / Proton)
* **Zero-Allocation Sub-Slicing:** Two-branch sub-slicing (`trimmed[6:]` and `trimmed[5:]`) in `StreamNormalizer.processLine` handles both `data: ` and `data:` formats, capturing token usage and cost across all upstream providers without allocations.

---

#### 🔑 8. Dynamic Environment Variable Resolution (`ENV_`)
* **Zero-Touch Secret Management:** `ApplyConfig` dynamically expands all `ENV_<VAR_NAME>` placeholders against the active process environment at runtime during hot-reloads and profile switches.

---

### 🧪 Verification Matrix

| Check / Metric | Scope | Result | Status |
| :--- | :--- | :--- | :---: |
| **Go Test Coverage (`test-cover`)** | All 16 Go packages | **96.6%** statement coverage (all ≥ 95.1%) | ✅ Passed |
| **Go Static Analysis (`vet`)** | Full repository | 0 warnings, 0 errors (`go vet ./...`) | ✅ Passed |
| **Go Race Detector (`test-race`)** | Full test suite | 0 data races (`go test -race ./...`) | ✅ Passed |
| **VS Code Extension Suite** | Extension core & webview | **14 / 14 suites, 263 / 263 tests passed (100%)** | ✅ Passed |
| **Security Audit (`sec`)** | Static AST analysis | 65 files, 15,300+ lines: **0 issues** (`gosec`) | ✅ Passed |
| **Benchmark Stress Test** | 350k live requests @ 1k workers | **30,284 req/s peak**, 0 dropped connections | ✅ Passed |
| **Documentation Link Audit** | All Markdown documents & SPA links | **0 broken links, 0 errors** | ✅ Passed |
| **Windows PE Metadata Audit** | `cmd/nacho-flow` Win32 resources | Publisher: `Spicebox`, Version: `1.1.0.0` | ✅ Passed |

---

### 📦 Installation & Quickstart

#### VS Code Companion Extension (Recommended)
Install directly from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=dixieflatline76.nacho-flow) or via CLI:
```bash
code --install-extension dixieflatline76.nacho-flow
```
* Select your desired profile (**Profile 1**, **Profile 2**, or **Profile 3**) directly from the status bar chip (`🌮`).
* Point your autonomous coding agent (Cline, Zoo Code, Cursor, OpenCode, Aider) to `http://127.0.0.1:8000/v1`.

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
  ghcr.io/dixieflatline76/nacho-flow:v1.1.0
```

#### Standalone Go Daemon (Build from Source)
```bash
go build -o nacho-flow ./cmd/nacho-flow
./nacho-flow --config ./extension/resources/profiles/profile1.yaml
```
