package nts_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/nts"
)

// TestTransformer_StripReasoning_AnthropicBlocks verifies that historical assistant
// content blocks with type "thinking", "redacted_thinking", "thought", and "reasoning"
// are pruned, while "tool_use" and "text" blocks are preserved.
func TestTransformer_StripReasoning_AnthropicBlocks(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 2

	tr := nts.NewTransformer(cfg)

	// 4 assistant turns:
	// Turn 1 (historical): thinking + text
	// Turn 2 (historical): redacted_thinking + thought + reasoning + tool_use
	// Turn 3 (recent): thinking + text
	// Turn 4 (recent): thinking + text
	reqJSON := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "turn 1"},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "deep calculation turn 1", "signature": "sig1"},
					{"type": "text", "text": "Response 1"}
				]
			},
			{"role": "user", "content": "turn 2"},
			{
				"role": "assistant",
				"content": [
					{"type": "redacted_thinking", "data": "redacted123"},
					{"type": "thought", "text": "old thought block"},
					{"type": "reasoning", "reasoning": "old reasoning block"},
					{"type": "tool_use", "id": "call_1", "name": "run_cmd", "input": {"cmd": "ls"}}
				]
			},
			{"role": "user", "content": "turn 3"},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "recent thinking 3", "signature": "sig3"},
					{"type": "text", "text": "Response 3"}
				]
			},
			{"role": "user", "content": "turn 4"},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "recent thinking 4", "signature": "sig4"},
					{"type": "text", "text": "Response 4"}
				]
			}
		]
	}`

	compacted, res, err := tr.TransformAnthropic([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected compaction not to be bypassed")
	}
	if res.BytesSaved <= 0 || res.TokensSaved <= 0 {
		t.Fatalf("expected positive bytes and tokens saved, got bytes=%d tokens=%d", res.BytesSaved, res.TokensSaved)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(compacted, &parsed); err != nil {
		t.Fatalf("failed to unmarshal compacted JSON: %v", err)
	}

	msgs := parsed["messages"].([]interface{})
	if len(msgs) != 8 {
		t.Fatalf("expected 8 messages preserving envelope structure, got %d", len(msgs))
	}

	// Assistant Turn 1 (msgs[1]): thinking should be stripped, text preserved
	asst1 := msgs[1].(map[string]interface{})
	blocks1 := asst1["content"].([]interface{})
	if len(blocks1) != 1 {
		t.Fatalf("expected 1 surviving block in asst 1, got %d: %v", len(blocks1), blocks1)
	}
	b1 := blocks1[0].(map[string]interface{})
	if b1["type"] != "text" || b1["text"] != "Response 1" {
		t.Errorf("expected text block 'Response 1', got: %v", b1)
	}

	// Assistant Turn 2 (msgs[3]): redacted_thinking, thought, reasoning stripped; tool_use preserved
	asst2 := msgs[3].(map[string]interface{})
	blocks2 := asst2["content"].([]interface{})
	if len(blocks2) != 1 {
		t.Fatalf("expected 1 surviving block in asst 2, got %d: %v", len(blocks2), blocks2)
	}
	b2 := blocks2[0].(map[string]interface{})
	if b2["type"] != "tool_use" || b2["id"] != "call_1" {
		t.Errorf("expected tool_use block preserved, got: %v", b2)
	}

	// Assistant Turn 3 (msgs[5]): recent turn, thinking preserved intact
	asst3 := msgs[5].(map[string]interface{})
	blocks3 := asst3["content"].([]interface{})
	if len(blocks3) != 2 {
		t.Fatalf("expected 2 blocks in recent asst 3, got %d", len(blocks3))
	}
	if blocks3[0].(map[string]interface{})["type"] != "thinking" {
		t.Errorf("expected recent thinking block intact in asst 3")
	}

	// Assistant Turn 4 (msgs[7]): recent turn, thinking preserved intact
	asst4 := msgs[7].(map[string]interface{})
	blocks4 := asst4["content"].([]interface{})
	if len(blocks4) != 2 {
		t.Fatalf("expected 2 blocks in recent asst 4, got %d", len(blocks4))
	}
	if blocks4[0].(map[string]interface{})["type"] != "thinking" {
		t.Errorf("expected recent thinking block intact in asst 4")
	}
}

// TestTransformer_StripReasoning_OpenAIField verifies deletion of reasoning_content,
// reasoning, and reason fields in historical assistant messages.
func TestTransformer_StripReasoning_OpenAIField(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 1

	tr := nts.NewTransformer(cfg)

	reqJSON := `{
		"model": "deepseek/deepseek-r1",
		"messages": [
			{"role": "user", "content": "Question 1"},
			{
				"role": "assistant",
				"content": "Answer 1",
				"reasoning_content": "Extensive 2000-token scratchpad derivation...",
				"reasoning": "secondary reasoning field",
				"reason": "tertiary reason field"
			},
			{"role": "user", "content": "Question 2"},
			{
				"role": "assistant",
				"content": "Answer 2",
				"reasoning_content": "Recent scratchpad to keep"
			}
		]
	}`

	compacted, res, err := tr.TransformOpenAI([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed || res.BytesSaved <= 0 {
		t.Fatalf("expected compaction with bytes saved, got: %+v", res)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(compacted, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	msgs := parsed["messages"].([]interface{})
	asst1 := msgs[1].(map[string]interface{})

	if _, exists := asst1["reasoning_content"]; exists {
		t.Errorf("expected reasoning_content deleted from historical turn 1")
	}
	if _, exists := asst1["reasoning"]; exists {
		t.Errorf("expected reasoning deleted from historical turn 1")
	}
	if _, exists := asst1["reason"]; exists {
		t.Errorf("expected reason deleted from historical turn 1")
	}
	if asst1["content"] != "Answer 1" {
		t.Errorf("expected content 'Answer 1' preserved, got: %v", asst1["content"])
	}

	// Recent turn 2 must retain reasoning_content
	asst2 := msgs[3].(map[string]interface{})
	if rc, exists := asst2["reasoning_content"]; !exists || rc != "Recent scratchpad to keep" {
		t.Errorf("expected reasoning_content preserved on recent turn 2, got: %v", rc)
	}
}

// TestTransformer_StripReasoning_InlineTags verifies inline thinking tags
// (<think>...</think>, <thinking>...</thinking>, etc.) in content strings are stripped.
func TestTransformer_StripReasoning_InlineTags(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 0 // Strip all historical assistant turns

	tr := nts.NewTransformer(cfg)

	reqJSON := `{
		"model": "deepseek-r1",
		"messages": [
			{"role": "user", "content": "Q1"},
			{"role": "assistant", "content": "<think>Internal deliberation</think>Here is the plan."},
			{"role": "user", "content": "Q2"},
			{"role": "assistant", "content": "<thinking>Alternative tags</thinking>Step 2."},
			{"role": "user", "content": "Q3"},
			{"role": "assistant", "content": "<|channel|>thought\nChannel tags\n<|channel|>Action 3."}
		]
	}`

	compacted, res, err := tr.TransformOpenAI([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed || res.BytesSaved <= 0 {
		t.Fatalf("expected bytes saved from inline thinking stripping")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(compacted, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	msgs := parsed["messages"].([]interface{})
	a1 := msgs[1].(map[string]interface{})["content"].(string)
	if strings.Contains(a1, "<think>") || strings.Contains(a1, "Internal deliberation") {
		t.Errorf("expected thinking stripped from a1, got: %q", a1)
	}
	if !strings.Contains(a1, "Here is the plan.") {
		t.Errorf("expected prose preserved in a1, got: %q", a1)
	}

	a2 := msgs[3].(map[string]interface{})["content"].(string)
	if strings.Contains(a2, "Alternative tags") {
		t.Errorf("expected thinking stripped from a2, got: %q", a2)
	}
	if !strings.Contains(a2, "Step 2.") {
		t.Errorf("expected prose preserved in a2, got: %q", a2)
	}

	a3 := msgs[5].(map[string]interface{})["content"].(string)
	if strings.Contains(a3, "Channel tags") {
		t.Errorf("expected channel tags stripped from a3, got: %q", a3)
	}
	if !strings.Contains(a3, "Action 3.") {
		t.Errorf("expected prose preserved in a3, got: %q", a3)
	}
}

// TestTransformer_KeepRecentReasoningTurns_Zero verifies that setting KeepRecentReasoningTurns = 0
// strips reasoning across all assistant turns.
func TestTransformer_KeepRecentReasoningTurns_Zero(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 0

	tr := nts.NewTransformer(cfg)

	reqJSON := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "1"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t1"}, {"type": "text", "text": "r1"}]},
			{"role": "user", "content": "2"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t2"}, {"type": "text", "text": "r2"}]}
		]
	}`

	compacted, res, err := tr.TransformAnthropic([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected compaction")
	}

	var parsed map[string]interface{}
	json.Unmarshal(compacted, &parsed)
	msgs := parsed["messages"].([]interface{})

	// Both turns should have thinking stripped
	for _, idx := range []int{1, 3} {
		asst := msgs[idx].(map[string]interface{})
		blocks := asst["content"].([]interface{})
		for _, b := range blocks {
			part := b.(map[string]interface{})
			if part["type"] == "thinking" {
				t.Errorf("turn %d still has thinking block", idx)
			}
		}
	}
}

