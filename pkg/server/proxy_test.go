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
	oracle.RegisterProvider(orProvider)
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
	oracle.RegisterProvider(orProvider)
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
