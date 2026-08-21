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
	retries, isRetry := tracker.RecordTurn(sessionID, h1)
	if retries != 0 || isRetry {
		t.Errorf("Expected Turn 0 to have retries=0, isRetry=false; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// Turn 1: Duplicate prompt (Retry 1)
	retries, isRetry = tracker.RecordTurn(sessionID, h1)
	if retries != 1 || !isRetry {
		t.Errorf("Expected Turn 1 to have retries=1, isRetry=true; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// Turn 2: Duplicate prompt again (Retry 2)
	retries, isRetry = tracker.RecordTurn(sessionID, h1)
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
	retries, isRetry = tracker.RecordTurn(sessionID, h2)
	if retries != 0 || isRetry {
		t.Errorf("Expected distinct prompt to reset retries=0, isRetry=false; got retries=%d, isRetry=%v", retries, isRetry)
	}
}

func TestSessionTracker_LazyEviction(t *testing.T) {
	ttl := 50 * time.Millisecond
	tracker := NewSessionTracker(ttl)

	sessionID := "sess-expiring"
	h := HashPrompt("same prompt")

	tracker.RecordTurn(sessionID, h)
	tracker.RecordTurn(sessionID, h) // retries = 1

	if tracker.GetRetries(sessionID) != 1 {
		t.Fatalf("Expected retries=1 before expiration")
	}

	// Wait for TTL to expire
	time.Sleep(70 * time.Millisecond)

	// Next turn with same prompt should treat it as fresh turn because TTL elapsed
	retries, isRetry := tracker.RecordTurn(sessionID, h)
	if retries != 0 || isRetry {
		t.Errorf("Expected expired session to reset to retries=0, isRetry=false; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// GetRetries after expiration for non-accessed session
	tracker.RecordTurn("sess-dead", h)
	time.Sleep(70 * time.Millisecond)
	if r := tracker.GetRetries("sess-dead"); r != 0 {
		t.Errorf("Expected GetRetries to return 0 for expired session, got %d", r)
	}
}

func TestSessionTracker_ResetAndEmptyKeys(t *testing.T) {
	tracker := NewSessionTracker(time.Minute)

	// Empty session key
	retries, isRetry := tracker.RecordTurn("", 12345)
	if retries != 0 || isRetry {
		t.Errorf("Expected empty session key to return 0, false")
	}
	if r := tracker.GetRetries(""); r != 0 {
		t.Errorf("Expected GetRetries for empty key to return 0, got %d", r)
	}

	sessionID := "sess-to-reset"
	h := HashPrompt("some prompt")
	tracker.RecordTurn(sessionID, h)
	tracker.RecordTurn(sessionID, h) // retries = 1

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
		tracker.RecordTurn(key, uint64(i+1))
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
				tracker.RecordTurn(sess, h)
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

	// Turn with 0 prompt hash
	st.RecordTurn("sess-zero", 0)
	retries, isRetry := st.RecordTurn("sess-zero", 0)
	if retries != 0 || isRetry {
		t.Errorf("Expected zero prompt hash not to trigger retry, got retries=%d, isRetry=%v", retries, isRetry)
	}
}
