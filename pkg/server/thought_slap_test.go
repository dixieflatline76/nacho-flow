// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
)

const f1StreamPayload = `data: {"id":"gen-1","choices":[{"index":0,"delta":{"reasoning_content":"Extensive reasoning about code architecture..."}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{},"finish_reason":"length"}]}

data: {"id":"gen-1","choices":[],"usage":{"prompt_tokens":500,"completion_tokens":4000,"total_tokens":4500}}

data: [DONE]
`

const toolCallStreamPayload = `data: {"id":"gen-rec","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"gen-rec","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"write_to_file","arguments":"{\"path\":\"main.go\"}"}}]}}]}

data: {"id":"gen-rec","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"id":"gen-rec","choices":[],"usage":{"prompt_tokens":600,"completion_tokens":100,"total_tokens":700}}

data: [DONE]
`

const contentStreamPayload = `data: {"id":"gen-rec","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"gen-rec","choices":[{"index":0,"delta":{"content":"Here is the direct answer without meta-reasoning."}}]}

data: {"id":"gen-rec","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"gen-rec","choices":[],"usage":{"prompt_tokens":550,"completion_tokens":50,"total_tokens":600}}

data: [DONE]
`

func setupTestServerWithThoughtSlap(t *testing.T, handler http.HandlerFunc, tsOptIn *contract.ThoughtSlapOptIn, rescueTier *contract.Tier) (*Server, *httptest.Server, *contract.Config) {
	t.Helper()
	mockUpstream := httptest.NewServer(handler)

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock-prov": {
				BaseURL: mockUpstream.URL,
				APIKey:  "test-key",
				Type:    provider.TypeOpenAICompatible,
			},
		},
		Tiers: []contract.Tier{
			{
				Name:        "Tier 1: Architect",
				Model:       "glm-5.3-flash",
				Provider:    "mock-prov",
				When:        "true",
				ThoughtSlap: tsOptIn,
			},
		},
	}

	if rescueTier != nil {
		cfg.Tiers = append(cfg.Tiers, *rescueTier)
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)
	return srv, mockUpstream, cfg
}

func TestThoughtSlap_RecoveredToolCall(t *testing.T) {
	var callCount int32
	var capturedBodies [][]byte

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		body, _ := io.ReadAll(r.Body)
		capturedBodies = append(capturedBodies, body)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		if count == 1 {
			_, _ = w.Write([]byte(f1StreamPayload))
		} else {
			_, _ = w.Write([]byte(toolCallStreamPayload))
		}
	})

	optIn := &contract.ThoughtSlapOptIn{Enabled: true}
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, nil)
	defer mockUpstream.Close()

	reqPayload := `{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"Implement feature X"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	res := rec.Body.String()

	// Invariant: Wire contains thought slap notice reasoning chunk
	if !strings.Contains(res, contract.ThoughtSlapNoticeText) {
		t.Errorf("expected response to contain thought slap notice %q, got:\n%s", contract.ThoughtSlapNoticeText, res)
	}

	// Invariant: Wire contains recovered tool call
	if !strings.Contains(res, "write_to_file") {
		t.Errorf("expected response to contain tool call 'write_to_file', got:\n%s", res)
	}

	// Invariant: Prelude suppressed (role: assistant not duplicated)
	if strings.Contains(res, `"role":"assistant"`) {
		t.Errorf("expected role-only prelude chunk to be suppressed, got:\n%s", res)
	}

	// Invariant: Exactly two upstream calls
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 upstream calls, got %d", callCount)
	}

	// Invariant: Attempt 1 received injected correction prompt
	if len(capturedBodies) >= 2 {
		if !strings.Contains(string(capturedBodies[1]), contract.DefaultThoughtSlapPrompt) {
			t.Errorf("expected recovery attempt to receive DefaultThoughtSlapPrompt, got:\n%s", string(capturedBodies[1]))
		}
	}

	// Invariant: Single [DONE] at stream end
	doneCount := strings.Count(res, "data: [DONE]")
	if doneCount != 1 {
		t.Errorf("expected exactly 1 [DONE], got %d", doneCount)
	}
}

func TestThoughtSlap_RecoveredContent(t *testing.T) {
	var callCount int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		if count == 1 {
			_, _ = w.Write([]byte(f1StreamPayload))
		} else {
			_, _ = w.Write([]byte(contentStreamPayload))
		}
	})

	optIn := &contract.ThoughtSlapOptIn{Enabled: true}
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, nil)
	defer mockUpstream.Close()

	reqPayload := `{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"Explain algorithm"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	res := rec.Body.String()
	if !strings.Contains(res, contract.ThoughtSlapNoticeText) {
		t.Errorf("expected thought slap notice, got:\n%s", res)
	}
	if !strings.Contains(res, "Here is the direct answer") {
		t.Errorf("expected recovered content, got:\n%s", res)
	}
	if strings.Count(res, "data: [DONE]") != 1 {
		t.Errorf("expected exactly 1 [DONE], got %d", strings.Count(res, "data: [DONE]"))
	}
}

