package server

import (
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

func TestAPI_StatsReset_Endpoint(t *testing.T) {
	tmpDir := t.TempDir()
	statsPath := filepath.Join(tmpDir, "stats.json")
	diskStore, err := store.NewDiskStore(statsPath)
	if err != nil {
		t.Fatalf("failed to create disk store: %v", err)
	}

	tracker := telemetry.NewStatsTracker(100)
	ringBuffer := telemetry.NewRingBufferSink(10)
	eventBroker := telemetry.NewEventBroker()

	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, tracker, nil)
	srv.SetDiskStore(diskStore)
	srv.SetRingBuffer(ringBuffer)
	srv.SetEventBroker(eventBroker)

	// Add dummy traffic
	tracker.Record(telemetry.Observation{
		Tier:      1,
		Tokens:    1000,
		CostSpent: 0.0,
		CostSaved: 0.003,
		IsLocal:   true,
	})
	tracker.Flush()
	ringBuffer.Emit(telemetry.TurnRecord{Tokens: 1000})

	// 1. Verify Method Not Allowed
	getReq := httptest.NewRequest(http.MethodGet, contract.PathAPIStatsReset, nil)
	getW := httptest.NewRecorder()
	srv.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /api/v1/stats/reset, got %d", getW.Code)
	}

	// 2. Perform POST reset
	postReq := httptest.NewRequest(http.MethodPost, contract.PathAPIStatsReset, nil)
	postW := httptest.NewRecorder()
	srv.ServeHTTP(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for POST /api/v1/stats/reset, got %d: %s", postW.Code, postW.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(postW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}

	snap := tracker.GetStats()
	if snap.TotalRequests != 0 {
		t.Errorf("expected 0 total requests after reset, got %d", snap.TotalRequests)
	}
	if len(ringBuffer.GetRecent(10)) != 0 {
		t.Errorf("expected 0 recent routes after reset, got %d", len(ringBuffer.GetRecent(10)))
	}
}

func TestAPI_StatsRecalculate_Endpoint(t *testing.T) {
	tmpDir := t.TempDir()
	statsPath := filepath.Join(tmpDir, "stats.json")
	trafficPath := filepath.Join(tmpDir, "traffic.jsonl")

	diskStore, err := store.NewDiskStore(statsPath)
	if err != nil {
		t.Fatalf("failed to create disk store: %v", err)
	}

	// Create dummy traffic log
	records := []telemetry.TurnRecord{
		{
			Timestamp:    time.Now().UTC().Add(-5 * time.Minute),
			Tokens:       2000,
			SelectedTier: "Tier 1: Local Free",
			TargetModel:  "gemma4:12b-it-qat",
			Provider:     "ollama",
			IsLocal:      true,
			StatusCode:   200,
		},
		{
			Timestamp:    time.Now().UTC().Add(-2 * time.Minute),
			Tokens:       5000,
			SelectedTier: "Tier 2: Cloud Workhorse",
			TargetModel:  "google/gemini-3.7-flash",
			Provider:     "openrouter",
			IsLocal:      false,
			StatusCode:   200,
		},
	}
	f, err := os.Create(trafficPath)
	if err != nil {
		t.Fatalf("failed to create traffic file: %v", err)
	}
	for _, r := range records {
		b, _ := json.Marshal(r)
		f.Write(append(b, '\n'))
	}
	f.Close()

	tracker := telemetry.NewStatsTracker(100)
	ringBuffer := telemetry.NewRingBufferSink(10)
	eventBroker := telemetry.NewEventBroker()

	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, tracker, nil)
	srv.SetDiskStore(diskStore)
	srv.SetRingBuffer(ringBuffer)
	srv.SetEventBroker(eventBroker)
	srv.SetTrafficLogPath(trafficPath)

	// 1. Verify Method Not Allowed
	getReq := httptest.NewRequest(http.MethodGet, contract.PathAPIStatsRecalculate, nil)
	getW := httptest.NewRecorder()
	srv.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /api/v1/stats/recalculate, got %d", getW.Code)
	}

	// 2. Perform POST recalculate
	postReq := httptest.NewRequest(http.MethodPost, contract.PathAPIStatsRecalculate, nil)
	postW := httptest.NewRecorder()
	srv.ServeHTTP(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for POST /api/v1/stats/recalculate, got %d: %s", postW.Code, postW.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(postW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	if resp["records_processed"] != float64(2) {
		t.Errorf("expected 2 records processed, got %v", resp["records_processed"])
	}

	snap := tracker.GetStats()
	if snap.TotalRequests != 2 {
		t.Errorf("expected 2 total requests, got %d", snap.TotalRequests)
	}
	if snap.TotalTokensRoutedLocally != 2000 {
		t.Errorf("expected 2000 local tokens, got %d", snap.TotalTokensRoutedLocally)
	}
}

