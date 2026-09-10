package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
)

// TestProxy_ToolCallLoopSevering tests that when a model emits 0 prose tokens
// and streams pure tool-calls with repetitive arguments (the exact failure mode observed with runaway tool loops),
// the in-flight streaming Cycle Killer tool lane engages and severs the stream with a system override.
func TestProxy_ToolCallLoopSevering(t *testing.T) {
	// Mock upstream simulating model streaming repetitive tool-call arguments
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support Flusher")
		}

		// Initial chunk opening execute_command tool call
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"execute_command\",\"arguments\":\"echo 'inspecting card types' && awk \"}}]}}]}\n\n"))
		flusher.Flush()

		// Repeated argument chunks simulating runaway command pipeline loop
		for i := 0; i < 20; i++ {
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"'{print $1}' cards.go && echo 'inspecting card types' && awk \"}}]}}]}\n\n"))
			flusher.Flush()
		}

		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleKiller: contract.CycleBreakerConfig{
			Enabled:             &enabled,
			RepetitionThreshold: 3,
			RepetitionWindow:    6,
			MaxToolTokens:       4096,
		},
		Providers: map[string]contract.ProviderConfig{
			"loop_provider": {
				BaseURL: mockUpstream.URL,
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Loop Tier",
				Model:    "loop-model-v1",
				Provider: "loop_provider",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "loop-model-v1",
		"stream": true,
		"messages": [{"role": "user", "content": "Fix the card replacement bug"}],
		"tools": [{"type": "function", "function": {"name": "execute_command"}}]
	}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	respBody := rec.Body.String()
	bodyStr := rec.Body.String()

	// Verify that the stream was intercepted and severed by the tool-call cycle killer with a protocol-safe SSE error
	if !strings.Contains(bodyStr, "tool repetition loop detected") {
		t.Fatalf("Expected stream to be severed with tool repetition cycle killer message, got body:\n%s", bodyStr)
	}

	if !strings.Contains(respBody, "cycle_killer_error") {
		t.Errorf("Expected severed stream to emit cycle_killer_error type, got body:\n%s", respBody)
	}
	if !strings.Contains(respBody, "tool_cycle_detected") {
		t.Errorf("Expected severed stream to emit tool_cycle_detected code, got body:\n%s", respBody)
	}
	// Verify that no invalid finish_reason: stop chunk was sent with incomplete JSON
	if strings.Contains(respBody, `"finish_reason":"stop"`) {
		t.Errorf("Severed tool call stream should not emit finish_reason: stop to prevent JSON parse crashes")
	}
	if !strings.Contains(respBody, "[DONE]") {
		t.Errorf("Expected severed stream to end with [DONE]")
	}
}

