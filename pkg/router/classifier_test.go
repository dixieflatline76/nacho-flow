package router

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestClassifier_MessageTextExtraction_Types(t *testing.T) {
	// 1. Plain string content
	m1 := classifyMessage{
		Role:    "user",
		Content: classifyContent{Text: "hello world"},
	}
	if s := m1.Text(); s != "hello world" {
		t.Errorf("expected 'hello world', got %q", s)
	}

	// 2. Multi-part array with text parts and tool_result content
	partJSON := []byte(`{"type": "tool_result", "content": [{"type": "text", "text": "result part"}]}`)
	var part classifyContentPart
	if err := json.Unmarshal(partJSON, &part); err != nil {
		t.Fatalf("failed to unmarshal content part: %v", err)
	}
	if part.Text != "result part" {
		t.Errorf("expected 'result part', got %q", part.Text)
	}

	m2 := classifyMessage{
		Role: "user",
		Content: classifyContent{
			Parts: []classifyContentPart{
				{Type: "text", Text: "first part"},
				part,
			},
		},
	}
	extracted := m2.Text()
	if !strings.Contains(extracted, "first part") || !strings.Contains(extracted, "result part") {
		t.Errorf("expected multi-part extraction, got %q", extracted)
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
		name              string
		customTools       []string
		body              string
		wantToolProgress  bool
		wantWriteProgress bool
		wantHistoryErrors int
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
			name:        "Custom write tools: custom_build tool recognized, write_to_file ignored",
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
			name:        "Custom write tools: write_to_file not recognized when custom list active",
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

func TestClassify_HasWriteCapability(t *testing.T) {
	c := &RequestClassifier{estimator: NewTokenEstimator()}
	c.SetKickstartWriteTools([]string{"write_to_file", "replace_in_file", "execute_command"})

	// Plan Mode: only read tools -> HasWriteCapability = false
	planPayload := `{"messages":[{"role":"user","content":"investigate auth"}],"tools":[{"function":{"name":"read_file"}},{"function":{"name":"list_dir"}},{"function":{"name":"ask_followup_question"}}]}`
	ctx, err := c.Classify([]byte(planPayload))
	if err != nil {
		t.Fatal(err)
	}
	if ctx.HasWriteCapability {
		t.Error("expected HasWriteCapability=false for read-only tools (Plan Mode)")
	}
	if !ctx.HasTools {
		t.Error("expected HasTools=true")
	}

	// Code Mode: includes write tools -> HasWriteCapability = true
	codePayload := `{"messages":[{"role":"user","content":"fix bug"}],"tools":[{"function":{"name":"read_file"}},{"function":{"name":"write_to_file"}},{"function":{"name":"execute_command"}}]}`
	ctx2, err := c.Classify([]byte(codePayload))
	if err != nil {
		t.Fatal(err)
	}
	if !ctx2.HasWriteCapability {
		t.Error("expected HasWriteCapability=true when write_to_file is in tools")
	}

	// Code Mode with tools as {"name": "..."} instead of {"function": {"name": "..."}}
	flatToolPayload := `{"messages":[{"role":"user","content":"fix bug"}],"tools":[{"name":"replace_in_file"}]}`
	ctxFlat, err := c.Classify([]byte(flatToolPayload))
	if err != nil {
		t.Fatal(err)
	}
	if !ctxFlat.HasWriteCapability {
		t.Error("expected HasWriteCapability=true for flat tool object")
	}

	// No tools at all -> HasWriteCapability = false, HasTools = false
	noToolsPayload := `{"messages":[{"role":"user","content":"hello"}]}`
	ctx3, err := c.Classify([]byte(noToolsPayload))
	if err != nil {
		t.Fatal(err)
	}
	if ctx3.HasWriteCapability {
		t.Error("expected HasWriteCapability=false when no tools present")
	}
	if ctx3.HasTools {
		t.Error("expected HasTools=false when no tools present")
	}

	// Empty write tools config -> HasWriteCapability stays false (default builtins or empty)
	c2 := &RequestClassifier{estimator: NewTokenEstimator()}
	c2.SetKickstartWriteTools([]string{})
	ctx4, _ := c2.Classify([]byte(codePayload))
	if ctx4.HasWriteCapability {
		t.Error("expected HasWriteCapability=false when write tools map is empty")
	}
}

// TestClassifier_Phase1B_OverflowParallelWriteCalls tests that more than 8 parallel tool calls
// do not get silently dropped and hasWriteProgress correctly evaluates to true.
func TestClassifier_Phase1B_OverflowParallelWriteCalls(t *testing.T) {
	c := &RequestClassifier{estimator: NewTokenEstimator()}
	c.SetKickstartWriteTools([]string{"write_to_file", "execute_command"})

	// Assistant issues 12 parallel write calls (call_1 to call_12)
	assistantCalls := []map[string]interface{}{}
	for i := 1; i <= 12; i++ {
		assistantCalls = append(assistantCalls, map[string]interface{}{
			"id":   fmt.Sprintf("call_%d", i),
			"type": "function",
			"function": map[string]interface{}{
				"name": "write_to_file",
			},
		})
	}

	// Tool responses only respond to call_9 through call_12 (beyond the first 8)
	messages := []map[string]interface{}{
		{
			"role":       "assistant",
			"tool_calls": assistantCalls,
		},
		{
			"role":         "tool",
			"tool_call_id": "call_11",
			"content":      "File successfully written",
		},
	}

	payload := map[string]interface{}{
		"messages": messages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := c.Classify(body)
	if err != nil {
		t.Fatal(err)
	}

	if !ctx.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=true for call_11 response even when >8 calls were issued, got false")
	}
	if !ctx.HasToolProgress {
		t.Errorf("expected HasToolProgress=true, got false")
	}
}

// TestClassifier_Phase1A_ConcurrentWriteToolsAccess tests lock-free reads while write tools reload concurrently.
func TestClassifier_Phase1A_ConcurrentWriteToolsAccess(t *testing.T) {
	c := &RequestClassifier{estimator: NewTokenEstimator()}
	c.SetKickstartWriteTools([]string{"write_to_file", "replace_in_file"})

	done := make(chan struct{})
	var wg sync.WaitGroup

	// Reader goroutines
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					lookup := c.GetKickstartWriteTools()
					_ = lookup["write_to_file"]
				}
			}
		}()
	}

	// Writer goroutine simulating dynamic config reload
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if i%2 == 0 {
				c.SetKickstartWriteTools([]string{"write_to_file", "execute_command", "git_commit"})
			} else {
				c.SetKickstartWriteTools([]string{"read_file", "list_dir"})
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()

	time.Sleep(60 * time.Millisecond)
	close(done)
	wg.Wait()
}

