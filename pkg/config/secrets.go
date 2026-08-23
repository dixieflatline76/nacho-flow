package config

import (
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// MaskSecret returns a masked version of a secret key preserving the prefix if possible.
func MaskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return "***"
	}
	// Preserve first 6 chars (e.g. "sk-or-" or "sk-ant-") and append "***"
	prefix := secret[:6]
	return prefix + "***"
}

// IsMasked returns true if the given string represents a masked placeholder.
func IsMasked(val string) bool {
	return strings.Contains(val, "***")
}

// SanitizeConfig returns a deep copy of the configuration with all secrets masked for safe UI display.
func SanitizeConfig(cfg *contract.Config) *contract.Config {
	if cfg == nil {
		return nil
	}

	sanitized := *cfg
	sanitized.AuthToken = MaskSecret(cfg.AuthToken)

	sanitized.Providers = make(map[string]contract.ProviderConfig, len(cfg.Providers))
	for id, p := range cfg.Providers {
		pCopy := p
		if pCopy.APIKey != "" {
			pCopy.APIKey = MaskSecret(p.APIKey)
		}
		sanitized.Providers[id] = pCopy
	}

	// Copy tiers slice
	sanitized.Tiers = make([]contract.Tier, len(cfg.Tiers))
	copy(sanitized.Tiers, cfg.Tiers)

	return &sanitized
}

// MergeSecrets merges incoming configuration with existing active secrets, ensuring that
// masked placeholders (e.g. "sk-***") do not overwrite active secrets in memory.
func MergeSecrets(existingCfg, newCfg *contract.Config) *contract.Config {
	if newCfg == nil {
		return existingCfg
	}
	if existingCfg == nil {
		return newCfg
	}

	merged := *newCfg

	// Check AuthToken
	if IsMasked(merged.AuthToken) || merged.AuthToken == "" {
		merged.AuthToken = existingCfg.AuthToken
	}

	// Check Provider API Keys
	merged.Providers = make(map[string]contract.ProviderConfig, len(newCfg.Providers))
	for id, p := range newCfg.Providers {
		pCopy := p
		if IsMasked(pCopy.APIKey) || pCopy.APIKey == "" {
			if existingP, ok := existingCfg.Providers[id]; ok {
				pCopy.APIKey = existingP.APIKey
			}
		}
		merged.Providers[id] = pCopy
	}

	// Copy tiers
	merged.Tiers = make([]contract.Tier, len(newCfg.Tiers))
	copy(merged.Tiers, newCfg.Tiers)

	return &merged
}
