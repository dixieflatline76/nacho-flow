package oracle

import (
	"blackjack/pkg/cards"
	"blackjack/pkg/engine"
	"blackjack/pkg/rules"
	"testing"
)

func TestCardCounter_Systems(t *testing.T) {
	// 1. Hi-Lo Counter
	hilo := NewCardCounter(HiLo, 6)
	if hilo.RunningCount() != 0 {
		t.Errorf("HiLo initial count should be 0, got %d", hilo.RunningCount())
	}
	if hilo.system.String() != "Hi-Lo" {
		t.Errorf("Expected Hi-Lo string")
	}

	// Deal +1 cards (2, 3, 4, 5, 6)
	hilo.ObserveCard(cards.NewCard(cards.Two, cards.Hearts))
	hilo.ObserveCard(cards.NewCard(cards.Three, cards.Spades))
	hilo.ObserveCard(cards.NewCard(cards.Four, cards.Clubs))
	hilo.ObserveCard(cards.NewCard(cards.Five, cards.Diamonds))
	hilo.ObserveCard(cards.NewCard(cards.Six, cards.Hearts))
	if hilo.RunningCount() != 5 {
		t.Errorf("Expected RC 5, got %d", hilo.RunningCount())
	}

	// Deal 0 cards (7, 8, 9)
	hilo.ObserveCard(cards.NewCard(cards.Seven, cards.Hearts))
	hilo.ObserveCard(cards.NewCard(cards.Eight, cards.Hearts))
	hilo.ObserveCard(cards.NewCard(cards.Nine, cards.Hearts))
	if hilo.RunningCount() != 5 {
		t.Errorf("Expected RC 5 after neutral cards, got %d", hilo.RunningCount())
	}

	// Deal -1 cards (10, J, Q, K, A)
	hilo.ObserveCard(cards.NewCard(cards.Ten, cards.Hearts))
	hilo.ObserveCard(cards.NewCard(cards.Jack, cards.Hearts))
	hilo.ObserveCard(cards.NewCard(cards.Queen, cards.Hearts))
	hilo.ObserveCard(cards.NewCard(cards.King, cards.Hearts))
	hilo.ObserveCard(cards.NewCard(cards.Ace, cards.Hearts))
	if hilo.RunningCount() != 0 {
		t.Errorf("Expected RC 0 after full cycle, got %d", hilo.RunningCount())
	}

	// 2. KO Counter (Unbalanced)
	ko := NewCardCounter(KO, 6)
	// Initial KO for 6 decks = 4 - (4*6) = -20
	if ko.RunningCount() != -20 {
		t.Errorf("Expected KO initial count -20, got %d", ko.RunningCount())
	}
	if ko.system.String() != "KO (Knock-Out)" {
		t.Errorf("Expected KO string")
	}
	// Seven is +1 in KO
	ko.ObserveCard(cards.NewCard(cards.Seven, cards.Diamonds))
	if ko.RunningCount() != -19 {
		t.Errorf("Expected KO count -19, got %d", ko.RunningCount())
	}
	// In KO, TrueCount returns running count
	if ko.TrueCount() != -19 {
		t.Errorf("Expected KO true count to equal RC -19, got %f", ko.TrueCount())
	}

	// 3. Omega II Counter
	omega := NewCardCounter(OmegaII, 6)
	if omega.RunningCount() != 0 {
		t.Errorf("Omega II initial count should be 0")
	}
	if omega.system.String() != "Omega II" {
		t.Errorf("Expected Omega II string")
	}
	// 4,5,6 are +2
	omega.ObserveCard(cards.NewCard(cards.Five, cards.Hearts))
	if omega.RunningCount() != 2 {
		t.Errorf("Omega II 5 should count as +2, got %d", omega.RunningCount())
	}
	// 10 is -2
	omega.ObserveCard(cards.NewCard(cards.King, cards.Clubs))
	if omega.RunningCount() != 0 {
		t.Errorf("Omega II K should count as -2, got %d", omega.RunningCount())
	}

	// Bounds test on NewCardCounter
	c0 := NewCardCounter(HiLo, 0)
	if c0.initialDecks != 1 {
		t.Errorf("Expected 1 deck fallback")
	}

	// Unknown system string
	if CountingSystem(99).String() != "Unknown" {
		t.Errorf("Expected Unknown system string")
	}
}

