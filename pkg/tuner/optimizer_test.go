package tuner

import (
	"context"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/expr-lang/expr"
)

// Test 2.1: Ground Truth Scenario 1: Exact 8,000 Token Cutoff + Concurrency Keywords
func TestOptimizer_GroundTruth_Scenario1_8kCliff(t *testing.T) {
	optimizer := NewOptimizerForTest()

	var records []telemetry.TurnRecord
	// 1,500 turns: 500 to 7,900 tokens (0% retries)
	for i := 0; i < 1500; i++ {
		tok := 500 + int(float64(i)/1500.0*7400.0)
		records = append(records, telemetry.TurnRecord{
			Tokens:   tok,
			Keywords: []string{"ui", "test"},
			IsLocal:  true,
			IsRetry:  false,
		})
	}
	// 1,500 turns: 8,000 to 16,000 tokens (100% retries)
	for i := 0; i < 1500; i++ {
		tok := 8000 + int(float64(i)/1500.0*8000.0)
		records = append(records, telemetry.TurnRecord{
			Tokens:   tok,
			Keywords: []string{"ui"},
			IsLocal:  true,
			IsRetry:  true,
		})
	}
	// 200 turns: Small 2k tokens with concurrency keywords (100% retries)
	for i := 0; i < 200; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:   2000,
			Keywords: []string{"mutex", "deadlock"},
			IsLocal:  true,
			IsRetry:  true,
		})
	}

	result, err := optimizer.Optimize(records, &contract.Config{})
	if err != nil {
		t.Fatalf("Optimizer failed: %v", err)
	}

	if result.OptimalThreshold != 8000 {
		t.Errorf("Expected optimal threshold 8000, got %d", result.OptimalThreshold)
	}

	expectedKws := []string{"deadlock", "mutex"}
	if strings.Join(result.FrictionKeywords, ",") != strings.Join(expectedKws, ",") {
		t.Errorf("Expected friction keywords %v, got %v", expectedKws, result.FrictionKeywords)
	}

	if !strings.Contains(result.SynthesizedRule, "Tokens < 8000") || !strings.Contains(result.SynthesizedRule, "deadlock") {
		t.Errorf("Unexpected synthesized rule: %s", result.SynthesizedRule)
	}
}

// Test 2.2: Ground Truth Scenario 2: Exact 14,000 Token Cutoff + Clean Keywords
func TestOptimizer_GroundTruth_Scenario2_14kClean(t *testing.T) {
	optimizer := NewOptimizerForTest()

	var records []telemetry.TurnRecord
	// 2,500 turns: 500 to 13,900 tokens (0% retries)
	for i := 0; i < 2500; i++ {
		tok := 500 + int(float64(i)/2500.0*13400.0)
		records = append(records, telemetry.TurnRecord{
			Tokens:   tok,
			Keywords: []string{"refactor", "doc"},
			IsLocal:  true,
			IsRetry:  false,
		})
	}
	// 1,000 turns: 14,000 to 24,000 tokens (100% retries)
	for i := 0; i < 1000; i++ {
		tok := 14000 + int(float64(i)/1000.0*10000.0)
		records = append(records, telemetry.TurnRecord{
			Tokens:   tok,
			Keywords: []string{"refactor"},
			IsLocal:  true,
			IsRetry:  true,
		})
	}

	result, err := optimizer.Optimize(records, &contract.Config{})
	if err != nil {
		t.Fatalf("Optimizer failed: %v", err)
	}

	if result.OptimalThreshold != 14000 {
		t.Errorf("Expected optimal threshold 14000, got %d", result.OptimalThreshold)
	}
	if len(result.FrictionKeywords) != 0 {
		t.Errorf("Expected 0 friction keywords, got %v", result.FrictionKeywords)
	}
	if result.SynthesizedRule != "Tokens < 14000" {
		t.Errorf("Unexpected synthesized rule: %s", result.SynthesizedRule)
	}
}

