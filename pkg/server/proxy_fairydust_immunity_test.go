package server

import (
	"bytes"
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

// TestProxy_FairyDust_Immunity_CycleKillerAndNextTurnReset verifies:
// 1. On a normal turn (FairyDusted == false), Cycle Killer severs repetitive command tool calls.
// 2. On a turn where Fairy Dust triggers (FairyDusted == true), Cycle Killer is bypassed and
//    the stream is NOT severed despite heavy tool argument repetition.
// 3. On the immediate next turn on the same session (FairyDusted == false), Cycle Killer is
//    fully re-armed, and the exact same repetitive tool payload is actively severed.
func TestProxy_FairyDust_Immunity_CycleKillerAndNextTurnReset(t *testing.T) {
	var turnCounter atomic.Int32

	// Upstream mock that returns repetitive command tool calls for all turns
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turnCounter.Add(1)

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

		// 15 repetitive argument chunks that trip RepetitionThreshold: 3 on command lane
		for i := 0; i < 15; i++ {
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
		FairyDust: contract.FairyDustConfig{
			Enabled: &enabled,
			Entries: []contract.FairyDustEntry{
				{
					Name:          "Tactical Code Review",
					Model:         "frontier-supervisor",
					Provider:      "test_provider",
					Frequency:     1, // Every write turn triggers fairy dust
					MaxPerSession: 5,
					Priority:      100,
				},
			},
		},
		Providers: map[string]contract.ProviderConfig{
			"test_provider": {
				BaseURL: mockUpstream.URL,
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Workhorse Tier",
				Provider: "test_provider",
				Model:    "standard-worker",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Fallback",
			Provider: "test_provider",
			Model:    "standard-fallback",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	// Turn 1: Read-only turn (no write progress -> Fairy Dust does NOT trigger)
	turn1Body := `{
		"model": "standard-worker",
		"stream": true,
		"messages": [
			{"role": "user", "content": "read file"},
			{"role": "assistant", "tool_calls": [{"id": "r1", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\":\"main.go\"}"}}]},
			{"role": "tool", "tool_call_id": "r1", "content": "package main"}
		],
		"tools": [{"type": "function", "function": {"name": "execute_command"}}]
	}`

	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(turn1Body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Session-ID", "fd-test-session")
	w1 := httptest.NewRecorder()

	srv.ServeHTTP(w1, req1)
	resp1 := w1.Body.String()

	// Turn 1 should be severed by Cycle Killer because it is a normal worker turn (FairyDusted == false)
	if !strings.Contains(resp1, "tool repetition loop detected") {
		t.Fatalf("Expected Turn 1 to be severed by Cycle Killer, got:\n%s", resp1)
	}

	// Turn 2: Write progress turn (write_to_file executed -> Fairy Dust TRIGGERS!)
	turn2Body := `{
		"model": "standard-worker",
		"stream": true,
		"messages": [
			{"role": "user", "content": "write file"},
			{"role": "assistant", "tool_calls": [{"id": "w1", "type": "function", "function": {"name": "write_to_file", "arguments": "{\"path\":\"a.go\",\"content\":\"package main\"}"}}]},
			{"role": "tool", "tool_call_id": "w1", "content": "File saved successfully"}
		],
		"tools": [{"type": "function", "function": {"name": "execute_command"}}]
	}`

	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(turn2Body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Session-ID", "fd-test-session")
	w2 := httptest.NewRecorder()

	srv.ServeHTTP(w2, req2)
	resp2 := w2.Body.String()

	// Turn 2 MUST have immunity: NO cycle breaker error, stream finishes cleanly with [DONE]
	if strings.Contains(resp2, "tool repetition loop detected") || strings.Contains(resp2, "cycle_killer_error") {
		t.Fatalf("Turn 2 (Fairy Dust) should have immunity but was severed with cycle killer:\n%s", resp2)
	}
	if !strings.Contains(resp2, "[DONE]") {
		t.Fatalf("Turn 2 (Fairy Dust) expected clean completion with [DONE], got:\n%s", resp2)
	}

	// Turn 3: Subsequent turn (read turn -> Fairy Dust does NOT trigger, RESETS TO NORMAL!)
	turn3Body := `{
		"model": "standard-worker",
		"stream": true,
		"messages": [
			{"role": "user", "content": "inspect status"},
			{"role": "assistant", "tool_calls": [{"id": "r2", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\":\"a.go\"}"}}]},
			{"role": "tool", "tool_call_id": "r2", "content": "package main"}
		],
		"tools": [{"type": "function", "function": {"name": "execute_command"}}]
	}`

	req3 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(turn3Body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Session-ID", "fd-test-session")
	w3 := httptest.NewRecorder()

	srv.ServeHTTP(w3, req3)
	resp3 := w3.Body.String()

	// Turn 3 MUST be re-armed: Cycle Killer MUST sever the repetitive stream
	if !strings.Contains(resp3, "tool repetition loop detected") {
		t.Fatalf("Turn 3 (Next Turn after Fairy Dust) MUST re-arm Cycle Killer and sever the stream, but was not severed:\n%s", resp3)
	}
}

// TestProxy_FairyDust_Immunity_KickstartAndShieldBypassed verifies that
// Kickstart prompts and Agent Shield interventions are completely suppressed during Fairy Dust turns.
func TestProxy_FairyDust_Immunity_KickstartAndShieldBypassed(t *testing.T) {
	var capturedUpstreamBody []byte

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUpstreamBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		// Model ends with a question phrase that would normally trigger Agent Shield
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Audit complete. Would you like me to proceed with more refactoring?\"}}]}\n\n"))
		flusher.Flush()
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer mockUpstream.Close()

	enabled := true
	cfg := &contract.Config{
		Port: 8000,
		AgentShield: contract.AgentShieldConfig{
			Enabled:            &enabled,
			QuestionHeuristics: []string{"would you like"},
		},
		CycleKiller: contract.CycleBreakerConfig{
			Enabled:            &enabled,
			KickstartThreshold: 1, // Aggressive kickstart
		},
		FairyDust: contract.FairyDustConfig{
			Enabled: &enabled,
			Entries: []contract.FairyDustEntry{
				{
					Name:          "Tactical Code Review",
					Model:         "frontier-supervisor",
					Provider:      "test_provider",
					Frequency:     1, // Every write turn is fairy dust
					MaxPerSession: 5,
				},
			},
		},
		Providers: map[string]contract.ProviderConfig{
			"test_provider": {
				BaseURL: mockUpstream.URL,
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Workhorse Tier",
				Provider: "test_provider",
				Model:    "standard-worker",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Fallback",
			Provider: "test_provider",
			Model:    "standard-fallback",
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	// Valid write progress turn to trigger Fairy Dust
	body := `{
		"model": "standard-worker",
		"stream": true,
		"messages": [
			{"role": "user", "content": "audit this code"},
			{"role": "assistant", "tool_calls": [{"id": "w1", "type": "function", "function": {"name": "write_to_file", "arguments": "{\"path\":\"a.go\",\"content\":\"package main\"}"}}]},
			{"role": "tool", "tool_call_id": "w1", "content": "File saved successfully"}
		],
		"tools": [{"type": "function", "function": {"name": "write_to_file"}}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", "fd-test-session-ks")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	resp := w.Body.String()

	// 1. Verify Kickstart prompt was NOT injected into upstream body
	if strings.Contains(string(capturedUpstreamBody), contract.DefaultKickstartPrompt) {
		t.Fatalf("Expected Kickstart prompt to be suppressed on Fairy Dust turn, but found in upstream body")
	}

	// 2. Verify Agent Shield was NOT triggered (clean response without shield intervention)
	if strings.Contains(resp, "shield_fallback") || strings.Contains(resp, "Agent Shield") {
		t.Fatalf("Expected Agent Shield to be suppressed on Fairy Dust turn, but got:\n%s", resp)
	}
	if !strings.Contains(resp, "Would you like me to proceed") {
		t.Fatalf("Expected original content to pass through without shield corruption, got:\n%s", resp)
	}
}
