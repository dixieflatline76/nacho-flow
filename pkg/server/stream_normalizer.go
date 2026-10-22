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
	"unsafe"

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
	finishReasonMarker   = []byte("\"finish_reason\":")
	dataPrefix           = []byte("data: ")
	dataPrefixNoSpace    = []byte("data:")
	doneMarker           = []byte("[DONE]")
	toolCallsMarker      = []byte("\"tool_calls\"")
	usageMarker          = []byte("\"usage\"")
	roleMarker           = []byte("\"role\"")
)

const honestTruncationNoticeChunk = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"\\n\\n> ⚠️ **Nacho Flow • Reasoning Limit Reached**\\n> Model reached its output token limit without producing a response.\\n\"}}]}\n\n"

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
	return findEarliestWriteTag(s, false)
}

func findEarliestCloseTag(s string) (startIdx, endIdx int, tagName string) {
	return findEarliestWriteTag(s, true)
}

func findEarliestWriteTag(s string, closing bool) (startIdx, endIdx int, tagName string) {
	curr := s
	offset := 0
	prefixLen := 1
	if closing {
		prefixLen = 2
	}

	for {
		var idx int
		if closing {
			idx = strings.Index(curr, "</")
		} else {
			idx = strings.IndexByte(curr, '<')
		}
		if idx == -1 {
			return -1, -1, ""
		}
		actualStart := offset + idx
		if !closing && idx+1 < len(curr) && curr[idx+1] == '/' {
			curr = curr[idx+1:]
			offset = actualStart + 1
			continue
		}

		rest := curr[idx+prefixLen:]
		nameEnd := 0
		for nameEnd < len(rest) {
			b := rest[nameEnd]
			if b == '>' || b == ' ' || b == '\t' || b == '\r' || b == '\n' || (!closing && b == '/') {
				break
			}
			nameEnd++
		}

		if nameEnd > 0 {
			name := strings.ToLower(rest[:nameEnd])
			if agentregistry.DefaultRegistry().IsWriteTool(name) {
				if gtIdx := strings.IndexByte(rest, '>'); gtIdx != -1 {
					return actualStart, actualStart + prefixLen + gtIdx + 1, name
				}
				return actualStart, -1, name
			}
		}

		curr = curr[idx+prefixLen:]
		offset = actualStart + prefixLen
	}
}

func findPartialOpenTagSuffix(s string) string {
	return findPartialWriteTagSuffix(s, false)
}

func findPartialCloseTagSuffix(s string) string {
	return findPartialWriteTagSuffix(s, true)
}

