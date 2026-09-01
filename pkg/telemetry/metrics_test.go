package telemetry

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type mockSink struct {
	mu      sync.Mutex
	records []TurnRecord
}

func (m *mockSink) Emit(record TurnRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, record)
}

func (m *mockSink) Close() error {
	return nil
}

func TestStatsTracker_ObservationAggregation(t *testing.T) {
	tracker := NewStatsTracker(100)
	defer tracker.Close()

	// Tier 1 (Local Free) - 2 requests
	tracker.Record(Observation{
		Tier:       1,
		Tokens:     10000,
		CostSpent:  0.0,
		CostSaved:  0.045, // $4.50/1M
		IsLocal:    true,
		IsFallback: false,
		LatencyMs:  120.5,
	})
	tracker.Record(Observation{
		Tier:       1,
		Tokens:     5000,
		CostSpent:  0.0,
		CostSaved:  0.0225,
		IsLocal:    true,
		IsFallback: false,
		LatencyMs:  80.0,
	})

	// Tier 2 (Cloud Coder)
	tracker.Record(Observation{
		Tier:      2,
		Tokens:    2000,
		CostSpent: 0.004,
		CostSaved: 0.002,
		IsLocal:   false,
	})

	// Tier 3 (Cloud Reasoning)
	tracker.Record(Observation{
		Tier:      3,
		Tokens:    4000,
		CostSpent: 0.012,
		CostSaved: 0.0,
		IsLocal:   false,
	})

	// Tier 4 (Cloud Vision)
	tracker.Record(Observation{
		Tier:      4,
		Tokens:    1500,
		CostSpent: 0.003,
		CostSaved: 0.0,
		IsLocal:   false,
	})

	// Explicit override
	tracker.Record(Observation{
		Tier:               99,
		IsExplicitOverride: true,
		Tokens:             500,
		CostSpent:          0.001,
	})

	// Fallback
	tracker.Record(Observation{
		Tier:       2,
		IsFallback: true,
		Tokens:     3000,
		CostSpent:  0.006,
	})

	// Allow worker to drain observations
	tracker.Flush()

	stats := tracker.GetStats()

	if stats.TotalRequests != 7 {
		t.Errorf("expected 7 total requests, got %d", stats.TotalRequests)
	}
	if stats.TierBreakdown.Tier1LocalFree != 2 {
		t.Errorf("expected 2 tier1 requests, got %d", stats.TierBreakdown.Tier1LocalFree)
	}
	if stats.TierBreakdown.Tier2CloudCoder != 2 { // 1 normal tier2 + 1 fallback on tier2
		t.Errorf("expected 2 tier2 requests, got %d", stats.TierBreakdown.Tier2CloudCoder)
	}
	if stats.TierBreakdown.Tier3CloudReasoning != 1 {
		t.Errorf("expected 1 tier3 request, got %d", stats.TierBreakdown.Tier3CloudReasoning)
	}
	if stats.TierBreakdown.Tier4CloudVision != 1 {
		t.Errorf("expected 1 tier4 request, got %d", stats.TierBreakdown.Tier4CloudVision)
	}
	if stats.TierBreakdown.ExplicitOverride != 1 {
		t.Errorf("expected 1 explicit override, got %d", stats.TierBreakdown.ExplicitOverride)
	}
	if stats.TierBreakdown.Fallbacks != 1 {
		t.Errorf("expected 1 fallback, got %d", stats.TierBreakdown.Fallbacks)
	}
	if stats.TotalTokensRoutedLocally != 15000 {
		t.Errorf("expected 15000 tokens routed locally, got %d", stats.TotalTokensRoutedLocally)
	}
	if math.Abs(stats.EstimatedCostSavedUSD-0.0695) > 0.0001 {
		t.Errorf("expected ~0.0695 cost saved, got %f", stats.EstimatedCostSavedUSD)
	}
	if math.Abs(stats.TotalCostSpentUSD-0.026) > 0.0001 {
		t.Errorf("expected ~0.026 cost spent, got %f", stats.TotalCostSpentUSD)
	}
	// Reduction Pct: 0.0695 / (0.0695 + 0.0260) = 0.0695 / 0.0955 ~= 72.77%
	if stats.CostReductionPct < 72.0 || stats.CostReductionPct > 73.5 {
		t.Errorf("expected ~72.77%% cost reduction, got %f%%", stats.CostReductionPct)
	}
}

