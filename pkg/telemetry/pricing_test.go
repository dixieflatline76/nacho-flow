package telemetry

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

// mockPricingProvider implements PricingProvider for testing.
type mockPricingProvider struct {
	mu     sync.RWMutex
	name   string
	prices map[string]ModelMetadata
	err    error
}

func (m *mockPricingProvider) Name() string {
	return m.name
}

func (m *mockPricingProvider) SetPrices(p map[string]ModelMetadata) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prices = p
}

func (m *mockPricingProvider) FetchPricing(ctx context.Context) (map[string]ModelMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.err != nil {
		return nil, m.err
	}
	cp := make(map[string]ModelMetadata, len(m.prices))
	for k, v := range m.prices {
		cp[k] = v
	}
	return cp, nil
}

func TestPricingOracle_RegistrationAndNamespacing(t *testing.T) {
	oracle := NewPricingOracle()

	provider1 := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"qwen/qwen-2.5-coder-32b": {
				ModelPricing:  ModelPricing{PromptCostPerMillion: 0.15, CompletionCostPerMillion: 0.60},
				ModelID:       "qwen/qwen-2.5-coder-32b",
				Name:          "Qwen 2.5 Coder 32B",
				SupportsTools: true,
			},
		},
	}

	provider2 := &mockPricingProvider{
		name: "deepseek",
		prices: map[string]ModelMetadata{
			"deepseek-coder": {
				ModelPricing:  ModelPricing{PromptCostPerMillion: 0.14, CompletionCostPerMillion: 0.28},
				ModelID:       "deepseek-coder",
				Name:          "DeepSeek Coder",
				SupportsTools: true,
			},
		},
	}

	oracle.RegisterProvider(provider1, 0)
	oracle.RegisterProvider(provider2, 0)

	err := oracle.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected sync error: %v", err)
	}

	// Lookup via provider and model
	pricing1, found := oracle.GetPrice("openrouter", "qwen/qwen-2.5-coder-32b")
	if !found {
		t.Fatalf("expected to find openrouter:qwen/qwen-2.5-coder-32b")
	}
	if pricing1.PromptCostPerMillion != 0.15 || pricing1.CompletionCostPerMillion != 0.60 {
		t.Errorf("unexpected pricing values: %+v", pricing1)
	}

	meta1, metaFound := oracle.GetModelMetadata("openrouter", "qwen/qwen-2.5-coder-32b")
	if !metaFound || !meta1.SupportsTools {
		t.Errorf("expected metadata with tool support")
	}

	pricing2, found := oracle.GetPrice("deepseek", "deepseek-coder")
	if !found {
		t.Fatalf("expected to find deepseek:deepseek-coder")
	}
	if pricing2.PromptCostPerMillion != 0.14 {
		t.Errorf("unexpected deepseek pricing: %+v", pricing2)
	}

	// Calculate cost: 1,000 prompt tokens + 500 completion tokens
	// Prompt cost: 1,000 / 1,000,000 * 0.15 = $0.00015
	// Completion cost: 500 / 1,000,000 * 0.60 = $0.00030
	// Total: $0.00045
	cost := oracle.CalculateCost("openrouter", "qwen/qwen-2.5-coder-32b", 1000, 500)
	expectedCost := 0.00045
	if fmt.Sprintf("%.6f", cost) != fmt.Sprintf("%.6f", expectedCost) {
		t.Errorf("expected cost %f, got %f", expectedCost, cost)
	}
}

