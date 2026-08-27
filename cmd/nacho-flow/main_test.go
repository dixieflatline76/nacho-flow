package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
	"github.com/kardianos/service"
)

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestVersionOutput(t *testing.T) {
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		*versionFlag = false
		*vFlag = false
	}()

	for _, flagArg := range []string{"version", "-v", "--version"} {
		os.Args = []string{"nacho-flow", flagArg}
		out := captureStdout(func() {
			main()
		})
		expected := "nacho-flow " + contract.Version
		if !bytes.Contains([]byte(out), []byte(expected)) {
			t.Errorf("Flag %s: Expected %q in output, got %q", flagArg, expected, out)
		}
	}
}

func TestParseLogLevel(t *testing.T) {
	cases := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}

	for _, c := range cases {
		got := parseLogLevel(c.input)
		if got != c.expected {
			t.Errorf("parseLogLevel(%q) = %v; want %v", c.input, got, c.expected)
		}
	}
}

func TestProgram_Stop_HandlesNilAndInitialized(t *testing.T) {
	p := &program{}
	if err := p.Stop(nil); err != nil {
		t.Errorf("Expected nil error stopping uninitialized program, got: %v", err)
	}

	// Initialize tracker and traffic logger
	tracker := telemetry.NewStatsTracker(100)
	trafficLog, err := telemetry.NewTrafficLogger(filepath.Join(t.TempDir(), "traffic.jsonl"), 100)
	if err != nil {
		t.Fatalf("NewTrafficLogger failed: %v", err)
	}

	p2 := &program{
		tracker:    tracker,
		trafficLog: trafficLog,
		cancelBg:   func() {},
	}

	if err := p2.Stop(nil); err != nil {
		t.Errorf("Expected nil error stopping initialized program, got: %v", err)
	}
}

func TestHandleTuneSubcommand_EmptyTrafficLog(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"
tiers:
  - name: "Local"
    provider: "ollama"
    when: "Tokens < 8000"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	trafficLogPath := filepath.Join(tmpDir, "traffic.jsonl")
	// Create empty file
	if err := os.WriteFile(trafficLogPath, []byte(""), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	out := captureStdout(func() {
		handleTuneSubcommand([]string{"-config", cfgPath, "-traffic-log", trafficLogPath})
	})

	if !bytes.Contains([]byte(out), []byte("No historical traffic records found")) {
		t.Errorf("Expected 'No historical traffic records found' in output, got: %s", out)
	}
}

func TestHandleTuneSubcommand_WithRecordsAndApply(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434/v1"
    type: "local"
tiers:
  - name: "Local Fast"
    provider: "ollama"
    when: "Tokens < 4000"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	trafficLogPath := filepath.Join(tmpDir, "traffic.jsonl")
	tl, err := telemetry.NewTrafficLogger(trafficLogPath, 100)
	if err != nil {
		t.Fatalf("NewTrafficLogger failed: %v", err)
	}

	// Write 5 sample records
	for i := 0; i < 5; i++ {
		tl.Emit(telemetry.TurnRecord{
			Timestamp: time.Now(),
			Tokens:    1000 + i*500,
			IsLocal:   true,
			IsRetry:   false,
		})
	}
	_ = tl.Close()

	out := captureStdout(func() {
		handleTuneSubcommand([]string{"-config", cfgPath, "-traffic-log", trafficLogPath, "-apply"})
	})

	if !bytes.Contains([]byte(out), []byte("NACHO FLOW ADVISORY TUNING REPORT")) {
		t.Errorf("Expected advisory report in output, got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("SUCCESS: Successfully updated")) {
		t.Errorf("Expected success confirmation in output, got: %s", out)
	}
}

func TestRunTune_ApplyError(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0600)

	trafficLogPath := filepath.Join(tmpDir, "traffic.jsonl")
	tl, err := telemetry.NewTrafficLogger(trafficLogPath, 100)
	if err != nil {
		t.Fatalf("NewTrafficLogger failed: %v", err)
	}
	tl.Emit(telemetry.TurnRecord{
		Timestamp: time.Now(),
		Tokens:    1000,
		IsLocal:   true,
	})
	_ = tl.Close()

	origApply := applyTuningFunc
	applyTuningFunc = func(configPath string, result *tuner.TuningResult) (string, error) {
		return "", fmt.Errorf("simulated apply error")
	}
	defer func() { applyTuningFunc = origApply }()

	err = runTune([]string{"-config", cfgPath, "-traffic-log", trafficLogPath, "-apply"})
	if err == nil {
		t.Errorf("expected error when applyTuningFunc fails, got nil")
	}
}

type mockService struct{}

func (m *mockService) Start() error                                           { return nil }
func (m *mockService) Stop() error                                            { return nil }
func (m *mockService) Restart() error                                         { return nil }
func (m *mockService) Install() error                                         { return nil }
func (m *mockService) Uninstall() error                                       { return nil }
func (m *mockService) Logger(errs chan<- error) (service.Logger, error)       { return nil, nil }
func (m *mockService) SystemLogger(errs chan<- error) (service.Logger, error) { return nil, nil }
func (m *mockService) Run() error                                             { return nil }
func (m *mockService) String() string                                         { return "mock" }
func (m *mockService) Platform() string                                       { return "mock" }
func (m *mockService) Status() (service.Status, error)                        { return service.StatusRunning, nil }

func TestProgram_FullLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: 59998
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
    type: "local"
tiers:
  - name: "Local"
    provider: "ollama"
    when: "Tokens < 8000"
default_tier:
  name: "Fallback"
  provider: "ollama"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	*configPathFlag = cfgPath
	p := &program{}

	mock := &mockService{}
	if err := p.Start(mock); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Poll until server is responding on 59998
	ready := false
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get("http://127.0.0.1:59998/api/v1/info")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			ready = true
			break
		}
	}
	if !ready {
		t.Fatalf("HTTP server failed to start within timeout")
	}

	// Graceful Stop
	if err := p.Stop(mock); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestProgram_FullLifecycle_WithOpenRouter(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: 59981
providers:
  openrouter:
    base_url: "http://127.0.0.1:11434"
    api_key: "sk-or-test"
tiers:
  - name: "Cloud"
    provider: "openrouter"
    when: "Tokens < 8000"
default_tier:
  name: "Fallback"
  provider: "openrouter"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	*configPathFlag = cfgPath
	*portFlag = 59981
	p := &program{}

	mock := &mockService{}
	if err := p.Start(mock); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ready := false
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get("http://127.0.0.1:59981/api/v1/info")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			ready = true
			break
		}
	}
	if !ready {
		t.Fatalf("HTTP server failed to start within timeout")
	}

	if err := p.Stop(mock); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	*portFlag = 0
	*configPathFlag = ""
}