// TestClassifier_Phase3_LongConversationDeferredScanning tests a 60-turn conversation
// to verify deferred trailing message parsing correctly extracts prompt, error signatures,
// and tool signals while only inspecting the last 8 messages.
func TestClassifier_Phase3_LongConversationDeferredScanning(t *testing.T) {
	c := &RequestClassifier{estimator: NewTokenEstimator()}
	c.SetKickstartWriteTools([]string{"write_to_file", "execute_command"})

	var messages []map[string]interface{}
	// Turns 1 to 56: generic history
	for i := 1; i <= 56; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		messages = append(messages, map[string]interface{}{
			"role":    role,
			"content": fmt.Sprintf("historical turn %d", i),
		})
	}

	// Turn 57: assistant issues write_to_file
	messages = append(messages, map[string]interface{}{
		"role": "assistant",
		"tool_calls": []map[string]interface{}{
			{
				"id":   "call_write_57",
				"type": "function",
				"function": map[string]interface{}{
					"name": "write_to_file",
				},
			},
		},
	})

	// Turn 58: tool returns a test failure
	messages = append(messages, map[string]interface{}{
		"role":         "tool",
		"tool_call_id": "call_write_57",
		"content":      "--- FAIL: TestRouterIntegration (0.05s)",
	})

	// Turn 59: assistant responds acknowledging error
	messages = append(messages, map[string]interface{}{
		"role":    "assistant",
		"content": "The test failed with an assertion error.",
	})

	// Turn 60: user prompt
	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": "Fix the failure in TestRouterIntegration",
	})

	payload := map[string]interface{}{
		"messages": messages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := c.Classify(body)
	if err != nil {
		t.Fatal(err)
	}

	if ctx.Prompt != "Fix the failure in TestRouterIntegration" {
		t.Errorf("unexpected Prompt: %q", ctx.Prompt)
	}
	if !ctx.HasTestFail {
		t.Errorf("expected HasTestFail=true, got false")
	}
	if ctx.HasTestPass {
		t.Errorf("expected HasTestPass=false, got true")
	}
	if ctx.HasTestProgress {
		t.Errorf("expected HasTestProgress=false when tests fail, got true")
	}
	if !ctx.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=true from call_write_57, got false")
	}
	if !ctx.HasToolProgress {
		t.Errorf("expected HasToolProgress=true, got false")
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

func TestClassify_ClineProtocol_WriteProgress(t *testing.T) {
	c := NewClassifier()
	if rc, ok := c.(interface{ SetKickstartWriteTools([]string) }); ok {
		rc.SetKickstartWriteTools([]string{"write_to_file", "replace_in_file", "execute_command"})
	}

	payload := []byte(`{
		"model": "nacho-hybrid",
		"messages": [
			{"role": "user", "content": "Please implement the feature"},
			{"role": "assistant", "content": "I am editing the code:\n<write_to_file>\n<path>foo.go</path>\n<content>package foo</content>\n</write_to_file>"},
			{"role": "user", "content": "[write_to_file for 'foo.go'] Result: File successfully written."}
		]
	}`)

	req, err := c.Classify(payload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if !req.HasToolProgress {
		t.Errorf("expected HasToolProgress=true for Cline write result")
	}
	if !req.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=true for Cline write result")
	}
}

func TestClassify_ClineProtocol_ReadStall(t *testing.T) {
	c := NewClassifier()
	if rc, ok := c.(interface{ SetKickstartWriteTools([]string) }); ok {
		rc.SetKickstartWriteTools([]string{"write_to_file", "replace_in_file", "execute_command"})
	}

	payload := []byte(`{
		"model": "nacho-hybrid",
		"messages": [
			{"role": "user", "content": "Please fix bug"},
			{"role": "assistant", "content": "Let me read the file:\n<read_file>\n<path>foo.go</path>\n</read_file>"},
			{"role": "user", "content": "[read_file for 'foo.go'] Result: package main"}
		]
	}`)

	req, err := c.Classify(payload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	// Per old behavior, read_file is not a write tool, so no tool/write progress is granted on user result
	if req.HasToolProgress {
		t.Errorf("expected HasToolProgress=false on Cline read stall to permit Kickstart accumulation")
	}
	if req.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=false on Cline read stall")
	}
}

func TestClassify_AnthropicProtocol_StringContent(t *testing.T) {
	c := NewClassifier()
	if rc, ok := c.(interface{ SetKickstartWriteTools([]string) }); ok {
		rc.SetKickstartWriteTools([]string{"execute_command"})
	}

	payload := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Run the tests"}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "toolu_1", "name": "execute_command", "input": {"command": "go test"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "toolu_1", "content": "--- FAIL: TestFoo (0.01s)\nFAIL\nexit status 1"}]}
		]
	}`)

	req, err := c.Classify(payload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if !req.HasTestFail {
		t.Errorf("expected HasTestFail=true from Anthropic tool_result string content")
	}
	if req.HasTestPass {
		t.Errorf("expected HasTestPass=false")
	}
	if req.HasTestProgress {
		t.Errorf("expected HasTestProgress=false when test fails")
	}
}

func TestClassify_AnthropicProtocol_ArrayContent(t *testing.T) {
	c := NewClassifier()
	if rc, ok := c.(interface{ SetKickstartWriteTools([]string) }); ok {
		rc.SetKickstartWriteTools([]string{"execute_command"})
	}

	payload := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Run the tests"}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "toolu_2", "name": "execute_command", "input": {"command": "go test"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "toolu_2", "content": [{"type": "text", "text": "--- PASS: TestFoo (0.01s)\nPASS\nok  \tpkg/foo\t0.02s"}]}]}
		]
	}`)

	req, err := c.Classify(payload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if !req.HasTestPass {
		t.Errorf("expected HasTestPass=true from Anthropic tool_result array content")
	}
	if req.HasTestFail {
		t.Errorf("expected HasTestFail=false")
	}
	if !req.HasTestProgress {
		t.Errorf("expected HasTestProgress=true on clean passing test run")
	}
	if !req.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=true from execute_command tool_use")
	}
}