// TestTransformer_KeepRecentReasoningTurns_Cutoff verifies exact sliding window cutoff calculation.
func TestTransformer_KeepRecentReasoningTurns_Cutoff(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 3

	tr := nts.NewTransformer(cfg)

	// 5 assistant turns: indices 0, 1 pruned; 2, 3, 4 kept intact
	var msgs []map[string]interface{}
	for i := 1; i <= 5; i++ {
		msgs = append(msgs, map[string]interface{}{
			"role":    "user",
			"content": "user",
		})
		msgs = append(msgs, map[string]interface{}{
			"role":              "assistant",
			"content":           "prose",
			"reasoning_content": "thinking scratchpad",
		})
	}

	payload := map[string]interface{}{
		"model":    "gpt-4o",
		"messages": msgs,
	}
	body, _ := json.Marshal(payload)

	compacted, res, err := tr.TransformOpenAI(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected compaction")
	}

	var parsed map[string]interface{}
	json.Unmarshal(compacted, &parsed)
	outMsgs := parsed["messages"].([]interface{})

	// Assistant turns are at odd indices: 1, 3, 5, 7, 9
	// Indices 1 (turn 0) and 3 (turn 1) must have reasoning_content deleted
	for _, idx := range []int{1, 3} {
		m := outMsgs[idx].(map[string]interface{})
		if _, exists := m["reasoning_content"]; exists {
			t.Errorf("expected reasoning_content deleted at index %d", idx)
		}
	}

	// Indices 5 (turn 2), 7 (turn 3), 9 (turn 4) must retain reasoning_content
	for _, idx := range []int{5, 7, 9} {
		m := outMsgs[idx].(map[string]interface{})
		if _, exists := m["reasoning_content"]; !exists {
			t.Errorf("expected reasoning_content retained at index %d", idx)
		}
	}
}

