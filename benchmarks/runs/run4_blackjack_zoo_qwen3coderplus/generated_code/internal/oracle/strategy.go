package oracle

import (
	"blackjack/internal/game"
	"blackjack/pkg/cards"
)

// Action represents a possible player action
type Action string

const (
	HitAction       Action = "hit"
	StandAction     Action = "stand"
	DoubleAction    Action = "double"
	SplitAction     Action = "split"
	SurrenderAction Action = "surrender"
)

// StrategyAdvisor provides optimal play recommendations based on card counting and basic strategy
type StrategyAdvisor struct {
	RuleSet *game.RuleSet
	Counter *CardCounter
}

// NewStrategyAdvisor creates a new strategy advisor with the specified rules and counting system
func NewStrategyAdvisor(ruleSet *game.RuleSet, counter *CardCounter) *StrategyAdvisor {
	return &StrategyAdvisor{
		RuleSet: ruleSet,
		Counter: counter,
	}
}

// GetRecommendedAction returns the optimal action based on player hand, dealer upcard, and true count
func (sa *StrategyAdvisor) GetRecommendedAction(playerHand []cards.Card, dealerUpcard cards.Card, currentBet float64) Action {
	playerSum := sa.getHandValue(playerHand)
	dealerValue := sa.getCardValue(dealerUpcard)

	// Determine if player hand is soft (contains ace counted as 11)
	isSoft := sa.isSoftHand(playerHand)

	// If hand is a pair, check for split opportunities first
	if len(playerHand) == 2 && playerHand[0].Rank == playerHand[1].Rank {
		action := sa.checkPairStrategy(playerHand[0].Rank, dealerValue, isSoft)
		if action != "" {
			return action
		}
	}

	// Determine action based on hand type
	if isSoft {
		return sa.getSoftHandStrategy(playerSum, dealerValue)
	} else {
		return sa.getHardHandStrategy(playerSum, dealerValue)
	}
}

// getHandValue calculates the value of a hand, preferring the highest value <= 21
func (sa *StrategyAdvisor) getHandValue(hand []cards.Card) int {
	value := 0
	aces := 0

	for _, card := range hand {
		if card.Rank == cards.Ace {
			aces++
			value += 11
		} else {
			value += sa.getCardValue(card)
		}
	}

	// Adjust for aces if value exceeds 21
	for aces > 0 && value > 21 {
		value -= 10 // Change ace from 11 to 1
		aces--
	}

	return value
}

// getCardValue returns the numeric value of a card
func (sa *StrategyAdvisor) getCardValue(card cards.Card) int {
	return card.Value
}

// isSoftHand returns true if the hand contains an ace counted as 11
func (sa *StrategyAdvisor) isSoftHand(hand []cards.Card) bool {
	minVal := 0
	hasAce := false

	for _, card := range hand {
		if card.Rank == cards.Ace {
			hasAce = true
			minVal += 1
		} else {
			minVal += sa.getCardValue(card)
		}
	}

	return hasAce && (minVal+10 <= 21)
}

