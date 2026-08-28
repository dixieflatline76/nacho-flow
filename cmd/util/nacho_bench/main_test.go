package main

import (
	"strings"
	"testing"
	"time"
)

func TestBench_ReplaceTagContent(t *testing.T) {
	doc := `Header
<!-- BENCHMARK:EXECUTIVE_SUMMARY_START -->
Old text
<!-- BENCHMARK:EXECUTIVE_SUMMARY_END -->
Footer`

	newSummary := "- **Peak Throughput**: **30,771 requests/second**"
	updated := replaceTagContent(doc, "<!-- BENCHMARK:EXECUTIVE_SUMMARY_START -->", "<!-- BENCHMARK:EXECUTIVE_SUMMARY_END -->", newSummary)

	expected := `Header
<!-- BENCHMARK:EXECUTIVE_SUMMARY_START -->
- **Peak Throughput**: **30,771 requests/second**
<!-- BENCHMARK:EXECUTIVE_SUMMARY_END -->
Footer`

	if updated != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, updated)
	}
}

func TestBench_RenderExecutiveSummary(t *testing.T) {
	summary := renderExecutiveSummary(31073.2, 0.188, 0.221, 350000, 1000)
	if !strings.Contains(summary, "31,073 requests/second") || !strings.Contains(summary, "1,000 parallel workers") || !strings.Contains(summary, "350,000 total requests") {
		t.Errorf("unexpected executive summary: %s", summary)
	}
}

func TestBench_RenderStressTable(t *testing.T) {
	results := []StepResult{
		{
			Concurrency: 50,
			TotalReqs:   25000,
			Completed:   25000,
			Duration:    time.Second,
			RPS:         25000.0,
			P50:         time.Millisecond,
			P99:         10 * time.Millisecond,
			HeapAllocMB: 52.3,
		},
	}

	table := renderStressTable(results)
	if !strings.Contains(table, "| **50 workers** | 25,000 | **100.0%** | **25000.0 req/s** | 1.00 ms | 10.00 ms | 52.3 MB |") {
		t.Errorf("unexpected stress table: %s", table)
	}
}

func TestBench_RenderABTable(t *testing.T) {
	raw := []StepResult{
		{Concurrency: 50, RPS: 30000.0, P50: time.Millisecond, P99: 5 * time.Millisecond},
	}
	heavy := []StepResult{
		{Concurrency: 50, RPS: 29000.0, P50: 1200 * time.Microsecond, P99: 5500 * time.Microsecond},
	}

	table := renderABTable(raw, heavy)
	if !strings.Contains(table, "| **50 workers** | 30000.0 req/s | 29000.0 req/s | **-3.3%** |") {
		t.Errorf("unexpected AB table: %s", table)
	}
}

func TestBench_RenderHeroStats(t *testing.T) {
	stats := renderHeroStats(30771.3)
	if !strings.Contains(stats, "30,771+") || !strings.Contains(stats, "Requests / Sec") {
		t.Errorf("unexpected hero stats: %s", stats)
	}
}
