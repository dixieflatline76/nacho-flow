package nts_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
	"github.com/dixieflatline76/nacho-flow/pkg/nts"
	"github.com/dixieflatline76/nacho-flow/pkg/wirejson"
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

func TestTransformer_DeterministicCompaction_WithCacheControl(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.PreserveCacheControl = true
	tr := nts.NewTransformer(cfg)

	// Tool result with ANSI color and cache_control.
	// Deterministic stream passes (ANSI strip, CR resolution, whitespace) must execute
	// so the message is byte-identical whether cache_control is present or absent.
	reqWithCache := `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","cache_control":{"type":"ephemeral"},"content":"\u001b[32mhello world\u001b[0m\r\n"}]}]}`
	reqWithoutCache := `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"\u001b[32mhello world\u001b[0m\r\n"}]}]}`

	outWith, resWith, err := tr.TransformAnthropic([]byte(reqWithCache))
	if err != nil {
		t.Fatalf("TransformAnthropic with cache_control error: %v", err)
	}
	if resWith.Bypassed {
		t.Fatalf("expected payload with cache_control to be modified deterministically")
	}

	outWithout, resWithout, err := tr.TransformAnthropic([]byte(reqWithoutCache))
	if err != nil {
		t.Fatalf("TransformAnthropic without cache_control error: %v", err)
	}
	if resWithout.Bypassed {
		t.Fatalf("expected payload without cache_control to be modified deterministically")
	}

	if !bytes.Contains(outWith, []byte("hello world")) || bytes.Contains(outWith, []byte("\u001b[32m")) {
		t.Errorf("expected ANSI stripped in cached output: %s", string(outWith))
	}
	if !bytes.Contains(outWithout, []byte("hello world")) || bytes.Contains(outWithout, []byte("\u001b[32m")) {
		t.Errorf("expected ANSI stripped in non-cached output: %s", string(outWithout))
	}
}

func TestTransformer_DeduplicateReminders_EndToEnd(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.DeduplicateReminders = true
	cfg.RemindersRefreshInterval = 10
	tr := nts.NewTransformer(cfg)

	env1 := "<environment_details>\n# Current Time\n2026-10-05T12:00:00Z\n\nREMINDERS\n\n| # | Task |\n| 1 | Test |\n</environment_details>"
	env2 := "<environment_details>\n# Current Time\n2026-10-05T12:01:00Z\n\nREMINDERS\n\n| # | Task |\n| 1 | Test |\n</environment_details>"

	req := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": env1},
			map[string]interface{}{"role": "assistant", "content": "I will run the test."},
			map[string]interface{}{"role": "user", "content": env2},
		},
	}
	rawReq, _ := wirejson.Marshal(req)

	out, res, err := tr.TransformAnthropic(rawReq)
	if err != nil {
		t.Fatalf("TransformAnthropic error: %v", err)
	}
	if res.Bypassed {
		t.Fatalf("expected payload to be transformed, but was bypassed")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	msgs := parsed["messages"].([]interface{})
	msg0 := msgs[0].(map[string]interface{})["content"].(string)
	msg2 := msgs[2].(map[string]interface{})["content"].(string)

	// Turn 0 must have full table
	if !strings.Contains(msg0, "| # | Task |") {
		t.Errorf("expected full table in Turn 0: %s", msg0)
	}
	// Turn 2 must have tombstone and NOT full table
	if !strings.Contains(msg2, nts.RemindersUnchangedNotice) {
		t.Errorf("expected tombstone in Turn 2: %s", msg2)
	}
	if strings.Contains(msg2, "| # | Task |") {
		t.Errorf("expected table removed in Turn 2: %s", msg2)
	}

	// Verify no HTML escaping in the output byte stream
	if bytes.Contains(out, []byte(`\u003c`)) || bytes.Contains(out, []byte(`\u003C`)) {
		t.Errorf("output contains HTML escape sequences: %s", string(out))
	}
}

