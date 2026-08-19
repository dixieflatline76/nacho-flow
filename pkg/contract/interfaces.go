package contract

import (
	"net/http"
)

// RequestContext represents the parsed metrics and metadata of an incoming OpenAI API request turn.
type RequestContext struct {
	Tokens    int      `json:"tokens"`
	HasImages bool     `json:"has_images"`
	HasTools  bool     `json:"has_tools"`
	Keywords  []string `json:"keywords"`
	Prompt    string   `json:"prompt"`
}

// Tier defines a single model routing rule in the 1..N evaluation pipeline.
type Tier struct {
	Name            string `yaml:"name" json:"name"`
	Model           string `yaml:"model" json:"model"`
	Provider        string `yaml:"provider" json:"provider"`
	When            string `yaml:"when" json:"when"`
	StripImages     bool   `yaml:"strip_images" json:"strip_images"`
	ReasoningEffort string `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
}

// ProviderConfig defines a first-class LLM provider configuration.
type ProviderConfig struct {
	BaseURL string            `yaml:"base_url" json:"base_url"`
	APIKey  string            `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Type    string            `yaml:"type,omitempty" json:"type,omitempty"` // "local" or "cloud"
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
}

// Config defines the top-level configuration loaded from config.yaml.
type Config struct {
	Port        int                       `yaml:"port" json:"port"`
	Providers   map[string]ProviderConfig `yaml:"providers" json:"providers"`
	Tiers       []Tier                    `yaml:"tiers" json:"tiers"`
	DefaultTier Tier                      `yaml:"default_tier" json:"default_tier"`
}

// Evaluator evaluates N tiers sequentially to find the matching tier for a request.
type Evaluator interface {
	SelectTier(reqCtx RequestContext) (Tier, error)
}

// Sanitizer cleans up payloads (e.g. stripping base64 images from historical turns).
type Sanitizer interface {
	SanitizePayload(body []byte, targetHasVision bool) ([]byte, bool)
}

// Classifier extracts RequestContext (tokens, tools, keywords, images) from raw OpenAI request JSON.
type Classifier interface {
	Classify(body []byte) (RequestContext, error)
}

// ProxyHandler is the HTTP handler interface for proxying requests.
type ProxyHandler interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}
