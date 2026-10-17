package tuner

import (
	"math"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
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

	b.ReportAllocs()
	for b.Loop() {
		_, _ = pipeline.Evaluate(ctx)
	}
}

func TestCandidateFilters_NamesAndNilContext(t *testing.T) {
	filters := []struct {
		filter CandidateFilter
		name   string
	}{
		{&ProviderCompatibilityFilter{}, "provider_compatibility"},
		{&LocalTierImmunityFilter{}, "local_tier_immunity"},
		{&ContextParityFilter{}, "context_parity"},
		{&ModalityFilter{}, "modality_preservation"},
		{&CognitiveParityFilter{}, "cognitive_parity"},
		{&CostReductionFilter{}, "cost_reduction"},
		{&DominanceOrderFilter{}, "dominance_order"},
	}
	for _, f := range filters {
		if f.filter.Name() != f.name {
			t.Errorf("expected filter name %q, got %q", f.name, f.filter.Name())
		}
		// Test nil context
		passed, reason := f.filter.Passes(&FilterContext{})
		if passed || reason != "nil_context" {
			t.Errorf("expected nil_context rejection for %s, got passed=%v, reason=%s", f.name, passed, reason)
		}
	}

	// Test ConvertTierToCandidate with nil
	nilCand := ConvertTierToCandidate(nil)
	if nilCand.ModelID != "" {
		t.Errorf("expected empty CandidateModel for nil tier, got %+v", nilCand)
	}

	// Test ConvertTierToCandidate with populated tier
	tier := &TierReplayConfig{
		Model:                    "test-model",
		CodingIndex:              85.0,
		ToolReliability:          0.95,
		PromptCostPerMillion:     1.0,
		CompletionCostPerMillion: 2.0,
		ComprehensiveRate:        1.5,
		SupportsVision:           true,
		SupportsTools:            true,
		IsLocal:                  true,
	}
	c := ConvertTierToCandidate(tier)
	if c.ModelID != "test-model" || !c.IsOpenWeights {
		t.Errorf("unexpected candidate from tier: %+v", c)
	}
}

func TestCandidatePipeline_EdgeCases(t *testing.T) {
	var nilPipeline *CandidateFilterPipeline
	passed, reason := nilPipeline.Evaluate(&FilterContext{})
	if !passed || reason != "" {
		t.Errorf("expected nil pipeline to pass, got passed=%v, reason=%s", passed, reason)
	}

	emptyPipeline := &CandidateFilterPipeline{}
	passed, reason = emptyPipeline.Evaluate(&FilterContext{})
	if !passed || reason != "" {
		t.Errorf("expected empty pipeline to pass, got passed=%v, reason=%s", passed, reason)
	}
}

func TestCompositeValueScorer_NilCases(t *testing.T) {
	scorer := &CompositeValueScorer{}
	if score := scorer.Score(nil, nil); score != 0.0 {
		t.Errorf("expected 0.0 for nil target and cand, got %f", score)
	}
	tier := &TierReplayConfig{Model: "m1", ComprehensiveRate: 1.0}
	if score := scorer.Score(tier, nil); score != 0.0 {
		t.Errorf("expected 0.0 for nil cand, got %f", score)
	}
	cand := &CandidateModel{ModelID: "c1", CodingIndex: 80.0}
	if score := scorer.Score(nil, cand); score != 0.0 {
		t.Errorf("expected 0.0 for nil target, got %f", score)
	}
}

func TestCandidateModel_IsFrontier(t *testing.T) {
	candRole := CandidateModel{TierRole: string(curation.RoleDeepReasoner)}
	if !candRole.IsFrontier() {
		t.Error("expected RoleDeepReasoner to be frontier")
	}

	candRec := CandidateModel{
		RecommendedTiers: []string{contract.TierIDFrontier},
		CodingIndex:      75.0,
	}
	if !candRec.IsFrontier() {
		t.Error("expected TierIDFrontier with coding index >= 70 to be frontier")
	}

	candLowIndex := CandidateModel{
		RecommendedTiers: []string{contract.TierIDFrontier},
		CodingIndex:      60.0,
	}
	if candLowIndex.IsFrontier() {
		t.Error("expected low coding index not to be frontier")
	}

	candStandard := CandidateModel{TierRole: "workhorse"}
	if candStandard.IsFrontier() {
		t.Error("expected standard workhorse not to be frontier")
	}
}

func TestEffectiveCodingIndex(t *testing.T) {
	base := 50.0

	// 1. Empty / unstated effort preserves base
	if eff := EffectiveCodingIndex(base, ""); eff != 50.0 {
		t.Errorf("expected 50.0 for empty effort, got %f", eff)
	}

	// 2. Multipliers across effort levels
	tests := []struct {
		effort   string
		expected float64
	}{
		{"low", 50.0 * 1.02},
		{"LOW", 50.0 * 1.02},
		{"med", 50.0 * 1.06},
		{"medium", 50.0 * 1.06},
		{"high", 50.0 * 1.10},
		{"max", 50.0 * 1.14},
		{"unknown", 50.0},
	}

	for _, tc := range tests {
		got := EffectiveCodingIndex(base, tc.effort)
		if math.Abs(got-tc.expected) > 1e-6 {
			t.Errorf("EffectiveCodingIndex(50.0, %q) = %f, expected %f", tc.effort, got, tc.expected)
		}
	}

	// Zero base produces zero
	if eff := EffectiveCodingIndex(0, "high"); eff != 0 {
		t.Errorf("expected 0 for base 0, got %f", eff)
	}
}

