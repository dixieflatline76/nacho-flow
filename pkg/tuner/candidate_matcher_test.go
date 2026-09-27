package tuner

import (
	"math"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestLocalTierImmunityFilter(t *testing.T) {
	filter := &LocalTierImmunityFilter{}

	// Case 1: Local tier with cloud candidate -> REJECT
	ctxLocalCloud := &FilterContext{
		TargetTier: &TierReplayConfig{IsLocal: true, Model: "ollama/gemma4"},
		Candidate:  &CandidateModel{ModelID: "openrouter/glm-5.3", IsOpenWeights: false},
	}
	passed, reason := filter.Passes(ctxLocalCloud)
	if passed || reason != "local_tier_protected" {
		t.Errorf("expected local tier to reject cloud candidate, got passed=%v, reason=%s", passed, reason)
	}

	// Case 2: Local tier with local/open-weights candidate -> PASS
	ctxLocalLocal := &FilterContext{
		TargetTier: &TierReplayConfig{IsLocal: true, Model: "ollama/gemma4"},
		Candidate:  &CandidateModel{ModelID: "ollama/qwen2.5-coder:7b", IsOpenWeights: true},
	}
	passed, reason = filter.Passes(ctxLocalLocal)
	if !passed {
		t.Errorf("expected local tier to allow open-weights candidate, got passed=false, reason=%s", reason)
	}

	// Case 3: Cloud tier with cloud candidate -> PASS
	ctxCloudCloud := &FilterContext{
		TargetTier: &TierReplayConfig{IsLocal: false, Model: "qwen/qwen3-coder-plus"},
		Candidate:  &CandidateModel{ModelID: "z-ai/glm-5.3-flash", IsOpenWeights: false},
	}
	passed, _ = filter.Passes(ctxCloudCloud)
	if !passed {
		t.Errorf("expected cloud tier to allow cloud candidate, got passed=false")
	}
}

func TestContextParityFilter(t *testing.T) {
	filter := &ContextParityFilter{}

	// Case 1: Tier requires 160k context, candidate has only 128k -> REJECT
	ctxDeficient := &FilterContext{
		TargetTier: &TierReplayConfig{TokenThreshold: 160000},
		Candidate:  &CandidateModel{ContextLength: 128000},
	}
	passed, reason := filter.Passes(ctxDeficient)
	if passed || reason != "insufficient_context_length" {
		t.Errorf("expected rejection for 128k context on 160k tier, got passed=%v, reason=%s", passed, reason)
	}

	// Case 2: Tier requires 160k context, candidate has 200k -> PASS
	ctxSufficient := &FilterContext{
		TargetTier: &TierReplayConfig{TokenThreshold: 160000},
		Candidate:  &CandidateModel{ContextLength: 200000},
	}
	passed, _ = filter.Passes(ctxSufficient)
	if !passed {
		t.Errorf("expected pass for 200k context on 160k tier, got passed=false")
	}

	// Case 3: Tier has no explicit threshold (fallback floor 32k), candidate has only 16k -> REJECT
	ctxFloorFail := &FilterContext{
		TargetTier: &TierReplayConfig{TokenThreshold: 0},
		Candidate:  &CandidateModel{ContextLength: 16000},
	}
	passed, reason = filter.Passes(ctxFloorFail)
	if passed || reason != "insufficient_context_length" {
		t.Errorf("expected rejection for 16k context against 32k floor, got passed=%v, reason=%s", passed, reason)
	}
}

func TestModalityFilter(t *testing.T) {
	filter := &ModalityFilter{}

	// Case 1: Tier requires tools, candidate has no tools -> REJECT
	ctxToolsMissing := &FilterContext{
		TargetTier: &TierReplayConfig{SupportsTools: true},
		Candidate:  &CandidateModel{SupportsTools: false},
	}
	passed, reason := filter.Passes(ctxToolsMissing)
	if passed || reason != "missing_required_tools" {
		t.Errorf("expected rejection for missing tools, got passed=%v, reason=%s", passed, reason)
	}

	// Case 2: Tier requires vision, candidate has no vision -> REJECT
	ctxVisionMissing := &FilterContext{
		TargetTier: &TierReplayConfig{RequiresImages: true},
		Candidate:  &CandidateModel{SupportsVision: false},
	}
	passed, reason = filter.Passes(ctxVisionMissing)
	if passed || reason != "missing_required_vision" {
		t.Errorf("expected rejection for missing vision, got passed=%v, reason=%s", passed, reason)
	}

	// Case 3: Modalities satisfied -> PASS
	ctxSatisfied := &FilterContext{
		TargetTier: &TierReplayConfig{SupportsTools: true, RequiresImages: true},
		Candidate:  &CandidateModel{SupportsTools: true, SupportsVision: true},
	}
	passed, _ = filter.Passes(ctxSatisfied)
	if !passed {
		t.Errorf("expected pass when all modalities are satisfied, got passed=false")
	}
}

func TestCognitiveParityFilter(t *testing.T) {
	filter := &CognitiveParityFilter{}
	policy := &TuningPolicy{CodingParityTolerance: 0.50}

	// Case 1: Tier coding index 70.0, candidate has 68.0 -> REJECT
	ctxSubParity := &FilterContext{
		TargetTier: &TierReplayConfig{CodingIndex: 70.0},
		Candidate:  &CandidateModel{CodingIndex: 68.0},
		Policy:     policy,
	}
	passed, reason := filter.Passes(ctxSubParity)
	if passed || reason != "sub_parity_coding_index" {
		t.Errorf("expected rejection for 68.0 vs 70.0 benchmark, got passed=%v, reason=%s", passed, reason)
	}

	// Case 2: Candidate within 0.50 tolerance (69.6 vs 70.0) -> PASS
	ctxNearParity := &FilterContext{
		TargetTier: &TierReplayConfig{CodingIndex: 70.0},
		Candidate:  &CandidateModel{CodingIndex: 69.6},
		Policy:     policy,
	}
	passed, _ = filter.Passes(ctxNearParity)
	if !passed {
		t.Errorf("expected pass for 69.6 vs 70.0 within tolerance, got passed=false")
	}

	// Case 3: Candidate exceeds benchmark (71.5 vs 70.0) -> PASS
	ctxUpgrade := &FilterContext{
		TargetTier: &TierReplayConfig{CodingIndex: 70.0},
		Candidate:  &CandidateModel{CodingIndex: 71.5},
		Policy:     policy,
	}
	passed, _ = filter.Passes(ctxUpgrade)
	if !passed {
		t.Errorf("expected pass for 71.5 vs 70.0 upgrade, got passed=false")
	}
}

func TestCostReductionFilter(t *testing.T) {
	filter := &CostReductionFilter{}
	policy := &TuningPolicy{MinSavingsPct: 20.0}

	// Case 1: Tier rate $1.46, candidate rate $1.30 (10.9% savings < 20%) -> REJECT
	ctxInsufficientSavings := &FilterContext{
		TargetTier: &TierReplayConfig{ComprehensiveRate: 1.46},
		Candidate:  &CandidateModel{ComprehensiveRate: 1.30},
		Policy:     policy,
	}
	passed, reason := filter.Passes(ctxInsufficientSavings)
	if passed || reason != "insufficient_cost_savings" {
		t.Errorf("expected rejection for 10.9%% savings, got passed=%v, reason=%s", passed, reason)
	}

	// Case 2: Tier rate $1.46, candidate rate $0.28 (80.8% savings >= 20%) -> PASS
	ctxSubstantialSavings := &FilterContext{
		TargetTier: &TierReplayConfig{ComprehensiveRate: 1.46},
		Candidate:  &CandidateModel{ComprehensiveRate: 0.28},
		Policy:     policy,
	}
	passed, _ = filter.Passes(ctxSubstantialSavings)
	if !passed {
		t.Errorf("expected pass for 80.8%% savings, got passed=false")
	}
}

func TestDominanceOrderFilter(t *testing.T) {
	filter := &DominanceOrderFilter{}
	target := &TierReplayConfig{Model: "tier-2-curr", CodingIndex: 65.0}

	// Case 1: Candidate drops below predecessor benchmark -> REJECT
	ctxPredViolation := &FilterContext{
		TargetTier:    target,
		ImmediatePred: &TierReplayConfig{Model: "tier-1-pred", CodingIndex: 50.0},
		Candidate:     &CandidateModel{ModelID: "cand", CodingIndex: 45.0},
	}
	passed, reason := filter.Passes(ctxPredViolation)
	if passed || reason != "violates_predecessor_escalation" {
		t.Errorf("expected rejection for candidate below predecessor benchmark, got passed=%v, reason=%s", passed, reason)
	}

	// Case 2: Candidate duplicates predecessor model ID -> REJECT
	ctxDupPred := &FilterContext{
		TargetTier:    target,
		ImmediatePred: &TierReplayConfig{Model: "tier-1-pred", CodingIndex: 50.0},
		Candidate:     &CandidateModel{ModelID: "tier-1-pred", CodingIndex: 55.0},
	}
	passed, reason = filter.Passes(ctxDupPred)
	if passed || reason != "duplicates_predecessor_model" {
		t.Errorf("expected rejection for duplicate predecessor model ID, got passed=%v, reason=%s", passed, reason)
	}

	// Case 3: Candidate leapfrogs successor benchmark -> REJECT
	ctxSuccViolation := &FilterContext{
		TargetTier:    target,
		ImmediatePred: &TierReplayConfig{Model: "tier-1-pred", CodingIndex: 50.0},
		ImmediateSucc: &TierReplayConfig{Model: "tier-3-succ", CodingIndex: 75.0},
		Candidate:     &CandidateModel{ModelID: "cand", CodingIndex: 80.0},
	}
	passed, reason = filter.Passes(ctxSuccViolation)
	if passed || reason != "violates_successor_escalation" {
		t.Errorf("expected rejection for candidate exceeding successor benchmark, got passed=%v, reason=%s", passed, reason)
	}

	// Case 4: Candidate fits cleanly between predecessor and successor -> PASS
	ctxClean := &FilterContext{
		TargetTier:    target,
		ImmediatePred: &TierReplayConfig{Model: "tier-1-pred", CodingIndex: 50.0},
		ImmediateSucc: &TierReplayConfig{Model: "tier-3-succ", CodingIndex: 75.0},
		Candidate:     &CandidateModel{ModelID: "cand", CodingIndex: 71.5},
	}
	passed, _ = filter.Passes(ctxClean)
	if !passed {
		t.Errorf("expected pass for candidate fitting escalation order, got passed=false")
	}
}

func TestCandidateFilterPipeline_EndToEnd(t *testing.T) {
	pipeline := NewDefaultFilterPipeline()
	policy := DefaultTuningPolicy()

	targetTier := &TierReplayConfig{
		TierName:          "Tier 2: Cloud Workhorse",
		Model:             "qwen/qwen3-coder-plus",
		CodingIndex:       70.0,
		ComprehensiveRate: 1.46,
		TokenThreshold:    160000,
		SupportsTools:     true,
		IsLocal:           false,
	}

	pred := &TierReplayConfig{
		Model:       "ollama/gemma4",
		CodingIndex: 43.4,
		IsLocal:     true,
	}

	succ := &TierReplayConfig{
		Model:       "google/gemini-3.8-flash",
		CodingIndex: 76.3,
	}

	// Qualifying candidate: z-ai/glm-5.3-flash
	qualifyingCand := &CandidateModel{
		ModelID:           "z-ai/glm-5.3-flash",
		CodingIndex:       71.5,
		ComprehensiveRate: 0.28,
		ContextLength:     160000,
		SupportsTools:     true,
		IsOpenWeights:     false,
	}

	ctx := &FilterContext{
		TargetTier:    targetTier,
		Candidate:     qualifyingCand,
		ImmediatePred: pred,
		ImmediateSucc: succ,
		Policy:        &policy,
	}

	passed, reason := pipeline.Evaluate(ctx)
	if !passed {
		t.Fatalf("expected qualifying candidate to pass pipeline, failed with reason=%s", reason)
	}

	// Scorer assertion
	scorer := &CompositeValueScorer{}
	score := scorer.Score(targetTier, qualifyingCand)
	if score <= qualifyingCand.CodingIndex {
		t.Errorf("expected ValueScore > CodingIndex due to discount, got %f", score)
	}
}

func TestConvertDealToCandidate(t *testing.T) {
	deal := contract.DealInfo{
		ModelID:            "meta/muse-spark-1.2-contributor",
		Name:               "Muse Spark 1.2",
		CodingIndex:        72.2,
		PromptCostPerM:     0.10,
		CompletionCostPerM: 0.20,
		ContextLength:      160000,
		SupportsTools:      true,
		SupportsVision:     true,
	}
	cand := ConvertDealToCandidate(deal)
	if cand.ModelID != deal.ModelID {
		t.Errorf("expected ModelID %s, got %s", deal.ModelID, cand.ModelID)
	}
	expectedRate := 0.10 + 0.25*0.20 // 0.15
	if math.Abs(cand.ComprehensiveRate-expectedRate) > 1e-6 {
		t.Errorf("expected rate %f, got %f", expectedRate, cand.ComprehensiveRate)
	}
}

func BenchmarkCandidateFilterPipeline_Evaluate(b *testing.B) {
	pipeline := NewDefaultFilterPipeline()
	target := &TierReplayConfig{
		Model:             "qwen/qwen3-coder-plus",
		CodingIndex:       70.0,
		ComprehensiveRate: 1.46,
		TokenThreshold:    160000,
		SupportsTools:     true,
	}
	cand := &CandidateModel{
		ModelID:           "z-ai/glm-5.3-flash",
		CodingIndex:       71.5,
		ComprehensiveRate: 0.28,
		ContextLength:     200000,
		SupportsTools:     true,
	}
	ctx := &FilterContext{
		TargetTier: target,
		Candidate:  cand,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = pipeline.Evaluate(ctx)
	}
}
