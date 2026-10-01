// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

func TestAnthropicProvider_Basics(t *testing.T) {
	cfg := contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com/v1",
		APIKey:  "test-anthropic-key",
		Type:    contract.ProviderTypeAnthropic,
	}
	p := provider.NewAnthropicProvider("anthropic-direct", cfg)

	if p.ID() != "anthropic-direct" {
		t.Errorf("expected ID 'anthropic-direct', got %q", p.ID())
	}
	if !strings.Contains(p.Name(), "Anthropic") {
		t.Errorf("expected Name to contain 'Anthropic', got %q", p.Name())
	}
	if p.IsLocal() {
		t.Errorf("expected IsLocal() to be false")
	}
	if p.GetAPIKey() != "test-anthropic-key" {
		t.Errorf("expected APIKey 'test-anthropic-key', got %q", p.GetAPIKey())
	}
	if p.CircuitBreaker() == nil {
		t.Errorf("expected non-nil CircuitBreaker")
	}
}

func TestBuildRequest_SystemExtractionAndCaching(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "sk-ant-test",
		Type:    contract.ProviderTypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "system", "content": "You are a coding assistant."},
			{"role": "developer", "content": "Be concise."},
			{"role": "user", "content": "Hello"}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-4-20250514", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if outReq.URL.String() != "https://api.anthropic.com/v1/messages" {
		t.Errorf("expected target URL https://api.anthropic.com/v1/messages, got %s", outReq.URL.String())
	}
	if outReq.Header.Get("x-api-key") != "sk-ant-test" {
		t.Errorf("missing or invalid x-api-key header")
	}
	if outReq.Header.Get("anthropic-version") != provider.AnthropicAPIVersion {
		t.Errorf("missing or invalid anthropic-version header")
	}

	bodyBytes, _ := io.ReadAll(outReq.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		t.Fatalf("failed unmarshaling output request: %v", err)
	}

	// Model should be overridden with tier target model
	if parsed["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %v", parsed["model"])
	}

	// System blocks should contain concatenated text with cache_control
	sysBlocks, ok := parsed["system"].([]interface{})
	if !ok || len(sysBlocks) != 1 {
		t.Fatalf("expected 1 system block, got %v", parsed["system"])
	}
	firstBlock := sysBlocks[0].(map[string]interface{})
	expectedText := "You are a coding assistant.\n\nBe concise."
	if firstBlock["text"] != expectedText {
		t.Errorf("expected system text %q, got %q", expectedText, firstBlock["text"])
	}
	cc, hasCC := firstBlock["cache_control"].(map[string]interface{})
	if !hasCC || cc["type"] != "ephemeral" {
		t.Errorf("expected ephemeral cache_control on system block, got %v", firstBlock["cache_control"])
	}

	// Top-level automatic caching should be enabled
	topCC, hasTopCC := parsed["cache_control"].(map[string]interface{})
	if !hasTopCC || topCC["type"] != "ephemeral" {
		t.Errorf("expected top-level ephemeral cache_control, got %v", parsed["cache_control"])
	}

	// Messages array should ONLY contain user message
	msgs, ok := parsed["messages"].([]interface{})
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected 1 message in messages array, got %v", parsed["messages"])
	}
	userMsg := msgs[0].(map[string]interface{})
	if userMsg["role"] != "user" {
		t.Errorf("expected role 'user', got %v", userMsg["role"])
	}
}

