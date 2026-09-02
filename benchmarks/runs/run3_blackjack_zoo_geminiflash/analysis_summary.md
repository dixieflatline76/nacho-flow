# Benchmark Run 3: Go Blackjack Engine (Zoo Code on Gemini 3.7 Flash)

## 📌 Run Overview

| Parameter | Value |
| :--- | :--- |
| **Date & Time** | 2026-09-02 ~14:20 - 15:10 UTC |
| **Scenario** | Go Blackjack Engine with Strategy Tables & Full Test Suite |
| **Agent / IDE** | Zoo Code in VS Code |
| **Workspace** | `C:\Users\karlk\development\Benchmark\Bench Zoo 1` |
| **Active Preset** | `config.zoo.yaml` |
| **Primary Cloud Tier** | Tier 2: `google/gemini-3.7-flash` |
| **Benchmark Target Baseline** | `anthropic/claude-sonnet-5` ($2.00/1M input, $10.00/1M output) |

---

## 💰 Financial & Telemetry Summary

- **Total Requests / Prompt Turns**: **105 turns**
- **Total Token Volume**: **5,278,375 tokens** (~5.28M tokens)
- **Cloud API Spend (Actual)**: **`$4.59`** *(Matches OpenRouter invoice within $0.03)*
- **Estimated Direct Sonnet Baseline**: **`$19.34`**
- **Estimated Cost Saved**: **`$14.75`**
- **Cost Reduction vs Frontier Baseline**: **`76.26%`**

---

## 🛡️ Cycle Killer & Reliability Defense Metrics

- **Loop Interventions Executed**: **3 loops intercepted**
- **Stage 1 Local Heal Rate**: **100% (3/3 healed via `[SYSTEM OVERRIDE]` without escalating to expensive cloud)**
- **GPU Lockup Time Rescued**: **11.4 Minutes**
- **Avoided Runaway Tokens**: **24,000 tokens**
- **Kickstart Escalations**: **1 session rescued**
- **Fairy Dust Strategic Checkpoints**: **7 checkpoints fired**

---

## 📁 Archived Assets in this Run Folder

1. **`zoo_api_conversation_history.json`**: Full raw API conversation history between Zoo Code and Nacho Flow.
2. **`zoo_ui_messages.json`**: Interactive UI messages, prompts, tool execution records, and agent reasoning.
3. **`zoo_task_metadata.json`**: Zoo Code task tracking metadata and execution timestamps.
4. **`zoo_history_item.json`**: Zoo Code session history index item.
5. **`nacho_traffic.jsonl`**: Complete per-request structured telemetry segment from Nacho Flow gateway.
6. **`nacho_router.log`**: Detailed routing decisions, tier matches, retry tracking, and Cycle Killer interventions.
7. **`nacho_service.log`**: System service lifecycle log on NachoUbuntu.
8. **`nacho_stats_snapshot.json`**: Cumulative telemetry snapshot at the conclusion of the run.
9. **`generated_code/`**: Complete copy of generated Go files (`main.go`, `pkg/`, `plans/`, `coverage.out`).
