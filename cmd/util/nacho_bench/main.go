package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/server"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

type StepResult struct {
	Concurrency int
	TotalReqs   int
	Completed   int64
	Failed      int64
	Duration    time.Duration
	RPS         float64
	P50         time.Duration
	P95         time.Duration
	P99         time.Duration
	Max         time.Duration
	HeapAllocMB float64
}

func runBenchStep(client *http.Client, ts *httptest.Server, tracker *telemetry.StatsTracker, totalRequests, concurrency int) StepResult {
	requestsPerWorker := totalRequests / concurrency
	payload := []byte(`{"model": "nacho-hybrid", "messages": [{"role": "user", "content": "Analyze high-throughput routing performance under load."}]}`)

	var completedReqs int64
	var failedReqs int64
	latencies := make([]time.Duration, 0, totalRequests)
	var latenciesMu sync.Mutex

	startOverall := time.Now()
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localLatencies := make([]time.Duration, 0, requestsPerWorker)

			for i := 0; i < requestsPerWorker; i++ {
				reqStart := time.Now()
				req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader(payload))
				if err != nil {
					atomic.AddInt64(&failedReqs, 1)
					continue
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				duration := time.Since(reqStart)

				if err != nil || resp.StatusCode != http.StatusOK {
					atomic.AddInt64(&failedReqs, 1)
				} else {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					atomic.AddInt64(&completedReqs, 1)
					localLatencies = append(localLatencies, duration)
				}
			}

			latenciesMu.Lock()
			latencies = append(latencies, localLatencies...)
			latenciesMu.Unlock()
		}()
	}

	wg.Wait()
	durationOverall := time.Since(startOverall)
	tracker.Flush()

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	var p50, p95, p99, maxLat time.Duration
	if len(latencies) > 0 {
		p50 = latencies[int(float64(len(latencies))*0.50)]
		p95 = latencies[int(float64(len(latencies))*0.95)]
		p99 = latencies[int(float64(len(latencies))*0.99)]
		maxLat = latencies[len(latencies)-1]
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	rps := float64(completedReqs) / durationOverall.Seconds()

	return StepResult{
		Concurrency: concurrency,
		TotalReqs:   totalRequests,
		Completed:   completedReqs,
		Failed:      failedReqs,
		Duration:    durationOverall,
		RPS:         rps,
		P50:         p50,
		P95:         p95,
		P99:         p99,
		Max:         maxLat,
		HeapAllocMB: float64(m.HeapAlloc) / 1024 / 1024,
	}
}

func main() {
	flag.Parse()

	fmt.Printf("========================================================================================\n")
	fmt.Printf("🌮 NACHO FLOW STRESS TEST & BREAKING POINT ANALYSIS\n")
	fmt.Printf("========================================================================================\n")
	fmt.Printf("CPUs Available: %d | OS: %s | Arch: %s\n", runtime.NumCPU(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Stress Plan:    Scaling concurrency: 50 -> 100 -> 250 -> 500 -> 1,000 parallel workers\n")
	fmt.Printf("========================================================================================\n\n")

	// Mock upstream
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"bench-cmpl","choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer mockUpstream.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"mock_local": {
				BaseURL: mockUpstream.URL,
				Type:    "local",
			},
			"mock_cloud": {
				BaseURL: mockUpstream.URL,
				Type:    "cloud",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Local ROCm GPU",
				Model:    "qwen2.5-coder:14b",
				Provider: "mock_local",
				When:     "Tokens < 16000 && !HasImages && !HasTools",
			},
			{
				Name:     "Cloud Agentic Fast",
				Model:    "qwen3-coder",
				Provider: "mock_cloud",
				When:     "Tokens >= 16000 || HasTools",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Cloud Fallback",
			Model:    "fallback-model",
			Provider: "mock_cloud",
		},
	}

	evaluator, err := strategy.NewExprEvaluator(cfg.Tiers, cfg.DefaultTier)
	if err != nil {
		panic(err)
	}

	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	oracle := telemetry.NewPricingOracle()
	tracker := telemetry.NewStatsTracker(500000)
	defer tracker.Close()

	nullLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := server.NewServerWithTelemetry(cfg, evaluator, classifier, sanitizer, oracle, tracker, nullLogger)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	// High concurrency benchmark client
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 60 * time.Second,
			}).DialContext,
			MaxIdleConns:        10000,
			MaxIdleConnsPerHost: 2000,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 10 * time.Second,
	}

	steps := []struct {
		concurrency int
		requests    int
	}{
		{concurrency: 50, requests: 25000},
		{concurrency: 100, requests: 50000},
		{concurrency: 250, requests: 75000},
		{concurrency: 500, requests: 100000},
		{concurrency: 1000, requests: 100000},
	}

	results := make([]StepResult, 0, len(steps))

	for i, step := range steps {
		fmt.Printf("▶ [STAGE %d/5] Running %d requests across %d concurrent workers...\n", i+1, step.requests, step.concurrency)
		res := runBenchStep(client, ts, tracker, step.requests, step.concurrency)
		results = append(results, res)
		fmt.Printf("   ✓ Done in %.2fs | RPS: %.1f | P50: %.2fms | P99: %.2fms | Heap: %.1f MB | Success: %d/%d (Fail: %d)\n\n",
			res.Duration.Seconds(), res.RPS, float64(res.P50.Microseconds())/1000.0, float64(res.P99.Microseconds())/1000.0, res.HeapAllocMB, res.Completed, res.TotalReqs, res.Failed)
	}

	stats := tracker.GetStats()

	fmt.Printf("========================================================================================\n")
	fmt.Printf("🏁 FINAL STRESS TEST REPORT SUMMARY\n")
	fmt.Printf("========================================================================================\n")
	fmt.Printf("%-12s | %-12s | %-12s | %-12s | %-10s | %-10s | %-10s\n", "Concurrency", "Total Reqs", "Success Rate", "Throughput", "P50 Lat", "P99 Lat", "Heap MB")
	fmt.Printf("----------------------------------------------------------------------------------------\n")
	for _, r := range results {
		successRate := float64(r.Completed) / float64(r.TotalReqs) * 100.0
		fmt.Printf("%-12d | %-12d | %-11.1f%% | %-9.1f r/s | %-7.2f ms | %-7.2f ms | %-7.1f MB\n",
			r.Concurrency, r.TotalReqs, successRate, r.RPS, float64(r.P50.Microseconds())/1000.0, float64(r.P99.Microseconds())/1000.0, r.HeapAllocMB)
	}
	fmt.Printf("----------------------------------------------------------------------------------------\n")
	fmt.Printf("Total Requests Tracked by Telemetry: %d\n", stats.TotalRequests)
	fmt.Printf("Total Local Tokens Tracked:          %d\n", stats.TotalTokensRoutedLocally)
	fmt.Printf("Total USD Savings Calculated:        $%.4f USD\n", stats.EstimatedCostSavedUSD)
	fmt.Printf("========================================================================================\n")
}
