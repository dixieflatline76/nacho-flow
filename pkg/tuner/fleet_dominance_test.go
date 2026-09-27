package tuner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/config"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestIsFrontierModel(t *testing.T) {
	frontierModels := []string{
		"anthropic/claude-sonnet-5",
		"anthropic/claude-opus-5",
		"openai/gpt-6-astra",
		"openai/o1",
		"openai/o3-mini",
		"openai/gpt-5.6-sol",
	}
	for _, m := range frontierModels {
		if !IsFrontierModel(m) {
			t.Errorf("Expected %q to be recognized as frontier model", m)
		}
	}

	nonFrontierModels := []string{
		"google/gemini-3.8-flash",
		"google/gemini-3.1-pro-preview",
		"qwen/qwen3-coder-plus",
		"z-ai/glm-5.3-flash",
		"gemma4:12b-it-qat",
	}
	for _, m := range nonFrontierModels {
		if IsFrontierModel(m) {
			t.Errorf("Expected %q to NOT be recognized as frontier model", m)
		}
	}
}

func TestAnalyzeFleetDominance_Detection(t *testing.T) {
	// Cascade where Tier 4 (Gemini 3.1 Pro score 68.8) escalates from Tier 3 (Gemini 3.8 Flash score 76.3)
	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:    "Tier 3: Debug & Reasoning Workhorse",
				Model:       "google/gemini-3.8-flash",
				CodingIndex: 76.3,
				RetryBound:  5,
			},
			{
				TierName:    "Tier 4: Large Context Synthesis",
				Model:       "google/gemini-3.1-pro-preview",
				CodingIndex: 68.8,
				RetryBound:  7,
			},
			{
				TierName:    "Tier 5: Frontier Powerhouse",
				Model:       "anthropic/claude-sonnet-5",
				CodingIndex: 71.5,
				RetryBound:  9,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:    "Default: Cost-Safe Catch-All",
			Model:       "anthropic/claude-sonnet-5",
			CodingIndex: 71.5,
		},
	}

	conflicts := AnalyzeFleetDominance(&cfg)
	if len(conflicts) != 1 {
		t.Fatalf("Expected exactly 1 dominance conflict, got %d: %+v", len(conflicts), conflicts)
	}

	c := conflicts[0]
	if c.TierName != "Tier 4: Large Context Synthesis" {
		t.Errorf("Expected conflict on Tier 4, got %q", c.TierName)
	}
	if c.CurrentBenchmark != 68.8 || c.RequiredBenchmark != 76.3 {
		t.Errorf("Expected benchmark 68.8 vs required 76.3, got %.1f vs %.1f", c.CurrentBenchmark, c.RequiredBenchmark)
	}
}

func TestEndToEndTune_RealTraffic(t *testing.T) {
	cfg, err := config.LoadConfig("testdata/baseline_config.yaml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	candidates := []string{
		filepath.Join("testdata", "traffic.jsonl"),
		filepath.Join("..", "..", "pkg", "tuner", "testdata", "traffic.jsonl"),
		filepath.Join("..", "..", "logs", "traffic.jsonl"),
		filepath.Join("logs", "traffic.jsonl"),
	}
	var trafficPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			trafficPath = p
			break
		}
	}
	if trafficPath == "" {
		t.Skip("traffic test fixture not found")
	}

	records, err := telemetry.ReadCompleteSessions(trafficPath, 0)
	if err != nil {
		t.Fatalf("ReadCompleteSessions failed: %v", err)
	}
	if len(records) == 0 {
		t.Skip("no complete sessions in traffic fixture")
	}

	opt := NewMinConflictsOptimizer(DefaultTuningPolicy())
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	// 1. Static Dominance Defect must be detected
	if len(res.StaticDominanceConflicts) == 0 {
		t.Errorf("Expected StaticDominanceConflicts to be populated on TuningResult")
	}

	// 2. Local GPU Workhorse context cliff must be tightened to 4000
	localTier := res.Tiers[2]
	if localTier.RecommendedModel != "gemma4:12b-it-qat" {
		t.Errorf("Expected local tier model gemma4:12b-it-qat to be preserved, got %q", localTier.RecommendedModel)
	}
	if localTier.OptimalThreshold != 4000 {
		t.Errorf("Expected local tier threshold to be optimized to 4000, got %d", localTier.OptimalThreshold)
	}
	if localTier.RestrictTools {
		t.Errorf("Local GPU tier should allow tools, but tools were restricted")
	}

	// 3. Tier 4 capability inversion must be resolved by pruning the redundant tier
	tier4 := res.Tiers[5]
	if !tier4.IsDisabled {
		t.Errorf("Expected Tier 4 (Gemini 3.1 Pro) to be pruned (IsDisabled=true) to resolve capability inversion, got IsDisabled=false")
	}
	if tier4.SynthesizedRule != "false" {
		t.Errorf("Expected pruned Tier 4 to have SynthesizedRule 'false', got %q", tier4.SynthesizedRule)
	}
	if !strings.Contains(tier4.ModelBenefit, "Redundant Escalation Tier Bypassed") {
		t.Errorf("Expected model benefit to explain redundant tier bypass, got %q", tier4.ModelBenefit)
	}

	// 4. Frontier tier and Default tier must preserve Claude Sonnet 5
	frontierTier := res.Tiers[6]
	if frontierTier.RecommendedModel != "anthropic/claude-sonnet-5" {
		t.Errorf("Expected Tier 5 (Frontier Powerhouse) to preserve anthropic/claude-sonnet-5, got %q", frontierTier.RecommendedModel)
	}

	if res.DefaultTier != nil && res.DefaultTier.RecommendedModel != "anthropic/claude-sonnet-5" {
		t.Errorf("Expected Default Tier to preserve anthropic/claude-sonnet-5, got %q", res.DefaultTier.RecommendedModel)
	}

	// 5. Retries eliminated
	if res.RetriesEliminated <= 0 {
		t.Errorf("Expected retries eliminated > 0, got %d", res.RetriesEliminated)
	}
}