func TestPricingOracle_AsyncProviderPolling_COW(t *testing.T) {
	gallery := curation.NewManager(t.TempDir(), "")
	classifier := NewClassifier(gallery)
	oracle := NewPricingOracleWithClassifier(classifier)

	providerFast := &mockPricingProvider{
		name: "fast_provider",
		prices: map[string]ModelMetadata{
			"model-fast": {
				ModelPricing:  ModelPricing{PromptCostPerMillion: 0.10, CompletionCostPerMillion: 0.20},
				ModelID:       "model-fast",
				SupportsTools: true,
			},
		},
	}

	providerSlow := &mockPricingProvider{
		name: "slow_provider",
		prices: map[string]ModelMetadata{
			"model-slow": {
				ModelPricing:  ModelPricing{PromptCostPerMillion: 0.50, CompletionCostPerMillion: 1.00},
				ModelID:       "model-slow",
				SupportsTools: true,
			},
		},
	}

	oracle.RegisterProvider(providerFast, 10*time.Millisecond)
	oracle.RegisterProvider(providerSlow, 30*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	oracle.StartBackgroundSync(ctx, 50*time.Millisecond)

	// Wait for background loops to populate
	time.Sleep(45 * time.Millisecond)

	_, foundFast := oracle.GetPrice("fast_provider", "model-fast")
	_, foundSlow := oracle.GetPrice("slow_provider", "model-slow")

	if !foundFast || !foundSlow {
		t.Errorf("expected both fast and slow provider models to be present via COW merging (fast=%v, slow=%v)", foundFast, foundSlow)
	}

	cancel()
}

func TestPricingOracle_DynamicHotReload(t *testing.T) {
	oracle := NewPricingOracle()
	p1 := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"initial-model": {ModelPricing: ModelPricing{PromptCostPerMillion: 1.0}, ModelID: "initial-model"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oracle.RegisterProvider(p1, 50*time.Millisecond)
	oracle.StartBackgroundSync(ctx, 100*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	// Hot reload with updated prices and new sync interval
	p1Updated := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"reloaded-model": {ModelPricing: ModelPricing{PromptCostPerMillion: 0.20}, ModelID: "reloaded-model"},
		},
	}
	oracle.RegisterProvider(p1Updated, 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	_, foundReloaded := oracle.GetPrice("openrouter", "reloaded-model")
	if !foundReloaded {
		t.Errorf("expected reloaded model to be populated dynamically on hot-reload")
	}
}

func TestPricingOracle_GetDeals_QualityFilteringAndRanking(t *testing.T) {
	gallery := curation.NewManager(t.TempDir(), "")
	classifier := NewClassifier(gallery)
	oracle := NewPricingOracleWithClassifier(classifier)

	// Setup 5 diverse models:
	// 1. Gemini 2.5 Flash Lite: 96.7% discount, 68.1 coding index, tools
	// 2. Claude 3.5 Sonnet: 0% discount (baseline benchmark $3.00), 92.4 coding index
	// 3. Cheap non-tool model: 98% discount, 0 coding index, no tools
	// 4. Free coding model: 100% discount, 75.0 coding index, tools
	// 5. Cheap sub-threshold translation model: 90% discount, 20.0 coding index
	models := map[string]ModelMetadata{
		"google/gemini-2.5-flash-lite": {
			ModelPricing:  ModelPricing{PromptCostPerMillion: 0.10, CompletionCostPerMillion: 0.40},
			ModelID:       "google/gemini-2.5-flash-lite",
			Name:          "Google: Gemini 2.5 Flash Lite",
			ContextLength: 1048576,
			SupportsTools: true,
			CodingIndex:   68.1,
		},
		"anthropic/claude-3.5-sonnet": {
			ModelPricing:  ModelPricing{PromptCostPerMillion: 3.00, CompletionCostPerMillion: 15.00},
			ModelID:       "anthropic/claude-3.5-sonnet",
			Name:          "Anthropic: Claude 3.5 Sonnet",
			ContextLength: 200000,
			SupportsTools: true,
			CodingIndex:   92.4,
		},
		"small-org/no-tools-cheap": {
			ModelPricing:  ModelPricing{PromptCostPerMillion: 0.05, CompletionCostPerMillion: 0.05},
			ModelID:       "small-org/no-tools-cheap",
			Name:          "No Tools Cheap",
			SupportsTools: false,
			CodingIndex:   0,
		},
		"dots-studio/dots-3-note:free": {
			ModelPricing:  ModelPricing{PromptCostPerMillion: 0.00, CompletionCostPerMillion: 0.00},
			ModelID:       "dots-studio/dots-3-note:free",
			Name:          "Dots 3 Note Free",
			SupportsTools: true,
			CodingIndex:   75.0,
		},
		"sub/low-quality-translation": {
			ModelPricing:  ModelPricing{PromptCostPerMillion: 0.20, CompletionCostPerMillion: 0.20},
			ModelID:       "sub/low-quality-translation",
			Name:          "Low Quality Translation",
			SupportsTools: true,
			CodingIndex:   20.0,
		},
	}

	p := &mockPricingProvider{
		name:   "openrouter",
		prices: models,
	}
	oracle.RegisterProvider(p, 0)
	_ = oracle.Sync(context.Background())

	dealsCfg := contract.DealsConfig{
		Enabled:           true,
		AlertThresholdPct: 50.0,
		MinCodingIndex:    40.0,
		RequireTools:      true,
	}

	deals := oracle.GetDeals(dealsCfg, 3.00, 10)
	if len(deals) != 2 {
		t.Fatalf("expected exactly 2 qualifying deals (free model and gemini-flash-lite), got %d", len(deals))
	}

	// 1st place should be Free model (100% discount)
	if deals[0].ModelID != "dots-studio/dots-3-note:free" || !deals[0].IsFree || deals[0].DiscountPct != 100.0 {
		t.Errorf("expected free model at rank 1, got %+v", deals[0])
	}

	// 2nd place should be Gemini 2.5 Flash Lite (~96.67% discount)
	if deals[1].ModelID != "google/gemini-2.5-flash-lite" || deals[1].DiscountPct < 96.0 {
		t.Errorf("expected flash lite at rank 2, got %+v", deals[1])
	}
}

func TestPricingOracle_GetDeals_ZeroSafeAndDefaults(t *testing.T) {
	oracle := NewPricingOracle()
	oracle.SetClassifier(NewClassifier(nil))

	// Uninitialized oracle -> returns nil
	rawOracle := &PricingOracle{}
	if deals := rawOracle.GetDeals(contract.DealsConfig{}, 0, 0); deals != nil {
		t.Errorf("expected nil deals for empty oracle")
	}

	// Zero rates with default benchmark price
	p := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"free-model": {
				ModelPricing:  ModelPricing{PromptCostPerMillion: 0, CompletionCostPerMillion: 0},
				ModelID:       "free-model",
				SupportsTools: true,
			},
		},
	}
	oracle.RegisterProvider(p, 0)
	_ = oracle.Sync(context.Background())

	deals := oracle.GetDeals(contract.DealsConfig{}, 0, 5)
	if len(deals) != 1 || deals[0].DiscountPct != 100.0 {
		t.Errorf("expected 100%% discount for zero rate model with default benchmark")
	}
}