func TestMain_Flags(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	if flag.Usage != nil {
		flag.Usage()
	}

	// 1. Version flag
	*versionFlag = true
	out := captureStdout(func() {
		main()
	})
	*versionFlag = false
	if !bytes.Contains([]byte(out), []byte("nacho-flow")) {
		t.Errorf("expected version output, got: %s", out)
	}

	// 2. Tune subcommand dispatch
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	validCfg := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
`
	_ = os.WriteFile(cfgPath, []byte(validCfg), 0600)
	trafficLogPath := filepath.Join(tmpDir, "traffic.jsonl")
	_ = os.WriteFile(trafficLogPath, []byte(""), 0600)

	os.Args = []string{"nacho-flow", "tune", "-config", cfgPath, "-traffic-log", trafficLogPath}
	out = captureStdout(func() {
		main()
	})
	if !bytes.Contains([]byte(out), []byte("No historical traffic records found")) {
		t.Errorf("expected tune output, got: %s", out)
	}

	// 3. Command positional versions
	for _, vCmd := range []string{"version", "-v", "--version"} {
		os.Args = []string{"nacho-flow", vCmd}
		out = captureStdout(func() {
			main()
		})
		if !bytes.Contains([]byte(out), []byte("nacho-flow")) {
			t.Errorf("expected version output for %s, got: %s", vCmd, out)
		}
	}
}

func TestHandleTuneSubcommand_Flags(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
tiers:
  - name: "Local"
    provider: "ollama"
    when: "Tokens < 8000"
`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0600)
	trafficLogPath := filepath.Join(tmpDir, "traffic.jsonl")
	tl, _ := telemetry.NewTrafficLogger(trafficLogPath, 100)
	tl.Emit(telemetry.TurnRecord{Timestamp: time.Now(), Tokens: 1000, IsLocal: true})
	_ = tl.Close()

	// Test with sample limit
	out := captureStdout(func() {
		handleTuneSubcommand([]string{"-config", cfgPath, "-traffic-log", trafficLogPath, "-sample", "10"})
	})
	if !bytes.Contains([]byte(out), []byte("NACHO FLOW ADVISORY TUNING REPORT")) {
		t.Errorf("expected advisory report, got: %s", out)
	}

	// Test with apply
	out = captureStdout(func() {
		err := runTune([]string{"-config", cfgPath, "-traffic-log", trafficLogPath, "-apply"})
		if err != nil {
			t.Errorf("expected nil error on apply, got: %v", err)
		}
	})
	if !bytes.Contains([]byte(out), []byte("SUCCESS")) {
		t.Errorf("expected success message on apply, got: %s", out)
	}

	// Error path 1: invalid flag
	if err := runTune([]string{"-invalid-flag"}); err == nil {
		t.Errorf("expected error for invalid flag, got nil")
	}

	// Error path 2: missing config
	if err := runTune([]string{"-config", filepath.Join(tmpDir, "nonexistent.yaml")}); err == nil {
		t.Errorf("expected error for missing config, got nil")
	}

	// Error path 3: bad optimizer analysis or config
	badCfgPath := filepath.Join(tmpDir, "bad_cfg_for_tune.yaml")
	_ = os.WriteFile(badCfgPath, []byte("port: 8000\nproviders:\n  p1:\n    base_url: ''\ntiers:\n  - name: 'T1'\n    provider: 'p1'\n    when: 'invalid ???'\n"), 0600)
	if err := runTune([]string{"-config", badCfgPath, "-traffic-log", trafficLogPath}); err == nil {
		t.Errorf("expected error for bad tier expression in tune, got nil")
	}

	// Error path 4: unreadable traffic log (is a directory)
	dirLogPath := filepath.Join(tmpDir, "dir_as_log")
	_ = os.Mkdir(dirLogPath, 0755)
	if err := runTune([]string{"-config", cfgPath, "-traffic-log", dirLogPath}); err == nil {
		t.Errorf("expected error when traffic log is a directory, got nil")
	}
}

