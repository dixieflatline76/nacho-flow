package tuner

import (
	"strings"
)

// ReplayConfig defines a candidate routing configuration to simulate.
type ReplayConfig struct {
	TokenThreshold   int // Tokens < T
	RetryBound       int // Retries < R
	RestrictImages   bool
	RestrictTools    bool
	FrictionKeywords []string
}

// ReplayResult captures the simulated outcome of replaying a session with a candidate config.
type ReplayResult struct {
	LocalTurns       int
	CloudTurns       int
	SimulatedRetries int     // Local retries before escalation
	SimulatedCost    float64 // Cloud cost incurred
	EscalationTurn   int     // Turn index where local→cloud handoff occurred (-1 if never)
	TurnsToResolve   int     // Total turns including cloud resolution attempts
}

// ReplaySession simulates routing decisions for a single session trajectory
// under a candidate (T, R) configuration, faithfully reproducing the
// retry state machine and enforcing the Counterfactual Attribution rules.
func ReplaySession(trajectory SessionTrajectory, config ReplayConfig, policy TuningPolicy) ReplayResult {
	result := ReplayResult{
		EscalationTurn: -1,
		TurnsToResolve: len(trajectory.Turns),
	}

	if len(trajectory.Turns) == 0 {
		return result
	}

	var currentRetries int
	var currentRootHash uint64
	var escalated bool

	for i, turn := range trajectory.Turns {
		// Task boundary reset check: if RootPromptHash changed, reset state
		if i == 0 {
			currentRootHash = turn.RootPromptHash
		} else if turn.RootPromptHash != 0 && turn.RootPromptHash != currentRootHash {
			currentRootHash = turn.RootPromptHash
			currentRetries = 0
			escalated = false
		}

		targetLocal := false
		if !escalated {
			targetLocal = turn.Tokens < config.TokenThreshold

			if config.RetryBound > 0 && currentRetries >= config.RetryBound {
				targetLocal = false
			}
			if config.RestrictImages && turn.HasImages {
				targetLocal = false
			}
			if config.RestrictTools && turn.HasTools {
				targetLocal = false
			}
			if targetLocal && len(config.FrictionKeywords) > 0 {
				for _, kw := range config.FrictionKeywords {
					for _, turnKw := range turn.Keywords {
						if strings.EqualFold(kw, turnKw) {
							targetLocal = false
							break
						}
					}
					if !targetLocal {
						break
					}
				}
			}
		}

		if targetLocal {
			result.LocalTurns++

			// 🚨 Counterfactual Attribution Trap:
			// If historical turn ran on Cloud and succeeded, Local cannot be assumed to succeed.
			if !turn.IsLocal {
				currentRetries++
				result.SimulatedRetries++
			} else if turn.IsRetry {
				currentRetries++
				result.SimulatedRetries++
			} else {
				// Historical turn ran on Local: check progress
				hasProgress := turn.HasWriteProgress || turn.HasTestPass
				if turn.HasTools && !turn.HasWriteCapability {
					// Plan mode read-only turns are immune to retry penalties
					hasProgress = true
				}

				if hasProgress {
					currentRetries = 0
				}
			}

			// If local reached retry bound after this turn, mark escalated for subsequent turns
			if config.RetryBound > 0 && currentRetries >= config.RetryBound {
				escalated = true
			}
		} else {
			// Routed to Cloud
			result.CloudTurns++
			if result.EscalationTurn == -1 && result.LocalTurns > 0 {
				result.EscalationTurn = i
			}

			cost := turn.CostSpentUSD
			if cost == 0 || turn.IsLocal {
				// Estimate cloud cost from tokens using policy rate
				cost = (float64(turn.Tokens) / 1_000_000.0) * policy.CostPerMillionCloud
			}
			result.SimulatedCost += cost

			// Cloud turn provides resolution
			if !turn.IsRetry || !turn.IsLocal {
				currentRetries = 0
			}
		}
	}

	return result
}
