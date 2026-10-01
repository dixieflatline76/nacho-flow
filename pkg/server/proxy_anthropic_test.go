// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
	"github.com/dixieflatline76/nacho-flow/pkg/strategy"
)

// TestProxy_Anthropic_NonStreaming verifies end-to-end proxying of a standard non-streaming
// OpenAI ChatCompletion request to an upstream Anthropic Messages API endpoint.
func TestProxy_Anthropic_NonStreaming(t *testing.T) {
	var capturedPath string
	var capturedAPIKey string
	var capturedVersion string
	var capturedBeta string
	var capturedBody []byte

	mockAnthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAPIKey = r.Header.Get("x-api-key")
		capturedVersion = r.Header.Get("anthropic-version")
		capturedBeta = r.Header.Get("anthropic-beta")
		capturedBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "msg_01X9DPChNuJbeAPCd9x74bC3",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "Hello from native Claude!"
				}
			],
			"model": "claude-3-5-sonnet-20241022",
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 20,
				"output_tokens": 10,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens": 8
			}
		}`))
	}))
	defer mockAnthropic.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"anthropic-direct": {
				Type:    contract.ProviderTypeAnthropic,
				BaseURL: mockAnthropic.URL,
				APIKey:  "sk-ant-test-token-xyz",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Anthropic Tier",
				Model:    "claude-3-5-sonnet-20241022",
				Provider: "anthropic-direct",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"messages": [
			{"role": "user", "content": "Hello, Claude!"}
		]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	// Verify upstream request parameters
	if capturedPath != "/v1/messages" {
		t.Errorf("Expected path '/v1/messages', got '%s'", capturedPath)
	}
	if capturedAPIKey != "sk-ant-test-token-xyz" {
		t.Errorf("Expected x-api-key 'sk-ant-test-token-xyz', got '%s'", capturedAPIKey)
	}
	if capturedVersion != "2023-06-01" {
		t.Errorf("Expected anthropic-version '2023-06-01', got '%s'", capturedVersion)
	}
	if capturedBeta != "prompt-caching-2024-07-31" {
		t.Errorf("Expected anthropic-beta 'prompt-caching-2024-07-31', got '%s'", capturedBeta)
	}

	// Verify upstream request body was translated to Anthropic format
	var parsedAnthropicReq struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(capturedBody, &parsedAnthropicReq); err != nil {
		t.Fatalf("Failed to parse captured Anthropic request body: %v", err)
	}
	if parsedAnthropicReq.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("Expected model 'claude-3-5-sonnet-20241022', got '%s'", parsedAnthropicReq.Model)
	}
	if len(parsedAnthropicReq.Messages) != 1 || parsedAnthropicReq.Messages[0].Content[0].Text != "Hello, Claude!" {
		t.Errorf("Unexpected messages translated: %+v", parsedAnthropicReq.Messages)
	}

	// Verify downstream response returned to client is standard OpenAI ChatCompletion
	var openAIResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
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

	if err := json.Unmarshal(rec.Body.Bytes(), &openAIResp); err != nil {
		t.Fatalf("Failed to unmarshal client response JSON: %v. Body: %s", err, rec.Body.String())
	}

	if openAIResp.Object != "chat.completion" {
		t.Errorf("Expected object 'chat.completion', got '%s'", openAIResp.Object)
	}
	if len(openAIResp.Choices) == 0 {
		t.Fatalf("Expected at least 1 choice in response")
	}
	if openAIResp.Choices[0].Message.Content != "Hello from native Claude!" {
		t.Errorf("Expected content 'Hello from native Claude!', got '%s'", openAIResp.Choices[0].Message.Content)
	}
	if openAIResp.Choices[0].FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got '%s'", openAIResp.Choices[0].FinishReason)
	}
	if openAIResp.Usage.PromptTokens != 28 {
		t.Errorf("Expected prompt_tokens 28, got %d", openAIResp.Usage.PromptTokens)
	}
	if openAIResp.Usage.CompletionTokens != 10 {
		t.Errorf("Expected completion_tokens 10, got %d", openAIResp.Usage.CompletionTokens)
	}
	if openAIResp.Usage.TotalTokens != 38 {
		t.Errorf("Expected total_tokens 38, got %d", openAIResp.Usage.TotalTokens)
	}
	if openAIResp.Usage.PromptTokensDetails.CachedTokens != 8 {
		t.Errorf("Expected cached_tokens 8, got %d", openAIResp.Usage.PromptTokensDetails.CachedTokens)
	}
}

// TestProxy_Anthropic_Streaming verifies end-to-end streaming SSE translation from
// upstream Anthropic SSE to downstream OpenAI SSE chunks.
func TestProxy_Anthropic_Streaming(t *testing.T) {
	mockAnthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_stream_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-3-5-sonnet\",\"stop_reason\":null,\"usage\":{\"input_tokens\":25,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":10}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Streamed \"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Claude answer\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":15}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}

		for _, ev := range events {
			_, _ = w.Write([]byte(ev))
			flusher.Flush()
		}
	}))
	defer mockAnthropic.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"anthropic-stream": {
				Type:    contract.ProviderTypeAnthropic,
				BaseURL: mockAnthropic.URL,
				APIKey:  "sk-ant-test-stream",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Anthropic Stream Tier",
				Model:    "claude-3-5-sonnet",
				Provider: "anthropic-stream",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"stream": true,
		"messages": [{"role": "user", "content": "Stream to me"}]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Streamed ") || !strings.Contains(body, "Claude answer") {
		t.Errorf("Expected content deltas in stream, got:\n%s", body)
	}

	if !strings.Contains(body, `data: [DONE]`) {
		t.Errorf("Expected terminal data: [DONE], got:\n%s", body)
	}

	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Errorf("Expected finish_reason 'stop', got:\n%s", body)
	}

	if !strings.Contains(body, `"prompt_tokens":35`) || !strings.Contains(body, `"completion_tokens":15`) {
		t.Errorf("Expected usage metrics in stream, got:\n%s", body)
	}
}

// TestProxy_Anthropic_ToolUse verifies bidirectional translation of tools and tool results.
func TestProxy_Anthropic_ToolUse(t *testing.T) {
	mockAnthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var parsed struct {
			Tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"input_schema"`
			} `json:"tools"`
		}
		_ = json.Unmarshal(body, &parsed)

		if len(parsed.Tools) != 1 || parsed.Tools[0].Name != "get_weather" {
			t.Errorf("Expected tool 'get_weather', got: %+v", parsed.Tools)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "msg_tool_1",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "tool_use",
					"id": "toolu_01A09q90tc1q visual",
					"name": "get_weather",
					"input": {"location": "San Francisco, CA"}
				}
			],
			"model": "claude-3-5-sonnet",
			"stop_reason": "tool_use",
			"usage": {
				"input_tokens": 50,
				"output_tokens": 20
			}
		}`))
	}))
	defer mockAnthropic.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"anthropic-tools": {
				Type:    contract.ProviderTypeAnthropic,
				BaseURL: mockAnthropic.URL,
				APIKey:  "sk-ant-tool-key",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Anthropic Tools Tier",
				Model:    "claude-3-5-sonnet",
				Provider: "anthropic-tools",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{
		"model": "nacho-hybrid",
		"messages": [{"role": "user", "content": "What is the weather?"}],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get current weather",
					"parameters": {
						"type": "object",
						"properties": {
							"location": {"type": "string"}
						}
					}
				}
			}
		]
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var openAIResp struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role      string `json:"role"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &openAIResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(openAIResp.Choices) == 0 || len(openAIResp.Choices[0].Message.ToolCalls) == 0 {
		t.Fatalf("Expected tool_calls in choices, got %+v", openAIResp)
	}

	tc := openAIResp.Choices[0].Message.ToolCalls[0]
	if tc.Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got '%s'", tc.Function.Name)
	}
	if !strings.Contains(tc.Function.Arguments, "San Francisco") {
		t.Errorf("Expected arguments to contain 'San Francisco', got '%s'", tc.Function.Arguments)
	}
	if openAIResp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("Expected finish_reason 'tool_calls', got '%s'", openAIResp.Choices[0].FinishReason)
	}
}

