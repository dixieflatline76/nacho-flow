package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// Test 2.1: OpenAICompatibleProvider capabilities
func TestOpenAICompatibleProvider_Capabilities(t *testing.T) {
	pCfg := contract.ProviderConfig{
		BaseURL: "https://api.langdock.com/v1",
		APIKey:  "secret-langdock-key",
		Type:    TypeOpenAICompatible,
		Headers: map[string]string{
			"X-Langdock-Org": "engineering",
		},
	}

	p := NewOpenAICompatibleProvider("langdock", pCfg)

	if p.ID() != "langdock" {
		t.Errorf("Expected ID 'langdock', got '%s'", p.ID())
	}
	if p.BaseURL() != "https://api.langdock.com/v1" {
		t.Errorf("Expected BaseURL 'https://api.langdock.com/v1', got '%s'", p.BaseURL())
	}
	if p.IsLocal() {
		t.Errorf("Expected IsLocal to be false for cloud provider")
	}

	// Capability check: AuthProvider
	if auth, ok := interface{}(p).(AuthProvider); !ok {
		t.Fatalf("Expected p to implement AuthProvider")
	} else if auth.GetAPIKey() != "secret-langdock-key" {
		t.Errorf("Expected APIKey 'secret-langdock-key', got '%s'", auth.GetAPIKey())
	}

	// Capability check: HeaderProvider
	if hdr, ok := interface{}(p).(HeaderProvider); !ok {
		t.Fatalf("Expected p to implement HeaderProvider")
	} else if hdr.GetHeaders()["X-Langdock-Org"] != "engineering" {
		t.Errorf("Expected header 'X-Langdock-Org: engineering', got '%v'", hdr.GetHeaders())
	}

	// Capability check: CircuitBreakerProvider
	if cbProv, ok := interface{}(p).(CircuitBreakerProvider); !ok {
		t.Fatalf("Expected p to implement CircuitBreakerProvider")
	} else {
		cb := cbProv.CircuitBreaker()
		if cb == nil {
			t.Fatalf("Expected non-nil CircuitBreaker")
		}
		if cb.State() != StateClosed {
			t.Errorf("Expected StateClosed, got %v", cb.State())
		}
	}

	// Lazy init of circuit breaker if nil
	pNilCB := &OpenAICompatibleProvider{}
	if cb := pNilCB.CircuitBreaker(); cb == nil {
		t.Errorf("Expected lazy initialization of circuit breaker, got nil")
	}
}

// Test 2.2: Local provider characteristics
func TestOpenAICompatibleProvider_LocalCharacteristics(t *testing.T) {
	pCfg := contract.ProviderConfig{
		BaseURL: "http://127.0.0.1:11434/v1",
		Type:    TypeOpenAICompatible,
		Local:   true,
	}

	p := NewOpenAICompatibleProvider("ollama", pCfg)

	if !p.IsLocal() {
		t.Errorf("Expected IsLocal to be true for Ollama")
	}
	if p.GetAPIKey() != "" {
		t.Errorf("Expected empty API key for local provider, got '%s'", p.GetAPIKey())
	}
	if len(p.GetHeaders()) != 0 {
		t.Errorf("Expected empty headers for local provider, got '%v'", p.GetHeaders())
	}
}

// Test 2.3: Registry concurrency and lookups
func TestRegistry_RegisterAndGet_Concurrency(t *testing.T) {
	reg := NewRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			providerID := fmt.Sprintf("provider-%d", id)
			p := NewOpenAICompatibleProvider(providerID, contract.ProviderConfig{
				BaseURL: fmt.Sprintf("http://127.0.0.1:%d/v1", 8000+id),
				Type:    TypeOpenAICompatible,
			})
			reg.Register(p)

			retrieved, ok := reg.Get(providerID)
			if !ok || retrieved.ID() != providerID {
				t.Errorf("Failed to retrieve provider %s concurrently", providerID)
			}
		}(i)
	}
	wg.Wait()

	if len(reg.All()) != 100 {
		t.Errorf("Expected 100 registered providers, got %d", len(reg.All()))
	}
}

