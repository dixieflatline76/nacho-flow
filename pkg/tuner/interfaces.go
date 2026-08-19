package tuner

import (
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// TuningResult captures the mathematical output of a tuning strategy run.
type TuningResult struct {
	OptimalThreshold    int      `json:"optimal_threshold"`
	FrictionKeywords    []string `json:"friction_keywords"`
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
