# Architectural Plan: Inverted Wire Declarator, Reasoning Normalization & Fairy Dust Propagation

> **Document**: [plans/REASONING_NORMALIZATION_AND_OPENROUTER_ARCHITECTURE_PLAN.md](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/plans/REASONING_NORMALIZATION_AND_OPENROUTER_ARCHITECTURE_PLAN.md)  
> **Status**: APPROVED ARCHITECTURE — READY FOR TDD IMPLEMENTATION  
> **Discipline**: Strict Test-Driven Development (TDD) — Red $\rightarrow$ Green $\rightarrow$ Refactor  
> **Quality Gate**: $\ge 95\%$ Statement Coverage Min Bar across modified packages (`pkg/provider`, `pkg/server`, `pkg/contract`)  
> **Performance Invariant**: Zero-allocation hot path ($0\text{ B/op}$) and sub-microsecond latency ($< 400\text{ ns}$) strictly preserved  
> **Safety Invariant**: Zero regression across existing agent harnesses (Zoo, Kilo, Cline, Cursor) and upstream providers (Anthropic, OpenRouter, Generic Cloud, ROCm/Ollama)

---

## 1. Executive Summary & Root Cause Forensics

### 1.1 The Triggering Incident
During a live Cline benchmark run on Turn 15, the agent attempted to execute a tool call and crashed with an unhandled upstream 400 error from OpenRouter:
```
Reasoning is mandatory for this endpoint and cannot be disabled.
```

