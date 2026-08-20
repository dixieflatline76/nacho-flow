package server

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// chunkReader simulates fragmented TCP streaming by delivering bytes in arbitrary chunk sizes.
type fragmentedReader struct {
	data      []byte
	pos       int
	chunkSize int
}

func (r *fragmentedReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	limit := r.chunkSize
	if limit <= 0 || limit > len(p) {
		limit = len(p)
	}
	remaining := len(r.data) - r.pos
	if limit > remaining {
		limit = remaining
	}
	copy(p, r.data[r.pos:r.pos+limit])
	r.pos += limit
	return limit, nil
}

func (r *fragmentedReader) Close() error {
	return nil
}

func TestStreamNormalizer_DeepSeekDirect_ReasoningContent(t *testing.T) {
	rawSSE := `data: {"id":"gen-1","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"reasoning_content":"Step 1: Analyzing issue\n"}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"reasoning_content":"Step 2: Solution found"}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"content":"Here is the fixed code:"}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"content":"\nfmt.Println(\"Hello\")"}}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, "<think>\\nStep 1: Analyzing issue") && !strings.Contains(result, "<think>\nStep 1: Analyzing issue") {
		t.Errorf("expected <think> opening tag, got:\n%s", result)
	}
	if !strings.Contains(result, "</think>") {
		t.Errorf("expected </think> closing tag, got:\n%s", result)
	}
	if !strings.Contains(result, "Here is the fixed code:") {
		t.Errorf("expected final answer content, got:\n%s", result)
	}
	if !strings.Contains(result, "data: [DONE]") {
		t.Errorf("expected [DONE] marker to pass through, got:\n%s", result)
	}
}

func TestStreamNormalizer_OpenRouter_Reasoning(t *testing.T) {
	rawSSE := `data: {"id":"gen-2","choices":[{"index":0,"delta":{"reasoning":"Thinking via OpenRouter..."}}]}

data: {"id":"gen-2","choices":[{"index":0,"delta":{"content":"Done thinking!"}}]}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, "<think>") {
		t.Errorf("expected <think> tag for OpenRouter reasoning, got:\n%s", result)
	}
	if !strings.Contains(result, "</think>") {
		t.Errorf("expected </think> tag for OpenRouter reasoning, got:\n%s", result)
	}
	if !strings.Contains(result, "Done thinking!") {
		t.Errorf("expected final content, got:\n%s", result)
	}
}

func TestStreamNormalizer_Ollama_NativeThink_NoDoubleWrap(t *testing.T) {
	// Ollama streams native <think> inside content directly
	rawSSE := `data: {"id":"gen-3","choices":[{"index":0,"delta":{"content":"<think>\nAnalyzing local model"}}]}

data: {"id":"gen-3","choices":[{"index":0,"delta":{"content":"\n</think>\n\nFinal answer"}}]}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	// Must NOT contain double think tags like <think><think>
	if strings.Contains(result, "<think><think>") || strings.Count(result, "<think>") > 1 {
		t.Errorf("detected double-wrapping of native <think> tag:\n%s", result)
	}
	if !strings.Contains(result, "Final answer") {
		t.Errorf("expected final answer, got:\n%s", result)
	}
}

func TestStreamNormalizer_StandardNonReasoning(t *testing.T) {
	rawSSE := `data: {"id":"gen-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Standard GPT-4o output"}}]}

data: {"id":"gen-4","choices":[{"index":0,"delta":{"content":" with more tokens."}}]}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if strings.Contains(result, "<think>") {
		t.Errorf("standard non-reasoning stream should not have <think> tag, got:\n%s", result)
	}
	if !strings.Contains(result, "Standard GPT-4o output with more tokens.") && (!strings.Contains(result, "Standard GPT-4o output") || !strings.Contains(result, " with more tokens.")) {
		t.Errorf("expected standard text chunks, got:\n%s", result)
	}
}

func TestStreamNormalizer_MixedReasoningAndToolCalls(t *testing.T) {
	rawSSE := `data: {"id":"gen-5","choices":[{"index":0,"delta":{"reasoning_content":"Deciding to invoke grep tool"}}]}

data: {"id":"gen-5","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"grep","arguments":"{\"query\":\"error\"}"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, "<think>") || !strings.Contains(result, "</think>") {
		t.Errorf("expected think tags closed before tool call, got:\n%s", result)
	}
	if !strings.Contains(result, "call_123") || !strings.Contains(result, "grep") {
		t.Errorf("expected tool_calls preserved intact, got:\n%s", result)
	}
}

func TestStreamNormalizer_InternalTagSanitization(t *testing.T) {
	rawSSE := `data: {"id":"gen-6","choices":[{"index":0,"delta":{"reasoning_content":"Explain XML: </think> is a tag"}}]}

data: {"id":"gen-6","choices":[{"index":0,"delta":{"content":"XML explanation complete."}}]}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	// Internal literal </think> inside reasoning should be escaped to &lt;/think&gt;
	if strings.Contains(result, "Explain XML: </think>") {
		t.Errorf("literal </think> inside reasoning should be sanitized, got:\n%s", result)
	}
	if !strings.Contains(result, "&lt;/think&gt;") {
		t.Errorf("expected &lt;/think&gt; sanitized text, got:\n%s", result)
	}
}

func TestStreamNormalizer_TCPFragmentation_ByteByByte(t *testing.T) {
	rawSSE := `data: {"id":"gen-7","choices":[{"index":0,"delta":{"reasoning_content":"Thinking fragment by fragment..."}}]}

data: {"id":"gen-7","choices":[{"index":0,"delta":{"content":"Final fragmented answer."}}]}

data: [DONE]

`
	frag := &fragmentedReader{
		data:      []byte(rawSSE),
		chunkSize: 1, // 1 byte per TCP packet read!
	}
	norm := NewStreamNormalizer(frag)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected error reading fragmented stream: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, "<think>") || !strings.Contains(result, "</think>") {
		t.Errorf("fragmented read lost think tags:\n%s", result)
	}
	if !strings.Contains(result, "Final fragmented answer.") {
		t.Errorf("fragmented read corrupted content:\n%s", result)
	}
}

