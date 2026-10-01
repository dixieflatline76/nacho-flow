// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

const (
	AnthropicAPIVersion       = "2023-06-01"
	AnthropicBetaCaching      = "prompt-caching-2024-07-31"
	DefaultAnthropicMaxTokens = 8192
)

// AnthropicProvider implements native Anthropic Messages API connectivity
// with zero-allocation slicing and direct wire payload formatting.
type AnthropicProvider struct {
	id             string
	name           string
	config         contract.ProviderConfig
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

// NewAnthropicProvider instantiates a new Anthropic provider.
func NewAnthropicProvider(id string, cfg contract.ProviderConfig) *AnthropicProvider {
	name := id
	if strings.Contains(strings.ToLower(id), "anthropic") {
		name = "Anthropic Direct"
	}
	return &AnthropicProvider{
		id:             id,
		name:           name,
		config:         cfg,
		client:         &http.Client{Timeout: 10 * time.Second},
		circuitBreaker: NewCircuitBreaker(DefaultFailureThreshold, DefaultCooldownDuration),
	}
}

func (p *AnthropicProvider) ID() string {
	return p.id
}

func (p *AnthropicProvider) Name() string {
	return p.name
}

func (p *AnthropicProvider) BaseURL() string {
	return p.config.BaseURL
}

func (p *AnthropicProvider) IsLocal() bool {
	return false
}

func (p *AnthropicProvider) GetAPIKey() string {
	return p.config.APIKey
}

func (p *AnthropicProvider) CircuitBreaker() *CircuitBreaker {
	if p.circuitBreaker == nil {
		p.circuitBreaker = NewCircuitBreaker(DefaultFailureThreshold, DefaultCooldownDuration)
	}
	return p.circuitBreaker
}

// Ping checks if the Anthropic API endpoint is reachable.
func (p *AnthropicProvider) Ping(ctx context.Context) error {
	baseURL := normalizeAnthropicBaseURL(p.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}
	if p.config.APIKey != "" {
		req.Header.Set("x-api-key", p.config.APIKey)
		req.Header.Set("anthropic-version", AnthropicAPIVersion)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("provider '%s' ping failed: %w", p.id, err)
	}
	_ = resp.Body.Close()
	return nil
}

// WrapResponseStream adapts the raw upstream Anthropic SSE stream into OpenAI SSE chunks.
func (p *AnthropicProvider) WrapResponseStream(resp *http.Response) io.ReadCloser {
	return NewAnthropicStreamAdapter(resp.Body)
}

// BuildUpstreamRequest converts incoming OpenAI ChatCompletion JSON into Anthropic Messages API format
// using typed fast structs and zero-allocation payload slicing.
func (p *AnthropicProvider) BuildUpstreamRequest(ctx context.Context, r *http.Request, model string, body []byte) (*http.Request, error) {
	var openAIReq fastOpenAIReq
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		return nil, fmt.Errorf("failed to parse incoming OpenAI request body: %w", err)
	}

	targetModel := normalizeAnthropicModel(model)
	if targetModel == "" {
		targetModel = normalizeAnthropicModel(openAIReq.Model)
	}
	if targetModel == "" {
		targetModel = "claude-sonnet-5"
	}

	// 1. Extract and hoist system messages using string builder
	var sysBuilder strings.Builder
	var nonSystemMsgs []fastOpenAIMsg
	for _, m := range openAIReq.Messages {
		if m.Role == "system" || m.Role == "developer" {
			text := extractTextContentFast(m.Content)
			if strings.TrimSpace(text) != "" {
				if sysBuilder.Len() > 0 {
					sysBuilder.WriteString("\n\n")
				}
				sysBuilder.WriteString(text)
			}
		} else {
			nonSystemMsgs = append(nonSystemMsgs, m)
		}
	}

	var anthropicSystemBlocks []fastAnthropicContentBlock
	if sysBuilder.Len() > 0 {
		anthropicSystemBlocks = []fastAnthropicContentBlock{
			{
				Type: "text",
				Text: sysBuilder.String(),
				CacheControl: &fastCacheControl{
					Type: "ephemeral",
				},
			},
		}
	}

	// 2. Translate conversation turns & enforce strict alternation with zero-alloc tool grouping
	anthropicMsgs, err := translateMessagesFast(nonSystemMsgs)
	if err != nil {
		return nil, fmt.Errorf("failed translating messages: %w", err)
	}

	// 3. Translate tool definitions (slicing parameters raw, 0 allocs)
	var anthropicTools []fastAnthropicTool
	if len(openAIReq.Tools) > 0 {
		anthropicTools = make([]fastAnthropicTool, 0, len(openAIReq.Tools))
		for _, t := range openAIReq.Tools {
			if t.Type == "function" || t.Function != nil {
				schema := t.Function.Parameters
				if len(schema) == 0 {
					schema = json.RawMessage(`{"type":"object"}`)
				}
				anthropicTools = append(anthropicTools, fastAnthropicTool{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					InputSchema: schema,
				})
			}
		}
		// Prompt cache breakpoint on last tool
		if len(anthropicTools) > 0 {
			anthropicTools[len(anthropicTools)-1].CacheControl = &fastCacheControl{
				Type: "ephemeral",
			}
		}
	}

	// 4. Translate tool_choice
	toolChoice, isNone := translateToolChoiceFast(openAIReq.ToolChoice)
	if isNone {
		anthropicTools = nil
		toolChoice = nil
	}

	// 5. Max tokens resolution
	maxTokens := DefaultAnthropicMaxTokens
	if openAIReq.MaxTokens != nil && *openAIReq.MaxTokens > 0 {
		maxTokens = *openAIReq.MaxTokens
	} else if openAIReq.MaxCompletionTokens != nil && *openAIReq.MaxCompletionTokens > 0 {
		maxTokens = *openAIReq.MaxCompletionTokens
	}

	// 6. Stop sequences
	var stopSeqs []string
	if len(openAIReq.Stop) > 0 {
		var singleStop string
		if err := json.Unmarshal(openAIReq.Stop, &singleStop); err == nil && singleStop != "" {
			stopSeqs = []string{singleStop}
		} else {
			var multiStop []string
			if err := json.Unmarshal(openAIReq.Stop, &multiStop); err == nil {
				stopSeqs = multiStop
			}
		}
	}

	// Thinking / temperature sanitization
	mLower := strings.ToLower(targetModel)
	isClaude5 := strings.Contains(mLower, "sonnet-5") ||
		strings.Contains(mLower, "opus-5") ||
		strings.Contains(mLower, "fable-5")

	isThinkingModel := strings.Contains(mLower, "thinking") ||
		isClaude5 ||
		strings.Contains(mLower, "sonnet-4") ||
		strings.Contains(mLower, "opus-4")

	effort := strings.ToLower(strings.TrimSpace(openAIReq.ReasoningEffort))
	if effort == "" && strings.Contains(mLower, "thinking") {
		effort = "medium"
	}

	var thinking *fastAnthropicThinking
	var outputConfig *fastAnthropicOutputConfig

	if isClaude5 {
		if effort != "" {
			thinking = &fastAnthropicThinking{
				Type: "adaptive",
			}
			outputConfig = &fastAnthropicOutputConfig{
				Effort: effort,
			}
			isThinkingModel = true
		}
	} else if effort != "" {
		budget := 4096
		switch effort {
		case "low":
			budget = 2048
		case "medium":
			budget = 4096
		case "high":
			budget = 8192
		}
		thinking = &fastAnthropicThinking{
			Type:         "enabled",
			BudgetTokens: budget,
		}
		if maxTokens <= budget {
			maxTokens = budget + 4096
		}
		isThinkingModel = true
	}

	if isThinkingModel || isClaude5 {
		// Anthropic rejects assistant prefill when thinking is enabled or on Claude 5
		if len(anthropicMsgs) > 0 && anthropicMsgs[len(anthropicMsgs)-1].Role == "assistant" {
			anthropicMsgs = anthropicMsgs[:len(anthropicMsgs)-1]
		}
	}

	if len(anthropicMsgs) == 0 {
		anthropicMsgs = []fastAnthropicMessage{
			{
				Role: "user",
				Content: []fastAnthropicContentBlock{
					{Type: "text", Text: "Hello"},
				},
			},
		}
	}

	// 7. Prompt caching on second-to-last user message
	injectConversationCacheBreakpointFast(anthropicMsgs)

	// 8. Assemble typed Anthropic request
	anthropicReq := fastAnthropicReq{
		Model:         targetModel,
		Messages:      anthropicMsgs,
		System:        anthropicSystemBlocks,
		MaxTokens:     maxTokens,
		Stream:        openAIReq.Stream,
		Thinking:      thinking,
		OutputConfig:  outputConfig,
		Tools:         anthropicTools,
		ToolChoice:    toolChoice,
		StopSequences: stopSeqs,
		CacheControl: &fastCacheControl{
			Type: "ephemeral",
		},
	}

	if thinking == nil && !isThinkingModel {
		anthropicReq.Temperature = openAIReq.Temperature
		anthropicReq.TopP = openAIReq.TopP
	}

	payloadBytes, err := json.Marshal(&anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling anthropic payload: %w", err)
	}

	// 9. Build HTTP request
	baseURL := normalizeAnthropicBaseURL(p.config.BaseURL)
	targetURL := baseURL + "/v1/messages"

	outReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed creating upstream request: %w", err)
	}

	outReq.Header.Set("content-type", "application/json")
	if p.config.APIKey != "" {
		outReq.Header.Set("x-api-key", p.config.APIKey)
	}
	outReq.Header.Set("anthropic-version", AnthropicAPIVersion)
	outReq.Header.Set("anthropic-beta", AnthropicBetaCaching)

	// Forward custom headers from config
	for k, v := range p.config.Headers {
		if !strings.EqualFold(k, "authorization") && !strings.EqualFold(k, "x-api-key") {
			outReq.Header.Set(k, v)
		}
	}

	return outReq, nil
}

