package nts

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripANSIInPlace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple color escape",
			input:    "\x1b[31mError:\x1b[0m something failed",
			expected: "Error: something failed",
		},
		{
			name:     "256-color and 24-bit RGB codes",
			input:    "\x1b[38;5;246m╭\x1b[0m\x1b[38;5;246m─\x1b[0m[\x1b[38;2;255;0;0mFAIL\x1b[0m]",
			expected: "╭─[FAIL]",
		},
		{
			name:     "cursor and terminal controls",
			input:    "\x1b[?25hVisible\x1b[?25lHidden\x1b[2JClear",
			expected: "VisibleHiddenClear",
		},
		{
			name:     "OSC title and bell sequence",
			input:    "\x1b]0;npm run test\x07Running tests...",
			expected: "Running tests...",
		},
		{
			name:     "no escapes present",
			input:    "Clean line with no ANSI at all.",
			expected: "Clean line with no ANSI at all.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			out := StripANSIInPlace(buf)
			if string(out) != tt.expected {
				t.Errorf("got %q, want %q", string(out), tt.expected)
			}
		})
	}
}

func TestResolveCarriageReturnsInPlace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single line ticker overwrite",
			input:    "downloading 10%\rdownloading 50%\rdownloading 100%\n",
			expected: "downloading 100%\n",
		},
		{
			name:     "multi-line with intermediate ticker",
			input:    "Starting...\nProgress: 0%\rProgress: 50%\rProgress: 100%\nDone.\n",
			expected: "Starting...\nProgress: 100%\nDone.\n",
		},
		{
			name:     "CRLF preservation",
			input:    "line 1\r\nline 2\r\nline 3\r\n",
			expected: "line 1\nline 2\nline 3\n",
		},
		{
			name:     "no carriage returns",
			input:    "pure unix newlines\nsecond line\n",
			expected: "pure unix newlines\nsecond line\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			out := ResolveCarriageReturnsInPlace(buf)
			if string(out) != tt.expected {
				t.Errorf("got %q, want %q", string(out), tt.expected)
			}
		})
	}
}

func TestCollapseDuplicatesInPlace(t *testing.T) {
	tests := []struct {
		name      string
		threshold int
		input     string
		expected  string
	}{
		{
			name:      "warning storm collapsed",
			threshold: 3,
			input: "ts-jest (WARN): Using hybrid module\n" +
				"ts-jest (WARN): Using hybrid module\n" +
				"ts-jest (WARN): Using hybrid module\n" +
				"ts-jest (WARN): Using hybrid module\n" +
				"ts-jest (WARN): Using hybrid module\n" +
				"Next step executed.\n",
			expected: "ts-jest (WARN): Using hybrid module\n" +
				"  [... identical line repeated 4 times ...]\n" +
				"Next step executed.\n",
		},
		{
			name:      "under threshold not collapsed",
			threshold: 3,
			input:     "warn 1\nwarn 1\nwarn 2\n",
			expected:  "warn 1\nwarn 1\nwarn 2\n",
		},
		{
			name:      "empty lines not counted as warning storm",
			threshold: 3,
			input:     "\n\n\n\ncode\n",
			expected:  "\n\n\n\ncode\n", // Whitespace normalizer handles empty lines
		},
		{
			name:      "ascii box rows not collapsed",
			threshold: 3,
			input:     "+---------------+\n|               |\n|               |\n|               |\n|               |\n+---------------+\n",
			expected:  "+---------------+\n|               |\n|               |\n|               |\n|               |\n+---------------+\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			out := CollapseDuplicatesInPlace(buf, tt.threshold)
			if string(out) != tt.expected {
				t.Errorf("got %q, want %q", string(out), tt.expected)
			}
		})
	}
}

