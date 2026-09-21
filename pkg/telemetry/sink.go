package telemetry

import "time"

// TurnRecord captures telemetry metadata for an individual prompt turn.
type TurnRecord struct {
	Timestamp                 time.Time `json:"timestamp"`
	RequestID                 string    `json:"request_id"`
	SessionID                 string    `json:"session_id,omitempty"`
	Tokens                    int       `json:"tokens"`
	HasImages                 bool      `json:"has_images"`
	HasTools                  bool      `json:"has_tools"`
	Keywords                  []string  `json:"keywords,omitempty"`
	SelectedTier              string    `json:"selected_tier"`
	TargetModel               string    `json:"target_model"`
	Provider                  string    `json:"provider"`
	IsLocal                   bool      `json:"is_local"`
	IsFallback                bool      `json:"is_fallback"`
	LatencyMs                 float64   `json:"latency_ms"`
	StatusCode                int       `json:"status_code"`
	IsRetry                   bool      `json:"is_retry"`
	CostSavedUSD              float64   `json:"cost_saved_usd"`
	CostSpentUSD              float64   `json:"cost_spent_usd"`
	ForcedTier                string    `json:"forced_tier,omitempty"`
	ForcedModel               string    `json:"forced_model,omitempty"`
	DirectiveUsed             string    `json:"directive_used,omitempty"`
	CycleBreakerTriggered     bool      `json:"cycle_breaker_triggered,omitempty"`
	CycleBreakerReason        string    `json:"cycle_breaker_reason,omitempty"`
	CycleContentTokens        int       `json:"cycle_content_tokens,omitempty"`
	CycleMaxNgramFreq         int       `json:"cycle_max_ngram_freq,omitempty"`
	CycleThinkingTokens       int       `json:"cycle_thinking_tokens,omitempty"`
	CycleMaxThinkingNgramFreq int       `json:"cycle_max_thinking_ngram_freq,omitempty"`
	CycleToolTokens           int       `json:"cycle_tool_tokens,omitempty"`
	CycleMaxToolNgramFreq     int       `json:"cycle_max_tool_ngram_freq,omitempty"`
	HasShellWrite             bool      `json:"has_shell_write,omitempty"`
	SessionKickstarted        bool      `json:"session_kickstarted,omitempty"`
	CachedTokens              int       `json:"cached_tokens,omitempty"`
	UpstreamCost              float64   `json:"upstream_cost,omitempty"`
	FairyDusted               bool      `json:"fairy_dusted,omitempty"`
	FairyDustEntry            string    `json:"fairy_dust_entry,omitempty"`
	NTSTokensSaved            int       `json:"nts_tokens_saved,omitempty"`
	NTSBytesSaved             int       `json:"nts_bytes_saved,omitempty"`

	// Session-aware fields for Auto-Tuner v2 session replay optimization
	RootPromptHash     uint64 `json:"root_prompt_hash,omitempty"`     // Identity of the root task
	Retries            int    `json:"retries,omitempty"`              // Session retry count at this turn
	HasWriteCapability bool   `json:"has_write_capability,omitempty"` // Plan Mode detection: tools present but no write tools
	HasWriteProgress   bool   `json:"has_write_progress,omitempty"`   // Did this turn produce file writes
	HasTestPass        bool   `json:"has_test_pass,omitempty"`        // Did tests pass this turn
	HasTestFail        bool   `json:"has_test_fail,omitempty"`        // Did tests fail this turn
}

// ObservationSink defines a decoupled consumer of observation events.
type ObservationSink interface {
	Emit(record TurnRecord)
	Close() error
}
