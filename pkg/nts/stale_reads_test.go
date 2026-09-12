package nts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentifyStaleToolCallIDs_OpenAI_MultiRead(t *testing.T) {
	msgs := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_1",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/board/board.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call_1",
			"content":      "package board\n// 500 lines of code from Turn 1",
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_2",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/board/board.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call_2",
			"content":      "package board\n// 500 lines of code from Turn 2",
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_3",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/board/board.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call_3",
			"content":      "package board\n// 500 lines of code from Turn 3 (latest)",
		},
	}

	staleIDs := IdentifyStaleToolCallIDs(msgs)
	if staleIDs == nil {
		t.Fatalf("expected staleIDs to be non-nil")
	}

	if _, ok := staleIDs["call_1"]; !ok {
		t.Errorf("expected call_1 to be marked stale")
	}
	if _, ok := staleIDs["call_2"]; !ok {
		t.Errorf("expected call_2 to be marked stale")
	}
	if _, ok := staleIDs["call_3"]; ok {
		t.Errorf("expected call_3 to NOT be marked stale (it is the latest active read)")
	}
}

func TestIdentifyStaleToolCallIDs_WriteThenRead(t *testing.T) {
	msgs := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_read_old",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/solver/solver.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call_read_old",
			"content":      "package solver\n// old code",
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_write",
					"function": map[string]interface{}{
						"name":      "write_to_file",
						"arguments": `{"path": "pkg/solver/solver.go", "content": "package solver\n// new code"}`,
					},
				},
			},
		},
	}

	staleIDs := IdentifyStaleToolCallIDs(msgs)
	if staleIDs == nil {
		t.Fatalf("expected staleIDs to be non-nil")
	}

	if _, ok := staleIDs["call_read_old"]; !ok {
		t.Errorf("expected call_read_old to be marked stale after write_to_file")
	}
}

