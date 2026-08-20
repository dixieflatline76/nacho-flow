package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestStatsTracker_ObservationAggregation(t *testing.T) {
	tracker := NewStatsTracker(100)
	defer tracker.Close()

	// Tier 1 (Local Free) - 2 requests
	tracker.Record(Observation{
		Tier:       1,
		Tokens:     10000,
		CostSaved:  0.045, // $4.50/1M
		IsLocal:    true,
		IsFallback: false,
		LatencyMs:  120.5,
	})
	tracker.Record(Observation{
		Tier:       1,
		Tokens:     5000,
		CostSaved:  0.0225,
		IsLocal:    true,
		IsFallback: false,
		LatencyMs:  80.0,
	})

	// Tier 2 (Cloud Coder)
	tracker.Record(Observation{
		Tier:      2,
		Tokens:    2000,
		CostSaved: 0.0,
		IsLocal:   false,
	})

	// Tier 3 (Cloud Reasoning)
	tracker.Record(Observation{
		Tier:      3,
		Tokens:    4000,
		CostSaved: 0.0,
		IsLocal:   false,
	})

	// Tier 4 (Cloud Vision)
	tracker.Record(Observation{
		Tier:      4,
		Tokens:    1500,
		CostSaved: 0.0,
		IsLocal:   false,
	})

	// Explicit override
	tracker.Record(Observation{
		Tier:               99,
		IsExplicitOverride: true,
		Tokens:             500,
	})

	// Fallback
	tracker.Record(Observation{
		Tier:       2,
		IsFallback: true,
		Tokens:     3000,
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
	if stats.EstimatedCostSavedUSD < 0.0674 || stats.EstimatedCostSavedUSD > 0.0676 {
		t.Errorf("expected ~0.0675 cost saved, got %f", stats.EstimatedCostSavedUSD)
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
}

func TestStatsTracker_HTTPHandler(t *testing.T) {
	tracker := NewStatsTracker(10)
	defer tracker.Close()

	tracker.Record(Observation{
		Tier:      1,
		Tokens:    1000,
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