func TestHandleServiceControl(t *testing.T) {
	mock := &mockService{}

	// 1. Not a service command
	handled, err := handleServiceControl(mock, []string{"nacho-flow", "run"})
	if handled || err != nil {
		t.Errorf("expected false/nil for non-service command, got handled=%v, err=%v", handled, err)
	}

	// 2. Service command without subcommand
	handled, err = handleServiceControl(mock, []string{"nacho-flow", "service"})
	if handled || err != nil {
		t.Errorf("expected false/nil for incomplete service command, got handled=%v, err=%v", handled, err)
	}

	// 3. Service command with valid mock subcommand
	handled, err = handleServiceControl(mock, []string{"nacho-flow", "service", "start"})
	if !handled || err != nil {
		t.Errorf("expected true/nil for service start, got handled=%v, err=%v", handled, err)
	}

	// 4. Svc command alias with valid stop
	handled, err = handleServiceControl(mock, []string{"nacho-flow", "svc", "stop"})
	if !handled || err != nil {
		t.Errorf("expected true/nil for svc stop, got handled=%v, err=%v", handled, err)
	}

	// 5. Invalid action returns handled=true with error
	handled, err = handleServiceControl(mock, []string{"nacho-flow", "service", "invalid_action"})
	if !handled || err == nil {
		t.Errorf("expected true/error for invalid action, got handled=%v, err=%v", handled, err)
	}
}

