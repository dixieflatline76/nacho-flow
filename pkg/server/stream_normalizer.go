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

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
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

	tagReplacer          = agentregistry.DefaultRegistry().TagReplacer()
	reasoningByteMarkers = agentregistry.DefaultRegistry().ReasoningByteMarkers()
	writeTagByteMarkers  = agentregistry.DefaultRegistry().WriteTagByteMarkers()
	writeToolsList       = agentregistry.DefaultRegistry().WriteToolsList()
)

func hasAnyReasoningMarker(payload []byte) bool {
	for _, marker := range reasoningByteMarkers {
		if bytes.Contains(payload, marker) {
			return true
		}
	}
	return false
}

func hasAnyWriteMarker(payload []byte) bool {
	for _, marker := range writeTagByteMarkers {
		if bytes.Contains(payload, marker) {
			return true
		}
	}
	return false
}

func findEarliestOpenTag(s string) (startIdx, endIdx int, tagName string) {
	curr := s
	offset := 0
	for {
		idx := strings.IndexByte(curr, '<')
		if idx == -1 {
			return -1, -1, ""
		}
		actualStart := offset + idx
		// Check if it's a closing tag
		if idx+1 < len(curr) && curr[idx+1] == '/' {
			curr = curr[idx+1:]
			offset = actualStart + 1
			continue
		}

		rest := curr[idx+1:]
		nameEnd := 0
		for nameEnd < len(rest) {
			b := rest[nameEnd]
			if b == '>' || b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '/' {
				break
			}
			nameEnd++
		}

		if nameEnd > 0 {
			name := strings.ToLower(rest[:nameEnd])
			if agentregistry.DefaultRegistry().IsWriteTool(name) {
				gtIdx := strings.IndexByte(rest, '>')
				if gtIdx != -1 {
					return actualStart, actualStart + 1 + gtIdx + 1, name
				}
				return actualStart, -1, name
			}
		}

		curr = curr[idx+1:]
		offset = actualStart + 1
	}
}

func findEarliestCloseTag(s string) (startIdx, endIdx int, tagName string) {
	curr := s
	offset := 0
	for {
		idx := strings.Index(curr, "</")
		if idx == -1 {
			return -1, -1, ""
		}
		actualStart := offset + idx
		rest := curr[idx+2:]
		nameEnd := 0
		for nameEnd < len(rest) {
			b := rest[nameEnd]
			if b == '>' || b == ' ' || b == '\t' || b == '\r' || b == '\n' {
				break
			}
			nameEnd++
		}

		if nameEnd > 0 {
			name := strings.ToLower(rest[:nameEnd])
			if agentregistry.DefaultRegistry().IsWriteTool(name) {
				gtIdx := strings.IndexByte(rest, '>')
				if gtIdx != -1 {
					return actualStart, actualStart + 2 + gtIdx + 1, name
				}
				return actualStart, -1, name
			}
		}

		curr = curr[idx+2:]
		offset = actualStart + 2
	}
}

func findPartialOpenTagSuffix(s string) string {
	checkLen := len(s)
	if checkLen > 25 {
		checkLen = 25
	}
	tail := s[len(s)-checkLen:]
	ltIdx := strings.LastIndexByte(tail, '<')
	if ltIdx == -1 {
		return ""
	}
	candidate := tail[ltIdx:]
	if strings.ContainsRune(candidate, '>') {
		return ""
	}
	if strings.HasPrefix(candidate, "</") {
		return ""
	}

	afterLt := strings.ToLower(candidate[1:])
	for _, tag := range writeToolsList {
		if strings.HasPrefix(tag, afterLt) {
			return candidate
		}
	}
	return ""
}

