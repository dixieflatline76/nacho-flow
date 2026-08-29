package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
)

func TestProxy_CycleBreaker_Stage1_LocalRetry(t *testing.T) {
	var localAttempts int32
	var capturedRetryMessages []map[string]interface{}

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&localAttempts, 1)

		if attempt == 1 {
			// First attempt: stream degenerate repetitive monologue
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			for i := 0; i < 6; i++ {
				w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Let us check the directory structure now. \"}}]}\n\n"))
			}
			w.Write([]byte("data: [DONE]\n\n"))
			return
		}

		// Second attempt (Stage 1 local retry): Verify [SYSTEM OVERRIDE] was injected
		bodyBytes, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(bodyBytes, &payload)
		if msgs, ok := payload["messages"].([]interface{}); ok {
			for _, m := range msgs {
				if msgMap, ok := m.(map[string]interface{}); ok {
					capturedRetryMessages = append(capturedRetryMessages, msgMap)
				}
			}
		}

		// Return clean tool call
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"write_to_file\",\"arguments\":\"{}\"}}]}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockLocal.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleBreaker: contract.CycleBreakerConfig{
			Enabled:             &enabled,
			MaxProseTokens:      800,
			RepetitionWindow:    5,
			RepetitionThreshold: 3,
			MaxRetries:          1,
		},
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {
				BaseURL: mockLocal.URL,
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local GPU",
				Model:    "gemma4:12b",
				Provider: "local_gpu",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"stream": true,
		"tools": [{"type": "function", "function": {"name": "write_to_file"}}],
		"messages": [{"role": "user", "content": "Implement the N-Queens solution"}]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	if atomic.LoadInt32(&localAttempts) != 2 {
		t.Fatalf("Expected 2 local attempts (initial + Stage 1 retry), got %d", atomic.LoadInt32(&localAttempts))
	}

	// Verify [SYSTEM OVERRIDE] was appended to messages in the retry
	var foundOverride bool
	for _, m := range capturedRetryMessages {
		if content, ok := m["content"].(string); ok && strings.Contains(content, "[SYSTEM OVERRIDE]") {
			foundOverride = true
			break
		}
	}
	if !foundOverride {
		t.Fatalf("Expected [SYSTEM OVERRIDE] prompt in retry messages, got: %+v", capturedRetryMessages)
	}

	if !strings.Contains(rec.Body.String(), "write_to_file") {
		t.Fatalf("Expected tool call write_to_file in client response, got: %s", rec.Body.String())
	}
}

func TestProxy_CycleBreaker_Stage2_CloudFailover(t *testing.T) {
	var localAttempts int32
	var cloudAttempts int32

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&localAttempts, 1)
		// Always emit repetitive monologue
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 6; i++ {
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Let us check the directory structure now. \"}}]}\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockLocal.Close()

	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cloudAttempts, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_cloud\",\"type\":\"function\",\"function\":{\"name\":\"write_to_file\",\"arguments\":\"{}\"}}]}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockCloud.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleBreaker: contract.CycleBreakerConfig{
			Enabled:             &enabled,
			MaxProseTokens:      800,
			RepetitionWindow:    5,
			RepetitionThreshold: 3,
			MaxRetries:          1,
		},
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {
				BaseURL: mockLocal.URL,
				Type:    "local",
			},
			"cloud_provider": {
				BaseURL: mockCloud.URL,
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local GPU",
				Model:    "gemma4:12b",
				Provider: "local_gpu",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 2: Cloud Fallback",
			Model:    "gemini-flash",
			Provider: "cloud_provider",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"stream": true,
		"tools": [{"type": "function", "function": {"name": "write_to_file"}}],
		"messages": [{"role": "user", "content": "Implement the N-Queens solution"}]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	if atomic.LoadInt32(&localAttempts) != 2 {
		t.Fatalf("Expected 2 local attempts (initial + Stage 1 retry), got %d", atomic.LoadInt32(&localAttempts))
	}

	if atomic.LoadInt32(&cloudAttempts) != 1 {
		t.Fatalf("Expected 1 cloud attempt after Stage 1 retry failed, got %d", atomic.LoadInt32(&cloudAttempts))
	}

	if !strings.Contains(rec.Body.String(), "call_cloud") {
		t.Fatalf("Expected cloud response in client body, got: %s", rec.Body.String())
	}
}

func TestProxy_CycleBreaker_NonStreaming_LocalRetry(t *testing.T) {
	var localAttempts int32

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&localAttempts, 1)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if attempt == 1 {
			// First attempt: return long repetitive monologue in JSON
			w.Write([]byte(`{
				"id": "cmpl-1",
				"choices": [{
					"message": {
						"role": "assistant",
						"content": "Let us check the directory structure now. Let us check the directory structure now. Let us check the directory structure now. Let us check the directory structure now."
					},
					"finish_reason": "stop"
				}]
			}`))
			return
		}

		// Second attempt: return valid tool call
		w.Write([]byte(`{
			"id": "cmpl-2",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "",
					"tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "write_to_file", "arguments": "{}"}}]
				},
				"finish_reason": "tool_calls"
			}]
		}`))
	}))
	defer mockLocal.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleBreaker: contract.CycleBreakerConfig{
			Enabled:             &enabled,
			MaxProseTokens:      800,
			RepetitionWindow:    5,
			RepetitionThreshold: 3,
			MaxRetries:          1,
		},
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {
				BaseURL: mockLocal.URL,
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local GPU",
				Model:    "gemma4:12b",
				Provider: "local_gpu",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"tools": [{"type": "function", "function": {"name": "write_to_file"}}],
		"messages": [{"role": "user", "content": "Implement the N-Queens solution"}]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	if atomic.LoadInt32(&localAttempts) != 2 {
		t.Fatalf("Expected 2 local attempts, got %d", atomic.LoadInt32(&localAttempts))
	}
}

