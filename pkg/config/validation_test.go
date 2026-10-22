// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package config_test

import (
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/config"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
			},
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
				Type:    provider.TypeOpenRouter,
			},
			"anthropic": {
				BaseURL: "https://api.anthropic.com",
				Type:    provider.TypeAnthropic,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local Tier",
				Model:    "llama-3-8b",
				Provider: "ollama",
			},
			{
				Name:     "Cloud Tier",
				Model:    "anthropic/claude-sonnet-5",
				Provider: "openrouter",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Model:    "anthropic/claude-sonnet-5",
			Provider: "openrouter",
		},
	}

	if err := config.ValidateConfig(cfg); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidateConfig_NilConfig(t *testing.T) {
	err := config.ValidateConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil config, got nil")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateConfig_NoProviders(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{},
	}
	err := config.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for empty providers, got nil")
	}
	if !strings.Contains(err.Error(), "at least one provider") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateConfig_ProviderTable(t *testing.T) {
	tests := []struct {
		name        string
		providerID  string
		cfg         contract.ProviderConfig
		expectedErr string
	}{
		{
			name:       "type local removed",
			providerID: "ollama",
			cfg: contract.ProviderConfig{
				Type:    "local",
				BaseURL: "http://127.0.0.1:11434",
			},
			expectedErr: `provider "ollama": type "local" was removed; use type: openai_compatible with local: true`,
		},
		{
			name:       "type cloud removed",
			providerID: "openrouter",
			cfg: contract.ProviderConfig{
				Type:    "cloud",
				BaseURL: "https://openrouter.ai/api/v1",
			},
			expectedErr: `provider "openrouter": type "cloud" was removed; use type: openai_compatible, or type: openrouter for OpenRouter`,
		},
		{
			name:       "missing type",
			providerID: "test_prov",
			cfg: contract.ProviderConfig{
				BaseURL: "http://127.0.0.1:8000",
			},
			expectedErr: `provider "test_prov": type is required and must be one of openai_compatible, anthropic, openrouter`,
		},
		{
			name:       "unknown type",
			providerID: "test_prov",
			cfg: contract.ProviderConfig{
				Type:    "fast",
				BaseURL: "http://127.0.0.1:8000",
			},
			expectedErr: `provider "test_prov": type is required and must be one of openai_compatible, anthropic, openrouter`,
		},
		{
			name:       "openai_compatible without base_url",
			providerID: "custom",
			cfg: contract.ProviderConfig{
				Type: provider.TypeOpenAICompatible,
			},
			expectedErr: `provider "custom": base_url is required for type openai_compatible`,
		},
		{
			name:       "local true on anthropic",
			providerID: "anthropic",
			cfg: contract.ProviderConfig{
				Type:  provider.TypeAnthropic,
				Local: true,
			},
			expectedErr: `provider "anthropic": local is only valid for type openai_compatible`,
		},
		{
			name:       "local true on openrouter",
			providerID: "openrouter",
			cfg: contract.ProviderConfig{
				Type:  provider.TypeOpenRouter,
				Local: true,
			},
			expectedErr: `provider "openrouter": local is only valid for type openai_compatible`,
		},
		{
			name:       "base_url not absolute http(s) URL",
			providerID: "custom",
			cfg: contract.ProviderConfig{
				Type:    provider.TypeOpenAICompatible,
				BaseURL: "invalid-url",
			},
			expectedErr: `provider "custom": base_url must be an absolute http(s) URL`,
		},
		{
			name:       "base_url non-http scheme",
			providerID: "custom",
			cfg: contract.ProviderConfig{
				Type:    provider.TypeOpenAICompatible,
				BaseURL: "ftp://example.com/v1",
			},
			expectedErr: `provider "custom": base_url must be an absolute http(s) URL`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &contract.Config{
				Providers: map[string]contract.ProviderConfig{
					tc.providerID: tc.cfg,
				},
				Tiers: []contract.Tier{
					{
						Name:     "T1",
						Model:    "m1",
						Provider: tc.providerID,
					},
				},
				DefaultTier: contract.Tier{
					Name:     "Default",
					Model:    "m1",
					Provider: tc.providerID,
				},
			}
			err := config.ValidateConfig(cfg)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.expectedErr)
			}
			if err.Error() != tc.expectedErr {
				t.Errorf("error = %q, want exact %q", err.Error(), tc.expectedErr)
			}
		})
	}
}

