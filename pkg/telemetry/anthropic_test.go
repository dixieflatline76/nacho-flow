// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestAnthropicPricingProvider_FetchPricing(t *testing.T) {
	prov := NewAnthropicPricingProvider()
	if prov.Name() != string(contract.ProviderTypeAnthropic) {
		t.Fatalf("expected name %s, got %s", contract.ProviderTypeAnthropic, prov.Name())
	}

	prices, err := prov.FetchPricing(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prices) == 0 {
		t.Fatalf("expected non-empty pricing map from curated catalog")
	}

	// 1. Verify canonical sonnet pricing ($3 prompt, $15 completion)
	sonnet, ok := prices["anthropic/claude-sonnet-5"]
	if !ok {
		t.Fatalf("expected anthropic/claude-sonnet-5 in prices")
	}
	if sonnet.PromptCostPerMillion != 3.0 || sonnet.CompletionCostPerMillion != 15.0 {
		t.Errorf("expected $3/$15 rates for sonnet, got prompt=%f, comp=%f",
			sonnet.PromptCostPerMillion, sonnet.CompletionCostPerMillion)
	}
	if !sonnet.SupportsVision || !sonnet.SupportsTools {
		t.Errorf("expected vision and tools supported for sonnet")
	}

	// 2. Verify bare model key index
	sonnetBare, ok := prices["claude-sonnet-5"]
	if !ok {
		t.Fatalf("expected bare claude-sonnet-5 in prices")
	}
	if sonnetBare.PromptCostPerMillion != 3.0 {
		t.Errorf("expected bare sonnet rate 3.0, got %f", sonnetBare.PromptCostPerMillion)
	}

	// 3. Verify Claude 3.7 Sonnet and dated aliases
	alias37, ok := prices["claude-3-7-sonnet-20250219"]
	if !ok {
		t.Fatalf("expected dated alias claude-3-7-sonnet-20250219 in prices")
	}
	if alias37.PromptCostPerMillion != 3.0 || alias37.CompletionCostPerMillion != 15.0 {
		t.Errorf("expected $3/$15 for claude-3-7-sonnet-20250219, got %v", alias37)
	}

	// 4. Verify Claude Opus pricing ($15 prompt, $75 completion)
	opus, ok := prices["anthropic/claude-opus-5"]
	if !ok {
		t.Fatalf("expected anthropic/claude-opus-5 in prices")
	}
	if opus.PromptCostPerMillion != 15.0 || opus.CompletionCostPerMillion != 75.0 {
		t.Errorf("expected $15/$75 rates for opus, got prompt=%f, comp=%f",
			opus.PromptCostPerMillion, opus.CompletionCostPerMillion)
	}

	// 5. Verify Claude Haiku pricing ($1 prompt, $5 completion)
	haiku, ok := prices["anthropic/claude-haiku-4.5"]
	if !ok {
		t.Fatalf("expected anthropic/claude-haiku-4.5 in prices")
	}
	if haiku.PromptCostPerMillion != 1.0 || haiku.CompletionCostPerMillion != 5.0 {
		t.Errorf("expected $1/$5 rates for haiku, got prompt=%f, comp=%f",
			haiku.PromptCostPerMillion, haiku.CompletionCostPerMillion)
	}
}

func TestAnthropicPricingFactory_Registration(t *testing.T) {
	factory, ok := LookupPricingFactory(string(contract.ProviderTypeAnthropic))
	if !ok {
		t.Fatalf("expected factory registered for %s", contract.ProviderTypeAnthropic)
	}

	cfg := contract.ProviderConfig{
		Type:                contract.ProviderTypeAnthropic,
		PricingSyncInterval: "12h",
	}

	prov, interval := factory("my-anthropic", cfg, time.Hour)
	if prov.Name() != string(contract.ProviderTypeAnthropic) {
		t.Errorf("expected provider name anthropic, got %s", prov.Name())
	}
	if interval != 12*time.Hour {
		t.Errorf("expected interval 12h, got %v", interval)
	}
}
