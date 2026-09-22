package tuner

import (
	"fmt"
	"math"
)

// ConflictReport captures the holistic conflict score and per-variable attribution ledger.
type ConflictReport struct {
	TotalConflict      float64            `json:"total_conflict"`
	TotalCostUSD       float64            `json:"total_cost_usd"`
	TotalWastedRetries int                `json:"total_wasted_retries"`
	TotalTurns         int                `json:"total_turns"`
	TotalCycleTrips    int                `json:"total_cycle_trips"`
	VariableConflicts  map[string]float64 `json:"variable_conflicts"`
	MaxConflictVar     string             `json:"max_conflict_var"`
}

// CheckHardConstraints verifies whether a candidate routing state satisfies all hard constraints.
// Returns true if valid, false if invalid.
func CheckHardConstraints(cfg *MultiTierConfig) bool {
	if cfg == nil {
		return false
	}

	for i := 0; i < len(cfg.Tiers); i++ {
		t := &cfg.Tiers[i]
		// 1. Hardware Context Limit / VRAM ceiling
		if t.MaxContext > 0 && t.TokenThreshold > 0 && t.TokenThreshold > t.MaxContext {
			return false
		}

		// 2. Monotonic Context Hierarchy: T_i <= T_{i+1}
		if i < len(cfg.Tiers)-1 {
			next := &cfg.Tiers[i+1]
			if t.TokenThreshold > 0 && next.TokenThreshold > 0 && t.TokenThreshold > next.TokenThreshold {
				return false
			}
		}
	}

	return true
}

// EvaluateFleetConflict evaluates the total scalar conflict C(X) across historical sessions.
// Zero-allocation invariant on the hot path: reuses the supplied MultiTierReplayResult buffer.
func EvaluateFleetConflict(
	trajectories []SessionTrajectory,
	cfg *MultiTierConfig,
	policy *TuningPolicy,
	buf *MultiTierReplayResult,
) float64 {
	if !CheckHardConstraints(cfg) {
		return math.Inf(1)
	}

	if buf == nil {
		var localBuf MultiTierReplayResult
		buf = &localBuf
	}

	ReplayMultiTierFleet(trajectories, cfg, policy, buf)

	costWeight := 1.0
	retryWeight := 2.0
	turnsWeight := 0.10
	cycleWeight := 5.0

	if policy != nil {
		costWeight = policy.CostWeight
		retryWeight = policy.RetryPenaltyUSD
		turnsWeight = policy.TurnsWeight
	}

	totalConflict := (costWeight * buf.TotalCostUSD) +
		(retryWeight * float64(buf.TotalWastedRetries)) +
		(turnsWeight * float64(buf.TotalTurns)) +
		(cycleWeight * float64(buf.TotalCycleTrips))

	return totalConflict
}

