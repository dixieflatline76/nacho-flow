package contract

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"
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
	RootPromptHash            uint64   `json:"root_prompt_hash,omitempty"`
	PromptHash                uint64   `json:"prompt_hash,omitempty"`
	MessageCount              int      `json:"message_count,omitempty"`
	Retries                   int      `json:"retries,omitempty"`
	IsRetry                   bool     `json:"is_retry,omitempty"`
	HasToolProgress           bool     `json:"has_tool_progress,omitempty"`
	HasWriteProgress          bool     `json:"has_write_progress,omitempty"`
	HasShellWrite             bool     `json:"has_shell_write,omitempty"`
	HasTestProgress           bool     `json:"has_test_progress,omitempty"`
	HasTestPass               bool     `json:"has_test_pass,omitempty"`
	HasTestFail               bool     `json:"has_test_fail,omitempty"`
	HasActiveRepair           bool     `json:"has_active_repair,omitempty"`
	HasWriteCapability        bool     `json:"has_write_capability,omitempty"`
	NoKickstart               bool     `json:"no_kickstart,omitempty"`
	NoCycleKiller             bool     `json:"no_cycle_killer,omitempty"`
	NoShield                  bool     `json:"no_shield,omitempty"`
	RawModeEnabled            bool     `json:"raw_mode_enabled,omitempty"`
	HistoryErrors             int      `json:"history_errors,omitempty"`
	CycleRetries              int      `json:"cycle_retries,omitempty"`
	CycleBreakerTriggered     bool     `json:"cycle_breaker_triggered,omitempty"`
	CycleBreakerReason        string   `json:"cycle_breaker_reason,omitempty"`
	CycleContentTokens        int      `json:"cycle_content_tokens,omitempty"`
	CycleMaxNgramFreq         int      `json:"cycle_max_ngram_freq,omitempty"`
	CycleThinkingTokens       int      `json:"cycle_thinking_tokens,omitempty"`
	CycleMaxThinkingNgramFreq int      `json:"cycle_max_thinking_ngram_freq,omitempty"`
	CycleToolTokens           int      `json:"cycle_tool_tokens,omitempty"`
	CycleMaxToolNgramFreq     int      `json:"cycle_max_tool_ngram_freq,omitempty"`
	SessionReadSlapped        bool     `json:"session_read_slapped,omitempty"`
	SessionReadSlapCount      int      `json:"session_read_slap_count,omitempty"`
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
	ClientID                  string   `json:"client_id,omitempty"`
	RawTokens                 int      `json:"raw_tokens,omitempty"`
	NTSTokensSaved            int      `json:"nts_tokens_saved,omitempty"`
	NTSBytesSaved             int      `json:"nts_bytes_saved,omitempty"`
	UpstreamFinishReason      string   `json:"upstream_finish_reason,omitempty"`
	EmptyTruncatedTurn        bool     `json:"empty_truncated_turn,omitempty"`
	ThoughtSlapAttempts       int      `json:"thought_slap_attempts,omitempty"`
	ThoughtSlapOutcome        string   `json:"thought_slap_outcome,omitempty"`
	ThoughtSlapTier           string   `json:"thought_slap_tier,omitempty"`
	SessionThoughtSlapCount   int      `json:"session_thought_slap_count,omitempty"`
	ThoughtSlapExtraTokens    int      `json:"thought_slap_extra_tokens,omitempty"`
	ThoughtSlapExtraCost      float64  `json:"thought_slap_extra_cost,omitempty"`
}

// ThoughtSlap outcomes for telemetry and recovery tracking.
const (
	ThoughtSlapOutcomeRecoveredToolCall = "recovered_tool_call"
	ThoughtSlapOutcomeRecoveredContent  = "recovered_content"
	ThoughtSlapOutcomeExhausted         = "exhausted"
	ThoughtSlapOutcomeUpstreamError     = "upstream_error"
	ThoughtSlapOutcomeClientCancelled   = "client_cancelled"
)

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

// DefaultThoughtSlapPrompt is the default prompt injected on Thought Slap recovery turns.
const DefaultThoughtSlapPrompt = "Your previous attempt reached the output token limit while reasoning and produced no response. Do not restart your analysis. Respond now using the tools available to you."

// ThoughtSlapNoticeText is the client-visible notice emitted before Thought Slap recovery calls.
const ThoughtSlapNoticeText = "Reasoning limit reached — asking the model to respond."

