package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/store"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// nonFlusherResponseWriter implements http.ResponseWriter without http.Flusher
type nonFlusherResponseWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (n *nonFlusherResponseWriter) Header() http.Header {
	if n.header == nil {
		n.header = make(http.Header)
	}
	return n.header
}

func (n *nonFlusherResponseWriter) Write(b []byte) (int, error) {
	return n.body.Write(b)
}

func (n *nonFlusherResponseWriter) WriteHeader(statusCode int) {
	n.code = statusCode
}

func TestServer_APIEvents_EdgeCases(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// 1. Non-flusher writer should return 500 Streaming unsupported
	nonFlusher := &nonFlusherResponseWriter{}
	req := httptest.NewRequest(http.MethodGet, contract.PathAPIEvents, nil)
	srv.handleAPIEvents(nonFlusher, req)
	if nonFlusher.code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-flusher, got %d", nonFlusher.code)
	}

	// 2. Server with nil eventBroker returns immediately after headers
	srvNilBroker := NewServer(&contract.Config{Port: 8000}, nil, nil, nil)
	wNil := httptest.NewRecorder()
	srvNilBroker.handleAPIEvents(wNil, req)
	if wNil.Code != http.StatusOK {
		t.Errorf("expected 200 from nil eventBroker, got %d", wNil.Code)
	}

	// 3. Client disconnect context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	reqCtx := req.WithContext(ctx)
	wCtx := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.handleAPIEvents(wCtx, reqCtx)
		close(done)
	}()

	// Cancel context to trigger exit
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleAPIEvents did not exit on ctx cancel")
	}

	// 4. Event delivery and broker unsubscribe
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	reqCtx2 := req.WithContext(ctx2)
	wCtx2 := httptest.NewRecorder()

	done2 := make(chan struct{})
	go func() {
		srv.handleAPIEvents(wCtx2, reqCtx2)
		close(done2)
	}()

	time.Sleep(20 * time.Millisecond)
	srv.eventBroker.PublishJSON(telemetry.EventStats, map[string]string{"foo": "bar"})
	time.Sleep(20 * time.Millisecond)
	cancel2()
	<-done2
}

func TestServer_APICircuits_And_Pricing_NilProviders(t *testing.T) {
	// Server with completely nil components
	srvBare := NewServer(&contract.Config{Port: 8000}, nil, nil, nil)

	// 1. handleAPICircuits with nil registry
	reqCircuits := httptest.NewRequest(http.MethodGet, contract.PathAPICircuits, nil)
	wCircuits := httptest.NewRecorder()
	srvBare.handleAPICircuits(wCircuits, reqCircuits)
	if wCircuits.Code != http.StatusOK {
		t.Errorf("expected 200 for circuits with nil registry, got %d", wCircuits.Code)
	}

	// 2. handleAPICircuitsReset with nil registry and nil broker
	reqReset := httptest.NewRequest(http.MethodPost, contract.PathAPICircuitsReset+"?provider=openai", nil)
	wReset := httptest.NewRecorder()
	srvBare.handleAPICircuitsReset(wReset, reqReset)
	if wReset.Code != http.StatusOK {
		t.Errorf("expected 200 for circuits reset with nil registry, got %d", wReset.Code)
	}

	// 3. handleAPIPricing with nil oracle
	reqPricing := httptest.NewRequest(http.MethodGet, contract.PathAPIPricing, nil)
	wPricing := httptest.NewRecorder()
	srvBare.handleAPIPricing(wPricing, reqPricing)
	if wPricing.Code != http.StatusOK {
		t.Errorf("expected 200 for pricing with nil oracle, got %d", wPricing.Code)
	}
}

func TestServer_APIStatsRecalculate_ErrorAndBranches(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// 1. Invalid traffic log path -> returns 500
	srv.SetTrafficLogPath("\x00invalid-path")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stats/recalculate", nil)
	w := httptest.NewRecorder()
	srv.handleAPIStatsRecalculate(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for invalid path, got %d", w.Code)
	}

	// 2. Valid path with ringBuffer > 500 items branch
	tempDir, err := os.MkdirTemp("", "recalc_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "traffic.jsonl")
	var fileRecords []telemetry.TurnRecord
	for i := 0; i < 550; i++ {
		fileRecords = append(fileRecords, telemetry.TurnRecord{
			RequestID: "req-1",
			Tokens:    100,
			IsLocal:   true,
		})
	}
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("failed to create traffic log: %v", err)
	}
	for _, rec := range fileRecords {
		data, _ := json.Marshal(rec)
		_, _ = f.Write(append(data, '\n'))
	}
	_ = f.Close()

	srv.SetTrafficLogPath(logPath)
	w2 := httptest.NewRecorder()
	srv.handleAPIStatsRecalculate(w2, req)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 for recalculation with 550 records, got %d", w2.Code)
	}
}

