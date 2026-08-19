package router

import (
	"testing"
)

func TestClassifier(t *testing.T) {
	inputJSON := `{
		"model": "gpt-4",
		"tools": [{"type": "function", "function": {"name": "read_file"}}],
		"messages": [
			{"role": "system", "content": "You are a helpful Go developer."},
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Help fix a mutex deadlock in Go"},
					{"type": "image_url", "image_url": {"url": "http://example.com/img.png"}}
				]
			}
		]
	}`

	classifier := NewClassifier()
	ctx, err := classifier.Classify([]byte(inputJSON))

	if err != nil {
		t.Fatalf("Failed to classify request: %v", err)
	}

	if !ctx.HasTools {
		t.Errorf("Expected HasTools = true")
	}

	if !ctx.HasImages {
		t.Errorf("Expected HasImages = true")
	}

	if ctx.Tokens == 0 {
		t.Errorf("Expected Tokens > 0")
	}

	foundMutex := false
	foundDeadlock := false
	for _, k := range ctx.Keywords {
		if k == "mutex" {
			foundMutex = true
		}
		if k == "deadlock" {
			foundDeadlock = true
		}
	}

	if !foundMutex || !foundDeadlock {
		t.Errorf("Expected keywords 'mutex' and 'deadlock' in extracted keywords: %v", ctx.Keywords)
	}
}
