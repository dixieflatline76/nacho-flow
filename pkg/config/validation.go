// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"fmt"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"gopkg.in/yaml.v3"
)

// ValidateRawYAML inspects the raw YAML node tree to detect removed legacy keys and DSL variables,
// failing fast with loud migration guidance before decoding.
func ValidateRawYAML(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]

		switch keyNode.Value {
		case "kickstart":
			if valNode.Kind == yaml.MappingNode {
				for j := 0; j < len(valNode.Content); j += 2 {
					k := valNode.Content[j].Value
					switch k {
					case "enabled":
						return fmt.Errorf("Config uses removed key 'kickstart.enabled'; move it to 'kickstart.read_slap.enabled'. See CHANGELOG v1.5")
					case "threshold":
						return fmt.Errorf("Config uses removed key 'kickstart.threshold'; move it to 'kickstart.read_slap.threshold'. See CHANGELOG v1.5")
					case "max_count":
						return fmt.Errorf("Config uses removed key 'kickstart.max_count'; move it to 'kickstart.read_slap.max_slaps'. See CHANGELOG v1.5")
					case "max_failures":
						return fmt.Errorf("Config uses removed key 'kickstart.max_failures'; move it to 'kickstart.read_slap.max_failures'. See CHANGELOG v1.5")
					case "write_only":
						return fmt.Errorf("Config uses removed key 'kickstart.write_only'; move it to 'kickstart.read_slap.write_only'. See CHANGELOG v1.5")
					case "write_tools", "custom_write_tools":
						return fmt.Errorf("Config uses removed key 'kickstart.%s'; move it to 'kickstart.read_slap.custom_write_tools'. See CHANGELOG v1.5", k)
					case "prompt":
						return fmt.Errorf("Config uses removed key 'kickstart.prompt'; move it to 'kickstart.read_slap.prompt_override'. See CHANGELOG v1.5")
					}
				}
			}
		case "cycle_killer", "cycle_breaker":
			block := keyNode.Value
			if valNode.Kind == yaml.MappingNode {
				for j := 0; j < len(valNode.Content); j += 2 {
					k := valNode.Content[j].Value
					if strings.HasPrefix(k, "kickstart_") || k == "write_tools" {
						return fmt.Errorf("Config uses removed key '%s.%s'; kickstart fields have moved to 'kickstart.read_slap.*'. See CHANGELOG v1.5", block, k)
					}
				}
			}
		case "tiers":
			if valNode.Kind == yaml.SequenceNode {
				for _, tierNode := range valNode.Content {
					if tierNode.Kind == yaml.MappingNode {
						tierName := ""
						for j := 0; j < len(tierNode.Content); j += 2 {
							if tierNode.Content[j].Value == "name" {
								tierName = tierNode.Content[j+1].Value
							}
						}
						for j := 0; j < len(tierNode.Content); j += 2 {
							k := tierNode.Content[j].Value
							v := tierNode.Content[j+1]
							if k == "when" {
								if strings.Contains(v.Value, "SessionKickstarted") {
									return fmt.Errorf("tier '%s' when expression uses removed DSL variable 'SessionKickstarted'; rename to 'SessionReadSlapped'. See CHANGELOG v1.5", tierName)
								}
								if strings.Contains(v.Value, "SessionKickstartCount") {
									return fmt.Errorf("tier '%s' when expression uses removed DSL variable 'SessionKickstartCount'; rename to 'SessionReadSlapCount'. See CHANGELOG v1.5", tierName)
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

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
		if p.Type != contract.ProviderTypeLocal && p.Type != contract.ProviderTypeCloud && p.Type != contract.ProviderTypeAnthropic && p.Type != contract.ProviderTypeOpenRouter {
			return fmt.Errorf(
				"provider %q: 'type' is required and must be 'local', 'cloud', 'anthropic', or 'openrouter' (got %q).\n"+
					"  Example for local engines (Ollama, vLLM, LM Studio, llama.cpp):\n"+
					"    %s:\n"+
					"      base_url: %s\n"+
					"      type: local\n"+
					"  Example for cloud APIs (OpenRouter, OpenAI):\n"+
					"    %s:\n"+
					"      base_url: %s\n"+
					"      type: cloud\n"+
					"  Example for native Anthropic API:\n"+
					"    %s:\n"+
					"      base_url: %s\n"+
					"      type: anthropic",
				id, p.Type, id, p.BaseURL, id, p.BaseURL, id, p.BaseURL,
			)
		}
	}

	// Validate Kickstart.ReadSlap
	if cfg.Kickstart.ReadSlap.Enabled != nil && *cfg.Kickstart.ReadSlap.Enabled {
		if cfg.Kickstart.ReadSlap.Threshold < 1 && cfg.Kickstart.ReadSlap.Threshold != 0 {
			return fmt.Errorf("config error: kickstart.read_slap.threshold must be >= 1 (got %d)", cfg.Kickstart.ReadSlap.Threshold)
		}
	}

	// Validate Kickstart.ThoughtSlap
	if cfg.Kickstart.ThoughtSlap.MaxAttempts > 0 && (cfg.Kickstart.ThoughtSlap.MaxAttempts < 1 || cfg.Kickstart.ThoughtSlap.MaxAttempts > 3) {
		return fmt.Errorf("config error: kickstart.thought_slap.max_attempts must be between 1 and 3 (got %d)", cfg.Kickstart.ThoughtSlap.MaxAttempts)
	}
	if cfg.Kickstart.ThoughtSlap.RescueTier != "" {
		if !tierExists(cfg, cfg.Kickstart.ThoughtSlap.RescueTier) {
			return fmt.Errorf("config error: kickstart.thought_slap.rescue_tier references unknown tier %q", cfg.Kickstart.ThoughtSlap.RescueTier)
		}
	}

	// Validate that all tiers reference existing providers, valid when variables, and valid thought_slap overrides
	for _, tier := range cfg.Tiers {
		if tier.When != "" {
			if strings.Contains(tier.When, "SessionKickstarted") {
				return fmt.Errorf("config error: tier '%s' when expression uses removed DSL variable 'SessionKickstarted'; rename to 'SessionReadSlapped'. See CHANGELOG v1.5", tier.Name)
			}
			if strings.Contains(tier.When, "SessionKickstartCount") {
				return fmt.Errorf("config error: tier '%s' when expression uses removed DSL variable 'SessionKickstartCount'; rename to 'SessionReadSlapCount'. See CHANGELOG v1.5", tier.Name)
			}
		}
		if tier.ThoughtSlap != nil && tier.ThoughtSlap.Enabled {
			ov := tier.ThoughtSlap.Overrides
			if ov.MaxAttempts > 0 && (ov.MaxAttempts < 1 || ov.MaxAttempts > 3) {
				return fmt.Errorf("config error: tier '%s' thought_slap max_attempts must be between 1 and 3 (got %d)", tier.Name, ov.MaxAttempts)
			}
			if ov.RescueTier != "" {
				if !tierExists(cfg, ov.RescueTier) {
					return fmt.Errorf("config error: tier '%s' thought_slap rescue_tier references unknown tier %q", tier.Name, ov.RescueTier)
				}
			}
		}
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

	for _, entry := range cfg.FairyDust.Entries {
		if entry.Provider != "" {
			if _, exists := cfg.Providers[entry.Provider]; !exists {
				return fmt.Errorf("config error: fairy_dust entry '%s' references unknown provider '%s'", entry.Name, entry.Provider)
			}
		}
	}

	return nil
}

func tierExists(cfg *contract.Config, name string) bool {
	if cfg.DefaultTier.Name == name {
		return true
	}
	for _, t := range cfg.Tiers {
		if t.Name == name {
			return true
		}
	}
	return false
}
