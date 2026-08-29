package server

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
)

func TestStreamNormalizer_CycleBreaker_Detection(t *testing.T) {
	enabled := true
	cbCfg := &contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      800,
		RepetitionWindow:    5,
		RepetitionThreshold: 3,
	}
	cb := shield.NewCycleBreaker(cbCfg)

	// Stream of SSE chunks with repetitive phrase
	streamData := ""
	for i := 0; i < 6; i++ {
		streamData += "data: {\"choices\":[{\"delta\":{\"content\":\"Let us check the directory structure now. \"}}]}\n\n"
	}
	streamData += "data: [DONE]\n\n"

	reader := io.NopCloser(bytes.NewReader([]byte(streamData)))
	normalizer := NewStreamNormalizer(reader)
	normalizer.SetCycleBreaker(cb)

	buf := make([]byte, 1024)
	for {
		_, err := normalizer.Read(buf)
		if err != nil {
			break
		}
	}

	violated, reason := normalizer.CheckCycleViolation()
	if !violated {
		t.Fatalf("expected stream normalizer to report cycle violation")
	}
	if !strings.Contains(reason, "repetition") {
		t.Fatalf("expected repetition reason, got %s", reason)
	}
}

func TestStreamNormalizer_CycleBreaker_ReasoningContent_Detection(t *testing.T) {
	enabled := true
	cbCfg := &contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           5000,
		RepetitionWindow:            5,
		ThinkingRepetitionThreshold: 5,
	}
	cb := shield.NewCycleBreaker(cbCfg)

	// Stream of SSE chunks with repetitive structured reasoning_content
	streamData := ""
	for i := 0; i < 6; i++ {
		streamData += "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"I should check the backtracking logic again carefully. \"}}]}\n\n"
	}
	streamData += "data: [DONE]\n\n"

	reader := io.NopCloser(bytes.NewReader([]byte(streamData)))
	normalizer := NewStreamNormalizer(reader)
	normalizer.SetCycleBreaker(cb)

	buf := make([]byte, 1024)
	for {
		_, err := normalizer.Read(buf)
		if err != nil {
			break
		}
	}

	violated, reason := normalizer.CheckCycleViolation()
	if !violated {
		t.Fatalf("expected stream normalizer to report cycle violation in reasoning_content")
	}
	if reason != "thinking_repetition_loop_detected" {
		t.Fatalf("expected thinking_repetition_loop_detected, got %s", reason)
	}
}

func TestStreamNormalizer_CycleBreaker_Gemma4_ChannelThought(t *testing.T) {
	enabled := true
	cbCfg := &contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           5000,
		RepetitionWindow:            5,
		ThinkingRepetitionThreshold: 5,
	}
	cb := shield.NewCycleBreaker(cbCfg)

	// Gemma 4 emits <|channel>thought ... </thought>
	streamData := "data: {\"choices\":[{\"delta\":{\"content\":\"<|channel>thought\\n\"}}]}\n\n"
	for i := 0; i < 6; i++ {
		streamData += "data: {\"choices\":[{\"delta\":{\"content\":\"Let me double check the N-queens bitwise mask row. \"}}]}\n\n"
	}
	streamData += "data: {\"choices\":[{\"delta\":{\"content\":\"</thought>\\n\"}}]}\n\n"
	streamData += "data: [DONE]\n\n"

	reader := io.NopCloser(bytes.NewReader([]byte(streamData)))
	normalizer := NewStreamNormalizer(reader)
	normalizer.SetCycleBreaker(cb)

	buf := make([]byte, 1024)
	for {
		_, err := normalizer.Read(buf)
		if err != nil {
			break
		}
	}

	violated, reason := normalizer.CheckCycleViolation()
	if !violated {
		t.Fatalf("expected stream normalizer to report cycle violation for Gemma 4 channel thought")
	}
	if reason != "thinking_repetition_loop_detected" {
		t.Fatalf("expected thinking_repetition_loop_detected, got %s", reason)
	}
}

func TestStreamNormalizer_Gemma4_ChannelThought_Normalization(t *testing.T) {
	rawSSE := "data: {\"choices\":[{\"delta\":{\"content\":\"<|channel>thought\\nReasoning about the problem...\\n</thought>\\nHere is the answer.\"}}]}\n\ndata: [DONE]\n\n"
	reader := io.NopCloser(strings.NewReader(rawSSE))
	normalizer := NewStreamNormalizer(reader)
	defer normalizer.Close()

	out, err := io.ReadAll(normalizer)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	result := string(out)
	if !strings.Contains(result, "<think>") {
		t.Fatalf("expected <think> tag in normalized output, got:\n%s", result)
	}
	if !strings.Contains(result, "</think>") {
		t.Fatalf("expected </think> tag in normalized output, got:\n%s", result)
	}
	if strings.Contains(result, "<|channel>thought") || strings.Contains(result, "</thought>") {
		t.Fatalf("raw Gemma 4 channel tags should be stripped/normalized, got:\n%s", result)
	}
}

func TestStreamNormalizer_CycleBreaker_CleanStream(t *testing.T) {
	enabled := true
	cbCfg := &contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      800,
		RepetitionWindow:    5,
		RepetitionThreshold: 3,
	}
	cb := shield.NewCycleBreaker(cbCfg)

	streamData := "data: {\"choices\":[{\"delta\":{\"content\":\"package main\\n\\nfunc main() {}\\n\"}}]}\n\n"
	streamData += "data: [DONE]\n\n"

	reader := io.NopCloser(bytes.NewReader([]byte(streamData)))
	normalizer := NewStreamNormalizer(reader)
	normalizer.SetCycleBreaker(cb)

	buf := make([]byte, 1024)
	for {
		_, err := normalizer.Read(buf)
		if err != nil {
			break
		}
	}

	violated, reason := normalizer.CheckCycleViolation()
	if violated {
		t.Fatalf("clean stream should not trigger violation, got: %s", reason)
	}
}
