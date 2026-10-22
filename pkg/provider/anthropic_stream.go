// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package provider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	"github.com/dixieflatline76/nacho-flow/pkg/zeroalloc"
)

// AnthropicStreamAdapter implements an io.ReadCloser that translates an upstream
// Anthropic SSE event stream into standard OpenAI chat completion SSE chunks on the fly.
// Operates without dynamic maps, writing pre-formatted JSON directly into its internal buffer.
type AnthropicStreamAdapter struct {
	upstream   io.ReadCloser
	reader     *bufio.Reader
	outBuf     bytes.Buffer
	closed     bool
	eofReached bool

	currentEventBuf [32]byte
	currentEventLen int
	msgID           string
	model           string

	currentBlockIndex int
	currentBlockType  string
	currentToolID     string
	currentToolName   string
	toolCallIndex     int

	inputTokens              int
	outputTokens             int
	cacheCreationInputTokens int
	cacheReadInputTokens     int
	hasEmittedUsage          bool
	hasEmittedDone           bool

	cache              *thinkingBlockCache
	thinkingBuf        bytes.Buffer
	signatureBuf       bytes.Buffer
	turnThinkingBlocks []cachedThinkingBlock
	turnToolIDs        []string
	hasFlushedCache    bool
}

// NewAnthropicStreamAdapter creates a new stream adapter wrapping an Anthropic HTTP response body.
func NewAnthropicStreamAdapter(r io.ReadCloser) *AnthropicStreamAdapter {
	return NewAnthropicStreamAdapterWithCache(r, nil)
}

// NewAnthropicStreamAdapterWithCache creates a new stream adapter that also captures signed
// thinking and redacted_thinking blocks into the provided bounded cache keyed by tool_use IDs.
func NewAnthropicStreamAdapterWithCache(r io.ReadCloser, cache *thinkingBlockCache) *AnthropicStreamAdapter {
	a := &AnthropicStreamAdapter{
		upstream: r,
		reader:   bufio.NewReaderSize(r, 32*1024),
		cache:    cache,
	}
	a.outBuf.Grow(1024)
	return a
}

func (a *AnthropicStreamAdapter) Read(p []byte) (n int, err error) {
	if a.closed {
		return 0, io.EOF
	}

	for a.outBuf.Len() == 0 && !a.eofReached {
		line, readErr := a.reader.ReadSlice('\n')
		if len(line) > 0 {
			a.processLine(line)
		}
		if readErr != nil {
			if readErr == io.EOF {
				a.eofReached = true
				a.flushThinkingCache()
				if !a.hasEmittedUsage && (a.inputTokens > 0 || a.outputTokens > 0) {
					a.emitUsage()
					a.hasEmittedUsage = true
				}
				if !a.hasEmittedDone {
					a.emitDone()
				}
			} else {
				return 0, readErr
			}
		}
	}

	if a.outBuf.Len() > 0 {
		n, err = a.outBuf.Read(p)
		if a.outBuf.Len() == 0 {
			a.outBuf.Reset()
		}
		return n, err
	}

	if a.eofReached {
		return 0, io.EOF
	}

	return 0, nil
}

func (a *AnthropicStreamAdapter) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	a.flushThinkingCache()
	if a.upstream != nil {
		return a.upstream.Close()
	}
	return nil
}

// ----------------- Event Parsing & Dispatch -----------------

func (a *AnthropicStreamAdapter) processLine(line []byte) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return
	}

	if bytes.HasPrefix(trimmed, []byte("event:")) {
		ev := bytes.TrimSpace(trimmed[6:])
		a.currentEventLen = copy(a.currentEventBuf[:], ev)
		return
	}

	if bytes.HasPrefix(trimmed, []byte("data:")) {
		dataPayload := bytes.TrimSpace(trimmed[5:])
		a.dispatchData(dataPayload)
	}
}

