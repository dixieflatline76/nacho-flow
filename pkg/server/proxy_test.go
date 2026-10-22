package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/store"
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

func TestServer_SettersAndNTSTransformer(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	evaluator, _ := strategy.NewExprEvaluator(nil, contract.Tier{Model: "default"})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	srv.SetTuningRunner(nil)
	srv.SetTrafficLogger(nil)
	srv.SetDiskStore(nil)
	srv.SetTrafficLogPath("traffic.jsonl")

	_ = srv.GetNTSTransformer()

	var emptySrv Server
	if emptySrv.GetNTSTransformer() != nil {
		t.Errorf("expected nil for empty server without state")
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

		if rec.Header().Get("Server") != contract.AppName+"/"+contract.Version {
			t.Errorf("Endpoint %s: Expected Server header %q, got %q", endpoint, contract.AppName+"/"+contract.Version, rec.Header().Get("Server"))
		}
		if rec.Header().Get("X-Nacho-Flow") != "true" {
			t.Errorf("Endpoint %s: Expected X-Nacho-Flow header 'true', got %q", endpoint, rec.Header().Get("X-Nacho-Flow"))
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Endpoint %s: Failed to parse JSON: %v", endpoint, err)
		}
		if resp["status"] != "ok" {
			t.Errorf("Endpoint %s: Expected status 'ok', got %v", endpoint, resp["status"])
		}
		if resp["app"] != contract.AppName {
			t.Errorf("Endpoint %s: Expected app %q, got %v", endpoint, contract.AppName, resp["app"])
		}
		if resp["service"] != contract.AppName {
			t.Errorf("Endpoint %s: Expected service %q, got %v", endpoint, contract.AppName, resp["service"])
		}
		if resp["version"] == nil || resp["version"] == "" {
			t.Errorf("Endpoint %s: Expected non-empty version, got %v", endpoint, resp["version"])
		}
		if resp["uptime"] == nil || resp["uptime"] == "" {
			t.Errorf("Endpoint %s: Expected non-empty uptime, got %v", endpoint, resp["uptime"])
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
				Type:    provider.TypeOpenAICompatible,
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
				Type:    provider.TypeOpenRouter,
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
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
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
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
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
			"mock_local": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
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
				{"id": "anthropic/claude-sonnet-5", "pricing": {"prompt": "0.000002", "completion": "0.000010"}},
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
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
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
				Type:    provider.TypeOpenAICompatible,
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
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
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
			"mock_p": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
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
				{"id": "anthropic/claude-sonnet-5", "pricing": {"prompt": "0.000002", "completion": "0.000010"}},
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
				Type:    provider.TypeOpenAICompatible,
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
			"local_gpu": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
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
			"local_gpu": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
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
			"mock_cloud": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible},
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
			"mock_cloud": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible},
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
	if !strings.Contains(body, "\"reasoning_content\":\"Live thinking...\"") {
		t.Errorf("expected reasoning_content in live SSE proxy output, got:\n%s", body)
	}
	if strings.Contains(body, "<think>") || strings.Contains(body, "</think>") {
		t.Errorf("unexpected think tags leaked into live SSE proxy output, got:\n%s", body)
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
			"p": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
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
			"local_p": {BaseURL: mockLocal.URL, Type: provider.TypeOpenAICompatible, Local: true},
			"cloud_p": {BaseURL: mockCloud.URL, Type: provider.TypeOpenAICompatible},
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
			"local_p": {BaseURL: mockLocal.URL, Type: provider.TypeOpenAICompatible, Local: true},
			"cloud_p": {BaseURL: mockCloud.URL, Type: provider.TypeOpenAICompatible},
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
			"dead_local": {BaseURL: deadLocalURL, Type: provider.TypeOpenAICompatible, Local: true},
			"cloud_p":    {BaseURL: mockCloud.URL, Type: provider.TypeOpenAICompatible},
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
			"local_p": {BaseURL: mockLocal.URL, Type: provider.TypeOpenAICompatible, Local: true},
			"cloud_p": {BaseURL: mockCloud.URL, Type: provider.TypeOpenAICompatible},
		},
		Tiers: []contract.Tier{
			{Name: "Tier Local", Model: "qwen-local", Provider: "local_p", When: "Retries < 2"},
		},
		DefaultTier: contract.Tier{Name: "Tier Cloud", Model: "claude-cloud", Provider: "cloud_p"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	sessionHeader := "opencode-session-test-42"
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

func TestProxy_AgentChurnDuringTestFailureEscalates(t *testing.T) {
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
			"local_p": {BaseURL: mockLocal.URL, Type: provider.TypeOpenAICompatible, Local: true},
			"cloud_p": {BaseURL: mockCloud.URL, Type: provider.TypeOpenAICompatible},
		},
		Tiers: []contract.Tier{
			{Name: "Tier Local", Model: "qwen-local", Provider: "local_p", When: "Retries < 2"},
		},
		DefaultTier: contract.Tier{Name: "Tier Cloud", Model: "claude-cloud", Provider: "cloud_p"},
		Kickstart: contract.KickstartConfig{
			ReadSlap: contract.ReadSlapConfig{
				WriteOnly: true,
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	sessionHeader := "agent-churn-session-99"

	// Turn 0: Fresh task start -> Local
	body0 := `{"model":"nacho-hybrid","messages":[{"role":"user","content":"Fix the blackjack hand calculation"}]}`
	rec0 := httptest.NewRecorder()
	req0 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body0))
	req0.Header.Set("x-session-id", sessionHeader)
	srv.ServeHTTP(rec0, req0)
	if rec0.Header().Get(contract.HeaderNachoRouterTier) != "Tier Local" {
		t.Fatalf("Turn 0 expected 'Tier Local', got %q", rec0.Header().Get(contract.HeaderNachoRouterTier))
	}

	// Turn 1: Agent writes a file edit, but tests FAIL.
	// Churn: HasTools=true, HasWriteProgress=true, HasTestFail=true.
	// In the presence of HasTestFail, it must NOT reset retries to 0; must increment retries to 1.
	body1 := `{
		"model":"nacho-hybrid",
		"tools":[{"type":"function","function":{"name":"write_to_file"}}],
		"messages":[
			{"role":"user","content":"Fix the blackjack hand calculation"},
			{"role":"assistant","content":"I will update hand.go","tool_calls":[{"id":"call_1","type":"function","function":{"name":"write_to_file","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"File saved successfully.\nFAIL: TestBlackjackHand\nexit status 1"}
		]
	}`
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body1))
	req1.Header.Set("x-session-id", sessionHeader)
	srv.ServeHTTP(rec1, req1)
	if rec1.Header().Get(contract.HeaderNachoRouterTier) != "Tier Local" {
		t.Fatalf("Turn 1 expected 'Tier Local' (retries=1 < 2), got %q", rec1.Header().Get(contract.HeaderNachoRouterTier))
	}

	// Turn 2: Agent writes another file edit, but tests FAIL AGAIN.
	// Churn continues: HasTools=true, HasWriteProgress=true, HasTestFail=true.
	// Must NOT reset retries to 0; must increment retries to 2.
	body2 := `{
		"model":"nacho-hybrid",
		"tools":[{"type":"function","function":{"name":"write_to_file"}}],
		"messages":[
			{"role":"user","content":"Fix the blackjack hand calculation"},
			{"role":"assistant","content":"I will update hand.go again","tool_calls":[{"id":"call_2","type":"function","function":{"name":"write_to_file","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"call_2","content":"File saved successfully.\nFAIL: TestBlackjackHand\nexit status 1"}
		]
	}`
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body2))
	req2.Header.Set("x-session-id", sessionHeader)
	srv.ServeHTTP(rec2, req2)
	// Because retries accumulated to 2, 'Retries < 2' evaluates to false -> MUST ESCALATE to Tier Cloud!
	if rec2.Header().Get(contract.HeaderNachoRouterTier) != "Tier Cloud" {
		t.Errorf("Turn 2 expected 'Tier Cloud' (auto-escalated after 2 failed edit attempts), got %q", rec2.Header().Get(contract.HeaderNachoRouterTier))
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
func (p *plainMockProvider) Platform() contract.Platform    { return contract.PlatformOpenAICompatible }
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
			"mock": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible},
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
			"mock": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible},
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
			"default_prov": {BaseURL: mockDefaultUpstream.URL, Type: provider.TypeOpenAICompatible},
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
	p := provider.NewOpenAICompatibleProvider("google", contract.ProviderConfig{Type: provider.TypeOpenAICompatible, BaseURL: "https://generativelanguage.googleapis.com"})
	reqCtx := contract.RequestContext{Tokens: 1000}
	usage := StreamUsage{PromptTokens: 800, CompletionTokens: 200, TotalTokens: 1000}
	srv.recordTelemetry(tier, p, reqCtx, usage, 200, time.Now(), false, slog.Default(), 0, 0)

	// Also local tier 1
	pLocal := provider.NewOpenAICompatibleProvider("ollama", contract.ProviderConfig{Type: provider.TypeOpenAICompatible, Local: true, BaseURL: "http://localhost:11434"})
	tierLocal := contract.Tier{Name: "Local Default", Model: "qwen", Provider: "ollama"}
	srv.recordTelemetry(tierLocal, pLocal, reqCtx, usage, 200, time.Now(), false, slog.Default(), 0, 0)
}

type mockSessionTurnSink struct {
	records []telemetry.TurnRecord
	mu      sync.Mutex
}

func (m *mockSessionTurnSink) Emit(rec telemetry.TurnRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, rec)
}

func (m *mockSessionTurnSink) Close() error { return nil }

