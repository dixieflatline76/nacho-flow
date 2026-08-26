package telemetry

import (
	"context"
	"math"
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
				"name": "Anthropic: Claude 3.5 Sonnet",
				"context_length": 200000,
				"pricing": {
					"prompt": "0.000003",
					"completion": "0.000015"
				},
				"architecture": {
					"input_modalities": ["text", "image"]
				},
				"supported_parameters": ["tools", "temperature"],
				"benchmarks": {
					"artificial_analysis": {
						"coding_index": 92.4,
						"agentic_index": 90.1
					}
				}
			},
			{
				"id": "google/gemini-2.5-flash-lite",
				"name": "Google: Gemini 2.5 Flash Lite",
				"context_length": 1048576,
				"pricing": {
					"prompt": "0.0000001",
					"completion": "0.0000004"
				},
				"architecture": {
					"input_modalities": ["text", "image"]
				},
				"supported_parameters": ["tools"],
				"expiration_date": "2026-12-31T23:59:59Z",
				"reasoning": {
					"mandatory": false
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
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	provider := NewOpenRouterPricingProviderWithURL(server.URL, "test-api-key")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	metaMap, err := provider.FetchPricing(ctx)
	if err != nil {
		t.Fatalf("unexpected error fetching pricing: %v", err)
	}

	// 1. Check Claude Sonnet
	claude, ok := metaMap["anthropic/claude-3.5-sonnet"]
	if !ok {
		t.Fatalf("expected anthropic/claude-3.5-sonnet in map")
	}
	if claude.PromptCostPerMillion != 3.0 {
		t.Errorf("expected prompt cost 3.0, got %f", claude.PromptCostPerMillion)
	}
	if claude.CompletionCostPerMillion != 15.0 {
		t.Errorf("expected completion cost 15.0, got %f", claude.CompletionCostPerMillion)
	}
	if !claude.SupportsTools {
		t.Errorf("expected claude to support tools")
	}
	if !claude.SupportsVision {
		t.Errorf("expected claude to support vision")
	}
	if claude.CodingIndex != 92.4 {
		t.Errorf("expected coding index 92.4, got %f", claude.CodingIndex)
	}

	// 2. Check Gemini Flash Lite
	gemini, ok := metaMap["google/gemini-2.5-flash-lite"]
	if !ok {
		t.Fatalf("expected google/gemini-2.5-flash-lite in map")
	}
	if math.Abs(gemini.PromptCostPerMillion-0.10) > 1e-4 {
		t.Errorf("expected prompt cost 0.10, got %f", gemini.PromptCostPerMillion)
	}
	if gemini.ContextLength != 1048576 {
		t.Errorf("expected 1048576 context length, got %d", gemini.ContextLength)
	}
	if !gemini.SupportsReasoning {
		t.Errorf("expected gemini reasoning support")
	}
	if gemini.ExpiresAt == nil || *gemini.ExpiresAt != "2026-12-31T23:59:59Z" {
		t.Errorf("expected expiration date")
	}
}

func TestOpenRouterPricingProvider_ErrorsAndEdgeCases(t *testing.T) {
	// 1. Server 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	provider := NewOpenRouterPricingProviderWithURL(server.URL, "test-api-key")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := provider.FetchPricing(ctx)
	if err == nil {
		t.Errorf("expected error on 500 status code, got nil")
	}

	// 2. Name & Default constructor
	defaultProvider := NewOpenRouterPricingProvider("test-key")
	if defaultProvider.Name() != "openrouter" {
		t.Errorf("Expected Name 'openrouter', got '%s'", defaultProvider.Name())
	}

	// 3. Invalid JSON returned
	badJSONServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not_valid_json"))
	}))
	defer badJSONServer.Close()

	badJSONProvider := NewOpenRouterPricingProviderWithURL(badJSONServer.URL, "test-key")
	_, errBadJSON := badJSONProvider.FetchPricing(ctx)
	if errBadJSON == nil {
		t.Errorf("expected error decoding invalid JSON, got nil")
	}

	// 4. Bad URL request creation error
	badURLProvider := NewOpenRouterPricingProviderWithURL("::invalid-url::", "")
	_, errBadURL := badURLProvider.FetchPricing(ctx)
	if errBadURL == nil {
		t.Errorf("expected error creating request with invalid URL")
	}
}