// Test 2.4: Registry initialization from contract.Config
func TestRegistry_NewFromConfig(t *testing.T) {
	cfg := &contract.Config{
		Port: 8000,
		Providers: map[string]contract.ProviderConfig{
			"ollama": {
				BaseURL: "http://127.0.0.1:11434/v1",
				Type:    TypeOpenAICompatible,
				Local:   true,
			},
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
				Type:    TypeOpenRouter,
				APIKey:  "sk-or-test",
			},
		},
	}

	reg := NewRegistryFromConfig(cfg)

	ollama, ok := reg.Get("ollama")
	if !ok || !ollama.IsLocal() {
		t.Fatalf("Failed to retrieve local ollama from config registry")
	}

	or, ok := reg.Get("openrouter")
	if !ok || or.BaseURL() != "https://openrouter.ai/api/v1" {
		t.Fatalf("Failed to retrieve openrouter from config registry")
	}
}

// Test 2.5: Provider Name resolution
func TestOpenAICompatibleProvider_Names(t *testing.T) {
	cases := []struct {
		id           string
		expectedName string
	}{
		{"ollama_local", "Ollama Local GPU"},
		{"openrouter_gateway", "OpenRouter AI Gateway"},
		{"langdock_corp", "Langdock Enterprise"},
		{"custom_llm", "custom_llm"},
	}

	for _, c := range cases {
		p := NewOpenAICompatibleProvider(c.id, contract.ProviderConfig{BaseURL: "http://localhost", Type: TypeOpenAICompatible})
		if p.Name() != c.expectedName {
			t.Errorf("For id '%s', expected Name '%s', got '%s'", c.id, c.expectedName, p.Name())
		}
	}
}

