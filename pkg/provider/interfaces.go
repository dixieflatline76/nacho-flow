package provider

import (
	"context"
	"io"
	"net/http"
)

// ModelPricing represents prompt and completion rates per million tokens.
type ModelPricing struct {
	PromptCostPerMillion     float64 `json:"prompt_cost_per_million"`
	CompletionCostPerMillion float64 `json:"completion_cost_per_million"`
}

// LLMProvider defines the fundamental identity and target endpoint of an LLM service.
type LLMProvider interface {
	ID() string
	Name() string
	BaseURL() string
	IsLocal() bool
}

// AuthProvider is an optional capability interface for providers requiring API Keys / Bearer tokens.
type AuthProvider interface {
	GetAPIKey() string
}

// HeaderProvider is an optional capability interface for providers requiring custom request headers.
type HeaderProvider interface {
	GetHeaders() map[string]string
}

// HealthCheckProvider is an optional capability interface for checking provider availability.
type HealthCheckProvider interface {
	Ping(ctx context.Context) error
}

// PricingProvider is an optional capability interface for dynamic token pricing plugins.
type PricingProvider interface {
	Name() string
	FetchPricing(ctx context.Context) (map[string]ModelPricing, error)
}

// CircuitBreakerProvider is an optional capability interface for providers with circuit breaking protection.
type CircuitBreakerProvider interface {
	CircuitBreaker() *CircuitBreaker
}

// ProtocolProvider is an optional capability interface for providers that communicate
// using non-OpenAI wire protocols (e.g. Anthropic Messages API, Gemini Direct) and require
// wire request construction, stream adaptation, and response translation.
type ProtocolProvider interface {
	BuildUpstreamRequest(ctx context.Context, r *http.Request, model string, body []byte) (*http.Request, error)
	WrapResponseStream(resp *http.Response) io.ReadCloser
	TranslateResponseBody(statusCode int, body []byte) ([]byte, error)
}