func TestIdentifyStaleToolCallIDs_LineRanges(t *testing.T) {
	// 1. Two different line ranges should NOT supersede each other
	msgsDifferentRanges := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_range_1",
					"function": map[string]interface{}{
						"name":      "view_file",
						"arguments": `{"path": "app.go", "startLine": 1, "endLine": 50}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_range_2",
					"function": map[string]interface{}{
						"name":      "view_file",
						"arguments": `{"path": "app.go", "startLine": 51, "endLine": 100}`,
					},
				},
			},
		},
	}

	staleDifferent := IdentifyStaleToolCallIDs(msgsDifferentRanges)
	if staleDifferent != nil {
		t.Errorf("expected nil stale IDs for different line ranges, got: %v", staleDifferent)
	}

	// 2. Later full read DOES supersede earlier line-range reads
	msgsWithFullRead := append(msgsDifferentRanges, map[string]interface{}{
		"role": "assistant",
		"tool_calls": []interface{}{
			map[string]interface{}{
				"id": "call_full",
				"function": map[string]interface{}{
					"name":      "read_file",
					"arguments": `{"path": "app.go"}`,
				},
			},
		},
	})

	staleWithFull := IdentifyStaleToolCallIDs(msgsWithFullRead)
	if staleWithFull == nil {
		t.Fatalf("expected stale IDs when full read supersedes range reads")
	}
	if _, ok := staleWithFull["call_range_1"]; !ok {
		t.Errorf("expected call_range_1 to be superseded by full read")
	}
	if _, ok := staleWithFull["call_range_2"]; !ok {
		t.Errorf("expected call_range_2 to be superseded by full read")
	}
	if _, ok := staleWithFull["call_full"]; ok {
		t.Errorf("expected call_full to NOT be marked stale")
	}
}

func TestTransformer_StaleFileReadEviction_OpenAI(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CompactStaleFileReads = true
	cfg.StaleReadDepth = 1
	tr := NewTransformer(cfg)

	largeFileContent := strings.Repeat("package board\nfunc Solve() bool { return true }\n", 100) // ~4,000 bytes

	payload := map[string]interface{}{
		"messages": []interface{}{
			// Turn 1: Read board.go
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "call_turn1",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/board/board.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_turn1",
				"content":      largeFileContent,
			},
			// Turn 2: Read solver.go (only read of solver.go, must remain untouched)
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "call_solver",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/solver/solver.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_solver",
				"content":      "package solver\nfunc MinConflicts() {}",
			},
			// Turn 3: Re-read board.go (latest read, must remain untouched)
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "call_turn3",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/board/board.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_turn3",
				"content":      largeFileContent,
			},
		},
	}

	rawJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	transformed, res, err := tr.TransformOpenAI(rawJSON)
	if err != nil {
		t.Fatalf("TransformOpenAI error: %v", err)
	}

	if res.BytesSaved <= 0 {
		t.Errorf("expected BytesSaved > 0, got %d", res.BytesSaved)
	}
	if res.TokensSaved <= 0 {
		t.Errorf("expected TokensSaved > 0, got %d", res.TokensSaved)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(transformed, &parsed); err != nil {
		t.Fatalf("failed to unmarshal transformed JSON: %v", err)
	}

	msgs := parsed["messages"].([]interface{})

	// Message 1 (tool result for call_turn1) MUST be evicted
	msg1Content := msgs[1].(map[string]interface{})["content"].(string)
	if msg1Content != StaleFileReadNotice {
		t.Errorf("expected call_turn1 content to be evicted to notice, got: %s", msg1Content)
	}

	// Message 3 (tool result for call_solver) MUST be untouched
	msg3Content := msgs[3].(map[string]interface{})["content"].(string)
	if msg3Content != "package solver\nfunc MinConflicts() {}" {
		t.Errorf("expected call_solver content to remain untouched, got: %s", msg3Content)
	}

	// Message 5 (tool result for call_turn3) MUST be untouched (latest read)
	msg5Content := msgs[5].(map[string]interface{})["content"].(string)
	if msg5Content != largeFileContent {
		t.Errorf("expected latest read (call_turn3) to remain completely untouched")
	}
}

func TestTransformer_StaleFileReadEviction_Anthropic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CompactStaleFileReads = true
	cfg.StaleReadDepth = 1
	tr := NewTransformer(cfg)

	largeFileContent := strings.Repeat("export const Card = () => {};\n", 80)

	payload := map[string]interface{}{
		"messages": []interface{}{
			// Turn 1: tool_use read_file
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   "toolu_1",
						"name": "read_file",
						"input": map[string]interface{}{
							"path": "src/Card.ts",
						},
					},
				},
			},
			// Turn 1: tool_result
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":         "tool_result",
						"tool_use_id":  "toolu_1",
						"content":      largeFileContent,
					},
				},
			},
			// Turn 2: tool_use write_to_file (supersedes toolu_1)
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "tool_use",
						"id":   "toolu_2",
						"name": "write_to_file",
						"input": map[string]interface{}{
							"path":    "src/Card.ts",
							"content": "export const Card = () => { return 42; };\n",
						},
					},
				},
			},
		},
	}

	rawJSON, _ := json.Marshal(payload)
	transformed, res, err := tr.TransformAnthropic(rawJSON)
	if err != nil {
		t.Fatalf("TransformAnthropic error: %v", err)
	}

	if res.BytesSaved <= 0 {
		t.Errorf("expected positive BytesSaved, got %d", res.BytesSaved)
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(transformed, &parsed)

	msgs := parsed["messages"].([]interface{})
	userContent := msgs[1].(map[string]interface{})["content"].([]interface{})
	part := userContent[0].(map[string]interface{})

	if part["content"] != StaleFileReadNotice {
		t.Errorf("expected Anthropic tool_result to be evicted to notice, got: %v", part["content"])
	}
}

func TestTransformer_StaleFileRead_PreserveCacheControl(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CompactStaleFileReads = true
	cfg.PreserveCacheControl = true
	cfg.StaleReadDepth = 1
	tr := NewTransformer(cfg)

	payload := map[string]interface{}{
		"messages": []interface{}{
			// Turn 1: read_file
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "call_cached",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "main.go"}`,
						},
					},
				},
			},
			// Tool result marked with cache_control breakpoint
			map[string]interface{}{
				"role":          "tool",
				"tool_call_id":  "call_cached",
				"content":       "package main\nfunc main() {}",
				"cache_control": map[string]interface{}{"type": "ephemeral"},
			},
			// Turn 2: re-read main.go
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "call_new",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "main.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_new",
				"content":      "package main\nfunc main() {}",
			},
		},
	}

	rawJSON, _ := json.Marshal(payload)
	transformed, res, err := tr.TransformOpenAI(rawJSON)
	if err != nil {
		t.Fatalf("TransformOpenAI error: %v", err)
	}

	// Because call_cached has cache_control and PreserveCacheControl is true, it MUST NOT be mutated
	if res.BytesSaved != 0 {
		t.Errorf("expected 0 bytes saved when cache_control is protected, got %d", res.BytesSaved)
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(transformed, &parsed)
	msgs := parsed["messages"].([]interface{})
	msg1Content := msgs[1].(map[string]interface{})["content"].(string)
	if msg1Content != "package main\nfunc main() {}" {
		t.Errorf("expected cached message to remain completely untouched")
	}
}