// TestTransformer_StrictAssistantRoleFiltering verifies that interleaved tool and user messages
// do not distort the count of assistant turns.
func TestTransformer_StrictAssistantRoleFiltering(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 2

	tr := nts.NewTransformer(cfg)

	// Flow:
	// User
	// Assistant 1 (historical - prune)
	// Tool call result 1
	// Tool call result 2
	// User
	// Assistant 2 (historical - prune)
	// Tool call result 3
	// Assistant 3 (recent - keep)
	// Tool call result 4
	// Assistant 4 (recent - keep)
	reqJSON := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "u1"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t1"}, {"type": "text", "text": "a1"}]},
			{"role": "tool", "tool_call_id": "call1", "content": "res1"},
			{"role": "tool", "tool_call_id": "call2", "content": "res2"},
			{"role": "user", "content": "u2"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t2"}, {"type": "text", "text": "a2"}]},
			{"role": "tool", "tool_call_id": "call3", "content": "res3"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t3"}, {"type": "text", "text": "a3"}]},
			{"role": "tool", "tool_call_id": "call4", "content": "res4"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t4"}, {"type": "text", "text": "a4"}]}
		]
	}`

	compacted, _, err := tr.TransformAnthropic([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal(compacted, &parsed)
	msgs := parsed["messages"].([]interface{})

	// Asst 1 (index 1): thinking pruned
	a1 := msgs[1].(map[string]interface{})["content"].([]interface{})
	if len(a1) != 1 || a1[0].(map[string]interface{})["type"] != "text" {
		t.Errorf("asst 1 thinking not pruned: %v", a1)
	}

	// Asst 2 (index 5): thinking pruned
	a2 := msgs[5].(map[string]interface{})["content"].([]interface{})
	if len(a2) != 1 || a2[0].(map[string]interface{})["type"] != "text" {
		t.Errorf("asst 2 thinking not pruned: %v", a2)
	}

	// Asst 3 (index 7): thinking kept
	a3 := msgs[7].(map[string]interface{})["content"].([]interface{})
	if len(a3) != 2 || a3[0].(map[string]interface{})["type"] != "thinking" {
		t.Errorf("asst 3 thinking not kept: %v", a3)
	}

	// Asst 4 (index 9): thinking kept
	a4 := msgs[9].(map[string]interface{})["content"].([]interface{})
	if len(a4) != 2 || a4[0].(map[string]interface{})["type"] != "thinking" {
		t.Errorf("asst 4 thinking not kept: %v", a4)
	}
}

// TestTransformer_EmptyContentFallback_AnthropicRFC400 verifies that if all content blocks
// in an assistant message are pruned, a fallback text block is injected to avoid Anthropic RFC 400 errors.
func TestTransformer_EmptyContentFallback_AnthropicRFC400(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 0

	tr := nts.NewTransformer(cfg)

	reqJSON := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "Question"},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "only thinking block here", "signature": "sig123"}
				]
			},
			{"role": "user", "content": "Next question"}
		]
	}`

	compacted, res, err := tr.TransformAnthropic([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected compaction")
	}

	var parsed map[string]interface{}
	json.Unmarshal(compacted, &parsed)
	msgs := parsed["messages"].([]interface{})
	asst := msgs[1].(map[string]interface{})
	blocks := asst["content"].([]interface{})

	if len(blocks) == 0 {
		t.Fatalf("empty content blocks! Violates Anthropic RFC 400")
	}
	b0 := blocks[0].(map[string]interface{})
	if b0["type"] != "text" {
		t.Errorf("expected fallback block type text, got %v", b0["type"])
	}
	if text, _ := b0["text"].(string); strings.TrimSpace(text) == "" {
		t.Errorf("expected non-empty fallback text")
	}
}

// TestTransformer_EmptyContentFallback_StringContent verifies fallback text injection
// when a string content becomes empty after stripping inline think tags.
func TestTransformer_EmptyContentFallback_StringContent(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 0

	tr := nts.NewTransformer(cfg)

	reqJSON := `{
		"model": "deepseek-r1",
		"messages": [
			{"role": "user", "content": "Q"},
			{"role": "assistant", "content": "<think>pure thought with no prose</think>"}
		]
	}`

	compacted, res, err := tr.TransformOpenAI([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected compaction")
	}

	var parsed map[string]interface{}
	json.Unmarshal(compacted, &parsed)
	msgs := parsed["messages"].([]interface{})
	asst := msgs[1].(map[string]interface{})
	content, _ := asst["content"].(string)

	if strings.TrimSpace(content) == "" {
		t.Errorf("expected fallback text when content string is stripped empty, got empty string")
	}
}

// TestTransformer_PreserveCacheControl_Migration verifies that cache_control on a pruned
// thinking block is migrated to the next surviving block or to the fallback block.
func TestTransformer_PreserveCacheControl_Migration(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 0

	tr := nts.NewTransformer(cfg)

	// Scenario A: Thinking block has cache_control, followed by text block
	reqJSONA := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "Q"},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "thought", "cache_control": {"type": "ephemeral"}},
					{"type": "text", "text": "Prose"}
				]
			}
		]
	}`

	compactedA, _, err := tr.TransformAnthropic([]byte(reqJSONA))
	if err != nil {
		t.Fatalf("scenario A error: %v", err)
	}
	var parsedA map[string]interface{}
	json.Unmarshal(compactedA, &parsedA)
	msgsA := parsedA["messages"].([]interface{})
	blocksA := msgsA[1].(map[string]interface{})["content"].([]interface{})

	if len(blocksA) != 1 {
		t.Fatalf("expected 1 surviving block, got %d", len(blocksA))
	}
	bA := blocksA[0].(map[string]interface{})
	if bA["type"] != "text" || bA["cache_control"] == nil {
		t.Errorf("expected cache_control migrated to surviving text block, got: %v", bA)
	}

	// Scenario B: Thinking block was the ONLY block and had cache_control
	reqJSONB := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "Q"},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "only block", "cache_control": {"type": "ephemeral"}}
				]
			}
		]
	}`

	compactedB, _, err := tr.TransformAnthropic([]byte(reqJSONB))
	if err != nil {
		t.Fatalf("scenario B error: %v", err)
	}
	var parsedB map[string]interface{}
	json.Unmarshal(compactedB, &parsedB)
	msgsB := parsedB["messages"].([]interface{})
	blocksB := msgsB[1].(map[string]interface{})["content"].([]interface{})

	if len(blocksB) != 1 {
		t.Fatalf("expected 1 fallback block, got %d", len(blocksB))
	}
	bB := blocksB[0].(map[string]interface{})
	if bB["type"] != "text" || bB["cache_control"] == nil {
		t.Errorf("expected cache_control preserved on fallback block, got: %v", bB)
	}
}

