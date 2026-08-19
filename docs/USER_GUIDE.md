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

### Pre-compiled Binaries (Recommended)
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

  # Tier 3: Local GPU (100% Free, Routine tasks < 16k context without images or tools)
  - name: "Local ROCm GPU"
    model: "qwen2.5-coder:14b"
    provider: "ollama"
    when: "Tokens < 16000 && !HasImages && !HasTools"
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

---

## 4. Running Modes (Interactive vs. OS Service)

### Option A: Interactive Mode (CLI)
Run directly from your terminal:
```bash
nacho-flow -config config.yaml -log-level info
```
- Logs output to **both standard output and `logs/router.log`** with automatic 10MB file rotation.

### Option B: Native OS Background Daemon
Install and manage Nacho Flow as a persistent background system service:

```bash
# Install as a system service (Windows Service / systemd / launchd)
nacho-flow service install

# Start the background service
nacho-flow service start

# Check service status / Stop service
nacho-flow service stop
nacho-flow service uninstall
```
- When running as a service, logs are automatically piped to your OS-native logger (systemd journal on Linux, Windows Event Log on Windows).
- Cumulative cost savings are automatically persisted to `~/.config/nacho-flow/stats.json`.

---

## 5. IDE & Agent Integrations

Nacho Flow exposes a standard OpenAI-compatible API on `http://127.0.0.1:8000/v1`.

### 1. Roo Code (VS Code Extension)
In **Roo Code Settings**:
- **API Provider**: `OpenAI Compatible`
- **Base URL**: `http://localhost:8000/v1`
- **API Key**: `sk-dummy` *(Nacho Flow injects your real keys)*
- **Model ID**: `nacho-hybrid`
- **Stream**: `ON`

### 2. Cline / Aider / Cursor / Continue
Set the OpenAI Base URL in your client configuration:
```bash
OPENAI_BASE_URL="http://127.0.0.1:8000/v1"
OPENAI_API_KEY="sk-dummy"
```

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

