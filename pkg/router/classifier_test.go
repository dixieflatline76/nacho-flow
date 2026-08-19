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
