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
			if IsLocalTier(tier, currentConfig.Providers) {
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

		rule, err := RewriteRuleAST(existingWhen, defaultThreshold, 0, nil, false, false)
		if err != nil {
			return nil, err
		}
		return &TuningResult{
			Tiers: []TierTuningResult{
				{
					TierName:         targetTierName,
					OptimalThreshold: defaultThreshold,
					SynthesizedRule:  rule,
					OriginalRule:     existingWhen,
				},
			},
		}, nil
	}

	trajectories := GroupBySession(records)
	if len(trajectories) > 0 {
		// 1. Modality Risk Analysis (Images & Tools)
		modalityAnalyzer := NewModalityRiskAnalyzer(opt.Policy)
		restrictImages, restrictTools := modalityAnalyzer.Analyze(records, targetTier)

		// 2. Keyword Friction Risk Analysis
		keywordAnalyzer := NewKeywordRiskAnalyzer(opt.Policy)
		highFrictionKws := keywordAnalyzer.Analyze(records)

		// 3. 2D Grid Sweep Optimizer across session trajectories
		gridRes := GridSweep(trajectories, targetTier, restrictImages, restrictTools, highFrictionKws, opt.Policy)

		// 4. Model Self-Recovery Dynamics Analysis
		recoveryStats := AnalyzeRecovery(trajectories)

		// 5. AST Rule Synthesis
		rule, err := RewriteRuleAST(existingWhen, gridRes.OptimalTokens, gridRes.OptimalRetries, highFrictionKws, restrictImages, restrictTools)
		if err != nil {
			return nil, err
		}

		// 6. Aggregate Baseline Metrics
		var currentCost float64
		var currentLocalRetries int
		for _, traj := range trajectories {
			currentCost += traj.TotalCost
			for _, turn := range traj.Turns {
				if turn.IsLocal && turn.IsRetry {
					currentLocalRetries++
				}
			}
		}

		totalProjectedRetries := int(gridRes.ProjectedRetries * float64(len(trajectories)))
		retriesAvoided := currentLocalRetries - totalProjectedRetries
		if retriesAvoided < 0 {
			retriesAvoided = 0
		}

		projectedSavings := currentCost - gridRes.ProjectedCost
		if projectedSavings < 0 {
			projectedSavings = 0
		}

		avgTurns := float64(len(records)) / float64(len(trajectories))

		return &TuningResult{
			Tiers: []TierTuningResult{
				{
					TierName:         targetTierName,
					OptimalThreshold: gridRes.OptimalTokens,
					OptimalRetries:   gridRes.OptimalRetries,
					FrictionKeywords: highFrictionKws,
					RestrictImages:   restrictImages,
					RestrictTools:    restrictTools,
					OriginalRule:     existingWhen,
					SynthesizedRule:  rule,
				},
			},
			CurrentCostUSD:      currentCost,
			ProjectedCostUSD:    gridRes.ProjectedCost,
			ProjectedSavingsUSD: projectedSavings,
			RetriesEliminated:   retriesAvoided,
			TotalSampleTurns:    len(records),
			TotalSessions:       len(trajectories),
			AvgTurnsPerSession:  avgTurns,
			EscalationRate:      gridRes.EscalationRate,
			RecoveryStats:       recoveryStats,
		}, nil
	}

	// Legacy single-turn fallback pipeline (when SessionIDs are absent)
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
	rule, err := RewriteRuleAST(existingWhen, bestT, 0, highFrictionKws, restrictImages, restrictTools)
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
		Tiers: []TierTuningResult{
			{
				TierName:         targetTierName,
				OptimalThreshold: bestT,
				FrictionKeywords: highFrictionKws,
				RestrictImages:   restrictImages,
				RestrictTools:    restrictTools,
				OriginalRule:     existingWhen,
				SynthesizedRule:  rule,
			},
		},
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
