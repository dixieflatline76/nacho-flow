package tuner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func loadRealTrafficData(tb testing.TB) []telemetry.TurnRecord {
	tb.Helper()
	// Try multiple relative locations for test execution flexibility
	candidates := []string{
		filepath.Join("testdata", "traffic.jsonl"),
		filepath.Join("..", "..", "pkg", "tuner", "testdata", "traffic.jsonl"),
		filepath.Join("..", "..", "logs", "traffic.jsonl"),
		filepath.Join("logs", "traffic.jsonl"),
		filepath.Join("..", "telemetry", "testdata", "historical_turns.json"),
	}

	var foundPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			foundPath = p
			break
		}
	}

	if foundPath == "" {
		tb.Skip("real traffic fixture not found in expected paths")
		return nil
	}

	data, err := os.ReadFile(foundPath)
	if err != nil {
		tb.Fatalf("failed reading real traffic data from %s: %v", foundPath, err)
	}

	var records []telemetry.TurnRecord
	if strings.HasSuffix(foundPath, ".jsonl") {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var rec telemetry.TurnRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			records = append(records, rec)
		}
	} else {
		if err := json.Unmarshal(data, &records); err != nil {
			tb.Fatalf("failed unmarshaling json fixture from %s: %v", foundPath, err)
		}
	}

	return records
}

func TestMultiTierReplay_CascadeRouting(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:       "Tier 1: Local GPU",
				Provider:       "ollama",
				IsLocal:        true,
				CostPerMillion: 0.0,
				TokenThreshold: 4000,
				RetryBound:     2,
				RestrictImages: true,
				RestrictTools:  true,
			},
			{
				TierName:       "Tier 2: Workhorse Cloud",
				Provider:       "openrouter",
				IsLocal:        false,
				CostPerMillion: 2.0,
				TokenThreshold: 16000,
				RetryBound:     3,
				RestrictImages: false,
				RestrictTools:  false,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 3: Frontier Fallback",
			Provider:       "anthropic",
			IsLocal:        false,
			CostPerMillion: 15.0,
		},
	}

	tests := []struct {
		name         string
		turn         telemetry.TurnRecord
		expectedTier string
	}{
		{
			name: "Small prompt without tools or images -> Tier 1",
			turn: telemetry.TurnRecord{
				Tokens:    2000,
				HasImages: false,
				HasTools:  false,
				IsLocal:   true,
			},
			expectedTier: "Tier 1: Local GPU",
		},
		{
			name: "Prompt exceeding Tier 1 tokens (8000) -> Tier 2",
			turn: telemetry.TurnRecord{
				Tokens:    8000,
				HasImages: false,
				HasTools:  false,
				IsLocal:   false,
			},
			expectedTier: "Tier 2: Workhorse Cloud",
		},
		{
			name: "Prompt with tools -> Tier 1 restricted -> Tier 2",
			turn: telemetry.TurnRecord{
				Tokens:    1500,
				HasImages: false,
				HasTools:  true,
				IsLocal:   false,
			},
			expectedTier: "Tier 2: Workhorse Cloud",
		},
		{
			name: "Prompt with images -> Tier 1 restricted -> Tier 2",
			turn: telemetry.TurnRecord{
				Tokens:    1500,
				HasImages: true,
				HasTools:  false,
				IsLocal:   false,
			},
			expectedTier: "Tier 2: Workhorse Cloud",
		},
		{
			name: "Prompt exceeding Tier 2 tokens (20000) -> Tier 3 Default",
			turn: telemetry.TurnRecord{
				Tokens:    20000,
				HasImages: false,
				HasTools:  true,
				IsLocal:   false,
			},
			expectedTier: "Tier 3: Frontier Fallback",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			traj := SessionTrajectory{
				SessionID: "sess-test",
				Turns: []telemetry.TurnRecord{
					tc.turn,
				},
			}
			traj.Turns[0].Timestamp = now
			traj.Turns[0].RootPromptHash = 0x999

			var result MultiTierReplayResult
			ReplayMultiTierSession(&traj, &cfg, &policy, &result)

			found := false
			for i := 0; i < result.NumTiers; i++ {
				if result.TierStats[i].TierName == tc.expectedTier {
					if result.TierStats[i].TurnsRouted != 1 {
						t.Fatalf("expected tier %s to have 1 turn, got %d", tc.expectedTier, result.TierStats[i].TurnsRouted)
					}
					found = true
				} else if result.TierStats[i].TurnsRouted != 0 {
					t.Fatalf("unexpected turn routed to tier %s", result.TierStats[i].TierName)
				}
			}
			if !found {
				t.Fatalf("expected tier %s was not evaluated in stats", tc.expectedTier)
			}
		})
	}
}

