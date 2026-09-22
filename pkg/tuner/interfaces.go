package tuner

import (
	"context"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// TuningPolicy defines the cost-utility parameters and statistical thresholds for optimization.
type TuningPolicy struct {
	Name                string  `json:"name"`
	CostPerMillionCloud float64 `json:"cost_per_million_cloud"`
	RetryPenaltyUSD     float64 `json:"retry_penalty_usd"`
	CostWeight          float64 `json:"cost_weight,omitempty"` // Weight for cloud cost (defaults to 1.0)
	TurnsWeight         float64 `json:"turns_weight"`          // Penalty per average session turn
	MinOccurrences      int     `json:"min_occurrences"`
	OddsRatioThreshold  float64 `json:"odds_ratio_threshold"`
	MinSessions         int     `json:"min_sessions"` // Minimum sessions for statistical significance
}

// DefaultTuningPolicy returns the recommended balanced flow-state protection policy.
func DefaultTuningPolicy() TuningPolicy {
	return TuningPolicy{
		Name:                "balanced_flow_state",
		CostPerMillionCloud: 2.50,
		RetryPenaltyUSD:     2.00,
		CostWeight:          1.0,
		TurnsWeight:         0.10,
		MinOccurrences:      10,
		OddsRatioThreshold:  1.5,
		MinSessions:         5,
	}
}

// TierTuningResult captures the tuned routing policy for an individual tier.
type TierTuningResult struct {
	TierName                 string   `json:"tier_name"`
	Model                    string   `json:"model,omitempty"`
	CodingIndex              float64  `json:"coding_index,omitempty"`
	ToolReliability          float64  `json:"tool_reliability,omitempty"`
	PromptCostPerMillion     float64  `json:"prompt_cost_per_million,omitempty"`
	CompletionCostPerMillion float64  `json:"completion_cost_per_million,omitempty"`
	ComprehensiveRate        float64  `json:"comprehensive_rate,omitempty"`
	IsDisabled               bool     `json:"is_disabled,omitempty"`
	OptimalThreshold         int      `json:"optimal_threshold"`
	OptimalRetries           int      `json:"optimal_retries"`
	FrictionKeywords         []string `json:"friction_keywords"`
	RestrictImages           bool     `json:"restrict_images"`
	RestrictTools            bool     `json:"restrict_tools"`
	PreservedClauses         []string `json:"preserved_clauses"`
	OriginalRule             string   `json:"original_rule,omitempty"`
	SynthesizedRule          string   `json:"synthesized_rule"`
}

// TuningResult captures the holistic fleet impact and per-tier policies.
type TuningResult struct {
	Tiers               []TierTuningResult       `json:"tiers"`
	CurrentCostUSD      float64                  `json:"current_cost_usd"`
	ProjectedCostUSD    float64                  `json:"projected_cost_usd"`
	ProjectedSavingsUSD float64                  `json:"projected_savings_usd"`
	RetriesEliminated   int                      `json:"retries_eliminated"`
	TotalSampleTurns    int                      `json:"total_sample_turns"`
	TotalSessions       int                      `json:"total_sessions"`
	AvgTurnsPerSession  float64                  `json:"avg_turns_per_session"`
	EscalationRate      float64                  `json:"escalation_rate"`
	RecoveryStats       map[string]RecoveryStats `json:"recovery_stats,omitempty"`
}

// OptimizationStrategy defines the contract for autonomous route tuning algorithms.
type OptimizationStrategy interface {
	Name() string
	Optimize(records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error)
	OptimizeWithContext(ctx context.Context, records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error)
}

// IsLocalTier returns true if the tier represents a local or on-prem inference engine.
func IsLocalTier(tier contract.Tier, providers map[string]contract.ProviderConfig) bool {
	if p, ok := providers[tier.Provider]; ok {
		return p.IsLocal()
	}
	return false
}
