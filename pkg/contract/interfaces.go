package contract

import (
	"net/http"
	"strings"
)

// RequestContext represents the parsed metrics and metadata of an incoming OpenAI API request turn.
type RequestContext struct {
	Tokens                    int      `json:"tokens"`
	HasImages                 bool     `json:"has_images"`
	HasTools                  bool     `json:"has_tools"`
	InteractiveTool           string   `json:"interactive_tool,omitempty"`
	Keywords                  []string `json:"keywords"`
	Prompt                    string   `json:"prompt"`
	CleanPrompt               string   `json:"clean_prompt,omitempty"`
	Retries                   int      `json:"retries,omitempty"`
	IsRetry                   bool     `json:"is_retry,omitempty"`
	HasToolProgress           bool     `json:"has_tool_progress,omitempty"`
	HasWriteProgress          bool     `json:"has_write_progress,omitempty"`
	HasTestProgress           bool     `json:"has_test_progress,omitempty"`
	HasWriteCapability        bool     `json:"has_write_capability,omitempty"`
	NoKickstart               bool     `json:"no_kickstart,omitempty"`
	NoCycleKiller             bool     `json:"no_cycle_killer,omitempty"`
	NoShield                  bool     `json:"no_shield,omitempty"`
	RawModeEnabled            bool     `json:"raw_mode_enabled,omitempty"`
	HistoryErrors             int      `json:"history_errors,omitempty"`
	CycleRetries              int      `json:"cycle_retries,omitempty"`
	CycleBreakerTriggered     bool     `json:"cycle_breaker_triggered,omitempty"`
	CycleBreakerReason        string   `json:"cycle_breaker_reason,omitempty"`
	CycleProseTokens          int      `json:"cycle_prose_tokens,omitempty"`
	CycleMaxNgramFreq         int      `json:"cycle_max_ngram_freq,omitempty"`
	CycleThinkingTokens       int      `json:"cycle_thinking_tokens,omitempty"`
	CycleMaxThinkingNgramFreq int      `json:"cycle_max_thinking_ngram_freq,omitempty"`
	SessionKickstarted        bool     `json:"session_kickstarted,omitempty"`
	SessionKickstartCount     int      `json:"session_kickstart_count,omitempty"`
	SessionKey                string   `json:"session_key,omitempty"`
	CoolingDownModels         []string `json:"cooling_down_models,omitempty"`
	ForcedTier                string   `json:"forced_tier,omitempty"`
	ForcedModel               string   `json:"forced_model,omitempty"`
	IsMetaDirective           bool     `json:"is_meta_directive,omitempty"`
	MetaDirective             string   `json:"meta_directive,omitempty"`
	MetaDirectiveRaw          string   `json:"meta_directive_raw,omitempty"`
	Features                  uint16   `json:"features,omitempty"`
	FairyDusted               bool     `json:"fairy_dusted,omitempty"`
	FairyDustEntry            string   `json:"fairy_dust_entry,omitempty"`
	FairyDustCount            int      `json:"fairy_dust_count,omitempty"`
}

// IsModelCoolingDown returns true if the specified model is currently cooling down on this session.
func (rc RequestContext) IsModelCoolingDown(model string) bool {
	if model == "" || len(rc.CoolingDownModels) == 0 {
		return false
	}
	for _, m := range rc.CoolingDownModels {
		if strings.EqualFold(m, model) {
			return true
		}
	}
	return false
}

// RouterConfig configures gateway routing behavior.
type RouterConfig struct {
	EnableInPromptDirectives *bool `yaml:"enable_in_prompt_directives,omitempty" json:"enable_in_prompt_directives,omitempty"`
}

// NormalizersConfig configures selective tool and stream normalizer sub-strategies.
type NormalizersConfig struct {
	Enabled  *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Markdown *bool `yaml:"markdown,omitempty" json:"markdown,omitempty"`
	BareJSON *bool `yaml:"bare_json,omitempty" json:"bare_json,omitempty"`
	ReAct    *bool `yaml:"react,omitempty" json:"react,omitempty"`
	Think    *bool `yaml:"think,omitempty" json:"think,omitempty"`
}

