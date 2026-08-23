package router

import (
	"strings"
)

// RawToolCall represents an extracted tool call structure from markdown, XML, or control tokens.
type RawToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// NormalizerPipeline manages a prioritized chain of ToolParser strategies.
type NormalizerPipeline struct {
	parsers []ToolParser
}

// NewDefaultPipeline constructs a pipeline initialized with all 8 standard format parsers.
func NewDefaultPipeline() *NormalizerPipeline {
	return &NormalizerPipeline{
		parsers: []ToolParser{
			&HermesXMLParser{},
			&MistralTokenParser{},
			&LlamaTagParser{},
			&LlamaPythonParser{},
			&ClaudeXMLParser{},
			&ReActParser{},
			&MarkdownFenceParser{},
			&BareJSONParser{},
		},
	}
}

var defaultPipeline = NewDefaultPipeline()

// hasCandidateTokens scans content in a single byte loop for candidate syntax anchors.
func hasCandidateTokens(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == '<' || b == '[' || b == '{' || b == '`' {
			return true
		}
		if (b == 'A' || b == 'a') && i+7 <= len(s) {
			if strings.EqualFold(s[i:i+7], "action:") {
				return true
			}
		}
	}
	return false
}

// Normalize runs content through the prioritized parser pipeline.
// Returns the remaining cleaned text, the extracted tool calls, and true if any calls were matched.
func (p *NormalizerPipeline) Normalize(content string) (string, []RawToolCall, bool) {
	if strings.TrimSpace(content) == "" {
		return content, nil, false
	}

	// Fast pre-filter: bail out in sub-microsecond single pass without regex/parsing
	if !hasCandidateTokens(content) {
		return content, nil, false
	}

	for _, parser := range p.parsers {
		if remaining, calls, matched := parser.Parse(content, 1); matched && len(calls) > 0 {
			return strings.TrimSpace(remaining), calls, true
		}
	}

	return content, nil, false
}

// NormalizeMarkdownToolCalls extracts tool calls across 8 open-source format families and converts them to OpenAI tool_calls.
// Returns the remaining cleaned message text, the extracted tool calls array, and true if any tool calls were parsed.
func NormalizeMarkdownToolCalls(content string) (string, []RawToolCall, bool) {
	return defaultPipeline.Normalize(content)
}
