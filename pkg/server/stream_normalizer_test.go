package server

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
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
	if !strings.Contains(result, "\"reasoning_content\":\"Step 1: Analyzing issue\\n\"") {
		t.Errorf("expected reasoning_content step 1, got:\n%s", result)
	}
	if !strings.Contains(result, "\"reasoning_content\":\"Step 2: Solution found\"") {
		t.Errorf("expected reasoning_content step 2, got:\n%s", result)
	}
	if strings.Contains(result, "<think>") || strings.Contains(result, "</think>") {
		t.Errorf("did not expect raw <think> tags in content, got:\n%s", result)
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
	if !strings.Contains(result, "\"reasoning_content\":\"Thinking via OpenRouter...\"") {
		t.Errorf("expected reasoning_content for OpenRouter reasoning, got:\n%s", result)
	}
	if strings.Contains(result, "<think>") || strings.Contains(result, "</think>") {
		t.Errorf("did not expect <think> tags for OpenRouter reasoning, got:\n%s", result)
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
	if !strings.Contains(result, "Analyzing local model") {
		t.Errorf("expected reasoning text extracted, got:\n%s", result)
	}
	if strings.Contains(result, "<think>") || strings.Contains(result, "</think>") {
		t.Errorf("detected unextracted <think> tag in content:\n%s", result)
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
	if !strings.Contains(result, "Deciding to invoke grep tool") {
		t.Errorf("expected reasoning text preserved, got:\n%s", result)
	}
	if strings.Contains(result, "<think>") || strings.Contains(result, "</think>") {
		t.Errorf("unexpected think tags in content, got:\n%s", result)
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
	if !strings.Contains(result, "Explain XML: </think> is a tag") {
		t.Errorf("literal XML inside reasoning should be preserved in reasoning_content, got:\n%s", result)
	}
	if strings.Contains(result, "<think>") {
		t.Errorf("unexpected <think> tag in result, got:\n%s", result)
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
	if !strings.Contains(result, "Thinking fragment by fragment...") {
		t.Errorf("fragmented read lost reasoning text:\n%s", result)
	}
	if strings.Contains(result, "<think>") || strings.Contains(result, "</think>") {
		t.Errorf("fragmented read leaked think tags into content:\n%s", result)
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
	if !strings.Contains(result, "Thinking mid-stream...") {
		t.Errorf("expected reasoning text on abrupt EOF, got:\n%s", result)
	}
	if strings.Contains(result, "<think>") || strings.Contains(result, "</think>") {
		t.Errorf("unexpected think tags on abrupt EOF, got:\n%s", result)
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
			if !strings.Contains(res, fmt.Sprintf("Thinking #%d", id)) || !strings.Contains(res, fmt.Sprintf("Answer #%d", id)) {
				t.Errorf("stream #%d unexpected result:\n%s", id, res)
			}
			if strings.Contains(res, "<think>") {
				t.Errorf("stream #%d leaked <think> tag into output:\n%s", id, res)
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

func BenchmarkSSE_NonReasoning_FastPath(b *testing.B) {
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

func BenchmarkSSE_ProcessLine_NonReasoning(b *testing.B) {
	rawLine := []byte("data: {\"id\":\"bench-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Fast token\"}}]}\n")
	norm := &StreamNormalizer{
		outBuf:   &bytes.Buffer{},
		features: uint16(router.FeatureDefaultAll),
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		norm.outBuf.Reset()
		norm.processLine(rawLine)
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

func BenchmarkStreamNormalizer_RouteContentDelta_WriteTags(b *testing.B) {
	enabled := true
	cb := shield.GetCycleBreaker(&contract.CycleBreakerConfig{Enabled: &enabled})
	defer shield.PutCycleBreaker(cb)

	norm := &StreamNormalizer{
		cycleBreaker: cb,
		features:     uint16(router.FeatureDefaultAll),
	}

	openChunk := "<write_to_file path=\"pkg/util/helper.go\">"
	codeChunk := "func Helper() string {\n\treturn \"ok\"\n}\n"
	closeChunk := "</write_to_file>"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		norm.inWriteTool = false
		norm.pendingWriteTagBuf = ""
		norm.routeContentDelta(openChunk)
		norm.routeContentDelta(codeChunk)
		norm.routeContentDelta(closeChunk)
	}
}

func BenchmarkStreamNormalizer_FastPassWrite(b *testing.B) {
	norm := &StreamNormalizer{
		outBuf:      &bytes.Buffer{},
		inWriteTool: true,
		features:    uint16(router.FeatureDefaultAll),
	}
	line := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"fmt.Println(\\\"hello\\\")\\n\"}}]}\n\n")
	payload := line[6 : len(line)-2]

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		norm.outBuf.Reset()
		norm.fastPassWrite(line, payload)
	}
}

func BenchmarkStreamNormalizer_FindEarliestOpenCloseTag(b *testing.B) {
	openSample := "prefix prose <write_to_file path=\"foo.go\"> content"
	closeSample := "content code </write_to_file> following prose"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _, _ = findEarliestOpenTag(openSample)
		_, _, _ = findEarliestCloseTag(closeSample)
	}
}

func BenchmarkStreamNormalizer_FindPartialTagSuffix(b *testing.B) {
	partialOpen := "prefix prose <write_"
	partialClose := "content code </write_"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = findPartialOpenTagSuffix(partialOpen)
		_ = findPartialCloseTagSuffix(partialClose)
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
	if !strings.Contains(string(out), "Thinking step...") {
		t.Errorf("expected reasoning text for reason field, got:\n%s", string(out))
	}
	if strings.Contains(string(out), "<think>") {
		t.Errorf("unexpected <think> tag for reason field, got:\n%s", string(out))
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
	if strings.Contains(result, "<|im_start|>think") || strings.Contains(result, "<|im_end|>") {
		t.Errorf("expected special tags stripped, got:\n%s", result)
	}
	if strings.Contains(result, "<think>") || strings.Contains(result, "</think>") {
		t.Errorf("expected think tags converted to reasoning_content, not kept in content, got:\n%s", result)
	}
	if !strings.Contains(result, "Analyzing problem with Qwen") {
		t.Errorf("expected reasoning text preserved in reasoning_content, got:\n%s", result)
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
	if strings.Contains(result, "<think>") || strings.Contains(result, "</think>") {
		t.Errorf("expected standard think tags converted to reasoning_content, got:\n%s", result)
	}
	if !strings.Contains(result, "Claude-style reasoning step") {
		t.Errorf("expected reasoning text preserved, got:\n%s", result)
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

func TestStreamNormalizer_ExtractContentFast(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "standard compact content",
			payload: `{"choices":[{"delta":{"content":"Hello world"}}]}`,
			want:    "Hello world",
		},
		{
			name:    "content with space after colon",
			payload: `{"choices":[{"delta":{"content": "With spaces"}}]}`,
			want:    "With spaces",
		},
		{
			name:    "content with escaped quotes and backslashes",
			payload: `{"choices":[{"delta":{"content":"Line 1\\n\"quoted\" text"}}]}`,
			want:    "Line 1\\n\"quoted\" text",
		},
		{
			name:    "content with escaped newline",
			payload: `{"choices":[{"delta":{"content":"Line 1\nLine 2"}}]}`,
			want:    "Line 1\nLine 2",
		},
		{
			name:    "empty content",
			payload: `{"choices":[{"delta":{"content":""}}]}`,
			want:    "",
		},
		{
			name:    "null content",
			payload: `{"choices":[{"delta":{"content":null}}]}`,
			want:    "",
		},
		{
			name:    "missing content field",
			payload: `{"choices":[{"delta":{"role":"assistant"}}]}`,
			want:    "",
		},
		{
			name:    "malformed payload",
			payload: `invalid json payload`,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContentFast([]byte(tt.payload))
			if got != tt.want {
				t.Errorf("extractContentFast(%s) = %q, want %q", tt.payload, got, tt.want)
			}
		})
	}
}

func TestStreamNormalizer_Ollama_UnicodeEscaped_ChannelTag(t *testing.T) {
	// Ollama encodes <channel|> using standard Go json.Marshal HTML-escaping: \u003cchannel|\u003e
	rawSSE := "data: {\"choices\":[{\"delta\":{\"content\":\"I will combine the models and solvers appropriately.\\n\\u003cchannel|\\u003e\"}}]}\n\ndata: [DONE]\n\n"
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if strings.Contains(result, "channel|") || strings.Contains(result, "<channel|>") || strings.Contains(result, "\\u003cchannel|") {
		t.Fatalf("expected channel tag to be stripped, got:\n%s", result)
	}
	if !strings.Contains(result, "I will combine the models and solvers appropriately.") {
		t.Fatalf("expected prose content to be preserved, got:\n%s", result)
	}
}

func TestStreamNormalizer_Split_ChannelTag_Across_Chunks(t *testing.T) {
	// Delimiter split across two SSE chunks: <channel| in chunk 1, > in chunk 2
	rawSSE := "data: {\"choices\":[{\"delta\":{\"content\":\"Starting solver...\\n<channel|\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\">\"}}]}\n\ndata: [DONE]\n\n"
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if strings.Contains(result, "<channel|>") || strings.Contains(result, "<channel|") || strings.Contains(result, ">") {
		t.Fatalf("expected split channel tag to be completely eliminated without leaking >, got:\n%s", result)
	}
	if !strings.Contains(result, "Starting solver...") {
		t.Fatalf("expected prose content to be preserved, got:\n%s", result)
	}
}

func TestStreamNormalizer_RawChannelTag_PrecedingToolCall(t *testing.T) {
	// Exact scenario from user screenshot: prose ending in <channel|> right before tool invocation
	rawSSE := "data: {\"choices\":[{\"delta\":{\"content\":\"Let's start with BacktrackingSolver. I will combine the models and solvers appropriately.\\n<channel|>\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"write_to_file\",\"arguments\":\"{\\\"file\\\":\\\"models.go\\\"}\"}}]}}]}\n\ndata: [DONE]\n\n"
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	out, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if strings.Contains(result, "<channel|>") || strings.Contains(result, "channel|") {
		t.Fatalf("channel tag leaked into SSE output:\n%s", result)
	}
	if !strings.Contains(result, "Let's start with BacktrackingSolver.") {
		t.Fatalf("prose content missing from output:\n%s", result)
	}
	if !strings.Contains(result, "write_to_file") {
		t.Fatalf("tool call missing from output:\n%s", result)
	}
}

func TestStreamNormalizer_ToolCall_CycleBreaking(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxToolTokens:       8192,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	// Repeating tool call arguments
	argPhrase := "execute migration task systematically on cluster node "
	rawSSE := fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"bash\",\"arguments\":\"%s\"}}]}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"%s\"}}]}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"%s\"}}]}}]}\n\n"+
			"data: [DONE]\n\n",
		argPhrase, argPhrase, argPhrase,
	)

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 256)
	violated := false
	var violationReason string
	for {
		_, err := norm.Read(buf)
		if v, reason := norm.CheckCycleViolation(); v {
			violated = true
			violationReason = reason
			break
		}
		if err != nil {
			break
		}
	}

	if !violated {
		t.Fatalf("expected tool argument repetition to trigger cycle violation")
	}
	if violationReason != "tool_repetition_loop_detected" {
		t.Fatalf("expected reason tool_repetition_loop_detected, got: %s", violationReason)
	}
	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0, got %d", cb.ToolTokens())
	}
}

func TestStreamNormalizer_ClineEditor_ImmunityToFileWrites(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	// Repeating table-driven test boilerplate that previously caused false-positive cycle kills in Cline v4
	tableTestBoilerplate := "for _, tt := range tests { t.Run(tt.name, func(t *testing.T) { "
	rawSSE := fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"editor\",\"arguments\":\"{\\\"path\\\":\\\"solver_test.go\\\",\\\"file_text\\\":\\\"\"}}]}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"%s\"}}]}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"%s\"}}]}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"%s\"}}]}}]}\n\n"+
			"data: [DONE]\n\n",
		tableTestBoilerplate, tableTestBoilerplate, tableTestBoilerplate,
	)

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 256)
	violated := false
	for {
		_, err := norm.Read(buf)
		if v, _ := norm.CheckCycleViolation(); v {
			violated = true
			break
		}
		if err != nil {
			break
		}
	}

	if violated {
		t.Fatalf("expected Cline editor table-driven tests write to be immune from cycle violation, but was severed")
	}
	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0, got %d", cb.ToolTokens())
	}
}

func TestStreamNormalizer_ClineXML_WriteToFile_Immunity(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	tableTestBoilerplate := "for _, tt := range tests { t.Run(tt.name, func(t *testing.T) { "
	rawSSE := fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"content\":\"I will create the test suite now.\\n<write_to_file>\\n<path>board_test.go</path>\\n<content>\\n\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"</content>\\n</write_to_file>\"}}]}\n\n"+
			"data: [DONE]\n\n",
		tableTestBoilerplate, tableTestBoilerplate, tableTestBoilerplate,
	)

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 256)
	violated := false
	for {
		_, err := norm.Read(buf)
		if v, _ := norm.CheckCycleViolation(); v {
			violated = true
			break
		}
		if err != nil {
			break
		}
	}

	if violated {
		t.Fatalf("expected Cline <write_to_file> XML stream to be immune from cycle violation, but was severed")
	}
	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0 inside <write_to_file>, got %d", cb.ToolTokens())
	}
}

