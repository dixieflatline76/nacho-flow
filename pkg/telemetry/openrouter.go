package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// OpenRouterPricingProvider fetches real-time pricing and model capabilities from OpenRouter API.
type OpenRouterPricingProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewOpenRouterPricingProvider creates a new provider pointing to the production OpenRouter endpoint.
func NewOpenRouterPricingProvider(apiKey string) *OpenRouterPricingProvider {
	return NewOpenRouterPricingProviderWithURL(contract.OpenRouterProduction, apiKey)
}

// NewOpenRouterPricingProviderWithURL creates a provider with a custom base URL (useful for testing).
func NewOpenRouterPricingProviderWithURL(baseURL, apiKey string) *OpenRouterPricingProvider {
	return &OpenRouterPricingProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the provider name.
func (p *OpenRouterPricingProvider) Name() string {
	return contract.ProviderOpenRouter
}

type openRouterModelsResponse struct {
	Data []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
		Pricing       struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		} `json:"pricing"`
		Architecture struct {
			InputModalities []string `json:"input_modalities"`
		} `json:"architecture"`
		SupportedParameters []string `json:"supported_parameters"`
		ExpirationDate      *string  `json:"expiration_date"`
		Benchmarks          *struct {
			ArtificialAnalysis *struct {
				CodingIndex  float64 `json:"coding_index"`
				AgenticIndex float64 `json:"agentic_index"`
			} `json:"artificial_analysis"`
		} `json:"benchmarks"`
		Reasoning *struct {
			Mandatory bool `json:"reasoning"`
		} `json:"reasoning"`
	} `json:"data"`
}

// FetchPricing queries OpenRouter models endpoint and parses enriched model metadata.
func (p *OpenRouterPricingProvider) FetchPricing(ctx context.Context) (map[string]ModelMetadata, error) {
	baseURL := p.baseURL
	for {
		trimmed := strings.TrimRight(baseURL, "/")
		trimmed = strings.TrimSuffix(trimmed, "/v1")
		trimmed = strings.TrimSuffix(trimmed, "/api")
		if trimmed == baseURL {
			break
		}
		baseURL = trimmed
	}
	reqURL := fmt.Sprintf("%s%s", baseURL, contract.OpenRouterModelsPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", contract.ContentTypeJSON)
	if p.apiKey != "" {
		req.Header.Set(contract.HeaderAuthorization, "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter pricing api returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed openRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := make(map[string]ModelMetadata, len(parsed.Data))
	for _, m := range parsed.Data {
		promptPerToken, _ := strconv.ParseFloat(m.Pricing.Prompt, 64)
		compPerToken, _ := strconv.ParseFloat(m.Pricing.Completion, 64)

		hasVision := false
		for _, mod := range m.Architecture.InputModalities {
			if mod == contract.ModalityImage {
				hasVision = true
				break
			}
		}

		hasTools := false
		for _, param := range m.SupportedParameters {
			if param == contract.ParamTools {
				hasTools = true
				break
			}
		}

		hasReasoning := false
		if m.Reasoning != nil {
			hasReasoning = true
		}

		var codingIdx, agenticIdx float64
		if m.Benchmarks != nil && m.Benchmarks.ArtificialAnalysis != nil {
			codingIdx = m.Benchmarks.ArtificialAnalysis.CodingIndex
			agenticIdx = m.Benchmarks.ArtificialAnalysis.AgenticIndex
		}

		meta := ModelMetadata{
			ModelPricing: ModelPricing{
				PromptCostPerMillion:     promptPerToken * contract.TokensPerMillion,
				CompletionCostPerMillion: compPerToken * contract.TokensPerMillion,
			},
			ModelID:           m.ID,
			Provider:          contract.ProviderOpenRouter,
			Name:              m.Name,
			ContextLength:     m.ContextLength,
			SupportsTools:     hasTools,
			SupportsVision:    hasVision,
			SupportsReasoning: hasReasoning,
			CodingIndex:       codingIdx,
			AgenticIndex:      agenticIdx,
			ExpiresAt:         m.ExpirationDate,
		}

		result[m.ID] = meta
	}

	return result, nil
}

func init() {
	RegisterPricingFactory(contract.ProviderOpenRouter, func(id string, cfg contract.ProviderConfig, defaultInterval time.Duration) (PricingProvider, time.Duration) {
		interval := defaultInterval
		if cfg.PricingSyncInterval != "" {
			if parsed, err := time.ParseDuration(cfg.PricingSyncInterval); err == nil {
				interval = parsed
			}
		}
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = contract.OpenRouterProduction
		}
		return NewOpenRouterPricingProviderWithURL(baseURL, cfg.APIKey), interval
	})
}
