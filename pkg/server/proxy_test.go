package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
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

	for _, endpoint := range []string{"/health", "/v1/health"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", endpoint, nil)
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Endpoint %s: Expected status 200, got %d", endpoint, rec.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Endpoint %s: Failed to parse JSON: %v", endpoint, err)
		}
		if resp["status"] != "ok" {
			t.Errorf("Endpoint %s: Expected status 'ok', got %v", endpoint, resp["status"])
		}
		if resp["service"] != "nacho-flow" {
			t.Errorf("Endpoint %s: Expected service 'nacho-flow', got %v", endpoint, resp["service"])
		}
		if resp["version"] == nil || resp["version"] == "" {
			t.Errorf("Endpoint %s: Expected non-empty version, got %v", endpoint, resp["version"])
		}
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

// Test 3.5: Local model emitting markdown tool-calling fence is normalized to OpenAI tool_calls
func TestProxy_LocalModel_MarkdownToolCallNormalization(t *testing.T) {
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Model outputs JSON inside markdown fence instead of native tool_calls
		responseJSON := `{
			"id": "chatcmpl-local-123",
			"choices": [{
				"finish_reason": "stop",
				"message": {
					"role": "assistant",
					"content": "I will read the file for you.\n` + "```json" + `\n{\"name\": \"read_file\", \"arguments\": {\"path\": \"main.go\"}}\n` + "```" + `"
				}
			}]
		}`
		_, _ = w.Write([]byte(responseJSON))
	}))
	defer mockOllama.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: mockOllama.URL,
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local Tier",
				Model:    "qwen2.5-coder:14b",
				Provider: "ollama",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	// Request with tools array
	reqPayload := `{
		"model": "nacho-hybrid",
		"messages": [{"role": "user", "content": "read main.go"}],
		"tools": [{"type": "function", "function": {"name": "read_file"}}]
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", rec.Code)
	}

	var parsedResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsedResp); err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	choices := parsedResp["choices"].([]interface{})
	firstChoice := choices[0].(map[string]interface{})
	finishReason := firstChoice["finish_reason"].(string)
	if finishReason != "tool_calls" {
		t.Errorf("Expected finish_reason to be 'tool_calls', got '%s'", finishReason)
	}

	msg := firstChoice["message"].(map[string]interface{})
	toolCalls, hasTools := msg["tool_calls"].([]interface{})
	if !hasTools || len(toolCalls) == 0 {
		t.Fatalf("Expected normalized tool_calls array, got none")
	}

	firstCall := toolCalls[0].(map[string]interface{})
	fn := firstCall["function"].(map[string]interface{})
	if fn["name"] != "read_file" {
		t.Errorf("Expected function name 'read_file', got '%s'", fn["name"])
	}
}

// Test 3.5: Dynamic Pricing Savings Calculation via PricingOracle
func TestProxy_DynamicPricingSavings_CalculatedFromOracle(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cmpl-price","choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock_local": {BaseURL: mockUpstream.URL, Type: "local"},
		},
		Tiers: []contract.Tier{
			{Name: "Local GPU", Model: "qwen2.5-coder:14b", Provider: "mock_local", When: "Tokens < 16000"},
		},
		DefaultTier: contract.Tier{Name: "Default", Model: "qwen2.5-coder:14b", Provider: "mock_local"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()

	oracle := telemetry.NewPricingOracle()
	// Mock OpenRouter pricing server
	mockORServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "anthropic/claude-3.5-sonnet", "pricing": {"prompt": "0.000003", "completion": "0.000015"}},
				{"id": "deepseek/deepseek-r1", "pricing": {"prompt": "0.00000055", "completion": "0.00000219"}}
			]
		}`))
	}))
	defer mockORServer.Close()

	orProvider := telemetry.NewOpenRouterPricingProviderWithURL(mockORServer.URL, "test-key")
	oracle.RegisterProvider(orProvider, 0)
	_ = oracle.Sync(context.Background())

	tracker := telemetry.NewStatsTracker(100)
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, classifier, sanitizer, oracle, tracker, nil, slog.Default())

	// Send 4000-character prompt (~1000 tokens)
	longPrompt := strings.Repeat("abcd ", 1000)
	reqPayload := fmt.Sprintf(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"%s"}]}`, longPrompt)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", rec.Code)
	}

	tracker.Flush()
	stats := tracker.GetStats()
	// 1000 tokens saved on local GPU @ $3.00/1M = $0.003
	if stats.EstimatedCostSavedUSD <= 0 {
		t.Errorf("Expected positive EstimatedCostSavedUSD from real-time pricing oracle, got %f", stats.EstimatedCostSavedUSD)
	}
}

// Test singleJoiningSlash all branches
func TestProxy_SingleJoiningSlash(t *testing.T) {
	cases := []struct {
		a, b, expected string
	}{
		{"http://localhost:8000/", "/v1/chat", "http://localhost:8000/v1/chat"},
		{"http://localhost:8000", "v1/chat", "http://localhost:8000/v1/chat"},
		{"http://localhost:8000/", "v1/chat", "http://localhost:8000/v1/chat"},
		{"http://localhost:8000", "/v1/chat", "http://localhost:8000/v1/chat"},
	}

	for _, c := range cases {
		res := singleJoiningSlash(c.a, c.b)
		if res != c.expected {
			t.Errorf("singleJoiningSlash(%q, %q) = %q; want %q", c.a, c.b, res, c.expected)
		}
	}
}

// Test 405 Method Not Allowed on non-POST
func TestProxy_MethodNotAllowed(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	srv := NewServer(cfg, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/chat/completions", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed, got %d", rec.Code)
	}
}

// Test 404 Not Found on unknown path
func TestProxy_NotFound(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	srv := NewServer(cfg, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/unknown/route", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found, got %d", rec.Code)
	}
}

// Test authenticateClient with api-key and X-API-Key headers
func TestProxy_AuthHeaders(t *testing.T) {
	cfg := &contract.Config{
		Port:      8000,
		AuthToken: "sk-my-secret-key",
	}
	srv := NewServer(cfg, nil, nil, nil)

	// Case 1: api-key header
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req1.Header.Set("api-key", "sk-my-secret-key")
	if !srv.authenticateClient(req1) {
		t.Errorf("Expected authenticateClient to succeed with api-key header")
	}

	// Case 2: X-API-Key header
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req2.Header.Set("X-API-Key", "sk-my-secret-key")
	if !srv.authenticateClient(req2) {
		t.Errorf("Expected authenticateClient to succeed with X-API-Key header")
	}

	// Case 3: Invalid key
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req3.Header.Set("X-API-Key", "wrong-key")
	if srv.authenticateClient(req3) {
		t.Errorf("Expected authenticateClient to fail with wrong-key")
	}

	// Case 4: Bearer token valid and invalid
	req4 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req4.Header.Set("Authorization", "Bearer sk-my-secret-key")
	if !srv.authenticateClient(req4) {
		t.Errorf("Expected valid bearer token to succeed")
	}

	req5 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{}`))
	req5.Header.Set("Authorization", "Bearer wrong-token")
	if srv.authenticateClient(req5) {
		t.Errorf("Expected invalid bearer token to fail")
	}
}