func TestProxy_RecordTelemetry_SessionFields(t *testing.T) {
	tracker := telemetry.NewStatsTracker(100)
	defer tracker.Close()

	sink := &mockSessionTurnSink{}
	tracker.AddSink(sink)

	srv := &Server{
		tracker: tracker,
		oracle:  telemetry.NewPricingOracle(),
	}

	tier := contract.Tier{
		Name:     "Tier 1: Local GPU",
		Model:    "qwen2.5-coder:14b",
		Provider: "ollama",
	}
	p := provider.NewOpenAICompatibleProvider("ollama", contract.ProviderConfig{Type: provider.TypeOpenAICompatible, Local: true, BaseURL: "http://localhost:11434"})

	reqCtx := contract.RequestContext{
		Tokens:             1200,
		SessionKey:         "sess-live-proxy-12345",
		RootPromptHash:     0xABCD1234EF567890,
		Retries:            3,
		HasWriteCapability: true,
		HasWriteProgress:   true,
		HasTestPass:        true,
		HasTestFail:        false,
		ClientID:           "cline",
	}
	usage := StreamUsage{PromptTokens: 1000, CompletionTokens: 200, TotalTokens: 1200}

	srv.recordTelemetry(tier, p, reqCtx, usage, 200, time.Now(), false, slog.Default(), 0, 0)
	tracker.Flush()

	sink.mu.Lock()
	defer sink.mu.Unlock()

	if len(sink.records) != 1 {
		t.Fatalf("Expected 1 sink record, got %d", len(sink.records))
	}

	rec := sink.records[0]
	if rec.ClientID != "cline" {
		t.Errorf("Expected ClientID 'cline', got '%s'", rec.ClientID)
	}
	if rec.SessionID != "sess-live-proxy-12345" {
		t.Errorf("Expected SessionID 'sess-live-proxy-12345', got '%s'", rec.SessionID)
	}
	if rec.RootPromptHash != 0xABCD1234EF567890 {
		t.Errorf("Expected RootPromptHash 0xABCD1234EF567890, got 0x%X", rec.RootPromptHash)
	}
	if rec.Retries != 3 {
		t.Errorf("Expected Retries 3, got %d", rec.Retries)
	}
	if !rec.HasWriteCapability {
		t.Errorf("Expected HasWriteCapability true, got false")
	}
	if !rec.HasWriteProgress {
		t.Errorf("Expected HasWriteProgress true, got false")
	}
	if !rec.HasTestPass {
		t.Errorf("Expected HasTestPass true, got false")
	}
	if rec.HasTestFail {
		t.Errorf("Expected HasTestFail false, got true")
	}
}

func TestProxy_ClientDetectionInPipeline(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
	}))
	defer mockUpstream.Close()

	tracker := telemetry.NewStatsTracker(100)
	defer tracker.Close()

	sink := &mockSessionTurnSink{}
	tracker.AddSink(sink)

	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"mock": {
				BaseURL: mockUpstream.URL,
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
			},
		},
		Tiers: []contract.Tier{
			{Name: "T1", Provider: "mock", Model: "qwen", When: "true"},
		},
		DefaultTier: contract.Tier{Name: "T1", Provider: "mock", Model: "qwen"},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	cls := router.NewClassifier()
	san := router.NewSanitizer()
	oracle := telemetry.NewPricingOracle()

	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, cls, san, oracle, tracker, nil, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"test client detection"}]}`))
	req.Header.Set("User-Agent", "Cline/3.1.0")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	tracker.Flush()

	sink.mu.Lock()
	defer sink.mu.Unlock()

	if len(sink.records) != 1 {
		t.Fatalf("Expected 1 telemetry record emitted, got %d", len(sink.records))
	}
	if sink.records[0].ClientID != "cline" {
		t.Errorf("Expected detected client_id 'cline', got '%s'", sink.records[0].ClientID)
	}
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
				Type:    provider.TypeOpenAICompatible,
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

	// Expected baseline: (50000/1M)*$2.00 + (1000/1M)*$10.00 = $0.10 + $0.01 = $0.11
	// Expected saved: $0.11 - $0.0165 = $0.0935
	if fmt.Sprintf("%.4f", stats.EstimatedCostSavedUSD) != "0.0935" {
		t.Errorf("expected $0.0935 saved in tracker, got %f", stats.EstimatedCostSavedUSD)
	}
}

func TestServer_NonStreamingAgentShieldFallback(t *testing.T) {
	// Mock backend that returns non-streaming prose response asking a question
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := `{"id":"chatcmpl-test","choices":[{"index":0,"message":{"role":"assistant","content":"I have written the architectural blueprint.\n\nAre you satisfied with this plan?"},"finish_reason":"stop"}]}`
		w.Write([]byte(resp))
	}))
	defer ts.Close()

	disabled := false
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"mock": {
				BaseURL: ts.URL,
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
			},
		},
		AgentShield: contract.AgentShieldConfig{
			QuestionHeuristics:   []string{"are you satisfied"},
			ModeSwitchHeuristics: []string{"ready to implement"},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Provider: "mock",
			Model:    "test-model",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(nil, cfg.DefaultTier)
	srv := NewServer(cfg, evaluator, router.NewClassifier(), router.NewSanitizer())

	// 1. Request with tools including ask_followup_question
	reqBody := `{"model":"auto","stream":false,"tools":[{"type":"function","function":{"name":"ask_followup_question"}}],"messages":[{"role":"user","content":"Write a plan"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	choices := parsed["choices"].([]interface{})
	firstChoice := choices[0].(map[string]interface{})
	if firstChoice["finish_reason"] != "tool_calls" {
		t.Fatalf("expected finish_reason 'tool_calls', got %v", firstChoice["finish_reason"])
	}

	msg := firstChoice["message"].(map[string]interface{})
	toolCalls := msg["tool_calls"].([]interface{})
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 synthesized tool call, got %d", len(toolCalls))
	}

	// 2. Test disabled shield config
	cfgDisabled := &contract.Config{
		Providers: cfg.Providers,
		AgentShield: contract.AgentShieldConfig{
			Enabled: &disabled,
		},
		DefaultTier: cfg.DefaultTier,
	}
	srvDisabled := NewServer(cfgDisabled, evaluator, router.NewClassifier(), router.NewSanitizer())
	if srvDisabled.shieldMgr != nil {
		t.Fatal("expected nil shieldMgr when enabled=false")
	}
}

func TestMergeUniqueSlices(t *testing.T) {
	// 1. Both empty
	if res := mergeUniqueSlices(nil, nil); len(res) != 0 {
		t.Errorf("expected empty slice, got %v", res)
	}

	// 2. Custom empty returns base
	base := []string{"a", "b"}
	if res := mergeUniqueSlices(base, nil); len(res) != 2 || res[0] != "a" {
		t.Errorf("expected base slice, got %v", res)
	}

	// 3. Base empty returns custom
	custom := []string{"c", "d"}
	if res := mergeUniqueSlices(nil, custom); len(res) != 2 || res[0] != "c" {
		t.Errorf("expected custom slice, got %v", res)
	}

	// 4. Merge with duplicates, whitespace, and empty strings
	b := []string{"  alpha  ", "beta", "gamma", ""}
	c := []string{"beta", "  delta  ", "alpha", "", "   "}
	res := mergeUniqueSlices(b, c)
	expected := []string{"alpha", "beta", "gamma", "delta"}
	if len(res) != len(expected) {
		t.Fatalf("expected len %d, got %d (%v)", len(expected), len(res), res)
	}
	for i, v := range expected {
		if res[i] != v {
			t.Errorf("expected index %d to be %q, got %q", i, v, res[i])
		}
	}
}

func TestServer_AgentShield_RegistryIntegration(t *testing.T) {
	// 1. Test initialization with zero overrides in config: pulls all from agentregistry
	cfg := &contract.Config{
		AgentShield: contract.AgentShieldConfig{
			// Leave question_heuristics, mode_switch_heuristics, and error_signatures empty
		},
		DefaultTier: contract.Tier{
			Model: "mock-model",
		},
	}
	clf := router.NewClassifier().(*router.RequestClassifier)
	srv := NewServer(cfg, nil, clf, nil)

	if srv.shieldMgr == nil {
		t.Fatal("expected srv.shieldMgr to be initialized from agentregistry defaults")
	}

	// Classifier error signatures should match agentregistry.DefaultRegistry().ErrorSignaturesList()
	sigs := clf.GetErrorSignatures()
	expectedSigs := agentregistry.DefaultRegistry().ErrorSignaturesList()
	if len(sigs) != len(expectedSigs) {
		t.Errorf("expected %d error signatures from registry, got %d", len(expectedSigs), len(sigs))
	}

	// 2. Test initialization with custom overrides: merged without duplicates
	cfgCustom := &contract.Config{
		AgentShield: contract.AgentShieldConfig{
			QuestionHeuristics:   []string{"custom question?", "should i"},
			ModeSwitchHeuristics: []string{"custom mode switch"},
			ErrorSignatures:      []string{"CUSTOM_ERROR_CODE", "Missing value for required parameter"},
		},
		Kickstart: contract.KickstartConfig{
			ReadSlap: contract.ReadSlapConfig{
				CustomWriteTools: []string{"custom_patcher", "editor"},
			},
		},
		DefaultTier: contract.Tier{
			Model: "mock-model",
		},
	}
	clfCustom := router.NewClassifier().(*router.RequestClassifier)
	srvCustom := NewServer(cfgCustom, nil, clfCustom, nil)

	if srvCustom.shieldMgr == nil {
		t.Fatal("expected non-nil shieldMgr")
	}
	customSigs := clfCustom.GetErrorSignatures()
	foundCustom := false
	for _, s := range customSigs {
		if s == "CUSTOM_ERROR_CODE" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Errorf("expected CUSTOM_ERROR_CODE in merged error signatures")
	}

	// 3. Verify hot-reload updates shieldMgr and classifier
	disabled := false
	newCfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"mock": {Type: provider.TypeOpenAICompatible, Local: true, BaseURL: "http://localhost:8000"},
		},
		DefaultTier: contract.Tier{
			Model:    "mock-model",
			Provider: "mock",
		},
		AgentShield: contract.AgentShieldConfig{
			Enabled: &disabled,
		},
	}
	if _, err := srvCustom.ApplyConfig(newCfg, false); err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}
	if srvCustom.shieldMgr != nil {
		t.Errorf("expected shieldMgr to be nil after disabling via ApplyConfig")
	}
}

func TestServer_NTSConfigInitialization(t *testing.T) {
	bTrue := true
	cfg := &contract.Config{
		NTS: contract.NTSConfig{
			Enabled:               &bTrue,
			StripANSI:             &bTrue,
			ResolveCR:             &bTrue,
			DeduplicateLines:      &bTrue,
			DedupThreshold:        3,
			StripBoilerplate:      &bTrue,
			NormalizeWhitespace:   &bTrue,
			PreserveFileReads:     &bTrue,
			PreserveFileWrites:    &bTrue,
			PreserveCacheControl:  &bTrue,
			CompactStaleFileReads: &bTrue,
			StaleReadDepth:        4,
		},
		DefaultTier: contract.Tier{
			Model: "mock-model",
		},
	}
	srv := NewServer(cfg, nil, nil, nil)
	if srv.ntsTransformer == nil {
		t.Fatal("expected non-nil ntsTransformer")
	}
}

