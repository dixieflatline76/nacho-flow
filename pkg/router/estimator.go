package router

import (
	"sync/atomic"
)

const (
	DefaultCharsPerToken = 3.2
	MinCharsPerToken     = 1.8
	MaxCharsPerToken     = 5.0
	EmaWeight            = 0.2
)

// TokenEstimator provides a lock-free, self-calibrating character-to-token ratio estimator.
type TokenEstimator struct {
	ratio atomic.Pointer[float64]
}

// NewTokenEstimator initializes an estimator calibrated to the default code/JSON ratio (3.2 chars/token).
func NewTokenEstimator() *TokenEstimator {
	e := &TokenEstimator{}
	initVal := DefaultCharsPerToken
	e.ratio.Store(&initVal)
	return e
}

// GetRatio returns the active characters-per-token ratio.
func (e *TokenEstimator) GetRatio() float64 {
	r := e.ratio.Load()
	if r == nil || *r <= 0 {
		return DefaultCharsPerToken
	}
	return *r
}

// Estimate calculates the approximate token count from the given character count.
func (e *TokenEstimator) Estimate(charCount int) int {
	if charCount <= 0 {
		return 0
	}
	r := e.GetRatio()
	tokens := int(float64(charCount) / r)
	if tokens == 0 && charCount > 0 {
		return 1
	}
	return tokens
}

// Calibrate updates the ratio using an Exponential Moving Average (EMA) against observed actual prompt tokens.
func (e *TokenEstimator) Calibrate(actualPromptTokens int, charCount int) {
	if actualPromptTokens <= 0 || charCount <= 0 {
		return
	}
	observed := float64(charCount) / float64(actualPromptTokens)
	if observed < MinCharsPerToken {
		observed = MinCharsPerToken
	}
	if observed > MaxCharsPerToken {
		observed = MaxCharsPerToken
	}
	curr := e.GetRatio()
	updated := (curr * (1.0 - EmaWeight)) + (observed * EmaWeight)
	e.ratio.Store(&updated)
}
