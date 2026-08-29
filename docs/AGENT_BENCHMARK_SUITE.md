# 🧪 Nacho Flow: Agentic Evaluation Benchmark Suite

This suite provides standardized, reproducible agentic benchmarks designed to evaluate and tune **Nacho Flow**'s routing intelligence, **Agent Shield** synthesis, **Universal Tool Normalizer**, and **Session Retry Escalation** across different `config.yaml` configurations.

---

## 🎯 Purpose & Methodology

When testing coding agents (e.g., **Zoo Code**, **Roo Code**, **Cline**) routed through Nacho Flow, synthetic micro-benchmarks do not capture real-world friction. This suite exercises:
1. **Architect Mode Planning**: Triggers conversational tools (`ask_followup_question`, `ask_question`) and stresses Agent Shield fallback heuristics.
2. **Multi-File Code Generation**: Stresses Tool Normalizer parsing and streaming delta transformations across modern toolchains.
3. **TDD & Test Iteration**: Compiles and runs test suites, intentionally provoking error-fix cycles that test **in-history retry detection** and **cloud escalation**.
4. **Cost & Performance Efficiency**: Measures token allocation between local models (Ollama, vLLM, LM Studio) and cloud frontier tiers (Gemini, Claude, DeepSeek).

---

## 📊 Benchmark Telemetry & Evaluation Matrix

For every benchmark run, record the following scorecard metrics:

| Metric | Description | Target |
| :--- | :--- | :--- |
| **Score** | Checkpoint Score (0 to 12) | $\ge 11/12$ |
| **Turns** | Total API request turns to completion | Minimize ($\le 25$) |
| **Local %** | Percentage of prompt/completion tokens handled locally | $\ge 60\%$ |
| **Cost ($)** | Total USD cost of cloud fallback tokens | $\le \$0.05$ |
| **Retries** | In-history retry / error recovery escalations | Tracked |
| **Shield Hits** | Number of synthetic tool calls injected by Agent Shield | $\ge 1$ in Architect mode |
| **Client Errors** | Number of `[ERROR]` prompt rejections from client | **0** |
| **Wall Clock** | Total elapsed time to green test suite | Recorded |

---

## 🏁 Track 1: Go Systems & Algorithm Benchmark

### Target Environment: Go 1.26+ / 1.27+ (Modern Toolchain & Deterministic GC)

### Use Case: Educational N-Queens Simulator & Visualizer

A compiled, concurrent Go application requiring conversational planning, multi-strategy algorithmic design (exact backtracking vs. polynomial heuristic search), and strict TDD.

### Expected Architecture
```text
nqueens/
├── cmd/nqueens/main.go
├── pkg/
│   ├── solver/
│   │   ├── strategy.go       # Solver interface
│   │   ├── backtracking.go   # Exact bitwise solver for N<=20
│   │   └── heuristic.go      # Min-conflicts / local search for N=1000+
│   ├── board/
│   │   └── board.go          # Board representation & conflict matrix
│   └── viz/
│       └── render.go         # Unicode chess queen (♛) ASCII & heatmap renderer
├── go.mod
└── README.md
```

### The Canonical Prompt

```text
Build an interactive, educational N-Queens simulator and visualizer CLI in Go (Go 1.26+) designed to teach non-computer science students the problem in a fun, delightful way.

1. Planning & Delight: In your initial plan, propose interactive features (e.g. step-by-step educational explanations, colorful Unicode ASCII chess boards with attack line heatmaps, and difficulty modes) and ask me for feedback before implementing.
2. Architecture: Implement extensible solver strategies using the Strategy pattern:
   - Classical Backtracking / Bitwise (for finding all 92 solutions at N=8 and teaching recursion)
   - Min-Conflicts / Local Search heuristic (for finding single solutions rapidly at N=1000 in under 5 seconds)
3. CLI & Modes: Provide interactive step-by-step playback, a --benchmark mode, and an educational tutorial mode.
4. Quality & TDD: Use TDD with table-driven tests, benchmark tests (testing.B), and a minimum test coverage of 95%.
```

### 12-Point Scoring Rubric

| # | Checkpoint Gate | Verification Command / Criteria | Points |
|---|---|---|:---:|
| 1 | **Architect Interaction** | Invokes `ask_followup_question` with valid `follow_up` options | 1 |
| 2 | **Go Module Setup** | Creates valid `go.mod` (Go 1.26+) with clean package hierarchy | 1 |
| 3 | **TDD Discipline** | Writes test files (`*_test.go`) before or alongside implementation | 1 |
| 4 | **Clean Compilation** | `go build ./...` compiles with zero warnings/errors | 1 |
| 5 | **Test Suite Pass** | `go test -v ./...` executes and passes 100% | 1 |
| 6 | **High Test Coverage** | `go test -cover ./...` reports $\ge 95\%$ test coverage | 1 |
| 7 | **Exact N=8 Solutions** | Solves $N=8$ finding exactly 92 distinct solutions | 1 |
| 8 | **Large N Heuristic** | Solves $N=1000$ in $< 5$ seconds using Min-Conflicts / Local Search | 1 |
| 9 | **Strategy Pattern** | Clean interface abstraction separating solvers | 1 |
| 10 | **ASCII/Unicode Rendering** | Renders board with queens (♛), coordinates, and attack lines | 1 |
| 11 | **Benchmark Suite** | Includes Go benchmark tests (`testing.B`) | 1 |
| 12 | **Documentation** | Provides `README.md` with algorithmic complexity analysis | 1 |