// Test 2.3: Ground Truth Scenario 3: Exact 10,000 Token Cutoff + SQL Keywords
func TestOptimizer_GroundTruth_Scenario3_10kSQL(t *testing.T) {
	optimizer := NewOptimizerForTest()

	var records []telemetry.TurnRecord
	// 2,000 turns: 500 to 9,900 tokens (0% retries)
	for i := 0; i < 2000; i++ {
		tok := 500 + int(float64(i)/2000.0*9400.0)
		records = append(records, telemetry.TurnRecord{
			Tokens:   tok,
			Keywords: []string{"frontend", "css"},
			IsLocal:  true,
			IsRetry:  false,
		})
	}
	// 1,000 turns: 10,000 to 20,000 tokens (100% retries)
	for i := 0; i < 1000; i++ {
		tok := 10000 + int(float64(i)/1000.0*10000.0)
		records = append(records, telemetry.TurnRecord{
			Tokens:   tok,
			Keywords: []string{"frontend"},
			IsLocal:  true,
			IsRetry:  true,
		})
	}
	// 300 turns: Small tokens with SQL keywords (100% retries)
	for i := 0; i < 300; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:   3000,
			Keywords: []string{"sql", "postgres", "migration"},
			IsLocal:  true,
			IsRetry:  true,
		})
	}

	result, err := optimizer.Optimize(records, &contract.Config{})
	if err != nil {
		t.Fatalf("Optimizer failed: %v", err)
	}

	if result.OptimalThreshold != 10000 {
		t.Errorf("Expected optimal threshold 10000, got %d", result.OptimalThreshold)
	}

	expectedKws := []string{"migration", "postgres", "sql"}
	if strings.Join(result.FrictionKeywords, ",") != strings.Join(expectedKws, ",") {
		t.Errorf("Expected friction keywords %v, got %v", expectedKws, result.FrictionKeywords)
	}
}

// Test 2.4: Multimodal Local Vision Success (64GB GPU)
func TestOptimizer_Multimodal_LocalVisionSuccess(t *testing.T) {
	optimizer := NewCostPenaltyOptimizer()

	var records []telemetry.TurnRecord
	for i := 0; i < 500; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:    4000,
			HasImages: true,
			IsLocal:   true,
			IsRetry:   false,
		})
	}

	result, err := optimizer.Optimize(records, &contract.Config{})
	if err != nil {
		t.Fatalf("Optimizer failed: %v", err)
	}

	if result.RestrictImages {
		t.Errorf("Expected RestrictImages=false for clean vision turns")
	}
	if strings.Contains(result.SynthesizedRule, "!HasImages") {
		t.Errorf("Rule should NOT restrict images: %s", result.SynthesizedRule)
	}
}

// Test 2.5: Multimodal Local Vision High Friction
func TestOptimizer_Multimodal_LocalVisionFriction(t *testing.T) {
	optimizer := NewCostPenaltyOptimizer()

	var records []telemetry.TurnRecord
	// 100 normal turns
	for i := 0; i < 100; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:    2000,
			HasImages: false,
			IsLocal:   true,
			IsRetry:   i < 5,
		})
	}
	// 20 turns with images (100% fail)
	for i := 0; i < 20; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:    2000,
			HasImages: true,
			IsLocal:   true,
			IsRetry:   true,
		})
	}

	result, err := optimizer.Optimize(records, &contract.Config{})
	if err != nil {
		t.Fatalf("Optimizer failed: %v", err)
	}

	if !result.RestrictImages {
		t.Errorf("Expected RestrictImages=true for failing vision turns")
	}
	if !strings.Contains(result.SynthesizedRule, "!HasImages") {
		t.Errorf("Rule should restrict images: %s", result.SynthesizedRule)
	}
}