func TestBuildRequest_ToolResultGroupingAndAlternation(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "sk-ant-test",
		Type:    contract.ProviderTypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "Fetch files"},
			{
				"role": "assistant",
				"content": "Checking files now.",
				"tool_calls": [
					{"id": "call_1", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\": \"a.go\"}"}},
					{"id": "call_2", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\": \"b.go\"}"}}
				]
			},
			{"role": "tool", "tool_call_id": "call_1", "content": "package a"},
			{"role": "tool", "tool_call_id": "call_2", "content": "package b"},
			{"role": "user", "content": "What do you see?"}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-5-sonnet", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bodyBytes, _ := io.ReadAll(outReq.Body)
	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string                 `json:"type"`
				Text      string                 `json:"text,omitempty"`
				ID        string                 `json:"id,omitempty"`
				Name      string                 `json:"name,omitempty"`
				Input     map[string]interface{} `json:"input,omitempty"`
				ToolUseID string                 `json:"tool_use_id,omitempty"`
				Content   string                 `json:"content,omitempty"`
			} `json:"content"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		t.Fatalf("failed unmarshaling request: %v", err)
	}

	// Verify alternation: user -> assistant -> user (merged tools + user prompt)
	if len(parsed.Messages) != 3 {
		t.Fatalf("expected 3 strictly alternating messages, got %d", len(parsed.Messages))
	}

	// 1. First user message
	if parsed.Messages[0].Role != "user" || parsed.Messages[0].Content[0].Text != "Fetch files" {
		t.Errorf("unexpected first message: %+v", parsed.Messages[0])
	}

	// 2. Assistant message with text + 2 tool_use blocks
	astMsg := parsed.Messages[1]
	if astMsg.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %s", astMsg.Role)
	}
	if len(astMsg.Content) != 3 {
		t.Fatalf("expected 3 blocks in assistant message (1 text + 2 tool_use), got %d", len(astMsg.Content))
	}
	if astMsg.Content[0].Type != "text" || astMsg.Content[0].Text != "Checking files now." {
		t.Errorf("unexpected assistant text block: %+v", astMsg.Content[0])
	}
	if astMsg.Content[1].Type != "tool_use" || astMsg.Content[1].ID != "call_1" || astMsg.Content[1].Input["path"] != "a.go" {
		t.Errorf("unexpected tool_use block 1: %+v", astMsg.Content[1])
	}
	if astMsg.Content[2].Type != "tool_use" || astMsg.Content[2].ID != "call_2" || astMsg.Content[2].Input["path"] != "b.go" {
		t.Errorf("unexpected tool_use block 2: %+v", astMsg.Content[2])
	}

	// 3. Merged user message with 2 tool_results AND the subsequent user prompt
	mergedUserMsg := parsed.Messages[2]
	if mergedUserMsg.Role != "user" {
		t.Errorf("expected role 'user', got %s", mergedUserMsg.Role)
	}
	if len(mergedUserMsg.Content) != 3 {
		t.Fatalf("expected 3 blocks in merged user message (2 tool_results + 1 user text), got %d", len(mergedUserMsg.Content))
	}
	if mergedUserMsg.Content[0].Type != "tool_result" || mergedUserMsg.Content[0].ToolUseID != "call_1" || mergedUserMsg.Content[0].Content != "package a" {
		t.Errorf("unexpected tool_result block 1: %+v", mergedUserMsg.Content[0])
	}
	if mergedUserMsg.Content[1].Type != "tool_result" || mergedUserMsg.Content[1].ToolUseID != "call_2" || mergedUserMsg.Content[1].Content != "package b" {
		t.Errorf("unexpected tool_result block 2: %+v", mergedUserMsg.Content[1])
	}
	if mergedUserMsg.Content[2].Type != "text" || mergedUserMsg.Content[2].Text != "What do you see?" {
		t.Errorf("unexpected user text block: %+v", mergedUserMsg.Content[2])
	}
}

func TestBuildRequest_ToolsAndToolChoice(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-5-sonnet",
		"messages": [{"role": "user", "content": "Execute tool"}],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "calc",
					"description": "Calculate math",
					"parameters": {"type": "object", "properties": {"expr": {"type": "string"}}}
				}
			},
			{
				"type": "function",
				"function": {
					"name": "lookup",
					"description": "Lookup constant"
				}
			}
		],
		"tool_choice": "required"
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-5-sonnet", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bodyBytes, _ := io.ReadAll(outReq.Body)
	var parsed struct {
		Tools []struct {
			Name         string                 `json:"name"`
			Description  string                 `json:"description"`
			InputSchema  map[string]interface{} `json:"input_schema"`
			CacheControl *struct {
				Type string `json:"type"`
			} `json:"cache_control,omitempty"`
		} `json:"tools"`
		ToolChoice struct {
			Type string `json:"type"`
		} `json:"tool_choice"`
		MaxTokens int `json:"max_tokens"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		t.Fatalf("failed unmarshaling request: %v", err)
	}

	if len(parsed.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(parsed.Tools))
	}
	if parsed.Tools[0].Name != "calc" || parsed.Tools[0].InputSchema["type"] != "object" {
		t.Errorf("unexpected tool 0: %+v", parsed.Tools[0])
	}
	if parsed.Tools[0].CacheControl != nil {
		t.Errorf("tool 0 should not have cache_control")
	}

	// Last tool should have cache_control breakpoint
	if parsed.Tools[1].Name != "lookup" || parsed.Tools[1].InputSchema["type"] != "object" {
		t.Errorf("unexpected tool 1: %+v", parsed.Tools[1])
	}
	if parsed.Tools[1].CacheControl == nil || parsed.Tools[1].CacheControl.Type != "ephemeral" {
		t.Errorf("expected ephemeral cache_control on last tool, got %+v", parsed.Tools[1].CacheControl)
	}

	// Tool choice "required" should map to {"type": "any"}
	if parsed.ToolChoice.Type != "any" {
		t.Errorf("expected tool_choice type 'any', got %q", parsed.ToolChoice.Type)
	}

	// Default max_tokens should be 8192
	if parsed.MaxTokens != 8192 {
		t.Errorf("expected default max_tokens 8192, got %d", parsed.MaxTokens)
	}
}

