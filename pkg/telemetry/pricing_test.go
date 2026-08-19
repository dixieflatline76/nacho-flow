package telemetry

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockPricingProvider implements PricingProvider for testing.
type mockPricingProvider struct {
	mu     sync.RWMutex
	name   string
	prices map[string]ModelPricing
	err    error
}

func (m *mockPricingProvider) Name() string {
	return m.name
}

func (m *mockPricingProvider) SetPrices(p map[string]ModelPricing) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prices = p
}

func (m *mockPricingProvider) FetchPricing(ctx context.Context) (map[string]ModelPricing, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.err != nil {
		return nil, m.err
	}
	cp := make(map[string]ModelPricing, len(m.prices))
	for k, v := range m.prices {
		cp[k] = v
	}
	return cp, nil
}

func TestPricingOracle_RegistrationAndNamespacing(t *testing.T) {
	oracle := NewPricingOracle()

	provider1 := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelPricing{
			"qwen/qwen-2.5-coder-32b": {PromptCostPerMillion: 0.15, CompletionCostPerMillion: 0.60},
		},
	}

	provider2 := &mockPricingProvider{
		name: "deepseek",
		prices: map[string]ModelPricing{
			"deepseek-coder": {PromptCostPerMillion: 0.14, CompletionCostPerMillion: 0.28},
		},
	}

	oracle.RegisterProvider(provider1)
	oracle.RegisterProvider(provider2)

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

func TestPricingOracle_AtomicSwap_Race(t *testing.T) {
	oracle := NewPricingOracle()

	provider := &mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelPricing{
			"model-a": {PromptCostPerMillion: 1.0, CompletionCostPerMillion: 2.0},
		},
	}
	oracle.RegisterProvider(provider)
	_ = oracle.Sync(context.Background())

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	// 50 concurrent readers reading continuously
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopChan:
					return
				default:
					_, _ = oracle.GetPrice("openrouter", "model-a")
					_ = oracle.CalculateCost("openrouter", "model-a", 1000, 2000)
				}
			}
		}()
	}

	// 5 concurrent writers updating pricing simultaneously
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				provider.SetPrices(map[string]ModelPricing{
					"model-a": {
						PromptCostPerMillion:     float64(workerID*10 + j),
						CompletionCostPerMillion: float64(workerID*10 + j*2),
					},
				})
				_ = oracle.Sync(context.Background())
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	close(stopChan)
	wg.Wait()
}