func TestStreamNormalizer_ClineXML_Editor_Immunity(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	matrixRow := ". . 1 . 1 . 1 .\n"
	rawSSE := fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"content\":\"<editor>\\n<command>create</command>\\n<path>board.go</path>\\n<file_text>\\n\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"</file_text>\\n</editor>\"}}]}\n\n"+
			"data: [DONE]\n\n",
		matrixRow, matrixRow, matrixRow,
	)

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 256)
	violated := false
	for {
		_, err := norm.Read(buf)
		if v, _ := norm.CheckCycleViolation(); v {
			violated = true
			break
		}
		if err != nil {
			break
		}
	}

	if violated {
		t.Fatalf("expected Cline <editor> XML stream to be immune from cycle violation, but was severed")
	}
	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0 inside <editor>, got %d", cb.ToolTokens())
	}
}

func TestStreamNormalizer_ClineXML_TransitionBackToProse(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	validCode := "package board\n\ntype Board struct {}\n"
	loopingProse := "I am analyzing the error now please wait. "
	rawSSE := fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"content\":\"<write_to_file>\\n<path>board.go</path>\\n<content>\\n%s</content>\\n</write_to_file>\\n\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: [DONE]\n\n",
		validCode, loopingProse, loopingProse, loopingProse,
	)

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 256)
	violated := false
	var violationReason string
	for {
		_, err := norm.Read(buf)
		if v, r := norm.CheckCycleViolation(); v {
			violated = true
			violationReason = r
			break
		}
		if err != nil {
			break
		}
	}

	if !violated {
		t.Fatalf("expected prose loop following closed XML tool tag to trigger cycle killer, but it passed")
	}
	if violationReason != "ngram_repetition_loop_detected" {
		t.Errorf("expected ngram_repetition_loop_detected, got %s", violationReason)
	}
}

