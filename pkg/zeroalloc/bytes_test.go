// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package zeroalloc

import (
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