func TestProgram_Run_ErrorPaths(t *testing.T) {
	mock := &mockService{}

	// 1. Missing config path in interactive mode prints help and returns nil
	*configPathFlag = filepath.Join(t.TempDir(), "nonexistent.yaml")
	p := &program{}
	out := captureStdout(func() {
		_ = p.run(mock)
	})
	if !bytes.Contains([]byte(out), []byte("No configuration file found")) {
		t.Errorf("expected interactive help message, got: %s", out)
	}

	// 2. Invalid tier expr returns compile error
	tmpDir := t.TempDir()
	badCfgPath := filepath.Join(tmpDir, "bad_cfg.yaml")
	badCfg := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
tiers:
  - name: "Bad Tier"
    provider: "ollama"
    when: "invalid expression syntax !!!"
`
	_ = os.WriteFile(badCfgPath, []byte(badCfg), 0600)
	*configPathFlag = badCfgPath
	p2 := &program{}
	err := p2.run(mock)
	if err == nil {
		t.Errorf("expected error for bad tier expression, got nil")
	}
}

func TestProgram_Start_GoroutineErrorLogging(t *testing.T) {
	mock := &mockService{}
	tmpDir := t.TempDir()
	badCfgPath := filepath.Join(tmpDir, "bad.yaml")
	cfgContent := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
tiers:
  - name: "Bad Tier"
    provider: "ollama"
    when: "invalid tier expression syntax !!!"
`
	_ = os.WriteFile(badCfgPath, []byte(cfgContent), 0600)
	*configPathFlag = badCfgPath

	// 1. Test asyncRun with slog present
	p := &program{
		slog: slog.Default(),
	}
	p.asyncRun(mock)

	// 2. Test asyncRun with slog nil
	pNoLog := &program{}
	pNoLog.asyncRun(mock)

	// 3. Test Start method
	if err := p.Start(mock); err != nil {
		t.Errorf("expected nil error from Start, got: %v", err)
	}
	*configPathFlag = ""
}

func TestProgram_Run_InvalidPort(t *testing.T) {
	mock := &mockService{}
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: -1
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0600)
	*configPathFlag = cfgPath

	p := &program{}
	err := p.run(mock)
	if err == nil {
		t.Errorf("expected error for invalid port -1, got nil")
	}
	*configPathFlag = ""
}

func TestSetupShutdownSignal(t *testing.T) {
	mock := &mockService{}
	sigChan := setupShutdownSignal(mock)
	sigChan <- os.Interrupt
	time.Sleep(50 * time.Millisecond)
	close(sigChan)
}

func TestProgram_Run_PortOverrideAndConfigFallback(t *testing.T) {
	mock := &mockService{}
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0600)

	*configPathFlag = cfgPath
	*portFlag = 59996 // Override port
	*logLevelFlag = "warn"

	p := &program{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.run(mock)
	}()

	for i := 0; i < 40; i++ {
		p.mu.Lock()
		srv := p.server
		p.mu.Unlock()
		if srv != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = p.Stop(mock)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("program.run timed out on Stop")
	}

	*portFlag = 0
	*configPathFlag = ""
}

func TestRunMain_AllBranches(t *testing.T) {
	*versionFlag = false
	*vFlag = false
	defer func() {
		*versionFlag = false
		*vFlag = false
	}()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	validCfg := `
port: 8000
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
`
	_ = os.WriteFile(cfgPath, []byte(validCfg), 0600)
	trafficLogPath := filepath.Join(tmpDir, "traffic.jsonl")
	_ = os.WriteFile(trafficLogPath, []byte(""), 0600)

	// 1. Version commands
	out := captureStdout(func() {
		_ = runMain([]string{"nacho-flow", "version"}, nil)
		_ = runMain([]string{"nacho-flow", "-v"}, nil)
		_ = runMain([]string{"nacho-flow", "--version"}, nil)
	})
	if !bytes.Contains([]byte(out), []byte("nacho-flow")) {
		t.Errorf("expected version output, got: %s", out)
	}

	// 2. Version flag
	*versionFlag = true
	out = captureStdout(func() {
		_ = runMain([]string{"nacho-flow"}, nil)
	})
	*versionFlag = false
	if !bytes.Contains([]byte(out), []byte("nacho-flow")) {
		t.Errorf("expected version flag output, got: %s", out)
	}

	// 3. Tune subcommand
	out = captureStdout(func() {
		_ = runMain([]string{"nacho-flow", "tune", "-config", cfgPath, "-traffic-log", trafficLogPath}, nil)
	})
	if !bytes.Contains([]byte(out), []byte("No historical traffic records found")) {
		t.Errorf("expected tune output, got: %s", out)
	}

	// 4. Custom serviceRunner execution
	runnerCalled := false
	err := runMain([]string{"nacho-flow"}, func(s service.Service) error {
		runnerCalled = true
		return nil
	})
	if err != nil || !runnerCalled {
		t.Errorf("expected serviceRunner to be called with nil error, got err=%v, called=%v", err, runnerCalled)
	}

	// 5. Service control command with valid start
	origServiceControl := serviceControlRunner
	serviceControlCalled := false
	serviceControlRunner = func(s service.Service, args []string) (bool, error) {
		serviceControlCalled = true
		return true, nil
	}
	_ = runMain([]string{"nacho-flow", "service", "start"}, nil)
	serviceControlRunner = origServiceControl
	if !serviceControlCalled {
		t.Errorf("expected serviceControlRunner to be called")
	}

	// 6. Tune subcommand with error
	err = runMain([]string{"nacho-flow", "tune", "-invalid-flag-for-tune"}, nil)
	if err == nil {
		t.Errorf("expected error from invalid tune flag, got nil")
	}

	// 7. Default runMain with serviceRunner nil
	origDefaultRunner := defaultServiceRunner
	defaultRunnerCalled := false
	defaultServiceRunner = func(s service.Service) error {
		defaultRunnerCalled = true
		return nil
	}
	defer func() { defaultServiceRunner = origDefaultRunner }()

	_ = runMain([]string{"nacho-flow"}, nil)
	if !defaultRunnerCalled {
		t.Errorf("expected defaultServiceRunner to be called")
	}
}