func TestStreamNormalizer_ClineXML_ChunkSplitTags(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	tableTestBoilerplate := "for _, tt := range tests { t.Run(tt.name, func(t *testing.T) { "
	rawSSE := fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"content\":\"<write_\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"to_file>\\n<content>\\n\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"</write_\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"to_file>\"}}]}\n\n"+
			"data: [DONE]\n\n",
		tableTestBoilerplate, tableTestBoilerplate, tableTestBoilerplate,
	)

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 256)
	violated := false
	for {
		_, err := norm.Read(buf)
		if v, _ := norm.CheckCycleViolation(); v {
			violated = true
			break
		}
		if err != nil {
			break
		}
	}

	if violated {
		t.Fatalf("expected chunk-split Cline <write_to_file> stream to be immune from cycle violation, but was severed")
	}
	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0 inside chunk-split <write_to_file>, got %d", cb.ToolTokens())
	}
}

func TestStreamNormalizer_ClineXML_InWriteToolAndNonWriteTags(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled: &enabled,
	})

	rawSSE := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello <custom_tag>inside custom</custom_tag> normal prose\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"<write_to_file>\\n<path>foo.go</path>\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"package main\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"</write_to_file>\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"Back to prose now.<write_\"}}]}\n\n" +
		"data: [DONE]\n\n"

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 512)
	for {
		_, err := norm.Read(buf)
		if err != nil {
			break
		}
	}

	if norm.InWriteTool() {
		t.Errorf("expected InWriteTool() to be false after stream finished")
	}
	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0")
	}
	if cb.ProseTokens() == 0 {
		t.Errorf("expected ProseTokens > 0")
	}
}

