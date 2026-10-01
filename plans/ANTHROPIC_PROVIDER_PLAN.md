# Native Anthropic Provider — Architectural Plan

> **Purpose**: This document is the architectural blueprint for adding a first-class `anthropic` provider type to nacho-flow. It is designed to be handed to Sonnet for detailed design elaboration and Gemini Flash for implementation.
>
> **Core Principle**: Implement the concrete `AnthropicProvider` first. Extract the `ProtocolProvider` interface last, shaped by what was actually needed.

---

## Context: How Nacho Flow Works Today

All providers currently speak **OpenAI wire protocol**. The proxy in `pkg/server/dispatch.go` does a dumb reverse-proxy: rewrite the `model` field, swap auth headers, forward the body as-is to the upstream URL, and pipe the SSE stream back.

The provider capability system (`pkg/provider/interfaces.go`) handles auth and headers via optional Go interfaces (`AuthProvider`, `HeaderProvider`), but **assumes the request/response body is OpenAI JSON**. That assumption is baked into `dispatch.go` lines ~150-220 (body preparation) and lines ~270-500 (stream reading).

This plan extends the provider system to support non-OpenAI protocols without changing any existing provider behavior.

---

## What the User Configures

```yaml
providers:
  anthropic:
    type: "anthropic"
    base_url: "https://api.anthropic.com"   # NOTE: no /v1 suffix
    api_key: "ENV_ANTHROPIC_API_KEY"

tiers:
  - name: "Direct Claude"
    model: "claude-sonnet-4-20250514"
    provider: "anthropic"
    when: "Tokens >= 16000 || Retries >= 2"
```

> **IMPORTANT**: The `type: "anthropic"` field in `ProviderConfig` is the discriminator. The registry factory uses it to instantiate `AnthropicProvider` instead of `GenericLLMProvider`.

---

## Phase 1: `AnthropicProvider` Struct + Request Translation

### New file: `pkg/provider/anthropic.go`

Create a concrete struct that implements the existing capability interfaces plus new methods for protocol adaptation:

```go
type AnthropicProvider struct {
    id             string
    config         contract.ProviderConfig
    circuitBreaker *CircuitBreaker
}

// Existing capability interfaces (same as GenericLLMProvider)
func (p *AnthropicProvider) ID() string
func (p *AnthropicProvider) Name() string
func (p *AnthropicProvider) BaseURL() string
func (p *AnthropicProvider) IsLocal() bool           // always false
func (p *AnthropicProvider) GetAPIKey() string
func (p *AnthropicProvider) CircuitBreaker() *CircuitBreaker

// NEW: Protocol adaptation
func (p *AnthropicProvider) BuildUpstreamRequest(ctx context.Context, r *http.Request, model string, body []byte) (*http.Request, error)
func (p *AnthropicProvider) WrapResponseStream(resp *http.Response) io.ReadCloser
```

### Request Translation Logic (`BuildUpstreamRequest`)

The method receives the raw OpenAI JSON body and transforms it:

