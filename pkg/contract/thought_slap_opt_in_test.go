package contract_test

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"gopkg.in/yaml.v3"
)

func TestThoughtSlapOptIn_YAMLUnmarshal(t *testing.T) {
	tests := []struct {
		name        string
		yamlData    string
		wantEnabled bool
		wantMaxAtt  int
		wantRescue  string
		wantPrompt  string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "true boolean opt-in",
			yamlData:    `thought_slap: true`,
			wantEnabled: true,
			wantMaxAtt:  0, // no tier override set
		},
		{
			name:        "false boolean opt-out",
			yamlData:    `thought_slap: false`,
			wantEnabled: false,
		},
		{
			name: "mapping with overrides",
			yamlData: `
thought_slap:
  max_attempts: 2
  rescue_tier: "Tier 1: Fast Coding Workhorse"
  prompt_override: "Stop reasoning. Call a tool now."
`,
			wantEnabled: true,
			wantMaxAtt:  2,
			wantRescue:  "Tier 1: Fast Coding Workhorse",
			wantPrompt:  "Stop reasoning. Call a tool now.",
		},
		{
			name: "mapping with single override",
			yamlData: `
thought_slap:
  rescue_tier: "Tier 2: Debug & Reasoning"
`,
			wantEnabled: true,
			wantRescue:  "Tier 2: Debug & Reasoning",
		},
		{
			name:     "bare null key rejected",
			yamlData: `thought_slap:`,
			wantErr:  true,
			errMsg:   "thought_slap needs a value: use `thought_slap: true` or a block of overrides",
		},
		{
			name:     "explicit null value rejected",
			yamlData: `thought_slap: null`,
			wantErr:  true,
			errMsg:   "thought_slap needs a value: use `thought_slap: true` or a block of overrides",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tier contract.Tier
			err := yaml.Unmarshal([]byte(tt.yamlData), &tier)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if tt.errMsg != "" && !containsStr(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got: %v", tt.errMsg, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected unmarshal error: %v", err)
			}
			if tier.ThoughtSlap == nil {
				t.Fatalf("expected non-nil ThoughtSlap")
			}
			if tier.ThoughtSlap.Enabled != tt.wantEnabled {
				t.Errorf("Enabled = %v, want %v", tier.ThoughtSlap.Enabled, tt.wantEnabled)
			}
			if tier.ThoughtSlap.Overrides.MaxAttempts != tt.wantMaxAtt {
				t.Errorf("MaxAttempts = %d, want %d", tier.ThoughtSlap.Overrides.MaxAttempts, tt.wantMaxAtt)
			}
			if tier.ThoughtSlap.Overrides.RescueTier != tt.wantRescue {
				t.Errorf("RescueTier = %q, want %q", tier.ThoughtSlap.Overrides.RescueTier, tt.wantRescue)
			}
			if tier.ThoughtSlap.Overrides.PromptOverride != tt.wantPrompt {
				t.Errorf("PromptOverride = %q, want %q", tier.ThoughtSlap.Overrides.PromptOverride, tt.wantPrompt)
			}
		})
	}
}

func TestResolveThoughtSlap(t *testing.T) {
	global := contract.ThoughtSlapConfig{
		MaxAttempts:    2,
		RescueTier:     "Tier 0 Global Rescue",
		PromptOverride: "Global prompt",
	}

	tests := []struct {
		name        string
		optIn       *contract.ThoughtSlapOptIn
		global      contract.ThoughtSlapConfig
		wantEnabled bool
		wantMaxAtt  int
		wantRescue  string
		wantPrompt  string
	}{
		{
			name:        "nil optIn is disabled",
			optIn:       nil,
			global:      global,
			wantEnabled: false,
		},
		{
			name:        "explicit false optIn is disabled",
			optIn:       &contract.ThoughtSlapOptIn{Enabled: false},
			global:      global,
			wantEnabled: false,
		},
		{
			name:        "true optIn inherits global defaults",
			optIn:       &contract.ThoughtSlapOptIn{Enabled: true},
			global:      global,
			wantEnabled: true,
			wantMaxAtt:  2,
			wantRescue:  "Tier 0 Global Rescue",
			wantPrompt:  "Global prompt",
		},
		{
			name:        "true optIn with empty global inherits built-in defaults",
			optIn:       &contract.ThoughtSlapOptIn{Enabled: true},
			global:      contract.ThoughtSlapConfig{},
			wantEnabled: true,
			wantMaxAtt:  1,  // built-in default
			wantRescue:  "", // same tier
			wantPrompt:  "", // caller uses contract.DefaultThoughtSlapPrompt
		},
		{
			name: "tier override takes precedence over global and defaults",
			optIn: &contract.ThoughtSlapOptIn{
				Enabled: true,
				Overrides: contract.ThoughtSlapConfig{
					MaxAttempts:    3,
					RescueTier:     "Tier 3 Specific",
					PromptOverride: "Tier specific prompt",
				},
			},
			global:      global,
			wantEnabled: true,
			wantMaxAtt:  3,
			wantRescue:  "Tier 3 Specific",
			wantPrompt:  "Tier specific prompt",
		},
		{
			name: "partial tier override inherits unset fields from global",
			optIn: &contract.ThoughtSlapOptIn{
				Enabled: true,
				Overrides: contract.ThoughtSlapConfig{
					RescueTier: "Tier 3 Specific",
				},
			},
			global:      global,
			wantEnabled: true,
			wantMaxAtt:  2, // from global
			wantRescue:  "Tier 3 Specific",
			wantPrompt:  "Global prompt", // from global
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, enabled := contract.ResolveThoughtSlap(tt.optIn, tt.global)
			if enabled != tt.wantEnabled {
				t.Errorf("enabled = %v, want %v", enabled, tt.wantEnabled)
			}
			if !enabled {
				return
			}
			if cfg.MaxAttempts != tt.wantMaxAtt {
				t.Errorf("MaxAttempts = %d, want %d", cfg.MaxAttempts, tt.wantMaxAtt)
			}
			if cfg.RescueTier != tt.wantRescue {
				t.Errorf("RescueTier = %q, want %q", cfg.RescueTier, tt.wantRescue)
			}
			if cfg.PromptOverride != tt.wantPrompt {
				t.Errorf("PromptOverride = %q, want %q", cfg.PromptOverride, tt.wantPrompt)
			}
		})
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && searchSubstr(s, substr)))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
