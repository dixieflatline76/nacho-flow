package tuner

import (
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestReplaySession_CounterfactualCloudTurn(t *testing.T) {
	// A session where turn 1 ran on Cloud (Claude 3.5) and succeeded.
	// When candidate config simulates routing turn 1 to Local, the simulator
	// MUST NOT assume Local succeeds. It must count as a failure/stall.
	now := time.Now().UTC()
	traj := SessionTrajectory{
		SessionID: "sess-counterfactual",
		Turns: []telemetry.TurnRecord{
			{
				Timestamp:        now,
				Tokens:           2000,
				IsLocal:          false, // Historically ran on Cloud
				IsRetry:          false,
				HasWriteProgress: true, // Cloud succeeded with a write
				CostSpentUSD:     0.05,
				RootPromptHash:   0x111,
			},
		},
	}

	config := ReplayConfig{
		TokenThreshold: 4000, // 2000 < 4000 -> candidate wants Local!
		RetryBound:     2,
	}
	policy := DefaultTuningPolicy()

	res := ReplaySession(traj, config, policy)

	// Candidate routed to Local
	if res.LocalTurns != 1 {
		t.Errorf("Expected 1 LocalTurn, got %d", res.LocalTurns)
	}
	if res.CloudTurns != 0 {
		t.Errorf("Expected 0 CloudTurns, got %d", res.CloudTurns)
	}
	// Under Counterfactual Attribution, historical cloud turn simulated on local is NOT credited as free win
	if res.SimulatedRetries != 1 {
		t.Errorf("Expected SimulatedRetries == 1 for unproven counterfactual local turn, got %d", res.SimulatedRetries)
	}
}

func TestReplaySession_EscalationAtRetryBound(t *testing.T) {
	now := time.Now().UTC()
	// 4-turn session: 3 failing local turns, then a 4th turn.
	// Config has RetryBound = 2.
	// Turn 0: Local (fails -> retry count 1)
	// Turn 1: Local (fails -> retry count 2 -> reaches bound!)
	// Turn 2: Escalates to Cloud!
	// Turn 3: Stays on Cloud
	traj := SessionTrajectory{
		SessionID: "sess-escalate",
		Turns: []telemetry.TurnRecord{
			{Timestamp: now.Add(1 * time.Second), Tokens: 1000, IsLocal: true, IsRetry: true, HasWriteProgress: false, RootPromptHash: 0x100},
			{Timestamp: now.Add(2 * time.Second), Tokens: 1000, IsLocal: true, IsRetry: true, HasWriteProgress: false, RootPromptHash: 0x100},
			{Timestamp: now.Add(3 * time.Second), Tokens: 1000, IsLocal: true, IsRetry: false, HasWriteProgress: true, RootPromptHash: 0x100},
			{Timestamp: now.Add(4 * time.Second), Tokens: 1000, IsLocal: true, IsRetry: false, HasWriteProgress: true, RootPromptHash: 0x100},
		},
	}

	config := ReplayConfig{
		TokenThreshold: 8000,
		RetryBound:     2,
	}
	policy := DefaultTuningPolicy()

	res := ReplaySession(traj, config, policy)

	if res.LocalTurns != 2 {
		t.Errorf("Expected 2 LocalTurns before escalation, got %d", res.LocalTurns)
	}
	if res.CloudTurns != 2 {
		t.Errorf("Expected 2 CloudTurns after escalation, got %d", res.CloudTurns)
	}
	if res.EscalationTurn != 2 {
		t.Errorf("Expected EscalationTurn = 2, got %d", res.EscalationTurn)
	}
	if res.SimulatedCost <= 0 {
		t.Errorf("Expected simulated cloud cost > 0, got %f", res.SimulatedCost)
	}
}

func TestReplaySession_TaskResetOnRootHash(t *testing.T) {
	now := time.Now().UTC()
	// Turn 0: Task 1 (fails -> retries = 1)
	// Turn 1: Task 2 has DIFFERENT RootPromptHash! Retries must reset to 0.
	traj := SessionTrajectory{
		SessionID: "sess-task-reset",
		Turns: []telemetry.TurnRecord{
			{Timestamp: now.Add(1 * time.Second), Tokens: 1000, IsLocal: true, IsRetry: true, HasWriteProgress: false, RootPromptHash: 0xAAA},
			{Timestamp: now.Add(2 * time.Second), Tokens: 1000, IsLocal: true, IsRetry: false, HasWriteProgress: true, RootPromptHash: 0xBBB},
		},
	}

	config := ReplayConfig{
		TokenThreshold: 8000,
		RetryBound:     1, // RetryBound=1 would escalate if retries carried over
	}
	policy := DefaultTuningPolicy()

	res := ReplaySession(traj, config, policy)

	// Since Turn 1 is a new task, it should NOT escalate!
	if res.LocalTurns != 2 {
		t.Errorf("Expected 2 LocalTurns (new task reset retries), got %d", res.LocalTurns)
	}
	if res.CloudTurns != 0 {
		t.Errorf("Expected 0 CloudTurns, got %d", res.CloudTurns)
	}
}

func TestReplaySession_FrictionKeywordRouting(t *testing.T) {
	now := time.Now().UTC()
	traj := SessionTrajectory{
		SessionID: "sess-kw",
		Turns: []telemetry.TurnRecord{
			{Timestamp: now, Tokens: 1000, IsLocal: true, Keywords: []string{"docker", "production"}},
		},
	}

	config := ReplayConfig{
		TokenThreshold:   8000,
		RetryBound:       2,
		FrictionKeywords: []string{"docker"},
	}
	policy := DefaultTuningPolicy()

	res := ReplaySession(traj, config, policy)

	// Contains friction keyword "docker", should bypass local directly to cloud
	if res.CloudTurns != 1 {
		t.Errorf("Expected 1 CloudTurn due to friction keyword, got %d", res.CloudTurns)
	}
	if res.LocalTurns != 0 {
		t.Errorf("Expected 0 LocalTurns, got %d", res.LocalTurns)
	}
}
