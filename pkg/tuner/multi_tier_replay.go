package tuner


// TierReplayConfig defines the routing guardrails and economic properties for an individual tier.
type TierReplayConfig struct {
	TierName         string   `json:"tier_name"`
	Provider         string   `json:"provider"`
	IsLocal          bool     `json:"is_local"`
	CostPerMillion   float64  `json:"cost_per_million"`
	TokenThreshold   int      `json:"token_threshold"` // Tokens < TokenThreshold (0 = unlimited)
	MaxContext       int      `json:"max_context"`     // Hardware context ceiling (0 = unlimited)
	RetryBound       int      `json:"retry_bound"`     // Retries < RetryBound (0 = unlimited)
	RestrictImages   bool     `json:"restrict_images"` // If true, prompts with images are blocked
	RestrictTools    bool     `json:"restrict_tools"`  // If true, prompts with tools are blocked
	ExcludedKeywords []string `json:"excluded_keywords"`
}

// MaxSupportedTiers is the maximum number of cascade tiers handled in fixed-size buffers.
const MaxSupportedTiers = 8

// MultiTierConfig defines the cascade of candidate tiers ending in the default fallback tier.
type MultiTierConfig struct {
	Tiers       []TierReplayConfig `json:"tiers"`        // Tiers 1 .. M-1 in priority order
	DefaultTier TierReplayConfig   `json:"default_tier"` // Tier M (unconditional sink/fallback)
}

// TierReplayStats accumulates turn statistics for a single tier during replay.
type TierReplayStats struct {
	TierName      string  `json:"tier_name"`
	TurnsRouted   int     `json:"turns_routed"`
	WastedRetries int     `json:"wasted_retries"`
	CostUSD       float64 `json:"cost_usd"`
	CycleTrips    int     `json:"cycle_trips"`
}

// MultiTierReplayResult captures the simulated fleet/session replay outcome.
type MultiTierReplayResult struct {
	TotalTurns         int                             `json:"total_turns"`
	TotalCostUSD       float64                         `json:"total_cost_usd"`
	TotalWastedRetries int                             `json:"total_wasted_retries"`
	TotalCycleTrips    int                             `json:"total_cycle_trips"`
	EscalatedSessions  int                             `json:"escalated_sessions"`
	ResolvedSessions   int                             `json:"resolved_sessions"`
	TierStats          [MaxSupportedTiers]TierReplayStats `json:"tier_stats"`
	NumTiers           int                             `json:"num_tiers"` // Count of active tiers (Tiers + DefaultTier)
}

// Reset clears the result accumulators for reuse without re-allocating.
func (r *MultiTierReplayResult) Reset() {
	r.TotalTurns = 0
	r.TotalCostUSD = 0
	r.TotalWastedRetries = 0
	r.TotalCycleTrips = 0
	r.EscalatedSessions = 0
	r.ResolvedSessions = 0
	for i := 0; i < len(r.TierStats); i++ {
		r.TierStats[i] = TierReplayStats{}
	}
	r.NumTiers = 0
}

// ReplayMultiTierSession simulates routing decisions for a single session trajectory
// across a multi-tier cascade, guaranteeing zero heap allocations on the hot path.
func ReplayMultiTierSession(
	trajectory *SessionTrajectory,
	cfg *MultiTierConfig,
	policy *TuningPolicy,
	result *MultiTierReplayResult,
) {
	if trajectory == nil || cfg == nil || result == nil {
		return
	}

	numTiers := len(cfg.Tiers) + 1
	if numTiers > MaxSupportedTiers {
		numTiers = MaxSupportedTiers
	}
	result.NumTiers = numTiers

	for i := 0; i < len(cfg.Tiers) && i < MaxSupportedTiers-1; i++ {
		result.TierStats[i].TierName = cfg.Tiers[i].TierName
	}
	if numTiers > 0 {
		result.TierStats[numTiers-1].TierName = cfg.DefaultTier.TierName
	}

	replayMultiTierSessionInternal(trajectory, cfg, policy, result)
}

// ReplayMultiTierFleet simulates routing across an entire fleet of session trajectories.
func ReplayMultiTierFleet(
	trajectories []SessionTrajectory,
	cfg *MultiTierConfig,
	policy *TuningPolicy,
	result *MultiTierReplayResult,
) {
	if result == nil || cfg == nil {
		return
	}
	result.Reset()

	numTiers := len(cfg.Tiers) + 1
	if numTiers > MaxSupportedTiers {
		numTiers = MaxSupportedTiers
	}
	result.NumTiers = numTiers

	for i := 0; i < len(cfg.Tiers) && i < MaxSupportedTiers-1; i++ {
		result.TierStats[i].TierName = cfg.Tiers[i].TierName
	}
	if numTiers > 0 {
		result.TierStats[numTiers-1].TierName = cfg.DefaultTier.TierName
	}

	for i := 0; i < len(trajectories); i++ {
		replayMultiTierSessionInternal(&trajectories[i], cfg, policy, result)
	}
}

