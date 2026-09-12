package nts

import (
	"time"
)

// Pipeline coordinates the execution of the 5 in-place compaction passes.
type Pipeline struct {
	config Config
}

// NewPipeline creates a new Pipeline configured with the given options.
func NewPipeline(cfg Config) *Pipeline {
	if cfg.DedupThreshold <= 0 {
		cfg.DedupThreshold = 3
	}
	return &Pipeline{config: cfg}
}

// Config returns the pipeline configuration.
func (p *Pipeline) Config() Config {
	return p.config
}

// Process runs all enabled compaction passes in-place on buffer b, respecting category immunity.
// It returns a ReductionResult with reduction metrics and the length of the compacted slice b[:res.ReducedBytes].
func (p *Pipeline) Process(b []byte, category ToolCategory) ReductionResult {
	origLen := len(b)

	if !p.config.Enabled || origLen == 0 {
		return ReductionResult{
			OriginalBytes: origLen,
			ReducedBytes:  origLen,
			Bypassed:      true,
			BypassReason:  "disabled_or_empty",
		}
	}

	// Dual-lane immunity checks
	if category == CategoryFileRead && p.config.PreserveFileReads {
		return ReductionResult{
			OriginalBytes: origLen,
			ReducedBytes:  origLen,
			Bypassed:      true,
			BypassReason:  "category_file_read",
		}
	}

	if category == CategoryFileWrite && p.config.PreserveFileWrites {
		return ReductionResult{
			OriginalBytes: origLen,
			ReducedBytes:  origLen,
			Bypassed:      true,
			BypassReason:  "category_file_write",
		}
	}

	start := time.Now()

	// Pass 1: ANSI & terminal escape sequence stripping
	if p.config.StripANSI {
		b = StripANSIInPlace(b)
	}

	// Pass 2: Carriage return overwrite normalization
	if p.config.ResolveCR {
		b = ResolveCarriageReturnsInPlace(b)
	}

	// Pass 3: Tool boilerplate & notice stripping
	if p.config.StripBoilerplate {
		b = StripToolBoilerplateInPlace(b)
	}

	// Pass 4: Whitespace cascade & trailing empty line normalization
	if p.config.NormalizeWhitespace {
		b = NormalizeWhitespaceInPlace(b)
	}

	// Pass 5: Consecutive duplicate lines collapsing
	if p.config.DeduplicateLines {
		b = CollapseDuplicatesInPlace(b, p.config.DedupThreshold)
	}


	duration := time.Since(start)
	reducedLen := len(b)
	bytesSaved := origLen - reducedLen
	tokensSaved := 0
	if bytesSaved > 0 {
		tokensSaved = (bytesSaved + 3) / 4 // 1 token ~ 4 characters
	}

	return ReductionResult{
		OriginalBytes: origLen,
		ReducedBytes:  reducedLen,
		BytesSaved:    bytesSaved,
		TokensSaved:   tokensSaved,
		Duration:      duration,
		Bypassed:      false,
	}
}
