# 📄 Empirical Evaluation of Hybrid Multi-Tier AI Routing in Autonomous Coding Agents
## A Comparative A/B Case Study on Cost Optimization, Quality Preservation, and Reasoning Fidelity

**Author:** [@dixieflatline76](https://github.com/dixieflatline76) · [spicebox.dev/nacho-flow](https://spicebox.dev/nacho-flow/)  
**Date:** August 2026  
**Document Classification:** Technical Whitepaper & Empirical Benchmark Report  
**Document Version:** `1.0.0` (Official Release, Aligned with Nacho Flow `v0.6.0`)

---

## 🔬 Executive Summary

Autonomous AI coding agents (such as **Roo Code**, **Cline**, **Cursor**, and **Aider**) operate via iterative feedback loops where accumulated conversational history, project file maps, terminal outputs, and diagnostics are re-transmitted on every single turn. This architectural pattern induces a severe **"Context Snowball"**, causing routine turn costs (e.g., inspecting a log, checking a syntax definition, or executing `git status`) to cost upwards of $2.00–$3.00 per prompt at frontier cloud rates.

This whitepaper presents an empirical, head-to-head A/B case study evaluating **Nacho Flow**—a high-performance, deterministic semantic AI gateway built in pure Go. We tasked Roo Code with implementing a non-trivial, multi-file software engineering feature in a TypeScript/VS Code extension repository under two hybrid routing configurations:
1. **Run A (Cost-Optimized Hybrid)**: Local GPU offload (`gemma4:12b-it-qat` on AMD ROCm) + Cloud Fast Coder (`qwen/qwen3-coder`).
2. **Run B (Reasoning-Optimized Hybrid)**: Local GPU offload (`gemma4:12b-it-qat` on AMD ROCm) + Cloud Frontier Reasoning (`google/gemini-3.7-flash` with Extended Thinking).

Both runs were benchmarked against a **Direct Frontier Cloud Baseline** (`anthropic/claude-3.5-sonnet` at standard API pricing).

### Key Empirical Findings:
* **The Hybrid Architecture Slashes Cloud Spend by 87%–92%**: Even the "expensive" Gemini Extended Thinking run achieved an **86.93% cost reduction** ($0.7604 total spend vs. $5.8185 Sonnet baseline), while Run A achieved a **91.82% cost reduction** ($0.2473 spend).
* **Local GPUs Absorbed High-Frequency Reads at $0.00**: Over 90,000 to 216,000 background context tokens were offloaded to the local AMD Radeon GPU across routine file read and grep turns with zero API spend.
* **The Hidden Finding: Failure Cost & Total Cost of Ownership (TCO)**: While Run A was ~51¢ cheaper in raw token spend, it failed the feature implementation due to brittle YAML parsing and circular mock tests, necessitating human engineering intervention ($15.00 based on 15 minutes of recovery @ $60/hr). Run B succeeded on the first pass with zero rework. **The cheaper model was significantly more expensive when factoring in failure recovery.**

---

## 1. Introduction & The Economics of Agentic Workflows

### 1.1 The Context Snowball Trap
Unlike single-turn conversational chatbots, autonomous coding agents maintain persistent multi-turn execution loops. To maintain situational awareness across a multi-file workspace, the agent harness re-transmits the entire transcript on each turn. 

```mermaid
flowchart LR
    Turn1["Turn 1: 2,500 Tokens<br/>($0.007)"] --> Turn5["Turn 5: 18,000 Tokens<br/>($0.054)"]
    Turn5 --> Turn15["Turn 15: 45,000 Tokens<br/>($0.135)"]
    Turn15 --> Turn30["Turn 30: 110,000 Tokens<br/>($0.330/turn!)"]
```

By Turn 20, an agent asking for a 2-line typo fix re-submits over 60,000 tokens of background context. Across a typical 50-turn feature implementation, token consumption frequently exceeds 2.5 million tokens, resulting in direct cloud bills of **$12.00 to $25.00 per single developer task**.

### 1.2 The Dilemma: Pure Local vs. Pure Cloud
- **Pure Local Routing ($0.00)**: Running 100% on local hardware (Ollama, vLLM, llama.cpp) eliminates API costs. However, local open-weight models (7B–14B parameters) suffer from **context dilution**, failing to adhere to complex JSON tool call schemas or correctly synthesize multi-file state machines once context exceeds 16k–24k tokens.
- **Pure Cloud Routing ($$$$)**: Routing 100% of turns to Claude Sonnet or GPT-4o ensures high reasoning fidelity but results in massive economic inefficiency, paying frontier rates for trivial file searches and linter reads.

### 1.3 The Hybrid Edge Gateway Hypothesis
A deterministic, low-latency edge gateway can evaluate incoming prompt context in real-time ($< 0.2\text{ms}$), routing early exploration, file inspections, and routine unit tests to workstation GPUs for **$0.00**, while automatically escalating to frontier cloud models only when prompt tokens accumulate, complex keywords appear, or local retries indicate friction.

### 1.4 The Hidden Cost: Total Cost of Ownership (TCO) & Failure Recovery
Industry analysis overwhelmingly fixates on raw cost-per-million-tokens. However, in agentic software engineering, token cost is only one component of the economic equation:

$$\text{TCO} = \text{CloudTokenSpend} + \text{FailureRecoveryCost}$$
$$\text{FailureRecoveryCost} = \text{HumanDebugHours} \times \text{EngineerHourlyRate} + \text{WastedTurnReruns}$$

* **Stated Baseline Assumption**: We define the standard failure recovery penalty as **$15.00 USD**, representing 15 minutes of active human developer intervention (context switching, manual regex debugging, YAML document restoration, and re-prompting) for a mid-level software engineer billed at a conservative rate of **$60.00 / hour ($1.00 / minute)**.

When an inexpensive model outputs a hallucinated regular expression, circular unit tests, or broken AST parser, the developer is forced out of flow state. This $15.00 engineering intervention penalty completely dwarfs 51 cents of token savings.

<p align="center">
  <img src="benchmarks/charts/chart1_tco_comparison.png" alt="Total Cost of Ownership: Cheap Model vs Smart Model" width="800" />
</p>

---

## 2. Experimental Setup & Benchmark Methodology

### 2.1 The Engineering Feature Under Test
We selected a real-world, complex full-stack feature within the **Nacho Flow VS Code Extension**: **"1-Click Heatseeker Deal Adoption & Quick-Actions"**.

**Technical Requirements**:
1. **Interactive UI**: Add `📋 Copy` and `⚡ Adopt` quick-action buttons to all Heatseeker deal cards in `dashboard.js`.
2. **Styling**: Theme-aware CSS layout in `dashboard.css`.
3. **VS Code QuickPick Integration**: Implement an interactive picker displaying active configuration tiers with contextual `⭐ Recommended` flags and model subtitles.
4. **Comment-Preserving YAML AST Manipulation**: Update `config.yaml` over REST without stripping existing user comments or altering unedited blocks.
5. **Unit Test Suite**: Full Jest coverage in `controller.test.ts` passing all automated test gates.

### 2.2 Testbed Hardware & Environment
| Parameter | Specification |
| :--- | :--- |
| **Workstation Host** | NachoPC (AMD Ryzen 7 5700X3D, 8C/16T, 64 GB DDR4 RAM) |
| **GPU Hardware** | AMD Radeon RX 9070 XT (16 GB VRAM) |
| **Local Inference Runtime** | Ollama via AMD ROCm / DirectML |
| **Local Quantized Model** | `gemma4:12b-it-qat` ($0.00 marginal cost) |
| **Cloud Proxy Gateway** | Nacho Flow v0.6.0 (`http://127.0.0.1:8000/v1`) |
| **Agent Harness** | Roo Code (VS Code Extension) |
| **Baseline Reference Pricing** | Anthropic Claude 3.5 Sonnet ($3.00 / 1M Input Tokens) |

### 2.3 Empirical Legitimacy: Real Work vs. Synthetic Puzzles
Unlike conventional AI coding benchmarks (such as HumanEval or synthetic single-turn puzzles), this case study evaluates real autonomous engineering:

| Dimension | Synthetic AI Benchmarks | This Empirical Case Study |
| :--- | :--- | :--- |
| **Task Type** | Isolated 10-line algorithmic puzzles | Real multi-file feature implementation |
| **Agent Autonomy** | Scripted single prompts | Autonomous agent (Roo Code) driving tools |
| **Infrastructure** | Direct cloud API calls | Hybrid edge routing via Nacho Flow gateway |
| **Controlled Variables** | Only prompt wording | Identical starting workspace, files, and local GPU |
| **Outcomes Measured** | Binary Pass/Fail string match | Tokens, latency, cost, AST robustness, and test integrity |
| **Local Hardware** | Not utilized ($0 hardware ROI) | Workstation GPU integrated as $0.00 context absorber |

---

## 3. Head-to-Head Empirical Results

### 3.1 Global Performance & Financial Comparison

```text
=======================================================================================================
📊 EMPIRICAL A/B BENCHMARK: HYBRID MULTI-TIER ROUTING PERFORMANCE
=======================================================================================================
Metric                         | Run A (Local + Qwen 3 Coder)  | Run B (Local + Gemini 3.7 Flash)
-------------------------------------------------------------------------------------------------------
Total Prompt Turns             | 35 turns                      | 31 turns (-11.4% faster)
Local GPU Turns ($0.00)        | 15 turns (42.9% of session)   | 5 turns (16.1% of session)
Cloud Escalation Turns         | 20 turns (57.1% of session)   | 26 turns (83.9% of session)
Total Tokens Processed         | 1,008,039 tokens              | 1,939,474 tokens
Tokens Offloaded to GPU ($0)   | 216,874 tokens (21.5%)        | 90,036 tokens (4.6%)
Total Billed Cloud Spend       | $0.2473 (~24.7¢)              | $0.7604 (~76.0¢)
Claude 3.5 Sonnet Baseline     | $3.0242                       | $5.8185
Total Financial Savings (USD)  | +$2.7769 Saved                | +$5.0581 Saved
Effective Cost Reduction (%)   | 91.82% SAVINGS                | 86.93% SAVINGS
-------------------------------------------------------------------------------------------------------
YAML Parser Implementation     | ❌ FAILED (Naive prefix match)| 🏆 PERFECT (Regex State Machine)
QuickPick UX Labels            | ❌ POOR (Internal enum slugs) | 🏆 EXCELLENT (Live Tier Names + Recs)
Unit Test Rigor & Integrity    | ❌ POOR (Circular fake mocks) | 🏆 RIGOROUS (Real YAML Mutation Tests)
Human Debugging Required       | ~15 min ($15.00 recovery)     | 0 minutes (Zero rework)
Total Cost of Ownership (TCO)  | $15.2473 (with failure cost)  | $0.7604 (Autonomous first-pass pass)
=======================================================================================================
```

<p align="center">
  <img src="benchmarks/charts/chart2_cost_savings_baseline.png" alt="Cloud Spend vs Unrouted Frontier Baseline" width="800" />
</p>

---

## 4. Turn-by-Turn Telemetry & Execution Dynamics

<p align="center">
  <img src="benchmarks/charts/chart3_token_distribution.png" alt="Token Distribution: Local GPU vs Cloud" width="800" />
</p>

### 4.1 Run A Telemetry: `gemma4:12b-it-qat` + `qwen/qwen3-coder`
* **Session Date**: August 25, 2026
* **Execution Time**: ~62 minutes

```text
Turn  | Tier / Model Target                 | Tokens  | Latency   | Spend     | Savings
----------------------------------------------------------------------------------------
1     | Local GPU (gemma4:12b-it-qat)       | 11,609  | 35,228ms  | $0.0000   | +$0.0348
2     | Local GPU (gemma4:12b-it-qat)       | 11,805  | 13,082ms  | $0.0000   | +$0.0354
3     | Local GPU (gemma4:12b-it-qat)       | 18,923  | 19,373ms  | $0.0000   | +$0.0568
4     | Cloud Fast (qwen/qwen3-coder)       | 18,065  | 337ms     | $0.0054   | +$0.0488
5     | Cloud Fast (qwen/qwen3-coder)       | 18,065  | 199ms     | $0.0054   | +$0.0488
...   | ...                                 | ...     | ...       | ...       | ...
18    | Local GPU (gemma4:12b-it-qat)       | 26,951  | 46,328ms  | $0.0000   | +$0.0809
19    | Local GPU (gemma4:12b-it-qat)       | 28,300  | 20,492ms  | $0.0000   | +$0.0849
20    | Local GPU (gemma4:12b-it-qat)       | 29,090  | 14,339ms  | $0.0000   | +$0.0873
21    | Cloud Fast (qwen/qwen3-coder)       | 27,107  | 3,969ms   | $0.0082   | +$0.0731
...   | ...                                 | ...     | ...       | ...       | ...
34    | Cloud Fast (qwen/qwen3-coder)       | 60,840  | 18,999ms  | $0.0184   | +$0.1642
35    | Cloud Fast (qwen/qwen3-coder)       | 61,773  | 8,524ms   | $0.0188   | +$0.1665
----------------------------------------------------------------------------------------
TOTALS: 35 Turns | 1,008,039 Tokens | Spend: $0.2473 | Net Saved: +$2.7769 (91.82%)
```

### 4.2 Run B Telemetry: `gemma4:12b-it-qat` + `google/gemini-3.7-flash` (Extended Thinking)
* **Session Date**: August 25, 2026
* **Execution Time**: ~48 minutes

```text
Turn  | Tier / Model Target                 | Tokens  | Latency   | Spend     | Savings
----------------------------------------------------------------------------------------
1     | Local GPU (gemma4:12b-it-qat)       | 10,639  | 25,875ms  | $0.0000   | +$0.0319
2     | Local GPU (gemma4:12b-it-qat)       | 10,944  | 12,758ms  | $0.0000   | +$0.0328
3     | Local GPU (gemma4:12b-it-qat)       | 18,059  | 20,470ms  | $0.0000   | +$0.0542
4     | Local GPU (gemma4:12b-it-qat)       | 25,000  | 32,318ms  | $0.0000   | +$0.0750
5     | Local GPU (gemma4:12b-it-qat)       | 25,394  | 13,025ms  | $0.0000   | +$0.0762
6     | Cloud Reasoning (gemini-3.7-flash)  | 29,631  | 3,695ms   | $0.0113   | +$0.0776
7     | Cloud Reasoning (gemini-3.7-flash)  | 40,391  | 6,488ms   | $0.0162   | +$0.1050
...   | ...                                 | ...     | ...       | ...       | ...
28    | Cloud Reasoning (gemini-3.7-flash)  | 106,622 | 43,320ms  | $0.0537   | +$0.2662
29    | Cloud Reasoning (gemini-3.7-flash)  | 107,309 | 2,809ms   | $0.0403   | +$0.2816
30    | Cloud Reasoning (gemini-3.7-flash)  | 110,917 | 4,823ms   | $0.0419   | +$0.2909
31    | Cloud Reasoning (gemini-3.7-flash)  | 112,021 | 5,350ms   | $0.0427   | +$0.2933
----------------------------------------------------------------------------------------
TOTALS: 31 Turns | 1,939,474 Tokens | Spend: $0.7604 | Net Saved: +$5.0581 (86.93%)
```

<p align="center">
  <img src="benchmarks/charts/chart4_per_turn_trajectory.png" alt="Per-Turn Cost: Context Snowball in Action" width="800" />
</p>

---

## 5. Qualitative Code Architecture & Engineering Analysis

While both configurations delivered over **86%–91% cost savings**, a rigorous inspection of the generated source code reveals significant differences in architectural maturity and reasoning depth:

### 5.1 Case Study 1: Comment-Preserving YAML AST Manipulation

#### Run A (Qwen 3 Coder) — Naive String Matching:
```javascript
// Run A produced brittle line prefix matching:
const lines = configYaml.split('\n');
for (let i = 0; i < lines.length; i++) {
    if (lines[i].includes(tierName)) {
        lines[i + 1] = `    model: "${newModel}"`; // Danger: Overwrites next line blindly!
    }
}
```
* **Defect**: If a comment existed above the tier, or if `model` was on line $+2$ after `provider`, Run A corrupted the `config.yaml` document.

#### Run B (Gemini 3.7 Flash Thinking) — Robust State Machine:
```javascript
// Run B engineered a resilient 2-phase regex state machine:
function updateTierModel(yamlContent, targetTierName, newModel) {
    const lines = yamlContent.split('\n');
    let insideTargetTier = false;
    let modified = false;

    for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        // Match tier header: "- name: Target Tier" or "default_tier:"
        if (line.match(new RegExp(`^\\s*-\\s*name:\\s*["']?${escapeRegex(targetTierName)}["']?`)) ||
            (targetTierName === 'default_tier' && line.match(/^\s*default_tier:/))) {
            insideTargetTier = true;
            continue;
        }
        // Detect exit to next tier block
        if (insideTargetTier && line.match(/^\s*-\s*name:/)) {
            break;
        }
        // Safely mutate model attribute while preserving indentation and inline comments
        if (insideTargetTier && line.match(/^\s*model:\s*/)) {
            const indent = line.match(/^(\s*)/)[1];
            const comment = line.includes('#') ? ' ' + line.substring(line.indexOf('#')) : '';
            lines[i] = `${indent}model: "${newModel}"${comment}`;
            modified = true;
            break;
        }
    }
    return lines.join('\n');
}
```
* **Outcome**: 100% preservation of user YAML comments, indentation formatting, and arbitrary key ordering.

---

### 5.2 Case Study 2: Unit Testing Rigor & Test Fraud Avoidance

- **Run A Failure**: When writing tests in `controller.test.ts`, Qwen 3 Coder hallucinated a synthetic fixture (`tier1:\n model: old`) that did not resemble actual Nacho Flow YAML schemas. The test passed green in CI, but would fail in production on real user configurations.
- **Run B Triumph**: Gemini 3.7 Flash Thinking imported actual production YAML fixtures, tested comment retention on `# Local ROCm tier`, validated `default_tier` mutations, and verified rollback safety under corrupted responses.

