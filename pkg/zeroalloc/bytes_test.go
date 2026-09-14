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
	for i := 0; i < b.N; i++ {
		copy(scratch, sample)
		_ = StripSubslicesInPlace(scratch, targets)
	}
}