func TestProviderCompatibilityFilter(t *testing.T) {
	filter := &ProviderCompatibilityFilter{}

	anthropicProviders := map[string]contract.ProviderConfig{
		"anthropic": {
			BaseURL: "https://api.anthropic.com",
			Type:    contract.ProviderTypeAnthropic,
		},
	}

	openrouterProviders := map[string]contract.ProviderConfig{
		"openrouter": {
			BaseURL: "https://openrouter.ai/api/v1",
			Type:    contract.ProviderTypeOpenRouter,
		},
	}

	dualProviders := map[string]contract.ProviderConfig{
		"anthropic": {
			BaseURL: "https://api.anthropic.com",
			Type:    contract.ProviderTypeAnthropic,
		},
		"openrouter": {
			BaseURL: "https://openrouter.ai/api/v1",
			Type:    contract.ProviderTypeOpenRouter,
		},
	}

	openaiProviders := map[string]contract.ProviderConfig{
		"openai": {
			BaseURL: "https://api.openai.com/v1",
			Type:    contract.ProviderTypeCloud,
		},
	}

	googleProviders := map[string]contract.ProviderConfig{
		"google": {
			BaseURL: "https://generativelanguage.googleapis.com",
			Type:    contract.ProviderTypeCloud,
		},
	}

	localProviders := map[string]contract.ProviderConfig{
		"llama_cluster": {
			BaseURL: "http://127.0.0.1:8080/v1",
			Type:    contract.ProviderTypeLocal,
		},
	}

	// 1. Tier with provider: "anthropic" must REJECT DeepSeek
	ctxAnthropicDeepSeek := &FilterContext{
		TargetTier: &TierReplayConfig{Provider: "anthropic", Model: "claude-haiku-4-5"},
		Candidate:  &CandidateModel{ModelID: "deepseek/deepseek-v4-flash-0731"},
		Providers:  anthropicProviders,
	}
	passed, reason := filter.Passes(ctxAnthropicDeepSeek)
	if passed || !strings.HasPrefix(reason, "provider_model_incompatible") {
		t.Errorf("expected rejection for DeepSeek on anthropic tier, got passed=%v, reason=%s", passed, reason)
	}

	// 2. Tier with provider: "anthropic" must REJECT GLM
	ctxAnthropicGLM := &FilterContext{
		TargetTier: &TierReplayConfig{Provider: "anthropic", Model: "claude-haiku-4-5"},
		Candidate:  &CandidateModel{ModelID: "z-ai/glm-5.3-flash"},
		Providers:  anthropicProviders,
	}
	passed, reason = filter.Passes(ctxAnthropicGLM)
	if passed || !strings.HasPrefix(reason, "provider_model_incompatible") {
		t.Errorf("expected rejection for GLM on anthropic tier, got passed=%v, reason=%s", passed, reason)
	}

	// 3. Even if openrouter is ALSO declared in providers, a tier with provider: "anthropic" must REJECT DeepSeek
	ctxDualAnthropicTier := &FilterContext{
		TargetTier: &TierReplayConfig{Provider: "anthropic", Model: "claude-haiku-4-5"},
		Candidate:  &CandidateModel{ModelID: "deepseek/deepseek-v4-flash-0731"},
		Providers:  dualProviders,
	}
	passed, reason = filter.Passes(ctxDualAnthropicTier)
	if passed || !strings.HasPrefix(reason, "provider_model_incompatible") {
		t.Errorf("expected rejection for DeepSeek on anthropic tier even with dual providers, got passed=%v, reason=%s", passed, reason)
	}

	// 4. Tier with provider: "anthropic" must ALLOW Claude candidate
	ctxAnthropicClaude := &FilterContext{
		TargetTier: &TierReplayConfig{Provider: "anthropic", Model: "claude-haiku-4-5"},
		Candidate:  &CandidateModel{ModelID: "claude-sonnet-5-5"},
		Providers:  anthropicProviders,
	}
	passed, _ = filter.Passes(ctxAnthropicClaude)
	if !passed {
		t.Errorf("expected pass for Claude model on anthropic tier, got passed=false")
	}

	// 5. Tier with provider: "openrouter" must ALLOW cross-vendor candidate
	ctxOpenRouterDeepSeek := &FilterContext{
		TargetTier: &TierReplayConfig{Provider: "openrouter", Model: "qwen/qwen3-coder-plus"},
		Candidate:  &CandidateModel{ModelID: "deepseek/deepseek-v4-flash-0731"},
		Providers:  openrouterProviders,
	}
	passed, _ = filter.Passes(ctxOpenRouterDeepSeek)
	if !passed {
		t.Errorf("expected pass for DeepSeek on openrouter tier, got passed=false")
	}

	// 6. Direct OpenAI tier must reject Claude
	ctxOpenAIClaude := &FilterContext{
		TargetTier: &TierReplayConfig{Provider: "openai", Model: "gpt-4o"},
		Candidate:  &CandidateModel{ModelID: "claude-sonnet-5-5"},
		Providers:  openaiProviders,
	}
	passed, reason = filter.Passes(ctxOpenAIClaude)
	if passed || !strings.HasPrefix(reason, "provider_model_incompatible") {
		t.Errorf("expected rejection for Claude on openai tier, got passed=%v, reason=%s", passed, reason)
	}

	// 7. Direct Google tier must reject Claude
	ctxGoogleClaude := &FilterContext{
		TargetTier: &TierReplayConfig{Provider: "google", Model: "gemini-2.5-flash"},
		Candidate:  &CandidateModel{ModelID: "claude-sonnet-5-5"},
		Providers:  googleProviders,
	}
	passed, reason = filter.Passes(ctxGoogleClaude)
	if passed || !strings.HasPrefix(reason, "provider_model_incompatible") {
		t.Errorf("expected rejection for Claude on google tier, got passed=%v, reason=%s", passed, reason)
	}

	// 8. Local tier must reject proprietary cloud models
	ctxLocalClaude := &FilterContext{
		TargetTier: &TierReplayConfig{Provider: "llama_cluster", Model: "qwen2.5-coder:7b", IsLocal: true},
		Candidate:  &CandidateModel{ModelID: "claude-sonnet-5-5"},
		Providers:  localProviders,
	}
	passed, reason = filter.Passes(ctxLocalClaude)
	if passed || !strings.HasPrefix(reason, "provider_model_incompatible") {
		t.Errorf("expected rejection for Claude on local tier, got passed=%v, reason=%s", passed, reason)
	}

	// 9. Test Model ID Prefix Normalization
	normAnthropic := NormalizeModelForProvider("anthropic/claude-3-5-sonnet", "anthropic")
	if normAnthropic != "claude-3-5-sonnet" {
		t.Errorf("expected 'claude-3-5-sonnet', got %q", normAnthropic)
	}
	normOpenRouter := NormalizeModelForProvider("anthropic/claude-3-5-sonnet", "openrouter")
	if normOpenRouter != "anthropic/claude-3-5-sonnet" {
		t.Errorf("expected 'anthropic/claude-3-5-sonnet', got %q", normOpenRouter)
	}
	normOpenAI := NormalizeModelForProvider("openai/gpt-4o", "openai")
	if normOpenAI != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got %q", normOpenAI)
	}
	normGoogle := NormalizeModelForProvider("google/gemini-2.5-flash", "google")
	if normGoogle != "gemini-2.5-flash" {
		t.Errorf("expected 'gemini-2.5-flash', got %q", normGoogle)
	}
	normDeepSeek := NormalizeModelForProvider("deepseek/deepseek-chat", "deepseek")
	if normDeepSeek != "deepseek-chat" {
		t.Errorf("expected 'deepseek-chat', got %q", normDeepSeek)
	}
}

