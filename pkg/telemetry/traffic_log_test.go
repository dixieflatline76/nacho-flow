package telemetry

import (
	"os"
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

// Test 1.3: ReadRecords non-existent file, limit capping, and closed logger handling
func TestTrafficLogger_ReadRecordsAndClosedLogger(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "capped_traffic.jsonl")

	logger, err := NewTrafficLogger(logPath, 50)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	for i := 0; i < 10; i++ {
		logger.Emit(TurnRecord{Tokens: 100 * (i + 1)})
	}
	_ = logger.Close()

	// Double close
	if err := logger.Close(); err != nil {
		t.Errorf("Expected double close to return nil error, got: %v", err)
	}

	// Emit on closed logger
	logger.Emit(TurnRecord{Tokens: 9999})

	// Read with limit 3
	records, err := ReadRecords(logPath, 3)
	if err != nil {
		t.Fatalf("Failed to read records with limit: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("Expected 3 records capped, got %d", len(records))
	}

	// Read non-existent file
	missingRecords, err := ReadRecords(filepath.Join(tempDir, "missing.jsonl"), 10)
	if err != nil {
		t.Fatalf("Expected nil error for missing file, got %v", err)
	}
	if len(missingRecords) != 0 {
		t.Errorf("Expected 0 records for missing file, got %d", len(missingRecords))
	}
}

// Test 1.4: Constructor defaults and error cases
func TestTrafficLogger_ConstructorAndReadRecordEdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	// Default path and buffer size
	logger, err := NewTrafficLogger(filepath.Join(tempDir, "default_traffic.jsonl"), 0)
	if err != nil {
		t.Fatalf("Failed to create logger with default buffer size: %v", err)
	}
	_ = logger.Close()

	// MkdirAll error: parent is a file
	parentFile := filepath.Join(tempDir, "parent_file")
	_ = os.WriteFile(parentFile, []byte("data"), 0600)
	_, err = NewTrafficLogger(filepath.Join(parentFile, "sub", "log.jsonl"), 10)
	if err == nil {
		t.Errorf("Expected error creating logger under regular file parent")
	}

	// ReadRecords error when reading a directory
	_, err = ReadRecords(tempDir, 10)
	if err == nil {
		t.Errorf("Expected error calling ReadRecords on a directory")
	}

	// ReadRecords with blank lines and corrupt lines
	corruptLogPath := filepath.Join(tempDir, "corrupt_lines.jsonl")
	corruptContent := "\n\n{\"tokens\": 500}\nnot_valid_json\n{\"tokens\": 600}\n\n"
	if err := os.WriteFile(corruptLogPath, []byte(corruptContent), 0600); err != nil {
		t.Fatalf("Failed to write corrupt log: %v", err)
	}

	records, err := ReadRecords(corruptLogPath, 0)
	if err != nil {
		t.Fatalf("Unexpected error reading corrupt lines: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("Expected 2 valid records parsed, got %d", len(records))
	}
}

// Test 1.5: Cycle Breaker metrics persistence and deserialization
func TestTrafficLogger_CycleBreakerMetrics_Roundtrip(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "cycle_traffic.jsonl")

	logger, err := NewTrafficLogger(logPath, 100)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	record := TurnRecord{
		Timestamp:                 time.Now().UTC(),
		RequestID:                 "req-cycle-telemetry",
		Tokens:                    4096,
		CycleBreakerTriggered:     false,
		CycleProseTokens:          2847,
		CycleMaxNgramFreq:         1,
		CycleThinkingTokens:       1203,
		CycleMaxThinkingNgramFreq: 1,
		CycleToolTokens:           1520,
		CycleMaxToolNgramFreq:     2,
		HasShellWrite:             true,
	}

	if logger.FilePath() != logPath {
		t.Errorf("Expected FilePath %s, got %s", logPath, logger.FilePath())
	}

	logger.Emit(record)
	if err := logger.Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	records, err := ReadRecords(logPath, 0)
	if err != nil {
		t.Fatalf("Failed to read records: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	got := records[0]
	if got.CycleProseTokens != 2847 {
		t.Errorf("Expected CycleProseTokens 2847, got %d", got.CycleProseTokens)
	}
	if got.CycleMaxNgramFreq != 1 {
		t.Errorf("Expected CycleMaxNgramFreq 1, got %d", got.CycleMaxNgramFreq)
	}
	if got.CycleThinkingTokens != 1203 {
		t.Errorf("Expected CycleThinkingTokens 1203, got %d", got.CycleThinkingTokens)
	}
	if got.CycleMaxThinkingNgramFreq != 1 {
		t.Errorf("Expected CycleMaxThinkingNgramFreq 1, got %d", got.CycleMaxThinkingNgramFreq)
	}
	if got.CycleToolTokens != 1520 {
		t.Errorf("Expected CycleToolTokens 1520, got %d", got.CycleToolTokens)
	}
	if got.CycleMaxToolNgramFreq != 2 {
		t.Errorf("Expected CycleMaxToolNgramFreq 2, got %d", got.CycleMaxToolNgramFreq)
	}
	if !got.HasShellWrite {
		t.Errorf("Expected HasShellWrite true, got false")
	}
}

// Test 1.6: Concurrent Close and Emit to verify zero panics and lock-free thread safety
func TestTrafficLogger_ConcurrentCloseAndEmit(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "concurrent_close.jsonl")

	logger, err := NewTrafficLogger(logPath, 500)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	var wg sync.WaitGroup
	emitters := 30
	recordsPerEmitter := 200

	for i := 0; i < emitters; i++ {
		wg.Add(1)
		go func(emitterID int) {
			defer wg.Done()
			for j := 0; j < recordsPerEmitter; j++ {
				logger.Emit(TurnRecord{
					Timestamp: time.Now().UTC(),
					RequestID: "req-concurrent",
					Tokens:    emitterID*1000 + j,
				})
				if j == recordsPerEmitter/2 && emitterID == 0 {
					// Concurrently trigger close from one goroutine
					go func() {
						_ = logger.Close()
					}()
				}
			}
		}(i)
	}

	wg.Wait()
	// Ensure Close completes safely even if double called
	if err := logger.Close(); err != nil {
		t.Errorf("Unexpected error on double close: %v", err)
	}
}

// Test 1.7: Queue saturation non-blocking drop under extreme overload
func TestTrafficLogger_QueueFullDrop(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "drop_traffic.jsonl")

	// Create logger with tiny buffer
	logger, err := NewTrafficLogger(logPath, 1)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Flood emits faster than worker can process without blocking
	for i := 0; i < 50; i++ {
		logger.Emit(TurnRecord{Tokens: i})
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}
}

func BenchmarkTrafficLogger_Emit(b *testing.B) {
	tempDir := b.TempDir()
	logPath := filepath.Join(tempDir, "bench_traffic.jsonl")

	logger, err := NewTrafficLogger(logPath, 50000)
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer func() { _ = logger.Close() }()

	rec := TurnRecord{
		Timestamp:   time.Now().UTC(),
		RequestID:   "bench-req",
		Tokens:      1500,
		TargetModel: "qwen2.5-coder",
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Emit(rec)
		}
	})
}
