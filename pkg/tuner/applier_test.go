package tuner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyTuning_Success_MatchingOllama(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `
port: 8000
tiers:
  - name: "Local Fast"
    provider: "ollama"
    when: "Tokens < 4000"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		SynthesizedRule: "Tokens < 8000 && !HasImages",
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

	if !containsStr(string(updated), "Tokens < 8000 && !HasImages") {
		t.Errorf("Updated config missing synthesized rule: %s", string(updated))
	}
}

func TestApplyTuning_Success_MatchingName(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `
port: 8000
tiers:
  - name: "My Local Tier"
    provider: "custom"
    when: "Tokens < 2000"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		SynthesizedRule: "Tokens < 5000",
	}

	_, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("ApplyTuning failed: %v", err)
	}

	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	if !containsStr(string(updated), "Tokens < 5000") {
		t.Errorf("Updated config missing synthesized rule: %s", string(updated))
	}
}

func TestApplyTuning_FallbackFirstTier(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `
port: 8000
tiers:
  - name: "Cloud Only"
    provider: "openrouter"
    when: "true"
`
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result := &TuningResult{
		SynthesizedRule: "Tokens < 12000",
	}

	_, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("ApplyTuning failed: %v", err)
	}

	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	if !containsStr(string(updated), "Tokens < 12000") {
		t.Errorf("Updated config missing synthesized rule: %s", string(updated))
	}
}

func TestApplyTuning_DefaultPath(t *testing.T) {
	// Calling with empty path should look for config.yaml in cwd
	_, err := ApplyTuning("", &TuningResult{SynthesizedRule: "Tokens < 1000"})
	// It will either succeed if config.yaml exists in cwd or fail cleanly
	if err != nil && !containsStr(err.Error(), "config.yaml") {
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

	result := &TuningResult{SynthesizedRule: "Tokens < 1000"}
	_, err := ApplyTuning(cfgPath, result)
	if err != nil {
		t.Fatalf("ApplyTuning failed on empty tiers: %v", err)
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

func TestApplyTuning_RenameDirectoryFailure(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("port: 8000\n"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Create a directory at config.yaml path during swap
	// We can test rename failure by passing a path that cannot be overwritten as a file
	dirAsFile := filepath.Join(tmpDir, "directory_as_file")
	if err := os.MkdirAll(filepath.Join(dirAsFile, "nested"), 0750); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	_, err := ApplyTuning(dirAsFile, &TuningResult{})
	if err == nil {
		t.Fatalf("Expected error when target is a directory, got nil")
	}
}

func TestApplyTuning_EmptyConfigPath(t *testing.T) {
	_, err := ApplyTuning("", &TuningResult{})
	if err == nil {
		t.Fatalf("Expected error when default config.yaml is missing, got nil")
	}
}


func containsStr(s, substr string) bool {
	return filepath.Base(s) != "" && len(s) >= len(substr) && (s == substr || (len(s) > 0 && len(substr) > 0 && searchSubstr(s, substr)))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
