### 🌮 Nacho Flow v1.4.0: Automated Profile Auto-Tuner, Pre-Tuned Presets, 3-Lane Stream Normalizer & Config Schema Versioning

**v1.4.0** is the first major General Availability release since **v1.1.0**. It brings together the architectural work from the v1.2 and v1.3 development cycles into a single stable release: an automated profile tuner that replays your session logs to optimize routing rules, pre-tuned presets for Zoo Code and Cline, 3-lane SSE stream normalization that stops control tokens from corrupting file edits, and config schema versioning.

---

### 🚀 What's New in v1.4.0

#### 🎛️ 1. Traffic-Based Profile Auto-Tuner (`nacho-flow tune`, `pkg/tuner`)
* **The Problem:** Writing routing rules by hand is pure guesswork. You pick an arbitrary context limit like `Tokens < 20000`, run an agent, and quickly hit real-world failure modes:
  * Your local model exceeds GPU VRAM and crashes Ollama.
  * An agent gets stuck in a loop, and your cascade blindly escalates turn after turn to frontier models, burning $10 on an unresolvable prompt.
  * Your cascade escalates to a model that is actually *worse* at coding than the tier before it (capability inversion).
  * On top of that, previous tuning tools were an all-or-nothing black box—you either accepted every proposed change or none at all.
* **The Solution:**
  * **Session Replay Simulation:** `nacho-flow tune` reads your real session history (`traffic.jsonl`) and replays it through an in-memory simulation engine (replaying 1,000,000 statements in 25ms with zero heap allocations) to calculate exactly what candidate rules would have cost and how many retries they would have prevented before touching your configuration.
  * **Fine-Grained Checkbox Selection:** Added checkboxes to the VS Code Auto-Tuner webview panel so you can inspect individual rule suggestions, threshold shifts, and model swaps, and apply only the ones you want.
  * **GPU VRAM Awareness:** Set your GPU VRAM ceiling (e.g. `--vram-gb=16` or via the dashboard dropdown) so the tuner never suggests local context limits that exceed your card's physical memory capacity.
  * **Per-Client Tuning (`client_id`):** Automatically detects whether traffic originated from Zoo Code, Cline, Cursor, or Aider via `/api/v1/telemetry/clients`. You can filter logs by agent in the dashboard and tune rules specifically for Zoo Code without Cline's sessions polluting the thresholds.
  * **Capability Inversion Pruning:** Automatically sets redundant tiers to `when: "false"` if an escalation tier uses a model with lower coding benchmarks than the tier before it, and halts escalation when an agent hits an unresolvable plateau.

---

#### 🎯 2. Pre-Tuned Presets for Zoo Code & Cline (`profile1.yaml`, `profile2.yaml`, `profile3.yaml`)
* **The Problem:** Different autonomous coding agents fail in very different ways. Zoo Code uses deep JSON tool calls and multi-turn test-and-debug loops, while Cline relies on XML tool fences and step-by-step diff edits. On generic out-of-the-box configurations:
  * Agents hit repetition cycle killers mid-stream because tabular output or test matrices were mistaken for infinite reasoning loops.
  * Read-only exploration turns were misidentified as stalled agents, triggering unwanted kickstart overrides.
