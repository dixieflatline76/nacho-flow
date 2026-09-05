package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestMetaRegistry_Commands(t *testing.T) {
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "Tier 1: Local ROCm", Model: "qwen2.5-coder:14b", Provider: "ollama"},
			{Name: "Tier 2: Cloud Workhorse", Model: "qwen3-coder-480b", Provider: "openrouter"},
		},
		DefaultTier: contract.Tier{Name: "Tier 2B: Frontier", Model: "claude-3.7-sonnet", Provider: "openrouter"},
	}

	stats := telemetry.NewStatsTracker(100)
	stats.Record(telemetry.Observation{
		IsLocal:   true,
		CostSaved: 1.25,
		CostSpent: 0.05,
	})
	stats.Flush()

	provReg := provider.NewRegistry()
	oracle := telemetry.NewPricingOracle()

	env := MetaEnv{
		Config:        cfg,
		Stats:         stats,
		Oracle:        oracle,
		Providers:     provReg,
		StartTime:     time.Now().Add(-10 * time.Minute),
		DaemonVersion: "v0.8.0",
	}

	registry := NewMetaRegistry()

	// 1. Help Command
	helpOut, err := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: "help"}, env)
	if err != nil || !strings.Contains(helpOut, "HotSauce Directives") {
		t.Errorf("HelpCommand failed: %v, out: %s", err, helpOut)
	}

	// 2. Tiers Command
	tiersOut, err := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: "tiers"}, env)
	if err != nil || !strings.Contains(tiersOut, "Tier 1: Local ROCm") || !strings.Contains(tiersOut, "Tier 2B: Frontier") {
		t.Errorf("TiersCommand failed: %v, out: %s", err, tiersOut)
	}

	// 3. Status Command
	statusOut, err := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: "status"}, env)
	if err != nil || !strings.Contains(statusOut, "Daemon Status") || !strings.Contains(statusOut, "Turn Telemetry") {
		t.Errorf("StatusCommand failed: %v, out: %s", err, statusOut)
	}

	// 4. Deals Command
	dealsOut, err := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: "deals"}, env)
	if err != nil || !strings.Contains(dealsOut, "Heat Seeker") {
		t.Errorf("DealsCommand failed: %v, out: %s", err, dealsOut)
	}

	// 5. Unknown Command with typo suggestion
	unknownOut, err := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: "unknown", MetaDirectiveRaw: "@nacho:lcoal"}, env)
	if err != nil || !strings.Contains(unknownOut, "@nacho:local") {
		t.Errorf("UnknownCommand did not suggest @nacho:local for @nacho:lcoal, out: %s", unknownOut)
	}

	// 6. Circuit Blocked Command
	circuitOut := RenderCircuitBlocked("Local ROCm GPU", "ollama")
	if !strings.Contains(circuitOut, "Circuit Alert") || !strings.Contains(circuitOut, "Local ROCm GPU") {
		t.Errorf("CircuitBlocked output invalid: %s", circuitOut)
	}
}

func TestMetaPresenters(t *testing.T) {
	content := "🌮 **Test Response Content**"

	// 1. JSON Presenter (stream = false)
	recJSON := httptest.NewRecorder()
	jsonPresenter := &JSONMetaPresenter{}
	if err := jsonPresenter.WriteResponse(recJSON, content, "nacho-meta-test"); err != nil {
		t.Fatalf("JSON presenter error: %v", err)
	}

	if recJSON.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %q", recJSON.Header().Get("Content-Type"))
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(recJSON.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}

	choices := parsed["choices"].([]interface{})
	firstChoice := choices[0].(map[string]interface{})
	msg := firstChoice["message"].(map[string]interface{})
	if msg["content"] != content {
		t.Errorf("expected content %q, got %q", content, msg["content"])
	}

	// 2. SSE Presenter (stream = true)
	recSSE := httptest.NewRecorder()
	ssePresenter := &SSEMetaPresenter{}
	if err := ssePresenter.WriteResponse(recSSE, content, "nacho-meta-test"); err != nil {
		t.Fatalf("SSE presenter error: %v", err)
	}

	if recSSE.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %q", recSSE.Header().Get("Content-Type"))
	}

	sseBody := recSSE.Body.String()
	if !strings.Contains(sseBody, "data: {") || !strings.Contains(sseBody, "data: [DONE]") {
		t.Errorf("invalid SSE body: %s", sseBody)
	}
}

