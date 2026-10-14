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

// ReasoningDialect specifies how an upstream LLM endpoint expects reasoning effort to be structured on the wire.
type ReasoningDialect uint8

const (
	// ReasoningDialectFlat formats reasoning effort as a top-level string: "reasoning_effort": "<val>".
	// Used by standard OpenAI, vLLM, DeepSeek, and Ollama.
	ReasoningDialectFlat ReasoningDialect = iota

	// ReasoningDialectNested formats reasoning effort as an object: "reasoning": {"effort": "<val>"}.
	// Used by OpenRouter.
	ReasoningDialectNested
)

// WireDeclarator specifies wire formatting and sanitization rules for an LLM provider.
type WireDeclarator struct {
	Dialect           ReasoningDialect
	ForbidsEffortNone bool
	StripFlatOnNested bool
}

// DefaultWireDeclarator is the safe fallback declarator for standard OpenAI-compatible endpoints.
var DefaultWireDeclarator = &WireDeclarator{
	Dialect:           ReasoningDialectFlat,
	ForbidsEffortNone: false,
	StripFlatOnNested: false,
}

// WireDeclaratorProvider is an optional capability interface allowing providers to declaratively
// specify their wire dialect and sanitization behavior on the zero-allocation hot path.
type WireDeclaratorProvider interface {
	WireDeclarator() *WireDeclarator
}