// CycleBreakerDefaultCorrectionPrompt is the default authoritative system override prompt injected on Stage 1 cycle breaking.
const CycleBreakerDefaultCorrectionPrompt = "[SYSTEM OVERRIDE] You produced excessive reasoning without calling any tools. Stop planning. Execute immediately. Call the appropriate tool NOW with the correct arguments. Do not explain your reasoning."

// DefaultKickstartPrompt is the default prompt injected when Kickstart
// detects consecutive turns without tool progress (file writes or terminal commands).
const DefaultKickstartPrompt = "[SYSTEM OVERRIDE] You have not produced any file writes or terminal commands in multiple consecutive turns. Stop meta-work (TODO updates, file reads, planning). Execute a concrete action: write a file or run a command NOW."

// DefaultFairyDustPrompt is the default prompt injected by a Fairy Dust entry
// when no custom prompt is configured.
const DefaultFairyDustPrompt = "[QUALITY CHECKPOINT] You are being consulted as a senior reviewer. Analyze the current context, recent changes, and progression. Identify: (1) logic bugs, (2) architectural drift from requirements, (3) compilation/type errors introduced by prior turns. Fix issues immediately with tool calls. If trajectory is correct, confirm and continue."

// CycleBreakerConfig configures active inference stream loop and monologue detection.
type CycleBreakerConfig struct {
	Enabled                     *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	MaxProseTokens              int      `yaml:"max_prose_tokens,omitempty" json:"max_prose_tokens,omitempty"`
	MaxThinkingTokens           int      `yaml:"max_thinking_tokens,omitempty" json:"max_thinking_tokens,omitempty"`
	RepetitionWindow            int      `yaml:"repetition_window,omitempty" json:"repetition_window,omitempty"`
	RepetitionThreshold         int      `yaml:"repetition_threshold,omitempty" json:"repetition_threshold,omitempty"`
	ThinkingRepetitionThreshold int      `yaml:"thinking_repetition_threshold,omitempty" json:"thinking_repetition_threshold,omitempty"`
	MaxRetries                  int      `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	CorrectionPrompt            string   `yaml:"correction_prompt,omitempty" json:"correction_prompt,omitempty"`
	KickstartThreshold          int      `yaml:"kickstart_threshold,omitempty" json:"kickstart_threshold,omitempty"`
	KickstartWriteOnly          bool     `yaml:"kickstart_write_only,omitempty" json:"kickstart_write_only,omitempty"`
	KickstartWriteTools         []string `yaml:"kickstart_write_tools,omitempty" json:"kickstart_write_tools,omitempty"`
	KickstartPrompt             string   `yaml:"kickstart_prompt,omitempty" json:"kickstart_prompt,omitempty"`
	KickstartMaxCount           int      `yaml:"kickstart_max_count,omitempty" json:"kickstart_max_count,omitempty"`
	KickstartMaxFailures        int      `yaml:"kickstart_max_failures,omitempty" json:"kickstart_max_failures,omitempty"`
	ModelCooldownSeconds        int      `yaml:"model_cooldown_seconds,omitempty" json:"model_cooldown_seconds,omitempty"`
	RetryFloor                  int      `yaml:"retry_floor,omitempty" json:"retry_floor,omitempty"`
}

// Tier defines a single model routing rule in the 1..N evaluation pipeline.
type Tier struct {
	Name            string              `yaml:"name" json:"name"`
	Model           string              `yaml:"model" json:"model"`
	Provider        string              `yaml:"provider" json:"provider"`
	When            string              `yaml:"when" json:"when"`
	StripImages     bool                `yaml:"strip_images" json:"strip_images"`
	ReasoningEffort string              `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
	MaxContext      int                 `yaml:"max_context,omitempty" json:"max_context,omitempty"`
	Raw             *bool               `yaml:"raw,omitempty" json:"raw,omitempty"`
	Shield          *bool               `yaml:"shield,omitempty" json:"shield,omitempty"`
	Normalizer      *bool               `yaml:"normalizer,omitempty" json:"normalizer,omitempty"`
	Normalizers     *NormalizersConfig  `yaml:"normalizers,omitempty" json:"normalizers,omitempty"`
	CycleKiller     *CycleBreakerConfig `yaml:"cycle_killer,omitempty" json:"cycle_killer,omitempty"`
	CycleBreaker    *CycleBreakerConfig `yaml:"cycle_breaker,omitempty" json:"cycle_breaker,omitempty"`
}