func TestServer_ResolveCycleBreaker_And_InjectCorrection(t *testing.T) {
	t.Run("resolveCycleBreaker various configurations", func(t *testing.T) {
		trueVal := true
		falseVal := false

		// 1. Disabled via global config
		cfgDisabled := &contract.Config{
			CycleKiller: contract.CycleBreakerConfig{
				Enabled: &falseVal,
			},
		}
		if cb := resolveCycleBreaker(contract.Tier{}, cfgDisabled); cb != nil {
			t.Errorf("expected nil cycle breaker when disabled globally")
		}

		// 2. Disabled via legacy CycleBreaker name
		cfgLegacyDisabled := &contract.Config{
			CycleBreaker: contract.CycleBreakerConfig{
				Enabled: &falseVal,
			},
		}
		if cb := resolveCycleBreaker(contract.Tier{}, cfgLegacyDisabled); cb != nil {
			t.Errorf("expected nil cycle breaker when legacy name is disabled")
		}

		// 3. Tier level overrides
		tierCustom := contract.Tier{
			CycleKiller: &contract.CycleBreakerConfig{
				Enabled:             &trueVal,
				MaxProseTokens:      400,
				RepetitionWindow:    50,
				RepetitionThreshold: 3,
				MaxRetries:          2,
				CorrectionPrompt:    "Custom correction prompt",
			},
		}
		cb := resolveCycleBreaker(tierCustom, &contract.Config{})
		if cb == nil {
			t.Fatalf("expected non-nil cycle breaker for custom tier")
		}
	})

	t.Run("injectCorrectionPrompt edge cases", func(t *testing.T) {
		// 1. Empty prompt fallback to default
		body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
		out := injectCorrectionPrompt(body, "")
		if !strings.Contains(string(out), "SYSTEM OVERRIDE") {
			t.Errorf("expected default correction prompt injected, got: %s", string(out))
		}

		// 2. Invalid JSON returns unmodified body
		badBody := []byte(`{invalid json`)
		outBad := injectCorrectionPrompt(badBody, "prompt")
		if string(outBad) != string(badBody) {
			t.Errorf("expected unmodified body for bad JSON")
		}

		// 3. JSON where messages is not array returns unmodified body
		noArray := []byte(`{"messages":"not an array"}`)
		outNoArr := injectCorrectionPrompt(noArray, "prompt")
		if string(outNoArr) != string(noArray) {
			t.Errorf("expected unmodified body when messages is not array")
		}
	})
}

func TestServer_StreamNormalizer_GetUsage_EdgeCases(t *testing.T) {
	// 1. Captured usage
	sn := NewStreamNormalizer(io.NopCloser(strings.NewReader("")))
	sn.capturedUsage = StreamUsage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}
	sn.hasUsage = true
	usage, ok := sn.GetUsage()
	if !ok || usage.TotalTokens != 30 {
		t.Errorf("expected captured usage with 30 total tokens, got %v, ok=%v", usage, ok)
	}

	// 2. Fallback estimated usage with < 4 chars (tokens == 1)
	sn2 := NewStreamNormalizer(io.NopCloser(strings.NewReader("")))
	sn2.hasUsage = false
	sn2.emittedCompletionChars = 2
	usage2, ok2 := sn2.GetUsage()
	if ok2 || usage2.CompletionTokens != 1 {
		t.Errorf("expected estimated 1 token for 2 chars, got %v, ok=%v", usage2, ok2)
	}

	// 3. Fallback estimated usage with 20 chars (tokens == 5)
	sn3 := NewStreamNormalizer(io.NopCloser(strings.NewReader("")))
	sn3.hasUsage = false
	sn3.emittedCompletionChars = 20
	usage3, ok3 := sn3.GetUsage()
	if ok3 || usage3.CompletionTokens != 5 {
		t.Errorf("expected estimated 5 tokens for 20 chars, got %v, ok=%v", usage3, ok3)
	}

	// 4. Zero chars emitted and no captured usage
	sn4 := NewStreamNormalizer(io.NopCloser(strings.NewReader("")))
	sn4.hasUsage = false
	sn4.emittedCompletionChars = 0
	usage4, ok4 := sn4.GetUsage()
	if ok4 || usage4.TotalTokens != 0 {
		t.Errorf("expected 0 tokens when zero chars emitted, got %v, ok=%v", usage4, ok4)
	}
}

func TestServer_ServeHTTP_HealthAndStatsResetEdgeCases(t *testing.T) {
	srv, _, broker := setupTestServer(t)

	// 1. Health with non-zero startTime (uptime populated)
	srv.startTime = time.Now().Add(-10 * time.Minute)
	reqHealth := httptest.NewRequest(http.MethodGet, contract.PathHealth, nil)
	wHealth := httptest.NewRecorder()
	srv.ServeHTTP(wHealth, reqHealth)
	if wHealth.Code != http.StatusOK || !strings.Contains(wHealth.Body.String(), "uptime") {
		t.Errorf("expected 200 with uptime in response, got %s", wHealth.Body.String())
	}

	// 2. Stats reset on server with nil tracker and nil diskStore
	srvNil := NewServer(&contract.Config{Port: 8000}, nil, nil, nil)
	reqReset := httptest.NewRequest(http.MethodPost, "/api/v1/stats/reset", nil)
	wReset := httptest.NewRecorder()
	srvNil.handleAPIStatsReset(wReset, reqReset)
	if wReset.Code != http.StatusOK {
		t.Errorf("expected 200 for reset on nil tracker, got %d", wReset.Code)
	}

	// 3. Events channel close branch
	reqEvents := httptest.NewRequest(http.MethodGet, contract.PathAPIEvents, nil)
	wEvents := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		srv.handleAPIEvents(wEvents, reqEvents)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	broker.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleAPIEvents did not terminate on broker channel close")
	}
}

