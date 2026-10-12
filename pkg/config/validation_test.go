// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package config_test

import (
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/config"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
				Type:    contract.ProviderTypeLocal,
			},
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
				Type:    contract.ProviderTypeCloud,
			},
			"anthropic": {
				BaseURL: "https://api.anthropic.com",
				Type:    contract.ProviderTypeAnthropic,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local Tier",
				Provider: "ollama",
			},
			{
				Name:     "Cloud Tier",
				Provider: "openrouter",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
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

func TestValidateConfig_MissingBaseURL(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "   ",
				Type:    contract.ProviderTypeLocal,
			},
		},
	}
	err := config.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing base_url, got nil")
	}
	if !strings.Contains(err.Error(), "missing required 'base_url'") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateConfig_InvalidOrMissingType(t *testing.T) {
	tests := []struct {
		name         string
		providerType contract.ProviderType
	}{
		{"empty type", ""},
		{"invalid type custom", "fast"},
		{"invalid type uppercase", "LOCAL"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &contract.Config{
				Providers: map[string]contract.ProviderConfig{
					"my-engine": {
						BaseURL: "http://10.0.0.5:8000",
						Type:    tc.providerType,
					},
				},
			}
			err := config.ValidateConfig(cfg)
			if err == nil {
				t.Fatalf("expected error for provider type %q, got nil", tc.providerType)
			}
			errMsg := err.Error()
			if !strings.Contains(errMsg, "type' is required and must be 'local', 'cloud', 'anthropic', or 'openrouter'") {
				t.Errorf("error does not contain required guidance: %s", errMsg)
			}
			if !strings.Contains(errMsg, "Example for local engines") || !strings.Contains(errMsg, "Example for cloud APIs") {
				t.Errorf("error does not contain structured examples: %s", errMsg)
			}
		})
	}
}

func TestValidateConfig_UnknownTierProvider(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
				Type:    contract.ProviderTypeLocal,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Unknown Tier",
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
				Type:    contract.ProviderTypeLocal,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
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
				Type:    contract.ProviderTypeLocal,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
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
					Type:    contract.ProviderTypeLocal,
				},
			},
			Tiers: []contract.Tier{
				{
					Name:     "Tier 1",
					Provider: "mock",
				},
			},
			DefaultTier: contract.Tier{
				Name:     "Default",
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
