// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package zeroalloc

import (
	"bytes"
	"testing"
)

func TestStripSubsliceInPlace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		target   string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			target:   "foo",
			expected: "",
		},
		{
			name:     "empty target",
			input:    "hello world",
			target:   "",
			expected: "hello world",
		},
		{
			name:     "no match",
			input:    "hello world",
			target:   "xyz",
			expected: "hello world",
		},
		{
			name:     "single match at start",
			input:    "foobar",
			target:   "foo",
			expected: "bar",
		},
		{
			name:     "single match at end",
			input:    "barfoo",
			target:   "foo",
			expected: "bar",
		},
		{
			name:     "multiple matches",
			input:    "foo hello foo world foo",
			target:   "foo",
			expected: " hello  world ",
		},
		{
			name:     "consecutive matches",
			input:    "foofoofoo",
			target:   "foo",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			res := StripSubsliceInPlace(buf, []byte(tt.target))
			if string(res) != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, string(res))
			}
		})
	}
}

func TestStripSubslicesInPlace(t *testing.T) {
	targets := [][]byte{
		[]byte("<|channel|>thought"),
		[]byte("<|channel>thought"),
		[]byte("<|channel>"),
		[]byte("<channel|>"),
		[]byte("<think>"),
		[]byte("</think>"),
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no triggers present",
			input:    "Hello world, I am thinking about Go.",
			expected: "Hello world, I am thinking about Go.",
		},
		{
			name:     "single token stripped",
			input:    "<|channel>thought miras.json is cool",
			expected: " miras.json is cool",
		},
		{
			name:     "triple consecutive tokens stripped",
			input:    "<|channel>thought<|channel>thought<|channel>thought日本語テキスト",
			expected: "日本語テキスト",
		},
		{
			name:     "interleaved tokens and text",
			input:    "start <think> reasoning </think> and <|channel> end",
			expected: "start  reasoning  and  end",
		},
		{
			name:     "prefix trigger but no match",
			input:    "<not_a_tag> keeps existing <think> removed </think>",
			expected: "<not_a_tag> keeps existing  removed ",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			res := StripSubslicesInPlace(buf, targets)
			if string(res) != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, string(res))
			}
		})
	}
}

func TestStripSubslicesInPlace_MultipleTriggers(t *testing.T) {
	targets := [][]byte{
		[]byte("<tag>"),
		[]byte("[tool]"),
		[]byte("{json}"),
	}

	// 1. Empty targets or empty input
	if string(StripSubslicesInPlace([]byte("hello"), nil)) != "hello" {
		t.Errorf("expected original on nil targets")
	}
	if string(StripSubslicesInPlace(nil, targets)) != "" {
		t.Errorf("expected empty on nil input")
	}

	// 2. No triggers present
	input := "No matches here."
	if string(StripSubslicesInPlace([]byte(input), targets)) != input {
		t.Errorf("expected unchanged when no triggers present")
	}

	// 3. Multi-trigger stripping and advancing
	multiInput := "Start <tag> middle [tool] and {json} end."
	expected := "Start  middle  and  end."
	res := string(StripSubslicesInPlace([]byte(multiInput), targets))
	if res != expected {
		t.Errorf("expected %q, got %q", expected, res)
	}

	// 4. Trigger byte present without target match
	unmatched := "[not a tool] <not a tag> {not json}"
	if string(StripSubslicesInPlace([]byte(unmatched), targets)) != unmatched {
		t.Errorf("expected %q, got %q", unmatched, string(StripSubslicesInPlace([]byte(unmatched), targets)))
	}
}

func TestFindTrailingPrefix(t *testing.T) {
	prefixes := [][]byte{
		[]byte("<|channel"),
		[]byte("<|channel>"),
		[]byte("<channel|"),
		[]byte("<think"),
	}

	tests := []struct {
		name      string
		input     string
		maxLen    int
		expected  int
		matchText string
	}{
		{
			name:     "no match",
			input:    "Hello world",
			maxLen:   24,
			expected: -1,
		},
		{
			name:     "empty input",
			input:    "",
			maxLen:   24,
			expected: -1,
		},
		{
			name:      "exact trailing prefix match",
			input:     "Here is some text ending in <|channel",
			maxLen:    24,
			expected:  28,
			matchText: "<|channel",
		},
		{
			name:     "prefix not at end",
			input:    "<|channel and then more text",
			maxLen:   24,
			expected: -1,
		},
		{
			name:      "shorter match at tail",
			input:     "abc<think",
			maxLen:    10,
			expected:  3,
			matchText: "<think",
		},
		{
			name:      "checkLen greater than n",
			input:     "<think",
			maxLen:    100,
			expected:  0,
			matchText: "<think",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			idx := FindTrailingPrefix(buf, tt.maxLen, prefixes)
			if idx != tt.expected {
				t.Fatalf("expected index %d, got %d", tt.expected, idx)
			}
			if idx != -1 && string(buf[idx:]) != tt.matchText {
				t.Fatalf("expected trailing text %q, got %q", tt.matchText, string(buf[idx:]))
			}
		})
	}

	if FindTrailingPrefix([]byte("test"), 10, nil) != -1 {
		t.Errorf("expected -1 for nil prefixes")
	}
}