func TestThoughtSlap_ExhaustedWithHonestTerminal(t *testing.T) {
	var callCount int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Both calls truncate on length with no content
		_, _ = w.Write([]byte(f1StreamPayload))
	})

	optIn := &contract.ThoughtSlapOptIn{
		Enabled: true,
		Overrides: contract.ThoughtSlapConfig{
			MaxAttempts: 1,
		},
	}
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, nil)
	defer mockUpstream.Close()

	reqPayload := `{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"Looping task"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	res := rec.Body.String()

	// Invariant: Emits thought slap notice before attempt 1
	if !strings.Contains(res, contract.ThoughtSlapNoticeText) {
		t.Errorf("expected thought slap notice, got:\n%s", res)
	}

	// Invariant: Emits Phase 1 honest truncation notice after exhaustion
	if !strings.Contains(res, "Reasoning Limit Reached") {
		t.Errorf("expected honest truncation notice on exhaustion, got:\n%s", res)
	}

	// Invariant: Preserves finish_reason: length
	if !strings.Contains(res, `"finish_reason":"length"`) {
		t.Errorf("expected finish_reason: length, got:\n%s", res)
	}

	// Invariant: Emits summed usage chunk (Decision D3)
	// Attempt 0: 500 prompt, 4000 comp, 4500 tot
	// Attempt 1: 500 prompt, 4000 comp, 4500 tot
	// Summed: 1000 prompt, 8000 comp, 9000 tot
	if !strings.Contains(res, `"prompt_tokens":1000`) || !strings.Contains(res, `"completion_tokens":8000`) {
		t.Errorf("expected summed usage (prompt: 1000, completion: 8000), got:\n%s", res)
	}

	// Invariant: Exactly 1 [DONE]
	if strings.Count(res, "data: [DONE]") != 1 {
		t.Errorf("expected exactly 1 [DONE], got %d", strings.Count(res, "data: [DONE]"))
	}

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 calls (initial + 1 attempt), got %d", callCount)
	}
}

func TestThoughtSlap_UpstreamError_ValidSSE(t *testing.T) {
	var callCount int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(f1StreamPayload))
		} else {
			// Attempt 1 fails with 500
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	})

	optIn := &contract.ThoughtSlapOptIn{Enabled: true}
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, nil)
	defer mockUpstream.Close()

	reqPayload := `{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"Failing task"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	// Invariant: HTTP Status is 200 because SSE headers were already committed!
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	res := rec.Body.String()

	// Invariant: Valid SSE terminal sequence emitted (no raw error text leaking outside SSE)
	if !strings.Contains(res, "Reasoning Limit Reached") {
		t.Errorf("expected honest truncation notice on upstream error, got:\n%s", res)
	}
	if !strings.Contains(res, `"finish_reason":"length"`) {
		t.Errorf("expected finish_reason: length, got:\n%s", res)
	}
	if strings.Count(res, "data: [DONE]") != 1 {
		t.Errorf("expected exactly 1 [DONE], got %d", strings.Count(res, "data: [DONE]"))
	}
}

