// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
)

var (
	bufPool = sync.Pool{
		New: func() any {
			return new(bytes.Buffer)
		},
	}
	readerPool = sync.Pool{
		New: func() any {
			return bufio.NewReaderSize(nil, 64*1024)
		},
	}

	// tagReplacer performs single-pass canonicalization and stripping of model-specific
	// reasoning delimiters and chat template artifacts (Qwen, Claude, Gemma).
	tagReplacer = strings.NewReplacer(
		// Longest/most specific canonical think open tags first
		"<|channel|>thought", "<think>",
		"<|channel>thought", "<think>",
		"<channel|thought>", "<think>",
		"<channel|thought", "<think>",
		"<|im_start|>think", "<think>",
		"<|im_start|>thought", "<think>",
		"<thinking>", "<think>",
		// Canonical think close tags
		"</thinking>", "</think>",
		"</thought>", "</think>",
		"<|im_end|>", "</think>",
		// Specific call prefixes
		"<|call:", "",
		"<call:", "",
		// Stripped turn/channel delimiters (longer first)
		"<channel|>", "",
		"<|channel|>", "",
		"<|channel>", "",
		"<channel|", "",
		"<|channel|", "",
		"<channel>", "",
		"</channel>", "",
		"<start_of_turn>", "",
		"<end_of_turn>", "",
	)

	// reasoningByteMarkers are byte signatures that indicate potential reasoning content
	// or model chat template delimiters requiring full chunk parsing rather than fast-path line passthrough.
	reasoningByteMarkers = [][]byte{
		[]byte("reasoning_content"),
		[]byte("\"reasoning\""),
		[]byte("\"reason\""),
		// Delimiters (both raw and escaped)
		[]byte("<think>"),
		[]byte("</think>"),
		[]byte("<thinking>"),
		[]byte("</thinking>"),
		[]byte("</thought>"),
		[]byte("<|im_start|>"),
		[]byte("<|im_end|>"),
		[]byte("<start_of_turn>"),
		[]byte("<end_of_turn>"),
		[]byte("start_of_turn"),
		[]byte("end_of_turn"),
		[]byte("<channel"),
		[]byte("channel|"),
		[]byte("|channel"),
		// JSON / HTML unicode escaped delimiters (\u003c / \u003C / \u003e)
		[]byte(`\u003cthink`),
		[]byte(`\u003Cthink`),
		[]byte(`\u003c/think`),
		[]byte(`\u003C/think`),
		[]byte(`\u003cthinking`),
		[]byte(`\u003Cthinking`),
		[]byte(`\u003c/thinking`),
		[]byte(`\u003C/thinking`),
		[]byte(`\u003c/thought`),
		[]byte(`\u003C/thought`),
		[]byte(`\u003c|im_start`),
		[]byte(`\u003C|im_start`),
		[]byte(`\u003c|im_end`),
		[]byte(`\u003C|im_end`),
		[]byte(`\u003cchannel`),
		[]byte(`\u003Cchannel`),
		[]byte(`\u003c|channel`),
		[]byte(`\u003C|channel`),
	}
)

func hasAnyReasoningMarker(payload []byte) bool {
	for _, marker := range reasoningByteMarkers {
		if bytes.Contains(payload, marker) {
			return true
		}
	}
	return false
}

type fastDelta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Reasoning        string          `json:"reasoning,omitempty"`
	Reason           string          `json:"reason,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
}

type fastToolCallChunk struct {
	Index    int `json:"index"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type fastStreamChoice struct {
	Index        int             `json:"index"`
	Delta        fastDelta       `json:"delta"`
	FinishReason *string         `json:"finish_reason,omitempty"`
	Logprobs     json.RawMessage `json:"logprobs,omitempty"`
}

type fastStreamChunk struct {
	ID                string             `json:"id,omitempty"`
	Object            string             `json:"object,omitempty"`
	Created           int64              `json:"created,omitempty"`
	Model             string             `json:"model,omitempty"`
	Choices           []fastStreamChoice `json:"choices"`
	SystemFingerprint string             `json:"system_fingerprint,omitempty"`
	Usage             json.RawMessage    `json:"usage,omitempty"`
}