func TestClassify_AnthropicProtocol_IsError(t *testing.T) {
	c := NewClassifier()

	payload := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "Read file"},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "toolu_3", "name": "read_file", "input": {"path": "foo"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "toolu_3", "content": "File does not exist", "is_error": true}]}
		]
	}`)

	req, err := c.Classify(payload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if req.HistoryErrors != 1 {
		t.Errorf("expected HistoryErrors=1 from is_error=true tool_result, got %d", req.HistoryErrors)
	}
}

func TestClassify_ConsecutiveUserErrors(t *testing.T) {
	c := NewClassifier()

	payload := []byte(`{
		"model": "nacho-hybrid",
		"messages": [
			{"role": "user", "content": "Initial prompt"},
			{"role": "assistant", "content": "Working..."},
			{"role": "user", "content": "[ERROR] You did not use a tool"},
			{"role": "assistant", "content": "Let me try again..."},
			{"role": "user", "content": "The tool execution failed"}
		]
	}`)

	req, err := c.Classify(payload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if req.HistoryErrors != 2 {
		t.Errorf("expected HistoryErrors=2 for consecutive user error turns, got %d", req.HistoryErrors)
	}
}

func TestClassify_LegacyFunctionCall(t *testing.T) {
	c := NewClassifier()
	if rc, ok := c.(interface{ SetKickstartWriteTools([]string) }); ok {
		rc.SetKickstartWriteTools([]string{"write_to_file"})
	}

	payload := []byte(`{
		"model": "gpt-3.5-turbo-0613",
		"messages": [
			{"role": "user", "content": "Write code"},
			{"role": "assistant", "content": null, "function_call": {"name": "write_to_file", "arguments": "{\"path\":\"main.go\"}"}},
			{"role": "tool", "content": "File written"}
		]
	}`)

	req, err := c.Classify(payload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if !req.HasToolProgress {
		t.Errorf("expected HasToolProgress=true for legacy function_call")
	}
	if !req.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=true for legacy function_call with write tool")
	}
}

func TestClassify_MultiPartAssistantXML(t *testing.T) {
	c := NewClassifier()
	if rc, ok := c.(interface{ SetKickstartWriteTools([]string) }); ok {
		rc.SetKickstartWriteTools([]string{"replace_in_file"})
	}

	payload := []byte(`{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "Fix the function"},
			{"role": "assistant", "content": [
				{"type": "text", "text": "Here is the fix:\n<replace_in_file path=\"foo.go\">\n<diff>...</diff>\n</replace_in_file>"}
			]},
			{"role": "user", "content": "[replace_in_file for 'foo.go'] Success"}
		]
	}`)

	req, err := c.Classify(payload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if !req.HasToolProgress {
		t.Errorf("expected HasToolProgress=true")
	}
	if !req.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=true from multi-part assistant XML tag")
	}
}

func BenchmarkClassify_MultiTurn_ZeroAlloc(b *testing.B) {
	classifier := NewClassifier()
	if rc, ok := classifier.(interface{ SetKickstartWriteTools([]string) }); ok {
		rc.SetKickstartWriteTools([]string{"write_to_file", "execute_command"})
	}

	jsonBody := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "Run tests"},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "t1", "name": "execute_command", "input": {"command": "go test"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "t1", "content": "--- PASS: TestOk (0.01s)\nPASS"}]}
		]
	}`)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = classifier.Classify(jsonBody)
	}
}

