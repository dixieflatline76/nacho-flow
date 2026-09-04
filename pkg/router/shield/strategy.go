package shield

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// FallbackStrategy defines how to synthesize arguments for a specific tool schema.
type FallbackStrategy interface {
	ToolName() string
	SynthesizeArgs(content string) (string, error)
	GenerateCallID(content string) string
}

// BaseStrategy provides deterministic ID generation.
type BaseStrategy struct{}

func (b *BaseStrategy) GenerateCallID(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("call_autowrap_%x", hash[:4])
}

// AskFollowupStrategy handles Zoo Code / Cline standard schema: {"question": "..."}
type AskFollowupStrategy struct {
	BaseStrategy
	Name string // e.g. "ask_followup_question" or "ask_question"
}

func (s *AskFollowupStrategy) ToolName() string {
	if s.Name == "" {
		return "ask_followup_question"
	}
	return s.Name
}

func (s *AskFollowupStrategy) SynthesizeArgs(content string) (string, error) {
	// Build a universal payload compatible with agent clients:
	// - Zoo Code requires "follow_up" array of {text, mode} objects
	// - Cline requires "options" array of strings
	// By including BOTH fields, the payload satisfies every client's schema validator.
	type followUpOption struct {
		Text string  `json:"text"`
		Mode *string `json:"mode"` // null in JSON
	}

	payload := map[string]interface{}{
		"question": content,
		"follow_up": []followUpOption{
			{Text: "Yes, proceed with this plan.", Mode: nil},
			{Text: "No, let me make changes first.", Mode: nil},
		},
		"options": []string{
			"Yes, proceed with this plan.",
			"No, let me make changes first.",
		},
	}

	b, err := json.Marshal(payload)
	return string(b), err
}

// ModeSwitchStrategy handles Cline mode switching: {"mode_slug": "code", "reason": "..."}
type ModeSwitchStrategy struct {
	BaseStrategy
	Name string // "switch_mode"
}

func (s *ModeSwitchStrategy) ToolName() string {
	if s.Name == "" {
		return "switch_mode"
	}
	return s.Name
}

func (s *ModeSwitchStrategy) SynthesizeArgs(content string) (string, error) {
	b, err := json.Marshal(map[string]string{
		"mode_slug": "code",
		"reason":    content,
	})
	return string(b), err
}