func TestPricingOracle_CalculateCost_NotFoundAndFallbacks(t *testing.T) {
	oracle := NewPricingOracle()
	cost := oracle.CalculateCost("unknown", "unknown-model", 1000, 1000)
	if cost != 0.0 {
		t.Errorf("Expected 0.0 cost for unknown model, got %f", cost)
	}

	provider := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"claude-sonnet": {
				ModelPricing: ModelPricing{PromptCostPerMillion: 3.0, CompletionCostPerMillion: 15.0},
				ModelID:      "claude-sonnet",
			},
		},
	}
	oracle.RegisterProvider(provider, 0)
	_ = oracle.Sync(context.Background())

	// Fallback direct model lookup
	price, found := oracle.GetPrice("some_other_provider", "claude-sonnet")
	if !found || price.PromptCostPerMillion != 3.0 {
		t.Errorf("Expected fallback direct model lookup to succeed, got found=%v, price=%+v", found, price)
	}
}

func TestPricingOracle_CalculateFinancials(t *testing.T) {
	oracle := NewPricingOracle()

	// 1. Zero tokens
	spent, saved := oracle.CalculateFinancials("openrouter", "qwen", false, 0, 0, 0, 0.0, 3.0)
	if spent != 0 || saved != 0 {
		t.Errorf("expected 0,0 for zero tokens, got spent=%f, saved=%f", spent, saved)
	}

	// 2. Local provider (100% free)
	spent, saved = oracle.CalculateFinancials("ollama", "qwen", true, 10000, 2000, 0, 0.0, 3.0)
	// Baseline: (10000/1M)*$2.00 + (2000/1M)*$10.00 = $0.020 + $0.020 = $0.040
	expectedSaved := (10000.0/1_000_000.0)*2.0 + (2000.0/1_000_000.0)*10.0
	if fmt.Sprintf("%.6f", saved) != fmt.Sprintf("%.6f", expectedSaved) {
		t.Errorf("expected %f saved for local, got %f", expectedSaved, saved)
	}

	// 3. Cloud provider with asymmetric prompt ($0.30/1M) and completion ($1.50/1M)
	provider := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"qwen-coder": {
				ModelPricing: ModelPricing{PromptCostPerMillion: 0.30, CompletionCostPerMillion: 1.50},
				ModelID:      "qwen-coder",
			},
		},
	}
	oracle.RegisterProvider(provider, 0)
	_ = oracle.Sync(context.Background())

	// 50,000 prompt tokens + 1,000 completion tokens
	// Prompt cost: (50,000 / 1M) * 0.30 = $0.015
	// Completion cost: (1,000 / 1M) * 1.50 = $0.0015
	// Total Spent: $0.0165
	// Baseline: (50000/1M)*$2.00 + (1000/1M)*$10.00 = $0.10 + $0.01 = $0.11
	// Saved: $0.11 - $0.0165 = $0.0935
	spent, saved = oracle.CalculateFinancials("openrouter", "qwen-coder", false, 50000, 1000, 0, 0.0, 3.0)
	if fmt.Sprintf("%.4f", spent) != "0.0165" {
		t.Errorf("expected 0.0165 spent, got %f", spent)
	}
	if fmt.Sprintf("%.4f", saved) != "0.0935" {
		t.Errorf("expected 0.0935 saved, got %f", saved)
	}

	// 4. Upstream cost override (OpenRouter ground truth)
	spent, saved = oracle.CalculateFinancials("openrouter", "qwen-coder", false, 50000, 1000, 0, 0.05, 3.0)
	if fmt.Sprintf("%.4f", spent) != "0.0500" {
		t.Errorf("upstream cost override: expected 0.0500 spent, got %f", spent)
	}
	// Baseline: (50000/1M)*$2.00 + (1000/1M)*$10.00 = $0.10 + $0.01 = $0.11
	expectedBaseline := (50000.0/1_000_000.0)*2.0 + (1000.0/1_000_000.0)*10.0
	expectedSavedUpstream := expectedBaseline - 0.05
	if fmt.Sprintf("%.4f", saved) != fmt.Sprintf("%.4f", expectedSavedUpstream) {
		t.Errorf("upstream cost override: expected %f saved, got %f", expectedSavedUpstream, saved)
	}

	// 5. Cache-aware discount (35k of 50k prompt tokens cached, 80% discount on cached)
	// Prompt: uncached = 15000 @ $0.30/1M = $0.0045
	//         cached   = 35000 @ $0.30/1M * 0.20 = $0.0021
	// Completion: 1000 @ $1.50/1M = $0.0015
	// Total spent: $0.0081
	// Baseline: (50000/1M)*$2.00 + (1000/1M)*$10.00 = $0.10 + $0.01 = $0.11
	// Saved: $0.11 - $0.0081 = $0.1019
	spent, saved = oracle.CalculateFinancials("openrouter", "qwen-coder", false, 50000, 1000, 35000, 0.0, 3.0)
	if fmt.Sprintf("%.4f", spent) != "0.0081" {
		t.Errorf("cache discount: expected 0.0081 spent, got %f", spent)
	}
	if fmt.Sprintf("%.4f", saved) != "0.1019" {
		t.Errorf("cache discount: expected 0.1019 saved, got %f", saved)
	}

	// 6. Live benchmark rates override snapshot defaults
	benchProvider := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"qwen-coder": {
				ModelPricing: ModelPricing{PromptCostPerMillion: 0.30, CompletionCostPerMillion: 1.50},
				ModelID:      "qwen-coder",
			},
			contract.DefaultBenchmarkModel: {
				ModelPricing: ModelPricing{PromptCostPerMillion: 2.00, CompletionCostPerMillion: 10.00},
				ModelID:      contract.DefaultBenchmarkModel,
			},
		},
	}
	oracleLive := NewPricingOracle()
	oracleLive.RegisterProvider(benchProvider, 0)
	_ = oracleLive.Sync(context.Background())

	// 100k prompt + 10k completion routed to qwen-coder, upstream cost = $0.05
	// Baseline (live rates): (100000/1M)*$2.00 + (10000/1M)*$10.00 = $0.20 + $0.10 = $0.30
	// Saved: $0.30 - $0.05 = $0.25
	spent, saved = oracleLive.CalculateFinancials("openrouter", "qwen-coder", false, 100000, 10000, 0, 0.05, 0.0)
	if fmt.Sprintf("%.4f", spent) != "0.0500" {
		t.Errorf("live benchmark: expected 0.0500 spent, got %f", spent)
	}
	if fmt.Sprintf("%.4f", saved) != "0.2500" {
		t.Errorf("live benchmark: expected 0.2500 saved, got %f", saved)
	}
}