// PromptTokensDetails captures per-category token breakdowns from upstream providers.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// StreamUsage records token usage metrics captured from streaming chunks or estimated via fallback.
type StreamUsage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	Cost                float64              `json:"cost,omitempty"`
}

// StreamNormalizer wraps an upstream SSE response stream, canonicalizes reasoning tokens
// into standard delta.reasoning_content, intercepts token usage, and enforces the Agentic Tool Fallback Shield.
type StreamNormalizer struct {
	upstream               io.ReadCloser
	reader                 *bufio.Reader
	outBuf                 *bytes.Buffer
	inThinking             bool
	inStructuredReasoning  bool
	alreadyTagged          bool // Preserved for backward test compatibility
	closed                 bool
	eofReached             bool
	capturedUsage          StreamUsage
	hasUsage               bool
	emittedCompletionChars int
	interactiveTool        string
	shieldMgr              *shield.ShieldManager
	tailBuffer             *shield.TailBuffer
	hasNativeTools         bool
	proseAccumulator       strings.Builder
	cycleBreaker           *shield.CycleBreaker
	cycleViolated          bool
	cycleViolationReason   string
	features               uint16
	pendingTagClosing      bool
	toolChunksScratch      []fastToolCallChunk
	currentToolCategory    shield.ToolActivityCategory
	hasActiveToolCall      bool
}

// NewStreamNormalizer constructs a new StreamNormalizer for an SSE io.ReadCloser.
func NewStreamNormalizer(r io.ReadCloser) *StreamNormalizer {
	br := readerPool.Get().(*bufio.Reader)
	br.Reset(r)
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()

	return &StreamNormalizer{
		upstream: r,
		reader:   br,
		outBuf:   buf,
		features: uint16(router.FeatureDefaultAll),
	}
}

// SetFeatures configures active FeatureFlags on the stream normalizer.
func (s *StreamNormalizer) SetFeatures(features uint16) {
	s.features = features
}

// SetShield attaches an Agentic Tool Fallback Shield to this streaming session.
func (s *StreamNormalizer) SetShield(interactiveTool string, mgr *shield.ShieldManager) {
	s.interactiveTool = interactiveTool
	s.shieldMgr = mgr
	if interactiveTool != "" && mgr != nil && s.tailBuffer == nil {
		s.tailBuffer = shield.GetTailBuffer()
	}
}

// SetCycleBreaker attaches an active inference CycleBreaker to this streaming session.
func (s *StreamNormalizer) SetCycleBreaker(cb *shield.CycleBreaker) {
	s.cycleBreaker = cb
}

// CheckCycleViolation reports whether the stream triggered a cycle breaker violation.
func (s *StreamNormalizer) CheckCycleViolation() (bool, string) {
	return s.cycleViolated, s.cycleViolationReason
}

// HasActiveToolCall returns whether a tool call delta stream is currently active.
func (s *StreamNormalizer) HasActiveToolCall() bool {
	return s.hasActiveToolCall
}

func (s *StreamNormalizer) Read(p []byte) (n int, err error) {
	if s.closed {
		return 0, io.EOF
	}

	for s.outBuf.Len() == 0 && !s.eofReached {
		line, readErr := s.reader.ReadBytes('\n')
		if len(line) > 0 {
			s.processLine(line)
		}
		if readErr != nil {
			if readErr == io.EOF {
				s.eofReached = true
				s.inThinking = false
				s.inStructuredReasoning = false
			} else {
				return 0, readErr
			}
		}
	}

	if s.outBuf.Len() > 0 {
		return s.outBuf.Read(p)
	}

	if s.eofReached {
		return 0, io.EOF
	}

	return 0, nil
}

