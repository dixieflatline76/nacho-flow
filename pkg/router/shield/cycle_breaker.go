package shield

import (
	"hash/fnv"
	"strings"
	"sync"
	"unicode"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

const (
	defaultMaxProseTokens              = 4096
	defaultMaxThinkingTokens           = 1500
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

// CycleBreaker monitors in-flight streaming deltas across isolated thinking and prose lanes
// to detect and break infinite circular reasoning loops and runaway prose monologues in real-time.
type CycleBreaker struct {
	mu                          sync.Mutex
	enabled                     bool
	maxProseTokens              int
	maxThinkingTokens           int
	repetitionWindow            int
	repetitionThreshold         int
	thinkingRepetitionThreshold int
	maxRetries                  int
	correctionPrompt            string

	// Prose lane state
	words        []string
	ngramCounts  map[uint64]int
	ngramHistory map[uint64]ngramOccurrence
	proseTokens  int
	maxNgramFreq int
	pendingWord  strings.Builder

	// Thinking lane state (isolated)
	thinkingWords        []string
	thinkingNgramCounts  map[uint64]int
	thinkingNgramHistory map[uint64]ngramOccurrence
	thinkingTokens       int
	maxThinkingNgramFreq int
	thinkingPendingWord  strings.Builder
}

// NewCycleBreaker initializes a CycleBreaker instance with provided or default configuration.
func NewCycleBreaker(cfg *contract.CycleBreakerConfig) *CycleBreaker {
	cb := &CycleBreaker{
		enabled:                     true,
		maxProseTokens:              defaultMaxProseTokens,
		maxThinkingTokens:           defaultMaxThinkingTokens,
		repetitionWindow:            defaultRepetitionWindow,
		repetitionThreshold:         defaultRepetitionThreshold,
		thinkingRepetitionThreshold: defaultThinkingRepetitionThreshold,
		maxRetries:                  defaultMaxRetries,
		correctionPrompt:            contract.CycleBreakerDefaultCorrectionPrompt,
		ngramCounts:                 make(map[uint64]int),
		ngramHistory:                make(map[uint64]ngramOccurrence),
		words:                       make([]string, 0, 128),
		thinkingNgramCounts:         make(map[uint64]int),
		thinkingNgramHistory:        make(map[uint64]ngramOccurrence),
		thinkingWords:               make([]string, 0, 128),
	}

	if cfg != nil {
		if cfg.Enabled != nil {
			cb.enabled = *cfg.Enabled
		}
		if cfg.MaxProseTokens > 0 {
			cb.maxProseTokens = cfg.MaxProseTokens
		}
		if cfg.MaxThinkingTokens > 0 {
			cb.maxThinkingTokens = cfg.MaxThinkingTokens
		}
		if cfg.RepetitionWindow > 0 {
			cb.repetitionWindow = cfg.RepetitionWindow
		}
		if cfg.RepetitionThreshold > 0 {
			cb.repetitionThreshold = cfg.RepetitionThreshold
		}
		if cfg.ThinkingRepetitionThreshold > 0 {
			cb.thinkingRepetitionThreshold = cfg.ThinkingRepetitionThreshold
		}
		if cfg.MaxRetries > 0 {
			cb.maxRetries = cfg.MaxRetries
		}
		if cfg.CorrectionPrompt != "" {
			cb.correctionPrompt = cfg.CorrectionPrompt
		}
	}

	return cb
}

// IsEnabled returns whether the cycle breaker is active.
func (cb *CycleBreaker) IsEnabled() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.enabled
}

// CorrectionPrompt returns the configured system override injection prompt.
func (cb *CycleBreaker) CorrectionPrompt() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.correctionPrompt
}

// MaxRetries returns the allowed number of local Stage 1 retries.
func (cb *CycleBreaker) MaxRetries() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.maxRetries
}

// ProseTokens returns the current accumulated non-thinking prose token count.
func (cb *CycleBreaker) ProseTokens() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.proseTokens
}

// MaxNgramFreq returns the highest observed N-gram frequency in the prose lane.
func (cb *CycleBreaker) MaxNgramFreq() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.maxNgramFreq
}

// ThinkingTokens returns the current accumulated thinking token count.
func (cb *CycleBreaker) ThinkingTokens() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.thinkingTokens
}

// MaxThinkingNgramFreq returns the highest observed N-gram frequency in the thinking lane.
func (cb *CycleBreaker) MaxThinkingNgramFreq() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.maxThinkingNgramFreq
}

// Reset clears accumulated words, n-grams, and token counters across both prose and thinking lanes.
func (cb *CycleBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.words = cb.words[:0]
	cb.ngramCounts = make(map[uint64]int)
	cb.ngramHistory = make(map[uint64]ngramOccurrence)
	cb.proseTokens = 0
	cb.maxNgramFreq = 0
	cb.pendingWord.Reset()

	cb.thinkingWords = cb.thinkingWords[:0]
	cb.thinkingNgramCounts = make(map[uint64]int)
	cb.thinkingNgramHistory = make(map[uint64]ngramOccurrence)
	cb.thinkingTokens = 0
	cb.maxThinkingNgramFreq = 0
	cb.thinkingPendingWord.Reset()
}