func TestMultiTierReplay_EscalationCascade(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:       "Tier 1: Local",
				Provider:       "ollama",
				IsLocal:        true,
				TokenThreshold: 8000,
				RetryBound:     2, // Escalates to Tier 2 when retries >= 2
			},
			{
				TierName:       "Tier 2: Workhorse",
				Provider:       "openrouter",
				IsLocal:        false,
				CostPerMillion: 2.0,
				TokenThreshold: 8000,
				RetryBound:     4, // Escalates to Tier 3 when retries >= 4
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 3: Fallback",
			Provider:       "anthropic",
			IsLocal:        false,
			CostPerMillion: 15.0,
		},
	}

	// 6 turns of failing attempts under the same root prompt:
	// Turn 0: Local (fails, retries becomes 1)
	// Turn 1: Local (fails, retries becomes 2) -> reaches Tier 1 bound
	// Turn 2: Escalates to Tier 2! (retries=2 < 4, Tier 2 fails, retries becomes 3)
	// Turn 3: Tier 2 (retries=3 < 4, Tier 2 fails, retries becomes 4) -> reaches Tier 2 bound
	// Turn 4: Escalates to Tier 3 Fallback! (reaches sink)
	// Turn 5: Fallback resolves (success, retries reset)
	traj := SessionTrajectory{
		SessionID: "sess-cascade",
		Turns: []telemetry.TurnRecord{
			{Timestamp: now.Add(1 * time.Second), Tokens: 1000, IsLocal: true, IsRetry: true, RootPromptHash: 0x42},
			{Timestamp: now.Add(2 * time.Second), Tokens: 1000, IsLocal: true, IsRetry: true, RootPromptHash: 0x42},
			{Timestamp: now.Add(3 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: true, RootPromptHash: 0x42},
			{Timestamp: now.Add(4 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: true, RootPromptHash: 0x42},
			{Timestamp: now.Add(5 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: true, RootPromptHash: 0x42},
			{Timestamp: now.Add(6 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: false, HasWriteProgress: true, RootPromptHash: 0x42},
		},
	}

	var res MultiTierReplayResult
	ReplayMultiTierSession(&traj, &cfg, &policy, &res)

	// Verify exact turn distribution
	// Tier 1 should get 2 turns
	// Tier 2 should get 2 turns
	// Tier 3 should get 2 turns
	if res.TierStats[0].TurnsRouted != 2 {
		t.Errorf("expected Tier 1 to get 2 turns, got %d", res.TierStats[0].TurnsRouted)
	}
	if res.TierStats[1].TurnsRouted != 2 {
		t.Errorf("expected Tier 2 to get 2 turns, got %d", res.TierStats[1].TurnsRouted)
	}
	if res.TierStats[2].TurnsRouted != 2 {
		t.Errorf("expected Tier 3 to get 2 turns, got %d", res.TierStats[2].TurnsRouted)
	}
	if res.TotalTurns != 6 {
		t.Errorf("expected 6 total turns, got %d", res.TotalTurns)
	}
	if res.TotalCostUSD <= 0 {
		t.Errorf("expected cloud cost > 0, got %f", res.TotalCostUSD)
	}
}

func TestMultiTierReplay_CounterfactualAttributionTrap(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:       "Tier 1: Local",
				Provider:       "ollama",
				IsLocal:        true,
				TokenThreshold: 8000,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 2: Cloud",
			Provider:       "openrouter",
			IsLocal:        false,
			CostPerMillion: 2.5,
		},
	}

	// Turn historically ran on Cloud and succeeded with write progress
	traj := SessionTrajectory{
		SessionID: "sess-cf-trap",
		Turns: []telemetry.TurnRecord{
			{
				Timestamp:        now,
				Tokens:           2000,
				IsLocal:          false, // Historically Cloud
				IsRetry:          false,
				HasWriteProgress: true,
				RootPromptHash:   0x123,
			},
		},
	}

	var res MultiTierReplayResult
	ReplayMultiTierSession(&traj, &cfg, &policy, &res)

	// Candidate routes to Local (2000 < 8000)
	if res.TierStats[0].TurnsRouted != 1 {
		t.Fatalf("expected turn to route to Tier 1, got %d", res.TierStats[0].TurnsRouted)
	}
	// Under counterfactual attribution, routing a historically cloud success to local
	// CANNOT assume local succeeds for free. It must count as a simulated retry/penalty.
	if res.TotalWastedRetries != 1 {
		t.Errorf("expected TotalWastedRetries == 1 for unproven counterfactual local turn, got %d", res.TotalWastedRetries)
	}
	if res.TierStats[0].WastedRetries != 1 {
		t.Errorf("expected Tier 1 WastedRetries == 1, got %d", res.TierStats[0].WastedRetries)
	}
}

func TestMultiTierReplay_KeywordExclusion(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:         "Tier 1: Local",
				Provider:         "ollama",
				IsLocal:          true,
				TokenThreshold:   8000,
				ExcludedKeywords: []string{"kubernetes", "terraform"},
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 2: Cloud",
			Provider:       "openrouter",
			IsLocal:        false,
			CostPerMillion: 2.0,
		},
	}

	traj := SessionTrajectory{
		SessionID: "sess-keyword",
		Turns: []telemetry.TurnRecord{
			{
				Timestamp:      now,
				Tokens:         1000,
				Keywords:       []string{"Kubernetes", "deploy"},
				RootPromptHash: 0x888,
			},
		},
	}

	var res MultiTierReplayResult
	ReplayMultiTierSession(&traj, &cfg, &policy, &res)

	if res.TierStats[0].TurnsRouted != 0 {
		t.Errorf("expected Tier 1 to be bypassed due to excluded keyword, got %d", res.TierStats[0].TurnsRouted)
	}
	if res.TierStats[1].TurnsRouted != 1 {
		t.Errorf("expected Tier 2 to receive the turn, got %d", res.TierStats[1].TurnsRouted)
	}
}

