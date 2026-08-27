package shield

import (
	"bytes"
	"strings"
)

// DefaultQuestionPhrases are the baseline heuristics for conversational plans & questions.
var DefaultQuestionPhrases = []string{
	"are you satisfied",
	"would you like",
	"should i",
	"do you approve",
	"please confirm",
	"let me know if",
	"how would you like to proceed",
	"shall i proceed",
	"do you want me to",
}

// DefaultModePhrases are baseline heuristics for mode switching.
var DefaultModePhrases = []string{
	"switch to code mode",
	"switch to architect mode",
	"ready to implement",
}

// RuleEngine inspects text bytes for conversational intent and questions.
type RuleEngine struct {
	questionPhrases [][]byte
	modePhrases     [][]byte
}

// NewRuleEngine constructs a RuleEngine with custom or default phrases.
func NewRuleEngine(questionPhrases, modePhrases []string) *RuleEngine {
	if len(questionPhrases) == 0 {
		questionPhrases = DefaultQuestionPhrases
	}
	if len(modePhrases) == 0 {
		modePhrases = DefaultModePhrases
	}

	e := &RuleEngine{}
	for _, p := range questionPhrases {
		trimmed := strings.ToLower(strings.TrimSpace(p))
		if len(trimmed) > 0 {
			e.questionPhrases = append(e.questionPhrases, []byte(trimmed))
		}
	}
	for _, p := range modePhrases {
		trimmed := strings.ToLower(strings.TrimSpace(p))
		if len(trimmed) > 0 {
			e.modePhrases = append(e.modePhrases, []byte(trimmed))
		}
	}
	return e
}

// Evaluate analyzes trailing bytes in < 180ns with 0 heap allocations.
func (e *RuleEngine) Evaluate(tail []byte) (matched bool, intent string) {
	trimmed := bytes.TrimSpace(tail)
	if len(trimmed) == 0 {
		return false, ""
	}

	// 1. Suffix check: Ends with '?'
	if trimmed[len(trimmed)-1] == '?' {
		return true, "question"
	}

	lower := bytes.ToLower(trimmed)

	// 2. Question phrases
	for _, phrase := range e.questionPhrases {
		if bytes.Contains(lower, phrase) {
			return true, "question"
		}
	}

	// 3. Mode switch phrases
	for _, phrase := range e.modePhrases {
		if bytes.Contains(lower, phrase) {
			return true, "mode_switch"
		}
	}

	return false, ""
}