func TestStatsTracker_DualFinancialMetrics(t *testing.T) {
	tracker := NewStatsTracker(100)
	defer tracker.Close()

	// 1. Zero state test (no requests) -> 0.0% reduction, no NaN
	zeroStats := tracker.GetStats()
	if math.IsNaN(zeroStats.CostReductionPct) || math.IsInf(zeroStats.CostReductionPct, 0) {
		t.Fatalf("CostReductionPct must not be NaN or Inf on zero stats")
	}
	if zeroStats.CostReductionPct != 0.0 {
		t.Errorf("expected 0.0 reduction pct on empty tracker, got %f", zeroStats.CostReductionPct)
	}

	// 2. 100% Free / Saved ($10 saved, $0 spent) -> 100.0% reduction
	tracker.Record(Observation{
		Tokens:    100000,
		CostSaved: 10.0,
		CostSpent: 0.0,
		IsLocal:   true,
	})
	tracker.Flush()

	stats := tracker.GetStats()
	if stats.CostReductionPct != 100.0 {
		t.Errorf("expected 100.0%% cost reduction, got %f%%", stats.CostReductionPct)
	}

	// 3. 50% Reduction ($10 saved, $10 spent) -> 50.0% reduction
	tracker.Record(Observation{
		Tokens:    100000,
		CostSaved: 0.0,
		CostSpent: 10.0,
		IsLocal:   false,
	})
	tracker.Flush()

	stats = tracker.GetStats()
	if stats.EstimatedCostSavedUSD != 10.0 || stats.TotalCostSpentUSD != 10.0 {
		t.Errorf("expected $10 saved and $10 spent, got saved=%f, spent=%f", stats.EstimatedCostSavedUSD, stats.TotalCostSpentUSD)
	}
	if stats.CostReductionPct != 50.0 {
		t.Errorf("expected 50.0%% cost reduction, got %f%%", stats.CostReductionPct)
	}
}

func TestStatsTracker_PreAggregatedTimeWindows(t *testing.T) {
	tracker := NewStatsTracker(100)
	defer tracker.Close()

	// Use fixed reference date: Wednesday 2026-08-19
	dayMonday := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	dayTuesday := time.Date(2026, 8, 18, 14, 0, 0, 0, time.UTC)
	dayWednesday := time.Date(2026, 8, 19, 16, 0, 0, 0, time.UTC)
	dayPrevMonth := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)

	// Previous month
	tracker.Record(Observation{
		Tokens:     5000,
		CostSpent:  1.0,
		CostSaved:  2.0,
		ObservedAt: dayPrevMonth,
	})

	// Monday this week
	tracker.Record(Observation{
		Tokens:     10000,
		CostSpent:  1.0,
		CostSaved:  9.0, // 90% reduction
		IsLocal:    true,
		ObservedAt: dayMonday,
	})

	// Tuesday this week
	tracker.Record(Observation{
		Tokens:     20000,
		CostSpent:  2.0,
		CostSaved:  8.0, // 80% reduction
		IsLocal:    false,
		ObservedAt: dayTuesday,
	})

	// Wednesday (Today)
	tracker.Record(Observation{
		Tokens:     10000,
		CostSpent:  1.0,
		CostSaved:  4.0, // 80% reduction
		IsLocal:    true,
		ObservedAt: dayWednesday,
	})

	tracker.Flush()
	stats := tracker.GetStats()

	// Verify Today (Wednesday 2026-08-19)
	today := stats.Windows.Today
	if today.Requests != 1 {
		t.Errorf("expected 1 request today, got %d", today.Requests)
	}
	if today.TokensTotal != 10000 || today.TokensLocal != 10000 {
		t.Errorf("expected 10000 total/local tokens today, got total=%d, local=%d", today.TokensTotal, today.TokensLocal)
	}
	if today.CostSpentUSD != 1.0 || today.CostSavedUSD != 4.0 {
		t.Errorf("expected $1 spent, $4 saved today, got spent=%f, saved=%f", today.CostSpentUSD, today.CostSavedUSD)
	}
	if today.CostReductionPct != 80.0 {
		t.Errorf("expected 80.0%% today reduction, got %f%%", today.CostReductionPct)
	}

	// Verify ThisWeek (Mon-Wed: 3 requests, 40000 tokens, $4 spent, $21 saved)
	week := stats.Windows.ThisWeek
	if week.Requests != 3 {
		t.Errorf("expected 3 requests this week, got %d", week.Requests)
	}
	if week.TokensTotal != 40000 || week.TokensLocal != 20000 {
		t.Errorf("expected 40000 total / 20000 local tokens this week, got total=%d, local=%d", week.TokensTotal, week.TokensLocal)
	}
	if week.CostSpentUSD != 4.0 || week.CostSavedUSD != 21.0 {
		t.Errorf("expected $4 spent, $21 saved this week, got spent=%f, saved=%f", week.CostSpentUSD, week.CostSavedUSD)
	}
	// 21 / (21 + 4) = 21 / 25 = 84%
	if week.CostReductionPct != 84.0 {
		t.Errorf("expected 84.0%% week reduction, got %f%%", week.CostReductionPct)
	}

	// Verify ThisMonth (August 2026: 3 requests, $4 spent, $21 saved)
	month := stats.Windows.ThisMonth
	if month.Requests != 3 {
		t.Errorf("expected 3 requests this month, got %d", month.Requests)
	}
	if month.CostSpentUSD != 4.0 || month.CostSavedUSD != 21.0 {
		t.Errorf("expected $4 spent, $21 saved this month, got spent=%f, saved=%f", month.CostSpentUSD, month.CostSavedUSD)
	}

	// Verify AllTime (4 requests: $5 spent, $23 saved)
	allTime := stats.Windows.AllTime
	if allTime.Requests != 4 {
		t.Errorf("expected 4 requests all time, got %d", allTime.Requests)
	}
	if allTime.CostSpentUSD != 5.0 || allTime.CostSavedUSD != 23.0 {
		t.Errorf("expected $5 spent, $23 saved all time, got spent=%f, saved=%f", allTime.CostSpentUSD, allTime.CostSavedUSD)
	}
}

