package tuner

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// GenerateSyntheticTrajectories creates deterministic, realistic agentic session trajectories
// partitioned from a single contiguous TurnRecord buffer for maximal cache efficiency.
func GenerateSyntheticTrajectories(totalTurns int, seed uint64) []SessionTrajectory {
	if totalTurns <= 0 {
		return nil
	}

	// #nosec G404 - deterministic pseudo-random generator for synthetic simulation
	rng := rand.New(rand.NewPCG(seed, seed^0x5DEECE66D))

	// Preallocate contiguous turn buffer
	allTurns := make([]telemetry.TurnRecord, totalTurns)
	now := time.Now().UTC()

	var trajectories []SessionTrajectory
	keywordsPool := [][]string{
		{"refactor", "tests"},
		{"ast", "diff"},
		{"deadlock", "mutex"},
		{"sql", "query"},
		{"frontend", "css"},
		{"router", "http"},
		nil,
	}

	turnIdx := 0
	sessionIDCounter := 1

	for turnIdx < totalTurns {
		// Session length: 4 to 14 turns
		remaining := totalTurns - turnIdx
		sessionLen := 4 + rng.IntN(11)
		if sessionLen > remaining {
			sessionLen = remaining
		}

		sessID := fmt.Sprintf("synth-sess-%07d", sessionIDCounter)
		sessionIDCounter++
		rootHash := uint64(sessionIDCounter*1000 + 7)

		startIdx := turnIdx
		hasKickstart := rng.IntN(100) < 3
		hasImages := rng.IntN(100) < 8

		var totalCost float64
		var totalRetries int

		for i := 0; i < sessionLen; i++ {
			tRecord := &allTurns[startIdx+i]
			tRecord.SessionID = sessID
			tRecord.Timestamp = now.Add(time.Duration(turnIdx) * time.Second)
			tRecord.RootPromptHash = rootHash

			// Context size distribution
			pct := rng.IntN(100)
			switch {
			case pct < 60:
				tRecord.Tokens = 800 + rng.IntN(2700) // 800 - 3500 (Local candidate)
			case pct < 85:
				tRecord.Tokens = 4000 + rng.IntN(32000) // 4k - 36k
			case pct < 95:
				tRecord.Tokens = 40000 + rng.IntN(110000) // 40k - 150k
			default:
				tRecord.Tokens = 160000 + rng.IntN(400000) // 160k - 560k (Large context)
			}

			// Tool & Modality flags
			tRecord.HasTools = rng.IntN(100) < 75
			if hasImages && i == 0 {
				tRecord.HasImages = true
			}
			if hasKickstart && i == 0 {
				tRecord.SessionKickstarted = true
			}

			// Retry dynamics
			if i > 0 && rng.IntN(100) < 30 {
				tRecord.IsRetry = true
				totalRetries++
			} else {
				tRecord.IsRetry = false
			}

			// Progress & Resolution
			if !tRecord.IsRetry && rng.IntN(100) < 40 {
				tRecord.HasWriteProgress = true
			}
			if rng.IntN(100) < 2 {
				tRecord.CycleBreakerTriggered = true
			}

			// Keywords
			kwIdx := rng.IntN(len(keywordsPool))
			tRecord.Keywords = keywordsPool[kwIdx]

			// Cost
			if tRecord.Tokens > 4000 {
				tRecord.CostSpentUSD = (float64(tRecord.Tokens) / 1_000_000.0) * 0.75
				totalCost += tRecord.CostSpentUSD
			}
		}

		sessionTurns := allTurns[startIdx : startIdx+sessionLen]
		resolved := sessionTurns[sessionLen-1].HasWriteProgress || !sessionTurns[sessionLen-1].IsRetry

		trajectories = append(trajectories, SessionTrajectory{
			SessionID:    sessID,
			Turns:        sessionTurns,
			TotalTurns:   sessionLen,
			TotalCost:    totalCost,
			TotalRetries: totalRetries,
			Resolved:     resolved,
		})

		turnIdx += sessionLen
	}

	return trajectories
}

// DefaultBenchConfig returns a realistic multi-tier cascade configuration for benchmarking.
func DefaultBenchConfig() MultiTierConfig {
	return MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:          "Kickstart Escalation",
				Provider:          "openrouter",
				Model:             "google/gemini-3.8-flash",
				CodingIndex:       76.3,
				RequiresKickstart: true,
				RetryBound:        3,
				SupportsVision:    true,
				SupportsTools:     true,
			},
			{
				TierName:       "Multimodal Vision",
				Provider:       "openrouter",
				Model:          "google/gemini-3.8-flash",
				CodingIndex:    76.3,
				RequiresImages: true,
				RetryBound:     2,
				SupportsVision: true,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 1: Local GPU Workhorse",
				Provider:       "ollama",
				Model:          "gemma4:12b-it-qat",
				IsLocal:        true,
				CodingIndex:    64.0,
				TokenThreshold: 4000,
				RetryBound:     2,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 2: Flagship Agent Coder",
				Provider:       "openrouter",
				Model:          "qwen/qwen3-coder-plus",
				CodingIndex:    74.0,
				CostPerMillion: 0.80,
				TokenThreshold: 160000,
				RetryBound:     2,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 3: Debug & Reasoning Workhorse",
				Provider:       "openrouter",
				Model:          "google/gemini-3.8-flash",
				CodingIndex:    76.3,
				CostPerMillion: 1.6875,
				TokenThreshold: 1000000,
				RetryBound:     5,
				SupportsVision: true,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 4: Frontier Powerhouse",
				Provider:       "openrouter",
				Model:          "anthropic/claude-sonnet-5",
				CodingIndex:    82.4,
				CostPerMillion: 3.75,
				IsFrontier:     true,
				RetryFloor:     5,
				RetryBound:     8,
				SupportsVision: true,
				SupportsTools:  true,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Default Catch-All",
			Provider:       "openrouter",
			Model:          "google/gemini-3.8-flash",
			CodingIndex:    76.3,
			CostPerMillion: 1.6875,
			SupportsVision: true,
			SupportsTools:  true,
		},
	}
}
