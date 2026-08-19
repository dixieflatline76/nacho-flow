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

func TestProxyServerRouting(t *testing.T) {
	// Mock Upstream OpenRouter Server
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)

		model, _ := payload["model"].(string)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		respJSON := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   model,
			"choices": []map[string]interface{}{{"message": map[string]string{"role": "assistant", "content": "Mock Response"}}},
		}
		json.NewEncoder(w).Encode(respJSON)
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]string{
			"mock": mockUpstream.URL,
		},
		Tiers: []contract.Tier{
			{
				Name:     "Mock Tier",
				Model:    "mock-model-v1",
				Provider: "mock",
				When:     "Tokens < 1000",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Model:    "default-model",
			Provider: "mock",
		},
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}

	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	// Test 1: Health check
	recHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest("GET", "/health", nil)
	srv.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Errorf("Expected health status 200, got %d", recHealth.Code)
	}

	// Test 2: Chat completion routing and model rewriting
	reqBody := `{"model": "original-model", "messages": [{"role": "user", "content": "Hello world"}]}`
	recChat := httptest.NewRecorder()
	reqChat := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))
	srv.ServeHTTP(recChat, reqChat)

	if recChat.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", recChat.Code, recChat.Body.String())
	}

	var respPayload map[string]interface{}
	json.Unmarshal(recChat.Body.Bytes(), &respPayload)
	if respPayload["model"] != "mock-model-v1" {
		t.Errorf("Expected rewritten model 'mock-model-v1', got %v", respPayload["model"])
	}
}
