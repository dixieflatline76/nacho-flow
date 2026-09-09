package server

import (
	"io"
	"strings"
	"testing"
)

func TestStreamNormalizer_NoSpacePrefix_UsageExtracted(t *testing.T) {
	rawSSE := `data:{"id":"gen-lumo-1","choices":[{"index":0,"delta":{"content":"Hello"}}]}

data:{"id":"gen-lumo-1","choices":[],"usage":{"prompt_tokens":68,"completion_tokens":11,"total_tokens":79}}

data:[DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	outBytes, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	outStr := string(outBytes)
	if !strings.Contains(outStr, "Hello") {
		t.Errorf("expected output to contain 'Hello', got: %s", outStr)
	}

	usage, ok := norm.GetUsage()
	if !ok {
		t.Fatalf("expected GetUsage() to return true, got false")
	}
	if usage.PromptTokens != 68 {
		t.Errorf("expected 68 prompt tokens, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 11 {
		t.Errorf("expected 11 completion tokens, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 79 {
		t.Errorf("expected 79 total tokens, got %d", usage.TotalTokens)
	}
}

func TestStreamNormalizer_StandardPrefix_UsageStillWorks(t *testing.T) {
	rawSSE := `data: {"id":"gen-std-1","choices":[{"index":0,"delta":{"content":"Hi"}}]}

data: {"id":"gen-std-1","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"cost":0.0045,"prompt_tokens_details":{"cached_tokens":30}}}

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
	if usage.PromptTokens != 100 {
		t.Errorf("expected 100 prompt tokens, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 20 {
		t.Errorf("expected 20 completion tokens, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 120 {
		t.Errorf("expected 120 total tokens, got %d", usage.TotalTokens)
	}
	if usage.Cost != 0.0045 {
		t.Errorf("expected 0.0045 cost, got %f", usage.Cost)
	}
	if usage.PromptTokensDetails == nil {
		t.Fatalf("expected PromptTokensDetails to be non-nil")
	}
	if usage.PromptTokensDetails.CachedTokens != 30 {
		t.Errorf("expected 30 cached tokens, got %d", usage.PromptTokensDetails.CachedTokens)
	}
}

func TestStreamNormalizer_NoSpacePrefix_DoneHandled(t *testing.T) {
	rawSSE := `data:{"id":"gen-done-1","choices":[{"index":0,"delta":{"content":"OK"}}]}

data:[DONE]

`
	r := io.NopCloser(strings.NewReader(rawSSE))
	norm := NewStreamNormalizer(r)
	defer norm.Close()

	outBytes, err := io.ReadAll(norm)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	outStr := string(outBytes)
	if !strings.Contains(outStr, "[DONE]") {
		t.Errorf("expected output to contain '[DONE]', got: %s", outStr)
	}
	if !strings.Contains(outStr, "OK") {
		t.Errorf("expected output to contain 'OK', got: %s", outStr)
	}
}