func TestValidateConfig_UnknownTierProvider(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Unknown Tier",
				Model:    "llama-3-8b",
				Provider: "vllm-nonexistent",
			},
		},
	}
	err := config.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown tier provider, got nil")
	}
	if !strings.Contains(err.Error(), "references unknown provider 'vllm-nonexistent'") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateConfig_UnknownDefaultTierProvider(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Model:    "anthropic/claude-sonnet-5",
			Provider: "cloud-ghost",
		},
	}
	err := config.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown default tier provider, got nil")
	}
	if !strings.Contains(err.Error(), "default_tier references unknown provider 'cloud-ghost'") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateConfig_UnknownFairyDustProvider(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Model:    "llama-3-8b",
			Provider: "ollama",
		},
		FairyDust: contract.FairyDustConfig{
			Entries: []contract.FairyDustEntry{
				{
					Name:     "Audit",
					Model:    "test-model",
					Provider: "unknown-provider",
				},
			},
		},
	}
	err := config.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown fairy dust provider, got nil")
	}
	if !strings.Contains(err.Error(), "fairy_dust entry 'Audit' references unknown provider 'unknown-provider'") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateRawYAML_LegacyKeys(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "legacy kickstart.threshold",
			yaml: `
kickstart:
  threshold: 5
`,
			wantErr: "Config uses removed key 'kickstart.threshold'; move it to 'kickstart.read_slap.threshold'. See CHANGELOG v1.5",
		},
		{
			name: "legacy kickstart.enabled",
			yaml: `
kickstart:
  enabled: true
`,
			wantErr: "Config uses removed key 'kickstart.enabled'; move it to 'kickstart.read_slap.enabled'. See CHANGELOG v1.5",
		},
		{
			name: "legacy kickstart.max_count",
			yaml: `
kickstart:
  max_count: 10
`,
			wantErr: "Config uses removed key 'kickstart.max_count'; move it to 'kickstart.read_slap.max_slaps'. See CHANGELOG v1.5",
		},
		{
			name: "legacy cycle_killer.kickstart_threshold",
			yaml: `
cycle_killer:
  kickstart_threshold: 5
`,
			wantErr: "Config uses removed key 'cycle_killer.kickstart_threshold'; kickstart fields have moved to 'kickstart.read_slap.*'. See CHANGELOG v1.5",
		},
		{
			name: "legacy cycle_breaker.write_tools",
			yaml: `
cycle_breaker:
  write_tools:
    - write_to_file
`,
			wantErr: "Config uses removed key 'cycle_breaker.write_tools'; kickstart fields have moved to 'kickstart.read_slap.*'. See CHANGELOG v1.5",
		},
		{
			name: "legacy DSL SessionKickstarted in tier when",
			yaml: `
tiers:
  - name: "Kickstart Escalation"
    when: "SessionKickstarted && Retries < 3"
`,
			wantErr: "tier 'Kickstart Escalation' when expression uses removed DSL variable 'SessionKickstarted'; rename to 'SessionReadSlapped'. See CHANGELOG v1.5",
		},
		{
			name: "legacy DSL SessionKickstartCount in tier when",
			yaml: `
tiers:
  - name: "Kickstart Escalation"
    when: "SessionKickstartCount > 2"
`,
			wantErr: "tier 'Kickstart Escalation' when expression uses removed DSL variable 'SessionKickstartCount'; rename to 'SessionReadSlapCount'. See CHANGELOG v1.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateRawYAML([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateConfig_Phase2Rules(t *testing.T) {
	enabled := true

	baseValidConfig := func() *contract.Config {
		return &contract.Config{
			Providers: map[string]contract.ProviderConfig{
				"mock": {
					BaseURL: "http://127.0.0.1:8000",
					Type:    provider.TypeOpenAICompatible,
					Local:   true,
				},
			},
			Tiers: []contract.Tier{
				{
					Name:     "Tier 1",
					Model:    "test-model",
					Provider: "mock",
				},
			},
			DefaultTier: contract.Tier{
				Name:     "Default",
				Model:    "test-model",
				Provider: "mock",
			},
		}
	}

	t.Run("invalid global thought_slap max_attempts > 3", func(t *testing.T) {
		cfg := baseValidConfig()
		cfg.Kickstart.ThoughtSlap.MaxAttempts = 4
		err := config.ValidateConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "thought_slap.max_attempts must be between 1 and 3") {
			t.Fatalf("expected max_attempts error, got: %v", err)
		}
	})

	t.Run("invalid global thought_slap rescue_tier unknown", func(t *testing.T) {
		cfg := baseValidConfig()
		cfg.Kickstart.ThoughtSlap.RescueTier = "Nonexistent"
		err := config.ValidateConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "rescue_tier references unknown tier \"Nonexistent\"") {
			t.Fatalf("expected unknown rescue_tier error, got: %v", err)
		}
	})

	t.Run("invalid tier thought_slap max_attempts > 3", func(t *testing.T) {
		cfg := baseValidConfig()
		cfg.Tiers[0].ThoughtSlap = &contract.ThoughtSlapOptIn{
			Enabled: true,
			Overrides: contract.ThoughtSlapConfig{
				MaxAttempts: 5,
			},
		}
		err := config.ValidateConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "thought_slap max_attempts must be between 1 and 3") {
			t.Fatalf("expected tier max_attempts error, got: %v", err)
		}
	})

	t.Run("invalid tier thought_slap rescue_tier unknown", func(t *testing.T) {
		cfg := baseValidConfig()
		cfg.Tiers[0].ThoughtSlap = &contract.ThoughtSlapOptIn{
			Enabled: true,
			Overrides: contract.ThoughtSlapConfig{
				RescueTier: "Ghost",
			},
		}
		err := config.ValidateConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "thought_slap rescue_tier references unknown tier \"Ghost\"") {
			t.Fatalf("expected tier unknown rescue_tier error, got: %v", err)
		}
	})

	t.Run("valid tier thought_slap rescue_tier pointing to default tier", func(t *testing.T) {
		cfg := baseValidConfig()
		cfg.Tiers[0].ThoughtSlap = &contract.ThoughtSlapOptIn{
			Enabled: true,
			Overrides: contract.ThoughtSlapConfig{
				RescueTier: "Default",
			},
		}
		if err := config.ValidateConfig(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid read_slap negative threshold", func(t *testing.T) {
		cfg := baseValidConfig()
		cfg.Kickstart.ReadSlap.Enabled = &enabled
		cfg.Kickstart.ReadSlap.Threshold = -1
		err := config.ValidateConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "kickstart.read_slap.threshold must be >= 1") {
			t.Fatalf("expected threshold error, got: %v", err)
		}
	})

	t.Run("legacy SessionKickstarted in tier when rejected", func(t *testing.T) {
		cfg := baseValidConfig()
		cfg.Tiers[0].When = "SessionKickstarted && Retries < 3"
		err := config.ValidateConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "rename to 'SessionReadSlapped'") {
			t.Fatalf("expected SessionKickstarted rejection, got: %v", err)
		}
	})
}