func TestEndToEndTune_RealTraffic_WithVRAM16(t *testing.T) {
	cfg, err := config.LoadConfig("testdata/baseline_config.yaml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	candidates := []string{
		filepath.Join("testdata", "traffic.jsonl"),
		filepath.Join("..", "..", "pkg", "tuner", "testdata", "traffic.jsonl"),
		filepath.Join("..", "..", "logs", "traffic.jsonl"),
		filepath.Join("logs", "traffic.jsonl"),
	}
	var trafficPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			trafficPath = p
			break
		}
	}
	if trafficPath == "" {
		t.Skip("traffic test fixture not found")
	}

	records, err := telemetry.ReadCompleteSessions(trafficPath, 0)
	if err != nil {
		t.Fatalf("ReadCompleteSessions failed: %v", err)
	}
	if len(records) == 0 {
		t.Skip("no complete sessions in traffic fixture")
	}

	policy := DefaultTuningPolicy()
	policy.LocalVRAMGB = 16
	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	// Local GPU Workhorse context cliff must still be tightened to 4000
	localTier := res.Tiers[2]
	if localTier.OptimalThreshold != 4000 {
		t.Errorf("Expected local tier threshold to be optimized to 4000 even with LocalVRAMGB=16, got %d", localTier.OptimalThreshold)
	}
	if localTier.SynthesizedRule != "Tokens < 4000 && Retries < 2" && localTier.SynthesizedRule != "Tokens < 4000 && Retries < 1" {
		t.Errorf("Expected local tier rule to tighten to 4000 tokens, got %q", localTier.SynthesizedRule)
	}
	if res.RetriesEliminated <= 0 {
		t.Errorf("Expected retries eliminated > 0, got %d", res.RetriesEliminated)
	}
}

func TestAnalyzeFleetDominance_EscalationModelDuplication(t *testing.T) {
	// Cascade where Tier 4 duplicates the model of Tier 3 on an escalation path (Retries >= 5 after Retries < 5)
	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:    "Tier 3: Debug & Reasoning Workhorse",
				Model:       "google/gemini-3.8-flash",
				CodingIndex: 76.3,
				RetryBound:  5,
			},
			{
				TierName:    "Tier 4: Frontier Powerhouse",
				Model:       "google/gemini-3.8-flash", // DUPLICATE!
				CodingIndex: 76.3,
				RetryFloor:  5,
				RetryBound:  8,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:    "Default: Cost-Safe Catch-All",
			Model:       "google/gemini-3.8-flash",
			CodingIndex: 76.3,
		},
	}

	conflicts := AnalyzeFleetDominance(&cfg)
	if len(conflicts) != 1 {
		t.Fatalf("Expected 1 dominance conflict for escalation model duplication, got %d: %+v", len(conflicts), conflicts)
	}
	if conflicts[0].TierIndex != 1 {
		t.Errorf("Expected conflict on Tier 4 (index 1), got %d", conflicts[0].TierIndex)
	}
	if conflicts[0].PredecessorIndex != 0 {
		t.Errorf("Expected predecessor to be Tier 3 (index 0), got %d", conflicts[0].PredecessorIndex)
	}
	if conflicts[0].Reason != `Escalation tier "Tier 4: Frontier Powerhouse" duplicates predecessor model "google/gemini-3.8-flash" (degenerate retry loop)` {
		t.Errorf("Unexpected conflict reason: %s", conflicts[0].Reason)
	}
}

func TestAnalyzeFleetDominance_DefaultTierDecoupledFromKickstartAndEscalation(t *testing.T) {
	// Cascade where Kickstart and Escalation run Claude Sonnet 5 (CodingIndex 82.4),
	// but the mainline workhorse is Gemini 3.8 Flash (76.3), and Default Catch-All is Gemini 3.8 Flash (76.3).
	// DefaultTier should NOT be flagged as inferior to Kickstart or Escalation.
	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:          "Kickstart Escalation",
				Model:             "anthropic/claude-sonnet-5",
				CodingIndex:       82.4,
				RequiresKickstart: true,
				RetryBound:        3,
			},
			{
				TierName:       "Multimodal Vision",
				Model:          "anthropic/claude-sonnet-5",
				CodingIndex:    82.4,
				RequiresImages: true,
				RetryBound:     2,
			},
			{
				TierName:    "Tier 1: Local GPU",
				Model:       "gemma4:12b-it-qat",
				CodingIndex: 64.0,
				RetryBound:  2,
			},
			{
				TierName:    "Tier 2: Workhorse",
				Model:       "google/gemini-3.8-flash",
				CodingIndex: 76.3,
				RetryBound:  5,
			},
			{
				TierName:    "Tier 3: Escalation",
				Model:       "anthropic/claude-opus-5",
				CodingIndex: 84.1,
				RetryFloor:  5,
				RetryBound:  8,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:    "Default Catch-All",
			Model:       "google/gemini-3.8-flash",
			CodingIndex: 76.3,
		},
	}

	conflicts := AnalyzeFleetDominance(&cfg)
	for _, c := range conflicts {
		if c.TierIndex == len(cfg.Tiers) {
			t.Errorf("DefaultTier should NOT have a dominance conflict against Kickstart, Vision, or Escalation, got: %s", c.Reason)
		}
	}
}
