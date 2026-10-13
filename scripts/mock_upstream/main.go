// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type chatRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

var (
	callCounts   = make(map[string]int)
	callCountsMu sync.Mutex
)

func getCallCount(model string) int {
	callCountsMu.Lock()
	defer callCountsMu.Unlock()
	count := callCounts[model]
	callCounts[model] = count + 1
	return count
}

func main() {
	port := flag.Int("port", 8085, "Port to listen on")
	fixtureDir := flag.String("fixtures", filepath.Join("pkg", "server", "testdata", "truncation"), "Path to truncation fixtures")
	flag.Parse()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","server":"mock_upstream"}`))
	})

	http.HandleFunc("/v1/chat/completions", handleChat(*fixtureDir))
	http.HandleFunc("/chat/completions", handleChat(*fixtureDir))

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("Mock upstream listening on http://%s (fixtures: %s)", addr, *fixtureDir)
	server := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleChat(fixtureDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		var req chatRequest
		_ = json.Unmarshal(bodyBytes, &req)
		model := req.Model

		log.Printf("Inbound chat request for model: %s (stream=%v)", model, req.Stream)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)

		var sseContent string
		switch model {
		case "mock/f1-reasoning-length", "f1-reasoning-length":
			sseContent = loadFixture(fixtureDir, "F1_reasoning_length.sse")
		case "mock/f2-reasoning-stop", "f2-reasoning-stop":
			sseContent = loadFixture(fixtureDir, "F2_reasoning_stop_empty.sse")
		case "mock/f3-whitespace-content-length", "f3-whitespace-content-length":
			sseContent = loadFixture(fixtureDir, "F3_whitespace_content_length.sse")
		case "mock/f4-partial-toolcall-length", "f4-partial-toolcall-length":
			sseContent = loadFixture(fixtureDir, "F4_partial_toolcall_length.sse")
		case "mock/f5-partial-prose-length", "f5-partial-prose-length":
			sseContent = loadFixture(fixtureDir, "F5_partial_prose_length.sse")
		case "mock/f6-usage-in-finish", "f6-usage-in-finish":
			sseContent = loadFixture(fixtureDir, "F6_usage_in_finish_chunk.sse")
		case "mock/f7-no-reasoning-length", "f7-no-reasoning-length":
			sseContent = loadFixture(fixtureDir, "F7_no_reasoning_length.sse")
		case "mock/f8-anthropic-max-tokens", "f8-anthropic-max-tokens":
			sseContent = loadFixture(fixtureDir, "F8_anthropic_max_tokens.sse")
		case "mock/f9-inline-think-length", "f9-inline-think-length":
			sseContent = loadFixture(fixtureDir, "F9_inline_think_length.sse")

		case "mock/f1-then-toolcall":
			turn := getCallCount(model)
			if turn%2 == 0 {
				log.Printf("mock/f1-then-toolcall [Call 0]: emitting F1 reasoning length")
				sseContent = loadFixture(fixtureDir, "F1_reasoning_length.sse")
			} else {
				log.Printf("mock/f1-then-toolcall [Call 1]: emitting recovered tool call")
				sseContent = `data: {"id":"gen-rec-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_mock_rec_1","type":"function","function":{"name":"execute_command","arguments":"{\"command\":\"echo recovery successful\"}"}}]}}]}

data: {"id":"gen-rec-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"id":"gen-rec-1","choices":[],"usage":{"prompt_tokens":150,"completion_tokens":40,"total_tokens":190}}

data: [DONE]

`
			}

		case "mock/f1-then-f1":
			log.Printf("mock/f1-then-f1: emitting F1")
			sseContent = loadFixture(fixtureDir, "F1_reasoning_length.sse")

		default:
			// Default fallback: emit a healthy response
			sseContent = `data: {"id":"gen-default","choices":[{"index":0,"delta":{"content":"Mock upstream healthy response."}}]}

data: {"id":"gen-default","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
		}

		lines := strings.Split(sseContent, "\n")
		for _, line := range lines {
			_, _ = fmt.Fprintf(w, "%s\n", line)
			if ok {
				flusher.Flush()
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func loadFixture(dir, filename string) string {
	path := filepath.Clean(filepath.Join(dir, filename))
	// #nosec G304 - mock test server loads static test fixtures
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("ERROR loading fixture %s: %v", path, err)
		return fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":\"Fixture error: %v\"}}]}\n\ndata: [DONE]\n\n", err)
	}
	return string(data)
}
