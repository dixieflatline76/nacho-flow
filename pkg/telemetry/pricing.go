package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// ModelPricing represents USD cost per million tokens.
type ModelPricing struct {
	PromptCostPerMillion     float64 `json:"prompt_cost_per_million"`
	CompletionCostPerMillion float64 `json:"completion_cost_per_million"`
}

// ModelMetadata represents enriched token pricing and capability metadata for an LLM model.
type ModelMetadata struct {
	ModelPricing
	ModelID           string  `json:"model_id"`
	Provider          string  `json:"provider,omitempty"`
	Name              string  `json:"name"`
	ContextLength     int     `json:"context_length"`
	SupportsTools     bool    `json:"supports_tools"`
	SupportsVision    bool    `json:"supports_vision"`
	SupportsReasoning bool    `json:"supports_reasoning"`
	CodingIndex       float64 `json:"coding_index"`
	AgenticIndex      float64 `json:"agentic_index"`
	ExpiresAt         *string `json:"expires_at,omitempty"`
}

// PricingProvider is the plugin interface implemented by provider pricing fetchers.
type PricingProvider interface {
	Name() string
	FetchPricing(ctx context.Context) (map[string]ModelMetadata, error)
}

type providerEntry struct {
	provider PricingProvider
	interval time.Duration
	cancel   context.CancelFunc
}

// PricingOracle manages lock-free model pricing lookup, capability ranking, and async background sync.
type PricingOracle struct {
	providers   map[string]*providerEntry
	metadataMap atomic.Pointer[map[string]ModelMetadata]
	lastSynced  atomic.Int64
	classifier  *Classifier
	mu          sync.Mutex
	rootCtx     context.Context
	defaultInt  time.Duration
}

// NewPricingOracle creates a new initialized PricingOracle.
func NewPricingOracle() *PricingOracle {
	oracle := &PricingOracle{
		providers:  make(map[string]*providerEntry),
		classifier: NewClassifier(nil),
	}
	emptyMap := make(map[string]ModelMetadata)
	oracle.metadataMap.Store(&emptyMap)
	return oracle
}

// NewPricingOracleWithClassifier creates an initialized PricingOracle with a custom capability classifier.
func NewPricingOracleWithClassifier(classifier *Classifier) *PricingOracle {
	oracle := NewPricingOracle()
	if classifier != nil {
		oracle.classifier = classifier
	}
	return oracle
}

// SetClassifier configures the active capability classifier.
func (o *PricingOracle) SetClassifier(c *Classifier) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if c != nil {
		o.classifier = c
	}
}

// RegisterProvider registers or updates a pricing provider plugin with its sync interval.
// If a provider is re-registered (e.g. during config hot-reload), old runners are gracefully stopped.
func (o *PricingOracle) RegisterProvider(p PricingProvider, syncInterval time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()

	name := strings.ToLower(p.Name())
	if existing, exists := o.providers[name]; exists && existing.cancel != nil {
		existing.cancel() // Stop previous polling loop for hot-reload
	}

	entry := &providerEntry{
		provider: p,
		interval: syncInterval,
	}
	o.providers[name] = entry

	// If background sync is already active, launch runner immediately
	if o.rootCtx != nil {
		o.startProviderRunner(entry, o.defaultInt)
	}
}

// StartBackgroundSync launches background polling loops for all registered providers and sets the root lifecycle context.
func (o *PricingOracle) StartBackgroundSync(ctx context.Context, defaultInterval time.Duration) {
	o.mu.Lock()
	o.rootCtx = ctx
	o.defaultInt = defaultInterval
	entries := make([]*providerEntry, 0, len(o.providers))
	for _, entry := range o.providers {
		entries = append(entries, entry)
	}
	o.mu.Unlock()

	for _, entry := range entries {
		o.startProviderRunner(entry, defaultInterval)
	}
}

