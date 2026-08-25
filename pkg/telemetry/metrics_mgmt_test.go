package telemetry

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestStatsTracker_Reset(t *testing.T) {
	tracker := NewStatsTracker(100)
	obsChan := tracker.obsChan

	obsChan <- Observation{
		Tier:      1,
		TierName:  "tier_1",
		Model:     "qwen2.5-coder:14b",
		Provider:  "ollama",
		Tokens:    500,
		CostSpent: 0.0,
		CostSaved: 0.0015,
		IsLocal:   true,
	}
	obsChan <- Observation{
		Tier:      2,
		TierName:  "tier_2",
		Model:     "google/gemini-3.7-flash",
		Provider:  "openrouter",
		Tokens:    2000,
		CostSpent: 0.0015,
		CostSaved: 0.0045,
		IsLocal:   false,
	}

	tracker.Flush()
	snap := tracker.GetStats()
	if snap.TotalRequests != 2 {
		t.Fatalf("expected 2 requests before reset, got %d", snap.TotalRequests)
	}

	tracker.Reset()

	cleanSnap := tracker.GetStats()
	if cleanSnap.TotalRequests != 0 {
		t.Errorf("expected 0 total requests after reset, got %d", cleanSnap.TotalRequests)
	}
	if cleanSnap.TotalCostSpentUSD != 0.0 {
		t.Errorf("expected 0.0 total cost spent, got %f", cleanSnap.TotalCostSpentUSD)
	}
	if cleanSnap.EstimatedCostSavedUSD != 0.0 {
		t.Errorf("expected 0.0 total cost saved, got %f", cleanSnap.EstimatedCostSavedUSD)
	}
	if len(cleanSnap.DailyBuckets) != 0 {
		t.Errorf("expected empty daily buckets, got %v", cleanSnap.DailyBuckets)
	}
}

func TestStatsTracker_RecalculateFromRecords(t *testing.T) {
	tracker := NewStatsTracker(100)

	oracle := NewPricingOracle()
	oracle.RegisterProvider(&mockPricingProvider{
		name: "openrouter",
		prices: map[string]ModelMetadata{
			"openrouter/google/gemini-3.7-flash": {
				ModelID: "google/gemini-3.7-flash",
				ModelPricing: ModelPricing{
					PromptCostPerMillion:     0.5,
					CompletionCostPerMillion: 2.0,
				},
			},
		},
	}, 0)
	_ = oracle.Sync(context.Background())

	records := []TurnRecord{
		{
			Timestamp:    time.Now().UTC().Add(-10 * time.Minute),
			Tokens:       1000,
			SelectedTier: "tier_1_local",
			TargetModel:  "gemma4:12b-it-qat",
			Provider:     "ollama",
			IsLocal:      true,
			StatusCode:   200,
		},
		{
			Timestamp:    time.Now().UTC().Add(-5 * time.Minute),
			Tokens:       2500,
			SelectedTier: "Tier 2: Cloud Workhorse",
			TargetModel:  "google/gemini-3.7-flash",
			Provider:     "openrouter",
			IsLocal:      false,
			StatusCode:   200,
		},
		{
			Timestamp:    time.Now().UTC().Add(-2 * time.Minute),
			Tokens:       3000,
			SelectedTier: "Tier 3: Cloud Reasoning",
			TargetModel:  "anthropic/claude-3.7-sonnet:thinking",
			Provider:     "openrouter",
			IsLocal:      false,
			IsFallback:   true,
			StatusCode:   200,
		},
		{
			Timestamp:    time.Now().UTC().Add(-1 * time.Minute),
			Tokens:       1500,
			SelectedTier: "Tier 4: Vision Specialist",
			TargetModel:  "google/gemini-flash-vision",
			Provider:     "openrouter",
			IsLocal:      false,
			StatusCode:   200,
		},
	}

	tracker.RecalculateFromRecords(records, oracle, 3.00)

	snap := tracker.GetStats()
	if snap.TotalRequests != 4 {
		t.Errorf("expected 4 total requests, got %d", snap.TotalRequests)
	}
	if snap.TotalTokensRoutedLocally != 1000 {
		t.Errorf("expected 1000 local tokens, got %d", snap.TotalTokensRoutedLocally)
	}
	if snap.TierBreakdown.Tier1LocalFree != 1 {
		t.Errorf("expected 1 Tier1 request, got %d", snap.TierBreakdown.Tier1LocalFree)
	}
	if snap.TierBreakdown.Tier2CloudCoder != 1 {
		t.Errorf("expected 1 Tier2 request, got %d", snap.TierBreakdown.Tier2CloudCoder)
	}
	if snap.TierBreakdown.Tier3CloudReasoning != 1 {
		t.Errorf("expected 1 Tier3 request, got %d", snap.TierBreakdown.Tier3CloudReasoning)
	}
	if snap.TierBreakdown.Tier4CloudVision != 1 {
		t.Errorf("expected 1 Tier4 request, got %d", snap.TierBreakdown.Tier4CloudVision)
	}
	if snap.TierBreakdown.Fallbacks != 1 {
		t.Errorf("expected 1 Fallback request, got %d", snap.TierBreakdown.Fallbacks)
	}

	// Verify helpers
	rb := NewRingBufferSink(10)
	rb.Emit(TurnRecord{Tokens: 100})
	if len(rb.GetRecent(10)) != 1 {
		t.Errorf("expected 1 record in ring buffer")
	}
	rb.Reset()
	if len(rb.GetRecent(10)) != 0 {
		t.Errorf("expected 0 records after Reset")
	}

	_ = oracle.LastSynced()
}

func TestStatsTracker_ConcurrentResetAndRecord(t *testing.T) {
	tracker := NewStatsTracker(1000)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				tracker.Record(Observation{
					Tier:      1,
					Tokens:    100,
					CostSpent: 0.0,
					CostSaved: 0.0003,
					IsLocal:   true,
				})
				if idx%3 == 0 && j == 25 {
					tracker.Reset()
				}
				_ = tracker.GetStats()
			}
		}(i)
	}

	wg.Wait()
}