func TestAPI_Stats_CORS_Preflight(t *testing.T) {
	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, nil, nil)
	optionsReq := httptest.NewRequest(http.MethodOptions, contract.PathAPIStatsReset, nil)
	optionsW := httptest.NewRecorder()
	srv.ServeHTTP(optionsW, optionsReq)

	if optionsW.Code != http.StatusOK {
		t.Errorf("expected 200 for OPTIONS preflight, got %d", optionsW.Code)
	}
}

func TestAPI_Stats_ReloadConfigFromDisk(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
port: 8000
providers:
  ollama:
    base_url: http://localhost:11434
tiers:
  - name: Tier 1
    model: test-model
    provider: ollama
    when: "Tokens < 1000"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, nil, nil)
	srv.SetConfigPath(configPath)

	if err := srv.ReloadConfigFromDisk(); err != nil {
		t.Fatalf("ReloadConfigFromDisk failed: %v", err)
	}

	cfg := srv.GetConfig()
	if len(cfg.Tiers) != 1 || cfg.Tiers[0].Name != "Tier 1" {
		t.Errorf("unexpected reloaded config tiers: %v", cfg.Tiers)
	}

	// Test with non-existent config path
	srv.SetConfigPath(filepath.Join(tmpDir, "non_existent.yaml"))
	if err := srv.ReloadConfigFromDisk(); err == nil {
		t.Errorf("expected error on non existent config file, got nil")
	}
}

func TestAPI_Stats_ConfigYamlEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := "port: 9000\nproviders:\n  p1:\n    base_url: http://localhost:8000\n"
	_ = os.WriteFile(configPath, []byte(yamlContent), 0600)

	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, nil, nil)
	srv.SetConfigPath(configPath)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config?format=yaml", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK for GET /api/v1/config?format=yaml, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "port: 9000") {
		t.Errorf("expected yaml body containing port: 9000, got: %s", w.Body.String())
	}
}

func TestAPI_Stats_ConfigUpdateErrors(t *testing.T) {
	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, nil, nil)

	// 1. Invalid JSON body
	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader("not a json"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid json config update, got %d", w.Code)
	}

	// 2. Invalid YAML body
	req = httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader("tiers: [invalid yaml"))
	req.Header.Set("Content-Type", "application/x-yaml")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid yaml config update, got %d", w.Code)
	}
}

func TestAPI_Stats_EventsSSE_Broadcast(t *testing.T) {
	broker := telemetry.NewEventBroker()
	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, nil, nil)
	srv.SetEventBroker(broker)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = broker.PublishJSON(telemetry.EventStats, map[string]string{"foo": "bar"})
	}()

	srv.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "event: stats") {
		t.Errorf("expected SSE body to contain stats event, got: %s", w.Body.String())
	}
}

func TestAPI_Stats_StreamUsage(t *testing.T) {
	// 1. With explicit usage
	norm := NewStreamNormalizer(io.NopCloser(strings.NewReader("data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":50,\"total_tokens\":150}}\n\n")))
	buf := make([]byte, 1024)
	_, _ = norm.Read(buf)
	usage, hasUsage := norm.GetUsage()
	if !hasUsage || usage.TotalTokens != 150 {
		t.Errorf("expected usage with 150 tokens, got %v (hasUsage=%v)", usage, hasUsage)
	}
	_ = norm.Close()

	// 2. Fallback estimated tokens from content length
	norm2 := NewStreamNormalizer(io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hello world test stream\"}}]}\n\ndata: [DONE]\n\n")))
	buf2 := make([]byte, 1024)
	_, _ = norm2.Read(buf2)
	usage2, hasUsage2 := norm2.GetUsage()
	if hasUsage2 || usage2.CompletionTokens == 0 {
		t.Errorf("expected fallback token estimation, got %v", usage2)
	}
	_ = norm2.Close()
}

