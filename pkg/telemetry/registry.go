// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package telemetry

import (
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// PricingProviderFactory constructs a PricingProvider instance and its sync interval from provider configuration.
type PricingProviderFactory func(id string, cfg contract.ProviderConfig, defaultInterval time.Duration) (PricingProvider, time.Duration)

var (
	pricingFactoriesMu sync.RWMutex
	pricingFactories   = make(map[string]PricingProviderFactory)
)

// RegisterPricingFactory registers a pricing provider factory function for a provider identifier.
// Provider identifiers are normalized to lowercase for case-insensitive lookup.
func RegisterPricingFactory(providerID string, factory PricingProviderFactory) {
	if providerID == "" || factory == nil {
		return
	}
	pricingFactoriesMu.Lock()
	defer pricingFactoriesMu.Unlock()
	pricingFactories[strings.ToLower(providerID)] = factory
}

// LookupPricingFactory retrieves the factory for a given provider identifier, if registered.
func LookupPricingFactory(providerID string) (PricingProviderFactory, bool) {
	pricingFactoriesMu.RLock()
	defer pricingFactoriesMu.RUnlock()
	f, ok := pricingFactories[strings.ToLower(providerID)]
	return f, ok
}

// ListRegisteredPricingProviders returns a slice of all registered pricing provider identifiers.
func ListRegisteredPricingProviders() []string {
	pricingFactoriesMu.RLock()
	defer pricingFactoriesMu.RUnlock()
	list := make([]string, 0, len(pricingFactories))
	for id := range pricingFactories {
		list = append(list, id)
	}
	return list
}