func TestHasPrefixAny(t *testing.T) {
	prefixes := [][]byte{
		[]byte("<|channel"),
		[]byte("<think"),
	}

	if !HasPrefixAny([]byte("<|channel>thought"), prefixes) {
		t.Errorf("expected true for <|channel>thought")
	}
	if !HasPrefixAny([]byte("<think>here"), prefixes) {
		t.Errorf("expected true for <think>here")
	}
	if HasPrefixAny([]byte("hello <think>"), prefixes) {
		t.Errorf("expected false for hello <think>")
	}
}

func BenchmarkStripSubslicesInPlace_ZeroAlloc(b *testing.B) {
	targets := [][]byte{
		[]byte("<|channel|>thought"),
		[]byte("<|channel>thought"),
		[]byte("<|channel>"),
		[]byte("<channel|>"),
		[]byte("<think>"),
		[]byte("</think>"),
	}

	sample := []byte(`itivos.br.content.json
"title": "Instructional Content - Basic N-Queens Implementation Plan",
"description": "This tool will be used to store and retrieve the structured plan."
}
<|channel>thought miras.json
"title": "Interactive Experience Design",
}
<|channel>thought<|channel>thought<|channel>thoughtEnd of plan.`)

	scratch := make([]byte, len(sample))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		copy(scratch, sample)
		_ = StripSubslicesInPlace(scratch, targets)
	}
}

func TestReplaceSubslicesInPlace(t *testing.T) {
	targets := [][]byte{
		[]byte("\"insert_line\": \"None\""),
		[]byte("\"insert_line\":\"None\""),
		[]byte("\"insert_line\": \"null\""),
		[]byte("\"insert_line\":\"null\""),
		[]byte("\"old_text\": \"None\""),
	}
	replacements := [][]byte{
		[]byte("\"insert_line\": null"),
		[]byte("\"insert_line\":null"),
		[]byte("\"insert_line\": null"),
		[]byte("\"insert_line\":null"),
		[]byte("\"old_text\": \"\""),
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "no triggers present",
			input:    `{"path": "main.go", "content": "hello"}`,
			expected: `{"path": "main.go", "content": "hello"}`,
		},
		{
			name:     "trigger byte present but no match",
			input:    `{"path": "insert_line_test.go"}`,
			expected: `{"path": "insert_line_test.go"}`,
		},
		{
			name:     "single spaced match",
			input:    `{"insert_line": "None", "new_text": "package main"}`,
			expected: `{"insert_line": null, "new_text": "package main"}`,
		},
		{
			name:     "single compact match",
			input:    `{"insert_line":"None","new_text":"package main"}`,
			expected: `{"insert_line":null,"new_text":"package main"}`,
		},
		{
			name:     "string null replacement",
			input:    `{"insert_line": "null", "new_text": "test"}`,
			expected: `{"insert_line": null, "new_text": "test"}`,
		},
		{
			name:     "multiple matches in same payload",
			input:    `{"insert_line": "None", "old_text": "None"}`,
			expected: `{"insert_line": null, "old_text": ""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := []byte(tt.input)
			res := ReplaceSubslicesInPlace(buf, targets, replacements)
			if string(res) != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, string(res))
			}
		})
	}

	// Nil / length mismatch safety checks
	if len(ReplaceSubslicesInPlace(nil, targets, replacements)) != 0 {
		t.Errorf("expected empty for nil buffer")
	}
	if string(ReplaceSubslicesInPlace([]byte("test"), nil, nil)) != "test" {
		t.Errorf("expected unchanged for nil targets")
	}
	if string(ReplaceSubslicesInPlace([]byte("test"), targets, replacements[:1])) != "test" {
		t.Errorf("expected unchanged for mismatched targets/replacements")
	}

	// Mixed trigger bytes (singleTrigger == false)
	mixedTargets := [][]byte{
		[]byte("alpha"),
		[]byte("beta"),
	}
	mixedReplacements := [][]byte{
		[]byte("a"),
		[]byte("b"),
	}
	mixedBuf := []byte("first alpha then beta and end")
	mixedRes := ReplaceSubslicesInPlace(mixedBuf, mixedTargets, mixedReplacements)
	if string(mixedRes) != "first a then b and end" {
		t.Errorf("expected 'first a then b and end', got %q", string(mixedRes))
	}

	// Safety guard check: repLen > targetLen must not corrupt buffer
	invalidTargets := [][]byte{[]byte("short")}
	invalidReplacements := [][]byte{[]byte("much_longer_replacement")}
	safeBuf := []byte("prefix short suffix")
	res := ReplaceSubslicesInPlace(safeBuf, invalidTargets, invalidReplacements)
	// Guard skips replacing since it would violate w <= r
	if string(res) != "prefix short suffix" {
		t.Errorf("expected uncorrupted buffer when rep > target, got %q", string(res))
	}
}

func BenchmarkReplaceSubslicesInPlace_ZeroAlloc(b *testing.B) {
	targets := [][]byte{
		[]byte("\"insert_line\": \"None\""),
		[]byte("\"insert_line\":\"None\""),
		[]byte("\"insert_line\": \"null\""),
		[]byte("\"insert_line\":\"null\""),
	}
	replacements := [][]byte{
		[]byte("\"insert_line\": null"),
		[]byte("\"insert_line\":null"),
		[]byte("\"insert_line\": null"),
		[]byte("\"insert_line\":null"),
	}

	sample := []byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"editor","arguments":"{\"insert_line\": \"None\", \"new_text\": \"package main\\n\\nfunc main() {}\"}"}}]}}]}`)
	scratch := make([]byte, len(sample))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		copy(scratch, sample)
		_ = ReplaceSubslicesInPlace(scratch, targets, replacements)
	}
}

