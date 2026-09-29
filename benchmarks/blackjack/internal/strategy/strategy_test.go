package strategy_test

import (
	"math/rand"
	"testing"

	"blackjack/internal/deck"
	"blackjack/internal/game"
	"blackjack/internal/strategy"
)

func makeTestHand(ranks ...deck.Rank) *game.Hand {
	suits := []deck.Suit{deck.Spades, deck.Hearts, deck.Diamonds, deck.Clubs}
	h := game.NewHand(10)
	for i, r := range ranks {
		h.AddCard(deck.NewCard(suits[i%4], r))
	}
	return h
}

// dealerCard creates a dealer upcard for table testing.
func dealerCard(rank deck.Rank) deck.Card {
	return deck.NewCard(deck.Clubs, rank)
}

// tableActionValue maps an action to a compact char for readability in assertions.
func TestHardTotalsMatrix(t *testing.T) {
	// Standard 4-8 deck S17 DAS basic strategy (hard totals).
	// Format: hand total -> map of dealer upcard to expected action.
	type exp struct {
		total int
		up    deck.Rank
		want  game.Action
	}

	cases := []exp{
		// Hard 8: always hit
		{8, deck.Two, game.ActionHit}, {8, deck.Six, game.ActionHit}, {8, deck.Ace, game.ActionHit},
		// Hard 9: double vs 3-6, else hit
		{9, deck.Two, game.ActionHit}, {9, deck.Three, game.ActionDouble},
		{9, deck.Four, game.ActionDouble}, {9, deck.Five, game.ActionDouble},
		{9, deck.Six, game.ActionDouble}, {9, deck.Seven, game.ActionHit},
		{9, deck.Ace, game.ActionHit},
		// Hard 10: double vs 2-9, hit vs 10/A
		{10, deck.Two, game.ActionDouble}, {10, deck.Nine, game.ActionDouble},
		{10, deck.Ten, game.ActionHit}, {10, deck.Ace, game.ActionHit},
		// Hard 11: double vs all except Ace (S17: hit vs Ace)
		{11, deck.Two, game.ActionDouble}, {11, deck.Ten, game.ActionDouble},
		{11, deck.Ace, game.ActionHit},
		// Hard 12: stand vs 4-6, hit otherwise
		{12, deck.Two, game.ActionHit}, {12, deck.Three, game.ActionHit},
		{12, deck.Four, game.ActionStand}, {12, deck.Five, game.ActionStand},
		{12, deck.Six, game.ActionStand}, {12, deck.Seven, game.ActionHit},
		{12, deck.Ace, game.ActionHit},
		// Hard 13: stand vs 2-6, hit vs 7-A
		{13, deck.Two, game.ActionStand}, {13, deck.Six, game.ActionStand},
		{13, deck.Seven, game.ActionHit}, {13, deck.Ace, game.ActionHit},
		// Hard 14: same
		{14, deck.Three, game.ActionStand}, {14, deck.Seven, game.ActionHit},
		{14, deck.Ten, game.ActionHit},
		// Hard 15: same
		{15, deck.Four, game.ActionStand}, {15, deck.Nine, game.ActionHit},
		// Hard 16: stand vs 2-6, hit vs 7-A
		{16, deck.Two, game.ActionStand}, {16, deck.Six, game.ActionStand},
		{16, deck.Seven, game.ActionHit}, {16, deck.Ten, game.ActionHit},
		{16, deck.Ace, game.ActionHit},
		// Hard 17+: always stand
		{17, deck.Two, game.ActionStand}, {17, deck.Ace, game.ActionStand},
		{18, deck.Ten, game.ActionStand}, {20, deck.Ace, game.ActionStand},
		{21, deck.Two, game.ActionStand},
	}

	for _, c := range cases {
		hand := buildHandTotal(c.total, false)
		got := strategy.OptimalAction(hand, dealerCard(c.up), true, false)
		if got != c.want {
			t.Errorf("hard %d vs %s: want %v, got %v", c.total, c.up, c.want, got)
		}
	}
}

