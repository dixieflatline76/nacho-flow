package deck_test

import (
	"math/rand"
	"testing"

	"blackjack/internal/deck"
)

func TestSuitAndRankStrings(t *testing.T) {
	suits := []deck.Suit{deck.Spades, deck.Hearts, deck.Diamonds, deck.Clubs, deck.Suit(99)}
	expectedSuits := []string{"♠", "♥", "♦", "♣", "?"}
	for i, s := range suits {
		if s.String() != expectedSuits[i] {
			t.Errorf("expected suit %s, got %s", expectedSuits[i], s.String())
		}
	}

	ranks := []deck.Rank{
		deck.Two, deck.Three, deck.Four, deck.Five, deck.Six,
		deck.Seven, deck.Eight, deck.Nine, deck.Ten,
		deck.Jack, deck.Queen, deck.King, deck.Ace, deck.Rank(99),
	}
	expectedRanks := []string{
		"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A", "?",
	}
	for i, r := range ranks {
		if r.String() != expectedRanks[i] {
			t.Errorf("expected rank %s, got %s", expectedRanks[i], r.String())
		}
	}
}

func TestCardCreationAndValues(t *testing.T) {
	c1 := deck.NewCard(deck.Hearts, deck.Ace)
	if c1.Value != 11 {
		t.Errorf("expected Ace value to be 11, got %d", c1.Value)
	}
	if c1.String() != "A♥" {
		t.Errorf("expected string 'A♥', got %s", c1.String())
	}

	c2 := deck.NewCard(deck.Spades, deck.King)
	if c2.Value != 10 {
		t.Errorf("expected King value to be 10, got %d", c2.Value)
	}
	if c2.String() != "K♠" {
		t.Errorf("expected string 'K♠', got %s", c2.String())
	}

	c3 := deck.NewCard(deck.Diamonds, deck.Seven)
	if c3.Value != 7 {
		t.Errorf("expected 7 value to be 7, got %d", c3.Value)
	}

	c4 := deck.NewCard(deck.Clubs, deck.Rank(99))
	if c4.Value != 0 {
		t.Errorf("expected unknown rank value 0, got %d", c4.Value)
	}
}

func TestNewShoeValidation(t *testing.T) {
	if _, err := deck.NewShoe(0, 0.75); err == nil {
		t.Errorf("expected error for 0 decks")
	}
	if _, err := deck.NewShoe(9, 0.75); err == nil {
		t.Errorf("expected error for 9 decks")
	}
	if _, err := deck.NewShoeWithRNG(0, 0.75, nil); err == nil {
		t.Errorf("expected error for 0 decks with RNG")
	}

	shoe, err := deck.NewShoe(6, 0.75)
	if err != nil {
		t.Fatalf("unexpected error creating 6-deck shoe: %v", err)
	}
	if shoe.TotalCards() != 6*52 {
		t.Errorf("expected %d cards, got %d", 6*52, shoe.TotalCards())
	}
	if shoe.Remaining() != 6*52 {
		t.Errorf("expected %d remaining cards, got %d", 6*52, shoe.Remaining())
	}
	if shoe.CutCardIndex() != int(float64(6*52)*0.75) {
		t.Errorf("cut card index mismatch: %d", shoe.CutCardIndex())
	}
}

func TestShoeDefaultPenetration(t *testing.T) {
	shoe, err := deck.NewShoe(2, -0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedCut := int(float64(2*52) * 0.75)
	if shoe.CutCardIndex() != expectedCut {
		t.Errorf("expected default penetration 0.75 cut index %d, got %d", expectedCut, shoe.CutCardIndex())
	}
}

func TestShoeDrawAndPenetrationTrigger(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	shoe, err := deck.NewShoeWithRNG(1, 0.5, rng)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 deck = 52 cards, cut card at index 26
	if shoe.NeedsReshuffle() {
		t.Errorf("shoe should not need reshuffle initially")
	}

	for i := 0; i < 25; i++ {
		_, err := shoe.Draw()
		if err != nil {
			t.Fatalf("unexpected draw error at %d: %v", i, err)
		}
		if shoe.NeedsReshuffle() {
			t.Errorf("reshuffle triggered prematurely at card %d", i)
		}
	}

	// 26th card should reach cut card index
	_, err = shoe.Draw()
	if err != nil {
		t.Fatalf("unexpected draw error: %v", err)
	}
	if !shoe.NeedsReshuffle() {
		t.Errorf("expected NeedsReshuffle to be true after drawing 26th card")
	}

	// Draw all remaining cards
	for i := 26; i < 52; i++ {
		_, err := shoe.Draw()
		if err != nil {
			t.Fatalf("failed drawing card %d: %v", i, err)
		}
	}

	// Shoe should now be empty
	_, err = shoe.Draw()
	if err != deck.ErrShoeEmpty {
		t.Errorf("expected ErrShoeEmpty, got %v", err)
	}

	// Test Reset
	shoe.Reset()
	if shoe.Remaining() != 52 {
		t.Errorf("expected 52 cards after reset, got %d", shoe.Remaining())
	}
	if shoe.NeedsReshuffle() {
		t.Errorf("expected NeedsReshuffle false after reset")
	}
}

func TestFisherYatesShuffleDistribution(t *testing.T) {
	// Verify that shuffling alters card order compared to sequential initialization
	shoe, _ := deck.NewShoe(1, 0.75)
	// Check that not all cards are in initial rank order
	sameOrder := true
	for i := 0; i < 5; i++ {
		c1, _ := shoe.Draw()
		c2, _ := shoe.Draw()
		if c1.Rank != deck.Two || c2.Rank != deck.Three {
			sameOrder = false
			break
		}
	}
	if sameOrder {
		t.Errorf("cards appear to be ordered sequentially without proper shuffle")
	}
}
