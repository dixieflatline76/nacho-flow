package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
