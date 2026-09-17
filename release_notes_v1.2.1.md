### 🌮 Nacho Flow v1.2.1: 3-Lane Stream Normalizer, Delimiter Leak Defense, Server Modularization & 45+ Agent Run Battery

Nacho Flow is an open-source, high-performance agent supervisor and model dispatcher written in pure Go. It sits between autonomous coding agents (Cline, Zoo Code, Cursor, OpenCode, Aider, Continue) and LLM providers to monitor token streams in real time, terminate runaway loops, unstuck frozen agents, and dynamically route prompts across local GPU models ($0.00) and cloud reasoning APIs.

**v1.2.1** is the production hardening and architectural stabilization release following the v1.2.0 pre-release, battle-tested across **46 full-length autonomous coding agent benchmark runs**.

---

### 🚀 What's New in v1.2.1

#### 🌊 1. 3-Lane Stream Normalizer & In-Flight Delimiter Leak Defense (`pkg/server`, `pkg/zeroalloc`)
* **The Problem:** Upstream models (Gemma 4, DeepSeek-R1, Qwen 2.5) frequently emit internal control tokens (e.g. `<|channel|>thought`, `<|\"|>}`, `<think>`) split across fragmented SSE chunks. In multi-turn coding sessions, these delimiters could leak into tool argument payloads, corrupting editor file writes or falsely tripping repetition cycle breakers.
* **The Solution:** 
  * Re-architected the SSE streaming pipeline into **3 isolated lanes**: Prose, Reasoning (`<think>`), and Tool Arguments.
  * **Zero-Allocation In-Place Sanitizer (`pkg/zeroalloc`):** Developed mutable byte slice algorithms (`StripSubslicesInPlace`, `ReplaceSubslicesInPlace`, `HasPrefixAny`) that purge trailing control delimiters at line boundaries with **0 B/op heap churn** and sub-100ns execution speed.
  * **Tool Lane Immunity:** Tool call payloads are sanitized and guarded so legitimate repetitive code structures (table-driven unit tests, mock assertions) never trip cycle breakers.
  * **Safe Termination:** Aborted turns emit standard OpenAI-compliant SSE frames without truncated JSON, eliminating client-side V8 parser crashes (`position 515` errors).

---

#### 🔬 2. Battle-Tested Across 46 Real-World Autonomous Agent Runs (`docs/BENCHMARKS.md`)
* **The Battery:** Unlike synthetic HTTP benchmarks, Nacho Flow has been rigorously validated across **46 full-length, multi-turn coding agent runs** (Cline and Zoo Code implementing complex multi-file projects, from Sudoku solvers to $N=1000$ N-Queens engines):
  * **Initial Model Tier Exploration (Sept 2):** Benchmarked DeepSeek Flash, Qwen 3.8, and Qwen3 Coder Plus across prompt routing tiers.
  * **Go N-Queens Classifier & Zero-Alloc Sanity (Sept 5–7):** Validated context classification, AST routing, and streaming stability over 8 consecutive runs.
  * **v1.0.3 Shield & Tool Cycle Tests (Sept 8–11):** Tested write protection runways, cycle severance, and reasoning model immunity.
  * **NTS In-Flight Compactor Lab (Sept 12–14):** Evaluated in-flight compaction across 10 multi-hour trace replays.
  * **v1.2.0 Pre-Release & Delimiter Recovery (Sept 15–17):** 17 production runs culminating in **ZooCode Run 8** (3.8ms $N=1000$ solver) and **Cline Run 9** (2.01s $N=1000$ solver with 95%+ unit test coverage).
* **Key Findings:** Over 100+ hours of continuous agent execution resulted in **zero socket leaks, zero daemon panics, and 100% SSE stream integrity** across 147 live upstream streaming requests.

---

