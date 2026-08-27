package shield

import (
	"strings"
)

// RawToolCall represents the normalized tool call output structure.
type RawToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ShieldManager coordinates strategies, rule matching, and tool call synthesis.
type ShieldManager struct {
	ruleEngine *RuleEngine
	strategies map[string]FallbackStrategy
}

// NewDefaultShieldManager constructs a ShieldManager with default rules and strategies.
func NewDefaultShieldManager() *ShieldManager {
	return NewShieldManager(nil, nil)
}

// NewShieldManager constructs a configured ShieldManager.
func NewShieldManager(questionPhrases, modePhrases []string) *ShieldManager {
	mgr := &ShieldManager{
		ruleEngine: NewRuleEngine(questionPhrases, modePhrases),
		strategies: make(map[string]FallbackStrategy),
	}

	// Register default strategies
	mgr.RegisterStrategy(&AskFollowupStrategy{Name: "ask_followup_question"})
	mgr.RegisterStrategy(&AskFollowupStrategy{Name: "ask_question"})
	mgr.RegisterStrategy(&AskFollowupStrategy{Name: "user_prompt"})
	mgr.RegisterStrategy(&ModeSwitchStrategy{Name: "switch_mode"})

	return mgr
}

// RegisterStrategy registers a fallback strategy by tool name.
func (sm *ShieldManager) RegisterStrategy(strat FallbackStrategy) {
	if strat == nil {
		return
	}
	sm.strategies[strings.ToLower(strat.ToolName())] = strat
}

// EvaluateAndSynthesize evaluates the message content against the client's declared interactive tool.
// If matching heuristics pass, returns a schema-compliant RawToolCall and true.
func (sm *ShieldManager) EvaluateAndSynthesize(content string, targetToolName string) (*RawToolCall, bool) {
	if targetToolName == "" || len(strings.TrimSpace(content)) == 0 {
		return nil, false
	}

	strat, exists := sm.strategies[strings.ToLower(targetToolName)]
	if !exists {
		// Fallback to generic ask followup strategy with target tool name
		strat = &AskFollowupStrategy{Name: targetToolName}
	}

	// Evaluate trailing 512 bytes
	contentBytes := []byte(content)
	tail := contentBytes
	if len(contentBytes) > 512 {
		tail = contentBytes[len(contentBytes)-512:]
	}

	matched, _ := sm.ruleEngine.Evaluate(tail)
	if !matched {
		return nil, false
	}

	argsJSON, err := strat.SynthesizeArgs(content)
	if err != nil {
		return nil, false
	}

	call := &RawToolCall{
		ID:   strat.GenerateCallID(content),
		Type: "function",
	}
	call.Function.Name = strat.ToolName()
	call.Function.Arguments = argsJSON

	return call, true
}

// RuleEngine returns the underlying RuleEngine.
func (sm *ShieldManager) RuleEngine() *RuleEngine {
	return sm.ruleEngine
}
