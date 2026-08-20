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
		SynthesizedRule:     "Tokens < 12500 && !HasImages && !HasTools && !any(Keywords, { # in ['sql', 'migration'] })",
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
	if !strings.Contains(report, "- when: \"Tokens < 16000 && !HasImages && !HasTools\"") {
		t.Errorf("Expected report to contain original when rule in diff")
	}
	if !strings.Contains(report, "+ when: \"Tokens < 12500") {
		t.Errorf("Expected report to contain synthesized when rule in diff")
	}
}

// Test 3.2: Atomic apply updates config and creates backup
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

// Test 3.3: ParetoBanditOptimizer constructor, name, and empty records
func TestOptimizer_ConstructorAndEmptyRecords(t *testing.T) {
	opt := NewParetoBanditOptimizer()
	if opt.Name() != "pareto_bandit" {
		t.Errorf("Expected name 'pareto_bandit', got '%s'", opt.Name())
	}

	res, err := opt.Optimize(nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error on empty records: %v", err)
	}
	if res.OptimalThreshold != 16000 {
		t.Errorf("Expected default 16000 threshold on empty records, got %d", res.OptimalThreshold)
	}
}

// Test 3.4: ApplyTuning error handling on missing file and malformed YAML
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