// Test Upstream Error Handler (502 Bad Gateway when upstream server down)
func TestProxy_UpstreamErrorHandler(t *testing.T) {
	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"dead_server": {
				BaseURL: "http://127.0.0.1:54321/v1", // Dead port
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{Name: "Dead Tier", Model: "test", Provider: "dead_server", When: "true"},
		},
	}

	srv := NewServer(cfg, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"hello"}]}`))

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("Expected 502 Bad Gateway for dead upstream, got %d", rec.Code)
	}
}

// Test Reasoning Effort and Tier 3 / Tier 4 names
func TestProxy_ReasoningEffortAndTierMetadata(t *testing.T) {
	var capturedBody []byte
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cmpl-reason","choices":[{"message":{"role":"assistant","content":"Done"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock_cloud": {
				BaseURL: mockUpstream.URL,
				Type:    "cloud",
				APIKey:  "test-key",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:            "Tier 3 Reasoning",
				Model:           "deepseek-r1",
				Provider:        "mock_cloud",
				ReasoningEffort: "high",
				When:            "true",
			},
		},
	}

	srv := NewServer(cfg, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"analyze complex algorithm"}]}`))

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(string(capturedBody), `"reasoning_effort":"high"`) {
		t.Errorf("Expected reasoning_effort to be injected into payload, got: %s", string(capturedBody))
	}
}