func TestExtractSessionKey(t *testing.T) {
	// 1. x-session-id header takes highest precedence
	r1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r1.Header.Set("x-session-id", "sess-12345")
	r1.Header.Set("session-id", "sess-lower-priority")
	r1.Header.Set("X-Forwarded-For", "192.168.1.10")
	r1.RemoteAddr = "10.0.0.1:54321"
	if got := extractSessionKey(r1); got != "sess-12345" {
		t.Errorf("expected sess-12345, got %s", got)
	}

	// 2. session-id header
	r2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r2.Header.Set("session-id", "sess-secondary")
	r2.RemoteAddr = "10.0.0.1:54321"
	if got := extractSessionKey(r2); got != "sess-secondary" {
		t.Errorf("expected sess-secondary, got %s", got)
	}

	// 3. X-Forwarded-For takes precedence over RemoteAddr
	r3 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r3.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
	r3.RemoteAddr = "10.0.0.1:54321"
	if got := extractSessionKey(r3); got != "203.0.113.195" {
		t.Errorf("expected 203.0.113.195, got %s", got)
	}

	// 4. X-Real-IP
	r4 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r4.Header.Set("X-Real-IP", "198.51.100.42")
	r4.RemoteAddr = "10.0.0.1:54321"
	if got := extractSessionKey(r4); got != "198.51.100.42" {
		t.Errorf("expected 198.51.100.42, got %s", got)
	}

	// 5. RemoteAddr IPv4 with ephemeral port stripped
	r5 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r5.RemoteAddr = "100.98.231.104:65143"
	if got := extractSessionKey(r5); got != "100.98.231.104" {
		t.Errorf("expected 100.98.231.104, got %s", got)
	}

	// 6. RemoteAddr IPv6 with ephemeral port stripped
	r6 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r6.RemoteAddr = "[2001:db8::1]:54321"
	if got := extractSessionKey(r6); got != "2001:db8::1" {
		t.Errorf("expected 2001:db8::1, got %s", got)
	}

	// 7. Bare host without port
	r7 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r7.RemoteAddr = "127.0.0.1"
	if got := extractSessionKey(r7); got != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %s", got)
	}
}

func TestServer_ServeHTTP_FullRoutingBranches(t *testing.T) {
	srvNoAuth := NewServer(&contract.Config{Port: 8000}, nil, nil, nil)
	srvNoAuth.tracker = telemetry.NewStatsTracker(10)

	// 1. /v1/health
	reqV1Health := httptest.NewRequest(http.MethodGet, contract.PathV1Health, nil)
	wV1Health := httptest.NewRecorder()
	srvNoAuth.ServeHTTP(wV1Health, reqV1Health)
	if wV1Health.Code != http.StatusOK {
		t.Errorf("expected 200 for /v1/health, got %d", wV1Health.Code)
	}

	// 2. /v1/models
	reqModels := httptest.NewRequest(http.MethodGet, contract.PathModels, nil)
	wModels := httptest.NewRecorder()
	srvNoAuth.ServeHTTP(wModels, reqModels)
	if wModels.Code != http.StatusOK {
		t.Errorf("expected 200 for /v1/models, got %d", wModels.Code)
	}

	// 3. /api/v1/nonexistent -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	w404 := httptest.NewRecorder()
	srvNoAuth.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown api path, got %d", w404.Code)
	}

	// 4. Unknown root path -> 404
	reqRoot404 := httptest.NewRequest(http.MethodGet, "/some/random/endpoint", nil)
	wRoot404 := httptest.NewRecorder()
	srvNoAuth.ServeHTTP(wRoot404, reqRoot404)
	if wRoot404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown root path, got %d", wRoot404.Code)
	}

	// 5. Auth rejection on protected route
	srvAuth := NewServer(&contract.Config{
		Port:      8000,
		AuthToken: "secret-token",
	}, nil, nil, nil)
	reqAuthFail := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, nil)
	wAuthFail := httptest.NewRecorder()
	srvAuth.ServeHTTP(wAuthFail, reqAuthFail)
	if wAuthFail.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthorized request, got %d", wAuthFail.Code)
	}

	// 6. Auth pass via Bearer
	reqAuthPass := httptest.NewRequest(http.MethodGet, contract.PathStats, nil)
	reqAuthPass.Header.Set("Authorization", "Bearer secret-token")
	wAuthPass := httptest.NewRecorder()
	srvAuth.tracker = telemetry.NewStatsTracker(10)
	srvAuth.ServeHTTP(wAuthPass, reqAuthPass)
	if wAuthPass.Code != http.StatusOK {
		t.Errorf("expected 200 for authorized stats request, got %d", wAuthPass.Code)
	}
}

func TestServer_ResolveCycleKillParams_Branches(t *testing.T) {
	// 1. Nil config / zero values -> defaults
	srv := &Server{}
	cooldown, floor := srv.resolveCycleKillParams()
	if cooldown != router.DefaultModelCooldown || floor != 3 {
		t.Errorf("expected %v,3 for nil config, got %v,%d", router.DefaultModelCooldown, cooldown, floor)
	}

	// 2. CycleBreaker fallback when CycleKiller is not set
	srv2 := NewServer(&contract.Config{
		CycleBreaker: contract.CycleBreakerConfig{
			ModelCooldownSeconds: 45,
			RetryFloor:           4,
		},
	}, nil, nil, nil)
	cooldown2, floor2 := srv2.resolveCycleKillParams()
	if cooldown2 != 45*time.Second || floor2 != 4 {
		t.Errorf("expected 45s,4 from CycleBreaker fallback, got %v,%d", cooldown2, floor2)
	}

	// 3. CycleKiller precedence
	srv3 := NewServer(&contract.Config{
		CycleKiller: contract.CycleBreakerConfig{
			ModelCooldownSeconds: 90,
			RetryFloor:           5,
		},
		CycleBreaker: contract.CycleBreakerConfig{
			ModelCooldownSeconds: 45,
			RetryFloor:           3,
		},
	}, nil, nil, nil)
	cooldown3, floor3 := srv3.resolveCycleKillParams()
	if cooldown3 != 90*time.Second || floor3 != 5 {
		t.Errorf("expected 90s,5 from CycleKiller precedence, got %v,%d", cooldown3, floor3)
	}
}

