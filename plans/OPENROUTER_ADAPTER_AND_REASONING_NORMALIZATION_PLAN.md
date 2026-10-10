# OpenRouter Adapter & Reasoning Normalization Plan

> **Document**: `plans/OPENROUTER_ADAPTER_AND_REASONING_NORMALIZATION_PLAN.md`  
> **Discipline**: Strict **Test-Driven Development (TDD)** — **Red -> Green -> Refactor**.  
> **Quality Gate**: **>= 95% Statement Coverage Min Bar** across all modified packages (`pkg/provider`, `pkg/server`).  
> **Performance Invariant**: **Hot path must remain strictly zero-allocation (0 B/op, 0 allocs/op)** on clean/bypass paths and sub-microsecond (< 400 ns) on payload modification.

---

## 1. Architectural Philosophy & Lessons Learned

### 1.1 The Golden Principle: Config YAML is the Single Source of Ground Truth
* **Mechanism, Not Policy**: The runtime gateway should not second-guess models or inject "smart" defaults. The intelligence belongs in the configuration layer (via `config.yaml`, `profile3.yaml`, `nacho-flow tune`, or future AI agent / GUI config builders).
* **Explicit Intent**:
  1. If `targetTier.ReasoningEffort == ""` (unconfigured / omitted), Nacho Flow **injects nothing**. The payload is passed through untouched with **0 byte modifications and 0 heap allocations**.
  2. If `targetTier.ReasoningEffort != ""` (explicitly configured), Nacho Flow translates that intent into the exact wire dialect required by the upstream target provider:
     * **OpenRouter**: `{"reasoning": {"effort": "<val>"}}`
     * **OpenAI / Generic Cloud**: `{"reasoning_effort": "<val>"}`
     * **Native Anthropic**: Handled in `pkg/provider/anthropic.go` as `{"thinking": {"type": "adaptive"}, "output_config": {"effort": "<val>"}}`
  3. If a user configures reasoning on a non-reasoning model (or declares a text-only model as image-capable), the upstream provider will reject it. That is the configuration's responsibility to correct, not Nacho Flow's job to obscure with speculative runtime heuristics.

---

### 1.2 Retrospective on Past Pitfalls

#### Lesson 1: Phase 0.1 (The Big Mistake — Breaking an Active Control Loop)
* **What Happened**: We removed the de-escalation logic in `pipeline.go:372-378` assuming it was dead code.
* **The Consequence**: It was actually an **Anti-Latching Circuit Breaker**. Removing it caused sessions that hit retries to permanently latch onto the expensive `DefaultTier` (Sonnet) for 15+ turns, resulting in a +115% cost explosion ($0.66 -> $1.51) and turn inflation.
* **The Permanent Rule**: *Never remove or alter an existing control loop, tier fallback, or lifecycle state machine without tracing its cross-turn failure modes.* (The circuit breaker has been permanently preserved and documented).

#### Lesson 2: Phase 0.2 (The Almost Mistake — Inverting a Global Default)
* **What Almost Happened**: An earlier proposal suggested inverting `stream_normalizer.go:931`: `"if not a command, default to FileWrite"`.
* **The Consequence**: Inverting the global default would have dismantled Cycle Killer's N-gram loop protection across 90% of tools (MCP tools, file reads, searches, and model hallucinations), blowing their ceiling from 4k to 32k tokens.
* **The Permanent Rule**: *Never invert a secure default to solve an edge-case. Use surgical, opt-in, data-driven specifications instead.*
* **The Solution Implemented**: A data-driven **Cycle Killer Tool Exemption List** (`cycle_killer_exempt_tools`) in `agentregistry` with a pinpoint `||` check in `stream_normalizer.go:931`.

---

## 2. Phase Status & Empirical Validations

### Phase 0.1: Anti-Latching Circuit Breaker Preservation [COMPLETED & PERMANENT]
- Retained `s.sessionTracker.RecordEscalation(sessionKey)` in `pipeline.go` with full architectural doc-comments preventing regression.

