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
				"architecture": map[string]interface{}{
					"input_modalities": []string{"text", "image"},
				},
				"supported_parameters": []string{"tools", "temperature"},
			},
			{
				"id":             "qwen/qwen-coder-32b",
				"name":           "Qwen Coder 32B",
				"context_length": 131072,
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

	// Verify Gemini override took precedence
	geminiProfile, exists := cat.Models["google/gemini-2.5-flash"]
	if !exists {
		t.Fatalf("expected gemini in catalog")
	}
	if geminiProfile.CodingIndex != 78.4 {
		t.Errorf("expected 78.4 coding index, got %f", geminiProfile.CodingIndex)
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