func TestContainsFoldASCII(t *testing.T) {
	tests := []struct {
		name     string
		haystack string
		needle   string
		expected bool
	}{
		{
			name:     "empty haystack and empty needle",
			haystack: "",
			needle:   "",
			expected: true,
		},
		{
			name:     "non-empty haystack and empty needle",
			haystack: "hello world",
			needle:   "",
			expected: true,
		},
		{
			name:     "empty haystack and non-empty needle",
			haystack: "",
			needle:   "a",
			expected: false,
		},
		{
			name:     "needle longer than haystack",
			haystack: "hi",
			needle:   "hello",
			expected: false,
		},
		{
			name:     "exact match same case",
			haystack: "are you satisfied",
			needle:   "are you satisfied",
			expected: true,
		},
		{
			name:     "exact match needle lower haystack upper",
			haystack: "ARE YOU SATISFIED",
			needle:   "are you satisfied",
			expected: true,
		},
		{
			name:     "exact match needle upper haystack lower",
			haystack: "please confirm",
			needle:   "PLEASE CONFIRM",
			expected: true,
		},
		{
			name:     "mixed case both",
			haystack: "ThIs Is A TeSt",
			needle:   "tHiS iS a tEsT",
			expected: true,
		},
		{
			name:     "prefix match",
			haystack: "Would you like a plan? Yes.",
			needle:   "would you like",
			expected: true,
		},
		{
			name:     "middle match",
			haystack: "Prefix text. Are you satisfied? Suffix.",
			needle:   "are you satisfied",
			expected: true,
		},
		{
			name:     "suffix match",
			haystack: "I am ready to implement",
			needle:   "ready to implement",
			expected: true,
		},
		{
			name:     "partial false start then true match",
			haystack: "whowhowould you like",
			needle:   "would you like",
			expected: true,
		},
		{
			name:     "partial match at end without full needle",
			haystack: "here is a question: would you",
			needle:   "would you like",
			expected: false,
		},
		{
			name:     "no match distinct characters",
			haystack: "completely different text",
			needle:   "are you satisfied",
			expected: false,
		},
		{
			name:     "utf8 multi-byte prefix and suffix with ascii match",
			haystack: "日本語 are you satisfied 日本語",
			needle:   "are you satisfied",
			expected: true,
		},
		{
			name:     "utf8 multi-byte exact match",
			haystack: "日本語テスト",
			needle:   "日本語",
			expected: true,
		},
		{
			name:     "nil slices",
			haystack: "",
			needle:   "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hBytes, nBytes []byte
			if tt.haystack != "" {
				hBytes = []byte(tt.haystack)
			}
			if tt.needle != "" {
				nBytes = []byte(tt.needle)
			}
			got := ContainsFoldASCII(hBytes, nBytes)
			if got != tt.expected {
				t.Fatalf("ContainsFoldASCII(%q, %q) = %v; want %v", tt.haystack, tt.needle, got, tt.expected)
			}
		})
	}

	// Explicit nil slice tests
	if !ContainsFoldASCII(nil, nil) {
		t.Errorf("expected true for ContainsFoldASCII(nil, nil)")
	}
	if !ContainsFoldASCII([]byte("abc"), nil) {
		t.Errorf("expected true for ContainsFoldASCII([]byte(\"abc\"), nil)")
	}
	if ContainsFoldASCII(nil, []byte("abc")) {
		t.Errorf("expected false for ContainsFoldASCII(nil, []byte(\"abc\"))")
	}
}