func TestAPI_Stats_PricingAndCircuits_Endpoints(t *testing.T) {
	oracle := telemetry.NewPricingOracle()
	srv := NewServerWithTelemetry(nil, nil, nil, nil, oracle, nil, nil)

	// Pricing endpoint with oracle
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pricing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/v1/pricing, got %d", w.Code)
	}

	// Circuits endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/v1/circuits", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/v1/circuits, got %d", w.Code)
	}

	// Config endpoint default JSON
	req = httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/v1/config, got %d", w.Code)
	}

	// Config endpoint fallback YAML
	req = httptest.NewRequest(http.MethodGet, "/api/v1/config?format=yaml", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for fallback YAML /api/v1/config, got %d", w.Code)
	}

	// Recalculate error with non-existent path
	srv.SetTrafficLogPath("/invalid/non/existent/path/traffic.jsonl")
	req = httptest.NewRequest(http.MethodPost, "/api/v1/stats/recalculate", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		// Non-existent file is handled gracefully returning empty records
		if w.Code != http.StatusInternalServerError && w.Code != http.StatusOK {
			t.Errorf("unexpected status on missing file recalculate: %d", w.Code)
		}
	}
}

func TestAPI_Stats_AuthenticateClient_EdgeCases(t *testing.T) {
	cfg := &contract.Config{
		AuthToken: "secret-key-123",
	}
	srv := NewServerWithTelemetry(cfg, nil, nil, nil, nil, nil, nil)

	// 1. Bearer token match
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret-key-123")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid Bearer token, got %d", w.Code)
	}

	// 2. X-API-Key match
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-API-Key", "secret-key-123")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid X-API-Key, got %d", w.Code)
	}

	// 3. api-key header match
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("api-key", "secret-key-123")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid api-key header, got %d", w.Code)
	}

	// 4. Invalid key -> 401 Unauthorized
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong key, got %d", w.Code)
	}
}

func TestAPI_Stats_ConfigUpdate_Success(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, nil, nil)
	srv.SetConfigPath(cfgPath)

	// 1. Successful JSON config update
	validJSON := `{
		"port": 8000,
		"providers": {
			"ollama": { "base_url": "http://localhost:11434" }
		},
		"tiers": [
			{ "name": "T1", "model": "m1", "provider": "ollama", "when": "Tokens < 100" }
		]
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(validJSON))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid JSON config update, got %d", w.Code)
	}

	// 2. Successful YAML config update
	validYAML := `
port: 8000
providers:
  ollama:
    base_url: http://localhost:11434
tiers:
  - name: T1
    model: m1
    provider: ollama
    when: "Tokens < 200"
