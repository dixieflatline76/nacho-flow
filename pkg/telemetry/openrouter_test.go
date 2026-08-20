package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenRouterPricingProvider_FetchPricing_Success(t *testing.T) {
	mockResponse := `{
		"data": [
			{
				"id": "anthropic/claude-3.5-sonnet",
				"pricing": {
					"prompt": "0.000003",
					"completion": "0.000015"
				}
			},
			{
				"id": "meta-llama/llama-3.1-8b-instruct",
				"pricing": {
					"prompt": "0.000000055",
					"completion": "0.000000055"
				}
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	provider := NewOpenRouterPricingProviderWithURL(server.URL, "test-api-key")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pricingMap, err := provider.FetchPricing(ctx)
	if err != nil {
		t.Fatalf("unexpected error fetching pricing: %v", err)
	}

	// claude-3.5-sonnet: prompt $3.00/1M, completion $15.00/1M
	claude, ok := pricingMap["anthropic/claude-3.5-sonnet"]
	if !ok {
		t.Fatalf("expected anthropic/claude-3.5-sonnet in pricing map")
	}
	if claude.PromptCostPerMillion != 3.0 {
		t.Errorf("expected prompt cost 3.0, got %f", claude.PromptCostPerMillion)
	}
	if claude.CompletionCostPerMillion != 15.0 {
		t.Errorf("expected completion cost 15.0, got %f", claude.CompletionCostPerMillion)
	}

	// llama-3.1-8b: prompt $0.055/1M, completion $0.055/1M
	llama, ok := pricingMap["meta-llama/llama-3.1-8b-instruct"]
	if !ok {
		t.Fatalf("expected meta-llama/llama-3.1-8b-instruct in pricing map")
	}
	if llama.PromptCostPerMillion != 0.055 {
		t.Errorf("expected prompt cost 0.055, got %f", llama.PromptCostPerMillion)
	}
}

func TestOpenRouterPricingProvider_FetchPricing_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	provider := NewOpenRouterPricingProviderWithURL(server.URL, "test-api-key")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := provider.FetchPricing(ctx)
	if err == nil {
		t.Errorf("expected error on 500 status code, got nil")
	}

	// Test default constructor and Name()
	defaultProvider := NewOpenRouterPricingProvider("test-key")
	if defaultProvider.Name() != "openrouter" {
		t.Errorf("Expected Name 'openrouter', got '%s'", defaultProvider.Name())
	}

	// Case: Invalid JSON returned with 200 OK
	badJSONServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not_valid_json"))
	}))
	defer badJSONServer.Close()

	badProvider := NewOpenRouterPricingProviderWithURL(badJSONServer.URL, "")
	_, err = badProvider.FetchPricing(context.Background())
	if err == nil {
		t.Errorf("Expected error decoding invalid JSON")
	}

	// Case: Dead server connection failure
	deadProvider := NewOpenRouterPricingProviderWithURL("http://127.0.0.1:54322", "")
	_, err = deadProvider.FetchPricing(context.Background())
	if err == nil {
		t.Errorf("Expected connection error for unreachable host")
	}
}