func TestServer_ServeHTTP_FairyDust_Kickstart_And_Directives(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cmpl-test","choices":[{"index":0,"message":{"role":"assistant","content":"mock response"}}]}`))
	}))
	defer mockUpstream.Close()

	// 1. Non-POST method to chat completions -> 405
	srv, _, _ := setupTestServer(t)
	reqGet := httptest.NewRequest(http.MethodGet, contract.PathChatCompletions, nil)
	reqGet.Header.Set("Authorization", "Bearer test-secret-token")
	wGet := httptest.NewRecorder()
	srv.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /v1/chat/completions, got %d", wGet.Code)
	}

	// 2. Meta directive @nacho:help
	reqHelp := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"messages":[{"role":"user","content":"@nacho:help"}]}`))
	reqHelp.Header.Set("Authorization", "Bearer test-secret-token")
	wHelp := httptest.NewRecorder()
	srv.ServeHTTP(wHelp, reqHelp)
	if wHelp.Code != http.StatusOK || !strings.Contains(wHelp.Body.String(), "Nacho Flow") {
		t.Errorf("expected 200 with help content, got code=%d body=%s", wHelp.Code, wHelp.Body.String())
	}

	// 3. Disabled in-prompt directives
	falseVal := false
	cfg := *srv.GetConfig()
	cfg.Router.EnableInPromptDirectives = &falseVal
	cfg.Providers = map[string]contract.ProviderConfig{
		"ollama":     {BaseURL: mockUpstream.URL},
		"openrouter": {BaseURL: mockUpstream.URL, APIKey: "test"},
	}
	reg := provider.NewRegistry()
	reg.Register(provider.NewGenericLLMProvider("ollama", cfg.Providers["ollama"]))
	reg.Register(provider.NewGenericLLMProvider("openrouter", cfg.Providers["openrouter"]))
	srvDisabledDirectives := NewServerWithTelemetryAndRegistry(&cfg, srv.GetEvaluator(), srv.classifier, srv.sanitizer, srv.oracle, srv.tracker, reg, nil)
	reqDisabled := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"messages":[{"role":"user","content":"@nacho:tier3 test"}]}`))
	reqDisabled.Header.Set("Authorization", "Bearer test-secret-token")
	wDisabled := httptest.NewRecorder()
	srvDisabledDirectives.ServeHTTP(wDisabled, reqDisabled)
	if wDisabled.Code != http.StatusOK {
		t.Errorf("expected 200 for normal completion with disabled directives, got %d: %s", wDisabled.Code, wDisabled.Body.String())
	}

	// 4. Fairy Dust triggered in ServeHTTP
	trueVal := true
	cfgFD := *srv.GetConfig()
	cfgFD.Providers = cfg.Providers
	cfgFD.FairyDust = contract.FairyDustConfig{
		Enabled: &trueVal,
		Entries: []contract.FairyDustEntry{
			{
				Name:          "tactical_review",
				Model:         "deepseek-r1",
				Frequency:     1,
				Priority:      10,
				MaxPerSession: 5,
			},
		},
	}
	cfgFD.CycleKiller = contract.CycleBreakerConfig{
		KickstartWriteTools: []string{"write_to_file", "replace_file_content"},
	}
	clfFD := router.NewClassifier()
	if c, ok := clfFD.(*router.RequestClassifier); ok {
		c.SetKickstartWriteTools([]string{"write_to_file", "replace_file_content"})
	}
	trackerFD := telemetry.NewStatsTracker(10)
	srvFD := NewServerWithTelemetryAndRegistry(&cfgFD, srv.GetEvaluator(), clfFD, srv.sanitizer, srv.oracle, trackerFD, reg, nil)
	// Send write tool payload
	writeBody := `{"messages":[{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"write_to_file","arguments":"{\"path\":\"test.go\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"file written successfully"},{"role":"user","content":"next step"}]}`
	reqFD := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(writeBody))
	reqFD.Header.Set("Authorization", "Bearer test-secret-token")
	reqFD.Header.Set("session-id", "test-fd-session")
	wFD := httptest.NewRecorder()
	srvFD.ServeHTTP(wFD, reqFD)
	if wFD.Code != http.StatusOK {
		t.Fatalf("expected 200 for fairy dusted turn, got %d: %s", wFD.Code, wFD.Body.String())
	}
	if targetModel := wFD.Header().Get("x-nacho-target-model"); targetModel != "deepseek-r1" {
		t.Errorf("expected target model deepseek-r1 for fairy dust, got %q", targetModel)
	}
	if routerTier := wFD.Header().Get("x-nacho-router-tier"); !strings.Contains(routerTier, "Fairy Dust: tactical_review") {
		t.Errorf("expected router tier to contain 'Fairy Dust: tactical_review', got %q", routerTier)
	}
	trackerFD.Flush()
	snapFD := trackerFD.GetStats()
	if snapFD.FairyDust.TotalTriggers != 1 {
		t.Errorf("expected FairyDust.TotalTriggers == 1, got %d", snapFD.FairyDust.TotalTriggers)
	}

	// 5. Kickstart max count cap escalation
	cfgKS := *srv.GetConfig()
	cfgKS.Providers = cfg.Providers
	cfgKS.CycleKiller = contract.CycleBreakerConfig{
		KickstartThreshold: 1,
		KickstartMaxCount:  1,
	}
	srvKS := NewServerWithTelemetryAndRegistry(&cfgKS, srv.GetEvaluator(), router.NewClassifier(), srv.sanitizer, srv.oracle, srv.tracker, reg, nil)
	noToolBody := `{"messages":[{"role":"user","content":"read something without tools"}]}`
	reqKS := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(noToolBody))
	reqKS.Header.Set("Authorization", "Bearer test-secret-token")
	reqKS.Header.Set("session-id", "test-ks-session")
	wKS := httptest.NewRecorder()
	srvKS.ServeHTTP(wKS, reqKS)
	if wKS.Code != http.StatusOK {
		t.Fatalf("expected 200 for kickstarted turn, got %d: %s", wKS.Code, wKS.Body.String())
	}
	if ksCount := srvKS.sessionTracker.GetKickstartCount("test-ks-session"); ksCount != 1 {
		t.Errorf("expected session kickstart count == 1, got %d", ksCount)
	}

	// 6. ServeHTTP /api/v1/deals
	reqDeals := httptest.NewRequest(http.MethodGet, contract.PathAPIDeals, nil)
	reqDeals.Header.Set("Authorization", "Bearer test-secret-token")
	wDeals := httptest.NewRecorder()
	srv.ServeHTTP(wDeals, reqDeals)
	if wDeals.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/v1/deals, got %d", wDeals.Code)
	}
}

