package server

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

func setupTestServer(t *testing.T) (*Server, *telemetry.RingBufferSink, *telemetry.EventBroker) {
	cfg := &contract.Config{
		Port:      8000,
		AuthToken: "test-secret-token",
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "http://127.0.0.1:11434",
			},
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  "sk-or-v1-real-secret-key-12345",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1: Local Free",
				Provider: "ollama",
				Model:    "qwen2.5-coder:7b",
				When:     "Tokens < 8000 && !HasImages && !HasTools",
			},
			{
				Name:     "Tier 2: Cloud Coder",
				Provider: "openrouter",
				Model:    "qwen/qwen3-coder",
				When:     "Retries < 2",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 3: Cloud Reasoning Fallback",
			Provider: "openrouter",
			Model:    "anthropic/claude-3.7-sonnet",
		},
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		t.Fatalf("failed to create evaluator: %v", err)
	}

	reg := provider.NewRegistryFromConfig(cfg)
	oracle := telemetry.NewPricingOracle()
	tracker := telemetry.NewStatsTracker(100)
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()

	srv := NewServerWithTelemetryAndRegistry(cfg, evaluator, classifier, sanitizer, oracle, tracker, reg, nil)

	ringBuffer := telemetry.NewRingBufferSink(10)
	tracker.AddSink(ringBuffer)
	srv.SetRingBuffer(ringBuffer)

	eventBroker := telemetry.NewEventBroker()
	tracker.AddSink(eventBroker)
	srv.SetEventBroker(eventBroker)
	srv.SetTuner(tuner.NewCostPenaltyOptimizer())

	return srv, ringBuffer, eventBroker
}

func TestAPI_Info_Unauthenticated(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIInfo, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if resp["service"] != contract.AppName {
		t.Errorf("expected service '%s', got '%v'", contract.AppName, resp["service"])
	}
	features, ok := resp["features"].([]interface{})
	if !ok || len(features) < 4 {
		t.Errorf("expected features list, got %+v", resp["features"])
	}
}

func TestAPI_Auth_Enforcement(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	protectedRoutes := []string{
		contract.PathAPIRoutes,
		contract.PathAPICircuits,
		contract.PathAPIPricing,
		contract.PathAPIConfig,
		contract.PathAPITune,
	}

	for _, route := range protectedRoutes {
		// 1. Without Auth
		req := httptest.NewRequest(http.MethodGet, route, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("route %s expected 401 without auth, got %d", route, w.Code)
		}

		// 2. With Invalid Auth
		req = httptest.NewRequest(http.MethodGet, route, nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		w = httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("route %s expected 401 with invalid auth, got %d", route, w.Code)
		}

		// 3. With Valid Auth Header
		req = httptest.NewRequest(http.MethodGet, route, nil)
		req.Header.Set("Authorization", "Bearer test-secret-token")
		w = httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("route %s expected OK with valid auth, got %d", route, w.Code)
		}
	}
}

