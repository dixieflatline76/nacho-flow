package telemetry

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fixtureWindowExpected struct {
	Requests            int64   `json:"requests"`
	TokensTotal         int64   `json:"tokens_total"`
	TokensLocal         int64   `json:"tokens_local"`
	CostSpentUSD        float64 `json:"cost_spent_usd"`
	CostSavedUSD        float64 `json:"cost_saved_usd"`
	CostReductionPct    float64 `json:"cost_reduction_pct"`
	CKInterventions     int64   `json:"ck_interventions"`
	CKAvoidedTokens     int64   `json:"ck_avoided_tokens"`
	CKAvoidedGPUSecs    int64   `json:"ck_avoided_gpu_seconds"`
	CKStage1LocalHeals  int64   `json:"ck_stage1_local_heals"`
	CKStage2Escalations int64   `json:"ck_stage2_cloud_escalations"`
	FDTriggers          int64   `json:"fd_triggers"`
}

type fixtureRootExpected struct {
	ReferenceTime string                           `json:"reference_time"`
	Today         fixtureWindowExpected            `json:"today"`
	Yesterday     fixtureWindowExpected            `json:"yesterday"`
	ThisWeek      fixtureWindowExpected            `json:"this_week"`
	ThisMonth     fixtureWindowExpected            `json:"this_month"`
	AllTime       fixtureWindowExpected            `json:"all_time"`
	ByDay         map[string]fixtureWindowExpected `json:"by_day"`
}

func mustLoadTurns(t *testing.T, relPath string) []TurnRecord {
	t.Helper()
	path := filepath.Clean(relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read turns fixture %q: %v", path, err)
	}
	var records []TurnRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("failed to unmarshal turns fixture %q: %v", path, err)
	}
	return records
}

func mustLoadExpected(t *testing.T, relPath string) fixtureRootExpected {
	t.Helper()
	path := filepath.Clean(relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read expected fixture %q: %v", path, err)
	}
	var root fixtureRootExpected
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("failed to unmarshal expected fixture %q: %v", path, err)
	}
	return root
}

const floatTolerance = 1e-6

func assertWindowMetrics(t *testing.T, label string, got TimeWindowMetrics, exp fixtureWindowExpected) {
	t.Helper()
	assertEqual(t, label+".Requests", got.Requests, exp.Requests)
	assertEqual(t, label+".TokensTotal", got.TokensTotal, exp.TokensTotal)
	assertEqual(t, label+".TokensLocal", got.TokensLocal, exp.TokensLocal)
	assertFloat(t, label+".CostSpentUSD", got.CostSpentUSD, exp.CostSpentUSD)
	assertFloat(t, label+".CostSavedUSD", got.CostSavedUSD, exp.CostSavedUSD)
	assertFloat(t, label+".CostReductionPct", got.CostReductionPct, exp.CostReductionPct)
	assertEqual(t, label+".CK.Interventions", got.CycleKiller.TotalInterventions, exp.CKInterventions)
	assertEqual(t, label+".CK.AvoidedTokens", got.CycleKiller.AvoidedRunawayTokens, exp.CKAvoidedTokens)
	assertEqual(t, label+".CK.AvoidedGPUSecs", got.CycleKiller.AvoidedGPUSeconds, exp.CKAvoidedGPUSecs)
	assertEqual(t, label+".CK.Stage1Heals", got.CycleKiller.Stage1LocalHeals, exp.CKStage1LocalHeals)
	assertEqual(t, label+".CK.Stage2Escalations", got.CycleKiller.Stage2CloudEscalations, exp.CKStage2Escalations)
	assertEqual(t, label+".FairyDust.TotalTriggers", got.FairyDust.TotalTriggers, exp.FDTriggers)
}

func assertFloat(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > floatTolerance {
		t.Errorf("%s mismatch: got %.6f, want %.6f (diff: %.2e)", label, got, want, math.Abs(got-want))
	}
}

func assertEqual[T comparable](t *testing.T, label string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s mismatch: got %v, want %v", label, got, want)
	}
}

func TestStatsTracker_HistoricalFixtures(t *testing.T) {
	records := mustLoadTurns(t, "testdata/historical_turns.json")
	expected := mustLoadExpected(t, "testdata/historical_expected.json")

	refTime, err := time.Parse(time.RFC3339, expected.ReferenceTime)
	if err != nil {
		t.Fatalf("invalid reference time %q: %v", expected.ReferenceTime, err)
	}

	tracker := NewStatsTracker(100)
	defer tracker.Close()

	tracker.RecalculateFromRecordsAt(records, nil, 0, refTime)
	snap := tracker.GetStats()

	// 1. Assert pre-aggregated time windows
	assertWindowMetrics(t, "Windows.Today", snap.Windows.Today, expected.Today)
	assertWindowMetrics(t, "Windows.Yesterday", snap.Windows.Yesterday, expected.Yesterday)
	assertWindowMetrics(t, "Windows.ThisWeek", snap.Windows.ThisWeek, expected.ThisWeek)
	assertWindowMetrics(t, "Windows.ThisMonth", snap.Windows.ThisMonth, expected.ThisMonth)
	assertWindowMetrics(t, "Windows.AllTime", snap.Windows.AllTime, expected.AllTime)

	// 2. Assert every daily bucket matches ground truth
	if len(snap.DailyBuckets) != len(expected.ByDay) {
		t.Errorf("daily buckets count mismatch: got %d, want %d", len(snap.DailyBuckets), len(expected.ByDay))
	}

	for dateKey, expBucket := range expected.ByDay {
		gotBucket, ok := snap.DailyBuckets[dateKey]
		if !ok {
			t.Errorf("missing daily bucket %q in snapshot", dateKey)
			continue
		}
		assertWindowMetrics(t, "DailyBuckets["+dateKey+"]", gotBucket, expBucket)
	}
}

