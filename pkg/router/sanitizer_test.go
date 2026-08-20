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

	// Case 3: Invalid JSON input
	out3, mod3 := sanitizer.SanitizePayload([]byte("invalid json"), false)
	if mod3 || string(out3) != "invalid json" {
		t.Errorf("Expected invalid JSON to pass through unmodified")
	}

	// Case 4: Non-array or empty messages
	_, mod4 := sanitizer.SanitizePayload([]byte(`{"messages": []}`), false)
	if mod4 {
		t.Errorf("Expected empty messages to pass through unmodified")
	}

	// Case 5: Message with only image parts -> gets sanitized to empty string content
	allImageJSON := `{"messages": [{"role": "user", "content": [{"type": "image_url", "image_url": {"url": "http://example.com/img.png"}}]}]}`
	out5, mod5 := sanitizer.SanitizePayload([]byte(allImageJSON), false)
	if !mod5 {
		t.Fatalf("Expected all-image content to be modified")
	}
	var parsed5 map[string]interface{}
	json.Unmarshal(out5, &parsed5)
	msgs5 := parsed5["messages"].([]interface{})
	firstMsg5 := msgs5[0].(map[string]interface{})
	if firstMsg5["content"] != "" {
		t.Errorf("Expected empty string content when all images stripped, got %v", firstMsg5["content"])
	}

	// Case 6: Messages containing non-map items or parts containing raw strings
	mixedJSON := `{"messages": ["non-map-msg", {"role": "user"}, {"role": "user", "content": ["raw-part-string"]}]}`
	out6, mod6 := sanitizer.SanitizePayload([]byte(mixedJSON), false)
	if mod6 {
		t.Errorf("Expected mixed non-image payload to not be modified")
	}
	_ = out6

	// Case 7: Message with missing content key alongside sanitized image
	missingContentWithImage := `{"messages": [{"role": "system"}, {"role": "user", "content": [{"type": "image_url", "image_url": {"url": "http://example.com/img.png"}}]}]}`
	out7, mod7 := sanitizer.SanitizePayload([]byte(missingContentWithImage), false)
	if !mod7 {
		t.Errorf("Expected modification when image present")
	}
	_ = out7
}

// BenchmarkSanitizer measures image payload stripping performance.
func BenchmarkSanitizer(b *testing.B) {
	inputJSON := []byte(`{
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
	}`)
	sanitizer := NewSanitizer()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = sanitizer.SanitizePayload(inputJSON, false)
	}
}
