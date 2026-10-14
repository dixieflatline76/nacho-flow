// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestOpenRouterProvider_Basics(t *testing.T) {
	p := NewOpenRouterProvider("openrouter", contract.ProviderConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "sk-or-test-key",
	})

	if p.ID() != "openrouter" {
		t.Errorf("expected ID 'openrouter', got %s", p.ID())
	}
	if p.Name() != "OpenRouter" {
		t.Errorf("expected Name 'OpenRouter', got %s", p.Name())
	}
	if p.BaseURL() != "https://openrouter.ai/api/v1" {
		t.Errorf("expected BaseURL 'https://openrouter.ai/api/v1', got %s", p.BaseURL())
	}
	if p.IsLocal() {
		t.Errorf("expected IsLocal() to be false")
	}
	if p.GetAPIKey() != "sk-or-test-key" {
		t.Errorf("expected APIKey 'sk-or-test-key', got %s", p.GetAPIKey())
	}
	wdp, ok := interface{}(p).(WireDeclaratorProvider)
	if !ok {
		t.Fatalf("expected OpenRouterProvider to implement WireDeclaratorProvider")
	}
	decl := wdp.WireDeclarator()
	if decl == nil {
		t.Fatalf("expected non-nil WireDeclarator")
	}
	if decl.Dialect != ReasoningDialectNested {
		t.Errorf("expected ReasoningDialectNested, got %v", decl.Dialect)
	}
	if !decl.ForbidsEffortNone {
		t.Errorf("expected ForbidsEffortNone to be true")
	}
	if !decl.StripFlatOnNested {
		t.Errorf("expected StripFlatOnNested to be true")
	}
}

func TestOpenRouterProvider_DefaultBaseURL(t *testing.T) {
	// When BaseURL is empty, it should default to the canonical OpenRouter endpoint
	p := NewOpenRouterProvider("openrouter", contract.ProviderConfig{
		APIKey: "sk-or-test-key",
	})
	if p.BaseURL() != DefaultOpenRouterBaseURL {
		t.Errorf("expected default BaseURL %s, got %s", DefaultOpenRouterBaseURL, p.BaseURL())
	}
}

func TestOpenRouterProvider_Headers(t *testing.T) {
	// Default headers should include HTTP-Referer and X-Title
	p := NewOpenRouterProvider("openrouter", contract.ProviderConfig{
		BaseURL: "https://openrouter.ai/api/v1",
	})

	headers := p.GetHeaders()
	if headers["HTTP-Referer"] != DefaultOpenRouterReferer {
		t.Errorf("expected default HTTP-Referer %s, got %s", DefaultOpenRouterReferer, headers["HTTP-Referer"])
	}
	if headers["X-Title"] != DefaultOpenRouterTitle {
		t.Errorf("expected default X-Title %s, got %s", DefaultOpenRouterTitle, headers["X-Title"])
	}

	// Custom headers should be merged, overriding defaults if specified
	pCustom := NewOpenRouterProvider("openrouter", contract.ProviderConfig{
		BaseURL: "https://openrouter.ai/api/v1",
		Headers: map[string]string{
			"HTTP-Referer": "https://custom.app",
			"X-Custom-Hdr": "custom-val",
		},
	})
	customHeaders := pCustom.GetHeaders()
	if customHeaders["HTTP-Referer"] != "https://custom.app" {
		t.Errorf("expected custom HTTP-Referer 'https://custom.app', got %s", customHeaders["HTTP-Referer"])
	}
	if customHeaders["X-Title"] != DefaultOpenRouterTitle {
		t.Errorf("expected default X-Title %s, got %s", DefaultOpenRouterTitle, customHeaders["X-Title"])
	}
	if customHeaders["X-Custom-Hdr"] != "custom-val" {
		t.Errorf("expected custom header 'custom-val', got %s", customHeaders["X-Custom-Hdr"])
	}
}

func TestRegistry_OpenRouterInstantiation(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"openrouter_explicit": {
				Type:    contract.ProviderTypeOpenRouter,
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  "key1",
			},
			"openrouter_legacy_cloud": {
				Type:    contract.ProviderTypeCloud,
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  "key2",
			},
			"generic_cloud": {
				Type:    contract.ProviderTypeCloud,
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "key3",
			},
		},
	}

	reg := NewRegistryFromConfig(cfg)

	p1, ok1 := reg.Get("openrouter_explicit")
	if !ok1 {
		t.Fatalf("expected openrouter_explicit to be found")
	}
	wdp1, ok1 := p1.(WireDeclaratorProvider)
	if !ok1 || wdp1.WireDeclarator().Dialect != ReasoningDialectNested {
		t.Errorf("expected openrouter_explicit to declare ReasoningDialectNested")
	}

	p2, ok2 := reg.Get("openrouter_legacy_cloud")
	if !ok2 {
		t.Fatalf("expected openrouter_legacy_cloud to be found")
	}
	wdp2, ok2 := p2.(WireDeclaratorProvider)
	if !ok2 || wdp2.WireDeclarator().Dialect != ReasoningDialectNested {
		t.Errorf("expected openrouter_legacy_cloud to declare ReasoningDialectNested")
	}

	p3, ok3 := reg.Get("generic_cloud")
	if !ok3 {
		t.Fatalf("expected generic_cloud to be found")
	}
	wdp3, ok3 := p3.(WireDeclaratorProvider)
	if !ok3 || wdp3.WireDeclarator().Dialect != ReasoningDialectFlat {
		t.Errorf("expected generic_cloud to declare ReasoningDialectFlat")
	}
}

func TestOpenRouterProvider_CircuitBreakerAndPing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := NewOpenRouterProvider("openrouter", contract.ProviderConfig{
		BaseURL: ts.URL,
		APIKey:  "test-key",
	})

	if cb := p.CircuitBreaker(); cb == nil {
		t.Fatalf("expected non-nil circuit breaker")
	}

	ctx := context.Background()
	if err := p.Ping(ctx); err != nil {
		t.Errorf("expected Ping() to succeed, got: %v", err)
	}
}

func BenchmarkWireDeclarator(b *testing.B) {
	p := NewOpenRouterProvider("openrouter", contract.ProviderConfig{})
	b.ReportAllocs()
	for b.Loop() {
		if wdp, ok := interface{}(p).(WireDeclaratorProvider); ok {
			_ = wdp.WireDeclarator()
		}
	}
}