// Test Invalid Target URL returns 500
func TestProxy_InvalidTargetURL_Returns500(t *testing.T) {
	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"bad_url": {
				BaseURL: "://invalid-url-syntax",
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{Name: "Bad URL Tier", Model: "test", Provider: "bad_url", When: "true"},
		},
	}

	srv := NewServer(cfg, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"hello"}]}`))

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500 Internal Server Error for invalid URL syntax, got %d", rec.Code)
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("simulated read error")
}

// Test request body read failure returns 400
func TestProxy_RequestBodyReadError(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", errReader{})

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request on read error, got %d", rec.Code)
	}
}

type errEvaluator struct{}

func (errEvaluator) SelectTier(reqCtx contract.RequestContext) (contract.Tier, error) {
	return contract.Tier{}, fmt.Errorf("evaluator failed")
}

// Test evaluator error falls back to DefaultTier
func TestProxy_EvaluatorError_FallbackToDefault(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fallback ok"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock_p": {BaseURL: mockUpstream.URL, Type: "local"},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Fallback Tier",
			Model:    "test-model",
			Provider: "mock_p",
		},
	}

	srv := NewServer(cfg, errEvaluator{}, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"hello"}]}`))

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 OK from fallback tier, got %d", rec.Code)
	}
	if rec.Header().Get("x-nacho-router-tier") != "Default Fallback Tier" {
		t.Errorf("Expected Default Fallback Tier in header, got '%s'", rec.Header().Get("x-nacho-router-tier"))
	}
}

// Test differential cost savings on cloud tier
func TestProxy_CloudPricingDifferential(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer mockUpstream.Close()

	mockORServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "anthropic/claude-3.5-sonnet", "pricing": {"prompt": "0.000003", "completion": "0.000015"}},
				{"id": "cheap-cloud", "pricing": {"prompt": "0.000001", "completion": "0.000002"}}
			]
		}`))
	}))
	defer mockORServer.Close()

	oracle := telemetry.NewPricingOracle()
	orProvider := telemetry.NewOpenRouterPricingProviderWithURL(mockORServer.URL, "test-key")
	oracle.RegisterProvider(orProvider, 0)
	_ = oracle.Sync(context.Background())

	tracker := telemetry.NewStatsTracker(100)

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"cloud_gw": {
				BaseURL: mockUpstream.URL,
				Type:    "cloud",
				APIKey:  "cloud-key",
			},
		},
		Tiers: []contract.Tier{
			{Name: "Tier 2 Coder", Model: "cheap-cloud", Provider: "cloud_gw", When: "true"},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	srv := NewServerWithTelemetry(cfg, evaluator, nil, nil, oracle, tracker, nil)

	// Send 10,000 tokens prompt -> saved (3.00 - 1.00) * 10,000 / 1M = $0.02
	rec := httptest.NewRecorder()
	longPrompt := fmt.Sprintf(`{"model":"test","messages":[{"role":"user","content":"%s"}]}`, strings.Repeat("abcd ", 10000))
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(longPrompt))

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", rec.Code)
	}

	tracker.Flush()
	stats := tracker.GetStats()
	if stats.EstimatedCostSavedUSD <= 0 {
		t.Errorf("Expected differential cost savings > 0, got %f", stats.EstimatedCostSavedUSD)
	}
}

// ---------------------------------------------------------------------------
// End-to-End Proxy Micro-Benchmarks (Testing Core HTTP Pipeline)
// ---------------------------------------------------------------------------

func BenchmarkProxy_ChatCompletions_RawPassThrough(b *testing.B) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cmpl-123","choices":[{"message":{"role":"assistant","content":"Standard code refactoring response."}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {BaseURL: mockUpstream.URL, Type: "local"},
		},
		Tiers: []contract.Tier{
			{Name: "Local GPU", Model: "qwen2.5-coder:14b", Provider: "local_gpu", When: "Tokens < 16000"},
		},
		DefaultTier: contract.Tier{Name: "Default", Model: "qwen2.5-coder:14b", Provider: "local_gpu"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	oracle := telemetry.NewPricingOracle()
	tracker := telemetry.NewStatsTracker(10000)
	reg := provider.NewRegistryFromConfig(cfg)
	nullLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, classifier, sanitizer, oracle, tracker, reg, nullLogger)

	reqPayload := []byte(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"refactor main.go"}]}`)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqPayload))
		srv.ServeHTTP(rec, req)
	}
}