func TestStatsTracker_SinkEmissionWithCostSpent(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	sink := &mockSink{}
	tracker.AddSink(sink)

	tracker.Record(Observation{
		Tier:       2,
		TierName:   "Tier 2: Cloud Coder",
		Model:      "qwen/qwen-2.5-coder-32b",
		Provider:   "openrouter",
		Tokens:     3500,
		CostSpent:  0.007,
		CostSaved:  0.0035,
		IsLocal:    false,
		LatencyMs:  450.2,
		StatusCode: 200,
	})
	tracker.Flush()

	sink.mu.Lock()
	defer sink.mu.Unlock()

	if len(sink.records) != 1 {
		t.Fatalf("expected 1 emitted turn record, got %d", len(sink.records))
	}
	rec := sink.records[0]
	if rec.CostSpentUSD != 0.007 {
		t.Errorf("expected CostSpentUSD 0.007, got %f", rec.CostSpentUSD)
	}
	if rec.CostSavedUSD != 0.0035 {
		t.Errorf("expected CostSavedUSD 0.0035, got %f", rec.CostSavedUSD)
	}
}

func TestStatsTracker_SinkEmission_FairyDustAndKickstart(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	sink := &mockSink{}
	tracker.AddSink(sink)

	tracker.Record(Observation{
		Tier:               3,
		TierName:           "Tier 3: Cloud Reasoning",
		Model:              "anthropic/claude-sonnet-5",
		Provider:           "openrouter",
		Tokens:             5000,
		CostSpent:          0.015,
		CostSaved:          0.005,
		IsLocal:            false,
		LatencyMs:          850.0,
		StatusCode:         200,
		SessionKickstarted: true,
		FairyDusted:        true,
		FairyDustEntry:     "Tactical Code Review",
		CachedTokens:       1200,
		UpstreamCost:       0.0145,
	})
	tracker.Flush()

	sink.mu.Lock()
	defer sink.mu.Unlock()

	if len(sink.records) != 1 {
		t.Fatalf("expected 1 emitted turn record, got %d", len(sink.records))
	}
	rec := sink.records[0]
	if !rec.SessionKickstarted {
		t.Errorf("expected SessionKickstarted true, got false")
	}
	if !rec.FairyDusted {
		t.Errorf("expected FairyDusted true, got false")
	}
	if rec.FairyDustEntry != "Tactical Code Review" {
		t.Errorf("expected FairyDustEntry 'Tactical Code Review', got %s", rec.FairyDustEntry)
	}
	if rec.CachedTokens != 1200 {
		t.Errorf("expected CachedTokens 1200, got %d", rec.CachedTokens)
	}
	if rec.UpstreamCost != 0.0145 {
		t.Errorf("expected UpstreamCost 0.0145, got %f", rec.UpstreamCost)
	}
}


func TestStatsTracker_RollingBucketPruning(t *testing.T) {
	tracker := NewStatsTracker(100)
	defer tracker.Close()

	now := time.Now().UTC()
	oldDate := now.AddDate(0, 0, -45) // 45 days ago

	tracker.Record(Observation{
		Tokens:     1000,
		CostSpent:  1.0,
		ObservedAt: oldDate,
	})
	tracker.Record(Observation{
		Tokens:     1000,
		CostSpent:  1.0,
		ObservedAt: now,
	})
	tracker.Flush()

	stats := tracker.GetStats()
	if stats.DailyBuckets == nil {
		t.Fatalf("expected DailyBuckets map to be initialized")
	}

	oldKey := oldDate.Format("2006-01-02")
	if _, exists := stats.DailyBuckets[oldKey]; exists {
		t.Errorf("expected bucket older than 31 days to be pruned, but found %s", oldKey)
	}

	todayKey := now.Format("2006-01-02")
	if _, exists := stats.DailyBuckets[todayKey]; !exists {
		t.Errorf("expected current day bucket %s to exist", todayKey)
	}
}

func TestStatsTracker_JSONPersistenceAndRestoration(t *testing.T) {
	now := time.Now().UTC()
	initial := StatsSnapshot{
		StartedAt:                now.Format(time.RFC3339),
		TotalRequests:            10,
		TotalTokensRoutedLocally: 50000,
		TotalCostSpentUSD:        2.50,
		EstimatedCostSavedUSD:    15.00,
		CostReductionPct:         85.71,
		TierBreakdown: TierMetrics{
			Tier1LocalFree:  8,
			Tier2CloudCoder: 2,
		},
		DailyBuckets: map[string]TimeWindowMetrics{
			now.Format("2006-01-02"): {
				Requests:         10,
				TokensTotal:      60000,
				TokensLocal:      50000,
				CostSpentUSD:     2.50,
				CostSavedUSD:     15.00,
				CostReductionPct: 85.71,
			},
		},
	}

	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatalf("failed to marshal initial snapshot: %v", err)
	}

	var unmarshaled StatsSnapshot
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}

	tracker := NewStatsTrackerWithInitialSnapshot(100, unmarshaled)
	defer tracker.Close()

	stats := tracker.GetStats()
	if stats.TotalRequests != 10 {
		t.Errorf("expected 10 total requests, got %d", stats.TotalRequests)
	}
	if stats.TotalCostSpentUSD != 2.50 {
		t.Errorf("expected $2.50 spent, got %f", stats.TotalCostSpentUSD)
	}
	if stats.EstimatedCostSavedUSD != 15.00 {
		t.Errorf("expected $15.00 saved, got %f", stats.EstimatedCostSavedUSD)
	}
	if stats.Windows.Today.Requests != 10 {
		t.Errorf("expected 10 requests in Today window, got %d", stats.Windows.Today.Requests)
	}
}