func TestSoftTotalsMatrix(t *testing.T) {
	type exp struct {
		handRanks []deck.Rank
		softTotal int
		up        deck.Rank
		want      game.Action
	}

	cases := []exp{
		// A,2 = soft 13: hit vs 2-3, double vs 4-6, hit vs 7+
		{[]deck.Rank{deck.Ace, deck.Two}, 13, deck.Two, game.ActionHit},
		{[]deck.Rank{deck.Ace, deck.Two}, 13, deck.Three, game.ActionHit},
		{[]deck.Rank{deck.Ace, deck.Two}, 13, deck.Four, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Two}, 13, deck.Five, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Two}, 13, deck.Six, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Two}, 13, deck.Seven, game.ActionHit},
		{[]deck.Rank{deck.Ace, deck.Two}, 13, deck.Ace, game.ActionHit},
		// A,3 = soft 14: same as A,2
		{[]deck.Rank{deck.Ace, deck.Three}, 14, deck.Four, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Three}, 14, deck.Five, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Three}, 14, deck.Ten, game.ActionHit},
		// A,4 = soft 15: double vs 3-6
		{[]deck.Rank{deck.Ace, deck.Four}, 15, deck.Three, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Four}, 15, deck.Six, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Four}, 15, deck.Two, game.ActionHit},
		{[]deck.Rank{deck.Ace, deck.Four}, 15, deck.King, game.ActionHit},
		// A,5 = soft 16: double vs 3-6
		{[]deck.Rank{deck.Ace, deck.Five}, 16, deck.Four, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Five}, 16, deck.Seven, game.ActionHit},
		// A,6 = soft 17: double vs 3-6, hit vs 2 and 7+
		{[]deck.Rank{deck.Ace, deck.Six}, 17, deck.Two, game.ActionHit},
		{[]deck.Rank{deck.Ace, deck.Six}, 17, deck.Three, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Six}, 17, deck.Four, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Six}, 17, deck.Five, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Six}, 17, deck.Six, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Six}, 17, deck.Seven, game.ActionHit},
		{[]deck.Rank{deck.Ace, deck.Six}, 17, deck.Ace, game.ActionHit},
		// A,7 = soft 18: stand vs 2,7,8; double vs 3-6; hit vs 9,10,A
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Two, game.ActionStand},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Three, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Four, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Five, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Six, game.ActionDouble},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Seven, game.ActionStand},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Eight, game.ActionStand},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Nine, game.ActionHit},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Ten, game.ActionHit},
		{[]deck.Rank{deck.Ace, deck.Seven}, 18, deck.Ace, game.ActionHit},
		// A,8 = soft 19: always stand
		{[]deck.Rank{deck.Ace, deck.Eight}, 19, deck.Six, game.ActionStand},
		{[]deck.Rank{deck.Ace, deck.Eight}, 19, deck.Ace, game.ActionStand},
		// A,9 = soft 20: always stand
		{[]deck.Rank{deck.Ace, deck.Nine}, 20, deck.Ten, game.ActionStand},
		{[]deck.Rank{deck.Ace, deck.Nine}, 20, deck.Ace, game.ActionStand},
	}

	for _, c := range cases {
		hand := makeTestHand(c.handRanks...)
		got := strategy.OptimalAction(hand, dealerCard(c.up), true, false)
		if got != c.want {
			t.Errorf("soft %d vs %s: want %v, got %v", c.softTotal, c.up, c.want, got)
		}
	}
}