func TestStreamNormalizer_ClineXML_MultipleWriteTagsAndEscapes(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled: &enabled,
	})

	rawSSE := "data: {\"choices\":[{\"delta\":{\"content\":\"<replace_in_file>\\n<path>a.go</path>\\ncodeA\\n</replace_in_file>\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"Between writes\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"<edit_file>\\ncodeB\\n</edit_file>\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"<create_file>\\ncodeC\\n</create_file>\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"<patch_file>\\ncodeD\\n</patch_file>\\n\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"Final thought\\n\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 512)
	for {
		_, err := norm.Read(buf)
		if err != nil {
			break
		}
	}

	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0 for multiple write tags")
	}
}

func TestStreamNormalizer_ClineXML_UnclosedTagAndIncompleteTagAtDone(t *testing.T) {
	enabled := true
	cb := shield.NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled: &enabled,
	})

	// Stream ends with unclosed tag
	rawSSE := "data: {\"choices\":[{\"delta\":{\"content\":\"<write_to_file>\\ncontent\"}}]}\n\n" +
		"data: [DONE]\n\n"

	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	norm.SetCycleBreaker(cb)
	defer norm.Close()

	buf := make([]byte, 512)
	for {
		_, err := norm.Read(buf)
		if err != nil {
			break
		}
	}

	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0")
	}
}

