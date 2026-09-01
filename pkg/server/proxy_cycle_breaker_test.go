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

func TestProxy_Kickstart_PromptInjection(t *testing.T) {
	var capturedBodies []string

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBodies = append(capturedBodies, string(bodyBytes))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Thinking about doing something..."}}]}`))
	}))
	defer mockLocal.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleKiller: contract.CycleBreakerConfig{
			Enabled:            &enabled,
			KickstartThreshold: 3,
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

	sessionID := "test-kickstart-sess-1"

	// Turn 1: No tool progress
	reqPayload1 := `{
		"model": "nacho-hybrid",
		"messages": [{"role": "user", "content": "Let us plan turn 1"}]
	}`
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload1))
	req1.Header.Set("X-Session-ID", sessionID)
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("Turn 1 failed: %d", rec1.Code)
	}

	// Turn 2: No tool progress
	reqPayload2 := `{
		"model": "nacho-hybrid",
		"messages": [{"role": "user", "content": "Let us plan turn 2"}]
	}`
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload2))
	req2.Header.Set("X-Session-ID", sessionID)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Turn 2 failed: %d", rec2.Code)
	}

	// At turn 1 and turn 2, kickstart threshold (3) was not reached yet
	if strings.Contains(capturedBodies[0], "[SYSTEM OVERRIDE]") {
		t.Errorf("Turn 1 should NOT have injected override")
	}
	if strings.Contains(capturedBodies[1], "[SYSTEM OVERRIDE]") {
		t.Errorf("Turn 2 should NOT have injected override")
	}

	// Turn 3: 3rd turn without progress -> Kickstart threshold 3 reached!
	reqPayload3 := `{
		"model": "nacho-hybrid",
		"messages": [{"role": "user", "content": "Let us plan turn 3"}]
	}`
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload3))
	req3.Header.Set("X-Session-ID", sessionID)
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("Turn 3 failed: %d", rec3.Code)
	}

	// Turn 3 body sent to upstream MUST contain the [SYSTEM OVERRIDE] prompt
	if !strings.Contains(capturedBodies[2], "[SYSTEM OVERRIDE]") {
		t.Errorf("Turn 3 expected injected [SYSTEM OVERRIDE], got: %s", capturedBodies[2])
	}

	// Turn 4: Tool progress occurs in conversation history
	reqPayload4 := `{
		"model": "nacho-hybrid",
		"messages": [
			{"role": "user", "content": "Let us plan turn 4"},
			{"role": "assistant", "content": "Let me read the file"},
			{"role": "tool", "content": "File contents successfully loaded"}
		]
	}`
	req4 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload4))
	req4.Header.Set("X-Session-ID", sessionID)
	rec4 := httptest.NewRecorder()
	srv.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("Turn 4 failed: %d", rec4.Code)
	}

	// Turn 4 should reset kickstart counter and NOT inject [SYSTEM OVERRIDE]
	if strings.Contains(capturedBodies[3], "[SYSTEM OVERRIDE]") {
		t.Errorf("Turn 4 with tool progress should NOT have injected override, got: %s", capturedBodies[3])
	}
}

func TestProxy_Kickstart_WriteOnly_PromptInjection(t *testing.T) {
	var capturedBodies []string

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBodies = append(capturedBodies, string(bodyBytes))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Thinking about doing something..."}}]}`))
	}))
	defer mockLocal.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleKiller: contract.CycleBreakerConfig{
			Enabled:            &enabled,
			KickstartThreshold: 2,
			KickstartWriteOnly: true,
			KickstartWriteTools: []string{"write_to_file", "replace_in_file", "execute_command"},
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

	sessionID := "test-kickstart-writeonly-sess"

	// Turn 1: Read-only tool execution (read_file) -> HasToolProgress=true, HasWriteProgress=false
	reqPayload1 := `{
		"model": "nacho-hybrid",
		"messages": [
			{"role": "user", "content": "Read file"},
			{"role": "assistant", "tool_calls": [{"id": "c1", "type": "function", "function": {"name": "read_file"}}]},
			{"role": "tool", "tool_call_id": "c1", "content": "contents of file"}
		]
	}`
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload1))
	req1.Header.Set("X-Session-ID", sessionID)
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("Turn 1 failed: %d", rec1.Code)
	}

	// Turn 2: Another read-only tool execution (list_dir) -> 2nd turn without write progress -> Kickstart triggers!
	reqPayload2 := `{
		"model": "nacho-hybrid",
		"messages": [
			{"role": "user", "content": "List files"},
			{"role": "assistant", "tool_calls": [{"id": "c2", "type": "function", "function": {"name": "list_dir"}}]},
			{"role": "tool", "tool_call_id": "c2", "content": "main.go, config.go"}
		]
	}`
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload2))
	req2.Header.Set("X-Session-ID", sessionID)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Turn 2 failed: %d", rec2.Code)
	}

	// Turn 1 should not have injected override
	if strings.Contains(capturedBodies[0], "[SYSTEM OVERRIDE]") {
		t.Errorf("Turn 1 should NOT have injected override")
	}
	// Turn 2 MUST have injected override because read-only tools do not count as write progress
	if !strings.Contains(capturedBodies[1], "[SYSTEM OVERRIDE]") {
		t.Errorf("Turn 2 expected injected [SYSTEM OVERRIDE] in write-only mode, got: %s", capturedBodies[1])
	}

	// Turn 3: Productive write tool execution (write_to_file) -> HasWriteProgress=true -> Counter resets!
	reqPayload3 := `{
		"model": "nacho-hybrid",
		"messages": [
			{"role": "user", "content": "Write file"},
			{"role": "assistant", "tool_calls": [{"id": "c3", "type": "function", "function": {"name": "write_to_file"}}]},
			{"role": "tool", "tool_call_id": "c3", "content": "File written"}
		]
	}`
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload3))
	req3.Header.Set("X-Session-ID", sessionID)
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("Turn 3 failed: %d", rec3.Code)
	}

	// Turn 3 must NOT have injected override
	if strings.Contains(capturedBodies[2], "[SYSTEM OVERRIDE]") {
		t.Errorf("Turn 3 with write progress should NOT have injected override, got: %s", capturedBodies[2])
	}
}