func TestStatsTracker_HighConcurrency_Race(t *testing.T) {
	tracker := NewStatsTracker(5000)
	defer tracker.Close()

	var wg sync.WaitGroup
	workers := 20
	iterations := 250

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				tracker.Record(Observation{
					Tier:      (j % 4) + 1,
					Tokens:    100,
					CostSpent: 0.0001,
					CostSaved: 0.00045,
					IsLocal:   (j%4)+1 == 1,
				})
				// Read snapshot concurrently
				if j%25 == 0 {
					_ = tracker.GetStats()
				}
			}
		}(i)
	}

	wg.Wait()
	tracker.Flush()

	stats := tracker.GetStats()
	expectedRequests := workers * iterations
	if stats.TotalRequests != int64(expectedRequests) {
		t.Errorf("expected %d requests, got %d", expectedRequests, stats.TotalRequests)
	}
	if stats.TotalCostSpentUSD <= 0 {
		t.Errorf("expected positive TotalCostSpentUSD under concurrency")
	}
	if stats.EstimatedCostSavedUSD <= 0 {
		t.Errorf("expected positive EstimatedCostSavedUSD under concurrency")
	}
	if stats.Windows.Today.Requests != int64(expectedRequests) {
		t.Errorf("expected %d requests in Today window, got %d", expectedRequests, stats.Windows.Today.Requests)
	}
}

func TestStatsTracker_HTTPHandler(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	tracker.Record(Observation{
		Tier:      1,
		Tokens:    1000,
		CostSpent: 0.0,
		CostSaved: 0.0045,
		IsLocal:   true,
	})
	tracker.Flush()

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	rec := httptest.NewRecorder()

	tracker.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var parsed map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&parsed); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}

	if _, ok := parsed["started_at"]; !ok {
		t.Errorf("expected started_at field in json")
	}
	if parsed["total_requests"] != float64(1) {
		t.Errorf("expected total_requests 1, got %v", parsed["total_requests"])
	}
	if parsed["total_tokens_routed_locally"] != float64(1000) {
		t.Errorf("expected total_tokens_routed_locally 1000, got %v", parsed["total_tokens_routed_locally"])
	}
	if _, ok := parsed["windows"]; !ok {
		t.Errorf("expected windows field in json response")
	}
	if _, ok := parsed["cost_reduction_pct"]; !ok {
		t.Errorf("expected cost_reduction_pct field in json response")
	}

	// Test Handler() adapter
	handler := tracker.Handler()
	if handler == nil {
		t.Fatalf("Expected non-nil http.HandlerFunc from tracker.Handler()")
	}

	// Test 405 on non-GET
	postReq := httptest.NewRequest(http.MethodPost, "/v1/stats", nil)
	postRec := httptest.NewRecorder()
	tracker.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed for POST /v1/stats, got %d", postRec.Code)
	}

	// Test default buffer clamping (bufferSize <= 0)
	clampedTracker := NewStatsTracker(0)
	clampedTracker.Close()
	// Double close should not panic
	clampedTracker.Close()
}

func TestStatsTracker_CycleKiller_LocalHeal(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	tracker.Record(Observation{
		Tier:                  1,
		Tokens:                500,
		IsLocal:               true,
		IsFallback:            false,
		CycleBreakerTriggered: true,
		CycleBreakerReason:    "thinking_budget_exceeded",
	})
	tracker.Flush()

	stats := tracker.GetStats()
	if stats.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected 1 total intervention, got %d", stats.CycleKiller.TotalInterventions)
	}
	if stats.CycleKiller.Stage1LocalHeals != 1 {
		t.Errorf("expected 1 stage 1 local heal, got %d", stats.CycleKiller.Stage1LocalHeals)
	}
	if stats.CycleKiller.Stage2CloudEscalations != 0 {
		t.Errorf("expected 0 stage 2 cloud escalations, got %d", stats.CycleKiller.Stage2CloudEscalations)
	}
	if stats.CycleKiller.AvoidedRunawayTokens != 8000 {
		t.Errorf("expected 8000 avoided tokens, got %d", stats.CycleKiller.AvoidedRunawayTokens)
	}
	expectedSeconds := int64(8000 / 35)
	if stats.CycleKiller.AvoidedGPUSeconds != expectedSeconds {
		t.Errorf("expected %d avoided GPU seconds, got %d", expectedSeconds, stats.CycleKiller.AvoidedGPUSeconds)
	}
	if stats.CycleKiller.LocalHealSuccessRatePct != 100.0 {
		t.Errorf("expected 100.0%% local heal success rate, got %f", stats.CycleKiller.LocalHealSuccessRatePct)
	}
}