// startProviderRunner executes an independent polling loop for a registered provider.
func (o *PricingOracle) startProviderRunner(entry *providerEntry, defaultInterval time.Duration) {
	interval := defaultInterval
	if entry.interval > 0 {
		interval = entry.interval
	}

	provCtx, provCancel := context.WithCancel(o.rootCtx)
	entry.cancel = provCancel

	go func(p PricingProvider, tickInterval time.Duration, ctx context.Context) {
		// Initial sync
		if prices, err := p.FetchPricing(ctx); err == nil {
			o.updateProviderData(p.Name(), prices)
		} else {
			slog.Warn("initial pricing sync failed for provider", "provider", p.Name(), "error", err)
		}

		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if prices, err := p.FetchPricing(ctx); err == nil {
					o.updateProviderData(p.Name(), prices)
				} else {
					slog.Warn("background pricing sync failed for provider", "provider", p.Name(), "error", err)
				}
			}
		}
	}(entry.provider, interval, provCtx)
}

// updateProviderData atomically copies the active metadata map, applies the provider's updates, and swaps the pointer.
func (o *PricingOracle) updateProviderData(providerName string, newPrices map[string]ModelMetadata) {
	o.mu.Lock()
	defer o.mu.Unlock()

	currentPtr := o.metadataMap.Load()
	mergedMap := make(map[string]ModelMetadata)
	if currentPtr != nil {
		for k, v := range *currentPtr {
			mergedMap[k] = v
		}
	}

	for modelID, meta := range newPrices {
		if meta.ModelID == "" {
			meta.ModelID = modelID
		}
		if meta.Provider == "" {
			meta.Provider = strings.ToLower(providerName)
		}
		key := fmt.Sprintf("%s%s%s", strings.ToLower(providerName), contract.PricingNamespaceSeparator, modelID)
		mergedMap[key] = meta
		if _, exists := mergedMap[modelID]; !exists {
			mergedMap[modelID] = meta
		}
	}

	o.metadataMap.Store(&mergedMap)
	o.lastSynced.Store(time.Now().UnixNano())
}

// LastSynced returns the UTC timestamp of the most recent successful pricing sync.
func (o *PricingOracle) LastSynced() time.Time {
	nano := o.lastSynced.Load()
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano).UTC()
}

// Sync manually queries all registered providers and updates the pricing map.
func (o *PricingOracle) Sync(ctx context.Context) error {
	o.mu.Lock()
	providers := make([]PricingProvider, 0, len(o.providers))
	for _, entry := range o.providers {
		providers = append(providers, entry.provider)
	}
	o.mu.Unlock()

	var lastErr error
	for _, p := range providers {
		prices, err := p.FetchPricing(ctx)
		if err != nil {
			slog.Warn("failed to fetch pricing from provider", "provider", p.Name(), "error", err)
			lastErr = err
			continue
		}
		o.updateProviderData(p.Name(), prices)
	}
	return lastErr
}

// GetPrice looks up the pricing for a given provider and model. Lock-free $O(1)$.
func (o *PricingOracle) GetPrice(provider, model string) (ModelPricing, bool) {
	meta, found := o.GetModelMetadata(provider, model)
	if !found {
		return ModelPricing{}, false
	}
	return meta.ModelPricing, true
}

// GetModelMetadata looks up the enriched metadata for a given provider and model. Lock-free $O(1)$.
func (o *PricingOracle) GetModelMetadata(provider, model string) (ModelMetadata, bool) {
	mPtr := o.metadataMap.Load()
	if mPtr == nil {
		return ModelMetadata{}, false
	}
	m := *mPtr

	key := fmt.Sprintf("%s%s%s", strings.ToLower(provider), contract.PricingNamespaceSeparator, model)
	if meta, ok := m[key]; ok {
		return meta, true
	}

	if meta, ok := m[model]; ok {
		return meta, true
	}

	return ModelMetadata{}, false
}