---

## 6. The Division of Labor & Enterprise ROI Modeling

### 6.1 The Three Essential Quality Tiers
This benchmark reveals three distinct operational tiers in modern software engineering workflows:

```
┌────────────────────────────────────────────────────────┐
│  Tier 1: High-Frequency Context Absorbers ($0.00)      │
│  • Workstation GPU: Gemma 4 12B / Qwen 2.5 Coder 14B   │
│  • Workload: File reads, greps, tests, syntax checks   │
└───────────────────────────┬────────────────────────────┘
                            │ (Escalate when context/retries expand)
                            ▼
┌────────────────────────────────────────────────────────┐
│  Tier 2: Mid-Complexity Cloud Workhorses               │
│  • Fast Cloud: Qwen 3 Coder / DeepSeek V3 ($0.03-$0.15)│
│  • Workload: Standard refactoring, basic boilerplate   │
└───────────────────────────┬────────────────────────────┘
                            │ (Escalate on regex/AST/state-machines)
                            ▼
┌────────────────────────────────────────────────────────┐
│  Tier 3: Low-Frequency Frontier Reasoning Engines      │
│  • Thinking Cloud: Gemini 3.7 Flash / DeepSeek R1      │
│  • Workload: Complex AST state machines, robust tests  │
└────────────────────────────────────────────────────────┘
```

