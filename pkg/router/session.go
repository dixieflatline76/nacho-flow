package router

import (
	"hash/fnv"
	"sync"
	"time"
)

const (
	DefaultSessionTTL    = 5 * time.Minute
	DefaultModelCooldown = 2 * time.Minute
	MaxTrackedSessions   = 10000
)

// SessionState tracks the retry count, prompt hash, and last activity timestamp of a session.
type SessionState struct {
	RetriesCount      int
	EscalationCount   int
	KickstartCount    int // consecutive turns without tool progress
	MinRetriesFloor   int // floor to maintain retry escalation across context resets
	LastTurnTime      time.Time
	PromptHash        uint64
	LastMetaDirective string
	LastMetaTime      time.Time
	CoolingDownModels map[string]time.Time // model -> cooldown expiration
	WriteProgressCount int            // total write-progress turns in this session
	FairyDustCounts    map[string]int // per-entry invocation counts (entry.Name -> count)
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
		// Clean expired model cooldowns lazily
		if state.CoolingDownModels != nil {
			for m, exp := range state.CoolingDownModels {
				if now.After(exp) {
					delete(state.CoolingDownModels, m)
				}
			}
		}

		// Check lazy expiration
		if now.Sub(state.LastTurnTime) > st.ttl {
			state.RetriesCount = 0
			state.EscalationCount = 0
			state.MinRetriesFloor = 0
			state.WriteProgressCount = 0
			state.FairyDustCounts = nil
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
				retries = 0
				isRetry = false
			} else {
				// No tool progress + same prompt = genuine retry
				state.RetriesCount++
				state.LastTurnTime = now
				retries = state.RetriesCount
				isRetry = true
			}
		} else {
			// Distinct turn within same session
			state.RetriesCount = 0
			state.EscalationCount = 0
			state.PromptHash = promptHash
			state.LastTurnTime = now
			retries = 0
			isRetry = false
		}

		// Apply MinRetriesFloor if set (e.g. from a recent cycle kill), and safely decay
		if state.MinRetriesFloor > 0 {
			if retries < state.MinRetriesFloor {
				retries = state.MinRetriesFloor
				isRetry = true
			}
			state.MinRetriesFloor--
		}

		return retries, isRetry
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

// RecordKickstartState updates the kickstart counter based on whether the turn had tool progress.
// If hasToolProgress is true, the kickstart counter is reset to 0.
// Returns (kickstartCount, isKickstarted) where isKickstarted is true when kickstartCount >= kickstartThreshold.
func (st *SessionTracker) RecordKickstartState(sessionKey string, hasToolProgress bool, kickstartThreshold int) (kickstartCount int, isKickstarted bool) {
	if sessionKey == "" || kickstartThreshold <= 0 {
		return 0, false
	}
	st.mu.Lock()
	defer st.mu.Unlock()

	state, exists := st.sessions[sessionKey]
	if !exists {
		return 0, false
	}

	if hasToolProgress {
		state.KickstartCount = 0
		return 0, false
	}
	state.KickstartCount++
	return state.KickstartCount, state.KickstartCount >= kickstartThreshold
}

// GetKickstartCount returns the current kickstart counter for sessionKey.
func (st *SessionTracker) GetKickstartCount(sessionKey string) int {
	if sessionKey == "" {
		return 0
	}
	st.mu.RLock()
	defer st.mu.RUnlock()

	state, exists := st.sessions[sessionKey]
	if !exists {
		return 0
	}
	return state.KickstartCount
}

// RecordCycleKill marks that the specified model was severed by Cycle Killer on this session.
// It sets a retry floor (default 3 or custom) to ensure immediate auto-escalation on the next turn,
// and places the model on a temporary cooldown to prevent re-selection.
func (st *SessionTracker) RecordCycleKill(sessionKey string, model string, cooldown time.Duration, retryFloor ...int) {
	if sessionKey == "" {
		return
	}
	if cooldown <= 0 {
		cooldown = DefaultModelCooldown
	}
	floor := 3
	if len(retryFloor) > 0 && retryFloor[0] > 0 {
		floor = retryFloor[0]
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	state, exists := st.sessions[sessionKey]
	if !exists {
		st.evictExpiredOrOldest(now)
		state = &SessionState{
			LastTurnTime: now,
		}
		st.sessions[sessionKey] = state
	}

	if floor > state.MinRetriesFloor {
		state.MinRetriesFloor = floor
	}

	if model != "" {
		if state.CoolingDownModels == nil {
			state.CoolingDownModels = make(map[string]time.Time)
		}
		state.CoolingDownModels[model] = now.Add(cooldown)
	}
}

// GetCoolingDownModels returns the list of active (non-expired) models currently cooling down for sessionKey.
func (st *SessionTracker) GetCoolingDownModels(sessionKey string) []string {
	if sessionKey == "" {
		return nil
	}

	st.mu.RLock()
	defer st.mu.RUnlock()

	state, exists := st.sessions[sessionKey]
	if !exists || len(state.CoolingDownModels) == 0 {
		return nil
	}

	now := time.Now()
	var models []string
	for m, exp := range state.CoolingDownModels {
		if now.Before(exp) {
			models = append(models, m)
		}
	}
	return models
}

// IsModelCoolingDown checks whether the given model is in an active cooldown for sessionKey.
func (st *SessionTracker) IsModelCoolingDown(sessionKey string, model string) bool {
	if sessionKey == "" || model == "" {
		return false
	}

	st.mu.RLock()
	defer st.mu.RUnlock()

	state, exists := st.sessions[sessionKey]
	if !exists || len(state.CoolingDownModels) == 0 {
		return false
	}

	exp, found := state.CoolingDownModels[model]
	if !found {
		return false
	}
	return time.Now().Before(exp)
}

// RecordWriteProgress increments the global write-progress counter for the session if writeProgress is true.
// Returns the updated WriteProgressCount (0 if session not found or no write progress).
func (st *SessionTracker) RecordWriteProgress(sessionKey string, writeProgress bool) int {
	if sessionKey == "" {
		return 0
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	state, exists := st.sessions[sessionKey]
	if !exists {
		return 0
	}

	if !writeProgress {
		return state.WriteProgressCount
	}
	state.WriteProgressCount++
	return state.WriteProgressCount
}

// CheckFairyDust checks if a specific fairy dust entry should trigger this turn.
// It compares WriteProgressCount against frequency and enforces the per-entry max cap.
// If triggered, it increments the entry's invocation count and returns (newCount, true).
// Must be called AFTER RecordWriteProgress for the same turn.
func (st *SessionTracker) CheckFairyDust(sessionKey, entryName string, frequency, maxPerSession int) (entryCount int, shouldTrigger bool) {
	if sessionKey == "" || entryName == "" || frequency <= 0 {
		return 0, false
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	state, exists := st.sessions[sessionKey]
	if !exists || state.WriteProgressCount == 0 {
		return 0, false
	}

	if state.WriteProgressCount%frequency != 0 {
		return 0, false
	}

	// Lazy init per-entry map
	if state.FairyDustCounts == nil {
		state.FairyDustCounts = make(map[string]int)
	}

	current := state.FairyDustCounts[entryName]
	if maxPerSession > 0 && current >= maxPerSession {
		return current, false
	}

	state.FairyDustCounts[entryName]++
	return state.FairyDustCounts[entryName], true
}
