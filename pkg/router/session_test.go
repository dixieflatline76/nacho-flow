package router

import (
	"sync"
	"testing"
	"time"
)

func TestSessionTracker_ConsecutiveRetries(t *testing.T) {
	tracker := NewSessionTracker(500 * time.Millisecond)

	sessionID := "sess-123"
	prompt1 := "Fix compilation error in main.go"
	h1 := HashPrompt(prompt1)

	// Turn 0: Fresh prompt
	retries, isRetry := tracker.RecordTurn(sessionID, h1, false)
	if retries != 0 || isRetry {
		t.Errorf("Expected Turn 0 to have retries=0, isRetry=false; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// Turn 1: Duplicate prompt (Retry 1)
	retries, isRetry = tracker.RecordTurn(sessionID, h1, false)
	if retries != 1 || !isRetry {
		t.Errorf("Expected Turn 1 to have retries=1, isRetry=true; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// Turn 2: Duplicate prompt again (Retry 2)
	retries, isRetry = tracker.RecordTurn(sessionID, h1, false)
	if retries != 2 || !isRetry {
		t.Errorf("Expected Turn 2 to have retries=2, isRetry=true; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// Query GetRetries
	if r := tracker.GetRetries(sessionID); r != 2 {
		t.Errorf("Expected GetRetries to return 2, got %d", r)
	}

	// Turn 3: User corrects prompt (new distinct turn)
	prompt2 := "Fix syntax error in router.go"
	h2 := HashPrompt(prompt2)
	retries, isRetry = tracker.RecordTurn(sessionID, h2, false)
	if retries != 0 || isRetry {
		t.Errorf("Expected distinct prompt to reset retries=0, isRetry=false; got retries=%d, isRetry=%v", retries, isRetry)
	}
}

func TestSessionTracker_LazyEviction(t *testing.T) {
	ttl := 50 * time.Millisecond
	tracker := NewSessionTracker(ttl)

	sessionID := "sess-expiring"
	h := HashPrompt("same prompt")

	tracker.RecordTurn(sessionID, h, false)
	tracker.RecordTurn(sessionID, h, false) // retries = 1

	if tracker.GetRetries(sessionID) != 1 {
		t.Fatalf("Expected retries=1 before expiration")
	}

	// Wait for TTL to expire
	time.Sleep(70 * time.Millisecond)

	// Next turn with same prompt should treat it as fresh turn because TTL elapsed
	retries, isRetry := tracker.RecordTurn(sessionID, h, false)
	if retries != 0 || isRetry {
		t.Errorf("Expected expired session to reset to retries=0, isRetry=false; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// GetRetries after expiration for non-accessed session
	tracker.RecordTurn("sess-dead", h, false)
	time.Sleep(70 * time.Millisecond)
	if r := tracker.GetRetries("sess-dead"); r != 0 {
		t.Errorf("Expected GetRetries to return 0 for expired session, got %d", r)
	}
}

func TestSessionTracker_ResetAndEmptyKeys(t *testing.T) {
	tracker := NewSessionTracker(time.Minute)

	// Empty session key
	retries, isRetry := tracker.RecordTurn("", 12345, false)
	if retries != 0 || isRetry {
		t.Errorf("Expected empty session key to return 0, false")
	}
	if r := tracker.GetRetries(""); r != 0 {
		t.Errorf("Expected GetRetries for empty key to return 0, got %d", r)
	}

	sessionID := "sess-to-reset"
	h := HashPrompt("some prompt")
	tracker.RecordTurn(sessionID, h, false)
	tracker.RecordTurn(sessionID, h, false) // retries = 1

	tracker.Reset(sessionID)
	if r := tracker.GetRetries(sessionID); r != 0 {
		t.Errorf("Expected retries to be 0 after Reset, got %d", r)
	}
}

func TestSessionTracker_BoundedCapacity(t *testing.T) {
	tracker := NewSessionTracker(time.Hour)
	tracker.maxSessions = 100 // Set low cap for testing

	// Add 150 unique sessions
	for i := 0; i < 150; i++ {
		key := "sess-" + string(rune(i))
		tracker.RecordTurn(key, uint64(i+1), false)
	}

	tracker.mu.RLock()
	count := len(tracker.sessions)
	tracker.mu.RUnlock()

	if count > 100 {
		t.Errorf("Expected sessions count to be bounded <= 100, got %d", count)
	}
}

func TestSessionTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewSessionTracker(time.Minute)
	var wg sync.WaitGroup
	workers := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sess := "sess-worker"
			h := HashPrompt("concurrent prompt")
			for j := 0; j < 100; j++ {
				tracker.RecordTurn(sess, h, false)
				_ = tracker.GetRetries(sess)
				if j == 50 {
					tracker.Reset(sess)
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestSessionTracker_EdgeCases(t *testing.T) {
	// Default TTL when 0 passed
	st := NewSessionTracker(0)
	if st.ttl != DefaultSessionTTL {
		t.Errorf("Expected DefaultSessionTTL %v, got %v", DefaultSessionTTL, st.ttl)
	}

	// Empty prompt hash
	if h := HashPrompt(""); h != 0 {
		t.Errorf("Expected 0 hash for empty prompt, got %d", h)
	}

	// Non existent session
	if r := st.GetRetries("random-unknown-id"); r != 0 {
		t.Errorf("Expected 0 retries for unknown session, got %d", r)
	}
}

func TestSessionTracker_MetaDebounce(t *testing.T) {
	st := NewSessionTracker(time.Minute)

	// 1. Empty session key should not debounce
	if st.ShouldDebounceMeta("", "help", 2*time.Second) {
		t.Errorf("empty session should never be debounced")
	}

	// 2. First call for session should not debounce
	sess := "session-debounce-1"
	if st.ShouldDebounceMeta(sess, "help", 2*time.Second) {
		t.Errorf("first meta call should not be debounced")
	}

	// 3. Immediate repeat call with same directive within window SHOULD debounce
	if !st.ShouldDebounceMeta(sess, "help", 2*time.Second) {
		t.Errorf("immediate repeat meta call should be debounced")
	}

	// 4. Different directive within window should NOT debounce
	if st.ShouldDebounceMeta(sess, "tiers", 2*time.Second) {
		t.Errorf("different directive should not be debounced")
	}

	// 5. Expired window should NOT debounce
	time.Sleep(20 * time.Millisecond)
	if st.ShouldDebounceMeta(sess, "tiers", 10*time.Millisecond) {
		t.Errorf("call after window expired should not be debounced")
	}
}

func TestSessionTracker_TurnZeroHash(t *testing.T) {
	st := NewSessionTracker(time.Minute)
	// Turn with 0 prompt hash
	st.RecordTurn("sess-zero", 0, false)
	retries, isRetry := st.RecordTurn("sess-zero", 0, false)
	if retries != 0 || isRetry {
		t.Errorf("Expected zero prompt hash not to trigger retry, got retries=%d, isRetry=%v", retries, isRetry)
	}
}

func TestSessionTracker_ToolProgressResetsRetries(t *testing.T) {
	tracker := NewSessionTracker(time.Minute)
	sess := "sess-agent"
	h := HashPrompt("Please proceed and implement")

	// Turn 0: First contact
	retries, isRetry := tracker.RecordTurn(sess, h, false)
	if retries != 0 || isRetry {
		t.Errorf("Turn 0: expected retries=0, isRetry=false; got %d, %v", retries, isRetry)
	}

	// Turn 1: Same prompt, but tool progress exists -> NOT a retry
	retries, isRetry = tracker.RecordTurn(sess, h, true)
	if retries != 0 || isRetry {
		t.Errorf("Turn 1 (tool progress): expected retries=0, isRetry=false; got %d, %v", retries, isRetry)
	}

	// Turn 2: Same prompt, tool progress again -> still NOT a retry
	retries, isRetry = tracker.RecordTurn(sess, h, true)
	if retries != 0 || isRetry {
		t.Errorf("Turn 2 (tool progress): expected retries=0, isRetry=false; got %d, %v", retries, isRetry)
	}

	// Turn 3: Same prompt, NO tool progress -> NOW it's a retry
	retries, isRetry = tracker.RecordTurn(sess, h, false)
	if retries != 1 || !isRetry {
		t.Errorf("Turn 3 (no progress): expected retries=1, isRetry=true; got %d, %v", retries, isRetry)
	}
}

func TestSessionTracker_EscalationBudget(t *testing.T) {
	tracker := NewSessionTracker(time.Minute)
	sess := "sess-escalation"
	h := HashPrompt("fix the bug")
	tracker.RecordTurn(sess, h, false) // initial turn

	// Escalation turns 1..3 should NOT exhaust budget
	for i := 1; i <= MaxEscalationTurns; i++ {
		exhausted := tracker.RecordEscalation(sess)
		if exhausted {
			t.Errorf("Escalation turn %d should NOT exhaust budget", i)
		}
	}

	// Escalation turn 4 SHOULD exhaust budget
	exhausted := tracker.RecordEscalation(sess)
	if !exhausted {
		t.Errorf("Escalation turn %d should exhaust budget", MaxEscalationTurns+1)
	}

	// Reset should clear the counter
	tracker.ResetEscalation(sess)
	exhausted = tracker.RecordEscalation(sess)
	if exhausted {
		t.Errorf("After reset, escalation should NOT be exhausted")
	}
}
