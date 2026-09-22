package tuner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// Test 3.1: Advisory report generation contains key diff and projections
func TestAdvisor_GeneratesReport(t *testing.T) {
	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:         "Local ROCm GPU",
				OptimalThreshold: 12500,
				OptimalRetries:   2,
				FrictionKeywords: []string{"sql", "migration"},
				RestrictImages:   false,
				RestrictTools:    false,
				OriginalModel:    "qwen2.5-coder:14b",
				RecommendedModel: "qwen3-coder-plus",
				ModelBenefit:     "SWE-bench +15%",
				OriginalRule:     "Tokens < 16000 && !HasImages && !HasTools",
				SynthesizedRule:  "Tokens < 12500 && !any(Keywords, { # in ['migration', 'sql'] })",
			},
			{
				TierName:         "Unlimited Tier",
				OptimalThreshold: 0,
				OptimalRetries:   0,
				RestrictImages:   true,
				RestrictTools:    true,
				OriginalRule:     "true",
				SynthesizedRule:  "true",
			},
		},
		DefaultTier: &TierTuningResult{
			TierName:         "Fallback Catch-All",
			OriginalModel:    "z-ai/glm-5.3-flash",
			RecommendedModel: "google/gemini-3.8-flash",
			ModelBenefit:     "Capability parity",
		},
		StaticDominanceConflicts: []StaticDominanceConflict{
			{
				TierIndex: 1,
				TierName:  "Local ROCm GPU",
				Reason:    "Escalation tier is strictly inferior to predecessor tier",
			},
		},
		RecoveryStats: map[string]RecoveryStats{
			"qwen2.5-coder:14b": {
				Model:             "qwen2.5-coder:14b",
				SelfRecoveryRate:  0.75,
				AvgTurnsToRecover: 1.8,
			},
		},
		TotalSessions:       10,
		AvgTurnsPerSession:  500.0,
		EscalationRate:      0.25,
		CurrentCostUSD:      45.00,
		ProjectedCostUSD:    48.20,
		ProjectedSavingsUSD: 14.50,
		RetriesEliminated:   340,
		TotalSampleTurns:    5000,
	}

	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {BaseURL: "http://127.0.0.1:11434", Type: contract.ProviderTypeLocal},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local ROCm GPU",
				Model:    "qwen2.5-coder:14b",
				Provider: "ollama",
				When:     "Tokens < 16000 && !HasImages && !HasTools",
			},
		},
	}

	report := GenerateAdvisoryReport(result, cfg)
	if report == "" {
		t.Fatalf("Expected non-empty advisory report")
	}

	expectedSnippets := []string{
		"NACHO FLOW ADVISORY TUNING REPORT",
		"5000 historical prompt turns evaluated",
		"12500 tokens",
		"~340 retries eliminated",
		"$14.50 USD saved",
		"Tokens < 12500 && !any(Keywords, { # in ['migration', 'sql'] })",
		"STATIC ROUTING DOMINANCE DEFECTS DETECTED",
		"MODEL SELF-RECOVERY ANALYSIS",
		"Fallback Catch-All",
		"Context Threshold:   Unlimited",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(report, snippet) {
			t.Errorf("Expected report to contain snippet %q, but it was missing", snippet)
		}
	}
}

// Test 3.2: Advisory report with friction modalities and non-local config
func TestAdvisor_FrictionModalities(t *testing.T) {
	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:         "Tier 1: Local GPU",
				OptimalThreshold: 8000,
				RestrictImages:   true,
				RestrictTools:    true,
				SynthesizedRule:  "Tokens < 8000 && !HasImages && !HasTools",
			},
		},
		TotalSampleTurns:    200,
		RetriesEliminated:   12,
		ProjectedSavingsUSD: 5.0,
	}

	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {BaseURL: "https://openrouter.ai/api/v1", Type: contract.ProviderTypeCloud},
		},
		Tiers: []contract.Tier{
			{Name: "Cloud Workhorse", Provider: "openrouter", When: "true"},
		},
	}

	report := GenerateAdvisoryReport(result, cfg)
	if !strings.Contains(report, "Multimodal Vision:   Restricted") {
		t.Errorf("Expected report to mention restricted vision, got: %s", report)
	}
	if !strings.Contains(report, "Agentic Tool Calls:  Restricted") {
		t.Errorf("Expected report to mention restricted tools, got: %s", report)
	}
}

