package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ModelPricing represents USD cost per million tokens.
type ModelPricing struct {
	PromptCostPerMillion     float64
	CompletionCostPerMillion float64
}

// PricingProvider is the plugin interface implemented by provider pricing fetchers.
type PricingProvider interface {
	Name() string
	FetchPricing(ctx context.Context) (map[string]ModelPricing, error)
}

// PricingOracle manages lock-free model pricing lookup across registered providers.
type PricingOracle struct {
	providers  []PricingProvider
	pricingMap atomic.Pointer[map[string]ModelPricing]
	mu         sync.Mutex
}

// NewPricingOracle creates a new initialized PricingOracle.
func NewPricingOracle() *PricingOracle {
	oracle := &PricingOracle{}
	emptyMap := make(map[string]ModelPricing)
	oracle.pricingMap.Store(&emptyMap)
	return oracle
}

// RegisterProvider registers a pricing provider plugin.
func (o *PricingOracle) RegisterProvider(p PricingProvider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.providers = append(o.providers, p)
}

// Sync queries all registered providers and atomically swaps the active pricing map.
func (o *PricingOracle) Sync(ctx context.Context) error {
	o.mu.Lock()
	providers := make([]PricingProvider, len(o.providers))
	copy(providers, o.providers)
	o.mu.Unlock()

	mergedMap := make(map[string]ModelPricing)

	var lastErr error
	for _, p := range providers {
		prices, err := p.FetchPricing(ctx)
		if err != nil {
			slog.Warn("failed to fetch pricing from provider", "provider", p.Name(), "error", err)
			lastErr = err
			continue
		}

		providerName := strings.ToLower(p.Name())
		for modelID, pricing := range prices {
			// Store namespaced key: e.g. "openrouter:anthropic/claude-3.5-sonnet"
			key := fmt.Sprintf("%s:%s", providerName, modelID)
			mergedMap[key] = pricing

			// Store direct model key as fallback if not present
			if _, exists := mergedMap[modelID]; !exists {
				mergedMap[modelID] = pricing
			}
		}
	}

	// Atomically swap the pointer to the new map (zero locks on read path)
	o.pricingMap.Store(&mergedMap)
	return lastErr
}

// GetPrice looks up the pricing for a given provider and model. Lock-free.
func (o *PricingOracle) GetPrice(provider, model string) (ModelPricing, bool) {
	mPtr := o.pricingMap.Load()
	if mPtr == nil {
		return ModelPricing{}, false
	}
	m := *mPtr

	// Try namespaced lookup
	key := fmt.Sprintf("%s:%s", strings.ToLower(provider), model)
	if p, ok := m[key]; ok {
		return p, true
	}

	// Fallback to direct model ID
	if p, ok := m[model]; ok {
		return p, true
	}

	return ModelPricing{}, false
}

// CalculateCost calculates the total estimated USD cost for a given request. Lock-free.
func (o *PricingOracle) CalculateCost(provider, model string, promptTokens, completionTokens int) float64 {
	pricing, found := o.GetPrice(provider, model)
	if !found {
		return 0.0
	}

	promptCost := (float64(promptTokens) / 1_000_000.0) * pricing.PromptCostPerMillion
	completionCost := (float64(completionTokens) / 1_000_000.0) * pricing.CompletionCostPerMillion
	return promptCost + completionCost
}

// StartBackgroundSync periodically updates pricing in the background.
func (o *PricingOracle) StartBackgroundSync(ctx context.Context, interval time.Duration) {
	// Perform initial sync
	_ = o.Sync(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := o.Sync(ctx); err != nil {
					slog.Warn("background pricing sync encountered error", "error", err)
				}
			}
		}
	}()
}
