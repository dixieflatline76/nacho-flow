package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

type mockTuningRunner struct {
	called         bool
	cfgPathPassed  string
	logPathPassed  string
	resultToReturn *tuner.TuningResult
	errToReturn    error
}

func (m *mockTuningRunner) RunTuning(ctx context.Context, configPath string, trafficLogPath string) (*tuner.TuningResult, error) {
	m.called = true
	m.cfgPathPassed = configPath
	m.logPathPassed = trafficLogPath
	return m.resultToReturn, m.errToReturn
}

func TestHandleAPITune_DelegatesToRunnerAndFlushes(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "traffic.jsonl")
	cfgPath := filepath.Join(tempDir, "config.yaml")

	logger, err := telemetry.NewTrafficLogger(logPath, 100)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	srv := &Server{
		configPath:     cfgPath,
		trafficLogPath: logPath,
		trafficLogger:  logger,
	}

	expectedResult := &tuner.TuningResult{
		TotalSessions: 7,
		Tiers: []tuner.TierTuningResult{
			{TierName: "Local GPU", OptimalThreshold: 4500},
		},
	}

	mockRunner := &mockTuningRunner{
		resultToReturn: expectedResult,
	}
	srv.SetTuningRunner(mockRunner)

	// Emit a record before calling API
	logger.Emit(telemetry.TurnRecord{
		RequestID:    "req-flush-test",
		SessionID:    "sess-flush",
		SelectedTier: "Local GPU",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tune", nil)
	w := httptest.NewRecorder()

	srv.handleAPITune(w, req)

	if !mockRunner.called {
		t.Fatalf("Expected mockTuningRunner to be called")
	}
	if mockRunner.cfgPathPassed != cfgPath {
		t.Errorf("Expected config path %s, got %s", cfgPath, mockRunner.cfgPathPassed)
	}
	if mockRunner.logPathPassed != logPath {
		t.Errorf("Expected log path %s, got %s", logPath, mockRunner.logPathPassed)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var res tuner.TuningResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode response JSON: %v", err)
	}
	if res.TotalSessions != 7 {
		t.Errorf("Expected 7 total sessions, got %d", res.TotalSessions)
	}

	// Verify that flush happened: record must be present on disk
	diskRecords, err := telemetry.ReadRecords(logPath, 0)
	if err != nil {
		t.Fatalf("Failed to read disk records: %v", err)
	}
	if len(diskRecords) != 1 {
		t.Errorf("Expected 1 flushed record on disk, got %d", len(diskRecords))
	}
}

func TestHandleAPITune_RunnerErrorReturns500(t *testing.T) {
	srv := &Server{}
	mockRunner := &mockTuningRunner{
		errToReturn: errors.New("child worker crashed"),
	}
	srv.SetTuningRunner(mockRunner)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tune", nil)
	w := httptest.NewRecorder()

	srv.handleAPITune(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("Expected 500 Internal Server Error, got %d", w.Code)
	}
	if !mockRunner.called {
		t.Errorf("Expected mock runner to be called")
	}
}

func TestInProcessTuningRunner(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "traffic.jsonl")
	cfgPath := filepath.Join(tempDir, "config.yaml")

	logger, err := telemetry.NewTrafficLogger(logPath, 100)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Emit(telemetry.TurnRecord{
		Timestamp:    time.Now().UTC(),
		RequestID:    "req-1",
		SessionID:    "sess-1",
		SelectedTier: "Local",
	})
	_ = logger.Close()

	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"local": {BaseURL: "http://localhost:11434", Type: "local"},
		},
		Tiers: []contract.Tier{
			{Name: "Local", Provider: "local", Model: "qwen2.5-coder:14b"},
		},
	}

	srv := &Server{
		tuner: tuner.NewMinConflictsOptimizer(tuner.DefaultTuningPolicy()),
	}
	srv.state.Store(&runtimeState{config: cfg})

	runner := NewInProcessTuningRunner(srv)
	ctx := context.Background()

	result, err := runner.RunTuning(ctx, cfgPath, logPath)
	if err != nil {
		t.Fatalf("InProcessTuningRunner failed: %v", err)
	}
	if result == nil {
		t.Fatalf("Expected non-nil TuningResult")
	}
	if result.TotalSessions != 1 {
		t.Errorf("Expected 1 session, got %d", result.TotalSessions)
	}
}

func TestProcessTuningRunner_ContextCancelled(t *testing.T) {
	// Point to executable or non-existent command with already-cancelled context
	runner := NewProcessTuningRunner("cmd.exe", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := runner.RunTuning(ctx, "config.yaml", "traffic.jsonl")
	if err == nil {
		t.Fatalf("Expected error for cancelled context, got nil")
	}
}
