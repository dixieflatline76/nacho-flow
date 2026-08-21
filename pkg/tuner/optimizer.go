package tuner

import (
	"sort"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// CostPenaltyOptimizer implements the cost-penalty heuristic tuning strategy.
type CostPenaltyOptimizer struct {
	MinOccurrences      int
	OddsRatioThreshold  float64
	CostPerMillionCloud float64
	RetryPenaltyUSD     float64
}

// NewCostPenaltyOptimizer creates a new initialized CostPenaltyOptimizer.
func NewCostPenaltyOptimizer() *CostPenaltyOptimizer {
	return &CostPenaltyOptimizer{
		MinOccurrences:      10,
		OddsRatioThreshold:  1.5,
		CostPerMillionCloud: 2.50,
		RetryPenaltyUSD:     2.00,
	}
}

func (opt *CostPenaltyOptimizer) Name() string {
	return "cost_penalty"
}

// Optimize executes the 3-step optimization: Keyword Friction Risk, Threshold Sweep, and Rule Distillation.
func (opt *CostPenaltyOptimizer) Optimize(records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error) {
	if len(records) == 0 {
		return &TuningResult{
			OptimalThreshold: 16000,
			SynthesizedRule:  "Tokens < 16000 && !HasImages && !HasTools",
		}, nil
	}

	// 1. Calculate Keyword Friction (Odds Ratio / Relative Risk)
	type kwStat struct {
		total   int
		retries int
	}
	stats := make(map[string]*kwStat)
	totalLocal := 0
	totalLocalRetries := 0

	currentCost := 0.0
	currentRetries := 0

	for _, r := range records {
		currentCost += r.CostSavedUSD // In telemetry, CostSavedUSD tracks cost metrics
		if r.IsRetry {
			currentRetries++
		}

		if r.IsLocal {
			totalLocal++
			if r.IsRetry {
				totalLocalRetries++
			}
			for _, kw := range r.Keywords {
				if stats[kw] == nil {
					stats[kw] = &kwStat{}
				}
				stats[kw].total++
				if r.IsRetry {
					stats[kw].retries++
				}
			}
		}
	}

	var highFriction []string
	if totalLocal > 0 {
		baselineRate := float64(totalLocalRetries) / float64(totalLocal)
		for kw, s := range stats {
			if s.total >= opt.MinOccurrences {
				kwRate := float64(s.retries) / float64(s.total)
				if baselineRate > 0 && (kwRate/baselineRate) >= opt.OddsRatioThreshold {
					highFriction = append(highFriction, kw)
				}
			}
		}
	}
	sort.Strings(highFriction)

	// 2. Multi-Objective Continuous Sweep on Candidate Thresholds T in [1000, 32000] in steps of 500
	bestT := 16000
	bestFitness := -1e9
	bestProjectedCost := 0.0
	bestProjectedRetries := 0

	for t := 1000; t <= 32000; t += 500 {
		simulatedCostUSD := 0.0
		simulatedRetries := 0

		for _, r := range records {
			candidateLocal := r.Tokens < t && !r.HasImages && !r.HasTools && !containsAny(r.Keywords, highFriction...)
			if candidateLocal {
				if r.IsRetry {
					simulatedRetries++
				}
			} else {
				simulatedCostUSD += (float64(r.Tokens) / 1_000_000.0) * opt.CostPerMillionCloud
			}
		}

		// Objective Function: Maximize (-CostUSD - (Retries * Penalty))
		fitness := -simulatedCostUSD - (float64(simulatedRetries) * opt.RetryPenaltyUSD)
		if fitness > bestFitness {
			bestFitness = fitness
			bestT = t
			bestProjectedCost = simulatedCostUSD
			bestProjectedRetries = simulatedRetries
		}
	}

	// 3. Distill to clean expr syntax
	rule, err := DistillRule(bestT, highFriction)
	if err != nil {
		return nil, err
	}

	retriesAvoided := currentRetries - bestProjectedRetries
	if retriesAvoided < 0 {
		retriesAvoided = 0
	}

	return &TuningResult{
		OptimalThreshold:    bestT,
		FrictionKeywords:    highFriction,
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
