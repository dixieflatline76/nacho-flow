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
)

// OpenRouterPricingProvider fetches real-time pricing from OpenRouter API.
type OpenRouterPricingProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewOpenRouterPricingProvider creates a new provider pointing to the production OpenRouter endpoint.
func NewOpenRouterPricingProvider(apiKey string) *OpenRouterPricingProvider {
	return NewOpenRouterPricingProviderWithURL("https://openrouter.ai", apiKey)
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
	return "openrouter"
}

type openRouterModelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Pricing struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		} `json:"pricing"`
	} `json:"data"`
}

// FetchPricing queries OpenRouter models endpoint and parses model token prices.
func (p *OpenRouterPricingProvider) FetchPricing(ctx context.Context) (map[string]ModelPricing, error) {
	reqURL := fmt.Sprintf("%s/api/v1/models", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
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

	result := make(map[string]ModelPricing, len(parsed.Data))
	for _, m := range parsed.Data {
		promptPerToken, _ := strconv.ParseFloat(m.Pricing.Prompt, 64)
		compPerToken, _ := strconv.ParseFloat(m.Pricing.Completion, 64)

		result[m.ID] = ModelPricing{
			PromptCostPerMillion:     promptPerToken * 1_000_000,
			CompletionCostPerMillion: compPerToken * 1_000_000,
		}
	}

	return result, nil
}