#### 🗜️ 3. NTS Strategic Pivot: Preserving the KV Cache Paradox (`pkg/nts`, `docs/BENCHMARKS.md`)
* **The Finding:** The 46-run battery revealed the **KV Cache Paradox**: while in-flight context compaction (NTS) prunes 30%–60% of raw historical tokens, modern frontier providers (Anthropic, OpenRouter, DeepSeek) offer up to **90% discounts on cached prompt tokens**. Mutating historical context in-flight invalidates byte-for-byte KV cache keys and risks diff-search drift in line-anchored agents.
* **The Decision:**
  * Nacho Flow defaults to **100% pristine context preservation (`nts.enabled: false`)** across all default presets and extension profiles. This guarantees maximum prompt cache hits (85%–96%+ hit rates) and the lowest net invoice cost out of the box.
  * NTS remains fully supported as a high-performance **Experimental Opt-In Labs Engine** for local GPU clusters, on-prem hardware, and models with strict context ceilings.
  * Published Section 7 in `docs/BENCHMARKS.md` documenting the full **Master Autonomous Agent Bake-Off Matrix**.

---

#### 🏗️ 4. Monolithic Server Decomposition (`pkg/server`)
* **The Problem:** The core HTTP server file `pkg/server/proxy.go` expanded to ~1,891 lines, combining routing, SSE streaming, classification, cycle killers, telemetry, and watchdog logic into a single monolithic file.
* **The Solution:** Cleanly decomposed `pkg/server` into 7 modular, domain-specific Go files:
  * **`proxy.go`** (440 lines): Lean HTTP router, atomic state management, and server lifecycle.
  * **`dispatch.go`** (652 lines): Upstream client forwarding, fallback tiers, and 3-lane SSE chunk loops.
  * **`pipeline.go`** (446 lines): Context classification, session guardrails, Fairy Dusting, and Kickstart resuscitations.
  * **`cycle_recovery.go`** (129 lines): Repetition killer, correction prompt injection, and loop interception.
  * **`telemetry.go`** (122 lines): Asynchronous stats recording, pricing calculations, and ring buffer dispatch.
  * **`circuit_breaker.go`** (63 lines): Health tracking, watchdog timers, and atomic memento rollbacks.
  * **`helpers.go`** (103 lines): Client authentication, session key resolution, and path utilities.

---

#### ⚡ 5. Dynamic Runtime Hot-Reload (`pkg/server/api.go`, `pkg/config`)
* Added live hot-reload endpoints (`PUT` / `PATCH` on `/api/v1/config`) allowing running daemons to hot-swap NTS configurations, provider keys, and routing rules without restarting or dropping active connections.

---

#### 🤖 6. Native Cline Agent Profile (`data/agents/cline.json`)
* Added official first-class support for the **Cline** agent harness into `pkg/agentregistry`, including custom file write signatures (`execute_command`, `write_to_file`, `replace_in_file`), error pattern matching, and diff-search protection.

---

### 🧪 Verification Matrix

| Check / Metric | Scope | Result | Status |
| :--- | :--- | :--- | :---: |
| **Go Test Coverage (`test-cover`)** | All 18 Go packages | **96.6%** statement coverage (all ≥ 95.1%) | ✅ Passed |
| **Go Static Analysis (`vet`)** | Full repository | 0 warnings, 0 errors (`go vet ./...`) | ✅ Passed |
| **Go Race Detector (`test-race`)** | Full test suite | 0 data races (`go test -race ./...`) | ✅ Passed |
| **VS Code Extension Suite** | Extension core & webview | **14 / 14 suites, 274 / 274 tests passed (100%)** | ✅ Passed |
| **Security Audit (`sec`)** | Static AST analysis | 72 files, 20,000+ lines: **0 issues** (`gosec`) | ✅ Passed |
| **Benchmark Stress Test** | 350k live requests @ 1k workers | **30,072 req/s peak**, 0 dropped connections | ✅ Passed |
| **Zero-Allocation Hot Paths** | Normalizer, byte filters, cycle breaker | **$0\text{ B/op}, 0\text{ allocs/op}$** verified | ✅ Passed |
| **Live Agent E2E Benchmarks** | ZooCode & Cline (N-Queens $N=1000$) | **46 runs, 147 live requests, 0 dropped streams** | ✅ Passed |

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

#### Standalone Go Daemon (Build from Source)
```bash
go build -o nacho-flow ./cmd/nacho-flow
./nacho-flow --config ./extension/resources/profiles/profile1.yaml
```