func TestDetectShellWrite(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected bool
	}{
		// Writes via redirection
		{"echo redirect write", "echo 'package main' > main.go", true},
		{"append redirect write", "echo 'export PATH' >> ~/.bashrc", true},
		{"cat to file", "cat source.go > dest.go", true},
		{"python redirect write", "python3 -c 'print(1)' > out.txt", true},
		{"pipe to tee", "cat file.txt | tee output.txt", true},
		{"pipe to tee append", "make build | tee -a build.log", true},
		{"pipe to dd of", "cat file | dd of=/tmp/out", true},

		// Heredocs
		{"heredoc single quote", "cat << 'EOF' > main.go\npackage main\nEOF", true},
		{"heredoc double quote", "cat << \"EOF\" > config.json\n{}\nEOF", true},
		{"heredoc unquoted", "cat <<EOF > notes.txt\nnotes\nEOF", true},

		// Direct file manipulation commands
		{"sed in-place", "sed -i 's/foo/bar/g' main.go", true},
		{"sed without in-place", "sed 's/foo/bar/g' main.go", false},
		{"patch command", "patch -p1 < fix.patch", true},
		{"touch command", "touch newfile.txt", true},
		{"mkdir command", "mkdir -p pkg/models", true},
		{"rm command", "rm -rf tmp/cache", true},
		{"rmdir command", "rmdir old_dir", true},
		{"cp command", "cp template.go handler.go", true},
		{"mv command", "mv old.go new.go", true},
		{"truncate command", "truncate -s 0 log.txt", true},
		{"tar extract", "tar -xzf archive.tar.gz", true},
		{"unzip command", "unzip bundle.zip", true},
		{"git checkout file", "git checkout -- main.go", true},
		{"git restore file", "git restore pkg/router.go", true},
		{"git apply patch", "git apply fix.diff", true},

		// Windows commands
		{"powershell Out-File", "Get-Process | Out-File proc.txt", true},
		{"powershell Set-Content", "Set-Content -Path file.txt -Value 'hello'", true},
		{"powershell Add-Content", "Add-Content -Path file.txt -Value 'hello'", true},
		{"powershell New-Item", "New-Item -ItemType File -Path test.txt", true},
		{"cmd copy", "copy file1.txt file2.txt", true},
		{"cmd move", "move old.txt new.txt", true},
		{"cmd del", "del /f /q *.tmp", true},
		{"cmd erase", "erase file.txt", true},
		{"cmd ren", "ren file1.txt file2.txt", true},
		{"cmd md", "md new_folder", true},

		// Non-writing commands (READS AND DEV/NULL GUARDS)
		{"bare cat read", "cat main.go", false},
		{"head read", "head -n 50 main.go", false},
		{"tail read", "tail -f server.log", false},
		{"grep pattern", "grep -rn 'func Handle' .", false},
		{"ls command", "ls -la /tmp", false},
		{"dir command", "dir C:\\Windows", false},
		{"echo string only", "echo 'starting migration...'", false},
		{"go test", "go test -v ./...", false},
		{"git log", "git log -n 5", false},
		{"git status", "git status --porcelain", false},
		{"curl to dev null", "curl -s http://localhost:8080/health > /dev/null", false},
		{"curl to dev null with stderr redirect", "curl -s http://localhost:8080/health > /dev/null 2>&1", false},
		{"command with stderr to stdout", "go build ./... 2>&1", false},
		{"command with stdout to stderr", "echo error >&2", false},
		{"command to nul Windows", "ping 127.0.0.1 > nul", false},
		{"powershell to null", "Test-Path . > $null", false},
		{"python comparison single quotes", "python -c 'if x > 5: print(x)'", false},
		{"python comparison double quotes", "python -c \"if x > 5: print(x)\"", false},
		{"awk comparison", "awk '$2 > 10 { print $1 }' input.txt", false},
		{"comparison operator greater equal", "test 10 >= 5", false},
		{"git merge branch", "git merge origin/main", false},
		{"git rebase master", "git rebase master", false},
		{"git cherry-pick commit", "git cherry-pick abc1234", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := detectShellWrite(tc.cmd)
			if res != tc.expected {
				t.Errorf("detectShellWrite(%q) = %v, expected %v", tc.cmd, res, tc.expected)
			}
		})
	}
}

