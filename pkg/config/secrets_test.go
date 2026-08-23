package config

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestSecrets_MaskAndMerge(t *testing.T) {
	orig := &contract.Config{
		Port:      8000,
		AuthToken: "sk-nacho-gateway-super-secret-12345",
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  "sk-or-v1-abcdef1234567890",
			},
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				When:     "tokens < 8000",
			},
		},
	}

	// 1. Sanitize
	sanitized := SanitizeConfig(orig)
	if sanitized.AuthToken != "sk-nac***" {
		t.Errorf("expected masked auth token 'sk-nac***', got '%s'", sanitized.AuthToken)
	}
	if sanitized.Providers["openrouter"].APIKey != "sk-or-***" {
		t.Errorf("expected masked API key 'sk-or-***', got '%s'", sanitized.Providers["openrouter"].APIKey)
	}
	if sanitized.Providers["ollama"].APIKey != "" {
		t.Errorf("expected empty ollama API key, got '%s'", sanitized.Providers["ollama"].APIKey)
	}

	// 2. Merge back with masked incoming payload
	incoming := &contract.Config{
		Port:      8000,
		AuthToken: sanitized.AuthToken, // "sk-nac***"
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  sanitized.Providers["openrouter"].APIKey, // "sk-or-***"
			},
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1 Modified",
				Provider: "ollama",
				When:     "tokens < 12000",
			},
		},
	}

	merged := MergeSecrets(orig, incoming)
	if merged.AuthToken != "sk-nacho-gateway-super-secret-12345" {
		t.Errorf("expected merged auth token to retain original secret, got '%s'", merged.AuthToken)
	}
	if merged.Providers["openrouter"].APIKey != "sk-or-v1-abcdef1234567890" {
		t.Errorf("expected merged provider API key to retain original secret, got '%s'", merged.Providers["openrouter"].APIKey)
	}
	if len(merged.Tiers) != 1 || merged.Tiers[0].When != "tokens < 12000" {
		t.Errorf("expected updated tier rule, got %+v", merged.Tiers)
	}

	// 3. User explicitly updates a secret
	incomingWithNewKey := &contract.Config{
		Port:      8000,
		AuthToken: "sk-brand-new-token",
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  "sk-brand-new-or-key",
			},
		},
	}
	mergedExplicit := MergeSecrets(orig, incomingWithNewKey)
	if mergedExplicit.AuthToken != "sk-brand-new-token" {
		t.Errorf("expected updated auth token, got '%s'", mergedExplicit.AuthToken)
	}
	if mergedExplicit.Providers["openrouter"].APIKey != "sk-brand-new-or-key" {
		t.Errorf("expected updated provider API key, got '%s'", mergedExplicit.Providers["openrouter"].APIKey)
	}

	// 4. Edge cases: empty string, short secret, nil configs
	if MaskSecret("") != "" {
		t.Errorf("expected empty string mask, got '%s'", MaskSecret(""))
	}
	if MaskSecret("short") != "***" {
		t.Errorf("expected '***' for short secret, got '%s'", MaskSecret("short"))
	}
	if SanitizeConfig(nil) != nil {
		t.Errorf("expected nil for SanitizeConfig(nil)")
	}
	if MergeSecrets(nil, orig) != orig {
		t.Errorf("expected orig for MergeSecrets(nil, orig)")
	}
	if MergeSecrets(orig, nil) != orig {
		t.Errorf("expected orig for MergeSecrets(orig, nil)")
	}
}