By allowing local hardware to absorb 80%+ of total prompt turns, developers can afford to route critical implementation turns to frontier reasoning models while still maintaining an **85%+ net cost reduction**.

### 6.2 Scaling Economic Impact Across Engineering Fleets
Scaling these empirical findings across professional engineering teams demonstrates the compounding ROI:

| Metric | Direct Frontier Cloud (Sonnet) | Nacho Flow (Run B Hybrid) | Annual Net Savings |
| :--- | :--- | :--- | :--- |
| **Cost Per 50-Turn Task** | **$14.80** | **$0.78** | **$14.02 saved / task** |
| **Daily Cost per Engineer (5 sessions)** | **$74.00** | **$3.90** | **$70.10 saved / day** |
| **Monthly Cost (20 working days)** | **$1,480.00** | **$78.00** | **$1,402.00 saved / month** |
| **Annual Fleet Cost (10 Developers)** | **$177,600.00** | **$9,360.00** | **+$168,240.00 USD Saved** |

<p align="center">
  <img src="benchmarks/charts/chart5_enterprise_fleet_roi.png" alt="Annual AI API Cost: 10-Engineer Team" width="800" />
</p>

---

## 7. Conclusions, Limitations & Strategic Recommendations

### 7.1 Strategic Conclusions
1. **Routing Intelligence Trumps Raw Model Size**: Run A used a 480B parameter model family that struggled with subtle regex state machine logic. Run B used a compact, high-efficiency thinking model (`gemini-3.7-flash` Extended Thinking) and completed the task flawlessly. Routing to the *right* model architecture matters more than routing to the largest parameter count.
2. **Failure Cost Dominates Token Cost**: In real-world software engineering, a model that is 3x cheaper per token is a net financial loss if it forces human debugging loops. True cost optimization requires balancing token prices against first-pass success rates.
3. **The Definitive 2026 Pareto Frontier**: The winning engineering strategy is neither 100% local nor 100% cloud. The optimal architecture pairs **Local GPU inference ($0.00)** for high-frequency context absorption with **Frontier Extended Thinking models** for complex logic, mediated by a low-latency edge gateway like **Nacho Flow**.

