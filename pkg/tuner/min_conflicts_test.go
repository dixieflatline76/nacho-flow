package tuner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
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
	// Claude Sonnet 5 in data/models.json has coding_index=71.5 from live Artificial Analysis
	if sonnetTier.CodingIndex < 50.0 {
		t.Errorf("Expected CodingIndex >= 50 for Claude Sonnet 5, got %f", sonnetTier.CodingIndex)
	}
	if sonnetTier.ComprehensiveRate <= 0.0 {
		t.Errorf("Expected positive ComprehensiveRate, got %f", sonnetTier.ComprehensiveRate)
	}
}

// TDD Test: When current routing is already optimal (0 savings, 0 retries eliminated),
// the optimizer MUST NOT recommend any changes or inject fake token bounds into escalation tiers.
func TestOptimalRouting_NoChangeRecommendedWhenAlreadyOptimal(t *testing.T) {
	now := time.Now().UTC()
	records := []telemetry.TurnRecord{
		{
			Timestamp:          now,
			SessionID:          "sess-1",
			RootPromptHash:     101,
			Tokens:             1500,
			CycleContentTokens: 200,
			SelectedTier:       "Kickstart Escalation (Gemini 3.8 Flash)",
			IsLocal:            false,
			StatusCode:         200,
			Keywords:           []string{"init"},
		},
		{
			Timestamp:          now.Add(time.Second),
			SessionID:          "sess-1",
			RootPromptHash:     101,
			Tokens:             5000,
			CycleContentTokens: 300,
			SelectedTier:       "Tier 1: Local GPU Workhorse",
			IsLocal:            true,
			StatusCode:         200,
		},
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Kickstart Escalation (Gemini 3.8 Flash)",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "SessionKickstarted && Retries < 3",
			},
			{
				Name:     "Tier: Multimodal Vision (Gemini 3.8 Flash)",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "HasImages && Retries < 2",
			},
			{
				Name:       "Tier 1: Local GPU Workhorse",
				Provider:   "ollama",
				Model:      "qwen2.5-coder:14b",
				When:       "Tokens < 20000 && Retries < 2",
				MaxContext: 32000,
			},
			{
				Name:       "Tier 2: Flagship Agent Coder (Qwen3 Coder Plus)",
				Provider:   "openrouter",
				Model:      "qwen/qwen-2.5-coder-32b-instruct",
				When:       "Tokens < 160000 && Retries < 2",
				MaxContext: 160000,
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

	// Kickstart Escalation MUST NOT be modified with "Tokens < 8000"
	kickstart := res.Tiers[0]
	if kickstart.SynthesizedRule != "SessionKickstarted && Retries < 3" {
		t.Errorf("CRITICAL BUG: Optimal tier rule was mutated! Expected %q, got %q",
			"SessionKickstarted && Retries < 3", kickstart.SynthesizedRule)
	}

	// All tiers must remain strictly identical to their original rule
	for i, tier := range res.Tiers {
		if tier.SynthesizedRule != tier.OriginalRule {
			t.Errorf("Tier %d (%s) expected SynthesizedRule == OriginalRule (%q), got %q",
				i, tier.TierName, tier.OriginalRule, tier.SynthesizedRule)
		}
	}
}

func TestMinConflicts_ModelSubstitution_OverpricedTier(t *testing.T) {
	now := time.Now().UTC()
	var records []telemetry.TurnRecord
	for s := 1; s <= 5; s++ {
		sessID := fmt.Sprintf("sess-cost-%d", s)
		for turnIdx := 0; turnIdx < 3; turnIdx++ {
			records = append(records, telemetry.TurnRecord{
				Timestamp:        now.Add(time.Duration(s*100+turnIdx) * time.Second),
				SessionID:        sessID,
				Tokens:           2500,
				IsLocal:          false,
				IsRetry:          false,
				HasWriteProgress: true,
				RootPromptHash:   uint64(1000 + s),
				CostSpentUSD:     0.25, // Overpriced cloud execution
			})
		}
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:       "Tier 1: Overpriced Cloud",
				Provider:   "openrouter",
				Model:      "anthropic/claude-fable-5", // $10 / $50 per M
				When:       "Tokens < 16000",
				MaxContext: 128000,
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

	policy := DefaultTuningPolicy()
	policy.CostWeight = 2.0 // Elevate cost pressure
	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	if len(res.Tiers) == 0 {
		t.Fatalf("expected tuned tiers")
	}

	tunedTier := res.Tiers[0]
	if tunedTier.RecommendedModel == "" {
		t.Errorf("expected a recommended model substitution for overpriced tier")
	}
	if tunedTier.RecommendedModel == "anthropic/claude-fable-5" {
		t.Errorf("expected optimizer to recommend a cheaper alternative to fable-5, got %s", tunedTier.RecommendedModel)
	}
	if tunedTier.ModelBenefit == "" {
		t.Errorf("expected non-empty ModelBenefit explanation")
	}
}

func TestMinConflicts_ModelSubstitution_UnderperformingTier(t *testing.T) {
	now := time.Now().UTC()
	var records []telemetry.TurnRecord
	for s := 1; s <= 5; s++ {
		sessID := fmt.Sprintf("sess-retry-%d", s)
		for turnIdx := 0; turnIdx < 3; turnIdx++ {
			records = append(records, telemetry.TurnRecord{
				Timestamp:        now.Add(time.Duration(s*100+turnIdx) * time.Second),
				SessionID:        sessID,
				Tokens:           500,
				IsLocal:          false,
				IsRetry:          true, // Constant retry loops due to low model benchmark
				HasWriteProgress: false,
				RootPromptHash:   uint64(2000 + s),
				CostSpentUSD:     0.005,
			})
		}
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:       "Tier 1: Low Benchmark Tier",
				Provider:   "openrouter",
				Model:      "amazon/nova-2-lite-v1", // CodingIndex: 23
				When:       "Tokens < 16000",
				MaxContext: 32000,
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

	if len(res.Tiers) == 0 {
		t.Fatalf("expected tuned tiers")
	}

	tunedTier := res.Tiers[0]
	if tunedTier.RecommendedModel == "" {
		t.Errorf("expected a recommended model substitution for underperforming tier")
	}
	if tunedTier.RecommendedModel == "amazon/nova-2-lite-v1" {
		t.Errorf("expected optimizer to recommend a higher benchmark alternative, got %s", tunedTier.RecommendedModel)
	}
}

func TestMinConflicts_LocalVRAM_Budgeting(t *testing.T) {
	now := time.Now().UTC()
	var records []telemetry.TurnRecord
	for s := 1; s <= 5; s++ {
		records = append(records, telemetry.TurnRecord{
			Timestamp:        now.Add(time.Duration(s*100) * time.Second),
			SessionID:        fmt.Sprintf("sess-vram-%d", s),
			Tokens:           2000,
			IsLocal:          true,
			IsRetry:          true,
			HasWriteProgress: false,
			RootPromptHash:   uint64(3000 + s),
		})
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:       "Tier 1: Local",
				Provider:   "ollama",
				Model:      "qwen2.5-coder:7b",
				When:       "Tokens < 16000",
				MaxContext: 16000,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 2: Fallback",
			Provider: "openrouter",
			Model:    "anthropic/claude-sonnet-5",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	// 1. With 8GB VRAM: must not recommend models > 8B
	policy8 := DefaultTuningPolicy()
	policy8.LocalVRAMGB = 8
	opt8 := NewMinConflictsOptimizer(policy8)
	res8, err8 := opt8.Optimize(records, cfg)
	if err8 != nil {
		t.Fatalf("Optimize with 8GB VRAM failed: %v", err8)
	}
	if res8.Tiers[0].RecommendedModel != "" {
		size := curation.ExtractParameterSize(res8.Tiers[0].RecommendedModel)
		if size > 8.0 {
			t.Errorf("expected candidate <= 8B for 8GB VRAM, got %s (%.1fB)", res8.Tiers[0].RecommendedModel, size)
		}
	}

	// 2. With 24GB VRAM: allowed up to 32B
	policy24 := DefaultTuningPolicy()
	policy24.LocalVRAMGB = 24
	opt24 := NewMinConflictsOptimizer(policy24)
	res24, err24 := opt24.Optimize(records, cfg)
	if err24 != nil {
		t.Fatalf("Optimize with 24GB VRAM failed: %v", err24)
	}
	if res24.Tiers[0].RecommendedModel != "" {
		size := curation.ExtractParameterSize(res24.Tiers[0].RecommendedModel)
		if size > 33.0 {
			t.Errorf("expected candidate <= 33B for 24GB VRAM, got %s (%.1fB)", res24.Tiers[0].RecommendedModel, size)
		}
	}
}

func TestMinConflicts_MultiTierCascade_LocalContextCliff(t *testing.T) {
	now := time.Now().UTC()
	var records []telemetry.TurnRecord

	// Simulate 10 sessions with regular coding traffic
	for s := 1; s <= 10; s++ {
		sessID := fmt.Sprintf("sess-cascade-cliff-%d", s)
		promptHash := uint64(5000 + s)

		// Turn 1: Small prompt (2000 tokens) -> succeeds on local
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*100+1) * time.Second),
			SessionID:          sessID,
			Tokens:             2000,
			IsLocal:            true,
			IsRetry:            false,
			HasWriteProgress:   true,
			SessionKickstarted: false,
			HasImages:          false,
			RootPromptHash:     promptHash,
		})

		// Turn 2: Medium prompt (3500 tokens) -> succeeds on local
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*100+2) * time.Second),
			SessionID:          sessID,
			Tokens:             3500,
			IsLocal:            true,
			IsRetry:            false,
			HasWriteProgress:   true,
			SessionKickstarted: false,
			HasImages:          false,
			RootPromptHash:     promptHash,
		})

		// Turn 3: Large prompt (12000 tokens) -> fails on local (wasted retry)
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*100+3) * time.Second),
			SessionID:          sessID,
			Tokens:             12000,
			IsLocal:            true,
			IsRetry:            true,
			HasWriteProgress:   false,
			SessionKickstarted: false,
			HasImages:          false,
			RootPromptHash:     promptHash,
		})

		// Turn 4: Large prompt retry (12000 tokens) -> fails again on local (2nd wasted retry)
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*100+4) * time.Second),
			SessionID:          sessID,
			Tokens:             12000,
			IsLocal:            true,
			IsRetry:            true,
			HasWriteProgress:   false,
			SessionKickstarted: false,
			HasImages:          false,
			RootPromptHash:     promptHash,
		})
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Kickstart Escalation (Flash)",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "SessionKickstarted && Retries < 3",
			},
			{
				Name:     "Tier: Multimodal Vision (Flash)",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "HasImages && Retries < 2",
			},
			{
				Name:       "Tier 1: Local GPU Workhorse",
				Provider:   "ollama",
				Model:      "qwen2.5-coder:14b",
				When:       "Tokens < 20000 && Retries < 2",
				MaxContext: 32000,
			},
			{
				Name:       "Tier 2: Flagship Agent Coder (Cloud)",
				Provider:   "openrouter",
				Model:      "qwen/qwen-2.5-coder-32b-instruct",
				When:       "Tokens < 160000 && Retries < 2",
				MaxContext: 160000,
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

	// 1. Kickstart Escalation must preserve original rule (no cannibalization)
	kickstart := res.Tiers[0]
	if kickstart.SynthesizedRule != "SessionKickstarted && Retries < 3" {
		t.Errorf("Kickstart tier mutated: expected %q, got %q", "SessionKickstarted && Retries < 3", kickstart.SynthesizedRule)
	}

	// 2. Multimodal Vision must preserve original rule
	vision := res.Tiers[1]
	if vision.SynthesizedRule != "HasImages && Retries < 2" {
		t.Errorf("Vision tier mutated: expected %q, got %q", "HasImages && Retries < 2", vision.SynthesizedRule)
	}

	// 3. Local GPU Workhorse MUST receive traffic and optimize its context cliff down from 20000!
	localTier := res.Tiers[2]
	t.Logf("Tuned Local GPU Tier: threshold=%d, retries=%d, rule=%q", localTier.OptimalThreshold, localTier.OptimalRetries, localTier.SynthesizedRule)
	t.Logf("Eliminated Retries: %d", res.RetriesEliminated)
	if localTier.SynthesizedRule == "Tokens < 20000 && Retries < 2" {
		t.Errorf("Expected Local GPU threshold to be optimized down from 20000 due to failures at 12000, but rule remained unchanged: %q", localTier.SynthesizedRule)
	}
	if localTier.OptimalThreshold >= 12000 || localTier.OptimalThreshold == 0 {
		t.Errorf("Expected optimal threshold < 12000 (e.g. 4000 or 8000), got %d", localTier.OptimalThreshold)
	}

	// 4. Retries should have been eliminated
	if res.RetriesEliminated <= 0 {
		t.Errorf("Expected positive retries eliminated, got %d", res.RetriesEliminated)
	}
}