func TestCardCounter_TrueCountAndBetSizing(t *testing.T) {
	hilo := NewCardCounter(HiLo, 4) // 4 decks = 208 cards

	// Feed 104 high cards -> 2 decks remaining, RC = +10
	hilo.runningCount = 10
	hilo.totalCardsSeen = 104 // 2 decks left

	remDecks := hilo.RemainingDecks()
	if remDecks != 2.0 {
		t.Errorf("Expected 2.0 remaining decks, got %f", remDecks)
	}

	tc := hilo.TrueCount()
	if tc != 5.0 {
		t.Errorf("Expected TC +5.0, got %f", tc)
	}

	if hilo.TrueCountRounded() != 5 {
		t.Errorf("Expected TrueCountRounded = 5, got %d", hilo.TrueCountRounded())
	}

	pen := hilo.ShoePenetration()
	if pen != 0.5 {
		t.Errorf("Expected 50%% penetration, got %f", pen)
	}

	mult := hilo.RecommendedBetMultiplier()
	if mult != 8 {
		t.Errorf("Expected max bet multiplier 8 at TC +5, got %d", mult)
	}

	// Test lower multipliers
	hilo.runningCount = 2
	hilo.totalCardsSeen = 0 // 4 decks left -> TC = 0.5
	if m := hilo.RecommendedBetMultiplier(); m != 1 {
		t.Errorf("Expected multiplier 1 at TC 0.5, got %d", m)
	}
	hilo.runningCount = 6 // TC = 1.5
	if m := hilo.RecommendedBetMultiplier(); m != 2 {
		t.Errorf("Expected multiplier 2 at TC 1.5, got %d", m)
	}
	hilo.runningCount = 10 // TC = 2.5
	if m := hilo.RecommendedBetMultiplier(); m != 4 {
		t.Errorf("Expected multiplier 4 at TC 2.5, got %d", m)
	}
	hilo.runningCount = 14 // TC = 3.5
	if m := hilo.RecommendedBetMultiplier(); m != 6 {
		t.Errorf("Expected multiplier 6 at TC 3.5, got %d", m)
	}
}