func TestProgram_Start_NormalExecution(t *testing.T) {
	mock := &mockService{}
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	validCfg := `
port: 59995
providers:
  mock_provider:
    api_key: "sk-test-key"
    base_url: "http://127.0.0.1:11434"
`
	_ = os.WriteFile(cfgPath, []byte(validCfg), 0600)
	*configPathFlag = cfgPath

	p := &program{}
	if err := p.Start(mock); err != nil {
		t.Errorf("expected nil error from Start, got: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	_ = p.Stop(mock)
	*configPathFlag = ""
}

func TestProgram_Run_OpenRouterAndStatsDiskStore(t *testing.T) {
	mock := &mockService{}
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `
port: 59994
providers:
  openrouter:
    api_key: "sk-or-live-key"
    base_url: "http://127.0.0.1:59993/v1"
`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0600)
	*configPathFlag = cfgPath
	p := &program{
		statsSyncInterval: 10 * time.Millisecond,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.run(mock)
	}()

	for i := 0; i < 40; i++ {
		p.mu.Lock()
		srv := p.server
		p.mu.Unlock()
		if srv != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = p.Stop(mock)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("program.run timed out on Stop")
	}
	*configPathFlag = ""
}

func TestHandleTuneSubcommand_ErrorPath(t *testing.T) {
	origLogFatal := logFatal
	fatalCalled := false
	logFatal = func(v ...any) {
		fatalCalled = true
	}
	defer func() { logFatal = origLogFatal }()

	// Trigger error path by passing invalid flag
	handleTuneSubcommand([]string{"-invalid-tune-flag"})
	if !fatalCalled {
		t.Errorf("expected logFatal to be called on invalid tune flag")
	}
}

func TestMain_ErrorPath(t *testing.T) {
	origLogFatal := logFatal
	fatalCalled := false
	logFatal = func(v ...any) {
		fatalCalled = true
	}
	defer func() { logFatal = origLogFatal }()

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Trigger main() error by passing invalid tune flag (fails immediately with 0 network/SCM)
	os.Args = []string{"nacho-flow", "tune", "-invalid-flag-for-main"}
	main()
	if !fatalCalled {
		t.Errorf("expected logFatal to be called on invalid tune flag in main")
	}
}

func TestRunMain_ServiceNewError(t *testing.T) {
	origServiceNew := serviceNewFunc
	serviceNewFunc = func(i service.Interface, c *service.Config) (service.Service, error) {
		return nil, fmt.Errorf("simulated service.New error")
	}
	defer func() { serviceNewFunc = origServiceNew }()

	err := runMain([]string{"nacho-flow"}, nil)
	if err == nil {
		t.Errorf("expected error from failed service.New, got nil")
	}
}

func TestProgram_Run_DaemonModeConfigError(t *testing.T) {
	origInteractive := serviceInteractiveFunc
	serviceInteractiveFunc = func() bool { return false }
	defer func() { serviceInteractiveFunc = origInteractive }()

	mock := &mockService{}
	*configPathFlag = "non_existent_config_file_12345.yaml"
	defer func() { *configPathFlag = "" }()

	p := &program{}
	err := p.run(mock)
	if err == nil {
		t.Errorf("expected error in daemon mode when config file does not exist, got nil")
	}
}
