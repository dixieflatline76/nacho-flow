package telemetry

import "time"

// TurnRecord captures telemetry metadata for an individual prompt turn.
type TurnRecord struct {
	Timestamp     time.Time `json:"timestamp"`
	RequestID     string    `json:"request_id"`
	SessionID     string    `json:"session_id,omitempty"`
	Tokens        int       `json:"tokens"`
	HasImages     bool      `json:"has_images"`
	HasTools      bool      `json:"has_tools"`
	Keywords      []string  `json:"keywords,omitempty"`
	SelectedTier  string    `json:"selected_tier"`
	TargetModel   string    `json:"target_model"`
	Provider      string    `json:"provider"`
	IsLocal       bool      `json:"is_local"`
	IsFallback    bool      `json:"is_fallback"`
	LatencyMs     float64   `json:"latency_ms"`
	StatusCode    int       `json:"status_code"`
	IsRetry       bool      `json:"is_retry"`
	CostSavedUSD  float64   `json:"cost_saved_usd"`
	CostSpentUSD  float64   `json:"cost_spent_usd"`
	ForcedTier    string    `json:"forced_tier,omitempty"`
	ForcedModel   string    `json:"forced_model,omitempty"`
	DirectiveUsed string    `json:"directive_used,omitempty"`
}

// ObservationSink defines a decoupled consumer of observation events.
type ObservationSink interface {
	Emit(record TurnRecord)
	Close() error
}
