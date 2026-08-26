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
		OptimalThreshold:    12500,
		FrictionKeywords:    []string{"sql", "migration"},
		RestrictImages:      false,
		RestrictTools:       false,
		TargetTierName:      "Local ROCm GPU",
		SynthesizedRule:     "Tokens < 12500 && !any(Keywords, { # in ['migration', 'sql'] })",
		CurrentCostUSD:      45.00,
		ProjectedCostUSD:    48.20,
		ProjectedSavingsUSD: 14.50,
		RetriesEliminated:   340,
		TotalSampleTurns:    5000,
	}

	cfg := &contract.Config{
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

	if !strings.Contains(report, "12500 tokens") {
		t.Errorf("Expected report to mention 12500 tokens")
	}
	if !strings.Contains(report, "340 retries eliminated") {
		t.Errorf("Expected report to mention retries eliminated")
	}
	if !strings.Contains(report, "Clean (0% retry rate — enabled locally)") {
		t.Errorf("Expected report to mention clean modalities")
	}
	if !strings.Contains(report, "- when: \"Tokens < 16000 && !HasImages && !HasTools\"") {
		t.Errorf("Expected report to contain original when rule in diff")
	}
	if !strings.Contains(report, "+ when: \"Tokens < 12500") {
		t.Errorf("Expected report to contain synthesized when rule in diff")
	}
}

// Test 3.2: Advisory report with friction modalities and non-local config
func TestAdvisor_FrictionModalities(t *testing.T) {
	result := &TuningResult{
		OptimalThreshold:    8000,
		RestrictImages:      true,
		RestrictTools:       true,
		TargetTierName:      "",
		SynthesizedRule:     "Tokens < 8000 && !HasImages && !HasTools",
		TotalSampleTurns:    200,
		RetriesEliminated:   12,
		ProjectedSavingsUSD: 5.0,
	}

	// Config with only cloud tiers (tests fallback oldRule branch)
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "Cloud Workhorse", Provider: "openrouter", When: "true"},
		},
	}

	report := GenerateAdvisoryReport(result, cfg)
	if !strings.Contains(report, "Multimodal Vision:              High Friction") {
		t.Errorf("Expected report to mention high friction vision")
	}
	if !strings.Contains(report, "Agentic Tool Calls:             High Friction") {
		t.Errorf("Expected report to mention high friction tools")
	}
	if !strings.Contains(report, "Local ROCm GPU") {
		t.Errorf("Expected fallback tier name")
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
		SynthesizedRule: "Tokens < 12000 && !HasTools",
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
	if res.OptimalThreshold != 16000 {
		t.Errorf("Expected default 16000 threshold on empty records, got %d", res.OptimalThreshold)
	}
}

// Test 3.5: ApplyTuning error handling on missing file and malformed YAML
func TestApplier_ErrorCases(t *testing.T) {
	result := &TuningResult{SynthesizedRule: "Tokens < 10000"}

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
		OptimalThreshold:    16000,
		FrictionKeywords:    nil,
		TargetTierName:      "Local ROCm GPU",
		SynthesizedRule:     "Tokens < 16000",
		ProjectedSavingsUSD: 0.0,
		TotalSampleTurns:    100,
	}

	report := GenerateAdvisoryReport(result, nil)
	if !strings.Contains(report, "None (Clean token progression across all domains)") {
		t.Errorf("Expected report to mention clean token progression")
	}
	if !strings.Contains(report, "Local ROCm GPU") {
		t.Errorf("Expected fallback tier name 'Local ROCm GPU' when config is nil")
	}
}

// Test 3.7: ApplyTuning rejects config with only cloud tiers
func TestApplier_RejectCloudOnly(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "cloud_only_config.yaml")

	yamlContent := `
port: 8000
tiers:
  - name: "Cloud Tier 1"
    model: "claude-sonnet"
    provider: "openrouter"
    when: "true"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	result := &TuningResult{SynthesizedRule: "Tokens < 8000"}
	_, err := ApplyTuning(configPath, result)
	if err == nil {
		t.Fatalf("Expected error when applying tuning to cloud-only config, got nil")
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

	result := &TuningResult{SynthesizedRule: "Tokens < 5000"}
	_, err := ApplyTuning(configDir, result)
	if err == nil {
		t.Errorf("Expected error applying tuning to a directory")
	}
}
