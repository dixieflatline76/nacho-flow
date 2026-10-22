// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/nts"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

func TestAnthropicProvider_Basics(t *testing.T) {
	cfg := contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com/v1",
		APIKey:  "test-anthropic-key",
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		if raw.MaxTokens != provider.DefaultAdaptiveThinkingMaxTokens {
			t.Errorf("expected adaptive thinking max_tokens %d, got %d", provider.DefaultAdaptiveThinkingMaxTokens, raw.MaxTokens)
		}
	})

	// Claude Haiku 5.5 uses adaptive thinking + summarized display + output_config.effort
	t.Run("Claude Haiku 5.5 Adaptive Thinking", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-haiku-5-5",
			"messages": [{"role": "user", "content": "Explain quantum computing"}],
			"reasoning_effort": "medium"
		}`

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bodyBytes, _ := io.ReadAll(outReq.Body)
		var raw struct {
			Model    string `json:"model"`
			Thinking struct {
				Type    string `json:"type"`
				Display string `json:"display"`
			} `json:"thinking"`
			OutputConfig struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
			Temperature *float64 `json:"temperature"`
			TopP        *float64 `json:"top_p"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}

		if raw.Model != "claude-haiku-5-5" {
			t.Errorf("expected model claude-haiku-5-5, got %s", raw.Model)
		}
		if raw.Thinking.Type != "adaptive" || raw.Thinking.Display != "summarized" {
			t.Errorf("expected thinking adaptive with display summarized, got: %+v", raw.Thinking)
		}
		if raw.OutputConfig.Effort != "medium" {
			t.Errorf("expected output_config.effort medium, got: %+v", raw.OutputConfig)
		}
		if raw.Temperature != nil || raw.TopP != nil {
			t.Errorf("expected temperature and top_p omitted for Haiku 5.5, got temp=%v, top_p=%v", raw.Temperature, raw.TopP)
		}
	})

	t.Run("Claude Haiku 5.5 Normalization and High Effort", func(t *testing.T) {
		openAIJSON := `{
			"model": "anthropic/claude-haiku-5.5",
			"messages": [{"role": "user", "content": "Quick sort implementation"}],
			"reasoning_effort": "high"
		}`

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "anthropic/claude-haiku-5.5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bodyBytes, _ := io.ReadAll(outReq.Body)
		var raw struct {
			Model    string `json:"model"`
			Thinking struct {
				Type    string `json:"type"`
				Display string `json:"display"`
			} `json:"thinking"`
			OutputConfig struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}

		if raw.Model != "anthropic/claude-haiku-5.5" {
			t.Errorf("expected verbatim model anthropic/claude-haiku-5.5, got %s", raw.Model)
		}
		if raw.Thinking.Type != "adaptive" || raw.Thinking.Display != "summarized" {
			t.Errorf("expected adaptive summarized thinking, got: %+v", raw.Thinking)
		}
		if raw.OutputConfig.Effort != "high" {
			t.Errorf("expected output_config.effort high, got: %+v", raw.OutputConfig)
		}
	})

	t.Run("Claude Haiku 5.5 Disabled Reasoning", func(t *testing.T) {
		openAIJSON := `{
			"model": "claude-haiku-5-5",
			"messages": [{"role": "user", "content": "Ping"}],
			"reasoning_effort": "none"
		}`

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bodyBytes, _ := io.ReadAll(outReq.Body)
		var raw struct {
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
			OutputConfig *struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}

		if raw.Thinking.Type != "disabled" {
			t.Errorf("expected thinking disabled, got: %+v", raw.Thinking)
		}
		if raw.OutputConfig != nil {
			t.Errorf("expected nil output_config when thinking disabled, got: %+v", raw.OutputConfig)
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
				Type         string `json:"type"`
				BudgetTokens int    `json:"budget_tokens"`
			} `json:"thinking"`
			OutputConfig struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}

		if raw.Model != "claude-3-7-sonnet-20250219" {
			t.Errorf("expected verbatim model claude-3-7-sonnet-20250219, got %s", raw.Model)
		}
		if raw.Thinking.Type != "enabled" || raw.Thinking.BudgetTokens != 4096 {
			t.Errorf("expected budget thinking for claude-3-7-sonnet-20250219, got: %+v", raw.Thinking)
		}
	})
}