func TestThoughtSlap_RescueTier(t *testing.T) {
	var callCount int32
	var capturedModels []string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		body, _ := io.ReadAll(r.Body)
		var reqMap map[string]any
		_ = json.Unmarshal(body, &reqMap)
		if m, ok := reqMap["model"].(string); ok {
			capturedModels = append(capturedModels, m)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if callCount == 1 {
			_, _ = w.Write([]byte(f1StreamPayload))
		} else {
			_, _ = w.Write([]byte(toolCallStreamPayload))
		}
	})

	optIn := &contract.ThoughtSlapOptIn{
		Enabled: true,
		Overrides: contract.ThoughtSlapConfig{
			RescueTier: "Tier 2: Fast Rescue",
		},
	}
	rescueTier := &contract.Tier{
		Name:     "Tier 2: Fast Rescue",
		Model:    "claude-3-5-haiku",
		Provider: "mock-prov",
		When:     "false",
	}

	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, rescueTier)
	defer mockUpstream.Close()

	reqPayload := `{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"Needs rescue"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if len(capturedModels) != 2 {
		t.Fatalf("expected 2 captured models, got %d: %v", len(capturedModels), capturedModels)
	}
	if capturedModels[0] != "glm-5.3-flash" {
		t.Errorf("expected initial call to model glm-5.3-flash, got %s", capturedModels[0])
	}
	if capturedModels[1] != "claude-3-5-haiku" {
		t.Errorf("expected rescue call to model claude-3-5-haiku, got %s", capturedModels[1])
	}
}

func TestThoughtSlap_PlanModeNotSuspended(t *testing.T) {
	var callCount int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if count == 1 {
			_, _ = w.Write([]byte(f1StreamPayload))
		} else {
			_, _ = w.Write([]byte(contentStreamPayload))
		}
	})

	optIn := &contract.ThoughtSlapOptIn{Enabled: true}
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, nil)
	defer mockUpstream.Close()

	// Plan mode: tools present (read-only tools) but no write tools
	reqPayload := `{
		"model":"nacho-hybrid",
		"stream":true,
		"messages":[{"role":"user","content":"Investigate file"}],
		"tools":[{"type":"function","function":{"name":"read_file"}}]
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Thought slap must NOT be suspended by Plan Mode!
	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected Thought Slap to execute in Plan Mode (2 calls), got %d calls", callCount)
	}

	res := rec.Body.String()
	if !strings.Contains(res, "Here is the direct answer") {
		t.Errorf("expected recovered content in Plan Mode, got:\n%s", res)
	}
}

func TestThoughtSlap_KickstartOff_Disabled(t *testing.T) {
	var callCount int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(f1StreamPayload))
	})

	optIn := &contract.ThoughtSlapOptIn{Enabled: true}
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, nil)
	defer mockUpstream.Close()

	// @nacho:kickstart-off in prompt disables both read slap and thought slap
	reqPayload := `{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"@nacho:kickstart-off Run without kickstart"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Must NOT attempt recovery
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 call when kickstart is off, got %d", callCount)
	}

	res := rec.Body.String()
	// Honest truncation notice emitted immediately without thought slap notice
	if strings.Contains(res, contract.ThoughtSlapNoticeText) {
		t.Errorf("thought slap notice should NOT be present when kickstart is off, got:\n%s", res)
	}
	if !strings.Contains(res, "Reasoning Limit Reached") {
		t.Errorf("expected honest truncation notice, got:\n%s", res)
	}
}

func TestThoughtSlap_NoOptIn_Disabled(t *testing.T) {
	var callCount int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(f1StreamPayload))
	})

	// Thought slap is nil (not opted in)
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, nil, nil)
	defer mockUpstream.Close()

	reqPayload := `{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"No opt in tier"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 call without opt-in, got %d", callCount)
	}

	res := rec.Body.String()
	if strings.Contains(res, contract.ThoughtSlapNoticeText) {
		t.Errorf("thought slap notice should NOT be present without opt-in, got:\n%s", res)
	}
}

