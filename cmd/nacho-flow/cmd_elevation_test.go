package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/server"
)

func TestFetchDeals_ErrorCases(t *testing.T) {
	// 1. HTTP 500 Internal Server Error
	tsError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal daemon error", http.StatusInternalServerError)
	}))
	defer tsError.Close()

	_, err := fetchDeals(tsError.URL, "test-api-key")
	if err == nil {
		t.Fatalf("expected error for HTTP 500, got nil")
	}

	// 2. HTTP 200 with invalid JSON
	tsBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer tsBadJSON.Close()

	_, err = fetchDeals(tsBadJSON.URL, "")
	if err == nil {
		t.Fatalf("expected error for invalid JSON, got nil")
	}

	// 3. Network connection refused
	_, err = fetchDeals("http://127.0.0.1:59999", "")
	if err == nil {
		t.Fatalf("expected network error connecting to invalid daemon, got nil")
	}
}

func TestIsAddressInUse_DetailedVariants(t *testing.T) {
	// 1. Nil error
	if isAddressInUse(nil) {
		t.Errorf("expected false for nil error")
	}

	// 2. Direct syscall.EADDRINUSE
	if !isAddressInUse(syscall.EADDRINUSE) {
		t.Errorf("expected true for direct syscall.EADDRINUSE")
	}

	// 3. Nested net.OpError with WSAEADDRINUSE (10048)
	opErrWin := &net.OpError{
		Op:  "listen",
		Net: "tcp",
		Err: &os.SyscallError{
			Syscall: "bind",
			Err:     syscall.Errno(10048),
		},
	}
	if !isAddressInUse(opErrWin) {
		t.Errorf("expected true for Windows 10048 WSAEADDRINUSE")
	}

	// 4. Nested net.OpError with standard EADDRINUSE
	opErrUnix := &net.OpError{
		Op:  "listen",
		Net: "tcp",
		Err: &os.SyscallError{
			Syscall: "bind",
			Err:     syscall.EADDRINUSE,
		},
	}
	if !isAddressInUse(opErrUnix) {
		t.Errorf("expected true for Unix EADDRINUSE")
	}

	// 5. Random other error
	otherErr := errors.New("something else completely")
	if isAddressInUse(otherErr) {
		t.Errorf("expected false for generic error")
	}
}

func TestDealsReporter_TableReporter_EmptyDeals(t *testing.T) {
	reporter := NewTableReporter()
	emptyResp := server.DealsResponse{
		BenchmarkModel:    "claude-3-5-sonnet",
		BenchmarkCostPerM: 3.00,
		DealsCount:        0,
		Deals:             []contract.DealInfo{},
	}

	var buf bytes.Buffer
	err := reporter.Render(&buf, emptyResp)
	if err != nil {
		t.Fatalf("unexpected error rendering empty deals: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("claude-3-5-sonnet")) {
		t.Errorf("expected benchmark model in table output, got: %s", buf.String())
	}
}

func captureStderrLocal(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestProgram_AsyncRun_Execution(t *testing.T) {
	oldInteractive := serviceInteractiveFunc
	oldConfig := *configPathFlag
	origDir, _ := os.Getwd()
	defer func() {
		serviceInteractiveFunc = oldInteractive
		*configPathFlag = oldConfig
		_ = os.Chdir(origDir)
	}()

	// Run from a temp dir so the log file lands somewhere clean
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)

	serviceInteractiveFunc = func() bool { return false }
	*configPathFlag = "/invalid/nonexistent/config.yaml"

	var p *program
	errOutput := captureStderrLocal(func() {
		p = &program{}
		p.asyncRun(nil)
	})
	// Close the lumberjack log rotator so TempDir cleanup can delete the file
	if p.logCloser != nil {
		_ = p.logCloser.Close()
	}

	// Assert run() emitted the config error to stderr
	if !strings.Contains(errOutput, "[FATAL:CONFIG_ERROR]") {
		t.Errorf("expected stderr to contain '[FATAL:CONFIG_ERROR]', got: %s", errOutput)
	}
	// Assert asyncRun forwarded the error through slog to the log file (non-interactive writes to disk)
	logFile := filepath.Join(tmpDir, "logs", "router.log")
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("expected log file at %s, got error: %v", logFile, err)
	}
	if !strings.Contains(string(logData), "Fatal runtime error") {
		t.Errorf("expected log file to contain 'Fatal runtime error', got: %s", string(logData))
	}
}

func TestTableReporter_Render_FlushError(t *testing.T) {
	reporter := NewTableReporter()
	resp := server.DealsResponse{
		BenchmarkModel:    "claude-3-5-sonnet",
		BenchmarkCostPerM: 3.00,
		DealsCount:        1,
		Deals: []contract.DealInfo{
			{
				ModelID:        "openrouter/deepseek/deepseek-chat",
				PromptCostPerM: 0.14,
			},
		},
	}
	err := reporter.Render(&errWriter{}, resp)
	if err == nil {
		t.Fatalf("expected error from TableReporter.Render with failing writer, got nil")
	}
}

func TestProgram_Run_InvalidEvaluatorRule(t *testing.T) {
	oldInteractive := serviceInteractiveFunc
	oldConfig := *configPathFlag
	defer func() {
		serviceInteractiveFunc = oldInteractive
		*configPathFlag = oldConfig
	}()

	serviceInteractiveFunc = func() bool { return false }
	tmpDir := t.TempDir()
	invalidConfigPath := filepath.Join(tmpDir, "invalid_rule.yaml")
	invalidYAML := `
port: 8999
tiers:
  - name: "Broken Rule Tier"
    provider: "ollama"
    model: "qwen"
    when: "invalid syntax %% syntax error"
default_tier:
  name: "Default"
  provider: "ollama"
  model: "qwen"
`
	_ = os.WriteFile(invalidConfigPath, []byte(invalidYAML), 0600)
	*configPathFlag = invalidConfigPath

	p := &program{}
	err := p.run(nil)
	if err == nil {
		t.Fatalf("expected error for invalid rule syntax in config, got nil")
	}
}
