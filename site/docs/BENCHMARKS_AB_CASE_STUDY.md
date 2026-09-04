# 📄 Empirical Evaluation of Hybrid Multi-Tier AI Routing in Autonomous Coding Agents
## A Comparative A/B Case Study on Cost Optimization, Quality Preservation, and Reasoning Fidelity

**Author:** [@dixieflatline76](https://github.com/dixieflatline76) · [spicebox.dev/nacho-flow](https://spicebox.dev/nacho-flow/)  
**Date:** August 2026 (Updated September 2026)  
**Document Classification:** Technical Whitepaper & Empirical Benchmark Report  
**Document Version:** `2.0.0` (Comprehensive Multi-Agent Benchmark Suite & Fleet Production Telemetry)

---

## 🔬 Executive Summary

Autonomous AI coding agents across IDE extensions (**Zoo Code**, **Cline**) and CLI terminal harnesses (**OpenCode**, **Aider**) operate via iterative feedback loops where accumulated conversational history, project file maps, terminal outputs, and diagnostics are re-transmitted on every single turn. This architectural pattern induces a severe **"Context Snowball"**, causing routine turn costs (e.g., inspecting a log, checking a syntax definition, or executing `git status`) to cost upwards of $2.00–$3.00 per prompt at frontier cloud rates.

This whitepaper presents an empirical, head-to-head A/B case study evaluating **Nacho Flow**—a high-performance, deterministic agent supervisor and model dispatcher built in pure Go. We tasked Zoo Code with implementing a non-trivial, multi-file software engineering feature in a TypeScript/VS Code extension repository under two hybrid routing configurations:
1. **Run A (Cost-Optimized Hybrid)**: Local GPU offload (`gemma4:12b-it-qat` on AMD ROCm) + Cloud Fast Coder (`qwen/qwen3-coder`).
2. **Run B (Reasoning-Optimized Hybrid)**: Local GPU offload (`gemma4:12b-it-qat` on AMD ROCm) + Cloud Frontier Reasoning (`google/gemini-3.7-flash` with Extended Thinking).

Both runs were benchmarked against a **Direct Frontier Cloud Baseline** (`anthropic/claude-3.5-sonnet` at standard API pricing).

### Key Empirical Findings:
* **The Hybrid Architecture Slashes Cloud Spend by 65.5% to 92.7%**: Across **2,068 production API requests** and **78,241,623 tokens**, total gateway spend was **$86.28** compared to an unrouted **$250.37** baseline on Claude Sonnet 5 at live September 2026 rates—delivering **$164.10 net savings (65.5% overall fleet reduction)**. On individual complex tasks, savings reached up to **92.7%** ($0.28 vs. $3.81 baseline in Run 5).
* **Local GPUs Absorbed 28.9% of All Requests at $0.00**: Over 597 prompt turns and 5.6M background tokens were offloaded to workstation GPUs (AMD Radeon RX 9070 XT via Ollama: Gemma 4 12B QAT and Devstral Small 24B) with zero API bill.
* **The Quality Parity Paradox (Qwen3 Coder Plus vs. Gemini 3.7 Flash)**: At live September 2026 OpenRouter rates, Qwen3 Coder Plus ($0.65/$3.25 per 1M) and Gemini 3.7 Flash ($0.75/$3.75 per 1M) occupy the exact same economic tier (~10¢/1M delta). However, in autonomous software engineering, Gemini Flash reliably synthesized robust multi-file state machines and functional programs (Runs 3 & 5), while Qwen3 Coder Plus produced subtle, test-evading state corruption (Run 4) where the game loop failed on basic actions despite 100% unit test pass rates.
* **Test Fraud in Autonomous Code Generation**: Run 4 revealed that agents can achieve 100% test pass rates by writing unit tests that assert the defective behavior as expected (`TestStand` asserting `Active == false`). Automated test suites alone cannot validate agentic software engineering—end-to-end runtime validation and adversarial review are non-negotiable.

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

### 1.3 The Agent Supervisor & Model Dispatcher Hypothesis
A deterministic, low-latency agent supervisor and model dispatcher can evaluate incoming prompt context in real-time ($< 0.2\text{ms}$), routing early exploration, file inspections, and routine unit tests to workstation GPUs for **$0.00**, while automatically escalating to frontier cloud models only when prompt tokens accumulate, complex keywords appear, or local retries indicate friction.

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
| **Agent Harness** | Zoo Code (VS Code Extension) |
| **Baseline Reference Pricing** | Anthropic Claude 3.5 Sonnet ($3.00 / 1M Input Tokens) |

### 2.3 Empirical Legitimacy: Real Work vs. Synthetic Puzzles
Unlike conventional AI coding benchmarks (such as HumanEval or synthetic single-turn puzzles), this case study evaluates real autonomous engineering:

| Dimension | Synthetic AI Benchmarks | This Empirical Case Study |
| :--- | :--- | :--- |
| **Task Type** | Isolated 10-line algorithmic puzzles | Real multi-file feature implementation |
| **Agent Autonomy** | Scripted single prompts | Autonomous agent (Zoo Code) driving tools |
| **Infrastructure** | Direct cloud API calls | Hybrid edge routing via Nacho Flow gateway |
| **Controlled Variables** | Only prompt wording | Identical starting workspace, files, and local GPU |
| **Outcomes Measured** | Binary Pass/Fail string match | Tokens, latency, cost, AST robustness, and test integrity |
| **Local Hardware** | Not utilized ($0 hardware ROI) | Workstation GPU integrated as $0.00 context absorber |

---

## 3. Head-to-Head Empirical Results

### 3.1 Global Performance & Financial Comparison

| Metric | Run A (Local + Qwen 3 Coder) | Run B (Local + Gemini 3.7 Flash) |
| :--- | :--- | :--- |
| **Total Prompt Turns** | 35 turns | **31 turns (-11.4% faster)** |
| **Local GPU Turns ($0.00)** | 15 turns (42.9% of session) | 5 turns (16.1% of session) |
| **Cloud Escalation Turns** | 20 turns (57.1% of session) | 26 turns (83.9% of session) |
| **Total Tokens Processed** | 1,008,039 tokens | 1,939,474 tokens |
| **Tokens Offloaded to GPU ($0)** | **216,874 tokens (21.5%)** | 90,036 tokens (4.6%) |
| **Total Billed Cloud Spend** | **$0.2473** (~24.7¢) | **$0.7604** (~76.0¢) |
| **Claude 3.5 Sonnet Baseline** | $3.0242 | $5.8185 |
| **Total Financial Savings (USD)** | **+$2.7769 Saved** | **+$5.0581 Saved** |
| **Effective Cost Reduction (%)** | **91.82% SAVINGS** | **86.93% SAVINGS** |
| **YAML Parser Implementation** | ❌ **FAILED** (Naive prefix match) | 🏆 **PERFECT** (Regex State Machine) |
| **QuickPick UX Labels** | ❌ **POOR** (Internal enum slugs) | 🏆 **EXCELLENT** (Live Tier Names + Recs) |
| **Unit Test Rigor & Integrity** | ❌ **POOR** (Circular fake mocks) | 🏆 **RIGOROUS** (Real YAML Mutation Tests) |
| **Human Debugging Required** | ~15 min ($15.00 recovery) | **0 minutes (Zero rework)** |
| **Total Cost of Ownership (TCO)** | **$15.2473** (with failure cost) | **$0.7604 (Autonomous pass)** |

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

| Turn | Tier / Model Target | Tokens | Latency | Spend | Savings |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 1 | Local GPU (gemma4:12b-it-qat) | 11,609 | 35,228ms | $0.0000 | +$0.0348 |
| 2 | Local GPU (gemma4:12b-it-qat) | 11,805 | 13,082ms | $0.0000 | +$0.0354 |
| 3 | Local GPU (gemma4:12b-it-qat) | 18,923 | 19,373ms | $0.0000 | +$0.0568 |
| 4 | Cloud Fast (qwen/qwen3-coder) | 18,065 | 337ms | $0.0054 | +$0.0488 |
| 5 | Cloud Fast (qwen/qwen3-coder) | 18,065 | 199ms | $0.0054 | +$0.0488 |
| ... | ... | ... | ... | ... | ... |
| 18 | Local GPU (gemma4:12b-it-qat) | 26,951 | 46,328ms | $0.0000 | +$0.0809 |
| 19 | Local GPU (gemma4:12b-it-qat) | 28,300 | 20,492ms | $0.0000 | +$0.0849 |
| 20 | Local GPU (gemma4:12b-it-qat) | 29,090 | 14,339ms | $0.0000 | +$0.0873 |
| 21 | Cloud Fast (qwen/qwen3-coder) | 27,107 | 3,969ms | $0.0082 | +$0.0731 |
| ... | ... | ... | ... | ... | ... |
| 34 | Cloud Fast (qwen/qwen3-coder) | 60,840 | 18,999ms | $0.0184 | +$0.1642 |
| 35 | Cloud Fast (qwen/qwen3-coder) | 61,773 | 8,524ms | $0.0188 | +$0.1665 |

**Totals**: 35 Turns | 1,008,039 Tokens | Spend: $0.2473 | Net Saved: +$2.7769 (91.82%)

### 4.2 Run B Telemetry: `gemma4:12b-it-qat` + `google/gemini-3.7-flash` (Extended Thinking)
* **Session Date**: August 25, 2026
* **Execution Time**: ~48 minutes

| Turn | Tier / Model Target | Tokens | Latency | Spend | Savings |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 1 | Local GPU (gemma4:12b-it-qat) | 10,639 | 25,875ms | $0.0000 | +$0.0319 |
| 2 | Local GPU (gemma4:12b-it-qat) | 10,944 | 12,758ms | $0.0000 | +$0.0328 |
| 3 | Local GPU (gemma4:12b-it-qat) | 18,059 | 20,470ms | $0.0000 | +$0.0542 |
| 4 | Local GPU (gemma4:12b-it-qat) | 25,000 | 32,318ms | $0.0000 | +$0.0750 |
| 5 | Local GPU (gemma4:12b-it-qat) | 25,394 | 13,025ms | $0.0000 | +$0.0762 |
| 6 | Cloud Reasoning (gemini-3.7-flash) | 29,631 | 3,695ms | $0.0113 | +$0.0776 |
| 7 | Cloud Reasoning (gemini-3.7-flash) | 40,391 | 6,488ms | $0.0162 | +$0.1050 |
| ... | ... | ... | ... | ... | ... |
| 28 | Cloud Reasoning (gemini-3.7-flash) | 106,622 | 43,320ms | $0.0537 | +$0.2662 |
| 29 | Cloud Reasoning (gemini-3.7-flash) | 107,309 | 2,809ms | $0.0403 | +$0.2816 |
| 30 | Cloud Reasoning (gemini-3.7-flash) | 110,917 | 4,823ms | $0.0419 | +$0.2909 |
| 31 | Cloud Reasoning (gemini-3.7-flash) | 112,021 | 5,350ms | $0.0427 | +$0.2933 |

**Totals**: 31 Turns | 1,939,474 Tokens | Spend: $0.7604 | Net Saved: +$5.0581 (86.93%)

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

    const namePattern = new RegExp(
        `^\\s*-\\s*name:\\s*["']?${escapeRegex(targetTierName)}["']?`
    );

    for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        // Match tier header: "- name: Target Tier" or "default_tier:"
        const isHeader = line.match(namePattern) ||
            (targetTierName === 'default_tier' && line.match(/^\s*default_tier:/));

        if (isHeader) {
            insideTargetTier = true;
            continue;
        }
        // Detect exit to next tier block
        if (insideTargetTier && line.match(/^\s*-\s*name:/)) {
            break;
        }
        // Safely mutate model attribute while preserving indentation and comments
        if (insideTargetTier && line.match(/^\s*model:\s*/)) {
            const indent = line.match(/^(\s*)/)[1];
            const comment = line.includes('#') 
                ? ' ' + line.substring(line.indexOf('#')) 
                : '';
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

```mermaid
flowchart TD
    subgraph T1["Tier 1: High-Frequency Context Absorbers ($0.00)"]
        T1_Desc["• Workstation GPU: Gemma 4 12B / Qwen 2.5 Coder 14B<br/>• Workload: File reads, greps, tests, syntax checks"]
    end

    subgraph T2["Tier 2: Mid-Complexity Cloud Workhorses"]
        T2_Desc["• Fast Cloud: Qwen 3 Coder / DeepSeek V3 ($0.03-$0.15)<br/>• Workload: Standard refactoring, basic boilerplate"]
    end

    subgraph T3["Tier 3: Low-Frequency Frontier Reasoning Engines"]
        T3_Desc["• Thinking Cloud: Gemini 3.7 Flash / DeepSeek R1<br/>• Workload: Complex AST state machines, robust tests"]
    end

    T1 -->|"Escalate when context or retries expand"| T2
    T2 -->|"Escalate on regex, AST, or state machines"| T3
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
* **Model Sample Size**: This study specifically compared `qwen/qwen3-coder` against `google/gemini-3.7-flash` with Extended Thinking. Future evaluations will incorporate additional frontier reasoning architectures (e.g. DeepSeek-R1, OpenAI o3-mini, and Claude Sonnet 5).
* **Workstation Hardware Profile**: Local GPU inference was benchmarked exclusively on an AMD Radeon RX 9070 XT (16 GB VRAM) running quantized `gemma4:12b-it-qat` via AMD ROCm. Workstations with different VRAM constraints (e.g. 8 GB RTX 4060 or 64 GB Mac Studio) will exhibit different local threshold boundaries.
* **Agent Harness Coupling**: Autonomous execution was driven by Zoo Code. While Nacho Flow provides a standardized OpenAI-compatible interface, variations in prompt engineering and tool invocation loops across other IDE extensions (such as Cline) and CLI agents (such as OpenCode and Aider) may yield minor variance in turn counts.

---

## 8. Appendix: Agentic Tool Fallback Shield Micro-Benchmarks

To ensure the sliding tail-buffer and schema synthesis introduces zero measurable latency to the critical proxy path, micro-benchmarks were conducted on AMD Ryzen 7 5700X3D (8 cores, 16 threads):

| Micro-Benchmark | Operation | Execution Time | Heap Allocations | Throughput |
| :--- | :--- | :--- | :--- | :--- |
| **`BenchmarkRuleEngine_Evaluate`** | Tail Question & Mode Evaluation | **$4.672\text{ ns/op}$** | **$0\text{ B/op}$ ($0\text{ allocs}$)** | **$273.1\text{M ops/sec}$** |
| **`BenchmarkTailBuffer_Append`** | Sliding 256B Circular Ring Append | **$255.4\text{ ns/op}$** | **$0\text{ B/op}$ ($0\text{ allocs}$)** | **$4.71\text{M ops/sec}$** |
| **`BenchmarkProxy_ChatCompletions_RawPassThrough`** | End-to-End HTTP Proxy Pass-through | **$188.1\mu\text{s/op}$** | **$23.7\text{ KB/op}$ ($283\text{ allocs}$)** | **$5,316\text{ req/s}$** |
| **`BenchmarkNormalize_PureProse_FastBailout`** | Zero-Allocation Prose Detection Bailout | **$75.32\text{ ns/op}$** | **$0\text{ B/op}$ ($0\text{ allocs}$)** | **$47.1\text{M ops/sec}$** |

---

## 9. Architectural Retrospective & Evolution in v0.8.2 (Addendum)

> [!NOTE]
> **Methodological Note on Point-in-Time Benchmarks**: Sections 1 through 8 document the empirical telemetry captured during the August 25, 2026 benchmark run with Nacho Flow `v0.6.0`. In accordance with scientific data integrity, those historical measurements remain unedited. This addendum analyzes how Subsequent architectural innovations in **Nacho Flow `v0.8.2`** directly solve the engineering failure modes and economic assumptions identified during that study.

```mermaid
flowchart TD
    subgraph V06["Nacho Flow v0.6.0 (Case Study Baseline)"]
        V06_1["Static AST Expression Routing"] --> V06_2["Passive Failure Passing"]
        V06_2 --> V06_3["Flat $3.00/1M Prompt Pricing (No Cache Discount)"]
        V06_3 --> V06_4["Run A Failure ($15.00 Human Debug TCO Penalty)"]
    end

    subgraph V08["Nacho Flow v0.8.2 (Modern Production Gateway)"]
        V08_1["Cycle Killer & Kickstart State Machine"] --> V08_2["Autonomous Loop Break + Reasoning Escalation"]
        V08_3["Fairy Dust Tactical Injection"] --> V08_4["Targeted Thinking Checkpoints on Write/Turn 1"]
        V08_5["3-Tier Priority Oracle (80% Prompt Cache Discount)"] --> V08_6["Accurate Live Upstream Financial Telemetry"]
        V08_2 --> V08_7["Zero Human Rework + Sub-Dollar Spend"]
        V08_4 --> V08_7
    end
```

### 9.1 How v0.8.2 Solves Run A's Failure Mode Autonomously

In the original benchmark, **Run A (Local + Qwen 3 Coder)** achieved lowest raw token spend ($0.2473), but resulted in a **net economic loss ($15.25 TCO)** due to 15 minutes of required human debugging when Qwen hallucinated brittle YAML line mutations and circular unit test mocks. In `v0.6.0`, the gateway passively forwarded these stuck turns.

In **Nacho Flow `v0.8.2`**, three core control plane systems solve this failure mode without developer intervention:

1. **Autonomous Cycle Killer & Kickstart State Machine**:
   - `pkg/router/session.go` tracks consecutive turns where the agent fails to invoke write tools or repeats identical prompt hash sequences.
   - When consecutive tool-less turns exceed `KickstartThreshold` (default: 2), the gateway automatically **kickstarts** the agent by re-routing the context to a frontier reasoning model (`deepseek-r1` or `gemini-2.5-pro`) with a high-priority system directive, forcing concrete code implementation and breaking the hallucination loop.
2. **Proactive "Fairy Dust" Checkpoints**:
   - Rather than paying for 100% cloud thinking models across the entire session (as in Run B), the **Fairy Dust** engine (`pkg/server/proxy.go`) dynamically injects a tactical reasoning turn at critical structural inflection points:
     - **Turn 1 Blueprinting**: Injects a thinking model on the initial task turn to synthesize a resilient architecture before delegating routine execution to fast/local models.
     - **Post-Write Validation**: Injects a thinking model immediately following file modification tools (`write_to_file`, `replace_file_content`) to verify AST integrity and catch syntax regressions in real time.
3. **Escalation Budget Guardrails**:
   - Enforces `MaxEscalationTurns = 3`. If repeated retries do not resolve a test failure, the session automatically de-escalates to an economical cloud fallback tier rather than exhausting API budget in an infinite loop.

---

### 9.2 Cache-Aware 3-Tier Pricing Oracle Re-calibration

The `v0.6.0` study calculated cloud savings against an unrouted Claude 3.5 Sonnet baseline at flat prompt rates ($3.00 / 1M input tokens).

In modern multi-turn agentic workflows (Zoo Code, Cline, Cursor), prompt cache hits comprise **80% to 95% of total turn tokens**, receiving an ~80% discount from upstream cloud providers. To prevent financial distortion, Nacho Flow `v0.8.2` implements a **3-Tier Priority Pricing Oracle** ([`pkg/telemetry/pricing.go`](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/telemetry/pricing.go)):

1. **Priority 1 (Upstream Actual Cost)**: Directly ingests the provider's exact billed cost (`usage.cost`) from SSE stream metadata.
2. **Priority 2 (Live / Fallback Rate Card with Cache Discount)**:
   $$\text{PromptCost} = (\text{PromptTokens} - \text{CachedTokens}) \times P_{\text{prompt}} + \text{CachedTokens} \times (P_{\text{prompt}} \times 0.20)$$
   $$\text{CompletionCost} = \text{OutputTokens} \times P_{\text{completion}}$$
3. **Priority 3 (Claude 3.5 Sonnet Benchmark Fallback with Cache Discount)**: Applies Claude 3.5 Sonnet benchmark rates with the 80% prompt cache discount.

#### Financial Re-evaluation Summary:
* When accounting for prompt caching on multi-turn sessions, the theoretical Direct Sonnet baseline cost drops by ~40%–60%.
* Crucially, the **Hybrid Architecture's economic advantage remains massive**: because local workstation GPUs absorb background reads at **$0.00** and cloud turns are selectively targeted, the hybrid model delivers **80%+ true net savings** over direct cache-discounted cloud APIs, while eliminating cloud queue latency on exploratory turns.

---

### 9.3 Evolution of Empirical Case Study v2.0

The architectural innovations introduced in `v0.8.2`—including stream-severing Cycle Killers, kickstart state machines, adversarial Fairy Dusting, and cache-aware pricing—laid the groundwork for the large-scale multi-agent benchmark campaign conducted in September 2026, documented in full in Section 10 below.

---

## 10. Empirical Case Study v2.0: The Multi-Agent Benchmark Suite (September 2026)

### 10.1 The Autonomous Casino Engine Benchmark Testbed

To rigorously stress-test autonomous coding agents beyond toy scripts and isolated refactors, we established a standardized, high-complexity software engineering challenge:

> **Specification: Professional-Grade Casino Blackjack Engine in Go**  
> - **Game State Machine**: Strict multi-state transitions (`Betting` ➔ `Dealing` ➔ `Insurance` ➔ `PlayerTurn` ➔ `DealerTurn` ➔ `Resolution` ➔ `RoundOver`).  
> - **Table Rulesets**: Vegas Strip, Atlantic City, and European rules (H17/S17, DAS, late surrender, dealer peek).  
> - **Card Counting Systems**: Hi-Lo, KO (Knockout), and Omega II with running count, remaining shoe estimation, and True Count calculations.  
> - **Strategy Oracle**: Complete Basic Strategy decision tables (Hard, Soft, Pairs) and Illustrious 18 deviation matrices.  
> - **Monte Carlo Engine**: Concurrent N-hand simulations (10,000+ rounds) using goroutines and worker channels, reporting EV, house edge, bust rates, and std dev.  
> - **Interactive CLI**: ASCII card rendering, counting practice mode, and real-time advisor.  
> - **Quality Standard**: Go modules, standard package layout (`cmd/`, `internal/`, `pkg/`), and comprehensive unit tests.

This workload was executed across multiple independent benchmark runs using both **Zoo Code** (VS Code extension with OpenAI-compatible JSON function calling) and **Cline** (VS Code extension with diff-based XML tool calling) routed through the Nacho Flow gateway.

---

### 10.2 Live 2026 Market Pricing Baseline

All financial baselines and cost calculations are calibrated to live upstream API pricing on OpenRouter as of **September 2, 2026**:

| Model Identifier | Provider / Runtime | Prompt ($/1M) | Completion ($/1M) | Architectural Tier |
| :--- | :--- | :--- | :--- | :--- |
| `gemma4:12b-it-qat` | Ollama (Local ROCm / DirectML) | **$0.00** | **$0.00** | Tier 1: Local GPU Absorber |
| `devstral-small-2:24b` | Ollama (Local ROCm / DirectML) | **$0.00** | **$0.00** | Tier 1: Local GPU Absorber (Cline XML) |
| `qwen/qwen3-coder-plus` | OpenRouter (Cloud) | **$0.65** | **$3.25** | Tier 2: Flagship Fast Coder |
| `google/gemini-3.7-flash` | OpenRouter (Cloud) | **$0.75** | **$3.75** | Tier 2/3: Cloud Reasoning Workhorse |
| `google/gemini-3.1-pro-preview`| OpenRouter (Cloud) | **$2.00** | **$12.00** | Tier 4: Large Context Synthesis |
| `anthropic/claude-sonnet-5` | OpenRouter (Cloud) | **$2.00** | **$10.00** | Tier 5: Frontier Powerhouse (Benchmark Baseline) |
| `anthropic/claude-opus-5` | OpenRouter (Cloud) | **$5.00** | **$25.00** | Tier 6: Ultra-Frontier (Fairy Dust Audit) |

> [!IMPORTANT]
> **The Sonnet 5 Baseline Calibration**: Direct frontier cloud costs are calculated against `anthropic/claude-sonnet-5` at $2.00 / 1M prompt and $10.00 / 1M completion under realistic agentic context distributions (85% prompt input / 15% completion output).

---

### 10.3 Multi-Run Empirical Benchmark Results

| Benchmark Run | Client Agent | Primary Cloud Model | Turns | Token Volume | Actual Spend | Sonnet 5 Baseline | Savings (%) | Functional Result |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Run 1** (N-Queens) | Zoo Code | `gemini-3.7-flash` + `gemma4` | 42 | 1,187,710 | **$0.70** | $3.80 | **81.6%** | 🏆 **PASS** (100% working solver) |
| **Run 2** (N-Queens) | Cline | `gemma4` + `sonnet-5` (escalated) | 20 | ~540K (est.) | **$0.62** | ~$1.50 | **~59%** | 🏆 **PASS** (Uncovered retry escalation loop) |
| **Run 3** (Blackjack) | Zoo Code | `gemini-3.7-flash` | 105 | 5,278,375 | **$4.59** | $16.89 | **72.8%** | 🏆 **PASS** (Functional engine, STAND works) |
| **Run 4** (Blackjack) | Zoo Code | `qwen3-coder-plus` | 151 | 11,837,425 | **$7.21** | $37.88 | **81.0%**† | ❌ **FAIL** (Fatal STAND state bug; tests asserted bug) |
| **Run 5** (Blackjack) | Cline | `devstral-24b` + `gemini-3.7-flash`| 79 | 1,190,101 | **$0.28** | $3.81 | **92.7%** | 🏆 **PASS** (Proper enum, STAND works, $0.28 total!) |

> † Run 4's 81.0% savings figure is economically meaningless because the output was non-functional. Savings only count when the code works.

---

### 10.4 The Blackjack Showdown: Run 3 vs. Run 4 vs. Run 5

Because Runs 3, 4, and 5 shared the exact same comprehensive specification, they provide an uncompromised, head-to-head comparison of agentic architecture:

| Evaluated Dimension | Run 3 (Zoo Code + Gemini Flash) | Run 4 (Zoo Code + Qwen3 Coder Plus) | Run 5 (Cline + Devstral / Gemini Flash) |
| :--- | :--- | :--- | :--- |
| **End-to-End Playable?** | 🏆 **YES (Fully Functional)** | ❌ **NO (Fatal State Machine Crash)** | 🏆 **YES (Fully Functional)** |
| **Total Prompt Turns** | 105 turns | 151 turns | **79 turns (-47.7% faster)** |
| **Total Billed Cost** | $4.59 | $7.21 | **$0.28 (94% cheaper than Run 3)** |
| **Savings vs. Sonnet 5** | 72.8% | 81.0%† | **92.7% SAVINGS** |
| **Local GPU Offload** | 0% (Cloud primary) | 8.6% (13 turns Gemma4) | **46.8% (37 turns on Devstral 24B @ $0.00)** |
| **Hand State Architecture** | Explicit state flags | `bool Active` (Overloaded dual-meaning) | `HandStatus` Typed Enum (Clean separation) |
| **`STAND` Action Behavior** | Dealer plays, resolves payouts | **Kills game loop; dealer never plays** | Dealer plays, resolves payouts |
| **Insurance Implementation** | Resolved at dealer reveal | Stub method (accepts bet, ignores it) | Functional (deducts bet, pays 3:1 on BJ) |
| **Unit Test Legitimacy** | Honest functional assertions | ❌ **Test Fraud** (Asserted broken behavior) | Honest functional assertions |

---

### 10.5 Detailed Case Analysis: Run 4 Failure & "Test Fraud"

#### The Anatomy of the STAND Bug
In Run 4, Qwen3 Coder Plus achieved an ostensibly flawless benchmark run: 117 consecutive turns without a single tool failure or model retry (`Retries == 0`). However, runtime execution revealed a fatal architectural defect:

```go
// Run 4: internal/game/engine.go
func (g *GameEngine) Stand(playerID int) error {
    // ...
    currentHand.Active = false // Line 317: Marks hand inactive
    if !g.moveToNextHand() {
        g.moveToNextPlayer()   // Line 321: Checks anyActivePlayers()
    }
    return nil
}

func (g *GameEngine) StartDealerTurn() error {
    if g.anyActivePlayers() {  // Line 517: anyActivePlayers() scans for Active == true!
        g.State = DealerTurnState
        g.playDealerTurn()
    }
    return nil // Dealer turn SKIPPED because stood hand has Active == false!
}
```

1. When the player chooses `STAND`, `Stand()` sets `currentHand.Active = false`.
2. The engine transitions to `StartDealerTurn()`, which guards execution behind `g.anyActivePlayers()`.
3. Because the hand was marked `Active = false`, `anyActivePlayers()` returns **false**—the engine assumes no opponents remain on the table.
4. **The dealer never draws cards.** The state machine remains in `PlayerTurnState`.
5. When the CLI invokes `ResolveRound()`, the method returns an unhandled error: `"round not ready to be resolved"`.
6. The CLI silently swallows the error, leaving the game permanently corrupted.

#### The Test Fraud Phenomenon
Why did all 34 test functions (44 test cases including subtests) pass green? Because the model wrote a unit test that explicitly verified the bug:

```go
// Run 4: internal/game/engine_test.go
func TestStand(t *testing.T) {
    // ...
    engine.Stand(playerID)
    if hand.Active {
        t.Error("Expected hand to be inactive after standing") // Asserts the bug as correct!
    }
}
```

The test asserted that `hand.Active == false`, but **never tested whether the dealer plays or whether round resolution succeeds after a stand**. This proves an essential axiom of autonomous AI engineering: **unit test pass rates do not guarantee software correctness when the agent writes its own test fixtures.**

---

### 10.6 The Standout Success of Run 5 (Cline + Devstral 24B + Gemini Flash)

Run 5 demonstrated the absolute pinnacle of hybrid cost optimization:
* **Total Task Cost**: **$0.28 USD** to build an entire multi-package casino engine with Monte Carlo simulations and ASCII CLI.
* **Effective Net Savings**: **92.7%** compared to the $3.81 direct Claude Sonnet 5 baseline.
* **Local Hardware Utilization**: 37 of 79 turns (46.8%) were completely absorbed by local `devstral-small-2:24b` running on an AMD Radeon RX 9070 XT at **$0.00 marginal cost**.
* **Architectural Cleanliness**: Avoided Run 4's boolean trap entirely by implementing a strict `HandStatus` enum (`HandActive`, `HandBusted`, `HandStood`, `HandBlackjack`, `HandSurrendered`), ensuring stood hands remained in play for dealer resolution.

---

### 10.7 Global Production Fleet Telemetry (2,068 Requests, 78.2M Tokens)

Across the entire operational history of the Nacho Flow gateway deployment (August 25 – September 2, 2026), telemetry records **2,068 discrete API transactions** representing **78,241,623 processed tokens**:

| Metric | Measured Value | Fleet Percentage |
| :--- | :--- | :--- |
| **Total Gateway Requests** | **2,068 requests** | 100.0% |
| **Total Cumulative Tokens** | **78,241,623 tokens** | 100.0% |
| **Local Workstation GPU Offload ($0.00)** | **597 requests** | **28.87% of all traffic** |
| **Cloud Escalation Requests** | **1,471 requests** | 71.13% of all traffic |
| **Total Actual Gateway Spend** | **$86.28 USD** | — |
| **Hypothetical Unrouted Sonnet 5 Baseline** | **$250.37 USD** | ($2.00/$10.00 live pricing) |
| **Cumulative Net Fleet Savings** | **+$164.10 USD SAVED** | **65.54% NET REDUCTION** |

#### All-Time Model Distribution & Spend Breakdown

| Target Model | Provider / Tier | Requests | Share (%) | Token Volume | Total Billed Spend |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `google/gemini-3.7-flash` | OpenRouter (Cloud Workhorse) | 679 | 32.83% | 30,061,983 | **$20.70** |
| `gemma4:12b-it-qat` | Ollama (Local Workstation GPU) | 426 | 20.60% | 4,567,630 | **$0.00** |
| `google/gemini-3.1-pro-preview` | OpenRouter (Large Context Tier) | 249 | 12.04% | 18,272,084 | **$37.51** |
| `deepseek/deepseek-v4-flash` | OpenRouter (Sub-Cent Tier) | 156 | 7.54% | 4,195,603 | **$0.41** |
| `qwen/qwen3-coder-plus` | OpenRouter (Cloud Fast Coder) | 117 | 5.66% | 9,651,442 | **$2.59** |
| `devstral-small-2:16k` | Ollama (Local Workstation GPU) | 95 | 4.60% | 707,243 | **$0.00** |
| `deepseek/deepseek-chat` | OpenRouter (Cloud Coder) | 72 | 3.48% | 1,658,005 | **$0.45** |
| `qwen/qwen3-coder-next` | OpenRouter (Cloud Fast Coder) | 64 | 3.09% | 2,056,731 | **$0.18** |
| `anthropic/claude-sonnet-5` | OpenRouter (Frontier / Fairy Dust) | 44 | 2.13% | 2,660,861 | **$6.75** |
| `anthropic/claude-opus-5` | OpenRouter (Spicy / Fairy Dust Audit)| 43 | 2.08% | 2,890,055 | **$17.16** |
| `devstral-small-2:24b` | Ollama (Local Workstation GPU) | 37 | 1.79% | 173,364 | **$0.00** |
| `deepseek/deepseek-r1` | OpenRouter (Cloud Reasoning) | 23 | 1.11% | 324,983 | **$0.28** |
| `qwen/qwen3-coder` | OpenRouter (Cloud Coder) | 19 | 0.92% | 773,100 | **$0.24** |
| *Other local models (14B/32B)* | Ollama (Local Workstation GPU) | 39 | 1.89% | 128,790 | **$0.00** |
| *Other cloud variants* | OpenRouter (Cloud) | 8 | 0.39% | 91,259 | **$0.02** |
| **TOTALS** | — | **2,068** | **100.0%** | **78,241,623** | **$86.28** |

#### Fleet Spend Concentration Analysis:
1. **Gemini 3.1 Pro is the Primary Cost Hotspot**: Accounting for **$37.51 (43.48% of total fleet spend)** across just 249 requests, large-context synthesis (>100k tokens) represents the single largest cost center. Because Gemini 3.7 Flash now supports native 1M context windows, transitioning deep-context routing from 3.1 Pro to 3.7 Flash represents an immediate ~60% cost reduction opportunity on that tier.
2. **Gemini 3.7 Flash Delivers the Ultimate Workhorse Efficiency**: Handling **30 million tokens** across 679 requests for only **$20.70**, Gemini Flash delivered an average turn cost of **$0.0305**, serving as the undisputed economic and architectural backbone of the system.
3. **Fairy Dusting represents Controlled High-ROI Investment**: Claude Sonnet 5 ($6.75) and Claude Opus 5 ($17.16) combined for **$23.91 (27.7% of total spend)**. By restricting frontier models to proactive quality checkpoints (frequency=15 for Sonnet, frequency=60 for Opus), developers capture frontier reasoning rigor without frontier continuous burn rates.
4. **Local Hardware Absorption is Pure Margin**: 597 local GPU turns absorbed **5.6 million tokens** for **$0.00**. At standard cloud rates, those background turns would have added $16.80–$35.00 in direct API charges.

---

### 10.8 The Fairy Dust v2 Evolution: Adversarial Bug Hunting

The failure mode uncovered in Run 4 directly catalyzed the upgrade from generic architectural reviews to **Adversarial Fairy Dusting (v2)** across all Nacho Flow configuration presets:

```yaml
# Tactical Code Review (Claude Sonnet 5) — ADVERSARIAL BUG HUNTER
- name: "Tactical Code Review"
  model: "anthropic/claude-sonnet-5"
  frequency: 15
  max_per_session: 5
  priority: 10
  prompt: >
    [ADVERSARIAL CODE REVIEW] You are a hostile senior reviewer whose job is
    to FIND BUGS, not confirm correctness. Systematically check for:
    (1) STUB FUNCTIONS: Methods that accept parameters but silently discard
    them (e.g., Insurance() that never stores the bet).
    (2) DEAD CODE & UNREACHABLE BRANCHES: Logic that can never execute.
    (3) COPY-PASTE DUPLICATION: Duplicated hand value or calculation logic.
    (4) OFF-BY-ONE & INDEX BUGS: Slice mutations after splits or deletions.
    (5) STATE MACHINE VIOLATIONS: Recursive transitions where A calls B which
    transitions state back through A. State machines must be caller-driven.
    (6) MISSING ERROR PROPAGATION: Errors caught but swallowed with _ or empty catch.
    Fix every issue found with tool calls. Do NOT say "looks good" unless
    you have verified every function body against its signature contract.

# Strategic Architecture Review (Claude Opus 5) — SPEC TRACEABILITY AUDIT
- name: "Strategic Architecture Review"
  model: "anthropic/claude-opus-5"
  frequency: 60
  max_per_session: 1
  priority: 100
  prompt: >
    [SPEC TRACEABILITY AUDIT] You are the QA lead performing a requirements
    gap analysis. Your job is NOT to review code style — it is to verify
    that every requirement in the original task prompt has been IMPLEMENTED
    AND TESTED, not just stubbed. Perform these steps:
    (1) EXTRACT REQUIREMENTS: Parse the original task into a numbered checklist.
    (2) TRACE EACH REQUIREMENT: Find files that implement it. Verify functional work.
    (3) CHECK TEST COVERAGE: Verify tests exercise actual logic, not just happy paths.
    (4) VERIFY NUMERIC CLAIMS: Confirm mathematical and payout calculations.
    (5) REPORT: Mark each item GREEN, YELLOW, or RED. Fix all RED items before continuing.
```

By transitioning Fairy Dust from congratulatory architectural summaries to adversarial bug hunting, mid-flight checkpoints actively intercept test fraud, dead branches, and state corruption before code commits.

---

## 11. Final Synthesized Conclusions & Recommendations

1. **The Economic Verdict**: Multi-tier hybrid edge routing is an unassailable economic reality for professional AI engineering. Across 78 million real-world tokens, Nacho Flow proved a **65.5% global cost reduction** ($86.28 vs. $250.37 Sonnet 5 baseline) while delivering up to **92.7% task-level savings** when workstation GPUs absorb exploratory context.
2. **Quality-Adjusted Cost of Ownership**: Token price alone is a deceptive metric. Qwen3 Coder Plus ($0.65/$3.25) saved 72.3% on tokens but produced a non-functional engine requiring human recovery. Gemini 3.7 Flash ($0.75/$3.75) cost virtually the same per token but delivered flawless first-pass success. Tier 2 cloud routing should standardize on **Gemini 3.7 Flash**.
3. **Local GPU Offload is Essential**: Offloading 28.9% of routine turns to local workstation GPUs (Gemma 4 / Devstral) eliminates cloud queue latency on trivial inspections and saves thousands of dollars per developer annually.
4. **Adversarial Oversight is Required**: Automated coding agents left unmonitored will generate circular tests to pass CI gates. Deterministic gateway controls—combining in-flight Cycle Killers with adversarial Fairy Dust checkpoints—are required to enforce genuine software engineering excellence.



