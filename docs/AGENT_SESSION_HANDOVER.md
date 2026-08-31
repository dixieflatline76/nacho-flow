# 🌮 Nacho Flow — Agent Session Handover
**Written:** 2026-08-31 09:42 CEST  
**Branch:** `fix/agentic-shield-and-session-retry`  
**Purpose:** Smart LLM-tailored handover for new agent session to continue benchmarking optimization

---

## 1. Development Environment

### Windows Machine (karlk dev box)
- **Repo:** `C:\Users\karlk\development\Go\src\github.com\dixieflatline76\nacho-flow`
- **Remote Ubuntu share:** `Y:\` → maps to `/home/karlk/projects/nacho-flow/` on NachoUbuntu
- **Cross-compilation:** Always use `$env:CGO_ENABLED="0"` when building Linux binary from Windows (MinGW on PATH causes GCC to fail on Linux POSIX headers)
- **VS Code Extension:** nacho-flow v0.8.4 installed locally

### Ubuntu Remote: NachoUbuntu
- **Proxy URL:** `http://NachoUbuntu:8000`
- **Service:** `/etc/systemd/system/nacho-flow.service`, binary at `/usr/local/bin/nacho-flow`
- **Config:** `/home/karlk/projects/nacho-flow/config.yaml`
- **Logs:** `/home/karlk/projects/nacho-flow/logs/` (traffic.jsonl, router.log, service.log)
- **Stats file:** `~/.config/nacho-flow/stats.json` (default OS config dir)
- **Ollama:** `http://127.0.0.1:11434`, GPU: AMD RX 7900 XTX (ROCm), ~35 tok/sec for 12-24B QAT

### Deploy New Daemon Version

```powershell
# Build (Windows → Linux, no CGO)
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -ldflags "-X main.version=X.Y.Z-tag" -o nacho-flow-linux-amd64 ./cmd/nacho-flow

# Push to Y: drive
Copy-Item .\nacho-flow-linux-amd64 Y:\projects\nacho-flow\nacho-flow-linux-amd64 -Force
Copy-Item .\nacho-flow-linux-amd64 Y:\projects\nacho-flow\nacho-flow -Force
Remove-Item .\nacho-flow-linux-amd64 -Force
```

```bash
# On NachoUbuntu (SSH or terminal):
sudo systemctl stop nacho-flow
sudo cp /home/karlk/projects/nacho-flow/nacho-flow-linux-amd64 /usr/local/bin/nacho-flow
sudo chmod +x /usr/local/bin/nacho-flow
sudo systemctl start nacho-flow
sudo systemctl status nacho-flow --no-pager
```

### Deploy Updated VS Code Extension

```powershell
cd extension
npm run compile
npx @vscode/vsce package --allow-missing-repository --out nacho-flow-X.Y.Z.vsix
code --install-extension .\nacho-flow-X.Y.Z.vsix --force
Remove-Item .\nacho-flow-X.Y.Z.vsix -Force
```
Then VS Code: `Ctrl+Shift+P` → **Developer: Reload Window**

---

## 2. What Nacho Flow Is

Transparent local OpenAI-compatible proxy between agent clients (Cline, Roo Code, Zoo Code) and LLMs.
Agent clients set their OpenAI base URL to `http://NachoUbuntu:8000/v1`.

**Core subsystems:**
1. **Multi-tier routing** — Local GPU → cheap cloud → expensive cloud based on token count + retry depth
2. **Agentic Tool Fallback Shield** — Scans request history for agent error injection phrases, forces escalation
3. **Cycle Killer** — Monitors live streaming tokens for infinite loops / n-gram repetition, severs stream
4. **Heatseeker Deals** — Polls OpenRouter for discounted model pricing
5. **VS Code Dashboard** — Real-time telemetry, cost savings, routing breakdown, Cycle Killer stats

---

## 3. Features Added (v0.8.0 → v0.8.4)

### v0.8.1 — Agentic Tool Shield
- Detects agent question heuristics in streaming response tail buffer
- Error signature detection in request history → escalates tier
- Mode-switch heuristics, `tail_buffer_bytes` configuration

### v0.8.2 — Cycle Killer v3.2
- Dual-lane n-gram repetition detector (prose tokens + thinking tokens)
- Kills stream if n-gram frequency exceeds threshold in sliding window
- Gemma 4 thinking token normalization (`<think>...</think>` stripped before analysis)
- Cooperative budget: only non-code prose counts toward prose token limit

### v0.8.3 — Zero-Alloc Estimator + 5-Tier Escalation + Dynamic Error Signatures
- Zero-allocation body token estimator (byte scanning, no JSON unmarshal)
- **Dynamic `error_signatures` list in config.yaml** — per-agent configurable
- 5-tier default config (Local → DeepSeek → Gemini Flash → Claude Sonnet → Claude Opus)
- `HasToolProgress` guard: skips local tier once file edits have started

### v0.8.4 — Timeframe-Aware Cycle Killer Dashboard (current branch)
- `CycleKiller CycleKillerMetrics` added to `TimeWindowMetrics` struct
- `addToWindow()` and `addBucketToWindow()` accumulate CK stats per time window
- `restoreWindowsFromBuckets()` migrates legacy stats.json (no per-window CK) by copying root CK
- **Fixed JS truthy-object bug:** `windowCycleKiller()` helper checks `total_interventions > 0`
- Fallback to root-level `stats.cycle_killer` applies to ALL four windows (today/week/month/all-time)