func TestProxy_CycleBreaker_AutoEscalationAndCooldown(t *testing.T) {
	// Mock Flash server: loops thinking monologue
	mockFlash := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Stream repeated thinking tokens exceeding threshold
		for i := 0; i < 8; i++ {
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking step over and over again \"}}]}\n\n"))
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockFlash.Close()

	// Mock Pro server: returns valid tool call
	mockPro := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_pro\",\"type\":\"function\",\"function\":{\"name\":\"write_to_file\",\"arguments\":\"{}\"}}]}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockPro.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleBreaker: contract.CycleBreakerConfig{
			Enabled:                     &enabled,
			MaxThinkingTokens:           10,
			RepetitionWindow:            20,
			RepetitionThreshold:         2,
			MaxRetries:                  0, // sever immediately
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 2: Fast Cloud Bridge (Flash)",
				Model:    "deepseek/deepseek-v4-flash",
				Provider: "openrouter-flash",
				When:     "Tokens < 64000 && Retries < 3",
			},
			{
				Name:     "Tier 3: Deep Coding Workhorse (Pro)",
				Model:    "deepseek/deepseek-chat",
				Provider: "openrouter-pro",
				When:     "Tokens < 64000 && Retries < 5",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 4: Frontier Default",
			Model:    "anthropic/claude-sonnet-5",
			Provider: "openrouter-pro",
		},
		Providers: map[string]contract.ProviderConfig{
			"openrouter-flash": {
				BaseURL: mockFlash.URL,
			},
			"openrouter-pro": {
				BaseURL: mockPro.URL,
			},
		},
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier, cfg.Providers)
	if err != nil {
		t.Fatalf("failed to compile evaluator: %v", err)
	}

	srv := NewServer(cfg, evaluator, nil, nil)
	sessionID := "sess-cooldown-integration-test"

	// Turn 1: Fresh request at 30k tokens with tools -> matches Tier 2 Flash -> severed by Cycle Killer
	reqPayload1 := `{
		"model":"nacho-hybrid",
		"stream":true,
		"messages":[{"role":"user","content":"Initial prompt"}],
		"tools":[{"type":"function","function":{"name":"write_to_file","description":"write"}}]
	}`
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload1))
	req1.Header.Set("X-Session-ID", sessionID)
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("Turn 1 failed with status %d", rec1.Code)
	}

	// Verify cooldown was registered on session tracker for Tier 2 model
	if !srv.sessionTracker.IsModelCoolingDown(sessionID, "deepseek/deepseek-v4-flash") {
		t.Errorf("expected deepseek/deepseek-v4-flash to be in cooldown after cycle kill")
	}

	// Turn 2: Context pruned (different prompt) -> Retries floor applied, Tier 2 in cooldown -> auto-escalates to Tier 3 Pro!
	reqPayload2 := `{
		"model":"nacho-hybrid",
		"stream":true,
		"messages":[{"role":"user","content":"Pruned context prompt"}],
		"tools":[{"type":"function","function":{"name":"write_to_file","description":"write"}}]
	}`
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload2))
	req2.Header.Set("X-Session-ID", sessionID)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("Turn 2 failed with status %d", rec2.Code)
	}
	tierHeader2 := rec2.Header().Get(contract.HeaderNachoRouterTier)
	if tierHeader2 != "Tier 3: Deep Coding Workhorse (Pro)" {
		t.Errorf("Turn 2 expected auto-escalation to Tier 3 Pro, got: %s", tierHeader2)
	}
}




