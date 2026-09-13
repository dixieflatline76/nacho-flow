package nts

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/data"
	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
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
		{
			name:     "crlf lines with trailing spaces and tabs",
			input:    "line 1   \t\r\nline 2  \r\nline 3\r\n",
			expected: "line 1\nline 2\nline 3\n",
		},
		{
			name:     "collapse consecutive crlf blank lines",
			input:    "start\r\n\r\n\r\n\r\nend\r\n",
			expected: "start\n\nend\n",
		},
		{
			name:     "strip excessive trailing crlf blank lines at end",
			input:    "line 1\r\nline 2\r\n\r\n\r\n\r\n",
			expected: "line 1\nline 2\n",
		},
		{
			name:     "pure whitespace with crlf and spaces",
			input:    "   \r\n\t\r\n   \r\n",
			expected: "\n",
		},
		{
			name:     "pure newlines only",
			input:    "\n\n\n\n",
			expected: "\n",
		},
		{
			name:     "empty buffer",
			input:    "",
			expected: "",
		},
		{
			name:     "single line without trailing newline",
			input:    "no newline   \t",
			expected: "no newline",
		},
		{
			name:     "multiple lines without trailing newline at eof",
			input:    "line 1   \nno newline  ",
			expected: "line 1\nno newline",
		},
		{
			name:     "leading blank lines collapsed",
			input:    "\n\n\n\nstart\n",
			expected: "\nstart\n",
		},
		{
			name:     "clean text preserved without modification",
			input:    "clean line 1\nclean line 2\n",
			expected: "clean line 1\nclean line 2\n",
		},
		{
			name:     "trailing carriage return only without lf at eof",
			input:    "trailing cr\r",
			expected: "trailing cr",
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

func BenchmarkNormalizeWhitespaceInPlace_ZeroAlloc(b *testing.B) {
	input := []byte("func Process(ctx context.Context) error {   \t\r\n\r\n\r\n\treturn nil\r\n}\r\n\r\n\r\n")
	buf := make([]byte, len(input))

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, input)
		_ = NormalizeWhitespaceInPlace(buf)
	}
}

func BenchmarkStripANSIInPlace_ZeroAlloc(b *testing.B) {
	input := []byte("\x1b[31;1mError:\x1b[0m Failed to compile module '\x1b[33mmain.ts\x1b[0m' at line 42:10\nNormal text continues here.")
	buf := make([]byte, len(input))

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, input)
		_ = StripANSIInPlace(buf)
	}
}

func BenchmarkResolveCarriageReturnsInPlace_ZeroAlloc(b *testing.B) {
	input := []byte("Downloading:  10%\rDownloading:  50%\rDownloading: 100%\nCompleted successfully.\r\nNext line.\n")
	buf := make([]byte, len(input))

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, input)
		_ = ResolveCarriageReturnsInPlace(buf)
	}
}

func BenchmarkStripToolBoilerplateInPlace_ZeroAlloc(b *testing.B) {
	input := []byte("<notice>Making multiple related changes in a single apply_diff is more efficient</notice>\n(Use `node --trace-warnings ...` to show where the warning was created)\n<error_details>\nSearch Content:\nline 1\nline 2\n</error_details>\nValid output line.")
	buf := make([]byte, len(input))

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, input)
		_ = StripToolBoilerplateInPlace(buf)
	}
}

func BenchmarkCollapseDuplicatesInPlace_ZeroAlloc(b *testing.B) {
	input := []byte("ts-jest (WARN): Using hybrid module\nts-jest (WARN): Using hybrid module\nts-jest (WARN): Using hybrid module\nts-jest (WARN): Using hybrid module\nUnique line.\n")
	buf := make([]byte, len(input))

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, input)
		_ = CollapseDuplicatesInPlace(buf, 3)
	}
}

func BenchmarkPipeline_Process_ZeroAlloc(b *testing.B) {
	pipeline := NewPipeline(DefaultConfig())
	input := []byte("\x1b[31mts-jest (WARN)\x1b[0m\rts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module\n" +
		"ts-jest (WARN): Using hybrid module   \t\n\n\n\n" +
		"<notice>Making multiple related changes in a single apply_diff is more efficient</notice>\n" +
		"Tests passed.\n\n\n")
	buf := make([]byte, len(input))

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, input)
		_ = pipeline.Process(buf, CategoryGeneric)
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