func TestServer_CircuitBlocked_ForcedDirective(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	// Tripping open the openrouter circuit
	reg := srv.GetRegistry()
	p, _ := reg.Get("openrouter")
	if cbProv, ok := p.(provider.CircuitBreakerProvider); ok {
		cb := cbProv.CircuitBreaker()
		for i := 0; i < 10; i++ {
			cb.RecordFailure()
		}
	}

	// 1. Non-streaming forced directive when circuit is open
	reqNonStream := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"messages":[{"role":"user","content":"@nacho:tier=\"Tier 2: Cloud Coder\" do something"}]}`))
	reqNonStream.Header.Set("Authorization", "Bearer test-secret-token")
	wNonStream := httptest.NewRecorder()
	srv.ServeHTTP(wNonStream, reqNonStream)
	if wNonStream.Code != http.StatusOK || !strings.Contains(wNonStream.Body.String(), "Circuit Alert") {
		t.Errorf("expected circuit blocked response, got code=%d body=%s", wNonStream.Code, wNonStream.Body.String())
	}

	// 2. Streaming forced directive when circuit is open
	reqStream := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"stream":true,"messages":[{"role":"user","content":"@nacho:tier=\"Tier 2: Cloud Coder\" do something"}]}`))
	reqStream.Header.Set("Authorization", "Bearer test-secret-token")
	wStream := httptest.NewRecorder()
	srv.ServeHTTP(wStream, reqStream)
	if wStream.Code != http.StatusOK || !strings.Contains(wStream.Body.String(), "Circuit Alert") {
		t.Errorf("expected circuit blocked streaming response, got code=%d body=%s", wStream.Code, wStream.Body.String())
	}
}

func TestServer_EscalationBudget_Deescalation(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cmpl-test","choices":[{"index":0,"message":{"role":"assistant","content":"mock response"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port:      8000,
		AuthToken: "test-secret-token",
		Providers: map[string]contract.ProviderConfig{
			"mock_prov": {BaseURL: mockUpstream.URL},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Cloud Fallback Tier",
				Provider: "mock_prov",
				Model:    "fallback-model",
				When:     "false",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Expensive Tier",
			Provider: "mock_prov",
			Model:    "expensive-model",
		},
	}
	reg := provider.NewRegistry()
	reg.Register(provider.NewGenericLLMProvider("mock_prov", cfg.Providers["mock_prov"]))
	srv := NewServerWithTelemetryAndRegistry(cfg, nil, nil, nil, nil, nil, reg, nil)

	// Turns 1..3: Uses default tier, verifies standard expensive model routing on stuck retries
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"messages":[{"role":"user","content":"stuck repeating turn"}]}`))
		req.Header.Set("Authorization", "Bearer test-secret-token")
		req.Header.Set("session-id", "budget-sess")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on turn %d, got %d", i, w.Code)
		}
		if tier := w.Header().Get("x-nacho-router-tier"); tier != "Default Expensive Tier" {
			t.Errorf("expected turn %d router tier 'Default Expensive Tier', got %q", i, tier)
		}
		if model := w.Header().Get("x-nacho-target-model"); model != "expensive-model" {
			t.Errorf("expected turn %d target model 'expensive-model', got %q", i, model)
		}
	}

	// Turn 4: Exceeds MaxEscalationTurns=3, verifies automatic de-escalation to first cloud tier
	req4 := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"messages":[{"role":"user","content":"stuck repeating turn"}]}`))
	req4.Header.Set("Authorization", "Bearer test-secret-token")
	req4.Header.Set("session-id", "budget-sess")
	w4 := httptest.NewRecorder()
	srv.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200 on turn 4 de-escalation, got %d", w4.Code)
	}
	if tier := w4.Header().Get("x-nacho-router-tier"); tier != "Cloud Fallback Tier" {
		t.Errorf("expected turn 4 de-escalated router tier 'Cloud Fallback Tier', got %q", tier)
	}
	if model := w4.Header().Get("x-nacho-target-model"); model != "fallback-model" {
		t.Errorf("expected turn 4 de-escalated target model 'fallback-model', got %q", model)
	}
}

