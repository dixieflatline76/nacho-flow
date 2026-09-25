// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package agentregistry_test

import (
	"net/http"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
)

func TestDetectClient_ExplicitHeader(t *testing.T) {
	reg := agentregistry.DefaultRegistry()
	if reg == nil {
		t.Fatal("expected non-nil default registry")
	}

	tests := []struct {
		name     string
		header   string
		value    string
		expected string
	}{
		{"X-Client-ID standard", "X-Client-ID", "cline", "cline"},
		{"X-Client-ID uppercase", "X-Client-ID", "ZOO", "zoo"},
		{"X-Client-ID custom enterprise", "X-Client-ID", "my-custom-agent", "my-custom-agent"},
		{"X-Agent-ID cursor", "X-Agent-ID", "cursor", "cursor"},
		{"X-Agent-ID whitespace trimmed", "X-Agent-ID", "  aider  ", "aider"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := make(http.Header)
			h.Set(tc.header, tc.value)
			got := reg.DetectClient(h, nil)
			if got != tc.expected {
				t.Fatalf("expected client %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestDetectClient_UserAgent(t *testing.T) {
	reg := agentregistry.DefaultRegistry()
	if reg == nil {
		t.Fatal("expected non-nil default registry")
	}

	tests := []struct {
		name      string
		userAgent string
		expected  string
	}{
		{"Cline standard", "cline/3.5.0 (VSCode)", "cline"},
		{"Claude-Dev legacy", "Mozilla/5.0 claude-dev/1.0", "cline"},
		{"Roo-Cline", "roo-cline-extension/3.54.0", "cline"},
		{"Zoo Code", "zoo-code/1.0.0", "zoo"},
		{"Zoo bare", "zoo/2.1", "zoo"},
		{"Cursor", "Cursor/0.40.0 (darwin-arm64)", "cursor"},
		{"Aider", "aider/0.50.0 python-requests", "aider"},
		{"Anthropic CLI", "claude-cli/1.2.0", "anthropic"},
		{"Unknown client", "curl/7.68.0", "unknown"},
		{"Empty user agent", "", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := make(http.Header)
			if tc.userAgent != "" {
				h.Set("User-Agent", tc.userAgent)
			}
			got := reg.DetectClient(h, nil)
			if got != tc.expected {
				t.Fatalf("for User-Agent %q: expected %q, got %q", tc.userAgent, tc.expected, got)
			}
		})
	}
}

func TestDetectClient_ToolPayloadFallback(t *testing.T) {
	reg := agentregistry.DefaultRegistry()
	if reg == nil {
		t.Fatal("expected non-nil default registry")
	}

	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{"Apply patch uniquely cline", `{"tools":[{"name":"apply_patch","description":"..."}]}`, "cline"},
		{"Multi replace uniquely zoo", `{"tools":[{"name":"multi_replace_file_content","description":"..."}]}`, "zoo"},
		{"Modify file uniquely cursor", `{"tools":[{"name":"modify_file","description":"..."}]}`, "cursor"},
		{"Non-unique read_file tool", `{"tools":[{"name":"read_file","description":"..."}]}`, "unknown"},
		{"Empty body", "", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := make(http.Header)
			h.Set("User-Agent", "GenericClient/1.0")
			got := reg.DetectClient(h, []byte(tc.body))
			if got != tc.expected {
				t.Fatalf("for payload %s: expected %q, got %q", tc.name, tc.expected, got)
			}
		})
	}
}

func TestRegisteredAgents(t *testing.T) {
	reg := agentregistry.DefaultRegistry()
	if reg == nil {
		t.Fatal("expected non-nil default registry")
	}

	agents := reg.RegisteredAgents()
	if len(agents) == 0 {
		t.Fatal("expected non-empty registered agents list")
	}

	expectedAgents := []string{"aider", "anthropic", "cline", "cursor", "standard", "zoo"}
	for _, ea := range expectedAgents {
		found := false
		for _, a := range agents {
			if a == ea {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected registered agents to contain %q, got %v", ea, agents)
		}
	}
}

func BenchmarkDetectClient_UserAgent(b *testing.B) {
	reg := agentregistry.DefaultRegistry()
	h := make(http.Header)
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) cline/3.5.0 VSCode/1.93.0")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.DetectClient(h, nil)
	}
}

func BenchmarkDetectClient_ToolPayload(b *testing.B) {
	reg := agentregistry.DefaultRegistry()
	h := make(http.Header)
	h.Set("User-Agent", "GenericClient/1.0")
	body := []byte(`{"messages":[{"role":"user","content":"test"}],"tools":[{"name":"multi_replace_file_content"}]}`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.DetectClient(h, body)
	}
}
