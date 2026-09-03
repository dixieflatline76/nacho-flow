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

	// Empty session key and missing session key tests
	if tracker.RecordEscalation("") {
		t.Errorf("expected false for empty session key")
	}
	if tracker.RecordEscalation("non-existent-session-key") {
		t.Errorf("expected false for non-existent session key")
	}
	tracker.ResetEscalation("")
	tracker.ResetEscalation("non-existent-session-key")
}

func TestSessionTracker_KickstartDetection(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	threshold := 3

	// Register a session first
	tracker.RecordTurn("sess-kickstart", HashPrompt("test prompt"), false)

	// Turns without tool progress should increment kickstart count
	count, kickstarted := tracker.RecordKickstartState("sess-kickstart", false, threshold, 0)
	if count != 1 || kickstarted {
		t.Errorf("turn 1: expected count=1 kickstarted=false, got count=%d kickstarted=%v", count, kickstarted)
	}

	count, kickstarted = tracker.RecordKickstartState("sess-kickstart", false, threshold, 0)
	if count != 2 || kickstarted {
		t.Errorf("turn 2: expected count=2 kickstarted=false, got count=%d kickstarted=%v", count, kickstarted)
	}

	// Third turn hits the threshold -> Kickstart fires!
	count, kickstarted = tracker.RecordKickstartState("sess-kickstart", false, threshold, 0)
	if count != 3 || !kickstarted {
		t.Errorf("turn 3: expected count=3 kickstarted=true, got count=%d kickstarted=%v", count, kickstarted)
	}

	// Continues to indicate kickstart state
	count, kickstarted = tracker.RecordKickstartState("sess-kickstart", false, threshold, 0)
	if count != 4 || !kickstarted {
		t.Errorf("turn 4: expected count=4 kickstarted=true, got count=%d kickstarted=%v", count, kickstarted)
	}
}

func TestSessionTracker_KickstartResetOnProgress(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	threshold := 3

	tracker.RecordTurn("sess-reset", HashPrompt("prompt"), false)

	// Accumulate 2 idle turns
	tracker.RecordKickstartState("sess-reset", false, threshold, 0)
	tracker.RecordKickstartState("sess-reset", false, threshold, 0)

	// Tool progress resets the counter
	count, kickstarted := tracker.RecordKickstartState("sess-reset", true, threshold, 0)
	if count != 0 || kickstarted {
		t.Errorf("after progress: expected count=0 kickstarted=false, got count=%d kickstarted=%v", count, kickstarted)
	}

	// Kickstart count starts fresh
	count, kickstarted = tracker.RecordKickstartState("sess-reset", false, threshold, 0)
	if count != 1 || kickstarted {
		t.Errorf("after reset + no progress: expected count=1 kickstarted=false, got count=%d kickstarted=%v", count, kickstarted)
	}
}

func TestSessionTracker_KickstartDisabledWhenThresholdZero(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)

	tracker.RecordTurn("sess-disabled", HashPrompt("prompt"), false)

	// Threshold of 0 means disabled
	count, kickstarted := tracker.RecordKickstartState("sess-disabled", false, 0, 0)
	if count != 0 || kickstarted {
		t.Errorf("threshold 0: expected disabled (count=0 kickstarted=false), got count=%d kickstarted=%v", count, kickstarted)
	}

	// Empty session key should also be a no-op
	count, kickstarted = tracker.RecordKickstartState("", false, 3, 0)
	if count != 0 || kickstarted {
		t.Errorf("empty key: expected disabled, got count=%d kickstarted=%v", count, kickstarted)
	}
}

