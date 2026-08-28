package router

import (
	"fmt"
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

	// 10 chars + space = ~11 chars / 2.0 ~= 5 tokens
	if ctx.Tokens < 4 || ctx.Tokens > 6 {
		t.Errorf("Expected ~5 tokens with calibrated estimator, got %d", ctx.Tokens)
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