func BenchmarkProxy_ChatCompletions_ToolNormalization(b *testing.B) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "cmpl-tool",
			"choices": [{
				"finish_reason": "stop",
				"message": {
					"role": "assistant",
					"content": "Searching codebase:\n` + "```json" + `\n{\"name\": \"search_code\", \"arguments\": {\"pattern\": \"atomic.Pointer\"}}\n` + "```" + `"
				}
			}]
		}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {BaseURL: mockUpstream.URL, Type: "local"},
		},
		Tiers: []contract.Tier{
			{Name: "Local GPU", Model: "qwen2.5-coder:14b", Provider: "local_gpu", When: "Tokens < 16000"},
		},
		DefaultTier: contract.Tier{Name: "Default", Model: "qwen2.5-coder:14b", Provider: "local_gpu"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	oracle := telemetry.NewPricingOracle()
	tracker := telemetry.NewStatsTracker(10000)
	reg := provider.NewRegistryFromConfig(cfg)
	nullLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, classifier, sanitizer, oracle, tracker, reg, nullLogger)

	reqPayload := []byte(`{
		"model": "nacho-hybrid",
		"messages": [{"role": "user", "content": "find atomic pointer"}],
		"tools": [{"type": "function", "function": {"name": "search_code"}}]
	}`)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqPayload))
		srv.ServeHTTP(rec, req)
	}
}

func TestProxy_NonStreaming_ReasoningModelNormalization(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-reason-test",
			"object": "chat.completion",
			"created": 1787200000,
			"model": "deepseek-r1",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Here is the final answer.",
					"reasoning_content": "Detailed reasoning about goroutine race conditions."
				},
				"finish_reason": "stop"
			}]
		}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock_cloud": {BaseURL: mockUpstream.URL, Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Tier 3", Model: "deepseek-r1", Provider: "mock_cloud", When: "true"},
		},
		DefaultTier: contract.Tier{Name: "Default", Model: "deepseek-r1", Provider: "mock_cloud"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"help with concurrency"}]}`))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var res fastChatCompletionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	expectedContent := "<think>\nDetailed reasoning about goroutine race conditions.\n</think>\n\nHere is the final answer."
	if len(res.Choices) == 0 || res.Choices[0].Message.Content != expectedContent {
		t.Errorf("expected normalized content %q, got %q", expectedContent, res.Choices[0].Message.Content)
	}
}

func TestProxy_LiveSSE_ReasoningStreamNormalization(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		_, _ = w.Write([]byte("data: {\"id\":\"sse-1\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"Live thinking...\"}}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: {\"id\":\"sse-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Live final answer.\"}}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock_cloud": {BaseURL: mockUpstream.URL, Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Tier 3", Model: "deepseek-r1", Provider: "mock_cloud", When: "true"},
		},
		DefaultTier: contract.Tier{Name: "Default", Model: "deepseek-r1", Provider: "mock_cloud"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"stream please"}]}`))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<think>") || !strings.Contains(body, "</think>") {
		t.Errorf("expected <think> and </think> in live SSE proxy output, got:\n%s", body)
	}
	if !strings.Contains(body, "Live final answer.") {
		t.Errorf("expected final answer in live SSE proxy output, got:\n%s", body)
	}
}

func TestProxy_APIKeyHeaderAuth(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port:      8000,
		AuthToken: "secret-test-token",
		Providers: map[string]contract.ProviderConfig{
			"p": {BaseURL: mockUpstream.URL, Type: "local"},
		},
		Tiers: []contract.Tier{
			{Name: "T", Model: "m", Provider: "p", When: "true"},
		},
		DefaultTier: contract.Tier{Name: "D", Model: "m", Provider: "p"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	// 1. api-key header
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid"}`))
	req.Header.Set("api-key", "secret-test-token")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with api-key header, got %d", rec.Code)
	}

	// 2. wrong api-key header
	recBad := httptest.NewRecorder()
	reqBad := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid"}`))
	reqBad.Header.Set("api-key", "wrong-token")
	srv.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with bad api-key header, got %d", recBad.Code)
	}

	// 3. wrong Bearer header
	recBadBearer := httptest.NewRecorder()
	reqBadBearer := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid"}`))
	reqBadBearer.Header.Set("Authorization", "Bearer invalid-token")
	srv.ServeHTTP(recBadBearer, reqBadBearer)
	if recBadBearer.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with bad Bearer token, got %d", recBadBearer.Code)
	}
}