func TestBuildRequest_ImageTranslation(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-5-sonnet",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Describe this image:"},
					{"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="}}
				]
			}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-5-sonnet", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bodyBytes, _ := io.ReadAll(outReq.Body)
	var parsed struct {
		Messages []struct {
			Content []struct {
				Type   string `json:"type"`
				Text   string `json:"text,omitempty"`
				Source *struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
				} `json:"source,omitempty"`
			} `json:"content"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		t.Fatalf("failed unmarshaling request: %v", err)
	}

	if len(parsed.Messages) != 1 || len(parsed.Messages[0].Content) != 2 {
		t.Fatalf("expected 1 message with 2 content blocks, got %+v", parsed.Messages)
	}

	imgBlock := parsed.Messages[0].Content[1]
	if imgBlock.Type != "image" || imgBlock.Source == nil {
		t.Fatalf("expected image content block, got %+v", imgBlock)
	}
	if imgBlock.Source.Type != "base64" || imgBlock.Source.MediaType != "image/png" || !strings.HasPrefix(imgBlock.Source.Data, "iVBOR") {
		t.Errorf("unexpected image source payload: %+v", imgBlock.Source)
	}
}

func TestBuildRequest_ThinkingModelSanitization(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-7-sonnet",
		"messages": [{"role": "user", "content": "Solve math"}],
		"temperature": 0.7,
		"top_p": 0.95
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-7-sonnet-thinking", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bodyBytes, _ := io.ReadAll(outReq.Body)
	var rawMap map[string]interface{}
	_ = json.Unmarshal(bodyBytes, &rawMap)

	if _, hasTemp := rawMap["temperature"]; hasTemp {
		t.Errorf("expected temperature to be stripped for thinking model, but was present: %v", rawMap["temperature"])
	}
	if _, hasTopP := rawMap["top_p"]; hasTopP {
		t.Errorf("expected top_p to be stripped for thinking model, but was present: %v", rawMap["top_p"])
	}
}

func TestTranslateResponseBody_Success(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	anthropicJSON := `{
		"id": "msg_01X9Z",
		"type": "message",
		"role": "assistant",
		"model": "claude-3-5-sonnet-20241022",
		"stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "I am calling the tool."},
			{"type": "thinking", "thinking": "Let me reason about this first."},
			{
				"type": "tool_use",
				"id": "toolu_01",
				"name": "lookup_symbol",
				"input": {"symbol": "AnthropicProvider"}
			}
		],
		"usage": {
			"input_tokens": 1500,
			"output_tokens": 350,
			"cache_read_input_tokens": 1200,
			"cache_creation_input_tokens": 300
		}
	}`

	translatedBytes, err := p.TranslateResponseBody(http.StatusOK, []byte(anthropicJSON))
	if err != nil {
		t.Fatalf("unexpected error translating response body: %v", err)
	}

	var openAIResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(translatedBytes, &openAIResp); err != nil {
		t.Fatalf("failed parsing translated OpenAI JSON: %v", err)
	}

	if openAIResp.ID != "msg_01X9Z" || openAIResp.Object != "chat.completion" {
		t.Errorf("unexpected top-level metadata: id=%s object=%s", openAIResp.ID, openAIResp.Object)
	}
	if len(openAIResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(openAIResp.Choices))
	}

	choice := openAIResp.Choices[0]
	if choice.Message.Content != "I am calling the tool." {
		t.Errorf("unexpected message content: %s", choice.Message.Content)
	}
	if choice.Message.ReasoningContent != "Let me reason about this first." {
		t.Errorf("unexpected reasoning content: %s", choice.Message.ReasoningContent)
	}
	if choice.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %s", choice.FinishReason)
	}

	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(choice.Message.ToolCalls))
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "toolu_01" || tc.Function.Name != "lookup_symbol" {
		t.Errorf("unexpected tool call: %+v", tc)
	}
	if !strings.Contains(tc.Function.Arguments, "AnthropicProvider") {
		t.Errorf("unexpected tool call arguments: %s", tc.Function.Arguments)
	}

	// Usage metrics
	if openAIResp.Usage.PromptTokens != 3000 || openAIResp.Usage.CompletionTokens != 350 || openAIResp.Usage.TotalTokens != 3350 {
		t.Errorf("unexpected usage counts: %+v", openAIResp.Usage)
	}
	if openAIResp.Usage.PromptTokensDetails.CachedTokens != 1200 {
		t.Errorf("expected 1200 cached tokens, got %d", openAIResp.Usage.PromptTokensDetails.CachedTokens)
	}
}

func TestTranslateResponseBody_Error(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	anthropicErrJSON := `{
		"type": "error",
		"error": {
			"type": "invalid_request_error",
			"message": "max_tokens: 16000 must be <= 8192"
		}
	}`

	translatedBytes, err := p.TranslateResponseBody(http.StatusBadRequest, []byte(anthropicErrJSON))
	if err != nil {
		t.Fatalf("unexpected error translating error response: %v", err)
	}

	var openAIErr struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(translatedBytes, &openAIErr); err != nil {
		t.Fatalf("failed parsing translated error JSON: %v", err)
	}

	if openAIErr.Error.Message != "max_tokens: 16000 must be <= 8192" {
		t.Errorf("unexpected error message: %s", openAIErr.Error.Message)
	}
	if openAIErr.Error.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, openAIErr.Error.Code)
	}
}

func TestBuildRequest_ToolChoiceNone(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-5-sonnet",
		"messages": [{"role": "user", "content": "Just talk"}],
		"tools": [
			{
				"type": "function",
				"function": {"name": "test_tool"}
			}
		],
		"tool_choice": "none"
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-5-sonnet", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bodyBytes, _ := io.ReadAll(outReq.Body)
	var raw map[string]interface{}
	_ = json.Unmarshal(bodyBytes, &raw)

	if _, hasTools := raw["tools"]; hasTools {
		t.Errorf("expected tools to be omitted when tool_choice is none, but got: %v", raw["tools"])
	}
	if _, hasToolChoice := raw["tool_choice"]; hasToolChoice {
		t.Errorf("expected tool_choice to be omitted when none, but got: %v", raw["tool_choice"])
	}
}

func TestBuildRequest_ThinkingModelPrefillStripping(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-7-sonnet-thinking",
		"messages": [
			{"role": "user", "content": "Solve problem"},
			{"role": "assistant", "content": "Thinking..."}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-7-sonnet-thinking", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bodyBytes, _ := io.ReadAll(outReq.Body)
	var raw struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(bodyBytes, &raw)

	if len(raw.Messages) != 1 || raw.Messages[0].Role != "user" {
		t.Fatalf("expected trailing assistant prefill to be stripped for thinking model, got: %+v", raw.Messages)
	}
}

func TestBuildRequest_ReasoningEffortToThinking(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	// Claude 5 models use adaptive thinking + output_config.effort
	t.Run("Claude 5 Adaptive Thinking", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-sonnet-5",
			"messages": [{"role": "user", "content": "Explain quantum computing"}],
			"reasoning_effort": "medium"
		}`

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bodyBytes, _ := io.ReadAll(outReq.Body)
		var raw struct {
			Model    string `json:"model"`
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
			OutputConfig struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}

		if raw.Model != "claude-sonnet-5" {
			t.Errorf("expected model claude-sonnet-5, got %s", raw.Model)
		}
		if raw.Thinking.Type != "adaptive" {
			t.Errorf("expected thinking adaptive, got: %+v", raw.Thinking)
		}
		if raw.OutputConfig.Effort != "medium" {
			t.Errorf("expected output_config.effort medium, got: %+v", raw.OutputConfig)
		}
	})

	// Legacy / Claude 4 models use enabled thinking + budget_tokens
	t.Run("Claude 4 Budget Thinking", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-sonnet-4-6",
			"messages": [{"role": "user", "content": "Explain quantum computing"}],
			"reasoning_effort": "medium"
		}`

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-4-6", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bodyBytes, _ := io.ReadAll(outReq.Body)
		var raw struct {
			Thinking struct {
				Type         string `json:"type"`
				BudgetTokens int    `json:"budget_tokens"`
			} `json:"thinking"`
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}

		if raw.Thinking.Type != "enabled" || raw.Thinking.BudgetTokens != 4096 {
			t.Errorf("expected thinking enabled with budget 4096, got: %+v", raw.Thinking)
		}
		if raw.MaxTokens <= raw.Thinking.BudgetTokens {
			t.Errorf("expected max_tokens (%d) > budget (%d)", raw.MaxTokens, raw.Thinking.BudgetTokens)
		}
	})

	// Legacy 3.7 model normalization
	t.Run("Legacy Model Normalization", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-3-7-sonnet-20250219",
			"messages": [{"role": "user", "content": "Explain quantum computing"}],
			"reasoning_effort": "medium"
		}`

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-7-sonnet-20250219", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bodyBytes, _ := io.ReadAll(outReq.Body)
		var raw struct {
			Model    string `json:"model"`
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
			OutputConfig struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}

		if raw.Model != "claude-sonnet-5" {
			t.Errorf("expected legacy model to be normalized to claude-sonnet-5, got %s", raw.Model)
		}
		if raw.Thinking.Type != "adaptive" || raw.OutputConfig.Effort != "medium" {
			t.Errorf("expected adaptive thinking for normalized claude-sonnet-5, got: %+v %+v", raw.Thinking, raw.OutputConfig)
		}
	})
}