func TestValidateRawYAML_AllEdgeCases(t *testing.T) {
	// 1. Invalid YAML syntax
	if err := config.ValidateRawYAML([]byte("foo: [unclosed")); err == nil {
		t.Error("expected syntax error for invalid yaml")
	}
	// 2. Empty YAML
	if err := config.ValidateRawYAML([]byte("")); err != nil {
		t.Errorf("expected nil for empty yaml, got %v", err)
	}
	// 3. Non-mapping YAML
	if err := config.ValidateRawYAML([]byte("hello world")); err != nil {
		t.Errorf("expected nil for scalar yaml, got %v", err)
	}
	// 4. Deprecated kickstart keys
	cases := []struct {
		yaml   string
		errSub string
	}{
		{"kickstart:\n  enabled: true", "kickstart.enabled"},
		{"kickstart:\n  threshold: 5", "kickstart.threshold"},
		{"kickstart:\n  max_count: 5", "kickstart.max_count"},
		{"kickstart:\n  max_failures: 2", "kickstart.max_failures"},
		{"kickstart:\n  write_only: true", "kickstart.write_only"},
		{"kickstart:\n  write_tools: ['w']", "kickstart.write_tools"},
		{"kickstart:\n  prompt: 'hello'", "kickstart.prompt"},
		{"cycle_killer:\n  kickstart_threshold: 5", "cycle_killer.kickstart_threshold"},
		{"cycle_breaker:\n  write_tools: ['w']", "cycle_breaker.write_tools"},
		{"tiers:\n  - name: t1\n    when: SessionKickstarted", "SessionKickstarted"},
		{"tiers:\n  - name: t1\n    when: SessionKickstartCount > 0", "SessionKickstartCount"},
	}
	for _, c := range cases {
		err := config.ValidateRawYAML([]byte(c.yaml))
		if err == nil || !strings.Contains(err.Error(), c.errSub) {
			t.Errorf("expected error containing %q for yaml:\n%s\ngot: %v", c.errSub, c.yaml, err)
		}
	}
}