// TranslateResponseBody converts an Anthropic Messages API non-streaming JSON response into standard OpenAI ChatCompletion JSON
// using typed structs and zero dynamic map allocations.
func (p *AnthropicProvider) TranslateResponseBody(statusCode int, body []byte) ([]byte, error) {
	if statusCode != http.StatusOK {
		var aErr fastAnthropicErrResp
		if err := json.Unmarshal(body, &aErr); err == nil && aErr.Error.Message != "" {
			var errOut fastOpenAIErrOut
			errOut.Error.Message = aErr.Error.Message
			errOut.Error.Type = aErr.Error.Type
			errOut.Error.Code = statusCode
			return json.Marshal(errOut)
		}
		return body, nil
	}

	var aResp fastAnthropicResp
	if err := json.Unmarshal(body, &aResp); err != nil {
		return body, fmt.Errorf("failed parsing anthropic response: %w", err)
	}

	var textBuilder strings.Builder
	var reasoningBuilder strings.Builder
	var toolCalls []fastOpenAIToolCallOut

	for _, block := range aResp.Content {
		switch block.Type {
		case "text":
			textBuilder.WriteString(block.Text)
		case "thinking":
			reasoningBuilder.WriteString(block.Thinking)
		case "tool_use":
			toolCalls = append(toolCalls, fastOpenAIToolCallOut{
				ID:   block.ID,
				Type: "function",
				Function: fastOpenAIToolCallFnOut{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}

	finishReason := mapStopReason(aResp.StopReason)
	if len(toolCalls) > 0 && finishReason == "stop" {
		finishReason = "tool_calls"
	}

	promptTokens := aResp.Usage.InputTokens + aResp.Usage.CacheReadInputTokens + aResp.Usage.CacheCreationInputTokens
	completionTokens := aResp.Usage.OutputTokens
	totalTokens := promptTokens + completionTokens

	openAIResp := fastOpenAICompletionOut{
		ID:      aResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   aResp.Model,
		Choices: []fastOpenAIChoiceOut{
			{
				Index: 0,
				Message: fastOpenAIMessageOut{
					Role:             "assistant",
					Content:          textBuilder.String(),
					ReasoningContent: reasoningBuilder.String(),
					ToolCalls:        toolCalls,
				},
				FinishReason: finishReason,
			},
		},
		Usage: fastOpenAIUsageOut{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
			PromptTokensDetails: &fastPromptTokensDetails{
				CachedTokens: aResp.Usage.CacheReadInputTokens,
			},
		},
	}

	return json.Marshal(openAIResp)
}

// ----------------- Typed Fast Request Structs (0 Dynamic Maps) -----------------

type fastOpenAIReq struct {
	Model               string           `json:"model"`
	Messages            []fastOpenAIMsg  `json:"messages"`
	MaxTokens           *int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int             `json:"max_completion_tokens,omitempty"`
	Temperature         *float64         `json:"temperature,omitempty"`
	TopP                *float64         `json:"top_p,omitempty"`
	Stream              bool             `json:"stream,omitempty"`
	Tools               []fastOpenAITool `json:"tools,omitempty"`
	ToolChoice          json.RawMessage  `json:"tool_choice,omitempty"`
	Stop                json.RawMessage  `json:"stop,omitempty"`
	ReasoningEffort     string           `json:"reasoning_effort,omitempty"`
}

type fastAnthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type fastAnthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type fastOpenAIMsg struct {
	Role       string               `json:"role"`
	Content    json.RawMessage      `json:"content"`
	ToolCalls  []fastOpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	Name       string               `json:"name,omitempty"`
}

type fastOpenAIToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function fastOpenAIToolCallFn `json:"function"`
}

type fastOpenAIToolCallFn struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type fastOpenAITool struct {
	Type     string            `json:"type"`
	Function *fastOpenAIToolFn `json:"function,omitempty"`
}

type fastOpenAIToolFn struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type fastAnthropicReq struct {
	Model         string                      `json:"model"`
	Messages      []fastAnthropicMessage      `json:"messages"`
	System        []fastAnthropicContentBlock `json:"system,omitempty"`
	MaxTokens     int                         `json:"max_tokens"`
	Stream        bool                        `json:"stream,omitempty"`
	Temperature   *float64                    `json:"temperature,omitempty"`
	TopP          *float64                    `json:"top_p,omitempty"`
	Thinking      *fastAnthropicThinking      `json:"thinking,omitempty"`
	OutputConfig  *fastAnthropicOutputConfig  `json:"output_config,omitempty"`
	Tools         []fastAnthropicTool         `json:"tools,omitempty"`
	ToolChoice    *fastAnthropicToolChoice    `json:"tool_choice,omitempty"`
	StopSequences []string                    `json:"stop_sequences,omitempty"`
	CacheControl  *fastCacheControl           `json:"cache_control,omitempty"`
}

type fastAnthropicMessage struct {
	Role    string                      `json:"role"`
	Content []fastAnthropicContentBlock `json:"content"`
}

type fastAnthropicContentBlock struct {
	Type         string               `json:"type"`
	Text         string               `json:"text,omitempty"`
	Source       *fastAnthropicSource `json:"source,omitempty"`
	ID           string               `json:"id,omitempty"`
	Name         string               `json:"name,omitempty"`
	Input        json.RawMessage      `json:"input,omitempty"`
	ToolUseID    string               `json:"tool_use_id,omitempty"`
	Content      string               `json:"content,omitempty"`
	CacheControl *fastCacheControl    `json:"cache_control,omitempty"`
}

type fastAnthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type fastAnthropicTool struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  json.RawMessage   `json:"input_schema"`
	CacheControl *fastCacheControl `json:"cache_control,omitempty"`
}

type fastAnthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type fastCacheControl struct {
	Type string `json:"type"`
}

// ----------------- Typed Fast Response Structs -----------------

type fastAnthropicResp struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Content      []fastAnthropicRespBlock `json:"content"`
	Model        string                   `json:"model"`
	StopReason   string                   `json:"stop_reason"`
	StopSequence *string                  `json:"stop_sequence"`
	Usage        fastAnthropicRespUsage   `json:"usage"`
}

type fastAnthropicRespBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
}

type fastAnthropicRespUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type fastAnthropicErrResp struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type fastOpenAICompletionOut struct {
	ID      string                `json:"id"`
	Object  string                `json:"object"`
	Created int64                 `json:"created"`
	Model   string                `json:"model"`
	Choices []fastOpenAIChoiceOut `json:"choices"`
	Usage   fastOpenAIUsageOut    `json:"usage"`
}

type fastOpenAIChoiceOut struct {
	Index        int                  `json:"index"`
	Message      fastOpenAIMessageOut `json:"message"`
	FinishReason string               `json:"finish_reason"`
}

type fastOpenAIMessageOut struct {
	Role             string                  `json:"role"`
	Content          string                  `json:"content"`
	ReasoningContent string                  `json:"reasoning_content,omitempty"`
	ToolCalls        []fastOpenAIToolCallOut `json:"tool_calls,omitempty"`
}

type fastOpenAIToolCallOut struct {
	ID       string                  `json:"id"`
	Type     string                  `json:"type"`
	Function fastOpenAIToolCallFnOut `json:"function"`
}

type fastOpenAIToolCallFnOut struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type fastOpenAIUsageOut struct {
	PromptTokens        int                      `json:"prompt_tokens"`
	CompletionTokens    int                      `json:"completion_tokens"`
	TotalTokens         int                      `json:"total_tokens"`
	PromptTokensDetails *fastPromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

type fastPromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type fastOpenAIErrOut struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// ----------------- Zero-Alloc Internal Translation Helpers -----------------

func extractTextContentFast(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s
		}
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(trimmed, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Type == "text" || p.Type == "" {
				sb.WriteString(p.Text)
			}
		}
		return sb.String()
	}
	return ""
}

