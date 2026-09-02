package ui

import (
	"blackjack/pkg/cards"
	"blackjack/pkg/engine"
	"blackjack/pkg/oracle"
	"blackjack/pkg/rules"
	"strings"
	"testing"
)

func TestRenderCard(t *testing.T) {
	renderer := NewCardRenderer(false) // No colors to make testing output easier

	card := cards.Card{Rank: cards.Ace, Suit: cards.Spades}
	lines := renderer.RenderCard(card, false)

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	expectedLine1 := "│ A       │"
	if lines[1] != expectedLine1 {
		t.Errorf("expected line 1 to be %q, got %q", expectedLine1, lines[1])
	}

	hiddenLines := renderer.RenderCard(card, true)
	if len(hiddenLines) != 5 {
		t.Fatalf("expected 5 hidden lines, got %d", len(hiddenLines))
	}
	if hiddenLines[1] != "│░░░░░░░░░│" {
		t.Errorf("expected hidden line 1 to be '│░░░░░░░░░│', got %q", hiddenLines[1])
	}
}

func TestRenderCard_WithColors(t *testing.T) {
	renderer := NewCardRenderer(true)
	card := cards.Card{Rank: cards.Ten, Suit: cards.Hearts}
	lines := renderer.RenderCard(card, false)

	// ♥ should be colored red
	if !strings.Contains(lines[2], Red) {
		t.Errorf("expected red color for hearts, line: %q", lines[2])
	}
}

func TestRenderHandMultiLine(t *testing.T) {
	renderer := NewCardRenderer(false)

	t.Run("EmptyHand", func(t *testing.T) {
		res := renderer.RenderHandMultiLine([]cards.Card{}, false)
		if res != "[Empty Hand]" {
			t.Errorf("expected '[Empty Hand]', got %q", res)
		}
	})

	t.Run("TwoCards", func(t *testing.T) {
		hand := []cards.Card{
			{Rank: cards.Ace, Suit: cards.Spades},
			{Rank: cards.Ten, Suit: cards.Hearts},
		}
		res := renderer.RenderHandMultiLine(hand, false)
		if !strings.Contains(res, "A") || !strings.Contains(res, "10") {
			t.Errorf("output missing card ranks, got:\n%s", res)
		}
		lines := strings.Split(res, "\n")
		// 5 lines + 1 empty line at the end
		if len(lines) != 6 {
			t.Errorf("expected 6 lines in multiline render, got %d", len(lines))
		}
	})

	t.Run("HideHoleCard", func(t *testing.T) {
		hand := []cards.Card{
			{Rank: cards.Ace, Suit: cards.Spades},
			{Rank: cards.Ten, Suit: cards.Hearts},
		}
		res := renderer.RenderHandMultiLine(hand, true)
		if strings.Contains(res, "10") {
			t.Errorf("output should not contain hidden card rank, got:\n%s", res)
		}
		if !strings.Contains(res, "░░░") {
			t.Errorf("output missing hidden card pattern, got:\n%s", res)
		}
	})
}

func TestFormatOracleHUD(t *testing.T) {
	counter := oracle.NewCardCounter(oracle.HiLo, 1)
	advisor := oracle.NewStrategyAdvisor(rules.VegasStrip())

	hand := &engine.Hand{
		Cards: []cards.Card{
			{Rank: cards.Ten, Suit: cards.Spades},
			{Rank: cards.Six, Suit: cards.Hearts},
		},
	}
	dealerUpcard := cards.Card{Rank: cards.Ten, Suit: cards.Diamonds}

	res := FormatOracleHUD(counter, advisor, hand, dealerUpcard)
	if !strings.Contains(res, "Running Count (RC):") {
		t.Errorf("missing RC in HUD:\n%s", res)
	}
	if !strings.Contains(res, "Advisor Recommendation:") {
		t.Errorf("missing Advisor Recommendation in HUD:\n%s", res)
	}
	// VegasStrip: 16 (10+6) vs Dealer 10 is late surrender.
	if !strings.Contains(res, "Surrender") {
		t.Errorf("expected VegasStrip 16 vs 10 recommendation to be Surrender, got:\n%s", res)
	}
}

func TestFormatTableState(t *testing.T) {
	renderer := NewCardRenderer(false)
	shoe := cards.NewShoe(1)
	game := engine.NewGameEngine(rules.VegasStrip(), shoe)

	_ = game.StartRound(10.0)
	counter := oracle.NewCardCounter(oracle.HiLo, 1)
	advisor := oracle.NewStrategyAdvisor(game.Rules)

	res := FormatTableState(renderer, game, counter, advisor, true)
	if !strings.Contains(res, "DEALER HAND") {
		t.Errorf("missing dealer hand section:\n%s", res)
	}
	if !strings.Contains(res, "PLAYER HANDS") {
		t.Errorf("missing player hand section:\n%s", res)
	}
	if !strings.Contains(res, "ORACLE HUD") {
		t.Errorf("missing oracle HUD section:\n%s", res)
	}
}

func TestFormatOracleHUD_Actions(t *testing.T) {
	counter := oracle.NewCardCounter(oracle.HiLo, 1)
	advisor := oracle.NewStrategyAdvisor(rules.VegasStrip())

	// Test soft and split actions in HUD
	handSplit := &engine.Hand{
		Cards: []cards.Card{
			{Rank: cards.Eight, Suit: cards.Spades},
			{Rank: cards.Eight, Suit: cards.Hearts},
		},
	}
	hudSplit := FormatOracleHUD(counter, advisor, handSplit, cards.Card{Rank: cards.Six, Suit: cards.Diamonds})
	if !strings.Contains(hudSplit, "Split") {
		t.Errorf("expected Split action in HUD, got:\n%s", hudSplit)
	}

	handDouble := &engine.Hand{
		Cards: []cards.Card{
			{Rank: cards.Five, Suit: cards.Spades},
			{Rank: cards.Six, Suit: cards.Hearts},
		},
	}
	hudDouble := FormatOracleHUD(counter, advisor, handDouble, cards.Card{Rank: cards.Five, Suit: cards.Diamonds})
	if !strings.Contains(hudDouble, "Double") {
		t.Errorf("expected Double action in HUD, got:\n%s", hudDouble)
	}

	handStand := &engine.Hand{
		Cards: []cards.Card{
			{Rank: cards.Ten, Suit: cards.Spades},
			{Rank: cards.Ten, Suit: cards.Hearts},
		},
	}
	hudStand := FormatOracleHUD(counter, advisor, handStand, cards.Card{Rank: cards.Six, Suit: cards.Diamonds})
	if !strings.Contains(hudStand, "Stand") {
		t.Errorf("expected Stand action in HUD, got:\n%s", hudStand)
	}
}
