# RFC-002: Agentic Tool Fallback Shield & Zero-Tool Prose Auto-Wrapper

* **Status**: Proposed / Future Milestone
* **Target Release**: v0.7.0 (Post-v0.6.0 Milestone)
* **Author**: dixieflatline76 / Nacho Flow Team
* **Topic**: Preventing Agent Harness 3-Strike Deadlocks in Zoo Code & Cline for Local Models

---

## 1. Executive Summary

This RFC specifies the technical architecture for the **Agentic Tool Fallback Shield** in Nacho Flow.

Modern IDE agentic extensions (such as **Zoo Code** and **Cline**) enforce a rigid invariant: *every assistant turn must invoke a tool call*. When local open-weights models (such as **Gemma 4**, **DeepSeek-R1**, or **Qwen 2.5 Coder**) attempt to ask a clarifying question, present a technical plan, or propose a mode switch in plain prose, the IDE agent harness rejects the turn with `[ERROR] You did not use a tool!` and terminates the task after 3 consecutive failures.

Nacho Flow's **Agentic Tool Fallback Shield** intercepts these conversational turns in real-time, dynamically detects the client's available tool schema (e.g. `ask_followup_question`, `ask_question`), and synthesizes a schema-compliant OpenAI tool call wrapping the model's prose before delivering it to the IDE.

```
+---------------------------------------------------------------------------------------------+
|                                    WITHOUT NACHO SHIELD                                     |
+---------------------------------------------------------------------------------------------+
Local LLM (Gemma 4)           Nacho Flow (Raw Proxy)               Zoo Code / Cline
      |                                |                                       |
      |-- "Here is the plan. OK?" ---->|-- Passes raw text with 0 tools ------>|
      |   (Pure prose response)        |   (tool_calls: [])                    |
      |                                |                                       |-- [REJECTED]
      |                                |                                       |   "You did not use
      |                                |<-- [ERROR] You did not use a tool! ---|   a tool in your
      |                                |                                           response!"
      |<-- Forward Error Prompt -------|
      |   (Gemma loops in <think> & crashes in 3 strikes) ❌
```

```
+---------------------------------------------------------------------------------------------+
|                                     WITH NACHO SHIELD                                       |
+---------------------------------------------------------------------------------------------+
Local LLM (Gemma 4)           Nacho Flow (Agent Shield)            Zoo Code / Cline
      |                                |                                       |
      |-- "Here is the plan. OK?" ---->|                                       |
      |   (Pure prose response)        |-- Detects tools schema has            |
      |                                |   "ask_followup_question"             |
      |                                |-- Auto-synthesizes tool payload       |
      |                                |   {name: "ask_followup_question",     |
      |                                |    arguments: {question: "..."}}      |
      |                                |                                       |
      |                                |-- Returns valid OpenAI tool_call ---->|-- [ACCEPTED]
      |                                |                                       |   Renders interactive
      |                                |                                       |   approval UI to user ✅
```

---

## 2. Problem Statement & The 3-Strike Deadlock

### 2.1 The Architect Mode Constraint
In coding agent harnesses (Zoo Code, Cline), agents operate across discrete modes:
* **`Architect Mode`**: Read-only planning and design. Code execution tools (`execute_command`, `write_to_file`) are disabled by the extension.
* **`Code Mode`**: Read/write implementation mode with execution permissions.

When a user submits an actionable task in Architect mode, local models frequently attempt to initialize the project via `execute_command`, receive a permission error from the extension, and pivot to presenting an architectural plan in plain text.

### 2.2 The Zero-Tool Rejection Loop
1. **Turn 1 (Prose Plan)**: The local model outputs markdown describing the technical plan and asks: *"Are you satisfied with this plan, or should I switch to Code mode?"*.
2. **Turn 2 (Rejection)**: The extension's client-side harness expects an explicit tool invocation (e.g. `ask_followup_question` or `switch_mode`). Receiving `tool_calls: []`, it injects an error prompt:
   ```text
   [ERROR] You did not use a tool in your previous response! Please retry with a tool use.
   ```
3. **Turn 3 (Reasoning Token Exhaustion)**: When presented with the error, local reasoning models (Gemma 4, DeepSeek-R1) generate extensive `<think>` blocks analyzing why the previous turn failed. By the time reasoning finishes, the output token budget (`num_predict`) is exhausted, emitting an empty body (`"text": "\n\n"`) with 0 tools.
4. **Task Termination**: After 3 consecutive zero-tool turns, the extension gives up:
   > `"The model provided text/reasoning but did not call any of the required tools."`

---

## 3. Design Invariants & Guardrails

To prevent false positives and maintain 100% fidelity with standard conversational requests, the Shield operates under the following **Core Invariants**:

1. **Schema-Aware Activation**: The shield **only activates** if the incoming client request payload defines one of the recognized conversational tools:
   - `ask_followup_question` (Zoo Code, Cline standard)
   - `ask_question` (Generic agentic schema)
   - `user_prompt` / `interactive_input`
2. **Intent Guard (Zero False Positives)**: Normal completion commentary (e.g. *"I have finished writing the tests"* or code diff blocks) must **never** be converted into a question tool. The auto-wrapper requires:
   - Response ends with a question mark (`?`), **OR**
   - Matches plan-approval heuristics (`"Are you satisfied"`, `"Would you like"`, `"Should I"`, `"Do you approve"`, `"Please confirm"`), **OR**
   - Explicit mode-switch phrasing (`"switch to code mode"`, `"proceed to implementation"`).
