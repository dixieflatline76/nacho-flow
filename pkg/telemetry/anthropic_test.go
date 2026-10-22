// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

func TestAnthropicPricingProvider_FetchPricing(t *testing.T) {
	prov := NewAnthropicPricingProvider()
	if prov.Name() != string(provider.TypeAnthropic) {
		t.Fatalf("expected name %s, got %s", provider.TypeAnthropic, prov.Name())
	}
	if prov.Platform() != contract.PlatformAnthropic {
		t.Fatalf("expected platform %s, got %s", contract.PlatformAnthropic, prov.Platform())
	}
	if !prov.IsCatalogDerived() {
		t.Fatalf("expected IsCatalogDerived() == true for AnthropicPricingProvider")
	}

	prices, err := prov.FetchPricing(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prices) == 0 {
		t.Fatalf("expected non-empty pricing map from curated catalog")
	}

	// 1. Verify bare wire model key index (never "anthropic/...")
	for k := range prices {
		if len(k) > 10 && k[:10] == "anthropic/" {
			t.Errorf("AnthropicPricingProvider must not emit prefixed key %q", k)
		}
	}

	sonnetBare, ok := prices["claude-sonnet-5"]
	if !ok {
		t.Fatalf("expected bare claude-sonnet-5 in prices")
	}
	if sonnetBare.PromptCostPerMillion != 3.0 || sonnetBare.CompletionCostPerMillion != 15.0 {
		t.Errorf("expected $3/$15 rates for sonnet, got prompt=%f, comp=%f",
			sonnetBare.PromptCostPerMillion, sonnetBare.CompletionCostPerMillion)
	}
	if !sonnetBare.SupportsVision || !sonnetBare.SupportsTools {
		t.Errorf("expected vision and tools supported for sonnet")
	}

	// 2. Verify dated/latest alias indexing (e.g. claude-sonnet-5-latest)
	aliasSonnet, ok := prices["claude-sonnet-5-latest"]
	if !ok {
		t.Fatalf("expected alias claude-sonnet-5-latest in prices")
	}
	if aliasSonnet.PromptCostPerMillion != 3.0 || aliasSonnet.CompletionCostPerMillion != 15.0 {
		t.Errorf("expected $3/$15 for claude-sonnet-5-latest, got %v", aliasSonnet)
	}

	// 3. Verify Claude Opus pricing ($15 prompt, $75 completion)
	opus, ok := prices["claude-opus-5"]
	if !ok {
		t.Fatalf("expected claude-opus-5 in prices")
	}
	if opus.PromptCostPerMillion != 15.0 || opus.CompletionCostPerMillion != 75.0 {
		t.Errorf("expected $15/$75 rates for opus, got prompt=%f, comp=%f",
			opus.PromptCostPerMillion, opus.CompletionCostPerMillion)
	}

	// 4. Verify Claude Haiku pricing ($1 prompt, $5 completion)
	haiku, ok := prices["claude-haiku-4-5"]
	if !ok {
		t.Fatalf("expected claude-haiku-4-5 in prices")
	}
	if haiku.PromptCostPerMillion != 1.0 || haiku.CompletionCostPerMillion != 5.0 {
		t.Errorf("expected $1/$5 rates for haiku, got prompt=%f, comp=%f",
			haiku.PromptCostPerMillion, haiku.CompletionCostPerMillion)
	}
}

func TestAnthropicPricingFactory_Registration(t *testing.T) {
	factory, ok := LookupPricingFactory(string(provider.TypeAnthropic))
	if !ok {
		t.Fatalf("expected factory registered for %s", provider.TypeAnthropic)
	}

	cfg := contract.ProviderConfig{
		Type:                provider.TypeAnthropic,
		PricingSyncInterval: "12h",
	}

	prov, interval := factory("my-anthropic", cfg, time.Hour)
	if prov.Name() != "my-anthropic" {
		t.Errorf("expected provider name my-anthropic, got %s", prov.Name())
	}
	if interval != 12*time.Hour {
		t.Errorf("expected interval 12h, got %v", interval)
	}
}

func TestAnthropicPricingProvider_NilManager(t *testing.T) {
	prov := NewAnthropicPricingProviderWithManager(nil)
	if prov == nil {
		t.Fatal("expected non-nil provider")
	}
	factory, ok := LookupPricingFactory(string(provider.TypeAnthropic))
	if !ok {
		t.Fatal("expected factory")
	}
	_, interval := factory("test", contract.ProviderConfig{}, 0)
	if interval != 24*time.Hour {
		t.Errorf("expected 24h default interval, got %v", interval)
	}
}
