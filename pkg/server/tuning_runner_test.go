package server

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

func TestInProcessTuningRunner_NotInitialized(t *testing.T) {
	runner := NewInProcessTuningRunner(nil)
	_, err := runner.RunTuning(context.Background(), "config.yaml", "traffic.jsonl")
	if err == nil {
		t.Errorf("Expected error when server is nil, got nil")
	}

	srv := &Server{}
	runner2 := NewInProcessTuningRunner(srv)
	_, err2 := runner2.RunTuning(context.Background(), "config.yaml", "traffic.jsonl")
	if err2 == nil {
		t.Errorf("Expected error when server.tuner is nil, got nil")
	}
}

func TestInProcessTuningRunner_WithRingBufferFallback(t *testing.T) {
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 1",
				Provider: "ollama",
				Model:    "qwen",
				When:     "Tokens < 16000",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 2",
			Provider: "openrouter",
			Model:    "claude",
			When:     "true",
		},
	}
	srv := &Server{
		ringBuffer: telemetry.NewRingBufferSink(10),
		tuner:      tuner.NewMinConflictsOptimizer(tuner.DefaultTuningPolicy()),
	}
	srv.state.Store(&runtimeState{config: cfg})

	runner := NewInProcessTuningRunner(srv)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := runner.RunTuning(ctx, "nonexistent-config.yaml", "nonexistent-traffic.jsonl")
	if err != nil {
		t.Fatalf("Unexpected error running in-process tuning: %v", err)
	}
	if res == nil {
		t.Fatalf("Expected non-nil tuning result")
	}
}

func TestProcessTuningRunner_InvalidExecutable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	runner := NewProcessTuningRunner("nonexistent-nacho-flow-binary-xyz", logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := runner.RunTuning(ctx, "config.yaml", "traffic.jsonl")
	if err == nil {
		t.Errorf("Expected error running invalid executable, got nil")
	}
}

func TestProcessTuningRunner_Options(t *testing.T) {
	runner := NewProcessTuningRunner("", nil)
	runner.LocalVRAMGB = 16
	runner.Strategy = "grid"
	runner.MaxSessions = 5

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Exercises arg construction, empty exe fallback to os.Executable()
	_, _ = runner.RunTuning(ctx, "config.yaml", "traffic.jsonl")
}

func TestInProcessTuningRunner_WithLocalVRAM(t *testing.T) {
	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "T1", Provider: "ollama", Model: "qwen", When: "Tokens < 16000"},
		},
		DefaultTier: contract.Tier{Name: "T2", Provider: "openrouter", Model: "claude", When: "true"},
	}
	srv := &Server{
		ringBuffer: telemetry.NewRingBufferSink(10),
		tuner:      tuner.NewMinConflictsOptimizer(tuner.DefaultTuningPolicy()),
	}
	srv.state.Store(&runtimeState{config: cfg})

	runner := NewInProcessTuningRunner(srv)
	runner.LocalVRAMGB = 16

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := runner.RunTuning(ctx, "config.yaml", "traffic.jsonl")
	if err != nil || res == nil {
		t.Fatalf("unexpected error with local VRAM: %v", err)
	}
}

func TestProcessTuningRunner_WithStderr(t *testing.T) {
	runner := NewProcessTuningRunner("cmd.exe", slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := runner.RunTuning(ctx, "config.yaml", "traffic.jsonl")
	if err == nil {
		t.Errorf("expected error from cmd.exe running 'tune'")
	}
}
