package oracle

import (
	"blackjack/pkg/cards"
	"blackjack/pkg/engine"
	"blackjack/pkg/rules"
	"fmt"
)

// StrategyAdvisor produces mathematically optimal Basic Strategy decisions.
type StrategyAdvisor struct {
	rules rules.RuleSet
}

// NewStrategyAdvisor creates an advisor aware of table rules.
func NewStrategyAdvisor(r rules.RuleSet) *StrategyAdvisor {
	return &StrategyAdvisor{rules: r}
}

// Recommendation encapsulates the suggested action and rationale.
type Recommendation struct {
	Action      engine.PlayerAction
	Description string
	Deviation   bool // True if count-based deviation
}

// Advise decides the optimal player move given player hand and dealer upcard.
func (sa *StrategyAdvisor) Advise(hand *engine.Hand, dealerUpcard cards.Card, trueCount float64) Recommendation {
	if hand == nil || len(hand.Cards) == 0 {
		return Recommendation{Action: engine.ActionStand, Description: "No hand"}
	}

	dealerVal := dealerUpcard.Rank.BlackjackValue()

	// 1. Surrender decisions (if allowed and 2 initial cards)
	if hand.CanSurrender() && sa.rules.Surrender == rules.SurrenderLate {
		tot, isSoft := hand.Score()
		if !isSoft {
			if tot == 16 && (dealerVal == 9 || dealerVal == 10 || dealerVal == 11) {
				// Don't surrender pair of 8s if split is allowed
				if !(len(hand.Cards) == 2 && hand.Cards[0].Rank == cards.Eight && sa.rules.MaxSplitHands > 1) {
					return Recommendation{
						Action:      engine.ActionSurrender,
						Description: fmt.Sprintf("Surrender Hard %d vs Dealer %s", tot, dealerUpcard.Rank.String()),
					}
				}
			}
			if tot == 15 && dealerVal == 10 {
				return Recommendation{
					Action:      engine.ActionSurrender,
					Description: fmt.Sprintf("Surrender Hard 15 vs Dealer 10"),
				}
			}
			if tot == 17 && dealerVal == 11 && sa.rules.DealerHitsSoft17 {
				return Recommendation{
					Action:      engine.ActionSurrender,
					Description: fmt.Sprintf("Surrender Hard 17 vs Dealer Ace on H17"),
				}
			}
		}
	}

	// 2. Pair Splitting
	if hand.CanSplit() && len(hand.Cards) == 2 {
		r := hand.Cards[0].Rank
		shouldSplit := sa.shouldSplitPair(r, dealerVal)
		if shouldSplit {
			return Recommendation{
				Action:      engine.ActionSplit,
				Description: fmt.Sprintf("Split pair of %ss vs Dealer %s", r.String(), dealerUpcard.Rank.String()),
			}
		}
	}

	// 3. Soft Totals (Contains Ace counted as 11)
	tot, isSoft := hand.Score()
	if isSoft && len(hand.Cards) >= 2 && tot <= 21 {
		return sa.adviseSoft(hand, tot, dealerVal, dealerUpcard)
	}

	// 4. Hard Totals
	return sa.adviseHard(hand, tot, dealerVal, dealerUpcard)
}

func (sa *StrategyAdvisor) shouldSplitPair(rank cards.Rank, dealerVal int) bool {
	switch rank {
	case cards.Ace, cards.Eight:
		return true // Always split Aces and 8s
	case cards.Nine:
		// Split vs 2-6, 8-9 (Stand vs 7, 10, Ace)
		return (dealerVal >= 2 && dealerVal <= 6) || dealerVal == 8 || dealerVal == 9
	case cards.Seven:
		// Split vs 2-7
		return dealerVal >= 2 && dealerVal <= 7
	case cards.Six:
		// Split vs 2-6 (or 3-6 if DAS false)
		if sa.rules.DoubleAfterSplit {
			return dealerVal >= 2 && dealerVal <= 6
		}
		return dealerVal >= 3 && dealerVal <= 6
	case cards.Five:
		return false // Never split 5s (Double instead)
	case cards.Four:
		// Split vs 5-6 only if DAS allowed
		if sa.rules.DoubleAfterSplit {
			return dealerVal == 5 || dealerVal == 6
		}
		return false
	case cards.Three, cards.Two:
		// Split vs 2-7 if DAS, or 4-7 if NDAS
		if sa.rules.DoubleAfterSplit {
			return dealerVal >= 2 && dealerVal <= 7
		}
		return dealerVal >= 4 && dealerVal <= 7
	case cards.Ten, cards.Jack, cards.Queen, cards.King:
		return false // Never split 10s (Stand on 20)
	}
	return false
}

