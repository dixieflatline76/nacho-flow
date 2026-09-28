package tuner

import (
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// ExtractedRoutingRules holds routing metadata extracted directly from a tier's expr When clause.
type ExtractedRoutingRules struct {
	TokenThreshold    int
	RetryBound        int
	RetryFloor        int
	RequiresKickstart bool
	RestrictImages    bool
	RequiresImages    bool
	RestrictTools     bool
	RequiresTools     bool
	ExcludedKeywords  []string
}

func isIdent(n ast.Node, name string) bool {
	if id, ok := n.(*ast.IdentifierNode); ok {
		return strings.EqualFold(id.Value, name)
	}
	return false
}

func isBoolLit(n ast.Node, val bool) bool {
	if b, ok := n.(*ast.BoolNode); ok {
		return b.Value == val
	}
	return false
}

func getIntLit(n ast.Node) (int, bool) {
	if i, ok := n.(*ast.IntegerNode); ok {
		return i.Value, true
	}
	return 0, false
}

// splitASTConjuncts recursively splits an AST binary tree joined by '&&'.
func splitASTConjuncts(n ast.Node) []ast.Node {
	if n == nil {
		return nil
	}
	if b, ok := n.(*ast.BinaryNode); ok && b.Operator == "&&" {
		return append(splitASTConjuncts(b.Left), splitASTConjuncts(b.Right)...)
	}
	return []ast.Node{n}
}

// walkAST traverses an AST subtree invoking fn for each node. If fn returns false, traversal halts for that branch.
func walkAST(n ast.Node, fn func(ast.Node) bool) {
	if n == nil {
		return
	}
	if !fn(n) {
		return
	}
	switch v := n.(type) {
	case *ast.UnaryNode:
		walkAST(v.Node, fn)
	case *ast.BinaryNode:
		walkAST(v.Left, fn)
		walkAST(v.Right, fn)
	case *ast.ArrayNode:
		for _, elem := range v.Nodes {
			walkAST(elem, fn)
		}
	case *ast.BuiltinNode:
		for _, arg := range v.Arguments {
			walkAST(arg, fn)
		}
	case *ast.PredicateNode:
		walkAST(v.Node, fn)
	case *ast.CallNode:
		walkAST(v.Callee, fn)
		for _, arg := range v.Arguments {
			walkAST(arg, fn)
		}
	}
}

// extractRoutingFromWhenAST parses a tier's expr condition and inspects the AST tree
// to extract optimizer routing constraints without regex.
func extractRoutingFromWhenAST(whenStr string) ExtractedRoutingRules {
	clean := strings.TrimSpace(whenStr)
	if clean == "" || clean == "true" || clean == "false" {
		return ExtractedRoutingRules{}
	}

	tree, err := parser.Parse(clean)
	if err != nil {
		return ExtractedRoutingRules{}
	}

	rules := ExtractedRoutingRules{}
	conjuncts := splitASTConjuncts(tree.Node)

	for _, c := range conjuncts {
		// 1. Kickstart prerequisite
		if isIdent(c, "SessionKickstarted") {
			rules.RequiresKickstart = true
		} else if b, ok := c.(*ast.BinaryNode); ok && b.Operator == "==" {
			if (isIdent(b.Left, "SessionKickstarted") && isBoolLit(b.Right, true)) ||
				(isIdent(b.Right, "SessionKickstarted") && isBoolLit(b.Left, true)) {
				rules.RequiresKickstart = true
			}
		}

		// 2. Modality negations and positive prerequisites
		if u, ok := c.(*ast.UnaryNode); ok && u.Operator == "!" {
			if isIdent(u.Node, "HasImages") {
				rules.RestrictImages = true
			}
			if isIdent(u.Node, "HasTools") {
				rules.RestrictTools = true
			}
		}
		if b, ok := c.(*ast.BinaryNode); ok && b.Operator == "==" {
			if (isIdent(b.Left, "HasImages") && isBoolLit(b.Right, false)) ||
				(isIdent(b.Right, "HasImages") && isBoolLit(b.Left, false)) {
				rules.RestrictImages = true
			}
			if (isIdent(b.Left, "HasTools") && isBoolLit(b.Right, false)) ||
				(isIdent(b.Right, "HasTools") && isBoolLit(b.Left, false)) {
				rules.RestrictTools = true
			}
			if (isIdent(b.Left, "HasImages") && isBoolLit(b.Right, true)) ||
				(isIdent(b.Right, "HasImages") && isBoolLit(b.Left, true)) {
				rules.RequiresImages = true
			}
			if (isIdent(b.Left, "HasTools") && isBoolLit(b.Right, true)) ||
				(isIdent(b.Right, "HasTools") && isBoolLit(b.Left, true)) {
				rules.RequiresTools = true
			}
		}
		if isIdent(c, "HasImages") {
			rules.RequiresImages = true
		}
		if isIdent(c, "HasTools") {
			rules.RequiresTools = true
		}

		// 3. Tokens & Retries
		if b, ok := c.(*ast.BinaryNode); ok {
			if isIdent(b.Left, "Tokens") && b.Operator == "<" {
				if val, ok := getIntLit(b.Right); ok && val > 0 {
					rules.TokenThreshold = val
				}
			}
			if isIdent(b.Left, "Retries") {
				if val, ok := getIntLit(b.Right); ok {
					if b.Operator == "<" && val > 0 {
						rules.RetryBound = val
					} else if b.Operator == ">=" && val > 0 {
						rules.RetryFloor = val
					} else if b.Operator == ">" && val >= 0 {
						rules.RetryFloor = val + 1
					}
				}
			}
		}

		// 4. Keywords
		hasKeywords := false
		var stringsFound []string
		walkAST(c, func(n ast.Node) bool {
			if isIdent(n, "Keywords") {
				hasKeywords = true
			}
			if s, ok := n.(*ast.StringNode); ok {
				stringsFound = append(stringsFound, s.Value)
			}
			return true
		})
		if hasKeywords && len(stringsFound) > 0 {
			rules.ExcludedKeywords = append(rules.ExcludedKeywords, stringsFound...)
		}
	}

	if rules.RestrictImages {
		rules.RequiresImages = false
	}
	if rules.RestrictTools {
		rules.RequiresTools = false
	}

	return rules
}

// containsTokenConstraint checks via AST whether an expression contains a Tokens comparison.
func containsTokenConstraint(exprStr string) bool {
	clean := strings.TrimSpace(exprStr)
	if clean == "" {
		return false
	}
	tree, err := parser.Parse(clean)
	if err != nil {
		return false
	}
	found := false
	walkAST(tree.Node, func(n ast.Node) bool {
		if b, ok := n.(*ast.BinaryNode); ok {
			if b.Operator == "<" || b.Operator == "<=" || b.Operator == ">" || b.Operator == ">=" || b.Operator == "==" {
				if isIdent(b.Left, "Tokens") || isIdent(b.Right, "Tokens") {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

// hasPositivePrerequisiteClause checks whether a preserved clause contains an un-negated requirement for the identifier.
func hasPositivePrerequisiteClause(clause string, name string) bool {
	clean := strings.TrimSpace(clause)
	if clean == "" {
		return false
	}
	tree, err := parser.Parse(clean)
	if err != nil {
		return false
	}

	found := false
	var check func(n ast.Node, negated bool)
	check = func(n ast.Node, negated bool) {
		if n == nil || found {
			return
		}
		if b, ok := n.(*ast.BinaryNode); ok && b.Operator == "==" {
			if (isIdent(b.Left, name) && isBoolLit(b.Right, true)) ||
				(isIdent(b.Right, name) && isBoolLit(b.Left, true)) {
				if !negated {
					found = true
				}
				return
			}
			if (isIdent(b.Left, name) && isBoolLit(b.Right, false)) ||
				(isIdent(b.Right, name) && isBoolLit(b.Left, false)) {
				return
			}
		}
		if u, ok := n.(*ast.UnaryNode); ok && u.Operator == "!" {
			check(u.Node, true)
			return
		}
		if isIdent(n, name) {
			if !negated {
				found = true
			}
			return
		}
		switch v := n.(type) {
		case *ast.BinaryNode:
			check(v.Left, negated)
			check(v.Right, negated)
		case *ast.UnaryNode:
			check(v.Node, negated)
		}
	}
	check(tree.Node, false)
	return found
}