// GetAllPricing returns a shallow copy of the active pricing map. Lock-free.
func (o *PricingOracle) GetAllPricing() map[string]ModelPricing {
	mPtr := o.metadataMap.Load()
	if mPtr == nil {
		return make(map[string]ModelPricing)
	}
	m := *mPtr
	copyMap := make(map[string]ModelPricing, len(m))
	for k, v := range m {
		copyMap[k] = v.ModelPricing
	}
	return copyMap
}

// CalculateCost calculates total estimated USD cost for prompt and completion tokens. Lock-free.
func (o *PricingOracle) CalculateCost(provider, model string, promptTokens, completionTokens int) float64 {
	pricing, found := o.GetPrice(provider, model)
	if !found {
		return 0.0
	}

	promptCost := (float64(promptTokens) / contract.TokensPerMillion) * pricing.PromptCostPerMillion
	completionCost := (float64(completionTokens) / contract.TokensPerMillion) * pricing.CompletionCostPerMillion
	return promptCost + completionCost
}

// resolveBenchmarkRates returns the prompt and completion per-million rates for the
// benchmark model. It checks live OpenRouter pricing data first (populated by background
// sync), and falls back to hardcoded snapshot constants if unavailable.
func (o *PricingOracle) resolveBenchmarkRates() (promptRate, completionRate float64) {
	if pricing, found := o.GetPrice(contract.ProviderOpenRouter, contract.DefaultBenchmarkModel); found {
		if pricing.PromptCostPerMillion > 0 {
			promptRate = pricing.PromptCostPerMillion
		}
		if pricing.CompletionCostPerMillion > 0 {
			completionRate = pricing.CompletionCostPerMillion
		}
	}
	if promptRate <= 0 {
		promptRate = contract.DefaultBenchmarkPromptPricePerMillion
	}
	if completionRate <= 0 {
		completionRate = contract.DefaultBenchmarkCompletionPricePerMillion
	}
	return
}

// CalculateFinancials computes dual financial telemetry: actual USD spent and estimated USD saved vs benchmark.
func (o *PricingOracle) CalculateFinancials(provider, model string, isLocal bool, promptTokens, completionTokens, cachedTokens int, upstreamCost float64, baselineRatePerM float64) (costSpent float64, costSaved float64) {
	// baselineRatePerM is deprecated; rates are resolved internally via resolveBenchmarkRates().
	_ = baselineRatePerM

	totalTokens := promptTokens + completionTokens
	if totalTokens <= 0 {
		return 0.0, 0.0
	}

	// Resolve benchmark prompt & completion rates from live OpenRouter data,
	// falling back to hardcoded snapshot if unavailable.
	bPrompt, bCompletion := o.resolveBenchmarkRates()
	baselineCost := (float64(promptTokens)/contract.TokensPerMillion)*bPrompt +
		(float64(completionTokens)/contract.TokensPerMillion)*bCompletion

	if isLocal {
		return 0.0, baselineCost
	}

	// Priority 1: Upstream ground-truth cost from OpenRouter usage.cost
	if upstreamCost > 0 {
		costSpent = upstreamCost
		if baselineCost > costSpent {
			costSaved = baselineCost - costSpent
		}
		return costSpent, costSaved
	}

	pricing, found := o.GetPrice(provider, model)
	if !found {
		return 0.0, 0.0
	}

	// Priority 2: Cache-aware calculation if cached tokens reported
	uncachedPromptTokens := promptTokens - cachedTokens
	if uncachedPromptTokens < 0 {
		uncachedPromptTokens = 0
	}

	// Heuristic: cached tokens pay 20% of prompt rate (80% discount).
	// Only two providers exist: Ollama (local=$0) and OpenRouter (cloud).
	// OpenRouter reports usage.cost (handled above), so this is a safety fallback.
	const cacheDiscountMultiplier = 0.20

	promptCost := (float64(uncachedPromptTokens) / contract.TokensPerMillion) * pricing.PromptCostPerMillion
	if cachedTokens > 0 {
		promptCost += (float64(cachedTokens) / contract.TokensPerMillion) * pricing.PromptCostPerMillion * cacheDiscountMultiplier
	}
	completionCost := (float64(completionTokens) / contract.TokensPerMillion) * pricing.CompletionCostPerMillion

	costSpent = promptCost + completionCost
	if baselineCost > costSpent {
		costSaved = baselineCost - costSpent
	}
	return costSpent, costSaved
}