func TestAnthropicProvider_CoverageExtensions(t *testing.T) {
	// 1. BaseURL
	cfg := contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com/v1",
		APIKey:  "sk-test",
		Type:    provider.TypeAnthropic,
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
		Type:    provider.TypeAnthropic,
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
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
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
		if raw.Model != "anthropic/claude-3-opus" {
			t.Errorf("expected anthropic/claude-3-opus, got %s", raw.Model)
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
		if pEmpty.BaseURL() != provider.DefaultAnthropicBaseURL {
			t.Errorf("expected default base URL %q, got %q", provider.DefaultAnthropicBaseURL, pEmpty.BaseURL())
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

func TestBuildRequest_NoHTMLEscaping(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "sk-ant-test",
		Type:    provider.TypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "a < b && c > d"}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-5-sonnet", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(outReq.Body)
	if err != nil {
		t.Fatalf("failed reading body: %v", err)
	}

	if strings.Contains(string(body), `\u003c`) || strings.Contains(string(body), `\u003e`) || strings.Contains(string(body), `\u0026`) {
		t.Errorf("BuildUpstreamRequest introduced HTML escape sequences: %s", string(body))
	}
	if !strings.Contains(string(body), "a < b && c > d") {
		t.Errorf("BuildUpstreamRequest did not preserve literal HTML characters: %s", string(body))
	}
}

func TestBuildRequest_ToolSchemaNoHTMLEscaping(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "sk-ant-test",
		Type:    provider.TypeAnthropic,
	})

	openAIJSON := `{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "hello"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "calc",
					"description": "Calculate if x < y & y > z",
					"parameters": {"type": "object", "properties": {"val": {"description": "<foo>"}}}
				}
			}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-5-sonnet", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, err := io.ReadAll(outReq.Body)
	if err != nil {
		t.Fatalf("failed reading body: %v", err)
	}

	if strings.Contains(string(body), `\u003c`) || strings.Contains(string(body), `\u003e`) || strings.Contains(string(body), `\u0026`) {
		t.Errorf("BuildUpstreamRequest escaped tool schema: %s", string(body))
	}
	if !strings.Contains(string(body), "Calculate if x < y & y > z") || !strings.Contains(string(body), "<foo>") {
		t.Errorf("BuildUpstreamRequest did not preserve literal characters in tool schema: %s", string(body))
	}
}

func TestBuildRequest_SendsModelVerbatim(t *testing.T) {
	cfg := contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "test-anthropic-key",
		Type:    provider.TypeAnthropic,
	}
	p := provider.NewAnthropicProvider("anthropic-direct", cfg)

	testCases := []string{
		"claude-3-7-sonnet-20250219",
		"anthropic/claude-opus-4.7",
		"claude-opus-4.7",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			openAIJSON := `{"messages": [{"role": "user", "content": "hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			outReq, err := p.BuildUpstreamRequest(context.Background(), req, tc, []byte(openAIJSON))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			bodyBytes, _ := io.ReadAll(outReq.Body)
			var raw struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(bodyBytes, &raw); err != nil {
				t.Fatalf("failed unmarshaling output: %v", err)
			}
			if raw.Model != tc {
				t.Fatalf("expected verbatim model %q, got %q", tc, raw.Model)
			}
		})
	}

	// Also verify that an empty model string (and empty request body model) returns error
	t.Run("EmptyModelRejected", func(t *testing.T) {
		openAIJSON := `{"messages": [{"role": "user", "content": "hi"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		_, err := p.BuildUpstreamRequest(context.Background(), req, "", []byte(openAIJSON))
		if err == nil {
			t.Fatal("expected error on empty model, got nil")
		}
		if !strings.Contains(err.Error(), "missing required model identifier") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})
}

func TestIsAdaptiveThinkingAnthropicModel(t *testing.T) {
	cases := []struct {
		model    string
		expected bool
	}{
		{"claude-3-5-haiku-20241022", false},
		{"claude-sonnet-4-5-20250929", false},
		{"claude-3-opus-20240229", false},
		{"claude-opus-4-6", false},
		{"claude-haiku-5-5", true},
		{"claude-sonnet-5-5", true},
		{"claude-opus-5-5", true},
		{"claude-opus-4-7", true},
		{"claude-opus-4.7", true},
		{"claude-opus-4-8", true},
		{"claude-opus-4.8", true},
		{"claude-sonnet-5", true},
		{"claude-sonnet-6", true},
	}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			actual := provider.IsAdaptiveThinkingAnthropicModel(tc.model)
			if actual != tc.expected {
				t.Fatalf("for model %q expected %v, got %v", tc.model, tc.expected, actual)
			}
		})
	}
}

func TestBuildRequest_ThinkingDefaults(t *testing.T) {
	cfg := contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "test-anthropic-key",
		Type:    provider.TypeAnthropic,
	}
	p := provider.NewAnthropicProvider("anthropic-direct", cfg)

	// empty effort on adaptive model -> thinking omitted
	t.Run("EmptyEffortAdaptiveModel", func(t *testing.T) {
		openAIJSON := `{"messages": [{"role": "user", "content": "hi"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-sonnet-5", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bodyBytes, _ := io.ReadAll(outReq.Body)
		var raw struct {
			Thinking *struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}
		if raw.Thinking != nil {
			t.Fatalf("expected nil thinking for empty effort, got %+v", raw.Thinking)
		}
	})

	// budget bump applied
	t.Run("BudgetBumpApplied", func(t *testing.T) {
		openAIJSON := `{"messages": [{"role": "user", "content": "hi"}], "reasoning_effort": "high", "max_tokens": 1000}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-3-opus", []byte(openAIJSON))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bodyBytes, _ := io.ReadAll(outReq.Body)
		var raw struct {
			MaxTokens int `json:"max_tokens"`
			Thinking  *struct {
				Type         string `json:"type"`
				BudgetTokens int    `json:"budget_tokens"`
			} `json:"thinking"`
		}
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}
		if raw.Thinking == nil || raw.Thinking.BudgetTokens != 8192 {
			t.Fatalf("expected budget 8192, got %+v", raw.Thinking)
		}
		if raw.MaxTokens != 8192+4096 {
			t.Fatalf("expected bumped max_tokens %d, got %d", 8192+4096, raw.MaxTokens)
		}
		// Explicit max_tokens higher than DefaultAdaptiveThinkingMaxTokens should be preserved
		openAIHighMax := `{"messages": [{"role": "user", "content": "hi"}], "reasoning_effort": "high", "max_tokens": 32768}`
		outReqHigh, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", []byte(openAIHighMax))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bodyHigh, _ := io.ReadAll(outReqHigh.Body)
		var rawHigh struct {
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.Unmarshal(bodyHigh, &rawHigh); err != nil {
			t.Fatalf("failed unmarshaling output: %v", err)
		}
		if rawHigh.MaxTokens != 32768 {
			t.Fatalf("expected explicit max_tokens 32768 preserved, got %d", rawHigh.MaxTokens)
		}
	})
}

func simulateAnthropicThinkingToolStream(t *testing.T, p *provider.AnthropicProvider, thinkingChunks []string, signature, redactedData string, toolIDs []string) {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("event: message_start\n")
	sb.WriteString(`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-haiku-5-5-20261001","usage":{"input_tokens":100,"output_tokens":10}}}` + "\n\n")

	idx := 0
	if signature != "" || len(thinkingChunks) > 0 {
		sb.WriteString("event: content_block_start\n")
		sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_start","index":%d,"content_block":{"type":"thinking","thinking":""}}`+"\n\n", idx))
		for _, ch := range thinkingChunks {
			escaped, _ := json.Marshal(ch)
			sb.WriteString("event: content_block_delta\n")
			sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_delta","index":%d,"delta":{"type":"thinking_delta","thinking":%s}}`+"\n\n", idx, string(escaped)))
		}
		if signature != "" {
			sb.WriteString("event: content_block_delta\n")
			sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_delta","index":%d,"delta":{"type":"signature_delta","signature":%q}}`+"\n\n", idx, signature))
		}
		sb.WriteString("event: content_block_stop\n")
		sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_stop","index":%d}`+"\n\n", idx))
		idx++
	}

	if redactedData != "" {
		sb.WriteString("event: content_block_start\n")
		sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_start","index":%d,"content_block":{"type":"redacted_thinking","data":%q}}`+"\n\n", idx, redactedData))
		sb.WriteString("event: content_block_stop\n")
		sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_stop","index":%d}`+"\n\n", idx))
		idx++
	}

	for _, id := range toolIDs {
		sb.WriteString("event: content_block_start\n")
		sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":%q,"name":"read_file"}}`+"\n\n", idx, id))
		sb.WriteString("event: content_block_delta\n")
		sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"main.go\"}"}}`+"\n\n", idx))
		sb.WriteString("event: content_block_stop\n")
		sb.WriteString(fmt.Sprintf(`data: {"type":"content_block_stop","index":%d}`+"\n\n", idx))
		idx++
	}

	sb.WriteString("event: message_delta\n")
	sb.WriteString(`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":42}}` + "\n\n")
	sb.WriteString("event: message_stop\n")
	sb.WriteString(`data: {"type":"message_stop"}` + "\n\n")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sb.String())),
	}
	stream := p.WrapResponseStream(resp)
	_, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("failed reading wrapped stream: %v", err)
	}
	_ = stream.Close()
}