func TestValidation_EmptyTierModelRejected(t *testing.T) {
	baseCfg := func() *contract.Config {
		return &contract.Config{
			Providers: map[string]contract.ProviderConfig{
				"ollama": {
					BaseURL: "http://127.0.0.1:11434",
					Type:    provider.TypeOpenAICompatible,
					Local:   true,
				},
			},
			Tiers: []contract.Tier{
				{
					Name:     "Tier1",
					Model:    "llama-3-8b",
					Provider: "ollama",
				},
			},
			DefaultTier: contract.Tier{
				Name:     "Default",
				Model:    "llama-3-8b",
				Provider: "ollama",
			},
		}
	}

	t.Run("empty model in tier", func(t *testing.T) {
		cfg := baseCfg()
		cfg.Tiers[0].Model = ""
		err := config.ValidateConfig(cfg)
		if err == nil {
			t.Fatal("expected error for empty tier model, got nil")
		}
		if !strings.Contains(err.Error(), "tier 'Tier1' is missing required 'model'") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("whitespace model in tier", func(t *testing.T) {
		cfg := baseCfg()
		cfg.Tiers[0].Model = "   "
		err := config.ValidateConfig(cfg)
		if err == nil {
			t.Fatal("expected error for whitespace tier model, got nil")
		}
		if !strings.Contains(err.Error(), "tier 'Tier1' is missing required 'model'") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("empty model in default_tier", func(t *testing.T) {
		cfg := baseCfg()
		cfg.DefaultTier.Model = ""
		err := config.ValidateConfig(cfg)
		if err == nil {
			t.Fatal("expected error for empty default_tier model, got nil")
		}
		if !strings.Contains(err.Error(), "default_tier 'Default' is missing required 'model'") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("empty model in unnamed default_tier", func(t *testing.T) {
		cfg := baseCfg()
		cfg.DefaultTier.Name = ""
		cfg.DefaultTier.Model = ""
		err := config.ValidateConfig(cfg)
		if err == nil {
			t.Fatal("expected error for empty default_tier model, got nil")
		}
		if !strings.Contains(err.Error(), "default_tier is missing required 'model'") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})
}
