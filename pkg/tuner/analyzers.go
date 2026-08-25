package tuner

import (
	"sort"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// ModalityRiskAnalyzer evaluates empirical failure rates for multimodal images and tool calls.
type ModalityRiskAnalyzer struct {
	policy TuningPolicy
}

// NewModalityRiskAnalyzer creates a new ModalityRiskAnalyzer with the given policy.
func NewModalityRiskAnalyzer(policy TuningPolicy) *ModalityRiskAnalyzer {
	return &ModalityRiskAnalyzer{policy: policy}
}

// Analyze returns whether images or tools should be restricted on the local tier based on empirical retry spikes.
func (a *ModalityRiskAnalyzer) Analyze(records []telemetry.TurnRecord, targetTier *contract.Tier) (restrictImages, restrictTools bool) {
	if len(records) == 0 {
		return false, false
	}

	totalLocal := 0
	totalLocalRetries := 0

	imageTurns := 0
	imageRetries := 0

	toolTurns := 0
	toolRetries := 0

	for _, r := range records {
		if !r.IsLocal {
			continue
		}
		totalLocal++
		if r.IsRetry {
			totalLocalRetries++
		}

		if r.HasImages {
			imageTurns++
			if r.IsRetry {
				imageRetries++
			}
		}

		if r.HasTools {
			toolTurns++
			if r.IsRetry {
				toolRetries++
			}
		}
	}

	if totalLocal == 0 {
		return false, false
	}

	baselineRate := float64(totalLocalRetries) / float64(totalLocal)

	// Evaluate Vision Modality Risk (Odds Ratio >= threshold OR dominant/catastrophic failure rate >= baseline)
	if imageTurns >= a.policy.MinOccurrences {
		imageRate := float64(imageRetries) / float64(imageTurns)
		if (baselineRate > 0 && (imageRate/baselineRate) >= a.policy.OddsRatioThreshold) ||
			(imageRate >= 0.5 && imageRate >= baselineRate) {
			restrictImages = true
		}
	}

	// Evaluate Tool Modality Risk (Odds Ratio >= threshold OR dominant/catastrophic failure rate >= baseline)
	if toolTurns >= a.policy.MinOccurrences {
		toolRate := float64(toolRetries) / float64(toolTurns)
		if (baselineRate > 0 && (toolRate/baselineRate) >= a.policy.OddsRatioThreshold) ||
			(toolRate >= 0.5 && toolRate >= baselineRate) {
			restrictTools = true
		}
	}

	return restrictImages, restrictTools
}

// KeywordRiskAnalyzer evaluates domain vocabulary causing local retry spikes.
type KeywordRiskAnalyzer struct {
	policy TuningPolicy
}

// NewKeywordRiskAnalyzer creates a new KeywordRiskAnalyzer with the given policy.
func NewKeywordRiskAnalyzer(policy TuningPolicy) *KeywordRiskAnalyzer {
	return &KeywordRiskAnalyzer{policy: policy}
}

// Analyze returns an alphabetically sorted list of high-friction keywords.
func (a *KeywordRiskAnalyzer) Analyze(records []telemetry.TurnRecord) []string {
	if len(records) == 0 {
		return nil
	}

	type kwStat struct {
		total   int
		retries int
	}
	stats := make(map[string]*kwStat)
	totalLocal := 0
	totalLocalRetries := 0

	for _, r := range records {
		if !r.IsLocal {
			continue
		}
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

	if totalLocal == 0 {
		return nil
	}

	baselineRate := float64(totalLocalRetries) / float64(totalLocal)
	var highFriction []string

	for kw, s := range stats {
		if s.total >= a.policy.MinOccurrences {
			kwRate := float64(s.retries) / float64(s.total)
			if (baselineRate > 0 && (kwRate/baselineRate) >= a.policy.OddsRatioThreshold) ||
				(kwRate >= 0.5 && kwRate >= baselineRate) {
				highFriction = append(highFriction, kw)
			}
		}
	}

	sort.Strings(highFriction)
	return highFriction
}

// ContextCliffAnalyzer executes a grid sweep across candidate token thresholds.
type ContextCliffAnalyzer struct {
	policy TuningPolicy
}

// NewContextCliffAnalyzer creates a new ContextCliffAnalyzer with the given policy.
func NewContextCliffAnalyzer(policy TuningPolicy) *ContextCliffAnalyzer {
	return &ContextCliffAnalyzer{policy: policy}
}

// Sweep finds the optimal token threshold T, projected cloud cost, and projected retries.
func (a *ContextCliffAnalyzer) Sweep(
	records []telemetry.TurnRecord,
	tier *contract.Tier,
	restrictImages, restrictTools bool,
	highFriction []string,
) (optimalThreshold int, projectedCost float64, projectedRetries int) {
	maxBound := 32000
	if tier != nil && tier.MaxContext > 0 {
		maxBound = tier.MaxContext
	}

	if maxBound < 1000 {
		maxBound = 1000
	}

	bestT := maxBound
	bestFitness := -1e9
	bestProjectedCost := 0.0
	bestProjectedRetries := 0

	for t := 1000; t <= maxBound; t += 500 {
		simulatedCostUSD := 0.0
		simulatedRetries := 0

		for _, r := range records {
			candidateLocal := r.Tokens < t &&
				(!restrictImages || !r.HasImages) &&
				(!restrictTools || !r.HasTools) &&
				!containsAny(r.Keywords, highFriction...)

			if candidateLocal {
				if r.IsRetry {
					simulatedRetries++
				}
			} else {
				simulatedCostUSD += (float64(r.Tokens) / 1_000_000.0) * a.policy.CostPerMillionCloud
			}
		}

		// Objective Function: Maximize (-CostUSD - (Retries * Penalty))
		fitness := -simulatedCostUSD - (float64(simulatedRetries) * a.policy.RetryPenaltyUSD)
		if fitness > bestFitness || (fitness == bestFitness && t > bestT) {
			bestFitness = fitness
			bestT = t
			bestProjectedCost = simulatedCostUSD
			bestProjectedRetries = simulatedRetries
		}
	}

	return bestT, bestProjectedCost, bestProjectedRetries
}