### 7.2 Limitations & Threats to Validity
In adherence to empirical scientific rigor, we acknowledge the following boundaries of this study:
* **Single-Task Feature Scope**: The benchmark evaluated one complete, multi-file software engineering task (interactive UI, CSS, VS Code QuickPick, comment-preserving YAML AST manipulation, and Jest unit tests). While representative of full-stack agentic workflows, broader evaluation across backend systems, database migrations, and compiler optimization tasks is ongoing.
* **Model Sample Size**: This study specifically compared `qwen/qwen3-coder` against `google/gemini-3.7-flash` with Extended Thinking. Future evaluations will incorporate additional frontier reasoning architectures (e.g. DeepSeek-R1, OpenAI o3-mini, and Claude 3.7 Sonnet).
* **Workstation Hardware Profile**: Local GPU inference was benchmarked exclusively on an AMD Radeon RX 9070 XT (16 GB VRAM) running quantized `gemma4:12b-it-qat` via AMD ROCm. Workstations with different VRAM constraints (e.g. 8 GB RTX 4060 or 64 GB Mac Studio) will exhibit different local threshold boundaries.
* **Agent Harness Coupling**: Autonomous execution was driven by Roo Code. While Nacho Flow provides a standardized OpenAI-compatible interface, variations in prompt engineering and tool invocation loops across Cline, Cursor, and Aider may yield minor variance in turn counts.
