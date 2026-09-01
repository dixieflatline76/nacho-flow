package router

import (
	"fmt"
	"strings"
	"testing"
)

func TestEstimateTokensEmptyAndNull(t *testing.T) {
	classifier := NewClassifier()

	// Empty messages array
	json1 := `{"messages": []}`
	ctx1, err1 := classifier.Classify([]byte(json1))
	if err1 != nil {
		t.Fatalf("Unexpected error for empty messages: %v", err1)
	}
	if ctx1.Tokens != 0 {
		t.Errorf("Expected 0 tokens for empty messages, got %d", ctx1.Tokens)
	}

	// Message with empty string content
	json2 := `{"messages": [{"role": "user", "content": ""}]}`
	ctx2, _ := classifier.Classify([]byte(json2))
	if ctx2.Tokens < 0 {
		t.Errorf("Expected tokens >= 0, got %d", ctx2.Tokens)
	}
}

func TestClassifyIntentInMultipartContent(t *testing.T) {
	jsonBody := `{
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "We have an intermittent deadlock in favorites_deadlock_test.go"}
				]
			}
		]
	}`

	classifier := NewClassifier()
	ctx, err := classifier.Classify([]byte(jsonBody))
	if err != nil {
		t.Fatalf("Failed to classify multipart content: %v", err)
	}

	foundDeadlock := false
	for _, k := range ctx.Keywords {
		if k == "deadlock" {
			foundDeadlock = true
		}
	}

	if !foundDeadlock {
		t.Errorf("Expected keyword 'deadlock' in extracted keywords, got: %v", ctx.Keywords)
	}
}

func TestMassiveContextTokenCalculation(t *testing.T) {
	classifier := NewClassifier()

	// Generate 60k token text (~240,000 chars)
	text := fmt.Sprintf("Fix goroutine deadlock in this massive codebase: %s", stringsRepeat("x", 240000))
	jsonBody := fmt.Sprintf(`{"messages": [{"role": "user", "content": "%s"}]}`, text)

	ctx, err := classifier.Classify([]byte(jsonBody))
	if err != nil {
		t.Fatalf("Failed to classify massive context: %v", err)
	}

	if ctx.Tokens < 50000 {
		t.Errorf("Expected tokens > 50000 for 240k chars, got %d", ctx.Tokens)
	}
}

func stringsRepeat(s string, count int) string {
	var b []byte
	for i := 0; i < count; i++ {
		b = append(b, s...)
	}
	return string(b)
}

// Test Classify error on invalid JSON
func TestClassifier_InvalidJSON_ReturnsError(t *testing.T) {
	classifier := NewClassifier()
	_, err := classifier.Classify([]byte("not-json"))
	if err == nil {
		t.Fatalf("Expected error for invalid JSON input, got nil")
	}
}

// Test tools array detection
func TestClassifier_ToolsDetection(t *testing.T) {
	classifier := NewClassifier()
	jsonBody := `{"model": "gpt-4", "tools": [{"type": "function", "function": {"name": "test_func"}}]}`
	ctx, err := classifier.Classify([]byte(jsonBody))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !ctx.HasTools {
		t.Errorf("Expected HasTools to be true for tools array")
	}
}

// Test message array edge cases (non-map messages, missing content, non-map parts)
func TestClassifier_MessageStructureEdgeCases(t *testing.T) {
	classifier := NewClassifier()
	jsonBody := `{
		"model": "gpt-4",
		"messages": [
			"invalid_string_message",
			{"role": "system"},
			{
				"role": "user",
				"content": [
					"invalid_string_part",
					{"type": "image_url"},
					{"type": "text", "text": "hello world"}
				]
			}
		]
	}`

	ctx, err := classifier.Classify([]byte(jsonBody))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !ctx.HasImages {
		t.Errorf("Expected HasImages to be true")
	}
	if ctx.Prompt != "hello world" {
		t.Errorf("Expected prompt 'hello world', got '%s'", ctx.Prompt)
	}
}