func TestStreamNormalizer_ClineXML_PartialTagHelpers(t *testing.T) {
	// Test findPartialOpenTagSuffix
	if p := findPartialOpenTagSuffix("hello <write_"); p != "<write_" {
		t.Errorf("expected <write_, got %s", p)
	}
	if p := findPartialOpenTagSuffix("x < 10"); p != "" {
		t.Errorf("expected empty string for x < 10, got %s", p)
	}
	if p := findPartialOpenTagSuffix("done </write_to_file>"); p != "" {
		t.Errorf("expected empty string for closed tag, got %s", p)
	}

	// Test findPartialCloseTagSuffix
	if p := findPartialCloseTagSuffix("code </edit_"); p != "</edit_" {
		t.Errorf("expected </edit_, got %s", p)
	}
	if p := findPartialCloseTagSuffix("code </div"); p != "" {
		t.Errorf("expected empty string for non-write close tag, got %s", p)
	}

	// Test findEarliestOpenTag with attributes and whitespace
	start, end, name := findEarliestOpenTag("prefix <write_to_file path=\"x\">content")
	if start != 7 || end != 31 || name != "write_to_file" {
		t.Errorf("unexpected open tag result: %d, %d, %s", start, end, name)
	}

	// Test findEarliestCloseTag
	start, end, name = findEarliestCloseTag("prefix </write_to_file>suffix")
	if start != 7 || end != 23 || name != "write_to_file" {
		t.Errorf("unexpected close tag result: %d, %d, %s", start, end, name)
	}
}

func TestStreamNormalizer_FastPassBranches(t *testing.T) {
	// 1. fastPassProse with tool_calls
	norm := NewStreamNormalizer(io.NopCloser(strings.NewReader("")))
	defer norm.Close()

	lineWithTool := []byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"test_fn\",\"arguments\":\"{}\"}}]}}]}\n\n")
	payloadWithTool := lineWithTool[6 : len(lineWithTool)-2]
	norm.fastPassProse(lineWithTool, payloadWithTool)

	// 2. fastPassProse with partial tag in content
	lineWithPartial := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"prefix <write_\"}}]}\n\n")
	payloadWithPartial := lineWithPartial[6 : len(lineWithPartial)-2]
	norm.fastPassProse(lineWithPartial, payloadWithPartial)
	if norm.pendingWriteTagBuf != "<write_" {
		t.Errorf("expected pendingWriteTagBuf <write_, got %s", norm.pendingWriteTagBuf)
	}

	// 3. fastPassWrite with tool_calls
	norm.inWriteTool = true
	norm.fastPassWrite(lineWithTool, payloadWithTool)

	// 4. fastPassWrite with partial close tag in content
	lineWithPartialClose := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"code </write_\"}}]}\n\n")
	payloadWithPartialClose := lineWithPartialClose[6 : len(lineWithPartialClose)-2]
	norm.fastPassWrite(lineWithPartialClose, payloadWithPartialClose)
	if norm.pendingWriteTagBuf != "</write_" {
		t.Errorf("expected pendingWriteTagBuf </write_, got %s", norm.pendingWriteTagBuf)
	}
}

func TestProxy_FormatCycleKillReasonAndGetters(t *testing.T) {
	if r := formatCycleKillReason("thinking_repetition_loop_detected"); r != "repetitive reasoning loop detected" {
		t.Errorf("unexpected format: %s", r)
	}
	if r := formatCycleKillReason("prose_budget_exceeded_with_repetition"); r != "prose token budget exceeded with repetition" {
		t.Errorf("unexpected format: %s", r)
	}
	if r := formatCycleKillReason("thinking_budget_exceeded_with_repetition"); r != "thinking token budget exceeded with repetition" {
		t.Errorf("unexpected format: %s", r)
	}
	if r := formatCycleKillReason("custom_error_code"); r != "custom error code" {
		t.Errorf("unexpected format: %s", r)
	}

	srv := &Server{}
	srv.SetLastDiskWriteUnixNano(12345)
	if srv.GetLastDiskWriteUnixNano() != 12345 {
		t.Errorf("expected 12345, got %d", srv.GetLastDiskWriteUnixNano())
	}
}