func TestServer_API_MethodNotAllowed_Branches(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// 1. POST /api/v1/circuits -> 405
	reqCircuitsPost := httptest.NewRequest(http.MethodPost, contract.PathAPICircuits, nil)
	reqCircuitsPost.Header.Set("Authorization", "Bearer test-secret-token")
	wCP := httptest.NewRecorder()
	srv.ServeHTTP(wCP, reqCircuitsPost)
	if wCP.Code != http.StatusMethodNotAllowed || !strings.Contains(wCP.Body.String(), "Method not allowed") {
		t.Errorf("expected 405 with 'Method not allowed' for POST /api/v1/circuits, got code=%d body=%s", wCP.Code, wCP.Body.String())
	}

	// 2. GET /api/v1/circuits/reset -> 405
	reqResetGet := httptest.NewRequest(http.MethodGet, contract.PathAPICircuitsReset, nil)
	reqResetGet.Header.Set("Authorization", "Bearer test-secret-token")
	wRG := httptest.NewRecorder()
	srv.ServeHTTP(wRG, reqResetGet)
	if wRG.Code != http.StatusMethodNotAllowed || !strings.Contains(wRG.Body.String(), "Method not allowed") {
		t.Errorf("expected 405 with 'Method not allowed' for GET /api/v1/circuits/reset, got code=%d body=%s", wRG.Code, wRG.Body.String())
	}

	// 3. POST /api/v1/pricing -> 405
	reqPricingPost := httptest.NewRequest(http.MethodPost, contract.PathAPIPricing, nil)
	reqPricingPost.Header.Set("Authorization", "Bearer test-secret-token")
	wPP := httptest.NewRecorder()
	srv.ServeHTTP(wPP, reqPricingPost)
	if wPP.Code != http.StatusMethodNotAllowed || !strings.Contains(wPP.Body.String(), "Method not allowed") {
		t.Errorf("expected 405 with 'Method not allowed' for POST /api/v1/pricing, got code=%d body=%s", wPP.Code, wPP.Body.String())
	}

	// 4. POST /api/v1/routes -> 405
	reqRoutesPost := httptest.NewRequest(http.MethodPost, contract.PathAPIRoutes, nil)
	reqRoutesPost.Header.Set("Authorization", "Bearer test-secret-token")
	wRP := httptest.NewRecorder()
	srv.ServeHTTP(wRP, reqRoutesPost)
	if wRP.Code != http.StatusMethodNotAllowed || !strings.Contains(wRP.Body.String(), "Method not allowed") {
		t.Errorf("expected 405 with 'Method not allowed' for POST /api/v1/routes, got code=%d body=%s", wRP.Code, wRP.Body.String())
	}

	// 5. GET /api/v1/stats/reset -> 405
	reqStatsResetGet := httptest.NewRequest(http.MethodGet, contract.PathAPIStatsReset, nil)
	reqStatsResetGet.Header.Set("Authorization", "Bearer test-secret-token")
	wSRG := httptest.NewRecorder()
	srv.ServeHTTP(wSRG, reqStatsResetGet)
	if wSRG.Code != http.StatusMethodNotAllowed || !strings.Contains(wSRG.Body.String(), "Method not allowed") {
		t.Errorf("expected 405 with 'Method not allowed' for GET /api/v1/stats/reset, got code=%d body=%s", wSRG.Code, wSRG.Body.String())
	}

	// 6. GET /api/v1/stats/recalculate -> 405
	reqStatsRecalcGet := httptest.NewRequest(http.MethodGet, contract.PathAPIStatsRecalculate, nil)
	reqStatsRecalcGet.Header.Set("Authorization", "Bearer test-secret-token")
	wSRecalcG := httptest.NewRecorder()
	srv.ServeHTTP(wSRecalcG, reqStatsRecalcGet)
	if wSRecalcG.Code != http.StatusMethodNotAllowed || !strings.Contains(wSRecalcG.Body.String(), "Method not allowed") {
		t.Errorf("expected 405 with 'Method not allowed' for GET /api/v1/stats/recalculate, got code=%d body=%s", wSRecalcG.Code, wSRecalcG.Body.String())
	}
}

func TestServer_DispatchTier_EdgeCases(t *testing.T) {
	// 1. Target provider URL invalid -> 500
	badURLProv := provider.NewGenericLLMProvider("bad_prov", contract.ProviderConfig{
		BaseURL: "http://invalid-url-with-control\x7f-chars",
	})
	reg := provider.NewRegistry()
	reg.Register(badURLProv)
	cfg := &contract.Config{
		Port:      8000,
		AuthToken: "test-secret-token",
		DefaultTier: contract.Tier{
			Name:     "Bad URL Tier",
			Provider: "bad_prov",
			Model:    "bad-model",
		},
	}
	srv := NewServerWithTelemetryAndRegistry(cfg, nil, nil, router.NewSanitizer(), nil, nil, reg, nil)
	reqBadURL := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"messages":[{"role":"user","content":"test"}]}`))
	reqBadURL.Header.Set("Authorization", "Bearer test-secret-token")
	wBadURL := httptest.NewRecorder()
	srv.ServeHTTP(wBadURL, reqBadURL)
	if wBadURL.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for invalid provider URL, got %d", wBadURL.Code)
	}

	// 2. Upstream provider returning 500 during fallback -> 502 Bad Gateway
	mock500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mock500.Close()

	prov500 := provider.NewGenericLLMProvider("prov500", contract.ProviderConfig{
		BaseURL: mock500.URL,
	})
	reg500 := provider.NewRegistry()
	reg500.Register(prov500)
	cfg500 := &contract.Config{
		Port:      8000,
		AuthToken: "test-secret-token",
		DefaultTier: contract.Tier{
			Name:     "500 Tier",
			Provider: "prov500",
			Model:    "500-model",
		},
	}
	srv500 := NewServerWithTelemetryAndRegistry(cfg500, nil, nil, router.NewSanitizer(), nil, nil, reg500, nil)
	req500 := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"messages":[{"role":"user","content":"test"}]}`))
	req500.Header.Set("Authorization", "Bearer test-secret-token")
	w500 := httptest.NewRecorder()
	srv500.ServeHTTP(w500, req500)
	if w500.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for upstream 500 without fallback, got %d", w500.Code)
	}
}

