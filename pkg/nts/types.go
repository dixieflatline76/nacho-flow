package nts

import "time"

// PassFunc defines a zero-allocation, in-place byte buffer transformer.
// It must obey the compaction invariant w <= r (write cursor <= read cursor)
// and return a slice of the input buffer b[:w].
type PassFunc func(b []byte) []byte

// Pass represents an individual compression strategy pass.
type Pass interface {
	Name() string
	ProcessInPlace(b []byte) []byte
}

// Config controls which compaction passes are enabled in Nacho Token Saver.
type Config struct {
	Enabled             bool `json:"enabled" yaml:"enabled"`
	StripANSI           bool `json:"strip_ansi" yaml:"strip_ansi"`
	ResolveCR           bool `json:"resolve_cr" yaml:"resolve_cr"`
	NormalizeWhitespace bool `json:"normalize_whitespace" yaml:"normalize_whitespace"`

	// Dual-lane immunity options (always recommended true)
	PreserveFileReads        bool `json:"preserve_file_reads" yaml:"preserve_file_reads"`
	PreserveFileWrites       bool `json:"preserve_file_writes" yaml:"preserve_file_writes"`
	PreserveCacheControl     bool `json:"preserve_cache_control" yaml:"preserve_cache_control"`
	StripHistoricalReasoning bool `json:"strip_historical_reasoning" yaml:"strip_historical_reasoning"`
	KeepRecentReasoningTurns int  `json:"keep_recent_reasoning_turns" yaml:"keep_recent_reasoning_turns"` // Number of recent assistant turns to preserve reasoning (default: 3)
	DeduplicateReminders     bool `json:"deduplicate_reminders" yaml:"deduplicate_reminders"`
	RemindersRefreshInterval int  `json:"reminders_refresh_interval" yaml:"reminders_refresh_interval"` // Re-emit full reminders table every K turns (default: 10)
	CollapseVerboseTests     bool `json:"collapse_verbose_tests" yaml:"collapse_verbose_tests"`
	DeduplicateToolOutputs   bool `json:"deduplicate_tool_outputs" yaml:"deduplicate_tool_outputs"`
	MinDuplicateOutputBytes  int  `json:"min_duplicate_output_bytes" yaml:"min_duplicate_output_bytes"` // Minimum bytes for duplicate tool output referencing (default: 200)
}

// DefaultConfig returns the production default configuration for NTS.
func DefaultConfig() Config {
	return Config{
		Enabled:                  true,
		StripANSI:                true,
		ResolveCR:                true,
		NormalizeWhitespace:      true,
		PreserveFileReads:        true,
		PreserveFileWrites:       true,
		PreserveCacheControl:     true,
		StripHistoricalReasoning: true,
		KeepRecentReasoningTurns: 3,
		DeduplicateReminders:     false,
		RemindersRefreshInterval: 10,
		CollapseVerboseTests:     false,
		DeduplicateToolOutputs:   false,
		MinDuplicateOutputBytes:  200,
	}
}

// ReductionResult records the telemetry of a single tool compaction operation.
type ReductionResult struct {
	OriginalBytes int           `json:"original_bytes"`
	ReducedBytes  int           `json:"reduced_bytes"`
	BytesSaved    int           `json:"bytes_saved"`
	TokensSaved   int           `json:"tokens_saved"`
	Duration      time.Duration `json:"duration_ns"`
	Bypassed      bool          `json:"bypassed"`
	BypassReason  string        `json:"bypass_reason,omitempty"`
}

// ToolCategory indicates the functional intent of a tool call for immunity checking.
type ToolCategory string

const (
	CategoryGeneric   ToolCategory = "generic"
	CategoryFileRead  ToolCategory = "file_read"
	CategoryFileWrite ToolCategory = "file_write"
)
