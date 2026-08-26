package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/server"
)

func TestCLI_DealsCommand_TableAndJSON(t *testing.T) {
	mockResponse := server.DealsResponse{
		BenchmarkModel:    contract.DefaultBenchmarkModel,
		BenchmarkCostPerM: 3.00,
		DealsCount:        2,
		Deals: []contract.DealInfo{
			{
				Provider:           "openrouter",
				ModelID:            "google/gemini-2.5-flash-lite",
				Name:               "Google: Gemini 2.5 Flash Lite",
				ContextLength:      1048576,
				PromptCostPerM:     0.10,
				CompletionCostPerM: 0.40,
				DiscountPct:        96.67,
				IsFree:             false,
				TierRole:           "vision_workhorse",
				CodingIndex:        68.1,
			},
			{
				Provider:           "openrouter",
				ModelID:            "dots-studio/dots-3-note:free",
				Name:               "Dots 3 Note Free",
				ContextLength:      512000,
				PromptCostPerM:     0.00,
				CompletionCostPerM: 0.00,
				DiscountPct:        100.0,
				IsFree:             true,
				TierRole:           "coding_workhorse",
				CodingIndex:        75.0,
			},
		},
		LastSynced: "2026-08-24T10:00:00Z",
	}

	authChecked := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != contract.PathAPIDeals {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "Bearer secret-token" {
			authChecked = true
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer ts.Close()

	// 1. Test Table Output Mode
	var bufTable bytes.Buffer
	errTable := runDealsCommand(ts.URL, "secret-token", false, &bufTable)
	if errTable != nil {
		t.Fatalf("runDealsCommand table mode failed: %v", errTable)
	}

	tableOutput := bufTable.String()
	if !strings.Contains(tableOutput, "HEAT SEEKER") {
		t.Errorf("expected header in table output: %s", tableOutput)
	}
	if !strings.Contains(tableOutput, "google/gemini-2.5-flash-lite") {
		t.Errorf("expected gemini model in table output")
	}
	if !strings.Contains(tableOutput, "1.0M") {
		t.Errorf("expected 1.0M context formatted")
	}
	if !strings.Contains(tableOutput, "512k") {
		t.Errorf("expected 512k context formatted")
	}
	if !authChecked {
		t.Errorf("expected bearer token to be sent")
	}

	// 2. Test JSON Output Mode
	var bufJSON bytes.Buffer
	errJSON := runDealsCommand(ts.URL, "", true, &bufJSON)
	if errJSON != nil {
		t.Fatalf("runDealsCommand JSON mode failed: %v", errJSON)
	}

	var parsedResp server.DealsResponse
	if err := json.Unmarshal(bufJSON.Bytes(), &parsedResp); err != nil {
		t.Fatalf("failed to decode json output: %v", err)
	}
	if parsedResp.DealsCount != 2 {
		t.Errorf("expected 2 deals in json output, got %d", parsedResp.DealsCount)
	}
}

func TestCLI_DealsCommand_Errors(t *testing.T) {
	// 1. Connection error (daemon down)
	var buf bytes.Buffer
	errDown := runDealsCommand("http://127.0.0.1:99999", "", false, &buf)
	if errDown == nil {
		t.Errorf("expected error connecting to down daemon")
	}

	// 2. Daemon returns HTTP 500
	tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal gateway error"))
	}))
	defer tsErr.Close()

	err500 := runDealsCommand(tsErr.URL, "", false, &buf)
	if err500 == nil {
		t.Errorf("expected error on HTTP 500 from daemon")
	}

	// 3. Daemon returns bad JSON
	tsBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not_json"))
	}))
	defer tsBadJSON.Close()

	errBadJSON := runDealsCommand(tsBadJSON.URL, "", false, &buf)
	if errBadJSON == nil {
		t.Errorf("expected error decoding invalid JSON response")
	}
}

func TestCLI_RunMain_DealsSubcommand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(server.DealsResponse{BenchmarkModel: "claude", BenchmarkCostPerM: 3.0})
	}))
	defer ts.Close()

	// Parse custom flags via runDeals args
	args := []string{"nacho-flow", "deals", "-host", "127.0.0.1", "-port", "8000"}
	_ = runDeals([]string{"-host", "127.0.0.1", "-port", "8000"})
	_ = runMain(args, nil)

	// Test heat-seek alias
	argsAlias := []string{"nacho-flow", "heat-seek", "-host", "127.0.0.1", "-port", "8000"}
	_ = runMain(argsAlias, nil)
}