3. **Zero-Overhead Fast Bailout**: If the response already contains valid `tool_calls: [...]`, the shield bails out in $< 50\text{ nanoseconds}$ with zero heap allocations.
4. **Deterministic ID Generation**: Synthetic tool calls generate deterministic prefix IDs (`call_autowrap_<hash>`) for transparent auditability in traffic logs.

---

## 4. Technical Architecture

### 4.1 Schema Extraction & Request Context
During request intake in `pkg/router/classifier.go`, the proxy records the list of interactive tool names declared in `r.Body.tools`:

```go
type RequestContext struct {
    // ... existing fields ...
    InteractiveToolName string // "ask_followup_question", "ask_question", or ""
}
```

```go
func extractInteractiveTool(tools []OpenAITool) string {
    for _, t := range tools {
        name := strings.ToLower(t.Function.Name)
        if name == "ask_followup_question" || name == "ask_question" {
            return t.Function.Name
        }
    }
    return ""
}
```

### 4.2 Prose Intent Evaluator (`pkg/router/tool_normalizer_fallback.go`)

```go
var questionHeuristics = []string{
    "are you satisfied",
    "would you like",
    "should i",
    "do you approve",
    "please confirm",
    "switch to code mode",
    "switch mode",
}

func shouldAutoWrapProse(content string, toolName string) bool {
    if toolName == "" || len(strings.TrimSpace(content)) == 0 {
        return false
    }
    
    trimmed := strings.TrimSpace(content)
    if strings.HasSuffix(trimmed, "?") {
        return true
    }
    
    lower := strings.ToLower(trimmed)
    for _, phrase := range questionHeuristics {
        if strings.Contains(lower, phrase) {
            return true
        }
    }
    return false
}
```

### 4.3 Non-Streaming Synthetic Tool Synthesis

When an upstream response finishes with `len(choice.Message.ToolCalls) == 0` and `shouldAutoWrapProse` returns true:

```go
func WrapProseInToolCall(choice *ChatChoice, toolName string) {
    callID := fmt.Sprintf("call_autowrap_%x", sha256.Sum256([]byte(choice.Message.Content))[:6])
    
    argsObj := map[string]string{
        "question": choice.Message.Content,
    }
    argsJSON, _ := json.Marshal(argsObj)
    
    choice.Message.ToolCalls = []ToolCall{
        {
            ID:   callID,
            Type: "function",
            Function: FunctionCall{
                Name:      toolName,
                Arguments: string(argsJSON),
            },
        },
    }
    // Clear raw prose content to avoid duplication in clients that display both
    choice.Message.Content = ""
}
```

### 4.4 Streaming SSE Pipeline Synthesis (`pkg/server/stream_normalizer.go`)

In streaming mode:
1. As tokens arrive, `StreamNormalizer` buffers trailing characters and monitors whether upstream emits any `tool_calls` delta chunks.
2. If the stream reaches `data: [DONE]` with zero tool chunks and pure prose matching `shouldAutoWrapProse`:
3. The normalizer transmits a synthetic tool-call delta before the final `[DONE]` event:
   ```sse
   data: {"id":"chatcmpl-autowrap","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_autowrap_8f1a","type":"function","function":{"name":"ask_followup_question","arguments":"{\"question\":\"..."}}]}}]}
   
   data: {"id":"chatcmpl-autowrap","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}
   
   data: [DONE]
   ```

---

## 5. Configuration Specification (`config.yaml`)

```yaml
# ==============================================================================
# Agentic Tool Fallback Shield (RFC-002)
# ==============================================================================
router:
  # Enable automatic synthesis of interactive question tool calls when local
  # models reply with conversational plans or questions in rigid agent modes
  # (prevents 3-strike "model did not use a tool" errors in Zoo/Cline).
  agent_tool_fallback: true

  # Whitelist of tool names eligible for auto-wrapping
  fallback_tool_whitelist:
    - "ask_followup_question"
    - "ask_question"
```

---

## 6. Micro-Benchmark & Latency Expectations

The Shield is designed to operate with negligible overhead:

| Code Path | Latency Target | Heap Allocations |
| :--- | :--- | :--- |
| **Bailout (Model called tools natively)** | $< 30\text{ ns}$ | **0 B/op (0 allocs)** |
| **Bailout (No interactive tool in schema)** | $< 25\text{ ns}$ | **0 B/op (0 allocs)** |
| **Active Prose Analysis (Regex/Suffix)** | $< 450\text{ ns}$ | **0 B/op (0 allocs)** |
| **Synthetic JSON Tool Serialization** | $< 2.1\mu\text{s}$ | **~450 B/op (4 allocs)** |

---

## 7. Execution Checklist

- [ ] Implement `extractInteractiveTool` in `pkg/router/classifier.go`.
- [ ] Implement `shouldAutoWrapProse` and `WrapProseInToolCall` in `pkg/router/tool_normalizer.go`.
- [ ] Integrate fallback synthesizer into non-streaming response handler in `pkg/server/proxy.go`.
- [ ] Integrate streaming fallback chunk emitter into `pkg/server/stream_normalizer.go`.
- [ ] Add unit tests in `pkg/router/tool_normalizer_test.go` covering Gemma 4, Qwen, and DeepSeek plan outputs.
- [ ] Add streaming integration tests in `pkg/server/proxy_test.go`.
- [ ] Add configuration flag `agent_tool_fallback` in `pkg/contract/interfaces.go` and `pkg/config/template.go`.
- [ ] Validate end-to-end against live Zoo Code and Cline instances in Architect mode.
