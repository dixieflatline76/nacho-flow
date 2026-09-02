package cards

import (
	"math/rand"
	"testing"
)

func TestCard_StringAndValues(t *testing.T) {
	tests := []struct {
		card     Card
		expected string
		bjVal    int
		isTen    bool
		isAce    bool
	}{
		{NewCard(Ace, Spades), "A♠", 11, false, true},
		{NewCard(King, Hearts), "K♥", 10, true, false},
		{NewCard(Queen, Diamonds), "Q♦", 10, true, false},
		{NewCard(Jack, Clubs), "J♣", 10, true, false},
		{NewCard(Ten, Spades), "10♠", 10, true, false},
		{NewCard(Nine, Hearts), "9♥", 9, false, false},
		{NewCard(Two, Diamonds), "2♦", 2, false, false},
		{NewCard(Rank(99), Suit(99)), "??", 0, false, false},
	}

	for _, tt := range tests {
		if got := tt.card.String(); got != tt.expected {
			t.Errorf("Card.String() = %v, want %v", got, tt.expected)
		}
		if got := tt.card.Rank.BlackjackValue(); got != tt.bjVal {
			t.Errorf("Rank.BlackjackValue() = %v, want %v", got, tt.bjVal)
		}
		if got := tt.card.IsTenValue(); got != tt.isTen {
			t.Errorf("Card.IsTenValue() = %v, want %v", got, tt.isTen)
		}
		if got := tt.card.IsAce(); got != tt.isAce {
			t.Errorf("Card.IsAce() = %v, want %v", got, tt.isAce)
		}
	}
}

func TestShoe_CreationAndDeal(t *testing.T) {
	decks := 6
	dealtList := []Card{}
	rng := rand.New(rand.NewSource(42))

	shoe := NewShoe(
		decks,
		WithCutCardPenetration(0.75),
		WithCustomRNG(rng),
		WithDealListener(func(c Card) {
			dealtList = append(dealtList, c)
		}),
	)

	if shoe.TotalDecks() != 6 {
		t.Fatalf("Expected 6 decks, got %d", shoe.TotalDecks())
	}

	totalCards := 6 * 52
	if shoe.RemainingCards() != totalCards {
		t.Fatalf("Expected %d remaining cards, got %d", totalCards, shoe.RemainingCards())
	}

	if shoe.DealtCount() != 0 {
		t.Fatalf("Expected 0 dealt cards, got %d", shoe.DealtCount())
	}

	c1 := shoe.Deal()
	if shoe.RemainingCards() != totalCards-1 {
		t.Fatalf("Expected %d cards left, got %d", totalCards-1, shoe.RemainingCards())
	}
	if shoe.DealtCount() != 1 {
		t.Fatalf("Expected 1 dealt card, got %d", shoe.DealtCount())
	}
	if len(dealtList) != 1 || dealtList[0] != c1 {
		t.Fatalf("Deal callback did not receive expected card")
	}

	// Test bounds for deck creation (0 -> 1 deck, 10 -> 8 decks)
	s1 := NewShoe(0)
	if s1.TotalDecks() != 1 {
		t.Errorf("Expected 1 deck fallback, got %d", s1.TotalDecks())
	}
	s8 := NewShoe(10, WithCutCardPenetration(1.5))
	if s8.TotalDecks() != 8 {
		t.Errorf("Expected 8 decks max, got %d", s8.TotalDecks())
	}

	// Test Deal until empty triggers reshuffle
	singleShoe := NewShoe(1)
	for i := 0; i < 52; i++ {
		singleShoe.Deal()
	}
	if singleShoe.RemainingCards() != 0 {
		t.Errorf("Expected 0 cards left, got %d", singleShoe.RemainingCards())
	}
	// 53rd deal should auto-reshuffle
	nextCard := singleShoe.Deal()
	if nextCard.Rank < Two || nextCard.Rank > Ace {
		t.Errorf("Invalid card dealt after auto reshuffle: %v", nextCard)
	}
	if singleShoe.RemainingCards() != 51 {
		t.Errorf("Expected 51 cards remaining after auto-reshuffle, got %d", singleShoe.RemainingCards())
	}
}

func TestShoe_PenetrationAndNeedsShuffle(t *testing.T) {
	shoe := NewShoe(1, WithCutCardPenetration(0.5)) // Cut card at 26 cards
	if shoe.NeedsShuffle() {
		t.Errorf("Should not need shuffle initially")
	}

	for i := 0; i < 25; i++ {
		shoe.Deal()
	}
	if shoe.NeedsShuffle() {
		t.Errorf("Should not need shuffle at 25 dealt cards")
	}

	shoe.Deal() // 26th card dealt
	if !shoe.NeedsShuffle() {
		t.Errorf("Expected NeedsShuffle to be true at 26 dealt cards")
	}

	pen := shoe.Penetration()
	if pen < 0.49 || pen > 0.51 {
		t.Errorf("Expected penetration ~0.50, got %f", pen)
	}

	remDecks := shoe.RemainingDecks()
	if remDecks < 0.49 || remDecks > 0.51 {
		t.Errorf("Expected remaining decks ~0.50, got %f", remDecks)
	}
}

func BenchmarkShoeDeal(b *testing.B) {
	shoe := NewShoe(6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = shoe.Deal()
	}
}

func BenchmarkShoeShuffle(b *testing.B) {
	shoe := NewShoe(6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shoe.ResetAndShuffle()
	}
}