func findPartialCloseTagSuffix(s string) string {
	checkLen := len(s)
	if checkLen > 25 {
		checkLen = 25
	}
	tail := s[len(s)-checkLen:]
	ltIdx := strings.LastIndex(tail, "</")
	if ltIdx == -1 {
		return ""
	}
	candidate := tail[ltIdx:]
	if strings.ContainsRune(candidate, '>') {
		return ""
	}

	afterSlash := strings.ToLower(candidate[2:])
	for _, tag := range writeToolsList {
		if strings.HasPrefix(tag, afterSlash) {
			return candidate
		}
	}
	return ""
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
	inWriteTool            bool
	pendingWriteTagBuf     string
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

// InWriteTool reports whether the normalizer is currently inside an XML write tool tag.
func (s *StreamNormalizer) InWriteTool() bool {
	return s.inWriteTool
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

	// Fast path: if outside reasoning, outside write tools, no pending tag to close, and no markers detected, pass line through directly
	if !s.inThinking && !s.inWriteTool && !s.pendingTagClosing && s.pendingWriteTagBuf == "" && !hasAnyReasoningMarker(payload) && !hasAnyWriteMarker(payload) {
		s.fastPassProse(line, payload)
		return
	}

	// Fast path: if inside write tools, outside reasoning, no pending tag to close, and no markers detected, pass line through directly as tool write
	if s.inWriteTool && !s.inThinking && !s.pendingTagClosing && s.pendingWriteTagBuf == "" && !hasAnyReasoningMarker(payload) && !hasAnyWriteMarker(payload) {
		s.fastPassWrite(line, payload)
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
		if partial := findPartialOpenTagSuffix(contentStr); partial != "" {
			s.routeContentDelta(contentStr)
			s.outBuf.Write(line)
			return
		}
		s.recordProse(contentStr)
	}
	s.outBuf.Write(line)
}

// fastPassWrite handles file write tool chunks with zero JSON overhead.
func (s *StreamNormalizer) fastPassWrite(line, payload []byte) {
	if bytes.Contains(payload, []byte("\"tool_calls\"")) {
		s.processChunkLine(line, payload)
		return
	}
	if contentStr := payloadContent(payload); contentStr != "" {
		if partial := findPartialCloseTagSuffix(contentStr); partial != "" {
			s.routeContentDelta(contentStr)
			s.outBuf.Write(line)
			return
		}
		s.recordToolDelta(contentStr)
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
		s.inWriteTool = false
		if s.pendingWriteTagBuf != "" {
			s.recordToolDelta(s.pendingWriteTagBuf)
			s.pendingWriteTagBuf = ""
		}
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
			s.routeContentDelta(choice.Delta.Content)
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
		raw = strings.TrimPrefix(raw, ">")
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
	s.routeContentDelta(proseDelta)
}

// routeContentDelta demuxes content streams between prose and active XML tool lanes.
func (s *StreamNormalizer) routeContentDelta(content string) {
	if content == "" && s.pendingWriteTagBuf == "" {
		return
	}

	full := content
	if s.pendingWriteTagBuf != "" {
		full = s.pendingWriteTagBuf + content
		s.pendingWriteTagBuf = ""
	}

	for len(full) > 0 {
		if !s.inWriteTool {
			openStart, openEnd, _ := findEarliestOpenTag(full)
			if openStart == -1 {
				if partial := findPartialOpenTagSuffix(full); partial != "" {
					prose := full[:len(full)-len(partial)]
					if prose != "" {
						s.recordProse(prose)
					}
					s.pendingWriteTagBuf = partial
					return
				}
				s.recordProse(full)
				return
			}

			proseBefore := full[:openStart]
			if proseBefore != "" {
				s.recordProse(proseBefore)
			}

			if openEnd == -1 {
				s.pendingWriteTagBuf = full[openStart:]
				return
			}

			s.inWriteTool = true
			s.currentToolCategory = shield.ToolCategoryFileWrite
			s.hasActiveToolCall = true
			tagText := full[openStart:openEnd]
			s.recordToolDelta(tagText)
			full = full[openEnd:]
		} else {
			closeStart, closeEnd, _ := findEarliestCloseTag(full)
			if closeStart == -1 {
				if partial := findPartialCloseTagSuffix(full); partial != "" {
					toolContent := full[:len(full)-len(partial)]
					if toolContent != "" {
						s.recordToolDelta(toolContent)
					}
					s.pendingWriteTagBuf = partial
					return
				}
				s.recordToolDelta(full)
				return
			}

			toolBefore := full[:closeStart]
			if toolBefore != "" {
				s.recordToolDelta(toolBefore)
			}

			if closeEnd == -1 {
				s.pendingWriteTagBuf = full[closeStart:]
				return
			}

			tagText := full[closeStart:closeEnd]
			s.recordToolDelta(tagText)
			s.inWriteTool = false
			if !s.hasNativeTools {
				s.hasActiveToolCall = false
			}
			s.currentToolCategory = shield.ToolCategoryCommand
			full = full[closeEnd:]
		}
	}
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
	if s.pendingWriteTagBuf != "" {
		if s.inWriteTool {
			s.recordToolDelta(s.pendingWriteTagBuf)
		} else {
			s.recordProse(s.pendingWriteTagBuf)
		}
		s.pendingWriteTagBuf = ""
	}
	s.inWriteTool = false

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
				if router.IsBuiltinWriteTool(name) {
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
	s.inWriteTool = false
	s.pendingWriteTagBuf = ""
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