func TestServer_ConstructorNilAndFallbackBranches(t *testing.T) {
	// 1. All nil arguments to NewServerWithTelemetryAndRegistry
	srv := NewServerWithTelemetryAndRegistry(nil, nil, nil, nil, nil, nil, nil, nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	// 2. Test write tools configuration from Kickstart.ReadSlap
	cfgWithTools := &contract.Config{
		Kickstart: contract.KickstartConfig{
			ReadSlap: contract.ReadSlapConfig{
				CustomWriteTools: []string{"tool_ks_w"},
			},
		},
	}
	clf := router.NewClassifier().(*router.RequestClassifier)
	s := NewServerWithTelemetryAndRegistry(cfgWithTools, nil, clf, nil, nil, nil, nil, nil)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestServer_ProxyHelpers(t *testing.T) {
	// 1. singleJoiningSlash
	if singleJoiningSlash("http://localhost", "/v1") != "http://localhost/v1" {
		t.Errorf("unexpected slash join")
	}
	if singleJoiningSlash("http://localhost/", "v1") != "http://localhost/v1" {
		t.Errorf("unexpected slash join")
	}
	if singleJoiningSlash("http://localhost/", "/v1") != "http://localhost/v1" {
		t.Errorf("unexpected slash join")
	}
	if singleJoiningSlash("http://localhost", "v1") != "http://localhost/v1" {
		t.Errorf("unexpected slash join")
	}

	// 2. hasCandidateToolTokens
	if !hasCandidateToolTokens([]byte("<tool_call>{}</tool_call>")) {
		t.Errorf("expected true for tool tokens")
	}
	if hasCandidateToolTokens([]byte("plain content")) {
		t.Errorf("expected false for plain content")
	}

	// 3. isDefectiveEmptyContent
	if !isDefectiveEmptyContent([]byte(`{"choices":[]}`)) {
		t.Errorf("expected defective for empty choices")
	}
	if !isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":""}}]}`)) {
		t.Errorf("expected defective for empty content")
	}
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"ok"}}]}`)) {
		t.Errorf("expected not defective for ok content")
	}
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"","reasoning":"thinking"}}]}`)) {
		t.Errorf("expected not defective for reasoning content")
	}
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"1"}]}}]}`)) {
		t.Errorf("expected not defective when tool_calls exist")
	}
	if isDefectiveEmptyContent([]byte(`invalid json`)) {
		t.Errorf("expected false on unmarshal error")
	}

	// 4. authenticateClient edge cases
	srv := NewServer(&contract.Config{AuthToken: "secret123"}, nil, nil, nil)
	rValidBearer := httptest.NewRequest("GET", "/", nil)
	rValidBearer.Header.Set("Authorization", "Bearer secret123")
	if !srv.authenticateClient(rValidBearer) {
		t.Errorf("expected valid bearer token")
	}

	rValidAPIKey := httptest.NewRequest("GET", "/", nil)
	rValidAPIKey.Header.Set("api-key", "secret123")
	if !srv.authenticateClient(rValidAPIKey) {
		t.Errorf("expected valid api-key header")
	}

	rInvalid := httptest.NewRequest("GET", "/", nil)
	rInvalid.Header.Set("Authorization", "Bearer wrong")
	if srv.authenticateClient(rInvalid) {
		t.Errorf("expected false for wrong token")
	}

	// 5. State getters with empty state
	emptySrv := &Server{}
	if emptySrv.GetConfig() == nil {
		t.Errorf("expected non-nil default config")
	}
	if emptySrv.GetEvaluator() != nil {
		t.Errorf("expected nil evaluator")
	}
	if emptySrv.GetRegistry() != nil {
		t.Errorf("expected nil registry")
	}
	emptySrv.armWatchdog(nil, time.Second)
	emptySrv.SetTuner(nil)
	emptySrv.SetDiskStore(nil)

	// 6. allowProvider / recordProviderSuccess / recordProviderFailure
	cbProv := provider.NewOpenAICompatibleProvider("ollama", contract.ProviderConfig{Type: provider.TypeOpenAICompatible, Local: true, BaseURL: "http://localhost:11434"})
	if !srv.allowProvider(cbProv) {
		t.Errorf("expected allowProvider true")
	}
	srv.recordProviderSuccess(cbProv)
	srv.recordProviderFailure(cbProv)
}

func TestProxy_AgentShield_NonStreamingRescue(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"index":0,"message":{"role":"assistant","content":"I have created the plan. Are you satisfied with this architecture?"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock_local": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
		},
		Tiers: []contract.Tier{
			{Name: "Local GPU", Model: "qwen", Provider: "mock_local", When: "true"},
		},
	}

	srv := NewServer(cfg, nil, nil, nil)
	rec := httptest.NewRecorder()
	reqBody := `{"model":"qwen","tools":[{"type":"function","function":{"name":"ask_followup_question","description":"Ask user a question"}}],"messages":[{"role":"user","content":"Plan the project"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	choices := res["choices"].([]interface{})
	msg := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	toolCalls, ok := msg["tool_calls"].([]interface{})
	if !ok || len(toolCalls) == 0 {
		t.Fatalf("expected tool_calls in rescued non-streaming response, got: %v", res)
	}

	fn := toolCalls[0].(map[string]interface{})["function"].(map[string]interface{})
	if fn["name"] != "ask_followup_question" {
		t.Fatalf("expected tool name 'ask_followup_question', got %v", fn["name"])
	}
}

func TestProxy_Phase3_DirectivesAndFeatureFlags(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"index":0,"message":{"role":"assistant","content":"I have created the plan. Do you want to proceed?"}}]}`))
	}))
	defer mockUpstream.Close()

	t.Run("@nacho:no-shield directive suppresses synthetic tool call", func(t *testing.T) {
		cfg := &contract.Config{
			Port: 8000,
			Providers: map[string]contract.ProviderConfig{
				"mock_local": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
			},
			Tiers: []contract.Tier{
				{Name: "Local GPU", Model: "qwen", Provider: "mock_local", When: "true"},
			},
		}

		srv := NewServer(cfg, nil, nil, nil)
		rec := httptest.NewRecorder()
		reqBody := `{"model":"qwen","tools":[{"type":"function","function":{"name":"ask_followup_question","description":"Ask user a question"}}],"messages":[{"role":"user","content":"@nacho:no-shield Plan the project"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var res map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		choices := res["choices"].([]interface{})
		msg := choices[0].(map[string]interface{})["message"].(map[string]interface{})
		if _, hasTools := msg["tool_calls"]; hasTools {
			t.Fatalf("expected NO tool_calls when @nacho:no-shield is passed, got: %v", msg)
		}
		if msg["content"] != "I have created the plan. Do you want to proceed?" {
			t.Fatalf("unexpected content: %v", msg["content"])
		}
	})

	t.Run("Tier with raw: true passes through without modification", func(t *testing.T) {
		rawTrue := true
		cfg := &contract.Config{
			Port: 8000,
			Providers: map[string]contract.ProviderConfig{
				"mock_local": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
			},
			Tiers: []contract.Tier{
				{Name: "Raw Tier", Model: "qwen", Provider: "mock_local", Raw: &rawTrue, When: "true"},
			},
		}

		srv := NewServer(cfg, nil, nil, nil)
		rec := httptest.NewRecorder()
		reqBody := `{"model":"qwen","tools":[{"type":"function","function":{"name":"ask_followup_question"}}],"messages":[{"role":"user","content":"Plan the project"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var res map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		choices := res["choices"].([]interface{})
		msg := choices[0].(map[string]interface{})["message"].(map[string]interface{})
		if _, hasTools := msg["tool_calls"]; hasTools {
			t.Fatalf("expected NO tool_calls on raw tier, got: %v", msg)
		}
	})

	t.Run("Tier with shield: false disables agentic fallback", func(t *testing.T) {
		shieldFalse := false
		cfg := &contract.Config{
			Port: 8000,
			Providers: map[string]contract.ProviderConfig{
				"mock_local": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible, Local: true},
			},
			Tiers: []contract.Tier{
				{Name: "No Shield Tier", Model: "qwen", Provider: "mock_local", Shield: &shieldFalse, When: "true"},
			},
		}

		srv := NewServer(cfg, nil, nil, nil)
		rec := httptest.NewRecorder()
		reqBody := `{"model":"qwen","tools":[{"type":"function","function":{"name":"ask_followup_question"}}],"messages":[{"role":"user","content":"Plan the project"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var res map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		choices := res["choices"].([]interface{})
		msg := choices[0].(map[string]interface{})["message"].(map[string]interface{})
		if _, hasTools := msg["tool_calls"]; hasTools {
			t.Fatalf("expected NO tool_calls on shield: false tier, got: %v", msg)
		}
	})
}

func TestResolveFeatureFlags_AllPermutations(t *testing.T) {
	bTrue := true
	bFalse := false

	// 1. Explicit directive non-zero / non-default overrides everything
	customFeatures := uint16(router.FeatureToolNormalizer | router.FeatureNormMarkdown)
	flags := resolveFeatureFlags(contract.RequestContext{Features: customFeatures}, contract.Tier{Raw: &bTrue}, true)
	if uint16(flags) != customFeatures {
		t.Fatalf("expected explicit features %d, got %d", customFeatures, flags)
	}

	// 2. Raw tier policy
	flags = resolveFeatureFlags(contract.RequestContext{}, contract.Tier{Raw: &bTrue}, true)
	if flags != router.FeatureRawPassThrough {
		t.Fatalf("expected FeatureRawPassThrough, got %v", flags)
	}

	// 3. Global shield disabled
	flags = resolveFeatureFlags(contract.RequestContext{}, contract.Tier{}, false)
	if flags.Has(router.FeatureShieldEnabled) {
		t.Fatalf("expected FeatureShieldEnabled to be false when global shield is false")
	}

	// 4. Tier Shield explicitly enabled
	flags = resolveFeatureFlags(contract.RequestContext{}, contract.Tier{Shield: &bTrue}, false)
	if !flags.Has(router.FeatureShieldEnabled) {
		t.Fatalf("expected FeatureShieldEnabled to be true when tier.Shield is true")
	}

	// 5. Tier Shield explicitly disabled
	flags = resolveFeatureFlags(contract.RequestContext{}, contract.Tier{Shield: &bFalse}, true)
	if flags.Has(router.FeatureShieldEnabled) {
		t.Fatalf("expected FeatureShieldEnabled to be false when tier.Shield is false")
	}

	// 6. Tier Normalizer disabled
	flags = resolveFeatureFlags(contract.RequestContext{}, contract.Tier{Normalizer: &bFalse}, true)
	if flags.Has(router.FeatureToolNormalizer) {
		t.Fatalf("expected FeatureToolNormalizer to be false when tier.Normalizer is false")
	}

	// 7. Tier Normalizers struct permutations
	flags = resolveFeatureFlags(contract.RequestContext{}, contract.Tier{
		Normalizers: &contract.NormalizersConfig{
			Enabled:  &bFalse,
			Markdown: &bFalse,
			BareJSON: &bFalse,
			ReAct:    &bFalse,
			Think:    &bFalse,
		},
	}, true)
	if flags.Has(router.FeatureToolNormalizer) || flags.Has(router.FeatureNormMarkdown) || flags.Has(router.FeatureNormBareJSON) || flags.Has(router.FeatureNormReAct) || flags.Has(router.FeatureThinkNormalizer) {
		t.Fatalf("expected normalizer sub-flags to be disabled, got %d", flags)
	}
}

func TestStreamNormalizer_Features_RawPassThrough(t *testing.T) {
	rawSSE := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"raw thinking\"}}]}\n\ndata: [DONE]\n\n"
	norm := NewStreamNormalizer(io.NopCloser(strings.NewReader(rawSSE)))
	norm.SetFeatures(uint16(router.FeatureRawPassThrough))

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	// Raw pass-through should preserve the reasoning_content delta verbatim without adding <think> tags
	if !strings.Contains(string(out), "reasoning_content") || strings.Contains(string(out), "<think>") {
		t.Fatalf("expected raw pass-through without <think> tags, got: %s", string(out))
	}
}

func TestResolveTierVision(t *testing.T) {
	bTrue := true
	bFalse := false
	oracle := telemetry.NewPricingOracle()

	tests := []struct {
		name     string
		tier     contract.Tier
		expected bool
	}{
		{
			name: "explicit has_vision = true in YAML",
			tier: contract.Tier{
				Model:     "custom/my-text-model",
				HasVision: &bTrue,
			},
			expected: true,
		},
		{
			name: "explicit has_vision = false in YAML overrides model name",
			tier: contract.Tier{
				Model:     "google/gemini-3.7-flash",
				HasVision: &bFalse,
			},
			expected: false,
		},
		{
			name: "strip_images = true forces false even with has_vision = true",
			tier: contract.Tier{
				Model:       "google/gemini-3.7-flash",
				HasVision:   &bTrue,
				StripImages: true,
			},
			expected: false,
		},
		{
			name: "gemini family automatically inferred as vision",
			tier: contract.Tier{
				Model: "google/gemini-2.5-pro",
			},
			expected: true,
		},
		{
			name: "claude family automatically inferred as vision",
			tier: contract.Tier{
				Model: "anthropic/claude-sonnet-5",
			},
			expected: true,
		},
		{
			name: "gpt-4o family automatically inferred as vision",
			tier: contract.Tier{
				Model: "openai/gpt-4o-mini",
			},
			expected: true,
		},
		{
			name: "vl family automatically inferred as vision",
			tier: contract.Tier{
				Model: "qwen/qwen-2.5-vl-72b",
			},
			expected: true,
		},
		{
			name: "qwen coder plus has no vision",
			tier: contract.Tier{
				Model:    "qwen/qwen3-coder-plus",
				Provider: "openrouter",
			},
			expected: false,
		},
		{
			name: "deepseek r1 has no vision",
			tier: contract.Tier{
				Model:    "deepseek/deepseek-r1",
				Provider: "openrouter",
			},
			expected: false,
		},
		{
			name: "random model with word flash is NOT assumed to have vision",
			tier: contract.Tier{
				Model:    "mistral/mistral-flash-code",
				Provider: "openrouter",
			},
			expected: false,
		},
		{
			name: "unknown model defaults safely to false",
			tier: contract.Tier{
				Model:    "unknown/local-model:7b",
				Provider: "ollama",
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tierCopy := tc.tier
			ResolveTierVision(&tierCopy, oracle)
			if tierCopy.ResolvedHasVision != tc.expected {
				t.Errorf("ResolveTierVision(%s) = %v; want %v", tc.tier.Model, tierCopy.ResolvedHasVision, tc.expected)
			}
		})
	}
}

type ntsCaptureSink struct {
	mu      sync.Mutex
	records []telemetry.TurnRecord
}

func (c *ntsCaptureSink) Emit(r telemetry.TurnRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r)
}

func (c *ntsCaptureSink) Close() error { return nil }

func TestProxy_NTSTokenSaver_CompactsToolOutputAndRecordsTelemetry(t *testing.T) {
	var receivedBody []byte
	var bodyMu sync.Mutex

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyMu.Lock()
		receivedBody, _ = io.ReadAll(r.Body)
		bodyMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-test",
			"choices": [{
				"finish_reason": "stop",
				"message": {"role": "assistant", "content": "Done"}
			}],
			"usage": {"prompt_tokens": 100, "completion_tokens": 10, "total_tokens": 110}
		}`))
	}))
	defer mockUpstream.Close()

	bTrue := true
	cfg := &contract.Config{
		Port: 8000,
		NTS: contract.NTSConfig{
			Enabled: &bTrue,
		},
		Providers: map[string]contract.ProviderConfig{
			"mock": {
				BaseURL: mockUpstream.URL,
				Type:    provider.TypeOpenAICompatible,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Cloud Tier",
				Model:    "claude-3-5-sonnet",
				Provider: "mock",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	oracle := telemetry.NewPricingOracle()
	tracker := telemetry.NewStatsTracker(10)
	sink := &ntsCaptureSink{}
	tracker.AddSink(sink)

	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, classifier, sanitizer, oracle, tracker, nil, slog.Default())

	// Request with tool response containing:
	// 1. ANSI escapes: \x1b[31mError\x1b[0m
	// 2. Carriage return overwrite: Loading...\rComplete!
	// 3. Duplicate warning storm: Warning: Deprecated API repeated 4 times
	// And a read_file tool response which must be 100% immune!
	toolOutputContent := "\x1b[31mError: build failed\x1b[0m\nLoading...\rComplete!\nWarning: Deprecated API\nWarning: Deprecated API\nWarning: Deprecated API\nWarning: Deprecated API\n"
	fileReadContent := "package main\n\n// Intentionally has \x1b[31m escaped comments\nfunc main() {}\n"

	reqPayload := map[string]interface{}{
		"model": "nacho-hybrid",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call_bash_1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "execute_command",
							"arguments": `{"command":"go build"}`,
						},
					},
					map[string]interface{}{
						"id":   "call_read_1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path":"main.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_bash_1",
				"content":      toolOutputContent,
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_read_1",
				"content":      fileReadContent,
			},
		},
	}

	payloadBytes, _ := json.Marshal(reqPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(payloadBytes))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	bodyMu.Lock()
	upstreamBytes := receivedBody
	bodyMu.Unlock()

	// 1. Tool output should have ANSI stripped
	if bytes.Contains(upstreamBytes, []byte("\x1b[31mError")) {
		t.Errorf("Expected ANSI codes to be stripped from tool output, but found in upstream body: %s", string(upstreamBytes))
	}
	// 2. Duplicate warnings should be collapsed
	if !bytes.Contains(upstreamBytes, []byte("identical line repeated 3 times")) {
		t.Errorf("Expected duplicate warnings to be collapsed, but notice not found: %s", string(upstreamBytes))
	}
	// 3. File read tool output MUST be preserved bit-for-bit (immunity test)
	if !bytes.Contains(upstreamBytes, []byte(`\u001b[31m escaped comments`)) {
		t.Errorf("Expected read_file output to retain ANSI codes (immune), but it was stripped: %s", string(upstreamBytes))
	}

	tracker.Flush()

	sink.mu.Lock()
	records := sink.records
	sink.mu.Unlock()

	if len(records) == 0 {
		t.Fatalf("Expected telemetry turn record to be emitted, got none")
	}

	rec0 := records[0]
	if rec0.NTSTokensSaved <= 0 {
		t.Errorf("Expected NTSTokensSaved > 0, got %d", rec0.NTSTokensSaved)
	}
	if rec0.NTSBytesSaved <= 0 {
		t.Errorf("Expected NTSBytesSaved > 0, got %d", rec0.NTSBytesSaved)
	}
}

func TestProxy_NTSTokenSaver_AnthropicMessages(t *testing.T) {
	var receivedBody []byte
	var bodyMu sync.Mutex

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyMu.Lock()
		receivedBody, _ = io.ReadAll(r.Body)
		bodyMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg-test",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "OK"}],
			"usage": {"input_tokens": 50, "output_tokens": 10}
		}`))
	}))
	defer mockUpstream.Close()

	bTrue2 := true
	cfg := &contract.Config{
		Port: 8000,
		NTS:  contract.NTSConfig{Enabled: &bTrue2},
		Providers: map[string]contract.ProviderConfig{
			"mock": {BaseURL: mockUpstream.URL, Type: provider.TypeOpenAICompatible},
		},
		Tiers: []contract.Tier{
			{Name: "Sonnet", Model: "claude-3-5-sonnet", Provider: "mock", When: "true"},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, router.NewClassifier(), router.NewSanitizer(), telemetry.NewPricingOracle(), telemetry.NewStatsTracker(10), nil, slog.Default())

	reqPayload := map[string]interface{}{
		"model": "claude-3-5-sonnet",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "toolu_exec_1",
						"content":     "\x1b[32mBuild Success!\x1b[0m\n\n\n\n\nDone\n",
					},
				},
			},
		},
	}

	payloadBytes, _ := json.Marshal(reqPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(payloadBytes))
	req.Header.Set("anthropic-version", "2023-06-01")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	bodyMu.Lock()
	upstreamBytes := receivedBody
	bodyMu.Unlock()

	if bytes.Contains(upstreamBytes, []byte("\x1b[32m")) {
		t.Errorf("Expected ANSI stripped from Anthropic tool_result, got: %s", string(upstreamBytes))
	}
	if bytes.Contains(upstreamBytes, []byte("\n\n\n\n")) {
		t.Errorf("Expected blank line cascade collapsed in Anthropic tool_result, got: %s", string(upstreamBytes))
	}
}