// Test 2.6: Ping health check
func TestOpenAICompatibleProvider_Ping(t *testing.T) {
	// Success case
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("Expected Authorization 'Bearer test-key', got '%s'", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := NewOpenAICompatibleProvider("test", contract.ProviderConfig{
		BaseURL: ts.URL,
		APIKey:  "test-key",
		Type:    TypeOpenAICompatible,
	})

	if err := p.Ping(context.Background()); err != nil {
		t.Errorf("Expected ping to succeed, got: %v", err)
	}

	// Failure case: unreachable endpoint
	pBroken := NewOpenAICompatibleProvider("broken", contract.ProviderConfig{
		BaseURL: "http://127.0.0.1:54321/unreachable",
		Type:    TypeOpenAICompatible,
	})
	if err := pBroken.Ping(context.Background()); err == nil {
		t.Errorf("Expected ping to fail for unreachable URL, got nil")
	}

	// Failure case: invalid URL syntax
	pInvalidURL := NewOpenAICompatibleProvider("invalid", contract.ProviderConfig{
		BaseURL: "http://\x7f",
		Type:    TypeOpenAICompatible,
	})
	if err := pInvalidURL.Ping(context.Background()); err == nil {
		t.Errorf("Expected ping to fail for invalid URL, got nil")
	}
}

// Test 2.7: Nil config returns empty registry
func TestRegistry_NilConfig(t *testing.T) {
	reg := NewRegistryFromConfig(nil)
	if reg == nil {
		t.Fatalf("Expected non-nil registry")
	}
	if len(reg.All()) != 0 {
		t.Errorf("Expected 0 providers for nil config, got %d", len(reg.All()))
	}
}

func TestRegistry_CircuitsStatusAndReset(t *testing.T) {
	p1 := NewOpenAICompatibleProvider("p1", contract.ProviderConfig{BaseURL: "http://localhost:8001", Type: TypeOpenAICompatible})
	p2 := NewOpenAICompatibleProvider("p2", contract.ProviderConfig{BaseURL: "http://localhost:8002", Type: TypeOpenAICompatible})

	reg := NewRegistry()
	reg.Register(p1)
	reg.Register(p2)

	// Trip p1
	cb1 := p1.CircuitBreaker()
	cb1.RecordFailure()
	cb1.RecordFailure()

	status := reg.GetCircuitsStatus()
	if len(status) != 2 {
		t.Fatalf("expected 2 circuits in status, got %d", len(status))
	}

	// Reset single provider
	if !reg.ResetCircuit("p1") {
		t.Errorf("expected ResetCircuit('p1') to succeed")
	}
	if cb1.State() != StateClosed {
		t.Errorf("expected p1 state closed, got %s", cb1.State())
	}

	// Reset unknown provider
	if reg.ResetCircuit("unknown") {
		t.Errorf("expected ResetCircuit('unknown') to return false")
	}

	// Trip both and reset all
	cb1.RecordFailure()
	cb1.RecordFailure()
	cb2 := p2.CircuitBreaker()
	cb2.RecordFailure()
	cb2.RecordFailure()

	if !reg.ResetCircuit("all") {
		t.Errorf("expected ResetCircuit('all') to succeed")
	}
	if cb1.State() != StateClosed || cb2.State() != StateClosed {
		t.Errorf("expected both closed after reset all")
	}

	// Reset with empty string
	if !reg.ResetCircuit("") {
		t.Errorf("expected ResetCircuit('') to succeed")
	}
}

func TestOpenAICompatibleProvider_BuildUpstreamRequest_And_Stream(t *testing.T) {
	// 1. Cloud provider with Auth and Custom Headers
	pCloud := NewOpenAICompatibleProvider("cloud", contract.ProviderConfig{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test-123",
		Type:    TypeOpenAICompatible,
		Headers: map[string]string{
			"X-Custom-Header": "custom-val",
		},
	})

	inReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o"}`))
	inReq.Header.Set("User-Agent", "TestClient/1.0")

	outReq, err := pCloud.BuildUpstreamRequest(context.Background(), inReq, "gpt-4o", []byte(`{"model":"gpt-4o"}`))
	if err != nil {
		t.Fatalf("BuildUpstreamRequest failed: %v", err)
	}

	if outReq.URL.String() != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("expected URL https://api.openai.com/v1/chat/completions, got %s", outReq.URL.String())
	}
	if outReq.Header.Get("Authorization") != "Bearer sk-test-123" {
		t.Errorf("expected Bearer sk-test-123, got %s", outReq.Header.Get("Authorization"))
	}
	if outReq.Header.Get("X-Custom-Header") != "custom-val" {
		t.Errorf("expected custom header preserved, got %s", outReq.Header.Get("X-Custom-Header"))
	}
	if outReq.Header.Get("User-Agent") != "TestClient/1.0" {
		t.Errorf("expected client header forwarded")
	}

	// 2. Local provider strips Authorization
	pLocal := NewOpenAICompatibleProvider("ollama", contract.ProviderConfig{
		BaseURL: "http://127.0.0.1:11434",
		Type:    TypeOpenAICompatible,
		Local:   true,
	})
	inReqLocal := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)
	inReqLocal.Header.Set("Authorization", "Bearer unwanted-token")

	outReqLocal, err := pLocal.BuildUpstreamRequest(context.Background(), inReqLocal, "llama3", []byte(`{}`))
	if err != nil {
		t.Fatalf("BuildUpstreamRequest local failed: %v", err)
	}
	if outReqLocal.Header.Get("Authorization") != "" {
		t.Errorf("expected Authorization header to be stripped for local provider")
	}
	if outReqLocal.URL.String() != "http://127.0.0.1:11434/chat/completions" {
		t.Errorf("expected URL http://127.0.0.1:11434/chat/completions, got %s", outReqLocal.URL.String())
	}

	// 3. Invalid target URL
	pBad := NewOpenAICompatibleProvider("bad", contract.ProviderConfig{
		BaseURL: "::://bad url",
		Type:    TypeOpenAICompatible,
	})
	if _, err := pBad.BuildUpstreamRequest(context.Background(), inReq, "gpt-4o", nil); err == nil {
		t.Errorf("expected error for invalid target URL")
	}

	// 4. WrapResponseStream and TranslateResponseBody
	mockResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("stream data")),
	}
	rc := pCloud.WrapResponseStream(mockResp)
	b, _ := io.ReadAll(rc)
	if string(b) != "stream data" {
		t.Errorf("expected pass-through stream body")
	}

	rawBody := []byte(`{"result":"ok"}`)
	transBody, err := pCloud.TranslateResponseBody(200, rawBody)
	if err != nil || string(transBody) != string(rawBody) {
		t.Errorf("expected pass-through translated response body")
	}

	// 5. singleJoiningSlash tests
	tests := []struct {
		a, b, want string
	}{
		{"http://a.com/", "/v1", "http://a.com/v1"},
		{"http://a.com", "v1", "http://a.com/v1"},
		{"http://a.com/", "v1", "http://a.com/v1"},
		{"http://a.com", "/v1", "http://a.com/v1"},
	}
	for _, tc := range tests {
		got := singleJoiningSlash(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("singleJoiningSlash(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestRegistry_ProviderTypesAndPlatforms(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"prov_openai": {
				Type:    TypeOpenAICompatible,
				BaseURL: "https://api.openai.com/v1",
			},
			"prov_local": {
				Type:    TypeOpenAICompatible,
				BaseURL: "http://127.0.0.1:11434/v1",
				Local:   true,
			},
			"prov_anthropic": {
				Type:   TypeAnthropic,
				APIKey: "sk-ant-test",
			},
			"prov_openrouter": {
				Type:   TypeOpenRouter,
				APIKey: "sk-or-test",
			},
		},
	}

	reg := NewRegistryFromConfig(cfg)

	// Check openai_compatible non-local
	pOpenAI, ok := reg.Get("prov_openai")
	if !ok {
		t.Fatalf("expected prov_openai registered")
	}
	if _, ok := pOpenAI.(*OpenAICompatibleProvider); !ok {
		t.Errorf("expected *OpenAICompatibleProvider, got %T", pOpenAI)
	}
	if pOpenAI.Platform() != contract.PlatformOpenAICompatible {
		t.Errorf("expected PlatformOpenAICompatible, got %s", pOpenAI.Platform())
	}
	if plat, found := reg.Platform("prov_openai"); !found || plat != contract.PlatformOpenAICompatible {
		t.Errorf("reg.Platform(prov_openai) = (%s, %v), want (%s, true)", plat, found, contract.PlatformOpenAICompatible)
	}

	// Check openai_compatible local
	pLocal, ok := reg.Get("prov_local")
	if !ok {
		t.Fatalf("expected prov_local registered")
	}
	if _, ok := pLocal.(*OpenAICompatibleProvider); !ok {
		t.Errorf("expected *OpenAICompatibleProvider, got %T", pLocal)
	}
	if pLocal.Platform() != contract.PlatformLocal {
		t.Errorf("expected PlatformLocal, got %s", pLocal.Platform())
	}
	if plat, found := reg.Platform("prov_local"); !found || plat != contract.PlatformLocal {
		t.Errorf("reg.Platform(prov_local) = (%s, %v), want (%s, true)", plat, found, contract.PlatformLocal)
	}

	// Check anthropic
	pAnthropic, ok := reg.Get("prov_anthropic")
	if !ok {
		t.Fatalf("expected prov_anthropic registered")
	}
	if _, ok := pAnthropic.(*AnthropicProvider); !ok {
		t.Errorf("expected *AnthropicProvider, got %T", pAnthropic)
	}
	if pAnthropic.Platform() != contract.PlatformAnthropic {
		t.Errorf("expected PlatformAnthropic, got %s", pAnthropic.Platform())
	}
	if plat, found := reg.Platform("prov_anthropic"); !found || plat != contract.PlatformAnthropic {
		t.Errorf("reg.Platform(prov_anthropic) = (%s, %v), want (%s, true)", plat, found, contract.PlatformAnthropic)
	}

	// Check openrouter
	pOR, ok := reg.Get("prov_openrouter")
	if !ok {
		t.Fatalf("expected prov_openrouter registered")
	}
	if _, ok := pOR.(*OpenRouterProvider); !ok {
		t.Errorf("expected *OpenRouterProvider, got %T", pOR)
	}
	if pOR.Platform() != contract.PlatformOpenRouter {
		t.Errorf("expected PlatformOpenRouter, got %s", pOR.Platform())
	}
	if plat, found := reg.Platform("prov_openrouter"); !found || plat != contract.PlatformOpenRouter {
		t.Errorf("reg.Platform(prov_openrouter) = (%s, %v), want (%s, true)", plat, found, contract.PlatformOpenRouter)
	}

	// Unknown provider ID
	if plat, found := reg.Platform("unknown"); found || plat != "" {
		t.Errorf("reg.Platform(unknown) = (%s, %v), want ('', false)", plat, found)
	}
}