---

## ♠️ Track 2: TypeScript & Node.js State-Machine Benchmark

### Target Environment: Node.js 22+ / 24+ LTS & TypeScript 5.8+ (Strict ESM)

### Use Case: Professional Blackjack Casino Engine & Card Counting Oracle

An event-driven TypeScript application featuring a casino rules state machine, probabilistic card-counting analysis, and strict typing.

### Expected Architecture
```text
blackjack-engine/
├── src/
│   ├── engine/
│   │   ├── card.ts           # Card values, suits (♠♥♦♣), and rendering
│   │   ├── shoe.ts           # Multi-deck shoe (1-8 decks) with shuffle penetration
│   │   ├── hand.ts           # Soft/hard values, blackjack detection, split handling
│   │   └── game.ts           # Casino state machine (Hit/Stand/Double/Split/Surrender/Insurance)
│   ├── oracle/
│   │   ├── hilo.ts           # Running count and true count calculation
│   │   └── strategy.ts       # Mathematically optimal Basic Strategy lookup table
│   ├── ui/
│   │   └── ascii.ts          # Box-drawing ASCII cards and odds HUD
│   └── cli.ts                # Interactive terminal gameplay & simulation runner
├── tests/
│   ├── engine.test.ts        # State transitions & rule edge cases
│   ├── hilo.test.ts          # True count penetration math tests
│   └── simulation.test.ts    # 1,000+ hand Monte Carlo basic strategy validation
├── tsconfig.json             # Strict ESM TypeScript config
├── package.json
└── README.md
```

### The Canonical Prompt

```text
Build a professional Blackjack casino engine and probability oracle CLI in TypeScript and Node.js (ESM) with interactive ASCII card graphics.

1. Planning & Rules: In your initial plan, propose casino rule variants (e.g. Vegas Strip vs. Atlantic City dealer Soft 17 rules, double after split, shoe deck counts from 1 to 8) and visual card themes, and ask me for feedback before writing code.
2. State Machine & Rules Engine: Implement a robust state machine supporting all standard actions: Hit, Stand, Double Down, Split (up to 4 hands), Insurance, and Surrender.
3. Probability & Card Counting Oracle: Implement a real-time card counting assistant calculating Running Count, True Count (based on remaining shoe penetration using Hi-Lo system), and recommending basic strategy actions.
4. Quality & TDD: Use TypeScript strict mode, Vitest or Jest, and write comprehensive unit tests with >= 95% code coverage, including a simulation test verifying basic strategy win rates over 1,000 hands.
```

### 12-Point Scoring Rubric

| # | Checkpoint Gate | Verification Command / Criteria | Points |
|---|---|---|:---:|
| 1 | **Architect Interaction** | Invokes `ask_followup_question` with valid `follow_up` options | 1 |
| 2 | **TypeScript Config** | Valid `package.json` (ESM) and strict `tsconfig.json` | 1 |
| 3 | **Type Checking** | `npx tsc --noEmit` completes with zero type errors | 1 |
| 4 | **Test Suite Pass** | `npm test` runs via Vitest/Jest and passes 100% | 1 |
| 5 | **High Coverage** | `npm run test:coverage` reports $\ge 95\%$ line/branch coverage | 1 |
| 6 | **State Transitions** | Correctly handles all transitions: Split, Double, Surrender, Natural 21 | 1 |
| 7 | **Multi-Deck Shoe** | Accurate shoe deck depletion and penetration mechanics | 1 |
| 8 | **Hi-Lo Counting Math** | True Count calculation accurately adjusts for remaining decks | 1 |
| 9 | **Basic Strategy Table** | Provides correct mathematically optimal advice matrix | 1 |
| 10 | **ASCII Card Visuals** | Renders styled ASCII card boxes with Unicode suits (♠ ♥ ♦ ♣) | 1 |
| 11 | **Monte Carlo Simulation** | Includes test running $\ge 1,000$ hands verifying house edge | 1 |
| 12 | **Documentation** | Provides `README.md` explaining rules, strategy tables, and usage | 1 |

---

## 🔬 Experimental Configurations for A/B Testing

Run both benchmarks across these standard `config.yaml` matrix presets:

### Configuration Matrix

| Preset Name | Description | Key Configuration Focus |
| :--- | :--- | :--- |
| **Preset A: Hybrid + Shield (Production)** | Local-first (`qwen2.5-coder`) with `Retries < 1` rule, falling back to Cloud Flash on errors. | Tests optimal cost/performance and Shield recovery. |
| **Preset B: Local-Only (Baseline)** | 100% routed to local GPU model without cloud fallback. | Measures pure local model competence and failure modes. |
| **Preset C: Cloud Frontier (Ceiling)** | 100% routed to Gemini 2.5 Flash or Claude 3.7 Sonnet. | Establishes the theoretical maximum score and turn speed. |
| **Preset D: Shield OFF (Ablation)** | Same as Preset A, but with `agent_shield.enabled: false`. | Isolates the exact error rate and tool deadlocks caused by missing shield. |

---

## 📝 Running a Benchmark Run

1. Start Nacho Flow with debug telemetry:
   ```bash
   nacho-flow -log-level debug
   ```
2. Open a new workspace in VS Code with **Zoo Code** or **Roo Code**.
3. Select the `nacho-hybrid` model provider endpoint (`http://localhost:8000/v1`).
4. Set mode to **Architect** and paste the canonical benchmark prompt.
5. Record telemetry from Nacho Flow's output and evaluate the final 12-point rubric.
