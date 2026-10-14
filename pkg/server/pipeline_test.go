package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// setupMockPipelineServer creates a test Server with a captured upstream HTTP server
// and controllable routing configuration.
func setupMockPipelineServer(
	t *testing.T,
	cfg *contract.Config,
	capturedBodies *[][]byte,
) (*Server, *httptest.Server) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err == nil && capturedBodies != nil {
			*capturedBodies = append(*capturedBodies, b)
		}
		_ = r.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "mock-upstream-model",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "ok",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]contract.ProviderConfig)
	}
	cfg.Providers["openrouter"] = contract.ProviderConfig{
		BaseURL: mockUpstream.URL,
		APIKey:  "test-key",
	}

	reg := provider.NewRegistry()
	reg.Register(provider.NewGenericLLMProvider("openrouter", cfg.Providers["openrouter"]))

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		t.Fatalf("failed to create evaluator: %v", err)
	}
	classifier := router.NewClassifier()
	if c, ok := classifier.(*router.RequestClassifier); ok {
		c.SetKickstartWriteTools([]string{"write_to_file", "replace_file_content"})
	}

	srv := NewServerWithTelemetryAndRegistry(
		cfg,
		evaluator,
		classifier,
		router.NewSanitizer(),
		telemetry.NewPricingOracle(),
		telemetry.NewStatsTracker(10),
		reg,
		nil,
	)

	return srv, mockUpstream
}

// TestPipeline_AntiLatchingBreaker_UnderBudgetAndDeescalate tests the Anti-Latching De-escalation Breaker
// (lines 364–386 in pipeline.go):
// 1. Turns 1–3 stay on DefaultTier (escalation counter 1, 2, 3 <= MaxEscalationTurns 3).
// 2. Turn 4 exceeds budget (counter 4 > 3) and forcibly de-escalates to the non-default workhorse tier.
// 3. Turn 5 on workhorse resets the escalation counter back to 0.
func TestPipeline_AntiLatchingBreaker_UnderBudgetAndDeescalate(t *testing.T) {
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Workhorse Tier",
				Model:    "qwen3-coder-480b",
				Provider: "openrouter",
				When:     "Retries < 2",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Frontier Default Tier",
			Model:    "claude-3.7-sonnet",
			Provider: "openrouter",
		},
	}

	srv, mockUpstream := setupMockPipelineServer(t, cfg, nil)
	defer mockUpstream.Close()

	sessionID := "sess-anti-latch-test"

	// Turns 1 to 3: Error turns where retries >= 2 locks out Workhorse Tier.
	// targetTier must remain "Frontier Default Tier" because budget <= 3.
	errorPayload := `{"model":"auto","messages":[{"role":"user","content":"Initial prompt"},{"role":"assistant","content":"Working..."},{"role":"tool","content":"{\"success\": false, \"error\": \"failed 1\"}"},{"role":"tool","content":"{\"success\": false, \"error\": \"failed 2\"}"}]}`
	for turn := 1; turn <= 3; turn++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(errorPayload)))
		req.Header.Set("x-session-id", sessionID)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("turn %d: unexpected status code %d: %s", turn, w.Code, w.Body.String())
		}
		routerTier := w.Header().Get("x-nacho-router-tier")
		if routerTier != "Frontier Default Tier" {
			t.Fatalf("turn %d: expected tier 'Frontier Default Tier', got %q", turn, routerTier)
		}
	}

	// Turn 4: Still on error, but escalation counter hits 4 (> MaxEscalationTurns=3).
	// Anti-Latching Breaker MUST forcibly de-escalate to "Workhorse Tier"!
	req4 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(errorPayload)))
	req4.Header.Set("x-session-id", sessionID)
	w4 := httptest.NewRecorder()
	srv.ServeHTTP(w4, req4)

	if w4.Code != http.StatusOK {
		t.Fatalf("turn 4: unexpected status code %d", w4.Code)
	}
	routerTier4 := w4.Header().Get("x-nacho-router-tier")
	if routerTier4 != "Workhorse Tier" {
		t.Fatalf("turn 4: expected de-escalation to 'Workhorse Tier', got %q", routerTier4)
	}

	// Turn 5: Clean turn with tool progress (retries = 0, no errors).
	// TargetTier is naturally "Workhorse Tier" (not default tier).
	// Tracker must reset escalation counter!
	cleanPayload := `{"model":"auto","messages":[{"role":"assistant","content":"","tool_calls":[{"id":"1","type":"function","function":{"name":"write_to_file","arguments":"{}"}}]},{"role":"tool","tool_call_id":"1","content":"done"},{"role":"user","content":"next task"}]}`
	req5 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(cleanPayload)))
	req5.Header.Set("x-session-id", sessionID)
	w5 := httptest.NewRecorder()
	srv.ServeHTTP(w5, req5)

	if w5.Code != http.StatusOK {
		t.Fatalf("turn 5: unexpected status code %d", w5.Code)
	}
	routerTier5 := w5.Header().Get("x-nacho-router-tier")
	if routerTier5 != "Workhorse Tier" {
		t.Fatalf("turn 5: expected 'Workhorse Tier', got %q", routerTier5)
	}

	// Turn 6: Error again. Since budget was reset to 0 in Turn 5, Turn 6 is Turn 1 of escalation.
	// It should again be under budget and stay on "Frontier Default Tier".
	req6 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(errorPayload)))
	req6.Header.Set("x-session-id", sessionID)
	w6 := httptest.NewRecorder()
	srv.ServeHTTP(w6, req6)

	if w6.Code != http.StatusOK {
		t.Fatalf("turn 6: unexpected status code %d", w6.Code)
	}
	routerTier6 := w6.Header().Get("x-nacho-router-tier")
	if routerTier6 != "Frontier Default Tier" {
		t.Fatalf("turn 6: expected 'Frontier Default Tier' after budget reset, got %q", routerTier6)
	}
}

