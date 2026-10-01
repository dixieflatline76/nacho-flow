package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// GenericLLMProvider adapts any contract.ProviderConfig into an LLMProvider.
type GenericLLMProvider struct {
	id             string
	name           string
	config         contract.ProviderConfig
	client         *http.Client
	circuitBreaker *CircuitBreaker
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
		id:             id,
		name:           name,
		config:         cfg,
		client:         &http.Client{Timeout: 3 * time.Second},
		circuitBreaker: NewCircuitBreaker(DefaultFailureThreshold, DefaultCooldownDuration),
	}
}

// CircuitBreaker returns the circuit breaker protecting this provider.
func (p *GenericLLMProvider) CircuitBreaker() *CircuitBreaker {
	if p.circuitBreaker == nil {
		p.circuitBreaker = NewCircuitBreaker(DefaultFailureThreshold, DefaultCooldownDuration)
	}
	return p.circuitBreaker
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
	return p.config.IsLocal()
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
	defer func() { _ = resp.Body.Close() }()
	return nil
}

// BuildUpstreamRequest constructs a standard OpenAI-compatible ChatCompletion HTTP request.
func (p *GenericLLMProvider) BuildUpstreamRequest(ctx context.Context, r *http.Request, model string, body []byte) (*http.Request, error) {
	targetURL, err := url.Parse(p.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL for provider '%s': %w", p.id, err)
	}

	reqPath := r.URL.Path
	targetBasePath := strings.TrimRight(targetURL.Path, "/")
	if strings.HasSuffix(targetBasePath, "/v1") && strings.HasPrefix(reqPath, "/v1/") {
		reqPath = strings.TrimPrefix(reqPath, "/v1")
	}
	fullTargetURL := singleJoiningSlash(targetURL.String(), reqPath)

	// #nosec G704 - outgoing proxy request to configured upstream provider endpoint
	outReq, err := http.NewRequestWithContext(ctx, r.Method, fullTargetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create upstream request for provider '%s': %w", p.id, err)
	}

	// Forward client headers
	for k, vv := range r.Header {
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Host = targetURL.Host

	// Strip client auth headers for local engines (Ollama, llama.cpp)
	if p.config.IsLocal() {
		outReq.Header.Del("Authorization")
	} else if p.config.APIKey != "" {
		outReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	// Apply custom provider headers
	for k, v := range p.config.Headers {
		outReq.Header.Set(k, v)
	}
	outReq.Header.Set("Content-Length", strconv.Itoa(len(body)))

	return outReq, nil
}

// WrapResponseStream passes through the raw response stream unmodified for standard OpenAI providers.
func (p *GenericLLMProvider) WrapResponseStream(resp *http.Response) io.ReadCloser {
	return resp.Body
}

// TranslateResponseBody passes through the response bytes unmodified for standard OpenAI providers.
func (p *GenericLLMProvider) TranslateResponseBody(statusCode int, body []byte) ([]byte, error) {
	return body, nil
}

func singleJoiningSlash(a, b string) string {
	aslashes := strings.HasSuffix(a, "/")
	bslashes := strings.HasPrefix(b, "/")
	switch {
	case aslashes && bslashes:
		return a + b[1:]
	case !aslashes && !bslashes:
		return a + "/" + b
	}
	return a + b
}
