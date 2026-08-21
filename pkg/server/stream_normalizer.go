package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
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
)

type fastDelta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Reasoning        string          `json:"reasoning,omitempty"`
	Reason           string          `json:"reason,omitempty"`
	ToolCalls        json.RawMessage `json:"tool_calls,omitempty"`
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

// StreamNormalizer wraps an upstream SSE response stream and normalizes reasoning tokens
// (from DeepSeek-R1, OpenRouter, etc.) into standard <think>...</think> tags within delta.content.
type StreamNormalizer struct {
	upstream      io.ReadCloser
	reader        *bufio.Reader
	outBuf        *bytes.Buffer
	inThinking    bool
	alreadyTagged bool
	closed        bool
	eofReached    bool
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
	}
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
				if s.inThinking {
					s.inThinking = false
					s.outBuf.WriteString("data: {\"choices\":[{\"delta\":{\"content\":\"\\n</think>\\n\\n\"}}]}\n\n")
				}
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

func marshalNoEscapeHTML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func (s *StreamNormalizer) processLine(line []byte) {
	trimmed := bytes.TrimRight(line, "\r\n")

	// Pass comments, empty lines, and non-data lines directly through
	if !bytes.HasPrefix(trimmed, []byte("data: ")) {
		s.outBuf.Write(line)
		return
	}

	payload := bytes.TrimPrefix(trimmed, []byte("data: "))
	if bytes.Equal(payload, []byte("[DONE]")) {
		if s.inThinking {
			s.inThinking = false
			s.outBuf.WriteString("data: {\"choices\":[{\"delta\":{\"content\":\"\\n</think>\\n\\n\"}}]}\n\n")
		}
		s.outBuf.Write(line)
		return
	}

	// Fast path: if chunk has no reasoning markers and no think tag, check if transition needed
	hasReasoningMarker := bytes.Contains(payload, []byte("reasoning_content")) ||
		bytes.Contains(payload, []byte("\"reasoning\"")) ||
		bytes.Contains(payload, []byte("\"reason\"")) ||
		bytes.Contains(payload, []byte("<|im_start|>think")) ||
		bytes.Contains(payload, []byte("<|im_start|>thought")) ||
		bytes.Contains(payload, []byte("<thinking>")) ||
		bytes.Contains(payload, []byte("</thinking>")) ||
		bytes.Contains(payload, []byte("<|im_end|>"))

	if !hasReasoningMarker && !s.inThinking && !bytes.Contains(payload, []byte("<think>")) {
		s.outBuf.Write(line)
		return
	}

	var chunk fastStreamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil || len(chunk.Choices) == 0 {
		// If not valid JSON, pass raw line through
		s.outBuf.Write(line)
		return
	}

	choice := &chunk.Choices[0]
	reasoningText := choice.Delta.ReasoningContent
	if reasoningText == "" {
		reasoningText = choice.Delta.Reasoning
	}
	if reasoningText == "" {
		reasoningText = choice.Delta.Reason
	}

	if reasoningText != "" {
		// Model is emitting reasoning tokens
		sanitized := strings.ReplaceAll(reasoningText, "</think>", "&lt;/think&gt;")
		choice.Delta.ReasoningContent = ""
		choice.Delta.Reasoning = ""
		choice.Delta.Reason = ""

		if !s.inThinking && !s.alreadyTagged {
			s.inThinking = true
			choice.Delta.Content = "<think>\n" + sanitized
		} else {
			choice.Delta.Content = sanitized
		}

		if newPayload, err := marshalNoEscapeHTML(chunk); err == nil {
			s.outBuf.WriteString("data: ")
			s.outBuf.Write(newPayload)
			s.outBuf.WriteString("\n\n")
			return
		}
	} else {
		// Normalize text-embedded reasoning tags (Qwen, Claude-style thinking tags)
		content := choice.Delta.Content
		if strings.Contains(content, "<|im_start|>think") {
			content = strings.ReplaceAll(content, "<|im_start|>think", "<think>")
			s.alreadyTagged = true
		}
		if strings.Contains(content, "<|im_start|>thought") {
			content = strings.ReplaceAll(content, "<|im_start|>thought", "<think>")
			s.alreadyTagged = true
		}
		if strings.Contains(content, "<thinking>") {
			content = strings.ReplaceAll(content, "<thinking>", "<think>")
			s.alreadyTagged = true
		}
		if strings.Contains(content, "</thinking>") {
			content = strings.ReplaceAll(content, "</thinking>", "</think>")
		}
		if strings.Contains(content, "<|im_end|>") {
			content = strings.ReplaceAll(content, "<|im_end|>", "</think>")
		}
		choice.Delta.Content = content

		if strings.Contains(choice.Delta.Content, "<think>") {
			s.alreadyTagged = true
		}

		if s.inThinking {
			// Transition from thinking to final answer, tool call, or finish
			s.inThinking = false
			choice.Delta.Content = "\n</think>\n\n" + choice.Delta.Content

			if newPayload, err := marshalNoEscapeHTML(chunk); err == nil {
				s.outBuf.WriteString("data: ")
				s.outBuf.Write(newPayload)
				s.outBuf.WriteString("\n\n")
				return
			}
		}

		if choice.Delta.Content != payloadContent(payload) {
			if newPayload, err := marshalNoEscapeHTML(chunk); err == nil {
				s.outBuf.WriteString("data: ")
				s.outBuf.Write(newPayload)
				s.outBuf.WriteString("\n\n")
				return
			}
		}
	}

	s.outBuf.Write(line)
}

func payloadContent(payload []byte) string {
	var c fastStreamChunk
	if err := json.Unmarshal(payload, &c); err == nil && len(c.Choices) > 0 {
		return c.Choices[0].Delta.Content
	}
	return ""
}

// Close closes the underlying stream and returns buffers to the pools.
func (s *StreamNormalizer) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
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
	return err
}