// TestProxy_FailingTestReadOnlyLoop_EscalatesToCloud verifies that when an agent
// runs a failing test, then runs read-only commands (cat/read_file) without making write progress,
// retries accumulate under KickstartWriteOnly and trigger auto-escalation to the cloud tier.
// It also verifies that when a genuine shell write is executed, retries reset back to 0.
func TestProxy_FailingTestReadOnlyLoop_EscalatesToCloud(t *testing.T) {
	localRequests := 0
	cloudRequests := 0

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		localRequests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Local response"}}]}`))
	}))
	defer mockLocal.Close()

	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cloudRequests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Cloud response"}}]}`))
	}))
	defer mockCloud.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		CycleKiller: contract.CycleBreakerConfig{
			Enabled:             &enabled,
			KickstartThreshold:  3,
			KickstartWriteOnly:  true,
			KickstartWriteTools: []string{"write_to_file", "replace_in_file"},
		},
		Providers: map[string]contract.ProviderConfig{
			"local_provider": {
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
				Name:     "Cloud Fallback Tier",
				Model:    "claude-3-7-sonnet",
				Provider: "cloud_provider",
				When:     "Retries >= 2",
			},
			{
				Name:     "Local Tier",
				Model:    "gemma4:12b",
				Provider: "local_provider",
				When:     "true",
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

	sessionID := "agent-loop-session-123"

	// Turn 1: User prompt. Initial request -> retries = 0 -> routes to Local Tier
	turn1Payload := `{
		"model": "auto",
		"messages": [
			{"role": "user", "content": "Fix the failing tests in cards_test.go"}
		],
		"tools": [
			{"type": "function", "function": {"name": "execute_command"}},
			{"type": "function", "function": {"name": "write_to_file"}}
		]
	}`
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(turn1Payload))
	req1.Header.Set("X-Session-ID", sessionID)
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("Turn 1 failed: %d", rec1.Code)
	}
	if localRequests != 1 || cloudRequests != 0 {
		t.Fatalf("Turn 1 expected Local=1 Cloud=0, got Local=%d Cloud=%d", localRequests, cloudRequests)
	}

	// Turn 2: Same prompt. Agent ran `go test ./...` and it failed.
	// HasTestFail=true, HasWriteProgress=false -> Under KickstartWriteOnly, failing tests don't reset retries!
	// retries becomes 1 -> routes to Local Tier (since retries < 2)
	turn2Payload := `{
		"model": "auto",
		"messages": [
			{"role": "user", "content": "Fix the failing tests in cards_test.go"},
			{"role": "assistant", "tool_calls": [{"id": "c1", "type": "function", "function": {"name": "execute_command", "arguments": "{\"command\":\"go test ./...\"}"}}]},
			{"role": "tool", "tool_call_id": "c1", "content": "--- FAIL: TestCardDeal (0.02s)\nFAIL\tpkg/cards\t0.03s"}
		],
		"tools": [
			{"type": "function", "function": {"name": "execute_command"}},
			{"type": "function", "function": {"name": "write_to_file"}}
		]
	}`
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(turn2Payload))
	req2.Header.Set("X-Session-ID", sessionID)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Turn 2 failed: %d", rec2.Code)
	}
	if localRequests != 2 || cloudRequests != 0 {
		t.Fatalf("Turn 2 expected Local=2 Cloud=0, got Local=%d Cloud=%d", localRequests, cloudRequests)
	}

	// Turn 3: Same prompt. Agent runs read-only command (`cat cards.go` or `read_file`) to inspect context.
	// In the old code, this read tool call was treated as forward progress and reset retries = 0!
	// With our guard, read-only tools do NOT count as write progress.
	// retries becomes 2 -> Triggers "Retries >= 2" -> ESCALATES TO CLOUD TIER!
	turn3Payload := `{
		"model": "auto",
		"messages": [
			{"role": "user", "content": "Fix the failing tests in cards_test.go"},
			{"role": "assistant", "tool_calls": [{"id": "c1", "type": "function", "function": {"name": "execute_command", "arguments": "{\"command\":\"go test ./...\"}"}}]},
			{"role": "tool", "tool_call_id": "c1", "content": "--- FAIL: TestCardDeal (0.02s)\nFAIL\tpkg/cards\t0.03s"},
			{"role": "assistant", "tool_calls": [{"id": "c2", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\":\"cards.go\"}"}}]},
			{"role": "tool", "tool_call_id": "c2", "content": "package cards\n\nfunc Deal() {}"}
		],
		"tools": [
			{"type": "function", "function": {"name": "execute_command"}},
			{"type": "function", "function": {"name": "write_to_file"}}
		]
	}`
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(turn3Payload))
	req3.Header.Set("X-Session-ID", sessionID)
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("Turn 3 failed: %d", rec3.Code)
	}
	if localRequests != 2 || cloudRequests != 1 {
		t.Fatalf("Turn 3 expected escalation to Cloud! Local=%d Cloud=%d", localRequests, cloudRequests)
	}

	// Turn 4: Same prompt. Cloud model performs a shell write: execute_command with `sed -i 's/foo/bar/' cards.go`.
	// HasShellWrite=true -> Genuine forward progress!
	// Retries reset to 0 -> Session de-escalates back to Local Tier!
	turn4Payload := `{
		"model": "auto",
		"messages": [
			{"role": "user", "content": "Fix the failing tests in cards_test.go"},
			{"role": "assistant", "tool_calls": [{"id": "c3", "type": "function", "function": {"name": "execute_command", "arguments": "{\"command\":\"sed -i 's/old/new/g' cards.go\"}"}}]},
			{"role": "tool", "tool_call_id": "c3", "content": "success"}
		],
		"tools": [
			{"type": "function", "function": {"name": "execute_command"}},
			{"type": "function", "function": {"name": "write_to_file"}}
		]
	}`
	req4 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(turn4Payload))
	req4.Header.Set("X-Session-ID", sessionID)
	rec4 := httptest.NewRecorder()
	srv.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("Turn 4 failed: %d", rec4.Code)
	}
	if localRequests != 3 || cloudRequests != 1 {
		t.Fatalf("Turn 4 expected de-escalation back to Local tier after shell write! Local=%d Cloud=%d", localRequests, cloudRequests)
	}
}