func TestTransformer_DeduplicateToolOutputs_EndToEnd(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.DeduplicateToolOutputs = true
	cfg.MinDuplicateOutputBytes = 200
	tr := nts.NewTransformer(cfg)

	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString(fmt.Sprintf("Step %02d: verified module checksum and package signature successfully\n", i))
	}
	repeatedOutput := sb.String()
	if len(repeatedOutput) < 200 {
		t.Fatalf("expected output >= 200 bytes, got %d", len(repeatedOutput))
	}

	// 1. Anthropic format
	reqAnthropic := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   "call_cmd_1",
						"name": "execute_command",
					},
				},
			},
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "call_cmd_1",
						"tool_name":   "execute_command",
						"content":     repeatedOutput,
					},
				},
			},
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   "call_cmd_2",
						"name": "execute_command",
					},
				},
			},
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "call_cmd_2",
						"tool_name":   "execute_command",
						"content":     repeatedOutput,
					},
				},
			},
		},
	}
	rawAnthropic, _ := wirejson.Marshal(reqAnthropic)

	outAnthropic, resA, errA := tr.TransformAnthropic(rawAnthropic)
	if errA != nil {
		t.Fatalf("TransformAnthropic error: %v", errA)
	}
	if resA.Bypassed {
		t.Fatalf("expected payload to be transformed, but was bypassed")
	}

	var parsedA map[string]interface{}
	if err := json.Unmarshal(outAnthropic, &parsedA); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	msgsA := parsedA["messages"].([]interface{})
	turn1Content := msgsA[1].(map[string]interface{})["content"].([]interface{})[0].(map[string]interface{})["content"].(string)
	turn3Content := msgsA[3].(map[string]interface{})["content"].([]interface{})[0].(map[string]interface{})["content"].(string)

	if !strings.Contains(turn1Content, "Step 00: verified module checksum") {
		t.Errorf("expected first tool execution to preserve output: %s", turn1Content)
	}
	if !strings.Contains(turn3Content, "[Output identical to previous command execution above") {
		t.Errorf("expected second tool execution to be replaced with tombstone: %s", turn3Content)
	}

	// 2. OpenAI format
	reqOpenAI := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call_oa_1",
						"type": "function",
						"function": map[string]interface{}{
							"name": "bash",
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_oa_1",
				"name":         "bash",
				"content":      repeatedOutput,
			},
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call_oa_2",
						"type": "function",
						"function": map[string]interface{}{
							"name": "bash",
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_oa_2",
				"name":         "bash",
				"content":      repeatedOutput,
			},
		},
	}
	rawOpenAI, _ := wirejson.Marshal(reqOpenAI)

	outOpenAI, resO, errO := tr.TransformOpenAI(rawOpenAI)
	if errO != nil {
		t.Fatalf("TransformOpenAI error: %v", errO)
	}
	if resO.Bypassed {
		t.Fatalf("expected OpenAI payload to be transformed, but was bypassed")
	}

	var parsedO map[string]interface{}
	if err := json.Unmarshal(outOpenAI, &parsedO); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	msgsO := parsedO["messages"].([]interface{})
	turn1ContentO := msgsO[1].(map[string]interface{})["content"].(string)
	turn3ContentO := msgsO[3].(map[string]interface{})["content"].(string)

	if !strings.Contains(turn1ContentO, "Step 00: verified module checksum") {
		t.Errorf("expected first tool execution to preserve output: %s", turn1ContentO)
	}
	if !strings.Contains(turn3ContentO, "[Output identical to previous command execution above") {
		t.Errorf("expected second tool execution to be replaced with tombstone: %s", turn3ContentO)
	}
}

