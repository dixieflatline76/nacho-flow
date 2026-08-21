# 🌮 Nacho Flow (`spicebox.dev/nacho-flow`)

[![GitHub Release](https://img.shields.io/github/v/release/dixieflatline76/nacho-flow?color=green&label=version)](https://github.com/dixieflatline76/nacho-flow/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/dixieflatline76/nacho-flow)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> **A fast, zero-dependency hybrid AI gateway that routes agent prompt turns between local GPUs and cloud APIs.**

**Nacho Flow** is an OpenAI-compatible proxy built in pure Go. It sits between autonomous coding agents ([Roo Code](https://github.com/RooVetGit/Roo-Code), [Cline](https://github.com/cline/cline), [Aider](https://github.com/paul-gauthier/aider), [Cursor](https://www.cursor.com), [Continue](https://continue.dev)) and LLM backends, dynamically evaluating each turn to route between **local GPUs** ([Ollama](https://ollama.com), [vLLM](https://github.com/vllm-project/vllm), [LM Studio](https://lmstudio.ai), [llama.cpp](https://github.com/ggerganov/llama.cpp)) and **cloud endpoints** ([OpenRouter](https://openrouter.ai), [DeepSeek](https://www.deepseek.com), [Langdock](https://www.langdock.com), [Azure OpenAI](https://azure.microsoft.com/en-us/products/ai-services/openai-service)).

Part of the **[spicebox.dev](https://spicebox.dev)** developer tool suite by [@dixieflatline76](https://github.com/dixieflatline76).

---

## 🌟 Why Nacho Flow?

### 💡 The Origin Story
I built Nacho Flow after hitting a very familiar wall: I ran out of free Antigravity IDE daily tokens mid-session, topped up an OpenRouter account with €10, and watched €2.50 disappear on a single prompt turn just asking the agent to analyze the `docs/` folder in the Spice project for context.

Autonomous coding agents (Roo Code, Cline, Aider, Cursor) operate in multi-turn feedback loops. As conversations progress, agent harnesses re-send the full transcript, file contents, and execution logs with every prompt turn. When paying cloud rates per million tokens, large prompt dumps quickly drain credits—even for trivial queries, inspections, or routine 1-line edits.

* **Local GPUs excel at small contexts**: A modern workstation GPU ([AMD Radeon RX 7900 XTX](https://www.amd.com/en/products/graphics/desktops/radeon.html), [NVIDIA RTX 4090](https://www.nvidia.com/en-us/geforce/graphics-cards/40-series/rtx-4090/), [Apple Mac Studio M-Series](https://www.apple.com/mac-studio/)) can run 14B–32B parameter coding models locally at high speeds and zero marginal cost for small-to-medium contexts (< 16k tokens).
* **Context growth strains local VRAM**: As conversations expand beyond 16k–32k tokens, local memory bandwidth and context limits become bottlenecks.
* **Turn-by-turn hybrid routing**: Nacho Flow solves this by evaluating prompt metadata turn-by-turn. Early iterative turns (routine edits, tests, inspections) stay on local hardware for free. When context expands or complex multi-file reasoning is required, requests automatically hand off to cloud APIs.

---

## 🥊 Nacho Flow vs. Cloud-Only Routers (e.g. OpenRouter Auto, LiteLLM)

| Dimension | Cloud Routers (e.g. `openrouter/auto`) | Nacho Flow (Hybrid Edge Gateway) |
| :--- | :--- | :--- |
| **Routing Domain** | **Cloud-to-Cloud only** (100% paid). | **Hybrid Edge Gateway** (Local GPU $\leftrightarrow$ Cloud APIs). |
| **Cost Floor** | **$0.00 is impossible**. Every prompt incurs cloud fees. | **$0.00 for routine turns** on local hardware. |
| **Hardware Utilization** | Completely ignores your local GPU / NPU. | Maximizes local VRAM on early turns before context limits are reached. |
| **Routing Logic** | Black-box trailing 7-day community spend. | **Deterministic AST Bytecode Rules** (`Tokens`, `Retries`, `Keywords`). |
| **Target Providers** | Single cloud aggregator lock-in. | **Any Provider**: [Ollama](https://ollama.com), [vLLM](https://github.com/vllm-project/vllm), [LM Studio](https://lmstudio.ai), [Langdock](https://www.langdock.com), [Azure](https://azure.microsoft.com), [DeepSeek](https://www.deepseek.com), [OpenRouter](https://openrouter.ai). |
| **Open-Source Tool Fixing** | None for local models. | **Normalizes 7 tool-calling format families & thinking tags on the fly**. |
| **Local Self-Healing** | None. | **Circuit Breakers & Delayed Header streaming failovers**. |

> [!TIP]
> **They are complementary!** You can configure `openrouter/auto` as your cloud fallback tier inside Nacho Flow: run routine prompt turns on your local GPU for **$0.00**, and let OpenRouter Auto Router pick the best cloud model whenever prompts exceed local limits.

---

## ✨ Key Features

* **⚡ High-Throughput Core**: Adds < 0.36 ms routing overhead and handles 30,000+ req/s using lock-free atomic RCU state and pooled HTTP transports.
* **🧠 Reasoning Stream Normalization (`<think>`)**: Intercepts SSE streams from DeepSeek-R1, QwQ, Qwen 2.5 (`<|im_start|>think`), and Anthropic-style models (`<thinking>`), converting reasoning tokens into `<think>...</think>` tags in real time for client UI accordions.
* **🚦 Local Provider Circuit Breaker**: Detects consecutive local connection or 5xx failures and fast-fails directly to cloud fallback tiers with 0ms dial delay.
* **🔄 Retry-Based Auto-Escalation**: Tracks session turn retries with a sliding 5-minute TTL, allowing routing rules to automatically escalate to cloud models when local attempts fail (`Retries < 2`).
* **📏 Adaptive Token Estimator**: Continuously calibrates character-to-token ratios using an Exponential Moving Average (EMA) to prevent context undercounting on code and structured JSON.
* **🛡️ Response Quality Validation & Delayed Headers**: Peeks initial SSE stream chunks before committing `HTTP 200` headers to enable transparent cloud failover if a local model returns an empty payload or unexpected termination.
* **📐 Model Context Window Guard (`max_context`)**: Evaluates model physical context limits with O(1) pre-guards to prevent 400 Context Length Exceeded errors.
* **🛠️ Universal Multi-Model Tool Normalizer**: Converts raw tool-call formats (Hermes `<tool_call>`, Mistral `[TOOL_CALLS]`, Llama 3 `<function>`, Claude XML, ReAct, DeepSeek-R1) into standard OpenAI `tool_calls` JSON structures.
* **🔒 Inbound Gateway Client Authentication**: Secures LAN and remote endpoints with optional Bearer token authentication while preserving a public `/health` endpoint.
* **🎯 Dynamic Expression Tiers (`expr-lang/expr`)**: Evaluates custom tier rules in `config.yaml` based on token estimates, tool calls, images, retries, and prompt keywords.
* **🖼️ Historical Image Sanitization**: Automatically strips base64 `image_url` payloads from older turns when routing to text-only models.
* **🏷️ Dynamic Version Reporting**: Exposes build version across `/health`, `/v1/health`, and CLI (`nacho-flow version`, `-v`).
* **💾 Persistent Telemetry Store**: Saves cumulative token counts and estimated cost metrics to disk (`~/.config/nacho-flow/stats.json`).
* **🧪 Engineered for Reliability**: 96.1% overall statement test coverage (minimum 95.1% across every single package, 100% on core strategy & config), 100% race-detector clean (`-race`), and static security audited (`gosec`).
* **🖥️ Cross-Platform Service Manager**: Runs interactively as a CLI or installs as a native background daemon on Windows (Windows Service), Linux (`systemd`), and macOS (`launchd`).
* **📦 Zero Dependencies**: Single static binary with zero CGO or Python requirements (`CGO_ENABLED=0`).

---

## 🛠️ Quickstart

### 1. Installation

**Universal Shell Installer (Linux & macOS)**:
```bash
curl -fsSL https://raw.githubusercontent.com/dixieflatline76/nacho-flow/main/scripts/install.sh | bash
```

**Docker / Podman (Multi-Arch Distroless Container)**:
```bash
docker run -d -p 8000:8000 \
  -v $(pwd)/config.yaml:/config/config.yaml \
  ghcr.io/dixieflatline76/nacho-flow:latest
```

**Homebrew (macOS & Linux)**:
```bash
brew install dixieflatline76/nacho-flow/nacho-flow
```

**Pre-compiled Binaries**:
Download the latest signed release for Windows, Linux, or macOS from [GitHub Releases](https://github.com/dixieflatline76/nacho-flow/releases).

**Or install via Go**:
```bash
go install github.com/dixieflatline76/nacho-flow/cmd/nacho-flow@latest
```

---

### 2. Configuration (`config.yaml`)

Create a `config.yaml` in your project folder or `~/.config/nacho-flow/config.yaml`:

```yaml
port: 8000
# Inbound Client Auth (Optional: Secures gateway on LAN / 0.0.0.0)
auth_token: "sk-nacho-secret-key"

# Upstream Providers (Local GPUs, Cloud Gateways, Private Tenants)
providers:
  # Local GPU (Ollama / vLLM / llama.cpp - $0.00 cost)
  ollama:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"

  # OpenRouter Cloud Gateway
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    api_key: "ENV_OPENROUTER_API_KEY"
    headers:
      HTTP-Referer: "https://spicebox.dev"
      X-Title: "nacho-flow"

  # Langdock Enterprise / Private Tenant
  langdock:
    base_url: "https://api.langdock.com/v1"
    api_key: "ENV_LANGDOCK_API_KEY"

# Dynamic Routing Tiers (Evaluated top-to-bottom: First Match Wins)
tiers:
  # Tier 1: Concurrency & Complex Reasoning Keywords
  - name: "Cloud Reasoning"
    model: "deepseek/deepseek-r1"
    provider: "openrouter"
    when: "any(Keywords, { # in ['deadlock', 'mutex', 'race', 'concurrency', 'atomic'] })"

  # Tier 2: Multimodal Vision (Screenshots attached)
  - name: "Cloud Vision"
    model: "google/gemini-2.5-flash-lite"
    provider: "openrouter"
    when: "HasImages"

  # Tier 3: Local ROCm GPU (100% Free, Routine tasks < 16k context, auto-escalates after 2 retries)
  - name: "Local ROCm GPU"
    model: "qwen2.5-coder:14b"
    provider: "ollama"
    max_context: 16384
    when: "Tokens < 16000 && !HasImages && !HasTools && Retries < 2"
    strip_images: true

  # Tier 4: Fast Agentic Cloud (Large context >= 16k, active tools, or retry recovery)
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

### 3. Running as a Background Daemon

To make Nacho Flow run continuously in the background (starts automatically on OS boot):

```bash
# Install as a native OS service (Windows Service / systemd / launchd)
nacho-flow service install

# Start the background daemon
nacho-flow service start
```

---

### 4. Connect Your IDE & Coding Agents

Nacho Flow exposes a standard OpenAI-compatible proxy endpoint on `http://localhost:8000/v1`. Since Nacho Flow dynamically routes and rewrites model IDs turn-by-turn based on your `config.yaml` tier rules, you can use `nacho-hybrid` (or any string) as your Model ID.

#### 🦘 Roo Code & Cline (VS Code)
In **Settings** (Gear Icon $\rightarrow$ API Configuration):
* **API Provider**: `OpenAI Compatible`
* **Base URL**: `http://localhost:8000/v1` *(or `http://127.0.0.1:8000/v1`)*
* **API Key**: `sk-nacho-secret-key` *(Matches `auth_token` if configured; otherwise use any string like `sk-local`)*
* **Model ID**: `nacho-hybrid`
* Under **Custom Model Info**:
  * **Supports Images / Vision**: `Enabled`
  * **Supports Computer Use / Tools**: `Enabled`
  * **Context Window**: `128,000` tokens
  * **Max Output**: `8,192` tokens

#### 🖱️ Cursor
In **Cursor Settings** $\rightarrow$ **Models**:
* **OpenAI API Base URL**: `http://localhost:8000/v1`
* **OpenAI API Key**: `sk-nacho-secret-key` *(or any dummy string)*
* Add custom model: `nacho-hybrid`

#### 🤖 Aider
```bash
export OPENAI_API_BASE="http://127.0.0.1:8000/v1"
export OPENAI_API_KEY="sk-nacho-secret-key"
aider --model openai/nacho-hybrid
```

#### ⏩ Continue.dev
In `~/.continue/config.json`:
```json
{
  "models": [
    {
      "title": "Nacho Flow (Hybrid Local + Cloud)",
      "provider": "openai",
      "model": "nacho-hybrid",
      "apiBase": "http://127.0.0.1:8000/v1",
      "apiKey": "sk-nacho-secret-key"
    }
  ]
}
```

---

## 🗺️ Product Roadmap

See our comprehensive **[Product & Commercial Roadmap (ROADMAP.md)](ROADMAP.md)** for detailed phase milestones spanning the open-source data plane, VS Code companion extension, remote fleet protocol, and commercial SaaS control plane.

---

## 📚 Documentation

For in-depth guides, benchmark data, and architecture deep-dives:
- **[Product & Commercial Roadmap](ROADMAP.md)**: Open-source data plane, IDE extension, fleet protocol, and SaaS control plane.
- **[Architecture & System Design](docs/ARCHITECTURE.md)**: Deep dive into the pipeline, RCU concurrency model, lock-free pricing oracle, and async telemetry.
- **[Performance & Benchmarks](docs/BENCHMARKS.md)**: High-concurrency stress test results (**30,800+ req/s, 350k requests up to 1,000 workers**) on AMD Ryzen hardware.
- **[Rule & Tier Tuning Guide](docs/TUNING_GUIDE.md)**: Practical recipes for writing and optimizing `expr` routing rules.
- **[User Guide](docs/USER_GUIDE.md)**: Full configuration reference, custom `expr` tier rules, OS service setup, and IDE walkthroughs.
- **[Developer Guide](docs/DEVELOPER_GUIDE.md)**: Development prerequisites, TDD workflow, plugin extension guide, and benchmarking.
- **[Contributing](CONTRIBUTING.md)**: Guidelines for opening issues, code style, and pull requests.

---

## 📜 License

MIT License © 2026 [dixieflatline76](https://github.com/dixieflatline76) | [spicebox.dev](https://spicebox.dev)
