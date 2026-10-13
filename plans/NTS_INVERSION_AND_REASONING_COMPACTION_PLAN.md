# 🌮 Nacho Flow: TDD NTS Ingress Timing Fix & Reasoning Compaction Plan

> **Branch**: `fix/nts-inversion-and-reasoning-compaction`  
> **Discipline**: Strict **Test-Driven Development (TDD)** — **Red -> Green -> Refactor**.  
> **Quality Gate**: **>= 95% Statement Coverage Min Bar** across all modified packages (`pkg/server`, `pkg/nts`).  
> **Performance Invariant**: **Hot path must remain strictly zero-allocation (0 B/op, 0 allocs/op)** on clean/bypass paths with zero redundant unmarshals.  
> **Directives**:
> - **Phase 1**: Fix Ingress Timing Inversion Bug (No commit; manual testing first).  
> - **Phase 2**: Implement In-Context Thinking/Reasoning Compaction with configurable retention (`keep_recent_reasoning_turns`).

---

## 1. TDD Workflow & Zero-Allocation Hot Path Standard

### 1.1 Strict TDD Lifecycle
Every unit of work across both phases strictly adheres to the TDD lifecycle:
1. **RED**: Write the failing test first, codifying expected behavior and edge cases. Confirm the test fails.
2. **GREEN**: Write the minimal code required to pass the test.
3. **REFACTOR**: Audit performance, verify zero-alloc heap constraints, and clean signatures.
4. **COVERAGE GATE**: Run `go test -coverprofile` and enforce that code coverage on the package meets or exceeds **95.0%**.

```bash
# Automated coverage gate verification command
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out | grep total
```

### 1.2 Hot Path Invariants: Wire-Speed & Zero-Allocation Constraints
In accordance with Nacho Flow's L4 systems architecture ([docs/ARCHITECTURE.md](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/docs/ARCHITECTURE.md)), the ingress and proxy hot paths must satisfy:
1. **Atomic Pointer Bailout (< 2 ns)**: When NTS is disabled, `s.GetNTSTransformer() == nil` is a single atomic pointer load (`s.state.Load()`) executing in $< 2\text{ ns}$ with **0 B/op, 0 allocs/op**.
2. **Zero-Alloc Bypass on Clean Payloads**: If a payload contains no tool outputs or reasoning to compact, `transformPayload` returns the original `body` byte slice directly (`if !modified { return body, totalResult, nil }`), bypassing `json.Marshal` entirely (**0 B/op, 0 allocs/op**).
3. **Net Allocation Neutrality**: Moving NTS from `preparePayload` to `handleCompletions` and removing it from `preparePayload` eliminates a redundant `json.Unmarshal` pass on every request and retry, keeping total ingress allocations net-neutral or lower.
4. **In-Place Mutation Invariants**: Any byte compaction operates with single-pass write cursors ($w \le r$), preventing intermediate buffer churn.
5. **Benchmark Enforcement**: Dedicated benchmark tests (`BenchmarkPipeline_Ingress_Bypass_ZeroAlloc`, `BenchmarkNTS_NoOp_ZeroAlloc`) will verify zero heap allocation regressions.

---

## 2. Phase 1: Ingress Timing Inversion Bug Fix (TDD)

### 2.1 Problem Diagnosis
In `pkg/server/pipeline.go`, `s.classifier.Classify(body)` runs on the raw incoming request body.
Then `pkg/strategy/expr_evaluator.go` evaluates tier rules (e.g., `when: "Tokens < 160000"`).
NTS is only invoked much later inside `preparePayload()` in `pkg/server/dispatch.go`.

**Consequence**: The router classifies and selects tiers using the uncompacted token estimate. Even if NTS strips tokens downstream, the request has already been escalated to an expensive tier.

### 2.2 TDD Step 1: RED — Write Failing Ingress Routing Integration Test
- **File**: `pkg/server/proxy_test.go`
- **Test**: `TestPipeline_IngressNTSCompaction_RoutesToLowerTier(t *testing.T)`
  - Setup:
    - Tier 1: Small Local Context (`when: "Tokens < 4000"`)
    - Tier 2: Cloud Fallback (`when: "true"`)
    - Enable NTS with ANSI and CR stripping.
  - Payload:
    - Tool message output with 5,000 bytes of ANSI progress bars.
    - Raw token estimate: ~4,500 tokens.
    - Post-compaction estimate: ~2,500 tokens.
  - Assertions:
    - Prior to fix, `SelectTier()` evaluates uncompacted `4500 < 4000` -> routes to Tier 2 (Test FAILS / RED).
    - `reqCtx.Tokens == 2500` (compacted size for tier routing).
    - `reqCtx.RawTokens == 4500` (uncompacted size for observability).
    - `reqCtx.NTSTokensSaved == 2000`.
    - Selected Tier MUST be `Tier 1: Small Local Context`.