func TestContainsFoldASCII_ZeroAlloc(t *testing.T) {
	haystack := []byte("I have summarized the technical design. Please confirm how we should proceed.")
	needle := []byte("please confirm")

	allocs := testing.AllocsPerRun(1000, func() {
		_ = ContainsFoldASCII(haystack, needle)
	})
	if allocs != 0 {
		t.Fatalf("ContainsFoldASCII allocated %f heap objects, expected 0", allocs)
	}
}

func BenchmarkContainsFoldASCII_ZeroAlloc(b *testing.B) {
	haystack := []byte("I have summarized the technical design. Please confirm how we should proceed.")
	needle := []byte("please confirm")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ContainsFoldASCII(haystack, needle)
	}
}

func TestExtractQuotedField(t *testing.T) {
	jsonPayload := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello \"world\"\n"}}`)
	val, ok := ExtractQuotedField(jsonPayload, []byte(`"text":`))
	if !ok {
		t.Fatalf("expected ExtractQuotedField to find text")
	}
	expected := `"hello \"world\"\n"`
	if string(val) != expected {
		t.Fatalf("expected %s, got %s", expected, string(val))
	}

	// Missing key
	_, ok = ExtractQuotedField(jsonPayload, []byte(`"nonexistent":`))
	if ok {
		t.Fatalf("expected ok=false for nonexistent key")
	}

	// Malformed (no quote)
	_, ok = ExtractQuotedField([]byte(`{"text": 123}`), []byte(`"text":`))
	if ok {
		t.Fatalf("expected ok=false for non-quoted value")
	}

	// Zero-alloc verification
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = ExtractQuotedField(jsonPayload, []byte(`"text":`))
	})
	if allocs != 0 {
		t.Fatalf("ExtractQuotedField allocated %f heap objects, expected 0", allocs)
	}
}

func TestAppendEscapedJSONString(t *testing.T) {
	buf := make([]byte, 0, 128)
	out := AppendEscapedJSONString(buf, "hello \"world\"\n\r\t")
	expected := `"hello \"world\"\n\r\t"`
	if string(out) != expected {
		t.Fatalf("expected %s, got %s", expected, string(out))
	}

	// Zero-alloc verification
	allocs := testing.AllocsPerRun(1000, func() {
		scratch := make([]byte, 0, 128)
		_ = AppendEscapedJSONString(scratch, "hello \"world\"\n\r\t")
	})
	if allocs != 0 {
		t.Fatalf("AppendEscapedJSONString allocated %f heap objects, expected 0", allocs)
	}
}

func TestWriteEscapedJSONString(t *testing.T) {
	var buf bytes.Buffer
	buf.Grow(128)
	WriteEscapedJSONString(&buf, "hello \"world\"\n\r\t\x00")
	expected := `"hello \"world\"\n\r\t\u0000"`
	if buf.String() != expected {
		t.Fatalf("expected %s, got %s", expected, buf.String())
	}

	// Zero-alloc verification with pre-allocated buffer
	var benchBuf bytes.Buffer
	benchBuf.Grow(128)
	allocs := testing.AllocsPerRun(1000, func() {
		benchBuf.Reset()
		WriteEscapedJSONString(&benchBuf, "hello \"world\"\n\r\t")
	})
	if allocs != 0 {
		t.Fatalf("WriteEscapedJSONString allocated %f heap objects, expected 0", allocs)
	}
}

func TestLookupCompositeKey(t *testing.T) {
	m := map[string]int{
		"anthropic::claude-sonnet-5": 100,
		"openai::gpt-4o":             200,
	}

	// Case-insensitive prefix match
	if v, ok := LookupCompositeKey(m, "Anthropic", "::", "claude-sonnet-5"); !ok || v != 100 {
		t.Errorf("expected 100, got %d (ok=%v)", v, ok)
	}

	// Non-existent key
	if _, ok := LookupCompositeKey(m, "anthropic", "::", "non-existent"); ok {
		t.Errorf("expected not found")
	}

	// Nil map safety
	if _, ok := LookupCompositeKey[int](nil, "anthropic", "::", "claude-sonnet-5"); ok {
		t.Errorf("expected not found on nil map")
	}

	// Zero-alloc verification
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = LookupCompositeKey(m, "Anthropic", "::", "claude-sonnet-5")
	})
	if allocs != 0 {
		t.Fatalf("LookupCompositeKey allocated %f heap objects, expected 0", allocs)
	}
}

func BenchmarkLookupCompositeKey_ZeroAlloc(b *testing.B) {
	m := map[string]int{
		"anthropic::claude-sonnet-5": 100,
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = LookupCompositeKey(m, "Anthropic", "::", "claude-sonnet-5")
	}
}
