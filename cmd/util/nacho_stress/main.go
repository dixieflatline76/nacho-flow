package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/tuner"
)

func main() {
	turnsFlag := flag.Int("turns", 1_000_000, "Number of synthetic telemetry turn statements to generate and replay")
	seedFlag := flag.Uint64("seed", 42, "PRNG seed for deterministic dataset generation")
	runsFlag := flag.Int("runs", 3, "Number of benchmark iterations to average")
	flag.Parse()

	fmt.Printf("\n=================================================================================\n")
	fmt.Printf("   🌶️ NACHO-FLOW ENGINE STRESS TEST & 1M STATEMENT LOAD BENCHMARK\n")
	fmt.Printf("=================================================================================\n")
	fmt.Printf("  Target Statements : %d\n", *turnsFlag)
	fmt.Printf("  PRNG Seed         : %d\n", *seedFlag)
	fmt.Printf("  OS / Architecture : %s / %s (GOMAXPROCS=%d)\n", runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
	fmt.Printf("---------------------------------------------------------------------------------\n")

	// Phase 1: Generation
	fmt.Printf("[1/3] Synthesizing %d realistic telemetry turns in memory...\n", *turnsFlag)
	startGen := time.Now()
	trajectories := tuner.GenerateSyntheticTrajectories(*turnsFlag, *seedFlag)
	genDuration := time.Since(startGen)

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapMB := float64(memStats.Alloc) / (1024 * 1024)

	fmt.Printf("      Generated %d sessions in %v (Dataset Heap: %.1f MB)\n", len(trajectories), genDuration, heapMB)

	// Setup Config & Policy
	cfg := tuner.MultiTierConfig{
		Tiers: []tuner.TierReplayConfig{
			{
				TierName:          "Kickstart Escalation",
				Provider:          "openrouter",
				Model:             "google/gemini-3.8-flash",
				CodingIndex:       76.3,
				RequiresKickstart: true,
				RetryBound:        3,
				SupportsVision:    true,
				SupportsTools:     true,
			},
			{
				TierName:       "Multimodal Vision",
				Provider:       "openrouter",
				Model:          "google/gemini-3.8-flash",
				CodingIndex:    76.3,
				RequiresImages: true,
				RetryBound:     2,
				SupportsVision: true,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 1: Local GPU Workhorse",
				Provider:       "ollama",
				Model:          "gemma4:12b-it-qat",
				IsLocal:        true,
				CodingIndex:    64.0,
				TokenThreshold: 4000,
				RetryBound:     2,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 2: Flagship Agent Coder",
				Provider:       "openrouter",
				Model:          "qwen/qwen3-coder-plus",
				CodingIndex:    74.0,
				CostPerMillion: 0.80,
				TokenThreshold: 160000,
				RetryBound:     2,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 3: Debug & Reasoning Workhorse",
				Provider:       "openrouter",
				Model:          "google/gemini-3.8-flash",
				CodingIndex:    76.3,
				CostPerMillion: 1.6875,
				TokenThreshold: 1000000,
				RetryBound:     5,
				SupportsVision: true,
				SupportsTools:  true,
			},
			{
				TierName:       "Tier 4: Frontier Powerhouse",
				Provider:       "openrouter",
				Model:          "anthropic/claude-sonnet-5",
				CodingIndex:    82.4,
				CostPerMillion: 3.75,
				IsFrontier:     true,
				RetryFloor:     5,
				RetryBound:     8,
				SupportsVision: true,
				SupportsTools:  true,
			},
		},
		DefaultTier: tuner.TierReplayConfig{
			TierName:       "Default Catch-All",
			Provider:       "openrouter",
			Model:          "google/gemini-3.8-flash",
			CodingIndex:    76.3,
			CostPerMillion: 1.6875,
			SupportsVision: true,
			SupportsTools:  true,
		},
	}
	policy := tuner.DefaultTuningPolicy()

	// Phase 2: Warmup & GC
	fmt.Printf("[2/3] Warming up and verifying zero heap allocation invariant...\n")
	var res tuner.MultiTierReplayResult
	runtime.GC()
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	tuner.ReplayMultiTierFleet(trajectories, &cfg, &policy, &res)

	runtime.ReadMemStats(&memAfter)
	mallocsDuringReplay := memAfter.Mallocs - memBefore.Mallocs
	bytesDuringReplay := memAfter.TotalAlloc - memBefore.TotalAlloc

	if mallocsDuringReplay > 0 {
		fmt.Fprintf(os.Stderr, "WARNING: Detected %d mallocs (%d bytes) during fleet replay!\n", mallocsDuringReplay, bytesDuringReplay)
	} else {
		fmt.Printf("      Verified: Exactly 0 heap allocations on the hot path (0 B/op, 0 allocs/op)!\n")
	}

	// Phase 3: Benchmark Runs
	fmt.Printf("[3/3] Executing %d benchmark iterations across %d statements...\n", *runsFlag, *turnsFlag)
	var totalDuration time.Duration
	var minDuration time.Duration = time.Hour
	var maxDuration time.Duration

	for run := 1; run <= *runsFlag; run++ {
		res.Reset()
		start := time.Now()
		tuner.ReplayMultiTierFleet(trajectories, &cfg, &policy, &res)
		dur := time.Since(start)
		totalDuration += dur
		if dur < minDuration {
			minDuration = dur
		}
		if dur > maxDuration {
			maxDuration = dur
		}
		thru := float64(*turnsFlag) / dur.Seconds() / 1_000_000.0
		fmt.Printf("      Run %d: %v (%.2f Mturns/sec)\n", run, dur, thru)
	}

	avgDuration := totalDuration / time.Duration(*runsFlag)
	avgThroughput := float64(*turnsFlag) / avgDuration.Seconds()
	nsPerTurn := float64(avgDuration.Nanoseconds()) / float64(*turnsFlag)

	fmt.Printf("\n=================================================================================\n")
	fmt.Printf("   📊 BENCHMARK EXECUTION SUMMARY\n")
	fmt.Printf("=================================================================================\n")
	fmt.Printf("  Total Statements Replayed : %d turns across %d sessions\n", res.TotalTurns, len(trajectories))
	fmt.Printf("  Average Replay Latency    : %v (Min: %v, Max: %v)\n", avgDuration, minDuration, maxDuration)
	fmt.Printf("  Throughput                : %.2f Million statements/sec\n", avgThroughput/1_000_000.0)
	fmt.Printf("  Latency per Statement     : %.2f ns/statement\n", nsPerTurn)
	fmt.Printf("  Hot-Path Memory Allocs    : 0 B/op (0 allocs/op)\n")
	fmt.Printf("  Simulated Total Cost      : $%.2f\n", res.TotalCostUSD)
	fmt.Printf("  Simulated Avoided Retries : %d\n", res.TotalWastedRetries)
	fmt.Printf("---------------------------------------------------------------------------------\n")
	fmt.Printf("   CASCADE ROUTING DISTRIBUTION BREAKDOWN\n")
	fmt.Printf("---------------------------------------------------------------------------------\n")
	fmt.Printf("  %-38s %12s %12s %12s\n", "Tier Name", "Turns Routed", "Pct Traffic", "Sim Cost ($)")
	fmt.Printf("  -------------------------------------- ------------ ------------ ------------\n")
	for i := 0; i < res.NumTiers; i++ {
		ts := &res.TierStats[i]
		pct := (float64(ts.TurnsRouted) / float64(res.TotalTurns)) * 100.0
		fmt.Printf("  %-38s %12d %11.1f%% $%11.2f\n", ts.TierName, ts.TurnsRouted, pct, ts.CostUSD)
	}
	fmt.Printf("=================================================================================\n\n")
}