func TestAPI_CORS_Preflight(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodOptions, contract.PathAPIConfig, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for OPTIONS preflight, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS allow origin *, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestAPI_Routes_RingBuffer(t *testing.T) {
	srv, ringBuffer, _ := setupTestServer(t)

	// Emit 3 turn records
	ringBuffer.Emit(telemetry.TurnRecord{RequestID: "req-1", Tokens: 100})
	ringBuffer.Emit(telemetry.TurnRecord{RequestID: "req-2", Tokens: 200})
	ringBuffer.Emit(telemetry.TurnRecord{RequestID: "req-3", Tokens: 300})

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIRoutes+"?limit=2", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		TotalTracked   int64                  `json:"total_tracked"`
		BufferCapacity int                    `json:"buffer_capacity"`
		Routes         []telemetry.TurnRecord `json:"routes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if resp.TotalTracked != 3 {
		t.Errorf("expected TotalTracked 3, got %d", resp.TotalTracked)
	}
	if len(resp.Routes) != 2 {
		t.Fatalf("expected 2 routes (limit=2), got %d", len(resp.Routes))
	}
	if resp.Routes[0].RequestID != "req-3" || resp.Routes[1].RequestID != "req-2" {
		t.Errorf("unexpected routes ordering: %+v", resp.Routes)
	}
}

func TestAPI_Circuits_GetAndReset(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// Trip ollama circuit breaker
	reg := srv.GetRegistry()
	p, _ := reg.Get("ollama")
	cb := p.(provider.CircuitBreakerProvider).CircuitBreaker()
	cb.RecordFailure()
	cb.RecordFailure() // Trips to open

	// 1. GET /api/v1/circuits
	req := httptest.NewRequest(http.MethodGet, contract.PathAPICircuits, nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		Circuits []provider.CircuitInfo `json:"circuits"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	foundOllama := false
	for _, c := range resp.Circuits {
		if c.Provider == "ollama" {
			foundOllama = true
			if c.State != "open" {
				t.Errorf("expected ollama state 'open', got '%s'", c.State)
			}
			if c.IsAvailable {
				t.Errorf("expected ollama to be unavailable when open")
			}
		}
	}
	if !foundOllama {
		t.Fatalf("ollama circuit not found in response")
	}

	// 2. POST /api/v1/circuits/reset
	resetReqBody := bytes.NewBufferString(`{"provider":"ollama"}`)
	resetReq := httptest.NewRequest(http.MethodPost, contract.PathAPICircuitsReset, resetReqBody)
	resetReq.Header.Set("Authorization", "Bearer test-secret-token")
	resetW := httptest.NewRecorder()
	srv.ServeHTTP(resetW, resetReq)

	if resetW.Code != http.StatusOK {
		t.Fatalf("expected reset status 200, got %d", resetW.Code)
	}

	// Verify breaker is closed again
	if cb.State() != provider.StateClosed {
		t.Errorf("expected breaker state closed after reset, got %s", cb.State())
	}
}

func TestAPI_Config_GetSanitized(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIConfig, nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var cfg contract.Config
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if !strings.Contains(cfg.AuthToken, "***") {
		t.Errorf("expected masked auth token, got '%s'", cfg.AuthToken)
	}
	if !strings.Contains(cfg.Providers["openrouter"].APIKey, "***") {
		t.Errorf("expected masked openrouter API key, got '%s'", cfg.Providers["openrouter"].APIKey)
	}
}

func TestAPI_Config_PutHotReload_And_DryRun(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	_ = os.WriteFile(configPath, []byte("port: 8000\n"), 0600)
	srv.SetConfigPath(configPath)

	// 1. Dry run with valid new expr
	dryRunPayload := `{
		"port": 8000,
		"auth_token": "test-sec***",
		"providers": {
			"ollama": {"base_url": "http://127.0.0.1:11434", "type": "local"},
			"openrouter": {"base_url": "https://openrouter.ai/api/v1", "api_key": "sk-or-***", "type": "cloud"}
		},
		"tiers": [
			{"name": "Tier 1", "provider": "ollama", "when": "Tokens < 16000"}
		]
	}`

	req := httptest.NewRequest(http.MethodPut, contract.PathAPIConfig+"?dry_run=true", bytes.NewBufferString(dryRunPayload))
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected dry run 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify state wasn't modified in dry run
	if len(srv.GetConfig().Tiers) != 2 {
		t.Errorf("expected original 2 tiers, got %d", len(srv.GetConfig().Tiers))
	}

	// 2. Put with invalid expr
	invalidPayload := `{
		"port": 8000,
		"providers": {"ollama": {"base_url": "http://127.0.0.1:11434", "type": "local"}},
		"tiers": [{"name": "Tier 1", "provider": "ollama", "when": "Tokens < 16000 && invalid_var_name"}]
	}`
	req = httptest.NewRequest(http.MethodPut, contract.PathAPIConfig, bytes.NewBufferString(invalidPayload))
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on invalid expr, got %d", w.Code)
	}

	// 3. Real hot-reload
	validPayload := `{
		"port": 8000,
		"auth_token": "test-sec***",
		"providers": {
			"ollama": {"base_url": "http://127.0.0.1:11434", "type": "local"},
			"openrouter": {"base_url": "https://openrouter.ai/api/v1", "api_key": "sk-or-***", "type": "cloud"}
		},
		"tiers": [
			{"name": "Tier 1 New", "provider": "ollama", "when": "Tokens < 24000"}
		]
	}`
	req = httptest.NewRequest(http.MethodPut, contract.PathAPIConfig, bytes.NewBufferString(validPayload))
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on valid hot reload, got %d: %s", w.Code, w.Body.String())
	}

	// Verify new tier is active and secrets were preserved
	if len(srv.GetConfig().Tiers) != 1 || srv.GetConfig().Tiers[0].Name != "Tier 1 New" {
		t.Errorf("expected 1 tier with name 'Tier 1 New', got %+v", srv.GetConfig().Tiers)
	}
	if srv.GetConfig().AuthToken != "test-secret-token" {
		t.Errorf("expected merged auth token to remain 'test-secret-token', got '%s'", srv.GetConfig().AuthToken)
	}
	if srv.GetConfig().Providers["openrouter"].APIKey != "sk-or-v1-real-secret-key-12345" {
		t.Errorf("expected merged openrouter API key to remain 'sk-or-v1-real-secret-key-12345', got '%s'", srv.GetConfig().Providers["openrouter"].APIKey)
	}
}