func findPartialWriteTagSuffix(s string, closing bool) string {
	checkLen := len(s)
	if checkLen > 25 {
		checkLen = 25
	}
	tail := s[len(s)-checkLen:]

	var ltIdx int
	prefixLen := 1
	if closing {
		ltIdx = strings.LastIndex(tail, "</")
		prefixLen = 2
	} else {
		ltIdx = strings.LastIndexByte(tail, '<')
	}
	if ltIdx == -1 {
		return ""
	}

	candidate := tail[ltIdx:]
	if strings.ContainsRune(candidate, '>') || (!closing && strings.HasPrefix(candidate, "</")) {
		return ""
	}

	afterPrefix := strings.ToLower(candidate[prefixLen:])
	for _, tag := range writeToolsList {
		if strings.HasPrefix(tag, afterPrefix) {
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
	upstream                io.ReadCloser
	reader                  *bufio.Reader
	outBuf                  *bytes.Buffer
	inThinking              bool
	inStructuredReasoning   bool
	alreadyTagged           bool // Preserved for backward test compatibility
	closed                  bool
	eofReached              bool
	capturedUsage           StreamUsage
	hasUsage                bool
	emittedCompletionChars  int
	interactiveTool         string
	shieldMgr               *shield.ShieldManager
	tailBuffer              *shield.TailBuffer
	hasNativeTools          bool
	proseAccumulator        strings.Builder
	cycleBreaker            *shield.CycleBreaker
	cycleViolated           bool
	cycleViolationReason    string
	features                uint16
	pendingDelimiterBuf     string
	toolChunksScratch       []fastToolCallChunk
	currentToolCategory     shield.ToolActivityCategory
	hasActiveToolCall       bool
	inWriteTool             bool
	pendingWriteTagBuf      string
	hasEmittedContent       bool
	hasNonWhitespaceContent bool
	hasEmittedToolCalls     bool
	hasEmittedReasoning     bool
	finishReason            string
	heldTerminal            *bytes.Buffer
	deferTerminal           bool
	suppressPrelude         bool
	preludeSuppressed       bool
	cmdHeaderBuf            [4096]byte
	cmdHeaderLen            int
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

// FinishReason returns the finish_reason extracted from upstream SSE chunks, or "" if not finished.
func (s *StreamNormalizer) FinishReason() string {
	return s.finishReason
}

// IsEmptyTruncatedTurn reports whether the stream terminated due to token length
// without emitting any tool calls and without emitting non-whitespace content.
func (s *StreamNormalizer) IsEmptyTruncatedTurn() bool {
	return s.finishReason == "length" && !s.hasEmittedToolCalls && !s.hasNonWhitespaceContent
}

// TerminalHeld reports whether terminal chunks (finish_reason, usage) are currently buffered.
func (s *StreamNormalizer) TerminalHeld() bool {
	return s.heldTerminal != nil && s.heldTerminal.Len() > 0
}

// SetDeferTerminal configures whether terminal chunks should be deferred when turn is empty and truncated.
func (s *StreamNormalizer) SetDeferTerminal(deferTerm bool) {
	s.deferTerminal = deferTerm
}

// SetSuppressPrelude configures whether the initial role-only chunk should be suppressed.
func (s *StreamNormalizer) SetSuppressPrelude(suppress bool) {
	s.suppressPrelude = suppress
}

// HasEmittedToolCalls reports whether the stream emitted any tool calls.
func (s *StreamNormalizer) HasEmittedToolCalls() bool {
	return s.hasEmittedToolCalls
}

// HasNonWhitespaceContent reports whether the stream emitted non-whitespace prose content.
func (s *StreamNormalizer) HasNonWhitespaceContent() bool {
	return s.hasNonWhitespaceContent
}

// HeldTerminal returns the buffered terminal bytes (if held).
func (s *StreamNormalizer) HeldTerminal() []byte {
	if s.heldTerminal != nil {
		return s.heldTerminal.Bytes()
	}
	return nil
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
			if readErr != io.EOF {
				return 0, readErr
			}
			s.eofReached = true
			wasInThinking := s.inThinking
			s.inThinking = false
			s.inStructuredReasoning = false
			s.flushPendingDelimiter(wasInThinking, false)
			if s.flushHeldTerminal() {
				return 0, nil
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

func (s *StreamNormalizer) flushPendingDelimiter(wasInThinking, skipIfToolCalls bool) {
	if s.pendingDelimiterBuf == "" {
		return
	}
	buf := s.pendingDelimiterBuf
	s.pendingDelimiterBuf = ""
	if skipIfToolCalls && s.hasEmittedToolCalls {
		return
	}
	if len(buf) >= 2 && agentregistry.DefaultRegistry().IsKnownDelimiterPrefix([]byte(buf)) {
		return
	}
	var delta fastDelta
	if wasInThinking {
		s.recordReasoning(buf)
		delta.ReasoningContent = buf
	} else {
		s.recordProse(buf)
		delta.Content = buf
	}
	s.emitChunk(fastStreamChunk{
		Choices: []fastStreamChoice{{Index: 0, Delta: delta}},
	})
}

func (s *StreamNormalizer) flushHeldTerminal() bool {
	if s.heldTerminal == nil || s.heldTerminal.Len() == 0 {
		return false
	}
	if s.IsEmptyTruncatedTurn() {
		if s.deferTerminal {
			return true
		}
		s.outBuf.WriteString(honestTruncationNoticeChunk)
	}
	s.outBuf.Write(s.heldTerminal.Bytes())
	s.heldTerminal.Reset()
	return false
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
	case bytes.HasPrefix(trimmed, dataPrefix):
		payload = trimmed[len(dataPrefix):] // len(dataPrefix) == 6
	case bytes.HasPrefix(trimmed, dataPrefixNoSpace):
		payload = trimmed[len(dataPrefixNoSpace):] // len(dataPrefixNoSpace) == 5
	default:
		if s.heldTerminal != nil && s.heldTerminal.Len() > 0 {
			s.heldTerminal.Write(line)
			return
		}
		s.outBuf.Write(line)
		return
	}
	if bytes.Equal(payload, doneMarker) {
		s.handleDone(line)
		return
	}

	if s.suppressPrelude && !s.preludeSuppressed {
		if isRoleOnlyPreludeChunk(payload) {
			s.preludeSuppressed = true
			return
		}
		s.preludeSuppressed = true
	}

	if bytes.Contains(payload, toolCallsMarker) {
		s.hasNativeTools = true
	}

	if bytes.Contains(payload, usageMarker) {
		s.captureUsage(payload)
	}

	// If terminal chunks are already held, any subsequent non-[DONE] chunk (e.g. usage-only chunk) is held too.
	if s.heldTerminal != nil && s.heldTerminal.Len() > 0 {
		s.heldTerminal.Write(line)
		return
	}

	// Fast path: if outside reasoning, outside write tools, no pending delimiter buffer, and no markers detected, pass line through directly
	if !s.inThinking && !s.inWriteTool && s.pendingDelimiterBuf == "" && s.pendingWriteTagBuf == "" && !hasAnyReasoningMarker(payload) && !hasAnyWriteMarker(payload) {
		s.fastPassProse(line, payload)
		return
	}

	// Fast path: if inside write tools, outside reasoning, no pending delimiter buffer, and no markers detected, pass line through directly as tool write
	if s.inWriteTool && !s.inThinking && s.pendingDelimiterBuf == "" && s.pendingWriteTagBuf == "" && !hasAnyReasoningMarker(payload) && !hasAnyWriteMarker(payload) {
		s.fastPassWrite(line, payload)
		return
	}

	s.processChunkLine(line, payload)
}

// fastPassProse handles prose chunks that require no JSON transformations.
func (s *StreamNormalizer) fastPassProse(line, payload []byte) {
	if bytes.Contains(payload, toolCallsMarker) || bytes.Contains(payload, finishReasonMarker) {
		s.processChunkLine(line, payload)
		return
	}
	if contentStr := payloadContent(payload); contentStr != "" {
		tailLen := len(contentStr)
		if tailLen > 24 {
			tailLen = 24
		}
		if strings.IndexByte(contentStr[len(contentStr)-tailLen:], '<') != -1 {
			if idx := agentregistry.DefaultRegistry().FindTrailingDelimiterPrefix([]byte(contentStr), 24); idx != -1 {
				s.processChunkLine(line, payload)
				return
			}
		}
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
	if bytes.Contains(payload, toolCallsMarker) || bytes.Contains(payload, finishReasonMarker) {
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

func stripControlTokensIfNeeded(text string) string {
	if text == "" || strings.IndexByte(text, '<') == -1 {
		return text
	}
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.WriteString(text)
	cleaned := agentregistry.DefaultRegistry().StripControlTokensInPlace(buf.Bytes())
	if len(cleaned) == len(text) {
		bufPool.Put(buf)
		return text
	}
	res := string(cleaned)
	bufPool.Put(buf)
	return res
}

// processChunkLine parses a JSON streaming chunk and canonicalizes reasoning vs content deltas.
func (s *StreamNormalizer) processChunkLine(line, payload []byte) {
	if bytes.Contains(payload, toolCallsMarker) {
		reg := agentregistry.DefaultRegistry()
		if reg != nil {
			modified := false
			if reg.HasParameterSanitizerMarker(payload) {
				line = reg.SanitizeParametersInPlace(line)
				modified = true
			}
			if reg.HasControlTokenMarker(payload) {
				line = reg.StripControlTokensInPlace(line)
				modified = true
			}
			if modified {
				trimmed := bytes.TrimRight(line, "\r\n")
				switch {
				case bytes.HasPrefix(trimmed, dataPrefix):
					payload = trimmed[len(dataPrefix):]
				case bytes.HasPrefix(trimmed, dataPrefixNoSpace):
					payload = trimmed[len(dataPrefixNoSpace):]
				}
			}
		}
	}

	var chunk fastStreamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil || len(chunk.Choices) == 0 {
		s.outBuf.Write(line)
		return
	}

	choice := &chunk.Choices[0]
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		s.finishReason = *choice.FinishReason
		s.hasActiveToolCall = false
		s.inWriteTool = false
		if s.pendingWriteTagBuf != "" {
			s.recordToolDelta(s.pendingWriteTagBuf)
			s.pendingWriteTagBuf = ""
		}
	}
	if len(choice.Delta.ToolCalls) > 0 {
		s.extractAndRecordToolCalls(choice.Delta.ToolCalls)
		s.pendingDelimiterBuf = ""
	}

	reasoningText := s.resolveReasoningField(&choice.Delta)

	if reasoningText != "" {
		s.inThinking = true
		s.inStructuredReasoning = true

		if s.pendingDelimiterBuf != "" {
			reasoningText = s.pendingDelimiterBuf + reasoningText
			s.pendingDelimiterBuf = ""
		}

		if idx := agentregistry.DefaultRegistry().FindTrailingDelimiterPrefix([]byte(reasoningText), 24); idx != -1 {
			s.pendingDelimiterBuf = reasoningText[idx:]
			reasoningText = reasoningText[:idx]
		}

		choice.Delta.ReasoningContent = stripControlTokensIfNeeded(reasoningText)

		choice.Delta.Reasoning = ""
		choice.Delta.Reason = ""

		s.recordReasoning(choice.Delta.ReasoningContent)
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

	if choice.FinishReason != nil && *choice.FinishReason != "" && !s.hasEmittedContent && !s.hasEmittedToolCalls {
		if s.heldTerminal == nil {
			s.heldTerminal = bufPool.Get().(*bytes.Buffer)
			s.heldTerminal.Reset()
		}
		s.heldTerminal.Write(line)
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
	if s.pendingDelimiterBuf != "" {
		raw = s.pendingDelimiterBuf + raw
		s.pendingDelimiterBuf = ""
	}

	if idx := agentregistry.DefaultRegistry().FindTrailingDelimiterPrefix([]byte(raw), 24); idx != -1 {
		s.pendingDelimiterBuf = raw[idx:]
		raw = raw[:idx]
	}

	if raw == "" {
		choice.Delta.ReasoningContent = ""
		choice.Delta.Content = ""
		choice.Delta.Reasoning = ""
		choice.Delta.Reason = ""
		return
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
		prefix := strings.ReplaceAll(parts[0], "</think>", "")
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
		proseDelta = strings.ReplaceAll(content, "</think>", "")
	}

	choice.Delta.ReasoningContent = stripControlTokensIfNeeded(reasoningDelta)
	choice.Delta.Content = stripControlTokensIfNeeded(proseDelta)
	choice.Delta.Reasoning = ""
	choice.Delta.Reason = ""

	s.recordReasoning(choice.Delta.ReasoningContent)
	s.routeContentDelta(choice.Delta.Content)
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

// emitChunk serializes and writes a modified SSE chunk into the output buffer using bufPool.
func (s *StreamNormalizer) emitChunk(chunk fastStreamChunk) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(chunk); err == nil {
		s.outBuf.WriteString("data: ")
		s.outBuf.Write(bytes.TrimRight(buf.Bytes(), "\n"))
		s.outBuf.WriteString("\n\n")
	}
	bufPool.Put(buf)
}

// handleDone finalizes the stream on [DONE] and synthesizes an agent tool call if required.
func (s *StreamNormalizer) handleDone(doneLine []byte) {
	wasInThinking := s.inThinking
	s.inThinking = false
	s.inStructuredReasoning = false
	s.hasActiveToolCall = false
	s.flushPendingDelimiter(wasInThinking, true)

	if s.pendingWriteTagBuf != "" {
		if s.inWriteTool {
			s.recordToolDelta(s.pendingWriteTagBuf)
		} else if !s.hasEmittedToolCalls {
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
				s.hasEmittedToolCalls = true
			}
		}
	}

	if s.flushHeldTerminal() {
		return
	}
	s.outBuf.Write(doneLine)
}

type syntheticToolDelta struct {
	ToolCalls [1]*shield.RawToolCall `json:"tool_calls"`
}

type syntheticToolChoice struct {
	Index int                `json:"index"`
	Delta syntheticToolDelta `json:"delta"`
}

type syntheticToolChunk struct {
	Choices [1]syntheticToolChoice `json:"choices"`
}

type syntheticFinishChoice struct {
	Index        int      `json:"index"`
	Delta        struct{} `json:"delta"`
	FinishReason string   `json:"finish_reason"`
}

type syntheticFinishChunk struct {
	Choices [1]syntheticFinishChoice `json:"choices"`
}

// emitSyntheticToolCall injects synthetic tool_calls and finish_reason SSE chunks.
func (s *StreamNormalizer) emitSyntheticToolCall(synthCall *shield.RawToolCall) {
	toolChunk := syntheticToolChunk{
		Choices: [1]syntheticToolChoice{{
			Index: 0,
			Delta: syntheticToolDelta{ToolCalls: [1]*shield.RawToolCall{synthCall}},
		}},
	}
	if deltaJSON, err := json.Marshal(toolChunk); err == nil {
		s.outBuf.WriteString("data: ")
		s.outBuf.Write(deltaJSON)
		s.outBuf.WriteString("\n\n")
	}

	finishChunk := syntheticFinishChunk{
		Choices: [1]syntheticFinishChoice{{
			Index:        0,
			FinishReason: "tool_calls",
		}},
	}
	if finishJSON, err := json.Marshal(finishChunk); err == nil {
		s.outBuf.WriteString("data: ")
		s.outBuf.Write(finishJSON)
		s.outBuf.WriteString("\n\n")
	}
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
	s.hasEmittedReasoning = true
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
	s.hasEmittedContent = true
	if strings.TrimSpace(text) != "" {
		s.hasNonWhitespaceContent = true
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
	s.hasEmittedToolCalls = true
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
	s.hasEmittedToolCalls = true
	clear(s.toolChunksScratch[:cap(s.toolChunksScratch)])
	s.toolChunksScratch = s.toolChunksScratch[:0]

	if err := json.Unmarshal(raw, &s.toolChunksScratch); err != nil {
		return
	}

	for i := range s.toolChunksScratch {
		fn := &s.toolChunksScratch[i].Function
		s.hasActiveToolCall = true

		if fn.Name != "" {
			s.currentToolCategory = router.ClassifyToolCategory(fn.Name)
			s.cmdHeaderLen = 0
		}
		if fn.Arguments == "" {
			continue
		}
		if s.currentToolCategory == shield.ToolCategoryCommand && s.cmdHeaderLen < len(s.cmdHeaderBuf) {
			n := copy(s.cmdHeaderBuf[s.cmdHeaderLen:], fn.Arguments)
			s.cmdHeaderLen += n
			header := unsafe.String(&s.cmdHeaderBuf[0], s.cmdHeaderLen)
			if router.DetectShellWrite(header) {
				s.currentToolCategory = shield.ToolCategoryFileWrite
			}
		}
		s.recordToolDelta(fn.Arguments)
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

func isRoleOnlyPreludeChunk(payload []byte) bool {
	if !bytes.Contains(payload, roleMarker) {
		return false
	}
	var c fastStreamChunk
	if err := json.Unmarshal(payload, &c); err == nil && len(c.Choices) > 0 {
		delta := c.Choices[0].Delta
		if delta.Role != "" && delta.Content == "" && delta.ReasoningContent == "" &&
			delta.Reasoning == "" && delta.Reason == "" && len(delta.ToolCalls) == 0 &&
			c.Choices[0].FinishReason == nil {
			return true
		}
	}
	return false
}

func marshalNoEscapeHTML(v any) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	res := bytes.TrimRight(buf.Bytes(), "\n")
	cp := make([]byte, len(res))
	copy(cp, res)
	return cp, nil
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
	s.cmdHeaderLen = 0
	var err error
	if s.upstream != nil {
		err = s.upstream.Close()
	}
	if s.outBuf != nil {
		s.outBuf.Reset()
		bufPool.Put(s.outBuf)
		s.outBuf = nil
	}
	if s.heldTerminal != nil {
		s.heldTerminal.Reset()
		bufPool.Put(s.heldTerminal)
		s.heldTerminal = nil
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
			TotalTokens:      0,
		}, false
	}
	return StreamUsage{}, false
}
