package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
)

func TestProxy_OpenRouter_EndToEnd(t *testing.T) {
	var mu sync.Mutex
	var capturedPayloads []map[string]interface{}
	var capturedHeaders []http.Header

	mockOpenRouter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		bodyBytes, _ := io.ReadAll(r.Body)
		var p map[string]interface{}
		_ = json.Unmarshal(bodyBytes, &p)
		capturedPayloads = append(capturedPayloads, p)
		capturedHeaders = append(capturedHeaders, r.Header.Clone())

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-test","choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer mockOpenRouter.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {
				BaseURL: mockOpenRouter.URL,
				Type:    contract.ProviderTypeOpenRouter,
				APIKey:  "sk-or-test-key",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:            "ReasoningTier",
				Model:           "glm-5.3-flash",
				Provider:        "openrouter",
				ReasoningEffort: "high",
				When:            `Prompt contains "think"`,
			},
			{
				Name:     "PassthroughTier",
				Model:    "glm-5.3-flash",
				Provider: "openrouter",
				When:     `true`,
			},
		},
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	if err != nil {
		t.Fatalf("Failed to create evaluator: %v", err)
	}
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	// 1. Request to ReasoningTier (reasoning_effort: "high")
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "reasoning-model",
		"reasoning_effort": "low",
		"messages": [{"role": "user", "content": "think carefully"}]
	}`))
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d (body: %s)", rec1.Code, rec1.Body.String())
	}

	mu.Lock()
	if len(capturedPayloads) < 1 {
		t.Fatalf("Expected at least 1 captured payload, got 0")
	}
	p1 := capturedPayloads[0]
	h1 := capturedHeaders[0]
	mu.Unlock()

	// Verify OpenRouter default headers
	if h1.Get("HTTP-Referer") == "" {
		t.Errorf("Expected HTTP-Referer header on OpenRouter upstream request")
	}
	if h1.Get("X-Title") == "" {
		t.Errorf("Expected X-Title header on OpenRouter upstream request")
	}

	// Verify reasoning translated to nested OpenRouter format and flat reasoning_effort stripped
	if _, exists := p1["reasoning_effort"]; exists {
		t.Errorf("Expected flat reasoning_effort to be stripped, got: %v", p1["reasoning_effort"])
	}
	reasoningObj, ok := p1["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'reasoning' object, got %T: %v", p1["reasoning"], p1["reasoning"])
	}
	if reasoningObj["effort"] != "high" {
		t.Errorf("Expected reasoning.effort == 'high', got %v", reasoningObj["effort"])
	}

	// 2. Request to PassthroughTier (ReasoningEffort omitted in tier)
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "passthrough-model",
		"messages": [{"role": "user", "content": "fast answer"}]
	}`))
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d (body: %s)", rec2.Code, rec2.Body.String())
	}

	mu.Lock()
	if len(capturedPayloads) < 2 {
		t.Fatalf("Expected 2 captured payloads, got %d", len(capturedPayloads))
	}
	p2 := capturedPayloads[1]
	mu.Unlock()

	// Verify Single Ground Truth: nothing injected when tier omits reasoning_effort
	if _, exists := p2["reasoning"]; exists {
		t.Errorf("Expected NO reasoning object when tier does not configure reasoning_effort, got: %v", p2["reasoning"])
	}
	if _, exists := p2["reasoning_effort"]; exists {
		t.Errorf("Expected NO reasoning_effort when tier does not configure reasoning_effort, got: %v", p2["reasoning_effort"])
	}
}
