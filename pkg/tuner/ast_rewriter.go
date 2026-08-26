package tuner

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/parser"
)

var (
	tokenClauseRegex    = regexp.MustCompile(`(?i)^\s*tokens\s*[<>=]`)
	modalityClauseRegex = regexp.MustCompile(`(?i)^!?\s*has(images|tools)$`)
	keywordClauseRegex  = regexp.MustCompile(`(?i)any\s*\(\s*keywords\s*,`)
)

// RewriteRuleAST synthesizes an optimal expr expression while preserving existing custom guardrails
// (e.g. Retries < 2, !IsRetry, custom tags) from the existing tier expression.
func RewriteRuleAST(existingWhen string, newThreshold int, frictionKws []string, restrictImages, restrictTools bool) (string, error) {
	if newThreshold <= 0 {
		return "", fmt.Errorf("optimal threshold must be positive, got %d", newThreshold)
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
			if isAutoTunableClause(cTrimmed) {
				continue
			}
			if cTrimmed != "" {
				preserved = append(preserved, cTrimmed)
			}
		}
	}

	// 2. Build updated clauses
	var clauses []string

	// Primary token bound
	clauses = append(clauses, fmt.Sprintf("Tokens < %d", newThreshold))

	// Empirical Modalities
	if restrictImages {
		clauses = append(clauses, "!HasImages")
	}
	if restrictTools {
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

// isAutoTunableClause returns true if a conjunct is managed directly by the optimizer (Tokens, Modalities, Keywords).
func isAutoTunableClause(clause string) bool {
	c := strings.TrimSpace(clause)
	if tokenClauseRegex.MatchString(c) {
		return true
	}
	if modalityClauseRegex.MatchString(c) {
		return true
	}
	if keywordClauseRegex.MatchString(c) {
		return true
	}
	return false
}
