# 🌮 Nacho Flow: User Guide

Welcome to the **Nacho Flow** user guide. This document explains how to configure, run, and integrate Nacho Flow with your favorite autonomous coding agents and IDEs.

---

## Table of Contents
1. [Installation & Setup](#1-installation--setup)
2. [Configuration Reference (`config.yaml`)](#2-configuration-reference-configyaml)
3. [Writing Custom Routing Tiers (`expr` Rules)](#3-writing-custom-routing-tiers-expr-rules)
4. [Running Modes (Interactive vs. OS Service)](#4-running-modes-interactive-vs-os-service)
5. [IDE & Agent Integrations](#5-ide--agent-integrations)
6. [Monitoring, Telemetry & Stats API](#6-monitoring-telemetry--stats-api)

---

## 1. Installation & Setup

### Homebrew (macOS & Linux - Recommended)
```bash
brew install dixieflatline76/nacho-flow/nacho-flow
```

### Pre-compiled Binaries
Download the latest binary for your operating system from [GitHub Releases](https://github.com/dixieflatline76/nacho-flow/releases):
- **Windows (AMD64)**: `nacho-flow-windows-amd64.zip` (Cryptographically signed with Azure Trusted Signing)
- **Linux (AMD64 / ARM64)**: `nacho-flow-linux-amd64.tar.gz` / `nacho-flow-linux-arm64.tar.gz`
- **macOS (Apple Silicon / Intel)**: `nacho-flow-darwin-arm64.tar.gz` / `nacho-flow-darwin-amd64.tar.gz`

### Install via Go
```bash
go install github.com/dixieflatline76/nacho-flow/cmd/nacho-flow@latest
```

---

## 2. Configuration Reference (`config.yaml`)

Create a `config.yaml` in your project folder or `~/.config/nacho-flow/config.yaml`:

```yaml
# Port to listen on (Default: 8000)
port: 8000

# Inbound Gateway Authentication (Optional: Secures gateway on LAN / 0.0.0.0)
auth_token: "sk-nacho-gateway-token"

# Upstream model providers (Local GPUs, Cloud Gateways, Direct APIs)
providers:
  # 1. Local GPU (Ollama / vLLM / llama.cpp - $0.00 cost)
  ollama:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"

  # 2. OpenRouter Aggregator
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    api_key: "ENV_OPENROUTER_API_KEY"
    headers:
      HTTP-Referer: "https://spicebox.dev"
      X-Title: "nacho-flow"

  # 3. Langdock Enterprise / Private Tenant
  langdock:
    base_url: "https://api.langdock.com/v1"
    api_key: "ENV_LANGDOCK_API_KEY"
    headers:
      X-Custom-Org: "engineering"

  # 4. DeepSeek Direct
  deepseek:
    base_url: "https://api.deepseek.com/v1"
    api_key: "ENV_DEEPSEEK_API_KEY"

# Ordered Dynamic Routing Tiers (Evaluated from top to bottom: First Match Wins)
tiers:
  # Tier 1: Concurrency & Complex Reasoning Keywords
  - name: "Cloud Reasoning"
    model: "deepseek/deepseek-r1"
    provider: "openrouter"
    when: "any(Keywords, { # in ['deadlock', 'mutex', 'race', 'concurrency', 'atomic'] })"

  # Tier 2: Multimodal Vision (Screenshots / Images)
  - name: "Cloud Vision"
    model: "google/gemini-2.5-flash-lite"
    provider: "openrouter"
    when: "HasImages"

  # Tier 3: Local GPU (100% Free, Routine tasks < 16k context without images)
  - name: "Local ROCm GPU"
    model: "qwen2.5-coder:14b"
    provider: "ollama"
    when: "Tokens < 16000 && !HasImages"
    strip_images: true

  # Tier 4: Fast Agentic Cloud (Large context >= 16k or active tool calls)
  - name: "Cloud Agentic Fast"
    model: "qwen/qwen3-coder-30b-a3b-instruct"
    provider: "openrouter"
    when: "Tokens >= 16000 || HasTools"

# Fallback Tier if no expr rules match
default_tier:
  name: "Cloud Fallback"
  model: "~deepseek/deepseek-v4-flash-latest"
  provider: "openrouter"
  when: "true"
```

---

## 3. Writing Custom Routing Tiers (`expr` Rules)

For a complete guide with recipes on tuning token thresholds, keyword extraction, and reasoning parameters, check out the **[Rule & Tier Tuning Guide](TUNING_GUIDE.md)**.

### Available Variables in `when` Expressions:
| Variable | Type | Description | Example |
| :--- | :--- | :--- | :--- |
| `Tokens` | `int` | Estimated token count of the entire prompt history | `Tokens < 16000` |
| `HasImages` | `bool` | `true` if the request includes screenshots or image URLs | `HasImages == true` |
| `HasTools` | `bool` | `true` if the request provides function/tool definitions | `HasTools == true` |
| `Keywords` | `[]string` | Extracted code keywords (`mutex`, `sql`, `refactor`) | `any(Keywords, { # == 'sql' })` |
| `Model` | `string` | The requested model ID sent by the client | `Model == 'nacho-hybrid'` |

### ⚠️ Critical Rule: Top-to-Bottom Precedence (First Match Wins)
Nacho Flow evaluates tiers sequentially from **top to bottom**. The **first tier whose `when` condition evaluates to `true` is selected**.

* **Place Narrow & Specialized Tiers at the TOP:** High-complexity keyword matching (`Keywords`), multimodal vision (`HasImages`), and agent tool calls (`HasTools`) must be listed before broad catch-all rules.
* **Place Broad & Free Local Tiers in the MIDDLE:** Rules like `Tokens < 16000 && !HasImages` should sit below specialized tiers to prevent shadowing reasoning prompts.
* **Fallback Tier at the BOTTOM:** The `default_tier` acts as the safety net if none of the above match.

### 🛠️ Universal Multi-Model Tool-Calling Normalizer
Open-source models (e.g. Qwen 2.5, Mistral, Llama 3.1, Hermes) often return tool calls formatted inside markdown code fences or specialized XML tags rather than native OpenAI `tool_calls` structures.

Nacho Flow includes a **zero-alloc lexical bracket balancer** that automatically detects and converts 7 format families on the fly:
1. **Hermes / Nous / Qwen ChatML**: `<tool_call>{"name":"...","arguments":{...}}</tool_call>`
2. **Mistral / Mixtral**: `[TOOL_CALLS] [{"name":"...","arguments":{...}}]`
3. **Llama 3 Tags**: `<function=name>{"param":"value"}</function>`
4. **Llama 3.1 Python Calls**: `<|python_tag|>tool_name.call(param="value")`
5. **Claude XML Format**: `<function_calls><invoke name="..."><parameter name="...">...</parameter></invoke></function_calls>`
6. **ReAct / LangChain**: `Action: tool_name\nAction Input: {...}`
7. **DeepSeek-R1 CoT Reasoning**: Preserves `<think>...</think>` internal thinking while extracting the embedded tool call block.

Nacho Flow converts all of these into standard OpenAI `tool_calls` arrays with stringified `function.arguments` JSON, enabling **flawless tool execution in Roo Code, Cline, Cursor, and Antigravity**.

---

## 4. Running Modes & OS Background Service Installation

Nacho Flow can run either interactively in your foreground terminal or as a persistent, autostarting background system service across Windows, macOS, and Linux.

---

### Option A: Interactive Mode (CLI Foreground)
Ideal for testing configurations, viewing live colorized terminal logs, and tuning rules:
```bash
nacho-flow -config config.yaml -log-level info
```
* Logs stream to **both standard output and `logs/router.log`** with automatic 10MB rotation.
* Telemetry records are written to `logs/traffic.jsonl`.
* Press `Ctrl+C` to cleanly shut down.

---

### Option B: Native OS Background Daemon (Autostart on Boot)

Nacho Flow integrates with native OS service managers via `nacho-flow service <command>` (`install`, `start`, `stop`, `restart`, `uninstall`).

#### 🪟 1. Windows Service Installation (Windows 10 / 11 / Server)
Install Nacho Flow to run in the background under the Windows Service Control Manager:

1. **Open PowerShell as Administrator** (Right-click PowerShell $\rightarrow$ *Run as Administrator*).
2. **Install and start the service:**
   ```powershell
   # Move binary and config to permanent location (e.g. C:\Program Files\nacho-flow\)
   .\nacho-flow.exe service install
   .\nacho-flow.exe service start
   ```
3. **Manage the service:**
   ```powershell
   # Check service status
   Get-Service nacho-flow
   
   # Stop or Restart
   Stop-Service nacho-flow
   Start-Service nacho-flow
   
   # Uninstall service
   .\nacho-flow.exe service uninstall
   ```
4. **Service Logs:** Viewable in **Windows Event Viewer** under `Windows Logs -> Application` (Source: `nacho-flow`).

---

#### 🍎 2. macOS Service Installation (`launchd` / LaunchDaemons)
Install Nacho Flow to start automatically on macOS boot using Apple's native `launchd`:

1. **Install and start the daemon (requires `sudo`):**
   ```bash
   # Install as a LaunchDaemon (/Library/LaunchDaemons/nacho-flow.plist)
   sudo nacho-flow service install
   sudo nacho-flow service start
   ```
2. **Manage the service:**
   ```bash
   # Check status / Stop / Uninstall
   sudo nacho-flow service stop
   sudo nacho-flow service uninstall
   ```
3. **Live Log Streaming:** Stream logs via macOS Unified Logging System:
   ```bash
   log stream --predicate 'process == "nacho-flow"' --level info
   ```

---

#### 🐧 3. Linux Service Installation (`systemd`)
Install Nacho Flow as a `systemd` service on Ubuntu, Debian, Arch, Fedora, or Rocky Linux:

1. **Install and enable the service (requires `sudo`):**
   ```bash
   sudo nacho-flow service install
   sudo nacho-flow service start
   ```
2. **Manage via `systemctl`:**
   ```bash
   # Check real-time service status
   sudo systemctl status nacho-flow

   # Enable auto-start on boot
   sudo systemctl enable nacho-flow

   # Restart or stop
   sudo systemctl restart nacho-flow
   sudo systemctl stop nacho-flow

   # Uninstall
   sudo nacho-flow service uninstall
   ```
3. **Live Log Streaming:**
   ```bash
   journalctl -u nacho-flow -f -o cat
   ```

---

### Service Telemetry & Persistence Notes
* **Data Storage:** When running as a service, cumulative cost savings and tier analytics are automatically persisted to `~/.config/nacho-flow/stats.json`.
* **Zero Overhead:** Background service execution consumes $< 25\text{MB}$ RAM and $0.00\%$ idle CPU.

---

## 5. IDE & Agent Integrations (Local & Multi-Device LAN)

Nacho Flow exposes a standard OpenAI-compatible API on `http://127.0.0.1:8000/v1` (or your host LAN / Tailscale IP, e.g. `http://192.168.1.100:8000/v1`).

### 1. Roo Code (VS Code Extension)
In **Roo Code Settings**:
- **API Provider**: `OpenAI Compatible`
- **Base URL**: `http://localhost:8000/v1` *(or `http://<lan-ip>:8000/v1`)*
- **API Key**: `sk-dummy` *(or your `auth_token` if enabled in `config.yaml`)*
- **Model ID**: `nacho-hybrid`
- **Image Support**: `ON`
- **Stream**: `ON`

### 2. Cline / Aider / Cursor / Continue
Set the OpenAI Base URL in your client configuration:
```bash
# Local Single-Machine Setup
OPENAI_BASE_URL="http://127.0.0.1:8000/v1"
OPENAI_API_KEY="sk-dummy"

# Remote Multi-Device / LAN Setup (with auth_token configured)
OPENAI_BASE_URL="http://192.168.1.100:8000/v1"
OPENAI_API_KEY="sk-nacho-gateway-token"
```

### 🔒 Dual-Layer Gateway Security (LAN / Tailscale)
* **Inbound Protection:** When `auth_token` is set in `config.yaml`, the gateway blocks any unauthorized LAN/Tailnet clients with `401 Unauthorized`.
* **Outbound Injection:** Once authenticated, the gateway attaches your upstream cloud credentials (`OpenRouter`, `Langdock`) or strips auth for `Ollama` automatically.

---

## 6. Monitoring, Telemetry & Stats API

Nacho Flow includes built-in live analytics and model endpoints:

### Check Health:
```bash
curl http://127.0.0.1:8000/health
```
```json
{"status":"ok","service":"nacho-flow"}
```

### View Live Analytics & Cost Savings:
```bash
curl http://127.0.0.1:8000/v1/stats
```
```json
{
  "started_at": "2026-08-19T20:15:46Z",
  "total_requests": 2540,
  "tier_breakdown": {
    "tier1_local_free": 1820,
    "tier2_cloud_coder": 510,
    "tier3_cloud_reasoning": 140,
    "tier4_cloud_vision": 60,
    "explicit_override": 0,
    "fallbacks": 10
  },
  "total_tokens_routed_locally": 14500000,
  "estimated_cost_saved_usd": 65.2500
}
```

---

## 7. Autonomous Rule Auto-Tuning (`nacho-flow tune`)

Nacho Flow features a built-in, pure Go **Recurrent Bandit Optimizer** that analyzes your team's real-world traffic, identifies prompt failure bottlenecks, and generates human-readable rule recommendations.

### Run Advisory Analysis (Dry-Run):
```bash
# Analyze historical traffic and generate recommendation diff
nacho-flow tune

# Analyze custom sample size from specific traffic log
nacho-flow tune --sample 10000 --traffic-log logs/traffic.jsonl
```

### Apply Recommendations Automatically:
```bash
# Applies the synthesized rule to config.yaml and saves an automatic timestamped backup (config.yaml.bak.<timestamp>)
nacho-flow tune --apply
```