// TestPipeline_HistoryErrorsOverride tests lines 348–355 in pipeline.go:
// In-history errors override session tracker's retries when they detect real failures.
func TestPipeline_HistoryErrorsOverride(t *testing.T) {
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Workhorse Tier",
				Model:    "qwen3-coder-480b",
				Provider: "openrouter",
				When:     "Retries < 1",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Frontier Default Tier",
			Model:    "claude-3.7-sonnet",
			Provider: "openrouter",
		},
	}

	srv, mockUpstream := setupMockPipelineServer(t, cfg, nil)
	defer mockUpstream.Close()

	// New session: tracker has 0 retries.
	// But payload has tool error message, so classifier extracts HistoryErrors = 1.
	// In-history errors should override retries from 0 to 1, locking out Workhorse Tier (Retries < 1).
	payload := `{"model":"auto","messages":[{"role":"user","content":"Initial prompt"},{"role":"assistant","content":"Working..."},{"role":"tool","content":"{\"success\": false, \"error\": \"test execution failed\"}"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(payload)))
	req.Header.Set("x-session-id", "sess-history-errors-test")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected code %d", w.Code)
	}
	tier := w.Header().Get("x-nacho-router-tier")
	if tier != "Frontier Default Tier" {
		t.Errorf("expected history errors to override retries and route to Frontier Default Tier, got %q", tier)
	}
}

// TestPipeline_EmbeddedGuardrailToggles tests lines 168–207 in pipeline.go:
// Embedded directives toggle guardrails in session tracker and mask feature flags.
func TestPipeline_EmbeddedGuardrailToggles(t *testing.T) {
	cfg := &contract.Config{
		DefaultTier: contract.Tier{
			Name:     "Default",
			Model:    "default-model",
			Provider: "openrouter",
		},
	}

	srv, mockUpstream := setupMockPipelineServer(t, cfg, nil)
	defer mockUpstream.Close()

	sessionKey := "sess-guardrails-test"

	testCases := []struct {
		directive   string
		checkAssert func(g router.SessionGuardrails) bool
		desc        string
	}{
		{"kickstart-off", func(g router.SessionGuardrails) bool { return g.KickstartDisabled }, "KickstartDisabled=true"},
		{"kickstart-on", func(g router.SessionGuardrails) bool { return !g.KickstartDisabled }, "KickstartDisabled=false"},
		{"cyclekiller-off", func(g router.SessionGuardrails) bool { return g.CycleKillerDisabled }, "CycleKillerDisabled=true"},
		{"cyclekiller-on", func(g router.SessionGuardrails) bool { return !g.CycleKillerDisabled }, "CycleKillerDisabled=false"},
		{"shield-off", func(g router.SessionGuardrails) bool { return g.ShieldDisabled }, "ShieldDisabled=true"},
		{"shield-on", func(g router.SessionGuardrails) bool { return !g.ShieldDisabled }, "ShieldDisabled=false"},
		{"raw-on", func(g router.SessionGuardrails) bool { return g.RawModeEnabled }, "RawModeEnabled=true"},
		{"raw-off", func(g router.SessionGuardrails) bool { return !g.RawModeEnabled }, "RawModeEnabled=false"},
		{"fairydust-off", func(g router.SessionGuardrails) bool { return g.FairyDustDisabled }, "FairyDustDisabled=true"},
		{"fairydust-on", func(g router.SessionGuardrails) bool { return !g.FairyDustDisabled }, "FairyDustDisabled=false"},
	}

	for _, tc := range testCases {
		payload := fmt.Sprintf(`{"model":"auto","messages":[{"role":"user","content":"@nacho:%s do something"}]}`, tc.directive)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(payload)))
		req.Header.Set("x-session-id", sessionKey)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("directive %s failed with code %d", tc.directive, w.Code)
		}
		g := srv.sessionTracker.GetGuardrails(sessionKey)
		if !tc.checkAssert(g) {
			t.Errorf("after directive %s, expected %s, got guardrails %+v", tc.directive, tc.desc, g)
		}
	}
}

// TestPipeline_ReadSlap_PromptOverrideAndMaxSlapsCap tests lines 246–294 and 452–467 in pipeline.go:
// 1. Idle turn without tool progress triggers Read Slap.
// 2. Custom PromptOverride is injected into the payload.
// 3. When kickstartCount reaches MaxSlaps, ForcedTier is set to "cloud".
func TestPipeline_ReadSlap_PromptOverrideAndMaxSlapsCap(t *testing.T) {
	var capturedBodies [][]byte
	customPrompt := "CUSTOM_WAKE_UP_AGENT_NOW"

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Local Tier",
				Model:    "local-model",
				Provider: "openrouter",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Cloud Tier",
			Model:    "cloud-model",
			Provider: "openrouter",
		},
		Kickstart: contract.KickstartConfig{
			ReadSlap: contract.ReadSlapConfig{
				Threshold:      1,
				MaxSlaps:       2,
				PromptOverride: customPrompt,
			},
		},
	}

	srv, mockUpstream := setupMockPipelineServer(t, cfg, &capturedBodies)
	defer mockUpstream.Close()

	sessionKey := "sess-read-slap-test"

	// Turn 1: Idle turn (no tool progress).
	// Threshold=1 -> RecordKickstartState flags Read Slap.
	idlePayload := `{"model":"auto","messages":[{"role":"user","content":"just thinking..."}]}`
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(idlePayload)))
	req1.Header.Set("x-session-id", sessionKey)
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("turn 1 failed: %d", w1.Code)
	}

	// Verify custom prompt was injected
	if len(capturedBodies) == 0 {
		t.Fatalf("expected captured body on turn 1")
	}
	lastBody := string(capturedBodies[len(capturedBodies)-1])
	if !strings.Contains(lastBody, customPrompt) {
		t.Errorf("expected injected body to contain custom prompt %q, got: %s", customPrompt, lastBody)
	}

	// Turn 2: Second idle turn -> kickstart count reaches 2 (>= MaxSlaps 2).
	// Read Slap cap must force-escalate to "cloud" tier!
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(idlePayload)))
	req2.Header.Set("x-session-id", sessionKey)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("turn 2 failed: %d", w2.Code)
	}
	targetModel := w2.Header().Get("x-nacho-target-model")
	if targetModel != "cloud-model" {
		t.Errorf("expected max slaps cap to force-escalate to cloud-model, got %q", targetModel)
	}
}

// TestPipeline_FairyDust_PriorityAndCustomPrompt tests lines 296–346, 388–407, and 469–483 in pipeline.go:
// 1. Two Fairy Dust candidates qualify on a write turn.
// 2. Candidate with higher priority wins.
// 3. Winning candidate's custom checkpoint prompt is injected into upstream body.
// 4. TargetTier is overridden with Fairy Dust tier name and model.
func TestPipeline_FairyDust_PriorityAndCustomPrompt(t *testing.T) {
	var capturedBodies [][]byte
	trueVal := true

	cfg := &contract.Config{
		DefaultTier: contract.Tier{
			Name:     "Default",
			Model:    "standard-model",
			Provider: "openrouter",
		},
		FairyDust: contract.FairyDustConfig{
			Enabled: &trueVal,
			Entries: []contract.FairyDustEntry{
				{
					Name:      "low_prio",
					Model:     "mid-tier-model",
					Frequency: 1,
					Priority:  5,
					Prompt:    "FD_PROMPT_LOW",
				},
				{
					Name:      "high_prio",
					Model:     "frontier-eval-model",
					Frequency: 1,
					Priority:  20,
					Prompt:    "FD_PROMPT_HIGH",
				},
			},
		},
		Kickstart: contract.KickstartConfig{
			ReadSlap: contract.ReadSlapConfig{
				CustomWriteTools: []string{"write_to_file"},
			},
		},
	}

	srv, mockUpstream := setupMockPipelineServer(t, cfg, &capturedBodies)
	defer mockUpstream.Close()

	sessionKey := "sess-fairydust-prio-test"

	// Send write progress turn
	writePayload := `{"messages":[{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"write_to_file","arguments":"{\"path\":\"main.go\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"written"},{"role":"user","content":"proceed"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(writePayload)))
	req.Header.Set("x-session-id", sessionKey)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected code %d: %s", w.Code, w.Body.String())
	}

	// Verify higher priority model won
	routerTier := w.Header().Get("x-nacho-router-tier")
	if !strings.Contains(routerTier, "Fairy Dust: high_prio") {
		t.Errorf("expected winning tier to be Fairy Dust: high_prio, got %q", routerTier)
	}
	targetModel := w.Header().Get("x-nacho-target-model")
	if targetModel != "frontier-eval-model" {
		t.Errorf("expected frontier-eval-model, got %q", targetModel)
	}

	// Verify winning custom prompt was injected
	if len(capturedBodies) == 0 {
		t.Fatalf("expected captured body")
	}
	bodyStr := string(capturedBodies[len(capturedBodies)-1])
	if !strings.Contains(bodyStr, "FD_PROMPT_HIGH") {
		t.Errorf("expected body to contain 'FD_PROMPT_HIGH', got: %s", bodyStr)
	}
}

// mockOpenCircuitProvider implements LLMProvider, CircuitBreakerProvider, and AuthProvider.
// Its circuit breaker is always OPEN (AllowRequest returns false).
type mockOpenCircuitProvider struct {
	id string
	cb *provider.CircuitBreaker
}

func newMockOpenCircuitProvider(id string) *mockOpenCircuitProvider {
	cb := provider.NewCircuitBreaker(1, time.Hour)
	cb.RecordFailure() // Trips state to OPEN
	return &mockOpenCircuitProvider{id: id, cb: cb}
}

func (p *mockOpenCircuitProvider) ID() string                               { return p.id }
func (p *mockOpenCircuitProvider) Name() string                             { return "Mock Open Circuit Provider" }
func (p *mockOpenCircuitProvider) BaseURL() string                          { return "http://127.0.0.1:9999" }
func (p *mockOpenCircuitProvider) IsLocal() bool                            { return false }
func (p *mockOpenCircuitProvider) GetAPIKey() string                        { return "dummy" }
func (p *mockOpenCircuitProvider) CircuitBreaker() *provider.CircuitBreaker { return p.cb }

// TestPipeline_ForcedProviderCircuitBlocked tests lines 409–427 in pipeline.go:
// When an explicit forced directive is used (@nacho:tier or @nacho:model) and the provider's
// circuit breaker is OPEN, request is immediately blocked with 503 without fallback.
func TestPipeline_ForcedProviderCircuitBlocked(t *testing.T) {
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "CloudTier",
				Model:    "cloud-model",
				Provider: "open_circuit_prov",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "FallbackTier",
			Model:    "fallback-model",
			Provider: "open_circuit_prov",
		},
	}

	reg := provider.NewRegistry()
	reg.Register(newMockOpenCircuitProvider("open_circuit_prov"))

	eval, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		t.Fatalf("failed to create evaluator: %v", err)
	}

	srv := NewServerWithTelemetryAndRegistry(
		cfg,
		eval,
		router.NewClassifier(),
		router.NewSanitizer(),
		telemetry.NewPricingOracle(),
		telemetry.NewStatsTracker(10),
		reg,
		nil,
	)

	// 1. Non-streaming forced directive
	payloadJSON := `{"model":"auto","messages":[{"role":"user","content":"@nacho:tier=\"CloudTier\" solve problem"}]}`
	reqJSON := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(payloadJSON)))
	wJSON := httptest.NewRecorder()
	srv.ServeHTTP(wJSON, reqJSON)

	if wJSON.Code != http.StatusOK {
		// Note: RenderCircuitBlocked returns 200 with chat alert payload according to wire contract
		t.Logf("Status code: %d", wJSON.Code)
	}
	respStr := wJSON.Body.String()
	if !strings.Contains(respStr, "Circuit Alert") || !strings.Contains(respStr, "Circuit Breaker: OPEN") {
		t.Errorf("expected circuit breaker alert in response body, got: %s", respStr)
	}

	// 2. Streaming forced directive
	payloadSSE := `{"model":"auto","stream":true,"messages":[{"role":"user","content":"@nacho:tier=\"CloudTier\" solve stream"}]}`
	reqSSE := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(payloadSSE)))
	wSSE := httptest.NewRecorder()
	srv.ServeHTTP(wSSE, reqSSE)

	sseBody := wSSE.Body.String()
	if !strings.Contains(sseBody, "Circuit Alert") || !strings.Contains(sseBody, "Circuit Breaker: OPEN") {
		t.Errorf("expected SSE circuit breaker alert, got: %s", sseBody)
	}
}

