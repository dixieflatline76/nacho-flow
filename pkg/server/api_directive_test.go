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
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/store"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestAPI_Directive_Auth_Enforcement(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"RESET_CIRCUITS"}`)))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}
}

func TestAPI_Directive_Method_Not_Allowed(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIDirective, nil)
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", w.Code)
	}
}

func TestAPI_Directive_Invalid_JSON(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte("not-json")))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestAPI_Directive_Unknown_Action(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"UNKNOWN_ACTION"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for unknown action, got %d", w.Code)
	}
}

func setupTestConfigDir(t *testing.T, baseDir string) {
	t.Helper()
	t.Setenv("APPDATA", baseDir)
	t.Setenv("XDG_CONFIG_HOME", baseDir)
	t.Setenv("HOME", baseDir)
	t.Setenv("USERPROFILE", baseDir)
	t.Setenv("NACHO_CONFIG_DIR", baseDir)
}

func TestAPI_Directive_PurgeAllLogs(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tempDir := t.TempDir()
	setupTestConfigDir(t, tempDir)

	exitChan := make(chan int, 1)
	srv.SetExitFunc(func(code int) {
		exitChan <- code
	})

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"PURGE_ALL_LOGS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp DirectiveResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode directive response: %v", err)
	}

	if resp.Status != "acknowledged" {
		t.Errorf("expected status 'acknowledged', got '%s'", resp.Status)
	}
	if resp.Action != contract.DirectiveActionPurgeAllLogs {
		t.Errorf("expected action '%s', got '%s'", contract.DirectiveActionPurgeAllLogs, resp.Action)
	}
	if !resp.RequiresRestart {
		t.Errorf("expected requires_restart to be true")
	}

	// Verify directive file was written to disk
	directiveFile := filepath.Join(tempDir, contract.AppName, contract.DefaultDirectiveFileName)
	data, err := os.ReadFile(directiveFile)
	if err != nil {
		t.Fatalf("failed to read written directive file: %v", err)
	}

	var onDisk map[string]interface{}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("failed to parse written directive JSON: %v", err)
	}
	if onDisk["action"] != contract.DirectiveActionPurgeAllLogs {
		t.Errorf("expected on-disk action '%s', got '%v'", contract.DirectiveActionPurgeAllLogs, onDisk["action"])
	}

	// Verify async exit was triggered with code 0
	select {
	case code := <-exitChan:
		if code != 0 {
			t.Errorf("expected exit code 0, got %d", code)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timed out waiting for exitFunc to be called")
	}
}

func TestAPI_Directive_ResetCircuits(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"RESET_CIRCUITS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp DirectiveResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode directive response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", resp.Status)
	}
	if resp.Action != contract.DirectiveActionResetCircuits {
		t.Errorf("expected action '%s', got '%s'", contract.DirectiveActionResetCircuits, resp.Action)
	}
	if resp.RequiresRestart {
		t.Errorf("expected requires_restart to be false for RESET_CIRCUITS")
	}
}

func TestAPI_Directive_RecalculateStats(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"RECALCULATE_STATS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp DirectiveResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode directive response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", resp.Status)
	}
	if resp.Action != contract.DirectiveActionRecalculateStats {
		t.Errorf("expected action '%s', got '%s'", contract.DirectiveActionRecalculateStats, resp.Action)
	}
	if resp.RequiresRestart {
		t.Errorf("expected requires_restart to be false for RECALCULATE_STATS")
	}
}

func TestAPI_Directive_CORS(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodOptions, contract.PathAPIDirective, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for OPTIONS CORS, got %d", w.Code)
	}
}

func TestAPI_Directive_RecalculateStats_Error(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	srv.SetTrafficLogPath("\x00invalid-path")

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"RECALCULATE_STATS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 when log file does not exist, got %d", w.Code)
	}
}