func TestProxy_NonStreaming_EmptyContent_FallsBackToCloud(t *testing.T) {
	// Mock local provider returning defective empty content
	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":""}}]}`))
	}))
	defer mockLocal.Close()

	// Mock cloud fallback returning valid content
	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Recovered from cloud fallback!"}}]}`))
	}))
	defer mockCloud.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"local_p": {BaseURL: mockLocal.URL, Type: "local"},
			"cloud_p": {BaseURL: mockCloud.URL, Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Tier Local", Model: "qwen-local", Provider: "local_p", When: "true"},
		},
		DefaultTier: contract.Tier{Name: "Tier Cloud", Model: "claude-cloud", Provider: "cloud_p"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid","messages":[{"role":"user","content":"test"}]}`))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK after fallback, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Recovered from cloud fallback!") {
		t.Errorf("Expected cloud fallback content, got: %s", body)
	}
	if rec.Header().Get(contract.HeaderNachoRouterTier) != "Tier Cloud" {
		t.Errorf("Expected router tier header 'Tier Cloud', got %q", rec.Header().Get(contract.HeaderNachoRouterTier))
	}
}

func TestProxy_Streaming_ImmediateDone_FallsBackToCloud(t *testing.T) {
	// Mock local provider closing stream immediately with [DONE]
	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockLocal.Close()

	// Mock cloud fallback streaming valid tokens
	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Cloud stream recovery\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockCloud.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"local_p": {BaseURL: mockLocal.URL, Type: "local"},
			"cloud_p": {BaseURL: mockCloud.URL, Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Tier Local", Model: "qwen-local", Provider: "local_p", When: "true"},
		},
		DefaultTier: contract.Tier{Name: "Tier Cloud", Model: "claude-cloud", Provider: "cloud_p"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"test"}]}`))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK after streaming fallback, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Cloud stream recovery") {
		t.Errorf("Expected cloud streaming recovery tokens, got: %s", body)
	}
	if rec.Header().Get(contract.HeaderNachoRouterTier) != "Tier Cloud" {
		t.Errorf("Expected router tier header 'Tier Cloud', got %q", rec.Header().Get(contract.HeaderNachoRouterTier))
	}
}

func TestProxy_CircuitBreaker_BypassesDownProvider(t *testing.T) {
	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Cloud ok"}}]}`))
	}))
	defer mockCloud.Close()

	// Dead local URL
	deadLocalURL := "http://127.0.0.1:59999"

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"dead_local": {BaseURL: deadLocalURL, Type: "local"},
			"cloud_p":    {BaseURL: mockCloud.URL, Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Tier Local", Model: "qwen-local", Provider: "dead_local", When: "true"},
		},
		DefaultTier: contract.Tier{Name: "Tier Cloud", Model: "claude-cloud", Provider: "cloud_p"},
	}

	reg := provider.NewRegistryFromConfig(cfg)
	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, router.NewClassifier(), router.NewSanitizer(), nil, nil, reg, nil)

	// Turn 1: Dead local fails, trips circuit breaker, falls back to cloud
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid"}`))
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("Turn 1 expected 200 via fallback, got %d", rec1.Code)
	}

	// Turn 2: Dead local fails second time, trips to StateOpen
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid"}`))
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Turn 2 expected 200 via fallback, got %d", rec2.Code)
	}

	// Check provider circuit breaker is now OPEN
	p, _ := reg.Get("dead_local")
	cb := p.(provider.CircuitBreakerProvider).CircuitBreaker()
	if cb.State() != provider.StateOpen {
		t.Errorf("Expected circuit breaker to be StateOpen, got %v", cb.State())
	}

	// Turn 3: Request should immediately bypass dead local and route to Cloud without dial delay
	start := time.Now()
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"nacho-hybrid"}`))
	srv.ServeHTTP(rec3, req3)
	elapsed := time.Since(start)

	if rec3.Code != http.StatusOK {
		t.Fatalf("Turn 3 expected 200 via fast-bypass, got %d", rec3.Code)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("Turn 3 took %v; expected instantaneous (<100ms) circuit-breaker fast-fail bypass", elapsed)
	}
}

func TestProxy_SessionRetry_AutoEscalatesToCloud(t *testing.T) {
	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Local response"}}]}`))
	}))
	defer mockLocal.Close()

	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Cloud response"}}]}`))
	}))
	defer mockCloud.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"local_p": {BaseURL: mockLocal.URL, Type: "local"},
			"cloud_p": {BaseURL: mockCloud.URL, Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Tier Local", Model: "qwen-local", Provider: "local_p", When: "Retries < 2"},
		},
		DefaultTier: contract.Tier{Name: "Tier Cloud", Model: "claude-cloud", Provider: "cloud_p"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	sessionHeader := "roo-session-test-42"
	promptBody := `{"model":"nacho-hybrid","messages":[{"role":"user","content":"Fix this bug in main.go"}]}`

	// Turn 0: Fresh (Retries: 0) -> Local
	rec0 := httptest.NewRecorder()
	req0 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(promptBody))
	req0.Header.Set("x-session-id", sessionHeader)
	srv.ServeHTTP(rec0, req0)
	if rec0.Header().Get(contract.HeaderNachoRouterTier) != "Tier Local" {
		t.Errorf("Turn 0 expected 'Tier Local', got %q", rec0.Header().Get(contract.HeaderNachoRouterTier))
	}

	// Turn 1: First retry (Retries: 1) -> Local
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(promptBody))
	req1.Header.Set("x-session-id", sessionHeader)
	srv.ServeHTTP(rec1, req1)
	if rec1.Header().Get(contract.HeaderNachoRouterTier) != "Tier Local" {
		t.Errorf("Turn 1 expected 'Tier Local', got %q", rec1.Header().Get(contract.HeaderNachoRouterTier))
	}

	// Turn 2: Second retry (Retries: 2) -> Auto-escalates to Cloud
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(promptBody))
	req2.Header.Set("x-session-id", sessionHeader)
	srv.ServeHTTP(rec2, req2)
	if rec2.Header().Get(contract.HeaderNachoRouterTier) != "Tier Cloud" {
		t.Errorf("Turn 2 expected 'Tier Cloud' (auto-escalated), got %q", rec2.Header().Get(contract.HeaderNachoRouterTier))
	}
}