func TestInjectCorrectionPrompt(t *testing.T) {
	// 1. Valid body with messages
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	res := injectCorrectionPrompt(body, "Please stop looping")
	if !strings.Contains(string(res), "Please stop looping") {
		t.Errorf("expected injected prompt, got %s", string(res))
	}

	// 2. Empty prompt falls back to default
	resDefault := injectCorrectionPrompt(body, "")
	if !strings.Contains(string(resDefault), contract.CycleBreakerDefaultCorrectionPrompt) {
		t.Errorf("expected default correction prompt, got %s", string(resDefault))
	}

	// 3. Invalid JSON returns body unchanged
	invalidJSON := []byte(`{invalid`)
	if string(injectCorrectionPrompt(invalidJSON, "prompt")) != string(invalidJSON) {
		t.Errorf("expected invalid JSON returned unchanged")
	}

	// 4. Missing messages field returns body unchanged
	noMessages := []byte(`{"model":"gpt-4"}`)
	if string(injectCorrectionPrompt(noMessages, "prompt")) != string(noMessages) {
		t.Errorf("expected payload without messages returned unchanged")
	}

	// 5. Tool order preserved, no \u003c, prefix byte-identical
	reqWithTools := []byte(`{"tools":[{"name":"z_tool"},{"name":"a_tool"}],"messages":[{"role":"user","content":"a < b & c"}],"seed":99999999999999999}`)
	origCopy := append([]byte(nil), reqWithTools...)
	promptWithSpecialChars := `Refactor: ensure <tag> & "quotes" and 'single' \ backslash works!`
	resSpecial := injectCorrectionPrompt(reqWithTools, promptWithSpecialChars)

	// Input buffer must remain unchanged
	if !bytes.Equal(reqWithTools, origCopy) {
		t.Errorf("injectCorrectionPrompt mutated the input slice!")
	}

	// Tool order must be preserved (not alphabetical)
	zIdx := bytes.Index(resSpecial, []byte("z_tool"))
	aIdx := bytes.Index(resSpecial, []byte("a_tool"))
	if zIdx == -1 || aIdx == -1 || zIdx > aIdx {
		t.Errorf("Tool order was not preserved in injectCorrectionPrompt: %s", string(resSpecial))
	}

	// No \u003c introduced
	if bytes.Contains(resSpecial, []byte(`\u003c`)) {
		t.Errorf("injectCorrectionPrompt introduced \\u003c: %s", string(resSpecial))
	}

	// Must be valid JSON
	if !json.Valid(resSpecial) {
		t.Errorf("injectCorrectionPrompt produced invalid JSON: %s", string(resSpecial))
	}

	// 6. Empty messages array
	emptyMsgBody := []byte(`{"model":"gpt-4","messages":[]}`)
	resEmpty := injectCorrectionPrompt(emptyMsgBody, "wake up")
	if !json.Valid(resEmpty) {
		t.Errorf("injectCorrectionPrompt on empty messages produced invalid JSON: %s", string(resEmpty))
	}
	if !strings.Contains(string(resEmpty), `[{"role":"user","content":"wake up"}]`) {
		t.Errorf("unexpected output on empty messages: %s", string(resEmpty))
	}

	// 7. Non-array messages field returns body unchanged
	notAnArray := []byte(`{"messages":"not-an-array"}`)
	if string(injectCorrectionPrompt(notAnArray, "p")) != string(notAnArray) {
		t.Errorf("expected string messages returned unchanged")
	}

	// 8. Unclosed messages array returns body unchanged
	unclosedArray := []byte(`{"messages": [`)
	if string(injectCorrectionPrompt(unclosedArray, "p")) != string(unclosedArray) {
		t.Errorf("expected unclosed messages returned unchanged")
	}
}

