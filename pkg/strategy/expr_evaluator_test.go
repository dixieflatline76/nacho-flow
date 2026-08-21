package strategy

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestFullRooCodeAgentTurnSequence(t *testing.T) {
	tiers := []contract.Tier{
		// Tier 1: Concurrency & Reasoning
		{
			Name:     "Cloud Reasoning",
			Model:    "deepseek/deepseek-r1",
			Provider: "openrouter",
			When:     "any(Keywords, { # in ['deadlock', 'mutex', 'race', 'concurrency'] })",
		},
		// Tier 2: Multimodal Vision
		{
			Name:     "Cloud Vision",
			Model:    "google/gemini-2.5-flash-lite",
			Provider: "openrouter",
			When:     "HasImages",
		},
		// Tier 3: Local ROCm GPU (< 16k context, no images or tools)
		{
			Name:        "Local ROCm GPU",
			Model:       "qwen2.5-coder:14b",
			Provider:    "ollama",
			When:        "Tokens < 16000 && !HasImages && !HasTools",
			StripImages: true,
		},
		// Tier 4: Fast Cloud Coder (large context >= 16k or active tools)
		{
			Name:     "Cloud Agentic Fast",
			Model:    "qwen/qwen3-coder-30b-a3b-instruct",
			Provider: "openrouter",
			When:     "Tokens >= 16000 || HasTools",
		},
	}

	defaultTier := contract.Tier{
		Name:     "Fallback",
		Model:    "deepseek/deepseek-v4-flash-latest",
		Provider: "openrouter",
	}

	eval, err := NewExprEvaluator(tiers, defaultTier)
	if err != nil {
		t.Fatalf("Failed to initialize ExprEvaluator: %v", err)
	}

	// Turn 1: Screenshot attached -> must route to Cloud Vision tier
	turn1 := contract.RequestContext{Tokens: 500, HasImages: true, HasTools: false}
	r1, _ := eval.SelectTier(turn1)
	if r1.Name != "Cloud Vision" {
		t.Errorf("Turn 1 expected 'Cloud Vision', got '%s'", r1.Name)
	}

	// Turn 2: Roo Code sends tool definitions for list_files -> must route to Cloud Agentic Fast tier
	turn2 := contract.RequestContext{Tokens: 1500, HasImages: false, HasTools: true}
	r2, _ := eval.SelectTier(turn2)
	if r2.Name != "Cloud Agentic Fast" {
		t.Errorf("Turn 2 expected 'Cloud Agentic Fast', got '%s'", r2.Name)
	}

	// Turn 3: Concurrency deadlock inquiry -> must route to Cloud Reasoning tier
	turn3 := contract.RequestContext{Tokens: 2000, Keywords: []string{"deadlock", "goroutine"}}
	r3, _ := eval.SelectTier(turn3)
	if r3.Name != "Cloud Reasoning" {
		t.Errorf("Turn 3 expected 'Cloud Reasoning', got '%s'", r3.Name)
	}

	// Turn 4: Routine small code edit -> must route to Local ROCm GPU tier
	turn4 := contract.RequestContext{Tokens: 3000, HasImages: false, HasTools: false}
	r4, _ := eval.SelectTier(turn4)
	if r4.Name != "Local ROCm GPU" {
		t.Errorf("Turn 4 expected 'Local ROCm GPU', got '%s'", r4.Name)
	}

	// Turn 5: Fallback when no tier matches
	evalFallbackOnly, _ := NewExprEvaluator([]contract.Tier{
		{Name: "Strict Tier", When: "Tokens == 999999"},
	}, defaultTier)
	r5, _ := evalFallbackOnly.SelectTier(contract.RequestContext{Tokens: 100})
	if r5.Name != "Fallback" {
		t.Errorf("Turn 5 expected 'Fallback', got '%s'", r5.Name)
	}
}

// Test compile error with invalid expr syntax
func TestExprEvaluator_InvalidExprSyntax_ReturnsError(t *testing.T) {
	tiers := []contract.Tier{
		{Name: "Bad Tier", When: "Tokens <<< 100"},
	}
	_, err := NewExprEvaluator(tiers, contract.Tier{Name: "Default"})
	if err == nil {
		t.Fatalf("Expected error for invalid expr syntax, got nil")
	}
}