func TestPipeline_RealToolSamplesDataset(t *testing.T) {
	samplesPath := filepath.Join(".", "testdata", "real_tool_samples.json")
	data, err := os.ReadFile(samplesPath)
	if err != nil {
		t.Fatalf("failed to read real_tool_samples.json: %v", err)
	}

	var samples []struct {
		Source   string `json:"source"`
		Category string `json:"category"`
		Output   string `json:"output"`
		Length   int    `json:"length"`
		Lines    int    `json:"lines"`
	}
	if err := json.Unmarshal(data, &samples); err != nil {
		t.Fatalf("failed to unmarshal real_tool_samples.json: %v", err)
	}

	if len(samples) < 1000 {
		t.Fatalf("expected at least 1000 real-world tool samples, got %d", len(samples))
	}

	cfg := DefaultConfig()
	pipeline := NewPipeline(cfg)

	sourceCodeReadsChecked := 0

	for i, s := range samples {
		var cat ToolCategory
		switch s.Category {
		case "source_code_read":
			cat = CategoryFileRead
		case "file_write":
			cat = CategoryFileWrite
		default:
			cat = CategoryGeneric
		}

		raw := []byte(s.Output)
		origLen := len(raw)

		// Assert 0 panics
		buf := append([]byte(nil), raw...)
		res := pipeline.Process(buf, cat)

		// Assert invariant w <= r
		if res.ReducedBytes > origLen {
			t.Fatalf("sample #%d (%s): invariant w <= r violated! reduced %d > original %d",
				i, s.Category, res.ReducedBytes, origLen)
		}

		pass1Output := append([]byte(nil), buf[:res.ReducedBytes]...)

		// Assert Dual-Lane Immunity: source_code_read must be 100% unmutated
		if cat == CategoryFileRead {
			sourceCodeReadsChecked++
			if !res.Bypassed {
				t.Fatalf("sample #%d (%s): expected source_code_read to be bypassed", i, s.Category)
			}
			if !bytes.Equal(pass1Output, raw) {
				t.Fatalf("sample #%d (%s): source_code_read was mutated! Got %d bytes, want %d bytes",
					i, s.Category, len(pass1Output), len(raw))
			}
		}

		// Assert Strict Idempotency: Process(Process(sample)) == Process(sample)
		buf2 := append([]byte(nil), pass1Output...)
		res2 := pipeline.Process(buf2, cat)
		pass2Output := buf2[:res2.ReducedBytes]

		if !bytes.Equal(pass1Output, pass2Output) {
			t.Fatalf("sample #%d (%s): idempotency violation!\nPass 1 (%d bytes)\nPass 2 (%d bytes)",
				i, s.Category, len(pass1Output), len(pass2Output))
		}
	}

	if sourceCodeReadsChecked < 50 {
		t.Errorf("expected at least 50 source code reads tested, got %d", sourceCodeReadsChecked)
	}
}