### 2.3 TDD Step 2: GREEN — Implement Ingress Compaction & Signature Cleanup
1. **Contract Additions** in `pkg/contract/interfaces.go`:
   - Add `RawTokens int`, `NTSTokensSaved int`, and `NTSBytesSaved int` to `RequestContext`.
2. **Re-order Ingress in `pkg/server/pipeline.go`**:
   - At the beginning of `handleCompletions()`, right after reading `body`:
     ```go
     // 0. Wire-speed ingress context compaction (NTS)
     var ntsTokensSaved, ntsBytesSaved int
     if ntsTr := s.GetNTSTransformer(); ntsTr != nil {
         if strings.Contains(r.URL.Path, "messages") || r.Header.Get("anthropic-version") != "" {
             if transformed, res, err := ntsTr.TransformAnthropic(body); err == nil && !res.Bypassed {
                 body = transformed
                 ntsTokensSaved = res.TokensSaved
                 ntsBytesSaved = res.BytesSaved
             }
         } else {
             if transformed, res, err := ntsTr.TransformOpenAI(body); err == nil && !res.Bypassed {
                 body = transformed
                 ntsTokensSaved = res.TokensSaved
                 ntsBytesSaved = res.BytesSaved
             }
         }
     }
     ```
   - Feed the compacted `body` directly into `s.classifier.Classify(body)`.
   - Store `ntsTokensSaved`, `ntsBytesSaved`, and `rawTokens` in `reqCtx`:
     ```go
     reqCtx.NTSTokensSaved = ntsTokensSaved
     reqCtx.NTSBytesSaved = ntsBytesSaved
     reqCtx.RawTokens = reqCtx.Tokens + ntsTokensSaved
     ```
   - Log both compacted `tokens` and `raw_tokens` in `pipeline.go` (`Routing request`).
3. **Clean `pkg/server/dispatch.go`**:
   - Completely remove the NTS transformer execution block from `preparePayload()`.
   - Simplify signature:
     ```go
     func (s *Server) preparePayload(r *http.Request, targetTier *contract.Tier, targetProvider provider.LLMProvider, body []byte) []byte
     ```
   - In `dispatchTier()`, pass `preparedBody` directly, and pass `reqCtx.NTSTokensSaved` and `reqCtx.NTSBytesSaved` to `handleStreaming`/`handleNonStreaming`.
   - Eliminates redundant JSON unmarshals and duplicate NTS runs on upstream retry / fallback dispatches.
4. Run `TestPipeline_IngressNTSCompaction_RoutesToLowerTier` -> **PASS (GREEN)**.

### 2.4 TDD Step 3: REFACTOR & Phase 1 Coverage Gate (>= 95%)
- Run race detection: `go test -v -race ./pkg/server/...`
- Statement coverage audit:
  ```bash
  go test -coverprofile=coverage_server.out ./pkg/server
  go tool cover -func=coverage_server.out
  ```
- Verify `pkg/server` achieves **>= 95.0%** statement coverage.

### 2.5 Manual Verification Procedure (Prior to Git Commit)
1. **Configure a Test Profile**:
   ```yaml
   tiers:
     - name: "Tier 1: Small Context (Local)"
       when: "Tokens < 4000"
     - name: "Tier 2: Cloud Fallback"
       when: "true"
   nts:
     enabled: true
     strip_ansi: true
     resolve_cr: true
   ```
2. **Build and Launch Daemon**:
   ```bash
   go run ./cmd/nacho-flow -config test_config.yaml -port 8088
   ```
3. **Send Inbound Test Payload**:
   Send HTTP POST to `http://localhost:8088/v1/chat/completions` with 4,500 raw tokens (5,000 bytes ANSI).
4. **Verification Criteria**:
   - Prior to fix: Evaluates `4500 < 4000` -> routes to Tier 2 (Cloud Fallback).
   - With Phase 1 fix: NTS runs at ingress -> compacted size is 2,500 tokens -> `2500 < 4000` -> **routes to Tier 1: Small Context (Local)**.
   - Logs show `tokens=2500 raw_tokens=4500 nts_tokens_saved=2000`.
   - **Do not commit** until manual verification is approved.

---

## 3. Phase 2: Thinking/Reasoning In-Context Compaction (TDD)

### 3.1 Assistant Turn Retention & Reasoning Pruning Engine (`pkg/nts`)
> **Note on Architecture**: `nacho-flow` already implements thinking tag canonicalization and lane transitions in `pkg/agentregistry` (`data/reasoning.json` + `TagReplacer()`) and `pkg/server/stream_normalizer.go` (`normalizeContentDelta`). Phase 2 integrates directly with these existing mechanisms rather than introducing redundant string parsers in `zeroalloc`.

