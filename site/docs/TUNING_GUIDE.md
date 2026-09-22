# 🎯 Nacho Flow: Rule & Tier Tuning Guide

This guide teaches you how to write, optimize, test, and tune dynamic routing rules in **Nacho Flow** using `expr-lang/expr` expressions.

## Table of Contents
1. [How the Evaluation Pipeline Works](#1-how-the-evaluation-pipeline-works)
2. [Available Context Variables & Tier Properties](#2-available-context-variables--tier-properties)
3. [Recommended Tier Ordering Strategy](#3-recommended-tier-ordering-strategy)
   - [3.1 🎯 GPU Hardware & Token Sizing Cheat Sheet](#-gpu-hardware--token-sizing-cheat-sheet)
   - [3.2 🏆 The 16GB VRAM Champion: Gemma 4 12B IT QAT & Perfected Ollama Tuning](#-the-16gb-vram-champion-gemma-4-12b-it-qat--perfected-ollama-tuning)
4. [Real-World Rule Recipes](#4-real-world-rule-recipes)
5. [Testing & Validating Your Rules](#5-testing--validating-your-rules)
6. [Autonomous Multi-Tier Auto-Tuning (`nacho-flow tune`)](#6-autonomous-multi-tier-auto-tuning-nacho-flow-tune)
   - [6.1 6D Constraint Satisfaction Formulation & Min-Conflicts Heuristic](#61-6d-constraint-satisfaction-formulation--min-conflicts-heuristic)
   - [6.2 Fractional Conflict Attribution & Fleet Dominance](#62-fractional-conflict-attribution--fleet-dominance)
   - [6.3 Step-by-Step Tuning Workflow & CLI Options](#63-step-by-step-tuning-workflow--cli-options)
   - [6.4 High-Throughput 1M-Statement Stress Testing (`nacho_stress`)](#64-high-throughput-1m-statement-stress-testing-nacho_stress)
7. [Manual Heuristic Tuning Tips](#7-manual-heuristic-tuning-tips-for-custom-power-rules)
8. [Agent-Specific Harness Tuning: Zoo Code vs. Cline](#8-agent-specific-harness-tuning-zoo-code-vs-cline)
9. [🧚 Fairy Dusting & Cost Shield Architecture](#9-fairy-dusting--cost-shield-architecture)
10. [Troubleshooting & FAQ](#10-troubleshooting--faq)

---

## 1. How the Evaluation Pipeline Works

When an agent (Zoo Code, Cline, Aider, Cursor) sends a prompt turn, Nacho Flow evaluates your configured tiers sequentially from **top to bottom** (*First Match Wins*):

```mermaid
flowchart TD
    Req["Incoming Prompt Turn"] --> Context["Classifier Extracts: Tokens, HasImages, HasTools, Keywords"]
    Context --> Tier1{"Tier 1 Expression Match?"}
    Tier1 -->|True| Route1["Route to Tier 1 Target"]
    Tier1 -->|False| Tier2{"Tier 2 Expression Match?"}
    Tier2 -->|True| Route2["Route to Tier 2 Target"]
    Tier2 -->|False| TierN{"Tier N Expression Match?"}
    TierN -->|True| RouteN["Route to Tier N Target"]
    TierN -->|False| Default["Route to Default Tier"]
```

---

## 2. Available Context Variables & Tier Properties

### Variables in `when` Expressions:
| Variable | Type | Description | Example Condition |
| :--- | :--- | :--- | :--- |
| `Tokens` | `int` | Real-time adaptive estimated token count across all prompt messages | `Tokens < 16000` |
| `HasImages` | `bool` | `true` if any message contains screenshots or image URLs | `HasImages == true` |
| `HasTools` | `bool` | `true` if function/tool definitions or active tool calls are present | `HasTools == true` |
| `HasWriteCapability` | `bool` | `true` if declared client tools contain at least one tool from `kickstart_write_tools`. Automatically `false` in Plan Mode / read-only exploration | `HasWriteCapability && Tokens < 16000` |
| `Keywords` | `[]string` | Code keywords detected **strictly in the latest user prompt** | `any(Keywords, { # in ['deadlock', 'mutex'] })` |
| `Retries` | `int` | Consecutive error count from in-history failures (`[ERROR]`, `<error_details>`) and identical prompt retries. Automatically resets to `0` when intermediate tools succeed (`role: tool`). | `Retries < 2` |
| `IsRetry` | `bool` | `true` if this turn is an error recovery turn | `!IsRetry` |
| `Model` | `string` | The requested model ID sent by the client (e.g. `nacho-hybrid`) | `Model == 'nacho-coder'` |
| `SessionKickstarted` | `bool` | `true` if the session exceeded `kickstart_threshold` without tool/write progress | `SessionKickstarted && Retries < 3` |
| `SessionKickstartCount` | `int` | Number of times kickstart resuscitation has fired this session | `SessionKickstartCount > 2` |
| `HasToolProgress` | `bool` | `true` if the previous turn contained successful tool execution | `HasToolProgress` |
| `HasWriteProgress` | `bool` | `true` if the previous turn contained productive file-write tool execution (`write_to_file`, `replace_in_file`) | `HasWriteProgress` |
| `HasShellWrite` | `bool` | `true` if terminal execution modified files (`sed -i`, `>`, `>>`, `| tee`, `patch`, `git restore`) | `HasShellWrite` |
| `HasTestProgress` | `bool` | `true` if the previous turn executed a test suite or build command | `HasTestProgress` |
| `HasTestPass` | `bool` | `true` if tests or compiler output completed with zero errors / passed cleanly | `HasTestPass` |
| `HasTestFail` | `bool` | `true` if tests failed, panics occurred, or compiler build errors were detected | `HasTestFail` |
| `HistoryErrors` | `int` | Number of consecutive trailing errors in conversation history | `HistoryErrors >= 2` |
| `CoolingDownModels` | `[]string` | Models currently under cooldown after Cycle Killer severed streams | `!('gemma4:12b-it-qat' in CoolingDownModels)` |

> [!TIP]
> **Automatic Escalation Safety Cap**: When a turn escalates to the `default_tier` (e.g. Claude Sonnet 5), Nacho Flow automatically limits consecutive frontier execution to `MaxEscalationTurns = 3`. If a problem cannot be fixed after 3 consecutive frontier turns, the gateway automatically de-escalates to Tier 2 (e.g. Gemini 3.7 Flash) to prevent runaway billing.

### Tier Properties:
- `max_context` (`int`): Optional. Upper bound of the model's context window (e.g. `16384`, `32768`, `65536`). If `Tokens > max_context`, Nacho Flow immediately skips this tier with zero expression overhead.
- `strip_images` (`bool`): If `true`, strips raw base64 image strings from older conversation turns to prevent 400 errors on text-only models.
- `reasoning_effort` (`string`): Passes `"low"`, `"medium"`, or `"high"` to supported reasoning providers (e.g. OpenAI o3-mini).
- `raw` (`bool`): If `true`, disables all tool normalizers, thinking-tag converters, and fallback shields for a 100% transparent, unadulterated SSE stream.
- `shield` (`bool`): If `false`, disables the Agentic Tool Fallback Shield (prevents converting trailing questions/plans into synthetic `ask_followup_question` tool calls), essential for automated headless scripts and batch CI runners.
- `normalizer` (`bool`): Master toggle for the Universal Tool Normalizer on this tier.
- `normalizers` (`object`): Granular sub-normalizer toggles:
  - `markdown` (`bool`): Normalizes ```json ... ``` markdown code blocks into OpenAI tool calls.
  - `bare_json` (`bool`): Normalizes raw top-level JSON objects into OpenAI tool calls.
  - `react` (`bool`): Normalizes ReAct `Action: / Action Input:` patterns into OpenAI tool calls.
- `cycle_breaker` (`object`): In-Flight Stream Guard and Monologue Breaker settings:
  - `enabled` (`bool`): Toggles real-time repetition and prose monologue detection.
  - `max_content_tokens` (`int`): Soft ceiling for conversational/text content tokens before triggering (default: `4096`).
  - `repetition_window` (`int`): Word window for N-gram sliding hash detector (default: `6`).
  - `repetition_threshold` (`int`): Repetition match threshold for instant stream abort (default: `3`).
  - `max_retries` (`int`): Number of Stage 1 local `$0.00` self-correction retries before cloud failover (default: `1`).
  - `correction_prompt` (`string`): Custom authoritative `[SYSTEM OVERRIDE]` injection prompt.

---

## 3. Recommended Tier Ordering Strategy

To maximize cost savings without degrading agent intelligence, follow the **Hierarchy of Complexity**:

```text
[1. Complex Keywords / Reasoning]  --> Route to DeepSeek-R1 / o1 (Specialized Brain)
[2. Multimodal Vision (Images)]    --> Route to Gemini Flash / Claude Sonnet 5 (Vision Encoders)
[3. Active Tool Calls]             --> Route to Cloud Fast Coder (High Tool Adherence)
[4. Routine Local Coding (< 16k)]  --> Route to Local GPU (Ollama/vLLM) ($0.00 / 100% Free)
[5. Retry Escalation (Retries>=2)] --> Route to Cloud Provider (Breaks Local Failure Loops)
[6. Large Context Overflow (>= 16k)]-> Route to Cheap Cloud Fast (e.g. Gemini 3.7 Flash)
[Default Fallback]                 --> Reliable Cloud Fallback (Capped at 3 turns max)
```

### 🎯 GPU Hardware & Token Sizing Cheat Sheet

Not sure what token limits to set for your local workstation? Use this reference table based on your available VRAM:

| Workstation Hardware | VRAM | Recommended Local Model | Suggested `Tokens` Bound |
| :--- | :--- | :--- | :--- |
| **8 GB VRAM** (RTX 3060/4060, Apple M1/M2 8GB) | 8 GB | `qwen2.5-coder:7b` | `Tokens < 8000` |
| **16 GB VRAM** (Radeon RX 6900/9070 XT, RTX 4080) | 16 GB | `gemma4:12b-it-qat` (🏆 **Recommended**) / `qwen-3.8:27b` (`IQ3_S`) | `Tokens < 20000` |
| **24 GB VRAM** (RTX 3090/4090, Apple M-Max 32GB) | 24 GB | `qwen-3.8:27b` (`Q4_K_M`) / `ornith-1.5:35b` | `Tokens < 32000` |
| **32 GB+ VRAM / Mac Studio** | 32 GB+ | `qwen-3.8:27b` (`Q8`) / `deepseek-r1:32b` | `Tokens < 48000` |

---

### 🏆 The 16GB VRAM Champion: Gemma 4 12B IT QAT & Perfected Ollama Tuning

For developers with **16 GB VRAM GPUs** (RTX 4080, RX 6900 XT / 7800 XT / 7900 GRE / 9070 XT, Apple Silicon 16GB/24GB Unified Memory), **Google Gemma 4 12B Instruction-Tuned QAT** (`gemma4:12b-it-qat`) provides the single best combination of speed, reasoning depth, and AST compliance available today.

#### Why Gemma 4 12B QAT Dominates the 16GB Class:
- **100% VRAM GPU Offload (Zero CPU Spillover)**: At ~7.5 GB to 8.5 GB VRAM footprint (Q4_0 Quantization-Aware Training), the model plus dynamic KV cache fits entirely within 16GB with ~7GB of safety headroom for your OS, IDEs, and browser.
- **Quantization-Aware Training (QAT)**: Unlike standard post-training quantization (PTQ) which often degrades AST syntax precision and tool syntax adherence, official QAT models are trained specifically for 4-bit weights, preserving exceptional instruction-following and code quality.
- **Blazing Token Generation**: Emits 60–90+ tokens/sec on 16GB consumer desktop GPUs for instant, zero-cost iterative coding turns.

---

#### ⚠️ Critical Pitfall: The "Runaway Monologue" Bug (DO NOT Hardcode `num_ctx` or `num_predict`)

During real-world agentic stress testing with Cline and Zoo Code, we identified a critical failure pattern with local models:

> [!CAUTION]
> **NEVER hardcode `PARAMETER num_predict 8192` or `PARAMETER num_ctx 32768` inside your Ollama Modelfile!**
> - **Why `num_predict` fails**: If baked into the model, Gemma 4 enters runaway reasoning loops inside `<think>...</think>` monologue traps, generating reasoning text for 45–60+ seconds without emitting active tool calls.
> - **Why hardcoded `num_ctx` fails**: Static 32k context allocation locks up VRAM buffers prematurely and interferes with dynamic windowing.
> - **The Nacho Flow Architecture**: Leave `num_predict` and `num_ctx` **OUT** of the Modelfile. Nacho Flow dynamically provisions context per request turn via `max_context: 32000` in `config.yaml`, while **Cycle Killer** enforces strict stream termination if repetition exceeds limits.

---

#### 🚀 Crucial Workstation Setup: Configure `OLLAMA_CONTEXT_LENGTH=32768` (Prevents Silent Context Truncation)

By default on Windows and certain headless installations, Ollama runs with a restricted context ceiling (`OLLAMA_CONTEXT_LENGTH: 16384` or `4096`).
When an autonomous agent (like Cline or Zoo) reaches long conversation turns (e.g. 18k tokens):
1. Nacho Flow correctly routes the request to Tier 1 (`Tokens < 20000`).
2. If Ollama's server context ceiling is only 16,384, **Ollama silently truncates the top of the prompt**.
3. In Cline and Zoo, the prompt prefix contains the **system prompt, tool schemas, and XML formatting instructions**.
4. When truncated, the model loses its tool definitions, enters a prolonged thinking loop inside `<think>...</think>` (burning 2,000+ reasoning tokens), and terminates with **0 content and 0 tool calls**, causing the agent to stall.

> [!IMPORTANT]
> **Do NOT hardcode `num_ctx` in the Modelfile.** Instead, set `OLLAMA_CONTEXT_LENGTH=32768` at the **Ollama server environment level**. This allows Ollama to allocate a full 32k context buffer dynamically without bloating the model weights or locking VRAM buffers prematurely.

##### 🪟 Windows Setup (PowerShell):
Set permanently for your user account:
```powershell
[System.Environment]::SetEnvironmentVariable('OLLAMA_CONTEXT_LENGTH', '32768', 'User')
```
*(Or in Command Prompt: `setx OLLAMA_CONTEXT_LENGTH 32768`)*

Then restart the Ollama tray application (Right-click tray icon → Quit, and relaunch), or run:
```powershell
Stop-Process -Name "ollama*", "ollama app" -Force -ErrorAction SilentlyContinue
Start-Process "C:\Users\<username>\AppData\Local\Programs\Ollama\ollama app.exe"
```

##### 🐧 Linux Setup (Ubuntu / systemd):
Configure the systemd service override:
```bash
sudo mkdir -p /etc/systemd/system/ollama.service.d
echo -e '[Service]\nEnvironment="OLLAMA_CONTEXT_LENGTH=32768"' | sudo tee /etc/systemd/system/ollama.service.d/override.conf
sudo systemctl daemon-reload
sudo systemctl restart ollama
```

##### 🍎 macOS Setup:
```bash
launchctl setenv OLLAMA_CONTEXT_LENGTH 32768
```
*(Or add `export OLLAMA_CONTEXT_LENGTH=32768` to your shell profile if launching via terminal)*.

##### 🔍 How to Verify:
Inspect Ollama's startup log (on Windows: `%LOCALAPPDATA%\Ollama\server.log`, on Linux: `journalctl -u ollama`):
You should see:
```text
level=INFO source=routes.go:1940 msg="server config" env="... OLLAMA_CONTEXT_LENGTH:32768 ..."
```

---

#### 🛠️ Perfected Ollama Modelfile & Sampling Parameters

To get flawless tool adherence, zero monologue traps, and clean diff output, create a custom Modelfile with our perfected sampling parameters:

```dockerfile
# Save as: Modelfile
FROM gemma4:12b-it-qat

# 🌮 Perfected Nacho Flow Sampling Parameters for Gemma 4
PARAMETER temperature 0.6
PARAMETER top_k 64
PARAMETER top_p 0.9
PARAMETER min_p 0.05
PARAMETER repeat_last_n 64
PARAMETER repeat_penalty 1.15
```

Build or update it in Ollama with one command:
```bash
ollama create gemma4:12b-it-qat -f Modelfile
```
*(Or simply pull the vanilla model via `ollama pull gemma4:12b-it-qat` — just ensure no previous custom Modelfile baked in `num_predict 8192` or static `num_ctx`)*.

---

#### 🌮 Gateway Routing Configuration (`config.yaml` / Extension Profiles)

In your Nacho Flow configuration (pre-configured in factory **Profile 1**, **Profile 2**, and **Profile 3**):

```yaml
tiers:
  # TIER 1: Local GPU Workhorse (Gemma 4 12B QAT) - 100% VRAM Offload
  - name: "Tier 1: Local GPU Workhorse"
    provider: "ollama"
    model: "gemma4:12b-it-qat"
    when: "Tokens < 20000 && Retries < 2"
    strip_images: false
    max_context: 32000

cycle_killer:
  enabled: true
  tool_lane:
    max_repeats: 6      # Fast-kill repetition loop threshold
```

---

## 4. Real-World Rule Recipes

### 🔥 Top 5 Copy-Paste Rule Patterns

| Pattern | `when` Condition | Purpose |
| :--- | :--- | :--- |
| **1. Standard Local Workhorse** | `Tokens < 16000 && !HasImages && Retries < 2` | Routes routine iterative turns to GPU for $0.00, breaking on failures. |
| **2. Concurrency / Math Escapement** | `any(Keywords, { # in ['deadlock', 'mutex', 'concurrency', 'race'] })` | Instantly flags deep reasoning keywords to DeepSeek-R1 / o1. |
| **3. Vision Escapement** | `HasImages` | Routes screenshot turns to vision models (Gemini Flash / Claude Sonnet). |
| **4. Tool-Safety Boundary** | `!HasTools && Tokens < 12000` | Restricts local models to pure code generation without calling external tools. |
| **5. Cloud Recovery Overflow** | `Tokens >= 16000 || HasTools || Retries >= 2` | Catch-all for large context overflow, tool execution, or retry recovery. |

---

### Recipe 1: Local-First GPU Routing with Auto-Escalation
Routes small, single-file edits and routine coding tasks to your local GPU, escalating to cloud when history accumulates, images are attached, or if the local model fails 2 consecutive times:

```yaml
tiers:
  - name: "Cloud Vision"
    model: "google/gemini-2.5-flash-lite"
    provider: "openrouter"
    when: "HasImages"

  - name: "Local ROCm / CUDA GPU"
    model: "qwen2.5-coder:14b"
    provider: "local_gpu"
    max_context: 16384
    when: "Tokens < 16000 && !HasImages && !HasTools && Retries < 2"
    strip_images: true

  - name: "Cloud Agentic Fast"
    model: "qwen/qwen3-coder-30b-a3b-instruct"
    provider: "openrouter"
    when: "Tokens >= 16000 || HasTools || Retries >= 2"

default_tier:
  name: "Cloud Fallback"
  model: "deepseek/deepseek-v4-flash-latest"
  provider: "openrouter"
  when: "true"
```

---

### Recipe 2: Domain-Specific Routing (SQL, Concurrency & Security)
Automatically detects deep architectural concepts and delegates them to specialized reasoning models (evaluated strictly on the latest user prompt):

```yaml
tiers:
  # Concurrency & Race Conditions -> DeepSeek-R1
  - name: "Deep Reasoning"
    model: "deepseek/deepseek-r1"
    provider: "openrouter"
    when: "any(Keywords, { # in ['deadlock', 'mutex', 'race', 'concurrency', 'atomic', 'goroutine'] })"

  # Database & Migrations -> Specialized Model
  - name: "SQL Specialist"
    model: "deepseek/deepseek-chat"
    provider: "deepseek"
    when: "any(Keywords, { # in ['sql', 'postgres', 'migration', 'index', 'query', 'schema'] })"

  # Everything else < 12k context without prior retries -> Local GPU
  - name: "Local Fast"
    model: "qwen2.5-coder:14b"
    provider: "local_gpu"
    max_context: 16384
    when: "Tokens < 12000 && !HasImages && Retries < 2"
```

---

### Recipe 3: Reasoning Effort Injection
For models supporting explicit reasoning controls (e.g. OpenAI o3-mini or Gemini thinking):

```yaml
tiers:
  - name: "Heavy Reasoning"
    model: "openai/o3-mini"
    provider: "openrouter"
    reasoning_effort: "high"
    when: "any(Keywords, { # in ['architecture', 'proof', 'refactor-entire-repo'] })"

  - name: "Fast Reasoning"
    model: "openai/o3-mini"
    provider: "openrouter"
    reasoning_effort: "low"
    when: "Tokens > 20000"
```

---

### Recipe 4: Headless Automated CI & Evaluation (No Interactive Tool Prompts)
When running automated CI scripts, batch test generators, or headless benchmark runners, you want plain text model output rather than interactive tool wrapping:

```yaml
tiers:
  - name: "Headless Automated CI Evaluator"
    model: "deepseek/deepseek-chat"
    provider: "openrouter"
    when: "any(Keywords, { # in ['ci_batch', 'test_eval', 'headless'] })"
    # Disables synthetic ask_followup_question tool calls so batch runners never hang
    shield: false
    # Retains standard tool normalization for valid function calls
    normalizer: true
```

---

### Recipe 5: Transparent Raw Stream & Tokenizer Benchmarking
When benchmarking exact upstream token throughput, debugging SSE streaming libraries, or developing custom client wrappers without proxy rewrites:

```yaml
tiers:
  - name: "Raw Stream Debugging & Benchmarking"
    model: "anthropic/claude-sonnet-5"
    provider: "openrouter"
    when: "any(Keywords, { # in ['raw_stream', 'inspect_bytes', 'benchmark'] })"
    # Completely bypasses all normalizers, reasoning stream wrappers, and fallback shields
    raw: true
```

---

### Recipe 6: Tailored Tool Parsing for Local Open-Weight Models
When running specialized local models (such as Qwen 2.5 Coder or Mistral NeMo) that output structured tool calls in Markdown code blocks or bare JSON, but produce code diffs containing words like `Action: ` that could trip ReAct regexes:

```yaml
tiers:
  - name: "Local Qwen Coder (Selective Parsers)"
    model: "qwen2.5-coder:14b"
    provider: "local_gpu"
    max_context: 16384
    when: "Tokens < 12000 && !HasImages && Retries < 2"
    shield: true
    normalizer: true
    # Selectively configure sub-normalizer strategies
    normalizers:
      markdown: true    # Normalizes ```json ... ``` tool blocks
      bare_json: true   # Normalizes raw JSON objects
      react: false      # Disables ReAct regex scanner to eliminate code diff false positives

---

### Recipe 7: 🎸 Cycle Killer & ⚡ Kickstart Defense
When using local models (e.g. Gemma 4 12B QAT, DeepSeek-R1 14B) or cloud tiers that occasionally suffer from degenerative circular reasoning, 4-minute monologue loops, or multi-turn read/plan stalls:

```yaml
tiers:
  - name: "Local GPU with Cycle Killer & Kickstart"
    model: "gemma4:12b-it-qat"
    provider: "ollama"
    when: "Tokens < 16000 && Retries == 0"
    cycle_killer:
      enabled: true
      phrase_length: 6          # Default sliding N-gram window (words)
      budget_max_repeats: 5     # Cooperative budget repeat limit
      thinking_lane:
        max_tokens: 4096        # Max reasoning token budget
        max_repeats: 6          # Fast-kill reasoning repetition loop
      content_lane:
        max_tokens: 6144        # Max non-tool content tokens
        max_repeats: 8          # Fast-kill content repetition loop
      tool_lane:
        max_tokens: 8192        # Max command/tool invocation arguments
        max_write_tokens: 32768 # Category A file writes ceiling (exempt from N-gram check)
        phrase_length: 4        # Tighter 4-word window for shell command loops
        max_repeats: 8          # Fast-kill tool repetition loop
      max_retries: 1            # Stage 1: retries locally with [SYSTEM OVERRIDE] @ $0.00
```
*(Also supports `cycle_breaker:` as a backwards-compatible alias, and works across both local and cloud tiers).*

---

## 4.5 Agent-Specific Presets: Cline vs Zoo Code

Different AI coding agents have fundamentally different tool-calling architectures that affect how well they work with local models. Nacho Flow ships two preset configurations:

| Config File | Optimized For | Key Trait |
| :--- | :--- | :--- |
| `config.yaml` | Zoo Code, Aider, OpenCode | `write_to_file` whole-file overwrites — local models handle this well |
| `config.cline.yaml` | Cline | `replace_in_file` diff edits — requires exact `old_text` match, needs cloud precision |

### Why Cline Needs a Different Config

Cline's `SdkDiffEditCoordinator` requires the model to reproduce the **exact existing file content** in an `old_text` parameter. Local 12B models frequently fail this, producing near-matches that Cline rejects. Zoo Code's `write_to_file` approach sends the entire new file content, which local models handle reliably.

### Key Tuning Differences

| Parameter | Zoo Code (`config.yaml`) | Cline (`config.cline.yaml`) | Why |
| :--- | :--- | :--- | :--- |
| Local Token Threshold | `Tokens < 16000` | `Tokens < 10000` | Cline starts diffing by Turn 3–5; escalate earlier |
| `HasToolProgress` Guard | Not used | `!HasToolProgress` | Once files are created, edits need cloud precision |
| Cycle Killer Prose Limit | 4096 | 6144 | Cline's XML tool format needs more prose room |
| Cycle Killer Repetition | 3 | 4 | Cline's XML is naturally more repetitive |
| `kickstart_threshold` | 5 | 0 (disabled by default) | Zoo Code is prone to `read_file`/`update_todo` semantic loops |
| `kickstart_write_only` | Optional (`true`) | Not needed | Ignores `read_file`/`list_dir` tool churn, only counts write/exec tools |
| Max Context (Local) | 64000 | 32000 | Force cloud escalation earlier for edit-heavy tasks |

### Usage

```bash
# For Cline users:
nacho-flow -config config.cline.yaml

# For Zoo Code / Aider / OpenCode users (default):
nacho-flow
```

---

## 5. Testing & Validating Your Rules

### Dry-Run Validation
Nacho Flow validates all `expr` rules at startup. If any syntax error or missing variable is detected, the server refuses to boot and displays the exact line and character:

```bash
$ nacho-flow -config config.yaml
# Evaluator compile error: unknown name "TokenCount" (did you mean "Tokens"?)
```

### ⚡ Test Rules On-The-Fly with HotSauce Directives
You don't need to restart the server or edit `config.yaml` to test how a model behaves on a specific turn. Simply splash a **HotSauce directive** right into your agent chat prompt:

```text
@nacho:tier="Deep Reasoning" please analyze the lock contention in this channel fan-out
@nacho:local write a unit test for this handler
@nacho:reasoning prove why this algorithm is O(N log N)
```
Nacho Flow strips the directive and routes the turn directly to the requested tier, allowing you to test candidate rules in real time.

### 🔍 Live Route Inspector (VS Code Webview)
If you use the [VS Code Companion Extension](EXTENSION_USER_GUIDE.md), open the **Nacho Flow Dashboard** (`Ctrl+Shift+P` → `Nacho Flow: Show Dashboard`). The **Live Route Inspector** displays a real-time table of your last 500 LLM requests with:
* Exact prompt token count
* Matched routing tier and rule reason
* Turn latency (ms)
* Estimated dollars saved ($0.00 vs Cloud)
* Upstream target provider

---

## 6. Autonomous Multi-Tier Auto-Tuning (`nacho-flow tune`)

While manual rule crafting is powerful, human developers shouldn't have to guess where local open-weight models begin to struggle or how complex multi-tier cascades interact. Nacho Flow features an autonomous **v3 Min-Conflicts Constraint Satisfaction Auto-Tuner** that replays historical traffic logs, identifies bottleneck boundaries, and synthesizes optimal multi-tier routing rules.

---

### 6.1 6D Constraint Satisfaction Formulation & Min-Conflicts Heuristic

Traditional grid search is limited to evaluating a single token boundary on a single local tier ($O(N)$). Modern agentic workflows, however, rely on **multi-tier cascades** (e.g. Kickstart Tier $\rightarrow$ Vision Tier $\rightarrow$ Local Workhorse $\rightarrow$ Cloud Escalation $\rightarrow$ Frontier Fallback).

To optimize an entire fleet of tiers simultaneously, Nacho Flow formulates rule tuning as a **Constraint Satisfaction Problem (CSP)** solved via an adaptive **Min-Conflicts Local Search** heuristic:

```mermaid
flowchart TD
    subgraph CSP["6D Constraint Satisfaction Formulation"]
        T["Tiers Variables (Token Thresholds Tk)"]
        R["Retry Ceilings (Rk)"]
        Tools["Tool Adherence Flags (HasTools)"]
        Vision["Vision Modality Flags (HasImages)"]
        KW["Friction Keywords (any(Keywords, ...))"]
        Model["Hardware Model Selection (VRAM & Benchmarks)"]
    end

    Logs["Historical Traffic Logs (traffic.jsonl)"] --> Replay["Zero-Alloc Fleet Replay Engine"]
    CSP --> Replay
    Replay --> Conflict["Fractional Conflict Evaluator"]
    Conflict --> Solver{"Min-Conflicts Solver (Local Search)"}
    Solver -->|Repair Variable| CSP
    Solver -->|Convergence / Low Conflict| Pareto["Pareto Fleet Dominance Verification"]
    Pareto --> Output["Advisory Tuning Report & AST Diff"]
```

#### The 6 Tunable Dimensions Per Tier:
1. **Context Token Threshold ($T_k$)**: Discrete search across $\{1\text{k}, 2\text{k}, 4\text{k}, 8\text{k}, 12\text{k}, 16\text{k}, 20\text{k}, 24\text{k}, 32\text{k}, 64\text{k}\}$.
2. **Retry Escalation Bound ($R_k$)**: Escalation ceiling $\{1, 2, 3, 4, 5, 6\}$ before kicking to cloud.
3. **Tool Adherence Toggle (`HasTools`)**: Disables local routing when agent declares tools if local tool-calling reliability fails.
4. **Vision Modality Toggle (`HasImages`)**: Prevents sending multi-modal image turns to text-only local models.
5. **High-Friction Keyword Exclusions (`any(Keywords, ...)`)**: Isolates concepts with retry odds ratios $\ge 1.5\times$ baseline.
6. **VRAM-Aware Model Substitution**: Inspects the vetted models catalog (`models.json`) to recommend drop-in model upgrades that fit within your target hardware ceiling (e.g. 16GB VRAM).

#### Hard Constraints vs. Soft Penalties:
* **Hard Monotonicity Constraint**: Token thresholds across tiers must monotonically increase or remain valid ($T_0 \le T_1 \le ... \le T_n$). Non-monotonic configurations receive a conflict penalty of $+\infty$.
* **Unbounded Default Tier Protection**: The final fallback tier must have no token or modality restriction ($T_{\text{default}} = \infty$) to guarantee 100% request completion.
* **Hardware VRAM Ceilings**: Local model substitutions cannot exceed `--vram-gb`.
* **Soft Cost & Friction Penalties**:
  $$\text{Objective Conflict} = \text{CloudDirectSpend} + (\text{LocalRetries} \times \text{RetryPenaltyUSD}) + (\text{SessionTurns} \times \text{TurnsWeight})$$

#### Zero-Allocation Replay Performance:
The inner evaluation loop (`pkg/tuner/conflict_evaluator.go`) operates with **zero heap allocations**:
* Replays **1,000,000 turn statements in 25ms** (~40,000,000 statements/sec).
* Pre-allocates trajectory memory and reuses rolling evaluation buffers (`MultiTierReplayResult.Reset()`), allowing thousands of candidate CSP assignments to be evaluated in fractions of a second.

---

### 6.2 Fractional Conflict Attribution & Fleet Dominance

When a prompt turn fails (incurring developer retry frustration or runaway cloud spend), how does the engine know *which* tier or variable caused it?

#### Fractional Attribution:
If a multi-tier session incurs penalty $P$ across $M$ active variables contributing to the failure, the conflict evaluator divides the penalty proportionally:
$$\text{Penalty}_{\text{var}} = \frac{P}{M}$$
This prevents the optimizer from penalizing innocent tiers while pinpointing the exact threshold or keyword rule that permitted the failure.

#### Pareto Fleet Dominance:
Before recommending any rule mutation, the tuner evaluates the candidate configuration against your existing baseline across 3 objectives:
1. **Direct Cloud Spend (USD)**
2. **Average Session Turn Latency (ms)**
3. **Fleet Retry Rate (%)**

A candidate configuration is adopted only if it **Pareto-dominates** the baseline (improving at least one metric without degrading the others) and triggers no static rule dominance conflicts (such as shadowed tiers or unreachable rules).

---

### 6.3 Step-by-Step Tuning Workflow & CLI Options

#### Step 1: Accumulate Natural Traffic
Run Nacho Flow during your normal coding workflow for a few days (recommended: 50 to 500 complete sessions). Telemetry is recorded asynchronously to `logs/traffic.jsonl`:
```json
{"timestamp":"2026-08-24T14:32:00Z","session_id":"sess-8891","tokens":9450,"has_images":false,"has_tools":true,"keywords":["docker","compose"],"retries":0,"is_retry":false,"is_local":true,"tier":"Tier 2: Local GPU","model":"gemma4:12b-it-qat","latency_ms":1250,"cost_saved_usd":0.0236}
```

#### Step 2: Run the Advisory Analysis (Dry-Run)
Run the auto-tuner in advisory mode:
```bash
# Run default v3 Min-Conflicts multi-tier optimization
nacho-flow tune

# Target specific local GPU hardware ceiling (e.g. 16GB VRAM)
nacho-flow tune --vram-gb=16

# Analyze a specific number of complete sessions
nacho-flow tune --max-sessions=100 --traffic-log=logs/traffic.jsonl

# Output machine-readable JSON for CI/CD or automation
nacho-flow tune --format=json
```

**Example Multi-Tier Advisory Output**:
```text
========================================================================================
🌮 NACHO FLOW ADVISORY TUNING REPORT (v3 Min-Conflicts Multi-Tier Optimizer)
========================================================================================

📊 Sample Size: 240 historical sessions evaluated (1,480 prompt turns)
⚙️ Strategy:    min_conflicts (6D CSP Heuristic Local Search)
🖥️ VRAM Target: 16 GB (GPU Bound Active)

🔍 MULTI-TIER FRICTION & BOTTLENECK SIGNALS:
  • Tier 1 (Kickstart):            Token limit 4,000 -> 8,000 | Retries < 3 (Clean)
  • Tier 2 (Local ROCm/Ollama):    Optimal context boundary 16,000 -> 24,000 tokens
  • Local Tool Adherence:          Clean (0% tool failures — tools remain enabled locally)
  • High-Friction Domain Keywords: ['deadlock', 'kubernetes', 'migration'] (Spikes local retries)
  • Recommended Model Upgrade:     gemma4:12b-it-qat (Index: 82.4, Fits 16GB VRAM)

📈 PROJECTED MONTHLY IMPACT:
  • Developer Retries Avoided:     ~32 retries eliminated
  • Cloud Spend Optimization:      +$48.50 USD saved / month
  • Fleet Pareto Dominance:        CONFIRMED (Zero regressions detected)

🛠️ RECOMMENDED CONFIGURATION DIFF:
----------------------------------------------------------------------------------------
  Tier: "Tier 2: Local GPU Free (Ollama 12B + Tool Normalizer)"
  - when: "Tokens < 12000 && !HasImages && Retries < 2"
  + when: "Tokens < 24000 && !any(Keywords, { # in ['deadlock', 'kubernetes', 'migration'] }) && Retries < 2"
----------------------------------------------------------------------------------------

To apply this recommendation with automatic backup:
  $ nacho-flow tune --apply
========================================================================================
```

#### Step 3: Apply Recommendations Automatically
```bash
# Applies the synthesized rule to config.yaml and creates an automatic timestamped backup
nacho-flow tune --apply
```
```text
✅ SUCCESS: Successfully updated config.yaml with optimal rules!
   Backup saved at: config.yaml.bak.20260923-000500
   Restart or reload nacho-flow to activate changes.
```

#### CLI Options Reference:
| Flag | Default | Description |
| :--- | :--- | :--- |
| `--strategy` | `min_conflicts` | Optimization engine: `min_conflicts` (v3 multi-tier CSP heuristic) or `grid_sweep` / `cost_penalty` (legacy v2 single-tier). |
| `--vram-gb` | `0` | Target local GPU VRAM ceiling in GB (e.g. `8`, `16`, `24`; `0` = infer from current model). |
| `--max-sessions` | `0` | Maximum complete multi-turn sessions to analyze (`0` = all). Replaces turn-level `--sample`. |
| `--config` | `config.yaml` | Path to target configuration file to inspect and update. |
| `--traffic-log` | `logs/traffic.jsonl` | Path to historical traffic JSONL log file. |
| `--format` | `text` | Output format: `text` (human-readable terminal report) or `json` (machine-readable). |
| `--apply` | `false` | Atomically writes synthesized rules to config file with `.bak.<timestamp>` backup. |

> [!TIP]
> **1-Click Auto-Tuning in VS Code**: You can also trigger the empirical optimizer, review recommended rule diffs, and hot-reload `config.yaml` with one click directly from the **Nacho Flow Analytics Dashboard** webview inside the [VS Code Companion Extension](EXTENSION_USER_GUIDE.md).

![Nacho Flow VS Code Auto-Tuner UI Recommendation Banner](images/vscode-autotuner-showcase.png)

---

### 6.4 High-Throughput 1M-Statement Stress Testing (`nacho_stress`)

To verify the speed and zero-allocation characteristics of the multi-tier replay engine on your local hardware, Nacho Flow includes the standalone `nacho_stress` utility:

```bash
# Generate and replay 1,000,000 synthetic turn statements
go run ./cmd/util/nacho_stress -turns 1000000 -runs 3
```

**Output**:
```text
=================================================================================
   🌶️ NACHO-FLOW ENGINE STRESS TEST & 1M STATEMENT LOAD BENCHMARK
=================================================================================
  Target Statements : 1000000
  PRNG Seed         : 42
  OS / Architecture : windows / amd64 (GOMAXPROCS=16)
---------------------------------------------------------------------------------
[1/3] Synthesizing 1000000 realistic telemetry turns in memory...
      Generated 166667 sessions in 3.65s (Dataset Heap: 147.2 MB)
[2/3] Executing 3 multi-tier fleet cascade replay runs across 1,000,000 statements...
      Run 1: 25.12 ms (39.81 M statements/sec) | Allocs: 0 B
      Run 2: 24.89 ms (40.18 M statements/sec) | Allocs: 0 B
      Run 3: 24.95 ms (40.08 M statements/sec) | Allocs: 0 B
      AVERAGE REPLAY: 24.99 ms (40.02 M statements/sec)
[3/3] Evaluating full fleet conflict attribution on 1,000,000 statements...
      Conflict Score: 12450.00 in 25.40 ms (0 allocs)
=================================================================================
```

---

## 7. Manual Heuristic Tuning Tips (For Custom Power Rules)

### 7.1 General Routing & Hardware Guardrails
1. **Check Live Financials First**: Run `curl http://127.0.0.1:8000/v1/stats` or view `@nacho:status` in your chat prompt to see your current local vs cloud distribution. Aim for **70%–85% local turns** on typical coding tasks.
2. **Start with Conservative Token Bounds**: If your local GPU has 16GB VRAM running a 14B model, set `Tokens < 12000`. If running an 8B model, set `Tokens < 8000`.
3. **Always Include `Retries < 2` on Local Tiers**: This ensures that if a local model produces a broken response, the agent's second attempt automatically escalates to a cloud frontier model (e.g. Claude Sonnet 5 or DeepSeek-R1) instead of looping indefinitely on local hardware.
4. **Use `strip_images: true` on Text-Only Local Models**: If your agent sends a screenshot in Turn 1, Turn 10 doesn't need 40,000 tokens of raw base64 image data sent to a text-only local model. Setting `strip_images: true` removes legacy base64 strings while preserving conversational text.

---

### 7.2 Tuning Tool Normalizers & Eliminating Regex False Positives
Modern open-weight models have vastly different formatting behaviors:
* **Disable ReAct on Structured Code Models**: Advanced coder models (like `qwen2.5-coder:14b` or `deepseek-chat`) output standard JSON or markdown blocks. If they generate code diffs or commit messages containing phrases like `Action: Added mutex lock`, aggressive ReAct regexes can trigger false-positive tool extractions. Set `normalizers.react: false` on tiers dedicated to structured models.
* **Keep Markdown Fences Active for Ollama/vLLM**: Many open-source models wrap their function calls in ` ```json ... ``` ` blocks. Setting `normalizers.markdown: true` ensures these are converted to standard OpenAI `tool_calls` without breaking client JSON parsers.
* **Raw Passthrough for Benchmarking**: If you want to benchmark raw upstream token velocity or test custom client parsers without proxy interception, set `raw: true` or use `@nacho:raw` in your prompt.

---

### 7.3 Tuning the Agentic Fallback Shield
In agent IDEs like Zoo Code or Cline, agents expect models to always invoke tools (`read_file`, `execute_command`). When smaller local models output conversational plans or ask questions ("Should I proceed with the edit?"), agent harnesses fail with "Model did not invoke a tool" 3-strike deadlocks.
* **For Interactive Coding (IDE)**: Keep `shield: true` (default). Nacho Flow's sliding tail-buffer (4.67ns, 0 allocs) detects trailing question heuristics and wraps the text into an `ask_followup_question` tool call, prompting you in the UI instead of crashing.
* **For Headless CI & Batch Scripts**: Set `shield: false` on your batch tier (or splash `@nacho:no-shield` in prompts). Automated scripts don't have interactive humans to answer tool questions, so returning raw conversational text prevents test runner timeouts.

---

### 7.4 Testing Rules On-The-Fly with Directives
Before committing changes to `config.yaml`, test different routing behaviors live in your chat:
* `@nacho:raw` — Force unadulterated pass-through on the current prompt.
* `@nacho:no-shield` — Disable fallback tool synthesis for the current turn.
* `@nacho:tier="<Name>"` — Route directly to a specific named tier to test its model output.

---

---

## 8. Agent-Specific Harness Tuning: Zoo Code vs. Cline

Different autonomous coding agents interact with LLMs using fundamentally different protocols. Tuning Nacho Flow for your specific extension ensures optimal performance, zero false-positive stream interruptions, and maximum cost efficiency.

### 8.1 Architectural Differences

| Dimension | Zoo Code | Cline |
| :--- | :--- | :--- |
| **Tool Calling Protocol** | OpenAI JSON `tools` parameter | XML tags embedded in conversational prose (`<write_to_file>`) |
| **Context Accumulation** | Compact sliding transcript + pruned tools | Full multi-turn conversation transcripts re-sent every turn |
| **Average Turn 50+ Context** | ~35k–45k tokens | ~80k–110k tokens |
| **Local Model Compatibility** | Gemma 4 12B, Qwen 2.5/3 (native JSON function calling) | Qwen 3 14B, Devstral (XML agent-trained models) |
| **Cycle Killer Content Threshold** | `max_content_tokens: 4096` | `max_content_tokens: 6144` (relaxed for XML preambles) |
| **Kickstart Recommendation** | **Enabled** (`kickstart_threshold: 5`) | **Disabled or High** (`kickstart_threshold: 0` / off) |

### 8.2 Zoo Code Tuning Profile (`config.zoo.yaml`)
Zoo Code uses native OpenAI tool calling. Local models like Gemma 4 produce JSON tool calls reliably without long conversational preambles:
```yaml
cycle_killer:
  enabled: true
  max_content_tokens: 4096
  max_thinking_tokens: 1500
  max_tool_tokens: 8192        # In-flight tool argument repetition breaker (RFC-002)
  repetition_threshold: 3
  kickstart_threshold: 5
  kickstart_write_only: true
```

### 8.3 Cline Tuning Profile (`config.cline.yaml`)
Cline models output XML tags within prose explanations. To avoid false-positive Cycle Killer stream severing, monitor streaming tool arguments, and track Cline-specific Zod schema failures:
```yaml
cycle_killer:
  enabled: true
  max_content_tokens: 6144       # Relaxed for XML preambles
  max_thinking_tokens: 2000    # Extra planning runway
  max_tool_tokens: 8192        # In-flight tool argument repetition breaker (RFC-002)
  repetition_threshold: 4      # XML formats are naturally more repetitive
  # kickstart_threshold: 0     # Disabled: Cline rarely idles in read loops
  kickstart_write_tools:       # Required for Fairy Dust write progress tracking
    - write_to_file
    - replace_in_file
    - execute_command
    - insert_code_block

# Cline Zod schema failure signatures:
agent_shield:
  enabled: true
  error_signatures:
    - "expected string, received undefined"
    - "✖ Invalid input"
    - "Invalid input:"
    - "Parameter 'old_text' is required"
    - "Missing parameter 'old_text'"
```

Cline validates model tool arguments using strict Zod schemas. When a model omits required parameters (e.g. `old_text` in diff tools) or produces unexpected types, Cline writes validation error messages into the conversation history. Configuring `error_signatures` allows Nacho Flow to detect these failures as `historyErrors`, increment retry tracking, and auto-escalate to Tier 4 / Cloud Fallback before the agent gets stuck in a loop.

---

## 9. 🧚 Fairy Dusting & Cost Shield Architecture

### 9.1 Proactive Quality Checkpoints
Rather than debugging syntax errors and missing module extensions 40 turns into a session, Fairy Dusting introduces scheduled frontier checkpoints triggered **strictly on productive write turns** (`WriteProgressCount`):

```yaml
fairy_dust:
  enabled: true
  entries:
    # Tactical Checkpoint: Catches missing imports (.js in ESM), type mismatches, syntax regressions
    - name: "Tactical Code Review"
      frequency: 15       # Fires every 15 file writes
      max_count: 5        # Hard cap: max 5 reviews per session
      provider: "openrouter"
      model: "anthropic/claude-sonnet-5"
      prompt: >
        [SYSTEM CHECKPOINT: TACTICAL CODE REVIEW]
        Inspect recent edits for syntax correctness, valid imports, and edge-case errors.

    # Strategic Checkpoint: Catches architectural drift, missing requirements, structural debt
    - name: "Strategic Architecture Review"
      frequency: 40       # Fires every 40 file writes
      max_count: 2        # Hard cap: max 2 deep audits per session
      provider: "openrouter"
      # model: "anthropic/claude-opus-5"
      model: "anthropic/claude-sonnet-5" # swap in opus 5 for tough jobs
      prompt: >
        [SYSTEM CHECKPOINT: STRATEGIC ARCHITECTURE REVIEW]
        Audit implementation against initial requirements and resolve systemic structural drift.
```

### 9.2 The Cost-Safe Default Tier Shield
In runaway error cascades or edge cases, routing must **never default to ultra-expensive models** ($15/$75 per 1M Opus 5):
1. **Set `default_tier` to Claude Sonnet 5**: Sonnet 5 ($3/$15) is 5× cheaper than Opus. A 30-turn runaway costs ~$2.50 instead of $12.65+.
2. **Isolate Opus with `when: "false"`**:
   ```yaml
   - name: "Tier 5: Opus On-Demand (Spicy Only)"
     provider: "openrouter"
     model: "anthropic/claude-opus-5"
     when: "false"
   ```
   This guarantees that automated routing never accidentally lands on Opus.
3. **Fairy Dust Cost Safety (Sonnet 5 Default)**: Even within Fairy Dusting periodic quality reviews, Opus 5 is commented out by default (`# model: "anthropic/claude-opus-5"`) and replaced with Claude Sonnet 5 (`model: "anthropic/claude-sonnet-5"`). For heavy codebases or difficult multi-file architectural refactors, developers can easily swap in Opus 5 by uncommenting the line. Outside Fairy Dust, Opus remains accessible solely via manual in-prompt `@nacho:model` / `X-Spicy-Model` overrides.

### 9.3 HotSauce Kickstart & Plan-Mode Tuning

**Kickstart** detects semantic idle churn where an agent gets trapped in reading/planning loops without writing code:

```yaml
cycle_killer:
  kickstart_threshold: 5        # Jolt after N idle turns without write tool progress
  kickstart_max_count: 10       # Max kickstart escalations before default cloud failover
  kickstart_max_failures: 3     # Suppress override after N consecutive failures
  kickstart_write_only: true    # Only file writes/terminal executions reset the idle counter
  kickstart_write_tools:        # Tools defining write capability
    - write_to_file
    - replace_in_file
    - replace_file_content
    - multi_replace_file_content
    - execute_command
```

#### Automatic Plan-Mode Auto-Suspension:
In pure investigation or planning phases (such as Cline/Zoo Code Plan Mode), the client only declares read/inspection tools (`view_file`, `list_dir`, `grep_search`). Nacho Flow automatically evaluates `HasWriteCapability == false` and **suspends Kickstart idle stall escalation**. The agent can read files and explore codebases across 50+ turns without premature `[SYSTEM OVERRIDE]` prompts.

#### Dynamic Session Control Directives:
If you want to manually disable or re-enable Kickstart or Cycle Killer mid-session without restarting the gateway:
- `@nacho:kickstart-off` / `@nacho:kickstart-on`: Suspends or resumes Kickstart idle tracking for the active session.
- `@nacho:cyclekiller-off` / `@nacho:cyclekiller-on`: Suspends or resumes Cycle Killer in-flight stream loop interruption.
- `@nacho:toggles`: Inspects the active state of all session guardrails.
- `@nacho:reset`: Resets session turns and restores all switches to configuration defaults.

---

## 10. Troubleshooting & FAQ

### Q: How do I know if my local GPU is actually processing turns?
- **Response Headers**: Check the `x-nacho-router-tier` and `x-nacho-target-model` headers returned in your agent's HTTP responses.
- **In-Prompt Directive**: Type `@nacho:status` directly into your chat prompt to see total tokens routed locally vs to cloud and dollars saved.
- **CLI Log Output**: When running interactively, the daemon prints green routing log entries:
  ```text
  INFO Routing request tier="Local ROCm GPU" model=qwen2.5-coder:14b tokens=4,120 is_fallback=false
  ```

### Q: What happens if Ollama or my local GPU runs out of VRAM or crashes?
- **Zero Broken Loops**: Nacho Flow's built-in **Circuit Breaker** detects the connection failure or defective empty response and immediately re-routes the prompt to your configured `default_tier` (Cloud Fallback) with **instant in-memory failover**.
- **Exception**: If you explicitly forced `@nacho:local` via a HotSauce directive, Nacho Flow respects your strict override and returns a zero-cost chat alert instead of billing your credit card.

### Q: How do I test a new rule before putting it in production?
- You can override any turn on-the-fly directly from your chat prompt using **🌶️ HotSauce Directives** (`@nacho:local`, `@nacho:cloud`, `@nacho:reasoning`, `@nacho:tier="..."`, `@nacho:model="..."`) without modifying `config.yaml`.

### Q: Why didn't Kickstart fire while my agent spent 15 turns reading code in Plan Mode?
- This is intentional! Nacho Flow's **Tool Schema Guard** inspects the declared tools in each request. If the tool schema contains zero write tools (`HasWriteCapability == false`), Kickstart automatically suspends idle stall escalation so your agent can plan and investigate uninterrupted.

### Q: How do I inspect or toggle session switches in my editor chat?
- Type `@nacho:toggles` alone in your chat prompt for an instant zero-cost ($0.00 / 0 tokens) view of all session guardrails.
- Type `@nacho:kickstart-off`, `@nacho:cyclekiller-off`, `@nacho:shield-off`, `@nacho:raw-on`, or `@nacho:reset` to change switches on the fly.



