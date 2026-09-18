### 🌮 Nacho Flow v1.2.2: Config Schema Versioning, Factory Template Diffs & Outdated Profile Alerts

Nacho Flow is an open-source, high-performance agent supervisor and model dispatcher written in pure Go. It sits between autonomous coding agents (Cline, Zoo Code, Cursor, OpenCode, Aider, Continue) and LLM providers to monitor token streams in real time, terminate runaway loops, unstuck frozen agents, and dynamically route prompts across local GPU models ($0.00) and cloud reasoning APIs.

**v1.2.2** introduces end-to-end configuration schema versioning, automated divergence detection between customized user profiles and updated factory presets, and non-intrusive VS Code sidebar notifications.

---

### 🚀 What's New in v1.2.2

#### 🏷️ 1. End-to-End Config Schema Versioning (`pkg/contract`, `extension/src/core/config`)
* **The Problem:** As Nacho Flow evolves, factory profiles introduce new guardrail switches (e.g. Cycle Killer thresholds, Kickstart caps, Fairy Dust prompts). Users running customized profiles in `AppData/Roaming` or `~/.config` had no automated way to know if their active config schema had fallen behind the latest factory defaults.
* **The Solution:**
  * Added an optional `version: string` field to the top-level Go contract `Config` in `pkg/contract/interfaces.go`.
  * Built a zero-dependency SemVer comparison and schema validation engine in `extension/src/core/config/config-version.ts` (`parseSemVer`, `compareSemVer`, `isConfigOutdated`, `diffWithTemplate`).
  * **100% Test Coverage:** Covered by 22 unit tests in `config-version.test.ts` verifying semantic comparison, malformed version handling, and fallback behavior.

---

#### 🔍 2. Factory Template Diff & Reset Commands (`extension`)
* **Live Side-by-Side Diffing (`nacho-flow.compareProfileWithTemplate`):**
  * Opens VS Code's native diff editor (`vscode.diff`) to visually compare the user's active runtime profile against the clean factory template on-the-fly without modifying disk state.
* **Safe Factory Reset (`nacho-flow.resetProfileToDefault`):**
  * Safely restores the active profile back to the pristine factory preset with an explicit confirmation dialog and automatic gateway reload.
* **Non-Intrusive Sidebar Notification Badge:**
  * When an active profile's schema version is older than the extension template, a discreet warning banner and `Compare with Template` button appears in the Nacho Flow sidebar panel.

---

#### 🔬 3. Live Agent Benchmark Validation: Zoo Code & Cline
* **Zoo Code Benchmark (`zoo test 1`):**
  * **92 Turns** completed end-to-end.
  * **$0.89 Total Spent** ($8.48 saved, **90.53% cost reduction**).
  * Built an interactive N-Queens simulator with 95%+ test coverage across all packages and a validated 2.35s $N=1000$ Min-Conflicts heuristic solver.
* **Cline Benchmark (`cline test 1`):**
  * **94 Turns** completed end-to-end.
  * **$1.37 Total Spent** ($6.53 saved, **82.6% cost reduction**).
  * Built a Bubble Tea TUI N-Queens simulator with an $N=1000$ Min-Conflicts solver executing in 1.88s.

---

### 🧪 Verification Matrix

| Check / Metric | Scope | Result | Status |
| :--- | :--- | :--- | :---: |
| **Go Test Coverage (`test-cover`)** | All 18 Go packages | **96.6%** statement coverage (all ≥ 95.1%) | ✅ Passed |
| **Go Static Analysis (`vet`)** | Full repository | 0 warnings, 0 errors (`go vet ./...`) | ✅ Passed |
| **Go Race Detector (`test-race`)** | Full test suite | 0 data races (`go test -race ./...`) | ✅ Passed |
| **VS Code Extension Suite** | Extension core, webviews, versioning | **15 / 15 suites, 299 / 299 tests passed (100%)** | ✅ Passed |
| **Extension TypeScript Compilation** | Extension codebase | 0 errors (`tsc -p ./`) | ✅ Passed |
| **GitHub Actions CI Matrix** | Windows, Ubuntu, macOS runners | All 3 platforms green on PR #59 | ✅ Passed |
| **Real-World Agent Benchmarks** | ZooCode (92 turns) & Cline (94 turns) | **186 turns, 0 silent stalls, 90.5% max cost savings** | ✅ Passed |

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