### 1.2 Forensic Trace & Mechanism of Failure
1. **Turn 15 Milestone**: Cumulative tool write operations reached 15, triggering **Fairy Dust (Tactical Code Review)** targeting model `z-ai/glm-5.3-flash` on provider `openrouter`.
2. **Contract Schema Dropping**: In `profile2.yaml`, the user explicitly configured:
   ```yaml
   - name: "Tactical Code Review"
     model: "z-ai/glm-5.3-flash"
     reasoning_effort: "high"
     provider: "openrouter"
   ```
   However, `FairyDustEntry` in [pkg/contract/interfaces.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/contract/interfaces.go#L643) lacked the `ReasoningEffort` struct field. Go's YAML unmarshaler silently discarded `"high"`, and `applyFairyDustTierOverride` initialized a `contract.Tier` with `ReasoningEffort: ""`.
3. **Client-Side Runtime Bias**: Cline (`@ai-sdk/openai-compatible/3.0.58`) sends `"reasoning_effort": "none"` by default on requests targeting custom proxy endpoints (`nacho-hybrid`).
4. **The Passthrough Leak**: In [pkg/server/dispatch.go:modifyTopLevelPayload](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L402), collection of `reSpans` and `reasoningSpans` was gated behind `if targetTier.ReasoningEffort != ""`. Because `ReasoningEffort` was empty string `""`, `dispatch.go` ignored the incoming reasoning fields, performed 0 edits, and leaked Cline's raw `"none"` straight upstream.
5. **Empirical OpenRouter Probe Execution**: Probing `z-ai/glm-5.3-flash` confirmed:
   - `{}` (no reasoning fields) $\rightarrow$ **200 OK** (runs with provider natural defaults).
   - `{"reasoning_effort": "none"}` $\rightarrow$ **400 Bad Request** (*Reasoning is mandatory*).
   - `{"reasoning": {"effort": "none"}}` $\rightarrow$ **400 Bad Request**.
   - `{"reasoning": {"effort": "high"}}` $\rightarrow$ **200 OK**.
6. **Why Zoo and Kilo Passed**: Neither Zoo nor Kilo sends `"reasoning_effort": "none"`. Leaving their payloads untouched allowed OpenRouter to run with default reasoning (HTTP 200).

---

## 2. Architectural Decision: The Inverted Wire Declarator Pattern

### 2.1 The Two Rejected Alternatives
1. **Full AST Unmarshaling (Anthropic style)**: In [pkg/provider/anthropic.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/anthropic.go#L104), converting OpenAI `/v1/chat/completions` into Anthropic `/v1/messages` allocates 50 KB–150 KB per request and takes $> 20\,\mu\text{s}$. Using this for OpenRouter or OpenAI-compatible paths would destroy Nacho-Flow's $0\text{ B/op}$ wire-speed guarantee and drop unmodeled client extension fields.
2. **Hardcoded Brand Branching**: Scattering `if isOpenRouter` checks across `pkg/server/dispatch.go` violates the Open-Closed Principle and couples the proxy core to a single commercial brand.

### 2.2 The Inverted Interface Architecture
Rather than `dispatch.go` knowing provider identities, each provider **statically declares its wire dialect and sanitization requirements** via a capability contract:

```
+------------------------------------+
|        pkg/provider                |
|                                    |
|   OpenRouterProvider               |
|      WireDeclarator() -------------> [Static Singleton: DialectNested, ForbidsEffortNone=true]
|                                    |
|   GenericProvider / LocalProvider  |
|      WireDeclarator() -------------> [Static Singleton: DialectFlat,   ForbidsEffortNone=false]
+------------------------------------+
                   ^
                   | implements WireDeclaratorProvider
                   |
+------------------+-----------------+
|        pkg/server/dispatch.go      |
|                                    |
|   modifyTopLevelPayload():         |
|     1. Queries decl via itab check |
|     2. Scans body in single pass   |
|     3. Applies declarative rules:  |
|        - Format dialect            |
|        - Strip forbidden "none"    |
|        - Coalesce adjacent deletes |
|     4. zeroalloc.SpliceInto()      |
|        (0 B/op, < 400 ns)          |
+------------------------------------+
```

### 2.3 Memory & Performance Proof ($0\text{ B/op}$)
* `WireDeclarator` is a 3-byte struct (`Dialect uint8`, `ForbidsEffortNone bool`, `StripFlatOnNested bool`).
* Each provider declares a package-level global variable (`openRouterWireDeclarator`). Returning `*WireDeclarator` is a single register move (`RAX`) of a static address. Zero heap allocations.
* Type-asserting `targetProvider.(WireDeclaratorProvider)` is an `itab` comparison on an existing pointer. Zero heap allocations.
* If no edits are required, `modifyTopLevelPayload` returns the original `body` unmodified. Zero heap allocations.
* If edits are required, `zeroalloc.SpliceInto` writes into a newly allocated output buffer in a single pass. Zero AST garbage.

---

## 3. The Three Required Architectural Fixes (Gemini Argon Review)

### 3.1 Fix 1: Per-Span Checking & Contiguous Span Coalescing (Eliminating Adjacent Comma Collision)

#### The Adjacent Delimiter Collision Problem:
In [pkg/server/dispatch.go:deleteMemberSpan](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L275), member deletion operates on a single member:
- If a preceding comma exists, it deletes `[comma_prev, vEnd)` (Rule 1).
- If no preceding comma exists (first member), it deletes `[kStart, comma_next + 1)` (Rule 2).

**The Bug**:
When two adjacent fields are deleted in the same payload:
E.g., `{"reasoning": {"effort": "none"}, "reasoning_effort": "none", "model": "gpt-4o"}`
1. Span 1 (`reasoning`): First member $\rightarrow$ Rule 2 deletes `[kStart, comma + 1)`.
2. Span 2 (`reasoning_effort`): Second member $\rightarrow$ Rule 1 deletes `[comma, vEnd)`.
3. **Collision**: Edit 1 ends at `comma + 1`, and Edit 2 starts at `comma`. They overlap by 1 byte!
4. **Failure**: [pkg/zeroalloc/json_scan.go#L347](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/zeroalloc/json_scan.go#L347) asserts `e.Start >= lastEnd`. Because `e.Start < lastEnd`, `SpliceInto` **aborts and returns `nil, false`**, causing `modifyTopLevelPayload` to return the raw unedited body with the forbidden `"none"` intact!

#### The Mathematical Solution (Contiguous Span Coalescing):
1. **Per-Span Checking**:
   - Inspect each `reSpan`: verify if `body[sp.vStart:sp.vEnd]` equals `"\"none\""`.
   - Inspect each `reasoningSpan`: call `zeroalloc.ObjectField(body, sp.vStart, sp.vEnd, "effort")`. If effort is `"none"`, mark for deletion. If `reasoning` contains other keys (e.g. `{"max_tokens": 1000}` without `effort: none`), do not delete the span!
2. **Span Coalescing**:
   Collect all spans marked for deletion in a stack buffer `[4]memberSpan` and sort by `kStart`.
   Iterate through the sorted spans: if between `span[i].vEnd` and `span[i+1].kStart` there are only whitespace and comma bytes, **coalesce them into a single member span**:
   ```go
   coalesced[c].vEnd = next.vEnd
   ```
3. **Delimiter Cleanliness**:
   Call `deleteMemberSpan` only once per coalesced span range. This guarantees:
   - Zero overlapping edit ranges.
   - Exact delimiter cleanup across head, tail, middle, and sole-member permutations.

### 3.2 Fix 2: Do Not Inherit `targetTier.ReasoningEffort` in Fairy Dust
- In [pkg/contract/interfaces.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/contract/interfaces.go#L643), add `ReasoningEffort string `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`` to `FairyDustEntry`.
- In [pkg/server/pipeline.go:applyFairyDustTierOverride](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/pipeline.go#L503):
  Set `ReasoningEffort: entry.ReasoningEffort` directly.
  **Do NOT fall back to `targetTier.ReasoningEffort`**. A routine tier may use `reasoning_effort: "low"` to save costs. When Fairy Dust triggers a frontier audit model, inheriting `"low"` would throttle the frontier model. If `entry.ReasoningEffort` is omitted, the frontier model must run with its native provider defaults.

### 3.3 Fix 3: Full Removal of Dead `provider.IsOpenRouter`
- Purge `IsOpenRouter()` and `OpenRouterProviderMarker` from [pkg/provider/interfaces.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/interfaces.go#L60) and [pkg/provider/openrouter.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/openrouter.go).
- Remove all `provider.IsOpenRouter` references from `pkg/server/dispatch.go`.
- Replace with capability-based `WireDeclaratorProvider`.

---

## 4. Ground Truth Wire Transformation Matrix

| Inbound Client Payload | `targetTier.ReasoningEffort` | Provider `WireDeclarator` | Splicer Outbound Action | Upstream Status |
| :--- | :--- | :--- | :--- | :--- |
| `{"reasoning_effort": "none"}` | `""` (unset) | `ForbidsEffortNone: true` | **Delete span** | **200 OK** (model natural default) |
| `{"reasoning": {"effort": "none"}}` | `""` (unset) | `ForbidsEffortNone: true` | **Delete span** | **200 OK** (model natural default) |
| `{"reasoning": {"effort": "none"}, "reasoning_effort": "none"}` | `""` (unset) | `ForbidsEffortNone: true` | **Coalesce & delete both** | **200 OK** (no duplicate comma collision) |
| `{"reasoning": {"max_tokens": 1000}, "reasoning_effort": "none"}` | `""` (unset) | `ForbidsEffortNone: true` | **Delete only `reasoning_effort`** | **200 OK** (preserves valid `reasoning`) |
| `{"reasoning_effort": "none"}` | `"high"` (Fairy Dust) | `DialectNested` | **Rewrite to `{"reasoning": {"effort": "high"}}`** | **200 OK** (YAML is ground truth) |
| `{}` (no reasoning fields) | `""` (unset) | Any provider | **Passthrough (0 edits, 0 B/op)** | **200 OK** (preserves existing behavior) |
| `{"reasoning_effort": "none"}` | `""` (unset) | `ForbidsEffortNone: false` | **Passthrough** | Upstream determines |

---

## 5. Detailed Step-by-Step TDD Implementation Plan

### Step 1: Contract Schema & Fairy Dust Propagation (Red $\rightarrow$ Green)
1. **[pkg/contract/interfaces.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/contract/interfaces.go)**:
   - Add `ReasoningEffort string `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`` to `FairyDustEntry`.
2. **[pkg/server/pipeline.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/pipeline.go)**:
   - In `applyFairyDustTierOverride`: assign `ReasoningEffort: entry.ReasoningEffort`.
3. **Tests**:
   - In `pkg/server/pipeline_test.go`: Assert that Fairy Dust override applies `entry.ReasoningEffort` directly without inheriting the routine tier's effort.

### Step 2: Wire Declarator Definitions & Provider Implementation (Red $\rightarrow$ Green)
1. **[pkg/provider/interfaces.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/interfaces.go)**:
   - Define:
     ```go
     type ReasoningDialect uint8
     const (
         ReasoningDialectFlat ReasoningDialect = iota
         ReasoningDialectNested
     )
     type WireDeclarator struct {
         Dialect           ReasoningDialect
         ForbidsEffortNone  bool
         StripFlatOnNested bool
     }
     type WireDeclaratorProvider interface {
         WireDeclarator() *WireDeclarator
     }
     ```
   - Remove `OpenRouterProviderMarker` and `IsOpenRouter`.
2. **[pkg/provider/openrouter.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/openrouter.go)**:
   - Statically declare `openRouterWireDeclarator = &WireDeclarator{Dialect: ReasoningDialectNested, ForbidsEffortNone: true, StripFlatOnNested: true}`.
   - Implement `WireDeclarator() *WireDeclarator`. Remove `IsOpenRouter()`.
3. **[pkg/provider/generic.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/generic.go)**:
   - Statically declare `genericWireDeclarator = &WireDeclarator{Dialect: ReasoningDialectFlat, ForbidsEffortNone: false, StripFlatOnNested: false}`.
   - Implement `WireDeclarator() *WireDeclarator`.

### Step 3: Fast Splicer Refactoring & Adjacent Span Coalescing (Red $\rightarrow$ Green)
1. **[pkg/server/dispatch.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go)**:
   - Replace `isOpenRouter` with capability lookup:
     ```go
     var decl *provider.WireDeclarator
     if wdp, ok := targetProvider.(provider.WireDeclaratorProvider); ok {
         decl = wdp.WireDeclarator()
     }
     ```
   - Unconditionally record `reSpans` and `reasoningSpans` during the top-level member scan.
   - Implement `coalesceMemberSpans(body []byte, spans []memberSpan) []memberSpan` on stack buffers.
   - In reasoning formatting block:
     - When `targetTier.ReasoningEffort != ""`: format according to `decl.Dialect`. If `decl.StripFlatOnNested`, delete flat spans using coalesced deletion.
     - When `targetTier.ReasoningEffort == ""`:
       - If `decl != nil && decl.ForbidsEffortNone`:
         - Check each `reSpan`: if `isEffortNone(...)`, mark for deletion.
         - Check each `reasoningSpan`: if `isNestedEffortNone(...)`, mark for deletion.
         - Coalesce marked delete spans and apply `deleteMemberSpan`.

### Step 4: Comprehensive Test Suite & Benchmark Gates (Red $\rightarrow$ Green)
1. **[pkg/server/dispatch_openrouter_test.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch_openrouter_test.go)**:
   - `TestModifyTopLevelPayload_OpenRouter_StripsClientNone_WhenTierUnset`
   - `TestModifyTopLevelPayload_OpenRouter_StripsNestedNone_WhenTierUnset`
   - `TestModifyTopLevelPayload_AdjacentTailDeletion_BothNone`: Tests both `"reasoning": {"effort": "none"}` and `"reasoning_effort": "none"` at the end of the JSON object.
   - `TestModifyTopLevelPayload_AdjacentHeadDeletion_BothNone`: Tests both at the beginning of the JSON object.
   - `TestModifyTopLevelPayload_SoleMembers_BothNone`: Tests object containing only reasoning fields (`{}`).
   - `TestModifyTopLevelPayload_PartialReasoning_PreservesValidFields`: Inbound `{"reasoning": {"max_tokens": 1000}, "reasoning_effort": "none"}` strips only `reasoning_effort`.
   - `TestModifyTopLevelPayload_OpenRouter_TierOverridesClientNone`: Fairy Dust `"high"` overrides client `"none"`.
2. **Benchmark Invariants**:
   - `BenchmarkModifyTopLevelPayload_OpenRouter_StripNone`: Verify **$0\text{ B/op}$** on clean/bypass and single-buffer allocation on modification.

### Step 5: Verification & Safety Gates
1. Run `go test -v -race ./pkg/contract/... ./pkg/provider/... ./pkg/server/...`.
2. Run full workspace regression: `go test ./...`.
3. Verify live Cline task by clicking **[Retry]**.