func TestClassify_ShellWriteProgress_SignalSeparation(t *testing.T) {
	c := NewClassifier()

	// 1. Bash cat read -> HasToolProgress=true, HasWriteProgress=false, HasShellWrite=false
	readPayload := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "Check context"},
			{"role": "assistant", "content": [
				{"type": "tool_use", "id": "t1", "name": "bash", "input": {"command": "cat main.go"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "t1", "content": "package main\n\nfunc main() {}"}
			]}
		]
	}`)

	reqRead, err := c.Classify(readPayload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if !reqRead.HasToolProgress {
		t.Errorf("expected HasToolProgress=true for successful cat read")
	}
	if reqRead.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=false for cat read (should not trigger structured write)")
	}
	if reqRead.HasShellWrite {
		t.Errorf("expected HasShellWrite=false for bare cat read")
	}

	// 2. Bash redirection write -> HasToolProgress=true, HasWriteProgress=false, HasShellWrite=true
	writePayload := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "Write main.go"},
			{"role": "assistant", "content": [
				{"type": "tool_use", "id": "t2", "name": "bash", "input": {"command": "echo 'package main' > main.go"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "t2", "content": ""}
			]}
		]
	}`)

	reqWrite, err := c.Classify(writePayload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if !reqWrite.HasToolProgress {
		t.Errorf("expected HasToolProgress=true for successful bash write")
	}
	if reqWrite.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=false for bash command (signal separation!)")
	}
	if !reqWrite.HasShellWrite {
		t.Errorf("expected HasShellWrite=true for bash redirection write")
	}

	// 3. Structured tool write -> HasToolProgress=true, HasWriteProgress=true, HasShellWrite=false
	if rc, ok := c.(interface{ SetKickstartWriteTools([]string) }); ok {
		rc.SetKickstartWriteTools([]string{"write_to_file"})
	}
	structPayload := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "Write file"},
			{"role": "assistant", "content": [
				{"type": "tool_use", "id": "t3", "name": "write_to_file", "input": {"file": "main.go", "content": "package main"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "t3", "content": "File saved"}
			]}
		]
	}`)

	reqStruct, err := c.Classify(structPayload)
	if err != nil {
		t.Fatalf("classify error: %v", err)
	}
	if !reqStruct.HasToolProgress {
		t.Errorf("expected HasToolProgress=true for structured write")
	}
	if !reqStruct.HasWriteProgress {
		t.Errorf("expected HasWriteProgress=true for structured write_to_file")
	}
	if reqStruct.HasShellWrite {
		t.Errorf("expected HasShellWrite=false for structured tool")
	}
}