func TestMultiTierReplay_RealLifeTraffic(t *testing.T) {
	records := loadRealTrafficData(t)
	if len(records) == 0 {
		t.Skip("no real traffic records found")
	}

	trajectories := GroupBySession(records)
	if len(trajectories) == 0 {
		t.Fatalf("expected non-empty session trajectories from real traffic, got 0")
	}

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:       "Tier 1: Local GPU Workhorse",
				Provider:       "ollama",
				IsLocal:        true,
				TokenThreshold: 6000,
				RetryBound:     2,
			},
			{
				TierName:       "Tier 2: Workhorse Cloud",
				Provider:       "openrouter",
				IsLocal:        false,
				CostPerMillion: 2.0,
				TokenThreshold: 32000,
				RetryBound:     4,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 3: Fallback",
			Provider:       "openrouter",
			IsLocal:        false,
			CostPerMillion: 10.0,
		},
	}
	policy := DefaultTuningPolicy()

	var fleetRes MultiTierReplayResult
	ReplayMultiTierFleet(trajectories, &cfg, &policy, &fleetRes)

	if fleetRes.TotalTurns == 0 {
		t.Errorf("expected TotalTurns > 0, got 0")
	}
	if fleetRes.NumTiers != 3 {
		t.Errorf("expected 3 active tiers in stats, got %d", fleetRes.NumTiers)
	}

	// Verify all turns accounted for
	sumTurns := 0
	for i := 0; i < fleetRes.NumTiers; i++ {
		sumTurns += fleetRes.TierStats[i].TurnsRouted
	}
	if sumTurns != fleetRes.TotalTurns {
		t.Errorf("tier turns sum (%d) does not match total turns (%d)", sumTurns, fleetRes.TotalTurns)
	}
}