func TestAnthropicProvider_ThinkingPreservation_WithNTS_And_Fallback(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "sk-ant-test",
		Type:    provider.TypeAnthropic,
	})

	// Simulate 5 streamed tool-calling turns with signed thinking blocks
	// Turn 1: Kickoff (>400 bytes of reasoning so NTS pins it as kickoff anchor)
	kickoffThought := strings.Repeat("Architectural kickoff analysis of the repository layout and types.\n", 8)
	simulateAnthropicThinkingToolStream(t, p, []string{kickoffThought[:200], kickoffThought[200:]}, "sig_turn_1", "", []string{"toolu_1a", "toolu_1b"})

	// Turn 2: Historical turn that NTS will prune (older than last 3 turns)
	turn2Thought := strings.Repeat("Checking intermediate helper file.\n", 8)
	simulateAnthropicThinkingToolStream(t, p, []string{turn2Thought}, "sig_turn_2", "", []string{"toolu_2"})

	// Turns 3, 4, 5: Recent 3 turns that NTS will preserve
	for i := 3; i <= 5; i++ {
		thought := fmt.Sprintf("Step %d detailed analysis with \"quotes\" and\nnewlines. %s", i, strings.Repeat("x", 220))
		simulateAnthropicThinkingToolStream(t, p, []string{thought[:50], thought[50:]}, fmt.Sprintf("sig_turn_%d", i), "", []string{fmt.Sprintf("toolu_%d", i)})
	}

	// Build 5-turn OpenAI conversation where client echoes reasoning_content
	buildMsgs := func(includeReasoning bool) []map[string]interface{} {
		var msgs []map[string]interface{}
		msgs = append(msgs, map[string]interface{}{"role": "user", "content": "Fix the bug"})

		thoughts := []string{
			kickoffThought,
			turn2Thought,
			fmt.Sprintf("Step 3 detailed analysis with \"quotes\" and\nnewlines. %s", strings.Repeat("x", 220)),
			fmt.Sprintf("Step 4 detailed analysis with \"quotes\" and\nnewlines. %s", strings.Repeat("x", 220)),
			fmt.Sprintf("Step 5 detailed analysis with \"quotes\" and\nnewlines. %s", strings.Repeat("x", 220)),
		}
		// Notice Turn 1 sends toolu_1b BEFORE toolu_1a to test Enhancement 2 (reversed parallel tool IDs)
		toolCallsPerTurn := [][]map[string]interface{}{
			{
				{"id": "toolu_1b", "type": "function", "function": map[string]interface{}{"name": "read_file", "arguments": `{"path":"b.go"}`}},
				{"id": "toolu_1a", "type": "function", "function": map[string]interface{}{"name": "read_file", "arguments": `{"path":"a.go"}`}},
			},
			{{"id": "toolu_2", "type": "function", "function": map[string]interface{}{"name": "read_file", "arguments": `{"path":"2.go"}`}}},
			{{"id": "toolu_3", "type": "function", "function": map[string]interface{}{"name": "read_file", "arguments": `{"path":"3.go"}`}}},
			{{"id": "toolu_4", "type": "function", "function": map[string]interface{}{"name": "read_file", "arguments": `{"path":"4.go"}`}}},
			{{"id": "toolu_5", "type": "function", "function": map[string]interface{}{"name": "read_file", "arguments": `{"path":"5.go"}`}}},
		}

		for idx := 0; idx < 5; idx++ {
			asst := map[string]interface{}{
				"role":       "assistant",
				"content":    "",
				"tool_calls": toolCallsPerTurn[idx],
			}
			if includeReasoning {
				asst["reasoning_content"] = thoughts[idx]
			}
			msgs = append(msgs, asst)
			for _, tc := range toolCallsPerTurn[idx] {
				msgs = append(msgs, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": tc["id"],
					"content":      "package main",
				})
			}
		}
		return msgs
	}

	t.Run("ModeA_NTS_Compacted_Payload", func(t *testing.T) {
		payloadMap := map[string]interface{}{
			"model":            "claude-haiku-5-5",
			"reasoning_effort": "low",
			"messages":         buildMsgs(true),
		}
		rawBytes, _ := json.Marshal(payloadMap)

		// Run real Stage 1 NTS compaction (KeepRecentReasoningTurns = 3, PreserveKickoff = true)
		ntsCfg := nts.DefaultConfig()
		ntsCfg.StripHistoricalReasoning = true
		ntsCfg.KeepRecentReasoningTurns = 3
		tr := nts.NewTransformer(ntsCfg)
		compactedBytes, _, err := tr.TransformOpenAI(rawBytes)
		if err != nil {
			t.Fatalf("NTS TransformOpenAI failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", compactedBytes)
		if err != nil {
			t.Fatalf("BuildUpstreamRequest failed: %v", err)
		}
		outBytes, _ := io.ReadAll(outReq.Body)

		var anthropicPayload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type      string  `json:"type"`
					Thinking  *string `json:"thinking"`
					Signature string  `json:"signature"`
					ID        string  `json:"id"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(outBytes, &anthropicPayload); err != nil {
			t.Fatalf("failed unmarshaling anthropic payload: %v", err)
		}

		// Assistant messages in anthropicPayload.Messages are at indices 1, 3, 5, 7, 9
		expectedSigs := map[int]string{
			1: "sig_turn_1", // Kickoff preserved by NTS
			3: "",           // Pruned by NTS (thinking block cleared!)
			5: "sig_turn_3", // Recent 3 preserved by NTS
			7: "sig_turn_4", // Recent 3 preserved by NTS
			9: "sig_turn_5", // Recent 3 preserved by NTS
		}

		for msgIdx, wantSig := range expectedSigs {
			asstMsg := anthropicPayload.Messages[msgIdx]
			if asstMsg.Role != "assistant" {
				t.Fatalf("expected assistant at index %d, got %s", msgIdx, asstMsg.Role)
			}
			firstBlock := asstMsg.Content[0]
			if wantSig == "" {
				if firstBlock.Type == "thinking" {
					t.Errorf("turn at msgIdx %d should have thinking block cleared, but got sig %q", msgIdx, firstBlock.Signature)
				}
			} else {
				if firstBlock.Type != "thinking" || firstBlock.Signature != wantSig {
					t.Errorf("turn at msgIdx %d expected first block thinking with sig %q, got type=%q sig=%q", msgIdx, wantSig, firstBlock.Type, firstBlock.Signature)
				}
				// Ensure parallel tool calls in Turn 1 did NOT duplicate the thinking block
				thinkingCount := 0
				for _, b := range asstMsg.Content {
					if b.Type == "thinking" {
						thinkingCount++
					}
				}
				if thinkingCount != 1 {
					t.Errorf("expected exactly 1 thinking block at msgIdx %d, got %d", msgIdx, thinkingCount)
				}
			}
		}

		// Verify unescaped thinking text on Turn 3 matches exact original string
		turn3Thinking := *anthropicPayload.Messages[5].Content[0].Thinking
		if !strings.Contains(turn3Thinking, "with \"quotes\" and\nnewlines.") {
			t.Errorf("expected properly unescaped thinking text on Turn 3, got %q", turn3Thinking)
		}
	})

	t.Run("ModeB_ClientStripsReasoning_FallbackWindow", func(t *testing.T) {
		payloadMap := map[string]interface{}{
			"model":            "claude-haiku-5-5",
			"reasoning_effort": "low",
			"messages":         buildMsgs(false), // Client stripped all reasoning_content!
		}
		rawBytes, _ := json.Marshal(payloadMap)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", rawBytes)
		if err != nil {
			t.Fatalf("BuildUpstreamRequest failed: %v", err)
		}
		outBytes, _ := io.ReadAll(outReq.Body)

		var anthropicPayload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type      string `json:"type"`
					Signature string `json:"signature"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(outBytes, &anthropicPayload); err != nil {
			t.Fatalf("failed unmarshaling anthropic payload: %v", err)
		}

		// Mode B should still keep Turn 1 (Kickoff) + Turns 3, 4, 5 (last 3) and clear Turn 2!
		expectedSigs := map[int]string{
			1: "sig_turn_1",
			3: "",
			5: "sig_turn_3",
			7: "sig_turn_4",
			9: "sig_turn_5",
		}
		for msgIdx, wantSig := range expectedSigs {
			firstBlock := anthropicPayload.Messages[msgIdx].Content[0]
			if wantSig == "" {
				if firstBlock.Type == "thinking" {
					t.Errorf("Mode B: expected msgIdx %d thinking cleared, got %q", msgIdx, firstBlock.Signature)
				}
			} else if firstBlock.Type != "thinking" || firstBlock.Signature != wantSig {
				t.Errorf("Mode B: msgIdx %d expected sig %q, got type=%q sig=%q", msgIdx, wantSig, firstBlock.Type, firstBlock.Signature)
			}
		}
	})

	t.Run("ModeB_Over512Messages_PreservesKickoffAndTail3", func(t *testing.T) {
		var longMsgs []map[string]interface{}
		longMsgs = append(longMsgs, map[string]interface{}{"role": "user", "content": "Start long session"})

		// Turn 1 at index 1 (Kickoff) + Turn 2 at index 3 (intermediate pruned)
		baseTurns := buildMsgs(false)[1:] // 5 assistant + tool pairs
		longMsgs = append(longMsgs, baseTurns[:4]...)

		// Insert 520 plain user/assistant turns so the tail 3 cached turns sit past message index 525 (> 512)
		for k := 0; k < 260; k++ {
			longMsgs = append(longMsgs,
				map[string]interface{}{"role": "user", "content": "ping"},
				map[string]interface{}{"role": "assistant", "content": "pong"},
			)
		}

		// Append Turns 3, 4, 5 at the very end (> 512 index)
		longMsgs = append(longMsgs, baseTurns[4:]...)

		payloadMap := map[string]interface{}{
			"model":            "claude-haiku-5-5",
			"reasoning_effort": "low",
			"messages":         longMsgs,
		}
		rawBytes, _ := json.Marshal(payloadMap)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", rawBytes)
		if err != nil {
			t.Fatalf("BuildUpstreamRequest failed on >512 messages: %v", err)
		}
		outBytes, _ := io.ReadAll(outReq.Body)

		if !bytes.Contains(outBytes, []byte(`"sig_turn_1"`)) {
			t.Errorf("expected Kickoff sig_turn_1 preserved in >512 message conversation")
		}
		if bytes.Contains(outBytes, []byte(`"sig_turn_2"`)) {
			t.Errorf("expected intermediate sig_turn_2 cleared in >512 message conversation")
		}
		for _, tailSig := range []string{"sig_turn_3", "sig_turn_4", "sig_turn_5"} {
			if !bytes.Contains(outBytes, []byte(tailSig)) {
				t.Errorf("expected tail signature %q (>512 index) preserved in Mode B", tailSig)
			}
		}
	})

	t.Run("ThinkingDisabled_SkipsRehydration", func(t *testing.T) {
		payloadMap := map[string]interface{}{
			"model":            "claude-haiku-5-5",
			"reasoning_effort": "none",
			"messages":         buildMsgs(true),
		}
		rawBytes, _ := json.Marshal(payloadMap)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", rawBytes)
		if err != nil {
			t.Fatalf("BuildUpstreamRequest failed: %v", err)
		}
		outBytes, _ := io.ReadAll(outReq.Body)
		if bytes.Contains(outBytes, []byte(`"signature"`)) {
			t.Errorf("expected zero thinking signatures when reasoning_effort is 'none', got: %s", string(outBytes))
		}
	})
}