// DefaultReadSlapPrompt is the default prompt injected when Read Slap
// detects consecutive turns without tool progress (file writes or terminal commands).
const DefaultReadSlapPrompt = "[SYSTEM OVERRIDE] You have not produced any file writes or terminal commands in multiple consecutive turns. Stop meta-work (TODO updates, file reads, planning). Execute a concrete action: write a file or run a command NOW."

// DefaultKickstartPrompt is an alias for DefaultReadSlapPrompt.
const DefaultKickstartPrompt = DefaultReadSlapPrompt

// DefaultFairyDustPrompt is the default prompt injected by a Fairy Dust entry
const DefaultFairyDustPrompt = "[QUALITY CHECKPOINT] You are being consulted as a senior reviewer. Analyze the current context, recent changes, and progression. Identify: (1) logic bugs, (2) architectural drift from requirements, (3) compilation/type errors introduced by prior turns. Fix issues immediately with tool calls. If trajectory is correct, confirm and continue."

// LaneConfig configures a single stream inspection lane (thinking, prose, or tool).
type LaneConfig struct {
	MaxTokens                 int `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	PhraseLength              int `yaml:"phrase_length,omitempty" json:"phrase_length,omitempty"`
	MaxRepeats                int `yaml:"max_repeats,omitempty" json:"max_repeats,omitempty"`
	BudgetMaxRepeats          int `yaml:"budget_max_repeats,omitempty" json:"budget_max_repeats,omitempty"`
	RepetitionWindow          int `yaml:"repetition_window,omitempty" json:"repetition_window,omitempty"`
	RepetitionThreshold       int `yaml:"repetition_threshold,omitempty" json:"repetition_threshold,omitempty"`
	BudgetRepetitionThreshold int `yaml:"budget_repetition_threshold,omitempty" json:"budget_repetition_threshold,omitempty"`
}

// ToolLaneConfig configures the tool arguments streaming lane.
type ToolLaneConfig struct {
	LaneConfig     `yaml:",inline" json:",inline"`
	MaxWriteTokens int      `yaml:"max_write_tokens,omitempty" json:"max_write_tokens,omitempty"`
	WriteTools     []string `yaml:"write_tools,omitempty" json:"write_tools,omitempty"`
}

// KickstartConfig configures cross-turn idle session resuscitation (read slap) and reasoning limit recovery (thought slap).
type KickstartConfig struct {
	ReadSlap    ReadSlapConfig    `yaml:"read_slap,omitempty" json:"read_slap,omitempty"`
	ThoughtSlap ThoughtSlapConfig `yaml:"thought_slap,omitempty" json:"thought_slap,omitempty"`
}

// ReadSlapConfig configures cross-turn idle loop detection and prompt nudging.
type ReadSlapConfig struct {
	Enabled          *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Threshold        int      `yaml:"threshold,omitempty" json:"threshold,omitempty"`
	MaxSlaps         int      `yaml:"max_slaps,omitempty" json:"max_slaps,omitempty"`
	MaxFailures      int      `yaml:"max_failures,omitempty" json:"max_failures,omitempty"`
	WriteOnly        bool     `yaml:"write_only,omitempty" json:"write_only,omitempty"`
	CustomWriteTools []string `yaml:"custom_write_tools,omitempty" json:"custom_write_tools,omitempty"`
	PromptOverride   string   `yaml:"prompt_override,omitempty" json:"prompt_override,omitempty"`
}

// ThoughtSlapConfig configures thinking truncation recovery.
type ThoughtSlapConfig struct {
	MaxAttempts    int    `yaml:"max_attempts,omitempty" json:"max_attempts,omitempty"`
	RescueTier     string `yaml:"rescue_tier,omitempty" json:"rescue_tier,omitempty"`
	PromptOverride string `yaml:"prompt_override,omitempty" json:"prompt_override,omitempty"`
}

// ThoughtSlapOptIn is the per-tier field for opting into Thought Slap and overriding defaults.
type ThoughtSlapOptIn struct {
	Enabled   bool              `yaml:"-" json:"enabled"`
	Overrides ThoughtSlapConfig `yaml:"-" json:"overrides"`
}

// UnmarshalYAML handles: bool (true/false), mapping (implies enabled + overrides), and rejects null/empty.
func (o *ThoughtSlapOptIn) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || (value.Kind == yaml.ScalarNode && (value.Tag == "!!null" || value.Value == "" || value.Value == "null")) {
		return fmt.Errorf("thought_slap needs a value: use `thought_slap: true` or a block of overrides")
	}
	if value.Kind == yaml.ScalarNode && value.Tag == "!!bool" {
		var b bool
		if err := value.Decode(&b); err != nil {
			return err
		}
		o.Enabled = b
		o.Overrides = ThoughtSlapConfig{}
		return nil
	}
	if value.Kind == yaml.MappingNode {
		o.Enabled = true
		return value.Decode(&o.Overrides)
	}
	return fmt.Errorf("thought_slap must be a boolean or a mapping of overrides, got YAML kind %d", value.Kind)
}

// MarshalYAML emits true when no overrides are set, else the mapping.
func (o ThoughtSlapOptIn) MarshalYAML() (interface{}, error) {
	if !o.Enabled {
		return false, nil
	}
	if o.Overrides == (ThoughtSlapConfig{}) {
		return true, nil
	}
	return o.Overrides, nil
}

// MarshalJSON emits true when no overrides are set, else the mapping.
func (o ThoughtSlapOptIn) MarshalJSON() ([]byte, error) {
	if !o.Enabled {
		return []byte("false"), nil
	}
	if o.Overrides == (ThoughtSlapConfig{}) {
		return []byte("true"), nil
	}
	return json.Marshal(o.Overrides)
}

// ResolveThoughtSlap merges a tier's opt-in with global defaults and built-in fallbacks.
// Resolution order: tier override -> global -> built-in default.
func ResolveThoughtSlap(optIn *ThoughtSlapOptIn, global ThoughtSlapConfig) (cfg ThoughtSlapConfig, enabled bool) {
	if optIn == nil || !optIn.Enabled {
		return ThoughtSlapConfig{}, false
	}
	cfg = ThoughtSlapConfig{
		MaxAttempts:    optIn.Overrides.MaxAttempts,
		RescueTier:     optIn.Overrides.RescueTier,
		PromptOverride: optIn.Overrides.PromptOverride,
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = global.MaxAttempts
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.RescueTier == "" {
		cfg.RescueTier = global.RescueTier
	}
	if cfg.PromptOverride == "" {
		cfg.PromptOverride = global.PromptOverride
	}
	return cfg, true
}

// CycleBreakerConfig configures active inference stream loop and monologue detection.
type CycleBreakerConfig struct {
	Enabled              *bool          `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	PhraseLength         int            `yaml:"phrase_length,omitempty" json:"phrase_length,omitempty"`
	BudgetMaxRepeats     int            `yaml:"budget_max_repeats,omitempty" json:"budget_max_repeats,omitempty"`
	ThinkingLane         LaneConfig     `yaml:"thinking_lane,omitempty" json:"thinking_lane,omitempty"`
	ReasoningLane        LaneConfig     `yaml:"reasoning_lane,omitempty" json:"reasoning_lane,omitempty"`
	ContentLane          LaneConfig     `yaml:"content_lane,omitempty" json:"content_lane,omitempty"`
	ToolLane             ToolLaneConfig `yaml:"tool_lane,omitempty" json:"tool_lane,omitempty"`
	MaxRetries           int            `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	CorrectionPrompt     string         `yaml:"correction_prompt,omitempty" json:"correction_prompt,omitempty"`
	ModelCooldownSeconds int            `yaml:"model_cooldown_seconds,omitempty" json:"model_cooldown_seconds,omitempty"`
	RetryFloor           int            `yaml:"retry_floor,omitempty" json:"retry_floor,omitempty"`

	// Flat fields for direct overrides and tests
	MaxContentTokens                  int `yaml:"max_content_tokens,omitempty" json:"max_content_tokens,omitempty"`
	MaxThinkingTokens                 int `yaml:"max_thinking_tokens,omitempty" json:"max_thinking_tokens,omitempty"`
	MaxToolTokens                     int `yaml:"max_tool_tokens,omitempty" json:"max_tool_tokens,omitempty"`
	MaxWriteTokens                    int `yaml:"max_write_tokens,omitempty" json:"max_write_tokens,omitempty"`
	RepetitionWindow                  int `yaml:"repetition_window,omitempty" json:"repetition_window,omitempty"`
	RepetitionThreshold               int `yaml:"repetition_threshold,omitempty" json:"repetition_threshold,omitempty"`
	ThinkingRepetitionThreshold       int `yaml:"thinking_repetition_threshold,omitempty" json:"thinking_repetition_threshold,omitempty"`
	BudgetRepetitionThreshold         int `yaml:"budget_repetition_threshold,omitempty" json:"budget_repetition_threshold,omitempty"`
	ThinkingBudgetRepetitionThreshold int `yaml:"thinking_budget_repetition_threshold,omitempty" json:"thinking_budget_repetition_threshold,omitempty"`
}

// ResolvePhraseLength returns the resolved phrase length (n-gram size) for a lane, falling back to global defaults.
func (c *CycleBreakerConfig) ResolvePhraseLength(lane ...*LaneConfig) int {
	if len(lane) > 0 && lane[0] != nil {
		if lane[0].PhraseLength > 0 {
			return lane[0].PhraseLength
		}
		if lane[0].RepetitionWindow > 0 {
			return lane[0].RepetitionWindow
		}
	}
	if c.PhraseLength > 0 {
		return c.PhraseLength
	}
	if c.RepetitionWindow > 0 {
		return c.RepetitionWindow
	}
	return 6
}

// ResolveBudgetMaxRepeats returns the resolved cooperative budget repetition threshold for a lane.
func (c *CycleBreakerConfig) ResolveBudgetMaxRepeats(lane ...*LaneConfig) int {
	if len(lane) > 0 && lane[0] != nil {
		if lane[0].BudgetMaxRepeats > 0 {
			return lane[0].BudgetMaxRepeats
		}
		if lane[0].BudgetRepetitionThreshold > 0 {
			return lane[0].BudgetRepetitionThreshold
		}
	}
	if c.BudgetMaxRepeats > 0 {
		return c.BudgetMaxRepeats
	}
	if c.BudgetRepetitionThreshold > 0 {
		return c.BudgetRepetitionThreshold
	}
	return 5
}

func (c *CycleBreakerConfig) ResolveThinkingMaxTokens() int {
	if c.ThinkingLane.MaxTokens > 0 {
		return c.ThinkingLane.MaxTokens
	}
	if c.ReasoningLane.MaxTokens > 0 {
		return c.ReasoningLane.MaxTokens
	}
	if c.MaxThinkingTokens > 0 {
		return c.MaxThinkingTokens
	}
	return 4096
}

func (c *CycleBreakerConfig) ResolveThinkingMaxRepeats() int {
	if c.ThinkingLane.MaxRepeats > 0 {
		return c.ThinkingLane.MaxRepeats
	}
	if c.ReasoningLane.MaxRepeats > 0 {
		return c.ReasoningLane.MaxRepeats
	}
	if c.ThinkingLane.RepetitionThreshold > 0 {
		return c.ThinkingLane.RepetitionThreshold
	}
	if c.ReasoningLane.RepetitionThreshold > 0 {
		return c.ReasoningLane.RepetitionThreshold
	}
	if c.ThinkingRepetitionThreshold > 0 {
		return c.ThinkingRepetitionThreshold
	}
	return 6
}

func (c *CycleBreakerConfig) ResolveThinkingPhraseLength() int {
	if c.ThinkingLane.PhraseLength > 0 || c.ThinkingLane.RepetitionWindow > 0 {
		return c.ResolvePhraseLength(&c.ThinkingLane)
	}
	if c.ReasoningLane.PhraseLength > 0 || c.ReasoningLane.RepetitionWindow > 0 {
		return c.ResolvePhraseLength(&c.ReasoningLane)
	}
	return c.ResolvePhraseLength(&c.ThinkingLane)
}

func (c *CycleBreakerConfig) ResolveThinkingBudgetMaxRepeats() int {
	if c.ThinkingBudgetRepetitionThreshold > 0 {
		return c.ThinkingBudgetRepetitionThreshold
	}
	if c.ThinkingLane.BudgetMaxRepeats > 0 || c.ThinkingLane.BudgetRepetitionThreshold > 0 {
		return c.ResolveBudgetMaxRepeats(&c.ThinkingLane)
	}
	if c.ReasoningLane.BudgetMaxRepeats > 0 || c.ReasoningLane.BudgetRepetitionThreshold > 0 {
		return c.ResolveBudgetMaxRepeats(&c.ReasoningLane)
	}
	return c.ResolveBudgetMaxRepeats(&c.ThinkingLane)
}

func (c *CycleBreakerConfig) ResolveContentMaxTokens() int {
	if c.ContentLane.MaxTokens > 0 {
		return c.ContentLane.MaxTokens
	}
	if c.MaxContentTokens > 0 {
		return c.MaxContentTokens
	}
	return 6144
}

func (c *CycleBreakerConfig) ResolveContentMaxRepeats() int {
	if c.ContentLane.MaxRepeats > 0 {
		return c.ContentLane.MaxRepeats
	}
	if c.ContentLane.RepetitionThreshold > 0 {
		return c.ContentLane.RepetitionThreshold
	}
	if c.RepetitionThreshold > 0 {
		return c.RepetitionThreshold
	}
	return 8
}

func (c *CycleBreakerConfig) ResolveContentPhraseLength() int {
	return c.ResolvePhraseLength(&c.ContentLane)
}

func (c *CycleBreakerConfig) ResolveContentBudgetMaxRepeats() int {
	return c.ResolveBudgetMaxRepeats(&c.ContentLane)
}

func (c *CycleBreakerConfig) ResolveToolMaxTokens() int {
	if c.ToolLane.MaxTokens > 0 {
		return c.ToolLane.MaxTokens
	}
	if c.MaxToolTokens > 0 {
		return c.MaxToolTokens
	}
	return 8192
}

func (c *CycleBreakerConfig) ResolveToolMaxWriteTokens() int {
	if c.ToolLane.MaxWriteTokens > 0 {
		return c.ToolLane.MaxWriteTokens
	}
	if c.MaxWriteTokens > 0 {
		return c.MaxWriteTokens
	}
	return 32768
}

func (c *CycleBreakerConfig) ResolveToolMaxRepeats() int {
	if c.ToolLane.MaxRepeats > 0 {
		return c.ToolLane.MaxRepeats
	}
	if c.ToolLane.RepetitionThreshold > 0 {
		return c.ToolLane.RepetitionThreshold
	}
	if c.RepetitionThreshold > 0 {
		return c.RepetitionThreshold
	}
	return 8
}

func (c *CycleBreakerConfig) ResolveToolPhraseLength() int {
	return c.ResolvePhraseLength(&c.ToolLane.LaneConfig)
}

func (c *CycleBreakerConfig) ResolveToolBudgetMaxRepeats() int {
	return c.ResolveBudgetMaxRepeats(&c.ToolLane.LaneConfig)
}

// Normalize synchronizes structured lane configs with flat legacy fields for bidirectional consistency.
func (c *CycleBreakerConfig) Normalize() {
	if c == nil {
		return
	}
	if c.ThinkingLane.MaxTokens == 0 && c.ReasoningLane.MaxTokens > 0 {
		c.ThinkingLane = c.ReasoningLane
	}
	if c.ReasoningLane.MaxTokens == 0 && c.ThinkingLane.MaxTokens > 0 {
		c.ReasoningLane = c.ThinkingLane
	}

	if c.MaxThinkingTokens == 0 && c.ThinkingLane.MaxTokens > 0 {
		c.MaxThinkingTokens = c.ThinkingLane.MaxTokens
	}
	if c.ThinkingLane.MaxTokens == 0 && c.MaxThinkingTokens > 0 {
		c.ThinkingLane.MaxTokens = c.MaxThinkingTokens
	}

	if c.MaxContentTokens == 0 && c.ContentLane.MaxTokens > 0 {
		c.MaxContentTokens = c.ContentLane.MaxTokens
	}
	if c.ContentLane.MaxTokens == 0 && c.MaxContentTokens > 0 {
		c.ContentLane.MaxTokens = c.MaxContentTokens
	}

	if c.MaxToolTokens == 0 && c.ToolLane.MaxTokens > 0 {
		c.MaxToolTokens = c.ToolLane.MaxTokens
	}
	if c.ToolLane.MaxTokens == 0 && c.MaxToolTokens > 0 {
		c.ToolLane.MaxTokens = c.MaxToolTokens
	}

	if c.MaxWriteTokens == 0 && c.ToolLane.MaxWriteTokens > 0 {
		c.MaxWriteTokens = c.ToolLane.MaxWriteTokens
	}
	if c.ToolLane.MaxWriteTokens == 0 && c.MaxWriteTokens > 0 {
		c.ToolLane.MaxWriteTokens = c.MaxWriteTokens
	}

	if c.RepetitionWindow == 0 && c.PhraseLength > 0 {
		c.RepetitionWindow = c.PhraseLength
	}
	if c.PhraseLength == 0 && c.RepetitionWindow > 0 {
		c.PhraseLength = c.RepetitionWindow
	}

	if c.ThinkingRepetitionThreshold == 0 && c.ThinkingLane.MaxRepeats > 0 {
		c.ThinkingRepetitionThreshold = c.ThinkingLane.MaxRepeats
	}
	if c.ThinkingLane.MaxRepeats == 0 && c.ThinkingRepetitionThreshold > 0 {
		c.ThinkingLane.MaxRepeats = c.ThinkingRepetitionThreshold
	}

	if c.RepetitionThreshold == 0 && c.ContentLane.MaxRepeats > 0 {
		c.RepetitionThreshold = c.ContentLane.MaxRepeats
	}
	if c.ContentLane.MaxRepeats == 0 && c.RepetitionThreshold > 0 {
		c.ContentLane.MaxRepeats = c.RepetitionThreshold
	}

	if c.BudgetRepetitionThreshold == 0 && c.BudgetMaxRepeats > 0 {
		c.BudgetRepetitionThreshold = c.BudgetMaxRepeats
	}
	if c.BudgetMaxRepeats == 0 && c.BudgetRepetitionThreshold > 0 {
		c.BudgetMaxRepeats = c.BudgetRepetitionThreshold
	}
}

// Tier defines a single model routing rule in the 1..N evaluation pipeline.
type Tier struct {
	Name              string              `yaml:"name" json:"name"`
	Model             string              `yaml:"model" json:"model"`
	Provider          string              `yaml:"provider" json:"provider"`
	When              string              `yaml:"when" json:"when"`
	StripImages       bool                `yaml:"strip_images" json:"strip_images"`
	HasVision         *bool               `yaml:"has_vision,omitempty" json:"has_vision,omitempty"`
	ResolvedHasVision bool                `yaml:"-" json:"resolved_has_vision"`
	StripTools        bool                `yaml:"strip_tools,omitempty" json:"strip_tools,omitempty"`
	HasTools          *bool               `yaml:"has_tools,omitempty" json:"has_tools,omitempty"`
	ReasoningEffort   string              `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
	MaxContext        int                 `yaml:"max_context,omitempty" json:"max_context,omitempty"`
	Raw               *bool               `yaml:"raw,omitempty" json:"raw,omitempty"`
	Shield            *bool               `yaml:"shield,omitempty" json:"shield,omitempty"`
	Normalizer        *bool               `yaml:"normalizer,omitempty" json:"normalizer,omitempty"`
	Normalizers       *NormalizersConfig  `yaml:"normalizers,omitempty" json:"normalizers,omitempty"`
	CycleKiller       *CycleBreakerConfig `yaml:"cycle_killer,omitempty" json:"cycle_killer,omitempty"`
	CycleBreaker      *CycleBreakerConfig `yaml:"cycle_breaker,omitempty" json:"cycle_breaker,omitempty"`
	ThoughtSlap       *ThoughtSlapOptIn   `yaml:"thought_slap,omitempty" json:"thought_slap,omitempty"`
}