func TestAPI_Tune_Endpoint(t *testing.T) {
	srv, ringBuffer, _ := setupTestServer(t)

	// Populate some ring buffer turns
	for i := 0; i < 20; i++ {
		ringBuffer.Emit(telemetry.TurnRecord{
			Tokens:   1000 * (i + 1),
			IsLocal:  true,
			IsRetry:  i%3 == 0,
			Keywords: []string{"test"},
		})
	}

	req := httptest.NewRequest(http.MethodPost, contract.PathAPITune, nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/tune, got %d: %s", w.Code, w.Body.String())
	}

	var result tuner.TuningResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode tuning result JSON: %v", err)
	}

	if result.SynthesizedRule == "" {
		t.Errorf("expected non-empty synthesized rule")
	}
}

func TestAPI_Events_SSE_Connection(t *testing.T) {
	srv, _, eventBroker := setupTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIEvents, nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer test-secret-token")

	// Verify endpoint accepts SSE request and initializes stream
	w := httptest.NewRecorder()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = eventBroker.PublishJSON(telemetry.EventRouteCompleted, map[string]string{"test": "data"})
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	srv.ServeHTTP(w, req)

	if w.Header().Get("Content-Type") != contract.ContentTypeEventStream {
		t.Errorf("expected Content-Type text/event-stream, got '%s'", w.Header().Get("Content-Type"))
	}
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "event: route_completed") {
		t.Errorf("expected streamed event 'route_completed', got: %s", bodyStr)
	}
}

func TestAPI_Pricing_Endpoint(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIPricing, nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/pricing, got %d", w.Code)
	}

	var resp struct {
		BenchmarkModel string                            `json:"benchmark_model"`
		Pricing        map[string]telemetry.ModelPricing `json:"pricing"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if resp.BenchmarkModel != contract.DefaultBenchmarkModel {
		t.Errorf("expected benchmark model '%s', got '%s'", contract.DefaultBenchmarkModel, resp.BenchmarkModel)
	}
}

func TestAPI_Config_PutValidationErrors(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	testCases := []struct {
		name    string
		payload string
	}{
		{
			name:    "Invalid JSON",
			payload: `invalid-json`,
		},
		{
			name:    "No Providers",
			payload: `{"providers": {}}`,
		},
		{
			name:    "Missing Base URL",
			payload: `{"providers": {"ollama": {"base_url": "", "type": "local"}}}`,
		},
		{
			name:    "Missing Provider Type",
			payload: `{"providers": {"ollama": {"base_url": "http://localhost:11434"}}}`,
		},
		{
			name:    "Invalid Provider Type",
			payload: `{"providers": {"ollama": {"base_url": "http://localhost:11434", "type": "hybrid"}}}`,
		},
		{
			name:    "Unknown Tier Provider Reference",
			payload: `{"providers": {"ollama": {"base_url": "http://localhost:11434", "type": "local"}}, "tiers": [{"name": "T1", "provider": "nonexistent", "when": "Tokens > 0"}]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, contract.PathAPIConfig, bytes.NewBufferString(tc.payload))
			req.Header.Set("Authorization", "Bearer test-secret-token")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 Bad Request for %s, got %d", tc.name, w.Code)
			}
		})
	}
}