func TestStripToolBoilerplateInPlace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strip apply diff notice",
			input:    "<notice>Making multiple related changes in a single apply_diff is more efficient</notice>\nFile updated successfully.\n",
			expected: "File updated successfully.\n",
		},
		{
			name:     "strip notice without closing tag",
			input:    "<notice>Making multiple related changes in a single apply_diff is more efficient\nAll 3 hunks applied.\n",
			expected: "All 3 hunks applied.\n",
		},
		{
			name:     "strip node trace-warnings boilerplate",
			input:    "Warning: experimental feature\n(Use `node --trace-warnings ...` to show where the warning was created)\nApp running.\n",
			expected: "Warning: experimental feature\nApp running.\n",
		},
		{
			name:     "compact error_details diff match dump",
			input: "<error_details>\nNo sufficiently similar match found (99% similar, needs 100%)\nDebug Info:\n- Similarity Score: 99%\n- Required Threshold: 100%\nSearch Content:\nline 1\nline 2\nBest Match Found:\nline 1\nline 2\n</error_details>\n",
			expected: "<error_details>\nNo sufficiently similar match found (99% similar, needs 100%)\nDebug Info:\n- Similarity Score: 99%\n- Required Threshold: 100%\n</error_details>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			out := StripToolBoilerplateInPlace(buf)
			if string(out) != tt.expected {
				t.Errorf("got %q, want %q", string(out), tt.expected)
			}
		})
	}
}

func TestNormalizeWhitespaceInPlace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "collapse 4 blank lines to 1",
			input:    "start\n\n\n\n\nend\n",
			expected: "start\n\nend\n",
		},
		{
			name:     "strip trailing whitespace on lines",
			input:    "line 1   \t\nline 2  \nline 3\n",
			expected: "line 1\nline 2\nline 3\n",
		},
		{
			name:     "strip excessive trailing blank lines at end",
			input:    "line 1\nline 2\n\n\n\n",
			expected: "line 1\nline 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			out := NormalizeWhitespaceInPlace(buf)
			if string(out) != tt.expected {
				t.Errorf("got %q, want %q", string(out), tt.expected)
			}
		})
	}
}

func TestPipeline_SequentialCompaction(t *testing.T) {
	cfg := DefaultConfig()
	pipeline := NewPipeline(cfg)

	input := "\x1b[31mts-jest (WARN)\x1b[0m\rts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module   \t\n\n\n\n" +
		"<notice>Making multiple related changes in a single apply_diff is more efficient</notice>\n" +
		"Tests passed.\n\n\n"

	buf := []byte(input)
	res := pipeline.Process(buf, CategoryGeneric)

	if res.Bypassed {
		t.Fatalf("expected generic category not to be bypassed")
	}

	outStr := string(buf[:res.ReducedBytes])
	if strings.Contains(outStr, "\x1b[") {
		t.Errorf("output still contains ANSI escape sequences")
	}
	if strings.Contains(outStr, "\r") {
		t.Errorf("output still contains carriage returns")
	}
	if strings.Contains(outStr, "<notice>") {
		t.Errorf("output still contains boilerplate notice")
	}
	if !strings.Contains(outStr, "repeated 3 times") {
		t.Errorf("output missing duplicate collapse marker, got:\n%s", outStr)
	}
	if strings.Contains(outStr, "\n\n\n") {
		t.Errorf("output contains uncollapsed blank lines")
	}
	if res.ReducedBytes >= res.OriginalBytes {
		t.Errorf("expected compaction: original %d, reduced %d", res.OriginalBytes, res.ReducedBytes)
	}
}

func TestDualLaneImmunity(t *testing.T) {
	cfg := DefaultConfig()
	pipeline := NewPipeline(cfg)

	// 1. FileRead immunity test on actual rules.go content from fixture #13
	testdataDir := filepath.Join(".", "testdata")
	groundTruthPath := filepath.Join(testdataDir, "rtk_ground_truth.json")

	data, err := os.ReadFile(groundTruthPath)
	if err == nil {
		var fixtures []struct {
			Category string `json:"category"`
			RawInput string `json:"raw_input"`
		}
		if err := json.Unmarshal(data, &fixtures); err == nil {
			for _, fix := range fixtures {
				if fix.Category == "source_code_read" {
					orig := []byte(fix.RawInput)
					copyBuf := append([]byte(nil), orig...)

					res := pipeline.Process(copyBuf, CategoryFileRead)
					if !res.Bypassed {
						t.Errorf("expected source_code_read to be bypassed")
					}
					if !bytes.Equal(copyBuf[:res.ReducedBytes], orig) {
						t.Fatalf("source code read was mutated! Got %d bytes, want %d bytes", res.ReducedBytes, len(orig))
					}
				}
			}
		}
	}

	// 2. FileWrite immunity test
	writePayload := []byte("write_to_file\n```go\nfunc hello() {}\n```")
	writeCopy := append([]byte(nil), writePayload...)
	resWrite := pipeline.Process(writeCopy, CategoryFileWrite)
	if !resWrite.Bypassed {
		t.Errorf("expected CategoryFileWrite to be bypassed")
	}
	if !bytes.Equal(writeCopy[:resWrite.ReducedBytes], writePayload) {
		t.Errorf("file write payload was mutated")
	}
}