// Test 2.6: Preserves User Guardrails (Retries < 2)
func TestOptimizer_PreservesGuardrails(t *testing.T) {
	optimizer := NewCostPenaltyOptimizer()

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Local GPU",
				Provider: "ollama",
				When:     "Tokens < 10000 && !HasImages && Retries < 2",
			},
		},
	}

	var records []telemetry.TurnRecord
	for i := 0; i < 100; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:  3000,
			IsLocal: true,
			IsRetry: false,
		})
	}

	result, err := optimizer.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimizer failed: %v", err)
	}

	if !strings.Contains(result.SynthesizedRule, "Retries < 2") {
		t.Errorf("Expected preserved Retries < 2 guardrail, got: %s", result.SynthesizedRule)
	}
}

// Test 2.7: Respects Tier MaxContext
func TestOptimizer_RespectsMaxContext(t *testing.T) {
	optimizer := NewCostPenaltyOptimizer()

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:       "Local GPU Free",
				Provider:   "ollama",
				MaxContext: 20000,
				When:       "Tokens < 10000",
			},
		},
	}

	var records []telemetry.TurnRecord
	for i := 0; i < 200; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:  1000 + i*150,
			IsLocal: true,
			IsRetry: false,
		})
	}

	result, err := optimizer.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimizer failed: %v", err)
	}

	if result.OptimalThreshold > 20000 {
		t.Errorf("Expected threshold <= 20000, got: %d", result.OptimalThreshold)
	}
}

// Test Fix 4: Historical cloud retries do not inflate local retries avoided
func TestOptimizer_CloudRetriesDoNotInflateAvoidedRetries(t *testing.T) {
	optimizer := NewCostPenaltyOptimizer()

	var records []telemetry.TurnRecord
	// 50 turns on cloud that had retries (e.g. rate limits)
	for i := 0; i < 50; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:  30000,
			IsLocal: false,
			IsRetry: true,
		})
	}
	// 50 turns on local, perfectly clean (0 retries)
	for i := 0; i < 50; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:  2000,
			IsLocal: true,
			IsRetry: false,
		})
	}

	result, err := optimizer.Optimize(records, &contract.Config{})
	if err != nil {
		t.Fatalf("Optimizer failed: %v", err)
	}

	// Because local turns had 0 retries, retries eliminated MUST be 0 (not 50)
	if result.RetriesEliminated != 0 {
		t.Errorf("Expected 0 retries eliminated when local turns had 0 retries, got: %d", result.RetriesEliminated)
	}
}

// Test IsLocalTier detector covering various local and cloud configurations
func TestIsLocalTier_Detection(t *testing.T) {
	tests := []struct {
		tier     contract.Tier
		expected bool
	}{
		{contract.Tier{Provider: "ollama", Name: "Tier 1"}, true},
		{contract.Tier{Provider: "vllm", Name: "Tier 1"}, true},
		{contract.Tier{Provider: "lmstudio", Name: "Tier 1"}, true},
		{contract.Tier{Provider: "localai", Name: "Tier 1"}, true},
		{contract.Tier{Provider: "llama.cpp", Name: "Tier 1"}, true},
		{contract.Tier{Provider: "llamacpp", Name: "Tier 1"}, true},
		{contract.Tier{Provider: "custom", Name: "My Local GPU"}, true},
		{contract.Tier{Provider: "custom", Name: "ROCm GPU Workstation"}, true},
		{contract.Tier{Provider: "custom", Name: "On-Premises Server"}, true},
		{contract.Tier{Provider: "openrouter", Name: "Claude Sonnet"}, false},
		{contract.Tier{Provider: "anthropic", Name: "Claude Opus"}, false},
		{contract.Tier{Provider: "openai", Name: "GPT-4o"}, false},
	}

	for _, tc := range tests {
		got := IsLocalTier(tc.tier)
		if got != tc.expected {
			t.Errorf("IsLocalTier(%+v) = %v, expected %v", tc.tier, got, tc.expected)
		}
	}
}

