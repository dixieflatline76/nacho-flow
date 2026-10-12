// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestStreamNormalizer_DeferTerminal_EmptyTruncatedTurn(t *testing.T) {
	// F1 reasoning length truncation without content or tool calls
	rawSSE := `data: {"id":"gen-1","choices":[{"index":0,"delta":{"reasoning_content":"Deep thinking..."}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{},"finish_reason":"length"}]}

data: {"id":"gen-1","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":500,"total_tokens":600}}

data: [DONE]
`
	normalizer := NewStreamNormalizer(io.NopCloser(strings.NewReader(rawSSE)))
	normalizer.SetDeferTerminal(true)

	var out bytes.Buffer
	buf := make([]byte, 1024)
	for {
		n, err := normalizer.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
	}

	if !normalizer.IsEmptyTruncatedTurn() {
		t.Errorf("expected IsEmptyTruncatedTurn() == true")
	}
	if !normalizer.TerminalHeld() {
		t.Errorf("expected TerminalHeld() == true when deferTerminal is enabled")
	}

	held := string(normalizer.HeldTerminal())
	if !strings.Contains(held, `"finish_reason":"length"`) {
		t.Errorf("expected held terminal to contain finish_reason: length, got: %q", held)
	}

	result := out.String()
	// Output must NOT contain honest truncation notice chunk or [DONE] yet
	if strings.Contains(result, "Reasoning Limit Reached") {
		t.Errorf("out buffer should NOT contain terminal notice yet, got: %s", result)
	}
	if strings.Contains(result, "[DONE]") {
		t.Errorf("out buffer should NOT contain [DONE] yet, got: %s", result)
	}
}

func TestStreamNormalizer_DeferTerminal_WithActionableOutput(t *testing.T) {
	t.Run("with content", func(t *testing.T) {
		rawSSE := `data: {"id":"gen-1","choices":[{"index":0,"delta":{"reasoning_content":"Thinking..."}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"content":"Here is the solution."}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
		normalizer := NewStreamNormalizer(io.NopCloser(strings.NewReader(rawSSE)))
		normalizer.SetDeferTerminal(true)

		var out bytes.Buffer
		buf := make([]byte, 1024)
		for {
			n, err := normalizer.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("unexpected read error: %v", err)
			}
		}

		if normalizer.IsEmptyTruncatedTurn() {
			t.Errorf("expected IsEmptyTruncatedTurn() == false")
		}
		if normalizer.TerminalHeld() {
			t.Errorf("expected TerminalHeld() == false when content was emitted")
		}
		if !normalizer.HasNonWhitespaceContent() {
			t.Errorf("expected HasNonWhitespaceContent() == true")
		}
		result := out.String()
		if !strings.Contains(result, "[DONE]") {
			t.Errorf("expected output to contain [DONE], got: %s", result)
		}
	})

	t.Run("with tool calls", func(t *testing.T) {
		rawSSE := `data: {"id":"gen-1","choices":[{"index":0,"delta":{"reasoning_content":"Thinking..."}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{}}"}}]}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
		normalizer := NewStreamNormalizer(io.NopCloser(strings.NewReader(rawSSE)))
		normalizer.SetDeferTerminal(true)

		var out bytes.Buffer
		buf := make([]byte, 1024)
		for {
			n, err := normalizer.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("unexpected read error: %v", err)
			}
		}

		if normalizer.IsEmptyTruncatedTurn() {
			t.Errorf("expected IsEmptyTruncatedTurn() == false")
		}
		if normalizer.TerminalHeld() {
			t.Errorf("expected TerminalHeld() == false when tool calls were emitted")
		}
		if !normalizer.HasEmittedToolCalls() {
			t.Errorf("expected HasEmittedToolCalls() == true")
		}
		result := out.String()
		if !strings.Contains(result, "[DONE]") {
			t.Errorf("expected output to contain [DONE], got: %s", result)
		}
	})
}

func TestStreamNormalizer_SuppressPrelude(t *testing.T) {
	rawSSE := `data: {"id":"gen-rec-1","choices":[{"index":0,"delta":{"role":"assistant"}}]}

data: {"id":"gen-rec-1","choices":[{"index":0,"delta":{"content":"Recovered content response"}}]}

data: {"id":"gen-rec-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	normalizer := NewStreamNormalizer(io.NopCloser(strings.NewReader(rawSSE)))
	normalizer.SetSuppressPrelude(true)

	var out bytes.Buffer
	buf := make([]byte, 1024)
	for {
		n, err := normalizer.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
	}

	result := out.String()
	// Role-only prelude chunk must be dropped
	if strings.Contains(result, `"role":"assistant"`) {
		t.Errorf("expected role-only prelude to be suppressed, got output: %s", result)
	}
	// Content must be present
	if !strings.Contains(result, "Recovered content response") {
		t.Errorf("expected content to be present, got output: %s", result)
	}
	if !strings.Contains(result, "[DONE]") {
		t.Errorf("expected [DONE] to be present, got output: %s", result)
	}
}