func translateMessagesFast(rawMsgs []fastOpenAIMsg) ([]fastAnthropicMessage, error) {
	var translated []fastAnthropicMessage

	for _, m := range rawMsgs {
		role := m.Role
		blocks, err := convertMessageContentFast(m)
		if err != nil {
			return nil, err
		}

		if len(blocks) == 0 {
			continue
		}

		// Tool messages map to user turns with tool_result blocks
		if role == "tool" {
			role = "user"
		}

		if len(translated) > 0 && translated[len(translated)-1].Role == role {
			// Merge consecutive same-role turns
			translated[len(translated)-1].Content = append(translated[len(translated)-1].Content, blocks...)
		} else {
			translated = append(translated, fastAnthropicMessage{
				Role:    role,
				Content: blocks,
			})
		}
	}

	// Anthropic requires starting with a user turn
	if len(translated) > 0 && translated[0].Role != "user" {
		translated = append([]fastAnthropicMessage{
			{
				Role: "user",
				Content: []fastAnthropicContentBlock{
					{Type: "text", Text: "Hello"},
				},
			},
		}, translated...)
	}

	return translated, nil
}

func convertMessageContentFast(m fastOpenAIMsg) ([]fastAnthropicContentBlock, error) {
	var blocks []fastAnthropicContentBlock

	if m.Role == "tool" {
		contentStr := extractTextContentFast(m.Content)
		blocks = append(blocks, fastAnthropicContentBlock{
			Type:      "tool_result",
			ToolUseID: m.ToolCallID,
			Content:   contentStr,
		})
		return blocks, nil
	}

	// Assistant message with tool calls
	if m.Role == "assistant" && len(m.ToolCalls) > 0 {
		text := extractTextContentFast(m.Content)
		if strings.TrimSpace(text) != "" {
			blocks = append(blocks, fastAnthropicContentBlock{
				Type: "text",
				Text: text,
			})
		}
		for _, tc := range m.ToolCalls {
			// Unescape JSON string arguments into raw object without dynamic map allocation
			inputJSON := tc.Function.Arguments
			trimmed := bytes.TrimSpace(inputJSON)
			if len(trimmed) > 0 && trimmed[0] == '"' {
				var unquoted string
				if err := json.Unmarshal(trimmed, &unquoted); err == nil && len(unquoted) > 0 {
					inputJSON = json.RawMessage(unquoted)
				}
			}
			if len(inputJSON) == 0 || bytes.Equal(bytes.TrimSpace(inputJSON), []byte(`""`)) {
				inputJSON = json.RawMessage(`{}`)
			}

			blocks = append(blocks, fastAnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: inputJSON,
			})
		}
		return blocks, nil
	}

	// Regular text or multimodal content
	if len(m.Content) == 0 {
		return blocks, nil
	}

	trimmed := bytes.TrimSpace(m.Content)
	if len(trimmed) == 0 {
		return blocks, nil
	}

	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err == nil && strings.TrimSpace(text) != "" {
			blocks = append(blocks, fastAnthropicContentBlock{
				Type: "text",
				Text: text,
			})
		}
		return blocks, nil
	}

	// Multimodal array
	var rawParts []struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}
	if err := json.Unmarshal(trimmed, &rawParts); err == nil {
		for _, part := range rawParts {
			switch part.Type {
			case "text":
				if strings.TrimSpace(part.Text) != "" {
					blocks = append(blocks, fastAnthropicContentBlock{
						Type: "text",
						Text: part.Text,
					})
				}
			case "image_url":
				if part.ImageURL != nil && part.ImageURL.URL != "" {
					source := parseImageSourceFast(part.ImageURL.URL)
					if source != nil {
						blocks = append(blocks, fastAnthropicContentBlock{
							Type:   "image",
							Source: source,
						})
					}
				}
			}
		}
	}

	return blocks, nil
}