// processLine frames, inspects, and routes an individual SSE stream line.
func (s *StreamNormalizer) processLine(line []byte) {
	trimmed := bytes.TrimRight(line, "\r\n")

	// Raw pass-through fast path
	if s.features == uint16(router.FeatureRawPassThrough) {
		s.outBuf.Write(line)
		return
	}

	// Support both "data: " (standard) and "data:" (no space) per SSE spec.
	// Lumo/Proton sends "data:{...}" without the space, which previously
	// caused the entire line to bypass captureUsage() silently.
	var payload []byte
	switch {
	case bytes.HasPrefix(trimmed, []byte("data: ")):
		payload = trimmed[6:] // len("data: ") == 6
	case bytes.HasPrefix(trimmed, []byte("data:")):
		payload = trimmed[5:] // len("data:") == 5
	default:
		s.outBuf.Write(line)
		return
	}
	if bytes.Equal(payload, []byte("[DONE]")) {
		s.handleDone(line)
		return
	}

	if bytes.Contains(payload, []byte("\"tool_calls\"")) {
		s.hasNativeTools = true
	}

	if bytes.Contains(payload, []byte("\"usage\"")) {
		s.captureUsage(payload)
	}

	// Fast path: if outside reasoning, no pending tag to close, and no markers detected, pass line through directly
	if !s.inThinking && !s.pendingTagClosing && !hasAnyReasoningMarker(payload) {
		s.fastPassProse(line, payload)
		return
	}

	s.processChunkLine(line, payload)
}

// fastPassProse handles prose chunks that require no JSON transformations.
func (s *StreamNormalizer) fastPassProse(line, payload []byte) {
	if bytes.Contains(payload, []byte("\"tool_calls\"")) {
		s.processChunkLine(line, payload)
		return
	}
	if contentStr := payloadContent(payload); contentStr != "" {
		s.recordProse(contentStr)
	}
	s.outBuf.Write(line)
}

// processChunkLine parses a JSON streaming chunk and canonicalizes reasoning vs content deltas.
func (s *StreamNormalizer) processChunkLine(line, payload []byte) {
	var chunk fastStreamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil || len(chunk.Choices) == 0 {
		s.outBuf.Write(line)
		return
	}

	choice := &chunk.Choices[0]
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		s.hasActiveToolCall = false
	}
	if len(choice.Delta.ToolCalls) > 0 {
		s.extractAndRecordToolCalls(choice.Delta.ToolCalls)
	}

	reasoningText := s.resolveReasoningField(&choice.Delta)

	if reasoningText != "" {
		s.inThinking = true
		s.inStructuredReasoning = true
		choice.Delta.ReasoningContent = reasoningText
		choice.Delta.Reasoning = ""
		choice.Delta.Reason = ""

		s.recordReasoning(reasoningText)
		if choice.Delta.Content != "" {
			s.recordProse(choice.Delta.Content)
		}

		s.emitChunk(chunk)
		return
	}

	if choice.Delta.Content != "" {
		s.normalizeContentDelta(choice)
		s.emitChunk(chunk)
		return
	}

	s.outBuf.Write(line)
}

// resolveReasoningField extracts reasoning text from any provider-specific field.
func (s *StreamNormalizer) resolveReasoningField(delta *fastDelta) string {
	if delta.ReasoningContent != "" {
		return delta.ReasoningContent
	}
	if delta.Reasoning != "" {
		return delta.Reasoning
	}
	return delta.Reason
}

// normalizeContentDelta extracts text-embedded think tags into reasoning_content.
func (s *StreamNormalizer) normalizeContentDelta(choice *fastStreamChoice) {
	if s.inStructuredReasoning {
		s.inStructuredReasoning = false
		s.inThinking = false
	}

	raw := choice.Delta.Content
	if s.pendingTagClosing {
		s.pendingTagClosing = false
		if strings.HasPrefix(raw, ">") {
			raw = strings.TrimPrefix(raw, ">")
		}
	}

	if strings.HasSuffix(raw, "<channel|") || strings.HasSuffix(raw, "<|channel|") || strings.HasSuffix(raw, "<|channel") {
		s.pendingTagClosing = true
	}

	content := tagReplacer.Replace(raw)
	var reasoningDelta string
	var proseDelta string

	if s.inThinking {
		if strings.Contains(content, "</think>") {
			parts := strings.SplitN(content, "</think>", 2)
			reasoningDelta = parts[0]
			proseDelta = parts[1]
			s.inThinking = false
		} else {
			reasoningDelta = content
		}
	} else if strings.Contains(content, "<think>") {
		parts := strings.SplitN(content, "<think>", 2)
		prefix := parts[0]
		after := parts[1]
		if strings.Contains(after, "</think>") {
			subParts := strings.SplitN(after, "</think>", 2)
			reasoningDelta = subParts[0]
			proseDelta = prefix + subParts[1]
			s.inThinking = false
		} else {
			reasoningDelta = after
			proseDelta = prefix
			s.inThinking = true
		}
	} else {
		proseDelta = content
	}

	choice.Delta.ReasoningContent = reasoningDelta
	choice.Delta.Content = proseDelta
	choice.Delta.Reasoning = ""
	choice.Delta.Reason = ""

	s.recordReasoning(reasoningDelta)
	s.recordProse(proseDelta)
}