// ProviderType defines the execution classification of an LLM backend.
type ProviderType string

const (
	// ProviderTypeLocal designates free local/on-prem inference (e.g. Ollama, vLLM, LM Studio, llama.cpp).
	// Incurred cost is exactly $0.00, and full baseline token savings are credited.
	ProviderTypeLocal ProviderType = "local"

	// ProviderTypeCloud designates metered cloud APIs (e.g. OpenRouter, Anthropic, OpenAI, DeepSeek Cloud).
	// Incurred cost is calculated against live or cached token rate pricing tables.
	ProviderTypeCloud ProviderType = "cloud"
)

// ProviderConfig defines a first-class LLM provider configuration.
type ProviderConfig struct {
	BaseURL             string            `yaml:"base_url" json:"base_url"`
	APIKey              string            `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Type                ProviderType      `yaml:"type" json:"type"` // MANDATORY: "local" or "cloud"
	Headers             map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	PricingSyncInterval string            `yaml:"pricing_sync_interval,omitempty" json:"pricing_sync_interval,omitempty"` // e.g. "15m", "24h"
}

// IsLocal returns true if the provider is a free local/on-prem inference engine.
// O(1) evaluation, zero heap allocations.
func (p ProviderConfig) IsLocal() bool {
	return p.Type == ProviderTypeLocal
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

// AgentShieldConfig configures the Agentic Tool Fallback Shield.
type AgentShieldConfig struct {
	Enabled              *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	TailBufferBytes      int      `yaml:"tail_buffer_bytes,omitempty" json:"tail_buffer_bytes,omitempty"`
	QuestionHeuristics   []string `yaml:"question_heuristics,omitempty" json:"question_heuristics,omitempty"`
	ModeSwitchHeuristics []string `yaml:"mode_switch_heuristics,omitempty" json:"mode_switch_heuristics,omitempty"`
	ErrorSignatures      []string `yaml:"error_signatures,omitempty" json:"error_signatures,omitempty"`
}

// FairyDustEntry defines a single proactive quality checkpoint configuration.
type FairyDustEntry struct {
	Name          string `yaml:"name" json:"name"`
	Model         string `yaml:"model" json:"model"`
	Provider      string `yaml:"provider,omitempty" json:"provider,omitempty"`
	Frequency     int    `yaml:"frequency" json:"frequency"`
	MaxPerSession int    `yaml:"max_per_session" json:"max_per_session"`
	Priority      int    `yaml:"priority,omitempty" json:"priority,omitempty"`
	Prompt        string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
}

// FairyDustConfig configures the multi-entry fairy dust quality checkpoint system.
type FairyDustConfig struct {
	Enabled *bool            `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Entries []FairyDustEntry `yaml:"entries,omitempty" json:"entries,omitempty"`
}

// Config defines the top-level configuration loaded from config.yaml.
type Config struct {
	Port         int                       `yaml:"port" json:"port"`
	AuthToken    string                    `yaml:"auth_token,omitempty" json:"auth_token,omitempty"`
	Router       RouterConfig              `yaml:"router,omitempty" json:"router,omitempty"`
	Deals        DealsConfig               `yaml:"deals,omitempty" json:"deals,omitempty"`
	AgentShield  AgentShieldConfig         `yaml:"agent_shield,omitempty" json:"agent_shield,omitempty"`
	CycleKiller  CycleBreakerConfig        `yaml:"cycle_killer,omitempty" json:"cycle_killer,omitempty"`
	CycleBreaker CycleBreakerConfig        `yaml:"cycle_breaker,omitempty" json:"cycle_breaker,omitempty"`
	FairyDust    FairyDustConfig           `yaml:"fairy_dust,omitempty" json:"fairy_dust,omitempty"`
	Providers    map[string]ProviderConfig `yaml:"providers" json:"providers"`
	Tiers        []Tier                    `yaml:"tiers" json:"tiers"`
	DefaultTier  Tier                      `yaml:"default_tier" json:"default_tier"`
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