func TestStaleReads_EdgeCasesAndBranchCoverage(t *testing.T) {
	// 1. calculateContentLength branches
	if calculateContentLength([]byte("hello")) != 5 {
		t.Errorf("expected 5 bytes for []byte")
	}
	contentBlocks := []interface{}{
		map[string]interface{}{"type": "text", "text": "abc"},
		map[string]interface{}{"type": "image_url"}, // no text
		map[string]interface{}{"type": "text", "text": "1234"},
	}
	if calculateContentLength(contentBlocks) != 7 {
		t.Errorf("expected 7 bytes for content blocks, got %d", calculateContentLength(contentBlocks))
	}
	if calculateContentLength(12345) != 0 {
		t.Errorf("expected 0 for unsupported type")
	}

	// 2. getIntFromMap branches
	mInt := map[string]interface{}{
		"startLine": float64(10),
		"endLine":   20,
		"other":     "30",
	}
	if getIntFromMap(mInt, "startLine") != 10 {
		t.Errorf("expected 10 from float64")
	}
	if getIntFromMap(mInt, "endLine") != 20 {
		t.Errorf("expected 20 from int")
	}
	if getIntFromMap(mInt, "other") != 30 {
		t.Errorf("expected 30 from string")
	}
	if getIntFromMap(mInt, "nonexistent") != 0 {
		t.Errorf("expected 0 for missing key")
	}

	// 3. normalizePath branches
	if normalizePath("C:\\Users\\Karl\\code.go") != "c:/Users/Karl/code.go" {
		t.Errorf("expected normalized Windows path, got %s", normalizePath("C:\\Users\\Karl\\code.go"))
	}
	if normalizePath("./relative/path.ts") != "relative/path.ts" {
		t.Errorf("expected trimmed ./ prefix, got %s", normalizePath("./relative/path.ts"))
	}

	// 4. extractFilePaths unsupported type
	p1, p2 := extractFilePaths(12345)
	if p1 != "" || p2 != "" {
		t.Errorf("expected empty paths for unsupported type")
	}

	// 5. extractPathsFromMap without recognized keys
	p3, p4 := extractPathsFromMap(map[string]interface{}{"unknown": "value"})
	if p3 != "" || p4 != "" {
		t.Errorf("expected empty paths when no key matches")
	}

	// 6. extractPathsFromString empty or missing key
	p5, p6 := extractPathsFromString("")
	if p5 != "" || p6 != "" {
		t.Errorf("expected empty paths for empty string")
	}
	p7, p8 := extractPathsFromString(`{"unknown":"val"}`)
	if p7 != "" || p8 != "" {
		t.Errorf("expected empty paths for unrecognized string")
	}

	// 7. IdentifyStaleToolCallIDs edge cases (empty or single message)
	if IdentifyStaleToolCallIDs(nil) != nil {
		t.Errorf("expected nil for nil messages")
	}
	if IdentifyStaleToolCallIDs([]interface{}{"not-a-map"}) != nil {
		t.Errorf("expected nil for non-map messages")
	}
	singleCall := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "c1",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "a.go"}`,
					},
				},
			},
		},
	}
	if IdentifyStaleToolCallIDs(singleCall) != nil {
		t.Errorf("expected nil for single tool call")
	}
}

func BenchmarkIdentifyStaleToolCallIDs(b *testing.B) {
	msgs := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "c1",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/board/board.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "c2",
					"function": map[string]interface{}{
						"name":      "write_to_file",
						"arguments": `{"path": "pkg/board/board.go", "content": "..."}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "c3",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/board/board.go"}`,
					},
				},
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = IdentifyStaleToolCallIDs(msgs)
	}
}

