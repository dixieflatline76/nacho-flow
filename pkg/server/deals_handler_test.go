package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

func TestDealsHandler_GET_Success(t *testing.T) {
	gallery := curation.NewManager(t.TempDir(), "")
	classifier := telemetry.NewClassifier(gallery)
	oracle := telemetry.NewPricingOracleWithClassifier(classifier)

	// Register a deal model
	mockP := &mockServerPricingProvider{
		name: "openrouter",
		prices: map[string]telemetry.ModelMetadata{
			"google/gemini-2.5-flash-lite": {
				ModelPricing:  telemetry.ModelPricing{PromptCostPerMillion: 0.10, CompletionCostPerMillion: 0.40},
				ModelID:       "google/gemini-2.5-flash-lite",
				Name:          "Google: Gemini 2.5 Flash Lite",
				ContextLength: 1048576,
				SupportsTools: true,
			},
		},
	}
	oracle.RegisterProvider(mockP, 0)
	_ = oracle.Sync(context.Background())

	cfg := &contract.Config{
		Deals: contract.DealsConfig{
			Enabled:           true,
			AlertThresholdPct: 50.0,
			MinCodingIndex:    40.0,
			RequireTools:      true,
		},
	}

	srv := &Server{
		oracle: oracle,
	}
	srv.state.Store(&runtimeState{
		config: cfg,
	})

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIDeals, nil)
	rec := httptest.NewRecorder()

	srv.handleAPIDeals(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	var resp DealsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.BenchmarkModel != contract.DefaultBenchmarkModel {
		t.Errorf("expected benchmark model %s, got %s", contract.DefaultBenchmarkModel, resp.BenchmarkModel)
	}

	if resp.DealsCount != 1 || len(resp.Deals) != 1 {
		t.Fatalf("expected 1 deal, got count=%d, len=%d", resp.DealsCount, len(resp.Deals))
	}

	if resp.Deals[0].ModelID != "google/gemini-2.5-flash-lite" {
		t.Errorf("expected gemini-2.5-flash-lite, got %s", resp.Deals[0].ModelID)
	}
}

func TestDealsHandler_MethodNotAllowedAndCORS(t *testing.T) {
	srv := &Server{
		oracle: telemetry.NewPricingOracle(),
	}
	srv.state.Store(&runtimeState{
		config: &contract.Config{},
	})

	// 1. POST request -> 405 Method Not Allowed
	reqPost := httptest.NewRequest(http.MethodPost, contract.PathAPIDeals, nil)
	recPost := httptest.NewRecorder()
	srv.handleAPIDeals(recPost, reqPost)
	if recPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", recPost.Code)
	}

	// 2. OPTIONS CORS request -> 200 OK with CORS headers
	reqOptions := httptest.NewRequest(http.MethodOptions, contract.PathAPIDeals, nil)
	recOptions := httptest.NewRecorder()
	srv.handleAPIDeals(recOptions, reqOptions)
	if recOptions.Code != http.StatusOK {
		t.Errorf("expected 200 OK for CORS OPTIONS, got %d", recOptions.Code)
	}
}

func TestDealsHandler_NilOracleAndEmpty(t *testing.T) {
	srv := &Server{
		oracle: nil, // Nil oracle safety
	}
	srv.state.Store(&runtimeState{
		config: &contract.Config{},
	})

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIDeals, nil)
	rec := httptest.NewRecorder()
	srv.handleAPIDeals(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with nil oracle, got %d", rec.Code)
	}

	var resp DealsResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.DealsCount != 0 || len(resp.Deals) != 0 {
		t.Errorf("expected 0 deals for nil oracle")
	}
}

type mockServerPricingProvider struct {
	name   string
	prices map[string]telemetry.ModelMetadata
}

func (m *mockServerPricingProvider) Name() string {
	return m.name
}

func (m *mockServerPricingProvider) FetchPricing(ctx context.Context) (map[string]telemetry.ModelMetadata, error) {
	return m.prices, nil
}