| Step | What | Why |
|------|-------|-----|
| 1 | Parse `messages[]` — extract all `role: "system"` entries and hoist to top-level `system` field | Anthropic rejects system messages in the array |
| 2 | Ensure strict `user` → `assistant` alternation — merge consecutive same-role messages | Anthropic returns 400 on consecutive same-role |
| 3 | Convert `role: "tool"` messages into `tool_result` content blocks inside a `user` message | Anthropic requires tool results inside user turns |
| 4 | Map `tools[].function.parameters` → `tools[].input_schema` | Schema key name difference |
| 5 | Set `max_tokens` if absent (default to tier's `max_context` or 8192) | Anthropic requires it; OpenAI makes it optional |
| 6 | Strip or remap `temperature` / `top_p` for thinking-capable models | Sonnet 5.5 rejects non-default sampling params |
| 7 | Remap `tool_choice` — convert `"required"` / `"any"` to `"auto"` | Opus/Sonnet 5.5 reject forced tool choice |
| 8 | Set auth: `x-api-key` header + `anthropic-version: 2023-06-01` | Not Bearer token auth |
| 9 | Set URL to `{base_url}/v1/messages` | Not `/v1/chat/completions` |
| 10 | Inject `cache_control` breakpoints on tools, system, and penultimate user turn | Enable prompt caching |

### Content Block Translation (OpenAI → Anthropic)

OpenAI messages have `content` as a string or array of `{type, text}` / `{type, image_url}`.
Anthropic messages have `content` as an array of typed blocks:

```
OpenAI: {"role": "user", "content": "hello"}
→ Anthropic: {"role": "user", "content": [{"type": "text", "text": "hello"}]}

OpenAI: {"role": "user", "content": [{"type": "image_url", "image_url": {"url": "data:..."}}]}
→ Anthropic: {"role": "user", "content": [{"type": "image", "source": {"type": "base64", "media_type": "...", "data": "..."}}]}

OpenAI: {"role": "assistant", "tool_calls": [{"id": "tc_1", "function": {"name": "write_to_file", "arguments": "{...}"}}]}
→ Anthropic: {"role": "assistant", "content": [{"type": "tool_use", "id": "tc_1", "name": "write_to_file", "input": {...}}]}

OpenAI: {"role": "tool", "tool_call_id": "tc_1", "content": "file written"}
→ Anthropic: (merged into user message) {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tc_1", "content": "file written"}]}
```

---

## Phase 2: SSE Stream Adapter

### New file: `pkg/provider/anthropic_stream.go`

Anthropic streams **named SSE events** (`event: message_start`, `event: content_block_delta`, etc.), not raw `data: {choices: [...]}` chunks.

Create a `AnthropicStreamAdapter` that implements `io.ReadCloser` and **translates Anthropic SSE into OpenAI SSE** on the fly:

```go
type AnthropicStreamAdapter struct {
    source  *bufio.Reader   // reads from resp.Body
    buf     bytes.Buffer    // buffered output (OpenAI-formatted chunks)
    closed  bool
    msgID   string          // from message_start
    model   string
    usage   StreamUsage     // accumulated from message_delta
}

func (a *AnthropicStreamAdapter) Read(p []byte) (int, error)
func (a *AnthropicStreamAdapter) Close() error
```

### Event Translation Map

The adapter reads one Anthropic event at a time, translates it, and buffers the OpenAI-formatted output:

| Anthropic Event | Action |
|-----------------|--------|
| `event: message_start` | Store message ID and model. No output chunk yet. |
| `event: content_block_start` (type `text`) | No output needed (OpenAI doesn't have block-start). |
| `event: content_block_start` (type `tool_use`) | Emit `data: {"choices":[{"delta":{"tool_calls":[{"index":N,"id":"...","type":"function","function":{"name":"...","arguments":""}}]}}]}\n\n` |
| `event: content_block_start` (type `thinking`) | Begin thinking block. Emit as `reasoning_content` delta. |
| `event: content_block_delta` (type `text_delta`) | Emit `data: {"choices":[{"delta":{"content":"..."}}]}\n\n` |
| `event: content_block_delta` (type `input_json_delta`) | Emit `data: {"choices":[{"delta":{"tool_calls":[{"index":N,"function":{"arguments":"..."}}]}}]}\n\n` |
| `event: content_block_delta` (type `thinking_delta`) | Emit as `reasoning_content` delta (StreamNormalizer already handles this). |
| `event: content_block_stop` | No output needed. |
| `event: message_delta` | Extract `stop_reason`, map to `finish_reason`. Extract `usage`. Emit final delta with `finish_reason`. |
| `event: message_stop` | Emit `data: [DONE]\n\n`. |
| `event: error` | Map to OpenAI error JSON. |

### Stop Reason Mapping

```
Anthropic "end_turn"      → OpenAI "stop"
Anthropic "tool_use"      → OpenAI "tool_calls"
Anthropic "max_tokens"    → OpenAI "length"
Anthropic "stop_sequence" → OpenAI "stop"
```

### Usage Mapping

Anthropic's `usage` in `message_delta`:
```json
{
  "input_tokens": 1234,
  "output_tokens": 567,
  "cache_creation_input_tokens": 100,
  "cache_read_input_tokens": 1000
}
```

Map to the OpenAI `usage` chunk that `StreamNormalizer.GetUsage()` already parses:
```json
{
  "prompt_tokens": 1234,
  "completion_tokens": 567,
  "prompt_tokens_details": {
    "cached_tokens": 1000
  }
}
```

> **KEY INSIGHT**: `StreamNormalizer` already ingests `prompt_tokens_details.cached_tokens` for OpenRouter responses. The adapter just needs to map Anthropic's field names into the same shape. Zero changes to `StreamNormalizer`.

---

## Phase 3: `dispatch.go` Integration

### Modification: `dispatch.go` → `dispatchTier()` (lines ~150–220)

This is the **minimal touch point**. Add a single type-assertion branch before the existing OpenAI reverse-proxy logic:

```go
// After provider exists check (~line 149)
// Before payload preparation (~line 152)

if ap, ok := targetProvider.(*AnthropicProvider); ok {
    // Anthropic protocol path
    outReq, err := ap.BuildUpstreamRequest(r.Context(), r, targetTier.Model, preparedBody)
    if err != nil { /* error handling */ }
    
    resp, err := s.transport.RoundTrip(outReq)
    // ... existing error handling pattern ...
    
    if isStreaming {
        // Wrap Anthropic SSE → OpenAI SSE
        adaptedStream := ap.WrapResponseStream(resp)
        normalizer := NewStreamNormalizer(adaptedStream)  // existing pipeline continues!
        // ... rest of streaming logic is IDENTICAL ...
    } else {
        // Non-streaming: read body, translate, continue
    }
    return
}

// Existing OpenAI path continues unchanged below
```

### What Does NOT Change

- `StreamNormalizer` — receives OpenAI-formatted chunks from the adapter
- `CycleKiller` — monitors token counts and repetition on the normalized stream
- `AgentShield` — evaluates prose endings on the normalized content
- `Telemetry` / `PricingOracle` — receives standard `StreamUsage` with mapped fields
- `NTS Transformer` — already has `TransformAnthropic()` for outbound payloads
- VS Code Extension — sees standard OpenAI events via `/api/v1/events`

---

## Phase 4: Interface Extraction (After Phases 1–3 Work)

Once `AnthropicProvider` is working and tested, extract the capability interface:

```go
// pkg/provider/interfaces.go

// ProtocolProvider is an optional capability for providers with non-OpenAI wire protocols.
type ProtocolProvider interface {
    BuildUpstreamRequest(ctx context.Context, r *http.Request, model string, body []byte) (*http.Request, error)
    WrapResponseStream(resp *http.Response) io.ReadCloser
}
```

Then replace the concrete type assertion in `dispatch.go`:
```go
// Before (Phase 3):
if ap, ok := targetProvider.(*AnthropicProvider); ok {

// After (Phase 4):
if proto, ok := targetProvider.(provider.ProtocolProvider); ok {
```

This is a **pure refactor** — zero behavior change, all tests still pass. Future providers (Bedrock, Vertex) just implement the same interface.

---

## Registry Factory Change

### Modify: `pkg/provider/registry.go` → `NewRegistryFromConfig()`

```go
for id, pCfg := range cfg.Providers {
    switch pCfg.Type {
    case "anthropic":
        r.Register(NewAnthropicProvider(id, pCfg))
    default:
        r.Register(NewGenericLLMProvider(id, pCfg))
    }
}
```

### Modify: `pkg/contract/interfaces.go` → `ProviderType` constants

Add `ProviderTypeAnthropic ProviderType = "anthropic"` alongside existing `ProviderTypeLocal` / `ProviderTypeCloud`.

---

## Model Gotcha Catalog (Must-Implement Rules)

These will cause **400 errors** from Anthropic if not handled in `BuildUpstreamRequest`:

| Gotcha | Rule |
|--------|------|
| `max_tokens` missing | Always set. Default to 8192 or tier `max_context`. |
| `temperature` / `top_p` on thinking models | Strip for Sonnet 5.5, Opus 5.5. |
| `tool_choice: "required"` or `"any"` | Map to `"auto"`. |
| Assistant prefill (trailing assistant message) | Strip trailing assistant messages on current models. |
| Consecutive same-role messages | Merge into single message with concatenated content blocks. |
| `thinking: {type: "disabled"}` | Use `{type: "adaptive"}` instead on current models. |
| Thinking blocks in history | Pass back unchanged. Don't edit prior turns. |

---

## Prompt Cache Strategy

Anthropic allows up to 4 `cache_control: {"type": "ephemeral"}` breakpoints per request.

Insert them automatically in `BuildUpstreamRequest`:
1. After the **last tool definition** in `tools[]`
2. After the **system prompt** (top-level `system`)
3. After the **second-to-last user message** in conversation history

Verify cache hits by checking `usage.cache_read_input_tokens > 0` in telemetry.

---

## Testing Strategy

### Unit Tests: `pkg/provider/anthropic_test.go`

| Test | What it validates |
|------|-------------------|
| `TestBuildRequest_SystemExtraction` | System messages hoisted to top-level field |
| `TestBuildRequest_ToolResultGrouping` | Consecutive tool messages merged into single user message |
| `TestBuildRequest_RoleAlternation` | Consecutive same-role messages merged |
| `TestBuildRequest_MaxTokensDefault` | Missing max_tokens gets default |
| `TestBuildRequest_ToolChoiceRemap` | `required` → `auto` |
| `TestBuildRequest_ThinkingModelSanitize` | Temperature stripped for Sonnet 5.5 |
| `TestBuildRequest_CacheBreakpoints` | cache_control inserted at correct positions |
| `TestBuildRequest_ImageTranslation` | OpenAI image_url → Anthropic base64 source block |

### Unit Tests: `pkg/provider/anthropic_stream_test.go`

| Test | What it validates |
|------|-------------------|
| `TestStreamAdapter_TextDelta` | `content_block_delta` → OpenAI content chunk |
| `TestStreamAdapter_ToolUse` | `tool_use` block start + `input_json_delta` → OpenAI tool_calls chunks |
| `TestStreamAdapter_ThinkingDelta` | Thinking blocks → `reasoning_content` deltas |
| `TestStreamAdapter_StopReasonMapping` | `end_turn` → `stop`, `tool_use` → `tool_calls` |
| `TestStreamAdapter_UsageMapping` | Anthropic usage → OpenAI usage with cached_tokens |
| `TestStreamAdapter_MessageStop` | `message_stop` → `data: [DONE]` |
| `TestStreamAdapter_ErrorEvent` | `event: error` → OpenAI error JSON |

### Integration Tests: `pkg/server/proxy_anthropic_test.go`

| Test | What it validates |
|------|-------------------|
| `TestAnthropicProvider_E2E_Streaming` | Mock Anthropic server → full pipeline → client receives valid OpenAI SSE |
| `TestAnthropicProvider_CycleKiller` | CycleKiller triggers on Anthropic-sourced stream |
| `TestAnthropicProvider_CircuitBreaker` | Provider failure → fallback to default tier |
| `TestAnthropicProvider_NonStreaming` | Non-streaming request/response translation |

---

## Files Changed Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `pkg/provider/anthropic.go` | **NEW** | `AnthropicProvider` struct, `BuildUpstreamRequest`, `WrapResponseStream` |
| `pkg/provider/anthropic_stream.go` | **NEW** | `AnthropicStreamAdapter` (io.ReadCloser translating SSE formats) |
| `pkg/provider/anthropic_test.go` | **NEW** | Request translation unit tests |
| `pkg/provider/anthropic_stream_test.go` | **NEW** | Stream adapter unit tests |
| `pkg/provider/registry.go` | **MODIFY** | Factory switch on `type: "anthropic"` (~5 lines) |
| `pkg/contract/interfaces.go` | **MODIFY** | Add `ProviderTypeAnthropic` constant (~1 line) |
| `pkg/config/validation.go` | **MODIFY** | Allow `ProviderTypeAnthropic` in provider validation (~4 lines) |
| `pkg/config/validation_test.go` | **MODIFY** | Add test case for `type: "anthropic"` provider config (~10 lines) |
| `pkg/server/dispatch.go` | **MODIFY** | Type-assert hooks for `AnthropicProvider` request & stream (~30 lines) |
| `pkg/server/proxy_anthropic_test.go` | **NEW** | E2E integration tests with mock Anthropic server |
| `docs/ARCHITECTURE.md` | **MODIFY** | Document provider protocol capability |
| `README.md` | **MODIFY** | Add Anthropic to installation/config section |

### Files NOT Changed (Zero Touch)

- `pkg/server/stream_normalizer.go` — receives standard OpenAI chunks
- `pkg/router/shield/` — evaluates on normalized content
- `pkg/server/pipeline.go` — classifier and routing untouched
- `pkg/server/telemetry.go` — receives standard `StreamUsage`
- `pkg/nts/` — already has `TransformAnthropic()`
- `extension/` — VS Code extension sees standard events
- All existing tests — zero regressions

---

## Do NOT Use the Official Go SDK

Build a lightweight HTTP adapter using `encoding/json` + `bufio.Scanner`. Reasons:
- Zero new external dependencies (preserves `CGO_ENABLED=0` single binary story)
- Direct byte-level SSE translation preserves sub-millisecond proxy overhead
- SDK wraps streams in typed channel iterators requiring extra re-marshaling
- Only tracks the stable HTTP REST spec, not SDK release cadence

---

## Estimated Scope

| Component | Estimated LOC | Complexity |
|-----------|--------------|------------|
| `anthropic.go` (request translation) | ~300 | Medium — lots of field mapping |
| `anthropic_stream.go` (SSE adapter) | ~250 | Medium — state machine for event types |
| `dispatch.go` changes | ~30 | Low — single branch |
| `registry.go` changes | ~5 | Trivial |
| Tests | ~400 | Medium — mock server + assertion coverage |
| **Total** | **~1000** | |

---

## Architectural Review Refinements (Addenda)

The following refinements address critical edge cases identified during architectural review. Implementers (Sonnet/Flash) must treat these as mandatory specifications:

### 1. Dual-Phase Streaming Usage Aggregation
Anthropic splits usage reporting across two different SSE events:
* **`event: message_start`**: Contains `message.usage.input_tokens`, `cache_creation_input_tokens`, and `cache_read_input_tokens`.
* **`event: message_delta`**: Contains `delta.stop_reason` and `usage.output_tokens`.

```
                    ┌─────────────────────────┐
message_start ─────►│ Accumulate Prompt &     │
                    │ Cache Tokens            │
                    └──────────┬──────────────┘
                               │
                    ┌──────────▼──────────────┐
message_delta ─────►│ Accumulate Output       │
                    │ Tokens & Stop Reason    │
                    └──────────┬──────────────┘
                               │
                    ┌──────────▼──────────────┐
                    │ Emit OpenAI `usage` SSE │
                    │ chunk before [DONE]     │
                    └─────────────────────────┘
```

**Rule**: [AnthropicStreamAdapter](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/anthropic_stream.go) must buffer input/cache metrics on `message_start`, merge completion metrics on `message_delta`, and emit the final OpenAI-compatible `usage` chunk before `data: [DONE]`. This ensures [StreamNormalizer.GetUsage()](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/stream_normalizer.go#L1006) and [PricingOracle.CalculateFinancials()](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/telemetry.go#L47) receive non-zero metrics.

---

### 2. Non-Streaming Response Translation
When `stream: false`, Anthropic returns a native JSON message. Without translation, [fastChatCompletionResponse](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L534) fails to unmarshal choices, causing downstream parsers in OpenAI-compatible clients to crash.

**Specification**:
Add response translation to [AnthropicProvider](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/anthropic.go):
```go
func (p *AnthropicProvider) TranslateResponseBody(statusCode int, body []byte) ([]byte, error)
```
* Maps `role: "assistant"` and `content[].text` into `choices[0].message.content`.
* Maps `content[].tool_use` blocks into `choices[0].message.tool_calls` (`id`, `type: "function"`, `function.name`, `function.arguments`).
* Maps `stop_reason` (`end_turn` → `"stop"`, `tool_use` → `"tool_calls"`, `max_tokens` → `"length"`).
* Maps `usage` into standard OpenAI `usage` object including `prompt_tokens_details.cached_tokens`.

---

### 3. Surgical Dispatch Integration (Zero Code Duplication)
Do **not** branch at line 189 with an early return in [dispatchTier()](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L140), which would require duplicating ~300 lines of streaming peek, cycle detection, client flush, and telemetry logic.

Instead, inject two targeted hooks:
1. **Request Construction Hook** ([pkg/server/dispatch.go:L217](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L217)):
   ```go
   var outReq *http.Request
   if ap, ok := targetProvider.(*provider.AnthropicProvider); ok {
       outReq, err = ap.BuildUpstreamRequest(r.Context(), r, targetTier.Model, preparedBody)
   } else {
       outReq, err = http.NewRequestWithContext(r.Context(), r.Method, fullTargetURL, bytes.NewReader(preparedBody))
       // Copy headers and apply AuthProvider / HeaderProvider as usual
   }
   ```
2. **Stream Wrapping Hook** ([pkg/server/dispatch.go:L270](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L270)):
   ```go
   streamSource := resp.Body
   if ap, ok := targetProvider.(*provider.AnthropicProvider); ok {
       streamSource = ap.WrapResponseStream(resp)
   }
   normalizer := NewStreamNormalizer(streamSource)
   ```
3. **Non-Streaming Hook** ([pkg/server/dispatch.go:L506](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L506)):
   ```go
   if ap, ok := targetProvider.(*provider.AnthropicProvider); ok {
       translated, err := ap.TranslateResponseBody(resp.StatusCode, bodyBytes)
       if err == nil {
           bodyBytes = translated
       }
   }
   ```
This preserves 100% of cycle breaker logic, shield enforcement, delayed 200 OK quality fallback, and telemetry without duplicating a single line.

---

### 4. Config Validation Updates
[pkg/config/validation.go:L27](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/config/validation.go#L27) currently enforces:
```go
if p.Type != contract.ProviderTypeLocal && p.Type != contract.ProviderTypeCloud {
    return fmt.Errorf("provider %q: 'type' is required and must be 'local' or 'cloud'...", id)
}
```
**Required Changes**:
* Add `ProviderTypeAnthropic ProviderType = "anthropic"` to [pkg/contract/interfaces.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/contract/interfaces.go#L421).
* Update [pkg/config/validation.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/config/validation.go#L27) to allow `contract.ProviderTypeAnthropic`.
* Ensure [ProviderConfig.IsLocal()](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/contract/interfaces.go#L439) continues to return `false` for `ProviderTypeAnthropic`.
* Add `pkg/config/validation.go` and `pkg/config/validation_test.go` to the files changed list.

---

### 5. Wire Protocol & Format Specifics
* **Tool Delta Field**: Anthropic streaming sends JSON increments inside `delta.partial_json`, not `delta.arguments`. Map `delta.partial_json` → `delta.tool_calls[0].function.arguments`.
* **Tool Result Turn Merging**: Consecutive `role: "tool"` turns from OpenAI clients (e.g. multi-tool execution) must be folded into a **single `user` turn** containing multiple `tool_result` content blocks.
* **Tool Choice Mapping**: Map OpenAI `"required"` → Anthropic `{"type": "any"}`, and `"auto"` → `{"type": "auto"}`. Do not downgrade `required` to `auto`.
* **Prompt Caching Placement**: Anthropic requires `cache_control: {"type": "ephemeral"}` to be placed:
  - Inside a system content block: `system: [{"type": "text", "text": "...", "cache_control": {"type": "ephemeral"}}]`
  - On a tool definition: `tools[i].cache_control = {"type": "ephemeral"}`
  - Inside a message content block: `messages[i].content[j].cache_control = {"type": "ephemeral"}`
* **Thinking Mode Sanitization**: When thinking is enabled, Anthropic requires `temperature: 1.0` (or omitted). Strip custom `temperature` and `top_p`. If thinking is not requested, omit the `thinking` key entirely (do not send `type: "disabled"`).
* **URL Normalization**: Automatically trim trailing slashes and `/v1` suffixes from `base_url` before appending `/v1/messages`.

---

### 6. Zero-Alloc Architecture & Single-Pipeline Buffer Ownership

To preserve Nacho Flow's sub-millisecond proxy throughput and prevent GC latency spikes, the Anthropic provider must strictly obey the repository's zero-allocation design patterns ([pkg/zeroalloc](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/zeroalloc/bytes.go) and [pkg/server/stream_normalizer.go](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/stream_normalizer.go)):

#### A. Prohibition of Dynamic Maps (`map[string]interface{}`)
* **Never** use `map[string]interface{}` for payload assembly or response decoding. Dynamic map creation causes heavy heap churn, runtime hash bucket resizing, and interface boxing.
* Use strictly typed fast structs ([fastAnthropicReq](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/anthropic.go), [fastAnthropicMessage](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/anthropic.go), [fastAnthropicContentBlock](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/anthropic.go)) with `json.RawMessage` byte slices.

#### B. Zero-Alloc Payload Slicing (`json.RawMessage`)
* **Tool Schemas**: OpenAI's `tools[].function.parameters` is already valid JSON schema bytes. Do not unmarshal into a map; pass the `json.RawMessage` slice directly into Anthropic's `tools[].input_schema` (`0 B/op`).
* **Tool Arguments**: OpenAI's `tool_calls[].function.arguments` is JSON text bytes. Do not unmarshal into an object; slice the raw bytes directly into Anthropic's `tool_use.input` (`0 B/op`).
* **Tool Results**: Content strings are mapped directly without intermediate boxing.

#### C. Single-Pipeline Buffer Ownership (No Redundant `sync.Pool` in Providers)
* **Unified Gateway Buffer Management**: The proxy server pipeline ([pkg/server/](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/)) centrally owns and recycles all request and streaming buffers:
  - [streamBufferPool](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L34): Central 4KB peek, read, and scratch buffers serving every active request turn entering [dispatchTier()](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/dispatch.go#L140).
  - [bufPool](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/stream_normalizer.go#L20) & [readerPool](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/stream_normalizer.go#L25): Central 64KB buffered stream readers and byte buffers serving the streaming normalizer pipeline.
  - [defaultTailBufferPool](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/router/shield/tail_buffer.go#L7): Central 256-byte circular ring buffers for [AgentShield](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/router/shield/).
  - [cycleBreakerPool](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/router/shield/cycle_breaker.go#L146): Central stateful loop detection instances.
* **Provider Invariant**: Providers in `pkg/provider/` are protocol adapters, not execution engines. They must **never create independent or duplicate `sync.Pool` instances**, which would fragment memory pools and introduce premature buffer recycling bugs.
* **Request Serialization**: In `BuildUpstreamRequest`, serialize typed fast structs directly into `json.Marshal`. The returned byte slice is passed to `bytes.NewReader` for `http.Request` without ad-hoc provider pools.
* **Stream Normalization**: In `WrapResponseStream` (Phase 2), the [AnthropicStreamAdapter](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/anthropic_stream.go) is a pure `io.ReadCloser`. Downstream, [NewStreamNormalizer](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/server/stream_normalizer.go#L266) already wraps this adapter with the gateway's centralized `readerPool` and `bufPool`.

#### D. Zero-Alloc Steady-State Streaming Token Delta Loop
* Anthropic SSE parsing in [AnthropicStreamAdapter](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/pkg/provider/anthropic_stream.go):
  - Use `bytes.HasPrefix`, `bytes.IndexByte`, and `zeroalloc` slice routines.
  - Parse events into a stack-allocated or recycled `fastAnthropicSSEChunk` struct.
  - Write pre-formatted OpenAI SSE prefixes/suffixes (`data: {"choices":[{"delta":{"content":`) directly into the output buffer.
  - **Incur 0 heap allocations per emitted text token delta chunk**.

#### E. Benchmark Verification
* Every implementation phase must include Go benchmarks (`BenchmarkAnthropicRequestTranslation`, `BenchmarkAnthropicStreamAdapter`) with `b.ReportAllocs()` asserting zero or near-zero heap allocations on the hot path.