func TestAnthropicProvider_EmptyThinkingSignatureOnly_And_Redacted_And_Eviction(t *testing.T) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "sk-ant-test",
		Type:    provider.TypeAnthropic,
	})

	// 1. Haiku 5.5 signature-only block (empty thinking text + valid signature) + redacted_thinking
	simulateAnthropicThinkingToolStream(t, p, nil, "sig_empty_haiku", "redacted_payload_xyz", []string{"toolu_empty_1", "toolu_empty_2"})

	// Test split parallel tool calls across consecutive assistant messages (merged by translateMessagesFast)
	openAIJSON := `{
		"model": "claude-haiku-5-5",
		"reasoning_effort": "low",
		"messages": [
			{"role": "user", "content": "Run checks"},
			{
				"role": "assistant",
				"content": "",
				"tool_calls": [{"id": "toolu_empty_1", "type": "function", "function": {"name": "read_file", "arguments": "{}"}}]
			},
			{
				"role": "assistant",
				"content": "",
				"tool_calls": [{"id": "toolu_empty_2", "type": "function", "function": {"name": "read_file", "arguments": "{}"}}]
			},
			{"role": "tool", "tool_call_id": "toolu_empty_1", "content": "ok1"},
			{"role": "tool", "tool_call_id": "toolu_empty_2", "content": "ok2"}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	outReq, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", []byte(openAIJSON))
	if err != nil {
		t.Fatalf("BuildUpstreamRequest failed: %v", err)
	}
	outBytes, _ := io.ReadAll(outReq.Body)

	// Verify `"thinking":""` is explicitly serialized on the wire (not omitted by omitempty)
	if !bytes.Contains(outBytes, []byte(`"type":"thinking","thinking":"","signature":"sig_empty_haiku"`)) {
		t.Errorf("expected explicit empty thinking field in serialized JSON, got: %s", string(outBytes))
	}
	if !bytes.Contains(outBytes, []byte(`"type":"redacted_thinking","data":"redacted_payload_xyz"`)) {
		t.Errorf("expected redacted_thinking block in serialized JSON, got: %s", string(outBytes))
	}
	// Verify split consecutive assistant messages were merged and thinking blocks were deduplicated
	if bytes.Count(outBytes, []byte(`"sig_empty_haiku"`)) != 1 {
		t.Errorf("expected deduplicated thinking block across merged assistant messages, count=%d", bytes.Count(outBytes, []byte(`"sig_empty_haiku"`)))
	}

	// 2. Test Unary (TranslateResponseBody) capture + Ring Buffer eviction (> 256 entries)
	for i := 0; i < 260; i++ {
		unaryResp := fmt.Sprintf(`{
			"id": "msg_%d",
			"type": "message",
			"role": "assistant",
			"model": "claude-haiku-5-5",
			"stop_reason": "tool_use",
			"content": [
				{"type": "thinking", "thinking": "unary thought %d", "signature": "sig_unary_%d"},
				{"type": "redacted_thinking", "data": "redacted_unary_%d"},
				{"type": "tool_use", "id": "toolu_unary_%d", "name": "read_file", "input": {}}
			],
			"usage": {"input_tokens": 10, "output_tokens": 10}
		}`, i, i, i, i, i)
		if _, err := p.TranslateResponseBody(http.StatusOK, []byte(unaryResp)); err != nil {
			t.Fatalf("TranslateResponseBody failed at %d: %v", i, err)
		}
	}

	// toolu_empty_1 (oldest) should be evicted after 260 new entries, while toolu_unary_259 (newest) is present
	checkEvictionJSON := `{
		"model": "claude-haiku-5-5",
		"reasoning_effort": "low",
		"messages": [
			{"role": "user", "content": "Check"},
			{
				"role": "assistant",
				"reasoning_content": "kept",
				"content": "",
				"tool_calls": [{"id": "toolu_empty_1", "type": "function", "function": {"name": "read_file", "arguments": "{}"}}]
			},
			{"role": "tool", "tool_call_id": "toolu_empty_1", "content": "ok"},
			{
				"role": "assistant",
				"reasoning_content": "unary thought 259",
				"content": "",
				"tool_calls": [{"id": "toolu_unary_259", "type": "function", "function": {"name": "read_file", "arguments": "{}"}}]
			},
			{"role": "tool", "tool_call_id": "toolu_unary_259", "content": "ok"}
		]
	}`
	outReq2, err := p.BuildUpstreamRequest(context.Background(), req, "claude-haiku-5-5", []byte(checkEvictionJSON))
	if err != nil {
		t.Fatalf("BuildUpstreamRequest failed: %v", err)
	}
	outBytes2, _ := io.ReadAll(outReq2.Body)
	if bytes.Contains(outBytes2, []byte(`"sig_empty_haiku"`)) {
		t.Errorf("expected oldest entry toolu_empty_1 to be evicted from 256-slot ring buffer")
	}
	if !bytes.Contains(outBytes2, []byte(`"sig_unary_259"`)) {
		t.Errorf("expected newest entry toolu_unary_259 to be present in ring buffer")
	}
}
