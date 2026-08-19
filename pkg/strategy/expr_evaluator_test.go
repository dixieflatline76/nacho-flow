package strategy

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestExprEvaluator(t *testing.T) {
	tiers := []contract.Tier{
		// Higher precedence: Reasoning Keywords
		{
			Name:     "Reasoning Tier",
			Model:    "deepseek/deepseek-r1",
			Provider: "openrouter",
			When:     "any(Keywords, { # in ['deadlock', 'mutex'] })",
		},
		// Higher precedence: Vision Screenshots
		{
			Name:     "Vision Tier",
			Model:    "google/gemini-2.5-flash-lite",
			Provider: "openrouter",
			When:     "HasImages",
		},
		// Lower precedence: Routine Local GPU (< 16k)
		{
			Name:     "Local Tier",
			Model:    "qwen2.5-coder:14b",
			Provider: "ollama",
			When:     "Tokens < 16000 && !HasImages && !HasTools",
		},
	}

	defaultTier := contract.Tier{
		Name:     "Fallback",
		Model:    "deepseek/deepseek-chat",
		Provider: "openrouter",
	}

	eval, err := NewExprEvaluator(tiers, defaultTier)
	if err != nil {
		t.Fatalf("Failed to initialize ExprEvaluator: %v", err)
	}

	// Test Case 1: Reasoning match (high precedence)
	ctx1 := contract.RequestContext{Tokens: 5000, Keywords: []string{"mutex", "lock"}}
	t1, _ := eval.SelectTier(ctx1)
	if t1.Name != "Reasoning Tier" {
		t.Errorf("Expected Reasoning Tier, got %s", t1.Name)
	}

	// Test Case 2: Vision match (high precedence)
	ctx2 := contract.RequestContext{Tokens: 5000, HasImages: true}
	t2, _ := eval.SelectTier(ctx2)
	if t2.Name != "Vision Tier" {
		t.Errorf("Expected Vision Tier, got %s", t2.Name)
	}

	// Test Case 3: Routine Local match
	ctx3 := contract.RequestContext{Tokens: 5000, HasImages: false, HasTools: false}
	t3, _ := eval.SelectTier(ctx3)
	if t3.Name != "Local Tier" {
		t.Errorf("Expected Local Tier, got %s", t3.Name)
	}

	// Test Case 4: Default Fallback
	ctx4 := contract.RequestContext{Tokens: 25000, HasImages: false, HasTools: true}
	t4, _ := eval.SelectTier(ctx4)
	if t4.Name != "Fallback" {
		t.Errorf("Expected Fallback, got %s", t4.Name)
	}
}
