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

// TDD Test: Disabled tier immunity (e.g. when: "false" like Opus On-Demand)
func TestDisabledTierImmunity_PreservesFalse(t *testing.T) {
	records := []telemetry.TurnRecord{
		{
			Timestamp:      time.Now().UTC(),
			SessionID:      "sess-1",
			RootPromptHash: 123,
			Tokens:         2000,
			SelectedTier:   "Tier 1: Local",
			IsLocal:        true,
			StatusCode:     200,
		},
		{
			Timestamp:      time.Now().UTC().Add(1 * time.Minute),
			SessionID:      "sess-1",
			RootPromptHash: 123,
			Tokens:         3000,
			SelectedTier:   "Tier 1: Local",
			IsLocal:        true,
			StatusCode:     200,
		},
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local",
				Provider: "ollama",
				Model:    "qwen2.5-coder:14b",
				When:     "Tokens < 16000 && Retries < 2",
			},
			{
				Name:     "Tier 5: Opus On-Demand (Spicy Only)",
				Provider: "openrouter",
				Model:    "anthropic/claude-opus-3",
				When:     "false", // Explicitly disabled
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 6: Frontier Fallback",
			Provider: "openrouter",
			Model:    "anthropic/claude-sonnet-5",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	if len(res.Tiers) != 2 {
		t.Fatalf("Expected 2 tiers in result, got %d", len(res.Tiers))
	}

	opusTier := res.Tiers[1]
	if opusTier.TierName != "Tier 5: Opus On-Demand (Spicy Only)" {
		t.Errorf("Expected tier name 'Tier 5: Opus On-Demand (Spicy Only)', got %s", opusTier.TierName)
	}

	// Must strictly be "false", never "Tokens < 8000 && Retries < 2 && false"!
	if opusTier.SynthesizedRule != "false" {
		t.Errorf("CRITICAL BUG: Expected disabled tier to remain 'false', got: %s", opusTier.SynthesizedRule)
	}
	if !opusTier.IsDisabled {
		t.Errorf("Expected IsDisabled to be true for when: 'false', got false")
	}
	if opusTier.OptimalThreshold != 0 {
		t.Errorf("Expected OptimalThreshold to be 0 for disabled tier, got %d", opusTier.OptimalThreshold)
	}
	if opusTier.OptimalRetries != 0 {
		t.Errorf("Expected OptimalRetries to be 0 for disabled tier, got %d", opusTier.OptimalRetries)
	}
}

// TDD Test: Zero-traffic active tier preserves original rule without dummy threshold additions
func TestZeroTrafficTier_PreservesOriginalRule(t *testing.T) {
	records := []telemetry.TurnRecord{
		{
			Timestamp:      time.Now().UTC(),
			SessionID:      "sess-1",
			RootPromptHash: 123,
			Tokens:         1500,
			SelectedTier:   "Tier 1: Local",
			IsLocal:        true,
			StatusCode:     200,
		},
	}

	originalRule := "Tokens > 50000 && HasImages"
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local",
				Provider: "ollama",
				Model:    "qwen2.5-coder:14b",
				When:     "Tokens < 16000",
			},
			{
				Name:     "Tier 2: High Context Vision",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     originalRule, // Never matched by the 1500-token text-only record
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 3: Fallback",
			Provider: "openrouter",
			Model:    "anthropic/claude-sonnet-5",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	t2 := res.Tiers[1]
	if t2.SynthesizedRule != originalRule {
		t.Errorf("Expected zero-traffic tier to preserve original rule %q, got %q", originalRule, t2.SynthesizedRule)
	}
}

// TDD Test: Model benchmarks and comprehensive dual pricing are populated from models.json catalog
func TestModelBenchmarkIntegration_AndDualPricing(t *testing.T) {
	records := []telemetry.TurnRecord{
		{
			Timestamp:          time.Now().UTC(),
			SessionID:          "sess-1",
			RootPromptHash:     123,
			Tokens:             2000,
			CycleContentTokens: 500, // 500 output tokens
			SelectedTier:       "Tier 1: Claude Sonnet",
			IsLocal:            false,
			StatusCode:         200,
		},
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Claude Sonnet",
				Provider: "openrouter",
				Model:    "anthropic/claude-sonnet-5",
				When:     "Tokens < 16000",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 2: Fallback",
			Provider: "openrouter",
			Model:    "anthropic/claude-sonnet-5",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	sonnetTier := res.Tiers[0]
	// Claude Sonnet 5 in data/models.json has coding_index=97.4
	if sonnetTier.CodingIndex < 90.0 {
		t.Errorf("Expected CodingIndex >= 90 for Claude Sonnet 5, got %f", sonnetTier.CodingIndex)
	}
	if sonnetTier.ComprehensiveRate <= 0.0 {
		t.Errorf("Expected positive ComprehensiveRate, got %f", sonnetTier.ComprehensiveRate)
	}
}