func TestTransformer_MultiTurn_CacheInvariance(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.DeduplicateReminders = true
	cfg.RemindersRefreshInterval = 5
	cfg.CollapseVerboseTests = true
	cfg.DeduplicateToolOutputs = true
	cfg.MinDuplicateOutputBytes = 150
	cfg.PreserveCacheControl = true
	tr := nts.NewTransformer(cfg)

	remindersTable := "# REMINDERS\n| # | Rule |\n|---|---|\n| 1 | Never mutate without tests |\n| 2 | Keep KV cache byte-neutral |"
	gitStatusOutput := "On branch main\nYour branch is up to date with 'origin/main'.\n\nChanges not staged for commit:\n\tmodified: pkg/server/dispatch.go\n\tmodified: pkg/nts/transformer.go\nno changes added to commit (use \"git add\" and/or \"git commit -a\")\n"

	cargoTestOutput := `running 4 tests
test board::tests::test_new_board ... ok
test board::tests::test_is_safe ... ok
test board::tests::test_failing ... FAILED
test board::tests::test_ignored ... ignored

failures:

---- board::tests::test_failing stdout ----
thread 'board::tests::test_failing' panicked at 'assertion failed: 5 == 8'

failures:
    board::tests::test_failing

test result: FAILED. 2 passed; 1 failed; 1 ignored; 0 measured; 0 filtered out; finished in 0.05s
`

	var allMessages []map[string]interface{}
	// Turn 0: User initial prompt
	allMessages = append(allMessages, map[string]interface{}{
		"role":    "user",
		"content": fmt.Sprintf("<environment_details>\n# Current Time\n2026-10-05T12:00:00Z\n\n%s\n</environment_details>\nPlease fix the tests.", remindersTable),
	})

	type transformedTurn struct {
		rawMessages [][]byte
	}
	var history []transformedTurn

	const totalTurns = 12
	for k := 1; k <= totalTurns; k++ {
		// Assistant turn
		callID := fmt.Sprintf("call_%d", k)
		toolName := "execute_command"
		if k%2 == 1 {
			// Cargo test
			allMessages = append(allMessages, map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   callID,
						"name": toolName,
					},
				},
			})
			// Tool result turn
			allMessages = append(allMessages, map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": callID,
						"tool_name":   toolName,
						"content":     fmt.Sprintf("<environment_details>\n# Current Time\n2026-10-05T12:%02d:00Z\n\n%s\n</environment_details>\n%s", k, remindersTable, cargoTestOutput),
					},
				},
			})
		} else {
			// Git status (repeating identical output)
			allMessages = append(allMessages, map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   callID,
						"name": toolName,
					},
				},
			})
			allMessages = append(allMessages, map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": callID,
						"tool_name":   toolName,
						"content":     fmt.Sprintf("<environment_details>\n# Current Time\n2026-10-05T12:%02d:00Z\n\n%s\n</environment_details>\n%s", k, remindersTable, gitStatusOutput),
					},
				},
			})
		}

		// Deep copy current messages up to current turn
		currentMsgs := make([]interface{}, len(allMessages))
		for i, m := range allMessages {
			data, _ := json.Marshal(m)
			var copied map[string]interface{}
			_ = json.Unmarshal(data, &copied)
			currentMsgs[i] = copied
		}

		// Set sliding cache breakpoint on the last user message
		lastUserIdx := len(currentMsgs) - 1
		if userMsg, ok := currentMsgs[lastUserIdx].(map[string]interface{}); ok {
			if contentList, ok := userMsg["content"].([]interface{}); ok && len(contentList) > 0 {
				if block, ok := contentList[0].(map[string]interface{}); ok {
					block["cache_control"] = map[string]interface{}{"type": "ephemeral"}
				}
			}
		}

		req := map[string]interface{}{
			"messages": currentMsgs,
		}
		rawReq, _ := wirejson.Marshal(req)

		out, res, err := tr.TransformAnthropic(rawReq)
		if err != nil {
			t.Fatalf("Turn %d TransformAnthropic failed: %v", k, err)
		}
		if res.Bypassed {
			t.Fatalf("Turn %d expected transformation, was bypassed", k)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("Turn %d unmarshal failed: %v", k, err)
		}
		parsedMsgs := parsed["messages"].([]interface{})

		var currentRawMsgs [][]byte
		for _, pm := range parsedMsgs {
			mBytes, _ := wirejson.Marshal(pm)
			currentRawMsgs = append(currentRawMsgs, mBytes)
		}

		// Cache Invariant Check against previous turn
		if len(history) > 0 {
			prevTurn := history[len(history)-1]
			// All historical messages up to len(prevTurn.rawMessages)-1 (excluding the one that had cache_control in prev turn)
			// MUST be byte-for-byte identical!
			prevBreakpointIdx := len(prevTurn.rawMessages) - 1
			for j := 0; j < prevBreakpointIdx; j++ {
				prevBytes := prevTurn.rawMessages[j]
				currBytes := currentRawMsgs[j]
				if !bytes.Equal(prevBytes, currBytes) {
					t.Fatalf("Cache invariance violated between turn %d and turn %d at message index %d:\nPREV: %s\nCURR: %s",
						k-1, k, j, string(prevBytes), string(currBytes))
				}
			}
		}

		history = append(history, transformedTurn{rawMessages: currentRawMsgs})
	}
}

