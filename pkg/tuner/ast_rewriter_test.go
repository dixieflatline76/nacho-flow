package tuner

import (
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/expr-lang/expr"
)

func TestRewriteRuleAST_PreservesCustomGuardrails(t *testing.T) {
	existing := "Tokens < 10000 && !HasImages && Retries < 2"
	rule, err := RewriteRuleAST(existing, 24000, 0, nil, false, false)
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
	rule, err := RewriteRuleAST(existing, 20000, 0, nil, false, false)
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
	rule, err := RewriteRuleAST(existing, 18000, 0, nil, false, false)
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
	rule, err := RewriteRuleAST(existing, 16000, 0, []string{"mutex", "deadlock"}, false, false)
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
	existing := "Tokens < 8000 && Retries < 2 && !HasImages && !HasTools"
	rule, err := RewriteRuleAST(existing, 12000, 0, nil, true, true)
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

func TestRewriteRuleAST_PreservesPositiveModalityPrerequisites(t *testing.T) {
	existing := "HasImages && Retries < 2"
	rule, err := RewriteRuleAST(existing, 0, 6, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if !strings.Contains(rule, "HasImages") {
		t.Errorf("Expected positive HasImages prerequisite preserved, got: %s", rule)
	}
	if !strings.Contains(rule, "Retries < 6") {
		t.Errorf("Expected tuned Retries < 6, got: %s", rule)
	}

	program, err := expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		t.Fatalf("Synthesized rule failed expr compilation: %v", err)
	}

	// Must reject text-only turns
	outNoImg, err := expr.Run(program, contract.RequestContext{HasImages: false, Retries: 1})
	if err != nil || outNoImg != false {
		t.Errorf("Expected false for text-only turn on vision tier, got %v (err=%v)", outNoImg, err)
	}

	// Must accept image turns within retry limit
	outImg, err := expr.Run(program, contract.RequestContext{HasImages: true, Retries: 5})
	if err != nil || outImg != true {
		t.Errorf("Expected true for image turn with retries=5, got %v (err=%v)", outImg, err)
	}
}

func TestRewriteRuleAST_HandlesNegativeModalityVariants(t *testing.T) {
	// Tests stripping of alternative negative forms (HasImages == false) when restrictions are relaxed
	existing := "Tokens < 10000 && HasImages == false && HasTools == false"
	rule, err := RewriteRuleAST(existing, 15000, 0, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if rule != "Tokens < 15000" {
		t.Errorf("Expected negative modality variants stripped when restrict=false, got: %s", rule)
	}
}

func TestRewriteRuleAST_ContradictionImmunity(t *testing.T) {
	// If a tier already requires images, passing restrictImages=true must NOT produce "!HasImages && HasImages"
	existing := "HasImages && Retries < 2"
	rule, err := RewriteRuleAST(existing, 0, 4, nil, true, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if strings.Contains(rule, "!HasImages") {
		t.Errorf("Contradiction violation: rule synthesized !HasImages despite positive HasImages guardrail: %s", rule)
	}
	if !strings.Contains(rule, "HasImages") {
		t.Errorf("Expected positive HasImages to remain intact: %s", rule)
	}
}

func TestRewriteRuleAST_ComplexNestingAndEscapedQuotes(t *testing.T) {
	existing := "Tokens < 10000 && (ForcedModel == \"test\\\"model\" || ForcedModel == 'hybrid\\'tag') && any([1, 2], { # > 0 })"
	rule, err := RewriteRuleAST(existing, 20000, 0, nil, false, false)
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
	rule, err := RewriteRuleAST("", 16000, 0, nil, false, false)
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
	_, err := RewriteRuleAST("Tokens < 10000", 0, 0, nil, false, false)
	if err == nil {
		t.Fatalf("Expected error for threshold 0, got nil")
	}
	_, err = RewriteRuleAST("Tokens < 10000", -500, 0, nil, false, false)
	if err == nil {
		t.Fatalf("Expected error for negative threshold, got nil")
	}
}

func TestRewriteRuleAST_MalformedExistingRule(t *testing.T) {
	_, err := RewriteRuleAST("Tokens < && invalid", 16000, 0, nil, false, false)
	if err == nil {
		t.Fatalf("Expected error for malformed existing rule, got nil")
	}
}

func TestRewriteRuleAST_InvalidKeywordQuotes(t *testing.T) {
	// Keyword with unescaped single quote
	_, err := RewriteRuleAST("Tokens < 10000", 16000, 0, []string{"invalid'kw"}, false, false)
	if err == nil {
		t.Fatalf("Expected error for keyword containing unescaped quote, got nil")
	}
}

func TestRewriteRuleAST_WithRetries(t *testing.T) {
	// Replacing existing Retries < 2 with tuned Retries < 1
	existing := "Tokens < 16000 && Retries < 2 && !HasImages"
	rule, err := RewriteRuleAST(existing, 4000, 1, nil, true, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if !strings.Contains(rule, "Tokens < 4000") {
		t.Errorf("Expected 'Tokens < 4000', got: %s", rule)
	}
	if !strings.Contains(rule, "Retries < 1") {
		t.Errorf("Expected tuned 'Retries < 1', got: %s", rule)
	}
	if strings.Contains(rule, "Retries < 2") {
		t.Errorf("Expected old 'Retries < 2' to be replaced, got: %s", rule)
	}
	if !strings.Contains(rule, "!HasImages") {
		t.Errorf("Expected '!HasImages', got: %s", rule)
	}

	if _, err := expr.Compile(rule, expr.Env(contract.RequestContext{})); err != nil {
		t.Fatalf("Synthesized rule failed expr compilation: %v", err)
	}
}

func TestRewriteRuleAST_EmptyClausesAndDefault(t *testing.T) {
	// 1. Both empty -> returns "true"
	r1, err1 := RewriteRuleAST("", 0, 0, nil, false, false)
	if err1 != nil || r1 != "true" {
		t.Errorf("expected 'true' for all empty, got %q, err=%v", r1, err1)
	}

	// 2. Existing was autotunable but removed, no new clauses -> returns cleanExisting
	r2, err2 := RewriteRuleAST("!HasImages", 0, 0, nil, false, false)
	if err2 != nil || r2 != "!HasImages" {
		t.Errorf("expected cleanExisting preserved, got %q, err=%v", r2, err2)
	}
}

func TestIsAutoTunableClause_Keywords(t *testing.T) {
	if !isAutoTunableClause("!any(Keywords, { # in ['k8s'] })", false) {
		t.Errorf("expected !any keyword clause to be autotunable")
	}
	if !isAutoTunableClause("Retries < 2", true) {
		t.Errorf("expected Retries < 2 to be autotunable when tuningRetries is true")
	}
	if isAutoTunableClause("Retries < 2", false) {
		t.Errorf("expected Retries < 2 NOT to be autotunable when tuningRetries is false")
	}
	if isAutoTunableClause("Retries >= 2", true) {
		t.Errorf("expected Retries >= 2 NOT to be autotunable (must preserve retry floor)")
	}
	if isAutoTunableClause("Retries > 1", true) {
		t.Errorf("expected Retries > 1 NOT to be autotunable (must preserve retry floor)")
	}
	if isAutoTunableClause("CustomVar == 42", true) {
		t.Errorf("expected CustomVar not to be autotunable")
	}
}

func TestRewriteRuleAST_PreservesRetryFloor(t *testing.T) {
	existing := "Tokens < 16000 && Retries >= 3 && Retries < 6"
	rule, err := RewriteRuleAST(existing, 16000, 8, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if !strings.Contains(rule, "Retries >= 3") {
		t.Errorf("Expected retry floor 'Retries >= 3' to be preserved, got rule: %s", rule)
	}
	if !strings.Contains(rule, "Retries < 8") {
		t.Errorf("Expected tuned retry bound 'Retries < 8', got rule: %s", rule)
	}
	if strings.Contains(rule, "Retries < 6") {
		t.Errorf("Expected old retry bound 'Retries < 6' to be removed, got rule: %s", rule)
	}

	program, err := expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		t.Fatalf("Rule compilation failed: %v", err)
	}

	// Should reject Retries < 3 (below floor)
	outLow, err := expr.Run(program, contract.RequestContext{Tokens: 1000, Retries: 2})
	if err != nil || outLow != false {
		t.Errorf("Expected false for Retries=2 (below floor 3), got out=%v, err=%v", outLow, err)
	}

	// Should match Retries between 3 and 7
	outMid, err := expr.Run(program, contract.RequestContext{Tokens: 1000, Retries: 5})
	if err != nil || outMid != true {
		t.Errorf("Expected true for Retries=5, got out=%v, err=%v", outMid, err)
	}

	// Should reject Retries >= 8 (above bound)
	outHigh, err := expr.Run(program, contract.RequestContext{Tokens: 1000, Retries: 8})
	if err != nil || outHigh != false {
		t.Errorf("Expected false for Retries=8 (above bound 8), got out=%v, err=%v", outHigh, err)
	}
}

func TestRewriteRuleAST_PreservesPositiveTools(t *testing.T) {
	existing := "HasTools && Retries < 2"
	rule, err := RewriteRuleAST(existing, 0, 4, nil, false, false)
	if err != nil {
		t.Fatalf("RewriteRuleAST failed: %v", err)
	}

	if !strings.Contains(rule, "HasTools") {
		t.Errorf("Expected positive 'HasTools' to be preserved, got rule: %s", rule)
	}
	if strings.Contains(rule, "!HasTools") {
		t.Errorf("Did not expect '!HasTools' to be injected into positive tool tier, got rule: %s", rule)
	}

	program, err := expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		t.Fatalf("Rule compilation failed: %v", err)
	}

	// Should reject non-tool prompts
	outNoTools, err := expr.Run(program, contract.RequestContext{HasTools: false, Retries: 1})
	if err != nil || outNoTools != false {
		t.Errorf("Expected false for HasTools=false, got out=%v, err=%v", outNoTools, err)
	}

	// Should match tool prompts
	outTools, err := expr.Run(program, contract.RequestContext{HasTools: true, Retries: 1})
	if err != nil || outTools != true {
		t.Errorf("Expected true for HasTools=true, got out=%v, err=%v", outTools, err)
	}
}