func TestStrategyAdvisor_HardTotals(t *testing.T) {
	advisor := NewStrategyAdvisor(rules.VegasStrip())

	tests := []struct {
		name        string
		playerCards []cards.Card
		dealerCard  cards.Card
		expectedAct engine.PlayerAction
	}{
		{
			name:        "Hard 16 vs Dealer 10 (Late Surrender)",
			playerCards: []cards.Card{cards.NewCard(cards.Ten, cards.Hearts), cards.NewCard(cards.Six, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Ten, cards.Spades),
			expectedAct: engine.ActionSurrender,
		},
		{
			name:        "Hard 15 vs Dealer 10 (Late Surrender)",
			playerCards: []cards.Card{cards.NewCard(cards.Ten, cards.Hearts), cards.NewCard(cards.Five, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Ten, cards.Spades),
			expectedAct: engine.ActionSurrender,
		},
		{
			name:        "Hard 16 vs Dealer 6 (Stand)",
			playerCards: []cards.Card{cards.NewCard(cards.Ten, cards.Hearts), cards.NewCard(cards.Six, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Six, cards.Spades),
			expectedAct: engine.ActionStand,
		},
		{
			name:        "Hard 11 vs Dealer 6 (Double)",
			playerCards: []cards.Card{cards.NewCard(cards.Five, cards.Hearts), cards.NewCard(cards.Six, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Six, cards.Spades),
			expectedAct: engine.ActionDouble,
		},
		{
			name:        "Hard 12 vs Dealer 4 (Stand)",
			playerCards: []cards.Card{cards.NewCard(cards.Ten, cards.Hearts), cards.NewCard(cards.Two, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Four, cards.Spades),
			expectedAct: engine.ActionStand,
		},
		{
			name:        "Hard 12 vs Dealer 2 (Hit)",
			playerCards: []cards.Card{cards.NewCard(cards.Ten, cards.Hearts), cards.NewCard(cards.Two, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Two, cards.Spades),
			expectedAct: engine.ActionHit,
		},
		{
			name:        "Hard 10 vs Dealer 9 (Double)",
			playerCards: []cards.Card{cards.NewCard(cards.Six, cards.Hearts), cards.NewCard(cards.Four, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Nine, cards.Spades),
			expectedAct: engine.ActionDouble,
		},
		{
			name:        "Hard 9 vs Dealer 3 (Double)",
			playerCards: []cards.Card{cards.NewCard(cards.Five, cards.Hearts), cards.NewCard(cards.Four, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Three, cards.Spades),
			expectedAct: engine.ActionDouble,
		},
		{
			name:        "Hard 8 vs Dealer 5 (Hit)",
			playerCards: []cards.Card{cards.NewCard(cards.Five, cards.Hearts), cards.NewCard(cards.Three, cards.Clubs)},
			dealerCard:  cards.NewCard(cards.Five, cards.Spades),
			expectedAct: engine.ActionHit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := engine.NewHand(10)
			for _, c := range tt.playerCards {
				h.AddCard(c)
			}
			rec := advisor.Advise(h, tt.dealerCard, 0)
			if rec.Action != tt.expectedAct {
				t.Errorf("Expected action %s, got %s (%s)", tt.expectedAct, rec.Action, rec.Description)
			}
		})
	}

	// Empty hand advise
	if rec := advisor.Advise(nil, cards.NewCard(cards.Ace, cards.Hearts), 0); rec.Action != engine.ActionStand {
		t.Errorf("Expected stand on nil hand")
	}
}

func TestStrategyAdvisor_SoftAndPairSplits(t *testing.T) {
	advisor := NewStrategyAdvisor(rules.VegasStrip())

	// Split Aces
	hAces := engine.NewHand(10)
	hAces.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	hAces.AddCard(cards.NewCard(cards.Ace, cards.Spades))
	rec := advisor.Advise(hAces, cards.NewCard(cards.Ace, cards.Clubs), 0)
	if rec.Action != engine.ActionSplit {
		t.Errorf("Aces should always split, got %s", rec.Action)
	}

	// Split 8s vs Dealer 10
	h8s := engine.NewHand(10)
	h8s.AddCard(cards.NewCard(cards.Eight, cards.Hearts))
	h8s.AddCard(cards.NewCard(cards.Eight, cards.Spades))
	rec = advisor.Advise(h8s, cards.NewCard(cards.Ten, cards.Clubs), 0)
	if rec.Action != engine.ActionSplit {
		t.Errorf("8s should split vs 10, got %s", rec.Action)
	}

	// Split 9s vs 9
	h9s := engine.NewHand(10)
	h9s.AddCard(cards.NewCard(cards.Nine, cards.Hearts))
	h9s.AddCard(cards.NewCard(cards.Nine, cards.Spades))
	rec = advisor.Advise(h9s, cards.NewCard(cards.Nine, cards.Clubs), 0)
	if rec.Action != engine.ActionSplit {
		t.Errorf("9s vs 9 should split, got %s", rec.Action)
	}

	// Split 7s vs 7
	h7s := engine.NewHand(10)
	h7s.AddCard(cards.NewCard(cards.Seven, cards.Hearts))
	h7s.AddCard(cards.NewCard(cards.Seven, cards.Spades))
	rec = advisor.Advise(h7s, cards.NewCard(cards.Seven, cards.Clubs), 0)
	if rec.Action != engine.ActionSplit {
		t.Errorf("7s vs 7 should split, got %s", rec.Action)
	}

	// Split 6s vs 5
	h6s := engine.NewHand(10)
	h6s.AddCard(cards.NewCard(cards.Six, cards.Hearts))
	h6s.AddCard(cards.NewCard(cards.Six, cards.Spades))
	rec = advisor.Advise(h6s, cards.NewCard(cards.Five, cards.Clubs), 0)
	if rec.Action != engine.ActionSplit {
		t.Errorf("6s vs 5 should split, got %s", rec.Action)
	}

	// Pair 5s vs 5 -> Double (not split)
	h5s := engine.NewHand(10)
	h5s.AddCard(cards.NewCard(cards.Five, cards.Hearts))
	h5s.AddCard(cards.NewCard(cards.Five, cards.Spades))
	rec = advisor.Advise(h5s, cards.NewCard(cards.Five, cards.Clubs), 0)
	if rec.Action != engine.ActionDouble {
		t.Errorf("5s vs 5 should double, got %s", rec.Action)
	}

	// Pair 4s vs 5 with DAS -> Split
	h4s := engine.NewHand(10)
	h4s.AddCard(cards.NewCard(cards.Four, cards.Hearts))
	h4s.AddCard(cards.NewCard(cards.Four, cards.Spades))
	rec = advisor.Advise(h4s, cards.NewCard(cards.Five, cards.Clubs), 0)
	if rec.Action != engine.ActionSplit {
		t.Errorf("4s vs 5 with DAS should split, got %s", rec.Action)
	}

	// Pair 2s vs 5 -> Split
	h2s := engine.NewHand(10)
	h2s.AddCard(cards.NewCard(cards.Two, cards.Hearts))
	h2s.AddCard(cards.NewCard(cards.Two, cards.Spades))
	rec = advisor.Advise(h2s, cards.NewCard(cards.Five, cards.Clubs), 0)
	if rec.Action != engine.ActionSplit {
		t.Errorf("2s vs 5 should split, got %s", rec.Action)
	}

	// Pair 10s -> Stand
	h10s := engine.NewHand(10)
	h10s.AddCard(cards.NewCard(cards.Ten, cards.Hearts))
	h10s.AddCard(cards.NewCard(cards.Ten, cards.Spades))
	rec = advisor.Advise(h10s, cards.NewCard(cards.Six, cards.Clubs), 0)
	if rec.Action != engine.ActionStand {
		t.Errorf("10s should stand, got %s", rec.Action)
	}

	// Soft 19 vs 6 on H17 -> Double
	h17Rules := rules.VegasStrip()
	h17Rules.DealerHitsSoft17 = true
	advH17 := NewStrategyAdvisor(h17Rules)
	hSoft19 := engine.NewHand(10)
	hSoft19.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	hSoft19.AddCard(cards.NewCard(cards.Eight, cards.Spades))
	rec = advH17.Advise(hSoft19, cards.NewCard(cards.Six, cards.Clubs), 0)
	if rec.Action != engine.ActionDouble {
		t.Errorf("Soft 19 vs 6 on H17 should double, got %s", rec.Action)
	}

	// Stand Soft 20 (A,9)
	hSoft20 := engine.NewHand(10)
	hSoft20.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	hSoft20.AddCard(cards.NewCard(cards.Nine, cards.Spades))
	rec = advisor.Advise(hSoft20, cards.NewCard(cards.Six, cards.Clubs), 0)
	if rec.Action != engine.ActionStand {
		t.Errorf("Soft 20 should stand, got %s", rec.Action)
	}

	// Double Soft 18 (A,7) vs Dealer 6
	hSoft18 := engine.NewHand(10)
	hSoft18.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	hSoft18.AddCard(cards.NewCard(cards.Seven, cards.Spades))
	rec = advisor.Advise(hSoft18, cards.NewCard(cards.Six, cards.Clubs), 0)
	if rec.Action != engine.ActionDouble {
		t.Errorf("Soft 18 vs 6 should double, got %s", rec.Action)
	}

	// Hit Soft 18 (A,7) vs Dealer 9
	rec = advisor.Advise(hSoft18, cards.NewCard(cards.Nine, cards.Clubs), 0)
	if rec.Action != engine.ActionHit {
		t.Errorf("Soft 18 vs 9 should hit, got %s", rec.Action)
	}

	// Double Soft 17 (A,6) vs Dealer 4
	hSoft17 := engine.NewHand(10)
	hSoft17.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	hSoft17.AddCard(cards.NewCard(cards.Six, cards.Spades))
	rec = advisor.Advise(hSoft17, cards.NewCard(cards.Four, cards.Clubs), 0)
	if rec.Action != engine.ActionDouble {
		t.Errorf("Soft 17 vs 4 should double, got %s", rec.Action)
	}

	// Double Soft 15 (A,4) vs Dealer 5
	hSoft15 := engine.NewHand(10)
	hSoft15.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	hSoft15.AddCard(cards.NewCard(cards.Four, cards.Spades))
	rec = advisor.Advise(hSoft15, cards.NewCard(cards.Five, cards.Clubs), 0)
	if rec.Action != engine.ActionDouble {
		t.Errorf("Soft 15 vs 5 should double, got %s", rec.Action)
	}

	// Double Soft 13 (A,2) vs Dealer 5
	hSoft13 := engine.NewHand(10)
	hSoft13.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	hSoft13.AddCard(cards.NewCard(cards.Two, cards.Spades))
	rec = advisor.Advise(hSoft13, cards.NewCard(cards.Five, cards.Clubs), 0)
	if rec.Action != engine.ActionDouble {
		t.Errorf("Soft 13 vs 5 should double, got %s", rec.Action)
	}

	// Surrender Hard 17 vs Ace on H17
	hHard17 := engine.NewHand(10)
	hHard17.AddCard(cards.NewCard(cards.Ten, cards.Hearts))
	hHard17.AddCard(cards.NewCard(cards.Seven, cards.Spades))
	rec = advH17.Advise(hHard17, cards.NewCard(cards.Ace, cards.Clubs), 0)
	if rec.Action != engine.ActionSurrender {
		t.Errorf("Hard 17 vs Ace on H17 should surrender, got %s", rec.Action)
	}

	// Test Non-DAS split fallback
	noDasRule := rules.VegasStrip()
	noDasRule.DoubleAfterSplit = false
	advNoDas := NewStrategyAdvisor(noDasRule)
	rec = advNoDas.Advise(h6s, cards.NewCard(cards.Two, cards.Clubs), 0)
	if rec.Action == engine.ActionSplit {
		t.Errorf("6s vs 2 should NOT split without DAS")
	}
	rec = advNoDas.Advise(h4s, cards.NewCard(cards.Five, cards.Clubs), 0)
	if rec.Action == engine.ActionSplit {
		t.Errorf("4s vs 5 should NOT split without DAS")
	}
	rec = advNoDas.Advise(h2s, cards.NewCard(cards.Two, cards.Clubs), 0)
	if rec.Action == engine.ActionSplit {
		t.Errorf("2s vs 2 should NOT split without DAS")
	}
}

func TestStrategyAdvisor_Insurance(t *testing.T) {
	advisor := NewStrategyAdvisor(rules.VegasStrip())

	// TC < +3.0 -> Decline
	accept, _ := advisor.AdviseInsurance(2.0)
	if accept {
		t.Errorf("Should decline insurance at TC +2.0")
	}

	// TC >= +3.0 -> Accept
	accept, _ = advisor.AdviseInsurance(3.5)
	if !accept {
		t.Errorf("Should accept insurance at TC +3.5")
	}
}

func TestCardCounter_CardValues(t *testing.T) {
	tests := []struct {
		system   CountingSystem
		cards    []cards.Rank
		expected []int
	}{
		{
			system:   HiLo,
			cards:    []cards.Rank{cards.Two, cards.Three, cards.Four, cards.Five, cards.Six, cards.Seven, cards.Eight, cards.Nine, cards.Ten, cards.Jack, cards.Queen, cards.King, cards.Ace},
			expected: []int{1, 1, 1, 1, 1, 0, 0, 0, -1, -1, -1, -1, -1},
		},
		{
			system:   KO,
			cards:    []cards.Rank{cards.Two, cards.Three, cards.Four, cards.Five, cards.Six, cards.Seven, cards.Eight, cards.Nine, cards.Ten, cards.Jack, cards.Queen, cards.King, cards.Ace},
			expected: []int{1, 1, 1, 1, 1, 1, 0, 0, -1, -1, -1, -1, -1},
		},
		{
			system:   OmegaII,
			cards:    []cards.Rank{cards.Two, cards.Three, cards.Four, cards.Five, cards.Six, cards.Seven, cards.Eight, cards.Nine, cards.Ten, cards.Jack, cards.Queen, cards.King, cards.Ace},
			expected: []int{1, 1, 2, 2, 2, 1, 0, -1, -2, -2, -2, -2, 0},
		},
		{
			system:   CountingSystem(99),
			cards:    []cards.Rank{cards.Two},
			expected: []int{0},
		},
	}

	for _, tt := range tests {
		counter := NewCardCounter(tt.system, 6)
		for i, rank := range tt.cards {
			c := cards.NewCard(rank, cards.Spades)
			if got := counter.CardValue(c); got != tt.expected[i] {
				t.Errorf("System %s, Card %s: expected %d, got %d", tt.system, rank, tt.expected[i], got)
			}
		}
	}
}

func TestStrategyAdvisor_CanDoubleRule(t *testing.T) {
	r := rules.VegasStrip()

	// Test double restriction AnyTwoCards
	r.DoubleRestriction = rules.DoubleAny2Cards
	sa := NewStrategyAdvisor(r)
	h1 := engine.NewHand(10)
	h1.AddCard(cards.NewCard(cards.Two, cards.Spades))
	h1.AddCard(cards.NewCard(cards.Three, cards.Hearts))
	if !sa.canDoubleRule(h1) {
		t.Error("Expected canDoubleRule to be true for AnyTwoCards")
	}

	// Test double restriction Double9To11
	r.DoubleRestriction = rules.Double9To11Only
	sa = NewStrategyAdvisor(r)
	h := engine.NewHand(10)
	h.AddCard(cards.NewCard(cards.Four, cards.Spades))
	h.AddCard(cards.NewCard(cards.Five, cards.Hearts)) // total 9
	if !sa.canDoubleRule(h) {
		t.Error("Expected canDoubleRule to be true for Double9To11 with total 9")
	}
	h2 := engine.NewHand(10)
	h2.AddCard(cards.NewCard(cards.Four, cards.Spades))
	h2.AddCard(cards.NewCard(cards.Four, cards.Hearts)) // total 8
	if sa.canDoubleRule(h2) {
		t.Error("Expected canDoubleRule to be false for Double9To11 with total 8")
	}

	// Test double restriction Double10To11
	r.DoubleRestriction = rules.Double10Or11Only
	sa = NewStrategyAdvisor(r)
	h3 := engine.NewHand(10)
	h3.AddCard(cards.NewCard(cards.Five, cards.Spades))
	h3.AddCard(cards.NewCard(cards.Five, cards.Hearts)) // total 10
	if !sa.canDoubleRule(h3) {
		t.Error("Expected canDoubleRule to be true for Double10To11 with total 10")
	}
	h4 := engine.NewHand(10)
	h4.AddCard(cards.NewCard(cards.Four, cards.Spades))
	h4.AddCard(cards.NewCard(cards.Five, cards.Hearts)) // total 9
	if sa.canDoubleRule(h4) {
		t.Error("Expected canDoubleRule to be false for Double10To11 with total 9")
	}
}

func TestStrategyAdvisor_AdviseSoft(t *testing.T) {
	r := rules.VegasStrip()
	r.DoubleRestriction = rules.DoubleAny2Cards
	sa := NewStrategyAdvisor(r)

	// Soft 18 vs Dealer 3 (Double/Stand)
	h1 := engine.NewHand(10)
	h1.AddCard(cards.NewCard(cards.Ace, cards.Spades))
	h1.AddCard(cards.NewCard(cards.Seven, cards.Hearts))
	if adv := sa.Advise(h1, cards.NewCard(cards.Three, cards.Spades), 0); adv.Action != engine.ActionDouble {
		t.Errorf("Soft 18 vs 3: expected %s, got %s", engine.ActionDouble, adv.Action)
	}

	// Soft 18 vs Dealer 3 but cannot double (Stand)
	r.DoubleRestriction = rules.Double10Or11Only // Cannot double soft 18
	sa2 := NewStrategyAdvisor(r)
	if adv := sa2.Advise(h1, cards.NewCard(cards.Three, cards.Spades), 0); adv.Action != engine.ActionStand {
		t.Errorf("Soft 18 vs 3 (no double): expected %s, got %s", engine.ActionStand, adv.Action)
	}

	// Soft 18 vs Dealer 7 (Stand)
	if adv := sa2.Advise(h1, cards.NewCard(cards.Seven, cards.Spades), 0); adv.Action != engine.ActionStand {
		t.Errorf("Soft 18 vs 7: expected %s, got %s", engine.ActionStand, adv.Action)
	}

	// Soft 18 vs Dealer 9 (Hit)
	if adv := sa2.Advise(h1, cards.NewCard(cards.Nine, cards.Spades), 0); adv.Action != engine.ActionHit {
		t.Errorf("Soft 18 vs 9: expected %s, got %s", engine.ActionHit, adv.Action)
	}

	// Soft 19 vs Dealer 6 on H17 (Double/Stand)
	h2 := engine.NewHand(10)
	h2.AddCard(cards.NewCard(cards.Ace, cards.Spades))
	h2.AddCard(cards.NewCard(cards.Eight, cards.Hearts))
	h17RulesForSoft19 := rules.VegasStrip()
	h17RulesForSoft19.DealerHitsSoft17 = true
	sa3 := NewStrategyAdvisor(h17RulesForSoft19)
	if adv := sa3.Advise(h2, cards.NewCard(cards.Six, cards.Spades), 0); adv.Action != engine.ActionDouble {
		t.Errorf("Soft 19 vs 6 on H17: expected %s, got %s", engine.ActionDouble, adv.Action)
	}

	// Soft 13 vs Dealer 6 (Double/Hit)
	h3 := engine.NewHand(10)
	h3.AddCard(cards.NewCard(cards.Ace, cards.Spades))
	h3.AddCard(cards.NewCard(cards.Two, cards.Hearts))
	if adv := sa3.Advise(h3, cards.NewCard(cards.Six, cards.Spades), 0); adv.Action != engine.ActionDouble {
		t.Errorf("Soft 13 vs 6: expected %s, got %s", engine.ActionDouble, adv.Action)
	}

	// Soft 13 vs Dealer 4 (Hit)
	if adv := sa3.Advise(h3, cards.NewCard(cards.Four, cards.Spades), 0); adv.Action != engine.ActionHit {
		t.Errorf("Soft 13 vs 4: expected %s, got %s", engine.ActionHit, adv.Action)
	}

	// 3+ card soft hands (cannot double)
	h4 := engine.NewHand(10)
	h4.AddCard(cards.NewCard(cards.Ace, cards.Spades))
	h4.AddCard(cards.NewCard(cards.Two, cards.Hearts))
	h4.AddCard(cards.NewCard(cards.Three, cards.Clubs)) // Soft 16 (A,2,3)
	if adv := sa3.Advise(h4, cards.NewCard(cards.Six, cards.Spades), 0); adv.Action != engine.ActionHit {
		t.Errorf("Soft 16 (3 cards) vs 6: expected %s, got %s", engine.ActionHit, adv.Action)
	}
}

func TestStrategyAdvisor_AdviseHard(t *testing.T) {
	r := rules.VegasStrip()
	r.DoubleRestriction = rules.DoubleAny2Cards
	sa := NewStrategyAdvisor(r)

	// Hard 16 vs Dealer 10 (Late Surrender)
	h1 := engine.NewHand(10)
	h1.AddCard(cards.NewCard(cards.Ten, cards.Spades))
	h1.AddCard(cards.NewCard(cards.Six, cards.Hearts))
	if adv := sa.Advise(h1, cards.NewCard(cards.Ten, cards.Spades), 0); adv.Action != engine.ActionSurrender {
		t.Errorf("Hard 16 vs 10: expected Surrender, got %s", adv.Action)
	}

	// Hard 16 vs 10 but no surrender
	r2 := rules.VegasStrip()
	r2.Surrender = rules.SurrenderNone
	sa2 := NewStrategyAdvisor(r2)
	if adv := sa2.Advise(h1, cards.NewCard(cards.Ten, cards.Spades), 0); adv.Action != engine.ActionHit {
		t.Errorf("Hard 16 vs 10 (no surrender): expected Hit, got %s", adv.Action)
	}

	// Hard 16 vs Dealer 10 (3+ cards) -> Hit (no surrender)
	h2 := engine.NewHand(10)
	h2.AddCard(cards.NewCard(cards.Ten, cards.Spades))
	h2.AddCard(cards.NewCard(cards.Four, cards.Hearts))
	h2.AddCard(cards.NewCard(cards.Two, cards.Clubs))
	if adv := sa.Advise(h2, cards.NewCard(cards.Ten, cards.Spades), 0); adv.Action != engine.ActionHit {
		t.Errorf("Hard 16 (3 cards) vs 10: expected Hit, got %s", adv.Action)
	}

	// Hard 11 vs 6 (Double/Hit)
	h3 := engine.NewHand(10)
	h3.AddCard(cards.NewCard(cards.Five, cards.Spades))
	h3.AddCard(cards.NewCard(cards.Six, cards.Hearts))
	if adv := sa.Advise(h3, cards.NewCard(cards.Six, cards.Spades), 0); adv.Action != engine.ActionDouble {
		t.Errorf("Hard 11 vs 6: expected Double, got %s", adv.Action)
	}

	// Hard 11 vs 6 but cannot double (Hit)
	r3 := rules.VegasStrip()
	r3.DoubleRestriction = rules.Double10Or11Only
	sa3 := NewStrategyAdvisor(r3)
	h3_3card := engine.NewHand(10)
	h3_3card.AddCard(cards.NewCard(cards.Five, cards.Spades))
	h3_3card.AddCard(cards.NewCard(cards.Four, cards.Hearts))
	h3_3card.AddCard(cards.NewCard(cards.Two, cards.Clubs))
	if adv := sa3.Advise(h3_3card, cards.NewCard(cards.Six, cards.Spades), 0); adv.Action != engine.ActionHit {
		t.Errorf("Hard 11 (3 cards) vs 6: expected Hit, got %s", adv.Action)
	}

	// Hard 15 vs 10 (Surrender/Hit)
	h4 := engine.NewHand(10)
	h4.AddCard(cards.NewCard(cards.Ten, cards.Spades))
	h4.AddCard(cards.NewCard(cards.Five, cards.Hearts))
	if adv := sa.Advise(h4, cards.NewCard(cards.Ten, cards.Spades), 0); adv.Action != engine.ActionSurrender {
		t.Errorf("Hard 15 vs 10: expected Surrender, got %s", adv.Action)
	}
}

func BenchmarkOracleAdvisor(b *testing.B) {
	advisor := NewStrategyAdvisor(rules.VegasStrip())
	h := engine.NewHand(10)
	h.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	h.AddCard(cards.NewCard(cards.Seven, cards.Spades))
	dealerUp := cards.NewCard(cards.Six, cards.Clubs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = advisor.Advise(h, dealerUp, 2.5)
	}
}
