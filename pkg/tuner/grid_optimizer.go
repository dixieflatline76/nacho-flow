package tuner

import (
	"math"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// GridSearchResult captures the optimal (Tokens, Retries) pair and its projected metrics.
type GridSearchResult struct {
	OptimalTokens    int     `json:"optimal_tokens"`
	OptimalRetries   int     `json:"optimal_retries"`
	ProjectedCost    float64 `json:"projected_cost"`
	ProjectedTurns   float64 `json:"projected_turns"`   // Average turns per session
	ProjectedRetries float64 `json:"projected_retries"` // Average wasted local retries per session
	EscalationRate   float64 `json:"escalation_rate"`   // Fraction of sessions that escalated to cloud
	RecoveryRate     float64 `json:"recovery_rate"`     // Fraction of escalations where cloud resolved
	Fitness          float64 `json:"fitness"`
}

// GridSweep runs a 2D (Tokens, Retries) grid search simulating routing across all session trajectories.
func GridSweep(
	trajectories []SessionTrajectory,
	tier *contract.Tier,
	restrictImages, restrictTools bool,
	frictionKws []string,
	policy TuningPolicy,
) GridSearchResult {
	if len(trajectories) == 0 {
		return GridSearchResult{}
	}

	maxBound := 32000
	if tier != nil && tier.MaxContext > 0 {
		maxBound = tier.MaxContext
	}
	minBound := 1000
	stepTokens := 500

	costWeight := policy.CostWeight
	if costWeight == 0 {
		costWeight = 1.0
	}
	turnsWeight := policy.TurnsWeight
	if turnsWeight == 0 {
		turnsWeight = 0.10
	}
	retryPenalty := policy.RetryPenaltyUSD
	if retryPenalty == 0 {
		retryPenalty = 2.0
	}

	bestFitness := -math.MaxFloat64
	bestResult := GridSearchResult{}
	found := false

	numSessions := float64(len(trajectories))

	for t := minBound; t <= maxBound; t += stepTokens {
		for r := 1; r <= 8; r++ {
			config := ReplayConfig{
				TokenThreshold:   t,
				RetryBound:       r,
				RestrictImages:   restrictImages,
				RestrictTools:    restrictTools,
				FrictionKeywords: frictionKws,
			}

			var totalCost float64
			var totalRetries int
			var totalTurns int
			var escalations int

			for _, traj := range trajectories {
				res := ReplaySession(traj, config, policy)
				totalCost += res.SimulatedCost
				totalRetries += res.SimulatedRetries
				totalTurns += res.TurnsToResolve
				if res.EscalationTurn >= 0 {
					escalations++
				}
			}

			avgTurns := float64(totalTurns) / numSessions
			avgRetries := float64(totalRetries) / numSessions
			escalationRate := float64(escalations) / numSessions

			// Multi-objective fitness function
			fitness := -totalCost*costWeight - float64(totalRetries)*retryPenalty - avgTurns*turnsWeight

			if !found || fitness > bestFitness {
				found = true
				bestFitness = fitness
				bestResult = GridSearchResult{
					OptimalTokens:    t,
					OptimalRetries:   r,
					ProjectedCost:    totalCost,
					ProjectedTurns:   avgTurns,
					ProjectedRetries: avgRetries,
					EscalationRate:   escalationRate,
					Fitness:          fitness,
				}
			}
		}
	}

	return bestResult
}
