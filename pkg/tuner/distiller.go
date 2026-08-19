package tuner

import (
	"fmt"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/expr-lang/expr"
)

// DistillRule synthesizes a clean expr rule from optimal threshold and friction keywords,
// and validates that it compiles cleanly with expr.Compile.
func DistillRule(optimalThreshold int, highFrictionKws []string) (string, error) {
	var rule string
	if len(highFrictionKws) == 0 {
		rule = fmt.Sprintf("Tokens < %d && !HasImages && !HasTools", optimalThreshold)
	} else {
		var quoted []string
		for _, kw := range highFrictionKws {
			quoted = append(quoted, fmt.Sprintf("'%s'", kw))
		}
		rule = fmt.Sprintf("Tokens < %d && !HasImages && !HasTools && !any(Keywords, { # in [%s] })", optimalThreshold, strings.Join(quoted, ", "))
	}

	// Verify expr syntax compiles against RequestContext
	_, err := expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		return "", fmt.Errorf("distilled rule failed expr compilation: %w", err)
	}

	return rule, nil
}
