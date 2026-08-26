# 🌮 Nacho Flow: Master Backlog & Action Items

This document tracks all prioritized fixes, high-impact features, and quality enhancements to ensure zero context loss across development sessions.

---

## 🚨 P0: Immediate Core Fixes (Active Development)

### 1. 🛡️ Diff Line-Number Sanitizer in Tool Normalizer
* **Problem**: When LLMs read files with line-number gutters (e.g. `168 | const x = 1;`), they often copy `:168:`, `168 | `, or `168: ` directly into `apply_diff` `<<<<<<< SEARCH` blocks, causing client-side diff match errors in Roo Code / Cline / Cursor.
* **Solution**: Add auto-sanitizing regex transforms inside `pkg/router/tool_normalizer.go` and `pkg/server/stream_normalizer.go` to strip line number prefixes in real-time from `SEARCH` blocks before delivery to the IDE.
* **Files**:
  - `pkg/router/tool_normalizer.go`
  - `pkg/router/diff_sanitizer.go`
  - `pkg/router/diff_sanitizer_test.go`
  - `pkg/server/stream_normalizer.go`
  - `pkg/server/stream_normalizer_test.go`

---

### 2. 🛠️ Backend Stats Management REST API (`/api/v1/stats/*`)
* **Problem**: Need clean, programmatic ways to reset stats to `$0.00` or trigger a full recomputation from `traffic.jsonl` with zero manual file manipulation or daemon restarts.
* **Solution**:
  - `POST /api/v1/stats/reset`: Atomically clears in-memory counters, wipes `stats.json`, resets ring buffer, and emits SSE event.
  - `POST /api/v1/stats/recalculate`: Re-reads `traffic.jsonl`, recalculates dual-rate costs using live `PricingOracle`, atomically updates state, persists to disk, and emits SSE event.
* **Files**:
  - `pkg/telemetry/metrics.go`
  - `pkg/telemetry/metrics_mgmt_test.go`
  - `pkg/server/api.go`
  - `pkg/server/api_stats_mgmt_test.go`
  - `extension/src/core/api/client.ts`

---

## ✅ P0: Completed Items

### 3. ⚡ Production-Grade 1-Click Heatseeker Deal Adoption & Quick-Actions (`v0.6.3`)
* **Status**: ✅ **COMPLETED & VERIFIED**
* **Delivered**:
  - `📋 Copy` and `⚡ Adopt` quick buttons on all deal cards in `dashboard.js`.
  - Dynamic active tier enumerator in `vscode.window.showQuickPick` with `⭐ Recommended` flags.
  - Comment-preserving regex block replacer for `- name: ...` and `default_tier:`.
  - 109/109 unit tests passing with **99.41% line coverage**.

### 4. 🎨 Auto-Tuner Rich Policy Recommendation Card (`v0.6.4`)
* **Status**: ✅ **COMPLETED & VERIFIED**
* **Delivered**:
  - Displays target tier, sample size (`72 real turns`), synthesized AST rule (`Tokens < 64000 && Retries == 0`), and glowing projected savings badge (`+$4.54`).

### 5. 💰 Dual-Rate Token Financial Accounting & OpenRouter Spot Sync
* **Status**: ✅ **COMPLETED & VERIFIED**
* **Delivered**:
  - Exact prompt vs completion token pricing calculation in `PricingOracle`.
  - Injected dedicated OpenRouter key with zero git secret leaks via systemd.
  - Fixed `/v1` upstream URL prefix deduplication.

---

## 📈 P1: Polish, Build Hygiene & Architecture

### 6. 🧹 Build Tooling & Extension Manifest Hygiene
* **Problem**: `esbuild` throws `Invalid option in build() call: "watch"` in `esbuild.config.js`, and `vsce package` warns about missing `repository` and `LICENSE`.
* **Solution**:
  - Update `esbuild.config.js` to modern context API for watching.
  - Add repository field to `extension/package.json` and copy `LICENSE` to `extension/`.
* **Files**:
  - `extension/esbuild.config.js`
  - `extension/package.json`

### 7. 🧠 Reasoning Token Normalization & Tag Cleanliness
* **Scope**: Ensure reasoning tokens from Gemini 3.7 Flash (`reasoning_tokens`) and DeepSeek R1 (`<think>`) stream cleanly without leaking internal tokens into git diffs or IDE file content.
* **Files**:
  - `pkg/server/stream_normalizer.go`

### 8. 🎛️ Dynamic Local Context Ceiling Calibration
* **Scope**: Automatically tune Tier 1's `Tokens < 16000` condition based on active ROCm VRAM headroom so large local models never trigger out-of-memory errors on high-concurrency tasks.
* **Files**:
  - `pkg/tuner/advisor.go`
  - `pkg/tuner/analyzers.go`

---

## 📊 P2: Empirical Benchmarks & Documentation

### 9. 🏆 Empirical Multi-Tier A/B Case Study
* **Status**: ✅ **COMPLETED & COMMITTED**
* **Deliverable**: [`docs/BENCHMARKS_AB_CASE_STUDY.md`](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/docs/BENCHMARKS_AB_CASE_STUDY.md) documenting Run A (Qwen 3 Coder) vs Run B (Gemini 3.7 Flash Thinking) with turn-by-turn telemetry and AST analysis.