// ProcessDelta parses a text delta chunk from the stream and checks for repetition loops or budget breaches.
// Routes to either the thinking lane or prose lane based on isThinking.
// Returns triggered=true with a descriptive reason if a violation occurs.
func (cb *CycleBreaker) ProcessDelta(content string, isThinking bool) (triggered bool, reason string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if !cb.enabled || content == "" {
		return false, ""
	}

	if isThinking {
		// ── Thinking Lane ──
		cb.thinkingTokens += (len(content) + 3) / 4

		for _, r := range content {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				cb.thinkingPendingWord.WriteRune(unicode.ToLower(r))
			} else {
				if cb.thinkingPendingWord.Len() > 0 {
					word := cb.thinkingPendingWord.String()
					cb.thinkingPendingWord.Reset()
					if cb.addThinkingWord(word) {
						return true, "thinking_repetition_loop_detected"
					}
				}
			}
		}

		if cb.thinkingTokens > cb.maxThinkingTokens && cb.maxThinkingNgramFreq >= 2 {
			return true, "thinking_budget_exceeded_with_repetition"
		}

		return false, ""
	}

	// ── Prose Lane ──
	cb.proseTokens += (len(content) + 3) / 4

	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cb.pendingWord.WriteRune(unicode.ToLower(r))
		} else {
			if cb.pendingWord.Len() > 0 {
				word := cb.pendingWord.String()
				cb.pendingWord.Reset()
				if cb.addWord(word) {
					return true, "ngram_repetition_loop_detected"
				}
			}
		}
	}

	if cb.proseTokens > cb.maxProseTokens && cb.maxNgramFreq >= 2 {
		return true, "prose_budget_exceeded_with_repetition"
	}

	return false, ""
}

// addWord appends a word and updates the sliding prose N-gram frequency table.
// Enforces a locality check: to trigger an ngram repetition loop, repetitions must
// occur within tight proximity (<= defaultMaxLoopDistance words apart from previous occurrence).
func (cb *CycleBreaker) addWord(word string) bool {
	cb.words = append(cb.words, word)
	wLen := len(cb.words)

	if wLen < cb.repetitionWindow {
		return false
	}

	// Compute FNV-1a hash of the trailing N-gram window
	h := fnv.New64a()
	for i := wLen - cb.repetitionWindow; i < wLen; i++ {
		_, _ = h.Write([]byte(cb.words[i]))
		_, _ = h.Write([]byte{0}) // separator
	}
	hashVal := h.Sum64()

	cb.ngramCounts[hashVal]++
	if cb.ngramCounts[hashVal] > cb.maxNgramFreq {
		cb.maxNgramFreq = cb.ngramCounts[hashVal]
	}

	consecutive := 1
	if prev, exists := cb.ngramHistory[hashVal]; exists {
		dist := wLen - prev.lastWordIdx
		if dist <= cb.maxLoopDistance() {
			consecutive = prev.consecutiveCount + 1
		}
	}
	cb.ngramHistory[hashVal] = ngramOccurrence{
		consecutiveCount: consecutive,
		lastWordIdx:      wLen,
	}

	return consecutive >= cb.repetitionThreshold
}

// maxLoopDistance returns the maximum distance between successive occurrences
// to be considered part of the same repetition loop.
// During the early runway (< 512 prose tokens), we require a tight loop (<= 24 words)
// so that valid algorithmic explanations, multi-case proofs, and bullet points
// are never killed at 125 tokens. Once past the runway, a wider distance (<= 48 words) is permitted.
func (cb *CycleBreaker) maxLoopDistance() int {
	if cb.proseTokens < 512 {
		return 24
	}
	return defaultMaxLoopDistance
}

// addThinkingWord appends a word and updates the sliding thinking N-gram frequency table.
// Enforces a locality check: to trigger a thinking repetition loop, repetitions must
// occur within tight proximity (<= maxThinkingLoopDistance words apart from previous occurrence).
func (cb *CycleBreaker) addThinkingWord(word string) bool {
	cb.thinkingWords = append(cb.thinkingWords, word)
	wLen := len(cb.thinkingWords)

	if wLen < cb.repetitionWindow {
		return false
	}

	h := fnv.New64a()
	for i := wLen - cb.repetitionWindow; i < wLen; i++ {
		_, _ = h.Write([]byte(cb.thinkingWords[i]))
		_, _ = h.Write([]byte{0}) // separator
	}
	hashVal := h.Sum64()

	cb.thinkingNgramCounts[hashVal]++
	if cb.thinkingNgramCounts[hashVal] > cb.maxThinkingNgramFreq {
		cb.maxThinkingNgramFreq = cb.thinkingNgramCounts[hashVal]
	}

	consecutive := 1
	if prev, exists := cb.thinkingNgramHistory[hashVal]; exists {
		dist := wLen - prev.lastWordIdx
		if dist <= cb.maxThinkingLoopDistance() {
			consecutive = prev.consecutiveCount + 1
		}
	}
	cb.thinkingNgramHistory[hashVal] = ngramOccurrence{
		consecutiveCount: consecutive,
		lastWordIdx:      wLen,
	}

	return consecutive >= cb.thinkingRepetitionThreshold
}

func (cb *CycleBreaker) maxThinkingLoopDistance() int {
	if cb.thinkingTokens < 512 {
		return 24
	}
	return defaultMaxLoopDistance
}