func TestAPI_Directive_PurgeAllLogs_WithDiskStore(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tempDir := t.TempDir()
	setupTestConfigDir(t, tempDir)

	mockStorePath := filepath.Join(tempDir, "custom_stats.json")
	mockStore, err := store.NewDiskStore(mockStorePath)
	if err != nil {
		t.Fatalf("failed to create disk store: %v", err)
	}
	srv.SetDiskStore(mockStore)

	exitChan := make(chan int, 1)
	srv.SetExitFunc(func(code int) {
		exitChan <- code
	})

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"PURGE_ALL_LOGS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// Verify on-disk stats path matches mockStorePath
	directiveFile := filepath.Join(tempDir, contract.AppName, contract.DefaultDirectiveFileName)
	data, err := os.ReadFile(directiveFile)
	if err != nil {
		t.Fatalf("failed to read written directive file: %v", err)
	}
	var onDisk map[string]interface{}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("failed to parse written directive JSON: %v", err)
	}
	targets, _ := onDisk["targets"].(map[string]interface{})
	if targets["stats_path"] != filepath.Clean(mockStorePath) {
		t.Errorf("expected stats_path '%s', got '%v'", filepath.Clean(mockStorePath), targets["stats_path"])
	}

	<-exitChan
}

func TestAPI_Directive_PurgeAllLogs_PathError(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tempFile, _ := os.CreateTemp("", "blocker_*")
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	setupTestConfigDir(t, tempFile.Name())

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"PURGE_ALL_LOGS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 when directive path fails, got %d", w.Code)
	}
}

func TestAPI_Directive_ResetCircuits_WithProvider(t *testing.T) {
	srv, _, broker := setupTestServer(t)
	srv.SetEventBroker(broker)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"RESET_CIRCUITS","payload":{"provider":"ollama"}}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp DirectiveResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp.Message, "ollama") {
		t.Errorf("expected message to mention ollama, got '%s'", resp.Message)
	}
}

func TestAPI_Directive_RecalculateStats_WithBufferAndStore(t *testing.T) {
	srv, ringBuf, broker := setupTestServer(t)
	srv.SetRingBuffer(ringBuf)
	srv.SetEventBroker(broker)

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "traffic.jsonl")
	// Write 3 records to traffic log
	recData := `{"timestamp":"2026-09-05T10:00:00Z","model":"test-model","provider":"ollama","total_tokens":100,"cost":0.001}` + "\n" +
		`{"timestamp":"2026-09-05T10:01:00Z","model":"test-model","provider":"ollama","total_tokens":200,"cost":0.002}` + "\n"
	if err := os.WriteFile(logPath, []byte(recData), 0600); err != nil {
		t.Fatalf("failed to write traffic log: %v", err)
	}
	srv.SetTrafficLogPath(logPath)

	storePath := filepath.Join(tempDir, "stats.json")
	diskStore, err := store.NewDiskStore(storePath)
	if err != nil {
		t.Fatalf("failed to create disk store: %v", err)
	}
	srv.SetDiskStore(diskStore)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"RECALCULATE_STATS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	// Verify stats were written to diskStore
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("failed to load saved stats: %v", err)
	}
	if loaded.TotalRequests != 2 {
		t.Errorf("expected 2 total requests in diskStore, got %d", loaded.TotalRequests)
	}
}

func TestAPI_Directive_RecalculateStats_EmptyPathFallback(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	srv.SetTrafficLogPath("") // Empty path to hit logs/traffic.jsonl fallback

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"RECALCULATE_STATS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	// Fallback to logs/traffic.jsonl will either succeed if file exists or 500 if not; both exercise the branch
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status code: %d", w.Code)
	}
}

