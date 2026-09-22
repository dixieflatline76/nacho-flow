package tuner

import (
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestMinConflicts_Convergence(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()

	// 5 sessions with 4 turns each:
	// Turns have ~3000 tokens. First 2 turns on local fail if Retries < 2 or Tokens < 2000.
	// Cloud workhorse succeeds.
	var records []telemetry.TurnRecord
	for s := 1; s <= 5; s++ {
		sessID := "sess-" + string(rune('0'+s))
		// Turn 0: 3500 tokens, local, fails
		records = append(records, telemetry.TurnRecord{
			Timestamp:        now.Add(time.Duration(s*100) * time.Second),
			SessionID:        sessID,
			Tokens:           3500,
			IsLocal:          true,
			IsRetry:          true,
			HasWriteProgress: false,
			RootPromptHash:   uint64(100 + s),
			CostSpentUSD:     0,
		})
		// Turn 1: 3500 tokens, local, fails
		records = append(records, telemetry.TurnRecord{
			Timestamp:        now.Add(time.Duration(s*100+1) * time.Second),
			SessionID:        sessID,
			Tokens:           3500,
			IsLocal:          true,
			IsRetry:          true,
			HasWriteProgress: false,
			RootPromptHash:   uint64(100 + s),
			CostSpentUSD:     0,
		})
		// Turn 2: 3500 tokens, cloud, succeeds
		records = append(records, telemetry.TurnRecord{
			Timestamp:        now.Add(time.Duration(s*100+2) * time.Second),
			SessionID:        sessID,
			Tokens:           3500,
			IsLocal:          false,
			IsRetry:          false,
			HasWriteProgress: true,
			RootPromptHash:   uint64(100 + s),
			CostSpentUSD:     0.02,
		})
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:       "Tier 1: Local GPU",
				Provider:   "ollama",
				When:       "Tokens < 1000 && Retries < 1",
				MaxContext: 16000,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 2: Cloud Fallback",
			Provider: "openrouter",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	opt := NewMinConflictsOptimizer(policy)
	start := time.Now()
	res, err := opt.Optimize(records, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil TuningResult")
	}
	if len(res.Tiers) != 1 {
		t.Fatalf("expected 1 tuned tier in result, got %d", len(res.Tiers))
	}

	t.Logf("Min-Conflicts converged in %v: Tier 1 optimal threshold=%d, retries=%d, rule=%s",
		elapsed, res.Tiers[0].OptimalThreshold, res.Tiers[0].OptimalRetries, res.Tiers[0].SynthesizedRule)

	if elapsed > 250*time.Millisecond {
		t.Errorf("expected convergence in < 250ms, took %v", elapsed)
	}
}

func TestMinConflicts_RealLifeTraffic(t *testing.T) {
	records := loadRealTrafficData(t)
	if len(records) == 0 {
		t.Skip("no real traffic records found")
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:       "Tier 1: Local GPU Workhorse",
				Provider:   "ollama",
				When:       "Tokens < 16000 && Retries < 2",
				MaxContext: 32000,
			},
			{
				Name:       "Tier 2: Cloud Coder",
				Provider:   "openrouter",
				When:       "Tokens < 32000 && Retries < 3",
				MaxContext: 64000,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 3: Frontier Fallback",
			Provider: "anthropic",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
			"anthropic":  {Type: contract.ProviderTypeCloud},
		},
	}

	policy := DefaultTuningPolicy()
	opt := NewMinConflictsOptimizer(policy)

	start := time.Now()
	res, err := opt.Optimize(records, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Optimize on real data failed: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil TuningResult")
	}
	if len(res.Tiers) != 2 {
		t.Fatalf("expected 2 tuned tiers, got %d", len(res.Tiers))
	}

	t.Logf("Real traffic tuned in %v across %d sessions (%d turns)",
		elapsed, res.TotalSessions, res.TotalSampleTurns)
	for i, tierRes := range res.Tiers {
		t.Logf("  Tier %d (%s): Threshold=%d, Retries=%d, Rule=%q",
			i+1, tierRes.TierName, tierRes.OptimalThreshold, tierRes.OptimalRetries, tierRes.SynthesizedRule)
	}
}