func TestOptimize_AnthropicProfileDoesNotRecommendDeepSeek(t *testing.T) {
	policy := DefaultTuningPolicy()
	opt := NewMinConflictsOptimizer(policy)

	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"anthropic": {
				BaseURL: "https://api.anthropic.com",
				Type:    contract.ProviderTypeAnthropic,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:            "Tier 1: Haiku 4.5 (Workhorse)",
				Provider:        "anthropic",
				Model:           "claude-haiku-4-5",
				ReasoningEffort: "medium",
				When:            "Retries < 3 && !HasTestFail",
			},
			{
				Name:            "Tier 2: Claude Sonnet 5.5",
				Provider:        "anthropic",
				Model:           "claude-sonnet-5-5",
				ReasoningEffort: "medium",
				When:            "Retries < 6",
			},
		},
		DefaultTier: contract.Tier{
			Name:            "Default Tier",
			Provider:        "anthropic",
			Model:           "claude-sonnet-5-5",
			ReasoningEffort: "high",
		},
	}

	var records []telemetry.TurnRecord
	for i := 0; i < 20; i++ {
		records = append(records, telemetry.TurnRecord{
			SessionID:    "sess-1",
			TargetModel:  "claude-haiku-4-5",
			SelectedTier: "Tier 1: Haiku 4.5 (Workhorse)",
			Provider:     "anthropic",
			HasImages:    true,
			HasTools:     true,
			Tokens:       5000,
			IsRetry:      true,
			StatusCode:   500,
		})
	}

	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("unexpected error during optimize: %v", err)
	}

	for _, tr := range res.Tiers {
		if tr.TierName == "Tier 1: Haiku 4.5 (Workhorse)" {
			if tr.RecommendedModel != "" && tr.RecommendedModel != "claude-haiku-4-5" {
				if strings.Contains(tr.RecommendedModel, "deepseek") || strings.Contains(tr.RecommendedModel, "glm") {
					t.Errorf("FATAL: Anthropic tier recommended incompatible cross-vendor model %q", tr.RecommendedModel)
				}
			}
		}
	}
}