1. **RED: Write Comprehensive NTS Test Suite in `pkg/nts/transformer_test.go`**:
   - `TestTransformer_StripReasoning_AnthropicBlocks`: Strips `"thinking"`, `"redacted_thinking"`, `"thought"`, `"reasoning"`. Preserves `"tool_use"` and `"text"`.
   - `TestTransformer_StripReasoning_OpenAIField`: Deletes `"reasoning_content"`, `"reasoning"`, `"reason"`. Verifies `BytesSaved` and `TokensSaved` accounting.
   - `TestTransformer_StripReasoning_InlineTags`: Reuses `agentregistry.DefaultRegistry().TagReplacer()` and lane transition logic to drop thinking lane text and preserve prose.
   - `TestTransformer_KeepRecentReasoningTurns_Zero`: 0 turns kept -> all historical assistant reasoning pruned.
   - `TestTransformer_KeepRecentReasoningTurns_Cutoff`: Sliding window -> preserves the last $N$ assistant turns intact; prunes older turns.
   - `TestTransformer_StrictAssistantRoleFiltering`: Verifies that interleaved `user` and `tool` messages do NOT alter the assistant turn window count.
   - `TestTransformer_EmptyContentFallback_AnthropicRFC400`: Asserts fallback `[{"type": "text", "text": "Proceeding with next step."}]` injected when all blocks are pruned.
   - `TestTransformer_EmptyContentFallback_StringContent`: Asserts fallback text injected if string becomes empty after tag stripping.
   - `TestTransformer_PreserveCacheControl_Migration`: Asserts `cache_control` on pruned thinking block is migrated to the next surviving block.
   - `TestTransformer_AlternatingRole_Integrity`: Asserts assistant envelopes are never deleted, preserving `[user, assistant, user]` schema.
   - `TestTransformer_MultiTurnHistoryReplay`: Replays 1.76 MB agent session verifying ~71,728 tokens reclaimed.
   - Confirm tests fail (RED).

2. **GREEN: Implement Engine in `pkg/nts/transformer.go` & Configuration**:
   - Add schema fields to `contract.NTSConfig` and `nts.Config`:
     ```go
     StripHistoricalReasoning *bool `yaml:"strip_historical_reasoning,omitempty" json:"strip_historical_reasoning,omitempty"`
     KeepRecentReasoningTurns *int  `yaml:"keep_recent_reasoning_turns,omitempty" json:"keep_recent_reasoning_turns,omitempty"`
     ```
   - Strict assistant turn boundary slicing:
     ```go
     var assistantIndices []int
     for idx, msgItem := range msgs {
         if msg, ok := msgItem.(map[string]interface{}); ok {
             if role, _ := msg["role"].(string); role == "assistant" {
                 assistantIndices = append(assistantIndices, idx)
             }
         }
     }
     ```
   - Calculate cutoff: `cutoff := len(assistantIndices) - keepTurns`.
   - Prune qualifying assistant turns (Anthropic blocks, OpenAI fields from catalog, inline `<think>` lane transitions, RFC 400 guard, cache migration).
   - Wire `BuildNTSTransformer` in `pkg/server/proxy.go` and update configuration templates.
   - Confirm all tests pass (GREEN).

3. **REFACTOR & Coverage Gate**:
   - `go test -coverprofile=coverage_nts.out ./pkg/nts`
   - Enforce statement coverage **>= 95.0%** (baseline is 97.4%).

---

## 4. Final Quality Gates & Verification Matrix

| Package | Baseline Coverage | Target Min Bar | Verification Command | Gate Enforcement |
| :--- | :---: | :---: | :--- | :--- |
| `pkg/nts` | 97.4% | **>= 95.0%** | `go test -race -cover ./pkg/nts/...` | Strict CI Failure if < 95% |
| `pkg/server` | 94.0% | **>= 95.0%** | `go test -race -cover ./pkg/server/...` | Strict CI Failure if < 95% |

### Full Repository Verification
```bash
# 1. Full Unit & Benchmark Test Suite with Race Detector
go test -v -race ./...

# 2. Package-by-Package 95% Coverage Verification Script
go test -coverprofile=cov_nts.out ./pkg/nts && go tool cover -func=cov_nts.out | grep total
go test -coverprofile=cov_za.out ./pkg/zeroalloc && go tool cover -func=cov_za.out | grep total
go test -coverprofile=cov_srv.out ./pkg/server && go tool cover -func=cov_srv.out | grep total

# 3. Static Analysis & Security Gate
golangci-lint run ./...
gosec -quiet ./...
```

