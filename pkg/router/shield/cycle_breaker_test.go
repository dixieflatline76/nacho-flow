package shield

import (
	"fmt"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestCycleBreaker_Disabled(t *testing.T) {
	disabled := false
	prompt := "Custom override"
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:          &disabled,
		CorrectionPrompt: prompt,
		MaxRetries:       2,
	})

	if cb.IsEnabled() {
		t.Errorf("expected IsEnabled to return false")
	}
	if cb.CorrectionPrompt() != prompt {
		t.Errorf("expected CorrectionPrompt %q, got %q", prompt, cb.CorrectionPrompt())
	}
	if cb.MaxRetries() != 2 {
		t.Errorf("expected MaxRetries 2, got %d", cb.MaxRetries())
	}

	// Process infinite repetition while disabled
	for i := 0; i < 20; i++ {
		triggered, reason := cb.ProcessDelta("Let's do this! Checking now! Proceeding! Actually, wait... ", false)
		if triggered {
			t.Fatalf("expected cycle breaker to not trigger when disabled, got reason: %s", reason)
		}
	}
}

func TestCycleBreaker_NgramRepetitionDetection(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      800,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	phrase := "Let's do this! Checking now! Proceeding! Actually wait... "

	triggered, reason := cb.ProcessDelta(phrase, false)
	if triggered {
		t.Fatalf("first repetition should not trigger, got %s", reason)
	}

	triggered, reason = cb.ProcessDelta(phrase, false)
	if triggered {
		t.Fatalf("second repetition should not trigger, got %s", reason)
	}

	// Third repetition should trigger instantly!
	triggered, reason = cb.ProcessDelta(phrase, false)
	if !triggered {
		t.Fatalf("expected cycle breaker to trigger on 3rd repetition")
	}
	if reason != "ngram_repetition_loop_detected" {
		t.Fatalf("expected ngram_repetition_loop_detected, got %s", reason)
	}
}

func TestCycleBreaker_ProseTokenCeiling(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      50,
		RepetitionWindow:    6,
		RepetitionThreshold: 5, // High threshold so ngram loop doesn't fire first
	})

	// Send unique words - should NOT trigger because maxNgramFreq stays 0
	for i := 0; i < 100; i++ {
		word := fmt.Sprintf("uniqueWord%d ", i)
		triggered, reason := cb.ProcessDelta(word, false)
		if triggered {
			t.Fatalf("unique words should never trigger cooperative budget check, got reason: %s at word %d", reason, i)
		}
	}

	if cb.ProseTokens() <= 50 {
		t.Fatalf("expected ProseTokens() > 50 (budget ceiling), got %d", cb.ProseTokens())
	}
	if cb.MaxNgramFreq() > 1 {
		t.Fatalf("expected MaxNgramFreq() <= 1 for all unique words, got %d", cb.MaxNgramFreq())
	}
}

func TestCycleBreaker_CooperativeProseBudget(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      200,
		RepetitionWindow:    6,
		RepetitionThreshold: 100, // Set very high so ngram loop never fires
	})

	// Send semi-repetitive content: same phrase interspersed with unique words
	// This will build up N-gram frequency >= 2 but below repetition threshold
	var triggered bool
	var reason string
	for round := 0; round < 50; round++ {
		cb.ProcessDelta(fmt.Sprintf("unique prefix number %d ", round), false)
		triggered, reason = cb.ProcessDelta("the quick brown fox jumps over ", false)
		if triggered {
			break
		}
	}

	if !triggered {
		t.Fatalf("expected cooperative budget to trigger on semi-repetitive content exceeding budget")
	}
	if reason != "prose_budget_exceeded_with_repetition" {
		t.Fatalf("expected prose_budget_exceeded_with_repetition, got %s", reason)
	}
}

func TestCycleBreaker_ThinkingTokenBudget(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           60,
		ThinkingRepetitionThreshold: 10, // High threshold so loop doesn't fire first
	})

	// Send unique words - should NOT trigger cooperative budget
	for i := 0; i < 100; i++ {
		word := fmt.Sprintf("thinkingStepNumber%d ", i)
		triggered, reason := cb.ProcessDelta(word, true)
		if triggered {
			t.Fatalf("unique thinking words should never trigger cooperative budget, got reason: %s at word %d", reason, i)
		}
	}

	if cb.ThinkingTokens() <= 60 {
		t.Fatalf("expected ThinkingTokens() > 60 (budget ceiling), got %d", cb.ThinkingTokens())
	}
	if cb.MaxThinkingNgramFreq() > 1 {
		t.Fatalf("expected MaxThinkingNgramFreq() <= 1 for all unique thinking words, got %d", cb.MaxThinkingNgramFreq())
	}
	if cb.ProseTokens() != 0 {
		t.Fatalf("expected ProseTokens() == 0 during thinking, got %d", cb.ProseTokens())
	}
}

