package contract

import (
	"net/http"
)

// RequestContext represents the parsed metrics and metadata of an incoming OpenAI API request turn.
type RequestContext struct {
	Tokens           int      `json:"tokens"`
	HasImages        bool     `json:"has_images"`
	HasTools         bool     `json:"has_tools"`
	Keywords         []string `json:"keywords"`
	Prompt           string   `json:"prompt"`
	CleanPrompt      string   `json:"clean_prompt,omitempty"`
	Retries          int      `json:"retries,omitempty"`
	IsRetry          bool     `json:"is_retry,omitempty"`
	ForcedTier       string   `json:"forced_tier,omitempty"`
	ForcedModel      string   `json:"forced_model,omitempty"`
	IsMetaDirective  bool     `json:"is_meta_directive,omitempty"`
	MetaDirective    string   `json:"meta_directive,omitempty"`
	MetaDirectiveRaw string   `json:"meta_directive_raw,omitempty"`
}

// RouterConfig configures gateway routing behavior.
type RouterConfig struct {
	EnableInPromptDirectives *bool `yaml:"enable_in_prompt_directives,omitempty" json:"enable_in_prompt_directives,omitempty"`
}

// Tier defines a single model routing rule in the 1..N evaluation pipeline.
type Tier struct {
	Name            string `yaml:"name" json:"name"`
	Model           string `yaml:"model" json:"model"`
	Provider        string `yaml:"provider" json:"provider"`
	When            string `yaml:"when" json:"when"`
	StripImages     bool   `yaml:"strip_images" json:"strip_images"`
	ReasoningEffort string `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
	MaxContext      int    `yaml:"max_context,omitempty" json:"max_context,omitempty"`
}

// ProviderConfig defines a first-class LLM provider configuration.
type ProviderConfig struct {
	BaseURL             string            `yaml:"base_url" json:"base_url"`
	APIKey              string            `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Type                string            `yaml:"type,omitempty" json:"type,omitempty"` // "local" or "cloud"
	Headers             map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	PricingSyncInterval string            `yaml:"pricing_sync_interval,omitempty" json:"pricing_sync_interval,omitempty"` // e.g. "15m", "24h"
}

// DealsConfig configures the spot market and discount detection engine.
type DealsConfig struct {
	Enabled           bool    `yaml:"enabled" json:"enabled"`
	AlertThresholdPct float64 `yaml:"alert_threshold_pct" json:"alert_threshold_pct"` // default: 50.0 (%)
	MinCodingIndex    float64 `yaml:"min_coding_index" json:"min_coding_index"`       // default: 40.0
	RequireTools      bool    `yaml:"require_tools" json:"require_tools"`             // default: true
}

// DealInfo represents an active promotional, subsidized, or high-value model deal.
type DealInfo struct {
	Provider           string   `json:"provider"`
	ModelID            string   `json:"model_id"`
	Name               string   `json:"name"`
	ContextLength      int      `json:"context_length"`
	PromptCostPerM     float64  `json:"prompt_cost_per_m"`
	CompletionCostPerM float64  `json:"completion_cost_per_m"`
	DiscountPct        float64  `json:"discount_pct"`
	IsFree             bool     `json:"is_free"`
	SupportsTools      bool     `json:"supports_tools"`
	SupportsVision     bool     `json:"supports_vision"`
	SupportsReasoning  bool     `json:"supports_reasoning"`
	TierRole           string   `json:"tier_role,omitempty"`
	CodingIndex        float64  `json:"coding_index,omitempty"`
	AgenticIndex       float64  `json:"agentic_index,omitempty"`
	RecommendedTiers   []string `json:"recommended_tiers,omitempty"`
	ExpiresAt          *string  `json:"expires_at,omitempty"`
}

// Config defines the top-level configuration loaded from config.yaml.
type Config struct {
	Port        int                       `yaml:"port" json:"port"`
	AuthToken   string                    `yaml:"auth_token,omitempty" json:"auth_token,omitempty"`
	Router      RouterConfig              `yaml:"router,omitempty" json:"router,omitempty"`
	Deals       DealsConfig               `yaml:"deals,omitempty" json:"deals,omitempty"`
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
