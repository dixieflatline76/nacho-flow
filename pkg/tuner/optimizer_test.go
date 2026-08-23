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
	if result.SynthesizedRule != "Tokens < 14000 && !HasImages && !HasTools" {
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

// Test 2.4: AST verification on distilled rules
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

func NewOptimizerForTest() *CostPenaltyOptimizer {
	return &CostPenaltyOptimizer{
		MinOccurrences:      10,
		OddsRatioThreshold:  1.5,
		CostPerMillionCloud: 2.50,
		RetryPenaltyUSD:     2.00,
	}
}