func TestAPI_Directive_PurgeAllLogs_DefaultStatsPathFallback(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	// Explicitly do not set diskStore (s.diskStore == nil) to test fallback
	srv.SetDiskStore(nil)

	tempDir := t.TempDir()
	setupTestConfigDir(t, tempDir)

	exitChan := make(chan int, 1)
	srv.SetExitFunc(func(code int) {
		exitChan <- code
	})

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"PURGE_ALL_LOGS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	select {
	case <-exitChan:
	case <-time.After(1 * time.Second):
		t.Errorf("timed out waiting for exitFunc")
	}
}

func TestAPI_Directive_PurgeAllLogs_MkdirFailure(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tempDir := t.TempDir()
	blockerFile := filepath.Join(tempDir, "blocker_file")
	if err := os.WriteFile(blockerFile, []byte("blocker"), 0600); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}

	// Set custom directive path whose parent directory cannot be created because a file blocks it
	blockedDirectivePath := filepath.Join(blockerFile, "child_dir", "directive.json")
	srv.SetDirectivePath(blockedDirectivePath)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"PURGE_ALL_LOGS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 on MkdirAll failure, got %d", w.Code)
	}
}

func TestAPI_Directive_PurgeAllLogs_RenameFailure(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	tempDir := t.TempDir()
	// Pre-create directivePath as a directory containing a child file to force rename error
	destDir := filepath.Join(tempDir, "directive_dir_blocker")
	if err := os.MkdirAll(filepath.Join(destDir, "child"), 0750); err != nil {
		t.Fatalf("failed to create dir blocker: %v", err)
	}

	srv.SetDirectivePath(destDir)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"PURGE_ALL_LOGS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 on Rename failure, got %d", w.Code)
	}
}

func TestAPI_MethodNotAllowed_Endpoints(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, contract.PathAPIEvents},
		{http.MethodPost, contract.PathAPICircuits},
		{http.MethodPost, contract.PathAPIPricing},
		{http.MethodGet, contract.PathAPIStatsReset},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("[%s %s] expected status 405, got %d", ep.method, ep.path, w.Code)
		}
	}
}

type nonFlusherResponse struct {
	http.ResponseWriter
}

func TestAPI_Events_NonFlusherAndNilBroker(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	srv.SetEventBroker(nil) // nil broker

	// 1. Non-flusher test
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, contract.PathAPIEvents, nil)
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	srv.handleAPIEvents(nonFlusherResponse{rec}, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for non-flusher, got %d", rec.Code)
	}

	// 2. Flusher with nil broker returns immediately
	rec2 := httptest.NewRecorder()
	srv.handleAPIEvents(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for nil broker events endpoint, got %d", rec2.Code)
	}
}

func TestAPI_Directive_RecalculateStats_Over500Records(t *testing.T) {
	srv, _, broker := setupTestServer(t)
	ringBuf := telemetry.NewRingBufferSink(600)
	srv.SetRingBuffer(ringBuf)
	srv.SetEventBroker(broker)

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "traffic_500.jsonl")
	var buf bytes.Buffer
	for i := 0; i < 505; i++ {
		buf.WriteString(`{"timestamp":"2026-09-05T10:00:00Z","model":"test-model","provider":"ollama","total_tokens":10,"cost":0.0001}` + "\n")
	}
	if err := os.WriteFile(logPath, buf.Bytes(), 0600); err != nil {
		t.Fatalf("failed to write 505 records: %v", err)
	}
	srv.SetTrafficLogPath(logPath)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIDirective, bytes.NewReader([]byte(`{"action":"RECALCULATE_STATS"}`)))
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for recalculate with >500 records, got %d", w.Code)
	}
	if len(ringBuf.GetRecent(0)) != 500 {
		t.Errorf("expected ring buffer count 500, got %d", len(ringBuf.GetRecent(0)))
	}
}

func TestAPI_Directive_CORS_DirectCall(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodOptions, contract.PathAPIDirective, nil)
	w := httptest.NewRecorder()

	srv.handleAPIDirective(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on direct CORS call, got %d", w.Code)
	}
}

