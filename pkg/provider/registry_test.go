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

// Test 2.1: GenericLLMProvider capabilities
func TestGenericLLMProvider_Capabilities(t *testing.T) {
	pCfg := contract.ProviderConfig{
		BaseURL: "https://api.langdock.com/v1",
		APIKey:  "secret-langdock-key",
		Type:    "cloud",
		Headers: map[string]string{
			"X-Langdock-Org": "engineering",
		},
	}

	p := NewGenericLLMProvider("langdock", pCfg)

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
	pNilCB := &GenericLLMProvider{}
	if cb := pNilCB.CircuitBreaker(); cb == nil {
		t.Errorf("Expected lazy initialization of circuit breaker, got nil")
	}
}

// Test 2.2: Local provider characteristics
func TestGenericLLMProvider_LocalCharacteristics(t *testing.T) {
	pCfg := contract.ProviderConfig{
		BaseURL: "http://127.0.0.1:11434/v1",
		Type:    "local",
	}

	p := NewGenericLLMProvider("ollama", pCfg)

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
			p := NewGenericLLMProvider(providerID, contract.ProviderConfig{
				BaseURL: fmt.Sprintf("http://127.0.0.1:%d/v1", 8000+id),
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
				Type:    "local",
			},
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
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
func TestGenericLLMProvider_Names(t *testing.T) {
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
		p := NewGenericLLMProvider(c.id, contract.ProviderConfig{BaseURL: "http://localhost"})
		if p.Name() != c.expectedName {
			t.Errorf("For id '%s', expected Name '%s', got '%s'", c.id, c.expectedName, p.Name())
		}
	}
}

// Test 2.6: Ping health check
func TestGenericLLMProvider_Ping(t *testing.T) {
	// Success case
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("Expected Authorization 'Bearer test-key', got '%s'", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := NewGenericLLMProvider("test", contract.ProviderConfig{
		BaseURL: ts.URL,
		APIKey:  "test-key",
	})

	if err := p.Ping(context.Background()); err != nil {
		t.Errorf("Expected ping to succeed, got: %v", err)
	}

	// Failure case: unreachable endpoint
	pBroken := NewGenericLLMProvider("broken", contract.ProviderConfig{
		BaseURL: "http://127.0.0.1:54321/unreachable",
	})
	if err := pBroken.Ping(context.Background()); err == nil {
		t.Errorf("Expected ping to fail for unreachable URL, got nil")
	}

	// Failure case: invalid URL syntax
	pInvalidURL := NewGenericLLMProvider("invalid", contract.ProviderConfig{
		BaseURL: "http://\x7f",
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
	p1 := NewGenericLLMProvider("p1", contract.ProviderConfig{BaseURL: "http://localhost:8001"})
	p2 := NewGenericLLMProvider("p2", contract.ProviderConfig{BaseURL: "http://localhost:8002"})

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

func TestGenericLLMProvider_BuildUpstreamRequest_And_Stream(t *testing.T) {
	// 1. Cloud provider with Auth and Custom Headers
	pCloud := NewGenericLLMProvider("cloud", contract.ProviderConfig{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test-123",
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
	pLocal := NewGenericLLMProvider("ollama", contract.ProviderConfig{
		BaseURL: "http://127.0.0.1:11434",
		Type:    contract.ProviderTypeLocal,
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
	pBad := NewGenericLLMProvider("bad", contract.ProviderConfig{
		BaseURL: "::://bad url",
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