func TestServer_APIStats_EdgeCases(t *testing.T) {
	// 1. Stats reset with diskStore and eventBroker
	tmpDir := t.TempDir()
	ds, err := store.NewDiskStore(filepath.Join(tmpDir, "stats.json"))
	if err != nil {
		t.Fatalf("failed to create disk store: %v", err)
	}
	broker := telemetry.NewEventBroker()
	defer broker.Close()
	tracker := telemetry.NewStatsTracker(10)
	ring := telemetry.NewRingBufferSink(10)

	srv := NewServer(&contract.Config{Port: 8000}, nil, nil, nil)
	srv.tracker = tracker
	srv.diskStore = ds
	srv.eventBroker = broker
	srv.ringBuffer = ring

	reqReset := httptest.NewRequest(http.MethodPost, contract.PathAPIStatsReset, nil)
	wReset := httptest.NewRecorder()
	srv.handleAPIStatsReset(wReset, reqReset)
	if wReset.Code != http.StatusOK {
		t.Errorf("expected 200 from stats reset with ds and broker, got %d", wReset.Code)
	}

	// 2. Stats recalculate with directory as log file -> 500 error response
	srv.trafficLogPath = tmpDir
	reqRecalc := httptest.NewRequest(http.MethodPost, contract.PathAPIStatsRecalculate, nil)
	wRecalc := httptest.NewRecorder()
	srv.handleAPIStatsRecalculate(wRecalc, reqRecalc)
	if wRecalc.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for directory as log file, got %d", wRecalc.Code)
	}

	// 3. Stats recalculate with valid log file, diskStore, and eventBroker
	validLogPath := filepath.Join(tmpDir, "traffic.jsonl")
	record := telemetry.TurnRecord{
		RequestID:    "req-1",
		Timestamp:    time.Now(),
		SelectedTier: "Local",
		TargetModel:  "qwen",
		Tokens:       100,
	}
	recBytes, _ := json.Marshal(record)
	_ = os.WriteFile(validLogPath, append(recBytes, '\n'), 0600)
	srv.trafficLogPath = validLogPath

	wRecalcOk := httptest.NewRecorder()
	srv.handleAPIStatsRecalculate(wRecalcOk, reqRecalc)
	if wRecalcOk.Code != http.StatusOK {
		t.Errorf("expected 200 for successful recalculate, got %d", wRecalcOk.Code)
	}
}

func TestServer_DispatchTier_Streaming_QualityDefect_And_CycleSevering(t *testing.T) {
	// 1. Streaming quality defect: local provider returns immediate data: [DONE] -> fails over to default cloud tier
	mockLocalDone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer mockLocalDone.Close()

	mockCloudGood := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"cloud reply\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer mockCloudGood.Close()

	cfg := &contract.Config{
		Port:      8000,
		AuthToken: "test-secret-token",
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal, BaseURL: mockLocalDone.URL},
			"openrouter": {Type: contract.ProviderTypeCloud, BaseURL: mockCloudGood.URL, APIKey: "test"},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local Tier",
				Provider: "ollama",
				Model:    "qwen",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Cloud Fallback Tier",
			Provider: "openrouter",
			Model:    "claude",
		},
	}

	reg := provider.NewRegistry()
	reg.Register(provider.NewGenericLLMProvider("ollama", cfg.Providers["ollama"]))
	reg.Register(provider.NewGenericLLMProvider("openrouter", cfg.Providers["openrouter"]))

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier, cfg.Providers)
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, router.NewClassifier(), router.NewSanitizer(), nil, nil, reg, nil)

	reqStream := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"stream":true,"messages":[{"role":"user","content":"test"}]}`))
	reqStream.Header.Set("Authorization", "Bearer test-secret-token")
	wStream := httptest.NewRecorder()
	srv.ServeHTTP(wStream, reqStream)
	if wStream.Code != http.StatusOK || !strings.Contains(wStream.Body.String(), "cloud reply") {
		t.Errorf("expected failover to cloud reply on empty stream defect, got code=%d body=%s", wStream.Code, wStream.Body.String())
	}

	// 2. Non-streaming defect: empty JSON content from local provider -> fails over to default cloud tier
	mockLocalEmptyJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer mockLocalEmptyJSON.Close()

	mockCloudGoodJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"good cloud content"}}]}`))
	}))
	defer mockCloudGoodJSON.Close()

	cfgJSON := &contract.Config{
		Port:      8000,
		AuthToken: "test-secret-token",
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal, BaseURL: mockLocalEmptyJSON.URL},
			"openrouter": {Type: contract.ProviderTypeCloud, BaseURL: mockCloudGoodJSON.URL, APIKey: "test"},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local Tier",
				Provider: "ollama",
				Model:    "qwen",
				When:     "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Cloud Fallback Tier",
			Provider: "openrouter",
			Model:    "claude",
		},
	}
	regJSON := provider.NewRegistry()
	regJSON.Register(provider.NewGenericLLMProvider("ollama", cfgJSON.Providers["ollama"]))
	regJSON.Register(provider.NewGenericLLMProvider("openrouter", cfgJSON.Providers["openrouter"]))

	evaluatorJSON, _ := strategy.NewExprEvaluator(cfgJSON.Tiers, cfgJSON.DefaultTier, cfgJSON.Providers)
	srvJSON := NewServerWithTelemetryAndRegistry(cfgJSON, evaluatorJSON, router.NewClassifier(), router.NewSanitizer(), nil, nil, regJSON, nil)

	reqNonStream := httptest.NewRequest(http.MethodPost, contract.PathChatCompletions, strings.NewReader(`{"messages":[{"role":"user","content":"test"}]}`))
	reqNonStream.Header.Set("Authorization", "Bearer test-secret-token")
	wNonStream := httptest.NewRecorder()
	srvJSON.ServeHTTP(wNonStream, reqNonStream)
	if wNonStream.Code != http.StatusOK || !strings.Contains(wNonStream.Body.String(), "good cloud content") {
		t.Errorf("expected failover to cloud reply on empty json defect, got code=%d body=%s", wNonStream.Code, wNonStream.Body.String())
	}
}