func TestRegistry_NoInference(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			// openai_compatible with openrouter.ai URL -> MUST remain OpenAICompatibleProvider / PlatformOpenAICompatible
			"gateway": {
				Type:    TypeOpenAICompatible,
				BaseURL: "https://openrouter.ai/api/v1",
			},
			// provider ID openrouter with type: openai_compatible -> MUST remain OpenAICompatibleProvider
			"openrouter": {
				Type:    TypeOpenAICompatible,
				BaseURL: "https://custom-proxy.internal/v1",
			},
		},
	}

	reg := NewRegistryFromConfig(cfg)

	pGateway, ok := reg.Get("gateway")
	if !ok {
		t.Fatalf("expected gateway registered")
	}
	if _, ok := pGateway.(*OpenAICompatibleProvider); !ok {
		t.Errorf("expected *OpenAICompatibleProvider for gateway, got %T", pGateway)
	}
	if pGateway.Platform() != contract.PlatformOpenAICompatible {
		t.Errorf("expected PlatformOpenAICompatible, got %s (did URL sniff trigger?)", pGateway.Platform())
	}

	pOR, ok := reg.Get("openrouter")
	if !ok {
		t.Fatalf("expected openrouter registered")
	}
	if _, ok := pOR.(*OpenAICompatibleProvider); !ok {
		t.Errorf("expected *OpenAICompatibleProvider for openrouter ID, got %T", pOR)
	}
	if pOR.Platform() != contract.PlatformOpenAICompatible {
		t.Errorf("expected PlatformOpenAICompatible, got %s (did ID sniff trigger?)", pOR.Platform())
	}
}