// Test 2.8: AST verification on distilled rules
func TestDistiller_ASTValidation(t *testing.T) {
	rule, err := DistillRule(12000, []string{"sql", "deadlock"})
	if err != nil {
		t.Fatalf("DistillRule failed: %v", err)
	}

	program, err := expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		t.Fatalf("expr.Compile failed: %v", err)
	}

	// Test evaluating RequestContext against program
	output, err := expr.Run(program, contract.RequestContext{
		Tokens:    8000,
		Keywords:  []string{"test"},
		HasImages: false,
		HasTools:  false,
	})
	if err != nil {
		t.Fatalf("expr.Run failed: %v", err)
	}
	if output != true {
		t.Errorf("Expected match true for 8000 tokens without SQL, got %v", output)
	}
}

func TestOptimizer_ZeroRecords(t *testing.T) {
	optimizer := NewCostPenaltyOptimizer()
	if optimizer.Name() != "cost_penalty" {
		t.Errorf("Expected name 'cost_penalty', got %q", optimizer.Name())
	}

	result, err := optimizer.Optimize(nil, &contract.Config{})
	if err != nil {
		t.Fatalf("Optimize failed on nil records: %v", err)
	}
	if result.OptimalThreshold != 16000 {
		t.Errorf("Expected default 16000 threshold, got %d", result.OptimalThreshold)
	}

	// Zero records with existing tier
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "Local GPU", Provider: "ollama", When: "Tokens < 8000 && Retries < 2"},
		},
	}
	res2, err := optimizer.Optimize(nil, cfg)
	if err != nil {
		t.Fatalf("Optimize failed on zero records with existing config: %v", err)
	}
	if !strings.Contains(res2.SynthesizedRule, "Retries < 2") {
		t.Errorf("Expected preserved Retries < 2 on zero records, got: %s", res2.SynthesizedRule)
	}

	// Zero records with malformed existing when
	badCfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "Local GPU", Provider: "ollama", When: "Tokens < && bad"},
		},
	}
	_, err = optimizer.Optimize(nil, badCfg)
	if err == nil {
		t.Fatalf("Expected error on zero records with malformed when expr")
	}

	// With records and malformed existing when
	_, err = optimizer.Optimize([]telemetry.TurnRecord{{IsLocal: true, Tokens: 500}}, badCfg)
	if err == nil {
		t.Fatalf("Expected error on malformed when expr with records")
	}
}

func TestOptimizer_RetriesAvoidanceClamping(t *testing.T) {
	optimizer := NewCostPenaltyOptimizer()
	// Records with 0 retries initially
	records := []telemetry.TurnRecord{
		{Tokens: 500, IsLocal: true, IsRetry: false},
	}
	result, err := optimizer.Optimize(records, &contract.Config{})
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}
	if result.RetriesEliminated < 0 {
		t.Errorf("Expected RetriesEliminated >= 0, got %d", result.RetriesEliminated)
	}
}

func TestDistiller_InvalidThreshold(t *testing.T) {
	_, err := DistillRule(0, nil)
	if err == nil {
		t.Errorf("Expected error for threshold 0, got nil")
	}
}

func TestOptimizer_OptimizeWithContext(t *testing.T) {
	optimizer := NewCostPenaltyOptimizer()

	// 1. Success case
	ctx := context.Background()
	res, err := optimizer.OptimizeWithContext(ctx, nil, &contract.Config{})
	if err != nil || res == nil {
		t.Fatalf("expected success on background context, got err: %v", err)
	}

	// 2. Already cancelled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = optimizer.OptimizeWithContext(cancCtx, nil, &contract.Config{})
	if err == nil {
		t.Fatalf("expected error on cancelled context, got nil")
	}
}

func TestOptimizer_CustomPolicy(t *testing.T) {
	policy := TuningPolicy{
		Name:                "custom_frugal",
		CostPerMillionCloud: 1.00,
		RetryPenaltyUSD:     0.50,
		MinOccurrences:      5,
		OddsRatioThreshold:  2.0,
	}
	opt := NewCostPenaltyOptimizerWithPolicy(policy)
	if opt.Policy.Name != "custom_frugal" {
		t.Errorf("Expected policy name custom_frugal, got: %s", opt.Policy.Name)
	}
}

func NewOptimizerForTest() *CostPenaltyOptimizer {
	return NewCostPenaltyOptimizer()
}
