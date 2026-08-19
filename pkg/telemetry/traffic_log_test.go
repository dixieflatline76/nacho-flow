package telemetry

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Test 1.1: Non-blocking high throughput logging and roundtrip read
func TestTrafficLogger_HighThroughput_Roundtrip(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "traffic.jsonl")

	logger, err := NewTrafficLogger(logPath, 5000)
	if err != nil {
		t.Fatalf("Failed to create TrafficLogger: %v", err)
	}

	var wg sync.WaitGroup
	recordCount := 1000

	for i := 0; i < recordCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			logger.Emit(TurnRecord{
				Timestamp:    time.Now().UTC(),
				RequestID:    "req-test",
				Tokens:       1000 + idx,
				Keywords:     []string{"sql", "refactor"},
				SelectedTier: "Local GPU",
				TargetModel:  "qwen2.5-coder",
				IsLocal:      true,
				IsRetry:      idx%5 == 0,
			})
		}(i)
	}
	wg.Wait()

	if err := logger.Close(); err != nil {
		t.Fatalf("Failed to close TrafficLogger: %v", err)
	}

	// Read records back
	records, err := ReadRecords(logPath, 0)
	if err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}

	if len(records) != recordCount {
		t.Errorf("Expected %d records, got %d", recordCount, len(records))
	}
}

// Test 1.2: StatsTracker Fan-Out to multiple ObservationSinks
func TestStatsTracker_FanOutToMultipleSinks(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "fanout_traffic.jsonl")

	trafficLog, err := NewTrafficLogger(logPath, 100)
	if err != nil {
		t.Fatalf("Failed to create TrafficLogger: %v", err)
	}

	tracker := NewStatsTracker(100)
	tracker.AddSink(trafficLog)

	// Emit observation to StatsTracker
	tracker.Record(Observation{
		Tier:       1,
		TierName:   "Local ROCm Tier",
		Model:      "qwen2.5-coder:14b",
		Provider:   "ollama",
		Tokens:     4500,
		CostSaved:  0.02025,
		IsLocal:    true,
		Keywords:   []string{"concurrency", "mutex"},
		StatusCode: 200,
	})

	tracker.Flush()
	_ = trafficLog.Close()
	tracker.Close()

	// Verify both in-memory stats snapshot and disk sink received event
	stats := tracker.GetStats()
	if stats.TotalRequests != 1 {
		t.Errorf("Expected TotalRequests 1, got %d", stats.TotalRequests)
	}

	records, err := ReadRecords(logPath, 0)
	if err != nil {
		t.Fatalf("Failed to read sink records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Expected 1 sink record, got %d", len(records))
	}
	if records[0].SelectedTier != "Local ROCm Tier" {
		t.Errorf("Expected SelectedTier 'Local ROCm Tier', got '%s'", records[0].SelectedTier)
	}
	if len(records[0].Keywords) != 2 || records[0].Keywords[0] != "concurrency" {
		t.Errorf("Expected keywords [concurrency, mutex], got %v", records[0].Keywords)
	}
}
