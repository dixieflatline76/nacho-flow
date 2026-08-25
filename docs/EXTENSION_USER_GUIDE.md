# 🧩 Nacho Flow: VS Code Companion Extension Guide

The **Nacho Flow VS Code Extension** provides real-time cost visibility, visual routing inspection, local process lifecycle management, circuit breaker control, and automated configuration for autonomous coding agents (such as Roo Code, Cline, and Cursor).

---

## 🌟 Key Features

1. **Integrated Sidebar Control Hub**:
   - **One-Click Local Lifecycle**: Start, stop, restart, and view live daemon engine logs directly inside VS Code with zero command-line overhead.
   - **Seamless Mode Toggling**: Effortlessly toggle between **This Machine** (`http://127.0.0.1:8000`) and **Remote Server** (LAN or Tailscale URL with Bearer token authentication).
   - **Upstream Inference Engine Status**: Real-time status chips and model discovery for Ollama, OpenRouter, vLLM, SGLang, and llama.cpp.
   - **One-Click Agent Copy**: Instant copy buttons for **Base URL**, **API Key**, and **Model ID** formatted for immediate paste into Roo Code, Cline, or Cursor.

2. **Real-Time Analytics Dashboard (`Ctrl+Shift+P` → `Nacho Flow: Show Dashboard`)**:
   - **Cost & Token Telemetry**: Live cards showing Total Spend, Total Savings, Savings Percentage, and Total Request count.
   - **Live Route Inspector**: Inspect the last 500 LLM requests in-memory with zero disk latency. See exact routing reasons, tokens, latency, cost, and provider.
   - **Interactive Circuit Breaker Control**: View live health, error trip counters, and reset tripped circuits with one click.
   - **Hot-Reloading Configuration Editor**: Edit `config.yaml` live in VS Code with instant syntax validation and hot-reload.

3. **Status Bar & Hover Telemetry Card**:
   - High-visibility status bar item (`🌮 $X.XX svd`) with live updates.
   - Rich Markdown hover card showing engine health, uptime, total savings, and provider circuit states.

---

## 🚀 Installation & Quick Start

### 1. Install Extension
Download the `.vsix` package from GitHub Releases or build from source:
```bash
code --install-extension nacho-flow-0.6.0.vsix
```

### 2. Launch Local Engine or Connect Remote
Open the **Nacho Flow** icon in the VS Code Activity Bar (Left Sidebar):
- **Local Mode (Default)**: Click **`▶ Start`** to launch the bundled native Nacho Flow binary.
- **Remote Mode**: Select **Remote Server**, enter your daemon URL (e.g. `http://192.168.0.205:8000`), optional Bearer Auth Token, and click **💾 Save Remote Server**.

### 3. Configure Your Coding Agent
Under **3. Coding Agents** in the sidebar:
- Click **📋 Copy** next to `Base URL` (`http://127.0.0.1:8000/v1` or remote).
- Click **📋 Copy** next to `API Key`.
- Click **📋 Copy** next to `Model ID` (`nacho-hybrid`).
- Paste into **Roo Code**, **Cline**, or **Cursor** settings under OpenAI Compatible.

---

## 🛠️ Maintenance & Quick Actions

| Sidebar Action | Command Palette | Description |
| :--- | :--- | :--- |
| **Open Full Analytics Dashboard** | `Nacho Flow: Show Dashboard` | Opens the full-page webview dashboard with charts and routes. |
| **Recalculate Stats from Logs** | `Nacho Flow: Refresh Statistics` | Recomputes historical cost savings from server logs. |
| **Reset Circuit Breakers** | `Nacho Flow: Reset Circuit Breaker` | Closes tripped provider circuits to restore traffic. |
| **Reset All Stats to $0.00** | — | Resets cumulative token and cost counters back to zero. |
| **Help & User Guide** | `Nacho Flow: Open User Guide & Documentation` | Opens the online User Guide at spicebox.dev. |
| **Support & Community** | `Nacho Flow: Open Support & Community` | Opens the Nacho Flow Support Portal. |