func TestAnthropicProvider_CoverageExtensions(t *testing.T) {
	// 1. BaseURL
	cfg := contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com/v1",
		APIKey:  "sk-test",
		Type:    contract.ProviderTypeAnthropic,
	}
	p := provider.NewAnthropicProvider("anthropic", cfg)
	if p.BaseURL() != "https://api.anthropic.com/v1" {
		t.Errorf("expected BaseURL %q, got %q", "https://api.anthropic.com/v1", p.BaseURL())
	}

	// 2. Ping success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pServer := provider.NewAnthropicProvider("test-ping", contract.ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "sk-test",
		Type:    contract.ProviderTypeAnthropic,
	})
	if err := pServer.Ping(context.Background()); err != nil {
		t.Errorf("expected ping to succeed, got %v", err)
	}

	// Ping failure with closed server
	server.Close()
	if err := pServer.Ping(context.Background()); err == nil {
		t.Errorf("expected ping to fail on closed server")
	}

	// 3. WrapResponseStream
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	}
	rc := p.WrapResponseStream(mockResp)
	if rc == nil {
		t.Errorf("expected non-nil WrapResponseStream reader")
	}
	_ = rc.Close()

	// 4. Tool Choice translation variations ("none", "required", "any", function object, invalid)
	t.Run("ToolChoice Variations", func(t *testing.T) {
		variations := []struct {
			toolChoice string
			validate   func(t *testing.T, outReq *http.Request)
		}{
			{
				toolChoice: `"none"`,
				validate: func(t *testing.T, outReq *http.Request) {
					var raw map[string]interface{}
					_ = json.NewDecoder(outReq.Body).Decode(&raw)
					if _, hasTools := raw["tools"]; hasTools {
						t.Errorf("expected no tools field when tool_choice is none")
					}
				},
			},
			{
				toolChoice: `"required"`,
				validate: func(t *testing.T, outReq *http.Request) {
					var raw map[string]interface{}
					_ = json.NewDecoder(outReq.Body).Decode(&raw)
					tc, ok := raw["tool_choice"].(map[string]interface{})
					if !ok || tc["type"] != "any" {
						t.Errorf("expected tool_choice type 'any' for required, got: %v", raw["tool_choice"])
					}
				},
			},
			{
				toolChoice: `{"type": "function", "function": {"name": "calculator"}}`,
				validate: func(t *testing.T, outReq *http.Request) {
					var raw map[string]interface{}
					_ = json.NewDecoder(outReq.Body).Decode(&raw)
					tc, ok := raw["tool_choice"].(map[string]interface{})
					if !ok || tc["type"] != "tool" || tc["name"] != "calculator" {
						t.Errorf("expected tool_choice tool calculator, got: %v", raw["tool_choice"])
					}
				},
			},
		}

		for _, v := range variations {
			openAIJSON := `{
				"model": "claude-sonnet-5",
				"messages": [{"role": "user", "content": "Compute 2+2"}],
				"tools": [{"type": "function", "function": {"name": "calculator", "description": "calc"}}],
				"tool_choice": ` + v.toolChoice + `
			}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
			if err != nil {
				t.Fatalf("unexpected error for tool_choice %s: %v", v.toolChoice, err)
			}
			v.validate(t, outReq)
		}
	})

	// 5. extractTextContentFast and message variations (array content with text/other, empty, developer role)
	t.Run("Complex Message Content", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-opus-5",
			"messages": [
				{
					"role": "system",
					"content": [
						{"type": "text", "text": "System part 1. "},
						{"type": "text", "text": "System part 2."}
					]
				},
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "Question text"}
					]
				}
			]
		}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-opus-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var raw struct {
			System []struct {
				Text string `json:"text"`
			} `json:"system"`
		}
		_ = json.NewDecoder(outReq.Body).Decode(&raw)
		if len(raw.System) != 1 || raw.System[0].Text != "System part 1. System part 2." {
			t.Errorf("expected concatenated array system text, got: %+v", raw.System)
		}
	})

	// 6. translateResponseBody variations (error response, stop_reason variations)
	t.Run("TranslateResponseBody Edge Cases", func(t *testing.T) {
		// Error response pass-through
		errResp := `{"error":{"type":"rate_limit_error","message":"rate exceeded"}}`
		outBytes, err := p.TranslateResponseBody(429, []byte(errResp))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(outBytes), "rate exceeded") {
			t.Errorf("expected error pass-through, got: %s", string(outBytes))
		}

		// Different stop_reasons: max_tokens, stop_sequence, pause
		stopReasons := []struct {
			anthropicReason string
			expectedReason  string
		}{
			{"max_tokens", "length"},
			{"stop_sequence", "stop"},
			{"pause", "stop"},
		}
		for _, sr := range stopReasons {
			anthResp := `{
				"id": "msg_sr",
				"type": "message",
				"role": "assistant",
				"model": "claude-3-5-sonnet",
				"content": [{"type": "text", "text": "Done"}],
				"stop_reason": "` + sr.anthropicReason + `",
				"usage": {"input_tokens": 10, "output_tokens": 5}
			}`
			translated, err := p.TranslateResponseBody(200, []byte(anthResp))
			if err != nil {
				t.Fatalf("failed translating: %v", err)
			}
			var openAIResp struct {
				Choices []struct {
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}
			_ = json.Unmarshal(translated, &openAIResp)
			if len(openAIResp.Choices) == 0 || openAIResp.Choices[0].FinishReason != sr.expectedReason {
				t.Errorf("for stop_reason %s expected finish_reason %s, got: %+v", sr.anthropicReason, sr.expectedReason, openAIResp.Choices)
			}
		}
	})

	// 7. Assistant prefill removal when thinking/Claude-5 is active, and assistant-start prepends user
	t.Run("Assistant Prefill and Message Ordering", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-sonnet-5",
			"messages": [
				{"role": "assistant", "content": "Assistant start"},
				{"role": "user", "content": "Follow up"},
				{"role": "assistant", "content": "I will think about this..."}
			],
			"reasoning_effort": "high"
		}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var raw struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(outReq.Body).Decode(&raw)
		// Should have prepended user turn, and removed trailing assistant prefill
		if len(raw.Messages) == 0 || raw.Messages[0].Role != "user" {
			t.Errorf("expected first message to be user, got: %+v", raw.Messages)
		}
		if raw.Messages[len(raw.Messages)-1].Role == "assistant" {
			t.Errorf("expected trailing assistant prefill to be stripped when thinking is enabled")
		}
	})

	// 8. MaxCompletionTokens and Multi-Stop Sequences
	t.Run("MaxCompletionTokens and Multi-Stop", func(t *testing.T) {
		openAIJSON := `{
			"max_completion_tokens": 1500,
			"stop": ["END", "STOP"]
		}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		// Test empty model fallback to "claude-sonnet-5"
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var raw struct {
			Model         string   `json:"model"`
			MaxTokens     int      `json:"max_tokens"`
			StopSequences []string `json:"stop_sequences"`
		}
		_ = json.NewDecoder(outReq.Body).Decode(&raw)
		if raw.Model != "claude-sonnet-5" {
			t.Errorf("expected default model claude-sonnet-5, got %s", raw.Model)
		}
		if raw.MaxTokens != 1500 {
			t.Errorf("expected max_tokens 1500, got %d", raw.MaxTokens)
		}
		if len(raw.StopSequences) != 2 || raw.StopSequences[0] != "END" {
			t.Errorf("expected multi stop sequences [END, STOP], got: %v", raw.StopSequences)
		}
	})

	// 9. Single stop string and invalid JSON body
	t.Run("Single Stop and Invalid JSON", func(t *testing.T) {
		openAIJSON := `{"stop": "HALT"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "anthropic/claude-3-opus", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var raw struct {
			Model         string   `json:"model"`
			StopSequences []string `json:"stop_sequences"`
		}
		_ = json.NewDecoder(outReq.Body).Decode(&raw)
		if raw.Model != "claude-opus-5" {
			t.Errorf("expected claude-opus-5, got %s", raw.Model)
		}
		if len(raw.StopSequences) != 1 || raw.StopSequences[0] != "HALT" {
			t.Errorf("expected stop sequences [HALT], got: %v", raw.StopSequences)
		}

		if _, err := p.BuildUpstreamRequest(context.Background(), req, "m", []byte("invalid json")); err == nil {
			t.Errorf("expected error on invalid JSON")
		}
	})

	// 10. Normalization of empty URL and model variants, plus lazy CircuitBreaker
	t.Run("Normalization Variants and Lazy CircuitBreaker", func(t *testing.T) {
		pEmpty := provider.NewAnthropicProvider("empty", contract.ProviderConfig{})
		if pEmpty.BaseURL() != "" {
			t.Errorf("expected empty config base URL preserved")
		}

		var nilP *provider.AnthropicProvider
		// Test lazy CB on non-nil empty struct
		emptyStruct := &provider.AnthropicProvider{}
		if emptyStruct.CircuitBreaker() == nil {
			t.Errorf("expected non-nil circuit breaker from lazy init")
		}
		_ = nilP
	})

	// 11. Multi-turn same-role merge, empty block continue, and tool role mapping
	t.Run("Message Roles Merging and Tool Mapping", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-sonnet-5",
			"messages": [
				{"role": "user", "content": "First user message"},
				{"role": "user", "content": "Second consecutive user message"},
				{"role": "user", "content": ""},
				{"role": "assistant", "content": "Calling tool", "tool_calls": [{"id":"call_1","type":"function","function":{"name":"calc","arguments":"{}"}}]},
				{"role": "tool", "tool_call_id": "call_1", "content": "4"}
			]
		}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var raw struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type      string `json:"type"`
					Text      string `json:"text"`
					ToolUseID string `json:"tool_use_id"`
				} `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(outReq.Body).Decode(&raw)
		// Consecutive user turns merged into 1 user turn with 2 blocks
		if len(raw.Messages) < 3 {
			t.Fatalf("expected at least 3 messages, got %d", len(raw.Messages))
		}
		if raw.Messages[0].Role != "user" || len(raw.Messages[0].Content) != 2 {
			t.Errorf("expected merged user turn with 2 text blocks, got: %+v", raw.Messages[0])
		}
		// Last turn is tool result mapped to user
		lastMsg := raw.Messages[len(raw.Messages)-1]
		if lastMsg.Role != "user" || lastMsg.Content[0].Type != "tool_result" {
			t.Errorf("expected tool result block in last user turn, got: %+v", lastMsg)
		}
	})

	// 12. TranslateResponseBody error branches and tool_calls finish_reason rewrite
	t.Run("TranslateResponseBody Edge Branches", func(t *testing.T) {
		// Non-200 with empty error message
		rawHTML := []byte("<html>502 Bad Gateway</html>")
		res, err := p.TranslateResponseBody(502, rawHTML)
		if err != nil || string(res) != string(rawHTML) {
			t.Errorf("expected pass-through on non-JSON error")
		}

		// Invalid JSON on 200 OK
		_, err = p.TranslateResponseBody(200, []byte("invalid json"))
		if err == nil {
			t.Errorf("expected error on invalid JSON 200 response")
		}

		// Tool use in content with finish_reason rewritten to tool_calls
		toolResp := `{
			"id": "msg_tool",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-5",
			"content": [
				{"type": "tool_use", "id": "call_1", "name": "calc", "input": {"x": 1}}
			],
			"stop_reason": "stop",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`
		translated, err := p.TranslateResponseBody(200, []byte(toolResp))
		if err != nil {
			t.Fatalf("failed translating tool resp: %v", err)
		}
		var openAIResp struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					ToolCalls []struct {
						ID       string `json:"id"`
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		_ = json.Unmarshal(translated, &openAIResp)
		if len(openAIResp.Choices) == 0 || openAIResp.Choices[0].FinishReason != "tool_calls" {
			t.Errorf("expected finish_reason tool_calls, got: %+v", openAIResp.Choices)
		}
		if len(openAIResp.Choices[0].Message.ToolCalls) == 0 || openAIResp.Choices[0].Message.ToolCalls[0].Function.Name != "calc" {
			t.Errorf("expected tool_call function name calc, got: %+v", openAIResp.Choices[0].Message.ToolCalls)
		}
	})

	// 13. Tool choice auto, default string, and empty object
	t.Run("ToolChoice Auto, Custom String and Empty Object", func(t *testing.T) {
		choices := []string{
			`"auto"`,
			`"custom_unknown"`,
			`{"type":"function","function":{"name":""}}`,
			`{"type":"custom"}`,
			`"   "`,
		}
		for _, tc := range choices {
			openAIJSON := `{
				"model": "claude-sonnet-5",
				"messages": [{"role": "user", "content": "Compute"}],
				"tools": [{"type": "function", "function": {"name": "calc", "description": "calc"}}],
				"tool_choice": ` + tc + `
			}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			_, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
			if err != nil {
				t.Fatalf("unexpected error for tool_choice %s: %v", tc, err)
			}
		}
	})

	// 14. Multimodal image edge cases (non-data URL, nil ImageURL, empty text in multimodal)
	t.Run("Multimodal Image Edge Cases", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-sonnet-5",
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "   "},
						{"type": "image_url", "image_url": {"url": "http://example.com/not-data-uri.png"}},
						{"type": "image_url", "image_url": null}
					]
				}
			]
		}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if outReq == nil {
			t.Errorf("expected non-nil request")
		}
	})

	// 15. Empty BaseURL and system message with whitespace/number only
	t.Run("Empty BaseURL and Whitespace System", func(t *testing.T) {
		pDefaultURL := provider.NewAnthropicProvider("default-url", contract.ProviderConfig{
			BaseURL: "",
			APIKey:  "sk-test",
		})
		openAIJSON := `{
			"model": "claude-sonnet-5",
			"messages": [
				{"role": "system", "content": "   "},
				{"role": "developer", "content": 12345},
				{"role": "user", "content": "hello"}
			]
		}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := pDefaultURL.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if outReq.URL.String() != "https://api.anthropic.com/v1/messages" {
			t.Errorf("expected default base URL https://api.anthropic.com/v1/messages, got %s", outReq.URL.String())
		}
	})
}
