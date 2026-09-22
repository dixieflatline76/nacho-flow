package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

func TestGenerateCatalog_MockAPI(t *testing.T) {
	mockResponse := map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"id":             "google/gemini-2.5-flash",
				"name":           "Gemini 2.5 Flash",
				"context_length": 1048576,
				"pricing": map[string]interface{}{
					"prompt":     "0.00000015",
					"completion": "0.00000060",
				},
				"architecture": map[string]interface{}{
					"input_modalities": []string{"text", "image"},
				},
				"supported_parameters": []string{"tools", "temperature"},
				"benchmarks": map[string]interface{}{
					"artificial_analysis": map[string]interface{}{
						"coding_index":  78.4,
						"agentic_index": 95.0,
					},
				},
			},
			{
				"id":             "anthropic/claude-sonnet-5",
				"name":           "Claude Sonnet 5",
				"context_length": 200000,
				"pricing": map[string]interface{}{
					"prompt":     "0.000002",
					"completion": "0.000010",
				},
				"architecture": map[string]interface{}{
					"input_modalities": []string{"text", "image"},
				},
				"supported_parameters": []string{"tools"},
				"benchmarks": map[string]interface{}{
					"artificial_analysis": map[string]interface{}{
						"coding_index":  97.4,
						"agentic_index": 99.0,
					},
				},
			},
			{
				"id":             "qwen/qwen-coder-32b",
				"name":           "Qwen Coder 32B",
				"context_length": 131072,
				"pricing": map[string]interface{}{
					"prompt":     "0.00000030",
					"completion": "0.00000090",
				},
				"architecture": map[string]interface{}{
					"input_modalities": []string{"text"},
				},
				"supported_parameters": []string{"tools"},
			},
			{
				"id":             "someorg/flash-vision-lite",
				"name":           "Flash Vision Lite",
				"context_length": 1000000,
				"architecture": map[string]interface{}{
					"input_modalities": []string{"text", "image"},
				},
				"supported_parameters": []string{"temperature"},
			},
			{
				"id":             "deepseek/deepseek-r1-custom",
				"name":           "DeepSeek R1 Custom",
				"context_length": 65536,
				"architecture": map[string]interface{}{
					"input_modalities": []string{"text"},
				},
				"supported_parameters": []string{"temperature"},
			},
			{
				"id":             "meta/llama-4-general-high",
				"name":           "Llama 4 General High",
				"context_length": 128000,
				"architecture": map[string]interface{}{
					"input_modalities": []string{"text"},
				},
				"benchmarks": map[string]interface{}{
					"artificial_analysis": map[string]interface{}{
						"coding_index": 90.5,
					},
				},
			},
			{
				"id":             "unknown/general-text-only",
				"name":           "General Text Only",
				"context_length": 8192,
				"architecture": map[string]interface{}{
					"input_modalities": []string{"text"},
				},
				"supported_parameters": []string{"temperature"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	cat, err := GenerateCatalog(context.Background(), server.URL, "")
	if err != nil {
		t.Fatalf("GenerateCatalog failed: %v", err)
	}

	if cat.Version != "v1.0.0" {
		t.Errorf("expected default version v1.0.0, got %s", cat.Version)
	}

	// Verify Gemini parsed with benchmarks and provenance from Artificial Analysis
	geminiProfile, exists := cat.Models["google/gemini-2.5-flash"]
	if !exists {
		t.Fatalf("expected gemini in catalog")
	}
	if geminiProfile.CodingIndex != 78.4 {
		t.Errorf("expected 78.4 coding index, got %f", geminiProfile.CodingIndex)
	}
	if geminiProfile.ToolReliability != 95.0 {
		t.Errorf("expected 95.0 tool reliability, got %f", geminiProfile.ToolReliability)
	}
	if geminiProfile.BenchmarkSource != "artificial_analysis" {
		t.Errorf("expected artificial_analysis benchmark source, got %s", geminiProfile.BenchmarkSource)
	}
	if geminiProfile.ProvenanceURL != "https://artificialanalysis.ai" {
		t.Errorf("expected artificialanalysis.ai provenance, got %s", geminiProfile.ProvenanceURL)
	}
	if !geminiProfile.SupportsVision {
		t.Errorf("expected gemini to support vision")
	}
	if !geminiProfile.SupportsTools {
		t.Errorf("expected gemini to support tools")
	}
	if geminiProfile.PromptCostPerMillion != 0.15 {
		t.Errorf("expected 0.15 prompt cost, got %f", geminiProfile.PromptCostPerMillion)
	}

	// Verify Claude Sonnet 5 recognized as frontier coding workhorse
	sonnetProfile, exists := cat.Models["anthropic/claude-sonnet-5"]
	if !exists {
		t.Fatalf("expected sonnet-5 in catalog")
	}
	if sonnetProfile.TierRole != curation.RoleCodingWorkhorse {
		t.Errorf("expected RoleCodingWorkhorse for sonnet-5, got %s", sonnetProfile.TierRole)
	}
	if sonnetProfile.PromptCostPerMillion != 2.0 {
		t.Errorf("expected 2.0 prompt cost, got %f", sonnetProfile.PromptCostPerMillion)
	}
	if len(sonnetProfile.RecommendedTiers) < 2 || sonnetProfile.RecommendedTiers[0] != "tier_4_frontier" {
		t.Errorf("expected tier_4_frontier for sonnet-5, got %v", sonnetProfile.RecommendedTiers)
	}

	// Verify Qwen was categorized as RoleCodingWorkhorse
	qwenProfile, exists := cat.Models["qwen/qwen-coder-32b"]
	if !exists || qwenProfile.TierRole != curation.RoleCodingWorkhorse {
		t.Errorf("expected RoleCodingWorkhorse for qwen-coder-32b")
	}

	// Verify Flash Vision Lite categorized as RoleVisionWorkhorse
	flashProfile, exists := cat.Models["someorg/flash-vision-lite"]
	if !exists || flashProfile.TierRole != curation.RoleVisionWorkhorse {
		t.Errorf("expected RoleVisionWorkhorse for flash-vision-lite")
	}

	// Verify R1 categorized as RoleDeepReasoner
	r1Profile, exists := cat.Models["deepseek/deepseek-r1-custom"]
	if !exists || r1Profile.TierRole != curation.RoleDeepReasoner {
		t.Errorf("expected RoleDeepReasoner for r1-custom")
	}

	// Verify Llama 4 high benchmark included with frontier tier
	llamaProfile, exists := cat.Models["meta/llama-4-general-high"]
	if !exists {
		t.Errorf("expected llama-4-general-high in catalog")
	} else if len(llamaProfile.RecommendedTiers) == 0 || llamaProfile.RecommendedTiers[0] != "tier_4_frontier" {
		t.Errorf("expected tier_4_frontier for llama-4-general-high, got %v", llamaProfile.RecommendedTiers)
	}

	// Verify uncatalogued general model with 0 coding index is excluded
	if _, exists := cat.Models["unknown/general-text-only"]; exists {
		t.Errorf("expected unknown/general-text-only to be filtered out of catalog")
	}
}

func TestGenerateCatalog_Errors(t *testing.T) {
	// 1. Unreachable server
	_, err := GenerateCatalog(context.Background(), "http://127.0.0.1:99999/unreachable", "v1.0.0")
	if err == nil {
		t.Errorf("expected error on unreachable server")
	}

	// 2. HTTP 500
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server500.Close()

	_, err500 := GenerateCatalog(context.Background(), server500.URL, "v1.0.0")
	if err500 == nil {
		t.Errorf("expected error on HTTP 500")
	}

	// 3. Bad JSON
	serverBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{invalid json"))
	}))
	defer serverBadJSON.Close()

	_, errBadJSON := GenerateCatalog(context.Background(), serverBadJSON.URL, "v1.0.0")
	if errBadJSON == nil {
		t.Errorf("expected error on bad JSON")
	}
}