func TestThoughtSlap_UnaryRecovery(t *testing.T) {
	var callCount int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if count == 1 {
			// Empty truncated turn (finish_reason: length, content: "", no tools)
			resp := map[string]any{
				"id": "unary-1",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":              "assistant",
							"content":           "",
							"reasoning_content": "Extensive unary reasoning...",
						},
						"finish_reason": "length",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     100,
					"completion_tokens": 1000,
					"total_tokens":      1100,
				},
			}
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		} else {
			// Recovered response with tool calls
			resp := map[string]any{
				"id": "unary-2",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]any{
								{
									"id":   "call_u1",
									"type": "function",
									"function": map[string]any{
										"name":      "execute_command",
										"arguments": "{\"command\":\"go test ./...\"}",
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     150,
					"completion_tokens": 50,
					"total_tokens":      200,
				},
			}
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		}
	})

	optIn := &contract.ThoughtSlapOptIn{Enabled: true}
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, nil)
	defer mockUpstream.Close()

	reqPayload := `{"model":"nacho-hybrid","stream":false,"messages":[{"role":"user","content":"Run tests"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 calls for unary recovery, got %d", callCount)
	}

	res := rec.Body.String()
	if !strings.Contains(res, "execute_command") {
		t.Errorf("expected recovered tool call in unary response, got:\n%s", res)
	}
}

func TestThoughtSlap_ClientCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var callCount int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(f1StreamPayload))
		} else {
			cancel()
			http.Error(w, "cancelled", http.StatusRequestTimeout)
			return
		}
	})

	optIn := &contract.ThoughtSlapOptIn{Enabled: true}
	srv, mockUpstream, _ := setupTestServerWithThoughtSlap(t, handler, optIn, nil)
	defer mockUpstream.Close()

	reqPayload := `{"model":"nacho-hybrid","stream":true,"messages":[{"role":"user","content":"Cancelled task"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload)).WithContext(ctx)
	srv.ServeHTTP(rec, req)

	// Since context is cancelled, recovery should abort with client_cancelled
	res := rec.Body.String()
	if !strings.Contains(res, "Reasoning Limit Reached") {
		t.Errorf("expected honest truncation notice on cancelled turn, got:\n%s", res)
	}
}

func TestThoughtSlap_HelpersCoverage(t *testing.T) {
	// findRescueTier
	if _, ok := findRescueTier("", nil); ok {
		t.Error("expected false for empty tierName")
	}
	cfg := &contract.Config{
		DefaultTier: contract.Tier{Name: "default_fallback"},
		Tiers: []contract.Tier{
			{Name: "tier_custom"},
		},
	}
	if tier, ok := findRescueTier("default_fallback", cfg); !ok || tier.Name != "default_fallback" {
		t.Error("expected to find default_fallback")
	}
	if tier, ok := findRescueTier("tier_custom", cfg); !ok || tier.Name != "tier_custom" {
		t.Error("expected to find tier_custom")
	}
	if _, ok := findRescueTier("non_existent", cfg); ok {
		t.Error("expected false for non_existent")
	}

	// accumulateUsage
	total := &StreamUsage{}
	attempt1 := StreamUsage{
		PromptTokens:        10,
		CompletionTokens:    20,
		TotalTokens:         30,
		Cost:                0.05,
		PromptTokensDetails: &PromptTokensDetails{CachedTokens: 5},
	}
	accumulateUsage(total, attempt1)
	if total.PromptTokens != 10 || total.PromptTokensDetails.CachedTokens != 5 {
		t.Errorf("unexpected first accumulateUsage result: %+v", total)
	}
	// Second attempt tests branch where total.PromptTokensDetails != nil
	attempt2 := StreamUsage{
		PromptTokens:        5,
		CompletionTokens:    5,
		TotalTokens:         10,
		Cost:                0.01,
		PromptTokensDetails: &PromptTokensDetails{CachedTokens: 2},
	}
	accumulateUsage(total, attempt2)
	if total.PromptTokens != 15 || total.PromptTokensDetails.CachedTokens != 7 {
		t.Errorf("unexpected second accumulateUsage result: %+v", total)
	}

	// StreamNormalizer.HeldTerminal()
	sn := &StreamNormalizer{}
	if sn.HeldTerminal() != nil {
		t.Error("expected nil for HeldTerminal when heldTerminal is nil")
	}
}
