package tuner

import (
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestGridSweep_OptimalRetriesAndThreshold(t *testing.T) {
	now := time.Now().UTC()

	// 5 synthetic sessions:
	// Turns under 4000 tokens succeed on local with 0 or 1 retry.
	// Turns over 4000 tokens fail 3 times before escalating to cloud, wasting 3 retries each time.
	var trajectories []SessionTrajectory
	for s := 0; s < 5; s++ {
		turns := []telemetry.TurnRecord{
			// Clean turn under 4000 tokens: succeeds
			{Timestamp: now.Add(time.Duration(s*100) * time.Second), Tokens: 2500, IsLocal: true, IsRetry: false, HasWriteProgress: true},
			// Large turn 8000 tokens: failed 3 times on local then resolved on cloud
			{Timestamp: now.Add(time.Duration(s*100+1) * time.Second), Tokens: 8000, IsLocal: true, IsRetry: true, HasWriteProgress: false},
			{Timestamp: now.Add(time.Duration(s*100+2) * time.Second), Tokens: 8000, IsLocal: true, IsRetry: true, HasWriteProgress: false},
			{Timestamp: now.Add(time.Duration(s*100+3) * time.Second), Tokens: 8000, IsLocal: true, IsRetry: true, HasWriteProgress: false},
			{Timestamp: now.Add(time.Duration(s*100+4) * time.Second), Tokens: 8000, IsLocal: false, IsRetry: false, HasWriteProgress: true, CostSpentUSD: 0.05},
		}
		trajectories = append(trajectories, SessionTrajectory{
			SessionID:  "sess-bench",
			Turns:      turns,
			TotalTurns: len(turns),
		})
	}

	tier := &contract.Tier{
		Name:       "Tier 1: Local GPU",
		Model:      "qwen2.5-coder:14b",
		Provider:   "ollama",
		MaxContext: 16000,
	}

	policy := DefaultTuningPolicy()
	policy.RetryPenaltyUSD = 5.0 // High penalty for wasted local retries

	res := GridSweep(trajectories, tier, false, false, nil, policy)

	// Since turns >= 8000 waste 3 retries on local, the optimizer must clamp Tokens < 8000
	if res.OptimalTokens > 8000 {
		t.Errorf("Expected OptimalTokens <= 8000 to avoid token cliff, got %d", res.OptimalTokens)
	}

	// OptimalRetries should be tuned to an aggressive low bound (<= 2) to prevent the 3 wasted retries
	if res.OptimalRetries < 1 || res.OptimalRetries > 2 {
		t.Errorf("Expected OptimalRetries to be 1 or 2, got %d", res.OptimalRetries)
	}

	if res.ProjectedCost <= 0 {
		t.Errorf("Expected ProjectedCost > 0, got %f", res.ProjectedCost)
	}

	if res.Fitness == 0 {
		t.Errorf("Expected non-zero Fitness calculation")
	}
}

func TestGridSweep_RespectsMaxContext(t *testing.T) {
	now := time.Now().UTC()
	trajectories := []SessionTrajectory{
		{
			SessionID: "sess-max-ctx",
			Turns: []telemetry.TurnRecord{
				{Timestamp: now, Tokens: 2000, IsLocal: true, HasWriteProgress: true},
			},
		},
	}

	tier := &contract.Tier{
		Name:       "Constrained VRAM Tier",
		MaxContext: 4000,
	}

	policy := DefaultTuningPolicy()
	res := GridSweep(trajectories, tier, false, false, nil, policy)

	if res.OptimalTokens > 4000 {
		t.Errorf("OptimalTokens %d exceeds MaxContext 4000", res.OptimalTokens)
	}
}

func TestGridSweep_EmptyTrajectories(t *testing.T) {
	policy := DefaultTuningPolicy()
	res := GridSweep(nil, nil, false, false, nil, policy)
	if res.OptimalTokens != 0 || res.OptimalRetries != 0 {
		t.Errorf("Expected zero result for empty trajectories, got %+v", res)
	}
}
