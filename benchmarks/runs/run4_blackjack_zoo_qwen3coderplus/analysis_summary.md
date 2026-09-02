# Benchmark Run 4: Blackjack Casino Engine — Zoo Code + Qwen3 Coder Plus Tier

## Run Configuration
- **Date**: 2026-09-02 (17:43 – 18:45 CEST, ~62 minutes)
- **Client**: Zoo Code (VS Code) — Context Window: 262,144 tokens
- **Nacho Flow Binary**: `0.9.0-kickstart-configurable-circuit-breaker`
- **Task**: Blackjack Casino Engine & Card Counting Probability Oracle CLI in Go
- **Workspace**: `Bench Zoo 3 - additional mid Qwen3-coder-plus tier`

## Tier Configuration
| Tier | Model | Condition |
|------|-------|-----------|
| Kickstart Escalation | `google/gemini-3.7-flash` | `SessionKickstarted && Retries < 3` |
| Tier 1: Local GPU | `gemma4:12b-it-qat` | `Tokens < 20000 && Retries < 2` |
| Tier 2: Flagship Coder | `qwen/qwen3-coder-plus` ($0.65/$3.25) | `Tokens < 100000 && Retries < 2` |
| Tier 3: Debug Workhorse | `google/gemini-3.7-flash` ($0.75/$3.75) | `Tokens < 200000 && Retries < 5` |
| Tier 4: Large Context | `google/gemini-3.1-pro-preview` ($2.00/$12.00) | `Retries < 7` |
| Tier 5: Frontier | `anthropic/claude-sonnet-5` ($2.00/$10.00) | `Retries < 9` |
| Default | `anthropic/claude-sonnet-5` | Always |

## Results Summary
- **Total Turns**: 151
- **Build**: ✅ `go build ./...` — 0 errors
- **Tests**: ✅ `go test ./...` — all pass
- **go vet**: ✅ 0 warnings
- **Coverage**: 58.9% (game), 45.3% (oracle), 87.2% (simulation), 0% (cards, ui, cmd/cli)

## Model Distribution
| Model | Turns | Cost ($) | Saved ($) | Role |
|-------|-------|----------|-----------|------|
| `gemma4:12b-it-qat` | 13 | $0.00 | $0.38 | Local GPU init & low-context |
| `qwen/qwen3-coder-plus` | 117 | $0.67 | $17.86 | **Primary coder (77% of turns)** |
| `google/gemini-3.7-flash` | 13 | $0.41 | $1.65 | Debug & retry escalation |
| `anthropic/claude-sonnet-5` | 6 | $1.93 | $0.00 | Fairy Dust tactical reviews |
| `anthropic/claude-opus-5` | 2 | $2.28 | $0.00 | Fairy Dust architecture reviews |

## Financial Summary
- **Total Tokens Processed**: 11,837,425
- **Total Cost Spent**: $7.21
- **Total Cost Saved**: $19.89
- **Savings Rate**: 73.4%
- **Fairy Dust Spend**: $4.22 (58% of total — deliberate quality checkpoints)
- **Actual Routing Spend** (excl. Fairy Dust): $3.00

## Key Observations
1. **Zero Retries on Qwen3 Coder Plus**: 117/117 Qwen turns succeeded on first attempt.
2. **Zero Kickstart Death Spirals**: New HasTestProgress signal + circuit breaker worked as designed.
3. **Prompt Caching Massive**: 120,000-token turns at ~$0.015 each due to ~120k cached tokens.
4. **Gemini Flash only needed for debugging**: 13 turns (8.6% of total) — intervened only when context exceeded 100k or on retries.
5. **Context compression happened around turn 120**: Context dropped from ~121k back to ~22k, but agent recovered cleanly and continued writing tests.

## Generated Code Structure
```
blackjack/
├── go.mod
├── cmd/cli/main.go                    # Interactive CLI entrypoint
├── internal/game/engine.go            # State machine (hit, stand, double, split, insurance, surrender)
├── internal/game/engine_test.go       # Game engine tests (58.9% coverage)
├── internal/game/rules.go             # Casino rule configurations (Vegas Strip, Atlantic City, European)
├── internal/oracle/counting.go        # Hi-Lo, KO, Omega II counting engines
├── internal/oracle/strategy.go        # Basic Strategy decision advisor
├── internal/oracle/strategy_test.go   # Oracle tests (45.3% coverage)
├── internal/simulation/engine.go      # Concurrent Monte Carlo simulation (10k+ hands)
├── internal/simulation/engine_test.go # Simulation tests (87.2% coverage)
├── pkg/cards/cards.go                 # Card, Deck, Shoe types
├── pkg/cards/cards_test.go            # Cards tests
└── pkg/ui/ascii_graphics.go           # ASCII card rendering
```

## Comparison with Run 3 (Gemini 3.7 Flash primary, no Qwen tier)
| Metric | Run 3 (Flash Primary) | Run 4 (Qwen Plus Primary) |
|--------|----------------------|---------------------------|
| Total Turns | ~120 | 151 |
| Total Cost | ~$5.50 | $7.21 |
| Savings Rate | ~65% | 73.4% |
| Primary Model Cost | $0.75/$3.75 per 1M | $0.65/$3.25 per 1M |
| Retries/Failures | Several | **Zero on Qwen** |
| Kickstart Spirals | Multiple observed | **Zero** |
| Test Coverage | Incomplete | 4/6 packages tested |

## Files in This Analysis
- `nacho_router.log` — Full Nacho Flow router log
- `nacho_traffic.jsonl` — Per-turn telemetry records
- `nacho_service.log` — Nacho Flow service startup log
- `zoo_api_conversation_history.json` — Full API message history
- `zoo_history_item.json` — Zoo Code task metadata
- `zoo_task_metadata.json` — Extended task metadata
- `zoo_ui_messages.json` — Zoo Code sidebar UI message log
- `generated_code/` — Complete generated codebase snapshot