func TestServer_ServeHTTP_EdgeBranches(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	srv.startTime = time.Now().Add(-10 * time.Minute)

	// 1. /v1/health with uptime populated
	recHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest(http.MethodGet, contract.PathV1Health, nil)
	srv.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK || !strings.Contains(recHealth.Body.String(), "uptime") {
		t.Errorf("expected 200 with uptime, got %d: %s", recHealth.Code, recHealth.Body.String())
	}

	// 2. GET /v1/chat/completions -> 405 Method Not Allowed
	recChatGet := httptest.NewRecorder()
	reqChatGet := httptest.NewRequest(http.MethodGet, contract.PathChatCompletions, nil)
	reqChatGet.Header.Set("Authorization", "Bearer test-secret-token")
	srv.ServeHTTP(recChatGet, reqChatGet)
	if recChatGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET chat completions, got %d", recChatGet.Code)
	}

	// 3. /v1/stats
	recStats := httptest.NewRecorder()
	reqStats := httptest.NewRequest(http.MethodGet, contract.PathStats, nil)
	reqStats.Header.Set("Authorization", "Bearer test-secret-token")
	srv.ServeHTTP(recStats, reqStats)
	if recStats.Code != http.StatusOK {
		t.Errorf("expected 200 for /v1/stats, got %d", recStats.Code)
	}
}

func TestServer_NTS_HotReload_EndToEnd_BoilerplateProof(t *testing.T) {
	var mu sync.Mutex
	var receivedBody []byte

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBody = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"OK"}}]}`))
	}))
	defer mockUpstream.Close()

	bTrue := true
	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock": {
				BaseURL: mockUpstream.URL,
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Mock Tier",
				Model:    "mock-model",
				Provider: "mock",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Mock Tier",
			Model:    "mock-model",
			Provider: "mock",
		},
		NTS: contract.NTSConfig{
			Enabled:          &bTrue,
			StripBoilerplate: &bTrue,
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier, cfg.Providers)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	testPayload := `{
		"model": "mock-model",
		"messages": [
			{"role": "user", "content": "run tests"},
			{"role": "assistant", "content": "", "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "execute_command", "arguments": "{}"}}]},
			{"role": "tool", "tool_call_id": "call_1", "name": "execute_command", "content": "Running test suite\n(Use ` + "`node --trace-warnings ...`" + ` to show where the warning was created)\nAll tests passed"}
		]
	}`

	// 1. Turn 1: NTS enabled with strip_boilerplate: true
	req1 := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(testPayload))
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on turn 1, got %d: %s", rec1.Code, rec1.Body.String())
	}
	mu.Lock()
	body1 := string(receivedBody)
	mu.Unlock()
	if strings.Contains(body1, "trace-warnings") {
		t.Fatalf("expected boilerplate to be STRIPPED on turn 1, but upstream received:\n%s", body1)
	}

	// 2. Hot-Reload: Toggle NTS to enabled: false WITHOUT restarting server
	bFalse := false
	hotReloadCfg := *cfg
	hotReloadCfg.NTS = contract.NTSConfig{
		Enabled: &bFalse,
	}
	if _, err := srv.ApplyConfig(&hotReloadCfg, false); err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}

	// 3. Turn 2: Exact same payload sent to running server
	req2 := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(testPayload))
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on turn 2, got %d: %s", rec2.Code, rec2.Body.String())
	}
	mu.Lock()
	body2 := string(receivedBody)
	mu.Unlock()
	if !strings.Contains(body2, "trace-warnings") {
		t.Fatalf("expected boilerplate to be PRESERVED after hot-reload disabled NTS, but it was missing:\n%s", body2)
	}
}

func TestProxy_LocalProvider_InboundHistoryScrubbed(t *testing.T) {
	var receivedBody string
	var mu sync.Mutex

	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBody = string(b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer mockOllama.Close()

	cfg := &contract.Config{
		DefaultTier: contract.Tier{
			Name:     "local-tier",
			Provider: "ollama",
			Model:    "gemma-4",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				Type:    provider.TypeOpenAICompatible,
				Local:   true,
				BaseURL: mockOllama.URL,
			},
		},
	}

	srv := NewServer(cfg, nil, nil, nil)

	// Turn 3 historical payload with delimiter bleed in a tool message
	payload := `{"model":"nacho-hybrid","messages":[` +
		`{"role":"user","content":"create dir"},` +
		`{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"execute_command","arguments":"{\"command\":\"mkdir internal_cli<|\\\"|>}\"}"}}]},` +
		`{"role":"tool","tool_call_id":"call_1","content":"Set-Location: parameter error}]<|\"|>}"}` +
		`]}`

	req := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	mu.Lock()
	body := receivedBody
	mu.Unlock()

	// Verify delimiter bleed is completely scrubbed before reaching local provider
	if strings.Contains(body, `<|`) || strings.Contains(body, `|>`) {
		t.Fatalf("expected control tokens to be scrubbed for local provider, got:\n%s", body)
	}
	if !strings.Contains(body, "parameter error}]") {
		t.Fatalf("expected clean content to be preserved, got:\n%s", body)
	}
}

func TestProxy_CloudProvider_HistoryPreserved(t *testing.T) {
	var receivedBody string
	var mu sync.Mutex

	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBody = string(b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer mockCloud.Close()

	cfg := &contract.Config{
		DefaultTier: contract.Tier{
			Name:     "cloud-tier",
			Provider: "openai",
			Model:    "gpt-4o",
		},
		Providers: map[string]contract.ProviderConfig{
			"openai": {
				Type:    provider.TypeOpenAICompatible,
				BaseURL: mockCloud.URL,
			},
		},
	}

	srv := NewServer(cfg, nil, nil, nil)

	// Payload with literal prompt tutorial mentioning tokens
	payload := `{"model":"nacho-hybrid","messages":[` +
		`{"role":"user","content":"How does <start_of_turn> work in LLMs?"}` +
		`]}`

	req := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(payload))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	mu.Lock()
	body := receivedBody
	mu.Unlock()

	// Verify cloud provider requests do NOT strip prompt tokens from user prompt
	if !strings.Contains(body, "<start_of_turn>") && !strings.Contains(body, `\u003cstart_of_turn\u003e`) {
		t.Fatalf("expected user prompt to be preserved for cloud provider, got:\n%s", body)
	}
}

func TestPipeline_IngressNTSCompaction_RoutesToLowerTier(t *testing.T) {
	var localHits, cloudHits int
	var mu sync.Mutex

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		localHits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-local","choices":[{"index":0,"message":{"role":"assistant","content":"local response"}}]}`))
	}))
	defer mockLocal.Close()

	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		cloudHits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-cloud","choices":[{"index":0,"message":{"role":"assistant","content":"cloud response"}}]}`))
	}))
	defer mockCloud.Close()

	bTrue := true
	cfg := &contract.Config{
		Port: 8000,
		NTS: contract.NTSConfig{
			Enabled:   &bTrue,
			StripANSI: &bTrue,
			ResolveCR: &bTrue,
		},
		Providers: map[string]contract.ProviderConfig{
			"local-prov": {BaseURL: mockLocal.URL, Type: provider.TypeOpenAICompatible, Local: true},
			"cloud-prov": {BaseURL: mockCloud.URL, Type: provider.TypeOpenAICompatible},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Small Local Context",
				Model:    "local-model",
				Provider: "local-prov",
				When:     "Tokens < 4000",
			},
			{
				Name:     "Tier 2: Cloud Fallback",
				Model:    "cloud-model",
				Provider: "cloud-prov",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 2: Cloud Fallback",
			Model:    "cloud-model",
			Provider: "cloud-prov",
			When:     "true",
		},
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		t.Fatalf("failed to create evaluator: %v", err)
	}

	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	// Construct tool progress payload:
	// Base context: ~8,000 bytes (~2,000 tokens)
	// Tool output with ANSI progress: ~10,000 bytes (~2,500 tokens)
	// Total raw: ~18,000 bytes (~4,500 tokens)
	ansiToolProgress := strings.Repeat("\x1b[32m[Progress] item 1234567890\x1b[0m\r", 250) + "Final complete line\n"
	basePrompt := strings.Repeat("system and user instructions context text. ", 180)

	payloadMap := map[string]interface{}{
		"model": "nacho-hybrid",
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": basePrompt,
			},
			map[string]interface{}{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id":   "call_download_1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "download_data",
							"arguments": "{}",
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_download_1",
				"content":      ansiToolProgress,
			},
			map[string]interface{}{
				"role":    "user",
				"content": "Please analyze the downloaded data.",
			},
		},
	}

	bodyBytes, err := json.Marshal(payloadMap)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	// Verify raw tokens exceed 4000
	rawCtx, err := classifier.Classify(bodyBytes)
	if err != nil {
		t.Fatalf("failed to classify raw payload: %v", err)
	}
	if rawCtx.Tokens < 4000 {
		t.Fatalf("expected rawCtx.Tokens >= 4000, got %d (len=%d)", rawCtx.Tokens, len(bodyBytes))
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	mu.Lock()
	lHits := localHits
	cHits := cloudHits
	mu.Unlock()

	// Prior to fix, raw tokens (~4500) >= 4000 causes routing to Tier 2 (cloudHits=1, localHits=0).
	// With ingress NTS compaction, compacted tokens (~2125) < 4000 routes to Tier 1 (localHits=1, cloudHits=0).
	if lHits != 1 || cHits != 0 {
		t.Fatalf("Ingress compaction routing failure: expected Tier 1 (localHits=1, cloudHits=0), got localHits=%d, cloudHits=%d", lHits, cHits)
	}
}