func TestClassifier_KeywordsScopedToLatestPrompt(t *testing.T) {
	classifier := NewClassifier()
	jsonBody := `{
		"model": "gpt-4",
		"messages": [
			{"role": "system", "content": "You are an expert postgres database administrator."},
			{"role": "user", "content": "Write a complex SQL migration for users table."},
			{"role": "assistant", "content": "Here is the SQL migration schema."},
			{"role": "user", "content": "Now fix the CSS flexbox styling for the button."}
		]
	}`

	ctx, err := classifier.Classify([]byte(jsonBody))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if ctx.Prompt != "Now fix the CSS flexbox styling for the button." {
		t.Errorf("Expected prompt to be latest user turn, got %q", ctx.Prompt)
	}

	hasCSS := false
	hasFlexbox := false
	hasSQL := false
	hasPostgres := false

	for _, k := range ctx.Keywords {
		if k == "css" {
			hasCSS = true
		}
		if k == "flexbox" {
			hasFlexbox = true
		}
		if k == "sql" {
			hasSQL = true
		}
		if k == "postgres" {
			hasPostgres = true
		}
	}

	if !hasCSS || !hasFlexbox {
		t.Errorf("Expected keywords to contain 'css' and 'flexbox', got: %v", ctx.Keywords)
	}
	if hasSQL || hasPostgres {
		t.Errorf("Keywords should NOT contain previous turn keywords ('sql', 'postgres'), got: %v", ctx.Keywords)
	}
}

func TestClassifier_WithCustomEstimator(t *testing.T) {
	estimator := NewTokenEstimator()
	// Set ratio to 2.0 (2 chars per token)
	estimator.Calibrate(500, 1000) // observed = 2.0 -> updated = 3.2*0.8 + 2.0*0.2 = 2.96
	// Let's calibrate multiple times to converge to 2.0
	for i := 0; i < 20; i++ {
		estimator.Calibrate(500, 1000)
	}

	classifier := NewClassifierWithEstimator(estimator)
	jsonBody := `{"messages": [{"role": "user", "content": "1234567890"}]}`
	ctx, err := classifier.Classify([]byte(jsonBody))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// 56 bytes payload / 2.0 ~= 28 tokens
	if ctx.Tokens < 25 || ctx.Tokens > 32 {
		t.Errorf("Expected ~28 tokens with calibrated estimator, got %d", ctx.Tokens)
	}

	if c, ok := classifier.(*RequestClassifier); !ok || c.GetEstimator() == nil {
		t.Fatalf("expected non-nil estimator")
	}

	// Nil estimator fallback branches
	cNil := NewClassifierWithEstimator(nil)
	if c, ok := cNil.(*RequestClassifier); !ok || c.GetEstimator() == nil {
		t.Fatalf("expected non-nil estimator for nil init")
	}

	cEmpty := &RequestClassifier{}
	if cEmpty.GetEstimator() == nil {
		t.Fatalf("expected lazy non-nil estimator on empty struct")
	}
}

func TestClassifier_InteractiveToolExtraction(t *testing.T) {
	c := NewClassifier()

	// 1. ask_question
	jsonBody := `{"messages":[{"role":"user","content":"test"}],"tools":[{"type":"function","function":{"name":"ask_question"}}]}`
	ctx, _ := c.Classify([]byte(jsonBody))
	if ctx.InteractiveTool != "ask_question" {
		t.Fatalf("expected ask_question, got %s", ctx.InteractiveTool)
	}

	// 2. user_prompt
	jsonBody = `{"messages":[{"role":"user","content":"test"}],"tools":[{"type":"function","function":{"name":"user_prompt"}}]}`
	ctx, _ = c.Classify([]byte(jsonBody))
	if ctx.InteractiveTool != "user_prompt" {
		t.Fatalf("expected user_prompt, got %s", ctx.InteractiveTool)
	}

	// 3. interactive_input
	jsonBody = `{"messages":[{"role":"user","content":"test"}],"tools":[{"name":"interactive_input"}]}`
	ctx, _ = c.Classify([]byte(jsonBody))
	if ctx.InteractiveTool != "interactive_input" {
		t.Fatalf("expected interactive_input, got %s", ctx.InteractiveTool)
	}

	// 4. unsupported tool
	jsonBody = `{"messages":[{"role":"user","content":"test"}],"tools":[{"type":"function","function":{"name":"random_tool"}}]}`
	ctx, _ = c.Classify([]byte(jsonBody))
	if ctx.InteractiveTool != "" {
		t.Fatalf("expected empty interactive tool, got %s", ctx.InteractiveTool)
	}

	// 5. Direct ExtractSupportedInteractiveTool test
	res := ExtractSupportedInteractiveTool([]interface{}{"not-a-map", map[string]interface{}{"invalid": 123}})
	if res != "" {
		t.Fatalf("expected empty result for invalid map, got %s", res)
	}
}