// Test 3.3: Atomic apply updates config and creates backup
func TestApplier_AtomicBackupAndApply(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	initialYAML := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"
tiers:
  - name: "Local GPU"
    model: "qwen2.5-coder"
    provider: "ollama"
    when: "Tokens < 16000"
`
	if err := os.WriteFile(configPath, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:        "Local GPU",
				SynthesizedRule: "Tokens < 12000 && !HasTools",
			},
		},
	}

	backupPath, err := ApplyTuning(configPath, result)
	if err != nil {
		t.Fatalf("ApplyTuning failed: %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("Expected backup file to exist at %s: %v", backupPath, err)
	}

	// Verify updated config has new rule
	updatedBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	if !strings.Contains(string(updatedBytes), "Tokens < 12000 && !HasTools") {
		t.Errorf("Expected updated config to contain new rule, got: %s", string(updatedBytes))
	}
}

// Test 3.4: CostPenaltyOptimizer constructor, name, and empty records
func TestOptimizer_ConstructorAndEmptyRecords(t *testing.T) {
	opt := NewCostPenaltyOptimizer()
	if opt.Name() != "cost_penalty" {
		t.Errorf("Expected name 'cost_penalty', got '%s'", opt.Name())
	}

	res, err := opt.Optimize(nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error on empty records: %v", err)
	}
	if len(res.Tiers) == 0 || res.Tiers[0].OptimalThreshold != 16000 {
		t.Errorf("Expected default 16000 threshold on empty records")
	}
}

// Test 3.5: ApplyTuning error handling on missing file and malformed YAML
func TestApplier_ErrorCases(t *testing.T) {
	result := &TuningResult{
		Tiers: []TierTuningResult{
			{TierName: "Local GPU", SynthesizedRule: "Tokens < 10000"},
		},
	}

	// Missing config file
	_, err := ApplyTuning(filepath.Join(t.TempDir(), "missing.yaml"), result)
	if err == nil {
		t.Fatalf("Expected error for missing config file, got nil")
	}

	// Malformed YAML
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.yaml")
	if err := os.WriteFile(configPath, []byte("port: [invalid"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err = ApplyTuning(configPath, result)
	if err == nil {
		t.Fatalf("Expected error for malformed YAML in ApplyTuning, got nil")
	}
}

// Test 3.6: Advisory report with no friction keywords, zero projected savings, and nil config
func TestAdvisor_NoFrictionKeywordsAndNilConfig(t *testing.T) {
	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:         "Local ROCm GPU",
				OptimalThreshold: 16000,
				FrictionKeywords: nil,
				SynthesizedRule:  "Tokens < 16000",
			},
		},
		ProjectedSavingsUSD: 0.0,
		TotalSampleTurns:    100,
	}

	report := GenerateAdvisoryReport(result, nil)
	if !strings.Contains(report, "Local ROCm GPU") {
		t.Errorf("Expected tier name 'Local ROCm GPU'")
	}
}

// Test 3.7: ApplyTuning rejects config with no matching tiers
func TestApplier_RejectNoMatchingTier(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "cloud_only_config.yaml")

	yamlContent := `
port: 8000
providers:
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    type: "cloud"
tiers:
  - name: "Cloud Tier 1"
    model: "claude-sonnet"
    provider: "openrouter"
    when: "true"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{TierName: "Nonexistent Tier", SynthesizedRule: "Tokens < 8000"},
		},
	}
	_, err := ApplyTuning(configPath, result)
	if err == nil {
		t.Fatalf("Expected error when applying tuning with no matching tier, got nil")
	}
}

// Test 3.8: DistillRule with special character keyword triggering expr compile error
func TestDistiller_InvalidKeywordCompilationError(t *testing.T) {
	_, err := DistillRule(16000, []string{"unclosed'quote"})
	if err == nil {
		t.Errorf("Expected expr compilation error for malformed keyword quotes")
	}
}

// Test 3.9: ApplyTuning destination directory collision
func TestApplier_DestinationDirectoryCollision(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config.yaml")
	_ = os.MkdirAll(filepath.Join(configDir, "non_empty"), 0750)

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{TierName: "Local GPU", SynthesizedRule: "Tokens < 5000"},
		},
	}
	_, err := ApplyTuning(configDir, result)
	if err == nil {
		t.Errorf("Expected error applying tuning to a directory")
	}
}

func TestAdvisor_GeneratesReport_v2(t *testing.T) {
	result := &TuningResult{
		TotalSampleTurns:   847,
		TotalSessions:      23,
		AvgTurnsPerSession: 36.8,
		EscalationRate:     0.783,
		Tiers: []TierTuningResult{
			{
				TierName:         "Tier 1: Local GPU",
				OptimalThreshold: 4000,
				OptimalRetries:   1,
				SynthesizedRule:  "Tokens < 4000 && Retries < 1",
			},
		},
		CurrentCostUSD:    12.40,
		ProjectedCostUSD:  8.20,
		RetriesEliminated: 119,
		RecoveryStats: map[string]RecoveryStats{
			"gemma-4-26b": {
				Model:            "gemma-4-26b",
				SelfRecoveryRate: 0.032,
			},
			"qwen3-coder-plus": {
				Model:            "qwen3-coder-plus",
				SelfRecoveryRate: 0.671,
			},
		},
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "Tier 1: Local GPU", When: "Tokens < 16000 && Retries < 2", Provider: "ollama"},
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama": {Type: "local"},
		},
	}

	report := GenerateAdvisoryReport(result, cfg)

	if !strings.Contains(report, "v2 — Session Replay") {
		t.Errorf("Expected v2 header, got: %s", report)
	}
	if !strings.Contains(report, "SESSION DYNAMICS:") {
		t.Errorf("Expected SESSION DYNAMICS section, got: %s", report)
	}
	if !strings.Contains(report, "Average Turns per Session:") {
		t.Errorf("Expected Average Turns per Session, got: %s", report)
	}
	if !strings.Contains(report, "MODEL SELF-RECOVERY ANALYSIS:") {
		t.Errorf("Expected MODEL SELF-RECOVERY ANALYSIS section, got: %s", report)
	}
	if !strings.Contains(report, "Retry Bound:") {
		t.Errorf("Expected Retry Bound metric, got: %s", report)
	}
}