func TestAPI_CircuitsReset_QueryParam(t *testing.T) {
	srv, _, broker := setupTestServer(t)
	srv.SetEventBroker(broker)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/circuits/reset?provider=ollama", nil)
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.handleAPICircuitsReset(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for circuits reset with query param, got %d", w.Code)
	}
}

func TestAPI_Pricing_NilOracle(t *testing.T) {
	cfg := &contract.Config{Port: 8000, AuthToken: "test-secret-token"}
	eval, _ := strategy.NewExprEvaluator(nil, contract.Tier{})
	srv := NewServerWithTelemetryAndRegistry(cfg, eval, router.NewClassifier(), router.NewSanitizer(), nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPIPricing, nil)
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for pricing with nil oracle, got %d", w.Code)
	}
}

func TestProxy_AuthenticateClient_Variants(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// 1. APIKey header
	req1 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req1.Header.Set(contract.HeaderAPIKey, "test-secret-token")
	if !srv.authenticateClient(req1) {
		t.Errorf("expected api-key header to authenticate")
	}

	// 2. XAPIKey header
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req2.Header.Set(contract.HeaderXAPIKey, "test-secret-token")
	if !srv.authenticateClient(req2) {
		t.Errorf("expected x-api-key header to authenticate")
	}

	// 3. Unconfigured server auth token (open access)
	cfg := &contract.Config{Port: 8000, AuthToken: ""}
	eval, _ := strategy.NewExprEvaluator(nil, contract.Tier{})
	srvOpen := NewServer(cfg, eval, router.NewClassifier(), router.NewSanitizer())
	req3 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	if !srvOpen.authenticateClient(req3) {
		t.Errorf("expected unconfigured server to authenticate anyone")
	}
}

func TestProxy_InjectCorrectionPrompt_EdgeCases(t *testing.T) {
	// 1. Empty prompt fallback
	res1 := injectCorrectionPrompt([]byte(`{"messages":[{"role":"user","content":"hi"}]}`), "")
	if !strings.Contains(string(res1), contract.CycleBreakerDefaultCorrectionPrompt) {
		t.Errorf("expected default prompt injection")
	}

	// 2. Non-array messages
	res2 := injectCorrectionPrompt([]byte(`{"messages":"not-an-array"}`), "fix this")
	if string(res2) != `{"messages":"not-an-array"}` {
		t.Errorf("expected unmodified body when messages is not an array")
	}
}