func TestTransformer_RealBenchmark_ClineAndZooTurns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CompactStaleFileReads = true
	tr := NewTransformer(cfg)

	// Realistic multi-turn sequence matching Cline (OpenAI tool_calls) and Zoo (Anthropic tool_use)
	// on real benchmark files (tsconfig.json, src/index.ts, vitest.config.ts)
	t.Run("Cline real benchmark turns: read tsconfig, edit, re-read", func(t *testing.T) {
		clinePayload := map[string]interface{}{
			"messages": []interface{}{
				// Turn 2: Cline reads tsconfig.json
				map[string]interface{}{
					"role": "assistant",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_cline_read1",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "read_file",
								"arguments": `{"path": "tsconfig.json"}`,
							},
						},
					},
				},
				map[string]interface{}{
					"role":         "tool",
					"tool_call_id": "call_cline_read1",
					"content":      "{\n  \"compilerOptions\": {\n    \"moduleResolution\": \"node\",\n    \"baseUrl\": \".\"\n  }\n}",
				},
				// Turn 5: Cline uses editor to modify tsconfig.json
				map[string]interface{}{
					"role": "assistant",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_cline_edit",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "editor",
								"arguments": `{"path": "tsconfig.json", "operation": "modified"}`,
							},
						},
					},
				},
				map[string]interface{}{
					"role":         "tool",
					"tool_call_id": "call_cline_edit",
					"content":      `{"path":"tsconfig.json","operation":"modified","notice":"You do not need to re-read the file"}`,
				},
				// Turn 8: Cline reads vitest.config.ts (first & only read)
				map[string]interface{}{
					"role": "assistant",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_cline_vitest",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "read_file",
								"arguments": `{"path": "vitest.config.ts"}`,
							},
						},
					},
				},
				map[string]interface{}{
					"role":         "tool",
					"tool_call_id": "call_cline_vitest",
					"content":      "import { defineConfig } from 'vitest/config';\nexport default defineConfig({});",
				},
			},
		}

		raw, _ := json.Marshal(clinePayload)
		transformed, res, err := tr.TransformOpenAI(raw)
		if err != nil {
			t.Fatalf("TransformOpenAI error: %v", err)
		}

		if res.BytesSaved <= 0 {
			t.Errorf("expected positive bytes saved for superseded tsconfig.json read")
		}

		var parsed map[string]interface{}
		_ = json.Unmarshal(transformed, &parsed)
		msgs := parsed["messages"].([]interface{})

		// First tsconfig read (index 1) MUST be evicted
		read1Content := msgs[1].(map[string]interface{})["content"].(string)
		if read1Content != StaleFileReadNotice {
			t.Errorf("expected Turn 2 read_file to be evicted, got: %s", read1Content)
		}

		// Vitest read (index 5) MUST be untouched
		vitestContent := msgs[5].(map[string]interface{})["content"].(string)
		if !strings.Contains(vitestContent, "defineConfig") {
			t.Errorf("expected vitest read to remain completely untouched")
		}
	})

	t.Run("Zoo Code real benchmark turns: Anthropic tool_use and apply_diff", func(t *testing.T) {
		zooPayload := map[string]interface{}{
			"messages": []interface{}{
				// Turn 3: Zoo reads src/index.ts
				map[string]interface{}{
					"role": "assistant",
					"content": []interface{}{
						map[string]interface{}{
							"type":  "tool_use",
							"id":    "tu_zoo_read1",
							"name":  "read_file",
							"input": map[string]interface{}{"path": "src/index.ts"},
						},
					},
				},
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{
							"type":        "tool_result",
							"tool_use_id": "tu_zoo_read1",
							"content":     "import './models/Card';\nimport './models/Deck';\n// 200 lines of game logic",
						},
					},
				},
				// Turn 10: Zoo applies diff to src/index.ts
				map[string]interface{}{
					"role": "assistant",
					"content": []interface{}{
						map[string]interface{}{
							"type":  "tool_use",
							"id":    "tu_zoo_diff",
							"name":  "apply_diff",
							"input": map[string]interface{}{"path": "src/index.ts", "diff": "-old\n+new"},
						},
					},
				},
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{
							"type":        "tool_result",
							"tool_use_id": "tu_zoo_diff",
							"content":     "Successfully applied diff to src/index.ts",
						},
					},
				},
			},
		}

		raw, _ := json.Marshal(zooPayload)
		transformed, res, err := tr.TransformAnthropic(raw)
		if err != nil {
			t.Fatalf("TransformAnthropic error: %v", err)
		}

		if res.BytesSaved <= 0 {
			t.Errorf("expected positive bytes saved for Zoo Code apply_diff superseding read")
		}

		var parsed map[string]interface{}
		_ = json.Unmarshal(transformed, &parsed)
		msgs := parsed["messages"].([]interface{})
		userContent := msgs[1].(map[string]interface{})["content"].([]interface{})
		part := userContent[0].(map[string]interface{})

		if part["content"] != StaleFileReadNotice {
			t.Errorf("expected Zoo Code read to be evicted to notice, got: %v", part["content"])
		}
	})
}