func TestIdempotency(t *testing.T) {
	cfg := DefaultConfig()
	pipeline := NewPipeline(cfg)

	sample := []byte("\x1b[32mPASS\x1b[0m\n\n\n\n" +
		"ts-jest[config] (WARN): Using hybrid module kind (Node16/18/Next)\n" +
		"ts-jest[config] (WARN): Using hybrid module kind (Node16/18/Next)\n" +
		"ts-jest[config] (WARN): Using hybrid module kind (Node16/18/Next)\n" +
		"ts-jest[config] (WARN): Using hybrid module kind (Node16/18/Next)\n" +
		"Done.\n\n\n")
	firstPass := append([]byte(nil), sample...)
	res1 := pipeline.Process(firstPass, CategoryGeneric)
	pass1Output := append([]byte(nil), firstPass[:res1.ReducedBytes]...)

	// Second pass over the already compacted output
	secondPass := append([]byte(nil), pass1Output...)
	res2 := pipeline.Process(secondPass, CategoryGeneric)
	pass2Output := secondPass[:res2.ReducedBytes]

	if !bytes.Equal(pass1Output, pass2Output) {
		t.Errorf("NTS is not idempotent!\nPass 1 (%d bytes):\n%s\nPass 2 (%d bytes):\n%s",
			len(pass1Output), string(pass1Output), len(pass2Output), string(pass2Output))
	}
}

func TestTransformer_MultiProtocolPayloads(t *testing.T) {
	cfg := DefaultConfig()
	transformer := NewTransformer(cfg)

	// 1. Anthropic tool_result message
	anthropicReq := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":      "tool_result",
						"tool_name": "execute_command",
						"content":   "\x1b[31mError\x1b[0m\n\n\n\nDone\n",
					},
					map[string]interface{}{
						"type":          "tool_result",
						"tool_name":     "read_file",
						"content":       "package main\n\nfunc main() {}\n",
						"cache_control": map[string]string{"type": "ephemeral"},
					},
				},
			},
		},
	}

	body, _ := json.Marshal(anthropicReq)
	resBytes, result, err := transformer.TransformAnthropic(body)
	if err != nil {
		t.Fatalf("TransformAnthropic failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(resBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal transformed JSON: %v", err)
	}

	msgs := parsed["messages"].([]interface{})
	parts := msgs[0].(map[string]interface{})["content"].([]interface{})

	execContent := parts[0].(map[string]interface{})["content"].(string)
	if strings.Contains(execContent, "\x1b[31m") {
		t.Errorf("ANSI not stripped from Anthropic tool_result")
	}

	readContent := parts[1].(map[string]interface{})["content"].(string)
	if readContent != "package main\n\nfunc main() {}\n" {
		t.Errorf("read_file content mutated in Anthropic payload")
	}

	if result.TokensSaved <= 0 {
		t.Errorf("expected positive tokens saved, got %d", result.TokensSaved)
	}

	// 2. OpenAI role: "tool" message
	openAIReq := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "tool",
				"name":    "bash",
				"content": "\x1b[32mOK\x1b[0m\n\n\n\nFinished\n",
			},
		},
	}

	openAIBody, _ := json.Marshal(openAIReq)
	resOAI, _, err := transformer.TransformOpenAI(openAIBody)
	if err != nil {
		t.Fatalf("TransformOpenAI failed: %v", err)
	}

	var parsedOAI map[string]interface{}
	if err := json.Unmarshal(resOAI, &parsedOAI); err != nil {
		t.Fatalf("failed to unmarshal transformed OpenAI JSON: %v", err)
	}

	oaiMsgs := parsedOAI["messages"].([]interface{})
	oaiContent := oaiMsgs[0].(map[string]interface{})["content"].(string)
	if strings.Contains(oaiContent, "\x1b[32m") {
		t.Errorf("ANSI not stripped from OpenAI tool message")
	}
}