func parseImageSourceFast(urlStr string) *fastAnthropicSource {
	if strings.HasPrefix(urlStr, "data:") {
		parts := strings.SplitN(urlStr, ",", 2)
		if len(parts) == 2 {
			meta := parts[0]
			data := parts[1]
			mediaType := "image/jpeg"
			semiIdx := strings.Index(meta, ";")
			if semiIdx > 5 {
				mediaType = meta[5:semiIdx]
			}
			return &fastAnthropicSource{
				Type:      "base64",
				MediaType: mediaType,
				Data:      data,
			}
		}
	}
	return nil
}

func translateToolChoiceFast(raw json.RawMessage) (*fastAnthropicToolChoice, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			switch s {
			case "none":
				return nil, true
			case "auto":
				return &fastAnthropicToolChoice{Type: "auto"}, false
			case "required":
				return &fastAnthropicToolChoice{Type: "any"}, false
			default:
				return &fastAnthropicToolChoice{Type: "auto"}, false
			}
		}
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(trimmed, &obj); err == nil && obj.Function.Name != "" {
		return &fastAnthropicToolChoice{
			Type: "tool",
			Name: obj.Function.Name,
		}, false
	}
	return nil, false
}

func injectConversationCacheBreakpointFast(msgs []fastAnthropicMessage) {
	userCount := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			userCount++
			if userCount == 2 {
				if len(msgs[i].Content) > 0 {
					msgs[i].Content[len(msgs[i].Content)-1].CacheControl = &fastCacheControl{
						Type: "ephemeral",
					}
				}
				return
			}
		}
	}
}

func mapStopReason(r string) string {
	switch r {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}

func normalizeAnthropicBaseURL(u string) string {
	cleaned := strings.TrimRight(strings.TrimSpace(u), "/")
	cleaned = strings.TrimSuffix(cleaned, "/v1")
	if cleaned == "" {
		return "https://api.anthropic.com"
	}
	return cleaned
}

func normalizeAnthropicModel(model string) string {
	cleaned := strings.TrimPrefix(strings.TrimSpace(model), "anthropic/")
	mLower := strings.ToLower(cleaned)
	switch {
	case strings.HasPrefix(mLower, "claude-3-7-sonnet"),
		strings.HasPrefix(mLower, "claude-3.7-sonnet"):
		return "claude-sonnet-5"
	case strings.HasPrefix(mLower, "claude-3-opus"),
		strings.HasPrefix(mLower, "claude-3.0-opus"):
		return "claude-opus-5"
	default:
		return cleaned
	}
}