`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(validYAML))
	req.Header.Set("Content-Type", "application/x-yaml")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid YAML config update, got %d", w.Code)
	}
}

func TestAPI_Stats_AllEndpoints_OPTIONS_CORS(t *testing.T) {
	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, nil, nil)
	endpoints := []string{
		contract.PathAPIInfo,
		contract.PathAPIEvents,
		contract.PathAPIRoutes,
		contract.PathAPICircuits,
		contract.PathAPICircuitsReset,
		contract.PathAPIPricing,
		contract.PathAPIConfig,
		contract.PathAPITune,
		contract.PathAPIDeals,
		contract.PathAPIStatsReset,
		contract.PathAPIStatsRecalculate,
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodOptions, ep, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for OPTIONS on %s, got %d", ep, w.Code)
		}
	}
}

func TestProxy_UpstreamProviderErrorsAndHeaders(t *testing.T) {
	// Upstream test server returning 502 Bad Gateway
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("missing Authorization header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Custom-Header") != "custom-value" {
			t.Errorf("missing custom header")
		}
		http.Error(w, "upstream service unavailable", http.StatusBadGateway)
	}))
	defer upstream.Close()

	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"mock_prov": {
				BaseURL: upstream.URL,
				APIKey:  "test-api-key",
				Headers: map[string]string{
					"X-Custom-Header": "custom-value",
				},
			},
		},
		Tiers: []contract.Tier{
			{Name: "T1", Model: "m1", Provider: "mock_prov", When: "true"},
		},
		DefaultTier: contract.Tier{Name: "T1", Model: "m1", Provider: "mock_prov", When: "true"},
	}

	eval, _ := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	class := router.NewClassifier()
	san := router.NewSanitizer()
	reg := provider.NewRegistryFromConfig(cfg)
	srv := NewServerWithTelemetryAndRegistry(cfg, eval, class, san, nil, nil, reg, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"nacho-auto","messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 from upstream proxy, got %d", w.Code)
	}
}

func TestProxy_MetaDirective_DirectChatCommand(t *testing.T) {
	cfg := &contract.Config{Port: 8000}
	class := router.NewClassifier()
	srv := NewServerWithTelemetry(cfg, nil, class, nil, nil, nil, nil)

	// 1. Non-streaming @nacho:status
	bodyJSON := `{"model":"nacho-auto","messages":[{"role":"user","content":"@nacho:status"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(bodyJSON))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for @nacho:status, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Nacho Flow Daemon Status") {
		t.Errorf("expected status output in body, got: %s", w.Body.String())
	}

	// 2. Streaming @nacho:status
	streamJSON := `{"model":"nacho-auto","stream":true,"messages":[{"role":"user","content":"@nacho:status"}]}`
	reqStream := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(streamJSON))
	wStream := httptest.NewRecorder()
	srv.ServeHTTP(wStream, reqStream)
	if wStream.Code != http.StatusOK {
		t.Errorf("expected 200 for streaming @nacho:status, got %d", wStream.Code)
	}
}

func TestAPI_Stats_NotFoundAndStatsRoutes(t *testing.T) {
	tracker := telemetry.NewStatsTracker(10)
	srv := NewServerWithTelemetry(nil, nil, nil, nil, nil, tracker, nil)

	// 1. Unknown /api/v1 route -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/unknown-endpoint", nil)
	w404 := httptest.NewRecorder()
	srv.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown route, got %d", w404.Code)
	}

	// 2. /v1/stats route
	reqStats := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	wStats := httptest.NewRecorder()
	srv.ServeHTTP(wStats, reqStats)
	if wStats.Code != http.StatusOK {
		t.Errorf("expected 200 for /v1/stats, got %d", wStats.Code)
	}
}

func TestAPI_MetaCommands_DirectExecute(t *testing.T) {
	reg := NewMetaRegistry()
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "Tier 1: Local Fast", Model: "qwen:7b", Provider: "ollama", When: "Tokens < 1000"},
		},
	}
	env := MetaEnv{
		Config:        cfg,
		Stats:         telemetry.NewStatsTracker(10),
		Oracle:        telemetry.NewPricingOracle(),
		StartTime:     time.Now(),
		DaemonVersion: "0.6.0",
	}

	ctx := context.Background()

	// 1. Help
	helpCmd := &HelpCommandHandler{registry: reg}
	helpOut, err := helpCmd.Execute(ctx, contract.RequestContext{}, env)
	if err != nil || !strings.Contains(helpOut, "@nacho:help") {
		t.Errorf("unexpected help output: %s, err: %v", helpOut, err)
	}

	// 2. Tiers
	tiersCmd := &TiersCommandHandler{}
	tiersOut, err := tiersCmd.Execute(ctx, contract.RequestContext{}, env)
	if err != nil || !strings.Contains(tiersOut, "Tier 1: Local Fast") {
		t.Errorf("unexpected tiers output: %s, err: %v", tiersOut, err)
	}

	// 3. Deals
	dealsCmd := &DealsCommandHandler{}
	dealsOut, err := dealsCmd.Execute(ctx, contract.RequestContext{}, env)
	if err != nil || !strings.Contains(dealsOut, "Heat Seeker") {
		t.Errorf("unexpected deals output: %s, err: %v", dealsOut, err)
	}
}