func TestClassifier_ExtractAllTextFromContent_Types(t *testing.T) {
	// 1. Plain string
	if s := extractAllTextFromContent("hello world"); s != "hello world" {
		t.Errorf("expected 'hello world', got %q", s)
	}

	// 2. Multi-part array with text and content keys
	parts := []interface{}{
		map[string]interface{}{"text": "first part"},
		map[string]interface{}{"content": "second part"},
		"not-a-map",
		map[string]interface{}{"other_key": 123},
	}
	extracted := extractAllTextFromContent(parts)
	if !strings.Contains(extracted, "first part") || !strings.Contains(extracted, "second part") {
		t.Errorf("expected multi-part extraction, got %q", extracted)
	}

	// 3. Unsupported types
	if s := extractAllTextFromContent(12345); s != "" {
		t.Errorf("expected empty string for int, got %q", s)
	}
	if s := extractAllTextFromContent(nil); s != "" {
		t.Errorf("expected empty string for nil, got %q", s)
	}
}

func TestClassifier_FeatureFlagsInDirectives(t *testing.T) {
	c := NewClassifier()

	// 1. @nacho:raw
	jsonBody := `{"messages":[{"role":"user","content":"@nacho:raw write code"}]}`
	ctx, _ := c.Classify([]byte(jsonBody))
	if ctx.Features != uint16(FeatureRawPassThrough) {
		t.Fatalf("expected FeatureRawPassThrough, got %d", ctx.Features)
	}

	// 2. @nacho:no-shield
	jsonBody = `{"messages":[{"role":"user","content":"@nacho:no-shield ask me questions"}]}`
	ctx, _ = c.Classify([]byte(jsonBody))
	expected := uint16(FeatureDefaultAll.MaskOut(FeatureShieldEnabled | FeatureShieldFollowup | FeatureShieldModeSwitch))
	if ctx.Features != expected {
		t.Fatalf("expected %d, got %d", expected, ctx.Features)
	}
}

