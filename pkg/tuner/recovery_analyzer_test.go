package tuner

import (
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestRecoveryAnalyzer_PerModelRates(t *testing.T) {
	now := time.Now().UTC()

	// Session 1: gemma fails on turn 1 and fails again on turn 2 (0% self-recovery)
	s1 := SessionTrajectory{
		SessionID: "sess-gemma",
		Turns: []telemetry.TurnRecord{
			{Timestamp: now.Add(1 * time.Second), TargetModel: "gemma-4-26b", IsRetry: false, HasWriteProgress: true},
			{Timestamp: now.Add(2 * time.Second), TargetModel: "gemma-4-26b", IsRetry: true, HasWriteProgress: false}, // failure 1
			{Timestamp: now.Add(3 * time.Second), TargetModel: "gemma-4-26b", IsRetry: true, HasWriteProgress: false}, // didn't recover on turn 3
			{Timestamp: now.Add(4 * time.Second), TargetModel: "claude-3-5", IsRetry: false, HasWriteProgress: true},
		},
	}

	// Session 2: qwen fails on turn 1 and immediately recovers on turn 2 (100% self-recovery)
	s2 := SessionTrajectory{
		SessionID: "sess-qwen",
		Turns: []telemetry.TurnRecord{
			{Timestamp: now.Add(10 * time.Second), TargetModel: "qwen3-coder-plus", IsRetry: false, HasWriteProgress: true},
			{Timestamp: now.Add(11 * time.Second), TargetModel: "qwen3-coder-plus", IsRetry: true, HasWriteProgress: false}, // failure 1
			{Timestamp: now.Add(12 * time.Second), TargetModel: "qwen3-coder-plus", IsRetry: false, HasWriteProgress: true}, // recovered!
		},
	}

	trajectories := []SessionTrajectory{s1, s2}
	stats := AnalyzeRecovery(trajectories)

	gemmaStats, ok := stats["gemma-4-26b"]
	if !ok {
		t.Fatalf("Missing stats for gemma-4-26b")
	}
	if gemmaStats.TotalFailures < 1 {
		t.Errorf("Expected >= 1 failures for gemma, got %d", gemmaStats.TotalFailures)
	}
	if gemmaStats.SelfRecoveryRate != 0.0 {
		t.Errorf("Expected 0.0 self-recovery rate for gemma, got %f", gemmaStats.SelfRecoveryRate)
	}

	qwenStats, ok := stats["qwen3-coder-plus"]
	if !ok {
		t.Fatalf("Missing stats for qwen3-coder-plus")
	}
	if qwenStats.TotalFailures != 1 {
		t.Errorf("Expected 1 failure for qwen, got %d", qwenStats.TotalFailures)
	}
	if qwenStats.SelfRecoveries != 1 {
		t.Errorf("Expected 1 self-recovery for qwen, got %d", qwenStats.SelfRecoveries)
	}
	if qwenStats.SelfRecoveryRate != 1.0 {
		t.Errorf("Expected 1.0 self-recovery rate for qwen, got %f", qwenStats.SelfRecoveryRate)
	}
	if qwenStats.AvgTurnsToRecover != 1.0 {
		t.Errorf("Expected 1.0 avg turns to recover for qwen, got %f", qwenStats.AvgTurnsToRecover)
	}
}

func TestRecoveryAnalyzer_ZeroFailures(t *testing.T) {
	now := time.Now().UTC()
	traj := []SessionTrajectory{
		{
			SessionID: "sess-clean",
			Turns: []telemetry.TurnRecord{
				{Timestamp: now, TargetModel: "perfect-model", IsRetry: false, HasWriteProgress: true},
			},
		},
	}

	stats := AnalyzeRecovery(traj)
	if s, ok := stats["perfect-model"]; ok {
		if s.TotalFailures != 0 || s.SelfRecoveryRate != 0 {
			t.Errorf("Expected 0 failures and 0 rate for clean model, got %+v", s)
		}
	}
}

func TestRecoveryAnalyzer_Empty(t *testing.T) {
	stats := AnalyzeRecovery(nil)
	if len(stats) != 0 {
		t.Errorf("Expected empty stats for nil trajectories, got %v", stats)
	}
}