// TestProxy_Anthropic_ErrorHandling verifies error translation from Anthropic format to OpenAI envelope.
func TestProxy_Anthropic_ErrorHandling(t *testing.T) {
	mockAnthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{
			"type": "error",
			"error": {
				"type": "invalid_request_error",
				"message": "max_tokens: 1000000 > 8192"
			}
		}`))
	}))
	defer mockAnthropic.Close()

	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"anthropic-err": {
				Type:    contract.ProviderTypeAnthropic,
				BaseURL: mockAnthropic.URL,
				APIKey:  "sk-ant-test",
			},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Anthropic Err Tier",
				Model:    "claude-3-5-sonnet",
				Provider: "anthropic-err",
				When:     "true",
			},
		},
	}

	evaluator, _ := strategy.NewExprEvaluator(cfg.Tiers, contract.Tier{})
	classifier := router.NewClassifier()
	sanitizer := router.NewSanitizer()
	srv := NewServer(cfg, evaluator, classifier, sanitizer)

	reqPayload := `{"model": "nacho-hybrid", "messages": [{"role": "user", "content": "hi"}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqPayload))
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Failed to parse error response: %v. Body: %s", err, rec.Body.String())
	}

	if !strings.Contains(errResp.Error.Message, "max_tokens") {
		t.Errorf("Expected error message to contain 'max_tokens', got '%s'", errResp.Error.Message)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("Expected error type 'invalid_request_error', got '%s'", errResp.Error.Type)
	}
}
