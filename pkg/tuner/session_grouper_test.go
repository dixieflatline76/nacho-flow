package tuner

import (
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestSessionGrouper_OrderedTrajectories(t *testing.T) {
	now := time.Now().UTC()

	// Records out of chronological order and interleaved between sessions
	records := []telemetry.TurnRecord{
		{
			SessionID:        "sess-b",
			Timestamp:        now.Add(20 * time.Second),
			Tokens:           1500,
			CostSpentUSD:     0.02,
			Retries:          1,
			IsRetry:          true,
			HasWriteProgress: false,
		},
		{
			SessionID:        "sess-a",
			Timestamp:        now.Add(5 * time.Second),
			Tokens:           1000,
			CostSpentUSD:     0.01,
			Retries:          0,
			IsRetry:          false,
			HasWriteProgress: false,
		},
		{
			SessionID:        "sess-b",
			Timestamp:        now.Add(10 * time.Second),
			Tokens:           1200,
			CostSpentUSD:     0.015,
			Retries:          0,
			IsRetry:          false,
			HasWriteProgress: true,
		},
		{
			SessionID:        "sess-a",
			Timestamp:        now.Add(15 * time.Second),
			Tokens:           2000,
			CostSpentUSD:     0.03,
			Retries:          0,
			IsRetry:          false,
			HasWriteProgress: true,
		},
	}

	trajectories := GroupBySession(records)

	if len(trajectories) != 2 {
		t.Fatalf("Expected 2 session trajectories, got %d", len(trajectories))
	}

	// Map trajectories by SessionID for deterministic testing
	trajMap := make(map[string]SessionTrajectory)
	for _, traj := range trajectories {
		trajMap[traj.SessionID] = traj
	}

	// Verify sess-a
	trajA, ok := trajMap["sess-a"]
	if !ok {
		t.Fatalf("Missing sess-a trajectory")
	}
	if trajA.TotalTurns != 2 {
		t.Errorf("Expected 2 turns for sess-a, got %d", trajA.TotalTurns)
	}
	if trajA.TotalCost < 0.039 || trajA.TotalCost > 0.041 {
		t.Errorf("Expected ~0.04 cost for sess-a, got %f", trajA.TotalCost)
	}
	if trajA.TotalRetries != 0 {
		t.Errorf("Expected 0 retries for sess-a, got %d", trajA.TotalRetries)
	}
	if !trajA.Resolved {
		t.Errorf("Expected sess-a to be Resolved (ended with write progress), got false")
	}
	// Verify chronological ordering of sess-a
	if trajA.Turns[0].Tokens != 1000 || trajA.Turns[1].Tokens != 2000 {
		t.Errorf("sess-a turns not sorted chronologically: %+v", trajA.Turns)
	}

	// Verify sess-b
	trajB, ok := trajMap["sess-b"]
	if !ok {
		t.Fatalf("Missing sess-b trajectory")
	}
	if trajB.TotalTurns != 2 {
		t.Errorf("Expected 2 turns for sess-b, got %d", trajB.TotalTurns)
	}
	if trajB.TotalRetries != 1 {
		t.Errorf("Expected 1 retry for sess-b, got %d", trajB.TotalRetries)
	}
	if trajB.Resolved {
		t.Errorf("Expected sess-b to NOT be Resolved (ended with retry and no progress), got true")
	}
	// Verify chronological ordering of sess-b
	if trajB.Turns[0].Tokens != 1200 || trajB.Turns[1].Tokens != 1500 {
		t.Errorf("sess-b turns not sorted chronologically: %+v", trajB.Turns)
	}
}

func TestSessionGrouper_DropsEmptySessionID(t *testing.T) {
	records := []telemetry.TurnRecord{
		{SessionID: "", Tokens: 500},
		{SessionID: "sess-valid", Tokens: 1000},
		{SessionID: "", Tokens: 1500},
	}

	trajectories := GroupBySession(records)
	if len(trajectories) != 1 {
		t.Fatalf("Expected 1 trajectory, got %d", len(trajectories))
	}
	if trajectories[0].SessionID != "sess-valid" {
		t.Errorf("Expected SessionID 'sess-valid', got '%s'", trajectories[0].SessionID)
	}
	if trajectories[0].TotalTurns != 1 {
		t.Errorf("Expected 1 turn, got %d", trajectories[0].TotalTurns)
	}
}

func TestSessionGrouper_EmptyInput(t *testing.T) {
	if traj := GroupBySession(nil); len(traj) != 0 {
		t.Errorf("Expected 0 trajectories for nil input, got %d", len(traj))
	}
	if traj := GroupBySession([]telemetry.TurnRecord{}); len(traj) != 0 {
		t.Errorf("Expected 0 trajectories for empty input, got %d", len(traj))
	}
}
