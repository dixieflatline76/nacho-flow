// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package provider_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

type stringReadCloser struct {
	*strings.Reader
	closed bool
}

func (s *stringReadCloser) Close() error {
	s.closed = true
	return nil
}

func newMockSSEStream(content string) *stringReadCloser {
	return &stringReadCloser{
		Reader: strings.NewReader(content),
	}
}

func TestStreamAdapter_TextDelta(t *testing.T) {
	sseInput := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-3-5-sonnet","usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":50}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello world"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	adapter := provider.NewAnthropicStreamAdapter(newMockSSEStream(sseInput))
	defer func() { _ = adapter.Close() }()

	outBytes, err := io.ReadAll(adapter)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	outStr := string(outBytes)

	// Verify standard OpenAI content chunk was emitted
	if !strings.Contains(outStr, `"content":"Hello world"`) {
		t.Errorf("expected content chunk 'Hello world', got:\n%s", outStr)
	}

	// Verify stop reason mapped to 'stop'
	if !strings.Contains(outStr, `"finish_reason":"stop"`) {
		t.Errorf("expected finish_reason 'stop', got:\n%s", outStr)
	}

	// Verify usage chunk with aggregated input/output and cached tokens
	if !strings.Contains(outStr, `"prompt_tokens":150`) || !strings.Contains(outStr, `"completion_tokens":12`) || !strings.Contains(outStr, `"cached_tokens":50`) {
		t.Errorf("expected aggregated usage chunk, got:\n%s", outStr)
	}

	// Verify [DONE] marker
	if !strings.Contains(outStr, "data: [DONE]\n\n") {
		t.Errorf("expected data: [DONE], got:\n%s", outStr)
	}
}

func TestStreamAdapter_ToolCallStreaming(t *testing.T) {
	sseInput := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_tool","model":"claude-3-5-sonnet","usage":{"input_tokens":500,"output_tokens":0}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_99","name":"read_file"}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"main.go\"}"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":35}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	adapter := provider.NewAnthropicStreamAdapter(newMockSSEStream(sseInput))
	defer func() { _ = adapter.Close() }()

	outBytes, err := io.ReadAll(adapter)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	outStr := string(outBytes)

	// 1. Tool start chunk
	if !strings.Contains(outStr, `"id":"toolu_99"`) || !strings.Contains(outStr, `"name":"read_file"`) {
		t.Errorf("missing tool start chunk with id and name, got:\n%s", outStr)
	}

	// Verify no literal '\n' escaping artifact in wire stream
	if strings.Contains(outStr, `\n\ndata:`) {
		t.Errorf("wire stream contains literal '\\n\\ndata:', expected real newlines. Got:\n%s", outStr)
	}

	// 2. Tool arguments delta chunk
	if !strings.Contains(outStr, `"arguments":"{\"path\": \"main.go\"}"`) {
		t.Errorf("missing tool arguments chunk, got:\n%s", outStr)
	}

	// 3. Finish reason should map to 'tool_calls'
	if !strings.Contains(outStr, `"finish_reason":"tool_calls"`) {
		t.Errorf("expected finish_reason 'tool_calls', got:\n%s", outStr)
	}

	// 4. Aggregated usage
	if !strings.Contains(outStr, `"prompt_tokens":500`) || !strings.Contains(outStr, `"completion_tokens":35`) {
		t.Errorf("missing aggregated usage chunk, got:\n%s", outStr)
	}

	// 5. Exactly one [DONE] marker
	if count := strings.Count(outStr, "data: [DONE]"); count != 1 {
		t.Errorf("expected exactly 1 'data: [DONE]', got %d", count)
	}
}

func TestStreamAdapter_ThinkingStreaming(t *testing.T) {
	sseInput := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_think","model":"claude-3-7-sonnet","usage":{"input_tokens":300,"output_tokens":0}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Analyzing code structure..."}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Refactoring looks good."}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":1}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":80}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	adapter := provider.NewAnthropicStreamAdapter(newMockSSEStream(sseInput))
	defer func() { _ = adapter.Close() }()

	outBytes, err := io.ReadAll(adapter)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	outStr := string(outBytes)

	// Verify reasoning_content chunk was emitted
	if !strings.Contains(outStr, `"reasoning_content":"Analyzing code structure..."`) {
		t.Errorf("missing reasoning_content chunk, got:\n%s", outStr)
	}

	// Verify subsequent content chunk was emitted
	if !strings.Contains(outStr, `"content":"Refactoring looks good."`) {
		t.Errorf("missing content chunk, got:\n%s", outStr)
	}
}

func TestStreamAdapter_ClosePropagates(t *testing.T) {
	mock := newMockSSEStream("event: message_stop\ndata: {}\n\n")
	adapter := provider.NewAnthropicStreamAdapter(mock)

	if err := adapter.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
	if !mock.closed {
		t.Errorf("expected upstream Close() to be called")
	}

	var buf [16]byte
	_, err := adapter.Read(buf[:])
	if err != io.EOF {
		t.Errorf("expected io.EOF on closed adapter, got: %v", err)
	}
}

func BenchmarkAnthropicStreamAdapter_DeltaThroughput(b *testing.B) {
	chunkData := []byte("event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"token "}}` + "\n\n")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		adapter := provider.NewAnthropicStreamAdapter(io.NopCloser(bytes.NewReader(chunkData)))
		var p [256]byte
		_, _ = adapter.Read(p[:])
		_ = adapter.Close()
	}
}

