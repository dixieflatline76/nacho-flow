package shield

import (
	"fmt"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestCycleBreaker_Disabled(t *testing.T) {
	disabled := false
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled: &disabled,
	})

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
		RepetitionThreshold: 5,
	})

	var triggered bool
	var reason string
	for i := 0; i < 100; i++ {
		word := fmt.Sprintf("uniqueWord%d ", i)
		triggered, reason = cb.ProcessDelta(word, false)
		if triggered {
			break
		}
	}

	if !triggered {
		t.Fatalf("expected cycle breaker to trigger on prose token ceiling")
	}
	if reason != "prose_token_budget_exceeded" {
		t.Fatalf("expected prose_token_budget_exceeded, got %s", reason)
	}
}

func TestCycleBreaker_ThinkingTokenBudget(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           60,
		ThinkingRepetitionThreshold: 10,
	})

	var triggered bool
	var reason string
	for i := 0; i < 100; i++ {
		word := fmt.Sprintf("thinkingStepNumber%d ", i)
		triggered, reason = cb.ProcessDelta(word, true)
		if triggered {
			break
		}
	}

	if !triggered {
		t.Fatalf("expected cycle breaker to trigger on thinking token ceiling")
	}
	if reason != "thinking_budget_exceeded" {
		t.Fatalf("expected thinking_budget_exceeded, got %s", reason)
	}
	if cb.ThinkingTokens() == 0 {
		t.Fatalf("expected ThinkingTokens() > 0, got %d", cb.ThinkingTokens())
	}
	if cb.ProseTokens() != 0 {
		t.Fatalf("expected ProseTokens() == 0 during thinking, got %d", cb.ProseTokens())
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
		MaxProseTokens:              100,
		MaxThinkingTokens:           100,
		RepetitionWindow:            4,
		RepetitionThreshold:         2,
		ThinkingRepetitionThreshold: 2,
	})

	cb.ProcessDelta("repeat this word phrase repeat this word phrase ", false)
	cb.ProcessDelta("thinking about this word phrase right now ", true)

	if cb.ProseTokens() == 0 || cb.ThinkingTokens() == 0 {
		t.Fatalf("expected non-zero counts before reset")
	}

	cb.Reset()

	if cb.ProseTokens() != 0 {
		t.Fatalf("expected 0 prose tokens after reset, got %d", cb.ProseTokens())
	}
	if cb.ThinkingTokens() != 0 {
		t.Fatalf("expected 0 thinking tokens after reset, got %d", cb.ThinkingTokens())
	}

	// Should not trigger on single phrase after reset
	triggered, _ := cb.ProcessDelta("repeat this word phrase ", false)
	if triggered {
		t.Fatalf("should not trigger on first prose phrase after reset")
	}
	triggered, _ = cb.ProcessDelta("thinking about this word phrase ", true)
	if triggered {
		t.Fatalf("should not trigger on first thinking phrase after reset")
	}
}