// checkPairStrategy returns the recommended action for a pair
func (sa *StrategyAdvisor) checkPairStrategy(rank cards.Rank, dealerValue int, isSoft bool) Action {
	// Convert dealer value to proper representation (11 for Ace)
	if dealerValue == 11 && rank == cards.Ace {
		// Special case for pair of aces
		return SplitAction
	}

	// Basic pair strategy based on rank and dealer upcard
	switch rank {
	case cards.Ace:
		return SplitAction // Always split aces
	case cards.Eight:
		return SplitAction // Always split eights
	case cards.Ten:
		return StandAction // Never split tens
	case cards.Nine:
		// Split 9-9 against dealer 2-6, 8-9; Stand against 7, 10, A
		if dealerValue >= 2 && dealerValue <= 6 || dealerValue == 8 || dealerValue == 9 {
			return SplitAction
		} else if dealerValue == 7 || dealerValue == 10 || dealerValue == 11 {
			return StandAction
		}
	case cards.Seven:
		// Split 7-7 against dealer 2-7; Hit against 8-A
		if dealerValue >= 2 && dealerValue <= 7 {
			return SplitAction
		} else {
			return HitAction
		}
	case cards.Six:
		// Split 6-6 against dealer 2-6; Hit against 7-A
		if dealerValue >= 2 && dealerValue <= 6 {
			return SplitAction
		} else {
			return HitAction
		}
	case cards.Five:
		// Treat as hard 10
		return sa.getHardHandStrategy(10, dealerValue)
	case cards.Four:
		// Split 4-4 against dealer 5-6 only (if DAS allowed)
		if sa.RuleSet.DoubleAfterSplit && dealerValue == 5 || dealerValue == 6 {
			return SplitAction
		} else {
			return HitAction
		}
	case cards.Three, cards.Two:
		// Split 2-2, 3-3 against dealer 2-7; Hit against 8-A
		if dealerValue >= 2 && dealerValue <= 7 {
			return SplitAction
		} else {
			return HitAction
		}
	}

	// Default: don't split
	return ""
}

// getSoftHandStrategy returns the recommended action for a soft hand
func (sa *StrategyAdvisor) getSoftHandStrategy(playerSum int, dealerValue int) Action {
	// Basic soft hand strategy
	switch playerSum {
	case 20: // A-9
		return StandAction
	case 19: // A-8
		// In standard basic strategy: double vs 6 (if DAS/double soft allowed) or Stand.
		// If testing standard stand:
		if dealerValue == 6 {
			return DoubleAction
		}
		return StandAction
	case 18: // A-7
		if dealerValue >= 3 && dealerValue <= 6 {
			return DoubleAction
		} else if dealerValue == 2 || dealerValue == 7 || dealerValue == 8 {
			return StandAction
		} else {
			return HitAction
		}
	case 17: // A-6
		if dealerValue >= 3 && dealerValue <= 6 {
			// Double if allowed, otherwise hit
			return DoubleAction
		} else {
			return HitAction
		}
	case 16, 15: // A-5, A-4
		if dealerValue >= 4 && dealerValue <= 6 {
			// Double if allowed, otherwise hit
			return DoubleAction
		} else {
			return HitAction
		}
	case 14, 13: // A-3, A-2
		if dealerValue == 5 || dealerValue == 6 {
			// Double if allowed, otherwise hit
			return DoubleAction
		} else {
			return HitAction
		}
	default:
		return HitAction
	}
}

// getHardHandStrategy returns the recommended action for a hard hand
func (sa *StrategyAdvisor) getHardHandStrategy(playerSum int, dealerValue int) Action {
	// Basic hard hand strategy
	if playerSum >= 17 {
		return StandAction
	} else if playerSum >= 13 {
		if dealerValue >= 2 && dealerValue <= 6 {
			return StandAction
		} else {
			return HitAction
		}
	} else if playerSum == 12 {
		if dealerValue >= 4 && dealerValue <= 6 {
			return StandAction
		} else {
			return HitAction
		}
	} else if playerSum == 11 {
		// Always double if allowed, otherwise hit
		return DoubleAction
	} else if playerSum == 10 {
		if dealerValue >= 2 && dealerValue <= 9 {
			// Double if allowed, otherwise hit
			return DoubleAction
		} else {
			return HitAction
		}
	} else if playerSum == 9 {
		if dealerValue >= 3 && dealerValue <= 6 {
			// Double if allowed, otherwise hit
			return DoubleAction
		} else {
			return HitAction
		}
	} else {
		// Hands 4-8 should always hit
		return HitAction
	}
}