### Phase 0.2: Data-Driven Cycle Killer Tool Exemption List [COMPLETED & PROVEN]
- **Implementation**:
  - `data/agents/cline.json` & `zoo.json`: Added `"cycle_killer_exempt_tools": ["attempt_completion"]`.
  - `data/agents/standard.json`: Added `"cycle_killer_exempt_tools": ["attempt_completion", "finish", "complete_task"]`.
  - `pkg/agentregistry`: Added `CycleKillerExemptTools []string`, exposed lock-free $O(1)$ zero-alloc `IsCycleKillerExempt()`.
  - `pkg/router`: Exposed `IsBuiltinCycleKillerExempt()`.
  - `pkg/server/stream_normalizer.go:931`: Added pinpoint check:
    ```go
    if router.IsBuiltinWriteTool(name) || router.IsBuiltinCycleKillerExempt(name) {
        s.currentToolCategory = shield.ToolCategoryFileWrite
    } else {
        s.currentToolCategory = shield.ToolCategoryCommand
    }
    ```
- **Empirical Benchmark Proof ($N=3$ Consecutive Morning Runs)**:
  - **Run 3**: 55 turns, 20.2 mins, 2 reads, 3.2M tokens -> **CLEAN PASS (`attempt_completion`)**
  - **Run 4**: 76 turns, 12.9 mins, 2 reads, 3.5M tokens -> **CLEAN PASS (`attempt_completion`)**
  - **Run 5**: 63 turns, 13.4 mins, 5 reads, 3.9M tokens -> **CLEAN PASS (`attempt_completion`)**
  - **Result**: 3 / 3 (100% completion rate), zero false positives from Cycle Killer.

---

## 3. Technical Architecture for Phase 1 & Phase 2

### 3.1 First-Class `OpenRouterProvider` (`pkg/provider/openrouter.go`)
Create a dedicated provider struct in `pkg/provider/openrouter.go`:
```go
type OpenRouterProvider struct {
    id             string
    name           string
    config         contract.ProviderConfig
    client         *http.Client
    circuitBreaker *CircuitBreaker
}
```
* **Capabilities**: Implements `LLMProvider`, `AuthProvider`, `HeaderProvider`, and `CircuitBreakerProvider`.
* **Default Base URL**: `https://openrouter.ai/api/v1`.
* **Standard Headers**: Automatically provides `HTTP-Referer: https://github.com/dixieflatline76/nacho-flow` and `X-Title: nacho-flow` (overridable via YAML).
* **Provider Identification**: Implements `IsOpenRouter() bool { return true }` (or declares `ReasoningFormat() ReasoningFormatOpenRouter`).

### 3.2 Zero-Allocation Wire Injection in `modifyTopLevelPayload` (`pkg/server/dispatch.go`)
In `modifyTopLevelPayload`:
* **Passthrough Invariant**:
  ```go
  if targetTier.ReasoningEffort == "" {
      // Do not touch reasoning fields; leave payload untouched.
  }
  ```
* **Injection / Translation**:
  When `targetTier.ReasoningEffort != ""`:
  - If target provider is **OpenRouter**:
    - Splice `"reasoning":{"effort":"<targetTier.ReasoningEffort>"}`.
    - If incoming body already has a flat `"reasoning_effort"`, strip it to prevent upstream 400 dual-parameter collisions.
  - If target provider is **OpenAI / Standard Cloud**:
    - Splice `"reasoning_effort":"<targetTier.ReasoningEffort>"`.
  - All modifications execute in a single zero-allocation copy pass using `zeroalloc.SpliceInto` (< 400 ns, 0 B/op).

---

## 4. Step-by-Step TDD Implementation Plan

### Phase 1: Provider Contracts & `OpenRouterProvider` (TDD)

1. **RED: Write Tests in `pkg/provider/openrouter_test.go`**:
   - `TestOpenRouterProvider_Basics`: Asserts ID, Name ("OpenRouter"), BaseURL default, and `IsLocal() == false`.
   - `TestOpenRouterProvider_Headers`: Asserts default `HTTP-Referer` and `X-Title` headers plus custom header merge.
   - `TestOpenRouterProvider_IsOpenRouter`: Asserts provider identifies as OpenRouter.
   - `TestRegistry_OpenRouterInstantiation`: Asserts provider instantiated as `*OpenRouterProvider` when:
     - `pCfg.Type == "openrouter"`, OR
     - `pCfg.Type == "cloud"` and ID is `"openrouter"` or URL contains `"openrouter.ai"`.

