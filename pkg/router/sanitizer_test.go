package router

import (
	"encoding/json"
	"testing"
)

func TestSanitizer(t *testing.T) {
	inputJSON := `{
		"model": "text-model",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Analyze this image"},
					{"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}}
				]
			}
		]
	}`

	sanitizer := NewSanitizer()

	// Case 1: Vision model target -> no modification
	output1, mod1 := sanitizer.SanitizePayload([]byte(inputJSON), true)
	if mod1 {
		t.Errorf("Expected no modification for vision model, but was modified")
	}
	if string(output1) != inputJSON {
		t.Errorf("Output changed unexpectedly for vision model")
	}

	// Case 2: Non-vision model target -> image_url stripped
	output2, mod2 := sanitizer.SanitizePayload([]byte(inputJSON), false)
	if !mod2 {
		t.Errorf("Expected payload to be modified for non-vision model")
	}

	var parsed map[string]interface{}
	json.Unmarshal(output2, &parsed)
	msgs := parsed["messages"].([]interface{})
	firstMsg := msgs[0].(map[string]interface{})
	parts := firstMsg["content"].([]interface{})

	if len(parts) != 1 {
		t.Errorf("Expected 1 part remaining, got %d", len(parts))
	}
	firstPart := parts[0].(map[string]interface{})
	if firstPart["type"] != "text" {
		t.Errorf("Expected remaining part to be text, got %v", firstPart["type"])
	}
}