func TestTransformer_IsToolErrorMessage(t *testing.T) {
	cfg := nts.DefaultConfig()
	tr := nts.NewTransformer(cfg)
	reg := agentregistry.DefaultRegistry()
	universalPfx := reg.GetReasoningCompactionConfig().UniversalToolErrorPrefixes
	clineProfile := reg.GetProfile("cline")

	// 1. OpenAI role="tool" with "Error: exit status 1" -> true
	toolMsg := map[string]interface{}{
		"role":    "tool",
		"content": "Error: exit status 1",
	}
	if !tr.IsToolErrorMessage(toolMsg, nil, universalPfx) {
		t.Errorf("expected OpenAI role=tool with Error prefix to be classified as error")
	}

	// 2. Anthropic tool_result (is_error: true + nested blocks) -> true
	anthropicErr1 := map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{
				"type":     "tool_result",
				"is_error": true,
				"content":  "operation aborted",
			},
		},
	}
	if !tr.IsToolErrorMessage(anthropicErr1, nil, universalPfx) {
		t.Errorf("expected Anthropic tool_result with is_error=true to be classified as error")
	}

	anthropicErr2 := map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{
				"type": "tool_result",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": `{"error": "file not found"}`,
					},
				},
			},
		},
	}
	if !tr.IsToolErrorMessage(anthropicErr2, nil, universalPfx) {
		t.Errorf("expected Anthropic nested tool_result with universal error prefix to be classified as error")
	}

	// 3. Cline user text block with "Editor operation failed" -> true
	clineUserMsg := map[string]interface{}{
		"role": "user",
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "Editor operation failed: invalid line offset",
			},
			map[string]interface{}{
				"type": "text",
				"text": "<environment_details>\n# Current Time\n2026-10-15T12:00:00Z\n</environment_details>",
			},
		},
	}
	if !tr.IsToolErrorMessage(clineUserMsg, clineProfile, universalPfx) {
		t.Errorf("expected Cline text block with registered error signature to be classified as error")
	}

	// 4. Human user prompt "Error: please fix my db" -> false
	humanPrompt := map[string]interface{}{
		"role":    "user",
		"content": "Error: please fix my db",
	}
	if tr.IsToolErrorMessage(humanPrompt, clineProfile, universalPfx) {
		t.Errorf("human user prompt starting with 'Error:' must NOT be classified as tool error")
	}

	// 5. 50KB source code read containing `{"error": "unauthorized"}` beyond 4KB -> false
	largeContent := strings.Repeat("// filler code line\n", 300) + `{"error": "unauthorized"}`
	if len(largeContent) < 5000 {
		t.Fatalf("test setup error: largeContent length is %d, expected > 5000", len(largeContent))
	}
	largeFileRead := map[string]interface{}{
		"role":    "tool",
		"content": largeContent,
	}
	if tr.IsToolErrorMessage(largeFileRead, nil, universalPfx) {
		t.Errorf("large file read with error signature beyond 4KB scan window must NOT be classified as error")
	}
}

