package tuner

import (
	"runtime"
	"testing"
	"time"
)

// TestStress_1MillionTurns_ZeroAlloc runs a complete 1,000,000 turn replay
// and verifies execution correctness and zero heap allocations during the replay loop.
func TestStress_1MillionTurns_ZeroAlloc(t *testing.T) {
	const totalTurns = 1_000_000
	t.Logf("Synthesizing %d realistic telemetry turns...", totalTurns)
	startGen := time.Now()
	trajectories := GenerateSyntheticTrajectories(totalTurns, 42)
	genDuration := time.Since(startGen)
	t.Logf("Generated %d sessions (%d turns) in %v", len(trajectories), totalTurns, genDuration)

	cfg := DefaultBenchConfig()
	policy := DefaultTuningPolicy()
	var res MultiTierReplayResult

	// Pre-warm GC
	runtime.GC()
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	startReplay := time.Now()
	ReplayMultiTierFleet(trajectories, &cfg, &policy, &res)
	replayDuration := time.Since(startReplay)

	runtime.ReadMemStats(&memAfter)

	if res.TotalTurns != totalTurns {
		t.Fatalf("Expected %d total turns, got %d", totalTurns, res.TotalTurns)
	}

	throughput := float64(totalTurns) / replayDuration.Seconds()
	nsPerTurn := float64(replayDuration.Nanoseconds()) / float64(totalTurns)

	t.Logf("=================================================================")
	t.Logf("🚀 1,000,000 STATEMENT ENGINE REPLAY BENCHMARK RESULT")
	t.Logf("=================================================================")
	t.Logf("Total Turns Processed: %d", res.TotalTurns)
	t.Logf("Total Sessions:        %d", len(trajectories))
	t.Logf("Replay Duration:       %v", replayDuration)
	t.Logf("Replay Throughput:     %.2f Million turns/sec", throughput/1_000_000.0)
	t.Logf("Latency per Turn:      %.2f ns/turn", nsPerTurn)
	t.Logf("Heap Allocs During:    %d bytes (%d mallocs)", memAfter.TotalAlloc-memBefore.TotalAlloc, memAfter.Mallocs-memBefore.Mallocs)
	t.Logf("-----------------------------------------------------------------")
	for i := 0; i < res.NumTiers; i++ {
		ts := &res.TierStats[i]
		t.Logf("  Tier %d [%-36s]: %7d turns | Cost: $%8.2f | Retries: %6d",
			i, ts.TierName, ts.TurnsRouted, ts.CostUSD, ts.WastedRetries)
	}
	t.Logf("=================================================================")

	// Verify zero allocations on replay
	if memAfter.Mallocs-memBefore.Mallocs > 0 {
		t.Errorf("Expected 0 mallocs during fleet replay, got %d", memAfter.Mallocs-memBefore.Mallocs)
	}
}

// BenchmarkReplayFleet_500K_Statements benchmarks raw cascade routing across 500,000 turns.
func BenchmarkReplayFleet_500K_Statements(b *testing.B) {
	trajectories := GenerateSyntheticTrajectories(500_000, 1337)
	cfg := DefaultBenchConfig()
	policy := DefaultTuningPolicy()
	var res MultiTierReplayResult

	b.ReportAllocs()
	b.SetBytes(500_000)

	for b.Loop() {
		res.Reset()
		ReplayMultiTierFleet(trajectories, &cfg, &policy, &res)
	}
}

// BenchmarkReplayFleet_1M_Statements benchmarks raw cascade routing across 1,000,000 turns.
func BenchmarkReplayFleet_1M_Statements(b *testing.B) {
	trajectories := GenerateSyntheticTrajectories(1_000_000, 2026)
	cfg := DefaultBenchConfig()
	policy := DefaultTuningPolicy()
	var res MultiTierReplayResult

	b.ReportAllocs()
	b.SetBytes(1_000_000)

	for b.Loop() {
		res.Reset()
		ReplayMultiTierFleet(trajectories, &cfg, &policy, &res)
	}
}

// BenchmarkEvaluateConflict_1M_Statements benchmarks full scalar conflict calculation on 1M turns.
func BenchmarkEvaluateConflict_1M_Statements(b *testing.B) {
	trajectories := GenerateSyntheticTrajectories(1_000_000, 777)
	cfg := DefaultBenchConfig()
	policy := DefaultTuningPolicy()
	var res MultiTierReplayResult

	b.ReportAllocs()
	b.SetBytes(1_000_000)

	for b.Loop() {
		EvaluateFleetConflict(trajectories, &cfg, &policy, &res)
	}
}