// replayMultiTierSessionInternal executes the session turn loop.
// Zero-allocation invariant: no make, new, interface, or slice header escape.
func replayMultiTierSessionInternal(
	trajectory *SessionTrajectory,
	cfg *MultiTierConfig,
	policy *TuningPolicy,
	result *MultiTierReplayResult,
) {
	numTurns := len(trajectory.Turns)
	if numTurns == 0 {
		return
	}

	var currentRetries int
	var currentRootHash uint64
	var sessionEscalated bool

	defaultTierIdx := len(cfg.Tiers)
	if defaultTierIdx >= MaxSupportedTiers {
		defaultTierIdx = MaxSupportedTiers - 1
	}

	for i := 0; i < numTurns; i++ {
		turn := &trajectory.Turns[i]

		// 1. Task boundary reset check
		if i == 0 {
			currentRootHash = turn.RootPromptHash
		} else if turn.RootPromptHash != 0 && turn.RootPromptHash != currentRootHash {
			currentRootHash = turn.RootPromptHash
			currentRetries = 0
		}

		// 2. Cascade selection: evaluate tiers 0 .. M-1 sequentially
		selectedTierIdx := defaultTierIdx
		var targetTier *TierReplayConfig

		for tIdx := 0; tIdx < len(cfg.Tiers) && tIdx < MaxSupportedTiers-1; tIdx++ {
			candidate := &cfg.Tiers[tIdx]

			// Context ceiling guard
			if candidate.MaxContext > 0 && turn.Tokens > candidate.MaxContext {
				continue
			}
			// Token threshold guard
			if candidate.TokenThreshold > 0 && turn.Tokens >= candidate.TokenThreshold {
				continue
			}
			// Retry bound guard
			if candidate.RetryBound > 0 && currentRetries >= candidate.RetryBound {
				continue
			}
			// Vision gate
			if candidate.RestrictImages && turn.HasImages {
				continue
			}
			// Tool gate
			if candidate.RestrictTools && turn.HasTools {
				continue
			}
			// Domain keyword gate
			if len(candidate.ExcludedKeywords) > 0 && hasKeyword(turn.Keywords, candidate.ExcludedKeywords) {
				continue
			}

			selectedTierIdx = tIdx
			targetTier = candidate
			break
		}

		// If no cascade tier matched, fall back to DefaultTier (Tier M)
		if targetTier == nil {
			selectedTierIdx = defaultTierIdx
			targetTier = &cfg.DefaultTier
		}

		// Escalation tracking: did turn route beyond the initial tier?
		if selectedTierIdx > 0 {
			sessionEscalated = true
		}

		// Record turn
		result.TotalTurns++
		result.TierStats[selectedTierIdx].TurnsRouted++

		// 3. Turn execution & Counterfactual Attribution
		if targetTier.IsLocal {
			// 🚨 Counterfactual Attribution Trap:
			// If historical turn ran on Cloud and succeeded, Local cannot be assumed to succeed.
			if !turn.IsLocal {
				currentRetries++
				result.TotalWastedRetries++
				result.TierStats[selectedTierIdx].WastedRetries++
			} else {
				hasProgress := turn.HasWriteProgress || turn.HasTestPass
				if turn.HasTools && !turn.HasWriteCapability {
					// Plan mode read-only turns are immune to retry penalties
					hasProgress = true
				}

				if hasProgress {
					currentRetries = 0
				} else if turn.IsRetry {
					currentRetries++
					result.TotalWastedRetries++
					result.TierStats[selectedTierIdx].WastedRetries++
				}
			}
		} else {
			// Cloud Tier (Workhorse or Frontier Fallback)
			cost := turn.CostSpentUSD
			if cost == 0 || (turn.IsLocal && targetTier.CostPerMillion > 0) {
				rate := targetTier.CostPerMillion
				if rate == 0 && policy != nil {
					rate = policy.CostPerMillionCloud
				}
				cost = (float64(turn.Tokens) / 1_000_000.0) * rate
			}
			result.TotalCostUSD += cost
			result.TierStats[selectedTierIdx].CostUSD += cost

			// Cloud resolution behavior
			if !turn.IsRetry || turn.IsLocal {
				currentRetries = 0
			} else {
				currentRetries++
				result.TotalWastedRetries++
				result.TierStats[selectedTierIdx].WastedRetries++
			}
		}

		// Cycle breaker tracking
		if turn.CycleBreakerTriggered {
			result.TotalCycleTrips++
			result.TierStats[selectedTierIdx].CycleTrips++
		}
	}

	if sessionEscalated {
		result.EscalatedSessions++
	}
	if trajectory.Resolved {
		result.ResolvedSessions++
	}
}

// hasKeyword checks if any turn keyword matches an excluded keyword with zero heap allocation.
func hasKeyword(turnKeywords, excludedKeywords []string) bool {
	for i := 0; i < len(excludedKeywords); i++ {
		ek := excludedKeywords[i]
		for j := 0; j < len(turnKeywords); j++ {
			if equalFoldASCII(ek, turnKeywords[j]) {
				return true
			}
		}
	}
	return false
}

// equalFoldASCII performs zero-allocation, case-insensitive ASCII string comparison.
func equalFoldASCII(s1, s2 string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := 0; i < len(s1); i++ {
		c1 := s1[i]
		c2 := s2[i]
		if c1 >= 'A' && c1 <= 'Z' {
			c1 += 'a' - 'A'
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 += 'a' - 'A'
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}