func TestMinConflicts_EscalationAntiDuplication(t *testing.T) {
	now := time.Now().UTC()
	var records []telemetry.TurnRecord

	// Simulate sessions that fail in Tier 3 (5 retries) and escalate to Tier 4
	for s := 1; s <= 5; s++ {
		sessID := fmt.Sprintf("sess-escalation-%d", s)
		rootHash := uint64(5000 + s)
		for turnIdx := 0; turnIdx < 7; turnIdx++ {
			records = append(records, telemetry.TurnRecord{
				Timestamp:        now.Add(time.Duration(s*100+turnIdx) * time.Second),
				SessionID:        sessID,
				Tokens:           2500,
				IsLocal:          false,
				IsRetry:          turnIdx > 0,
				HasWriteProgress: turnIdx == 6, // succeeds on turn 6 (Tier 4)
				RootPromptHash:   rootHash,
				CostSpentUSD:     0.01,
			})
		}
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 3: Workhorse",
				Provider: "openrouter",
				Model:    "google/gemini-3.8-flash",
				When:     "Retries < 5",
			},
			{
				Name:     "Tier 4: Frontier Powerhouse",
				Provider: "openrouter",
				Model:    "deepseek/deepseek-v4-pro", // underperforming score 59.4
				When:     "Retries >= 5 && Retries < 8",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
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

	tier3 := res.Tiers[0]
	tier4 := res.Tiers[1]

	// Invariant 1: Tier 4 must NOT duplicate Tier 3's model
	if tier4.RecommendedModel == tier3.RecommendedModel {
		t.Fatalf("Anti-duplication violation: Escalation Tier 4 (%s) duplicated Tier 3 model (%s)", tier4.RecommendedModel, tier3.RecommendedModel)
	}

	// Invariant 2: Tier 4 must be a superior or frontier model
	if tier4.RecommendedModel == "deepseek/deepseek-v4-pro" {
		t.Errorf("Expected Tier 4 underperforming model to be substituted")
	}
	if tier4.CodingIndex < tier3.CodingIndex && !IsFrontierModel(tier4.RecommendedModel) {
		t.Errorf("Expected Tier 4 recommended model to be superior or frontier, got index %.1f (%s)", tier4.CodingIndex, tier4.RecommendedModel)
	}
}

func TestMinConflicts_DefaultTier_RestoresFlashWithoutPriceSpike(t *testing.T) {
	now := time.Now().UTC()
	var records []telemetry.TurnRecord

	// Traffic that cascades to Default Tier (e.g. large context turns)
	for s := 1; s <= 5; s++ {
		sessID := fmt.Sprintf("sess-default-%d", s)
		records = append(records, telemetry.TurnRecord{
			Timestamp:        now.Add(time.Duration(s*100) * time.Second),
			SessionID:        sessID,
			Tokens:           500000, // exceeds Tier 1
			IsLocal:          false,
			IsRetry:          false,
			HasWriteProgress: true,
			RootPromptHash:   uint64(6000 + s),
			CostSpentUSD:     0.05,
		})
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Workhorse",
				Provider: "openrouter",
				Model:    "google/gemini-3.8-flash",
				When:     "Tokens < 160000",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Catch-All",
			Provider: "openrouter",
			Model:    "z-ai/glm-5.3-flash", // CodingIndex 71.5 < Tier 1's 76.3
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

	if res.DefaultTier == nil {
		t.Fatalf("expected DefaultTier tuning result")
	}

	// Should recommend Gemini 3.8 Flash (rate ~$0.35/M) instead of jumping to Grok 4.6 ($3.50/M)
	if res.DefaultTier.RecommendedModel != "google/gemini-3.8-flash" {
		t.Errorf("Expected DefaultTier to recommend google/gemini-3.8-flash, got %s (comp rate: %.2f)",
			res.DefaultTier.RecommendedModel, res.DefaultTier.ComprehensiveRate)
	}
}

func TestMinConflicts_EdgeCasesAndHelpers(t *testing.T) {
	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	if opt.Name() != "min_conflicts" {
		t.Errorf("expected Name() == 'min_conflicts', got %s", opt.Name())
	}

	// 1. Optimize with nil config
	resNil, errNil := opt.Optimize(nil, nil)
	if errNil != nil || resNil == nil || len(resNil.Tiers) != 0 {
		t.Errorf("expected empty tuning result for nil config")
	}

	// 2. Optimize with empty records and various tier configs
	cfgEmptyRecords := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:  "Disabled Tier",
				Model: "disabled/model",
				When:  "false",
			},
			{
				Name:       "Low Context Tier",
				Model:      "google/gemini-3.8-flash",
				When:       "Tokens < 20000",
				MaxContext: 8000,
			},
		},
	}
	resEmptyRec, errEmptyRec := opt.Optimize([]telemetry.TurnRecord{}, cfgEmptyRecords)
	if errEmptyRec != nil || resEmptyRec == nil || len(resEmptyRec.Tiers) != 2 {
		t.Fatalf("expected 2 tier results for empty records, got %v", resEmptyRec)
	}
	if !resEmptyRec.Tiers[0].IsDisabled || resEmptyRec.Tiers[0].SynthesizedRule != "false" {
		t.Errorf("expected disabled tier to synthesize 'false'")
	}
	if resEmptyRec.Tiers[1].OptimalThreshold != 8000 {
		t.Errorf("expected threshold capped to MaxContext 8000, got %d", resEmptyRec.Tiers[1].OptimalThreshold)
	}

	// 3. OptimizeWithContext
	ctx := context.Background()
	resCtx, errCtx := opt.OptimizeWithContext(ctx, nil, cfgEmptyRecords)
	if errCtx != nil || resCtx == nil {
		t.Errorf("unexpected error in OptimizeWithContext: %v", errCtx)
	}

	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, errCanc := opt.OptimizeWithContext(cancCtx, nil, cfgEmptyRecords)
	if errCanc == nil {
		t.Errorf("expected error for canceled context in OptimizeWithContext")
	}

	// 4. IsFrontierTier & IsFrontierModel
	if !IsFrontierTier("Tier 5", "anthropic/claude-opus-5") {
		t.Errorf("expected claude-opus-5 to be frontier tier")
	}
	if IsFrontierTier("Tier 1", "unknown/fake-model-xyz") {
		t.Errorf("expected fake model not to be frontier")
	}
	if IsFrontierModel("") {
		t.Errorf("expected empty model not to be frontier")
	}

	// 5. copyMultiTierConfig & hashState
	if copyMultiTierConfig(nil).Tiers != nil {
		t.Errorf("expected nil tiers for nil config copy")
	}

	cfgWithKeywords := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:         "Tier With KW",
				TokenThreshold:   4000,
				RetryBound:       2,
				RestrictImages:   true,
				RestrictTools:    true,
				Model:            "test/model",
				ExcludedKeywords: []string{"deploy", "release"},
			},
		},
		DefaultTier: TierReplayConfig{
			Model: "default/model",
		},
	}
	cpCfg := copyMultiTierConfig(&cfgWithKeywords)
	if len(cpCfg.Tiers[0].ExcludedKeywords) != 2 {
		t.Errorf("expected copied keywords")
	}

	h1 := hashState(&cfgWithKeywords)
	h2 := hashState(&cpCfg)
	if h1 != h2 || h1 == 0 {
		t.Errorf("expected identical non-zero hashes, got %d and %d", h1, h2)
	}

	// 6. equalStringSlices
	if !equalStringSlices([]string{"a", "b"}, []string{"a", "b"}) {
		t.Errorf("expected equal slices")
	}
	if equalStringSlices([]string{"a"}, []string{"a", "b"}) {
		t.Errorf("expected unequal slices on length")
	}
	if equalStringSlices([]string{"a", "c"}, []string{"a", "b"}) {
		t.Errorf("expected unequal slices on content")
	}

	// 7. ReplayMultiTierSession nil check
	ReplayMultiTierSession(nil, nil, nil, nil)
}