func TestServer_ServeHTTP_AllAPIRoutesDispatch(t *testing.T) {
	srv, ring, broker := setupTestServer(t)
	srv.startTime = time.Now().Add(-10 * time.Minute)

	// Helper to send authorized request
	send := func(method, path string, body string) *httptest.ResponseRecorder {
		var r *http.Request
		if body != "" {
			r = httptest.NewRequest(method, path, strings.NewReader(body))
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		r.Header.Set("Authorization", "Bearer test-secret-token")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}

	// 1. GET /health
	wHealth := send(http.MethodGet, contract.PathHealth, "")
	if wHealth.Code != http.StatusOK || !strings.Contains(wHealth.Body.String(), `"status":"ok"`) {
		t.Errorf("expected 200 with status ok for /health, got code=%d body=%s", wHealth.Code, wHealth.Body.String())
	}

	// 2. GET /api/v1/info
	wInfo := send(http.MethodGet, contract.PathAPIInfo, "")
	if wInfo.Code != http.StatusOK || !strings.Contains(wInfo.Body.String(), `"version":`) {
		t.Errorf("expected 200 with version for /api/v1/info, got code=%d body=%s", wInfo.Code, wInfo.Body.String())
	}

	// 3. OPTIONS /api/v1/config (CORS preflight)
	wOptions := send(http.MethodOptions, contract.PathAPIConfig, "")
	if wOptions.Code != http.StatusNoContent && wOptions.Code != http.StatusOK {
		t.Errorf("expected 204 or 200 for OPTIONS, got %d", wOptions.Code)
	}

	// 4. GET /api/v1/routes
	ring.Emit(telemetry.TurnRecord{RequestID: "req-test-route"})
	wRoutes := send(http.MethodGet, contract.PathAPIRoutes, "")
	if wRoutes.Code != http.StatusOK || !strings.Contains(wRoutes.Body.String(), "req-test-route") {
		t.Errorf("expected 200 with emitted route for /api/v1/routes, got code=%d body=%s", wRoutes.Code, wRoutes.Body.String())
	}

	// 5. GET /api/v1/circuits
	wCircuits := send(http.MethodGet, contract.PathAPICircuits, "")
	if wCircuits.Code != http.StatusOK || !strings.Contains(wCircuits.Body.String(), `"circuits"`) {
		t.Errorf("expected 200 with circuits payload for /api/v1/circuits, got code=%d body=%s", wCircuits.Code, wCircuits.Body.String())
	}

	// 6. POST /api/v1/circuits/reset
	wCircuitsReset := send(http.MethodPost, contract.PathAPICircuitsReset, `{"provider":"all"}`)
	if wCircuitsReset.Code != http.StatusOK || !strings.Contains(wCircuitsReset.Body.String(), `"status":"ok"`) {
		t.Errorf("expected 200 with status ok for /api/v1/circuits/reset, got code=%d body=%s", wCircuitsReset.Code, wCircuitsReset.Body.String())
	}

	// 7. GET /api/v1/pricing
	wPricing := send(http.MethodGet, contract.PathAPIPricing, "")
	if wPricing.Code != http.StatusOK || !strings.Contains(wPricing.Body.String(), `"pricing"`) {
		t.Errorf("expected 200 with pricing payload for /api/v1/pricing, got code=%d body=%s", wPricing.Code, wPricing.Body.String())
	}

	// 8. GET /api/v1/config
	wConfig := send(http.MethodGet, contract.PathAPIConfig, "")
	if wConfig.Code != http.StatusOK || !strings.Contains(wConfig.Body.String(), `"port"`) {
		t.Errorf("expected 200 with port in config for /api/v1/config, got code=%d body=%s", wConfig.Code, wConfig.Body.String())
	}

	// 9. POST /api/v1/tune
	wTune := send(http.MethodPost, contract.PathAPITune, `{"max_iterations":5}`)
	if wTune.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/v1/tune, got %d", wTune.Code)
	}

	// 10. GET /api/v1/deals
	wDeals := send(http.MethodGet, contract.PathAPIDeals, "")
	if wDeals.Code != http.StatusOK || !strings.Contains(wDeals.Body.String(), `"deals"`) {
		t.Errorf("expected 200 with deals payload for /api/v1/deals, got code=%d body=%s", wDeals.Code, wDeals.Body.String())
	}

	// 11. POST /api/v1/stats/reset
	wStatsReset := send(http.MethodPost, contract.PathAPIStatsReset, "")
	if wStatsReset.Code != http.StatusOK || !strings.Contains(wStatsReset.Body.String(), `"status":"ok"`) {
		t.Errorf("expected 200 with status ok for /api/v1/stats/reset, got code=%d body=%s", wStatsReset.Code, wStatsReset.Body.String())
	}

	// 12. POST /api/v1/stats/recalculate
	wStatsRecalc := send(http.MethodPost, contract.PathAPIStatsRecalculate, "")
	if wStatsRecalc.Code != http.StatusOK || !strings.Contains(wStatsRecalc.Body.String(), `"status":"ok"`) {
		t.Errorf("expected 200 with status ok for /api/v1/stats/recalculate, got code=%d body=%s", wStatsRecalc.Code, wStatsRecalc.Body.String())
	}

	_ = broker
}
