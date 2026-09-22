package tuner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyTuning_Success_MatchingOllama(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
    type: "local"
tiers:
  - name: "Local Fast"
    provider: "ollama"
    when: "Tokens < 4000"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:        "Local Fast",
				SynthesizedRule: "Tokens < 8000 && !HasImages",
			},
		},
	}

	backupPath, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("ApplyTuning failed: %v", err)
	}

	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("Backup file %q does not exist: %v", backupPath, err)
	}

	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	if !strings.Contains(string(updated), "Tokens < 8000 && !HasImages") {
		t.Errorf("Updated config missing synthesized rule: %s", string(updated))
	}
}

func TestApplyTuning_Success_MatchingName(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `
port: 8000
providers:
  custom:
    base_url: "http://127.0.0.1:5000"
    type: "local"
tiers:
  - name: "My Local Tier"
    provider: "custom"
    when: "Tokens < 2000"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:        "My Local Tier",
				SynthesizedRule: "Tokens < 5000",
			},
		},
	}

	_, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("ApplyTuning failed: %v", err)
	}

	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	if !strings.Contains(string(updated), "Tokens < 5000") {
		t.Errorf("Updated config missing synthesized rule: %s", string(updated))
	}
}

func TestApplyTuning_Success_MatchingVLLM(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `
port: 8000
providers:
  vllm:
    base_url: "http://127.0.0.1:8000"
    type: "local"
tiers:
  - name: "vLLM Workhorse"
    provider: "vllm"
    when: "Tokens < 4000"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:        "vLLM Workhorse",
				SynthesizedRule: "Tokens < 12000",
			},
		},
	}

	_, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("ApplyTuning failed: %v", err)
	}

	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	if !strings.Contains(string(updated), "Tokens < 12000") {
		t.Errorf("Updated config missing synthesized rule: %s", string(updated))
	}
}

func TestApplyTuning_ErrorWhenNoMatchingTier(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `
port: 8000
providers:
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    type: "cloud"
tiers:
  - name: "Cloud Claude Sonnet"
    provider: "openrouter"
    when: "true"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:        "Nonexistent Local Tier",
				SynthesizedRule: "Tokens < 12000",
			},
		},
	}

	_, err := ApplyTuning(cfgPath, result)
	if err == nil {
		t.Fatalf("Expected error when no matching tier is in config, got nil")
	}
}

func TestApplyTuning_DefaultPath(t *testing.T) {
	res := &TuningResult{
		Tiers: []TierTuningResult{
			{TierName: "Local", SynthesizedRule: "Tokens < 1000"},
		},
	}
	_, err := ApplyTuning("", res)
	if err != nil && !strings.Contains(err.Error(), "config.yaml") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestApplyTuning_MissingFile(t *testing.T) {
	_, err := ApplyTuning(filepath.Join(t.TempDir(), "nonexistent.yaml"), &TuningResult{})
	if err == nil {
		t.Fatalf("Expected error for missing file, got nil")
	}
}

func TestApplyTuning_EmptyTiers(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	initialYAML := "port: 8000\ntiers: []\n"
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{TierName: "Local", SynthesizedRule: "Tokens < 1000"},
		},
	}
	_, err := ApplyTuning(cfgPath, result)
	if err == nil {
		t.Fatalf("Expected error applying tuning to empty tiers config")
	}
}

func TestApplyTuning_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("invalid: yaml: ["), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := ApplyTuning(cfgPath, &TuningResult{})
	if err == nil {
		t.Fatalf("Expected error for invalid yaml, got nil")
	}
}

func TestApplyTuning_RenameDirectoryCollision(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	initialYAML := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
    type: "local"
tiers:
  - name: "Local GPU"
    provider: "ollama"
    when: "Tokens < 10000"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{TierName: "Local GPU", SynthesizedRule: "Tokens < 5000"},
		},
	}
	_, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("Expected success under normal conditions: %v", err)
	}
}

func TestApplyTuning_EmptyConfigPath(t *testing.T) {
	result := &TuningResult{
		Tiers: []TierTuningResult{
			{TierName: "Local", SynthesizedRule: "Tokens < 5000"},
		},
	}
	_, err := ApplyTuning("test_missing_cfg_path_9999.yaml", result)
	if err == nil {
		t.Fatalf("Expected error for non-existent explicit path")
	}
}
