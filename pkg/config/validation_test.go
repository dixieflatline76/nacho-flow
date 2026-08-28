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
			if !strings.Contains(errMsg, "type' is required and must be 'local' or 'cloud'") {
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