func TestPairsMatrix(t *testing.T) {
	type exp struct {
		handRanks []deck.Rank
		up        deck.Rank
		want      game.Action
	}

	cases := []exp{
		// 2,2: split vs 2-6 (DAS), hit vs 7+
		{[]deck.Rank{deck.Two, deck.Two}, deck.Two, game.ActionSplit},
		{[]deck.Rank{deck.Two, deck.Two}, deck.Six, game.ActionSplit},
		{[]deck.Rank{deck.Two, deck.Two}, deck.Seven, game.ActionHit},
		{[]deck.Rank{deck.Two, deck.Two}, deck.Ace, game.ActionHit},
		// 3,3: same as 2,2 with DAS
		{[]deck.Rank{deck.Three, deck.Three}, deck.Five, game.ActionSplit},
		{[]deck.Rank{deck.Three, deck.Three}, deck.Nine, game.ActionHit},
		// 4,4: split vs 5-6 (DAS), hit otherwise
		{[]deck.Rank{deck.Four, deck.Four}, deck.Four, game.ActionHit},
		{[]deck.Rank{deck.Four, deck.Four}, deck.Five, game.ActionSplit},
		{[]deck.Rank{deck.Four, deck.Four}, deck.Six, game.ActionSplit},
		{[]deck.Rank{deck.Four, deck.Four}, deck.Seven, game.ActionHit},
		// 5,5: never split (treat as hard 10)
		{[]deck.Rank{deck.Five, deck.Five}, deck.Two, game.ActionDouble},
		{[]deck.Rank{deck.Five, deck.Five}, deck.Nine, game.ActionDouble},
		{[]deck.Rank{deck.Five, deck.Five}, deck.Ten, game.ActionHit},
		{[]deck.Rank{deck.Five, deck.Five}, deck.Ace, game.ActionHit},
		// 6,6: split vs 2-6, hit vs 7+
		{[]deck.Rank{deck.Six, deck.Six}, deck.Three, game.ActionSplit},
		{[]deck.Rank{deck.Six, deck.Six}, deck.Seven, game.ActionHit},
		// 7,7: split vs 2-7, hit vs 8+
		{[]deck.Rank{deck.Seven, deck.Seven}, deck.Six, game.ActionSplit},
		{[]deck.Rank{deck.Seven, deck.Seven}, deck.Seven, game.ActionSplit},
		{[]deck.Rank{deck.Seven, deck.Seven}, deck.Eight, game.ActionHit},
		// 8,8: always split
		{[]deck.Rank{deck.Eight, deck.Eight}, deck.Ten, game.ActionSplit},
		{[]deck.Rank{deck.Eight, deck.Eight}, deck.Ace, game.ActionSplit},
		// 9,9: split vs 2-6,8,9; stand vs 7,10,A
		{[]deck.Rank{deck.Nine, deck.Nine}, deck.Two, game.ActionSplit},
		{[]deck.Rank{deck.Nine, deck.Nine}, deck.Six, game.ActionSplit},
		{[]deck.Rank{deck.Nine, deck.Nine}, deck.Seven, game.ActionStand},
		{[]deck.Rank{deck.Nine, deck.Nine}, deck.Eight, game.ActionSplit},
		{[]deck.Rank{deck.Nine, deck.Nine}, deck.Nine, game.ActionSplit},
		{[]deck.Rank{deck.Nine, deck.Nine}, deck.Ten, game.ActionStand},
		{[]deck.Rank{deck.Nine, deck.Nine}, deck.Ace, game.ActionStand},
		// 10,10: always stand
		{[]deck.Rank{deck.Ten, deck.Ten}, deck.Five, game.ActionStand},
		{[]deck.Rank{deck.Ten, deck.Ten}, deck.Ace, game.ActionStand},
		{[]deck.Rank{deck.King, deck.Queen}, deck.Six, game.ActionStand},
		// A,A: always split
		{[]deck.Rank{deck.Ace, deck.Ace}, deck.Ten, game.ActionSplit},
		{[]deck.Rank{deck.Ace, deck.Ace}, deck.Ace, game.ActionSplit},
	}

	for _, c := range cases {
		hand := makeTestHand(c.handRanks...)
		got := strategy.OptimalAction(hand, dealerCard(c.up), true, true)
		if got != c.want {
			t.Errorf("pair %s vs %s: want %v, got %v", hand.String(), c.up, c.want, got)
		}
	}
}

func TestFallbackWhenDoubleNotAllowed(t *testing.T) {
	// Hard 11 vs 6 would double, but fallback to hit when can't double
	hand := buildHandTotal(11, false)
	got := strategy.OptimalAction(hand, dealerCard(deck.Six), false, false)
	if got != game.ActionHit {
		t.Errorf("hard 11 vs 6 (no double): want Hit, got %v", got)
	}

	// Soft 18 (A,7) vs 4 would double, fallback to Stand
	softHand := makeTestHand(deck.Ace, deck.Seven)
	got = strategy.OptimalAction(softHand, dealerCard(deck.Four), false, false)
	if got != game.ActionStand {
		t.Errorf("A7 vs 4 (no double): want Stand, got %v", got)
	}

	// Soft 15 (A,4) vs 5 would double, fallback to Hit
	softHand = makeTestHand(deck.Ace, deck.Four)
	got = strategy.OptimalAction(softHand, dealerCard(deck.Five), false, false)
	if got != game.ActionHit {
		t.Errorf("A4 vs 5 (no double): want Hit, got %v", got)
	}
}

