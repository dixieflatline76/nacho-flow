package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/server"
)

func TestTableReporter_Render(t *testing.T) {
	resp := server.DealsResponse{
		BenchmarkModel:    "anthropic/claude-3.5-sonnet",
		BenchmarkCostPerM: 3.00,
		DealsCount:        2,
		Deals: []contract.DealInfo{
			{
				ModelID:            "google/gemini-2.5-flash-lite",
				TierRole:           "vision_workhorse",
				ContextLength:      1048576,
				PromptCostPerM:     0.10,
				CompletionCostPerM: 0.40,
				DiscountPct:        96.7,
				IsFree:             false,
				CodingIndex:        68.1,
				RecommendedTiers:   []string{"tier_1_vision"},
			},
			{
				ModelID:            "dots-studio/dots-3-note:free",
				TierRole:           "coding_workhorse",
				ContextLength:      512000,
				PromptCostPerM:     0.00,
				CompletionCostPerM: 0.00,
				DiscountPct:        100.0,
				IsFree:             true,
			},
		},
	}

	var buf bytes.Buffer
	reporter := NewTableReporter()
	if err := reporter.Render(&buf, resp); err != nil {
		t.Fatalf("TableReporter.Render failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "HEAT SEEKER") {
		t.Errorf("expected banner in output: %s", out)
	}
	if !strings.Contains(out, "google/gemini-2.5-flash-lite") {
		t.Errorf("expected gemini model in table output")
	}
	if !strings.Contains(out, "1.0M") {
		t.Errorf("expected formatted context window 1.0M in output")
	}
	if !strings.Contains(out, "512k") {
		t.Errorf("expected formatted context window 512k in output")
	}
	if !strings.Contains(out, "Recommended for tier_1_vision") {
		t.Errorf("expected reason why in output")
	}
	if !strings.Contains(out, TipExtensionDashboard) {
		t.Errorf("expected tip at bottom of output")
	}
}

func TestJSONReporter_Render(t *testing.T) {
	resp := server.DealsResponse{
		BenchmarkModel:    "anthropic/claude-3.5-sonnet",
		BenchmarkCostPerM: 3.00,
		DealsCount:        1,
		Deals: []contract.DealInfo{
			{
				ModelID:     "google/gemini-2.5-flash-lite",
				DiscountPct: 96.7,
			},
		},
	}

	var buf bytes.Buffer
	reporter := NewJSONReporter()
	if err := reporter.Render(&buf, resp); err != nil {
		t.Fatalf("JSONReporter.Render failed: %v", err)
	}

	var decoded server.DealsResponse
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal json reporter output: %v", err)
	}
	if decoded.DealsCount != 1 || decoded.BenchmarkModel != "anthropic/claude-3.5-sonnet" {
		t.Errorf("unexpected decoded payload: %+v", decoded)
	}
}

func TestNewDealsReporter(t *testing.T) {
	tableRep := NewDealsReporter(false)
	if _, ok := tableRep.(*TableReporter); !ok {
		t.Errorf("expected *TableReporter, got %T", tableRep)
	}

	jsonRep := NewDealsReporter(true)
	if _, ok := jsonRep.(*JSONReporter); !ok {
		t.Errorf("expected *JSONReporter, got %T", jsonRep)
	}
}

type errWriter struct{}

func (e *errWriter) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("write error")
}

func TestJSONReporter_Render_Error(t *testing.T) {
	reporter := NewJSONReporter()
	err := reporter.Render(&errWriter{}, server.DealsResponse{})
	if err == nil {
		t.Errorf("expected error writing to errWriter, got nil")
	}
}

