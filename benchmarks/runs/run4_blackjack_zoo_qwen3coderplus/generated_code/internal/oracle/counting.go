package oracle

import (
	"blackjack/pkg/cards"
)

// CountSystem represents different card counting systems
type CountSystem string

const (
	HiLoCount    CountSystem = "hi-lo"
	KOCount      CountSystem = "ko"
	OmegaIICount CountSystem = "omega-ii"
)

// CardCounter tracks the running count and calculates true count
type CardCounter struct {
	System          CountSystem
	RunningCount    float64
	DecksUsed       float64
	TotalDecks      int
	DeckPenetration float64 // How much of the shoe has been dealt (0.0 to 1.0)
}

// NewCardCounter creates a new card counter with the specified system
func NewCardCounter(system CountSystem, totalDecks int) *CardCounter {
	return &CardCounter{
		System:          system,
		RunningCount:    0,
		DecksUsed:       0,
		TotalDecks:      totalDecks,
		DeckPenetration: 0,
	}
}

// UpdateCount updates the running count based on the card played
func (cc *CardCounter) UpdateCount(card cards.Card) {
	switch cc.System {
	case HiLoCount:
		cc.runningCountHiLo(card)
	case KOCount:
		cc.runningCountKO(card)
	case OmegaIICount:
		cc.runningCountOmegaII(card)
	}
}

// runningCountHiLo updates the running count using the Hi-Lo system
// 2-6: +1, 7-9: 0, 10-A: -1
func (cc *CardCounter) runningCountHiLo(card cards.Card) {
	switch card.Rank {
	case cards.Two, cards.Three, cards.Four, cards.Five, cards.Six:
		cc.RunningCount++
	case cards.Ten, cards.Jack, cards.Queen, cards.King, cards.Ace:
		cc.RunningCount--
		// 7, 8, 9 have no effect on Hi-Lo count
	}
}

// runningCountKO updates the running count using the KO system
// 2-7: +1, 8-9: 0, 10-A: -1
func (cc *CardCounter) runningCountKO(card cards.Card) {
	switch card.Rank {
	case cards.Two, cards.Three, cards.Four, cards.Five, cards.Six, cards.Seven:
		cc.RunningCount++
	case cards.Ten, cards.Jack, cards.Queen, cards.King, cards.Ace:
		cc.RunningCount--
		// 8, 9 have no effect on KO count
	}
}

// runningCountOmegaII updates the running count using the Omega II system
// 4, 5, 6: +2, 3, 7: +1, 8: 0, 2, 9: -1, 10-A: -2
func (cc *CardCounter) runningCountOmegaII(card cards.Card) {
	switch card.Rank {
	case cards.Four, cards.Five, cards.Six:
		cc.RunningCount += 2
	case cards.Three, cards.Seven:
		cc.RunningCount++
	case cards.Eight:
		// No change
	case cards.Two, cards.Nine:
		cc.RunningCount--
	case cards.Ten, cards.Jack, cards.Queen, cards.King, cards.Ace:
		cc.RunningCount -= 2
	}
}

// UpdateDecksPlayed updates the number of decks played and penetration
func (cc *CardCounter) UpdateDecksPlayed(cardsPlayed int) {
	// Each deck has 52 cards
	cc.DecksUsed = float64(cardsPlayed) / 52.0
	cc.DeckPenetration = cc.DecksUsed / float64(cc.TotalDecks)
}

// TrueCount calculates the true count based on the running count and remaining decks
func (cc *CardCounter) TrueCount() float64 {
	remainingDecks := float64(cc.TotalDecks) - cc.DecksUsed

	if remainingDecks <= 0 {
		return cc.RunningCount
	}

	return cc.RunningCount / remainingDecks
}

// BetRecommendation suggests a bet multiplier based on the true count
func (cc *CardCounter) BetRecommendation(baseBet float64) float64 {
	trueCount := cc.TrueCount()

	// Basic betting strategy: increase bet with higher true count
	// This is a simplified version - real systems are more complex
	if trueCount <= 0 {
		return baseBet // Bet minimum or base amount
	}

	// Scale bet proportionally to true count (can be adjusted)
	scaleFactor := trueCount
	if scaleFactor > 5 {
		scaleFactor = 5 // Cap the bet multiplier
	}

	return baseBet * scaleFactor
}

// PenetrationPercentage returns how much of the shoe has been dealt
func (cc *CardCounter) PenetrationPercentage() float64 {
	return cc.DeckPenetration * 100
}

// Reset resets the counter to initial state
func (cc *CardCounter) Reset(totalDecks int) {
	cc.RunningCount = 0
	cc.DecksUsed = 0
	cc.TotalDecks = totalDecks
	cc.DeckPenetration = 0
}

// GetCountInfo returns a summary of the current count information
func (cc *CardCounter) GetCountInfo() map[string]interface{} {
	return map[string]interface{}{
		"system":              cc.System,
		"running_count":       cc.RunningCount,
		"true_count":          cc.TrueCount(),
		"decks_used":          cc.DecksUsed,
		"total_decks":         cc.TotalDecks,
		"penetration_percent": cc.PenetrationPercentage(),
	}
}