func TestProxy_IsDefectiveEmptyContent_Branches(t *testing.T) {
	// Invalid JSON -> false
	if isDefectiveEmptyContent([]byte("invalid json")) {
		t.Errorf("Expected false for invalid json")
	}

	// No choices array -> true
	if !isDefectiveEmptyContent([]byte(`{"id":"123"}`)) {
		t.Errorf("Expected true for missing choices")
	}

	// Empty choices array -> true
	if !isDefectiveEmptyContent([]byte(`{"choices":[]}`)) {
		t.Errorf("Expected true for empty choices")
	}

	// Choice not a map -> true
	if !isDefectiveEmptyContent([]byte(`{"choices":["not-a-map"]}`)) {
		t.Errorf("Expected true for non-map choice")
	}

	// Message not a map -> true
	if !isDefectiveEmptyContent([]byte(`{"choices":[{"message":"not-a-map"}]}`)) {
		t.Errorf("Expected true for non-map message")
	}

	// Empty message -> true
	if !isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":""}}]}`)) {
		t.Errorf("Expected true for empty content without tools")
	}

	// Message with reasoning field (alternative reasoning tag) -> false
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"","reasoning":"thinking step"}}]}`)) {
		t.Errorf("Expected false for message with reasoning")
	}

	// Message with tool_calls -> false
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"1"}]}}]}`)) {
		t.Errorf("Expected false for message with tool_calls")
	}

	// Message with valid content -> false
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"Hello world"}}]}`)) {
		t.Errorf("Expected false for message with valid content")
	}
}

type plainMockProvider struct {
	id string
}

func (p *plainMockProvider) ID() string                     { return p.id }
func (p *plainMockProvider) Name() string                   { return p.id }
func (p *plainMockProvider) BaseURL() string                { return "http://localhost" }
func (p *plainMockProvider) IsLocal() bool                  { return false }
func (p *plainMockProvider) Ping(ctx context.Context) error { return nil }

func TestProxy_NonCircuitBreakerProvider_Fallbacks(t *testing.T) {
	srv := NewServer(&contract.Config{}, nil, nil, nil)
	plain := &plainMockProvider{id: "plain"}

	if !srv.allowProvider(plain) {
		t.Errorf("Expected allowProvider to return true for non-CB provider")
	}

	// These should not panic
	srv.recordProviderFailure(plain)
	srv.recordProviderSuccess(plain)
}

func TestProxy_HasCandidateToolTokens(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{`<tool_call>`, true},
		{`[TOOL_CALLS]`, true},
		{`<function=`, true},
		{`<invoke>`, true},
		{`Action: `, true},
		{`<|python_tag|>`, true},
		{"```json\n{}", true},
		{`Just normal text`, false},
	}

	for _, c := range cases {
		got := hasCandidateToolTokens([]byte(c.input))
		if got != c.expected {
			t.Errorf("hasCandidateToolTokens(%q) = %v; want %v", c.input, got, c.expected)
		}
	}
}