func BenchmarkReplayMultiTierSession_ZeroAlloc(b *testing.B) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:         "Tier 1: Local",
				Provider:         "ollama",
				IsLocal:          true,
				TokenThreshold:   4000,
				RetryBound:       2,
				RestrictImages:   true,
				RestrictTools:    true,
				ExcludedKeywords: []string{"ast", "concurrency"},
			},
			{
				TierName:         "Tier 2: Cloud Workhorse",
				Provider:         "openrouter",
				IsLocal:          false,
				CostPerMillion:   2.5,
				TokenThreshold:   16000,
				RetryBound:       4,
				ExcludedKeywords: []string{"deep-kernel"},
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 3: Frontier",
			Provider:       "anthropic",
			IsLocal:        false,
			CostPerMillion: 15.0,
		},
	}

	traj := SessionTrajectory{
		SessionID: "sess-bench",
		Turns: []telemetry.TurnRecord{
			{Timestamp: now.Add(1 * time.Second), Tokens: 2000, IsLocal: true, IsRetry: false, HasWriteProgress: true, RootPromptHash: 0x111, Keywords: []string{"refactor", "tests"}},
			{Timestamp: now.Add(2 * time.Second), Tokens: 3500, IsLocal: true, IsRetry: true, HasWriteProgress: false, RootPromptHash: 0x111, Keywords: []string{"bugfix"}},
			{Timestamp: now.Add(3 * time.Second), Tokens: 5000, IsLocal: false, IsRetry: true, HasWriteProgress: false, RootPromptHash: 0x111, Keywords: []string{"ast"}},
			{Timestamp: now.Add(4 * time.Second), Tokens: 12000, IsLocal: false, IsRetry: false, HasWriteProgress: true, RootPromptHash: 0x111, Keywords: []string{"speed"}},
			{Timestamp: now.Add(5 * time.Second), Tokens: 24000, IsLocal: false, IsRetry: false, HasWriteProgress: true, RootPromptHash: 0x111, Keywords: []string{"deploy"}},
		},
	}

	var res MultiTierReplayResult

	b.ReportAllocs()

	for b.Loop() {
		res.Reset()
		ReplayMultiTierSession(&traj, &cfg, &policy, &res)
	}

	// Verify it actually executed
	if res.TotalTurns != 5 {
		b.Fatalf("expected 5 turns, got %d", res.TotalTurns)
	}
}

func BenchmarkReplayMultiTierFleet_RealLifeData(b *testing.B) {
	records := loadRealTrafficData(b)
	if len(records) == 0 {
		b.Skip("no real traffic records found")
	}

	trajectories := GroupBySession(records)
	if len(trajectories) == 0 {
		b.Skip("no session trajectories")
	}

	policy := DefaultTuningPolicy()
	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:         "Tier 1: Local",
				Provider:         "ollama",
				IsLocal:          true,
				TokenThreshold:   4000,
				RetryBound:       2,
				RestrictImages:   true,
				RestrictTools:    true,
				ExcludedKeywords: []string{"ast", "concurrency"},
			},
			{
				TierName:         "Tier 2: Cloud Workhorse",
				Provider:         "openrouter",
				IsLocal:          false,
				CostPerMillion:   2.5,
				TokenThreshold:   16000,
				RetryBound:       4,
				ExcludedKeywords: []string{"deep-kernel"},
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Tier 3: Frontier",
			Provider:       "anthropic",
			IsLocal:        false,
			CostPerMillion: 15.0,
		},
	}

	var res MultiTierReplayResult

	b.ReportAllocs()

	for b.Loop() {
		res.Reset()
		ReplayMultiTierFleet(trajectories, &cfg, &policy, &res)
	}

	if res.TotalTurns == 0 {
		b.Fatalf("expected TotalTurns > 0")
	}
}