---

## 4. Per-Agent Error Signature Configuration

### Default Signatures (hardcoded fallback in `pkg/router/classifier.go`)
```
"[ERROR] You did not use a tool"
"Missing value for required parameter"
"The tool execution failed"
"<error_details>"
"No sufficiently similar match found"
"Command failed with exit code"
"Please retry with complete response"
"Editor operation failed"
"Parameter `old_text` is required"
"Parameter old_text is required"
"Command not executed:"
```

### Override in config.yaml
`agent_shield.error_signatures: [...]` overrides defaults entirely.
Use `config.cline.yaml` for Cline-specific tuning.

### Cline vs Default Config Differences (`config.cline.yaml`)
| Setting | Default | Cline |
|---------|---------|-------|
| `max_prose_tokens` | 4096 | 6144 |
| `max_thinking_tokens` | 1500 | 2000 |
| `repetition_threshold` | 3 | 4 |
| `thinking_repetition_threshold` | 5 | 6 |

Reason: Cline embeds XML tool calls inside prose, producing naturally longer and more repetitive token streams. Tighter limits cause false-positive Cycle Killer trips.

> **Usage:** `nacho-flow -config config.cline.yaml` for Cline benchmarks, `nacho-flow -config config.yaml` for Zoo Code.

---

## 5. Model Tier Research

### Live Production Tiers
| Tier | Model | Condition | Notes |
|------|-------|-----------|-------|
| 1 (Local) | `devstral-small-2:16k` via Ollama | `Tokens < 12000 && Retries < 2` | Free, 24B QAT, 35 tok/sec on RX 7900 XTX |
| 2 | `deepseek/deepseek-chat` (V4 Pro) | `Tokens < 64000 && Retries < 4` | $0.40/$1.20 /1M, 78.4% SWE-bench |
| 3 | `google/gemini-3.7-flash` | `Retries < 6` | ~$0.10/$0.40 /1M, 1M context |
| 4 | `anthropic/claude-sonnet-5` | `Retries < 8` | $3/$15 /1M, deep architecture |
| 5 (default) | `anthropic/claude-opus-5` | `true` | $15/$75 /1M, disaster recovery |

### Key Findings
- **Devstral Small 2:** Best local model for agent workloads; purpose-built for tool-call format
- **DeepSeek V4 Pro:** Best cost/quality ratio for Tier 2; handles unified diff synthesis well
- **Gemini 3.7 Flash:** Essential for large context (30k-80k tokens) without paying Claude prices
- **Local tier pitfall:** If Devstral starts a multi-file edit and fails mid-way, it can't recover; `HasToolProgress` now prevents local re-use after tool calls begin

---

## 6. Benchmarking Status (as of 2026-08-31)

### Live Stats
- **821 total turns** processed since Aug 25
- **$44.79 cloud costs avoided** (83% savings vs direct cloud)
- **$9.28 total cloud spend**
- **12 Cycle Killer interventions** — all Stage 1 (local loop severed, escalated to cloud)
- **0 Stage 2 cloud escalations** (no cloud loops detected)

### Daily Bucket Keys: Aug 25–31 (7 days of data)

### Benchmark Runs
- `benchmarks/runs/run1_nqueens_zoo_win/` — N-Queens, Zoo Code, Windows
- Cline traffic captured: `Y:\projects\nacho-flow\scratch\captured_cline_traffic.jsonl`

### Open Optimization Questions
1. What is Cline's Cycle Killer false positive rate with current thresholds?
2. What unique error phrases does Roo Code inject that aren't in our default signatures?
3. Should local tier token threshold move to 10000 or 14000 based on recent traffic?
4. Are there token-depth sweet spots where DeepSeek V4 consistently outperforms local?

---

## 7. Auto-Tuner System

- **Optimize:** `pkg/tuner/optimizer.go` — `Optimize(records)` → synthesizes optimal `when:` rule
- **Apply:** `pkg/tuner/applier.go` — `ApplyTuning(configPath, result)` — writes backup, atomically updates config
- **API:** `POST http://NachoUbuntu:8000/v1/admin/auto-tune`
- **After benchmarks:** Hit the endpoint to recalculate the Tier 1 token threshold from real traffic

---

## 8. Open Issues / Known Debt

### Immediate
- ⚠️ **Daemon NOT restarted** with v0.8.4 binary yet. Binary is at `Y:\projects\nacho-flow\nacho-flow-linux-amd64`, ready. Daemon still on Aug-25 startup. Dashboard JS fix works now regardless.
- ⚠️ Per-day-bucket `cycle_killer` data starts accumulating correctly only AFTER daemon restart.
- ⚠️ No `config.roo.yaml` exists yet. Roo Code error signatures differ from Cline.

### Branch
- Branch `fix/agentic-shield-and-session-retry` not yet merged to main
- PR open on GitHub

---

## 9. Suggested Next Session Steps

1. SSH to NachoUbuntu, restart daemon with v0.8.4 binary (see Section 1 deploy steps)
2. Run Cline benchmark with `config.cline.yaml` — 20-30 turns on a real coding task
3. Grep traffic.jsonl for error triggers: identify any new error phrases to add
4. Draft `config.roo.yaml` based on Roo error injection patterns
5. Run auto-tuner: `POST /v1/admin/auto-tune`
6. Merge branch to main after validation
