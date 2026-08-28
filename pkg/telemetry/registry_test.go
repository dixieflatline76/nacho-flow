// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package telemetry_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

type dummyPricingProvider struct {
	name string
}

func (d *dummyPricingProvider) Name() string { return d.name }
func (d *dummyPricingProvider) FetchPricing(ctx context.Context) (map[string]telemetry.ModelMetadata, error) {
	return nil, nil
}

func TestPricingFactoryRegistry_Basics(t *testing.T) {
	// Test nil and empty guards
	telemetry.RegisterPricingFactory("", nil)
	telemetry.RegisterPricingFactory("invalid", nil)
	telemetry.RegisterPricingFactory("", func(id string, cfg contract.ProviderConfig, defaultInterval time.Duration) (telemetry.PricingProvider, time.Duration) {
		return nil, 0
	})

	// Register custom factory
	telemetry.RegisterPricingFactory("MockCloud", func(id string, cfg contract.ProviderConfig, defaultInterval time.Duration) (telemetry.PricingProvider, time.Duration) {
		interval := defaultInterval
		if cfg.PricingSyncInterval != "" {
			if parsed, err := time.ParseDuration(cfg.PricingSyncInterval); err == nil {
				interval = parsed
			}
		}
		return &dummyPricingProvider{name: id}, interval
	})

	// Case-insensitive lookup
	factory, ok := telemetry.LookupPricingFactory("mockcloud")
	if !ok {
		t.Fatalf("expected to find factory for 'mockcloud'")
	}

	prov, interval := factory("mockcloud", contract.ProviderConfig{
		PricingSyncInterval: "2h",
	}, 10*time.Minute)

	if prov == nil || prov.Name() != "mockcloud" {
		t.Errorf("unexpected provider name: %v", prov)
	}
	if interval != 2*time.Hour {
		t.Errorf("expected 2h interval, got %v", interval)
	}

	// Lookup non-existent
	_, notFound := telemetry.LookupPricingFactory("non_existent_pricing_prov")
	if notFound {
		t.Errorf("expected notFound=true for non_existent_pricing_prov")
	}

	// List registered providers
	list := telemetry.ListRegisteredPricingProviders()
	foundMock := false
	foundOpenRouter := false
	for _, p := range list {
		if strings.EqualFold(p, "mockcloud") {
			foundMock = true
		}
		if strings.EqualFold(p, contract.ProviderOpenRouter) {
			foundOpenRouter = true
		}
	}
	if !foundMock {
		t.Errorf("expected 'mockcloud' in registered providers list: %v", list)
	}
	if !foundOpenRouter {
		t.Errorf("expected 'openrouter' in registered providers list: %v", list)
	}
}

func TestPricingFactoryRegistry_OpenRouterAutoRegistration(t *testing.T) {
	factory, ok := telemetry.LookupPricingFactory(contract.ProviderOpenRouter)
	if !ok {
		t.Fatalf("expected OpenRouter factory to be auto-registered via init()")
	}

	pCfg := contract.ProviderConfig{
		BaseURL:             "https://openrouter.ai/api/v1",
		APIKey:              "sk-or-test-key",
		Type:                contract.ProviderTypeCloud,
		PricingSyncInterval: "45m",
	}

	prov, interval := factory("openrouter", pCfg, time.Hour)
	if prov == nil {
		t.Fatalf("expected non-nil provider from factory")
	}
	if prov.Name() != contract.ProviderOpenRouter {
		t.Errorf("expected provider name %q, got %q", contract.ProviderOpenRouter, prov.Name())
	}
	if interval != 45*time.Minute {
		t.Errorf("expected interval 45m, got %v", interval)
	}

	// Test default baseURL and default interval fallback
	pCfgDefault := contract.ProviderConfig{
		APIKey: "sk-or-test-key",
		Type:   contract.ProviderTypeCloud,
	}
	provDef, intervalDef := factory("openrouter", pCfgDefault, 30*time.Minute)
	if provDef == nil {
		t.Fatalf("expected non-nil default provider")
	}
	if intervalDef != 30*time.Minute {
		t.Errorf("expected default interval 30m, got %v", intervalDef)
	}
}
