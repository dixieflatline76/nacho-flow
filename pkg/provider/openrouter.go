// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

const (
	// DefaultOpenRouterBaseURL is the standard API base URL for OpenRouter.
	DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

	// DefaultOpenRouterReferer is the attribution URL sent to OpenRouter.
	DefaultOpenRouterReferer = "https://github.com/dixieflatline76/nacho-flow"

	// DefaultOpenRouterTitle is the application title sent to OpenRouter.
	DefaultOpenRouterTitle = "nacho-flow"
)

// OpenRouterProvider represents a first-class OpenRouter aggregator endpoint.
// It implements LLMProvider, AuthProvider, HeaderProvider, CircuitBreakerProvider,
// and OpenRouterProviderMarker.
type OpenRouterProvider struct {
	id             string
	name           string
	config         contract.ProviderConfig
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

// NewOpenRouterProvider instantiates a new OpenRouterProvider with default headers and base URL.
func NewOpenRouterProvider(id string, cfg contract.ProviderConfig) *OpenRouterProvider {
	name := id
	if strings.Contains(strings.ToLower(id), "openrouter") {
		name = "OpenRouter"
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		cfg.BaseURL = DefaultOpenRouterBaseURL
	}
	return &OpenRouterProvider{
		id:             id,
		name:           name,
		config:         cfg,
		client:         &http.Client{Timeout: 10 * time.Second},
		circuitBreaker: NewCircuitBreaker(DefaultFailureThreshold, DefaultCooldownDuration),
	}
}

func (p *OpenRouterProvider) ID() string {
	return p.id
}

func (p *OpenRouterProvider) Name() string {
	return p.name
}

func (p *OpenRouterProvider) BaseURL() string {
	return p.config.BaseURL
}

func (p *OpenRouterProvider) IsLocal() bool {
	return false
}

func (p *OpenRouterProvider) GetAPIKey() string {
	return p.config.APIKey
}

func (p *OpenRouterProvider) IsOpenRouter() bool {
	return true
}

func (p *OpenRouterProvider) CircuitBreaker() *CircuitBreaker {
	if p.circuitBreaker == nil {
		p.circuitBreaker = NewCircuitBreaker(DefaultFailureThreshold, DefaultCooldownDuration)
	}
	return p.circuitBreaker
}

// GetHeaders returns the headers to be injected into requests to OpenRouter.
// Automatically supplies HTTP-Referer and X-Title defaults if not overridden in config.
func (p *OpenRouterProvider) GetHeaders() map[string]string {
	headers := make(map[string]string, len(p.config.Headers)+2)
	headers["HTTP-Referer"] = DefaultOpenRouterReferer
	headers["X-Title"] = DefaultOpenRouterTitle

	for k, v := range p.config.Headers {
		headers[k] = v
	}
	return headers
}

// Ping checks if the OpenRouter endpoint is reachable.
func (p *OpenRouterProvider) Ping(ctx context.Context) error {
	baseURL := strings.TrimRight(p.config.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}
	for k, v := range p.GetHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("provider '%s' ping failed: %w", p.id, err)
	}
	_ = resp.Body.Close()
	return nil
}
