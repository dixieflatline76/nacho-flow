package shield

import (
	"hash/fnv"
	"strings"
	"sync"
	"unicode"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// ToolActivityCategory classifies tool actions to apply appropriate safety ceilings
// and repetition rules based on whether the action modifies files or executes commands.
type ToolActivityCategory int

const (
	// ToolCategoryCommand applies sliding N-gram loop detection and defaultMaxToolTokens (4096).
	ToolCategoryCommand ToolActivityCategory = iota
	// ToolCategoryFileWrite exempts content from N-gram loop detection and applies defaultMaxWriteTokens (32768).
	ToolCategoryFileWrite
)

const (
	defaultMaxProseTokens              = 4096
	defaultMaxThinkingTokens           = 1500
	defaultMaxToolTokens               = 4096
	defaultMaxWriteTokens              = 32768
	defaultRepetitionWindow            = 6
	defaultRepetitionThreshold         = 3
	defaultThinkingRepetitionThreshold = 5
	defaultMaxRetries                  = 1
	defaultMaxLoopDistance             = 48
)

type ngramOccurrence struct {
	consecutiveCount int
	lastWordIdx      int
}

// Config encapsulates the static, immutable settings of the cycle breaker.
// Completely decoupled from ephemeral runtime stream counter maps.
type Config struct {
	Enabled                     bool
	MaxProseTokens              int
	MaxThinkingTokens           int
	MaxToolTokens               int
	MaxWriteTokens              int
	RepetitionWindow            int
	RepetitionThreshold         int
	ThinkingRepetitionThreshold int
	MaxRetries                  int
	CorrectionPrompt            string
}

// streamLane encapsulates sliding N-gram frequency tracking and token accumulation for a single stream lane.
type streamLane struct {
	words        []string
	ngramCounts  map[uint64]int
	ngramHistory map[uint64]ngramOccurrence
	tokens       int
	maxNgramFreq int
	pendingWord  strings.Builder
}

func newAllocatedStreamLane() streamLane {
	return streamLane{
		words:        make([]string, 0, 128),
		ngramCounts:  make(map[uint64]int),
		ngramHistory: make(map[uint64]ngramOccurrence),
	}
}

// reset clears word slices and maps in-place with zero heap allocations (reusing map buckets).
func (l *streamLane) reset() {
	l.words = l.words[:0]
	clear(l.ngramCounts)
	clear(l.ngramHistory)
	l.tokens = 0
	l.maxNgramFreq = 0
	l.pendingWord.Reset()
}

func (l *streamLane) maxLoopDistance() int {
	if l.tokens < 512 {
		return 24
	}
	return defaultMaxLoopDistance
}

func (l *streamLane) addWord(word string, window int, threshold int) bool {
	l.words = append(l.words, word)
	wLen := len(l.words)

	if wLen < window {
		return false
	}

	h := fnv.New64a()
	for i := wLen - window; i < wLen; i++ {
		_, _ = h.Write([]byte(l.words[i]))
		_, _ = h.Write([]byte{0}) // separator
	}
	hashVal := h.Sum64()

	l.ngramCounts[hashVal]++
	if l.ngramCounts[hashVal] > l.maxNgramFreq {
		l.maxNgramFreq = l.ngramCounts[hashVal]
	}

	consecutive := 1
	if prev, exists := l.ngramHistory[hashVal]; exists {
		dist := wLen - prev.lastWordIdx
		if dist <= l.maxLoopDistance() {
			consecutive = prev.consecutiveCount + 1
		}
	}
	l.ngramHistory[hashVal] = ngramOccurrence{
		consecutiveCount: consecutive,
		lastWordIdx:      wLen,
	}

	return consecutive >= threshold
}

// CycleBreaker monitors in-flight streaming deltas across isolated thinking, prose, and tool lanes
// to detect and break infinite circular reasoning loops, runaway prose monologues, and cyclic tool calls in real-time.
// Request-scoped, lock-free, and recycled via sync.Pool.
type CycleBreaker struct {
	cfg   Config
	prose streamLane
	think streamLane
	tool  streamLane
}

var cycleBreakerPool = sync.Pool{
	New: func() any {
		return &CycleBreaker{
			prose: newAllocatedStreamLane(),
			think: newAllocatedStreamLane(),
			tool:  newAllocatedStreamLane(),
		}
	},
}

// Configure applies configuration values to this CycleBreaker without reallocating maps.
func (cb *CycleBreaker) Configure(cfg *contract.CycleBreakerConfig) {
	cb.cfg = Config{
		Enabled:                     true,
		MaxProseTokens:              defaultMaxProseTokens,
		MaxThinkingTokens:           defaultMaxThinkingTokens,
		MaxToolTokens:               defaultMaxToolTokens,
		MaxWriteTokens:              defaultMaxWriteTokens,
		RepetitionWindow:            defaultRepetitionWindow,
		RepetitionThreshold:         defaultRepetitionThreshold,
		ThinkingRepetitionThreshold: defaultThinkingRepetitionThreshold,
		MaxRetries:                  defaultMaxRetries,
		CorrectionPrompt:            contract.CycleBreakerDefaultCorrectionPrompt,
	}

	if cfg != nil {
		if cfg.Enabled != nil {
			cb.cfg.Enabled = *cfg.Enabled
		}
		if cfg.MaxProseTokens > 0 {
			cb.cfg.MaxProseTokens = cfg.MaxProseTokens
		}
		if cfg.MaxThinkingTokens > 0 {
			cb.cfg.MaxThinkingTokens = cfg.MaxThinkingTokens
		}
		if cfg.MaxToolTokens > 0 {
			cb.cfg.MaxToolTokens = cfg.MaxToolTokens
		}
		if cfg.MaxWriteTokens > 0 {
			cb.cfg.MaxWriteTokens = cfg.MaxWriteTokens
		}
		if cfg.RepetitionWindow > 0 {
			cb.cfg.RepetitionWindow = cfg.RepetitionWindow
		}
		if cfg.RepetitionThreshold > 0 {
			cb.cfg.RepetitionThreshold = cfg.RepetitionThreshold
		}
		if cfg.ThinkingRepetitionThreshold > 0 {
			cb.cfg.ThinkingRepetitionThreshold = cfg.ThinkingRepetitionThreshold
		}
		if cfg.MaxRetries > 0 {
			cb.cfg.MaxRetries = cfg.MaxRetries
		}
		if cfg.CorrectionPrompt != "" {
			cb.cfg.CorrectionPrompt = cfg.CorrectionPrompt
		}
	}
}

// GetCycleBreaker borrows a CycleBreaker instance from the pool configured with cfg.
func GetCycleBreaker(cfg *contract.CycleBreakerConfig) *CycleBreaker {
	cb := cycleBreakerPool.Get().(*CycleBreaker)
	cb.Reset()
	cb.Configure(cfg)
	return cb
}

// PutCycleBreaker resets the CycleBreaker in-place and returns it to the pool for reuse.
func PutCycleBreaker(cb *CycleBreaker) {
	if cb == nil {
		return
	}
	cb.Reset()
	cycleBreakerPool.Put(cb)
}

// NewCycleBreaker initializes a CycleBreaker instance. Delegates to GetCycleBreaker for zero-alloc pool reuse.
func NewCycleBreaker(cfg *contract.CycleBreakerConfig) *CycleBreaker {
	return GetCycleBreaker(cfg)
}

// IsEnabled returns whether the cycle breaker is active (lock-free).
func (cb *CycleBreaker) IsEnabled() bool {
	return cb.cfg.Enabled
}

// CorrectionPrompt returns the configured system override injection prompt (lock-free).
func (cb *CycleBreaker) CorrectionPrompt() string {
	return cb.cfg.CorrectionPrompt
}

// MaxRetries returns the allowed number of local Stage 1 retries (lock-free).
func (cb *CycleBreaker) MaxRetries() int {
	return cb.cfg.MaxRetries
}

// ProseTokens returns the current accumulated non-thinking prose token count (lock-free).
func (cb *CycleBreaker) ProseTokens() int {
	return cb.prose.tokens
}

// MaxNgramFreq returns the highest observed N-gram frequency in the prose lane (lock-free).
func (cb *CycleBreaker) MaxNgramFreq() int {
	return cb.prose.maxNgramFreq
}

// ThinkingTokens returns the current accumulated thinking token count (lock-free).
func (cb *CycleBreaker) ThinkingTokens() int {
	return cb.think.tokens
}

// MaxThinkingNgramFreq returns the highest observed N-gram frequency in the thinking lane (lock-free).
func (cb *CycleBreaker) MaxThinkingNgramFreq() int {
	return cb.think.maxNgramFreq
}

// ToolTokens returns the current accumulated tool lane token count (lock-free).
func (cb *CycleBreaker) ToolTokens() int {
	return cb.tool.tokens
}

// MaxToolNgramFreq returns the highest observed N-gram frequency in the tool lane (lock-free).
func (cb *CycleBreaker) MaxToolNgramFreq() int {
	return cb.tool.maxNgramFreq
}

// MaxWriteTokens returns the configured maximum tool tokens for file writes (lock-free).
func (cb *CycleBreaker) MaxWriteTokens() int {
	return cb.cfg.MaxWriteTokens
}

// Reset clears accumulated words, n-grams, and token counters across all lanes in-place with 0 allocations.
func (cb *CycleBreaker) Reset() {
	cb.prose.reset()
	cb.think.reset()
	cb.tool.reset()
}

// ProcessDelta parses a text delta chunk from the stream and checks for repetition loops or budget breaches.
// Routes to either the thinking lane or prose lane based on isThinking. Lock-free hot path.
func (cb *CycleBreaker) ProcessDelta(content string, isThinking bool) (triggered bool, reason string) {
	if !cb.cfg.Enabled || content == "" {
		return false, ""
	}

	if isThinking {
		lane := &cb.think
		lane.tokens += (len(content) + 3) / 4

		for _, r := range content {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				lane.pendingWord.WriteRune(unicode.ToLower(r))
			} else {
				if lane.pendingWord.Len() > 0 {
					word := lane.pendingWord.String()
					lane.pendingWord.Reset()
					if lane.addWord(word, cb.cfg.RepetitionWindow, cb.cfg.ThinkingRepetitionThreshold) {
						return true, "thinking_repetition_loop_detected"
					}
				}
			}
		}

		if lane.tokens > cb.cfg.MaxThinkingTokens && lane.maxNgramFreq >= 2 {
			return true, "thinking_budget_exceeded_with_repetition"
		}

		return false, ""
	}

	lane := &cb.prose
	lane.tokens += (len(content) + 3) / 4

	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			lane.pendingWord.WriteRune(unicode.ToLower(r))
		} else {
			if lane.pendingWord.Len() > 0 {
				word := lane.pendingWord.String()
				lane.pendingWord.Reset()
				if lane.addWord(word, cb.cfg.RepetitionWindow, cb.cfg.RepetitionThreshold) {
					return true, "ngram_repetition_loop_detected"
				}
			}
		}
	}

	if lane.tokens > cb.cfg.MaxProseTokens && lane.maxNgramFreq >= 2 {
		return true, "prose_budget_exceeded_with_repetition"
	}

	return false, ""
}