// emitChunk serializes and writes a modified SSE chunk into the output buffer.
func (s *StreamNormalizer) emitChunk(chunk fastStreamChunk) {
	if newPayload, err := marshalNoEscapeHTML(chunk); err == nil {
		s.outBuf.WriteString("data: ")
		s.outBuf.Write(newPayload)
		s.outBuf.WriteString("\n\n")
	}
}

// handleDone finalizes the stream on [DONE] and synthesizes an agent tool call if required.
func (s *StreamNormalizer) handleDone(doneLine []byte) {
	s.inThinking = false
	s.inStructuredReasoning = false
	s.pendingTagClosing = false
	s.hasActiveToolCall = false

	shieldEnabled := (s.features & uint16(router.FeatureShieldEnabled)) != 0
	if shieldEnabled && !s.hasNativeTools && s.interactiveTool != "" && s.shieldMgr != nil && s.tailBuffer != nil {
		if matched, _ := s.shieldMgr.RuleEngine().Evaluate(s.tailBuffer.Bytes()); matched {
			if synthCall, ok := s.shieldMgr.EvaluateAndSynthesize(s.proseAccumulator.String(), s.interactiveTool); ok && synthCall != nil {
				s.emitSyntheticToolCall(synthCall)
			}
		}
	}

	s.outBuf.Write(doneLine)
}

// emitSyntheticToolCall injects synthetic tool_calls and finish_reason SSE chunks.
func (s *StreamNormalizer) emitSyntheticToolCall(synthCall *shield.RawToolCall) {
	deltaJSON, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []any{
						map[string]any{
							"index": 0,
							"id":    synthCall.ID,
							"type":  synthCall.Type,
							"function": map[string]string{
								"name":      synthCall.Function.Name,
								"arguments": synthCall.Function.Arguments,
							},
						},
					},
				},
			},
		},
	})
	s.outBuf.WriteString("data: ")
	s.outBuf.Write(deltaJSON)
	s.outBuf.WriteString("\n\n")

	finishJSON, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "tool_calls",
			},
		},
	})
	s.outBuf.WriteString("data: ")
	s.outBuf.Write(finishJSON)
	s.outBuf.WriteString("\n\n")
}

// captureUsage extracts upstream usage metadata from the stream chunk.
func (s *StreamNormalizer) captureUsage(payload []byte) {
	var rawChunk struct {
		Usage *StreamUsage `json:"usage,omitempty"`
	}
	if json.Unmarshal(payload, &rawChunk) == nil && rawChunk.Usage != nil &&
		(rawChunk.Usage.PromptTokens > 0 || rawChunk.Usage.CompletionTokens > 0 || rawChunk.Usage.TotalTokens > 0) {
		s.capturedUsage = *rawChunk.Usage
		s.hasUsage = true
	}
}

// recordReasoning feeds reasoning tokens into character counting and the cycle breaker.
func (s *StreamNormalizer) recordReasoning(text string) {
	if text == "" {
		return
	}
	s.emittedCompletionChars += len(text)
	if s.cycleBreaker != nil && !s.cycleViolated {
		if triggered, reason := s.cycleBreaker.ProcessDelta(text, true); triggered {
			s.cycleViolated = true
			s.cycleViolationReason = reason
		}
	}
}