// TestProxy_StreamOptionsAndCalibration verifies that the proxy automatically injects
// stream_options: {"include_usage": true} into streaming requests, calibrates the estimator when
// usage arrives, and emits a single deduplicated warning when the provider omits usage.
func TestProxy_StreamOptionsAndCalibration(t *testing.T) {
	var capturedStreamOptions []byte
	requestCount := 0

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		body, _ := io.ReadAll(r.Body)
		if bytes.Contains(body, []byte("stream_options")) {
			capturedStreamOptions = body
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
		flusher.Flush()

		if requestCount == 1 {
			// Request 1: Emit final chunk with usage object
			w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1500,\"completion_tokens\":10,\"total_tokens\":1510}}\n\n"))
			flusher.Flush()
		}
		// Request 2 & 3: Emit NO usage object (simulating upstream provider omitting usage)
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
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

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	srv := NewServer(cfg, evaluator, classifier, sanitizer)
	srv.logger = logger

	rcClassifier := classifier.(*router.RequestClassifier)
	initialRatio := rcClassifier.GetEstimator().GetRatio()

	// 1. First streaming request: verify stream_options injected and estimator calibrated
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "stream-model-v1",
		"stream": true,
		"messages": [{"role": "user", "content": "Hello"}]
	}`))
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("Request 1 failed: %d", rec1.Code)
	}

	if !bytes.Contains(capturedStreamOptions, []byte(`"stream_options":{"include_usage":true}`)) {
		t.Errorf("Expected stream_options with include_usage: true to be injected upstream, got: %s", string(capturedStreamOptions))
	}

	if rcClassifier.GetEstimator().GetRatio() == initialRatio {
		t.Errorf("Expected estimator ratio to calibrate from streaming usage, but it remained %v", initialRatio)
	}

	// 2. Second request: Upstream omits usage -> verify WARN log
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "stream-model-v1",
		"stream": true,
		"messages": [{"role": "user", "content": "Second turn without usage"}]
	}`))
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Request 2 failed: %d", rec2.Code)
	}

	logStr := logBuf.String()
	if !strings.Contains(logStr, "zero usage tokens reported by provider") || !strings.Contains(logStr, "stream_provider") {
		t.Errorf("Expected warning about zero usage tokens for stream_provider, got: %s", logStr)
	}

	// 3. Third request: Upstream omits usage again -> verify WARN log is NOT duplicated
	logBuf.Reset()
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{
		"model": "stream-model-v1",
		"stream": true,
		"messages": [{"role": "user", "content": "Third turn without usage"}]
	}`))
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("Request 3 failed: %d", rec3.Code)
	}

	if strings.Contains(logBuf.String(), "zero usage tokens reported by provider") {
		t.Errorf("Expected warning to be deduplicated per provider, but it logged again: %s", logBuf.String())
	}
}
