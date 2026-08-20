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