func (a *AnthropicStreamAdapter) dispatchData(payload []byte) {
	// Zero-allocation wire fast-path for the 99.9% hot streaming path: content_block_delta
	if bytes.Contains(payload, []byte(`"content_block_delta"`)) {
		if quotedText, ok := zeroalloc.ExtractQuotedField(payload, []byte(`"text":`)); ok {
			a.outBuf.WriteString(`data: {"choices":[{"index":0,"delta":{"content":`)
			a.outBuf.Write(quotedText)
			a.outBuf.WriteString("}}]}\n\n")
			return
		}
		if quotedThinking, ok := zeroalloc.ExtractQuotedField(payload, []byte(`"thinking":`)); ok {
			a.outBuf.WriteString(`data: {"choices":[{"index":0,"delta":{"reasoning_content":`)
			a.outBuf.Write(quotedThinking)
			a.outBuf.WriteString("}}]}\n\n")
			if a.cache != nil && len(quotedThinking) >= 2 {
				a.thinkingBuf.Write(quotedThinking[1 : len(quotedThinking)-1])
			}
			return
		}
		if quotedSig, ok := zeroalloc.ExtractQuotedField(payload, []byte(`"signature":`)); ok {
			if a.cache != nil && len(quotedSig) >= 2 {
				a.signatureBuf.Write(quotedSig[1 : len(quotedSig)-1])
			}
			return
		}
		if quotedArgs, ok := zeroalloc.ExtractQuotedField(payload, []byte(`"partial_json":`)); ok {
			a.outBuf.WriteString(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":`)
			a.outBuf.WriteString(strconv.Itoa(a.toolCallIndex))
			a.outBuf.WriteString(`,"function":{"arguments":`)
			a.outBuf.Write(quotedArgs)
			a.outBuf.WriteString("}}]}}]}\n\n")
			return
		}
	}

	var sse fastAnthropicSSE
	if err := json.Unmarshal(payload, &sse); err != nil {
		return
	}

	eventType := sse.Type
	if eventType == "" && a.currentEventLen > 0 {
		eventType = string(a.currentEventBuf[:a.currentEventLen])
	}

	switch eventType {
	case "message_start":
		if sse.Message != nil {
			a.msgID = sse.Message.ID
			a.model = sse.Message.Model
			if sse.Message.Usage != nil {
				a.inputTokens = sse.Message.Usage.InputTokens
				a.cacheCreationInputTokens = sse.Message.Usage.CacheCreationInputTokens
				a.cacheReadInputTokens = sse.Message.Usage.CacheReadInputTokens
			}
		}

	case "content_block_start":
		if sse.ContentBlock != nil {
			a.currentBlockIndex = sse.Index
			a.currentBlockType = sse.ContentBlock.Type
			switch sse.ContentBlock.Type {
			case "tool_use":
				a.currentToolID = sse.ContentBlock.ID
				a.currentToolName = sse.ContentBlock.Name
				if a.cache != nil && sse.ContentBlock.ID != "" {
					a.turnToolIDs = append(a.turnToolIDs, sse.ContentBlock.ID)
				}
				a.emitToolStart(a.toolCallIndex, sse.ContentBlock.ID, sse.ContentBlock.Name)
			case "thinking":
				if a.cache != nil {
					a.thinkingBuf.Reset()
					a.signatureBuf.Reset()
					if a.thinkingBuf.Cap() < 16*1024 {
						a.thinkingBuf.Grow(16 * 1024)
					}
					if a.signatureBuf.Cap() < 1024 {
						a.signatureBuf.Grow(1024)
					}
					if sse.ContentBlock.Thinking != "" {
						writeEscapedInner(&a.thinkingBuf, sse.ContentBlock.Thinking)
					}
					if sse.ContentBlock.Signature != "" {
						a.signatureBuf.WriteString(sse.ContentBlock.Signature)
					}
				}
			case "redacted_thinking":
				if a.cache != nil && sse.ContentBlock.Data != "" {
					a.turnThinkingBlocks = append(a.turnThinkingBlocks, cachedThinkingBlock{
						Type: "redacted_thinking",
						Data: sse.ContentBlock.Data,
					})
				}
			}
		}

	case "content_block_delta":
		if sse.Delta != nil {
			switch sse.Delta.Type {
			case "text_delta":
				if sse.Delta.Text != "" {
					a.emitContentDelta(sse.Delta.Text)
				}
			case "input_json_delta":
				if sse.Delta.PartialJSON != "" {
					a.emitToolArgsDelta(sse.Delta.PartialJSON)
				}
			case "thinking_delta":
				if sse.Delta.Thinking != "" {
					a.emitReasoningDelta(sse.Delta.Thinking)
					if a.cache != nil {
						writeEscapedInner(&a.thinkingBuf, sse.Delta.Thinking)
					}
				}
			case "signature_delta":
				if a.cache != nil && sse.Delta.Signature != "" {
					a.signatureBuf.WriteString(sse.Delta.Signature)
				}
			}
		}

	case "content_block_stop":
		switch a.currentBlockType {
		case "tool_use":
			a.toolCallIndex++
			a.currentBlockType = ""
		case "thinking":
			a.finalizeThinkingBlock()
			a.currentBlockType = ""
		default:
			a.currentBlockType = ""
		}

	case "message_delta":
		if sse.Usage != nil {
			a.outputTokens = sse.Usage.OutputTokens
		}
		if sse.Delta != nil && sse.Delta.StopReason != nil {
			reason := mapStopReason(*sse.Delta.StopReason)
			a.emitFinishReason(reason)
		}
		if !a.hasEmittedUsage {
			a.emitUsage()
			a.hasEmittedUsage = true
		}

	case "message_stop":
		a.flushThinkingCache()
		if !a.hasEmittedUsage {
			a.emitUsage()
			a.hasEmittedUsage = true
		}
		a.emitDone()

	case "error":
		if sse.Error != nil {
			a.emitError(sse.Error.Message)
		}
	}
}

// ----------------- Zero-Alloc Direct Emitters -----------------

func (a *AnthropicStreamAdapter) emitContentDelta(text string) {
	a.outBuf.WriteString(`data: {"choices":[{"index":0,"delta":{"content":`)
	writeJSONStringDirect(&a.outBuf, text)
	a.outBuf.WriteString("}}]}\n\n")
}

func (a *AnthropicStreamAdapter) emitReasoningDelta(thinking string) {
	a.outBuf.WriteString(`data: {"choices":[{"index":0,"delta":{"reasoning_content":`)
	writeJSONStringDirect(&a.outBuf, thinking)
	a.outBuf.WriteString("}}]}\n\n")
}

func (a *AnthropicStreamAdapter) emitToolStart(callIdx int, id string, name string) {
	a.outBuf.WriteString(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":`)
	a.outBuf.WriteString(strconv.Itoa(callIdx))
	a.outBuf.WriteString(`,"id":`)
	writeJSONStringDirect(&a.outBuf, id)
	a.outBuf.WriteString(`,"type":"function","function":{"name":`)
	writeJSONStringDirect(&a.outBuf, name)
	a.outBuf.WriteString(",\"arguments\":\"\"}}]}}]}\n\n")
}

func (a *AnthropicStreamAdapter) emitToolArgsDelta(partialJSON string) {
	a.outBuf.WriteString(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":`)
	a.outBuf.WriteString(strconv.Itoa(a.toolCallIndex))
	a.outBuf.WriteString(`,"function":{"arguments":`)
	writeJSONStringDirect(&a.outBuf, partialJSON)
	a.outBuf.WriteString("}}]}}]}\n\n")
}

func (a *AnthropicStreamAdapter) emitFinishReason(reason string) {
	a.outBuf.WriteString(`data: {"choices":[{"index":0,"delta":{},"finish_reason":`)
	writeJSONStringDirect(&a.outBuf, reason)
	a.outBuf.WriteString("}]}\n\n")
}

func (a *AnthropicStreamAdapter) emitUsage() {
	promptTokens := a.inputTokens + a.cacheReadInputTokens + a.cacheCreationInputTokens
	total := promptTokens + a.outputTokens
	a.outBuf.WriteString(`data: {"choices":[],"usage":{"prompt_tokens":`)
	a.outBuf.WriteString(strconv.Itoa(promptTokens))
	a.outBuf.WriteString(`,"completion_tokens":`)
	a.outBuf.WriteString(strconv.Itoa(a.outputTokens))
	a.outBuf.WriteString(`,"total_tokens":`)
	a.outBuf.WriteString(strconv.Itoa(total))
	a.outBuf.WriteString(`,"prompt_tokens_details":{"cached_tokens":`)
	a.outBuf.WriteString(strconv.Itoa(a.cacheReadInputTokens))
	a.outBuf.WriteString("}}}\n\n")
}

func (a *AnthropicStreamAdapter) emitDone() {
	if a.hasEmittedDone {
		return
	}
	a.hasEmittedDone = true
	a.outBuf.WriteString("data: [DONE]\n\n")
}

func (a *AnthropicStreamAdapter) emitError(msg string) {
	a.outBuf.WriteString(`data: {"error":{"message":`)
	writeJSONStringDirect(&a.outBuf, msg)
	a.outBuf.WriteString(",\"type\":\"anthropic_stream_error\"}}\n\n")
}

func writeJSONStringDirect(buf *bytes.Buffer, s string) {
	zeroalloc.WriteEscapedJSONString(buf, s)
}

func (a *AnthropicStreamAdapter) finalizeThinkingBlock() {
	if a.cache != nil && a.signatureBuf.Len() > 0 {
		a.turnThinkingBlocks = append(a.turnThinkingBlocks, cachedThinkingBlock{
			Type:      "thinking",
			Thinking:  unescapeJSONInnerBytes(a.thinkingBuf.Bytes()),
			Signature: a.signatureBuf.String(),
		})
	}
	a.thinkingBuf.Reset()
	a.signatureBuf.Reset()
}

func (a *AnthropicStreamAdapter) flushThinkingCache() {
	if a.hasFlushedCache || a.cache == nil {
		return
	}
	if a.currentBlockType == "thinking" && a.signatureBuf.Len() > 0 {
		a.finalizeThinkingBlock()
		a.currentBlockType = ""
	}
	a.hasFlushedCache = true
	if len(a.turnToolIDs) == 0 || len(a.turnThinkingBlocks) == 0 {
		return
	}
	for _, id := range a.turnToolIDs {
		a.cache.Put(id, a.turnThinkingBlocks)
	}
}

func writeEscapedInner(buf *bytes.Buffer, s string) {
	var tmp bytes.Buffer
	zeroalloc.WriteEscapedJSONString(&tmp, s)
	b := tmp.Bytes()
	if len(b) >= 2 {
		buf.Write(b[1 : len(b)-1])
	}
}

func unescapeJSONInnerBytes(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if bytes.IndexByte(raw, '\\') == -1 {
		return string(raw)
	}
	quoted := make([]byte, 0, len(raw)+2)
	quoted = append(quoted, '"')
	quoted = append(quoted, raw...)
	quoted = append(quoted, '"')
	var unescaped string
	if err := json.Unmarshal(quoted, &unescaped); err == nil {
		return unescaped
	}
	return string(raw)
}

// ----------------- Typed Fast SSE JSON Structs -----------------

type fastAnthropicSSE struct {
	Type         string               `json:"type"`
	Index        int                  `json:"index"`
	Message      *fastSSEMessage      `json:"message,omitempty"`
	ContentBlock *fastSSEContentBlock `json:"content_block,omitempty"`
	Delta        *fastSSEDelta        `json:"delta,omitempty"`
	Usage        *fastSSEUsage        `json:"usage,omitempty"`
	Error        *fastSSEError        `json:"error,omitempty"`
}

type fastSSEMessage struct {
	ID    string        `json:"id"`
	Model string        `json:"model"`
	Usage *fastSSEUsage `json:"usage,omitempty"`
}

type fastSSEContentBlock struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"`
}

type fastSSEDelta struct {
	Type        string  `json:"type"`
	Text        string  `json:"text,omitempty"`
	Thinking    string  `json:"thinking,omitempty"`
	Signature   string  `json:"signature,omitempty"`
	PartialJSON string  `json:"partial_json,omitempty"`
	StopReason  *string `json:"stop_reason,omitempty"`
}

type fastSSEUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type fastSSEError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