func TestPricingOracle_SyncErrorAndNilMap(t *testing.T) {
	oracle := NewPricingOracle()
	errProvider := &mockPricingProvider{
		name: "failing_provider",
		err:  fmt.Errorf("simulated API outage"),
	}
	oracle.RegisterProvider(errProvider, 0)

	err := oracle.Sync(context.Background())
	if err == nil {
		t.Errorf("Expected error from sync with failing provider, got nil")
	}

	rawOracle := &PricingOracle{}
	_, found := rawOracle.GetPrice("any", "model")
	if found {
		t.Errorf("Expected false for uninitialized oracle")
	}
	if all := rawOracle.GetAllPricing(); len(all) != 0 {
		t.Errorf("Expected empty map from uninitialized oracle")
	}
}

func TestPricingOracle_GetAllPricing(t *testing.T) {
	oracle := NewPricingOracle()
	provider := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"qwen": {
				ModelPricing: ModelPricing{PromptCostPerMillion: 0.2, CompletionCostPerMillion: 0.6},
				ModelID:      "qwen",
			},
		},
	}
	oracle.RegisterProvider(provider, 0)
	_ = oracle.Sync(context.Background())

	all := oracle.GetAllPricing()
	if len(all) == 0 {
		t.Fatalf("expected non-empty pricing map")
	}
	if all["openrouter::qwen"].PromptCostPerMillion != 0.2 {
		t.Errorf("expected price 0.2, got %f", all["openrouter::qwen"].PromptCostPerMillion)
	}
}
