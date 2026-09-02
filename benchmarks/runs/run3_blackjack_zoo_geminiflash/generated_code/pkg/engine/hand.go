package engine

import (
	"blackjack/pkg/cards"
	"fmt"
)

// HandStatus represents the current state of a hand.
type HandStatus int

const (
	StatusInPlay HandStatus = iota
	StatusStood
	StatusDoubled
	StatusBusted
	StatusBlackjack
	StatusSurrendered
)

func (s HandStatus) String() string {
	switch s {
	case StatusInPlay:
		return "In Play"
	case StatusStood:
		return "Stood"
	case StatusDoubled:
		return "Doubled"
	case StatusBusted:
		return "Busted"
	case StatusBlackjack:
		return "Blackjack"
	case StatusSurrendered:
		return "Surrendered"
	default:
		return "Unknown"
	}
}

// Hand represents a single blackjack hand.
type Hand struct {
	Cards         []cards.Card
	Bet           float64
	Status        HandStatus
	IsSplitHand   bool
	FromSplitAces bool
	Doubled       bool
	InsuranceBet  float64
	InsuranceWon  bool
	Payout        float64 // Net payout multiplier (e.g. +1.5 for BJ, -1.0 for loss, 0 for push)
	NetProfit     float64 // Total net currency profit/loss for this hand
}

// NewHand creates an empty hand with initial wager.
func NewHand(bet float64) *Hand {
	return &Hand{
		Cards:  make([]cards.Card, 0, 8),
		Bet:    bet,
		Status: StatusInPlay,
	}
}

// AddCard appends a card to the hand.
func (h *Hand) AddCard(c cards.Card) {
	h.Cards = append(h.Cards, c)
}

// Score computes the best total value and whether the hand is soft (contains an Ace counted as 11).
func (h *Hand) Score() (total int, isSoft bool) {
	aces := 0
	sum := 0

	for _, c := range h.Cards {
		if c.IsAce() {
			aces++
			sum += 11
		} else {
			sum += c.Rank.BlackjackValue()
		}
	}

	for sum > 21 && aces > 0 {
		sum -= 10
		aces--
	}

	return sum, (aces > 0)
}

// Total returns just the integer score.
func (h *Hand) Total() int {
	t, _ := h.Score()
	return t
}

// IsBusted returns true if hand total exceeds 21.
func (h *Hand) IsBusted() bool {
	return h.Total() > 21
}

// IsBlackjack returns true if initial 2 cards equal 21 (and not split hand).
func (h *Hand) IsBlackjack() bool {
	if h.IsSplitHand {
		return false // Split 21 is a 21, not a natural Blackjack
	}
	if len(h.Cards) != 2 {
		return false
	}
	return h.Total() == 21
}

// CanSplit returns true if hand has exactly 2 cards of equal point value.
func (h *Hand) CanSplit() bool {
	if len(h.Cards) != 2 {
		return false
	}
	return h.Cards[0].Rank.BlackjackValue() == h.Cards[1].Rank.BlackjackValue()
}

// CanDouble returns true if hand has exactly 2 cards and is in play.
func (h *Hand) CanDouble() bool {
	return len(h.Cards) == 2 && h.Status == StatusInPlay
}

// CanSurrender returns true if hand has exactly 2 cards and has not doubled/split.
func (h *Hand) CanSurrender() bool {
	return len(h.Cards) == 2 && h.Status == StatusInPlay && !h.IsSplitHand
}

// String provides a human-readable representation of the hand.
func (h *Hand) String() string {
	val, soft := h.Score()
	softStr := ""
	if soft && val < 21 {
		softStr = " (soft)"
	}
	var cardStrs string
	for _, c := range h.Cards {
		cardStrs += c.String() + " "
	}
	return fmt.Sprintf("[%s] Total: %d%s - %s (Bet: $%.2f)", cardStrs, val, softStr, h.Status.String(), h.Bet)
}
