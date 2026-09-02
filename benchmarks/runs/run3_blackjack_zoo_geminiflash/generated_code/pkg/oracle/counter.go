package oracle

import (
	"blackjack/pkg/cards"
	"math"
)

// CountingSystem represents a card counting model.
type CountingSystem int

const (
	HiLo CountingSystem = iota
	KO
	OmegaII
)

func (c CountingSystem) String() string {
	switch c {
	case HiLo:
		return "Hi-Lo"
	case KO:
		return "KO (Knock-Out)"
	case OmegaII:
		return "Omega II"
	default:
		return "Unknown"
	}
}

// CardCounter tracks seen cards and computes running count, true count, and shoe stats.
type CardCounter struct {
	system         CountingSystem
	initialDecks   int
	runningCount   int
	aceSideCount   int
	totalCardsSeen int
}

// NewCardCounter initializes a card counter for a given system and deck count.
func NewCardCounter(system CountingSystem, initialDecks int) *CardCounter {
	if initialDecks < 1 {
		initialDecks = 1
	}

	cc := &CardCounter{
		system:       system,
		initialDecks: initialDecks,
	}
	cc.Reset()
	return cc
}

// Reset resets running count and stats based on the system starting count (e.g. KO initial count).
func (cc *CardCounter) Reset() {
	cc.totalCardsSeen = 0
	cc.aceSideCount = 0

	switch cc.system {
	case KO:
		// KO initial running count = 4 - (4 * decks)
		cc.runningCount = 4 - (4 * cc.initialDecks)
	default:
		cc.runningCount = 0
	}
}

// CardValue returns the point count value of a single card under current system.
func (cc *CardCounter) CardValue(card cards.Card) int {
	switch cc.system {
	case HiLo:
		// 2-6: +1, 7-9: 0, 10-A: -1
		switch card.Rank {
		case cards.Two, cards.Three, cards.Four, cards.Five, cards.Six:
			return 1
		case cards.Seven, cards.Eight, cards.Nine:
			return 0
		case cards.Ten, cards.Jack, cards.Queen, cards.King, cards.Ace:
			return -1
		}
	case KO:
		// 2-7: +1, 8-9: 0, 10-A: -1
		switch card.Rank {
		case cards.Two, cards.Three, cards.Four, cards.Five, cards.Six, cards.Seven:
			return 1
		case cards.Eight, cards.Nine:
			return 0
		case cards.Ten, cards.Jack, cards.Queen, cards.King, cards.Ace:
			return -1
		}
	case OmegaII:
		// 2,3,7: +1; 4,5,6: +2; 9: -1; 10,J,Q,K: -2; 8,A: 0
		switch card.Rank {
		case cards.Two, cards.Three, cards.Seven:
			return 1
		case cards.Four, cards.Five, cards.Six:
			return 2
		case cards.Nine:
			return -1
		case cards.Ten, cards.Jack, cards.Queen, cards.King:
			return -2
		case cards.Eight, cards.Ace:
			return 0
		}
	}
	return 0
}

// ObserveCard feeds a revealed card into the counting engine.
func (cc *CardCounter) ObserveCard(card cards.Card) {
	cc.totalCardsSeen++
	cc.runningCount += cc.CardValue(card)
	if card.IsAce() {
		cc.aceSideCount++
	}
}

// RunningCount returns the current running count.
func (cc *CardCounter) RunningCount() int {
	return cc.runningCount
}

// RemainingDecks computes remaining decks based on total initial decks and cards seen.
func (cc *CardCounter) RemainingDecks() float64 {
	totalCards := cc.initialDecks * 52
	remaining := totalCards - cc.totalCardsSeen
	decks := float64(remaining) / 52.0
	if decks < 0.5 {
		return 0.5 // minimum floor to avoid extreme division
	}
	return decks
}

// TrueCount returns the True Count (RC / Remaining Decks). For unbalanced KO, returns Running Count.
func (cc *CardCounter) TrueCount() float64 {
	if cc.system == KO {
		// KO is an unbalanced count; key count pivots at -4 or 0
		return float64(cc.runningCount)
	}
	remDecks := cc.RemainingDecks()
	return float64(cc.runningCount) / remDecks
}

// TrueCountRounded returns True Count rounded to nearest integer (standard convention).
func (cc *CardCounter) TrueCountRounded() int {
	tc := cc.TrueCount()
	return int(math.Round(tc))
}

// ShoePenetration returns the fraction of total cards seen (0.0 to 1.0).
func (cc *CardCounter) ShoePenetration() float64 {
	totalCards := cc.initialDecks * 52
	if totalCards == 0 {
		return 0
	}
	return float64(cc.totalCardsSeen) / float64(totalCards)
}

// RecommendedBetMultiplier advises how many standard betting units to wager based on Kelly criterion / True Count.
func (cc *CardCounter) RecommendedBetMultiplier() int {
	tc := cc.TrueCount()
	if tc <= 1.0 {
		return 1 // Minimum bet unit
	} else if tc <= 2.0 {
		return 2
	} else if tc <= 3.0 {
		return 4
	} else if tc <= 4.0 {
		return 6
	} else {
		return 8 // Max bet spread unit
	}
}
