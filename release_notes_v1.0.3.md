### 🌮 Nacho Flow v1.0.3: In-Flight Tool Cycle Breaking & Router Resilience

Nacho Flow is an open-source, high-performance agent supervisor and model dispatcher written in pure Go. It sits between autonomous coding agents (Cline, Zoo Code, Cursor, OpenCode, Aider, Continue) and LLM providers to monitor token streams in real time, terminate runaway loops, resuscitate stalled agents, and dynamically route prompts across local GPU models ($0.00) and cloud reasoning APIs.

---

### 🚀 What's New in v1.0.3

* **In-Flight Tool Argument Cycle Breaking (RFC-002 Core):**
  * **Dedicated Tool Token Streaming Lane**: Added real-time token tracking (`ProcessToolDelta`) in the Cycle Killer shield to monitor streaming tool-call arguments chunk-by-chunk. When an agent enters a runaway command repetition or argument loop, the cycle breaker severs the stream with a clean system override **before** the tool payload reaches the client-side harness for execution.
  * **Zero-Allocation Stream Normalizer**: Extracted streaming delta tool calls on the hot path without intermediate memory allocations or copies.

* **Zero-Allocation Shell Write Detector & Retry Reset Guard:**
  * **`detectShellWrite` Classifier**: Implemented a high-throughput shell command classifier that distinguishes mutating terminal operations (`sed -i`, `>`, `>>`, `| tee`, `patch`, `git restore`) from read-only inspections (`cat`, `ls`, `grep`, `head`, `tail`).
  * **Plan Mode Test-Retry Guard**: Decoupled tool progress signals (`HasWriteProgress` vs. `HasShellWrite`). Under `KickstartWriteOnly`, read-only terminal commands no longer falsely reset test-retry accumulators while tests continue to fail.

* **Router Test Coverage Boost:**
  * Added comprehensive edge-case test suites for classifier token estimators, custom rule AST invalidation, and tier fallback handlers.
  * Increased `pkg/router` statement coverage from **94.1%** to **97.9%**, lifting global repository coverage to **96.7%** (with all 16 Go packages $\ge 95.1\%$).

* **Documentation & Repository Audit:**
  * Retired implemented RFC-002 specification from the core repository.
  * Updated architecture and rule tuning documentation to reflect the dedicated tool argument lane and test progress signals.
  * Configured official repository label taxonomy (`area/*`, `type/*`, `priority/*`) and linked active milestones to the **Nacho Flow Roadmap** GitHub Project.

---

### 🧪 Verification Matrix

| Check / Metric | Scope | Result | Status |
| :--- | :--- | :--- | :---: |
| **Go Test Coverage (`test-cover`)** | All 16 Go packages | **96.7%** statement coverage (all $\ge 95.1\%$) | ✅ Passed |
| **Go Static Analysis (`vet`)** | Full repository | 0 warnings, 0 errors (`go vet ./...`) | ✅ Passed |
| **Go Race Detector (`test-race`)** | Full test suite | 0 data races (`go test -race ./...`) | ✅ Passed |
| **VS Code Extension Suite** | Extension core & webview | **14 / 14 suites, 223 / 223 tests passed (100%)** | ✅ Passed |
| **Security Audit (`sec`)** | Static AST analysis | 65 files, 15,300+ lines: **0 issues** (`gosec`) | ✅ Passed |
| **Tool Repetition Integration** | Runaway streaming command loop | Stream cleanly severed, system override injected | ✅ Validated |
| **Documentation Link Audit** | All Markdown documents & SPA links | **0 broken links, 0 errors** | ✅ Passed |

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