func TestStreamNormalizer_AbruptEOF_TerminalTagClosing(t *testing.T) {
	// Upstream closes socket abruptly while still in reasoning phase
	rawSSE := `data: {"id":"gen-8","choices":[{"index":0,"delta":{"reasoning_content":"Thinking mid-stream..."}}]}`

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, "<think>") {
		t.Errorf("expected <think> tag, got:\n%s", result)
	}
	if !strings.Contains(result, "</think>") {
		t.Errorf("expected </think> closed on abrupt EOF, got:\n%s", result)
	}
}

func TestStreamNormalizer_MalformedLinesAndComments(t *testing.T) {
	rawSSE := `: ping keepalive comment

event: message
data: {"id":"gen-9","choices":[{"index":0,"delta":{"reasoning_content":"Valid reasoning."}}]}

invalid non-sse line here

data: {"id":"gen-9","choices":[{"index":0,"delta":{"content":"Valid content."}}]}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, ": ping keepalive comment") {
		t.Errorf("expected comments passed through, got:\n%s", result)
	}
	if !strings.Contains(result, "Valid reasoning.") || !strings.Contains(result, "Valid content.") {
		t.Errorf("expected valid data lines processed, got:\n%s", result)
	}
}

func TestStreamNormalizer_HighConcurrency_Race(t *testing.T) {
	const concurrentStreams = 50
	var wg sync.WaitGroup
	wg.Add(concurrentStreams)

	for i := 0; i < concurrentStreams; i++ {
		go func(id int) {
			defer wg.Done()
			rawSSE := fmt.Sprintf(`data: {"id":"gen-%d","choices":[{"index":0,"delta":{"reasoning_content":"Thinking #%d"}}]}

data: {"id":"gen-%d","choices":[{"index":0,"delta":{"content":"Answer #%d"}}]}

data: [DONE]

`, id, id, id, id)

			norm := NewStreamNormalizer(io.NopCloser(strings.NewReader(rawSSE)))
			defer norm.Close()

			out, err := io.ReadAll(norm)
			if err != nil {
				t.Errorf("stream #%d failed: %v", id, err)
				return
			}
			res := string(out)
			if !strings.Contains(res, "<think>") || !strings.Contains(res, fmt.Sprintf("Answer #%d", id)) {
				t.Errorf("stream #%d unexpected result:\n%s", id, res)
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent stream test timed out (possible deadlock)")
	}
}

func BenchmarkSSE_NonReasoning_ZeroAlloc(b *testing.B) {
	rawSSE := []byte(`data: {"id":"bench-1","choices":[{"index":0,"delta":{"content":"Fast token"}}]}

`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r := io.NopCloser(bytes.NewReader(rawSSE))
		norm := NewStreamNormalizer(r)
		_, _ = io.Copy(io.Discard, norm)
		_ = norm.Close()
	}
}

func BenchmarkSSE_ReasoningTransform(b *testing.B) {
	rawSSE := []byte(`data: {"id":"bench-2","choices":[{"index":0,"delta":{"reasoning_content":"Fast thinking token"}}]}

`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r := io.NopCloser(bytes.NewReader(rawSSE))
		norm := NewStreamNormalizer(r)
		_, _ = io.Copy(io.Discard, norm)
		_ = norm.Close()
	}
}

type streamErrReader struct {
	err error
}

func (e *streamErrReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

func (e *streamErrReader) Close() error {
	return e.err
}

func TestStreamNormalizer_ErrorPathsAndCloser(t *testing.T) {
	// 1. Upstream read error
	expectedErr := fmt.Errorf("network socket reset")
	errNorm := NewStreamNormalizer(&streamErrReader{err: expectedErr})
	p := make([]byte, 100)
	_, err := errNorm.Read(p)
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
	_ = errNorm.Close()

	// 2. Read after close
	_, err = errNorm.Read(p)
	if err != io.EOF {
		t.Errorf("expected EOF reading closed stream, got %v", err)
	}

	// 3. Double close
	if err := errNorm.Close(); err != nil {
		t.Errorf("expected nil on second Close(), got %v", err)
	}

	// 4. Reason field (alternative provider format)
	rawReasonSSE := `data: {"id":"gen-reason","choices":[{"index":0,"delta":{"reason":"Thinking step..."}}]}

data: {"id":"gen-reason","choices":[{"index":0,"delta":{"content":"Final answer"}}]}

`
	reasonNorm := NewStreamNormalizer(io.NopCloser(strings.NewReader(rawReasonSSE)))
	out, _ := io.ReadAll(reasonNorm)
	_ = reasonNorm.Close()
	if !strings.Contains(string(out), "<think>") {
		t.Errorf("expected <think> tag for reason field, got:\n%s", string(out))
	}

	// 5. marshalNoEscapeHTML error branch
	_, err = marshalNoEscapeHTML(make(chan int))
	if err == nil {
		t.Errorf("expected error marshaling channel, got nil")
	}

	// 6. alreadyTagged reasoning chunk
	normWithTagged := NewStreamNormalizer(io.NopCloser(strings.NewReader("")))
	normWithTagged.alreadyTagged = true
	normWithTagged.processLine([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"already tagged reasoning\"}}]}\n"))
	outStr := normWithTagged.outBuf.String()
	if strings.Contains(outStr, "<think>") {
		t.Errorf("should not inject <think> when alreadyTagged=true, got: %s", outStr)
	}
}