func (sa *StrategyAdvisor) adviseSoft(hand *engine.Hand, tot int, dealerVal int, dealerUpcard cards.Card) Recommendation {
	canDouble := hand.CanDouble() && sa.canDoubleRule(hand)

	switch tot {
	case 20, 21: // A,9 or A,10
		return Recommendation{Action: engine.ActionStand, Description: fmt.Sprintf("Stand Soft %d", tot)}
	case 19: // A,8
		if canDouble && dealerVal == 6 && sa.rules.DealerHitsSoft17 {
			return Recommendation{Action: engine.ActionDouble, Description: "Double Soft 19 vs 6 on H17"}
		}
		return Recommendation{Action: engine.ActionStand, Description: fmt.Sprintf("Stand Soft %d", tot)}
	case 18: // A,7
		if canDouble && (dealerVal >= 2 && dealerVal <= 6) {
			return Recommendation{Action: engine.ActionDouble, Description: "Double Soft 18 vs 2-6"}
		}
		if dealerVal >= 2 && dealerVal <= 8 {
			return Recommendation{Action: engine.ActionStand, Description: "Stand Soft 18 vs 2-8"}
		}
		return Recommendation{Action: engine.ActionHit, Description: "Hit Soft 18 vs 9, 10, A"}
	case 17: // A,6
		if canDouble && (dealerVal >= 3 && dealerVal <= 6) {
			return Recommendation{Action: engine.ActionDouble, Description: "Double Soft 17 vs 3-6"}
		}
		return Recommendation{Action: engine.ActionHit, Description: "Hit Soft 17"}
	case 15, 16: // A,4 or A,5
		if canDouble && (dealerVal >= 4 && dealerVal <= 6) {
			return Recommendation{Action: engine.ActionDouble, Description: fmt.Sprintf("Double Soft %d vs 4-6", tot)}
		}
		return Recommendation{Action: engine.ActionHit, Description: fmt.Sprintf("Hit Soft %d", tot)}
	case 13, 14: // A,2 or A,3
		if canDouble && (dealerVal == 5 || dealerVal == 6) {
			return Recommendation{Action: engine.ActionDouble, Description: fmt.Sprintf("Double Soft %d vs 5-6", tot)}
		}
		return Recommendation{Action: engine.ActionHit, Description: fmt.Sprintf("Hit Soft %d", tot)}
	default:
		return Recommendation{Action: engine.ActionHit, Description: fmt.Sprintf("Hit Soft %d", tot)}
	}
}

func (sa *StrategyAdvisor) adviseHard(hand *engine.Hand, tot int, dealerVal int, dealerUpcard cards.Card) Recommendation {
	canDouble := hand.CanDouble() && sa.canDoubleRule(hand)

	switch {
	case tot >= 17:
		return Recommendation{Action: engine.ActionStand, Description: fmt.Sprintf("Stand Hard %d", tot)}
	case tot >= 13 && tot <= 16:
		if dealerVal >= 2 && dealerVal <= 6 {
			return Recommendation{Action: engine.ActionStand, Description: fmt.Sprintf("Stand Hard %d vs Dealer %s", tot, dealerUpcard.Rank.String())}
		}
		return Recommendation{Action: engine.ActionHit, Description: fmt.Sprintf("Hit Hard %d vs Dealer %s", tot, dealerUpcard.Rank.String())}
	case tot == 12:
		if dealerVal >= 4 && dealerVal <= 6 {
			return Recommendation{Action: engine.ActionStand, Description: "Stand Hard 12 vs 4-6"}
		}
		return Recommendation{Action: engine.ActionHit, Description: "Hit Hard 12"}
	case tot == 11:
		if canDouble {
			if dealerVal == 11 && !sa.rules.DealerHitsSoft17 {
				// S17 double 11 vs 2-10, hit vs Ace
				return Recommendation{Action: engine.ActionHit, Description: "Hit 11 vs Ace (S17)"}
			}
			return Recommendation{Action: engine.ActionDouble, Description: "Double 11"}
		}
		return Recommendation{Action: engine.ActionHit, Description: "Hit 11"}
	case tot == 10:
		if canDouble && (dealerVal >= 2 && dealerVal <= 9) {
			return Recommendation{Action: engine.ActionDouble, Description: "Double 10 vs 2-9"}
		}
		return Recommendation{Action: engine.ActionHit, Description: "Hit 10"}
	case tot == 9:
		if canDouble && (dealerVal >= 3 && dealerVal <= 6) {
			return Recommendation{Action: engine.ActionDouble, Description: "Double 9 vs 3-6"}
		}
		return Recommendation{Action: engine.ActionHit, Description: "Hit 9"}
	default: // 8 or less
		return Recommendation{Action: engine.ActionHit, Description: fmt.Sprintf("Hit Hard %d", tot)}
	}
}

func (sa *StrategyAdvisor) canDoubleRule(hand *engine.Hand) bool {
	if hand.IsSplitHand && !sa.rules.DoubleAfterSplit {
		return false
	}
	tot := hand.Total()
	switch sa.rules.DoubleRestriction {
	case rules.Double9To11Only:
		return tot >= 9 && tot <= 11
	case rules.Double10Or11Only:
		return tot >= 10 && tot <= 11
	default:
		return true
	}
}

// AdviseInsurance advises whether to accept insurance based on True Count (Hi-Lo TC >= +3 is positive EV).
func (sa *StrategyAdvisor) AdviseInsurance(trueCount float64) (accept bool, reason string) {
	if trueCount >= 3.0 {
		return true, fmt.Sprintf("True Count is +%.1f (>= +3.0) - Insurance is positive EV (+EV)", trueCount)
	}
	return false, fmt.Sprintf("True Count is +%.1f (< +3.0) - Insurance is negative EV (-EV)", trueCount)
}
