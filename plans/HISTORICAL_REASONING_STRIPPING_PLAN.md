# NTS Ingress Historical Reasoning Pruning — Architectural Plan

> **Goal**: Add an assistant message reasoning pruning pass to Nacho Token Saver (NTS) in `pkg/nts/transformer.go` to drop dead reasoning monologues (`type: "reasoning"`, `type: "thinking"`, `reasoning_content`) from past assistant turns before forwarding requests upstream.
>
> **Core Value**: Slashes **50,000 to 75,000+ dead tokens** from late-stage multi-turn agent sessions without touching tool definitions, tool inputs, or tool results. Guarantees zero risk of diff amnesia, line drift, or syntax errors.

---

## 1. Spice Run Audit Findings
- Task ID: `01a0f858-57f1-77ed-b6f0-1890425b5382`
- History: 453 messages, 226 assistant turns.
- **59 structured reasoning blocks** totaling **286,913 characters (~71,728 tokens)** accumulated in Zoo Code's `api_conversation_history.json`.
- Late-stage turns carried ~384,000 tokens, exceeding Tier 2's `Tokens < 160000` limit and forcing 71 turns to `google/gemini-3.8-flash` ($0.75/1M) instead of `z-ai/glm-5.3-flash` ($0.06/1M), driving cost from $0.55 to $11.90.
- All 269 `tool_use` blocks and file reads must remain completely intact.

---

## 2. Core Pruning Target Formats

NTS inspects messages where `role == "assistant"`:

1. **Anthropic / Content Block format** (Zoo Code, Roo Code, Claude 3.7 Sonnet):
   - Content blocks where `type == "reasoning"`, `type == "thinking"`, `type == "redacted_thinking"`, or `type == "thought"`.
   - Pruned in-place using two-pointer slice filtering (`w <= r`).
2. **OpenAI / DeepSeek / OpenRouter format**:
   - Top-level message properties: `"reasoning_content"` and `"reasoning"`.
   - Deleted from the message map.
3. **Inline `<think>` format**:
   - Assistant text containing `<think>...</think>`.
   - Stripped using in-place enclosed tag removal.

---

## 3. Protocol & Safety Invariants

1. **Protocol RFC Compliance (Anthropic HTTP 400 Guard)**:
   - If an assistant message contains *only* reasoning blocks and no tool calls or text, removing them leaves `content: []`.
   - Anthropic Messages API strictly rejects `content: []` with HTTP 400 (`messages.N.content: Input should have at least 1 item`).
   - **Safeguard**: If `len(contentList) == 0` after pruning, inject fallback text block: `[{"type": "text", "text": "Proceeding with next step."}]`.
2. **Never Delete Messages**:
   - Retain the assistant message structure to preserve alternating `user -> assistant` role sequences.
3. **Tool Call Integrity**:
   - `tool_use` blocks (and OpenAI `tool_calls`) are 100% untouched. Tool IDs, tool names, and arguments remain byte-for-byte identical so subsequent `tool_result` / `tool` blocks always resolve.
4. **Role Immunity**:
   - User prompts, tool results, system prompts, and developer prompts are strictly ignored.
5. **File Read/Write Immunity**:
   - `compact_stale_file_reads` remains `false`.
   - `preserve_file_reads` and `preserve_file_writes` remain `true`.
   - File contents and diffs are never pruned.
6. **Zero Allocation**:
   - In-place slice compaction (`w <= r`).

---

## 4. Implementation Changes

### 4.1 Schema & Configuration
- **`pkg/contract/interfaces.go`**:
  ```go
  type NTSConfig struct {
      ...
      StripHistoricalReasoning *bool `yaml:"strip_historical_reasoning,omitempty" json:"strip_historical_reasoning,omitempty"`
  }
  ```
- **`pkg/nts/types.go`**:
  ```go
  type Config struct {
      ...
      StripHistoricalReasoning bool `json:"strip_historical_reasoning" yaml:"strip_historical_reasoning"`
  }

  func DefaultConfig() Config {
      return Config{
          ...
          CompactStaleFileReads:    false, // Proven unsafe in Run 14
          StripHistoricalReasoning: true,  // Safe: dead internal monologue
      }
  }
  ```

### 4.2 Compaction Logic in `pkg/nts/transformer.go`
```go
// In processMessages():
if role, _ := msg["role"].(string); role == "assistant" && t.pipeline.config.StripHistoricalReasoning {
    // 1. Delete top-level reasoning fields
    for _, field := range []string{"reasoning_content", "reasoning"} {
        if val, exists := msg[field]; exists {
            if s, ok := val.(string); ok {
                bSaved := len(s)
                total.BytesSaved += bSaved
                total.TokensSaved += (bSaved + 3) / 4
            }
            delete(msg, field)
            modified = true
        }
    }

    // 2. Filter polymorphic content blocks
    if contentList, ok := msg["content"].([]interface{}); ok {
        w := 0
        for r := 0; r < len(contentList); r++ {
            part, ok := contentList[r].(map[string]interface{})
            if !ok {
                contentList[w] = contentList[r]
                w++
                continue
            }

            pType, _ := part["type"].(string)
            if isReasoningType(pType) {
                bSaved := calculateBlockBytes(part)
                total.BytesSaved += bSaved
                total.TokensSaved += (bSaved + 3) / 4
                modified = true
                continue
            }

            contentList[w] = contentList[r]
            w++
        }
        for i := w; i < len(contentList); i++ {
            contentList[i] = nil
        }
        contentList = contentList[:w]

        // Anthropic RFC safeguard: content cannot be empty array
        if len(contentList) == 0 {
            contentList = []interface{}{
                map[string]interface{}{
                    "type": "text",
                    "text": "Proceeding with next step.",
                },
            }
            modified = true
        }
        msg["content"] = contentList
    }
    continue
}
```

### 4.3 Proxy Wiring & Profiles
- **`pkg/server/proxy.go`**: Map `StripHistoricalReasoning` in `BuildNTSTransformer`.
- **`profiles/profile1.yaml`**, **`profile2.yaml`**, **`profile3.yaml`**, **`pkg/config/template.go`**:
  Add `strip_historical_reasoning: true`.

---

## 5. Testing Plan
1. Unit tests in `pkg/nts/transformer_test.go`:
   - Anthropic `type: "reasoning"` content blocks.
   - Anthropic `type: "thinking"` content blocks.
   - OpenAI `reasoning_content` top-level field.
   - Mixed blocks: `reasoning` stripped while `tool_use` and `text` are preserved.
   - Empty content fallback: assistant message with only reasoning becomes `text: "Proceeding with next step."`.
   - Tool result immunity: file reads and command outputs are untouched.
   - Trace replay test against the 1.76 MB Spice `api_conversation_history.json`.
2. Full test suite validation: `go test -v -count=1 ./pkg/nts/... ./pkg/server/...`.
