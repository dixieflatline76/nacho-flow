# The $26 Credit Evaporation: Why OpenRouter's "Pareto Code" Router Blew Up on Turn 23

## (And How Nacho Flow Shipped the Full 60-Turn Production Application for 15 Cents)

**An Empirical Field Case Study on Request-Level Cloud Routers vs. Agentic Lifecycle Proxies in Autonomous Software Engineering**

**Author:** [@dixieflatline76](https://github.com/dixieflatline76) · [nacho-flow](https://github.com/dixieflatline76/nacho-flow)  
**Date:** October 2026  
**Document Classification:** Technical Whitepaper & Empirical Benchmark Report  
**Target Architecture:** [Zoo Code](https://zoocode.dev) v3.88.0 on Windows 11 Pro / Go 1.26+ Toolchain  
**Dataset:** Auditable Zoo Code Task Log (`01a1345d-33fe-7288-a563-8dbd9ab670cc`) and OpenRouter Pareto Code Control Run

---

## Executive Summary: The $1.67 Single-Line Diff

In recent AI systems marketing, developer toolchains frequently promote **Cloud-Level Model Routers** (such as OpenRouter's `openrouter/pareto-code`, `openrouter/auto`, or similar heuristic routing endpoints):

> *"You don't need local gateways or complex multi-tier state machines. Just point your agent at an intelligent cloud router like Pareto Code. It automatically routes every prompt to the optimal high-percentile coding model on Artificial Analysis leaderboards."*

To test this proposition against **Nacho Flow**, we conducted a strictly controlled benchmark. We gave Zoo Code the standard multi-package Go systems challenge: build an educational, interactive Blackjack Simulator, Vegas S17 basic strategy trainer, and 10,000-round Monte Carlo CLI from scratch in Go 1.26+.

The result was an immediate, catastrophic economic failure for the cloud router:

1. **The $26.24 Credit Evaporation**: On **Turn 23**, before the agent could even implement the CLI entrypoint (`main.go`) or the simulation runner (`sim.go`), OpenRouter abruptly halted execution with an unhandled credit limit error:
   ```
   HTTP 402: This request would exceed your available credits given your current in-flight requests.
   Retry after in-flight requests settle, or add credits.
   ```
2. **The Micro-Action Cost Trap**: Because `openrouter/pareto-code` is a stateless, request-level router, it routed routine agentic actions to top-tier frontier models (secretly dumping 97.8% of requests into `Claude Fable 5.1`) without context pruning:
   - **Turn 4**: Executing `go version` cost **$0.4872 USD** (~49¢ for a toolcheck).
   - **Turn 9**: Running `go test -cover ./internal/deck` cost **$0.7337 USD** (~73¢ for a 0.2s test).
   - **Turn 15**: Running `go test -cover ./internal/game` cost **$1.2271 USD**.
   - **Turn 20**: Running `go test -cover ./internal/strategy` cost **$1.5511 USD**.
   - **Turn 22**: Fixing a **single redundant `Sprintf` line** in `render.go` cost **$1.6658 USD**!
3. **The Nacho Flow Contrast**: On the exact same prompt and harness, **Nacho Flow (Run 14)** completed all **60 turns autonomously to 100% completion**, delivering **95.0% statement coverage**, **0 data races**, and a **2.75M hands/sec Monte Carlo engine** for a total session cost of **~$0.15 USD**.

<p align="center">
  <img src="benchmarks/pareto_vs_nacho_flowchart.png" alt="Architecture Comparison: OpenRouter Pareto Code vs Nacho Flow Supervised Runtime" style="width: 100%; max-width: 1040px; height: auto;" />
  <br/>
  <em>Figure: Head-to-Head Architecture Comparison — OpenRouter Pareto Code (Halted at Turn 24) vs. Nacho Flow Supervised Runtime (100% Complete at Turn 60).</em>
</p>

---

## Visual Telemetry Evidence

### Evidence Item A: Zoo Code Harness Configuration
Zoo Code was configured with zero intermediaries, pointing directly to OpenRouter's official `openrouter/pareto-code` endpoint:

<p align="center">
  <img src="benchmarks/openrouter_pareto_zoo_settings.png" alt="Zoo Code Provider Configuration: OpenRouter Pareto Code" width="410" />
  <br/>
  <em>Figure 1: Zoo Code provider settings configuring the official <code>openrouter/pareto-code</code> router with direct API key credentials.</em>
</p>

### Evidence Item B: OpenRouter Live Activity Dashboard
Immediately after the run abruptly crashed on Turn 23, the OpenRouter Activity Dashboard revealed what the router actually did under the hood:

<p align="center">
  <img src="benchmarks/openrouter_activity_dashboard_fable.png" alt="OpenRouter Activity Dashboard: 97.8% Spend on Claude Fable 5.1" style="width: 100%; max-width: 950px; height: auto;" />
  <br/>
  <em>Figure 2: OpenRouter usage breakdown across models in the 3-hour test window. <strong>97.8% of the dollar volume ($13.10 on this workspace slice, $26.24 across the run) was routed into Claude Fable 5.1</strong>, while GLM 5.3 Flash accounted for only 2.2% ($0.294).</em>
</p>

> [!CAUTION]
> **The Pareto Illusion**: OpenRouter's router is marketed as balancing price and performance along the Pareto frontier. In an autonomous multi-turn agent loop, it did the exact opposite: it greedily routed almost **98% of the session into Anthropic's flagship `Claude Fable 5.1`**, incinerating developer balances on routine command executions (`go version`) and trivial single-line diffs.

---

## Head-to-Head Comparison Scorecard

| Metric | OpenRouter `openrouter/pareto-code` | Nacho Flow Supervised Runtime (Run 14) | Impact / Factor |
| :--- | :---: | :---: | :---: |
| **Session Status** | 💥 **ABORTED (Out of Credits)** | 🏆 **100% COMPLETE (Autonomous Pass)** | **Nacho Flow finished** |
| **Turns Completed** | **23 turns** (halted at Turn 24) | **60 turns** (full end-to-end) | ⚡ **+37 productive turns** |
| **Total Monetary Spend** | **$26.2420 USD** | **~$0.1500 USD** | 💰 **164× cheaper (99.4% savings)** |
| **Cost at Turn 23** | **$26.2420 USD** | **~$0.0810 USD** | 💸 **323× cheaper at same milestone** |
| **Cost of 1-line Code Fix** | **$1.6658 USD** (Turn 22) | **~$0.0018 USD** (Tier 1 Fast) | 📉 **925× cheaper per edit** |
| **Cost to Run `go test`** | **$1.5511 USD** (Turn 20) | **~$0.0015 USD** (Tier 1 Fast) | 📉 **1,000× cheaper per test** |
| **Active Context at Turn 23** | **83,356 uncompressed tokens** | **~48,000 normalized tokens** | 🛡️ **-35,000+ tokens stripped/turn** |
| **Statement Coverage** | Incomplete (No UI/Main/Sim) | **~95.0% Overall** | 🎯 **Full test suite passing** |
| **Race Detector (`-race`)** | Unbuilt / Untested | **PASS (0 race conditions)** | 🛡️ **Thread-safe Go code** |
| **Monte Carlo Engine** | Not reached | **2,750,000 hands/sec (3.6ms)** | 🏎️ **Production-grade performance** |

---

## Complete Auditable Turn-by-Turn Evidence Log

The following auditable telemetry was extracted directly from the task history `01a1345d-33fe-7288-a563-8dbd9ab670cc` stored in Zoo Code's local storage:

```
Task ID: 01a1345d-33fe-7288-a563-8dbd9ab670cc
Harness: Zoo Code v3.88.0
Provider: OpenRouter
Model Slug: openrouter/pareto-code
Underlying Model: Anthropic Claude Fable 5.1 (97.8% of spend)
```

| Turn | Tokens In | Tokens Out | Turn Cost | Cumulative Cost | Primary Action Taken | Failure / Interruption |
| :---: | :---: | :---: | :---: | :---: | :--- | :--- |
| **1** | 11,899 | 6,022 | **$0.8402** | $0.8402 | `newFileCreated: plans/blackjack-plan.md` | Clean |
| **2** | 18,196 | 837 | **$0.4476** | $1.2878 | `updateTodoList` | Clean |
| **3** | 20,199 | 466 | **$0.4506** | $1.7384 | `switchMode` | Clean |
| **4** | 23,734 | 125 | **$0.4872** | $2.2256 | `execute_command: go version` *(50¢ for a version check!)* | Clean |
| **5** | 25,084 | 114 | **$0.5131** | $2.7386 | `newFileCreated: go.mod` | Clean |
| **6** | 26,408 | 1,032 | **$0.6314** | $3.3700 | `newFileCreated: card.go` | Clean |
| **7** | 28,655 | 1,691 | **$0.7422** | $4.1122 | `newFileCreated: shoe.go` | Clean |
| **8** | 31,563 | 3,372 | **$0.9685** | $5.0807 | `newFileCreated: deck_test.go` | Clean |
| **9** | 36,153 | 106 | **$0.7337** | $5.8143 | `execute_command: go test -cover ./internal/deck` | Clean |
| **10** | 37,501 | 856 | **$0.8356** | $6.6499 | `updateTodoList` | Clean |
| **11** | 39,520 | 949 | **$0.8853** | $7.5352 | `newFileCreated: rules.go` | Clean |
| **12** | 41,685 | 1,433 | **$0.9770** | $8.5122 | `newFileCreated: hand.go` | Clean |
| **13** | 44,334 | 4,337 | **$1.3204** | $9.8326 | `newFileCreated: round.go` | Clean |
| **14** | 49,886 | 9,729 | **$1.9706** | $11.8032 | `newFileCreated: game_test.go` | Clean |
| **15** | 60,832 | 105 | **$1.2271** | $13.0304 | `execute_command: go test -cover ./internal/game` | Clean |
| **16** | 62,179 | 851 | **$1.3287** | $14.3591 | `updateTodoList` | Clean |
| **17** | 64,193 | 1,742 | **$1.4581** | $15.8171 | `newFileCreated: tables.go` | Clean |
| **18** | 67,153 | 1,260 | **$1.4691** | $17.2862 | `newFileCreated: strategy.go` | Clean |
| **19** | 69,632 | 6,169 | **$2.0095** | $19.2957 | `newFileCreated: strategy_test.go` | Clean |
| **20** | 77,022 | 107 | **$1.5511** | $20.8469 | `execute_command: go test -cover ./internal/strategy` | Clean |
| **21** | 78,373 | 2,183 | **$1.7858** | $22.6326 | `newFileCreated: render.go` | Clean |
| **22** | 81,772 | 304 | **$1.6658** | $24.2985 | `appliedDiff: render.go` *(**$1.67 for a 1-line Sprintf fix!**)* | Clean |
| **23** | 83,356 | 2,764 | **$1.9435** | **$26.2420** | `newFileCreated: render_test.go` | Clean |
| **24** | 0 | 0 | **$0.0000** | **$26.2420** | `thinking / planning` | 💥 **HTTP 402: Exceeded Available Credits** |

---

## Architectural Autopsy: Why Request-Level Cloud Routers Fail Agents

The fundamental design flaw of endpoints like `openrouter/pareto-code` is that they evaluate requests **statelessly in isolation**. They treat Turn 22 (a 1-line regex fix) with the exact same heuristic as Turn 1 (architecting an entire software system).

### 1. The Ignorance of Agentic Lifecycle Phases
An autonomous coding workflow consists of sharply distinct computational phases:
- **Phase A (Architecture Kickoff)**: High-dimensional reasoning, planning, structural contracts. Requires frontier reasoning.
- **Phase B (Iterative File Writing)**: Rapid, deterministic syntax emission. Requires high speed, moderate reasoning.
- **Phase C (Linter & Execution Feedback)**: Executing shell commands (`go test`, `go vet`, `git status`) and parsing return codes. Requires almost zero reasoning.
- **Phase D (Micro-Refinement)**: Modifying 1 to 5 lines of code based on compiler feedback.

Because OpenRouter's Pareto router has no awareness of these phases, it assigned **Claude Fable 5.1** to Phase C and Phase D tasks. Paying un-cached frontier token rates ($20.00/1M in, $100.00/1M out on Claude Fable 5.1) on an 80,000-token prompt just to read a 3-line terminal string is an economic disaster.

### 2. The Unchecked Context Snowball
In an unmediated agent session:
- Every file read, directory listing, compiler error, and tool definition remains in the context window forever.
- By Turn 20, Zoo Code was uploading **80+ Kilobytes of raw text on every single turn**.
- **Nacho Token Saver (NTS)** actively inspects, compresses, and normalizes conversational history before it ever touches an API socket. By stripping duplicate tool responses and redundant terminal output, NTS saved over **35,000 tokens per turn** in the later stages of the benchmark.

Without NTS, the Pareto router bills the developer for an exponential context curve:

| Turn Milestone | Task Phase | OpenRouter Pareto Code Turn Cost | Nacho Flow Supervised Turn Cost | Cost Delta |
| :---: | :--- | :---: | :---: | :---: |
| **Turn 1** | System Plan Architecture | **$0.8402** | ~$0.0300 | 28× cheaper |
| **Turn 4** | Environment check (`go version`) | **$0.4872** | ~$0.0015 | 324× cheaper |
| **Turn 8** | Deck unit tests creation | **$0.9685** | ~$0.0020 | 484× cheaper |
| **Turn 12** | Hand logic scaffolding | **$0.9770** | ~$0.0020 | 488× cheaper |
| **Turn 16** | Todo list progress check | **$1.3287** | ~$0.0015 | 885× cheaper |
| **Turn 20** | Run `go test` on strategy | **$1.5511** | ~$0.0015 | 1,034× cheaper |
| **Turn 22** | 1-line `Sprintf` format fix | **$1.6658** | ~$0.0018 | 925× cheaper |
| **Turn 23** | Render test file creation | **$1.9435** | ~$0.0025 | 777× cheaper |
| **Turn 24** | Execution blocked | 💥 **HTTP 402 OUT OF CREDITS** | — | **Nacho Flow continues** |

### 3. Lack of Autonomous Recovery Interventions
When the credit balance exhausted on Turn 24:
- OpenRouter returned a raw HTTP 402.
- Zoo Code entered an automated retry backoff loop (`10s` $\rightarrow$ `20s` $\rightarrow$ `halt`), completely stalling the developer workstation.
- In Nacho Flow, the proxy's **Thought Slap**, **Read Slap**, and **Circuit Breaker** mechanisms hold the client connection, splice recovery guidance, and throttle consumption to preserve developer autonomy.

---

## Practical Implications for Engineering Teams

1. **"Smart" Cloud Routers Are Not Agent Supervisors**: A cloud router that simply selects a model per request based on Artificial Analysis leaderboards is fundamentally inadequate for multi-turn agent harnesses. It optimizes for single-turn prompt fidelity, not multi-turn agentic economics.
2. **The True Cost of Agentic Coding is the Context Tail**: The bulk of an agent bill is never the initial prompt; it is turns 20 through 80, where 90% of the token payload is repetitive conversational debt.
3. **Local Gateways Are Essential Infrastructure**: Running an intelligent edge proxy like **Nacho Flow** is not merely a cost-saving measure—it is the difference between an agent completing its work autonomously or burning through developer credit balances and halting mid-build.

---

### Reference Artifacts
- **Auditable Evidence JSON**: [`benchmarks/pareto_evidence.json`](benchmarks/pareto_evidence.json)
- **Zoo Code Harness Config Screenshot**: [`benchmarks/openrouter_pareto_zoo_settings.png`](benchmarks/openrouter_pareto_zoo_settings.png)
- **OpenRouter Activity Dashboard Screenshot**: [`benchmarks/openrouter_activity_dashboard_fable.png`](benchmarks/openrouter_activity_dashboard_fable.png)
- **Head-to-Head Architecture Diagram**: [`benchmarks/pareto_vs_nacho_flowchart.png`](benchmarks/pareto_vs_nacho_flowchart.png)
- **Supervised Benchmark Reference**: Nacho Flow Run 14 (Zoo Code v3.88.0 + GLM-5 Supervised Pipeline)
- **Control Benchmark Reference**: OpenRouter Pareto Code Control Run (`openrouter/pareto-code` Direct)
