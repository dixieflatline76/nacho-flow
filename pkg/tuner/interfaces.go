package tuner

import (
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// TuningPolicy defines the cost-utility parameters and statistical thresholds for optimization.
type TuningPolicy struct {
	Name                string  `json:"name"`
	CostPerMillionCloud float64 `json:"cost_per_million_cloud"`
	RetryPenaltyUSD     float64 `json:"retry_penalty_usd"`
	MinOccurrences      int     `json:"min_occurrences"`
	OddsRatioThreshold  float64 `json:"odds_ratio_threshold"`
}

// DefaultTuningPolicy returns the recommended balanced flow-state protection policy.
func DefaultTuningPolicy() TuningPolicy {
	return TuningPolicy{
		Name:                "balanced_flow_state",
		CostPerMillionCloud: 2.50,
		RetryPenaltyUSD:     2.00,
		MinOccurrences:      10,
		OddsRatioThreshold:  1.5,
	}
}

// TuningResult captures the mathematical and empirical output of a tuning strategy run.
type TuningResult struct {
	OptimalThreshold    int      `json:"optimal_threshold"`
	FrictionKeywords    []string `json:"friction_keywords"`
	RestrictImages      bool     `json:"restrict_images"`
	RestrictTools       bool     `json:"restrict_tools"`
	PreservedClauses    []string `json:"preserved_clauses"`
	TargetTierName      string   `json:"target_tier_name"`
	SynthesizedRule     string   `json:"synthesized_rule"`
	CurrentCostUSD      float64  `json:"current_cost_usd"`
	ProjectedCostUSD    float64  `json:"projected_cost_usd"`
	ProjectedSavingsUSD float64  `json:"projected_savings_usd"`
	RetriesEliminated   int      `json:"retries_eliminated"`
	TotalSampleTurns    int      `json:"total_sample_turns"`
}

// OptimizationStrategy defines the contract for autonomous route tuning algorithms.
type OptimizationStrategy interface {
	Name() string
	Optimize(records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error)
}

// IsLocalTier returns true if the tier represents a local or on-prem inference engine.
func IsLocalTier(tier contract.Tier, providers map[string]contract.ProviderConfig) bool {
	if p, ok := providers[tier.Provider]; ok {
		return p.IsLocal()
	}
	return false
}