// TestTransformer_AlternatingRole_Integrity verifies assistant envelopes are never deleted.
func TestTransformer_AlternatingRole_Integrity(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 0

	tr := nts.NewTransformer(cfg)

	reqJSON := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "u1"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t1"}]},
			{"role": "user", "content": "u2"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t2"}]}
		]
	}`

	compacted, _, err := tr.TransformAnthropic([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal(compacted, &parsed)
	msgs := parsed["messages"].([]interface{})

	if len(msgs) != 4 {
		t.Fatalf("expected exactly 4 messages, got %d (assistant message was incorrectly deleted)", len(msgs))
	}
	for i, expectedRole := range []string{"user", "assistant", "user", "assistant"} {
		actualRole := msgs[i].(map[string]interface{})["role"]
		if actualRole != expectedRole {
			t.Errorf("message %d: expected role %s, got %v", i, expectedRole, actualRole)
		}
	}
}

// TestTransformer_StripHistoricalReasoning_Disabled verifies that when
// StripHistoricalReasoning is false, reasoning is untouched.
func TestTransformer_StripHistoricalReasoning_Disabled(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = false
	cfg.KeepRecentReasoningTurns = 0

	tr := nts.NewTransformer(cfg)

	reqJSON := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "u1"},
			{"role": "assistant", "content": [{"type": "thinking", "thinking": "t1"}, {"type": "text", "text": "r1"}]}
		]
	}`

	compacted, res, err := tr.TransformAnthropic([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Bypassed {
		t.Fatalf("expected bypassed when StripHistoricalReasoning is false and no tool outputs present")
	}

	var parsed map[string]interface{}
	json.Unmarshal(compacted, &parsed)
	msgs := parsed["messages"].([]interface{})
	blocks := msgs[1].(map[string]interface{})["content"].([]interface{})
	if len(blocks) != 2 {
		t.Errorf("expected 2 blocks untouched, got %d", len(blocks))
	}
}

// TestTransformer_ReasoningCompaction_EdgeCases covers branch variations:
// unclosed think tags, stray closing tags, custom JSON reasoning structures,
// content blocks with mixed non-map items, and assistant messages with tool_calls.
func TestTransformer_ReasoningCompaction_EdgeCases(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 0

	tr := nts.NewTransformer(cfg)

	// Payload with edge cases:
	// Turn 1: content string with unclosed think tag and prefix stray closing tag
	// Turn 2: reasoning field is a JSON object map; assistant has tool_calls with empty content
	// Turn 3: content is a slice with a non-map element and a block without standard fields
	// Turn 4: content with pure delimiters and no think tags
	reqJSON := `{
		"model": "deepseek-r1",
		"messages": [
			{"role": "user", "content": "u1"},
			{"role": "assistant", "content": "stray </think> prefix <think>unclosed thought to end"},
			{"role": "user", "content": "u2"},
			{
				"role": "assistant",
				"reasoning_content": {"complex": "data"},
				"tool_calls": [{"id": "c1", "type": "function", "function": {"name": "test"}}]
			},
			{"role": "user", "content": "u3"},
			{
				"role": "assistant",
				"content": [
					"non-map primitive item",
					{"type": "thinking", "custom_field": 12345},
					{"type": "text", "text": "normal prose"}
				]
			},
			{"role": "user", "content": "u4"},
			{"role": "assistant", "content": "<start_of_turn>clean prose with delimiters<end_of_turn>"}
		]
	}`

	compacted, res, err := tr.TransformOpenAI([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed || res.BytesSaved <= 0 {
		t.Fatalf("expected compaction with bytes saved")
	}

	var parsed map[string]interface{}
	json.Unmarshal(compacted, &parsed)
	msgs := parsed["messages"].([]interface{})

	// Turn 1 (msgs[1]): unclosed think tag should be pruned; prefix preserved
	a1 := msgs[1].(map[string]interface{})["content"].(string)
	if strings.Contains(a1, "unclosed thought") {
		t.Errorf("expected unclosed thought pruned, got: %q", a1)
	}
	if !strings.Contains(a1, "stray") || !strings.Contains(a1, "prefix") {
		t.Errorf("expected prefix preserved, got: %q", a1)
	}

	// Turn 2 (msgs[3]): complex object reasoning_content deleted, tool_calls kept, content should remain empty
	a2 := msgs[3].(map[string]interface{})
	if _, exists := a2["reasoning_content"]; exists {
		t.Errorf("expected complex reasoning_content deleted")
	}
	if _, exists := a2["tool_calls"]; !exists {
		t.Errorf("expected tool_calls preserved")
	}

	// Turn 3 (msgs[5]): slice with non-map primitive, custom thinking block pruned, normal prose kept
	a3Blocks := msgs[5].(map[string]interface{})["content"].([]interface{})
	if len(a3Blocks) != 2 {
		t.Fatalf("expected 2 surviving blocks in a3, got %d: %v", len(a3Blocks), a3Blocks)
	}
	if a3Blocks[0] != "non-map primitive item" {
		t.Errorf("expected non-map primitive preserved, got: %v", a3Blocks[0])
	}

	// Turn 4 (msgs[7]): delimiters stripped
	a4 := msgs[7].(map[string]interface{})["content"].(string)
	if strings.Contains(a4, "<start_of_turn>") || strings.Contains(a4, "<end_of_turn>") {
		t.Errorf("expected delimiters stripped from a4, got: %q", a4)
	}
	if !strings.Contains(a4, "clean prose with delimiters") {
		t.Errorf("expected prose preserved in a4, got: %q", a4)
	}
}

// TestTransformer_CleanPayload_FastBypass verifies that a clean user message payload
// is bypassed immediately without heap mutations.
func TestTransformer_CleanPayload_FastBypass(t *testing.T) {
	cfg := nts.DefaultConfig()
	tr := nts.NewTransformer(cfg)

	cleanPayload := []byte(`{"model": "gpt-4o", "messages": [{"role": "user", "content": "Hello world"}]}`)

	compacted, res, err := tr.TransformOpenAI(cleanPayload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Bypassed || res.BypassReason != "no_candidates" {
		t.Errorf("expected no_candidates bypass, got: %+v", res)
	}
	if len(compacted) != len(cleanPayload) {
		t.Errorf("expected untouched byte slice")
	}
}

// BenchmarkNTS_CleanPayload_ZeroAlloc benchmarks the fast bypass path on clean request payloads.
// Invariant: strictly 0 B/op, 0 allocs/op.
func BenchmarkNTS_CleanPayload_ZeroAlloc(b *testing.B) {
	cfg := nts.DefaultConfig()
	tr := nts.NewTransformer(cfg)

	cleanPayload := []byte(`{"model": "gpt-4o", "messages": [{"role": "user", "content": "How do I optimize mutex contention in high concurrency Go?"}]}`)

	b.ReportAllocs()

	for b.Loop() {
		out, res, err := tr.TransformOpenAI(cleanPayload)
		if err != nil {
			b.Fatal(err)
		}
		if !res.Bypassed {
			b.Fatal("expected bypassed")
		}
		if len(out) != len(cleanPayload) {
			b.Fatal("expected identical slice")
		}
	}
}

func TestTransformer_InnerJSON_NoHTMLEscaping(t *testing.T) {
	cfg := nts.DefaultConfig()
	tr := nts.NewTransformer(cfg)

	// Tool result that is itself JSON containing HTML chars and ANSI codes
	reqJSON := `{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "run command"},
			{"role": "assistant", "content": null, "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "bash", "arguments": "{}"}}]},
			{"role": "tool", "tool_call_id": "call_1", "content": "[{\"type\":\"text\",\"text\":\"a < b \\u001b[31mred\"}]"}
		],
		"tools": [{"type": "function", "function": {"name": "bash"}}]
	}`

	out, res, err := tr.TransformOpenAI([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected payload to be modified, but was bypassed")
	}

	// Output must NOT contain \u003c inside the JSON string or anywhere else
	if bytes.Contains(out, []byte(`\u003c`)) {
		t.Errorf("TransformOpenAI introduced HTML escape \\u003c into output: %s", string(out))
	}

	// Decode the transformed message to ensure inner tool content contains "a < b"
	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("failed unmarshaling output: %v", err)
	}

	var toolContent string
	for _, m := range parsed.Messages {
		if m.Role == "tool" {
			toolContent = m.Content
			break
		}
	}
	if strings.Contains(toolContent, `\u003c`) {
		t.Errorf("Tool content contains literal \\u003c: %s", toolContent)
	}
	if !strings.Contains(toolContent, "a < b") {
		t.Errorf("Tool content does not contain 'a < b': %s", toolContent)
	}
}

func TestTransformer_EditedTurn_NoHTMLEscaping(t *testing.T) {
	cfg := nts.DefaultConfig()
	tr := nts.NewTransformer(cfg)

	// User message with HTML chars, followed by tool output with ANSI codes
	reqJSON := `{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "if (x < y && y > z) { return true; }"},
			{"role": "assistant", "content": null, "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "bash", "arguments": "{}"}}]},
			{"role": "tool", "tool_call_id": "call_1", "content": "output with \u001b[32mgreen\u001b[0m"}
		],
		"tools": [{"type": "function", "function": {"name": "bash"}}]
	}`

	out, res, err := tr.TransformOpenAI([]byte(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected payload to be modified, but was bypassed")
	}

	if bytes.Contains(out, []byte(`\u003c`)) || bytes.Contains(out, []byte(`\u003e`)) || bytes.Contains(out, []byte(`\u0026`)) {
		t.Errorf("TransformOpenAI introduced HTML escape sequences: %s", string(out))
	}
	if !bytes.Contains(out, []byte("x < y && y > z")) {
		t.Errorf("TransformOpenAI did not preserve literal HTML characters in user content: %s", string(out))
	}
}

func TestTransformer_PreservesToolsAndUnchangedMessages(t *testing.T) {
	cfg := nts.DefaultConfig()
	tr := nts.NewTransformer(cfg)

	// Tools defined in specific order with nested objects
	toolsJSON := `[{"name":"z_tool","description":"desc z"},{"name":"a_tool","description":"desc a"}]`
	reqJSON := `{"seed":9007199254740993,"tools":` + toolsJSON + `,"messages":[{"role":"user","content":"keep this unmodified"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"z_tool","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"strip \u001b[31mcolor\u001b[0m"},{"role":"user","content":"second user message"}]}`

	in := []byte(reqJSON)
	inCopy := append([]byte(nil), in...)

	out, res, err := tr.TransformOpenAI(in)
	if err != nil {
		t.Fatalf("TransformOpenAI error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected modification")
	}

	// 1. tools array must be byte-identical
	if !bytes.Contains(out, []byte(toolsJSON)) {
		t.Errorf("tools array was not byte-identical to input: %s", string(out))
	}

	// 2. Unchanged messages must be byte-identical
	if !bytes.Contains(out, []byte(`{"role":"user","content":"keep this unmodified"}`)) {
		t.Errorf("unchanged first message was re-encoded: %s", string(out))
	}
	if !bytes.Contains(out, []byte(`{"role":"user","content":"second user message"}`)) {
		t.Errorf("unchanged second message was re-encoded: %s", string(out))
	}

	// 3. Large integer seed > 2^53 must be preserved
	if !bytes.Contains(out, []byte(`"seed":9007199254740993`)) {
		t.Errorf("seed was mutated or reordered: %s", string(out))
	}

	// 4. BytesSaved must match actual wire length difference
	actualDiff := len(inCopy) - len(out)
	if res.BytesSaved != actualDiff {
		t.Errorf("BytesSaved = %d, want exact wire diff %d", res.BytesSaved, actualDiff)
	}

	// 5. Deterministic: running again on same input produces byte-identical output
	in2 := append([]byte(nil), inCopy...)
	out2, _, err2 := tr.TransformOpenAI(in2)
	if err2 != nil || !bytes.Equal(out, out2) {
		t.Errorf("TransformOpenAI was not deterministic")
	}
}

func TestTransformer_GrowthFallback(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 0

	tr := nts.NewTransformer(cfg)

	// Assistant message with empty thinking block is shorter (68 bytes) than the fallback text block (75+ bytes)
	reqJSON := `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":""}]},{"role":"user","content":"next"}]}`

	out, res, err := tr.TransformAnthropic([]byte(reqJSON))
	if err != nil {
		t.Fatalf("TransformAnthropic error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected payload to be modified, but was bypassed")
	}

	if !bytes.Contains(out, []byte("Proceeding with next step.")) {
		t.Errorf("expected fallback text in growth output: %s", string(out))
	}
}

func TestTransformer_ScannerMismatchFallback(t *testing.T) {
	cfg := nts.DefaultConfig()
	tr := nts.NewTransformer(cfg)

	// Duplicate messages key: first has 2 elements, second has 1 element.
	// ObjectField finds the first (len(spans)=2), json.Unmarshal keeps the second (len(msgs)=1).
	reqJSON := `{"messages":[{"role":"user","content":"first"},{"role":"user","content":"second"}],"messages":[{"role":"tool","tool_call_id":"call_1","content":"\u001b[31mstuff\u001b[0m"}]}`

	out, res, err := tr.TransformOpenAI([]byte(reqJSON))
	if err != nil {
		t.Fatalf("TransformOpenAI error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected modification")
	}

	// Should have safely fallen back to wirejson.Marshal without panic
	if !bytes.Contains(out, []byte("stuff")) {
		t.Errorf("expected transformed tool content in output: %s", string(out))
	}
}