func TestEdgeCasesAndBranchCoverage(t *testing.T) {
	// 1. ANSI edge cases: OSC with ST terminator, charset selector, truncated escape at EOF
	t.Run("ANSI edge cases", func(t *testing.T) {
		input := "\x1b]8;;http://example.com\x1b\\link\x1b]8;;\x1b\\\x1b(Bcharset\x1b"
		out := StripANSIInPlace([]byte(input))
		if string(out) != "linkcharset" {
			t.Errorf("got %q, want %q", string(out), "linkcharset")
		}
	})

	// 2. Dedup edge cases: threshold <= 1, empty buffer
	t.Run("Dedup edge cases", func(t *testing.T) {
		empty := CollapseDuplicatesInPlace(nil, 3)
		if len(empty) != 0 {
			t.Errorf("expected empty slice")
		}
		lowThresh := CollapseDuplicatesInPlace([]byte("test\n"), 1)
		if string(lowThresh) != "test\n" {
			t.Errorf("got %q, want test\\n", string(lowThresh))
		}
	})

	// 3. Pipeline edge cases: DedupThreshold <= 0, Config() getter, disabled, empty
	t.Run("Pipeline edge cases", func(t *testing.T) {
		cfg := Config{
			Enabled:        false,
			DedupThreshold: -1,
		}
		p := NewPipeline(cfg)
		if p.Config().DedupThreshold != 3 {
			t.Errorf("expected DedupThreshold defaulted to 3")
		}

		resDisabled := p.Process([]byte("test"), CategoryGeneric)
		if !resDisabled.Bypassed || resDisabled.BypassReason != "disabled_or_empty" {
			t.Errorf("expected disabled bypass")
		}

		p.config.Enabled = true
		resEmpty := p.Process(nil, CategoryGeneric)
		if !resEmpty.Bypassed || resEmpty.BypassReason != "disabled_or_empty" {
			t.Errorf("expected empty bypass")
		}
	})

	// 4. Transformer edge cases: Pipeline() getter, unmarshal errors, missing fields, write categorization
	t.Run("Transformer edge cases", func(t *testing.T) {
		cfg := DefaultConfig()
		tr := NewTransformer(cfg)
		if tr.Pipeline() == nil {
			t.Errorf("expected non-nil Pipeline")
		}

		// Invalid JSON
		_, res1, err1 := tr.TransformAnthropic([]byte("invalid json"))
		if err1 == nil || !res1.Bypassed {
			t.Errorf("expected error on invalid JSON Anthropic")
		}
		_, res2, err2 := tr.TransformOpenAI([]byte("invalid json"))
		if err2 == nil || !res2.Bypassed {
			t.Errorf("expected error on invalid JSON OpenAI")
		}

		// Missing messages
		_, res3, _ := tr.TransformAnthropic([]byte("{}"))
		if !res3.Bypassed || res3.BypassReason != "no_messages" {
			t.Errorf("expected no_messages bypass")
		}
		_, res4, _ := tr.TransformOpenAI([]byte("{}"))
		if !res4.Bypassed || res4.BypassReason != "no_messages" {
			t.Errorf("expected no_messages bypass")
		}

		// Invalid messages type (not array)
		_, res5, _ := tr.TransformAnthropic([]byte(`{"messages": "not an array"}`))
		if !res5.Bypassed || res5.BypassReason != "invalid_messages" {
			t.Errorf("expected invalid_messages bypass")
		}
		_, res6, _ := tr.TransformOpenAI([]byte(`{"messages": "not an array"}`))
		if !res6.Bypassed || res6.BypassReason != "invalid_messages" {
			t.Errorf("expected invalid_messages bypass")
		}

		// Tool categories: write, replace, patch, read, view, cat, other
		cats := []struct {
			name string
			want ToolCategory
		}{
			{"write_to_file", CategoryFileWrite},
			{"replace_file_content", CategoryFileWrite},
			{"apply_patch", CategoryFileWrite},
			{"view_file", CategoryFileRead},
			{"cat", CategoryFileRead},
			{"read_file", CategoryFileRead},
			{"run_command", CategoryGeneric},
		}
		for _, tc := range cats {
			got := categorizeToolName(tc.name)
			if got != tc.want {
				t.Errorf("categorizeToolName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		}

		// Unmodified payloads
		cleanAnthropic, _, _ := tr.TransformAnthropic([]byte(`{"messages": [{"role": "user", "content": [{"type": "text", "text": "hello"}]}]}`))
		if !bytes.Contains(cleanAnthropic, []byte("hello")) {
			t.Errorf("expected clean Anthropic payload untouched")
		}
		cleanOpenAI, _, _ := tr.TransformOpenAI([]byte(`{"messages": [{"role": "user", "content": "hello"}]}`))
		if !bytes.Contains(cleanOpenAI, []byte("hello")) {
			t.Errorf("expected clean OpenAI payload untouched")
		}

		// Tool ID indexing and malformed message edge cases for Anthropic
		anthropicWithUse := map[string]interface{}{
			"messages": []interface{}{
				"invalid_item",
				map[string]interface{}{"invalid": "no content"},
				map[string]interface{}{"content": "string not list"},
				map[string]interface{}{
					"role": "assistant",
					"content": []interface{}{
						"not_a_map",
						map[string]interface{}{"type": "text", "text": "let me run that"},
						map[string]interface{}{"type": "tool_use", "id": "tu_1", "name": "execute_command"},
						map[string]interface{}{"type": "tool_use", "id": "tu_read", "name": "read_file"},
					},
				},
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{
							"type":        "tool_result",
							"tool_use_id": "tu_1",
							"content":     "\x1b[31mError\x1b[0m\n",
						},
						map[string]interface{}{
							"type":        "tool_result",
							"tool_use_id": "tu_read",
							"content":     "package main\n\x1b[31m",
						},
					},
				},
			},
		}
		rawAnth, _ := json.Marshal(anthropicWithUse)
		resAnth, _, _ := tr.TransformAnthropic(rawAnth)
		if !strings.Contains(string(resAnth), "Error") || strings.Contains(string(resAnth), "\x1b[31m") {
			t.Errorf("expected transformed tool_result in Anthropic with tool_use indexing")
		}

		// Tool ID indexing and malformed message edge cases for OpenAI
		openAIWithCalls := map[string]interface{}{
			"messages": []interface{}{
				"invalid_item",
				map[string]interface{}{
					"role": "assistant",
					"tool_calls": []interface{}{
						"not_a_map",
						map[string]interface{}{
							"id":   "tc_1",
							"type": "function",
							"function": map[string]interface{}{
								"name": "execute_command",
							},
						},
						map[string]interface{}{
							"id":   "tc_read",
							"type": "function",
							"function": map[string]interface{}{
								"name": "read_file",
							},
						},
					},
				},
				map[string]interface{}{
					"role":         "tool",
					"tool_call_id": "tc_1",
					"content":      "\x1b[31mError\x1b[0m\n",
				},
				map[string]interface{}{
					"role":         "tool",
					"tool_call_id": "tc_read",
					"content":      "package main\n\x1b[31m",
				},
			},
		}
		rawOAI, _ := json.Marshal(openAIWithCalls)
		resOAI2, _, _ := tr.TransformOpenAI(rawOAI)
		if !strings.Contains(string(resOAI2), "Error") || strings.Contains(string(resOAI2), "\x1b[31m") {
			t.Errorf("expected transformed tool role in OpenAI with tool_calls indexing")
		}
	})
}

func TestTransformer_ClineAndZooPayloads(t *testing.T) {
	cfg := DefaultConfig()
	tr := NewTransformer(cfg)

	t.Run("Cline inner JSON tool results and editor immunity", func(t *testing.T) {
		clineReq := map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"role": "assistant",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"id":   "call_cmd_1",
							"type": "function",
							"function": map[string]interface{}{
								"name": "run_commands",
							},
						},
						map[string]interface{}{
							"id":   "call_edit_1",
							"type": "function",
							"function": map[string]interface{}{
								"name": "editor",
							},
						},
					},
				},
				map[string]interface{}{
					"role":         "tool",
					"tool_call_id": "call_cmd_1",
					"content":      `[{"query":"go test -v ./...","result":"[Command exited with code 1]\nFAIL\n:\\Program Files\\PowerShell\\7\\pwsh.exe\u001b\\"}]`,
				},
				map[string]interface{}{
					"role":         "tool",
					"tool_call_id": "call_edit_1",
					"content":      "{\"query\":\"edit:foo.go\",\"result\":\"Edited foo.go\\n--- diff\\n-old\\n+new\\n\",\"success\":true}",
				},
			},
		}

		rawBody, err := json.Marshal(clineReq)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		resBody, res, err := tr.TransformOpenAI(rawBody)
		if err != nil {
			t.Fatalf("TransformOpenAI failed: %v", err)
		}

		if res.Bypassed {
			t.Fatalf("expected not bypassed, got %s", res.BypassReason)
		}
		if res.TokensSaved <= 0 {
			t.Errorf("expected positive tokens saved from run_commands, got %d", res.TokensSaved)
		}

		// Unmarshal and inspect
		var parsed map[string]interface{}
		if err := json.Unmarshal(resBody, &parsed); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		msgs := parsed["messages"].([]interface{})
		cmdMsg := msgs[1].(map[string]interface{})
		cmdContent := cmdMsg["content"].(string)

		if strings.Contains(cmdContent, `\u001b`) || strings.Contains(cmdContent, "\x1b") {
			t.Errorf("ANSI escape not stripped from Cline inner JSON result: %s", cmdContent)
		}

		editMsg := msgs[2].(map[string]interface{})
		editContent := editMsg["content"].(string)
		if !strings.Contains(editContent, "Edited foo.go") {
			t.Errorf("editor content corrupted: %s", editContent)
		}
	})

	t.Run("Zoo Code Anthropic-in-OpenAI content blocks", func(t *testing.T) {
		zooReq := map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"role": "assistant",
					"content": []interface{}{
						map[string]interface{}{
							"type": "tool_use",
							"id":   "call_zoo_exec",
							"name": "execute_command",
						},
						map[string]interface{}{
							"type": "tool_use",
							"id":   "call_zoo_diff",
							"name": "apply_diff",
						},
					},
				},
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{
							"type":        "tool_result",
							"tool_use_id": "call_zoo_exec",
							"content": []interface{}{
								map[string]interface{}{
									"type": "text",
									"text": "\x1b[31mFAIL\x1b[0m\n\n\n\nDone\n",
								},
							},
						},
						map[string]interface{}{
							"type":        "tool_result",
							"tool_use_id": "call_zoo_diff",
							"content":     `{"path":"main.go","operation":"modified"}`,
						},
					},
				},
			},
		}

		rawBody, _ := json.Marshal(zooReq)
		resBody, res, err := tr.TransformOpenAI(rawBody)
		if err != nil {
			t.Fatalf("TransformOpenAI failed: %v", err)
		}

		if res.Bypassed {
			t.Fatalf("expected not bypassed, got %s", res.BypassReason)
		}
		if res.TokensSaved <= 0 {
			t.Errorf("expected positive tokens saved from Zoo execute_command, got %d", res.TokensSaved)
		}

		var parsed map[string]interface{}
		_ = json.Unmarshal(resBody, &parsed)
		userMsg := parsed["messages"].([]interface{})[1].(map[string]interface{})
		parts := userMsg["content"].([]interface{})
		execPart := parts[0].(map[string]interface{})
		subList := execPart["content"].([]interface{})
		textVal := subList[0].(map[string]interface{})["text"].(string)

		if strings.Contains(textVal, "\x1b[31m") {
			t.Errorf("ANSI escape not stripped from Zoo content block: %s", textVal)
		}
	})
}


func BenchmarkNTS_InPlaceCompaction(t *testing.B) {
	cfg := DefaultConfig()
	pipeline := NewPipeline(cfg)

	raw := []byte("\x1b[31mts-jest (WARN)\x1b[0m\rts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module\n\n\n\n" +
		"<notice>Making multiple related changes in a single apply_diff is more efficient</notice>\n" +
		"Done.\n\n\n")

	t.ResetTimer()
	t.ReportAllocs()

	scratch := make([]byte, len(raw))
	for i := 0; i < t.N; i++ {
		copy(scratch, raw)
		_ = pipeline.Process(scratch, CategoryGeneric)
	}
}

