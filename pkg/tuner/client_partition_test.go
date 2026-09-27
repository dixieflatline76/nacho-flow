package tuner

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestTuningWithClientFilter(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "traffic.jsonl")

	tl, err := telemetry.NewTrafficLogger(logPath, 100)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Session 1: Cline (fast turns, small context)
	tl.Emit(telemetry.TurnRecord{
		Timestamp:          time.Now().UTC(),
		SessionID:          "sess-cline",
		RootPromptHash:     101,
		Tokens:             1500,
		ClientID:           "cline",
		SelectedTier:       "Tier 2",
		TargetModel:        "glm-5.3-flash",
		HasWriteCapability: true,
		HasWriteProgress:   true,
	})
	tl.Emit(telemetry.TurnRecord{
		Timestamp:          time.Now().UTC(),
		SessionID:          "sess-cline",
		RootPromptHash:     101,
		Tokens:             2500,
		ClientID:           "cline",
		SelectedTier:       "Tier 2",
		TargetModel:        "glm-5.3-flash",
		HasWriteCapability: true,
		HasTestPass:        true,
	})

	// Session 2: Zoo (heavy turns, large context)
	tl.Emit(telemetry.TurnRecord{
		Timestamp:          time.Now().UTC(),
		SessionID:          "sess-zoo",
		RootPromptHash:     202,
		Tokens:             45000,
		ClientID:           "zoo",
		SelectedTier:       "Tier 2",
		TargetModel:        "glm-5.3-flash",
		HasWriteCapability: true,
		HasWriteProgress:   false,
	})
	tl.Emit(telemetry.TurnRecord{
		Timestamp:          time.Now().UTC(),
		SessionID:          "sess-zoo",
		RootPromptHash:     202,
		Tokens:             85000,
		ClientID:           "zoo",
		SelectedTier:       "Tier 3",
		TargetModel:        "claude-3-7-sonnet",
		HasWriteCapability: true,
		HasTestPass:        true,
	})

	_ = tl.Close()

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{Name: "Tier 1", Model: "qwen2.5-coder:14b", Provider: "ollama", When: "Tokens < 4000"},
			{Name: "Tier 2", Model: "glm-5.3-flash", Provider: "openrouter", When: "Tokens < 60000"},
		},
		DefaultTier: contract.Tier{Name: "Tier 3", Model: "claude-3-7-sonnet", Provider: "openrouter"},
	}

	policy := DefaultTuningPolicy()
	optimizer := NewMinConflictsOptimizer(policy)

	// 1. Optimize for Cline only
	clineRecords, err := telemetry.ReadCompleteSessionsFiltered(logPath, 0, "cline")
	if err != nil {
		t.Fatalf("ReadCompleteSessionsFiltered cline failed: %v", err)
	}
	if len(clineRecords) != 2 {
		t.Fatalf("Expected 2 records for cline, got %d", len(clineRecords))
	}

	resCline, err := optimizer.Optimize(clineRecords, cfg)
	if err != nil {
		t.Fatalf("Optimizer failed for cline: %v", err)
	}
	if resCline.TotalSessions != 1 {
		t.Errorf("Expected 1 session for cline, got %d", resCline.TotalSessions)
	}

	// 2. Optimize for Zoo only
	zooRecords, err := telemetry.ReadCompleteSessionsFiltered(logPath, 0, "zoo")
	if err != nil {
		t.Fatalf("ReadCompleteSessionsFiltered zoo failed: %v", err)
	}
	if len(zooRecords) != 2 {
		t.Fatalf("Expected 2 records for zoo, got %d", len(zooRecords))
	}

	resZoo, err := optimizer.Optimize(zooRecords, cfg)
	if err != nil {
		t.Fatalf("Optimizer failed for zoo: %v", err)
	}
	if resZoo.TotalSessions != 1 {
		t.Errorf("Expected 1 session for zoo, got %d", resZoo.TotalSessions)
	}

	// 3. Optimize for All (Fleet)
	allRecords, err := telemetry.ReadCompleteSessionsFiltered(logPath, 0, "all")
	if err != nil {
		t.Fatalf("ReadCompleteSessionsFiltered all failed: %v", err)
	}
	if len(allRecords) != 4 {
		t.Fatalf("Expected 4 records for all, got %d", len(allRecords))
	}

	resAll, err := optimizer.Optimize(allRecords, cfg)
	if err != nil {
		t.Fatalf("Optimizer failed for all: %v", err)
	}
	if resAll.TotalSessions != 2 {
		t.Errorf("Expected 2 sessions for all, got %d", resAll.TotalSessions)
	}
}
