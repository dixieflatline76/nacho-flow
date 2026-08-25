package tuner

import (
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/expr-lang/expr"
)

func TestRewriteRuleAST_PreservesCustomGuardrails(t *testing.T) {
	existing := "Tokens < 10000 && !HasImages && Retries < 2"
	rule, err := RewriteRuleAST(existing, 24000, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if !strings.Contains(rule, "Tokens < 24000") {
		t.Errorf("Expected updated Tokens < 24000, got: %s", rule)
	}
	if !strings.Contains(rule, "Retries < 2") {
		t.Errorf("Expected preserved 'Retries < 2', got: %s", rule)
	}
	if strings.Contains(rule, "!HasImages") {
		t.Errorf("Expected !HasImages to be removed when restrictImages=false, got: %s", rule)
	}

	// Verify expr compilation
	_, err = expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		t.Fatalf("Synthesized rule failed expr compilation: %v", err)
	}
}

// Test Fix 2: Preserves user variables and literals that contain tokens or keywords as substrings
func TestRewriteRuleAST_PreservesCustomTokensAndKeywordsVariables(t *testing.T) {
	existing := `Tokens < 10000 && Retries < 2 && (ForcedModel == "tokens_v2" || ForcedTier == "keywords_fast")`
	rule, err := RewriteRuleAST(existing, 20000, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if !strings.Contains(rule, "Tokens < 20000") {
		t.Errorf("Expected updated Tokens < 20000, got: %s", rule)
	}
	if !strings.Contains(rule, `ForcedModel == "tokens_v2"`) {
		t.Errorf("Expected preserved ForcedModel == \"tokens_v2\", got: %s", rule)
	}
	if !strings.Contains(rule, `ForcedTier == "keywords_fast"`) {
		t.Errorf("Expected preserved ForcedTier == \"keywords_fast\", got: %s", rule)
	}
	if !strings.Contains(rule, "Retries < 2") {
		t.Errorf("Expected preserved Retries < 2, got: %s", rule)
	}
}

// Test Fix 3: Escaped backslash before quote (e.g. "C:\\")
func TestRewriteRuleAST_EscapedBackslashBeforeQuote(t *testing.T) {
	existing := `Tokens < 10000 && (ForcedModel == "C:\\" || ForcedModel == "test\"tag") && Retries < 2`
	rule, err := RewriteRuleAST(existing, 18000, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed on escaped backslash: %v", err)
	}

	if !strings.Contains(rule, "Tokens < 18000") {
		t.Errorf("Expected Tokens < 18000, got: %s", rule)
	}
	if !strings.Contains(rule, `ForcedModel == "C:\\"`) {
		t.Errorf("Expected preserved C:\\ literal, got: %s", rule)
	}
	if !strings.Contains(rule, "Retries < 2") {
		t.Errorf("Expected preserved Retries < 2, got: %s", rule)
	}
}

func TestRewriteRuleAST_InjectsKeywordExclusion(t *testing.T) {
	existing := "Tokens < 10000 && Retries < 2"
	rule, err := RewriteRuleAST(existing, 16000, []string{"mutex", "deadlock"}, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if !strings.Contains(rule, "Tokens < 16000") {
		t.Errorf("Expected Tokens < 16000, got: %s", rule)
	}
	if !strings.Contains(rule, "Retries < 2") {
		t.Errorf("Expected preserved Retries < 2, got: %s", rule)
	}
	if !strings.Contains(rule, "!any(Keywords, { # in ['deadlock', 'mutex'] })") {
		t.Errorf("Expected sorted keyword filter, got: %s", rule)
	}

	// Test evaluation
	program, err := expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		t.Fatalf("Synthesized rule failed expr compilation: %v", err)
	}

	// Should pass: clean turn
	out1, err := expr.Run(program, contract.RequestContext{
		Tokens:   12000,
		Keywords: []string{"frontend", "css"},
		Retries:  0,
	})
	if err != nil || out1 != true {
		t.Errorf("Expected match for clean turn, got out=%v, err=%v", out1, err)
	}

	// Should fail: high friction keyword
	out2, err := expr.Run(program, contract.RequestContext{
		Tokens:   12000,
		Keywords: []string{"mutex"},
		Retries:  0,
	})
	if err != nil || out2 != false {
		t.Errorf("Expected false for friction turn, got out=%v, err=%v", out2, err)
	}

	// Should fail: retries >= 2
	out3, err := expr.Run(program, contract.RequestContext{
		Tokens:   12000,
		Keywords: []string{"frontend"},
		Retries:  2,
	})
	if err != nil || out3 != false {
		t.Errorf("Expected false for retries=2, got out=%v, err=%v", out3, err)
	}
}

func TestRewriteRuleAST_AddsRestrictImagesAndTools(t *testing.T) {
	existing := "Tokens < 8000 && Retries < 2 && HasImages && HasTools && !HasTools"
	rule, err := RewriteRuleAST(existing, 12000, nil, true, true)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if !strings.Contains(rule, "Tokens < 12000") {
		t.Errorf("Expected Tokens < 12000, got: %s", rule)
	}
	if !strings.Contains(rule, "!HasImages") {
		t.Errorf("Expected !HasImages when restrictImages=true, got: %s", rule)
	}
	if !strings.Contains(rule, "!HasTools") {
		t.Errorf("Expected !HasTools when restrictTools=true, got: %s", rule)
	}
	if !strings.Contains(rule, "Retries < 2") {
		t.Errorf("Expected preserved Retries < 2, got: %s", rule)
	}

	_, err = expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		t.Fatalf("Synthesized rule failed expr compilation: %v", err)
	}
}

