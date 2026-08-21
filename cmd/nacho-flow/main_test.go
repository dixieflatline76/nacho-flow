package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
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
	defer func() { os.Args = oldArgs }()

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
