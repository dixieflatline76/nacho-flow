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

func TestApplyTuning_ModelSubstitutionAndFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	initialYAML := `
port: 8000
tiers:
  - name: "Tier 1: Fast"
    model: "qwen2.5-coder:7b"
    when: "Tokens < 8000"
default_tier:
  name: "Fallback"
  model: "anthropic/claude-sonnet-5"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:         "Tier 1: Fast",
				OriginalModel:    "qwen2.5-coder:7b",
				RecommendedModel: "qwen2.5-coder:14b",
				SynthesizedRule:  "Tokens < 12000",
			},
		},
		DefaultTier: &TierTuningResult{
			TierName:         "Fallback",
			OriginalModel:    "anthropic/claude-sonnet-5",
			RecommendedModel: "z-ai/glm-5.3-flash",
		},
	}

	_, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("ApplyTuning failed: %v", err)
	}

	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	s := string(updated)
	if !strings.Contains(s, "model: qwen2.5-coder:14b") {
		t.Errorf("expected Tier 1 model substitution to qwen2.5-coder:14b, got: %s", s)
	}
	if !strings.Contains(s, "Tokens < 12000") {
		t.Errorf("expected synthesized rule Tokens < 12000, got: %s", s)
	}
	if !strings.Contains(s, "model: z-ai/glm-5.3-flash") {
		t.Errorf("expected fallback tier model substitution to z-ai/glm-5.3-flash, got: %s", s)
	}
}

func TestApplyTuning_ErrorBranches(t *testing.T) {
	// 1. Nonexistent file
	_, err := ApplyTuning("nonexistent-config-file-12345.yaml", &TuningResult{})
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}

	// 2. Invalid YAML
	tmpDir := t.TempDir()
	badYAML := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(badYAML, []byte(":::invalid yaml:::"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	_, err = ApplyTuning(badYAML, &TuningResult{})
	if err == nil {
		t.Errorf("expected error for invalid YAML")
	}

	// 3. Result with empty rules/models
	validYAML := filepath.Join(tmpDir, "valid.yaml")
	if err := os.WriteFile(validYAML, []byte("port: 8000\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	emptyResult := &TuningResult{
		Tiers: []TierTuningResult{
			{TierName: "Empty Tier"},
		},
	}
	_, err = ApplyTuning(validYAML, emptyResult)
	if err == nil {
		t.Errorf("expected error when no tiers matched")
	}
}

func TestApplyTuning_SingleLocalTierFallback(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	initialYAML := `port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
    type: "local"
tiers:
  - name: "Existing Local Tier"
    provider: "ollama"
    model: "qwen2.5-coder:7b"
    when: "Tokens < 4000"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		Tiers: []TierTuningResult{
			{
				TierName:         "Generic Local",
				RecommendedModel: "qwen2.5-coder:14b",
				OriginalModel:    "qwen2.5-coder:7b",
				SynthesizedRule:  "Tokens < 8000",
			},
		},
	}
	_, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
