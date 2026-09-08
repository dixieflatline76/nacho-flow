package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
)

func TestProxy_StreamOptions_Injection(t *testing.T) {
	var capturedPayloads []map[string]interface{}

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var p map[string]interface{}
		_ = json.Unmarshal(bodyBytes, &p)
		capturedPayloads = append(capturedPayloads, p)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock": {
				BaseURL: mockUpstream.URL,
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Model:    "test-model",
				Provider: "mock",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	// 1. Streaming request without stream_options -> inject stream_options.include_usage = true
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "test-model",
		"stream": true,
		"messages": [{"role": "user", "content": "hi"}]
	}`))
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec1.Code)
	}

	if len(capturedPayloads) < 1 {
		t.Fatal("Expected at least 1 captured payload")
	}
	opts1, ok := capturedPayloads[0]["stream_options"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected stream_options map in payload, got %#v", capturedPayloads[0]["stream_options"])
	}
	if inc, _ := opts1["include_usage"].(bool); !inc {
		t.Errorf("Expected stream_options.include_usage == true, got %#v", opts1["include_usage"])
	}

	// 2. Streaming request with existing stream_options -> preserve other keys and set include_usage = true
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "test-model",
		"stream": true,
		"stream_options": {"custom_flag": true},
		"messages": [{"role": "user", "content": "hi again"}]
	}`))
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec2.Code)
	}

	opts2, ok := capturedPayloads[1]["stream_options"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected stream_options map in payload, got %#v", capturedPayloads[1]["stream_options"])
	}
	if inc, _ := opts2["include_usage"].(bool); !inc {
		t.Errorf("Expected stream_options.include_usage == true, got %#v", opts2["include_usage"])
	}
	if custom, _ := opts2["custom_flag"].(bool); !custom {
		t.Errorf("Expected stream_options.custom_flag == true preserved, got %#v", opts2["custom_flag"])
	}

	// 3. Non-streaming request -> do not inject stream_options
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "test-model",
		"stream": false,
		"messages": [{"role": "user", "content": "non-streaming"}]
	}`))
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)

	if _, exists := capturedPayloads[2]["stream_options"]; exists {
		t.Errorf("Expected stream_options NOT to be injected on non-streaming request, got %#v", capturedPayloads[2]["stream_options"])
	}
}

func TestProxy_Streaming_TokenCalibrationAndZeroUsageWarn(t *testing.T) {
	var requestCount int32

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		if count == 1 {
			// Request 1: Stream with final usage chunk
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
			w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1200,\"completion_tokens\":50,\"total_tokens\":1250}}\n\n"))
			w.Write([]byte("data: [DONE]\n\n"))
		} else {
			// Request 2: Stream with 0 total tokens (missing usage)
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"no usage\"}}]}\n\n"))
			w.Write([]byte("data: [DONE]\n\n"))
		}
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"stream_provider": {
				BaseURL: mockUpstream.URL,
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Stream Tier",
				Model:    "stream-model-v1",
				Provider: "stream_provider",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()

	// Capture logs to verify warning
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	srv := NewServer(cfg, evaluator, classifier, sanitizer)
	srv.logger = logger

	rcClassifier, ok := classifier.(*router.RequestClassifier)
	if !ok {
		t.Fatal("Expected *router.RequestClassifier")
	}
	initialRatio := rcClassifier.GetEstimator().GetRatio()

	// Request 1: should calibrate estimator using usage.PromptTokens (1200)
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "stream-model-v1",
		"stream": true,
		"messages": [{"role": "user", "content": "calibrate me"}]
	}`))
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec1.Code)
	}

	newRatio := rcClassifier.GetEstimator().GetRatio()
	if newRatio == initialRatio {
		t.Errorf("Expected estimator ratio to change after calibration from streaming usage, was %v", newRatio)
	}

	// Request 2: missing usage on 200 stream -> should log WARN
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "stream-model-v1",
		"stream": true,
		"messages": [{"role": "user", "content": "zero usage"}]
	}`))
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec2.Code)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "zero usage") && !strings.Contains(logOutput, "stream_provider") {
		t.Errorf("Expected WARN log about zero usage tokens for provider, got: %s", logOutput)
	}

	// Request 3: second stream with zero usage from the same provider -> should NOT log again
	logBuf.Reset()
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "stream-model-v1",
		"stream": true,
		"messages": [{"role": "user", "content": "zero usage again"}]
	}`))
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec3.Code)
	}
	if strings.Contains(logBuf.String(), "zero usage") {
		t.Errorf("Expected warning NOT to be logged again for the same provider, got: %s", logBuf.String())
	}
}