// recordProse feeds prose tokens into buffers, character counting, and the cycle breaker.
func (s *StreamNormalizer) recordProse(text string) {
	if text == "" {
		return
	}
	s.emittedCompletionChars += len(text)
	if s.tailBuffer != nil {
		s.tailBuffer.Append([]byte(text))
		s.proseAccumulator.WriteString(text)
	}
	if s.cycleBreaker != nil && !s.cycleViolated {
		if triggered, reason := s.cycleBreaker.ProcessDelta(text, false); triggered {
			s.cycleViolated = true
			s.cycleViolationReason = reason
		}
	}
}

func isStructuredWriteTool(name string) bool {
	switch strings.ToLower(name) {
	case "write_to_file", "replace_file_content", "multi_replace_file_content",
		"apply_diff", "create_file", "edit_file", "write_file", "save_file",
		"create_or_update_file", "patch_file":
		return true
	default:
		return false
	}
}

// recordToolDelta feeds tool argument tokens into character counting and the cycle breaker tool lane.
func (s *StreamNormalizer) recordToolDelta(argText string) {
	if argText == "" {
		return
	}
	s.emittedCompletionChars += len(argText)
	if s.cycleBreaker != nil && !s.cycleViolated {
		if triggered, reason := s.cycleBreaker.ProcessToolDelta(argText, s.currentToolCategory); triggered {
			s.cycleViolated = true
			s.cycleViolationReason = reason
		}
	}
}

func (s *StreamNormalizer) extractAndRecordToolCalls(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	s.toolChunksScratch = s.toolChunksScratch[:0]
	if err := json.Unmarshal(raw, &s.toolChunksScratch); err == nil {
		for i := range s.toolChunksScratch {
			s.hasActiveToolCall = true
			if s.toolChunksScratch[i].Function.Name != "" {
				name := s.toolChunksScratch[i].Function.Name
				if isStructuredWriteTool(name) {
					s.currentToolCategory = shield.ToolCategoryFileWrite
				} else {
					s.currentToolCategory = shield.ToolCategoryCommand
				}
			}
			args := s.toolChunksScratch[i].Function.Arguments
			if args != "" {
				if s.currentToolCategory == shield.ToolCategoryCommand && router.DetectShellWrite(args) {
					s.currentToolCategory = shield.ToolCategoryFileWrite
				}
				s.recordToolDelta(args)
			}
		}
	}
}

// extractContentFast delegates to payloadContent to ensure full JSON unescaping without data corruption.
func extractContentFast(payload []byte) string {
	return payloadContent(payload)
}

func payloadContent(payload []byte) string {
	var c fastStreamChunk
	if err := json.Unmarshal(payload, &c); err == nil && len(c.Choices) > 0 {
		return c.Choices[0].Delta.Content
	}
	return ""
}

func marshalNoEscapeHTML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Close closes the underlying stream and returns pooled buffers.
func (s *StreamNormalizer) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.hasActiveToolCall = false
	var err error
	if s.upstream != nil {
		err = s.upstream.Close()
	}
	if s.outBuf != nil {
		s.outBuf.Reset()
		bufPool.Put(s.outBuf)
		s.outBuf = nil
	}
	if s.reader != nil {
		s.reader.Reset(nil)
		readerPool.Put(s.reader)
		s.reader = nil
	}
	if s.tailBuffer != nil {
		shield.PutTailBuffer(s.tailBuffer)
		s.tailBuffer = nil
	}
	if s.cycleBreaker != nil {
		shield.PutCycleBreaker(s.cycleBreaker)
		s.cycleBreaker = nil
	}
	return err
}

// GetUsage returns captured upstream usage metrics or fallback estimated completion tokens.
func (s *StreamNormalizer) GetUsage() (StreamUsage, bool) {
	if s.hasUsage {
		return s.capturedUsage, true
	}
	if s.emittedCompletionChars > 0 {
		estTokens := s.emittedCompletionChars / 4
		if estTokens == 0 {
			estTokens = 1
		}
		return StreamUsage{
			CompletionTokens: estTokens,
			TotalTokens:      estTokens,
		}, false
	}
	return StreamUsage{}, false
}