// EvaluateFleetWithAttribution calculates C(X) and divides conflict penalties proportionally
// across candidate repair variables using Fractional Attribution to avoid variable starvation.
func EvaluateFleetWithAttribution(
	trajectories []SessionTrajectory,
	cfg *MultiTierConfig,
	policy *TuningPolicy,
	monitoredKeywords []string,
) ConflictReport {
	if !CheckHardConstraints(cfg) {
		return ConflictReport{
			TotalConflict:     math.Inf(1),
			VariableConflicts: make(map[string]float64),
		}
	}

	costWeight := 1.0
	retryWeight := 2.0
	turnsWeight := 0.10
	cycleWeight := 5.0

	if policy != nil {
		costWeight = policy.CostWeight
		retryWeight = policy.RetryPenaltyUSD
		turnsWeight = policy.TurnsWeight
	}

	variableLedger := make(map[string]float64)
	var totalCostUSD float64
	var totalWastedRetries int
	var totalTurns int
	var totalCycleTrips int

	defaultTierIdx := len(cfg.Tiers)
	if defaultTierIdx >= MaxSupportedTiers {
		defaultTierIdx = MaxSupportedTiers - 1
	}

	// Buffer for collecting repair variables per turn without heap thrash
	repairVarsBuf := make([]string, 0, 16)

	for sIdx := range trajectories {
		traj := &trajectories[sIdx]
		var currentRetries int
		var currentRootHash uint64

		for i := 0; i < len(traj.Turns); i++ {
			turn := &traj.Turns[i]
			totalTurns++

			// Boundary reset
			if i == 0 {
				currentRootHash = turn.RootPromptHash
			} else if turn.RootPromptHash != 0 && turn.RootPromptHash != currentRootHash {
				currentRootHash = turn.RootPromptHash
				currentRetries = 0
			}

			retriesBeforeTurn := currentRetries

			// Cascade selection
			selectedTierIdx := defaultTierIdx
			var targetTier *TierReplayConfig

			for tIdx := 0; tIdx < len(cfg.Tiers) && tIdx < MaxSupportedTiers-1; tIdx++ {
				candidate := &cfg.Tiers[tIdx]
				if candidate.IsDisabled {
					continue
				}
				if candidate.MaxContext > 0 && turn.Tokens > candidate.MaxContext {
					continue
				}
				if candidate.TokenThreshold > 0 && turn.Tokens >= candidate.TokenThreshold {
					continue
				}
				if candidate.RetryBound > 0 && currentRetries >= candidate.RetryBound {
					continue
				}
				if candidate.RestrictImages && turn.HasImages {
					continue
				}
				if candidate.RestrictTools && turn.HasTools {
					continue
				}
				if len(candidate.ExcludedKeywords) > 0 && hasKeyword(turn.Keywords, candidate.ExcludedKeywords) {
					continue
				}

				selectedTierIdx = tIdx
				targetTier = candidate
				break
			}

			if targetTier == nil {
				selectedTierIdx = defaultTierIdx
				targetTier = &cfg.DefaultTier
			}

			// Evaluate turn outcome & determine if turn failed
			var turnFailed bool
			var turnCost float64

			if targetTier.IsLocal {
				if !turn.IsLocal {
					currentRetries++
					totalWastedRetries++
					turnFailed = true
				} else {
					hasProgress := turn.HasWriteProgress || turn.HasTestPass
					if turn.HasTools && !turn.HasWriteCapability {
						hasProgress = true
					}
					if hasProgress {
						currentRetries = 0
					} else if turn.IsRetry {
						currentRetries++
						totalWastedRetries++
						turnFailed = true
					}
				}
			} else {
				// Cloud tier
				turnCost = turn.CostSpentUSD
				if turnCost == 0 || turn.IsLocal || targetTier.CostPerMillion > 0 {
					outputTokens := turn.CycleContentTokens + turn.CycleThinkingTokens + turn.CycleToolTokens
					if outputTokens > 0 && targetTier.CompletionCostPerMillion > 0 {
						turnCost = (float64(turn.Tokens)/1_000_000.0)*targetTier.PromptCostPerMillion +
							(float64(outputTokens)/1_000_000.0)*targetTier.CompletionCostPerMillion
					} else {
						rate := targetTier.ComprehensiveRate
						if rate <= 0 {
							rate = targetTier.CostPerMillion
						}
						if rate <= 0 && policy != nil {
							rate = policy.CostPerMillionCloud
						}
						turnCost = (float64(turn.Tokens) / 1_000_000.0) * rate
					}
				}
				totalCostUSD += turnCost

				if !turn.IsRetry || turn.IsLocal || targetTier.CodingIndex >= 90.0 {
					currentRetries = 0
				} else {
					currentRetries++
					totalWastedRetries++
					turnFailed = true
				}
			}

			if turn.CycleBreakerTriggered {
				totalCycleTrips++
				turnFailed = true
			}

			// Fractional Attribution on Failure (only for non-default tiers and non-disabled tiers)
			if turnFailed && selectedTierIdx < len(cfg.Tiers) && !cfg.Tiers[selectedTierIdx].IsDisabled {
				repairVarsBuf = repairVarsBuf[:0]

				// Candidate 1: Tightening Token Cliff
				if turn.Tokens > 0 {
					repairVarsBuf = append(repairVarsBuf, fmt.Sprintf("tier_%d:tokens", selectedTierIdx))
				}

				// Candidate 2: Tightening Retry Bound (only if retriesBeforeTurn >= 1)
				if retriesBeforeTurn >= 1 {
					repairVarsBuf = append(repairVarsBuf, fmt.Sprintf("tier_%d:retries", selectedTierIdx))
				}

				// Candidate 3: Tool Gate
				if turn.HasTools && !targetTier.RestrictTools {
					repairVarsBuf = append(repairVarsBuf, fmt.Sprintf("tier_%d:tools", selectedTierIdx))
				}

				// Candidate 4: Vision Gate
				if turn.HasImages && !targetTier.RestrictImages {
					repairVarsBuf = append(repairVarsBuf, fmt.Sprintf("tier_%d:images", selectedTierIdx))
				}

				// Candidate 5: Monitored Keyword Exclusions
				for _, kw := range monitoredKeywords {
					if !hasKeyword([]string{kw}, targetTier.ExcludedKeywords) && hasKeyword(turn.Keywords, []string{kw}) {
						repairVarsBuf = append(repairVarsBuf, fmt.Sprintf("keyword:%s", kw))
					}
				}

				// Divide turn penalty equally among all repairable variables
				k := len(repairVarsBuf)
				if k > 0 {
					penalty := retryWeight
					if turn.CycleBreakerTriggered {
						penalty += cycleWeight
					}
					penalty += costWeight * turnCost

					fractionalPenalty := penalty / float64(k)
					for _, v := range repairVarsBuf {
						variableLedger[v] += fractionalPenalty
					}
				}
			}
		}
	}

	totalConflict := (costWeight * totalCostUSD) +
		(retryWeight * float64(totalWastedRetries)) +
		(turnsWeight * float64(totalTurns)) +
		(cycleWeight * float64(totalCycleTrips))

	// Find the variable contributing highest conflict penalty
	var maxVar string
	var maxVal float64
	for v, val := range variableLedger {
		if val > maxVal {
			maxVal = val
			maxVar = v
		}
	}

	return ConflictReport{
		TotalConflict:      totalConflict,
		TotalCostUSD:       totalCostUSD,
		TotalWastedRetries: totalWastedRetries,
		TotalTurns:         totalTurns,
		TotalCycleTrips:    totalCycleTrips,
		VariableConflicts:  variableLedger,
		MaxConflictVar:     maxVar,
	}
}