func TestMultiTierReplay_PositivePrerequisites(t *testing.T) {
	now := time.Now().UTC()
	policy := DefaultTuningPolicy()

	cfg := MultiTierConfig{
		Tiers: []TierReplayConfig{
			{
				TierName:          "Tier 0: Kickstart",
				Provider:          "openrouter",
				IsLocal:           false,
				RequiresKickstart: true,
				RetryBound:        3,
				SupportsVision:    true,
				SupportsTools:     true,
			},
			{
				TierName:       "Tier 1: Vision",
				Provider:       "openrouter",
				IsLocal:        false,
				RequiresImages: true,
				RetryBound:     2,
				SupportsVision: true,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 2: Local GPU",
				Provider:       "ollama",
				IsLocal:        true,
				TokenThreshold: 20000,
				RetryBound:     2,
				SupportsVision: false,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 3: Escalation",
				Provider:       "openrouter",
				IsLocal:        false,
				RetryFloor:     5,
				RetryBound:     8,
				SupportsVision: true,
				SupportsTools:  true,
			},
		},
		DefaultTier: TierReplayConfig{
			TierName:       "Default Tier",
			Provider:       "openrouter",
			IsLocal:        false,
			CostPerMillion: 2.0,
			SupportsVision: true,
			SupportsTools:  true,
		},
	}

	traj := SessionTrajectory{
		SessionID: "sess-prereq",
		Turns: []telemetry.TurnRecord{
			// Turn 0: Regular coding turn -> must bypass Tier 0 & 1 -> land on Tier 2 (Local GPU)
			{Timestamp: now.Add(1 * time.Second), Tokens: 2500, IsLocal: true, SessionKickstarted: false, HasImages: false, RootPromptHash: 0x1},
			// Turn 1: Kickstarted turn -> lands on Tier 0 (Kickstart)
			{Timestamp: now.Add(2 * time.Second), Tokens: 1500, IsLocal: false, SessionKickstarted: true, HasImages: false, RootPromptHash: 0x2},
			// Turn 2: Image turn -> lands on Tier 1 (Vision)
			{Timestamp: now.Add(3 * time.Second), Tokens: 3000, IsLocal: false, SessionKickstarted: false, HasImages: true, RootPromptHash: 0x3},
			// Turn 3: 5 retries turn -> lands on Tier 3 (Escalation with RetryFloor=5)
			{Timestamp: now.Add(4 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: true, RootPromptHash: 0x4},
			{Timestamp: now.Add(5 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: true, RootPromptHash: 0x4},
			{Timestamp: now.Add(6 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: true, RootPromptHash: 0x4},
			{Timestamp: now.Add(7 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: true, RootPromptHash: 0x4},
			{Timestamp: now.Add(8 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: true, RootPromptHash: 0x4},
			{Timestamp: now.Add(9 * time.Second), Tokens: 1000, IsLocal: false, IsRetry: false, RootPromptHash: 0x4}, // 5th failure escalates to RetryFloor=5
		},
	}

	var res MultiTierReplayResult
	ReplayMultiTierSession(&traj, &cfg, &policy, &res)

	// Turn 0: Tier 2 (Local GPU)
	if res.TierStats[2].TurnsRouted == 0 {
		t.Fatalf("Expected Tier 2 (Local GPU) to receive regular turns, got 0!")
	}
	// Turn 1: Tier 0 (Kickstart)
	if res.TierStats[0].TurnsRouted == 0 {
		t.Fatalf("Expected Tier 0 (Kickstart) to receive kickstarted turn, got 0!")
	}
	// Turn 2: Tier 1 (Vision)
	if res.TierStats[1].TurnsRouted == 0 {
		t.Fatalf("Expected Tier 1 (Vision) to receive image turn, got 0!")
	}
}