func TestProxy_DispatchTier_MissingProviderFallbackTerminal(t *testing.T) {
	cfg := &contract.Config{
		Port: 8000,
		Tiers: []contract.Tier{
			{Name: "Nonexistent Tier", Model: "test", Provider: "ghost_provider", When: "true"},
		},
		// No default tier
	}
	srv := NewServer(cfg, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))

	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("Expected 502 Bad Gateway when provider not found, got %d", rec.Code)
	}
}

func TestProxy_ReasoningEffort_Injection(t *testing.T) {
	var receivedBody []byte
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock": {BaseURL: mockUpstream.URL},
		},
		Tiers: []contract.Tier{
			{
				Name:            "Reasoning High",
				Model:           "o1-preview",
				Provider:        "mock",
				ReasoningEffort: "high",
				When:            "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"complex puzzle"}]}`))
	srv.ServeHTTP(rec, req)

	if !strings.Contains(string(receivedBody), `"reasoning_effort":"high"`) {
		t.Errorf("Expected reasoning_effort:high injected into upstream payload, got: %s", string(receivedBody))
	}
}

func TestProxy_NonStreaming_ReasonFieldAndEstimatorCalibration(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"Answer","reason":"Step 1 reasoning"}}],
			"usage":{"prompt_tokens":120}
		}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock": {BaseURL: mockUpstream.URL},
		},
		Tiers: []contract.Tier{
			{Name: "T1", Model: "m1", Provider: "mock", When: "true"},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	clf := router.NewClassifier()
	srv := NewServer(cfg, evaluator, clf, router.NewSanitizer())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello world"}]}`))
	req.Header.Set("session-id", "test-session-fallback-header")
	srv.ServeHTTP(rec, req)

	respBody := rec.Body.String()
	if !strings.Contains(respBody, "<think>") || !strings.Contains(respBody, "Step 1 reasoning") {
		t.Errorf("Expected <think> wrapping reason field, got: %s", respBody)
	}
}

