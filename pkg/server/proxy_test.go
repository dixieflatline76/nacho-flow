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
		t.Errorf("Expected positive USD cost savings calculated from oracle, got %f", stats.EstimatedCostSavedUSD)
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
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqPayload))
		srv.ServeHTTP(rec, req)
	}
}
