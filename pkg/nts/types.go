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
	DeduplicateLines    bool `json:"deduplicate_lines" yaml:"deduplicate_lines"`
	DedupThreshold      int  `json:"dedup_threshold" yaml:"dedup_threshold"` // Minimum repeat count to collapse (default: 3)
	StripBoilerplate    bool `json:"strip_boilerplate" yaml:"strip_boilerplate"`
	NormalizeWhitespace bool `json:"normalize_whitespace" yaml:"normalize_whitespace"`

	// Dual-lane immunity options (always recommended true)
	PreserveFileReads    bool `json:"preserve_file_reads" yaml:"preserve_file_reads"`
	PreserveFileWrites   bool `json:"preserve_file_writes" yaml:"preserve_file_writes"`
	PreserveCacheControl bool `json:"preserve_cache_control" yaml:"preserve_cache_control"`
}

// DefaultConfig returns the production default configuration for NTS.
func DefaultConfig() Config {
	return Config{
		Enabled:              true,
		StripANSI:            true,
		ResolveCR:            true,
		DeduplicateLines:     true,
		DedupThreshold:       3,
		StripBoilerplate:     true,
		NormalizeWhitespace:  true,
		PreserveFileReads:    true,
		PreserveFileWrites:   true,
		PreserveCacheControl: true,
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