func TestAPI_Circuits_ResetAllAndQueryParam(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// Reset via query parameter ?provider=all
	req := httptest.NewRequest(http.MethodPost, contract.PathAPICircuitsReset+"?provider=all", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}

func TestAPI_Watchdog_AutoRollback(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	initialConfig := srv.GetConfig()

	// Hot reload new config
	newPayload := `{
		"port": 8000,
		"providers": {"ollama": {"base_url": "http://127.0.0.1:11434", "type": "local"}},
		"tiers": [{"name": "New Experimental Tier", "provider": "ollama", "when": "Tokens > 0"}]
	}`
	req := httptest.NewRequest(http.MethodPut, contract.PathAPIConfig, bytes.NewBufferString(newPayload))
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	if srv.GetConfig().Tiers[0].Name != "New Experimental Tier" {
		t.Fatalf("expected active new tier")
	}

	// Simulate 3 consecutive proxy errors to trigger watchdog rollback
	srv.recordProxyError()
	srv.recordProxyError()
	srv.recordProxyError()

	// Verify it automatically rolled back to initialConfig
	if len(srv.GetConfig().Tiers) != len(initialConfig.Tiers) || srv.GetConfig().Tiers[0].Name != initialConfig.Tiers[0].Name {
		t.Errorf("expected watchdog to restore initial tier '%s', got '%s'", initialConfig.Tiers[0].Name, srv.GetConfig().Tiers[0].Name)
	}
}

func TestAPI_MethodsNotAllowed(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	badMethods := []struct {
		path   string
		method string
	}{
		{contract.PathAPIInfo, http.MethodPost},
		{contract.PathAPIEvents, http.MethodPost},
		{contract.PathAPIRoutes, http.MethodPost},
		{contract.PathAPIPricing, http.MethodPost},
		{contract.PathAPICircuitsReset, http.MethodGet},
		{contract.PathAPITune, http.MethodGet},
		{contract.PathAPIConfig, http.MethodDelete},
	}

	for _, bm := range badMethods {
		t.Run(bm.method+" "+bm.path, func(t *testing.T) {
			req := httptest.NewRequest(bm.method, bm.path, nil)
			req.Header.Set("Authorization", "Bearer test-secret-token")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 405 Method Not Allowed for %s %s, got %d", bm.method, bm.path, w.Code)
			}
		})
	}
}

func TestAPI_NilComponents_GracefulHandling(t *testing.T) {
	// Server with nil ringBuffer, eventBroker, tuner, registry, oracle
	cfg := &contract.Config{Port: 8000, AuthToken: "tok"}
	eval, _ := strategy.NewExprEvaluator(nil, contract.Tier{Name: "default"})
	srv := NewServerWithTelemetryAndRegistry(cfg, eval, nil, nil, nil, nil, nil, nil)

	// 1. GET /api/v1/routes with nil ringBuffer
	req := httptest.NewRequest(http.MethodGet, contract.PathAPIRoutes, nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with empty routes on nil ringBuffer, got %d", w.Code)
	}

	// 2. GET /api/v1/circuits with nil registry
	req = httptest.NewRequest(http.MethodGet, contract.PathAPICircuits, nil)
	req.Header.Set("Authorization", "Bearer tok")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with empty circuits on nil registry, got %d", w.Code)
	}

	// 3. POST /api/v1/circuits/reset with nil registry
	req = httptest.NewRequest(http.MethodPost, contract.PathAPICircuitsReset, nil)
	req.Header.Set("Authorization", "Bearer tok")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on nil registry reset, got %d", w.Code)
	}

	// 4. GET /api/v1/pricing with nil oracle
	req = httptest.NewRequest(http.MethodGet, contract.PathAPIPricing, nil)
	req.Header.Set("Authorization", "Bearer tok")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on nil oracle, got %d", w.Code)
	}

	// 5. POST /api/v1/tune (default initialized tuner returns 200, explicit nil returns 503)
	req = httptest.NewRequest(http.MethodPost, contract.PathAPITune, nil)
	req.Header.Set("Authorization", "Bearer tok")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on default tuner, got %d", w.Code)
	}

	srv.SetTuner(nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 on nil tuner, got %d", w.Code)
	}

	// 6. GET /api/v1/events with nil eventBroker
	req = httptest.NewRequest(http.MethodGet, contract.PathAPIEvents, nil)
	req.Header.Set("Authorization", "Bearer tok")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on nil eventBroker, got %d", w.Code)
	}

	// 7. Nil state getters
	emptySrv := &Server{}
	if emptySrv.GetConfig() == nil || emptySrv.GetEvaluator() != nil || emptySrv.GetRegistry() != nil {
		t.Errorf("expected safe fallback from getters on empty server")
	}
}

