package strategy

import (
	"blackjack/internal/game"
	"blackjack/internal/deck"
)

// Basic strategy tables: 4-8 decks, S17, DAS.
// Columns are dealer upcard values: 2,3,4,5,6,7,8,9,10,A

type tableRow [10]game.Action

// Card is an alias so callers don't need to import both deck and game packages directly.
type Card = deck.Card

// upcardIndex converts a dealer upcard to a table column (0-9).
func upcardIndex(c deck.Card) int {
	switch {
	case c.Rank >= deck.Two && c.Rank <= deck.Nine:
		return int(c.Rank) - 2
	case c.Rank == deck.Ten || c.Rank == deck.Jack || c.Rank == deck.Queen || c.Rank == deck.King:
		return 8
	case c.Rank == deck.Ace:
		return 9
	default:
		return 0
	}
}

// Hard totals 5-21 (index 0 = total 5). Standard hard total basic strategy for S17.
var hardTable = []tableRow{
	/*5*/ {game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*6*/ {game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*7*/ {game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*8*/ {game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*9*/ {game.ActionHit, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*10*/ {game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit, game.ActionHit},
	/*11*/ {game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit},
	/*12*/ {game.ActionHit, game.ActionHit, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*13*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*14*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*15*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*16*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*17*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand},
	/*18*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand},
	/*19*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand},
	/*20*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand},
	/*21*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand},
}

// Soft totals (index 0 = A,2 / soft 13). Standard soft total basic strategy for S17.
var softTable = []tableRow{
	/*A2 (13)*/ {game.ActionHit, game.ActionHit, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*A3 (14)*/ {game.ActionHit, game.ActionHit, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*A4 (15)*/ {game.ActionHit, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*A5 (16)*/ {game.ActionHit, game.ActionHit, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*A6 (17)*/ {game.ActionHit, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*A7 (18)*/ {game.ActionStand, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionStand, game.ActionStand, game.ActionHit, game.ActionHit, game.ActionHit},
	/*A8 (19)*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand},
	/*A9 (20)*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand},
}

// Pairs table (index 0 = 2,2 ... 8 = 10,10, 9 = A,A). With DAS.
var pairsTable = []tableRow{
	/*2,2*/ {game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*3,3*/ {game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*4,4*/ {game.ActionHit, game.ActionHit, game.ActionHit, game.ActionSplit, game.ActionSplit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*5,5*/ {game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionDouble, game.ActionHit, game.ActionHit},
	/*6,6*/ {game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*7,7*/ {game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionHit, game.ActionHit, game.ActionHit, game.ActionHit},
	/*8,8*/ {game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit},
	/*9,9*/ {game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionStand, game.ActionSplit, game.ActionSplit, game.ActionStand, game.ActionStand},
	/*10,10*/ {game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand, game.ActionStand},
	/*A,A*/ {game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit, game.ActionSplit},
}

// OptimalAction returns the optimal basic strategy action for a player hand vs dealer upcard.
// If the primary action (Double/Split) is not allowed, it falls back to the correct alternate action.
func OptimalAction(hand *game.Hand, dealerUpcard deck.Card, canDouble bool, canSplit bool) game.Action {
	idx := upcardIndex(dealerUpcard)

	// Pair splitting logic
	if hand.CanSplit() && canSplit {
		first := hand.Cards[0]
		var pairIdx int
		if first.Rank == deck.Ace {
			pairIdx = 9
		} else if first.Value >= 10 {
			pairIdx = 8
		} else {
			pairIdx = int(first.Rank) - 2
		}
		if pairIdx >= 0 && pairIdx < len(pairsTable) {
			action := pairsTable[pairIdx][idx]
			if action == game.ActionSplit && canSplit {
				return game.ActionSplit
			}
			if action == game.ActionDouble {
				if canDouble {
					return game.ActionDouble
				}
				return game.ActionHit
			}
			return action
		}
	}

	_, soft, isSoft := hand.Total()
	total := soft

	if !isSoft {
		if total < 5 {
			total = 5
		}
		if total > 21 {
			total = 21
		}
		row := hardTable[total-5]
		action := row[idx]

		// If double recommended but not allowed, hit (or stand for 12 vs 4-6 edge cases)
		if action == game.ActionDouble && !canDouble {
			if total == 12 {
				return row[idx]
			}
			return game.ActionHit
		}
		return action
	}

	// Soft totals: A,2 (soft 13) through A,9 (soft 20)
	// soft total ranges 13-20, map A2->0 ... A9->7
	softIdx := total - 13
	if softIdx < 0 {
		softIdx = 0
	}
	if softIdx > 7 {
		softIdx = 7
	}
	row := softTable[softIdx]
	action := row[idx]

	if action == game.ActionDouble && !canDouble {
		// A7 soft 18 falls back to Stand when double unavailable
		if softIdx == 5 {
			return game.ActionStand
		}
		return game.ActionHit
	}
	return action
}