func TestPipeline_RTKDefectImmunity(t *testing.T) {
	groundTruthPath := filepath.Join(".", "testdata", "rtk_ground_truth.json")
	data, err := os.ReadFile(groundTruthPath)
	if err != nil {
		t.Fatalf("failed to read rtk_ground_truth.json: %v", err)
	}

	var fixtures []struct {
		ID             string `json:"id"`
		Category       string `json:"category"`
		RawInput       string `json:"raw_input"`
		ExpectedOutput string `json:"expected_output"`
		RawLen         int    `json:"raw_len"`
		ReducedLen     int    `json:"reduced_len"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("failed to unmarshal rtk_ground_truth.json: %v", err)
	}

	cfg := DefaultConfig()
	pipeline := NewPipeline(cfg)

	for _, fix := range fixtures {
		var cat ToolCategory
		if fix.Category == "source_code_read" {
			cat = CategoryFileRead
		} else {
			cat = CategoryGeneric
		}

		raw := []byte(fix.RawInput)
		res := pipeline.Process(append([]byte(nil), raw...), cat)

		// Crucial negative-control assertion:
		// RTK's flawed algorithm truncated source code reads (fixtures 13, 14, 15) from 6.5KB to 71 bytes!
		// Nacho Flow MUST NOT reproduce this defect: source code reads MUST be 100% preserved.
		if fix.Category == "source_code_read" {
			if !res.Bypassed {
				t.Errorf("[%s] expected source_code_read to be bypassed by Dual-Lane Immunity", fix.ID)
			}
			if res.ReducedBytes != len(raw) {
				t.Fatalf("[%s] DEFECT DETECTED: Nacho Flow reduced source code from %d to %d bytes (RTK flawed target was %d)",
					fix.ID, len(raw), res.ReducedBytes, fix.ReducedLen)
			}
		}
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

	// Subtest: CRLF multi-line idempotency
	sampleCRLF := []byte("line 1   \t\r\n\r\n\r\n\r\nline 2  \r\nDone.\r\n\r\n\r\n")
	firstPassCRLF := append([]byte(nil), sampleCRLF...)
	resCRLF1 := pipeline.Process(firstPassCRLF, CategoryGeneric)
	pass1CRLFOutput := append([]byte(nil), firstPassCRLF[:resCRLF1.ReducedBytes]...)

	secondPassCRLF := append([]byte(nil), pass1CRLFOutput...)
	resCRLF2 := pipeline.Process(secondPassCRLF, CategoryGeneric)
	pass2CRLFOutput := secondPassCRLF[:resCRLF2.ReducedBytes]

	if !bytes.Equal(pass1CRLFOutput, pass2CRLFOutput) {
		t.Errorf("NTS CRLF is not idempotent!\nPass 1 (%d bytes):\n%s\nPass 2 (%d bytes):\n%s",
			len(pass1CRLFOutput), string(pass1CRLFOutput), len(pass2CRLFOutput), string(pass2CRLFOutput))
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

func TestCollapseDuplicatesInPlace_NoTrailingNewlinePanic(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		threshold int
		expected  string
	}{
		{
			name:      "Cline 3x False panic regression - no trailing newline",
			input:     "False\nFalse\nFalse",
			threshold: 3,
			expected:  "False\nFalse\nFalse",
		},
		{
			name:      "2x test without trailing newline",
			input:     "test\ntest",
			threshold: 3,
			expected:  "test\ntest",
		},
		{
			name:      "Collapsible repeat without trailing newline",
			input:     "This is a very long line that exceeds the collapse notice length\nThis is a very long line that exceeds the collapse notice length\nThis is a very long line that exceeds the collapse notice length",
			threshold: 3,
			expected:  "This is a very long line that exceeds the collapse notice length\n  [... identical line repeated 2 times ...]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			res := CollapseDuplicatesInPlace(buf, tt.threshold)
			if string(res) != tt.expected {
				t.Fatalf("expected:\n%q\ngot:\n%q", tt.expected, string(res))
			}
		})
	}
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
	for t.Loop() {
		copy(scratch, raw)
		_ = pipeline.Process(scratch, CategoryGeneric)
	}
}

func TestTransformer_ToolImmunity_CatalogAlignment(t *testing.T) {
	cfg := DefaultConfig()
	tr := NewTransformer(cfg)

	// Catalog write tools: editor (Cline/Zoo), insert_code_block (Zoo), reapply (Cursor), create_file (Standard/Cursor/Zoo)
	catalogWriteTools := []string{"editor", "insert_code_block", "reapply", "create_file", "write_to_file"}
	for _, tool := range catalogWriteTools {
		cat := categorizeToolName(tool)
		if cat != CategoryFileWrite {
			t.Errorf("expected tool %s to be categorized as CategoryFileWrite, got %s", tool, cat)
		}

		// Verify dual-lane write immunity at pipeline level
		pipelineRes := tr.Pipeline().Process([]byte("+line1\n+line1\n+line1\n+line1\n"), cat)
		if !pipelineRes.Bypassed || pipelineRes.BypassReason != "category_file_write" {
			t.Errorf("expected pipeline bypass for %s with category_file_write, got %+v", tool, pipelineRes)
		}

		// Verify dual-lane write immunity end-to-end: repetitive diff lines must NOT be deduplicated
		req := map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"role":    "tool",
					"name":    tool,
					"content": "+line1\n+line1\n+line1\n+line1\n",
				},
			},
		}
		body, _ := json.Marshal(req)
		resBody, res, err := tr.TransformOpenAI(body)
		if err != nil {
			t.Fatalf("TransformOpenAI failed for %s: %v", tool, err)
		}
		if res.TokensSaved != 0 {
			t.Errorf("expected 0 tokens saved for bypassed write tool %s, got %d", tool, res.TokensSaved)
		}
		if string(resBody) != string(body) {
			t.Errorf("expected body to be unmodified for write tool %s", tool)
		}
	}

	// Catalog read tools: read_files (Cline), view (Anthropic), open_file (Standard), read_file (Standard)
	catalogReadTools := []string{"read_files", "view", "open_file", "read_file"}
	for _, tool := range catalogReadTools {
		cat := categorizeToolName(tool)
		if cat != CategoryFileRead {
			t.Errorf("expected tool %s to be categorized as CategoryFileRead, got %s", tool, cat)
		}
	}

	// Command / generic tools: bash, run_commands, execute_command
	genericTools := []string{"bash", "run_commands", "execute_command", "terminal"}
	for _, tool := range genericTools {
		cat := categorizeToolName(tool)
		if cat != CategoryGeneric {
			t.Errorf("expected tool %s to be categorized as CategoryGeneric, got %s", tool, cat)
		}
	}
}

func TestTransformer_AllAgentToolLanesContract(t *testing.T) {
	entries, err := fs.ReadDir(data.CatalogFS, "agents")
	if err != nil {
		t.Fatalf("failed to read agents dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "manifest.json" {
			continue
		}

		raw, err := fs.ReadFile(data.CatalogFS, "agents/"+entry.Name())
		if err != nil {
			t.Fatalf("failed to read agent profile %s: %v", entry.Name(), err)
		}

		var profile agentregistry.AgentProfile
		if err := json.Unmarshal(raw, &profile); err != nil {
			t.Fatalf("failed to parse agent profile %s: %v", entry.Name(), err)
		}

		t.Run(profile.ID+"/FileReadTools_Lane", func(t *testing.T) {
			for _, tool := range profile.FileReadTools {
				cat := categorizeToolName(tool)
				if cat != CategoryFileRead {
					t.Errorf("agent %s: tool %q must be categorized as CategoryFileRead, got %v", profile.ID, tool, cat)
				}
			}
		})

		t.Run(profile.ID+"/FileWriteTools_Lane", func(t *testing.T) {
			for _, tool := range profile.WriteTools {
				cat := categorizeToolName(tool)
				if cat != CategoryFileWrite {
					t.Errorf("agent %s: tool %q must be categorized as CategoryFileWrite, got %v", profile.ID, tool, cat)
				}
			}
		})
	}
}

func TestTransformer_SingleBlockFastPath(t *testing.T) {
	// Single block
	singleBlock := []interface{}{
		map[string]interface{}{
			"type": "text",
			"text": "error: cannot find module 'foo'",
		},
	}
	if str := extractContentString(singleBlock); str != "error: cannot find module 'foo'" {
		t.Errorf("expected single block extraction, got %q", str)
	}

	// Multi block fallback
	multiBlock := []interface{}{
		map[string]interface{}{
			"type": "text",
			"text": "part1: ",
		},
		map[string]interface{}{
			"type": "text",
			"text": "part2",
		},
	}
	if str := extractContentString(multiBlock); str != "part1: part2" {
		t.Errorf("expected multi block concatenation, got %q", str)
	}

	// Empty and non-text
	emptyBlock := []interface{}{}
	if str := extractContentString(emptyBlock); str != "" {
		t.Errorf("expected empty string for empty block, got %q", str)
	}
}

func TestTransformer_LazyIndexToolNames(t *testing.T) {
	cfg := DefaultConfig()
	tr := NewTransformer(cfg)

	// Pure chat payload without any tool calls
	chatReq := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "Hello world, what is Go?",
			},
			map[string]interface{}{
				"role":    "assistant",
				"content": "Go is an open-source programming language.",
			},
		},
	}
	body, _ := json.Marshal(chatReq)

	resBody, res, err := tr.TransformOpenAI(body)
	if err != nil {
		t.Fatalf("TransformOpenAI failed: %v", err)
	}
	if res.TokensSaved != 0 {
		t.Errorf("expected 0 tokens saved for chat payload, got %d", res.TokensSaved)
	}
	if string(resBody) != string(body) {
		t.Errorf("expected unmodified body for chat payload")
	}

	// Verify indexToolNames returns nil for messages with no tools
	msgs := chatReq["messages"].([]interface{})
	names := indexToolNames(msgs)
	if names != nil {
		t.Errorf("expected nil names map for chat-only messages, got %+v", names)
	}
}

func TestTransformer_StaleReadEviction_Parity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CompactStaleFileReads = true
	cfg.StaleReadDepth = 1
	tr := NewTransformer(cfg)

	// OpenAI Format
	openAIReq := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "tc1",
						"function": map[string]interface{}{
							"name":      "read_file",
							"arguments": `{"path":"main.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "tc1",
				"content":      "package main\n\nfunc main() {\n\tprintln(1)\n}\n",
			},
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "tc2",
						"function": map[string]interface{}{
							"name":      "write_to_file",
							"arguments": `{"path":"main.go"}`,
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "tc2",
				"content":      "File saved successfully",
			},
		},
	}
	openAIBody, _ := json.Marshal(openAIReq)
	_, resOAI, err := tr.TransformOpenAI(openAIBody)
	if err != nil {
		t.Fatalf("TransformOpenAI failed: %v", err)
	}

	// Anthropic Format with equivalent turns
	anthropicReq := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type":  "tool_use",
						"id":    "tc1",
						"name":  "read_file",
						"input": map[string]interface{}{"path": "main.go"},
					},
				},
			},
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":         "tool_result",
						"tool_use_id":  "tc1",
						"content":      "package main\n\nfunc main() {\n\tprintln(1)\n}\n",
					},
				},
			},
			map[string]interface{}{
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type":  "tool_use",
						"id":    "tc2",
						"name":  "write_to_file",
						"input": map[string]interface{}{"path": "main.go"},
					},
				},
			},
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": "tc2",
						"content":     "File saved successfully",
					},
				},
			},
		},
	}
	anthropicBody, _ := json.Marshal(anthropicReq)
	_, resAnthropic, err := tr.TransformAnthropic(anthropicBody)
	if err != nil {
		t.Fatalf("TransformAnthropic failed: %v", err)
	}

	if resOAI.OriginalBytes != resAnthropic.OriginalBytes {
		t.Errorf("OriginalBytes mismatch: OAI=%d, Anthropic=%d", resOAI.OriginalBytes, resAnthropic.OriginalBytes)
	}
	if resOAI.BytesSaved != resAnthropic.BytesSaved {
		t.Errorf("BytesSaved mismatch: OAI=%d, Anthropic=%d", resOAI.BytesSaved, resAnthropic.BytesSaved)
	}
	if resOAI.TokensSaved != resAnthropic.TokensSaved {
		t.Errorf("TokensSaved mismatch: OAI=%d, Anthropic=%d", resOAI.TokensSaved, resAnthropic.TokensSaved)
	}
}

func BenchmarkTransformer_TransformOpenAI_ToolOutput(b *testing.B) {
	cfg := DefaultConfig()
	tr := NewTransformer(cfg)

	req := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{
					map[string]interface{}{
						"id": "c1",
						"function": map[string]interface{}{
							"name": "bash",
						},
					},
				},
			},
			map[string]interface{}{
				"role":         "tool",
				"tool_call_id": "c1",
				"content":      "\x1b[31mFAIL: test_api.go\x1b[0m\rFAIL: test_api.go\nFAIL: test_api.go\nFAIL: test_api.go\n\n\n\nDone.\n",
			},
		},
	}
	raw, _ := json.Marshal(req)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = tr.TransformOpenAI(raw)
	}
}

func BenchmarkTransformer_TransformOpenAI_ChatOnly(b *testing.B) {
	cfg := DefaultConfig()
	tr := NewTransformer(cfg)

	req := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "Explain quicksort in simple terms.",
			},
			map[string]interface{}{
				"role":    "assistant",
				"content": "Quicksort is a divide-and-conquer algorithm.",
			},
		},
	}
	raw, _ := json.Marshal(req)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = tr.TransformOpenAI(raw)
	}
}