func TestStatsTracker_CycleKiller_CloudEscalation(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	tracker.Record(Observation{
		Tier:                  2,
		Tokens:                500,
		IsLocal:               false,
		IsFallback:            true,
		CycleBreakerTriggered: true,
		CycleBreakerReason:    "prose_token_budget_exceeded",
	})
	tracker.Flush()

	stats := tracker.GetStats()
	if stats.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected 1 total intervention, got %d", stats.CycleKiller.TotalInterventions)
	}
	if stats.CycleKiller.Stage1LocalHeals != 0 {
		t.Errorf("expected 0 stage 1 local heals, got %d", stats.CycleKiller.Stage1LocalHeals)
	}
	if stats.CycleKiller.Stage2CloudEscalations != 1 {
		t.Errorf("expected 1 stage 2 cloud escalation, got %d", stats.CycleKiller.Stage2CloudEscalations)
	}
	if stats.CycleKiller.LocalHealSuccessRatePct != 0.0 {
		t.Errorf("expected 0.0%% local heal success rate, got %f", stats.CycleKiller.LocalHealSuccessRatePct)
	}
}

func TestStatsTracker_CycleKiller_HealRate(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	// 3 Local Heals
	for i := 0; i < 3; i++ {
		tracker.Record(Observation{
			Tier:                  1,
			IsLocal:               true,
			IsFallback:            false,
			CycleBreakerTriggered: true,
		})
	}
	// 1 Cloud Escalation
	tracker.Record(Observation{
		Tier:                  2,
		IsLocal:               false,
		IsFallback:            true,
		CycleBreakerTriggered: true,
	})
	tracker.Flush()

	stats := tracker.GetStats()
	if stats.CycleKiller.TotalInterventions != 4 {
		t.Errorf("expected 4 total interventions, got %d", stats.CycleKiller.TotalInterventions)
	}
	if stats.CycleKiller.Stage1LocalHeals != 3 {
		t.Errorf("expected 3 stage 1 local heals, got %d", stats.CycleKiller.Stage1LocalHeals)
	}
	if stats.CycleKiller.Stage2CloudEscalations != 1 {
		t.Errorf("expected 1 stage 2 cloud escalation, got %d", stats.CycleKiller.Stage2CloudEscalations)
	}
	if stats.CycleKiller.LocalHealSuccessRatePct != 75.0 {
		t.Errorf("expected 75.0%% local heal success rate, got %f", stats.CycleKiller.LocalHealSuccessRatePct)
	}
}

func TestStatsTracker_CycleKiller_ZeroDivision(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	stats := tracker.GetStats()
	if stats.CycleKiller.LocalHealSuccessRatePct != 0.0 {
		t.Errorf("expected 0.0%% local heal rate when interventions == 0, got %f", stats.CycleKiller.LocalHealSuccessRatePct)
	}
}

func TestStatsTracker_CycleKiller_NotTriggered(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	tracker.Record(Observation{
		Tier:                  1,
		Tokens:                500,
		IsLocal:               true,
		IsFallback:            false,
		CycleBreakerTriggered: false,
	})
	tracker.Flush()

	stats := tracker.GetStats()
	if stats.CycleKiller.TotalInterventions != 0 {
		t.Errorf("expected 0 total interventions, got %d", stats.CycleKiller.TotalInterventions)
	}
	if stats.CycleKiller.AvoidedRunawayTokens != 0 {
		t.Errorf("expected 0 avoided runaway tokens, got %d", stats.CycleKiller.AvoidedRunawayTokens)
	}
	if stats.CycleKiller.AvoidedGPUSeconds != 0 {
		t.Errorf("expected 0 avoided GPU seconds, got %d", stats.CycleKiller.AvoidedGPUSeconds)
	}
}

func TestStatsTracker_CycleKiller_Recalculate(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	records := []TurnRecord{
		{
			Tokens:                100,
			SelectedTier:          "Tier 1: Local GPU Free",
			IsLocal:               true,
			IsFallback:            false,
			CycleBreakerTriggered: true,
		},
		{
			Tokens:                200,
			SelectedTier:          "Tier 2: Cloud Workhorse",
			IsLocal:               false,
			IsFallback:            true,
			CycleBreakerTriggered: true,
		},
		{
			Tokens:                300,
			SelectedTier:          "Tier 1: Local GPU Free",
			IsLocal:               true,
			IsFallback:            false,
			CycleBreakerTriggered: false,
		},
	}

	tracker.RecalculateFromRecords(records, nil, 3.00)

	stats := tracker.GetStats()
	if stats.CycleKiller.TotalInterventions != 2 {
		t.Errorf("expected 2 total interventions after recalculate, got %d", stats.CycleKiller.TotalInterventions)
	}
	if stats.CycleKiller.Stage1LocalHeals != 1 {
		t.Errorf("expected 1 stage 1 local heal, got %d", stats.CycleKiller.Stage1LocalHeals)
	}
	if stats.CycleKiller.Stage2CloudEscalations != 1 {
		t.Errorf("expected 1 stage 2 cloud escalation, got %d", stats.CycleKiller.Stage2CloudEscalations)
	}
	if stats.CycleKiller.AvoidedRunawayTokens != 16000 {
		t.Errorf("expected 16000 avoided tokens, got %d", stats.CycleKiller.AvoidedRunawayTokens)
	}
	if stats.CycleKiller.LocalHealSuccessRatePct != 50.0 {
		t.Errorf("expected 50.0%% local heal success rate, got %f", stats.CycleKiller.LocalHealSuccessRatePct)
	}
}