// GetAdvancedAction returns the optimal action with card counting adjustments
func (sa *StrategyAdvisor) GetAdvancedAction(playerHand []cards.Card, dealerUpcard cards.Card, currentBet float64) Action {
	// Get the basic strategy action first
	basicAction := sa.GetRecommendedAction(playerHand, dealerUpcard, currentBet)

	// Apply card counting adjustments
	trueCount := sa.Counter.TrueCount()

	// Adjust based on true count deviations from basic strategy
	playerSum := sa.getHandValue(playerHand)
	dealerValue := sa.getCardValue(dealerUpcard)

	// Specific index plays based on true count
	switch {
	case playerSum == 16 && dealerValue == 10:
		// 16 vs 10: Stand at TC >= 0, Hit at TC < 0
		if trueCount >= 0 && basicAction == HitAction {
			return StandAction
		}
	case playerSum == 15 && dealerValue == 10:
		// 15 vs 10: Stand at TC >= 4, Hit at TC < 4
		if trueCount >= 4 && basicAction == HitAction {
			return StandAction
		}
	case playerSum == 10 && dealerValue == 10:
		// 10 vs 10: Double at TC >= 4, otherwise hit
		if trueCount >= 4 && basicAction != DoubleAction {
			return DoubleAction
		}
	case playerSum == 12 && dealerValue >= 4 && dealerValue <= 6:
		// 12 vs 4,5,6: Stand at higher TC
		if (dealerValue == 4 && trueCount >= 5) ||
			(dealerValue == 5 && trueCount >= 3) ||
			(dealerValue == 6 && trueCount >= 2) {
			return StandAction
		}
	case playerSum == 13 && dealerValue == 2:
		// 13 vs 2: Stand at TC <= -2
		if trueCount <= -2 && basicAction == StandAction {
			return HitAction
		}
	case playerSum == 13 && dealerValue == 3:
		// 13 vs 3: Stand at TC <= -1
		if trueCount <= -1 && basicAction == StandAction {
			return HitAction
		}
	}

	// For pair splitting adjustments
	if len(playerHand) == 2 && playerHand[0].Rank == playerHand[1].Rank {
		rank := playerHand[0].Rank
		switch {
		case rank == cards.Nine && dealerValue == 7:
			// 9-9 vs 7: Stand at TC >= 4, Split otherwise
			if trueCount >= 4 {
				return StandAction
			}
		case rank == cards.Ten && dealerValue == 5: // 10-10
			// 10-10 vs 5: Double at TC >= 5 (if allowed)
			if trueCount >= 5 && sa.RuleSet.DoubleAfterSplit {
				return DoubleAction
			}
		case rank == cards.Eight && dealerValue == 6:
			// 8-8 vs 6: Stand at very high TC (rare)
		}
	}

	return basicAction
}

// ShouldTakeInsurance returns whether the player should take insurance
func (sa *StrategyAdvisor) ShouldTakeInsurance(dealerUpcard cards.Card, currentBet float64) bool {
	if dealerUpcard.Rank != cards.Ace {
		return false // Insurance only available when dealer shows Ace
	}

	// In a neutral or negative count, insurance is generally not favorable
	// In a positive count rich in 10s, insurance becomes more favorable
	trueCount := sa.Counter.TrueCount()

	// Insurance becomes favorable when true count is significantly positive
	// This is typically when TC >= 3 for Hi-Lo, but varies by system
	switch sa.Counter.System {
	case HiLoCount:
		return trueCount >= 3
	case KOCount:
		return trueCount >= 3
	case OmegaIICount:
		return trueCount >= 2
	default:
		return false
	}
}

// ShouldSurrender returns whether the player should surrender
func (sa *StrategyAdvisor) ShouldSurrender(playerHand []cards.Card, dealerUpcard cards.Card) bool {
	if !sa.RuleSet.LateSurrender {
		return false // Surrender not allowed
	}

	playerSum := sa.getHandValue(playerHand)
	dealerValue := sa.getCardValue(dealerUpcard)

	// Basic surrender strategy
	switch {
	case playerSum == 16 && (dealerValue == 9 || dealerValue == 10 || dealerValue == 11):
		return true
	case playerSum == 15 && dealerValue == 10:
		return true
	default:
		return false
	}
}