func TestMetaDirective_WorksWithoutProviders(t *testing.T) {
	// Empty config with 0 providers
	emptyCfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{},
		Tiers:     []contract.Tier{},
	}

	env := MetaEnv{
		Config:        emptyCfg,
		Stats:         telemetry.NewStatsTracker(10),
		Oracle:        telemetry.NewPricingOracle(),
		Providers:     provider.NewRegistry(),
		StartTime:     time.Now(),
		DaemonVersion: "v0.8.0",
	}

	registry := NewMetaRegistry()

	// All meta directives must succeed without panic or error
	directives := []string{"help", "tiers", "status", "deals"}
	for _, dir := range directives {
		out, err := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: dir}, env)
		if err != nil {
			t.Errorf("directive %q failed with empty config: %v", dir, err)
		}
		if out == "" {
			t.Errorf("directive %q returned empty output", dir)
		}
	}
}

func TestMetaRegistry_Dispatch(t *testing.T) {
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "Local GPU", Model: "qwen", Provider: "ollama"},
		},
		DefaultTier: contract.Tier{Name: "Cloud", Model: "claude", Provider: "openrouter"},
	}

	env := MetaEnv{
		Config:        cfg,
		Stats:         telemetry.NewStatsTracker(10),
		Oracle:        telemetry.NewPricingOracle(),
		Providers:     provider.NewRegistry(),
		StartTime:     time.Now(),
		DaemonVersion: "v0.8.0",
	}

	registry := NewMetaRegistry()
	sessionTracker := router.NewSessionTracker(time.Minute)

	// 1. Dispatch JSON non-streaming
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reqCtx1 := contract.RequestContext{
		IsMetaDirective: true,
		MetaDirective:   "help",
	}
	registry.Dispatch(rec1, req1, reqCtx1, env, sessionTracker, false)
	if rec1.Code != http.StatusOK || !strings.Contains(rec1.Body.String(), "HotSauce Directives") {
		t.Errorf("Dispatch JSON failed: code=%d, body=%s", rec1.Code, rec1.Body.String())
	}

	// 2. Dispatch SSE streaming
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reqCtx2 := contract.RequestContext{
		IsMetaDirective: true,
		MetaDirective:   "tiers",
	}
	registry.Dispatch(rec2, req2, reqCtx2, env, sessionTracker, true)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "data: [DONE]") {
		t.Errorf("Dispatch SSE failed: code=%d, body=%s", rec2.Code, rec2.Body.String())
	}

	// 3. Dispatch debounced repeat
	rec3 := httptest.NewRecorder()
	registry.Dispatch(rec3, req2, reqCtx2, env, sessionTracker, false)
	if !strings.Contains(rec3.Body.String(), "recently served") {
		t.Errorf("expected debounce message, got: %s", rec3.Body.String())
	}

	// 4. Unknown with no match
	unknownNoMatch, _ := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: "unknown", MetaDirectiveRaw: "@nacho:supercalifragilistic"}, env)
	if !strings.Contains(unknownNoMatch, "that is not a recognized directive") {
		t.Errorf("expected not recognized message, got: %s", unknownNoMatch)
	}

	// 5. Deals with empty oracle
	dealsEmpty, _ := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: "deals"}, MetaEnv{})
	if !strings.Contains(dealsEmpty, "Pricing oracle is not active") {
		t.Errorf("expected not active message, got: %s", dealsEmpty)
	}

	// 6. Status with empty startTime and nil stats
	statusEmpty, _ := registry.Execute(context.Background(), contract.RequestContext{MetaDirective: "status"}, MetaEnv{})
	if !strings.Contains(statusEmpty, "N/A") {
		t.Errorf("expected N/A uptime, got: %s", statusEmpty)
	}
}

