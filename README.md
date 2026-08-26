<!-- markdownlint-disable MD033 MD041 -->
<p align="center">
  <img src="images/nacho-flow-banner.png" alt="Nacho Flow" width="800" />
</p>

# 🌮 Nacho Flow

<p align="center">
  <a href="https://github.com/dixieflatline76/nacho-flow/actions/workflows/ci.yml"><img src="https://github.com/dixieflatline76/nacho-flow/actions/workflows/ci.yml/badge.svg" alt="CI Status"></a>
  <a href="https://marketplace.visualstudio.com/items?itemName=dixieflatline76.nacho-flow"><img src="https://img.shields.io/badge/VS%20Code-Companion%20Extension-blue?logo=visual-studio-code&logoColor=white" alt="VS Code Extension"></a>
  <a href="https://pkg.go.dev/github.com/dixieflatline76/nacho-flow"><img src="https://pkg.go.dev/badge/github.com/dixieflatline76/nacho-flow.svg" alt="Go Reference"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/github/go-mod/go-version/dixieflatline76/nacho-flow?logo=go&logoColor=white" alt="Go Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue.svg" alt="License: AGPL-3.0"></a>
  <a href="https://github.com/dixieflatline76/nacho-flow/releases/latest"><img src="https://img.shields.io/github/v/release/dixieflatline76/nacho-flow?color=blue&label=release" alt="Latest Release"></a>
  <a href="https://github.com/dixieflatline76/homebrew-nacho-flow"><img src="https://img.shields.io/badge/Homebrew-nacho--flow-FBB040?logo=homebrew&logoColor=white" alt="Homebrew"></a>
  <a href="https://github.com/dixieflatline76/nacho-flow/pkgs/container/nacho-flow"><img src="https://img.shields.io/badge/Docker-GHCR-2496ED?logo=docker&logoColor=white" alt="Docker GHCR"></a>
</p>

> **You just paid $2.00 to ask your AI agent to check a log file. There's a better way.**
>
> A fast, zero-dependency hybrid AI gateway that routes agent prompt turns between **local GPUs ($0.00)** and **cloud APIs (up to 94.7% cost reduction)** with 100% reasoning fidelity.

