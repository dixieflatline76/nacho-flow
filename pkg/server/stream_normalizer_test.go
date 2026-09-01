package server

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
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

	for b.Loop() {
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

	for b.Loop() {
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

	// 7. payloadContent helper branches
	if payloadContent([]byte("invalid json")) != "" {
		t.Errorf("expected empty string for invalid json in payloadContent")
	}
	if payloadContent([]byte(`{"choices":[]}`)) != "" {
		t.Errorf("expected empty string for 0 choices in payloadContent")
	}

	// 8. processLine with malformed JSON data line
	normMalformed := NewStreamNormalizer(io.NopCloser(strings.NewReader("")))
	normMalformed.processLine([]byte("data: {malformed json\n"))
	if !strings.Contains(normMalformed.outBuf.String(), "data: {malformed json") {
		t.Errorf("expected malformed line passed through directly")
	}
}

func TestStreamNormalizer_QwenImStartThink(t *testing.T) {
	rawSSE := `data: {"id":"qwen-1","choices":[{"index":0,"delta":{"content":"<|im_start|>think\nAnalyzing problem with Qwen"}}]}

data: {"id":"qwen-1","choices":[{"index":0,"delta":{"content":"\n<|im_end|>\nHere is the answer"}}]}

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
	if strings.Contains(result, "<|im_start|>think") {
		t.Errorf("expected <|im_start|>think to be replaced with <think>, got:\n%s", result)
	}
	if !strings.Contains(result, "<think>") {
		t.Errorf("expected <think> tag in normalized output, got:\n%s", result)
	}
	if !strings.Contains(result, "</think>") {
		t.Errorf("expected </think> tag in normalized output, got:\n%s", result)
	}
	if !strings.Contains(result, "Here is the answer") {
		t.Errorf("expected final answer text, got:\n%s", result)
	}
}

func TestStreamNormalizer_ThinkingTags(t *testing.T) {
	rawSSE := `data: {"id":"claude-style-1","choices":[{"index":0,"delta":{"content":"<thinking>\nClaude-style reasoning step"}}]}

data: {"id":"claude-style-1","choices":[{"index":0,"delta":{"content":"\n</thinking>\nFinal response text"}}]}

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
	if strings.Contains(result, "<thinking>") || strings.Contains(result, "</thinking>") {
		t.Errorf("expected <thinking> / </thinking> tags to be normalized, got:\n%s", result)
	}
	if !strings.Contains(result, "<think>") || !strings.Contains(result, "</think>") {
		t.Errorf("expected standard <think>...</think> tags, got:\n%s", result)
	}
	if !strings.Contains(result, "Final response text") {
		t.Errorf("expected final response text, got:\n%s", result)
	}
}

func TestStreamNormalizer_UsageExtraction(t *testing.T) {
	rawSSE := `data: {"id":"gen-usage-1","choices":[{"index":0,"delta":{"content":"Hello world!"}}]}

data: {"id":"gen-usage-1","choices":[],"usage":{"prompt_tokens":58000,"completion_tokens":1200,"total_tokens":59200}}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	_, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	usage, ok := norm.GetUsage()
	if !ok {
		t.Fatalf("expected usage to be extracted from final SSE chunk")
	}
	if usage.PromptTokens != 58000 {
		t.Errorf("expected prompt_tokens 58000, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 1200 {
		t.Errorf("expected completion_tokens 1200, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 59200 {
		t.Errorf("expected total_tokens 59200, got %d", usage.TotalTokens)
	}
}

func TestStreamNormalizer_FallbackTokenEstimation(t *testing.T) {
	rawSSE := `data: {"id":"gen-fallback-1","choices":[{"index":0,"delta":{"content":"This is sixteen characters."}}]}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	_, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	usage, ok := norm.GetUsage()
	if ok {
		t.Fatalf("expected ok=false for estimated fallback usage")
	}
	if usage.CompletionTokens == 0 {
		t.Errorf("expected non-zero estimated completion tokens from emitted text")
	}

	// Test 0 chars branch
	emptyNorm := NewStreamNormalizer(io.NopCloser(strings.NewReader("")))
	defer emptyNorm.Close()
	zeroUsage, ok := emptyNorm.GetUsage()
	if ok || zeroUsage.CompletionTokens != 0 {
		t.Fatalf("expected 0 completion tokens for empty stream")
	}

	// Test 1 char branch (estTokens >= 1)
	singleNorm := NewStreamNormalizer(io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n\n")))
	defer singleNorm.Close()
	_, _ = io.ReadAll(singleNorm)
	oneUsage, ok := singleNorm.GetUsage()
	if ok || oneUsage.CompletionTokens < 1 {
		t.Fatalf("expected >= 1 completion token for single character, got %d", oneUsage.CompletionTokens)
	}
}

func TestStreamNormalizer_AgentShieldFallback(t *testing.T) {
	t.Run("Synthesizes tool call delta before [DONE] on trailing question", func(t *testing.T) {
		rawSSE := `data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"I have planned the Go CLI app. "}}]}

data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"Are you satisfied with this plan?"}}]}

data: [DONE]

`
		r := io.NopCloser(strings.NewReader(rawSSE))
		norm := NewStreamNormalizer(r)
		defer norm.Close()

		mgr := shield.NewDefaultShieldManager()
		norm.SetShield("ask_followup_question", mgr)

		outBytes, err := io.ReadAll(norm)
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}

		outStr := string(outBytes)
		if !strings.Contains(outStr, "\"ask_followup_question\"") {
			t.Fatalf("expected output stream to contain synthesized 'ask_followup_question', got: %s", outStr)
		}
		if !strings.Contains(outStr, "\"finish_reason\":\"tool_calls\"") {
			t.Fatalf("expected finish_reason 'tool_calls' in stream, got: %s", outStr)
		}
		if !strings.HasSuffix(strings.TrimSpace(outStr), "data: [DONE]") {
			t.Fatalf("expected stream to terminate with data: [DONE], got: %s", outStr)
		}
	})

	t.Run("Bypasses shield when native tool calls are present", func(t *testing.T) {
		rawSSE := `data: {"id":"chatcmpl-2","choices":[{"index":0,"delta":{"content":"Let me execute the command: "}}]}

data: {"id":"chatcmpl-2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"execute_command","arguments":"{\"command\":\"go test\"}"}}]}}]}

data: [DONE]

`
		r := io.NopCloser(strings.NewReader(rawSSE))
		norm := NewStreamNormalizer(r)
		defer norm.Close()

		mgr := shield.NewDefaultShieldManager()
		norm.SetShield("ask_followup_question", mgr)

		outBytes, err := io.ReadAll(norm)
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}

		outStr := string(outBytes)
		if strings.Contains(outStr, "call_autowrap_") {
			t.Fatalf("expected no autowrap when native tool calls exist, got: %s", outStr)
		}
	})

	t.Run("Bypasses shield when message is normal completion with no question", func(t *testing.T) {
		rawSSE := `data: {"id":"chatcmpl-3","choices":[{"index":0,"delta":{"content":"Files were created successfully."}}]}

data: [DONE]

`
		r := io.NopCloser(strings.NewReader(rawSSE))
		norm := NewStreamNormalizer(r)
		defer norm.Close()

		mgr := shield.NewDefaultShieldManager()
		norm.SetShield("ask_followup_question", mgr)

		outBytes, err := io.ReadAll(norm)
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}

		outStr := string(outBytes)
		if strings.Contains(outStr, "ask_followup_question") {
			t.Fatalf("expected no tool call synthesis for non-question completion, got: %s", outStr)
		}
	})
}