func TestProxy_ForcedDirective_CircuitOpen_BlocksRequest(t *testing.T) {
	deadLocalURL := "http://127.0.0.1:59999"

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"dead_local": {BaseURL: deadLocalURL, Type: "local"},
			"cloud_p":    {BaseURL: "http://127.0.0.1:59998", Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Local GPU", Model: "qwen", Provider: "dead_local", When: "Tokens < 8000"},
		},
		DefaultTier: contract.Tier{Name: "Cloud Fallback", Model: "claude", Provider: "cloud_p"},
	}

	reg := provider.NewRegistryFromConfig(cfg)
	// Trip circuit breaker on dead_local
	if deadP, found := reg.Get("dead_local"); found {
		if cbp, ok := deadP.(provider.CircuitBreakerProvider); ok {
			for i := 0; i < 5; i++ {
				cbp.CircuitBreaker().RecordFailure()
			}
		}
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, router.NewClassifier(), router.NewSanitizer(), nil, nil, reg, nil)

	// 1. Non-streaming forced local request
	reqBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"@nacho:local refactor this"}]}`)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for chat alert, got %d", rec.Code)
	}

	bodyStr := rec.Body.String()
	if !strings.Contains(bodyStr, "Circuit Alert") || !strings.Contains(bodyStr, "Local GPU") {
		t.Errorf("expected circuit alert in response, got: %s", bodyStr)
	}

	// 2. Streaming forced local request
	reqBodyStream := []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"@nacho:local refactor this"}]}`)
	reqStream := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBodyStream))
	recStream := httptest.NewRecorder()

	srv.ServeHTTP(recStream, reqStream)

	if recStream.Code != http.StatusOK {
		t.Fatalf("expected status 200 for streaming chat alert, got %d", recStream.Code)
	}

	streamStr := recStream.Body.String()
	if !strings.Contains(streamStr, "data: [DONE]") || !strings.Contains(streamStr, "Circuit Alert") {
		t.Errorf("expected streaming circuit alert, got: %s", streamStr)
	}

	// 3. Forced tier named with circuit open
	reqNamed := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"@nacho:tier=\"Local GPU\" test"}]}`)))
	recNamed := httptest.NewRecorder()
	srv.ServeHTTP(recNamed, reqNamed)
	if !strings.Contains(recNamed.Body.String(), "Circuit Alert") {
		t.Errorf("expected circuit alert for named tier, got: %s", recNamed.Body.String())
	}
}

type failingCommand struct{}

func (f *failingCommand) Name() string        { return "failcmd" }
func (f *failingCommand) Description() string { return "failing test command" }
func (f *failingCommand) Execute(ctx context.Context, reqCtx contract.RequestContext, env MetaEnv) (string, error) {
	return "", fmt.Errorf("simulated command failure")
}

type mockPricingProv struct {
	name   string
	prices map[string]telemetry.ModelMetadata
}

func (m *mockPricingProv) Name() string { return m.name }
func (m *mockPricingProv) FetchPricing(ctx context.Context) (map[string]telemetry.ModelMetadata, error) {
	return m.prices, nil
}

func TestMetaRegistry_DealsAndFailingCommand(t *testing.T) {
	oracle := telemetry.NewPricingOracle()
	mockProv := &mockPricingProv{
		name: "openrouter",
		prices: map[string]telemetry.ModelMetadata{
			"test/cheap-model": {
				ModelPricing: telemetry.ModelPricing{
					PromptCostPerMillion:     0.05,
					CompletionCostPerMillion: 0.10,
				},
				ModelID:       "test/cheap-model",
				Name:          "Cheap Model",
				SupportsTools: true,
				CodingIndex:   80.0,
			},
			"test/free-model": {
				ModelPricing: telemetry.ModelPricing{
					PromptCostPerMillion:     0.0,
					CompletionCostPerMillion: 0.0,
				},
				ModelID:       "test/free-model",
				Name:          "Free Model",
				SupportsTools: true,
				CodingIndex:   90.0,
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oracle.RegisterProvider(mockProv, time.Hour)
	oracle.StartBackgroundSync(ctx, time.Hour)
	time.Sleep(10 * time.Millisecond)

	env := MetaEnv{
		Config: &contract.Config{
			Deals: contract.DealsConfig{
				Enabled:           true,
				AlertThresholdPct: 20.0,
				MinCodingIndex:    50.0,
			},
			Tiers: []contract.Tier{
				{Name: "Only Tier", Model: "m1", Provider: "p1"},
			},
		},
		Oracle: oracle,
	}

	reg := NewMetaRegistry()
	reg.Register(&failingCommand{})

	// 1. Deals with detected deals (including FREE model)
	dealsOut, err := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "deals"}, env)
	if err != nil || !strings.Contains(dealsOut, "Heat Seeker") {
		t.Errorf("deals rendering failed: %v, out: %s", err, dealsOut)
	}

	// 2. Tiers with no default tier & nil config
	tiersOut, err := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "tiers"}, env)
	if err != nil || !strings.Contains(tiersOut, "Only Tier") {
		t.Errorf("tiers rendering failed: %v, out: %s", err, tiersOut)
	}

	tiersNil, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "tiers"}, MetaEnv{})
	if !strings.Contains(tiersNil, "No tiers currently configured") {
		t.Errorf("expected no tiers message, got: %s", tiersNil)
	}

	// 3. Status with default version fallback
	statusDef, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "status"}, MetaEnv{DaemonVersion: ""})
	if !strings.Contains(statusDef, "Version & Uptime") {
		t.Errorf("expected status output with default version, got: %s", statusDef)
	}

	// 4. Dispatch failing command
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reg.Dispatch(rec, req, contract.RequestContext{MetaDirective: "failcmd"}, env, nil, false)
	if !strings.Contains(rec.Body.String(), "Directive Error") {
		t.Errorf("expected Directive Error, got: %s", rec.Body.String())
	}
}

func TestProxy_DirectivesDisabledByConfig(t *testing.T) {
	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Standard response"}}]}`))
	}))
	defer mockCloud.Close()

	disabled := false
	cfg := &contract.Config{
		Port: 8000,
		Router: contract.RouterConfig{
			EnableInPromptDirectives: &disabled,
		},
		Providers: map[string]contract.ProviderConfig{
			"cloud_p": {BaseURL: mockCloud.URL, Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Cloud Tier", Model: "claude", Provider: "cloud_p", When: "Tokens < 8000"},
		},
		DefaultTier: contract.Tier{Name: "Cloud Fallback", Model: "claude", Provider: "cloud_p"},
	}

	reg := provider.NewRegistryFromConfig(cfg)
	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, router.NewClassifier(), router.NewSanitizer(), nil, nil, reg, nil)

	// Even with @nacho:help, should NOT trigger meta handler because directives are disabled
	reqBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"@nacho:help"}]}`)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "Standard response") {
		t.Errorf("expected regular response when directives disabled, got: %s", rec.Body.String())
	}
}

func TestMetaRegistry_TogglesAndReset(t *testing.T) {
	reg := NewMetaRegistry()
	tracker := router.NewSessionTracker(5 * time.Minute)
	sessionKey := "test-meta-toggles"
	tracker.RecordTurn(sessionKey, router.HashPrompt("init"), false)

	env := MetaEnv{
		SessionTracker: tracker,
		SessionKey:     sessionKey,
	}

	// 1. Initial toggles state
	out, err := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "toggles"}, env)
	if err != nil {
		t.Fatalf("toggles command failed: %v", err)
	}
	if !strings.Contains(out, "Session Toggles & Guardrails") {
		t.Errorf("expected header, got: %s", out)
	}
	if !strings.Contains(out, "HotSauce Kickstart") || !strings.Contains(out, "Cycle Killer") {
		t.Errorf("expected guardrail names, got: %s", out)
	}

	// 2. Standalone Kickstart toggle
	kOff, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "kickstart-off"}, env)
	if !strings.Contains(kOff, "Kickstart Suspended") {
		t.Errorf("expected Kickstart Suspended, got: %s", kOff)
	}
	if !tracker.GetGuardrails(sessionKey).KickstartDisabled {
		t.Error("expected KickstartDisabled=true")
	}

	kOn, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "kickstart-on"}, env)
	if !strings.Contains(kOn, "Kickstart Active") {
		t.Errorf("expected Kickstart Active, got: %s", kOn)
	}
	if tracker.GetGuardrails(sessionKey).KickstartDisabled {
		t.Error("expected KickstartDisabled=false")
	}

	// 3. Standalone Cycle Killer toggle
	ckOff, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "cyclekiller-off"}, env)
	if !strings.Contains(ckOff, "Cycle Killer Disabled") {
		t.Errorf("expected Cycle Killer Disabled, got: %s", ckOff)
	}
	if !tracker.GetGuardrails(sessionKey).CycleKillerDisabled {
		t.Error("expected CycleKillerDisabled=true")
	}

	ckOn, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "cyclekiller-on"}, env)
	if !strings.Contains(ckOn, "Cycle Killer Active") {
		t.Errorf("expected Cycle Killer Active, got: %s", ckOn)
	}
	if tracker.GetGuardrails(sessionKey).CycleKillerDisabled {
		t.Error("expected CycleKillerDisabled=false")
	}

	// 4. Standalone Shield toggle
	sOff, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "shield-off"}, env)
	if !strings.Contains(sOff, "Fallback Shield Disabled") {
		t.Errorf("expected Shield Disabled, got: %s", sOff)
	}
	if !tracker.GetGuardrails(sessionKey).ShieldDisabled {
		t.Error("expected ShieldDisabled=true")
	}

	sOn, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "shield-on"}, env)
	if !strings.Contains(sOn, "Fallback Shield Active") {
		t.Errorf("expected Shield Active, got: %s", sOn)
	}
	if tracker.GetGuardrails(sessionKey).ShieldDisabled {
		t.Error("expected ShieldDisabled=false")
	}

	// 5. Standalone Raw toggle
	rOn, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "raw-on"}, env)
	if !strings.Contains(rOn, "Raw Pass-Through Active") {
		t.Errorf("expected Raw Active, got: %s", rOn)
	}
	if !tracker.GetGuardrails(sessionKey).RawModeEnabled {
		t.Error("expected RawModeEnabled=true")
	}

	rOff, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "raw-off"}, env)
	if !strings.Contains(rOff, "Raw Pass-Through Disabled") {
		t.Errorf("expected Raw Disabled, got: %s", rOff)
	}
	if tracker.GetGuardrails(sessionKey).RawModeEnabled {
		t.Error("expected RawModeEnabled=false")
	}

	// 6. Standalone Fairy Dust toggle
	fdOff, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "fairydust-off"}, env)
	if !strings.Contains(fdOff, "Fairy Dusting Disabled") {
		t.Errorf("expected Fairy Dust Disabled, got: %s", fdOff)
	}
	if !tracker.GetGuardrails(sessionKey).FairyDustDisabled {
		t.Error("expected FairyDustDisabled=true")
	}

	fdOn, _ := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "fairydust-on"}, env)
	if !strings.Contains(fdOn, "Fairy Dusting Active") {
		t.Errorf("expected Fairy Dust Active, got: %s", fdOn)
	}
	if tracker.GetGuardrails(sessionKey).FairyDustDisabled {
		t.Error("expected FairyDustDisabled=false")
	}

	// 7. Reset session
	tracker.SetKickstartDisabled(sessionKey, true)
	tracker.SetCycleKillerDisabled(sessionKey, true)
	resetOut, err := reg.Execute(context.Background(), contract.RequestContext{MetaDirective: "reset"}, env)
	if err != nil {
		t.Fatalf("reset command failed: %v", err)
	}
	if !strings.Contains(resetOut, "Session Reset") {
		t.Errorf("expected Session Reset header, got: %s", resetOut)
	}
	if tracker.GetGuardrails(sessionKey) != (router.SessionGuardrails{}) {
		t.Errorf("expected guardrails cleared after reset, got %+v", tracker.GetGuardrails(sessionKey))
	}
}

func TestMetaCommands_Metadata(t *testing.T) {
	reg := NewMetaRegistry()
	for name, cmd := range reg.commands {
		if cmd.Name() == "" {
			t.Errorf("expected non-empty Name() for command %s", name)
		}
		if cmd.Description() == "" {
			t.Errorf("expected non-empty Description() for command %s", name)
		}
	}
}

func TestProxy_SessionGuardrailsAndKickstartAutoSuspend(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"I investigated the codebase."}}]}`))
	}))
	defer mockUpstream.Close()

	enabled := true
	writeOnly := true
	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"local_p": {BaseURL: mockUpstream.URL, Type: "local"},
			"cloud_p": {BaseURL: mockUpstream.URL, Type: "cloud"},
		},
		Tiers: []contract.Tier{
			{Name: "Tier 1: Local", Model: "local-model", Provider: "local_p"},
		},
		DefaultTier: contract.Tier{Name: "Tier 2: Cloud", Model: "cloud-model", Provider: "cloud_p"},
		CycleKiller: contract.CycleBreakerConfig{
			Enabled:             &enabled,
			KickstartThreshold:  2,
			KickstartWriteOnly:  writeOnly,
			KickstartWriteTools: []string{"write_to_file", "replace_in_file"},
		},
	}

	reg := provider.NewRegistryFromConfig(cfg)
	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	classifier := router.NewClassifier()
	if c, ok := classifier.(*router.RequestClassifier); ok {
		c.SetKickstartWriteTools([]string{"write_to_file", "replace_in_file"})
	}
	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, classifier, router.NewSanitizer(), nil, nil, reg, nil)

	// 1. Send Plan Mode turn (tools present but no write tools)
	// Even after 3 turns with no write progress, kickstart should be auto-suspended!
	planPayload := `{"model":"local-model","messages":[{"role":"user","content":"explore auth architecture"}],"tools":[{"function":{"name":"read_file"}}]}`
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(planPayload)))
		req.Header.Set("x-session-id", "sess-plan-mode-test")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("turn %d failed with code %d", i+1, rec.Code)
		}
	}
	// Kickstart count should NOT be 3 (should be 0 or suspended)
	if srv.sessionTracker.GetKickstartCount("sess-plan-mode-test") > 1 {
		t.Errorf("expected Kickstart count <= 1 in Plan Mode, got %d", srv.sessionTracker.GetKickstartCount("sess-plan-mode-test"))
	}

	// 2. Embedded @nacho:kickstart-off directive
	embeddedPayload := `{"model":"local-model","messages":[{"role":"user","content":"@nacho:kickstart-off check system health"}]}`
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(embeddedPayload)))
	req2.Header.Set("x-session-id", "sess-toggle-embedded")
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("failed with code %d", rec2.Code)
	}
	if !srv.sessionTracker.GetGuardrails("sess-toggle-embedded").KickstartDisabled {
		t.Error("expected KickstartDisabled=true after embedded directive")
	}

	// 3. Embedded @nacho:cyclekiller-off directive
	embeddedCK := `{"model":"local-model","messages":[{"role":"user","content":"@nacho:cyclekiller-off batch refactor"}]}`
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(embeddedCK)))
	req3.Header.Set("x-session-id", "sess-toggle-ck")
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("failed with code %d", rec3.Code)
	}
	if !srv.sessionTracker.GetGuardrails("sess-toggle-ck").CycleKillerDisabled {
		t.Error("expected CycleKillerDisabled=true after embedded directive")
	}

	// 4. Standalone @nacho:toggles returns 200 with guardrails markdown
	togglesReq := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"nacho-auto","messages":[{"role":"user","content":"@nacho:toggles"}]}`)))
	togglesReq.Header.Set("x-session-id", "sess-toggle-embedded")
	recToggles := httptest.NewRecorder()
	srv.ServeHTTP(recToggles, togglesReq)
	if recToggles.Code != http.StatusOK {
		t.Fatalf("expected 200 for @nacho:toggles, got %d", recToggles.Code)
	}
	if !strings.Contains(recToggles.Body.String(), "HotSauce Kickstart") {
		t.Errorf("expected toggles markdown, got: %s", recToggles.Body.String())
	}

	// 5. Standalone @nacho:reset returns 200 and resets guardrails
	resetReq := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"nacho-auto","messages":[{"role":"user","content":"@nacho:reset"}]}`)))
	resetReq.Header.Set("x-session-id", "sess-toggle-embedded")
	recReset := httptest.NewRecorder()
	srv.ServeHTTP(recReset, resetReq)
	if recReset.Code != http.StatusOK {
		t.Fatalf("expected 200 for @nacho:reset, got %d", recReset.Code)
	}
	if srv.sessionTracker.GetGuardrails("sess-toggle-embedded").KickstartDisabled {
		t.Error("expected KickstartDisabled=false after reset")
	}
}