// ProcessToolDelta parses a tool argument delta chunk from the stream and checks for repetition loops or budget breaches.
// Operates on an isolated tool lane to protect against degenerate loops inside tool arguments.
// When category is ToolCategoryFileWrite, sliding N-gram loop detection is bypassed with zero-alloc fast exit.
// Lock-free hot path.
func (cb *CycleBreaker) ProcessToolDelta(content string, category ...ToolActivityCategory) (triggered bool, reason string) {
	if !cb.cfg.Enabled || content == "" {
		return false, ""
	}

	actCat := ToolCategoryCommand
	if len(category) > 0 {
		actCat = category[0]
	}

	lane := &cb.tool
	deltaTokens := (len(content) + 3) / 4
	lane.tokens += deltaTokens

	// Category A: File writes (zero-alloc fast path)
	if actCat == ToolCategoryFileWrite {
		if lane.tokens > cb.cfg.MaxWriteTokens {
			return true, "write_budget_exceeded"
		}
		return false, ""
	}

	// Category B: Commands & tool invocations (bounded by maxToolTokens, sliding N-gram loop detection)
	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			lane.pendingWord.WriteRune(unicode.ToLower(r))
		} else {
			if lane.pendingWord.Len() > 0 {
				word := lane.pendingWord.String()
				lane.pendingWord.Reset()
				if lane.addWord(word, cb.cfg.RepetitionWindow, cb.cfg.RepetitionThreshold) {
					return true, "tool_repetition_loop_detected"
				}
			}
		}
	}

	if lane.tokens > cb.cfg.MaxToolTokens && lane.maxNgramFreq >= 2 {
		return true, "tool_budget_exceeded_with_repetition"
	}

	return false, ""
}
