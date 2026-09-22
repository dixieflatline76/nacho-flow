package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

func TestRunTune_JSONFormat(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.yaml")
	logPath := filepath.Join(tempDir, "traffic.jsonl")

	sampleCfg := `
providers:
  local:
    base_url: "http://localhost:11434"
    type: "local"
  cloud:
    base_url: "https://api.deepseek.com"
    type: "cloud"
tiers:
  - name: "Local GPU"
    provider: "local"
    model: "qwen2.5-coder:14b"
    when: "Tokens < 4000"
  - name: "Cloud Frontier"
    provider: "cloud"
    model: "deepseek-chat"
`
	if err := os.WriteFile(cfgPath, []byte(sampleCfg), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	logger, err := telemetry.NewTrafficLogger(logPath, 100)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	baseTime := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	// Emit Session 1: 3 turns
	for i := 1; i <= 3; i++ {
		logger.Emit(telemetry.TurnRecord{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Minute),
			RequestID:      "req-1-" + string(rune('0'+i)),
			SessionID:      "sess-1",
			RootPromptHash: 111,
			Tokens:         1500 * i,
			SelectedTier:   "Local GPU",
		})
	}
	// Emit Session 2: 2 turns
	for i := 1; i <= 2; i++ {
		logger.Emit(telemetry.TurnRecord{
			Timestamp:      baseTime.Add(time.Duration(10+i) * time.Minute),
			RequestID:      "req-2-" + string(rune('0'+i)),
			SessionID:      "sess-2",
			RootPromptHash: 222,
			Tokens:         2500 * i,
			SelectedTier:   "Cloud Frontier",
		})
	}
	_ = logger.Close()

	// Capture stdout when running with --format=json
	out := captureStdout(func() {
		err := runTune([]string{
			"--config=" + cfgPath,
			"--traffic-log=" + logPath,
			"--format=json",
		})
		if err != nil {
			t.Fatalf("runTune failed: %v", err)
		}
	})

	// Must NOT contain human-readable banner text or ANSI escapes
	if strings.Contains(out, "ADVISORY REPORT") || strings.Contains(out, "ℹ️") {
		t.Errorf("Expected pure JSON, but stdout contained banner text: %s", out)
	}

	// Must be valid JSON matching tuner.TuningResult
	var res tuner.TuningResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil {
		t.Fatalf("Failed to parse stdout as TuningResult JSON: %v\nOutput: %s", err, out)
	}

	if res.TotalSessions != 2 {
		t.Errorf("Expected 2 sessions evaluated, got %d", res.TotalSessions)
	}
	if len(res.Tiers) == 0 {
		t.Errorf("Expected tuned tiers in result, got 0")
	}
}

func TestRunTune_MaxSessions(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.yaml")
	logPath := filepath.Join(tempDir, "traffic.jsonl")

	sampleCfg := `
providers:
  local:
    base_url: "http://localhost:11434"
    type: "local"
tiers:
  - name: "Local GPU"
    provider: "local"
    model: "qwen2.5-coder:14b"
    when: "Tokens < 4000"
`
	if err := os.WriteFile(cfgPath, []byte(sampleCfg), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	logger, err := telemetry.NewTrafficLogger(logPath, 100)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	baseTime := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	for s := 1; s <= 3; s++ {
		for turn := 1; turn <= 2; turn++ {
			logger.Emit(telemetry.TurnRecord{
				Timestamp:      baseTime.Add(time.Duration(s*10+turn) * time.Minute),
				RequestID:      "req",
				SessionID:      "sess-" + string(rune('0'+s)),
				RootPromptHash: uint64(s * 100),
				Tokens:         1000,
				SelectedTier:   "Local GPU",
			})
		}
	}
	_ = logger.Close()

	out := captureStdout(func() {
		err := runTune([]string{
			"--config=" + cfgPath,
			"--traffic-log=" + logPath,
			"--format=json",
			"--max-sessions=1",
		})
		if err != nil {
			t.Fatalf("runTune failed: %v", err)
		}
	})

	var res tuner.TuningResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, out)
	}

	if res.TotalSessions != 1 {
		t.Errorf("Expected 1 session with --max-sessions=1, got %d", res.TotalSessions)
	}
}

func TestRunTune_EmptyLog_JSONFormat(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.yaml")
	logPath := filepath.Join(tempDir, "empty.jsonl")

	sampleCfg := `
providers:
  local:
    base_url: "http://localhost:11434"
    type: "local"
tiers:
  - name: "Local GPU"
    provider: "local"
    model: "qwen2.5-coder:14b"
    when: "Tokens < 4000"
`
	_ = os.WriteFile(cfgPath, []byte(sampleCfg), 0644)
	_ = os.WriteFile(logPath, []byte(""), 0644)

	out := captureStdout(func() {
		err := runTune([]string{
			"--config=" + cfgPath,
			"--traffic-log=" + logPath,
			"--format=json",
		})
		if err != nil {
			t.Fatalf("runTune failed: %v", err)
		}
	})

	var res tuner.TuningResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, out)
	}

	if res.TotalSessions != 0 {
		t.Errorf("Expected 0 sessions for empty log, got %d", res.TotalSessions)
	}
}

func TestRunTune_StrategiesAndVRAM(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.yaml")
	logPath := filepath.Join(tempDir, "traffic.jsonl")

	sampleCfg := `providers:
  local:
    base_url: "http://localhost:11434"
    type: "local"
tiers:
  - name: "Local GPU"
    provider: "local"
    model: "qwen2.5-coder:14b"
    when: "Tokens < 4000"
`
	_ = os.WriteFile(cfgPath, []byte(sampleCfg), 0644)

	tl, _ := telemetry.NewTrafficLogger(logPath, 100)
	tl.Emit(telemetry.TurnRecord{
		Timestamp: time.Now().UTC(),
		SessionID: "s-1",
		Tokens:    1000,
		IsLocal:   true,
	})
	_ = tl.Close()

	// 1. Test with --strategy=grid_sweep
	_ = captureStdout(func() {
		err := runTune([]string{
			"--config=" + cfgPath,
			"--traffic-log=" + logPath,
			"--strategy=grid_sweep",
			"--sample=10",
		})
		if err != nil {
			t.Fatalf("grid_sweep failed: %v", err)
		}
	})

	// 2. Test with --strategy=cost_penalty
	_ = captureStdout(func() {
		err := runTune([]string{
			"--config=" + cfgPath,
			"--traffic-log=" + logPath,
			"--strategy=cost_penalty",
		})
		if err != nil {
			t.Fatalf("cost_penalty failed: %v", err)
		}
	})

	// 3. Test with --vram-gb=16
	_ = captureStdout(func() {
		err := runTune([]string{
			"--config=" + cfgPath,
			"--traffic-log=" + logPath,
			"--vram-gb=16",
		})
		if err != nil {
			t.Fatalf("vram-gb failed: %v", err)
		}
	})
}