// UnmarshalYAML decodes Tier and enforces that thought_slap cannot be a bare null key.
func (t *Tier) UnmarshalYAML(value *yaml.Node) error {
	type rawTier Tier
	var raw rawTier
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*t = Tier(raw)

	if value.Kind == yaml.MappingNode {
		for i := 0; i < len(value.Content); i += 2 {
			if value.Content[i].Value == "thought_slap" {
				valNode := value.Content[i+1]
				if valNode.Kind == yaml.ScalarNode && (valNode.Tag == "!!null" || valNode.Value == "" || valNode.Value == "null") {
					return fmt.Errorf("thought_slap needs a value: use `thought_slap: true` or a block of overrides")
				}
			}
		}
	}
	return nil
}

// ProviderType selects the wire-protocol implementation for a provider.
// Concrete ProviderType values are defined and registered by provider implementations in pkg/provider.
type ProviderType string

// ProviderConfig defines a first-class LLM provider configuration.
type ProviderConfig struct {
	Type                ProviderType      `yaml:"type" json:"type"`                             // MANDATORY
	BaseURL             string            `yaml:"base_url,omitempty" json:"base_url,omitempty"` // required for openai_compatible; optional override otherwise
	Local               bool              `yaml:"local,omitempty" json:"local,omitempty"`       // openai_compatible only
	APIKey              string            `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Headers             map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	PricingSyncInterval string            `yaml:"pricing_sync_interval,omitempty" json:"pricing_sync_interval,omitempty"`
}

// IsLocal returns true if the provider is a free local/on-prem inference engine.
// O(1) evaluation, zero heap allocations.
func (p ProviderConfig) IsLocal() bool {
	return p.Local
}

// Platform identifies the upstream platform family a provider implementation speaks to.
// Declared by provider implementations; never inferred outside pkg/provider.
type Platform string

const (
	PlatformLocal            Platform = "local"
	PlatformOpenAICompatible Platform = "openai_compatible"
	PlatformAnthropic        Platform = "anthropic"
	PlatformOpenRouter       Platform = "openrouter"
)

// PlatformCatalogPolicy declares how the curated catalog, pricing oracle, and tuner treat a Platform.
type PlatformCatalogPolicy struct {
	HasCuratedCatalog       bool // true = platform resolves models from the curated catalog
	AllowRecommendations    bool // true = tuner may recommend candidate models for this platform
	RequiresProviderBinding bool // true = only models with profile.Providers[platform] are valid
	RequiresOpenWeights     bool // true = only open-weights models are valid ($0 local inference)
	AllowUncatalogedModels  bool // true = uncataloged model IDs pass through FormatModelForPlatform
}

var platformCatalogPolicies = map[Platform]PlatformCatalogPolicy{
	"": {
		HasCuratedCatalog:      true,
		AllowRecommendations:   true,
		AllowUncatalogedModels: true,
	},
	PlatformOpenRouter: {
		HasCuratedCatalog:      true,
		AllowRecommendations:   true,
		AllowUncatalogedModels: true,
	},
	PlatformLocal: {
		HasCuratedCatalog:      true,
		AllowRecommendations:   true,
		RequiresOpenWeights:    true,
		AllowUncatalogedModels: true,
	},
	PlatformAnthropic: {
		HasCuratedCatalog:       true,
		AllowRecommendations:    true,
		RequiresProviderBinding: true,
	},
	PlatformOpenAICompatible: {
		HasCuratedCatalog:    false,
		AllowRecommendations: false,
	},
}

// RegisterPlatformCatalogPolicy registers or overrides the catalog policy for a platform.
func RegisterPlatformCatalogPolicy(p Platform, policy PlatformCatalogPolicy) {
	platformCatalogPolicies[p] = policy
}

// LookupPlatformCatalogPolicy returns the declarative catalog policy for a platform (O(1), 0 allocs).
func LookupPlatformCatalogPolicy(p Platform) (PlatformCatalogPolicy, bool) {
	policy, ok := platformCatalogPolicies[p]
	return policy, ok
}

// PlatformResolver maps a configured provider ID to the Platform declared by its implementation.
type PlatformResolver interface {
	Platform(providerID string) (Platform, bool)
}

// DealsConfig configures the spot market and discount detection engine.
type DealsConfig struct {
	Enabled           bool    `yaml:"enabled" json:"enabled"`
	AlertThresholdPct float64 `yaml:"alert_threshold_pct" json:"alert_threshold_pct"`         // default: 30.0 (%)
	MinCodingIndex    float64 `yaml:"min_coding_index" json:"min_coding_index"`               // default: 40.0
	RequireTools      bool    `yaml:"require_tools" json:"require_tools"`                     // default: true
	ExcludeBatch      *bool   `yaml:"exclude_batch,omitempty" json:"exclude_batch,omitempty"` // default: true
	ExcludeFree       *bool   `yaml:"exclude_free,omitempty" json:"exclude_free,omitempty"`   // default: true
}

// ShouldExcludeBatch returns true if non-interactive batch endpoints should be filtered out (default: true).
func (c DealsConfig) ShouldExcludeBatch() bool {
	if c.ExcludeBatch != nil {
		return *c.ExcludeBatch
	}
	return true
}

// ShouldExcludeFree returns true if heavily rate-limited free-tier endpoints should be filtered out (default: true).
func (c DealsConfig) ShouldExcludeFree() bool {
	if c.ExcludeFree != nil {
		return *c.ExcludeFree
	}
	return true
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
	ValueScore         float64  `json:"value_score,omitempty"`
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
	Name            string `yaml:"name" json:"name"`
	Model           string `yaml:"model" json:"model"`
	Provider        string `yaml:"provider,omitempty" json:"provider,omitempty"`
	Frequency       int    `yaml:"frequency" json:"frequency"`
	MaxPerSession   int    `yaml:"max_per_session" json:"max_per_session"`
	Priority        int    `yaml:"priority,omitempty" json:"priority,omitempty"`
	Prompt          string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	ReasoningEffort string `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
}

