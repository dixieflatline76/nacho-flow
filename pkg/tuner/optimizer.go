package tuner

import (
	"context"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// CostPenaltyOptimizer implements the autonomous multivariate cost-penalty tuning strategy.
type CostPenaltyOptimizer struct {
	Policy TuningPolicy
}

// NewCostPenaltyOptimizer creates a new CostPenaltyOptimizer with default policy.
func NewCostPenaltyOptimizer() *CostPenaltyOptimizer {
	return &CostPenaltyOptimizer{
		Policy: DefaultTuningPolicy(),
	}
}

// NewCostPenaltyOptimizerWithPolicy creates an optimizer with a custom TuningPolicy.
func NewCostPenaltyOptimizerWithPolicy(policy TuningPolicy) *CostPenaltyOptimizer {
	return &CostPenaltyOptimizer{
		Policy: policy,
	}
}

func (opt *CostPenaltyOptimizer) Name() string {
	return "cost_penalty"
}

// Optimize executes the modular analyzer pipeline and AST synthesis.
func (opt *CostPenaltyOptimizer) Optimize(records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error) {
	// Find target local tier from config using unified IsLocalTier detector
	var targetTier *contract.Tier
	var existingWhen string
	targetTierName := "Local GPU"

	if currentConfig != nil {
		for i, tier := range currentConfig.Tiers {
			if IsLocalTier(tier) {
				targetTier = &currentConfig.Tiers[i]
				existingWhen = tier.When
				targetTierName = tier.Name
				break
			}
		}
	}

	if len(records) == 0 {
		defaultThreshold := 16000
		if targetTier != nil && targetTier.MaxContext > 0 && targetTier.MaxContext < defaultThreshold {
			defaultThreshold = targetTier.MaxContext
		}

		rule, err := RewriteRuleAST(existingWhen, defaultThreshold, nil, false, false)
		if err != nil {
			return nil, err
		}
		return &TuningResult{
			OptimalThreshold: defaultThreshold,
			SynthesizedRule:  rule,
			TargetTierName:   targetTierName,
		}, nil
	}

	// 1. Modality Risk Analysis (Images & Tools)
	modalityAnalyzer := NewModalityRiskAnalyzer(opt.Policy)
	restrictImages, restrictTools := modalityAnalyzer.Analyze(records, targetTier)

	// 2. Keyword Friction Risk Analysis
	keywordAnalyzer := NewKeywordRiskAnalyzer(opt.Policy)
	highFrictionKws := keywordAnalyzer.Analyze(records)

	// 3. Continuous Multi-Objective Token Threshold Sweep
	contextAnalyzer := NewContextCliffAnalyzer(opt.Policy)
	bestT, bestProjectedCost, bestProjectedRetries := contextAnalyzer.Sweep(records, targetTier, restrictImages, restrictTools, highFrictionKws)

	// 4. AST Rule Synthesis (Preserving custom user guardrails)
	rule, err := RewriteRuleAST(existingWhen, bestT, highFrictionKws, restrictImages, restrictTools)
	if err != nil {
		return nil, err
	}

	// 5. Aggregate Baseline Metrics (Focusing on Local Retries to measure local friction avoided)
	currentCost := 0.0
	currentLocalRetries := 0
	for _, r := range records {
		currentCost += r.CostSavedUSD
		if r.IsLocal && r.IsRetry {
			currentLocalRetries++
		}
	}

	retriesAvoided := currentLocalRetries - bestProjectedRetries
	if retriesAvoided < 0 {
		retriesAvoided = 0
	}

	return &TuningResult{
		OptimalThreshold:    bestT,
		FrictionKeywords:    highFrictionKws,
		RestrictImages:      restrictImages,
		RestrictTools:       restrictTools,
		TargetTierName:      targetTierName,
		SynthesizedRule:     rule,
		CurrentCostUSD:      currentCost,
		ProjectedCostUSD:    bestProjectedCost,
		ProjectedSavingsUSD: currentCost - bestProjectedCost,
		RetriesEliminated:   retriesAvoided,
		TotalSampleTurns:    len(records),
	}, nil
}

func containsAny(slice []string, targets ...string) bool {
	for _, s := range slice {
		for _, t := range targets {
			if s == t {
				return true
			}
		}
	}
	return false
}

// OptimizeWithContext executes the optimization algorithm respecting context cancellation/timeout.
func (opt *CostPenaltyOptimizer) OptimizeWithContext(ctx context.Context, records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error) {
	type resultPair struct {
		res *TuningResult
		err error
	}
	done := make(chan resultPair, 1)

	go func() {
		res, err := opt.Optimize(records, currentConfig)
		done <- resultPair{res: res, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.res, r.err
	}
}
