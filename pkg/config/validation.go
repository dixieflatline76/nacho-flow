// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"fmt"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// ValidateConfig enforces strict schema contracts across providers, tiers, and server settings.
func ValidateConfig(cfg *contract.Config) error {
	if cfg == nil {
		return fmt.Errorf("config error: configuration is nil")
	}

	if len(cfg.Providers) == 0 {
		return fmt.Errorf("config error: at least one provider must be defined in 'providers'")
	}

	for id, p := range cfg.Providers {
		if strings.TrimSpace(p.BaseURL) == "" {
			return fmt.Errorf("config error: provider '%s' is missing required 'base_url'", id)
		}
		if p.Type != contract.ProviderTypeLocal && p.Type != contract.ProviderTypeCloud {
			return fmt.Errorf(
				"provider %q: 'type' is required and must be 'local' or 'cloud' (got %q).\n"+
					"  Example for local engines (Ollama, vLLM, LM Studio, llama.cpp):\n"+
					"    %s:\n"+
					"      base_url: %s\n"+
					"      type: local\n"+
					"  Example for cloud APIs (OpenRouter, Anthropic, OpenAI):\n"+
					"    %s:\n"+
					"      base_url: %s\n"+
					"      type: cloud",
				id, p.Type, id, p.BaseURL, id, p.BaseURL,
			)
		}
	}

	// Validate that all tiers reference existing providers
	for _, tier := range cfg.Tiers {
		if tier.Provider != "" {
			if _, exists := cfg.Providers[tier.Provider]; !exists {
				return fmt.Errorf("config error: tier '%s' references unknown provider '%s'", tier.Name, tier.Provider)
			}
		}
	}

	if cfg.DefaultTier.Provider != "" {
		if _, exists := cfg.Providers[cfg.DefaultTier.Provider]; !exists {
			return fmt.Errorf("config error: default_tier references unknown provider '%s'", cfg.DefaultTier.Provider)
		}
	}

	return nil
}
