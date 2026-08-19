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
	for i := 0; i < b.N; i++ {
		_, _ = eval.SelectTier(reqCtx)
	}
}
