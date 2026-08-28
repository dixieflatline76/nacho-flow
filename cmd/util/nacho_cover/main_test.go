package main

import (
	"strings"
	"testing"
)

func TestReplaceTagContent(t *testing.T) {
	doc := `Header
<!-- COVERAGE:GO_TABLE_START -->
Old Table Content
<!-- COVERAGE:GO_TABLE_END -->
Footer`

	newContent := "| Package | Coverage |\n| :--- | :--- |\n| pkg/config | 100.0% |"
	updated := replaceTagContent(doc, "<!-- COVERAGE:GO_TABLE_START -->", "<!-- COVERAGE:GO_TABLE_END -->", newContent)

	expected := `Header
<!-- COVERAGE:GO_TABLE_START -->
| Package | Coverage |
| :--- | :--- |
| pkg/config | 100.0% |
<!-- COVERAGE:GO_TABLE_END -->
Footer`

	if updated != expected {
		t.Fatalf("expected:\n%s\n\ngot:\n%s", expected, updated)
	}
}

func TestReplaceTagContent_MissingTags(t *testing.T) {
	doc := "No tags here"
	updated := replaceTagContent(doc, "<!-- START -->", "<!-- END -->", "new content")
	if updated != doc {
		t.Fatalf("expected untouched doc when tags missing, got: %s", updated)
	}
}

func TestRenderGoTable(t *testing.T) {
	coverages := []PackageCoverage{
		{Package: "pkg/config", Responsibility: "Atomic Config Loader", CoveragePct: 99.4},
		{Package: "pkg/router", Responsibility: "Classifier Pipeline", CoveragePct: 96.9},
	}

	table := renderGoTable(coverages)
	if !strings.Contains(table, "| `pkg/config` | Atomic Config Loader | **99.4%** |") {
		t.Errorf("expected config row in table, got:\n%s", table)
	}
	if !strings.Contains(table, "| `pkg/router` | Classifier Pipeline | **96.9%** |") {
		t.Errorf("expected router row in table, got:\n%s", table)
	}
}

func TestRenderExtTable(t *testing.T) {
	cov := ExtensionCoverage{
		TotalSuites: 13,
		TotalTests:  169,
		PassedTests: 169,
		StmtPct:     96.19,
		LinePct:     96.58,
		FuncPct:     95.71,
	}

	table := renderExtTable(cov)
	if !strings.Contains(table, "**13 / 13 Suites**") || !strings.Contains(table, "**169 / 169 (100%)**") || !strings.Contains(table, "96.19%") {
		t.Errorf("unexpected extension table: %s", table)
	}
}

func TestRenderSummary(t *testing.T) {
	summary := renderSummary(96.4, []PackageCoverage{
		{Package: "pkg/strategy", CoveragePct: 97.9},
		{Package: "pkg/config", CoveragePct: 99.4},
	})
	if !strings.Contains(summary, "96.4%") {
		t.Errorf("expected global 96.4%% in summary, got:\n%s", summary)
	}
}

func TestParseGoCoverage(t *testing.T) {
	sampleOutput := `
ok  	github.com/dixieflatline76/nacho-flow/pkg/config	0.123s	coverage: 99.4% of statements
ok  	github.com/dixieflatline76/nacho-flow/pkg/router	0.456s	coverage: 96.9% of statements
total:											(statements)				96.4%
`
	covs, total, err := parseGoCoverage(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if total != 96.4 {
		t.Errorf("expected total 96.4, got %f", total)
	}
	if len(covs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(covs))
	}
	if covs[0].Package != "pkg/config" || covs[0].CoveragePct != 99.4 {
		t.Errorf("unexpected first package: %+v", covs[0])
	}
}

func TestParseJestCoverage(t *testing.T) {
	sampleOutput := `
Test Suites: 13 passed, 13 total
Tests:       169 passed, 169 total
---------------------------|---------|----------|---------|---------|
All files                  |   96.19 |    81.64 |   95.71 |   96.58 |
---------------------------|---------|----------|---------|---------|
`
	cov, err := parseJestCoverage(sampleOutput)
	if err != nil {
		t.Fatalf("unexpected jest parse error: %v", err)
	}
	if cov.TotalSuites != 13 || cov.PassedTests != 169 || cov.StmtPct != 96.19 || cov.LinePct != 96.58 || cov.FuncPct != 95.71 {
		t.Errorf("unexpected jest cov: %+v", cov)
	}
}