type mockSimpleLLMProvider struct {
	id      string
	name    string
	baseURL string
	isLocal bool
	apiKey  string
	headers map[string]string
}

func (m *mockSimpleLLMProvider) ID() string      { return m.id }
func (m *mockSimpleLLMProvider) Name() string    { return m.name }
func (m *mockSimpleLLMProvider) BaseURL() string { return m.baseURL }
func (m *mockSimpleLLMProvider) IsLocal() bool   { return m.isLocal }
func (m *mockSimpleLLMProvider) Platform() contract.Platform {
	if m.isLocal {
		return contract.PlatformLocal
	}
	return contract.PlatformOpenAICompatible
}
func (m *mockSimpleLLMProvider) GetAPIKey() string             { return m.apiKey }
func (m *mockSimpleLLMProvider) GetHeaders() map[string]string { return m.headers }

func TestServer_Dispatch_FallbackAndHelpersCoverage(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	srv := NewServer(cfg, nil, nil, nil)

	// 1. buildOutboundRequest with custom LLMProvider (non-ProtocolProvider) with /v1 prefix, auth, headers
	pCustom := &mockSimpleLLMProvider{
		id:      "custom-cloud",
		name:    "Custom Cloud",
		baseURL: "https://api.example.com/v1",
		isLocal: false,
		apiKey:  "sk-test-key-123",
		headers: map[string]string{"X-Test-Hdr": "custom-val"},
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	r.Header.Set("User-Agent", "NachoFlowTest")
	body := []byte(`{"model":"nacho-hybrid"}`)

	outReq, err := srv.buildOutboundRequest(context.Background(), r, pCustom, "gpt-4o", body)
	if err != nil {
		t.Fatalf("unexpected error building outbound request: %v", err)
	}
	if outReq.Header.Get("Authorization") != "Bearer sk-test-key-123" {
		t.Errorf("expected Bearer token, got: %s", outReq.Header.Get("Authorization"))
	}
	if outReq.Header.Get("X-Test-Hdr") != "custom-val" {
		t.Errorf("expected custom header, got: %s", outReq.Header.Get("X-Test-Hdr"))
	}

	// 2. buildOutboundRequest with local provider (should delete Authorization header)
	pLocal := &mockSimpleLLMProvider{
		id:      "custom-local",
		name:    "Custom Local",
		baseURL: "http://127.0.0.1:11434",
		isLocal: true,
	}
	rLocal := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	rLocal.Header.Set("Authorization", "Bearer stale-token")
	outReqLocal, err := srv.buildOutboundRequest(context.Background(), rLocal, pLocal, "llama-3", body)
	if err != nil {
		t.Fatalf("unexpected error building local request: %v", err)
	}
	if outReqLocal.Header.Get("Authorization") != "" {
		t.Errorf("expected Authorization header deleted for local provider, got: %s", outReqLocal.Header.Get("Authorization"))
	}

	// 3. buildOutboundRequest with malformed URL
	pBadURL := &mockSimpleLLMProvider{
		id:      "bad-url",
		baseURL: "://invalid-url",
	}
	_, errBad := srv.buildOutboundRequest(context.Background(), r, pBadURL, "bad-model", body)
	if errBad == nil {
		t.Errorf("expected error for malformed URL, got nil")
	}

	// 4. wrapResponseStream and translateResponseBody with simple LLMProvider fallback
	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader("sample response stream")),
	}
	wrapped := srv.wrapResponseStream(resp, pCustom)
	if wrapped != resp.Body {
		t.Errorf("expected fallback wrapResponseStream to return resp.Body")
	}

	translated, err := srv.translateResponseBody(http.StatusOK, []byte("raw response"), pCustom)
	if err != nil || string(translated) != "raw response" {
		t.Errorf("expected fallback translateResponseBody to return raw bytes, got %s, err=%v", string(translated), err)
	}

	// 5. preparePayload with StripImages and control tokens
	tierWithStripImages := contract.Tier{
		Name:        "Test Tier",
		Model:       "test-model",
		StripImages: true,
	}
	bodyWithControlTokens := []byte(`{"model":"old","messages":[{"role":"user","content":"<|im_start|>system\ntest<|im_end|>"}]}`)
	prepared := srv.preparePayload(&tierWithStripImages, pLocal, bodyWithControlTokens)
	if len(prepared) == 0 {
		t.Errorf("expected non-empty prepared body")
	}
}

func TestBuildNTSTransformer_AllBranches(t *testing.T) {
	bTrue := true
	bFalse := false
	turns := 2

	// 1. Disabled
	trNil := BuildNTSTransformer(contract.NTSConfig{Enabled: &bFalse})
	if trNil != nil {
		t.Fatalf("expected nil transformer when disabled")
	}

	// 2. All branches enabled
	cfg := contract.NTSConfig{
		Enabled:                  &bTrue,
		StripANSI:                &bTrue,
		ResolveCR:                &bTrue,
		DeduplicateLines:         &bTrue,
		DedupThreshold:           4,
		StripBoilerplate:         &bTrue,
		NormalizeWhitespace:      &bTrue,
		PreserveFileReads:        &bTrue,
		PreserveFileWrites:       &bTrue,
		PreserveCacheControl:     &bTrue,
		CompactStaleFileReads:    &bTrue,
		StaleReadDepth:           5,
		StripHistoricalReasoning: &bTrue,
		KeepRecentReasoningTurns: &turns,
		DeduplicateReminders:     &bTrue,
		RemindersRefreshInterval: 7,
		CollapseVerboseTests:     &bTrue,
		DeduplicateToolOutputs:   &bTrue,
		MinDuplicateOutputBytes:  350,
	}
	tr := BuildNTSTransformer(cfg)
	if tr == nil {
		t.Fatalf("expected non-nil transformer")
	}
	if tr.Pipeline().Config().DedupThreshold != 4 {
		t.Errorf("expected DedupThreshold 4, got %d", tr.Pipeline().Config().DedupThreshold)
	}
	if tr.Pipeline().Config().StaleReadDepth != 5 {
		t.Errorf("expected StaleReadDepth 5, got %d", tr.Pipeline().Config().StaleReadDepth)
	}
	if tr.Pipeline().Config().KeepRecentReasoningTurns != 2 {
		t.Errorf("expected KeepRecentReasoningTurns 2, got %d", tr.Pipeline().Config().KeepRecentReasoningTurns)
	}
	if !tr.Pipeline().Config().DeduplicateReminders {
		t.Errorf("expected DeduplicateReminders true")
	}
	if tr.Pipeline().Config().RemindersRefreshInterval != 7 {
		t.Errorf("expected RemindersRefreshInterval 7, got %d", tr.Pipeline().Config().RemindersRefreshInterval)
	}
	if !tr.Pipeline().Config().CollapseVerboseTests {
		t.Errorf("expected CollapseVerboseTests true")
	}
	if !tr.Pipeline().Config().DeduplicateToolOutputs {
		t.Errorf("expected DeduplicateToolOutputs true")
	}
	if tr.Pipeline().Config().MinDuplicateOutputBytes != 350 {
		t.Errorf("expected MinDuplicateOutputBytes 350, got %d", tr.Pipeline().Config().MinDuplicateOutputBytes)
	}
}

func TestPreparePayload_NoHTMLEscaping(t *testing.T) {
	srv := &Server{}
	tier := &contract.Tier{
		Model: "test-model",
	}
	// Body containing HTML characters: <, >, &
	body := []byte(`{"model":"old","messages":[{"role":"user","content":"if (a < b && c > d) return true;"}]}`)
	prepared := srv.preparePayload(tier, nil, body)

	if bytes.Contains(prepared, []byte(`\u003c`)) || bytes.Contains(prepared, []byte(`\u003e`)) || bytes.Contains(prepared, []byte(`\u0026`)) {
		t.Errorf("preparePayload introduced HTML escape sequences: %s", string(prepared))
	}
	if !bytes.Contains(prepared, []byte("a < b && c > d")) {
		t.Errorf("preparePayload did not preserve literal HTML characters: %s", string(prepared))
	}
}