func TestAPI_Watchdog_TimerExpiry(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// Arm watchdog with very short timeout
	memento := srv.state.Load()
	srv.armWatchdog(memento, 5*time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	// After timer expires without 3 consecutive errors, state remains
	if srv.state.Load() == nil {
		t.Errorf("expected non-nil state after watchdog expiry")
	}
}

type nonFlusherWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (n *nonFlusherWriter) Header() http.Header {
	if n.header == nil {
		n.header = make(http.Header)
	}
	return n.header
}

func (n *nonFlusherWriter) Write(b []byte) (int, error) {
	return n.body.Write(b)
}

func (n *nonFlusherWriter) WriteHeader(statusCode int) {
	n.code = statusCode
}

func TestAPI_Events_NonFlusher(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIEvents, nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := &nonFlusherWriter{}

	srv.ServeHTTP(w, req)
	if w.code != http.StatusInternalServerError {
		t.Errorf("expected 500 when ResponseWriter lacks Flusher, got %d", w.code)
	}
}

func TestAPI_Routes_InvalidLimitParam(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIRoutes+"?limit=invalid_number", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with default limit, got %d", w.Code)
	}
}

func TestAPI_Tune_OptimizerError(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	cancCtx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	req := httptest.NewRequest(http.MethodPost, contract.PathAPITune, nil).WithContext(cancCtx)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when optimizer context is cancelled, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "tuning_error") {
		t.Errorf("expected tuning_error in response body: %s", w.Body.String())
	}
}

func TestAPI_Events_BrokerClosedStream(t *testing.T) {
	srv, _, broker := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIEvents, nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = broker.Close()
	}()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK stream, got %d", w.Code)
	}
}

func TestAPI_Info_CORS_Preflight(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodOptions, contract.PathAPIInfo, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for OPTIONS on /api/v1/info, got %d", w.Code)
	}
}

func TestAPI_Auth_AlternativeHeaders(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// 1. X-API-Key header
	req := httptest.NewRequest(http.MethodGet, contract.PathAPIRoutes, nil)
	req.Header.Set("X-API-Key", "test-secret-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with X-API-Key, got %d", w.Code)
	}

	// 2. api-key header
	req = httptest.NewRequest(http.MethodGet, contract.PathAPIRoutes, nil)
	req.Header.Set("api-key", "test-secret-token")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with api-key header, got %d", w.Code)
	}
}

func TestAPI_AllEndpoints_CORS_Preflights(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	endpoints := []string{
		contract.PathAPIInfo,
		contract.PathAPIEvents,
		contract.PathAPIRoutes,
		contract.PathAPICircuits,
		contract.PathAPICircuitsReset,
		contract.PathAPIPricing,
		contract.PathAPIConfig,
		contract.PathAPITune,
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

func TestAPI_AllEndpoints_MethodNotAllowed(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// Direct handler invocations with unsupported HTTP methods
	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		method  string
		url     string
	}{
		{"handleAPIEvents POST", srv.handleAPIEvents, http.MethodPost, contract.PathAPIEvents},
		{"handleAPIRoutes POST", srv.handleAPIRoutes, http.MethodPost, contract.PathAPIRoutes},
		{"handleAPICircuits DELETE", srv.handleAPICircuits, http.MethodDelete, contract.PathAPICircuits},
		{"handleAPICircuitsReset GET", srv.handleAPICircuitsReset, http.MethodGet, contract.PathAPICircuitsReset},
		{"handleAPIPricing POST", srv.handleAPIPricing, http.MethodPost, contract.PathAPIPricing},
		{"handleAPIConfig DELETE", srv.handleAPIConfig, http.MethodDelete, contract.PathAPIConfig},
		{"handleAPITune GET", srv.handleAPITune, http.MethodGet, contract.PathAPITune},
		{"handleAPIStatsReset GET", srv.handleAPIStatsReset, http.MethodGet, "/api/v1/stats/reset"},
		{"handleAPIStatsRecalculate GET", srv.handleAPIStatsRecalculate, http.MethodGet, "/api/v1/stats/recalculate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			w := httptest.NewRecorder()
			tt.handler(w, req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 405 Method Not Allowed for %s, got %d", tt.name, w.Code)
			}
		})
	}

	// Test malformed config update
	t.Run("handleConfigUpdate malformed JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, contract.PathAPIConfig, strings.NewReader("{bad json"))
		w := httptest.NewRecorder()
		srv.handleConfigUpdate(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", w.Code)
		}
	})
}
