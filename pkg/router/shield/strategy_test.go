package shield

import (
	"encoding/json"
	"testing"
)

func TestAskFollowupStrategy_SynthesizesCompliantSchema(t *testing.T) {
	strat := &AskFollowupStrategy{Name: "ask_followup_question"}
	args, err := strat.SynthesizeArgs("What features would you like?")
	if err != nil {
		t.Fatalf("SynthesizeArgs error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		t.Fatalf("Failed to unmarshal args: %v", err)
	}

	// Verify "question" field
	q, ok := payload["question"].(string)
	if !ok || q != "What features would you like?" {
		t.Errorf("Expected question field, got %v", payload["question"])
	}

	// Verify "follow_up" array (Zoo Code / Roo Code compatibility)
	followUp, ok := payload["follow_up"].([]interface{})
	if !ok || len(followUp) < 2 {
		t.Fatalf("Expected follow_up array with 2+ items, got %v", payload["follow_up"])
	}
	first := followUp[0].(map[string]interface{})
	if first["text"] != "Yes, proceed with this plan." {
		t.Errorf("Unexpected follow_up[0].text: %v", first["text"])
	}
	if first["mode"] != nil {
		t.Errorf("Expected follow_up[0].mode to be null, got %v", first["mode"])
	}

	// Verify "options" array (Cline compatibility)
	options, ok := payload["options"].([]interface{})
	if !ok || len(options) < 2 {
		t.Fatalf("Expected options array with 2+ items, got %v", payload["options"])
	}
	if options[0] != "Yes, proceed with this plan." {
		t.Errorf("Unexpected options[0]: %v", options[0])
	}
}

func TestAskFollowupStrategy_ToolName(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"ask_followup_question", "ask_followup_question"},
		{"ask_question", "ask_question"},
		{"", "ask_followup_question"}, // default
	}
	for _, tt := range tests {
		s := &AskFollowupStrategy{Name: tt.name}
		if got := s.ToolName(); got != tt.expected {
			t.Errorf("AskFollowupStrategy{Name: %q}.ToolName() = %q, want %q", tt.name, got, tt.expected)
		}
	}
}