2. **GREEN: Implement Provider & Registry Wiring**:
   - In `pkg/contract/interfaces.go`: Add `ProviderTypeOpenRouter ProviderType = "openrouter"`.
   - In `pkg/config/validation.go`: Allow `contract.ProviderTypeOpenRouter` in type validation.
   - In `pkg/provider/openrouter.go`: Implement `OpenRouterProvider`.
   - In `pkg/provider/registry.go`: Instantiate `OpenRouterProvider` when type is `"openrouter"` or cloud OpenRouter.

3. **REFACTOR & Verify**:
   - Run `go test -v -race ./pkg/provider/...` and `go test -v ./pkg/config/...`.

---

### Phase 2: Surgical OpenRouter Reasoning Splicer (TDD)

1. **RED: Write Tests in `pkg/server/dispatch_openrouter_test.go`**:
   - `TestModifyTopLevelPayload_OpenRouter_InjectsNestedReasoning`:
     Tier specifies `reasoning_effort: "high"`, provider is OpenRouter -> injects `"reasoning":{"effort":"high"}`.
   - `TestModifyTopLevelPayload_OpenRouter_StripsFlatAndInjectsNested`:
     Inbound request with flat `{"reasoning_effort":"low"}`, tier specifies `reasoning_effort: "high"`, provider is OpenRouter -> emits `"reasoning":{"effort":"high"}` and strips flat field.
   - `TestModifyTopLevelPayload_OpenRouter_PassthroughWhenUnconfigured`:
     Tier has no `reasoning_effort` (`""`) -> payload returned byte-identical with 0 allocations, regardless of provider.
   - `TestModifyTopLevelPayload_Standard_PreservesFlatSyntax`:
     Tier specifies `reasoning_effort: "high"`, provider is standard cloud -> injects flat `"reasoning_effort":"high"`.
   - `BenchmarkModifyTopLevelPayload_OpenRouter_ZeroAlloc`:
     Asserts steady-state execution in < 400 ns with 0 B/op.

2. **GREEN: Implement Splicing in `pkg/server/dispatch.go`**:
   - Resolve whether target provider is OpenRouter.
   - Update `modifyTopLevelPayload` to apply the provider-specific wire format only when `targetTier.ReasoningEffort != ""`.

3. **REFACTOR & Verify**:
   - Run `go test -v -race ./pkg/server/...`.
   - Run benchmarks: `go test -bench=BenchmarkModifyTopLevelPayload ./pkg/server`.

---

### Phase 3: End-to-End Integration Verification

1. **Write Integration Test in `pkg/server/proxy_openrouter_test.go`**:
   - Mock OpenRouter upstream capturing the raw received HTTP request body.
   - Verify requests routed to an OpenRouter tier with `reasoning_effort: "medium"` receive `{"reasoning":{"effort":"medium"}}`.
   - Verify requests routed to an OpenRouter tier without `reasoning_effort` receive no reasoning parameters.
2. **Profile Validation**:
   - Verify `profile3.yaml` works against the updated gateway seamlessly.

---

## 5. Quality Gates & Acceptance Criteria

| Check | Target | Verification Command |
| :--- | :---: | :--- |
| `pkg/provider` Statement Coverage | **>= 95.0%** | `go test -coverprofile=cov_prov.out ./pkg/provider && go tool cover -func=cov_prov.out` |
| `pkg/server` Statement Coverage | **>= 95.0%** | `go test -coverprofile=cov_srv.out ./pkg/server && go tool cover -func=cov_srv.out` |
| Race Detector | **0 Data Races** | `go test -v -race ./...` |
| Hotpath Performance | **< 400 ns, 0 B/op** | `go test -bench=BenchmarkModifyTopLevelPayload ./pkg/server` |
| Full Repository Suite | **100% Pass** | `go test ./...` |