func TestStatsTracker_CycleKiller_TimeWindowBucketing(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	day1 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 8, 21, 14, 0, 0, 0, time.UTC)

	// Day 1: 1 local loop heal intervention
	tracker.Record(Observation{
		Tier:                  1,
		Tokens:                5000,
		CostSpent:             0.0,
		CostSaved:             0.02,
		IsLocal:               true,
		IsFallback:            false,
		ObservedAt:            day1,
		CycleBreakerTriggered: true,
	})

	// Day 2: 1 clean turn, 0 loop interventions
	tracker.Record(Observation{
		Tier:                  1,
		Tokens:                3000,
		CostSpent:             0.0,
		CostSaved:             0.01,
		IsLocal:               true,
		IsFallback:            false,
		ObservedAt:            day2,
		CycleBreakerTriggered: false,
	})

	tracker.Flush()
	stats := tracker.GetStats()

	// Today (Day 2) should have 0 Cycle Killer interventions
	if stats.Windows.Today.CycleKiller.TotalInterventions != 0 {
		t.Errorf("expected Today (Day 2) CycleKiller.TotalInterventions == 0, got %d", stats.Windows.Today.CycleKiller.TotalInterventions)
	}
	if stats.Windows.Today.CycleKiller.AvoidedRunawayTokens != 0 {
		t.Errorf("expected Today (Day 2) AvoidedRunawayTokens == 0, got %d", stats.Windows.Today.CycleKiller.AvoidedRunawayTokens)
	}

	// Daily bucket for Day 1 should have 1 intervention
	b1 := stats.DailyBuckets["2026-08-20"]
	if b1.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected Day 1 bucket TotalInterventions == 1, got %d", b1.CycleKiller.TotalInterventions)
	}
	if b1.CycleKiller.Stage1LocalHeals != 1 {
		t.Errorf("expected Day 1 bucket Stage1LocalHeals == 1, got %d", b1.CycleKiller.Stage1LocalHeals)
	}

	// AllTime should have 1 intervention
	if stats.Windows.AllTime.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected AllTime CycleKiller.TotalInterventions == 1, got %d", stats.Windows.AllTime.CycleKiller.TotalInterventions)
	}
	if stats.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected Global CycleKiller.TotalInterventions == 1, got %d", stats.CycleKiller.TotalInterventions)
	}
}

func TestStatsTracker_CycleKiller_WeeklyMonthlyAggregation(t *testing.T) {
	tracker := NewStatsTracker(20)
	defer tracker.Close()

	// Wednesday, August 26, 2026 (Week 35)
	day1 := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	// Thursday, August 27, 2026 (Week 35)
	day2 := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	// Day 1: Stage 1 local heal
	tracker.Record(Observation{
		Tier:                  1,
		Tokens:                4000,
		IsLocal:               true,
		IsFallback:            false,
		ObservedAt:            day1,
		CycleBreakerTriggered: true,
	})

	// Day 2: Stage 2 cloud escalation
	tracker.Record(Observation{
		Tier:                  2,
		Tokens:                6000,
		CostSpent:             0.005,
		IsLocal:               false,
		IsFallback:            true,
		ObservedAt:            day2,
		CycleBreakerTriggered: true,
	})

	tracker.Flush()
	stats := tracker.GetStats()

	// Today (Day 2) should show only Day 2's escalation
	if stats.Windows.Today.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected Today TotalInterventions == 1, got %d", stats.Windows.Today.CycleKiller.TotalInterventions)
	}
	if stats.Windows.Today.CycleKiller.Stage2CloudEscalations != 1 {
		t.Errorf("expected Today Stage2CloudEscalations == 1, got %d", stats.Windows.Today.CycleKiller.Stage2CloudEscalations)
	}
	if stats.Windows.Today.CycleKiller.Stage1LocalHeals != 0 {
		t.Errorf("expected Today Stage1LocalHeals == 0, got %d", stats.Windows.Today.CycleKiller.Stage1LocalHeals)
	}

	// ThisWeek (Week 35) should aggregate Day 1 + Day 2 = 2 interventions
	if stats.Windows.ThisWeek.CycleKiller.TotalInterventions != 2 {
		t.Errorf("expected ThisWeek TotalInterventions == 2, got %d", stats.Windows.ThisWeek.CycleKiller.TotalInterventions)
	}
	if stats.Windows.ThisWeek.CycleKiller.Stage1LocalHeals != 1 {
		t.Errorf("expected ThisWeek Stage1LocalHeals == 1, got %d", stats.Windows.ThisWeek.CycleKiller.Stage1LocalHeals)
	}
	if stats.Windows.ThisWeek.CycleKiller.Stage2CloudEscalations != 1 {
		t.Errorf("expected ThisWeek Stage2CloudEscalations == 1, got %d", stats.Windows.ThisWeek.CycleKiller.Stage2CloudEscalations)
	}
	if stats.Windows.ThisWeek.CycleKiller.AvoidedRunawayTokens != 16000 {
		t.Errorf("expected ThisWeek AvoidedRunawayTokens == 16000, got %d", stats.Windows.ThisWeek.CycleKiller.AvoidedRunawayTokens)
	}
	if stats.Windows.ThisWeek.CycleKiller.LocalHealSuccessRatePct != 50.0 {
		t.Errorf("expected ThisWeek LocalHealSuccessRatePct == 50.0, got %f", stats.Windows.ThisWeek.CycleKiller.LocalHealSuccessRatePct)
	}

	// ThisMonth (August 2026) should also have 2 interventions
	if stats.Windows.ThisMonth.CycleKiller.TotalInterventions != 2 {
		t.Errorf("expected ThisMonth TotalInterventions == 2, got %d", stats.Windows.ThisMonth.CycleKiller.TotalInterventions)
	}

	// Verify JSON serialization includes windowed cycle_killer
	bytes, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("failed to marshal stats to JSON: %v", err)
	}
	var unmarshaled StatsSnapshot
	if err := json.Unmarshal(bytes, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if unmarshaled.Windows.Today.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected unmarshaled Today TotalInterventions == 1, got %d", unmarshaled.Windows.Today.CycleKiller.TotalInterventions)
	}
	if unmarshaled.Windows.ThisWeek.CycleKiller.TotalInterventions != 2 {
		t.Errorf("expected unmarshaled ThisWeek TotalInterventions == 2, got %d", unmarshaled.Windows.ThisWeek.CycleKiller.TotalInterventions)
	}
}

