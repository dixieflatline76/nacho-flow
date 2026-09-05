package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
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
    type: "cloud"
    headers:
      HTTP-Referer: "https://spicebox.dev"
      X-Title: "nacho-flow"
  langdock:
    base_url: "https://api.langdock.com/v1"
    api_key: "sk-langdock-test"
    type: "cloud"
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
    type: "cloud"
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
    type: "local"
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
    type: "local"
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
    type: "local"
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

// Test 1.7: Non-existent config path returns error
func TestConfig_NonExistentPath_ReturnsError(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatalf("Expected error for non-existent config path, got nil")
	}
	if !strings.Contains(err.Error(), "could not find") {
		t.Errorf("Expected error to mention could not find, got: %v", err)
	}
}

// Test 1.8: Malformed YAML returns parse error
func TestConfig_MalformedYAML_ReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("port: [invalid yaml syntax"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatalf("Expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse YAML config") {
		t.Errorf("Expected error to mention failed to parse YAML config, got: %v", err)
	}
}

// Test 1.9: Empty providers map returns error
func TestConfig_EmptyProviders_ReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("port: 8000\nproviders: {}\n"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatalf("Expected error for empty providers, got nil")
	}
	if !strings.Contains(err.Error(), "at least one provider must be defined") {
		t.Errorf("Expected error to mention at least one provider must be defined, got: %v", err)
	}
}

// Test 1.10: Default tier referencing unknown provider returns error
func TestConfig_DefaultTierUnknownProvider_ReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
port: 8000
providers:
  local:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"
default_tier:
  name: "Fallback"
  model: "qwen2.5"
  provider: "unknown_fallback_provider"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatalf("Expected error for default_tier unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "default_tier references unknown provider") {
		t.Errorf("Expected error to mention default_tier references unknown provider, got: %v", err)
	}
}

// Test 1.11: Default port 8000 assigned when port is omitted
func TestConfig_DefaultPortAssignment(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
providers:
  local:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Port != 8000 {
		t.Errorf("Expected default port 8000, got %d", cfg.Port)
	}
}

// Test 1.12: Auto-bootstrap creates default starter config when no config exists
func TestConfig_AutoBootstrap_CleanEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("NACHO_CONFIG_DIR", tempDir)
	t.Setenv("APPDATA", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	// Change working dir to empty temp dir so local ./config.yaml doesn't exist
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig(\"\") failed on clean environment: %v", err)
	}

	if cfg == nil {
		t.Fatalf("Expected non-nil config")
	}

	if cfg.Port != 8000 {
		t.Errorf("Expected default port 8000, got %d", cfg.Port)
	}

	// Verify standard providers
	if _, ok := cfg.Providers["ollama"]; !ok {
		t.Errorf("Expected 'ollama' provider in starter config")
	}
	if _, ok := cfg.Providers["openrouter"]; !ok {
		t.Errorf("Expected 'openrouter' provider in starter config")
	}

	// Verify tiers
	if len(cfg.Tiers) < 3 {
		t.Errorf("Expected at least 3 tiers in starter config, got %d", len(cfg.Tiers))
	}

	// Verify default tier
	if cfg.DefaultTier.Provider != "openrouter" {
		t.Errorf("Expected default tier provider 'openrouter', got %s", cfg.DefaultTier.Provider)
	}
}

// Test 1.13: Auto-bootstrap does not overwrite existing configuration
func TestConfig_AutoBootstrap_ExistingConfigUntouched(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("NACHO_CONFIG_DIR", tempDir)
	t.Setenv("APPDATA", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	userConfig, err := contract.GetUserConfigDir()
	if err != nil {
		t.Fatalf("GetUserConfigDir failed: %v", err)
	}

	configDir := filepath.Join(userConfig, "nacho-flow")
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	customContent := `
port: 9876
providers:
  custom_prov:
    base_url: "http://127.0.0.1:5000/v1"
    type: "local"
default_tier:
  name: "Custom Fallback"
  model: "custom-model"
  provider: "custom_prov"
`
	if err := os.WriteFile(configPath, []byte(customContent), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Port != 9876 {
		t.Errorf("Expected custom port 9876, got %d", cfg.Port)
	}

	// Verify file content was not overwritten
	readBack, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(readBack), "custom_prov") {
		t.Errorf("Existing config was overwritten!")
	}
}

// Test 1.14: Explicit custom path missing returns strict error
func TestConfig_ExplicitCustomPath_MissingFails(t *testing.T) {
	tempDir := t.TempDir()
	nonExistent := filepath.Join(tempDir, "missing", "custom-config.yaml")

	_, err := LoadConfig(nonExistent)
	if err == nil {
		t.Fatalf("Expected error for missing explicit custom config, got nil")
	}
	if !strings.Contains(err.Error(), "could not find") {
		t.Errorf("Expected 'could not find' in error, got: %v", err)
	}

	// Ensure file was not created
	if _, statErr := os.Stat(nonExistent); !os.IsNotExist(statErr) {
		t.Errorf("Missing custom config should not have been created on disk")
	}
}

// Test 1.15: NACHO_CONFIG_DIR override directs config discovery to custom directory
func TestConfig_NachoConfigDirOverride(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("NACHO_CONFIG_DIR", tempDir)

	configDir := filepath.Join(tempDir, contract.AppName)
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	configPath := filepath.Join(configDir, contract.DefaultConfigFileName)
	customContent := `
port: 9123
providers:
  test_prov:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"
default_tier:
  name: "Fallback"
  model: "test-model"
  provider: "test_prov"
`
	if err := os.WriteFile(configPath, []byte(customContent), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Change working dir to empty temp dir so local ./config.yaml doesn't exist
	emptyDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(emptyDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Port != 9123 {
		t.Errorf("Expected port 9123 loaded from NACHO_CONFIG_DIR, got %d", cfg.Port)
	}
}

