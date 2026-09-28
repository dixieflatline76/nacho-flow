package tuner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// RewriteRuleAST synthesizes an optimal expr expression while preserving existing custom guardrails
// from the existing tier expression.
func RewriteRuleAST(existingWhen string, newThreshold int, optimalRetries int, frictionKws []string, restrictImages, restrictTools bool) (string, error) {
	if newThreshold < 0 {
		return "", fmt.Errorf("optimal threshold cannot be negative, got %d", newThreshold)
	}
	if newThreshold == 0 && containsTokenConstraint(existingWhen) {
		return "", fmt.Errorf("optimal threshold must be positive for token-constrained tier, got %d", newThreshold)
	}

	// 1. Extract preserved clauses from existing expression
	var preserved []string
	cleanExisting := strings.TrimSpace(existingWhen)

	if cleanExisting != "" {
		// Verify existing expr parses cleanly
		_, err := parser.Parse(cleanExisting)
		if err != nil {
			return "", fmt.Errorf("existing expression is invalid: %w", err)
		}

		// Extract individual conjuncts separated by '&&'
		clauses := splitConjuncts(cleanExisting)
		for _, c := range clauses {
			cTrimmed := strings.TrimSpace(c)
			if isAutoTunableClause(cTrimmed, optimalRetries > 0) {
				continue
			}
			if cTrimmed != "" {
				preserved = append(preserved, cTrimmed)
			}
		}
	}

	// 2. Build updated clauses
	var clauses []string

	// Primary token bound (only if positive)
	if newThreshold > 0 {
		clauses = append(clauses, fmt.Sprintf("Tokens < %d", newThreshold))
	}

	// Tuned retry bound
	if optimalRetries > 0 {
		clauses = append(clauses, fmt.Sprintf("Retries < %d", optimalRetries))
	}

	// Check if preserved already contains positive modality requirements to prevent contradictions
	hasPositiveImages := false
	hasPositiveTools := false
	for _, p := range preserved {
		if hasPositivePrerequisiteClause(p, "HasImages") {
			hasPositiveImages = true
		}
		if hasPositivePrerequisiteClause(p, "HasTools") {
			hasPositiveTools = true
		}
	}

	// Empirical Modalities
	if restrictImages && !hasPositiveImages {
		clauses = append(clauses, "!HasImages")
	}
	if restrictTools && !hasPositiveTools {
		clauses = append(clauses, "!HasTools")
	}

	// High friction keywords (sorted alphabetically for deterministic output)
	if len(frictionKws) > 0 {
		sortedKws := make([]string, len(frictionKws))
		copy(sortedKws, frictionKws)
		sort.Strings(sortedKws)

		var quoted []string
		for _, kw := range sortedKws {
			quoted = append(quoted, fmt.Sprintf("'%s'", kw))
		}
		clauses = append(clauses, fmt.Sprintf("!any(Keywords, { # in [%s] })", strings.Join(quoted, ", ")))
	}

	// Append preserved custom guardrails
	clauses = append(clauses, preserved...)

	if len(clauses) == 0 {
		if cleanExisting != "" {
			return cleanExisting, nil
		}
		return "true", nil
	}

	// Join into unified expression
	result := strings.Join(clauses, " && ")

	// 3. Verify final expression compiles against RequestContext
	_, err := expr.Compile(result, expr.Env(contract.RequestContext{}))
	if err != nil {
		return "", fmt.Errorf("synthesized rule failed expr compilation: %w", err)
	}

	return result, nil
}

// splitConjuncts splits an expression by top-level '&&' operators.
func splitConjuncts(exprStr string) []string {
	var parts []string
	var current strings.Builder
	parenDepth := 0
	bracketDepth := 0
	inQuote := false
	var quoteChar rune

	chars := []rune(exprStr)
	for i := 0; i < len(chars); i++ {
		ch := chars[i]

		if inQuote {
			current.WriteRune(ch)
			if ch == quoteChar {
				// Count consecutive preceding backslashes to determine if quote is escaped
				backslashes := 0
				for k := i - 1; k >= 0 && chars[k] == '\\'; k-- {
					backslashes++
				}
				if backslashes%2 == 0 {
					inQuote = false
				}
			}
			continue
		}

		switch ch {
		case '\'', '"':
			inQuote = true
			quoteChar = ch
			current.WriteRune(ch)
		case '(':
			parenDepth++
			current.WriteRune(ch)
		case ')':
			parenDepth--
			current.WriteRune(ch)
		case '[':
			bracketDepth++
			current.WriteRune(ch)
		case ']':
			bracketDepth--
			current.WriteRune(ch)
		case '&':
			if i+1 < len(chars) && chars[i+1] == '&' && parenDepth == 0 && bracketDepth == 0 {
				parts = append(parts, strings.TrimSpace(current.String()))
				current.Reset()
				i++ // skip second '&'
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, strings.TrimSpace(current.String()))
	}

	return parts
}

// isAutoTunableClause returns true if a conjunct is managed directly by the optimizer (Tokens, Retries, Modalities, Keywords).
func isAutoTunableClause(clause string, tuningRetries bool) bool {
	c := strings.TrimSpace(clause)
	if c == "" {
		return false
	}
	tree, err := parser.Parse(c)
	if err != nil {
		return false
	}
	node := tree.Node

	// 1. Tokens comparison: Tokens <op> N
	if b, ok := node.(*ast.BinaryNode); ok {
		if b.Operator == "<" || b.Operator == "<=" || b.Operator == ">" || b.Operator == ">=" || b.Operator == "==" {
			if isIdent(b.Left, "Tokens") || isIdent(b.Right, "Tokens") {
				return true
			}
		}
	}

	// 2. Retries constraint (only if tuningRetries is true)
	if tuningRetries {
		if b, ok := node.(*ast.BinaryNode); ok {
			if isIdent(b.Left, "Retries") {
				if b.Operator == "<" || b.Operator == "<=" {
					return true
				}
				if b.Operator == "==" {
					if val, ok := getIntLit(b.Right); ok && val == 0 {
						return true
					}
				}
			}
		}
		if isIdent(node, "IsRetry") {
			return true
		}
		if u, ok := node.(*ast.UnaryNode); ok && u.Operator == "!" {
			if isIdent(u.Node, "IsRetry") {
				return true
			}
		}
	}

	// 3. Modalities: !HasImages, !HasTools, HasImages == false, HasTools == false
	if u, ok := node.(*ast.UnaryNode); ok && u.Operator == "!" {
		if isIdent(u.Node, "HasImages") || isIdent(u.Node, "HasTools") {
			return true
		}
	}
	if b, ok := node.(*ast.BinaryNode); ok && b.Operator == "==" {
		if (isIdent(b.Left, "HasImages") && isBoolLit(b.Right, false)) ||
			(isIdent(b.Right, "HasImages") && isBoolLit(b.Left, false)) ||
			(isIdent(b.Left, "HasTools") && isBoolLit(b.Right, false)) ||
			(isIdent(b.Right, "HasTools") && isBoolLit(b.Left, false)) {
			return true
		}
	}

	// 4. Keywords: any(Keywords, ...) or !any(Keywords, ...)
	target := node
	if u, ok := node.(*ast.UnaryNode); ok && u.Operator == "!" {
		target = u.Node
	}
	if bi, ok := target.(*ast.BuiltinNode); ok && strings.EqualFold(bi.Name, "any") {
		if len(bi.Arguments) > 0 && isIdent(bi.Arguments[0], "Keywords") {
			return true
		}
	}

	return false
}