func TestPreparePayload_PreservesKeyOrderAndPrecision(t *testing.T) {
	srv := &Server{}
	tier := &contract.Tier{
		Model:           "new-model-xyz",
		ReasoningEffort: "high",
	}

	// Payload with custom key order and a >2^53 integer
	body := []byte(`{"tools":[{"name":"bash"}],"model":"old","seed":9007199254740993,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	prepared := srv.preparePayload(tier, nil, body)

	// Top-level key order must be preserved: tools before model, model before seed, seed before stream, stream before messages
	toolsIdx := bytes.Index(prepared, []byte(`"tools"`))
	modelIdx := bytes.Index(prepared, []byte(`"model"`))
	seedIdx := bytes.Index(prepared, []byte(`"seed"`))
	streamIdx := bytes.Index(prepared, []byte(`"stream"`))
	msgsIdx := bytes.Index(prepared, []byte(`"messages"`))

	if !(toolsIdx < modelIdx && modelIdx < seedIdx && seedIdx < streamIdx && streamIdx < msgsIdx) {
		t.Errorf("Top-level key order was not preserved in preparePayload: %s", string(prepared))
	}

	// Seed must retain exact value
	if !bytes.Contains(prepared, []byte(`9007199254740993`)) {
		t.Errorf("seed precision lost: %s", string(prepared))
	}

	// Model must be replaced
	if !bytes.Contains(prepared, []byte(`"new-model-xyz"`)) {
		t.Errorf("model was not updated: %s", string(prepared))
	}

	// stream_options must include include_usage: true
	if !bytes.Contains(prepared, []byte(`"include_usage":true`)) && !bytes.Contains(prepared, []byte(`"include_usage": true`)) {
		t.Errorf("stream_options missing include_usage: %s", string(prepared))
	}

	// Output must be valid JSON
	if !json.Valid(prepared) {
		t.Errorf("preparePayload produced invalid JSON: %s", string(prepared))
	}
}

func TestPreparePayload_EdgeBranches(t *testing.T) {
	srv := &Server{}
	tier := &contract.Tier{
		Model: "short",
	}

	// 1. Replacement with shorter model name leaves original buffer untouched
	inPlaceBody := []byte(`{"model":"very-long-model-name-here","other":"keep"}`)
	outInPlace := srv.preparePayload(tier, nil, inPlaceBody)
	if !strings.Contains(string(outInPlace), `"model":"short"`) {
		t.Errorf("expected replacement: %s", string(outInPlace))
	}
	if !bytes.Equal(inPlaceBody, []byte(`{"model":"very-long-model-name-here","other":"keep"}`)) {
		t.Errorf("preparePayload mutated input buffer!")
	}

	// 2. stream_options already has include_usage: false
	tierStream := &contract.Tier{Model: "m", ReasoningEffort: "low"}
	bodyWithOpts := []byte(`{"model":"old","stream":true,"stream_options":{"include_usage":false}}`)
	outOpts := srv.preparePayload(tierStream, nil, bodyWithOpts)
	if !strings.Contains(string(outOpts), `"include_usage":true`) {
		t.Errorf("expected include_usage replaced with true: %s", string(outOpts))
	}

	// 3. stream_options is null (non-object)
	bodyNullOpts := []byte(`{"model":"old","stream":true,"stream_options":null}`)
	outNullOpts := srv.preparePayload(tierStream, nil, bodyNullOpts)
	if !strings.Contains(string(outNullOpts), `"stream_options":{"include_usage":true}`) {
		t.Errorf("expected null stream_options replaced: %s", string(outNullOpts))
	}

	// 4. Duplicate keys at top level
	bodyDup := []byte(`{"model":"first","other":1,"model":"second"}`)
	outDup := srv.preparePayload(tier, nil, bodyDup)
	if strings.Contains(string(outDup), "first") || strings.Contains(string(outDup), "second") {
		t.Errorf("expected all occurrences of model replaced: %s", string(outDup))
	}

	// 5. Empty object {}
	bodyEmpty := []byte(`{}`)
	outEmpty := srv.preparePayload(tierStream, nil, bodyEmpty)
	if !strings.Contains(string(outEmpty), `"model":"m"`) || !strings.Contains(string(outEmpty), `"reasoning_effort":"low"`) {
		t.Errorf("expected inserted model and reasoning_effort in empty object: %s", string(outEmpty))
	}

	// 6. Malformed JSON inputs return body unchanged
	malformed := [][]byte{
		[]byte(`not json`),
		[]byte(`[1, 2, 3]`),
		[]byte(`{unquoted: 1}`),
		[]byte(`{"key" 1}`),
		[]byte(`{"key": `),
		[]byte(`{"key": invalid}`),
		[]byte(`{"model":}`),
		[]byte(`{"model":"gpt",`),
		[]byte(`{"model":`),
	}
	for _, m := range malformed {
		res := srv.preparePayload(tier, nil, m)
		if !bytes.Equal(res, m) {
			t.Errorf("expected malformed JSON returned unchanged: %s", string(res))
		}
	}

	// 7. Empty or unclosed JSON
	if res := srv.preparePayload(tier, nil, []byte("")); len(res) != 0 {
		t.Errorf("expected empty body returned unchanged")
	}
	if res := srv.preparePayload(tier, nil, []byte("{")); !bytes.Equal(res, []byte("{")) {
		t.Errorf("expected bare { returned unchanged")
	}

	// 8. stream_options with include_usage already true
	bodyTrueOpts := []byte(`{"model":"old","stream":true,"stream_options":{"include_usage":true}}`)
	outTrueOpts := srv.preparePayload(tierStream, nil, bodyTrueOpts)
	if !strings.Contains(string(outTrueOpts), `"include_usage":true`) {
		t.Errorf("expected include_usage kept: %s", string(outTrueOpts))
	}

	// 9. stream_options is empty object {}
	bodyEmptyOpts := []byte(`{"model":"old","stream":true,"stream_options":{}}`)
	outEmptyOpts := srv.preparePayload(tierStream, nil, bodyEmptyOpts)
	if !strings.Contains(string(outEmptyOpts), `"stream_options":{"include_usage":true}`) {
		t.Errorf("expected empty stream_options populated: %s", string(outEmptyOpts))
	}

	// 10. stream_options has custom fields but not include_usage
	bodyCustomOpts := []byte(`{"model":"old","stream":true,"stream_options":{"custom":123}}`)
	outCustomOpts := srv.preparePayload(tierStream, nil, bodyCustomOpts)
	if !strings.Contains(string(outCustomOpts), `"include_usage":true`) || !strings.Contains(string(outCustomOpts), `"custom":123`) {
		t.Errorf("expected include_usage appended to stream_options: %s", string(outCustomOpts))
	}

	// 11. Bare empty object {}
	bodyEmptyObj := []byte(`{}`)
	outEmptyObj := srv.preparePayload(tierStream, nil, bodyEmptyObj)
	if !strings.Contains(string(outEmptyObj), `"model":"m"`) || !strings.Contains(string(outEmptyObj), `"reasoning_effort":"low"`) {
		t.Errorf("expected populated empty object: %s", string(outEmptyObj))
	}

	// 12. Missing model but has other elements
	bodyNoModel := []byte(`{"stream":true,"seed":123}`)
	outNoModel := srv.preparePayload(tierStream, nil, bodyNoModel)
	if !strings.Contains(string(outNoModel), `"model":"m"`) || !strings.Contains(string(outNoModel), `"stream_options":{"include_usage":true}`) {
		t.Errorf("expected model injected: %s", string(outNoModel))
	}
}

func TestPreparePayload_LeavesCallerBodyUnmodified(t *testing.T) {
	srv := &Server{}
	body := []byte(`{"model":"some-very-long-client-model","reasoning_effort":"medium","messages":[{"role":"user","content":"hi"}]}`)
	orig := append([]byte(nil), body...)

	prepared := srv.preparePayload(&contract.Tier{Model: "short", ReasoningEffort: "low"}, nil, body)

	// 1. Output must be valid JSON with the new model
	if !json.Valid(prepared) {
		t.Fatalf("prepared is not valid JSON: %s", string(prepared))
	}
	if !bytes.Contains(prepared, []byte(`"model":"short"`)) {
		t.Errorf("prepared does not contain short model: %s", string(prepared))
	}

	// 2. Caller's original body slice must be 100% byte-identical
	if !bytes.Equal(body, orig) {
		t.Fatalf("caller's body slice was mutated! got: %s, want: %s", string(body), string(orig))
	}
}

func TestIsDefectiveEmptyContent_EdgeBranches(t *testing.T) {
	// not valid JSON
	if isDefectiveEmptyContent([]byte("invalid json")) {
		t.Errorf("expected false on invalid json")
	}
	// invalid choice
	if !isDefectiveEmptyContent([]byte(`{"choices":["not-an-object"]}`)) {
		t.Errorf("expected defective on non-object choice")
	}
	// choice with non-object message
	if !isDefectiveEmptyContent([]byte(`{"choices":[{"message":"not-object"}]}`)) {
		t.Errorf("expected defective on non-object message")
	}
	// choice with invalid message JSON
	if !isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content": 123}}]}`)) {
		t.Errorf("expected defective on invalid message JSON")
	}
	// empty choices array
	if !isDefectiveEmptyContent([]byte(`{"choices":[]}`)) {
		t.Errorf("expected defective on empty choices")
	}
	// choice with reasoning only
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"","reasoning":"pondering"}}]}`)) {
		t.Errorf("expected non-defective on reasoning content")
	}
	// choice with tool calls only
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1"}]}}]}`)) {
		t.Errorf("expected non-defective on tool calls")
	}
	// choice with reasoning_content
	if isDefectiveEmptyContent([]byte(`{"choices":[{"message":{"content":"","reasoning_content":"deep pondering"}}]}`)) {
		t.Errorf("expected non-defective on reasoning_content")
	}
	// choice with primitive message
	if !isDefectiveEmptyContent([]byte(`{"choices":[{"message":123}]}`)) {
		t.Errorf("expected defective on primitive message")
	}
}

func TestPurgeAllLogs_FallbackPaths(t *testing.T) {
	srv := &Server{
		directivePath:  t.TempDir() + "/directive.json",
		trafficLogPath: t.TempDir() + "/traffic.jsonl",
		exitFunc:       func(int) {},
	}
	w := httptest.NewRecorder()
	srv.handlePurgeAllLogsDirective(w, DirectiveRequest{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	srv2 := &Server{
		exitFunc: func(int) {},
	}
	w2 := httptest.NewRecorder()
	srv2.handlePurgeAllLogsDirective(w2, DirectiveRequest{})
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	ds, _ := store.NewDiskStore(filepath.Join(t.TempDir(), "stats.json"))
	srv3 := &Server{
		diskStore: ds,
		exitFunc:  func(int) {},
	}
	w3 := httptest.NewRecorder()
	srv3.handlePurgeAllLogsDirective(w3, DirectiveRequest{})
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w3.Code)
	}

	blockingFile := filepath.Join(t.TempDir(), "block")
	_ = os.WriteFile(blockingFile, []byte("x"), 0600)
	srvErr := &Server{
		directivePath: filepath.Join(blockingFile, "sub", "directive.json"),
		exitFunc:      func(int) {},
	}
	wErr := httptest.NewRecorder()
	srvErr.handlePurgeAllLogsDirective(wErr, DirectiveRequest{})
	if wErr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", wErr.Code)
	}
}