// GetDeals scans the atomically cached models and returns top qualifying deals ranked by Quality-to-Price.
func (o *PricingOracle) GetDeals(cfg contract.DealsConfig, benchmarkCostPerM float64, limit int) []contract.DealInfo {
	if benchmarkCostPerM <= 0 {
		benchmarkCostPerM = contract.DefaultBenchmarkPricePerMillion
	}
	if limit <= 0 {
		limit = contract.DefaultDealsLimit
	}

	metaPtr := o.metadataMap.Load()
	if metaPtr == nil {
		return nil
	}

	// Apply sensible defaults when cfg has zero values (unconfigured)
	requireTools := cfg.RequireTools
	minCoding := cfg.MinCodingIndex
	alertThreshold := cfg.AlertThresholdPct
	if !cfg.RequireTools && cfg.MinCodingIndex == 0 && cfg.AlertThresholdPct == 0 {
		requireTools = true
		minCoding = contract.DefaultDealsMinCodingIndex
		alertThreshold = contract.DefaultDealsAlertThresholdPct
	}

	var deals []contract.DealInfo
	for modelID, meta := range *metaPtr {
		// Skip namespaced duplicate keys
		if strings.Contains(modelID, contract.PricingNamespaceSeparator) {
			continue
		}

		// Skip models with negative/unpriced metadata
		if meta.PromptCostPerMillion < 0 || meta.CompletionCostPerMillion < 0 {
			continue
		}

		// Multi-tier capability & role classification
		role, codingScore, recTiers := o.classifier.ClassifyModel(meta)

		if requireTools && !meta.SupportsTools {
			continue
		}

		if minCoding > 0 && codingScore > 0 && codingScore < minCoding {
			continue
		}

		var discountPct float64
		if benchmarkCostPerM > 0 {
			if meta.PromptCostPerMillion == 0 {
				discountPct = contract.DiscountFullFree
			} else if benchmarkCostPerM > meta.PromptCostPerMillion {
				discountPct = ((benchmarkCostPerM - meta.PromptCostPerMillion) / benchmarkCostPerM) * 100.0
			}
		}

		if alertThreshold > 0 && discountPct < alertThreshold {
			continue
		}

		providerName := meta.Provider
		if providerName == "" {
			providerName = contract.ProviderOpenRouter
		}

		deals = append(deals, contract.DealInfo{
			Provider:           providerName,
			ModelID:            modelID,
			Name:               meta.Name,
			ContextLength:      meta.ContextLength,
			PromptCostPerM:     meta.PromptCostPerMillion,
			CompletionCostPerM: meta.CompletionCostPerMillion,
			DiscountPct:        discountPct,
			IsFree:             meta.PromptCostPerMillion == 0 && meta.CompletionCostPerMillion == 0,
			SupportsTools:      meta.SupportsTools,
			SupportsVision:     meta.SupportsVision,
			SupportsReasoning:  meta.SupportsReasoning,
			TierRole:           string(role),
			CodingIndex:        codingScore,
			AgenticIndex:       meta.AgenticIndex,
			RecommendedTiers:   recTiers,
			ExpiresAt:          meta.ExpiresAt,
		})
	}

	// Sort by DiscountPct DESC, then CodingIndex DESC
	sort.Slice(deals, func(i, j int) bool {
		if deals[i].DiscountPct != deals[j].DiscountPct {
			return deals[i].DiscountPct > deals[j].DiscountPct
		}
		return deals[i].CodingIndex > deals[j].CodingIndex
	})

	if len(deals) > limit {
		deals = deals[:limit]
	}
	return deals
}
