# RFC-002: In-Flight Tool Call Argument Cycle Breaking in StreamNormalizer

* **Status**: Proposed / Under Review
* **Target Release**: v1.1.0
* **Author**: dixieflatline76 / Nacho Flow Team
* **Topic**: Streaming Tool-Call Argument Inspection, Repetition Defense & Monologue Prevention

---

## 1. Executive Summary

Nacho Flow's **Cycle Killer** currently monitors in-flight Streaming Server-Sent Events (SSE) across two isolated lanes:
1. **Prose Lane**: Evaluates standard assistant output (`delta.content`) for runaway conversational monologues and N-gram repetition loops.
2. **Thinking Lane**: Evaluates raw/tagged reasoning tokens (`delta.reasoning_content` or `<think>...</think>`) for circular internal deliberation.

However, in modern autonomous agent frameworks (e.g., **Zoo Code**, **Cline**, **OpenCode**, **Aider**), frontier and proprietary coding models increasingly emit **zero conversational prose** during execution turns. Instead, all model-generated tokens are encapsulated directly inside native tool calls:

```json
{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"execute_command","arguments":"sed -i 's/foo/bar/g' file.txt ..."}}]}}]}
```

Because `StreamNormalizer` historically only inspects `delta.content`, `reqCtx.CycleProseTokens` remains `0` during pure tool-calling sequences. If a model falls into an infinite circular tool-argument loop (e.g., infinite `sed`/`awk` replacements, repeating bash pipelines, recursive file-search commands, or repetitive code edits inside JSON arguments), the Cycle Killer cannot observe or sever the stream.

This RFC proposes extending `StreamNormalizer` to extract, tokenize, and inspect `delta.tool_calls[].function.arguments` in real time, ensuring complete stream defense across both conversational prose and agentic tool invocations without sacrificing sub-millisecond latency.

---

## 2. Motivation & Empirical Evidence

During benchmark runs with proprietary models (e.g., a 198-turn evaluation session using an unreleased OpenAI-compatible model), empirical telemetry revealed:
* **Tool Turns**: 193 out of 198 turns (97.5%) were pure tool calls.
* **Prose Token Count**: `cycle_prose_tokens` logged `0` across all 195 cloud turns because `delta.content` was null/empty.
* **Failure Mode**: When models iterated on failing test loops (observed 97 times), circular argument patterns in bash and edit tools were invisible to the streaming cycle breaker.

```
Current Architecture:
[Upstream SSE Stream]
         │
         ├── delta.content ───────────► [CycleBreaker Prose Lane] ──► ✅ Monologue Detected
         ├── delta.reasoning_content ─► [CycleBreaker Think Lane] ──► ✅ Circular Deliberation Detected
         └── delta.tool_calls ────────► [Bypasses CycleBreaker]   ──► ❌ Loop Unchecked (0 tokens recorded)
```

```
Proposed Architecture:
[Upstream SSE Stream]
         │
         ├── delta.content ───────────► [CycleBreaker Prose Lane]
         ├── delta.reasoning_content ─► [CycleBreaker Think Lane]
         └── delta.tool_calls ────────► [CycleBreaker Tool Lane]  ──► 🛡️ Argument Repetition Severed (<3s)
```

---

## 3. Technical Design & Architecture

### 3.1 Fast-Path Streaming Extraction

In `pkg/server/stream_normalizer.go`, `fastDelta` already parses `tool_calls` as raw JSON:

```go
type fastDelta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Reasoning        string          `json:"reasoning,omitempty"`
	Reason           string          `json:"reason,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
}
```

To extract streaming arguments without full multi-allocation JSON unmarshaling on every SSE chunk:
1. Define a lightweight streaming slice structure for tool call chunks:
```go
type fastToolCallChunk struct {
	Index    int `json:"index"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}
```
2. When `bytes.Contains(payload, []byte("\"tool_calls\""))` is true, extract `function.arguments` delta text.

### 3.2 Dual vs Unified Lane Strategy

We evaluate two design approaches for feeding tool arguments into `CycleBreaker`:

#### Option A: Unified Stream Ingestion (Simplest)
Feed argument delta strings directly into `CycleBreaker.ProcessDelta(argText, false)`.
* **Pros**: Reuses existing N-gram sliding window, zero new state structures.
* **Cons**: Code payloads (e.g. repeated struct initializers or boilerplate imports inside a file write tool) might trigger false-positive repetition detection prematurely.

#### Option B: Dedicated Tool Argument Lane (Recommended)
Add an isolated `ToolLane` in `CycleBreaker` with independent thresholds:
```go
type CycleBreaker struct {
	// ... existing prose & thinking lanes ...
	toolWords        []string
	toolNgramCounts  map[uint64]int
	toolNgramHistory map[uint64]ngramOccurrence
	toolTokens       int
	maxToolTokens    int  // default: 8192 (allows large file edits)
}
```
* **Pros**: 
  - Preserves distinct token telemetry (`cycle_tool_tokens`, `cycle_max_tool_ngram_freq`).
  - Allows higher token budgets for legitimate large file writes while catching tight loop cycles (e.g., repeating bash flags or cyclical edit commands).
* **Cons**: Small increase in struct memory footprint (~24 bytes).

### 3.3 Interception & Graceful Stream Termination

When a tool argument repetition loop or runaway budget violation occurs:
1. `StreamNormalizer.CheckCycleViolation()` flags `cycleViolated = true`.
2. Emits an authoritative termination frame:
   ```json
   data: {"choices":[{"index":0,"delta":{"content":"\n\n[SYSTEM OVERRIDE: Repetitive tool-call argument loop severed by Nacho Flow Cycle Killer.]"}}]}
   data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
   data: [DONE]
   ```
3. Triggers Stage 1 self-correction retry with `injectCorrectionPrompt()` or Stage 2 cloud tier escalation.

---

## 4. Performance & Allocation Invariants

1. **Zero Heap Allocation on Pure Prose Turns**: If no `tool_calls` byte marker is present in the chunk, the code path is completely bypassed.
2. **Buffer Pooling**: Reusable argument parsing buffers returned to `sync.Pool` on stream completion.
3. **P99 Latency Guarantee**: Streaming delta inspection overhead must remain $< 50\,\mu\text{s}$ per chunk.

---

## 5. Configuration Schema Additions

Extend `cycle_killer` in `config.yaml` to allow fine-grained control:

```yaml
cycle_killer:
  enabled: true
  max_prose_tokens: 4096
  max_thinking_tokens: 8192
  max_tool_argument_tokens: 8192       # Maximum token budget for single-turn tool arguments
  repetition_window: 6                 # N-gram size (in words)
  repetition_threshold: 3              # Loop count threshold
  inspect_tool_arguments: true         # Enable/disable tool argument loop monitoring
```

---

## 6. Implementation Milestones

1. **Phase 1 (Parser Extension)**: Add `fastToolCallChunk` unmarshaling in `StreamNormalizer.processChunkLine()`.
2. **Phase 2 (CycleBreaker Tool Lane)**: Implement `ProcessToolDelta()` with independent N-gram tracking in `pkg/router/shield/cycle_breaker.go`.
3. **Phase 3 (Telemetry & Sink)**: Expose `cycle_tool_tokens` in `TrafficLogEntry` and update `router.log`.
4. **Phase 4 (Test Suite)**: Comprehensive unit tests covering infinite bash loops, repetitive edit commands, and large file write non-interference.
