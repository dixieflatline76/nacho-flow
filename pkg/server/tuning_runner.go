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
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

// TuningParams encapsulates parameters for configuring an auto-tuning run.
type TuningParams struct {
	LocalVRAMGB int    `json:"local_vram_gb,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
	WriteOnly   bool   `json:"write_only,omitempty"`
	Apply       bool   `json:"apply,omitempty"`
}

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
	LocalVRAMGB    int
	ClientID       string
	logger         *slog.Logger
}

// NewProcessTuningRunner creates a new ProcessTuningRunner.
// If executablePath is empty, it resolves os.Executable() dynamically at runtime.
func NewProcessTuningRunner(executablePath string, logger *slog.Logger) *ProcessTuningRunner {
	return &ProcessTuningRunner{
		ExecutablePath: executablePath,
		MaxSessions:    0, // Uncapped: evaluate all complete sessions
		Strategy:       "min_conflicts",
		LocalVRAMGB:    0,
		ClientID:       "all",
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
	if p.LocalVRAMGB > 0 {
		args = append(args, fmt.Sprintf("--vram-gb=%d", p.LocalVRAMGB))
	}
	if p.ClientID != "" && !strings.EqualFold(p.ClientID, "all") {
		args = append(args, fmt.Sprintf("--client=%s", p.ClientID))
	}

	// #nosec G204 - self-invoked executable for worker sub-process execution
	cmd := exec.CommandContext(ctx, exe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start tuner worker process: %w", err)
	}

	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	if p.logger != nil {
		p.logger.Info("Spawned isolated tuner worker process",
			slog.Int("pid", pid),
			slog.String("executable", exe),
			slog.String("strategy", strategy),
			slog.String("traffic_log", trafficLogPath),
		)
	}

	err := cmd.Wait()
	duration := time.Since(start)

	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("tuner worker process (pid %d) failed after %s: %s", pid, duration, stderrStr)
		}
		return nil, fmt.Errorf("tuner worker process (pid %d) failed after %s: %w", pid, duration, err)
	}

	if p.logger != nil {
		p.logger.Info("Tuner worker process completed successfully",
			slog.Int("pid", pid),
			slog.Duration("duration", duration),
		)
	}

	var result tuner.TuningResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return nil, fmt.Errorf("failed to decode tuner worker JSON output: %w (output: %s)", err, stdout.String())
	}

	return &result, nil
}

// InProcessTuningRunner runs the tuning optimization directly in-process.
// It is primarily intended for fast, zero-dependency unit tests and fallback scenarios.
type InProcessTuningRunner struct {
	server      *Server
	LocalVRAMGB int
	ClientID    string
}

// NewInProcessTuningRunner creates an InProcessTuningRunner bound to the given Server.
func NewInProcessTuningRunner(server *Server) *InProcessTuningRunner {
	return &InProcessTuningRunner{server: server, LocalVRAMGB: 0, ClientID: "all"}
}

// RunTuning loads complete sessions and runs s.tuner.OptimizeWithContext in-process.
func (r *InProcessTuningRunner) RunTuning(ctx context.Context, configPath string, trafficLogPath string) (*tuner.TuningResult, error) {
	if r.server == nil || r.server.tuner == nil {
		return nil, errors.New("auto-tuning optimizer is not initialized on this server")
	}

	records, err := telemetry.ReadCompleteSessionsFiltered(trafficLogPath, 0, r.ClientID)
	if err != nil || len(records) == 0 {
		// Fallback to in-memory ring buffer if traffic log is empty or unreadable
		if r.server.ringBuffer != nil {
			records = r.server.ringBuffer.GetRecent(500)
			if r.ClientID != "" && !strings.EqualFold(r.ClientID, "all") {
				filtered := make([]telemetry.TurnRecord, 0, len(records))
				for _, rec := range records {
					cid := rec.ClientID
					if cid == "" {
						cid = "unknown"
					}
					if strings.EqualFold(cid, r.ClientID) {
						filtered = append(filtered, rec)
					}
				}
				records = filtered
			}
		}
	}

	cfg := r.server.GetConfig()
	policy := tuner.DefaultTuningPolicy()
	if r.LocalVRAMGB > 0 {
		policy.LocalVRAMGB = r.LocalVRAMGB
	}
	if cfg != nil {
		if cfg.Kickstart.WriteOnly || cfg.CycleKiller.KickstartWriteOnly || cfg.CycleBreaker.KickstartWriteOnly {
			policy.WriteOnly = true
		}
		if r.server.oracle != nil {
			policy.CandidateDeals = r.server.oracle.GetDeals(cfg.Deals, 0.0, 0)
		}
	}
	optimizer := tuner.NewMinConflictsOptimizer(policy)

	return optimizer.OptimizeWithContext(ctx, records, cfg)
}