* **The Solution:** Bundled three profile presets tuned directly from real multi-turn agent runs (including the full Blackjack project benchmark where Zoo Code + GLM-5.3-Flash delivered a complete Go TUI project for $0.34 total):
  * **Profile 1 & Profile 2 (Zoo Code / Standard Hybrid):** Uses `z-ai/glm-5.3-flash` for Tier 2 ($0.65/M), Gemini 3.8 Flash for Tier 3, a 16,384 thinking-token ceiling with adjusted repetition detection (8-word n-gram, 12 repeats max so tables don't get cut off), and `SessionKickstarted && Retries < 3` to power past initial read-only exploration loops.
  * **Profile 3 (Cline):** Uses `qwen/qwen3-coder-plus` for Tier 2 ($0.65/M), Gemini 3.8 Flash for Tier 3, a 4,096 thinking-token limit (6 repeats), and a tight kickstart threshold (`Retries < 1 && SessionKickstarted`) tailored for Cline's XML tool calling syntax.
  * **1-Click Profile Switching:** The VS Code extension lets you switch profiles instantly from the status bar chip (`🌮`), spawning the daemon with clean `--config` process isolation.

---

#### 🌊 3. 3-Lane SSE Stream Normalizer & Delimiter Leak Defense (`pkg/server`, `pkg/zeroalloc`)
* **The Problem:** Open-weights models (Gemma 4, DeepSeek-R1, Qwen 2.5) frequently emit internal control tokens (e.g. `<|channel|>thought`, `<think>`, `<|"|>}`) split across fragmented SSE streaming chunks. In long agent sessions, these delimiter fragments leaked into tool argument payloads, corrupting editor file writes or causing V8 JSON parser crashes (`position 515` errors) that froze the client extension.
* **The Solution:**
  * Re-architected the SSE proxy into 3 isolated streaming lanes: Prose, Reasoning (`<think>`), and Tool Arguments.
  * Built zero-allocation in-place byte sanitizers (`StripSubslicesInPlace`, `ReplaceSubslicesInPlace`) that strip trailing control delimiters at line boundaries with zero heap allocations ($0\text{ B/op}$) and sub-100ns latency.
  * Handled stream terminations cleanly so aborted turns emit valid OpenAI-compliant SSE frames without truncated JSON, eliminating client-side parser crashes.

---

#### 📋 4. Config Schema Versioning & Default Template Diffs (`pkg/config`, Extension)
* **The Problem:** As routing features and normalizer flags evolve across releases, user configuration files silently drift. Developers had no easy way to know if their local `config.yaml` was missing new fields or using deprecated options, and restoring defaults meant manually copying YAML from the repository.
* **The Solution:**
  * Added SemVer schema validation between active user profiles and bundled templates.
  * Visual sidebar alerts notify developers when their active profile schema is older than the current extension version.
  * Added live side-by-side diffing (`nacho-flow.compareProfileWithTemplate`) to see exactly what changed, and a safe reset command (`nacho-flow.resetProfileToDefault`) that makes a timestamped `.bak` backup before restoring defaults.

---

### 🧪 Verification Matrix

| Check / Metric | Scope | Result | Status |
| :--- | :--- | :--- | :---: |
| **Go Test Coverage (`cover-sync`)** | All 19 Go packages & CLI tools | **95.8%** statement coverage (all ≥ 95.0%) | ✅ Passed |
| **Go Static Analysis (`vet`)** | Full repository | 0 warnings, 0 errors (`go vet ./...`) | ✅ Passed |
| **Go Race Detector (`test-race`)** | Full test suite | 0 data races (`go test -race ./...`) | ✅ Passed |
| **Security Audit (`gosec`)** | Full codebase (98 files, 25k lines) | 0 issues (`gosec -exclude=G706`) | ✅ Passed |
| **VS Code Extension Suite** | Extension core, webviews, schemas | **15 / 15 suites, 333 / 333 tests passed (100%)** | ✅ Passed |
| **Benchmark Concurrency Stress** | 50 -> 100 -> 250 -> 500 -> 1000 workers | 100% success rate (0 errors across 350k reqs) | ✅ Passed |
| **GitHub Actions CI Matrix** | Windows, Ubuntu, macOS runners | All 3 platforms green on `main` | ✅ Passed |

---

### 📦 Installation & Quickstart

#### VS Code Companion Extension (Recommended)
Install directly from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=dixieflatline76.nacho-flow) or via CLI:
```bash
code --install-extension dixieflatline76.nacho-flow
```
* Select your desired profile (**Profile 1: Standard Hybrid**, **Profile 2: Zoo Code Preset**, or **Profile 3: Cline Preset**) directly from the status bar chip (`🌮`).
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
