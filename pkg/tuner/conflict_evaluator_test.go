package tuner

import (
	"math"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestEvaluateConflicts_HardConstraints(t *testing.T) {
	policy := DefaultTuningPolicy()

	// 1. Monotonic Context Hierarchy Violation: Tier 1 (16000) > Tier 2 (8000)
	nonMonotonicCfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{TierName: "Tier 1", TokenThreshold: 16000},
			{TierName: "Tier 2", TokenThreshold: 8000},
		},
		DefaultTier: TierReplayConfig{TierName: "Tier 3"},
	}

	trajectories := []SessionTrajectory{
		{
			SessionID: "sess-1",
			Turns: []telemetry.TurnRecord{
				{Tokens: 1000, Timestamp: time.Now().UTC()},
			},
		},
	}

	report := EvaluateFleetWithAttribution(trajectories, &nonMonotonicCfg, &policy, nil)
	if !math.IsInf(report.TotalConflict, 1) {
		t.Fatalf("expected +Inf conflict for non-monotonic context hierarchy, got %f", report.TotalConflict)
	}

	// 2. Hardware VRAM Ceiling Violation: Tier 1 TokenThreshold (8000) > MaxContext (4000)
	hardwareLimitCfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{TierName: "Tier 1", TokenThreshold: 8000, MaxContext: 4000},
		},
		DefaultTier: TierReplayConfig{TierName: "Tier 2"},
	}

	reportHw := EvaluateFleetWithAttribution(trajectories, &hardwareLimitCfg, &policy, nil)
	if !math.IsInf(reportHw.TotalConflict, 1) {
		t.Fatalf("expected +Inf conflict for hardware limit violation, got %f", reportHw.TotalConflict)
	}
}

func TestEvaluateConflicts_FractionalAttribution(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	policy.RetryPenaltyUSD = 3.0 // Pi(t) = 3.0 for simple calculation
	policy.CostWeight = 0.0      // Focus on retry penalties
	policy.TurnsWeight = 0.0

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:       "Tier 1: Local",
				Provider:       "ollama",
				IsLocal:        true,
				TokenThreshold: 8000,
				RetryBound:     3,
				RestrictTools:  false, // Currently allows tools
			},
		},
		DefaultTier: TierReplayConfig{
			TierName: "Tier 2: Default Cloud",
			Provider: "openrouter",
			IsLocal:  false,
		},
	}

	// Construct a failing turn on Tier 1 that could be repaired by 3 candidate variables:
	// 1. Tokens cliff (tokens = 5000: tightening threshold from 8000 to <=5000 would route to Tier 2)
	// 2. Tool gating (has_tools = true: setting RestrictTools = true would route to Tier 2)
	// 3. Keyword allocation ("compiler": excluding "compiler" from Tier 1 would route to Tier 2)
	trajectories := []SessionTrajectory{
		{
			SessionID: "sess-fractional",
			Turns: []telemetry.TurnRecord{
				{
					Timestamp:          now,
					Tokens:             5000,
					HasTools:           true,
					HasWriteCapability: true,
					Keywords:           []string{"compiler", "parsing"},
					IsLocal:            true,
					IsRetry:            true,
					HasWriteProgress:   false,
					RootPromptHash:     0x123,
				},
			},
		},
	}

	monitoredKeywords := []string{"compiler"}

	report := EvaluateFleetWithAttribution(trajectories, &cfg, &policy, monitoredKeywords)

	if math.IsInf(report.TotalConflict, 0) {
		t.Fatalf("unexpected infinite conflict: %f", report.TotalConflict)
	}

	// Penalty is 3.0. With 3 repair variables, each must receive exactly 3.0 / 3 = 1.0
	expectedPenaltyPerVar := 3.0 / 3.0

	tokenVar := "tier_0:tokens"
	toolVar := "tier_0:tools"
	kwVar := "keyword:compiler"

	if math.Abs(report.VariableConflicts[tokenVar]-expectedPenaltyPerVar) > 0.001 {
		t.Errorf("expected %s to have penalty %f, got %f", tokenVar, expectedPenaltyPerVar, report.VariableConflicts[tokenVar])
	}
	if math.Abs(report.VariableConflicts[toolVar]-expectedPenaltyPerVar) > 0.001 {
		t.Errorf("expected %s to have penalty %f, got %f", toolVar, expectedPenaltyPerVar, report.VariableConflicts[toolVar])
	}
	if math.Abs(report.VariableConflicts[kwVar]-expectedPenaltyPerVar) > 0.001 {
		t.Errorf("expected %s to have penalty %f, got %f", kwVar, expectedPenaltyPerVar, report.VariableConflicts[kwVar])
	}

	// Verify total conflict equals 3.0
	if math.Abs(report.TotalConflict-3.0) > 0.001 {
		t.Errorf("expected TotalConflict == 3.0, got %f", report.TotalConflict)
	}
}

func TestEvaluateConflicts_RealLifeTraffic(t *testing.T) {
	records := loadRealTrafficData(t)
	if len(records) == 0 {
		t.Skip("no real traffic records found")
	}

	trajectories := GroupBySession(records)
	if len(trajectories) == 0 {
		t.Skip("no trajectories")
	}

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:       "Tier 1: Local GPU",
				Provider:       "ollama",
				IsLocal:        true,
				TokenThreshold: 4000,
				RetryBound:     2,
			},
			{
				TierName:       "Tier 2: Workhorse",
				Provider:       "openrouter",
				IsLocal:        false,
				CostPerMillion: 2.0,
				TokenThreshold: 16000,
				RetryBound:     4,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 3: Fallback",
			Provider:       "openrouter",
			IsLocal:        false,
			CostPerMillion: 10.0,
		},
	}
	policy := DefaultTuningPolicy()
	keywords := []string{"ast", "concurrency", "tdd", "kubernetes"}

	report := EvaluateFleetWithAttribution(trajectories, &cfg, &policy, keywords)

	if math.IsInf(report.TotalConflict, 0) || math.IsNaN(report.TotalConflict) {
		t.Fatalf("expected valid numeric TotalConflict, got %f", report.TotalConflict)
	}
	if report.TotalTurns == 0 {
		t.Errorf("expected TotalTurns > 0")
	}
	if report.MaxConflictVar == "" && len(report.VariableConflicts) > 0 {
		t.Errorf("expected non-empty MaxConflictVar")
	}
}

func BenchmarkEvaluateFleetConflict_ZeroAlloc(b *testing.B) {
	records := loadRealTrafficData(b)
	if len(records) == 0 {
		b.Skip("no real traffic records found")
	}

	trajectories := GroupBySession(records)
	if len(trajectories) == 0 {
		b.Skip("no trajectories")
	}

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:       "Tier 1: Local GPU",
				Provider:       "ollama",
				IsLocal:        true,
				TokenThreshold: 4000,
				RetryBound:     2,
			},
			{
				TierName:       "Tier 2: Workhorse",
				Provider:       "openrouter",
				IsLocal:        false,
				CostPerMillion: 2.0,
				TokenThreshold: 16000,
				RetryBound:     4,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 3: Fallback",
			Provider:       "openrouter",
			IsLocal:        false,
			CostPerMillion: 10.0,
		},
	}
	policy := DefaultTuningPolicy()
	var buf MultiTierReplayResult

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c := EvaluateFleetConflict(trajectories, &cfg, &policy, &buf)
		if c <= 0 {
			b.Fatalf("expected conflict > 0, got %f", c)
		}
	}
}