func TestStreamNormalizer_UsageExtraction_CacheAndCost(t *testing.T) {
	rawSSE := `data: {"id":"gen-1","choices":[{"index":0,"delta":{"content":"Hello world"}}]}

data: {"id":"gen-1","choices":[],"usage":{"prompt_tokens":50000,"completion_tokens":1000,"total_tokens":51000,"prompt_tokens_details":{"cached_tokens":35000},"cost":0.024}}

data: [DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	_, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	usage, ok := norm.GetUsage()
	if !ok {
		t.Fatalf("expected GetUsage() to return true, got false")
	}
	if usage.PromptTokens != 50000 {
		t.Errorf("expected 50000 prompt tokens, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 1000 {
		t.Errorf("expected 1000 completion tokens, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 51000 {
		t.Errorf("expected 51000 total tokens, got %d", usage.TotalTokens)
	}
	if usage.PromptTokensDetails == nil {
		t.Fatalf("expected PromptTokensDetails to be non-nil")
	}
	if usage.PromptTokensDetails.CachedTokens != 35000 {
		t.Errorf("expected 35000 cached tokens, got %d", usage.PromptTokensDetails.CachedTokens)
	}
	if usage.Cost != 0.024 {
		t.Errorf("expected 0.024 cost, got %f", usage.Cost)
	}
}