// Test tier with empty When and non-boolean expr result
func TestExprEvaluator_EmptyWhenAndNonBoolResult(t *testing.T) {
	tiers := []contract.Tier{
		{Name: "Empty When", When: ""},
		{Name: "String Result", When: "'not_a_boolean'"},
	}
	eval, err := NewExprEvaluator(tiers, contract.Tier{Name: "DefaultFallback"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	result, err := eval.SelectTier(contract.RequestContext{Tokens: 100})
	if err != nil {
		t.Fatalf("Unexpected error during select: %v", err)
	}
	if result.Name != "DefaultFallback" {
		t.Errorf("Expected fallback to DefaultFallback, got '%s'", result.Name)
	}
}

// Test tier where expr returns a runtime error (e.g. index out of range)
func TestExprEvaluator_RuntimeEvaluationError(t *testing.T) {
	tiers := []contract.Tier{
		{Name: "IndexOutOfRange", When: "Keywords[99] == 'test'"},
	}
	eval, err := NewExprEvaluator(tiers, contract.Tier{Name: "DefaultFallback"})
	if err != nil {
		t.Fatalf("Unexpected compilation error: %v", err)
	}

	// Passing empty Keywords causes index out of range error during expr.Run
	result, err := eval.SelectTier(contract.RequestContext{Keywords: []string{}})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Name != "DefaultFallback" {
		t.Errorf("Expected fallback to DefaultFallback on runtime error, got '%s'", result.Name)
	}
}

func TestEvaluator_RetryAutoEscalation(t *testing.T) {
	tiers := []contract.Tier{
		{
			Name:     "Local Tier",
			Model:    "qwen2.5-coder:14b",
			Provider: "ollama",
			When:     "Tokens < 16000 && !HasImages && !HasTools && Retries < 2",
		},
	}
	defaultTier := contract.Tier{
		Name:     "Cloud Fallback",
		Model:    "anthropic/claude-3.5-sonnet",
		Provider: "openrouter",
	}

	eval, err := NewExprEvaluator(tiers, defaultTier)
	if err != nil {
		t.Fatalf("Failed to compile evaluator: %v", err)
	}

	// Turn 0: Fresh request (Retries: 0) -> routes to Local Tier
	t0, _ := eval.SelectTier(contract.RequestContext{Tokens: 4000, Retries: 0, IsRetry: false})
	if t0.Name != "Local Tier" {
		t.Errorf("Expected Turn 0 to route to 'Local Tier', got %q", t0.Name)
	}

	// Turn 1: First retry (Retries: 1) -> still routes to Local Tier
	t1, _ := eval.SelectTier(contract.RequestContext{Tokens: 4000, Retries: 1, IsRetry: true})
	if t1.Name != "Local Tier" {
		t.Errorf("Expected Turn 1 to route to 'Local Tier', got %q", t1.Name)
	}

	// Turn 2: Second consecutive retry (Retries: 2) -> auto-escalates to Cloud Fallback
	t2, _ := eval.SelectTier(contract.RequestContext{Tokens: 4000, Retries: 2, IsRetry: true})
	if t2.Name != "Cloud Fallback" {
		t.Errorf("Expected Turn 2 to auto-escalate to 'Cloud Fallback', got %q", t2.Name)
	}
}

func TestEvaluator_MaxContext_SkipsTier(t *testing.T) {
	tiers := []contract.Tier{
		{
			Name:       "Local 8k GPU",
			Model:      "qwen2.5-coder:7b",
			Provider:   "ollama",
			When:       "Tokens < 16000",
			MaxContext: 8192,
		},
	}
	defaultTier := contract.Tier{
		Name:     "Cloud 128k Fallback",
		Model:    "anthropic/claude-3.5-sonnet",
		Provider: "openrouter",
	}

	eval, err := NewExprEvaluator(tiers, defaultTier)
	if err != nil {
		t.Fatalf("Failed to compile evaluator: %v", err)
	}

	// 1. Tokens = 6000 <= MaxContext (8192) and Tokens < 16000 -> routes to Local 8k GPU
	r1, _ := eval.SelectTier(contract.RequestContext{Tokens: 6000})
	if r1.Name != "Local 8k GPU" {
		t.Errorf("Expected 'Local 8k GPU', got %q", r1.Name)
	}

	// 2. Tokens = 10000 > MaxContext (8192) even though Tokens < 16000 -> must skip Local 8k GPU and route to Cloud Fallback
	r2, _ := eval.SelectTier(contract.RequestContext{Tokens: 10000})
	if r2.Name != "Cloud 128k Fallback" {
		t.Errorf("Expected 'Cloud 128k Fallback' when exceeding MaxContext, got %q", r2.Name)
	}
}

// BenchmarkExprEvaluator measures nanosecond tier evaluation speed.
func BenchmarkExprEvaluator(b *testing.B) {
	tiers := []contract.Tier{
		{Name: "Reasoning", Model: "r1", Provider: "openrouter", When: "any(Keywords, { # in ['mutex', 'deadlock'] })"},
		{Name: "Vision", Model: "flash", Provider: "openrouter", When: "HasImages"},
		{Name: "Local", Model: "qwen", Provider: "ollama", When: "Tokens < 16000 && !HasImages && !HasTools"},
	}
	defaultTier := contract.Tier{Name: "Default", Model: "v4-flash", Provider: "openrouter"}

	eval, _ := NewExprEvaluator(tiers, defaultTier)
	reqCtx := contract.RequestContext{Tokens: 5000, HasImages: false, HasTools: false, Keywords: []string{"go", "slice"}}

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = eval.SelectTier(reqCtx)
	}
}