func TestRun_FullExecution(t *testing.T) {
	mockResponse := map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"id":   "google/gemini-2.5-flash",
				"name": "Gemini 2.5 Flash",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	outFile := tempDir + "/models.json"
	embedFile := tempDir + "/embed_models.json"

	args := []string{"-version", "v1.5.0", "-out", outFile, "-embed-out", embedFile}
	err := run(args, server.URL)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	// Flag parse error
	_ = run([]string{"-invalid-flag"}, "")

	// Generator error from invalid server URL
	errGen := run([]string{}, "http://127.0.0.1:99999")
	if errGen == nil {
		t.Errorf("expected error from invalid server URL")
	}

	// Run with empty out and embed paths
	errEmpty := run([]string{"-out", "", "-embed-out", ""}, server.URL)
	if errEmpty != nil {
		t.Errorf("expected success with empty output paths, got %v", errEmpty)
	}

	// Write error paths
	_ = run([]string{"-out", tempDir, "-embed-out", embedFile}, server.URL)
	_ = run([]string{"-out", outFile, "-embed-out", tempDir}, server.URL)

	// Test main() execution with intercepted logFatal and isolated args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"gen_catalog", "-out", outFile, "-embed-out", embedFile}
	oldLogFatal := logFatal
	defer func() { logFatal = oldLogFatal }()
	logFatal = func(format string, v ...interface{}) {}
	main()
}