func TestFallbackWhenSplitNotAllowed(t *testing.T) {
	// 8,8 vs 10 would split, but without split allowed should fall back to hard 16 => Hit
	hand := makeTestHand(deck.Eight, deck.Eight)
	got := strategy.OptimalAction(hand, dealerCard(deck.Ten), false, false)
	if got != game.ActionHit {
		t.Errorf("8,8 vs 10 (no split): want Hit, got %v", got)
	}

	// A,A vs 6 would split, fallback should treat as soft 12 -> hard 12 => Hit
	hand = makeTestHand(deck.Ace, deck.Ace)
	got = strategy.OptimalAction(hand, dealerCard(deck.Six), false, false)
	if got != game.ActionHit {
		t.Errorf("A,A vs 6 (no split): want Hit, got %v", got)
	}
}

func TestFiveFivePairFallbackToHardTen(t *testing.T) {
	// 5,5 vs 6 should double (as hard 10)
	hand := makeTestHand(deck.Five, deck.Five)
	got := strategy.OptimalAction(hand, dealerCard(deck.Six), true, true)
	if got != game.ActionDouble {
		t.Errorf("5,5 vs 6: want Double, got %v", got)
	}
}

// buildHandTotal builds a non-pair hand that sums to target with given softness.
func buildHandTotal(total int, soft bool) *game.Hand {
	h := game.NewHand(10)
	if soft {
		// A + (total-11) cards
		rest := total - 11
		h.AddCard(deck.NewCard(deck.Spades, deck.Ace))
		if rest > 0 {
			h.AddCard(deck.NewCard(deck.Hearts, deck.Rank(rest)))
		}
		return h
	}
	// Build hard total using non-pair cards
	switch total {
	case 8:
		h.AddCard(deck.NewCard(deck.Spades, deck.Five))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Three))
	case 9:
		h.AddCard(deck.NewCard(deck.Spades, deck.Five))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Four))
	case 10:
		h.AddCard(deck.NewCard(deck.Spades, deck.Six))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Four))
	case 11:
		h.AddCard(deck.NewCard(deck.Spades, deck.Eight))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Three))
	case 12:
		h.AddCard(deck.NewCard(deck.Spades, deck.Eight))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Four))
	case 13:
		h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Three))
	case 14:
		h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Four))
	case 15:
		h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Five))
	case 16:
		h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Six))
	case 17:
		h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Seven))
	case 18:
		h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Eight))
	case 19:
		h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Nine))
	case 20:
		h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
		h.AddCard(deck.NewCard(deck.Hearts, deck.King))
	case 21:
		h.AddCard(deck.NewCard(deck.Spades, deck.Seven))
		h.AddCard(deck.NewCard(deck.Hearts, deck.Seven))
		h.AddCard(deck.NewCard(deck.Diamonds, deck.Seven))
	}
	return h
}

func TestStrategyAgainstLiveEngineIntegration(t *testing.T) {
	// Smoke test that the strategy functions work against a live engine and shoe.
	rng := rand.New(rand.NewSource(7))
	shoe, _ := deck.NewShoeWithRNG(6, 0.75, rng)
	rules := game.DefaultRules()
	engine := game.NewEngineWithShoe(rules, 10000, shoe)

	if err := engine.StartRound(10); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Iterate hands with strategy
	for !engine.AllHandsResolved() {
		hand := engine.CurrentHand()
		if hand == nil {
			break
		}
		action := strategy.OptimalAction(hand, engine.DealerUpcard(), hand.CanDouble(), hand.CanSplit())
		if err := engine.ApplyAction(action); err != nil {
			// If action fails (e.g., cannot split more), try again with restrictions
			action = strategy.OptimalAction(hand, engine.DealerUpcard(), false, false)
			if err := engine.ApplyAction(action); err != nil {
				_ = engine.ApplyAction(game.ActionStand)
			}
		}
	}

	results := engine.ResolveRound()
	if len(results) == 0 {
		t.Errorf("expected results after integration round")
	}
}