**Nacho Flow** is an OpenAI-compatible proxy built in pure Go. It sits between autonomous coding agents ([Roo Code](https://github.com/RooVetGit/Roo-Code), [Cline](https://github.com/cline/cline), [Aider](https://github.com/paul-gauthier/aider), [Cursor](https://www.cursor.com), [Continue](https://continue.dev)) and LLM backends, dynamically evaluating each turn to route between **local GPUs** ([Ollama](https://ollama.com), [vLLM](https://github.com/vllm-project/vllm), [LM Studio](https://lmstudio.ai), [llama.cpp](https://github.com/ggerganov/llama.cpp)) and **cloud endpoints** ([OpenRouter](https://openrouter.ai), [DeepSeek](https://www.deepseek.com), [Langdock](https://www.langdock.com), [Azure OpenAI](https://azure.microsoft.com/en-us/products/ai-services/openai-service)). Includes an integrated **VS Code Companion Extension** with real-time cost telemetry, visual route inspector, and one-click agent setup.

🌐 **Website & Documentation**: [spicebox.dev/nacho-flow](https://spicebox.dev/nacho-flow/)  
Part of the **[spicebox.dev](https://spicebox.dev)** developer tool suite by [@dixieflatline76](https://github.com/dixieflatline76).

---

## 🌟 The Problem: The Token Snowball Trap

### 💡 The Origin Story
I built Nacho Flow after hitting a very familiar wall: I ran out of free AI coding assistant daily tokens mid-session, topped up an OpenRouter account with €10, and watched €2.50 disappear on a single prompt turn just asking the agent to analyze the `docs/` folder in the Spice project for context.

Autonomous coding agents operate in multi-turn feedback loops. As conversations progress, agent harnesses re-send the full transcript, file contents, and execution logs with every prompt turn:
1. **The Context Snowball**: An agent session starts at 2,000 tokens. By Turn 15, it is re-transmitting 45,000+ tokens with *every single prompt*.
2. **The $2.50 Trivial Turn**: When paying cloud rates per million tokens, asking the agent to check a 1-line typo, run a linter, or execute `git status` burns $1.50–$3.00 just to process the background context.
3. **The Local Dilemma**: Running 100% locally on Ollama/vLLM is free, but smaller models hit context/reasoning ceilings on complex architectural tasks.
4. **The Hybrid Edge Solution**: Nacho Flow evaluates prompt metadata in sub-millisecond Go. Routine turns (inspections, syntax fixes, unit tests) stay on your local GPU for **$0.00**. Complex multi-file reasoning automatically escalates to Claude or DeepSeek-R1.

---

## 📊 Real-World Session Economics (65-Turn Agent Task)

| Metric | Pure Cloud (Claude Sonnet 5 direct) | Nacho Flow Hybrid (Local GPU + Cloud Escalation) | Savings / Impact |
| :--- | :--- | :--- | :--- |
| **Total Session Cost** | **$14.80** | **$0.78** | **94.7% Spend Reduction** |
| **Local GPU Turns ($0.00)** | 0 turns (0% hardware ROI) | **48 turns** (Ollama / vLLM) | Max workstation GPU utilization |
| **Cloud Escalation Turns** | 65 turns (100% paid) | **17 turns** (DeepSeek-R1 / Claude) | 100% reasoning fidelity preserved |
| **Total Tokens Billed** | 2,180,000 tokens | 410,000 tokens | **81.2% fewer tokens sent to cloud** |
| **Failover Protection** | 0ms / None (Fails on API errors) | **Automatic Circuit Breaker & 0ms failover** | Zero broken agent loops |

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

* **⚡ High-Throughput Core**: Adds < 0.18 ms (180.8 µs) routing overhead and sustains 32,000+ req/s using lock-free atomic RCU state and pooled HTTP transports.
* **🔥 "Heat Seeker" Live Model Deals & Curated Gallery**: Built-in deal scout finding flash discounts, subsidized models, and free endpoints with tier recommendations (`nacho-flow deals` / `nacho-flow heat-seek` & `GET /api/v1/deals`).
* **🏛️ 3-Tier Curated Intelligence & OTA Sync**: Pre-packages verified SWE-bench & tool reliability scores (`//go:embed models.json`) with automatic Over-The-Air GitHub semver updates.
* **🧩 Real-Time IDE Control & Live Telemetry**: Powers the official VS Code Companion Extension with zero-polling SSE live metrics, real-time cost savings graphs, active route inspection, and seamless daemon lifecycle controls.
* **🧠 Reasoning Stream Normalization (`<think>`)**: Intercepts SSE streams from DeepSeek-R1, QwQ, Qwen 2.5 (`<|im_start|>think`), and Anthropic-style models (`<thinking>`), converting reasoning tokens into `<think>...</think>` tags in real time for client UI accordions.
* **🚦 Local Provider Circuit Breaker**: Detects consecutive local connection or 5xx failures and fast-fails directly to cloud fallback tiers with 0ms dial delay.
* **🔄 Retry-Based Auto-Escalation**: Tracks session turn retries with a sliding 5-minute TTL, allowing routing rules to automatically escalate to cloud models when local attempts fail (`Retries < 2`).
* **📏 Code-Aware Adaptive Token Estimator**: Dynamically corrects token calculations for dense code diffs, markdown, and structured JSON so agent harnesses never suffer premature tier escalations or unexpected context overflows.
* **🛡️ Response Quality Validation & Delayed Headers**: Peeks initial SSE stream chunks before committing `HTTP 200` headers to enable transparent cloud failover if a local model returns an empty payload or unexpected termination.
* **📐 Model Context Window Guard (`max_context`)**: Evaluates model physical context limits with O(1) pre-guards to prevent 400 Context Length Exceeded errors.
* **🛠️ Universal Strategy-Pipeline Tool Normalizer**: Converts 8 raw tool-call format families (Hermes `<tool_call>`, Mistral `[TOOL_CALLS]`, Llama 3 `<function>`, Llama Python `<|python_tag|>`, Claude XML `<invoke>`, ReAct `Action:`, Markdown code fences, and Ollama/Qwen Bare JSON) into standard OpenAI `tool_calls` JSON structures via a modular Strategy Pipeline.
* **🔒 Inbound Gateway Client Authentication**: Secures LAN and remote endpoints with optional Bearer token authentication while preserving a public `/health` endpoint.
* **🌶️ HotSauce Directives (In-Prompt Routing)**: Splash heat onto any prompt turn to override routing (`@nacho:local`, `@nacho:cloud`, `@nacho:frontier`, `@nacho:reasoning`, `@nacho:tier="..."`, `@nacho:model="..."`) or query daemon metadata (`@nacho:help`, `@nacho:tiers`, `@nacho:status`, `@nacho:deals`) with < 7ns zero-alloc bailout, $0.00 cost, and strict fallback bypass.
* **🎯 Dynamic Expression Tiers (`expr-lang/expr`)**: Evaluates custom tier rules in `config.yaml` based on token estimates, tool calls, images, retries, and prompt keywords.
* **🖼️ Historical Image Sanitization**: Automatically strips base64 `image_url` payloads from older turns when routing to text-only models.
* **🏷️ Dynamic Version Reporting**: Exposes build version across `/health`, `/v1/health`, and CLI (`nacho-flow version`, `-v`).
* **💾 Persistent Telemetry Store**: Saves cumulative token counts and estimated cost metrics to disk (`~/.config/nacho-flow/stats.json`).
* **🧪 Engineered for Reliability**: Strictly $\ge 95.0\%\text{--}100\%$ statement test coverage across all packages (100% on strategy & config, 97.1% on router, 95.4% on daemon binary), 100% race-detector clean (`-race`), and static security audited (`gosec`).
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

Visit the official website and interactive documentation portal at **[spicebox.dev/nacho-flow](https://spicebox.dev/nacho-flow/)**.

For in-depth guides, benchmark data, and architecture deep-dives:
- **[Interactive Docs Hub](https://spicebox.dev/nacho-flow/docs.html)**: Live web documentation with interactive diagrams and search.
- **[VS Code Companion Extension Guide](docs/EXTENSION_USER_GUIDE.md)**: Sidebar control hub, status bar widget, route inspector, and agent setup.
- **[Product & Commercial Roadmap](ROADMAP.md)**: Open-source data plane, IDE extension, fleet protocol, and SaaS control plane.
- **[Architecture & System Design](docs/ARCHITECTURE.md)**: Deep dive into the pipeline, RCU concurrency model, lock-free pricing oracle, and async telemetry.
- **[Performance & Benchmarks](docs/BENCHMARKS.md)**: High-concurrency stress test results (**30,800+ req/s, 350k requests up to 1,000 workers**) on AMD Ryzen hardware.
- **[Rule & Tier Tuning Guide](docs/TUNING_GUIDE.md)**: Practical recipes for writing and optimizing `expr` routing rules.
- **[User Guide](docs/USER_GUIDE.md)**: Full configuration reference, custom `expr` tier rules, OS service setup, and IDE walkthroughs.
- **[Developer Guide](docs/DEVELOPER_GUIDE.md)**: Development prerequisites, TDD workflow, plugin extension guide, and benchmarking.
- **[Contributing](CONTRIBUTING.md)**: Guidelines for opening issues, code style, and pull requests.

---

## 📜 Licensing & Commercial Options

Nacho Flow is available under a **Dual-Licensing Model**:

1. **Free & Open-Source (GNU AGPL-3.0 with Additional Use Grant)**:
   * 100% free for individual developers, open-source projects, and **internal enterprise self-hosting** (see our [Enterprise Safe Harbor in LICENSE](LICENSE)).
   * If you distribute or host a modified version as a public cloud service to third parties, you must make your complete source code available under AGPL-3.0.
2. **Spicebox Commercial & Enterprise OEM License**:
   * For organizations embedding Nacho Flow into closed-source proprietary software, commercial SaaS platforms, or requiring custom legal indemnification and priority support SLAs.
   * See **[COMMERCIAL_LICENSE.md](COMMERCIAL_LICENSE.md)** or contact [`karl@spicebox.dev`](mailto:karl@spicebox.dev).

Contributions are accepted under our **[Contributor License Agreement (.github/CLA.md)](.github/CLA.md)**.

---

Copyright © 2026 [Karl Kwong / Spicebox](https://spicebox.dev) · Licensed under **GNU AGPL-3.0** with Commercial Dual-Licensing.