func TestPipeline_FairyDust_ReasoningEffort_DirectAssignment(t *testing.T) {
	s := &Server{}
	cfg := &contract.Config{
		FairyDust: contract.FairyDustConfig{
			Entries: []contract.FairyDustEntry{
				{
					Name:            "audit-high",
					Model:           "glm-5.3-flash",
					Provider:        "openrouter",
					ReasoningEffort: "high",
				},
				{
					Name:            "audit-native-default",
					Model:           "glm-5.3-flash",
					Provider:        "openrouter",
					ReasoningEffort: "",
				},
			},
		},
	}

	routineTier := contract.Tier{
		Name:            "Routine Tier",
		Model:           "cheap-model",
		Provider:        "local",
		ReasoningEffort: "low",
	}

	// 1. Explicit effort in FairyDustEntry overrides routine tier
	reqCtx1 := &contract.RequestContext{
		FairyDusted:      true,
		FairyDustEntry:   "audit-high",
		FairyDustCount:   1,
	}
	out1 := s.applyFairyDustTierOverride(reqCtx1, routineTier, cfg)
	if out1.ReasoningEffort != "high" {
		t.Errorf("expected ReasoningEffort 'high', got %q", out1.ReasoningEffort)
	}
	if out1.Model != "glm-5.3-flash" {
		t.Errorf("expected model 'glm-5.3-flash', got %q", out1.Model)
	}

	// 2. Omitted effort in FairyDustEntry does NOT inherit routine tier's "low"
	reqCtx2 := &contract.RequestContext{
		FairyDusted:      true,
		FairyDustEntry:   "audit-native-default",
		FairyDustCount:   1,
	}
	out2 := s.applyFairyDustTierOverride(reqCtx2, routineTier, cfg)
	if out2.ReasoningEffort != "" {
		t.Errorf("expected ReasoningEffort to be empty (native default), got %q", out2.ReasoningEffort)
	}

	// 3. When not FairyDusted, routine tier is untouched
	reqCtx3 := &contract.RequestContext{
		FairyDusted: false,
	}
	out3 := s.applyFairyDustTierOverride(reqCtx3, routineTier, cfg)
	if out3.ReasoningEffort != "low" {
		t.Errorf("expected routine tier ReasoningEffort 'low' to be retained when not fairy dusted, got %q", out3.ReasoningEffort)
	}
}