func TestStatsTracker_LegacyMigration_Fixtures(t *testing.T) {
	expected := mustLoadExpected(t, "testdata/historical_expected.json")

	refTime, err := time.Parse(time.RFC3339, expected.ReferenceTime)
	if err != nil {
		t.Fatalf("invalid reference time %q: %v", expected.ReferenceTime, err)
	}

	// 1. Scenario A: Legacy Snapshot (Root CK > 0, but no bucket has CK > 0)
	rootCK := CycleKillerMetrics{
		TotalInterventions:     expected.AllTime.CKInterventions,
		AvoidedRunawayTokens:   expected.AllTime.CKAvoidedTokens,
		AvoidedGPUSeconds:      expected.AllTime.CKAvoidedGPUSecs,
		Stage1LocalHeals:       expected.AllTime.CKStage1LocalHeals,
		Stage2CloudEscalations: expected.AllTime.CKStage2Escalations,
	}

	legacySnapshot := StatsSnapshot{
		TotalRequests: expected.AllTime.Requests,
		CycleKiller:   rootCK,
		DailyBuckets:  make(map[string]TimeWindowMetrics),
	}

	// Populate daily buckets with requests/tokens/spend but 0 CycleKiller (exact legacy shape)
	for dateKey, expBucket := range expected.ByDay {
		legacySnapshot.DailyBuckets[dateKey] = TimeWindowMetrics{
			Requests:         expBucket.Requests,
			TokensTotal:      expBucket.TokensTotal,
			TokensLocal:      expBucket.TokensLocal,
			CostSpentUSD:     expBucket.CostSpentUSD,
			CostSavedUSD:     expBucket.CostSavedUSD,
			CostReductionPct: expBucket.CostReductionPct,
			CycleKiller:      CycleKillerMetrics{}, // zeroed
		}
	}

	// Build tracker directly (not via NewStatsTrackerWithInitialSnapshot)
	// to avoid double-calling restoreWindowsFromBuckets. The constructor
	// calls it with time.Now(); we need it called once with refTime.
	tracker := &StatsTracker{
		obsChan:  make(chan Observation, 100),
		doneChan: make(chan struct{}),
		stats:    legacySnapshot,
	}
	go tracker.worker()
	tracker.restoreWindowsFromBuckets(refTime)
	snap := tracker.GetStats()

	// Assert that isLegacySnapshot fired and distributed root CK to windows with traffic
	if snap.Windows.Yesterday.Requests > 0 {
		assertEqual(t, "Legacy.Yesterday.CK.TotalInterventions", snap.Windows.Yesterday.CycleKiller.TotalInterventions, rootCK.TotalInterventions)
	}
	if snap.Windows.ThisWeek.Requests > 0 {
		assertEqual(t, "Legacy.ThisWeek.CK.TotalInterventions", snap.Windows.ThisWeek.CycleKiller.TotalInterventions, rootCK.TotalInterventions)
	}
	if snap.Windows.ThisMonth.Requests > 0 {
		assertEqual(t, "Legacy.ThisMonth.CK.TotalInterventions", snap.Windows.ThisMonth.CycleKiller.TotalInterventions, rootCK.TotalInterventions)
	}

	// 2. Scenario B: Modern Snapshot (At least one bucket has CK > 0)
	modernSnapshot := legacySnapshot
	modernSnapshot.DailyBuckets = make(map[string]TimeWindowMetrics, len(legacySnapshot.DailyBuckets))
	for k, v := range legacySnapshot.DailyBuckets {
		modernSnapshot.DailyBuckets[k] = v
	}

	// Give one historical bucket a nonzero intervention
	twoDaysAgoKey := refTime.AddDate(0, 0, -2).Format("2006-01-02")
	b := modernSnapshot.DailyBuckets[twoDaysAgoKey]
	b.CycleKiller.TotalInterventions = 1
	b.CycleKiller.Stage1LocalHeals = 1
	modernSnapshot.DailyBuckets[twoDaysAgoKey] = b

	tracker2 := &StatsTracker{
		obsChan:  make(chan Observation, 100),
		doneChan: make(chan struct{}),
		stats:    modernSnapshot,
	}
	go tracker2.worker()
	tracker2.restoreWindowsFromBuckets(refTime)
	snap2 := tracker2.GetStats()

	// isLegacySnapshot should be false — Yesterday (which had 0 in bucket) must stay 0, not inherit rootCK
	if snap2.Windows.Yesterday.Requests > 0 && expected.Yesterday.CKInterventions == 0 {
		assertEqual(t, "Modern.Yesterday.CK.TotalInterventions", snap2.Windows.Yesterday.CycleKiller.TotalInterventions, int64(0))
	}
}
