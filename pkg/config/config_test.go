package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
port: 9090
openrouter_key: "test-key"
providers:
  ollama: "http://localhost:11434/v1"
  openrouter: "https://openrouter.ai/api/v1"
tiers:
  - name: "Local"
    model: "qwen2.5-coder:14b"
    provider: "ollama"
    when: "Tokens < 1000"
default_tier:
  name: "Fallback"
  model: "deepseek/deepseek-chat"
  provider: "openrouter"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Expected port 9090, got %d", cfg.Port)
	}

	if cfg.OpenRouterKey != "test-key" {
		t.Errorf("Expected openrouter_key 'test-key', got %s", cfg.OpenRouterKey)
	}

	if len(cfg.Tiers) != 1 {
		t.Errorf("Expected 1 tier, got %d", len(cfg.Tiers))
	}

	if cfg.DefaultTier.Model != "deepseek/deepseek-chat" {
		t.Errorf("Expected default model 'deepseek/deepseek-chat', got %s", cfg.DefaultTier.Model)
	}
}
