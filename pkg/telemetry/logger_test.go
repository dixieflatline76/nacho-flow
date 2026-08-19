package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockServiceLogger implements service.Logger for unit testing.
type mockServiceLogger struct {
	infos    []string
	warnings []string
	errors   []string
}

func (m *mockServiceLogger) Info(v ...interface{}) error {
	m.infos = append(m.infos, fmt.Sprint(v...))
	return nil
}

func (m *mockServiceLogger) Infof(format string, a ...interface{}) error {
	m.infos = append(m.infos, fmt.Sprintf(format, a...))
	return nil
}

func (m *mockServiceLogger) Warning(v ...interface{}) error {
	m.warnings = append(m.warnings, fmt.Sprint(v...))
	return nil
}

func (m *mockServiceLogger) Warningf(format string, a ...interface{}) error {
	m.warnings = append(m.warnings, fmt.Sprintf(format, a...))
	return nil
}

func (m *mockServiceLogger) Error(v ...interface{}) error {
	m.errors = append(m.errors, fmt.Sprint(v...))
	return nil
}

func (m *mockServiceLogger) Errorf(format string, a ...interface{}) error {
	m.errors = append(m.errors, fmt.Sprintf(format, a...))
	return nil
}

func TestLogger_InteractiveMode_LevelFiltering(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test_router.log")

	var buf bytes.Buffer
	logger, closer := NewInteractiveLogger(&buf, logFile, slog.LevelInfo)
	defer closer.Close()

	logger.Debug("debug message should be ignored")
	logger.Info("info message should appear", slog.String("tier", "tier1"))
	logger.Warn("warn message should appear", slog.Int("status", 429))

	// Verify buffer output (stdout)
	output := buf.String()
	if strings.Contains(output, "debug message should be ignored") {
		t.Errorf("expected debug message to be filtered out, got: %s", output)
	}
	if !strings.Contains(output, "info message should appear") {
		t.Errorf("expected info message to appear, got: %s", output)
	}
	if !strings.Contains(output, "tier=tier1") && !strings.Contains(output, `"tier":"tier1"`) {
		t.Errorf("expected attribute tier=tier1 in output, got: %s", output)
	}

	// Flush and close before reading and before tempdir cleanup
	if err := closer.Close(); err != nil {
		t.Fatalf("failed to close file logger: %v", err)
	}

	// Verify file was created and written to
	fileData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	fileStr := string(fileData)
	if !strings.Contains(fileStr, "info message should appear") {
		t.Errorf("expected file to contain info message, got: %s", fileStr)
	}
}

func TestLogger_ServiceMode_Adapter(t *testing.T) {
	mockSvc := &mockServiceLogger{}
	logger := NewServiceLogger(mockSvc, slog.LevelInfo)

	ctx := context.Background()
	logger.Log(ctx, slog.LevelInfo, "service started", slog.Int("port", 8080))
	logger.Log(ctx, slog.LevelWarn, "high memory pressure", slog.Float64("usage", 0.92))
	logger.Log(ctx, slog.LevelError, "upstream connection refused", slog.String("host", "127.0.0.1"))

	if len(mockSvc.infos) != 1 || !strings.Contains(mockSvc.infos[0], "service started") {
		t.Errorf("expected info log recorded in mock service, got: %v", mockSvc.infos)
	}
	if len(mockSvc.warnings) != 1 || !strings.Contains(mockSvc.warnings[0], "high memory pressure") {
		t.Errorf("expected warning log recorded in mock service, got: %v", mockSvc.warnings)
	}
	if len(mockSvc.errors) != 1 || !strings.Contains(mockSvc.errors[0], "upstream connection refused") {
		t.Errorf("expected error log recorded in mock service, got: %v", mockSvc.errors)
	}
}

func TestLogger_ContextEnrichment(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx := WithRequestID(context.Background(), "req-12345")
	enrichedLogger := FromContext(ctx, logger)

	enrichedLogger.Info("handling proxy completion", slog.String("model", "qwen2.5-coder"))

	out := buf.String()
	if !strings.Contains(out, "req-12345") {
		t.Errorf("expected request_id req-12345 in log line, got: %s", out)
	}
	if !strings.Contains(out, "model=qwen2.5-coder") {
		t.Errorf("expected model=qwen2.5-coder in log line, got: %s", out)
	}
}
