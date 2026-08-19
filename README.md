# 🌮 Nacho Flow (`spicebox.dev/nacho-flow`)

> **Slash your monthly AI coding bills by 90–95% without sacrificing model intelligence.**

**Nacho Flow** is an ultra-high-performance, zero-dependency OpenAI-compatible hybrid AI gateway built in pure Go. It automatically routes agent prompt turns between your **local GPU** (Ollama / vLLM / ROCm for $0.00) and **cheap cloud APIs** (OpenRouter / Langdock / DeepSeek / Azure) with **< 0.29 ms overhead** and **32,250+ requests/sec throughput**.

Part of the **[spicebox.dev](https://spicebox.dev)** developer tool suite by [@dixieflatline76](https://github.com/dixieflatline76).

---

## 🌟 Why Nacho Flow?

Autonomous coding agents (Roo Code, Cline, Aider, Cursor) dump full conversation histories into every prompt turn. After 10 turns, context hits 100k+ tokens—costing **$2.00 to $5.00 per prompt** on flagship models like Claude 3.5 Sonnet.

* **Local GPUs choke on huge history**: Local GPUs (RX 6900 XT / RTX 3090 / Mac Studio) run blindingly fast on small contexts (< 16k tokens), but run out of VRAM as history accumulates.
* **Nacho Flow solves the Agentic Context Trap**: It evaluates incoming prompts turn-by-turn. Turn 1 (small context) runs on your local GPU for **$0.00**. When history grows past 16k tokens or requests tool calls, it seamlessly hands off to cloud endpoints.

---

## ✨ Key Features

* **⚡ Wire-Speed Throughput (32,250+ req/sec)**: Built with zero-lock concurrency (`atomic.Pointer` RCU) and pooled HTTP transports (< 0.29 ms routing overhead, < 96 MB RAM under 1,000 parallel workers).
* **🔌 Composable Capability Provider Subsystem**: Supports local GPUs (Ollama, vLLM, LM Studio) and cloud endpoints (OpenRouter, Langdock, DeepSeek, Azure, AWS) with dynamic header and bearer auth injection.
* **🎯 Dynamic Expression Tiers (`expr-lang/expr`)**: Configure unlimited custom routing rules in `config.yaml` based on context size, images, tool calling, and code keywords.
* **🖼️ History Image Sanitization**: Automatically strips raw base64 `image_url` payloads from older historical turns when routing to text-only models, eliminating `400 Bad Request` crashes.
* **💾 Persistent Telemetry Store**: Automatically saves cumulative token savings and USD cost metrics to disk (`~/.config/nacho-flow/stats.json`) across daemon restarts.
* **🖥️ Cross-Platform Service Manager**: Runs interactively as a CLI OR installs natively as a persistent background daemon on **Windows** (Windows Service), **Linux** (systemd), and **macOS** (launchd).
* **📦 Zero Dependencies**: Single static binary with zero CGO or Python requirements (`CGO_ENABLED=0`).

---

## 🛠️ Quickstart

### 1. Installation

**Download Pre-compiled Binary**:
Download the latest release for Windows, Linux, or macOS from [GitHub Releases](https://github.com/dixieflatline76/nacho-flow/releases).

**Or install via Go**:
```bash
go install github.com/dixieflatline76/nacho-flow/cmd/nacho-flow@latest
```

---

### 2. Configuration (`config.yaml`)

Create a `config.yaml` in your project folder or `~/.config/nacho-flow/config.yaml`:

```yaml
port: 8000

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

  # Tier 3: Local ROCm GPU (100% Free, Routine tasks < 16k context without images or tools)
  - name: "Local ROCm GPU"
    model: "qwen2.5-coder:14b"
    provider: "ollama"
    when: "Tokens < 16000 && !HasImages && !HasTools"
    strip_images: true

  # Tier 4: Fast Agentic Cloud (Large context >= 16k or active tools)
  - name: "Cloud Agentic Fast"
    model: "qwen/qwen3-coder-30b-a3b-instruct"
    provider: "openrouter"
    when: "Tokens >= 16000 || HasTools"

default_tier:
  name: "Cloud Fallback"
  model: "~deepseek/deepseek-v4-flash-latest"
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

### 4. Connect Your IDE (Roo Code / Cline / Aider / Cursor)

In VS Code **Roo Code Settings**:
* **Provider**: `OpenAI Compatible`
* **Base URL**: `http://localhost:8000/v1`
* **API Key**: `sk-dummy` *(Nacho Flow holds your real keys)*
* **Model ID**: `nacho-hybrid`
* **Stream / Image Support**: `ON`

---

## 🗺️ Vision & Future Roadmap

* **🤖 Autonomous "Agent-on-Agent" Auto-Tuning**: A built-in optimizer agent (`nacho-flow tune`) that analyzes live routing telemetry, detects prompt retry patterns, and synthesizes/tunes `expr` rules in a closed loop.
* **🛡️ 100% Air-Gapped Enterprise Deployment**: Ultra-lightweight distroless Docker container (< 15MB) with zero vulnerabilities, deploying as a private Kubernetes pod / service mesh sidecar.
* **🔍 Data Loss Prevention (DLP) Sanitizer**: Inline secret scrubbing (API keys, credentials, PII) in `pkg/router/sanitizer.go` before prompts leave the private perimeter.

---

## 📚 Documentation

For in-depth guides, benchmark data, and architecture deep-dives:
- **[Architecture & System Design](docs/ARCHITECTURE.md)**: Deep dive into the pipeline, RCU concurrency model, lock-free pricing oracle, and async telemetry.
- **[Performance & Benchmarks](docs/BENCHMARKS.md)**: High-concurrency stress test results (**32,254+ r/s up to 1,000 workers**) on AMD Ryzen hardware.
- **[Rule & Tier Tuning Guide](docs/TUNING_GUIDE.md)**: Practical recipes for writing and optimizing `expr` routing rules.
- **[User Guide](docs/USER_GUIDE.md)**: Full configuration reference, custom `expr` tier rules, OS service setup, and IDE walkthroughs.
- **[Developer Guide](docs/DEVELOPER_GUIDE.md)**: Development prerequisites, TDD workflow, plugin extension guide, and benchmarking.
- **[Contributing](CONTRIBUTING.md)**: Guidelines for opening issues, code style, and pull requests.

---

## 📜 License

MIT License © 2026 [dixieflatline76](https://github.com/dixieflatline76) | [spicebox.dev](https://spicebox.dev)