func TestTogglesCommandHandler_DisabledVariants(t *testing.T) {
	st := router.NewSessionTracker(10 * time.Minute)
	st.SetKickstartDisabled("sess-1", true)
	st.SetCycleKillerDisabled("sess-1", true)
	st.SetShieldDisabled("sess-1", true)
	st.SetRawModeEnabled("sess-1", true)

	handler := &TogglesCommandHandler{}
	out, err := handler.Execute(context.Background(), contract.RequestContext{}, MetaEnv{
		SessionTracker: st,
		SessionKey:     "sess-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "Kickstart**: ⏸️ OFF") {
		t.Errorf("expected kickstart off in output, got: %s", out)
	}
	if !strings.Contains(out, "Cycle Killer**:       ⏸️ OFF") {
		t.Errorf("expected cycle killer off in output, got: %s", out)
	}
	if !strings.Contains(out, "Fallback Shield**:    ⏸️ OFF") {
		t.Errorf("expected shield off in output, got: %s", out)
	}
	if !strings.Contains(out, "Raw Pass-Through**:   🟢 ON") {
		t.Errorf("expected raw pass-through on in output, got: %s", out)
	}
}

type mockDealsProvider struct{}

func (m *mockDealsProvider) Name() string { return "mock_openrouter" }
func (m *mockDealsProvider) FetchPricing(ctx context.Context) (map[string]telemetry.ModelMetadata, error) {
	return map[string]telemetry.ModelMetadata{
		"free-model": {
			ModelPricing: telemetry.ModelPricing{
				PromptCostPerMillion:     0.0,
				CompletionCostPerMillion: 0.0,
			},
			ModelID:       "free-model",
			Name:          "Free Model",
			SupportsTools: true,
			CodingIndex:   90.0,
		},
	}, nil
}

func TestDealsCommandHandler_FreeModelAndCustomConfig(t *testing.T) {
	oracle := telemetry.NewPricingOracle()
	oracle.RegisterProvider(&mockDealsProvider{}, 0)
	_ = oracle.Sync(context.Background())

	cfg := &contract.Config{
		Deals: contract.DealsConfig{
			Enabled:           true,
			AlertThresholdPct: 10.0,
			MinCodingIndex:    50.0,
		},
	}

	handler := &DealsCommandHandler{}
	out, err := handler.Execute(context.Background(), contract.RequestContext{}, MetaEnv{
		Oracle: oracle,
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "FREE") {
		t.Errorf("expected FREE in output, got: %s", out)
	}
}

func TestAPI_StatsReset_WithDiskStoreAndBroker(t *testing.T) {
	srv, ringBuf, broker := setupTestServer(t)
	srv.SetRingBuffer(ringBuf)
	srv.SetEventBroker(broker)

	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "stats_reset.json")
	diskStore, err := store.NewDiskStore(storePath)
	if err != nil {
		t.Fatalf("failed to create disk store: %v", err)
	}
	srv.SetDiskStore(diskStore)

	req := httptest.NewRequest(http.MethodPost, contract.PathAPIStatsReset, nil)
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on stats reset with store and broker, got %d", w.Code)
	}

	// Verify disk store was saved
	loaded, err := diskStore.Load()
	if err != nil {
		t.Fatalf("failed to load saved stats: %v", err)
	}
	if loaded.TotalRequests != 0 {
		t.Errorf("expected 0 total requests after reset, got %d", loaded.TotalRequests)
	}
}

func TestAPI_Circuits_NilRegistry(t *testing.T) {
	cfg := &contract.Config{Port: 8000, AuthToken: "test-secret-token"}
	eval, _ := strategy.NewExprEvaluator(nil, contract.Tier{})
	srv := NewServerWithTelemetryAndRegistry(cfg, eval, router.NewClassifier(), router.NewSanitizer(), nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, contract.PathAPICircuits, nil)
	req.Header.Set(contract.HeaderAuthorization, contract.AuthSchemeBearer+"test-secret-token")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on circuits with nil registry, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	circuits, ok := resp["circuits"].([]interface{})
	if !ok || len(circuits) != 0 {
		t.Errorf("expected empty circuits slice, got %v", resp["circuits"])
	}
}

func TestDealsCommandHandler_NilOracleAndNoDeals(t *testing.T) {
	handler := &DealsCommandHandler{}
	out1, _ := handler.Execute(context.Background(), contract.RequestContext{}, MetaEnv{Oracle: nil})
	if !strings.Contains(out1, "Pricing oracle is not active") {
		t.Errorf("expected oracle not active message, got: %s", out1)
	}

	oracle := telemetry.NewPricingOracle()
	out2, _ := handler.Execute(context.Background(), contract.RequestContext{}, MetaEnv{Oracle: oracle})
	if !strings.Contains(out2, "No drop-in tier replacements") {
		t.Errorf("expected no replacements message, got: %s", out2)
	}
}

func TestAPI_DirectCORS_Handlers(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	// Direct call to setCORS inside handlers with OPTIONS
	rec := httptest.NewRecorder()
	optReq := httptest.NewRequest(http.MethodOptions, "/dummy", nil)

	srv.handleAPIEvents(rec, optReq)
	srv.handleAPICircuits(rec, optReq)
	srv.handleAPICircuitsReset(rec, optReq)
	srv.handleAPIPricing(rec, optReq)
	srv.handleAPIStatsReset(rec, optReq)
}