func TestScanTrailingMessages_WriteProgress(t *testing.T) {
	c := NewClassifier().(*RequestClassifier)
	// Explicit tool list — no hidden Go defaults, mirrors config-driven behavior
	configuredTools := []string{"write_to_file", "replace_in_file", "execute_command", "apply_diff"}

	tests := []struct {
		name                 string
		customTools          []string
		body                 string
		wantToolProgress     bool
		wantWriteProgress    bool
		wantHistoryErrors    int
	}{
		{
			name: "OpenAI format: write_to_file tool call and response",
			body: `{
				"messages": [
					{"role": "user", "content": "Write the code"},
					{"role": "assistant", "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "write_to_file", "arguments": "{}"}}]},
					{"role": "tool", "tool_call_id": "call_1", "content": "File written successfully"}
				]
			}`,
			wantToolProgress:  true,
			wantWriteProgress: true,
			wantHistoryErrors: 0,
		},
		{
			name: "OpenAI format: read-only read_file tool call and response",
			body: `{
				"messages": [
					{"role": "user", "content": "Read the code"},
					{"role": "assistant", "tool_calls": [{"id": "call_2", "type": "function", "function": {"name": "read_file", "arguments": "{}"}}]},
					{"role": "tool", "tool_call_id": "call_2", "content": "package main\nfunc main() {}"}
				]
			}`,
			wantToolProgress:  true,
			wantWriteProgress: false,
			wantHistoryErrors: 0,
		},
		{
			name: "OpenAI format: multiple tool calls (read + execute)",
			body: `{
				"messages": [
					{"role": "user", "content": "Check and run"},
					{"role": "assistant", "tool_calls": [
						{"id": "c1", "type": "function", "function": {"name": "read_file"}},
						{"id": "c2", "type": "function", "function": {"name": "execute_command"}}
					]},
					{"role": "tool", "tool_call_id": "c1", "content": "ok"},
					{"role": "tool", "tool_call_id": "c2", "content": "tests passed"}
				]
			}`,
			wantToolProgress:  true,
			wantWriteProgress: true,
			wantHistoryErrors: 0,
		},
		{
			name: "Anthropic format: tool_use execute_command",
			body: `{
				"messages": [
					{"role": "user", "content": "Run tests"},
					{"role": "assistant", "content": [
						{"type": "text", "text": "Running test suite"},
						{"type": "tool_use", "id": "tu_1", "name": "execute_command", "input": {}}
					]},
					{"role": "user", "content": [
						{"type": "tool_result", "tool_use_id": "tu_1", "content": "PASS"}
					]}
				]
			}`,
			wantToolProgress:  true,
			wantWriteProgress: true,
			wantHistoryErrors: 0,
		},
		{
			name: "Anthropic format: tool_use list_files (read-only)",
			body: `{
				"messages": [
					{"role": "user", "content": "List files"},
					{"role": "assistant", "content": [
						{"type": "tool_use", "id": "tu_2", "name": "list_files", "input": {}}
					]},
					{"role": "user", "content": [
						{"type": "tool_result", "tool_use_id": "tu_2", "content": "file1.go, file2.go"}
					]}
				]
			}`,
			wantToolProgress:  true,
			wantWriteProgress: false,
			wantHistoryErrors: 0,
		},
		{
			name: "Custom write tools: custom_build tool recognized, write_to_file ignored",
			customTools: []string{"custom_build"},
			body: `{
				"messages": [
					{"role": "assistant", "tool_calls": [{"id": "c3", "type": "function", "function": {"name": "custom_build"}}]},
					{"role": "tool", "tool_call_id": "c3", "content": "Build succeeded"}
				]
			}`,
			wantToolProgress:  true,
			wantWriteProgress: true,
			wantHistoryErrors: 0,
		},
		{
			name: "Custom write tools: write_to_file not recognized when custom list active",
			customTools: []string{"custom_build"},
			body: `{
				"messages": [
					{"role": "assistant", "tool_calls": [{"id": "c4", "type": "function", "function": {"name": "write_to_file"}}]},
					{"role": "tool", "tool_call_id": "c4", "content": "File written"}
				]
			}`,
			wantToolProgress:  true,
			wantWriteProgress: false,
			wantHistoryErrors: 0,
		},
		{
			name: "Failed tool execution is not progress",
			body: `{
				"messages": [
					{"role": "assistant", "tool_calls": [{"id": "c5", "type": "function", "function": {"name": "write_to_file"}}]},
					{"role": "tool", "tool_call_id": "c5", "content": "The tool execution failed: permission denied"}
				]
			}`,
			wantToolProgress:  false,
			wantWriteProgress: false,
			wantHistoryErrors: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.customTools != nil {
				c.SetKickstartWriteTools(tc.customTools)
			} else {
				c.SetKickstartWriteTools(configuredTools)
			}

			ctx, err := c.Classify([]byte(tc.body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ctx.HasToolProgress != tc.wantToolProgress {
				t.Errorf("HasToolProgress = %v, want %v", ctx.HasToolProgress, tc.wantToolProgress)
			}
			if ctx.HasWriteProgress != tc.wantWriteProgress {
				t.Errorf("HasWriteProgress = %v, want %v", ctx.HasWriteProgress, tc.wantWriteProgress)
			}
			if ctx.HistoryErrors != tc.wantHistoryErrors {
				t.Errorf("HistoryErrors = %d, want %d", ctx.HistoryErrors, tc.wantHistoryErrors)
			}
		})
	}
}

// BenchmarkClassifier measures request parsing and keyword extraction speed per operation.
func BenchmarkClassifier(b *testing.B) {
	classifier := NewClassifier()
	jsonBody := []byte(`{
		"model": "gpt-4",
		"tools": [{"type": "function", "function": {"name": "read_file"}}],
		"messages": [
			{"role": "system", "content": "You are a helpful Go developer working on Spice."},
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Help fix a mutex deadlock in favorites_deadlock_test.go"},
					{"type": "image_url", "image_url": {"url": "http://example.com/img.png"}}
				]
			}
		]
	}`)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = classifier.Classify(jsonBody)
	}
}