func TestSessionTracker_KickstartCircuitBreaker(t *testing.T) {
	// Circuit breaker: after 3 consecutive kickstart failures (wasKickstarted=true but no progress),
	// isKickstarted returns false to suppress the override and prevent death spirals.
	tracker := NewSessionTracker(5 * time.Minute)
	threshold := 2

	tracker.RecordTurn("sess-cb", HashPrompt("prompt"), false)

	// Build up to the threshold — kickstart fires
	tracker.RecordKickstartState("sess-cb", false, threshold, 0)                  // count=1, not kicked
	count, kicked := tracker.RecordKickstartState("sess-cb", false, threshold, 0) // count=2, kicked
	if count != 2 || !kicked {
		t.Fatalf("setup: expected count=2 kicked=true, got count=%d kicked=%v", count, kicked)
	}

	// Notify that last turn was already kickstarted but model still produced no progress
	tracker.RecordKickstartFailure("sess-cb") // failure 1
	count, kicked = tracker.RecordKickstartState("sess-cb", false, threshold, 0)
	if !kicked {
		t.Errorf("failure 1: kickstart should still fire (1/3 failures), got kicked=%v", kicked)
	}

	tracker.RecordKickstartFailure("sess-cb") // failure 2
	count, kicked = tracker.RecordKickstartState("sess-cb", false, threshold, 0)
	if !kicked {
		t.Errorf("failure 2: kickstart should still fire (2/3 failures), got kicked=%v", kicked)
	}

	tracker.RecordKickstartFailure("sess-cb") // failure 3 — circuit trips
	count, kicked = tracker.RecordKickstartState("sess-cb", false, threshold, 0)
	if kicked {
		t.Errorf("failure 3: circuit breaker should suppress kickstart after 3 failures, got kicked=%v count=%d", kicked, count)
	}
}

func TestSessionTracker_KickstartCircuitBreakerReset(t *testing.T) {
	// After circuit breaker engages, tool progress should reset both count and failures.
	tracker := NewSessionTracker(5 * time.Minute)
	threshold := 2

	tracker.RecordTurn("sess-cb-reset", HashPrompt("prompt"), false)

	// Trip the circuit breaker
	tracker.RecordKickstartState("sess-cb-reset", false, threshold, 0)
	tracker.RecordKickstartState("sess-cb-reset", false, threshold, 0)
	tracker.RecordKickstartFailure("sess-cb-reset")
	tracker.RecordKickstartFailure("sess-cb-reset")
	tracker.RecordKickstartFailure("sess-cb-reset")

	// Verify circuit is tripped
	_, kicked := tracker.RecordKickstartState("sess-cb-reset", false, threshold, 0)
	if kicked {
		t.Fatalf("setup: expected circuit breaker to suppress kickstart, got kicked=true")
	}

	// Now tool progress comes in — should reset everything
	count, kicked := tracker.RecordKickstartState("sess-cb-reset", true, threshold, 0)
	if count != 0 || kicked {
		t.Errorf("progress: expected count=0 kicked=false after reset, got count=%d kicked=%v", count, kicked)
	}

	// Verify kickstart works again after reset (failures cleared)
	tracker.RecordKickstartState("sess-cb-reset", false, threshold, 0)
	tracker.RecordKickstartFailure("sess-cb-reset")
	tracker.RecordKickstartFailure("sess-cb-reset")
	// Only 2 failures — circuit should NOT trip yet
	_, kickedAfterReset := tracker.RecordKickstartState("sess-cb-reset", false, threshold, 0)
	if !kickedAfterReset {
		t.Errorf("after reset: expected kickstart to fire normally after 2 failures (< 3), got kicked=false")
	}
}

func TestSessionTracker_GetKickstartCount(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)

	// Non-existent session
	if got := tracker.GetKickstartCount("no-such"); got != 0 {
		t.Errorf("expected 0 for non-existent session, got %d", got)
	}

	tracker.RecordTurn("sess-get", HashPrompt("p"), false)
	tracker.RecordKickstartState("sess-get", false, 5, 0)
	tracker.RecordKickstartState("sess-get", false, 5, 0)

	if got := tracker.GetKickstartCount("sess-get"); got != 2 {
		t.Errorf("expected kickstart count 2, got %d", got)
	}
}

