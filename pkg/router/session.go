package router

import (
	"hash/fnv"
	"sync"
	"time"
)

const (
	DefaultSessionTTL  = 5 * time.Minute
	MaxTrackedSessions = 10000
)

// SessionState tracks the retry count, prompt hash, and last activity timestamp of a session.
type SessionState struct {
	RetriesCount      int
	EscalationCount   int
	LastTurnTime      time.Time
	PromptHash        uint64
	LastMetaDirective string
	LastMetaTime      time.Time
}

// SessionTracker tracks consecutive turn retries per session without leaking background goroutines.
type SessionTracker struct {
	mu          sync.RWMutex
	sessions    map[string]*SessionState
	ttl         time.Duration
	maxSessions int
}

// NewSessionTracker initializes a SessionTracker with the specified TTL and bounded size.
func NewSessionTracker(ttl time.Duration) *SessionTracker {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	return &SessionTracker{
		sessions:    make(map[string]*SessionState),
		ttl:         ttl,
		maxSessions: MaxTrackedSessions,
	}
}

// HashPrompt computes an FNV-1a 64-bit hash of the prompt for fast retry comparison.
func HashPrompt(prompt string) uint64 {
	if prompt == "" {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(prompt))
	return h.Sum64()
}

// RecordTurn records a turn for sessionKey and promptHash.
// If hasToolProgress is true (e.g. intermediate tool calls succeeded in an agent loop),
// the turn is treated as forward progress and Retries is reset to 0 even if promptHash is identical.
// Returns (retries, isRetry).
func (st *SessionTracker) RecordTurn(sessionKey string, promptHash uint64, hasToolProgress bool) (retries int, isRetry bool) {
	if sessionKey == "" {
		return 0, false
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	state, exists := st.sessions[sessionKey]

	if exists {
		// Check lazy expiration
		if now.Sub(state.LastTurnTime) > st.ttl {
			state.RetriesCount = 0
			state.EscalationCount = 0
			state.PromptHash = promptHash
			state.LastTurnTime = now
			return 0, false
		}

		// Check if it's the exact same prompt
		if state.PromptHash == promptHash && promptHash != 0 {
			if hasToolProgress {
				// Agent is making forward progress (successful tool calls in history).
				// This is a normal multi-step autonomous loop, NOT a retry.
				state.RetriesCount = 0
				state.EscalationCount = 0
				state.LastTurnTime = now
				return 0, false
			}
			// No tool progress + same prompt = genuine retry
			state.RetriesCount++
			state.LastTurnTime = now
			return state.RetriesCount, true
		}

		// Distinct turn within same session
		state.RetriesCount = 0
		state.EscalationCount = 0
		state.PromptHash = promptHash
		state.LastTurnTime = now
		return 0, false
	}

	// Enforce capacity bounds with lazy cleanup
	st.evictExpiredOrOldest(now)

	st.sessions[sessionKey] = &SessionState{
		RetriesCount: 0,
		LastTurnTime: now,
		PromptHash:   promptHash,
	}
	return 0, false
}

func (st *SessionTracker) evictExpiredOrOldest(now time.Time) {
	if len(st.sessions) < st.maxSessions {
		return
	}
	for k, v := range st.sessions {
		if now.Sub(v.LastTurnTime) > st.ttl {
			delete(st.sessions, k)
		}
	}
	if len(st.sessions) >= st.maxSessions {
		for k := range st.sessions {
			delete(st.sessions, k)
			break
		}
	}
}

// Reset clears session state for the given session key.
func (st *SessionTracker) Reset(sessionKey string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.sessions, sessionKey)
}

// GetRetries returns the current retries count for sessionKey.
func (st *SessionTracker) GetRetries(sessionKey string) int {
	if sessionKey == "" {
		return 0
	}
	st.mu.RLock()
	defer st.mu.RUnlock()

	state, exists := st.sessions[sessionKey]
	if !exists {
		return 0
	}
	if time.Since(state.LastTurnTime) > st.ttl {
		return 0
	}
	return state.RetriesCount
}

const MaxEscalationTurns = 3

// RecordEscalation increments the escalation counter for the session.
// Returns true if the budget is exhausted (consecutive frontier turns > MaxEscalationTurns).
func (st *SessionTracker) RecordEscalation(sessionKey string) bool {
	if sessionKey == "" {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	state, exists := st.sessions[sessionKey]
	if !exists {
		return false
	}
	state.EscalationCount++
	return state.EscalationCount > MaxEscalationTurns
}

// ResetEscalation resets the escalation counter (called when NOT on the default tier).
func (st *SessionTracker) ResetEscalation(sessionKey string) {
	if sessionKey == "" {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	state, exists := st.sessions[sessionKey]
	if exists {
		state.EscalationCount = 0
	}
}

// ShouldDebounceMeta checks if an identical meta directive was executed within window for sessionKey.
// If true, returns true indicating the call should be debounced.
// If false, records current timestamp/directive and returns false.
func (st *SessionTracker) ShouldDebounceMeta(sessionKey, directive string, window time.Duration) bool {
	if sessionKey == "" || directive == "" || window <= 0 {
		return false
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	state, exists := st.sessions[sessionKey]
	if !exists {
		st.evictExpiredOrOldest(now)
		st.sessions[sessionKey] = &SessionState{
			LastTurnTime:      now,
			LastMetaDirective: directive,
			LastMetaTime:      now,
		}
		return false
	}

	state.LastTurnTime = now
	if state.LastMetaDirective == directive && now.Sub(state.LastMetaTime) < window {
		return true
	}

	state.LastMetaDirective = directive
	state.LastMetaTime = now
	return false
}