func TestStatsTracker_YesterdayTimeWindow(t *testing.T) {
	now := time.Now().UTC()
	todayTime := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, time.UTC)
	tomorrowTime := todayTime.AddDate(0, 0, 1)
	yesterdayTime := todayTime.AddDate(0, 0, -1)
	yesterdayLateTime := time.Date(yesterdayTime.Year(), yesterdayTime.Month(), yesterdayTime.Day(), 23, 59, 0, 0, time.UTC)
	yesterdayKey := yesterdayTime.Format("2006-01-02")

	// Seed with prior daily bucket for yesterdayKey (Yesterday)
	initial := StatsSnapshot{
		StartedAt: "2026-08-01T00:00:00Z",
		DailyBuckets: map[string]TimeWindowMetrics{
			yesterdayKey: {
				Requests:         15,
				TokensTotal:      75000,
				TokensLocal:      60000,
				CostSpentUSD:     0.25,
				CostSavedUSD:     1.75,
				CostReductionPct: 87.5,
				CycleKiller: CycleKillerMetrics{
					TotalInterventions:     3,
					AvoidedRunawayTokens:   24000,
					AvoidedGPUSeconds:      685,
					Stage1LocalHeals:       2,
					Stage2CloudEscalations: 1,
				},
			},
		},
	}

	tracker := NewStatsTrackerWithInitialSnapshot(100, initial)
	defer tracker.Close()

	// 1. Check restoration from DailyBuckets on startup
	stats := tracker.GetStats()
	if stats.Windows.Yesterday.Requests != 15 {
		t.Errorf("expected Yesterday.Requests == 15, got %d", stats.Windows.Yesterday.Requests)
	}
	if stats.Windows.Yesterday.TokensTotal != 75000 {
		t.Errorf("expected Yesterday.TokensTotal == 75000, got %d", stats.Windows.Yesterday.TokensTotal)
	}
	if stats.Windows.Yesterday.CostSavedUSD != 1.75 {
		t.Errorf("expected Yesterday.CostSavedUSD == 1.75, got %f", stats.Windows.Yesterday.CostSavedUSD)
	}
	if stats.Windows.Yesterday.CycleKiller.TotalInterventions != 3 {
		t.Errorf("expected Yesterday.CycleKiller.TotalInterventions == 3, got %d", stats.Windows.Yesterday.CycleKiller.TotalInterventions)
	}
	if stats.Windows.Yesterday.CycleKiller.LocalHealSuccessRatePct < 66.0 || stats.Windows.Yesterday.CycleKiller.LocalHealSuccessRatePct > 67.0 {
		t.Errorf("expected Yesterday.CycleKiller.LocalHealSuccessRatePct ~= 66.67, got %f", stats.Windows.Yesterday.CycleKiller.LocalHealSuccessRatePct)
	}

	// 2. Record new observation on Today (2026-08-31)
	tracker.Record(Observation{
		Tier:                  1,
		Tokens:                5000,
		CostSaved:             0.05,
		IsLocal:               true,
		ObservedAt:            todayTime,
		CycleBreakerTriggered: true,
	})
	// Record a late observation arriving timestamped for Yesterday
	tracker.Record(Observation{
		Tier:       1,
		Tokens:     1000,
		CostSaved:  0.01,
		IsLocal:    true,
		ObservedAt: yesterdayLateTime,
	})
	tracker.Flush()

	stats = tracker.GetStats()
	// Yesterday must now reflect the 15 original + 1 late request = 16 requests
	if stats.Windows.Yesterday.Requests != 16 {
		t.Errorf("expected Yesterday.Requests to be 16 after late arrival, got %d", stats.Windows.Yesterday.Requests)
	}
	if stats.Windows.Yesterday.CycleKiller.TotalInterventions != 3 {
		t.Errorf("expected Yesterday interventions to remain 3, got %d", stats.Windows.Yesterday.CycleKiller.TotalInterventions)
	}
	// Today must reflect the today observation (1 request)
	if stats.Windows.Today.Requests != 1 {
		t.Errorf("expected Today.Requests == 1, got %d", stats.Windows.Today.Requests)
	}
	if stats.Windows.Today.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected Today interventions == 1, got %d", stats.Windows.Today.CycleKiller.TotalInterventions)
	}

	// 3. Day rollover: simulate tomorrow (2026-09-01)
	tracker.Record(Observation{
		Tier:       2,
		Tokens:     10000,
		CostSpent:  0.02,
		ObservedAt: tomorrowTime,
	})
	tracker.Flush()

	stats = tracker.GetStats()
	// On 2026-09-01, Yesterday should now be 2026-08-31 (1 request, 1 intervention)
	if stats.Windows.Yesterday.Requests != 1 {
		t.Errorf("expected Yesterday.Requests after rollover == 1, got %d", stats.Windows.Yesterday.Requests)
	}
	if stats.Windows.Yesterday.CycleKiller.TotalInterventions != 1 {
		t.Errorf("expected Yesterday.CycleKiller.TotalInterventions after rollover == 1, got %d", stats.Windows.Yesterday.CycleKiller.TotalInterventions)
	}
	if stats.Windows.Today.Requests != 1 {
		t.Errorf("expected Today.Requests on new day == 1, got %d", stats.Windows.Today.Requests)
	}

	// 4. Verify JSON marshaling includes yesterday
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("failed to marshal stats JSON: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	windows, ok := raw["windows"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing windows object in JSON")
	}
	if _, ok := windows["yesterday"]; !ok {
		t.Errorf("missing yesterday window in JSON serialization: %v", string(data))
	}
}


