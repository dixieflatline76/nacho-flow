package provider

import (
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
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
	BaseURL() string // effective URL, provider default already applied
	IsLocal() bool
	Platform() contract.Platform
}

// Descriptor defines metadata and factory for a provider wire-protocol implementation.
type Descriptor struct {
	Type           contract.ProviderType
	DefaultBaseURL string // "" = base_url is required in config
	AllowsLocal    bool   // true = local: true is permitted in config for this provider
	Platform       func(cfg contract.ProviderConfig) contract.Platform
	New            func(id string, cfg contract.ProviderConfig) LLMProvider
}

var (
	descriptorsMu sync.RWMutex
	descriptors   = make(map[contract.ProviderType]Descriptor)
)

// RegisterDescriptor registers a provider descriptor. Typically called in init().
func RegisterDescriptor(d Descriptor) {
	descriptorsMu.Lock()
	defer descriptorsMu.Unlock()
	descriptors[d.Type] = d
}

// LookupDescriptor finds a registered descriptor by provider type.
func LookupDescriptor(t contract.ProviderType) (Descriptor, bool) {
	descriptorsMu.RLock()
	defer descriptorsMu.RUnlock()
	d, ok := descriptors[t]
	return d, ok
}

// ResolvePlatform resolves the Platform for a ProviderConfig via its registered Descriptor.
func ResolvePlatform(cfg contract.ProviderConfig) (contract.Platform, bool) {
	d, ok := LookupDescriptor(cfg.Type)
	if !ok {
		return "", false
	}
	if d.Platform != nil {
		return d.Platform(cfg), true
	}
	return "", false
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
