// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package telemetry

import (
	"context"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

// AnthropicPricingProvider populates Anthropic pricing and capabilities from the curated catalog.
type AnthropicPricingProvider struct {
	id  string
	mgr *curation.Manager
}

// NewAnthropicPricingProvider creates a new Anthropic pricing provider using the default catalog manager.
func NewAnthropicPricingProvider() *AnthropicPricingProvider {
	return NewAnthropicPricingProviderWithManager(curation.DefaultManager())
}

// NewAnthropicPricingProviderWithManager creates a new Anthropic pricing provider with a custom curation manager.
func NewAnthropicPricingProviderWithManager(mgr *curation.Manager) *AnthropicPricingProvider {
	if mgr == nil {
		mgr = curation.DefaultManager()
	}
	return &AnthropicPricingProvider{
		id:  string(contract.PlatformAnthropic),
		mgr: mgr,
	}
}

// Name returns the configured provider identifier (defaults to "anthropic").
func (p *AnthropicPricingProvider) Name() string {
	if p.id != "" {
		return p.id
	}
	return string(contract.PlatformAnthropic)
}

// Platform returns contract.PlatformAnthropic.
func (p *AnthropicPricingProvider) Platform() contract.Platform {
	return contract.PlatformAnthropic
}

// IsCatalogDerived reports that this pricing provider reads directly from the curated catalog
// and must not feed its entries back into RegisterDynamicModels.
func (p *AnthropicPricingProvider) IsCatalogDerived() bool {
	return true
}

// FetchPricing extracts active Anthropic models with Providers[PlatformAnthropic] bindings from the curated catalog
// and indexes them strictly by their Anthropic wire model ID and bare aliases (never under "anthropic/..." keys).
func (p *AnthropicPricingProvider) FetchPricing(ctx context.Context) (map[string]ModelMetadata, error) {
	cat := p.mgr.GetActiveCatalog()
	if cat == nil || len(cat.Models) == 0 {
		return make(map[string]ModelMetadata), nil
	}

	results := make(map[string]ModelMetadata, len(cat.Models))
	provName := p.Name()
	for id, profile := range cat.Models {
		if strings.Contains(id, ":batch") || strings.Contains(id, "-batch") {
			continue
		}
		binding, hasBinding := profile.Providers[contract.PlatformAnthropic]
		if !hasBinding || binding.ModelID == "" {
			continue
		}

		promptCost := profile.PromptCostPerMillion
		if binding.PromptCostPerMillion != nil {
			promptCost = *binding.PromptCostPerMillion
		}
		compCost := profile.CompletionCostPerMillion
		if binding.CompletionCostPerMillion != nil {
			compCost = *binding.CompletionCostPerMillion
		}

		meta := ModelMetadata{
			ModelPricing: ModelPricing{
				PromptCostPerMillion:     promptCost,
				CompletionCostPerMillion: compCost,
			},
			ModelID:           binding.ModelID,
			Provider:          provName,
			Name:              profile.Name,
			SupportsVision:    profile.SupportsVision,
			SupportsTools:     profile.SupportsTools,
			SupportsReasoning: profile.TierRole == curation.RoleDeepReasoner || profile.TierRole == curation.RoleCodingWorkhorse,
			CodingIndex:       profile.CodingIndex,
			AgenticIndex:      profile.ToolReliability,
		}

		results[binding.ModelID] = meta

		for _, alias := range profile.Aliases {
			if alias == "" || strings.Contains(alias, "/") {
				continue
			}
			if _, exists := results[alias]; !exists {
				aliasMeta := meta
				aliasMeta.ModelID = alias
				results[alias] = aliasMeta
			}
		}
	}

	return results, nil
}

func init() {
	RegisterPricingFactory(string(contract.PlatformAnthropic), func(id string, cfg contract.ProviderConfig, defaultInterval time.Duration) (PricingProvider, time.Duration) {
		interval := defaultInterval
		if interval == 0 {
			interval = 24 * time.Hour
		}
		if cfg.PricingSyncInterval != "" {
			if parsed, err := time.ParseDuration(cfg.PricingSyncInterval); err == nil {
				interval = parsed
			}
		}
		prov := NewAnthropicPricingProvider()
		if id != "" {
			prov.id = id
		}
		return prov, interval
	})
}
