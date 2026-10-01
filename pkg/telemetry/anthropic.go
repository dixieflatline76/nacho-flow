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
	return &AnthropicPricingProvider{mgr: mgr}
}

// Name returns the provider identifier "anthropic".
func (p *AnthropicPricingProvider) Name() string {
	return string(contract.ProviderTypeAnthropic)
}

// FetchPricing extracts all Anthropic models from the curated catalog and indexes them by both full ID and base ID.
func (p *AnthropicPricingProvider) FetchPricing(ctx context.Context) (map[string]ModelMetadata, error) {
	cat := p.mgr.GetActiveCatalog()
	if cat == nil || len(cat.Models) == 0 {
		return make(map[string]ModelMetadata), nil
	}

	results := make(map[string]ModelMetadata, len(cat.Models))
	for id, profile := range cat.Models {
		idLower := strings.ToLower(id)
		if !strings.HasPrefix(idLower, "anthropic/") {
			continue
		}

		meta := ModelMetadata{
			ModelPricing: ModelPricing{
				PromptCostPerMillion:     profile.PromptCostPerMillion,
				CompletionCostPerMillion: profile.CompletionCostPerMillion,
			},
			ModelID:           id,
			Provider:          string(contract.ProviderTypeAnthropic),
			Name:              profile.Name,
			SupportsVision:    profile.SupportsVision,
			SupportsTools:     profile.SupportsTools,
			SupportsReasoning: profile.TierRole == curation.RoleDeepReasoner || profile.TierRole == curation.RoleCodingWorkhorse,
			CodingIndex:       profile.CodingIndex,
			AgenticIndex:      profile.ToolReliability,
		}

		// Index by full key (e.g. "anthropic/claude-3-7-sonnet")
		results[id] = meta

		// Index by bare model name (e.g. "claude-3-7-sonnet")
		baseID := id
		if slash := strings.LastIndex(id, "/"); slash >= 0 {
			baseID = id[slash+1:]
		}
		if _, exists := results[baseID]; !exists {
			bareMeta := meta
			bareMeta.ModelID = baseID
			results[baseID] = bareMeta
		}

		// Dynamically index catalog-defined aliases (e.g. "claude-3-7-sonnet-20250219")
		for _, alias := range profile.Aliases {
			if alias == "" {
				continue
			}
			if _, exists := results[alias]; !exists {
				aliasMeta := meta
				aliasMeta.ModelID = alias
				results[alias] = aliasMeta
			}
			if !strings.Contains(alias, "/") {
				fullAlias := "anthropic/" + alias
				if _, exists := results[fullAlias]; !exists {
					fullMeta := meta
					fullMeta.ModelID = fullAlias
					results[fullAlias] = fullMeta
				}
			}
		}
	}

	return results, nil
}

func init() {
	RegisterPricingFactory(string(contract.ProviderTypeAnthropic), func(id string, cfg contract.ProviderConfig, defaultInterval time.Duration) (PricingProvider, time.Duration) {
		interval := defaultInterval
		if interval == 0 {
			interval = 24 * time.Hour
		}
		if cfg.PricingSyncInterval != "" {
			if parsed, err := time.ParseDuration(cfg.PricingSyncInterval); err == nil {
				interval = parsed
			}
		}
		return NewAnthropicPricingProvider(), interval
	})
}
