package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

// TuningRunner abstracts the execution of an auto-tuning optimization run.
// This interface enables transparent switching between an isolated child worker process
// (production) and an in-process solver (unit tests / fallback).
type TuningRunner interface {
	RunTuning(ctx context.Context, configPath string, trafficLogPath string) (*tuner.TuningResult, error)
}

// ProcessTuningRunner spawns an isolated child worker process (`nacho-flow tune --format=json`)
// to parse traffic logs and compute optimization policies out-of-process.
// This provides complete fault isolation, prevents heap fragmentation, and ensures
// the gateway daemon's hot path (/v1/chat/completions) remains unaffected by GC spikes.
type ProcessTuningRunner struct {
	ExecutablePath string
	MaxSessions    int
	Strategy       string
	logger         *slog.Logger
}

// NewProcessTuningRunner creates a new ProcessTuningRunner.
// If executablePath is empty, it resolves os.Executable() dynamically at runtime.
func NewProcessTuningRunner(executablePath string, logger *slog.Logger) *ProcessTuningRunner {
	return &ProcessTuningRunner{
		ExecutablePath: executablePath,
		MaxSessions:    0, // Uncapped: evaluate all complete sessions
		Strategy:       "min_conflicts",
		logger:         logger,
	}
}

// RunTuning executes the child worker process and returns the parsed TuningResult.
func (p *ProcessTuningRunner) RunTuning(ctx context.Context, configPath string, trafficLogPath string) (*tuner.TuningResult, error) {
	exe := p.ExecutablePath
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("failed to determine nacho-flow executable path: %w", err)
		}
	}

	strategy := p.Strategy
	if strategy == "" {
		strategy = "min_conflicts"
	}

	args := []string{
		"tune",
		"--format=json",
		fmt.Sprintf("--config=%s", configPath),
		fmt.Sprintf("--traffic-log=%s", trafficLogPath),
		fmt.Sprintf("--strategy=%s", strategy),
		fmt.Sprintf("--max-sessions=%d", p.MaxSessions),
	}

	cmd := exec.CommandContext(ctx, exe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if p.logger != nil {
		p.logger.Info("Spawning isolated tuner worker process",
			slog.String("executable", exe),
			slog.String("strategy", strategy),
			slog.String("traffic_log", trafficLogPath),
		)
	}

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("tuner worker process failed: %s", stderrStr)
		}
		return nil, fmt.Errorf("tuner worker process failed: %w", err)
	}

	var result tuner.TuningResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to decode tuner worker JSON output: %w (output: %s)", err, stdout.String())
	}

	return &result, nil
}

// InProcessTuningRunner runs the tuning optimization directly in-process.
// It is primarily intended for fast, zero-dependency unit tests and fallback scenarios.
type InProcessTuningRunner struct {
	server *Server
}

// NewInProcessTuningRunner creates an InProcessTuningRunner bound to the given Server.
func NewInProcessTuningRunner(server *Server) *InProcessTuningRunner {
	return &InProcessTuningRunner{server: server}
}

// RunTuning loads complete sessions and runs s.tuner.OptimizeWithContext in-process.
func (r *InProcessTuningRunner) RunTuning(ctx context.Context, configPath string, trafficLogPath string) (*tuner.TuningResult, error) {
	if r.server == nil || r.server.tuner == nil {
		return nil, errors.New("auto-tuning optimizer is not initialized on this server")
	}

	records, err := telemetry.ReadCompleteSessions(trafficLogPath, 0)
	if err != nil || len(records) == 0 {
		// Fallback to in-memory ring buffer if traffic log is empty or unreadable
		if r.server.ringBuffer != nil {
			records = r.server.ringBuffer.GetRecent(500)
		}
	}

	return r.server.tuner.OptimizeWithContext(ctx, records, r.server.GetConfig())
}