func TestProxy_MissingProvider_FallbackToDefaultTier(t *testing.T) {
	mockDefaultUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"fallback response"}}]}`))
	}))
	defer mockDefaultUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"default_prov": {BaseURL: mockDefaultUpstream.URL},
		},
		Tiers: []contract.Tier{
			{Name: "Missing Tier", Model: "m1", Provider: "non_existent_prov", When: "true"},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Fallback Tier",
			Model:    "m_default",
			Provider: "default_prov",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 via fallback default tier, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "fallback response") {
		t.Errorf("Expected fallback response body, got: %s", rec.Body.String())
	}
}

func TestProxy_RecordTelemetry_VisionTier4(t *testing.T) {
	tracker := telemetry.NewStatsTracker(100)
	srv := &Server{
		tracker: tracker,
		oracle:  telemetry.NewPricingOracle(),
	}

	tier := contract.Tier{
		Name:     "Vision High",
		Model:    "gemini-vision",
		Provider: "google",
	}
	p := provider.NewGenericLLMProvider("google", contract.ProviderConfig{Type: "cloud"})
	reqCtx := contract.RequestContext{Tokens: 1000}
	usage := StreamUsage{PromptTokens: 800, CompletionTokens: 200, TotalTokens: 1000}
	srv.recordTelemetry(tier, p, reqCtx, usage, 200, time.Now(), false, slog.Default())

	// Also local tier 1
	pLocal := provider.NewGenericLLMProvider("ollama", contract.ProviderConfig{Type: "local"})
	tierLocal := contract.Tier{Name: "Local Default", Model: "qwen", Provider: "ollama"}
	srv.recordTelemetry(tierLocal, pLocal, reqCtx, usage, 200, time.Now(), false, slog.Default())
}

func TestProxy_IsDefectiveEmptyContent_DeepBranches(t *testing.T) {
	cases := [][]byte{
		[]byte(`invalid json`),
		[]byte(`{"choices":[]}`),
		[]byte(`{"choices":["not-a-map"]}`),
		[]byte(`{"choices":[{"message":"not-a-map"}]}`),
		[]byte(`{"choices":[{"message":{"content":"","tool_calls":[],"reasoning":""}}]}`),
	}

	for _, c := range cases {
		if !isDefectiveEmptyContent(c) && string(c) != "invalid json" {
			t.Errorf("Expected defective true for %s", string(c))
		}
	}
}

func TestStreamNormalizer_Read_WhenClosed(t *testing.T) {
	sn := NewStreamNormalizer(io.NopCloser(strings.NewReader("data: test\n\n")))
	_ = sn.Close()

	buf := make([]byte, 100)
	n, err := sn.Read(buf)
	if n != 0 || err != io.EOF {
		t.Errorf("Expected 0, EOF when reading closed normalizer, got n=%d, err=%v", n, err)
	}
}

func TestProxy_ServeHTTP_RoutingEdgeCases(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	evaluator, _ := strategy.NewExprEvaluator(nil, contract.Tier{Model: "default"})
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	// 1. Unknown /api/v1/ route -> 404
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown-endpoint", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown api route, got %d", rec.Code)
	}

	// 2. Unknown general path -> 404
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", strings.NewReader(`{}`))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unsupported non-chat route, got %d", rec.Code)
	}

	// 3. Method Not Allowed on chat completions -> 405
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /v1/chat/completions, got %d", rec.Code)
	}
}

type proxyMockPricingProvider struct {
	name   string
	prices map[string]telemetry.ModelMetadata
}

func (p *proxyMockPricingProvider) Name() string { return p.name }
func (p *proxyMockPricingProvider) FetchPricing(ctx context.Context) (map[string]telemetry.ModelMetadata, error) {
	return p.prices, nil
}

func TestProxy_StreamingUsage_DualRateCostAccounting(t *testing.T) {
	// Upstream test server returning streaming SSE with final usage chunk
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello streaming\"}}]}\n\n")
		// Emit final usage block (OpenRouter style)
		fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":50000,\"completion_tokens\":1000,\"total_tokens\":51000}}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock_cloud": {
				BaseURL: upstream.URL,
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Cloud Tier",
				Provider: "mock_cloud",
				Model:    "qwen-coder-test",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Tier",
			Provider: "mock_cloud",
			Model:    "qwen-coder-test",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	oracle := telemetry.NewPricingOracle()
	tracker := telemetry.NewStatsTracker(100)

	// Set pricing for qwen-coder-test: $0.30 prompt / $1.50 completion per 1M
	oracle.RegisterProvider(&proxyMockPricingProvider{
		name: "mock_cloud",
		prices: map[string]telemetry.ModelMetadata{
			"qwen-coder-test": {
				ModelPricing: telemetry.ModelPricing{PromptCostPerMillion: 0.30, CompletionCostPerMillion: 1.50},
				ModelID:      "qwen-coder-test",
			},
		},
	}, 0)
	_ = oracle.Sync(context.Background())

	srv := NewServerWithTelemetry(cfg, evaluator, router.NewClassifier(), router.NewSanitizer(), oracle, tracker, nil)

	rec := httptest.NewRecorder()
	reqBody := `{"model":"auto","stream":true,"messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// Allow tracker channel to drain
	time.Sleep(50 * time.Millisecond)
	stats := tracker.GetStats()

	if stats.TotalRequests != 1 {
		t.Errorf("expected 1 total request in tracker, got %d", stats.TotalRequests)
	}

	// Expected spend: (50000 / 1M) * 0.30 + (1000 / 1M) * 1.50 = $0.015 + $0.0015 = $0.0165
	if fmt.Sprintf("%.4f", stats.TotalCostSpentUSD) != "0.0165" {
		t.Errorf("expected $0.0165 spent in tracker, got %f", stats.TotalCostSpentUSD)
	}

	// Expected baseline: (51000 / 1M) * 3.00 = $0.153
	// Expected saved: 0.153 - 0.0165 = $0.1365
	if fmt.Sprintf("%.4f", stats.EstimatedCostSavedUSD) != "0.1365" {
		t.Errorf("expected $0.1365 saved in tracker, got %f", stats.EstimatedCostSavedUSD)
	}
}
