package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// GenericLLMProvider adapts any contract.ProviderConfig into an LLMProvider.
type GenericLLMProvider struct {
	id     string
	name   string
	config contract.ProviderConfig
	client *http.Client
}

// NewGenericLLMProvider creates a new generic provider instance.
func NewGenericLLMProvider(id string, cfg contract.ProviderConfig) *GenericLLMProvider {
	name := id
	if strings.Contains(strings.ToLower(id), "ollama") {
		name = "Ollama Local GPU"
	} else if strings.Contains(strings.ToLower(id), "openrouter") {
		name = "OpenRouter AI Gateway"
	} else if strings.Contains(strings.ToLower(id), "langdock") {
		name = "Langdock Enterprise"
	}

	return &GenericLLMProvider{
		id:     id,
		name:   name,
		config: cfg,
		client: &http.Client{Timeout: 3 * time.Second},
	}
}

func (p *GenericLLMProvider) ID() string {
	return p.id
}

func (p *GenericLLMProvider) Name() string {
	return p.name
}

func (p *GenericLLMProvider) BaseURL() string {
	return p.config.BaseURL
}

func (p *GenericLLMProvider) IsLocal() bool {
	return p.config.Type == "local" || strings.Contains(strings.ToLower(p.id), "local") || strings.Contains(strings.ToLower(p.id), "ollama")
}

// GetAPIKey returns the resolved API key.
func (p *GenericLLMProvider) GetAPIKey() string {
	return p.config.APIKey
}

// GetHeaders returns custom headers configured for this provider.
func (p *GenericLLMProvider) GetHeaders() map[string]string {
	if p.config.Headers == nil {
		return map[string]string{}
	}
	return p.config.Headers
}

// Ping checks if the provider endpoint is reachable.
func (p *GenericLLMProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.config.BaseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("provider '%s' ping failed: %w", p.id, err)
	}
	resp.Body.Close()
	return nil
}