func TestCycleBreaker_CooperativeThinkingBudget(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           200,
		RepetitionWindow:            6,
		ThinkingRepetitionThreshold: 100, // Set very high so loop never fires
	})

	var triggered bool
	var reason string
	for round := 0; round < 50; round++ {
		cb.ProcessDelta(fmt.Sprintf("unique thinking step %d ", round), true)
		triggered, reason = cb.ProcessDelta("let me reconsider this approach carefully ", true)
		if triggered {
			break
		}
	}

	if !triggered {
		t.Fatalf("expected cooperative thinking budget to trigger on semi-repetitive content exceeding budget")
	}
	if reason != "thinking_budget_exceeded_with_repetition" {
		t.Fatalf("expected thinking_budget_exceeded_with_repetition, got %s", reason)
	}
}

func TestCycleBreaker_ThinkingRepetitionLoop(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           5000,
		RepetitionWindow:            6,
		ThinkingRepetitionThreshold: 5,
	})

	phrase := "Wait let me check the types again right now. "

	// First 4 repetitions should NOT trigger (threshold is 5)
	for i := 1; i <= 4; i++ {
		triggered, reason := cb.ProcessDelta(phrase, true)
		if triggered {
			t.Fatalf("repetition %d should not trigger, got %s", i, reason)
		}
	}

	// 5th repetition should trigger!
	triggered, reason := cb.ProcessDelta(phrase, true)
	if !triggered {
		t.Fatalf("expected cycle breaker to trigger on 5th thinking repetition")
	}
	if reason != "thinking_repetition_loop_detected" {
		t.Fatalf("expected thinking_repetition_loop_detected, got %s", reason)
	}
}

func TestCycleBreaker_DualLaneIsolation(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxProseTokens:              1000,
		MaxThinkingTokens:           1000,
		RepetitionWindow:            6,
		RepetitionThreshold:         3,
		ThinkingRepetitionThreshold: 5,
	})

	phrase := "Let us explore all the possibilities thoroughly. "

	// Emit 4x in thinking mode (under 5x threshold)
	for i := 0; i < 4; i++ {
		triggered, reason := cb.ProcessDelta(phrase, true)
		if triggered {
			t.Fatalf("thinking repetition should not trigger at 4x, got %s", reason)
		}
	}

	// Emit 2x in prose mode (under 3x threshold)
	for i := 0; i < 2; i++ {
		triggered, reason := cb.ProcessDelta(phrase, false)
		if triggered {
			t.Fatalf("prose repetition should not trigger at 2x, got %s", reason)
		}
	}

	// Neither lane triggered because their N-gram tables are completely isolated!
	if cb.ThinkingTokens() == 0 || cb.ProseTokens() == 0 {
		t.Fatalf("expected non-zero token counts in both lanes, got thinking=%d, prose=%d",
			cb.ThinkingTokens(), cb.ProseTokens())
	}
}

func TestCycleBreaker_ResetClearsBothLanes(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxProseTokens:              50,
		MaxThinkingTokens:           50,
		RepetitionWindow:            4,
		RepetitionThreshold:         3,
		ThinkingRepetitionThreshold: 3,
	})

	// Build up repeated N-grams in both lanes
	cb.ProcessDelta("repeat this word phrase repeat this word phrase ", false)
	cb.ProcessDelta("thinking about this word phrase thinking about this word phrase ", true)

	if cb.ProseTokens() == 0 || cb.ThinkingTokens() == 0 {
		t.Fatalf("expected non-zero counts before reset")
	}
	if cb.MaxNgramFreq() < 2 {
		t.Fatalf("expected MaxNgramFreq() >= 2 before reset, got %d", cb.MaxNgramFreq())
	}
	if cb.MaxThinkingNgramFreq() < 2 {
		t.Fatalf("expected MaxThinkingNgramFreq() >= 2 before reset, got %d", cb.MaxThinkingNgramFreq())
	}

	cb.Reset()

	if cb.ProseTokens() != 0 {
		t.Fatalf("expected 0 prose tokens after reset, got %d", cb.ProseTokens())
	}
	if cb.ThinkingTokens() != 0 {
		t.Fatalf("expected 0 thinking tokens after reset, got %d", cb.ThinkingTokens())
	}
	if cb.MaxNgramFreq() != 0 {
		t.Fatalf("expected 0 MaxNgramFreq after reset, got %d", cb.MaxNgramFreq())
	}
	if cb.MaxThinkingNgramFreq() != 0 {
		t.Fatalf("expected 0 MaxThinkingNgramFreq after reset, got %d", cb.MaxThinkingNgramFreq())
	}

	// Verify post-reset: sending unique tokens exceeding budget (50 tokens) does NOT trigger,
	// confirming that stale maxNgramFreq was cleanly erased.
	for i := 0; i < 60; i++ {
		triggered, reason := cb.ProcessDelta(fmt.Sprintf("postResetUniqueWord%d ", i), false)
		if triggered {
			t.Fatalf("should not trigger after reset on unique words exceeding budget, got %s", reason)
		}
	}
}
