package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestGetModelsEndpoint(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	evaluator, _ := strategy.NewExprEvaluator(nil, contract.Tier{Model: "default"})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/models", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		t.Fatalf("Expected non-empty data array in models response")
	}

	firstModel := data[0].(map[string]interface{})
	if firstModel["id"] != "nacho-hybrid" {
		t.Errorf("Expected model ID 'nacho-hybrid', got %v", firstModel["id"])
	}
}

func TestHealthEndpoint(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	evaluator, _ := strategy.NewExprEvaluator(nil, contract.Tier{Model: "default"})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %v", resp["status"])
	}
}

func TestStatsEndpoint(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	evaluator, _ := strategy.NewExprEvaluator(nil, contract.Tier{Model: "default"})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/stats", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse /v1/stats JSON: %v", err)
	}

	if _, ok := resp["started_at"]; !ok {
		t.Errorf("Expected started_at in stats output")
	}
}

// Test 3.1: Dynamic Provider with Langdock custom Auth and Headers
func TestProxy_DynamicProvider_LangdockAuthAndHeaders(t *testing.T) {
	var capturedAuth string
	var capturedCustomHeader string

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedCustomHeader = r.Header.Get("X-Langdock-Org")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"cmpl-langdock","choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"langdock": {
				BaseURL: mockUpstream.URL,
				APIKey:  "secret-langdock-token",
				Type:    "cloud",
				Headers: map[string]string{
					"X-Langdock-Org": "engineering-dept",
				},
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Langdock Cloud",
				Model:    "claude-3-5-sonnet",
				Provider: "langdock",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{"model": "nacho-hybrid", "messages": [{"role": "user", "content": "Langdock test"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	if capturedAuth != "Bearer secret-langdock-token" {
		t.Errorf("Expected Auth header 'Bearer secret-langdock-token', got '%s'", capturedAuth)
	}

	if capturedCustomHeader != "engineering-dept" {
		t.Errorf("Expected X-Langdock-Org header 'engineering-dept', got '%s'", capturedCustomHeader)
	}
}

// Test 3.2: Dynamic Provider with OpenRouter Headers
func TestProxy_DynamicProvider_OpenRouterHeaders(t *testing.T) {
	var capturedAuth, capturedReferer, capturedTitle string

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedReferer = r.Header.Get("HTTP-Referer")
		capturedTitle = r.Header.Get("X-Title")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"cmpl-or","choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {
				BaseURL: mockUpstream.URL,
				APIKey:  "sk-or-token-xyz",
				Headers: map[string]string{
					"HTTP-Referer": "https://spicebox.dev",
					"X-Title":      "nacho-flow",
				},
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Cloud Tier",
				Model:    "deepseek-r1",
				Provider: "openrouter",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{"model": "nacho-hybrid", "messages": [{"role": "user", "content": "OpenRouter test"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	if capturedAuth != "Bearer sk-or-token-xyz" {
		t.Errorf("Expected Auth 'Bearer sk-or-token-xyz', got '%s'", capturedAuth)
	}
	if capturedReferer != "https://spicebox.dev" {
		t.Errorf("Expected HTTP-Referer 'https://spicebox.dev', got '%s'", capturedReferer)
	}
	if capturedTitle != "nacho-flow" {
		t.Errorf("Expected X-Title 'nacho-flow', got '%s'", capturedTitle)
	}
}

// Test 3.3: Dynamic Provider with Local GPU (Zero Auth, Tagged as Local)
func TestProxy_DynamicProvider_LocalGPU_ZeroAuth(t *testing.T) {
	var capturedAuth string

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"cmpl-local","choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer mockUpstream.Close()

	tracker := telemetry.NewStatsTracker(10)
	defer tracker.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {
				BaseURL: mockUpstream.URL,
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local GPU",
				Model:    "qwen2.5-coder:14b",
				Provider: "local_gpu",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServerWithTelemetry(cfg, evaluator, classifier, sanitizer, nil, tracker, nil)

	reqPayload := `{"model": "nacho-hybrid", "messages": [{"role": "user", "content": "Local GPU test"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	if capturedAuth != "" {
		t.Errorf("Expected no Authorization header for local provider, got '%s'", capturedAuth)
	}

	tracker.Flush()
	stats := tracker.GetStats()
	if stats.TierBreakdown.Tier1LocalFree != 1 {
		t.Errorf("Expected Tier1LocalFree to be 1, got %d", stats.TierBreakdown.Tier1LocalFree)
	}
}

// Test 3.4: Unknown Provider returns 502 without panicking
func TestProxy_UnknownProvider_Returns502WithoutPanic(t *testing.T) {
	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"valid_provider": {
				BaseURL: "http://127.0.0.1:9999",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Broken Tier",
				Model:    "test-model",
				Provider: "missing_provider",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{"model": "nacho-hybrid", "messages": [{"role": "user", "content": "Test"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("Expected status 502 Bad Gateway for missing provider, got %d", rec.Code)
	}
}