func TestTransformer_PrecededByToolError(t *testing.T) {
	cfg := nts.DefaultConfig()
	tr := nts.NewTransformer(cfg)
	reg := agentregistry.DefaultRegistry()
	universalPfx := reg.GetReasoningCompactionConfig().UniversalToolErrorPrefixes

	// Case A: Parallel tool calls where one failed
	msgs := []interface{}{
		map[string]interface{}{"role": "user", "content": "do tasks"},
		map[string]interface{}{"role": "assistant", "content": "calling tools"},
		map[string]interface{}{"role": "tool", "content": "Error: command failed"},
		map[string]interface{}{"role": "tool", "content": "success output"},
		map[string]interface{}{"role": "assistant", "content": "handling failure"}, // idx 4
	}

	if !tr.PrecededByToolError(msgs, 4, nil, universalPfx) {
		t.Errorf("expected assistant turn 4 to be preceded by tool error due to parallel failed tool")
	}

	// Case B: Parallel tool calls both succeeded
	msgsClean := []interface{}{
		map[string]interface{}{"role": "user", "content": "do tasks"},
		map[string]interface{}{"role": "assistant", "content": "calling tools"},
		map[string]interface{}{"role": "tool", "content": "success 1"},
		map[string]interface{}{"role": "tool", "content": "success 2"},
		map[string]interface{}{"role": "assistant", "content": "proceeding"}, // idx 4
	}

	if tr.PrecededByToolError(msgsClean, 4, nil, universalPfx) {
		t.Errorf("expected clean tool executions to not be preceded by error")
	}

	// Case C: Error occurred in earlier turn, but intervened by another assistant turn
	msgsIntervened := []interface{}{
		map[string]interface{}{"role": "tool", "content": "Error: old failure"},
		map[string]interface{}{"role": "assistant", "content": "retry"},
		map[string]interface{}{"role": "tool", "content": "success now"},
		map[string]interface{}{"role": "assistant", "content": "final step"}, // idx 3
	}

	if tr.PrecededByToolError(msgsIntervened, 3, nil, universalPfx) {
		t.Errorf("expected scan to stop at previous assistant turn and not see old failure")
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestTransformer_KickoffHighWatermark(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 2
	cfg.HysteresisCompaction = boolPtr(false) // Test raw sliding window retention with kickoff protection

	tr := nts.NewTransformer(cfg)

	// Build 7 turns:
	// Turn 1 (idx 1): 180B recon
	// Turn 2 (idx 3): 150B recon
	// Turn 3 (idx 5): 650B architecture plan (>= 400B kickoff threshold)
	// Turn 4 (idx 7): 250B substantial reasoning
	// Turn 5 (idx 9): 250B substantial reasoning
	// Turn 6 (idx 11): 250B substantial reasoning
	// Turn 7 (idx 13): 250B substantial reasoning
	var msgs []interface{}
	for i := 1; i <= 7; i++ {
		msgs = append(msgs, map[string]interface{}{"role": "user", "content": fmt.Sprintf("step %d", i)})
		var reasoningLen int
		switch i {
		case 1:
			reasoningLen = 180
		case 2:
			reasoningLen = 150
		case 3:
			reasoningLen = 650
		default:
			reasoningLen = 250
		}
		msgs = append(msgs, map[string]interface{}{
			"role":              "assistant",
			"reasoning_content": strings.Repeat("a", reasoningLen),
			"content":           fmt.Sprintf("done step %d", i),
		})
	}

	toPrune, hasAny := tr.IdentifyHistoricalReasoningIndices(msgs, nil)
	if !hasAny {
		t.Fatalf("expected historical reasoning to be pruned")
	}

	turn1Idx := 1
	turn3Idx := 5
	turn6Idx := 11
	turn7Idx := 13

	// Turn 1 (180B recon) must be pruned
	if !toPrune.Has(turn1Idx) {
		t.Errorf("expected Turn 1 (180B recon) to be pruned")
	}

	// Turn 3 (650B plan) must be permanently pinned as Kickoff Anchor
	if toPrune.Has(turn3Idx) {
		t.Errorf("expected Turn 3 (650B plan) to be preserved as Kickoff Anchor, but was pruned")
	}

	// Recent 2 turns (Turn 6 and Turn 7) must be preserved
	if toPrune.Has(turn6Idx) {
		t.Errorf("expected Turn 6 (recent turn) to be preserved")
	}
	if toPrune.Has(turn7Idx) {
		t.Errorf("expected Turn 7 (most recent turn) to be preserved")
	}
}

func TestTransformer_MicroThoughtsDoNotConsumeQuota(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 2
	cfg.HysteresisCompaction = boolPtr(false)

	tr := nts.NewTransformer(cfg)

	// Turn 1 (idx 1): 500B reasoning plan (substantial)
	// Turns 2..6 (idx 3, 5, 7, 9, 11): 5 turns of 60B micro-thoughts (< 200B min_substantial_bytes)
	var msgs []interface{}
	msgs = append(msgs, map[string]interface{}{"role": "user", "content": "initial prompt"})
	msgs = append(msgs, map[string]interface{}{
		"role":              "assistant",
		"reasoning_content": strings.Repeat("p", 500),
		"content":           "Here is the detailed plan.",
	})

	for i := 2; i <= 6; i++ {
		msgs = append(msgs, map[string]interface{}{"role": "user", "content": fmt.Sprintf("step %d", i)})
		msgs = append(msgs, map[string]interface{}{
			"role":              "assistant",
			"reasoning_content": strings.Repeat("m", 60), // micro-thought < 200B
			"content":           fmt.Sprintf("tactical step %d", i),
		})
	}

	toPrune, _ := tr.IdentifyHistoricalReasoningIndices(msgs, nil)

	turn1Idx := 1
	if toPrune.Has(turn1Idx) {
		t.Errorf("Turn 1 substantial plan was pruned by subsequent micro-thoughts; micro-thoughts must not consume retention budget")
	}
}

func TestTransformer_HysteresisCacheStability(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 2
	cfg.HysteresisCompaction = boolPtr(true)

	tr := nts.NewTransformer(cfg)

	// Build 10 substantial assistant turns (each with 300B reasoning)
	var msgs []interface{}
	for i := 1; i <= 10; i++ {
		msgs = append(msgs, map[string]interface{}{"role": "user", "content": fmt.Sprintf("user %d", i)})
		msgs = append(msgs, map[string]interface{}{
			"role":              "assistant",
			"reasoning_content": strings.Repeat("s", 300),
			"content":           fmt.Sprintf("assistant %d", i),
		})
	}

	// Turn 1..5 (messages 0..9): below high watermark (6 substantial turns), prunes nothing!
	for turns := 1; turns <= 5; turns++ {
		subMsgs := msgs[:turns*2]
		toPrune, hasAny := tr.IdentifyHistoricalReasoningIndices(subMsgs, nil)
		if hasAny || !toPrune.IsEmpty() {
			t.Errorf("Turn %d: expected 0 pruned turns below high watermark, got prune set", turns)
		}
	}

	// Turn 6 (messages 0..11): reaches high watermark 6!
	// pruneSubstantialCount = ((6 - 2) / 4) * 4 = 4. Turns 1..4 pruned (idx 1, 3, 5, 7)
	msgsTurn6 := msgs[:12]
	toPrune6, hasAny6 := tr.IdentifyHistoricalReasoningIndices(msgsTurn6, nil)
	if !hasAny6 {
		t.Fatalf("Turn 6: expected compaction at high watermark")
	}

	for _, idx := range []int{1, 3, 5, 7} {
		if !toPrune6.Has(idx) {
			t.Errorf("Turn 6: expected assistant msg at idx %d to be pruned", idx)
		}
	}
	if toPrune6.Has(9) || toPrune6.Has(11) {
		t.Errorf("Turn 6: recent turns 5 and 6 must be preserved")
	}

	// Turns 7, 8, 9 must produce the EXACT same pruned bitset as Turn 6! (100% KV cache stability)
	for turns := 7; turns <= 9; turns++ {
		subMsgs := msgs[:turns*2]
		toPruneN, hasAnyN := tr.IdentifyHistoricalReasoningIndices(subMsgs, nil)
		if !hasAnyN {
			t.Fatalf("Turn %d: expected compaction", turns)
		}
		for _, idx := range []int{1, 3, 5, 7} {
			if !toPruneN.Has(idx) {
				t.Errorf("Turn %d: expected assistant msg at idx %d to remain pruned (prefix invariance)", turns, idx)
			}
		}
		// Any newer turns (turns >= 5, i.e. idx >= 9) must NOT be pruned
		for idx := 9; idx < turns*2; idx += 2 {
			if toPruneN.Has(idx) {
				t.Errorf("Turn %d: assistant msg at idx %d must be preserved", turns, idx)
			}
		}
	}

	// Turn 10: reaches high watermark 10!
	// pruneSubstantialCount = ((10 - 2) / 4) * 4 = 8. Turns 1..8 pruned!
	msgsTurn10 := msgs[:20]
	toPrune10, _ := tr.IdentifyHistoricalReasoningIndices(msgsTurn10, nil)
	for i := 1; i <= 15; i += 2 { // Turns 1..8 are indices 1, 3, 5, 7, 9, 11, 13, 15
		if !toPrune10.Has(i) {
			t.Errorf("Turn 10: expected assistant msg at idx %d to be pruned", i)
		}
	}
	if toPrune10.Has(17) || toPrune10.Has(19) {
		t.Errorf("Turn 10: recent turns 9 and 10 must be preserved")
	}
}

func TestTransformer_ErrorStormAndRecoveryInvariance(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 2
	cfg.HysteresisCompaction = boolPtr(true)
	cfg.FreezeOnToolErrors = boolPtr(true)

	tr := nts.NewTransformer(cfg)

	// Baseline: 6 clean substantial turns
	// User 1 -> Assistant 1 -> Tool 1 -> ... -> User 6 -> Assistant 6
	var msgs []interface{}
	for i := 1; i <= 6; i++ {
		msgs = append(msgs, map[string]interface{}{"role": "user", "content": fmt.Sprintf("user %d", i)})
		msgs = append(msgs, map[string]interface{}{
			"role":              "assistant",
			"reasoning_content": strings.Repeat("c", 300),
			"content":           fmt.Sprintf("assistant %d", i),
		})
	}
	// Assistant 6 is at index 11.
	// Check baseline at Turn 6:
	toPruneBase, _ := tr.IdentifyHistoricalReasoningIndices(msgs, nil)
	for _, idx := range []int{1, 3, 5, 7} {
		if !toPruneBase.Has(idx) {
			t.Fatalf("Baseline Turn 6: expected msg %d to be pruned", idx)
		}
	}

	// Now Turn 6 executes a tool that FAILS!
	msgs = append(msgs, map[string]interface{}{
		"role":    "tool",
		"content": "Error: command failed with exit status 1",
	}) // idx 12

	// Retry Turn 7: Assistant 7 fails again
	msgs = append(msgs, map[string]interface{}{
		"role":              "assistant",
		"reasoning_content": strings.Repeat("r", 300),
		"content":           "retrying with flag --force",
	}) // idx 13
	msgs = append(msgs, map[string]interface{}{
		"role":    "tool",
		"content": "Error: syntax error in script",
	}) // idx 14

	// Check during error streak at Turn 7:
	toPruneErr1, _ := tr.IdentifyHistoricalReasoningIndices(msgs, nil)
	for _, idx := range []int{1, 3, 5, 7} {
		if !toPruneErr1.Has(idx) {
			t.Errorf("Error Streak Turn 7: historical turns 1..4 (idx %d) must stay pruned", idx)
		}
	}
	if toPruneErr1.Has(11) {
		t.Errorf("Error Streak Turn 7: causal setup Turn 6 (idx 11) must be protected")
	}
	if toPruneErr1.Has(13) {
		t.Errorf("Error Streak Turn 7: active retry Turn 7 (idx 13) must be protected")
	}

	// Retry Turn 8: Assistant 8 fails again
	msgs = append(msgs, map[string]interface{}{
		"role":              "assistant",
		"reasoning_content": strings.Repeat("r", 300),
		"content":           "retrying with sudo",
	}) // idx 15
	msgs = append(msgs, map[string]interface{}{
		"role":    "tool",
		"content": "Error: unable to lock database",
	}) // idx 16

	// Check during error streak at Turn 8:
	toPruneErr2, _ := tr.IdentifyHistoricalReasoningIndices(msgs, nil)
	for _, idx := range []int{1, 3, 5, 7} {
		if !toPruneErr2.Has(idx) {
			t.Errorf("Error Streak Turn 8: historical turns 1..4 (idx %d) must stay pruned", idx)
		}
	}
	if toPruneErr2.Has(11) || toPruneErr2.Has(13) || toPruneErr2.Has(15) {
		t.Errorf("Error Streak Turn 8: setup and all retries must be protected")
	}

	// Turn 9: RECOVERY TURN! Assistant 9 succeeds!
	msgs = append(msgs, map[string]interface{}{
		"role":              "assistant",
		"reasoning_content": strings.Repeat("r", 300),
		"content":           "fixing lockfile and rerunning",
	}) // idx 17
	msgs = append(msgs, map[string]interface{}{
		"role":    "tool",
		"content": "success: database updated",
	}) // idx 18

	// Check on RECOVERY TURN:
	toPruneRecovery, _ := tr.IdentifyHistoricalReasoningIndices(msgs, nil)
	for _, idx := range []int{1, 3, 5, 7} {
		if !toPruneRecovery.Has(idx) {
			t.Errorf("Recovery Turn 9: historical turns 1..4 (idx %d) must remain 100%% byte-identical to pre-error baseline", idx)
		}
	}
}

func BenchmarkIdentifyHistoricalReasoningIndices(b *testing.B) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 2
	cfg.HysteresisCompaction = boolPtr(true)
	tr := nts.NewTransformer(cfg)
	reg := agentregistry.DefaultRegistry()
	agentProfile := reg.GetProfile("cline")

	// Construct 60-message fixture OUTSIDE the benchmark loop
	var msgs []interface{}
	for i := 0; i < 30; i++ {
		msgs = append(msgs, map[string]interface{}{
			"role":    "user",
			"content": fmt.Sprintf("do task %d", i),
		})
		msgs = append(msgs, map[string]interface{}{
			"role":              "assistant",
			"reasoning_content": strings.Repeat("thinking step ", 20),
			"content":           fmt.Sprintf("result %d", i),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tr.IdentifyHistoricalReasoningIndices(msgs, agentProfile)
	}
}

func TestTransformer_SetRegistry_Pipeline_ToolError(t *testing.T) {
	cfg := nts.DefaultConfig()
	cfg.StripHistoricalReasoning = true
	cfg.KeepRecentReasoningTurns = 1
	tr := nts.NewTransformer(cfg)
	reg := agentregistry.DefaultRegistry()
	tr.SetRegistry(reg)
	if tr.Pipeline() == nil {
		t.Error("expected non-nil pipeline")
	}

	// OpenAI format with role="tool" and list content containing an error
	req := `{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "run command"},
			{"role": "assistant", "content": "ok"},
			{"role": "tool", "tool_call_id": "call_1", "content": [
				{"type": "text", "text": "Error: command failed with exit code 1"}
			]},
			{"role": "assistant", "reasoning_content": "fixing error step", "content": "fixed"}
		]
	}`
	out, _, err := tr.TransformOpenAI([]byte(req))
	if err != nil {
		t.Fatalf("TransformOpenAI failed: %v", err)
	}
	if !bytes.Contains(out, []byte("fixing error step")) {
		t.Error("expected error-preceded assistant reasoning to be preserved")
	}
}