func TestProxy_CycleBreaker_TierOverride_Disabled(t *testing.T) {
	var localAttempts int32

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&localAttempts, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "cmpl-1",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Let us check the directory structure now. Let us check the directory structure now. Let us check the directory structure now. Let us check the directory structure now."
				},
				"finish_reason": "stop"
			}]
		}`))
	}))
	defer mockLocal.Close()

	disabled := false
	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {
				BaseURL: mockLocal.URL,
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local GPU",
				Model:    "gemma4:12b",
				Provider: "local_gpu",
				When:     "true",
				CycleBreaker: &contract.CycleBreakerConfig{
					Enabled: &disabled,
				},
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"tools": [{"type": "function", "function": {"name": "write_to_file"}}],
		"messages": [{"role": "user", "content": "Implement the N-Queens solution"}]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	// Should not retry because cycle_breaker was explicitly disabled on tier
	if atomic.LoadInt32(&localAttempts) != 1 {
		t.Fatalf("Expected 1 local attempt with disabled cycle breaker, got %d", atomic.LoadInt32(&localAttempts))
	}
}

func TestProxy_CycleBreaker_Phase2_MidStreamSevering(t *testing.T) {
	var closedGracefully int32

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)

		// 1. Stream 3KB of distinct clean text to pass the 2KB peek phase cleanly
		for i := 0; i < 40; i++ {
			chunk := "data: {\"choices\":[{\"delta\":{\"content\":\"Architecture plan section step number " + strings.Repeat("a", 70) + " complete.\n\"}}]}\n\n"
			w.Write([]byte(chunk))
			if ok {
				flusher.Flush()
			}
		}

		// 2. Begin repeating degenerate loop (exceeds repetition threshold)
		for i := 0; i < 15; i++ {
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Let us check the directory structure now. \"}}]}\n\n"))
			if ok {
				flusher.Flush()
			}
		}

		w.Write([]byte("data: [DONE]\n\n"))
		atomic.StoreInt32(&closedGracefully, 1)
	}))
	defer mockLocal.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleBreaker: contract.CycleBreakerConfig{
			Enabled:             &enabled,
			MaxProseTokens:      800,
			RepetitionWindow:    5,
			RepetitionThreshold: 3,
			MaxRetries:          1,
		},
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {
				BaseURL: mockLocal.URL,
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local GPU",
				Model:    "gemma4:12b",
				Provider: "local_gpu",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"stream": true,
		"tools": [{"type": "function", "function": {"name": "write_to_file"}}],
		"messages": [{"role": "user", "content": "Implement the N-Queens solution"}]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	// Must contain clean initial architecture chunks
	if !strings.Contains(body, "Architecture plan section step number") {
		t.Errorf("Expected body to contain initial streamed chunks, got: %s", body)
	}

	// Must finish cleanly with [DONE]
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("Expected body to terminate with [DONE], got: %s", body)
	}

	// Must have severed upstream and emitted finish_reason: "stop"
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Errorf("Expected body to contain finish_reason stop chunk, got: %s", body)
	}
}

func TestProxy_CycleBreaker_ThinkingRunawaySevering(t *testing.T) {
	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)

		// 1. Initial chunk with thinking tag
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"<think>\\n\"}}]}\n\n"))
		if ok {
			flusher.Flush()
		}

		// 2. Stream repetitive thinking loop (exceeds thinking repetition threshold)
		for i := 0; i < 15; i++ {
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Wait let me check the types again right now. \"}}]}\n\n"))
			if ok {
				flusher.Flush()
			}
		}

		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockLocal.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleBreaker: contract.CycleBreakerConfig{
			Enabled:                     &enabled,
			MaxThinkingTokens:           5000,
			RepetitionWindow:            5,
			ThinkingRepetitionThreshold: 4,
			MaxRetries:                  0, // Phase 2 direct sever
		},
		Providers: map[string]contract.ProviderConfig{
			"local_gpu": {
				BaseURL: mockLocal.URL,
				Type:    "local",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local GPU",
				Model:    "gemma4:12b",
				Provider: "local_gpu",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"stream": true,
		"tools": [{"type": "function", "function": {"name": "write_to_file"}}],
		"messages": [{"role": "user", "content": "Implement the N-Queens solution"}]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	// Must terminate cleanly with [DONE]
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("Expected body to terminate with [DONE], got: %s", body)
	}

	// Must have severed upstream and emitted finish_reason: "stop"
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Errorf("Expected body to contain finish_reason stop chunk, got: %s", body)
	}
}



