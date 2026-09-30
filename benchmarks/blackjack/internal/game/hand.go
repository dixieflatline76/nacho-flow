package game

import (
	"blackjack/internal/deck"
)

// Hand represents a blackjack hand with cards and associated metadata.
type Hand struct {
	Cards       []deck.Card
	Bet         int
	IsSplit     bool
	IsDoubled   bool
	IsBusted    bool
	IsStanding  bool
	IsSurrender bool
}

// NewHand creates a new empty hand with initial bet.
func NewHand(bet int) *Hand {
	return &Hand{Bet: bet}
}

// AddCard appends a card to the hand.
func (h *Hand) AddCard(c deck.Card) {
	h.Cards = append(h.Cards, c)
}

// Total computes (hard, soft, isSoft) where soft indicates an Ace counted as 11.
// hard: value with all aces as 1 (or final count when 11 is usable).
// soft: true when at least one ace currently counts as 11 without busting.
func (h *Hand) Total() (hard int, soft int, isSoft bool) {
	hard = 0
	aces := 0
	for _, c := range h.Cards {
		if c.Rank == deck.Ace {
			aces++
			hard += 1
		} else {
			hard += c.Value
		}
	}

	// Try to count an ace as 11 to maximize score while staying <= 21
	soft = hard
	extraAces := aces
	for extraAces > 0 {
		if soft+10 <= 21 {
			soft += 10
			extraAces--
		} else {
			break
		}
	}

	isSoft = soft != hard && soft <= 21
	return hard, soft, isSoft
}

// BestValue returns the highest total <= 21, or lowest if busted.
func (h *Hand) BestValue() int {
	_, soft, _ := h.Total()
	if soft <= 21 {
		return soft
	}
	hard, _, _ := h.Total()
	return hard
}

// IsBlackjack returns true if hand is a natural blackjack (2 cards totaling 21) and not from a split.
func (h *Hand) IsBlackjack() bool {
	if h.IsSplit || len(h.Cards) != 2 {
		return false
	}
	_, soft, _ := h.Total()
	return soft == 21
}

// CanSplit returns true if hand can be split (2 cards of same blackjack value).
func (h *Hand) CanSplit() bool {
	if len(h.Cards) != 2 {
		return false
	}
	return cardValue(h.Cards[0]) == cardValue(h.Cards[1])
}

// CanDouble returns true if hand can be doubled (only on first 2 cards).
func (h *Hand) CanDouble() bool {
	return len(h.Cards) == 2 && !h.IsDoubled && !h.IsStanding
}

// cardValue gets the standard blackjack value of a card for pairing purposes.
func cardValue(c deck.Card) int {
	if c.Rank == deck.Ace {
		return 11
	}
	if c.Value > 10 {
		return 10
	}
	return c.Value
}

// String renders cards as compact string, e.g., "10♠ A♥".
func (h *Hand) String() string {
	s := ""
	for i, c := range h.Cards {
		if i > 0 {
			s += " "
		}
		s += c.String()
	}
	return s
}

// DealerHand renders the dealer hand, hiding the hole card if faceDown is true.
func (h *Hand) DealerHand(faceDown bool) string {
	if len(h.Cards) == 0 {
		return ""
	}
	if faceDown {
		return "?? " + h.Cards[1].String()
	}
	return h.String()
}