func TestProvider_DefaultBaseURLs(t *testing.T) {
	// Anthropic defaults vs override
	pAntDefault := NewAnthropicProvider("ant1", contract.ProviderConfig{
		Type:   TypeAnthropic,
		APIKey: "key",
	})
	if pAntDefault.BaseURL() != DefaultAnthropicBaseURL {
		t.Errorf("expected default %s, got %s", DefaultAnthropicBaseURL, pAntDefault.BaseURL())
	}

	pAntCustom := NewAnthropicProvider("ant2", contract.ProviderConfig{
		Type:    TypeAnthropic,
		BaseURL: "https://custom.anthropic.internal",
		APIKey:  "key",
	})
	if pAntCustom.BaseURL() != "https://custom.anthropic.internal" {
		t.Errorf("expected custom URL, got %s", pAntCustom.BaseURL())
	}

	// OpenRouter defaults vs override
	pORDefault := NewOpenRouterProvider("or1", contract.ProviderConfig{
		Type:   TypeOpenRouter,
		APIKey: "key",
	})
	if pORDefault.BaseURL() != DefaultOpenRouterBaseURL {
		t.Errorf("expected default %s, got %s", DefaultOpenRouterBaseURL, pORDefault.BaseURL())
	}

	pORCustom := NewOpenRouterProvider("or2", contract.ProviderConfig{
		Type:    TypeOpenRouter,
		BaseURL: "https://custom.openrouter.internal/v1",
		APIKey:  "key",
	})
	if pORCustom.BaseURL() != "https://custom.openrouter.internal/v1" {
		t.Errorf("expected custom URL, got %s", pORCustom.BaseURL())
	}
}