func TestSessionTracker_RecordCycleKill_MinRetriesFloor(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	sessionKey := "sess-cycle-kill"

	// 1. Initial turn
	retries, isRetry := tracker.RecordTurn(sessionKey, HashPrompt("prompt 1"), false)
	if retries != 0 || isRetry {
		t.Fatalf("expected initial turn retries=0, got %d", retries)
	}

	// 2. Cycle Killer fires on this session
	tracker.RecordCycleKill(sessionKey, "deepseek/deepseek-v4-flash", 2*time.Minute)

	// 3. Client prunes context (distinct prompt hash), next turn arrives
	retries, isRetry = tracker.RecordTurn(sessionKey, HashPrompt("pruned prompt 2"), false)
	if retries != 3 || !isRetry {
		t.Errorf("expected floor to force retries=3, isRetry=true after context reset; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// 4. Next turn decays floor from 3 to 2
	retries, isRetry = tracker.RecordTurn(sessionKey, HashPrompt("pruned prompt 3"), false)
	if retries != 2 || !isRetry {
		t.Errorf("expected decayed floor retries=2; got retries=%d", retries)
	}

	// 5. Next turn decays floor from 2 to 1
	retries, isRetry = tracker.RecordTurn(sessionKey, HashPrompt("pruned prompt 4"), false)
	if retries != 1 || !isRetry {
		t.Errorf("expected decayed floor retries=1; got retries=%d", retries)
	}

	// 6. Next turn decays floor to 0 (normal distinct turn)
	retries, isRetry = tracker.RecordTurn(sessionKey, HashPrompt("pruned prompt 5"), false)
	if retries != 0 || isRetry {
		t.Errorf("expected decayed floor retries=0, isRetry=false; got retries=%d, isRetry=%v", retries, isRetry)
	}

	// 7. Verify floor never underflows negative
	retries, isRetry = tracker.RecordTurn(sessionKey, HashPrompt("pruned prompt 6"), false)
	if retries != 0 || isRetry {
		t.Errorf("expected floor to stay 0 without negative underflow; got retries=%d", retries)
	}
}

func TestSessionTracker_CoolingDownModels(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	sessionKey := "sess-cooldown"

	// 1. Initially no models cooling down
	if models := tracker.GetCoolingDownModels(sessionKey); len(models) != 0 {
		t.Errorf("expected empty cooling down models initially, got %v", models)
	}
	if tracker.IsModelCoolingDown(sessionKey, "deepseek/deepseek-v4-flash") {
		t.Errorf("expected false for IsModelCoolingDown initially")
	}

	// 2. Record cycle kill with short cooldown for testing
	cooldown := 50 * time.Millisecond
	tracker.RecordCycleKill(sessionKey, "deepseek/deepseek-v4-flash", cooldown)

	// Verify model is now in cooldown
	if !tracker.IsModelCoolingDown(sessionKey, "deepseek/deepseek-v4-flash") {
		t.Errorf("expected model to be cooling down")
	}
	models := tracker.GetCoolingDownModels(sessionKey)
	if len(models) != 1 || models[0] != "deepseek/deepseek-v4-flash" {
		t.Errorf("expected [deepseek/deepseek-v4-flash], got %v", models)
	}

	// Unrelated model is not cooling down
	if tracker.IsModelCoolingDown(sessionKey, "google/gemini-3.1-pro-preview") {
		t.Errorf("expected unrelated model to not be cooling down")
	}

	// Unrelated session is isolated
	if tracker.IsModelCoolingDown("sess-other", "deepseek/deepseek-v4-flash") {
		t.Errorf("expected distinct session to not have cooldown")
	}

	// 3. Wait for cooldown to expire
	time.Sleep(70 * time.Millisecond)

	if tracker.IsModelCoolingDown(sessionKey, "deepseek/deepseek-v4-flash") {
		t.Errorf("expected model cooldown to have expired")
	}
	if models := tracker.GetCoolingDownModels(sessionKey); len(models) != 0 {
		t.Errorf("expected empty models list after expiration, got %v", models)
	}
}

// --- Fairy Dust Tests ---

func TestFairyDust_TriggersAtFrequency(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	sessionKey := "sess-fairy-1"
	tracker.RecordTurn(sessionKey, HashPrompt("prompt"), true)

	const freq = 5
	const maxPS = 3
	entryName := "Tactical Code Review"

	for i := 1; i < freq; i++ {
		tracker.RecordWriteProgress(sessionKey, true)
		_, triggered := tracker.CheckFairyDust(sessionKey, entryName, freq, maxPS)
		if triggered {
			t.Errorf("write turn %d: expected no trigger before frequency boundary", i)
		}
	}
	wc := tracker.RecordWriteProgress(sessionKey, true)
	count, triggered := tracker.CheckFairyDust(sessionKey, entryName, freq, maxPS)
	if !triggered {
		t.Errorf("expected trigger at write turn %d, got none", wc)
	}
	if count != 1 {
		t.Errorf("expected invocation count=1, got %d", count)
	}
	for i := 6; i < freq*2; i++ {
		tracker.RecordWriteProgress(sessionKey, true)
		_, trig := tracker.CheckFairyDust(sessionKey, entryName, freq, maxPS)
		if trig {
			t.Errorf("write turn %d: unexpected trigger between frequency windows", i)
		}
	}
	tracker.RecordWriteProgress(sessionKey, true)
	count2, triggered2 := tracker.CheckFairyDust(sessionKey, entryName, freq, maxPS)
	if !triggered2 {
		t.Errorf("expected second trigger at write turn 10")
	}
	if count2 != 2 {
		t.Errorf("expected invocation count=2, got %d", count2)
	}
}

func TestFairyDust_MultipleEntries_IndependentTracking(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	sessionKey := "sess-fairy-multi"
	tracker.RecordTurn(sessionKey, HashPrompt("prompt"), true)

	tactical := "Tactical Code Review"
	strategic := "Strategic Architecture Review"

	for i := 1; i <= 10; i++ {
		tracker.RecordWriteProgress(sessionKey, true)
		_, tTriggered := tracker.CheckFairyDust(sessionKey, tactical, 5, 10)
		_, sTriggered := tracker.CheckFairyDust(sessionKey, strategic, 8, 10)
		expectTactical := (i == 5 || i == 10)
		if tTriggered != expectTactical {
			t.Errorf("write turn %d: tactical triggered=%v, want=%v", i, tTriggered, expectTactical)
		}
		expectStrategic := (i == 8)
		if sTriggered != expectStrategic {
			t.Errorf("write turn %d: strategic triggered=%v, want=%v", i, sTriggered, expectStrategic)
		}
	}
}

func TestFairyDust_RespectsPerEntryMaxCap(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	sessionKey := "sess-fairy-cap"
	tracker.RecordTurn(sessionKey, HashPrompt("p"), true)

	entryA := "Entry A"
	entryB := "Entry B"
	const freq = 2
	const maxA = 2

	aCount := 0
	bCount := 0
	for i := 1; i <= 10; i++ {
		tracker.RecordWriteProgress(sessionKey, true)
		if i%freq == 0 {
			_, aTriggered := tracker.CheckFairyDust(sessionKey, entryA, freq, maxA)
			_, bTriggered := tracker.CheckFairyDust(sessionKey, entryB, freq, 100)
			if aTriggered {
				aCount++
			}
			if bTriggered {
				bCount++
			}
		}
	}
	if aCount != maxA {
		t.Errorf("expected entry A to trigger exactly %d times (cap), got %d", maxA, aCount)
	}
	if bCount != 5 {
		t.Errorf("expected entry B to trigger 5 times (uncapped), got %d", bCount)
	}
}

func TestFairyDust_IgnoresReadOnlyTurns(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	sessionKey := "sess-fairy-readonly"
	tracker.RecordTurn(sessionKey, HashPrompt("p"), true)

	for i := 0; i < 10; i++ {
		wc := tracker.RecordWriteProgress(sessionKey, false)
		if wc != 0 {
			t.Errorf("read-only turn %d: expected WriteProgressCount=0, got %d", i, wc)
		}
		_, triggered := tracker.CheckFairyDust(sessionKey, "entry", 1, 10)
		if triggered {
			t.Errorf("read-only turn %d: unexpected trigger", i)
		}
	}
}

func TestFairyDust_ResetsOnSessionExpiry(t *testing.T) {
	ttl := 50 * time.Millisecond
	tracker := NewSessionTracker(ttl)
	sessionKey := "sess-fairy-expiry"
	tracker.RecordTurn(sessionKey, HashPrompt("p"), true)

	for i := 0; i < 5; i++ {
		tracker.RecordWriteProgress(sessionKey, true)
	}
	_, triggered := tracker.CheckFairyDust(sessionKey, "entry", 5, 10)
	if !triggered {
		t.Fatal("expected fairy dust to trigger at write turn 5")
	}

	time.Sleep(ttl + 20*time.Millisecond)
	tracker.RecordTurn(sessionKey, HashPrompt("new-prompt"), false)

	wc := tracker.RecordWriteProgress(sessionKey, false)
	if wc != 0 {
		t.Errorf("expected WriteProgressCount=0 after session expiry reset, got %d", wc)
	}
}

func TestFairyDust_CollisionHighestPriorityWins(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	sessionKey := "sess-fairy-collision"
	tracker.RecordTurn(sessionKey, HashPrompt("p"), true)

	sonnet := "Tactical Code Review"
	opus := "Strategic Architecture Review"

	for i := 0; i < 4; i++ {
		tracker.RecordWriteProgress(sessionKey, true)
	}

	_, sonnetTriggered := tracker.CheckFairyDust(sessionKey, sonnet, 4, 10)
	_, opusTriggered := tracker.CheckFairyDust(sessionKey, opus, 4, 10)

	if !sonnetTriggered {
		t.Error("expected sonnet to trigger at write turn 4")
	}
	if !opusTriggered {
		t.Error("expected opus to trigger at write turn 4")
	}

	type candidate struct {
		name     string
		priority int
	}
	candidates := []candidate{{sonnet, 10}, {opus, 100}}
	var winner candidate
	for _, c := range candidates {
		if winner.name == "" || c.priority > winner.priority {
			winner = c
		}
	}
	if winner.name != opus {
		t.Errorf("expected opus (priority 100) to win collision, got %q", winner.name)
	}
}

func TestSessionTracker_GuardrailsAndReset(t *testing.T) {
	tracker := NewSessionTracker(5 * time.Minute)
	key := "sess-guardrails-test"

	// 1. Non-existent and empty key return zero-value
	if g := tracker.GetGuardrails(""); g != (SessionGuardrails{}) {
		t.Errorf("expected zero-value for empty session key, got %+v", g)
	}
	if g := tracker.GetGuardrails("non-existent"); g != (SessionGuardrails{}) {
		t.Errorf("expected zero-value for non-existent session key, got %+v", g)
	}

	// 2. Setting on non-existent creates and tracks guardrails safely
	tracker.SetKickstartDisabled("pre-init", true)
	if !tracker.GetGuardrails("pre-init").KickstartDisabled {
		t.Error("expected KickstartDisabled=true for pre-initialized session")
	}

	// 3. Create session
	tracker.RecordTurn(key, HashPrompt("prompt 1"), false)
	gInit := tracker.GetGuardrails(key)
	if gInit.KickstartDisabled || gInit.CycleKillerDisabled || gInit.ShieldDisabled || gInit.RawModeEnabled || gInit.FairyDustDisabled {
		t.Errorf("expected all guardrails default to false, got %+v", gInit)
	}

	// 4. Toggle each guardrail
	tracker.SetKickstartDisabled(key, true)
	tracker.SetCycleKillerDisabled(key, true)
	tracker.SetShieldDisabled(key, true)
	tracker.SetRawModeEnabled(key, true)
	tracker.SetFairyDustDisabled(key, true)

	gActive := tracker.GetGuardrails(key)
	if !gActive.KickstartDisabled || !gActive.CycleKillerDisabled || !gActive.ShieldDisabled || !gActive.RawModeEnabled || !gActive.FairyDustDisabled {
		t.Errorf("expected all guardrails active, got %+v", gActive)
	}

	// 5. Untoggle individual guardrail
	tracker.SetKickstartDisabled(key, false)
	if tracker.GetGuardrails(key).KickstartDisabled {
		t.Error("expected KickstartDisabled to be false after unsetting")
	}

	// 6. Reset session
	tracker.RecordKickstartState(key, false, 5, 0)
	tracker.RecordCycleKill(key, "test-model", 2*time.Minute)
	tracker.RecordWriteProgress(key, true)

	tracker.ResetSession(key)

	gReset := tracker.GetGuardrails(key)
	if gReset != (SessionGuardrails{}) {
		t.Errorf("expected zero-value guardrails after ResetSession, got %+v", gReset)
	}
	if kc := tracker.GetKickstartCount(key); kc != 0 {
		t.Errorf("expected kickstart count 0 after reset, got %d", kc)
	}
	if cd := tracker.GetCoolingDownModels(key); len(cd) != 0 {
		t.Errorf("expected cooling down models empty after reset, got %+v", cd)
	}
	if wp := tracker.RecordWriteProgress(key, false); wp != 0 {
		t.Errorf("expected write progress 0 after reset, got %d", wp)
	}

	// 7. ResetSession and Set* methods on empty or non-existent does not panic
	tracker.ResetSession("")
	tracker.ResetSession("no-such-key")

	tracker.SetKickstartDisabled("", true)
	tracker.SetCycleKillerDisabled("", true)
	tracker.SetCycleKillerDisabled("non-existent", true)
	tracker.SetShieldDisabled("", true)
	tracker.SetShieldDisabled("non-existent", true)
	tracker.SetRawModeEnabled("", true)
	tracker.SetRawModeEnabled("non-existent", true)
	tracker.SetFairyDustDisabled("", true)
	tracker.SetFairyDustDisabled("non-existent", true)
}
