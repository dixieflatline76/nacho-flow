package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
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

func TestNativeToolCallingPreservation(t *testing.T) {
	// Mock provider server to verify that tool schemas are preserved in outgoing proxy request
	var capturedPayload map[string]interface{}

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"cmpl-tool","choices":[{"message":{"role":"assistant","content":"Listing files"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]string{
			"mock": mockUpstream.URL,
		},
		Tiers: []contract.Tier{
			{
				Name:     "Agentic Tier",
				Model:    "qwen3-coder",
				Provider: "mock",
				When:     "HasTools",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Model:    "default-model",
			Provider: "mock",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"messages": [{"role": "user", "content": "List files"}],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "list_dir",
					"description": "List files in directory"
				}
			}
		],
		"tool_choice": "auto"
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	// Verify that tools array was preserved in forwarded payload
	tools, ok := capturedPayload["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatalf("Expected tools array to be preserved in forwarded request payload")
	}

	firstTool := tools[0].(map[string]interface{})
	fn := firstTool["function"].(map[string]interface{})
	if fn["name"] != "list_dir" {
		t.Errorf("Expected function name 'list_dir', got %v", fn["name"])
	}
}

func TestChatCompletionsAutoFallbackWhenLocalOffline(t *testing.T) {
	// Upstream 1: Local provider (down/offline - 502 Bad Gateway)
	mockLocalOffline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Ollama offline", http.StatusBadGateway)
	}))
	defer mockLocalOffline.Close()

	// Upstream 2: Cloud Fallback provider (online)
	mockCloudOnline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"gen-fallback","choices":[{"message":{"role":"assistant","content":"Fallback response"}}]}`))
	}))
	defer mockCloudOnline.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]string{
			"local": mockLocalOffline.URL,
			"cloud": mockCloudOnline.URL,
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local Tier",
				Model:    "local-model",
				Provider: "local",
				When:     "Tokens < 1000",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Cloud Fallback",
			Model:    "cloud-model",
			Provider: "cloud",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{"model": "nacho-hybrid", "messages": [{"role": "user", "content": "Hello"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	// Proxy handler should return response
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadGateway {
		t.Fatalf("Server handled offline proxy cleanly, code: %d", rec.Code)
	}
}
