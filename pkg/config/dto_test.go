package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"gopkg.in/yaml.v3"
)

func TestDTO_ToPublicDTO_Nil(t *testing.T) {
	if ToPublicDTO(nil) != nil {
		t.Errorf("expected ToPublicDTO(nil) == nil")
	}
	bytes, err := SerializeConfigYAML(nil)
	if err != nil || bytes != nil {
		t.Errorf("expected SerializeConfigYAML(nil) to return nil, nil")
	}
}

func TestDTO_ToPublicDTO_FullAndSerialization(t *testing.T) {
	enableDirectives := true
	orig := &contract.Config{
		Port:      8000,
		AuthToken: "sk-nacho-gateway-super-secret-12345",
		Router: contract.RouterConfig{
			EnableInPromptDirectives: &enableDirectives,
		},
		Deals: contract.DealsConfig{
			Enabled:           true,
			AlertThresholdPct: 50.0,
			MinCodingIndex:    40.0,
			RequireTools:      true,
		},
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {
				BaseURL:             "https://openrouter.ai/api/v1",
				APIKey:              "sk-or-v1-abcdef1234567890",
				Type:                "cloud",
				Headers:             map[string]string{"HTTP-Referer": "https://nacho-flow.dev"},
				PricingSyncInterval: "15m",
			},
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:            "Tier 1",
				Model:           "gemma4:12b",
				Provider:        "ollama",
				When:            "tokens < 8000",
				StripImages:     true,
				ReasoningEffort: "low",
				MaxContext:      16000,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Cloud",
			Model:    "google/gemini-3.7-flash",
			Provider: "openrouter",
		},
	}

	dto := ToPublicDTO(orig)
	if dto == nil {
		t.Fatalf("expected non-nil DTO")
	}

	// 1. Validate Masking
	if dto.ClientAuth != "sk-nac***" {
		t.Errorf("expected ClientAuth 'sk-nac***', got '%s'", dto.ClientAuth)
	}
	if dto.Providers["openrouter"].Key != "sk-or-***" {
		t.Errorf("expected Key 'sk-or-***', got '%s'", dto.Providers["openrouter"].Key)
	}
	if dto.Providers["ollama"].Key != "" {
		t.Errorf("expected empty ollama Key, got '%s'", dto.Providers["ollama"].Key)
	}

	// 2. Validate JSON wire compatibility
	jsonBytes, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, `"auth_token":"sk-nac***"`) {
		t.Errorf("expected json to contain 'auth_token' tag, got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"api_key":"sk-or-***"`) {
		t.Errorf("expected json to contain 'api_key' tag, got: %s", jsonStr)
	}
	if strings.Contains(jsonStr, "super-secret") || strings.Contains(jsonStr, "abcdef1234567890") {
		t.Errorf("raw secret leaked into JSON wire payload!")
	}

	// 3. Validate YAML wire compatibility
	yamlBytes, err := yaml.Marshal(dto)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "auth_token: sk-nac***") {
		t.Errorf("expected yaml to contain 'auth_token: sk-nac***', got: %s", yamlStr)
	}
	if !strings.Contains(yamlStr, "api_key: sk-or-***") {
		t.Errorf("expected yaml to contain 'api_key: sk-or-***', got: %s", yamlStr)
	}
	if strings.Contains(yamlStr, "super-secret") || strings.Contains(yamlStr, "abcdef1234567890") {
		t.Errorf("raw secret leaked into YAML wire payload!")
	}

	// 4. Validate SerializeConfigYAML for disk persistence
	persistBytes, err := SerializeConfigYAML(orig)
	if err != nil {
		t.Fatalf("SerializeConfigYAML failed: %v", err)
	}
	persistStr := string(persistBytes)
	if !strings.Contains(persistStr, "auth_token: sk-nacho-gateway-super-secret-12345") {
		t.Errorf("expected persisted yaml to contain full auth token, got: %s", persistStr)
	}
	if !strings.Contains(persistStr, "api_key: sk-or-v1-abcdef1234567890") {
		t.Errorf("expected persisted yaml to contain full api key, got: %s", persistStr)
	}
}