func TestStreamAdapter_ZeroAllocDelta(t *testing.T) {
	chunk := []byte("event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"token "}}` + "\n\n")
	var streamBuf bytes.Buffer
	for i := 0; i < 2000; i++ {
		streamBuf.Write(chunk)
	}
	adapter := provider.NewAnthropicStreamAdapter(io.NopCloser(bytes.NewReader(streamBuf.Bytes())))
	defer func() { _ = adapter.Close() }()

	var p [256]byte
	// Prime the adapter
	_, _ = adapter.Read(p[:])

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = adapter.Read(p[:])
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocs per streaming token delta on active stream, got %f", allocs)
	}
}

func BenchmarkAnthropicStreamAdapter_StreamingHotPath(b *testing.B) {
	const numChunks = 1000
	var streamBuf bytes.Buffer
	chunk := []byte("event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"token "}}` + "\n\n")
	for i := 0; i < numChunks; i++ {
		streamBuf.Write(chunk)
	}
	streamBuf.WriteString("event: message_stop\ndata: {}\n\n")
	data := streamBuf.Bytes()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		adapter := provider.NewAnthropicStreamAdapter(io.NopCloser(bytes.NewReader(data)))
		var p [4096]byte
		for {
			n, err := adapter.Read(p[:])
			if n == 0 || err != nil {
				break
			}
		}
		_ = adapter.Close()
	}
}

func TestStreamAdapter_ErrorAndFallbackDeltas(t *testing.T) {
	// 1. Error Event
	t.Run("Error Event", func(t *testing.T) {
		errSSE := "event: error\n" +
			`data: {"type":"error","error":{"type":"overloaded_error","message":"Anthropic servers overloaded"}}` + "\n\n"

		adapter := provider.NewAnthropicStreamAdapter(newMockSSEStream(errSSE))
		defer func() { _ = adapter.Close() }()

		outBytes, err := io.ReadAll(adapter)
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
		if !strings.Contains(string(outBytes), "Anthropic servers overloaded") {
			t.Errorf("expected error chunk emitted, got: %s", string(outBytes))
		}
	})

	// 2. Structured fallback deltas that don't match fast-path ExtractQuotedField
	// (e.g. space before colon: "text" : "val")
	t.Run("Fallback Structured Deltas", func(t *testing.T) {
		fallbackSSE := "event: content_block_delta\n" +
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text" : "fallback text"}}` + "\n\n" +
			"event: content_block_delta\n" +
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking" : "fallback thought"}}` + "\n\n" +
			"event: content_block_delta\n" +
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json" : "{\"k\":1}"}}` + "\n\n"

		adapter := provider.NewAnthropicStreamAdapter(newMockSSEStream(fallbackSSE))
		defer func() { _ = adapter.Close() }()

		outBytes, err := io.ReadAll(adapter)
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
		outStr := string(outBytes)
		if !strings.Contains(outStr, `"content":"fallback text"`) {
			t.Errorf("expected fallback text, got: %s", outStr)
		}
		if !strings.Contains(outStr, `"reasoning_content":"fallback thought"`) {
			t.Errorf("expected fallback thinking, got: %s", outStr)
		}
		if !strings.Contains(outStr, `"arguments":"{\"k\":1}"`) {
			t.Errorf("expected fallback tool args, got: %s", outStr)
		}
	})

	// 3. Close with non-nil and nil underlying reader
	t.Run("Close", func(t *testing.T) {
		mockRC := newMockSSEStream("")
		adapter := provider.NewAnthropicStreamAdapter(mockRC)
		if err := adapter.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
		if !mockRC.closed {
			t.Errorf("expected underlying stream to be closed")
		}
		// Double close safe
		if err := adapter.Close(); err != nil {
			t.Errorf("second Close failed: %v", err)
		}
	})
}

type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

func (e *errorReader) Close() error {
	return nil
}

func TestStreamAdapter_ReadEdgeCases(t *testing.T) {
	// 1. Read on closed adapter returns io.EOF
	adapter := provider.NewAnthropicStreamAdapter(newMockSSEStream(""))
	_ = adapter.Close()
	var buf [64]byte
	n, err := adapter.Read(buf[:])
	if n != 0 || err != io.EOF {
		t.Errorf("expected 0, io.EOF on closed adapter read, got n=%d, err=%v", n, err)
	}

	// 2. Underlying reader returns non-EOF error
	errExpected := fmt.Errorf("network reset")
	errAdapter := provider.NewAnthropicStreamAdapter(&errorReader{err: errExpected})
	_, err = errAdapter.Read(buf[:])
	if err == nil || !strings.Contains(err.Error(), "network reset") {
		t.Errorf("expected network reset error, got: %v", err)
	}

	// 3. Abrupt EOF after message_start emits usage and done
	abruptSSE := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-3-5-sonnet","usage":{"input_tokens":42}}}` + "\n\n"
	abruptAdapter := provider.NewAnthropicStreamAdapter(newMockSSEStream(abruptSSE))
	defer func() { _ = abruptAdapter.Close() }()
	out, err := io.ReadAll(abruptAdapter)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if !strings.Contains(string(out), `"prompt_tokens":42`) || !strings.Contains(string(out), "[DONE]") {
		t.Errorf("expected abrupt EOF to emit usage and [DONE], got:\n%s", string(out))
	}

	// 4. Double emit done
	doubleDoneSSE := "event: message_stop\ndata: {}\n\nevent: message_stop\ndata: {}\n\n"
	doubleAdapter := provider.NewAnthropicStreamAdapter(newMockSSEStream(doubleDoneSSE))
	defer func() { _ = doubleAdapter.Close() }()
	_, _ = io.ReadAll(doubleAdapter)
}