func TestMinConflicts_AlreadyOptimalConfig(t *testing.T) {
	// A single clean turn that already routes to local workhorse and succeeds
	records := []telemetry.TurnRecord{
		{
			Timestamp:        time.Now().UTC(),
			SessionID:        "sess-clean",
			Tokens:           1000,
			IsLocal:          true,
			IsRetry:          false,
			HasWriteProgress: true,
			RootPromptHash:   999,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "qwen2.5-coder:14b",
				When:     "Tokens < 16000",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 2",
			Provider: "openrouter",
			Model:    "google/gemini-3.8-flash",
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
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RetriesEliminated != 0 {
		t.Errorf("expected 0 retries eliminated on clean data")
	}
}

func TestMinConflicts_InvalidTierAST(t *testing.T) {
	records := []telemetry.TurnRecord{
		{
			Timestamp: time.Now().UTC(),
			SessionID: "sess-bad-ast",
			Tokens:    1000,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name: "Bad AST Tier",
				When: "Tokens < < < invalid expression syntax",
			},
		},
		DefaultTier: contract.Tier{
			Name: "Default",
			When: "true",
		},
	}
	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	_, err := opt.Optimize(records, cfg)
	if err == nil {
		t.Errorf("expected error for invalid AST in tier")
	}
}

func TestMinConflicts_AllRepairBranches(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	policy.LocalVRAMGB = 16

	records := []telemetry.TurnRecord{
		// Turn 1: has tools on tier without tools
		{
			Timestamp:      now,
			SessionID:      "sess-tools",
			Tokens:         500,
			HasTools:       true,
			IsLocal:        false,
			IsRetry:        true,
			RootPromptHash: 101,
		},
		// Turn 2: has images on tier without images
		{
			Timestamp:      now.Add(time.Second),
			SessionID:      "sess-images",
			Tokens:         500,
			HasImages:      true,
			IsLocal:        false,
			IsRetry:        true,
			RootPromptHash: 102,
		},
		// Turn 3: keyword failure
		{
			Timestamp:      now.Add(2 * time.Second),
			SessionID:      "sess-kw",
			Tokens:         500,
			Keywords:       []string{"deploy", "migration"},
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: 103,
		},
		// Turn 4: retries failure
		{
			Timestamp:      now.Add(3 * time.Second),
			SessionID:      "sess-retries",
			Tokens:         500,
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: 104,
		},
		// Turn 5: default tier failure
		{
			Timestamp:      now.Add(4 * time.Second),
			SessionID:      "sess-default",
			Tokens:         500000,
			IsLocal:        false,
			IsRetry:        true,
			RootPromptHash: 105,
		},
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:       "Tier 1: Local GPU",
				Provider:   "ollama",
				Model:      "gemma4:12b-it-qat",
				When:       "Tokens < 16000 && Retries < 2",
				MaxContext: 16000,
			},
			{
				Name:       "Tier 2: Cloud Weak",
				Provider:   "openrouter",
				Model:      "deepseek/deepseek-v4-pro",
				When:       "Tokens < 160000",
				MaxContext: 160000,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Tier",
			Provider: "openrouter",
			Model:    "z-ai/glm-5.3-flash",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("unexpected error in Optimize: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestMinConflicts_RepairRetries(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	records := []telemetry.TurnRecord{
		{
			Timestamp:      now,
			SessionID:      "sess-r",
			Tokens:         500,
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: 501,
		},
		{
			Timestamp:      now.Add(time.Second),
			SessionID:      "sess-r",
			Tokens:         500,
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: 501,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "gemma4:12b-it-qat",
				When:     "Tokens < 16000 && Retries < 5",
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "google/gemini-3.8-flash", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(policy)
	_, _ = opt.Optimize(records, cfg)
}

func TestMinConflicts_RepairDefaultTierModel(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	records := []telemetry.TurnRecord{
		{
			Timestamp:      now,
			SessionID:      "sess-def",
			Tokens:         50000,
			IsLocal:        false,
			IsRetry:        true,
			RootPromptHash: 601,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "openrouter",
				Model:    "google/gemini-3.8-flash",
				When:     "Tokens < 4000",
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "deepseek/deepseek-v4-pro", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(policy)
	_, _ = opt.Optimize(records, cfg)
}

func TestMinConflicts_RepairCloudModel(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	records := []telemetry.TurnRecord{
		{
			Timestamp:      now,
			SessionID:      "sess-cloud",
			Tokens:         500,
			IsLocal:        false,
			IsRetry:        true,
			RootPromptHash: 701,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "openrouter",
				Model:    "deepseek/deepseek-v4-pro",
				When:     "Tokens < 160000",
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "google/gemini-3.8-flash", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(policy)
	_, _ = opt.Optimize(records, cfg)
}

func TestMinConflicts_RepairTools(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	policy.LocalVRAMGB = 0
	records := []telemetry.TurnRecord{
		{
			Timestamp:      now,
			SessionID:      "sess-tool",
			Tokens:         500,
			HasTools:       true,
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: 801,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:       "Tier 1",
				Provider:   "ollama",
				Model:      "",
				When:       "Tokens < 16000",
				StripTools: true,
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "google/gemini-3.8-flash", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("unexpected error in Optimize: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestMinConflicts_RepairImages(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	policy.LocalVRAMGB = 0
	records := []telemetry.TurnRecord{
		{
			Timestamp:      now,
			SessionID:      "sess-img",
			Tokens:         500,
			HasImages:      true,
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: 802,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:        "Tier 1",
				Provider:    "ollama",
				Model:       "",
				When:        "Tokens < 16000",
				StripImages: true,
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "google/gemini-3.8-flash", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("unexpected error in Optimize: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestMinConflicts_RepairKeywords(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	policy.MinOccurrences = 2
	policy.LocalVRAMGB = 0
	var records []telemetry.TurnRecord
	for i := 0; i < 15; i++ {
		records = append(records, telemetry.TurnRecord{
			Timestamp:      now.Add(time.Duration(i) * time.Second),
			SessionID:      fmt.Sprintf("sess-kw-%d", i),
			Tokens:         500,
			Keywords:       []string{"kubernetes"},
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: uint64(900 + i),
		})
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "qwen2.5-coder:7b",
				When:     "Tokens < 16000",
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "google/gemini-3.8-flash", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("unexpected error in Optimize: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestMinConflicts_NoTunableTiers(t *testing.T) {
	records := []telemetry.TurnRecord{
		{
			Timestamp: time.Now().UTC(),
			SessionID: "sess-1",
			Tokens:    1000,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{},
		DefaultTier: contract.Tier{
			Name: "Default",
			When: "true",
		},
	}
	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	res, err := opt.Optimize(records, cfg)
	if err != nil || len(res.Tiers) != 0 {
		t.Errorf("expected empty tiers and nil error, got res=%v, err=%v", res, err)
	}
}

func TestMinConflicts_LegacySessionFallback(t *testing.T) {
	records := []telemetry.TurnRecord{
		{
			Timestamp: time.Now().UTC(),
			SessionID: "", // empty SessionID drops out of normal GroupBySession
			Tokens:    1000,
			IsLocal:   true,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "qwen2.5-coder:7b",
				When:     "Tokens < 16000",
			},
		},
		DefaultTier: contract.Tier{
			Name: "Default",
			When: "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama": {Type: contract.ProviderTypeLocal},
		},
	}
	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TotalSessions != 1 {
		t.Errorf("expected 1 session from legacy-session fallback, got %d", res.TotalSessions)
	}
}

func TestMinConflicts_MoreThan8Keywords(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()
	policy.MinOccurrences = 1
	var records []telemetry.TurnRecord
	for i := 0; i < 12; i++ {
		kw := fmt.Sprintf("kw-%d", i)
		records = append(records, telemetry.TurnRecord{
			Timestamp:      now.Add(time.Duration(i) * time.Second),
			SessionID:      fmt.Sprintf("sess-kw8-%d", i),
			Tokens:         1500,
			Keywords:       []string{kw},
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: uint64(1000 + i),
		})
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "qwen2.5-coder:7b",
				When:     "Tokens < 16000",
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "google/gemini-3.8-flash", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestMinConflicts_EmptyRecords_InvalidAST(t *testing.T) {
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "qwen2.5-coder:7b",
				When:     "Tokens < < < invalid expression syntax",
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "google/gemini-3.8-flash", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	res, err := opt.Optimize([]telemetry.TurnRecord{}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Tiers) != 1 || res.Tiers[0].SynthesizedRule != "Tokens < < < invalid expression syntax" {
		t.Errorf("expected original rule preserved when AST rewrite fails on empty records, got %q", res.Tiers[0].SynthesizedRule)
	}
}

func TestMinConflicts_TierASTParseErrorOnFinalSynthesis(t *testing.T) {
	now := time.Now().UTC()
	records := []telemetry.TurnRecord{
		{
			Timestamp:        now,
			SessionID:        "sess-ast-err",
			Tokens:           12000,
			IsLocal:          true,
			IsRetry:          true,
			HasWriteProgress: false,
			RootPromptHash:   2001,
		},
	}
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "qwen2.5-coder:7b",
				When:     "Tokens < < < 16000",
			},
		},
		DefaultTier: contract.Tier{Name: "Def", Provider: "openrouter", Model: "google/gemini-3.8-flash", When: "true"},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	_, err := opt.Optimize(records, cfg)
	if err == nil || !strings.Contains(err.Error(), "failed to rewrite AST") {
		t.Errorf("expected 'failed to rewrite AST' error, got %v", err)
	}
}

func TestMinConflicts_DefaultTier_FrontierAndLocalProtection(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()

	// 1. Frontier fallback protection (anthropic/claude-opus-5)
	recordsFrontier := []telemetry.TurnRecord{
		{
			Timestamp:      now,
			SessionID:      "sess-def-frontier",
			Tokens:         500000,
			IsLocal:        false,
			IsRetry:        true,
			RootPromptHash: 3001,
		},
	}
	cfgFrontier := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "openrouter",
				Model:    "google/gemini-3.8-flash",
				When:     "Tokens < 4000",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Frontier Default",
			Provider: "openrouter",
			Model:    "anthropic/claude-opus-5",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}
	opt := NewMinConflictsOptimizer(policy)
	_, _ = opt.Optimize(recordsFrontier, cfgFrontier)

	// 2. Local fallback protection
	recordsLocal := []telemetry.TurnRecord{
		{
			Timestamp:      now,
			SessionID:      "sess-def-local",
			Tokens:         500000,
			IsLocal:        true,
			IsRetry:        true,
			RootPromptHash: 3002,
		},
	}
	cfgLocal := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "qwen2.5-coder:7b",
				When:     "Tokens < 4000",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Local Default",
			Provider: "ollama",
			Model:    "qwen2.5-coder:14b",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama": {Type: contract.ProviderTypeLocal},
		},
	}
	_, _ = opt.Optimize(recordsLocal, cfgLocal)
}