func TestRewriteRuleAST_ComplexNestingAndEscapedQuotes(t *testing.T) {
	existing := "Tokens < 10000 && (ForcedModel == \"test\\\"model\" || ForcedModel == 'hybrid\\'tag') && any([1, 2], { # > 0 })"
	rule, err := RewriteRuleAST(existing, 20000, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed on complex nesting: %v", err)
	}
	if !strings.Contains(rule, "Tokens < 20000") {
		t.Errorf("Expected Tokens < 20000, got: %s", rule)
	}
	if !strings.Contains(rule, "ForcedModel") {
		t.Errorf("Expected preserved ForcedModel clause, got: %s", rule)
	}
}

func TestRewriteRuleAST_BlankInitialRule(t *testing.T) {
	rule, err := RewriteRuleAST("", 16000, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}
	if rule != "Tokens < 16000" {
		t.Errorf("Expected 'Tokens < 16000', got: %s", rule)
	}
}

func TestDistillRuleWithContext(t *testing.T) {
	rule, err := DistillRuleWithContext("Tokens < 10000 && Retries < 2", 18000, []string{"sql"}, true, false)
	if err != nil {
		t.Fatalf("DistillRuleWithContext failed: %v", err)
	}
	if !strings.Contains(rule, "Tokens < 18000") || !strings.Contains(rule, "!HasImages") || !strings.Contains(rule, "Retries < 2") {
		t.Errorf("Unexpected rule: %s", rule)
	}
}

func TestRewriteRuleAST_InvalidThreshold(t *testing.T) {
	_, err := RewriteRuleAST("Tokens < 10000", 0, nil, false, false)
	if err == nil {
		t.Fatalf("Expected error for threshold 0, got nil")
	}
	_, err = RewriteRuleAST("Tokens < 10000", -500, nil, false, false)
	if err == nil {
		t.Fatalf("Expected error for negative threshold, got nil")
	}
}

func TestRewriteRuleAST_MalformedExistingRule(t *testing.T) {
	_, err := RewriteRuleAST("Tokens < && invalid", 16000, nil, false, false)
	if err == nil {
		t.Fatalf("Expected error for malformed existing rule, got nil")
	}
}

func TestRewriteRuleAST_InvalidKeywordQuotes(t *testing.T) {
	// Keyword with unescaped single quote
	_, err := RewriteRuleAST("Tokens < 10000", 16000, []string{"invalid'kw"}, false, false)
	if err == nil {
		t.Fatalf("Expected error for keyword containing unescaped quote, got nil")
	}
}