// FairyDustConfig configures the multi-entry fairy dust quality checkpoint system.
type FairyDustConfig struct {
	Enabled *bool            `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Entries []FairyDustEntry `yaml:"entries,omitempty" json:"entries,omitempty"`
}

// NTSConfig configures the Nacho Token Saver (NTS) in-place compaction engine.
type NTSConfig struct {
	Enabled                  *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	StripANSI                *bool `yaml:"strip_ansi,omitempty" json:"strip_ansi,omitempty"`
	ResolveCR                *bool `yaml:"resolve_cr,omitempty" json:"resolve_cr,omitempty"`
	DeduplicateLines         *bool `yaml:"deduplicate_lines,omitempty" json:"deduplicate_lines,omitempty"`
	DedupThreshold           int   `yaml:"dedup_threshold,omitempty" json:"dedup_threshold,omitempty"`
	StripBoilerplate         *bool `yaml:"strip_boilerplate,omitempty" json:"strip_boilerplate,omitempty"`
	NormalizeWhitespace      *bool `yaml:"normalize_whitespace,omitempty" json:"normalize_whitespace,omitempty"`
	PreserveFileReads        *bool `yaml:"preserve_file_reads,omitempty" json:"preserve_file_reads,omitempty"`
	PreserveFileWrites       *bool `yaml:"preserve_file_writes,omitempty" json:"preserve_file_writes,omitempty"`
	PreserveCacheControl     *bool `yaml:"preserve_cache_control,omitempty" json:"preserve_cache_control,omitempty"`
	CompactStaleFileReads    *bool `yaml:"compact_stale_file_reads,omitempty" json:"compact_stale_file_reads,omitempty"`
	StaleReadDepth           int   `yaml:"stale_read_depth,omitempty" json:"stale_read_depth,omitempty"`
	StripHistoricalReasoning *bool `yaml:"strip_historical_reasoning,omitempty" json:"strip_historical_reasoning,omitempty"`
	KeepRecentReasoningTurns *int  `yaml:"keep_recent_reasoning_turns,omitempty" json:"keep_recent_reasoning_turns,omitempty"`
	DeduplicateReminders     *bool `yaml:"deduplicate_reminders,omitempty" json:"deduplicate_reminders,omitempty"`
	RemindersRefreshInterval int   `yaml:"reminders_refresh_interval,omitempty" json:"reminders_refresh_interval,omitempty"`
	CollapseVerboseTests     *bool `yaml:"collapse_verbose_tests,omitempty" json:"collapse_verbose_tests,omitempty"`
	DeduplicateToolOutputs   *bool `yaml:"deduplicate_tool_outputs,omitempty" json:"deduplicate_tool_outputs,omitempty"`
	MinDuplicateOutputBytes  int   `yaml:"min_duplicate_output_bytes,omitempty" json:"min_duplicate_output_bytes,omitempty"`
}

// Config defines the top-level configuration loaded from config.yaml.
type Config struct {
	Version      string                    `yaml:"version,omitempty" json:"version,omitempty"`
	Port         int                       `yaml:"port" json:"port"`
	Host         string                    `yaml:"host,omitempty" json:"host,omitempty"`
	AuthToken    string                    `yaml:"auth_token,omitempty" json:"auth_token,omitempty"`
	Router       RouterConfig              `yaml:"router,omitempty" json:"router,omitempty"`
	Deals        DealsConfig               `yaml:"deals,omitempty" json:"deals,omitempty"`
	AgentShield  AgentShieldConfig         `yaml:"agent_shield,omitempty" json:"agent_shield,omitempty"`
	CycleKiller  CycleBreakerConfig        `yaml:"cycle_killer,omitempty" json:"cycle_killer,omitempty"`
	CycleBreaker CycleBreakerConfig        `yaml:"cycle_breaker,omitempty" json:"cycle_breaker,omitempty"`
	Kickstart    KickstartConfig           `yaml:"kickstart,omitempty" json:"kickstart,omitempty"`
	FairyDust    FairyDustConfig           `yaml:"fairy_dust,omitempty" json:"fairy_dust,omitempty"`
	NTS          NTSConfig                 `yaml:"nts,omitempty" json:"nts,omitempty"`
	Providers    map[string]ProviderConfig `yaml:"providers" json:"providers"`
	Tiers        []Tier                    `yaml:"tiers" json:"tiers"`
	DefaultTier  Tier                      `yaml:"default_tier" json:"default_tier"`
}

// Normalize ensures all nested structures (CycleKiller, CycleBreaker, Tiers) are normalized.
func (c *Config) Normalize() {
	if c == nil {
		return
	}
	c.CycleKiller.Normalize()
	c.CycleBreaker.Normalize()
	for i := range c.Tiers {
		if c.Tiers[i].CycleKiller != nil {
			c.Tiers[i].CycleKiller.Normalize()
		}
		if c.Tiers[i].CycleBreaker != nil {
			c.Tiers[i].CycleBreaker.Normalize()
		}
	}
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