func TestIdentifyStaleToolCallIDs_OffsetLimitRanges(t *testing.T) {
	// Test that Zoo Code style offset/limit slice parameters are parsed correctly
	// Disjoint slices (1..30 vs 85..115) should NOT supersede each other.
	msgs := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "slice_1",
					"name": "read_file",
					"input": map[string]interface{}{
						"path":   "server.go",
						"mode":   "slice",
						"offset": 1,
						"limit":  30,
					},
				},
			},
		},
		map[string]interface{}{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "slice_2",
					"name": "read_file",
					"input": map[string]interface{}{
						"path":   "server.go",
						"mode":   "slice",
						"offset": 85,
						"limit":  30,
					},
				},
			},
		},
	}

	stale := IdentifyStaleToolCallIDs(msgs)
	if stale != nil {
		t.Errorf("expected disjoint offset/limit slices to NOT supersede each other, got: %v", stale)
	}

	// But an identical offset/limit slice DOES supersede
	msgsIdentical := append(msgs, map[string]interface{}{
		"role": "assistant",
		"content": []interface{}{
			map[string]interface{}{
				"type": "tool_use",
				"id":   "slice_3",
				"name": "read_file",
				"input": map[string]interface{}{
					"path":   "server.go",
					"mode":   "slice",
					"offset": 1,
					"limit":  30,
				},
			},
		},
	})

	stale2 := IdentifyStaleToolCallIDs(msgsIdentical)
	if stale2 == nil || len(stale2) != 1 {
		t.Fatalf("expected 1 stale ID, got: %v", stale2)
	}
	if _, ok := stale2["slice_1"]; !ok {
		t.Errorf("expected slice_1 to be superseded by identical slice_3")
	}
}

func TestTransformer_RealTasksDatasetReplay(t *testing.T) {
	tasksDir := filepath.Join(os.Getenv("APPDATA"), "Code", "User", "globalStorage", "zoocodeorganization.zoo-code", "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		t.Skip("skipping real tasks replay: tasks directory not found")
	}

	cfg := DefaultConfig()
	cfg.CompactStaleFileReads = true
	tr := NewTransformer(cfg)

	totalBytesSaved := 0
	tasksWithCompaction := 0

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		histPath := filepath.Join(tasksDir, e.Name(), "api_conversation_history.json")
		data, err := os.ReadFile(histPath)
		if err != nil {
			continue
		}

		var rawMsgs []interface{}
		if err := json.Unmarshal(data, &rawMsgs); err != nil {
			continue
		}

		payload := map[string]interface{}{
			"messages": rawMsgs,
		}
		rawPayload, _ := json.Marshal(payload)

		transformed, res, err := tr.TransformAnthropic(rawPayload)
		if err != nil {
			t.Fatalf("task %s failed to transform: %v", e.Name(), err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(transformed, &parsed); err != nil {
			t.Fatalf("task %s transformed output is invalid JSON: %v", e.Name(), err)
		}

		if res.BytesSaved > 0 {
			tasksWithCompaction++
			totalBytesSaved += res.BytesSaved
		}
	}

	if tasksWithCompaction == 0 || totalBytesSaved == 0 {
		t.Errorf("expected real tasks dataset to produce positive savings, got 0 bytes")
	}
}

func TestIdentifyStaleToolCallIDs_ConfigurableDepth(t *testing.T) {
	msgs := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_1",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/solver/solver.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call_1",
			"content":      "content 1",
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_2",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/solver/solver.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call_2",
			"content":      "content 2",
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_3",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/solver/solver.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call_3",
			"content":      "content 3",
		},
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_4",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path": "pkg/solver/solver.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call_4",
			"content":      "content 4",
		},
	}

	// 1. With depth = 3: only call_1 is stale, call_2, call_3, call_4 are retained
	staleDepth3 := IdentifyStaleToolCallIDs(msgs, 3)
	if staleDepth3 == nil {
		t.Fatalf("expected staleDepth3 to be non-nil")
	}
	if _, ok := staleDepth3["call_1"]; !ok {
		t.Errorf("expected call_1 to be stale with depth 3")
	}
	if _, ok := staleDepth3["call_2"]; ok {
		t.Errorf("expected call_2 to NOT be stale with depth 3")
	}
	if _, ok := staleDepth3["call_3"]; ok {
		t.Errorf("expected call_3 to NOT be stale with depth 3")
	}
	if _, ok := staleDepth3["call_4"]; ok {
		t.Errorf("expected call_4 to NOT be stale with depth 3")
	}

	// 2. With default (depth <= 0 or 1): call_1, call_2, call_3 are stale
	staleDepth1 := IdentifyStaleToolCallIDs(msgs, 1)
	if _, ok := staleDepth1["call_1"]; !ok {
		t.Errorf("expected call_1 to be stale with depth 1")
	}
	if _, ok := staleDepth1["call_2"]; !ok {
		t.Errorf("expected call_2 to be stale with depth 1")
	}
	if _, ok := staleDepth1["call_3"]; !ok {
		t.Errorf("expected call_3 to be stale with depth 1")
	}
	if _, ok := staleDepth1["call_4"]; ok {
		t.Errorf("expected call_4 to NOT be stale with depth 1")
	}
}