func TestStatsTracker_CycleKiller_LegacyMigrationBackfillsBuckets(t *testing.T) {
	// Simulate a stats.json from the pre-fix build: root CycleKiller has data,
	// but daily buckets have zero CycleKiller (the production bug scenario).
	initial := StatsSnapshot{
		StartedAt:     "2026-08-29T10:00:00Z",
		TotalRequests: 100,
		CycleKiller: CycleKillerMetrics{
			TotalInterventions:   12,
			AvoidedRunawayTokens: 96000,
			AvoidedGPUSeconds:    2736,
			Stage1LocalHeals:     12,
		},
		DailyBuckets: map[string]TimeWindowMetrics{
			"2026-08-29": {Requests: 80, TokensTotal: 500000},
			"2026-08-30": {Requests: 20, TokensTotal: 100000},
		},
	}

	tracker := NewStatsTrackerWithInitialSnapshot(10, initial)
	defer tracker.Close()
	stats := tracker.GetStats()

	// After migration, the largest bucket (Aug 29, 80 requests) should have CycleKiller data
	b29 := stats.DailyBuckets["2026-08-29"]
	if b29.CycleKiller.TotalInterventions != 12 {
		t.Errorf("expected backfilled bucket 2026-08-29 TotalInterventions == 12, got %d",
			b29.CycleKiller.TotalInterventions)
	}
	if b29.CycleKiller.Stage1LocalHeals != 12 {
		t.Errorf("expected backfilled bucket Stage1LocalHeals == 12, got %d",
			b29.CycleKiller.Stage1LocalHeals)
	}

	// Smaller bucket should NOT have been touched
	b30 := stats.DailyBuckets["2026-08-30"]
	if b30.CycleKiller.TotalInterventions != 0 {
		t.Errorf("expected non-target bucket 2026-08-30 TotalInterventions == 0, got %d",
			b30.CycleKiller.TotalInterventions)
	}

	// Verify the migration is permanent: serialize, deserialize, reload into new tracker
	data, _ := json.MarshalIndent(stats, "", "  ")
	var restored StatsSnapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	tracker2 := NewStatsTrackerWithInitialSnapshot(10, restored)
	defer tracker2.Close()
	stats2 := tracker2.GetStats()

	// After second load, isLegacySnapshot should be false (bucket has data)
	// so the migration does NOT re-run, and bucket data stays correct
	b29r := stats2.DailyBuckets["2026-08-29"]
	if b29r.CycleKiller.TotalInterventions != 12 {
		t.Errorf("expected persisted bucket 2026-08-29 TotalInterventions == 12 after reload, got %d",
			b29r.CycleKiller.TotalInterventions)
	}
}
