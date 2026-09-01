# Benchmark Run 2 Analysis: Cline Windows — N-Queens Go Simulator

**Date**: 2026-08-29 12:16:56 - 12:37:04 CEST  
**Client**: Cline (Windows 11) in Autonomous Act Mode  
**Endpoint**: `nacho-hybrid` (Nacho Flow v0.8.2 on Linux)  
**Total Requests**: 20 requests  

---

## 1. Executive Telemetry & Cost Summary

| Metric | Measured Value |
| :--- | :--- |
| **Total Turns / API Requests** | **20 requests** (stopped early) |
| **Tier 1 (Local GPU Free - Gemma 4 12B QAT)** | **8 turns (40.0%)** — Cost: **$0.00** |
| **Tier 3 (Deep Reasoner - DeepSeek R1)** | **4 turns (20.0%)** — Cost: **$0.028** |
| **Tier 4 (Frontier Powerhouse - Claude Sonnet 5)** | **8 turns (40.0%)** — Cost: **$0.592** |
| **Total Cost Spent** | **$0.62 USD** |
| **Average Sonnet Turn Cost** | **~$0.074 / request** |

---

## 2. 🚨 Root Cause: Why Cline Escalated to Claude Sonnet 5

### The Mechanism of the Bug:

1. **How Autonomous Agents Work**:
   When you send a prompt like `"Please proceed and implement"`, Cline enters an autonomous loop making 10–20 consecutive tool-calling turns (reading files, writing code, running `go test`).
   Across all 20 API requests, the *latest user message text* in the `messages` payload is always the exact same string: `"Please proceed and implement"`.

2. **The Session Tracker Flaw**:
   Nacho Flow's `sessionTracker` currently tracks retries by hashing the latest user prompt text:
   ```go
   promptHash := router.HashPrompt(reqCtx.Prompt)
   retries, isRetry := s.sessionTracker.RecordTurn(sessionKey, promptHash)
   ```
   Because the prompt text was identical for every tool step, `sessionTracker` mistakenly thought the user was failing and retrying the same prompt 9 times in a row!

3. **The Escalation Cascade**:
   * **Turn 11**: Prompt `"Please proceed and implement"` $\rightarrow$ `Retries = 0` $\rightarrow$ **Tier 1 (Local GPU)**
   * **Turn 12**: Prompt `"Please proceed and implement"` $\rightarrow$ `Retries = 1` $\rightarrow$ **Tier 3 (DeepSeek R1)**
   * **Turn 13**: Prompt `"Please proceed and implement"` $\rightarrow$ `Retries = 2` $\rightarrow$ **Fails Tier 1 & 2 & 3 $\rightarrow$ Escalates to Tier 4 (Claude Sonnet 5)**
   * **Turns 14–20**: `Retries = 3..9` $\rightarrow$ **Locked onto Claude Sonnet 5 at $3.00/$15.00 per 1M tokens**!

---

## 3. 🛠️ The Permanent Fix for Nacho Flow

1. **Do NOT Treat Normal Tool Execution as a Retry**:
   If the previous message in history was a **successful tool result** (`role: tool` or `tool_result` without error strings), it is a **normal multi-step agent turn**, NOT a user retry!
2. **In-History Error Gate**:
   `Retries` must ONLY increment when the previous turn contains an **explicit tool failure** (`[ERROR]`, `Missing value`, `<error_details>`, etc.).
3. **Result**:
   Cline will stay on **Tier 1 (Local GPU)** and **Tier 2 (Gemini 3.7 Flash)** for all normal autonomous tool turns, and ONLY escalate to Sonnet 5 if a tool actually breaks!