func TestTransformer_ToolErrorImmunity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CompactStaleFileReads = true
	cfg.StaleReadDepth = 1 // strict depth 1
	tr := NewTransformer(cfg)

	// An error response followed by a successful read
	payload := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "call_err",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/solver/solver.go", "anchor_line": 0}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_err",
				"content":      "Error: anchor_line must be a 1-indexed line number (got 0). Line numbers start at 1.",
			},
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "call_ok",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/solver/solver.go", "anchor_line": 1}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "call_ok",
				"content":      "package solver\nfunc Solve() {}",
			},
		},
	}

	rawBytes, _ := json.Marshal(payload)
	transformed, _, err := tr.TransformOpenAI(rawBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	transStr := string(transformed)
	if !strings.Contains(transStr, "anchor_line must be a 1-indexed line number") {
		t.Errorf("tool error was incorrectly evicted or compacted by NTS: %s", transStr)
	}
	if strings.Contains(transStr, StaleFileReadNotice) {
		t.Errorf("error tool result should not be marked as StaleFileReadNotice: %s", transStr)
	}
}

func TestTransformer_ConfigurableDepth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CompactStaleFileReads = true
	cfg.StaleReadDepth = 3
	tr := NewTransformer(cfg)

	payload := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "c1",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/foo.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "c1",
				"content":      "read 1 content that is long enough to compact 1234567890",
			},
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "c2",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/foo.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "c2",
				"content":      "read 2 content that is long enough to compact 1234567890",
			},
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "c3",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/foo.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "c3",
				"content":      "read 3 content that is long enough to compact 1234567890",
			},
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "c4",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path": "pkg/foo.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "c4",
				"content":      "read 4 content that is long enough to compact 1234567890",
			},
		},
	}

	rawBytes, _ := json.Marshal(payload)
	transformed, res, err := tr.TransformOpenAI(rawBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.BytesSaved == 0 {
		t.Fatalf("expected bytes saved from c1 eviction")
	}

	var parsed map[string]interface{}
	json.Unmarshal(transformed, &parsed)
	msgs := parsed["messages"].([]interface{})

	// c1 (msg index 1) should be evicted
	if content := msgs[1].(map[string]interface{})["content"].(string); content != StaleFileReadNotice {
		t.Errorf("expected c1 to be evicted, got: %s", content)
	}
	// c2, c3, c4 (msg index 3, 5, 7) should NOT be evicted
	if content := msgs[3].(map[string]interface{})["content"].(string); content == StaleFileReadNotice {
		t.Errorf("expected c2 to be preserved with depth 3")
	}
	if content := msgs[5].(map[string]interface{})["content"].(string); content == StaleFileReadNotice {
		t.Errorf("expected c3 to be preserved with depth 3")
	}
	if content := msgs[7].(map[string]interface{})["content"].(string); content == StaleFileReadNotice {
		t.Errorf("expected c4 to be preserved with depth 3")
	}
}

