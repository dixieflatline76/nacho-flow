package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test 1.1: Structured providers parsing
func TestConfig_StructuredProviders_Success(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
port: 9000
providers:
  local_gpu:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    api_key: "sk-or-test-key"
    headers:
      HTTP-Referer: "https://spicebox.dev"
      X-Title: "nacho-flow"
  langdock:
    base_url: "https://api.langdock.com/v1"
    api_key: "sk-langdock-test"
    headers:
      X-Custom-Org: "engineering"
tiers:
  - name: "Local Fast"
    model: "qwen2.5-coder:14b"
    provider: "local_gpu"
    when: "Tokens < 8000"
default_tier:
  name: "Cloud Fallback"
  model: "deepseek/deepseek-chat"
  provider: "openrouter"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.Port)
	}

	// Verify local provider
	local, ok := cfg.Providers["local_gpu"]
	if !ok {
		t.Fatalf("Missing local_gpu provider")
	}
	if local.BaseURL != "http://127.0.0.1:11434/v1" || local.Type != "local" {
		t.Errorf("Unexpected local_gpu config: %+v", local)
	}

	// Verify OpenRouter provider
	or, ok := cfg.Providers["openrouter"]
	if !ok {
		t.Fatalf("Missing openrouter provider")
	}
	if or.BaseURL != "https://openrouter.ai/api/v1" || or.APIKey != "sk-or-test-key" {
		t.Errorf("Unexpected openrouter config: %+v", or)
	}
	if or.Headers["HTTP-Referer"] != "https://spicebox.dev" {
		t.Errorf("Missing HTTP-Referer header in openrouter config")
	}

	// Verify Langdock provider
	ld, ok := cfg.Providers["langdock"]
	if !ok {
		t.Fatalf("Missing langdock provider")
	}
	if ld.BaseURL != "https://api.langdock.com/v1" || ld.APIKey != "sk-langdock-test" {
		t.Errorf("Unexpected langdock config: %+v", ld)
	}
	if ld.Headers["X-Custom-Org"] != "engineering" {
		t.Errorf("Missing X-Custom-Org header in langdock config")
	}
}

// Test 1.2: Environment variable resolution for API keys
func TestConfig_EnvKeyResolution(t *testing.T) {
	os.Setenv("TEST_LANGDOCK_API_SECRET", "resolved-secret-value-123")
	defer os.Unsetenv("TEST_LANGDOCK_API_SECRET")

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
port: 8000
providers:
  langdock:
    base_url: "https://api.langdock.com/v1"
    api_key: "ENV_TEST_LANGDOCK_API_SECRET"
tiers:
  - name: "Langdock Tier"
    model: "claude-3-5-sonnet"
    provider: "langdock"
    when: "true"
default_tier:
  name: "Langdock Fallback"
  model: "claude-3-5-sonnet"
  provider: "langdock"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	ld := cfg.Providers["langdock"]
	if ld.APIKey != "resolved-secret-value-123" {
		t.Errorf("Expected resolved APIKey 'resolved-secret-value-123', got '%s'", ld.APIKey)
	}
}

// Test 1.3: Validation fails if provider is missing base_url
func TestConfig_Validation_MissingBaseURL(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
port: 8000
providers:
  broken_provider:
    api_key: "secret"
tiers:
  - name: "Broken Tier"
    model: "test-model"
    provider: "broken_provider"
    when: "true"
default_tier:
  name: "Fallback"
  model: "test-model"
  provider: "broken_provider"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatalf("Expected error for missing base_url, got nil")
	}
	if !strings.Contains(err.Error(), "base_url") {
		t.Errorf("Expected error to mention base_url, got: %v", err)
	}
}

// Test 1.4: Validation fails if tier references an unknown provider
func TestConfig_Validation_TierReferencingUnknownProvider(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434/v1"
tiers:
  - name: "Ghost Tier"
    model: "claude-3-5-sonnet"
    provider: "non_existent_provider"
    when: "Tokens > 1000"
default_tier:
  name: "Fallback"
  model: "qwen2.5"
  provider: "ollama"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatalf("Expected error for tier referencing non_existent_provider, got nil")
	}
	if !strings.Contains(err.Error(), "non_existent_provider") {
		t.Errorf("Expected error to mention non_existent_provider, got: %v", err)
	}
}

// Test 1.5: AuthToken parsing and environment variable substitution
func TestConfig_AuthTokenResolution(t *testing.T) {
	t.Setenv("SECRET_GATEWAY_TOKEN", "sk-custom-secret-12345")
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
port: 8080
auth_token: "ENV_SECRET_GATEWAY_TOKEN"
providers:
  ollama:
    base_url: "http://127.0.0.1:11434/v1"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.AuthToken != "sk-custom-secret-12345" {
		t.Errorf("Expected AuthToken 'sk-custom-secret-12345', got '%s'", cfg.AuthToken)
	}
}

// Test 1.6: NACHO_AUTH_TOKEN global fallback when auth_token is unset
func TestConfig_AuthTokenEnvFallback(t *testing.T) {
	t.Setenv("NACHO_AUTH_TOKEN", "sk-fallback-env-token")
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
port: 8080
providers:
  ollama:
    base_url: "http://127.0.0.1:11434/v1"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.AuthToken != "sk-fallback-env-token" {
		t.Errorf("Expected AuthToken 'sk-fallback-env-token', got '%s'", cfg.AuthToken)
	}
}
