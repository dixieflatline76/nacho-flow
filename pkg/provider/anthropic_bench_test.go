// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

func BenchmarkAnthropic_BuildUpstreamRequest(b *testing.B) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "sk-ant-bench",
		Type:    contract.ProviderTypeAnthropic,
	})

	openAIJSON := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "system", "content": "You are a software engineer."},
			{"role": "user", "content": "Refactor this function"},
			{"role": "assistant", "content": "Here is the plan."},
			{"role": "user", "content": "Please write the code."}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "write_to_file",
					"description": "Writes code to a file",
					"parameters": {
						"type": "object",
						"properties": {
							"path": {"type": "string"},
							"content": {"type": "string"}
						},
						"required": ["path", "content"]
					}
				}
			}
		],
		"tool_choice": "auto",
		"max_tokens": 4096
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, err := p.BuildUpstreamRequest(ctx, req, "claude-3-5-sonnet", openAIJSON)
		if err != nil {
			b.Fatalf("BuildUpstreamRequest failed: %v", err)
		}
	}
}

func BenchmarkAnthropic_TranslateResponseBody(b *testing.B) {
	p := provider.NewAnthropicProvider("anthropic", contract.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		Type:    contract.ProviderTypeAnthropic,
	})

	anthropicJSON := []byte(`{
		"id": "msg_01X9Z",
		"type": "message",
		"role": "assistant",
		"model": "claude-3-5-sonnet-20241022",
		"stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "Writing the code now."},
			{
				"type": "tool_use",
				"id": "toolu_01",
				"name": "write_to_file",
				"input": {"path": "main.go", "content": "package main\n\nfunc main() {}\n"}
			}
		],
		"usage": {
			"input_tokens": 1200,
			"output_tokens": 150,
			"cache_read_input_tokens": 800,
			"cache_creation_input_tokens": 0
		}
	}`)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, err := p.TranslateResponseBody(http.StatusOK, anthropicJSON)
		if err != nil {
			b.Fatalf("TranslateResponseBody failed: %v", err)
		}
	}
}
