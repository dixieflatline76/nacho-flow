# 🌮 Nacho Flow (`spicerack.dev/nacho-flow`)

> **Slash your monthly AI coding bills by 90–95% without sacrificing model intelligence.**

**Nacho Flow** is a high-performance, zero-dependency OpenAI-compatible hybrid AI gateway built in pure Go. It automatically routes agent prompts between your **local GPU** (Ollama / vLLM / ROCm for $0.00) and **ultra-cheap cloud APIs** (OpenRouter / DeepSeek / Gemini) on a turn-by-turn basis.

Part of the **[spicerack.dev](https://spicerack.dev)** developer tool suite by [@dixieflatline76](https://github.com/dixieflatline76).

---

## 🌟 Why Nacho Flow?

Autonomous coding agents (Roo Code, Cline, Aider) dump full conversation histories into every prompt turn. After 10 turns, context hits 100k+ tokens—costing **$2.00 to $5.00 per prompt** on flagship models like Claude 3.5 Sonnet.

* **Local GPUs choke on huge history**: Local GPUs (e.g. RX 6900 XT / RTX 3090 / Mac Studio) run fast at small contexts (< 16k tokens), but run out of VRAM as history accumulates.
* **Nacho Flow solves the Agentic Context Trap**: It evaluates incoming prompts turn-by-turn. Turn 1 (small context) runs on your local GPU for **$0.00**. When history grows past 16k tokens or requests tool calls, it seamlessly hands off to cloud endpoints.

---

## ✨ Features

* **⚡ Microsecond Latency**: Built with native Go `net/http/httputil.ReverseProxy` (< 1ms proxy overhead, < 15MB RAM footprint).
* **🎯 1..N Dynamic Tiers (`expr-lang/expr`)**: Configure unlimited custom routing rules in `config.yaml` using intuitive Go-like expressions.
* **🖼️ History Image Sanitization**: Automatically strips raw base64 `image_url` payloads from older historical turns when routing to text-only models, eliminating `400 Bad Request` crashes.
* **🖥️ Cross-Platform Service Manager**: Runs interactively as a CLI OR installs natively as a background service on **Windows** (Windows Service), **Linux** (systemd), and **macOS** (launchd).
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
openrouter_key: "ENV_OPENROUTER_API_KEY"

providers:
  ollama: "http://127.0.0.1:11434/v1"
  openrouter: "https://openrouter.ai/api/v1"

tiers:
  # Tier 1 (High Precedence): Concurrency & Reasoning Keywords
  - name: "Cloud Reasoning"
    model: "deepseek/deepseek-r1"
    provider: "openrouter"
    when: "any(Keywords, { # in ['deadlock', 'mutex', 'race', 'concurrency', 'atomic'] })"

  # Tier 2: Multimodal Vision (Screenshots attached)
  - name: "Cloud Vision"
    model: "google/gemini-2.5-flash-lite"
    provider: "openrouter"
    when: "HasImages"

  # Tier 3: Local ROCm / CUDA GPU (100% Free, Routine tasks < 16k context)
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

### 4. Connect Your IDE (Roo Code / Cline / Aider)

In VS Code **Roo Code Settings**:
* **Provider**: `OpenAI Compatible`
* **Base URL**: `http://localhost:8000/v1`
* **API Key**: `sk-dummy` *(the router holds your real keys)*
* **Model ID**: `nacho-hybrid`
* **Stream / Image Support**: `ON`

---

## 📜 License

MIT License © 2026 [dixieflatline76](https://github.com/dixieflatline76) | [spicerack.dev](https://spicerack.dev)