func BenchmarkPreparePayload_LargeFixture(b *testing.B) {
	srv := &Server{}
	tier := &contract.Tier{
		Model:           "claude-3-5-sonnet",
		ReasoningEffort: "medium",
	}

	// Create a large 600 KB payload with tools and many messages
	var buf bytes.Buffer
	buf.WriteString(`{"tools":[{"name":"bash"},{"name":"editor"}],"model":"old-model","stream":true,"messages":[`)
	for i := 0; i < 200; i++ {
		if i > 0 {
			buf.WriteString(`,`)
		}
		buf.WriteString(`{"role":"user","content":"message content line `)
		buf.WriteString(strings.Repeat("lots of tokens and text in history ", 50))
		buf.WriteString(`"}`)
	}
	buf.WriteString(`]}`)
	payload := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = srv.preparePayload(tier, nil, payload)
	}
}

func TestDumpOutbound_Disabled(t *testing.T) {
	t.Setenv("NACHO_DUMP_OUTBOUND", "")
	req, err := http.NewRequest(http.MethodPost, "http://localhost/test", bytes.NewReader([]byte(`{"hello":"world"}`)))
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	logger := slog.Default()
	dumpOutbound(req, logger)
	// Nothing panicked or errored
}

func TestDumpOutbound_NilOrNoGetBody(t *testing.T) {
	t.Setenv("NACHO_DUMP_OUTBOUND", t.TempDir())
	logger := slog.Default()
	dumpOutbound(nil, logger)

	req := &http.Request{}
	dumpOutbound(req, logger)
}

func TestDumpOutbound_Enabled(t *testing.T) {
	dumpDir := t.TempDir()
	t.Setenv("NACHO_DUMP_OUTBOUND", dumpDir)

	payload := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)
	req, err := http.NewRequest(http.MethodPost, "http://localhost/test", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	logger := slog.Default()
	dumpOutbound(req, logger)

	files, err := os.ReadDir(dumpDir)
	if err != nil {
		t.Fatalf("failed to read dump dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 dump file, got %d", len(files))
	}

	dumped, err := os.ReadFile(filepath.Join(dumpDir, files[0].Name()))
	if err != nil {
		t.Fatalf("failed to read dumped file: %v", err)
	}
	if !bytes.Equal(dumped, payload) {
		t.Errorf("dumped payload mismatch: got %s, want %s", string(dumped), string(payload))
	}

	info, err := files[0].Info()
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}
	// Verify file mode includes 0600 permissions
	if info.Mode().Perm()&0600 != 0600 {
		t.Errorf("expected 0600 perm, got %v", info.Mode().Perm())
	}
}

func TestDumpOutbound_UnwritableDirectory(t *testing.T) {
	tempDir := t.TempDir()
	// Create a regular file where the directory would be, causing MkdirAll to fail
	blockingFile := filepath.Join(tempDir, "blocked")
	if err := os.WriteFile(blockingFile, []byte("blocker"), 0600); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}

	invalidDir := filepath.Join(blockingFile, "sub")
	t.Setenv("NACHO_DUMP_OUTBOUND", invalidDir)

	payload := []byte(`{"test":123}`)
	req, err := http.NewRequest(http.MethodPost, "http://localhost/test", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}
	logger := slog.Default()
	// Must not panic, request continues
	dumpOutbound(req, logger)
}

func TestDumpOutbound_Errors(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("NACHO_DUMP_OUTBOUND", tempDir)
	logger := slog.Default()

	// 1. GetBody error
	reqGetBodyErr := &http.Request{
		GetBody: func() (io.ReadCloser, error) {
			return nil, errors.New("get body error")
		},
	}
	dumpOutbound(reqGetBodyErr, logger)

	// 2. Read error
	reqReadErr := &http.Request{
		GetBody: func() (io.ReadCloser, error) {
			return io.NopCloser(iotest.ErrReader(errors.New("read error"))), nil
		},
	}
	dumpOutbound(reqReadErr, logger)
}

func TestInjectCorrectionPrompt_EdgeCases(t *testing.T) {
	// 1. Missing messages
	if res := injectCorrectionPrompt([]byte(`{}`), "prompt"); !bytes.Equal(res, []byte(`{}`)) {
		t.Errorf("expected unchanged: %s", string(res))
	}
	// 2. Messages is not an array
	if res := injectCorrectionPrompt([]byte(`{"messages":"not an array"}`), "prompt"); !bytes.Equal(res, []byte(`{"messages":"not an array"}`)) {
		t.Errorf("expected unchanged: %s", string(res))
	}
	// 3. Messages is unclosed
	if res := injectCorrectionPrompt([]byte(`{"messages":[`), "prompt"); !bytes.Equal(res, []byte(`{"messages":[`)) {
		t.Errorf("expected unchanged: %s", string(res))
	}
	// 4. Empty messages array
	resEmpty := injectCorrectionPrompt([]byte(`{"messages":[]}`), "custom")
	if !bytes.Contains(resEmpty, []byte(`"role":"user","content":"custom"`)) {
		t.Errorf("expected custom prompt in empty array: %s", string(resEmpty))
	}
	// 5. Default prompt
	resDef := injectCorrectionPrompt([]byte(`{"messages":[]}`), "")
	if !bytes.Contains(resDef, []byte(contract.CycleBreakerDefaultCorrectionPrompt)) {
		t.Errorf("expected default prompt: %s", string(resDef))
	}
}

func TestReplayCapturedFixtures(t *testing.T) {
	scratchDir := `C:\Users\karlk\.gemini\antigravity-ide\brain\44c3d9a2-2eff-4331-ab35-baa30a415a0d\scratch`
	fixtures := []struct {
		file     string
		expected int
	}{
		{"req_final.json", 6},
		{"req_new.json", 3},
		{"req_run3.json", 5},
	}

	if _, err := os.Stat(scratchDir); os.IsNotExist(err) {
		t.Skip("scratch directory does not exist on this environment")
	}

	bTrue := true
	bFalse := false
	i3 := 3
	tr := BuildNTSTransformer(contract.NTSConfig{
		Enabled:                  &bTrue,
		StripANSI:                &bTrue,
		ResolveCR:                &bTrue,
		DeduplicateLines:         &bFalse,
		DedupThreshold:           3,
		StripBoilerplate:         &bFalse,
		NormalizeWhitespace:      &bTrue,
		PreserveFileReads:        &bTrue,
		PreserveFileWrites:       &bTrue,
		PreserveCacheControl:     &bTrue,
		CompactStaleFileReads:    &bFalse,
		StaleReadDepth:           3,
		StripHistoricalReasoning: &bTrue,
		KeepRecentReasoningTurns: &i3,
	})

	srv := &Server{}
	tier := &contract.Tier{
		Model:           "claude-3-7-sonnet-20250219",
		ReasoningEffort: "high",
	}

	for _, f := range fixtures {
		fullPath := filepath.Join(scratchDir, f.file)
		raw, err := os.ReadFile(fullPath)
		if err != nil {
			t.Skipf("fixture %s not found: %v", f.file, err)
			continue
		}

		in := append([]byte(nil), raw...)

		// 1. Replay through NTS
		outNTS, resNTS, err := tr.TransformOpenAI(in)
		if err != nil {
			t.Fatalf("[%s] NTS transform error: %v", f.file, err)
		}

		actualDiff := len(raw) - len(outNTS)
		if resNTS.BytesSaved != actualDiff {
			t.Errorf("[%s] BytesSaved mismatch: reported %d, actual diff %d", f.file, resNTS.BytesSaved, actualDiff)
		}

		if bytes.Count(outNTS, []byte(`\u003c`)) > bytes.Count(raw, []byte(`\u003c`)) {
			t.Errorf("[%s] NTS introduced \\u003c", f.file)
		}

		// 2. Replay through preparePayload
		outPrep := srv.preparePayload(tier, nil, outNTS)
		if bytes.Count(outPrep, []byte(`\u003c`)) > bytes.Count(raw, []byte(`\u003c`)) {
			t.Errorf("[%s] preparePayload introduced \\u003c", f.file)
		}

		// 3. Verify valid JSON
		if !json.Valid(outPrep) {
			t.Errorf("[%s] output is invalid JSON", f.file)
		}
	}
}

func TestServer_Dispatch_FallbackPreservesPristineBody(t *testing.T) {
	var fallbackBody []byte
	tsFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsFail.Close()

	tsFallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fallbackBody = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer tsFallback.Close()

	pFail := &mockSimpleLLMProvider{
		id:      "prov_fail",
		name:    "Prov Fail",
		baseURL: tsFail.URL,
	}
	pFallback := &mockSimpleLLMProvider{
		id:      "prov_fallback",
		name:    "Prov Fallback",
		baseURL: tsFallback.URL,
	}

	reg := provider.NewRegistry()
	reg.Register(pFail)
	reg.Register(pFallback)

	cfg := &contract.Config{
		Port: 8000,
		DefaultTier: contract.Tier{
			Name:     "default",
			Provider: "prov_fallback",
			Model:    "fallback-model",
		},
		Tiers: []contract.Tier{
			{Name: "default", Provider: "prov_fallback", Model: "fallback-model"},
			// Primary only shortens the model and inserts no keys. Before the fix this was
			// the in-place path that corrupted the body reused by the fallback.
			{Name: "primary", Provider: "prov_fail", Model: "short"},
		},
	}
	evaluator, _ := strategy.NewExprEvaluator(nil, contract.Tier{Model: "default"})
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, router.NewClassifier(), router.NewSanitizer(), nil, nil, reg, slog.Default())

	clientBody := []byte(`{"model":"very-long-client-model-name-12345","messages":[{"role":"user","content":"test"}]}`)
	origClientBody := append([]byte(nil), clientBody...)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(clientBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.dispatchTier(w, req, contract.RequestContext{Tokens: 10}, cfg.Tiers[1], clientBody, time.Now(), slog.Default(), false)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Equal(clientBody, origClientBody) {
		t.Fatalf("primary tier mutated the client body: %s", string(clientBody))
	}
	want := `{"model":"fallback-model","messages":[{"role":"user","content":"test"}]}`
	if string(fallbackBody) != want {
		t.Fatalf("fallback payload mismatch:\n got: %s\nwant: %s", string(fallbackBody), want)
	}
}
