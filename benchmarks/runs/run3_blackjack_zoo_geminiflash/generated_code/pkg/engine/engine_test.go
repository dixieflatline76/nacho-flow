package engine

import (
	"blackjack/pkg/cards"
	"blackjack/pkg/rules"
	"testing"
)

func TestHandScoring(t *testing.T) {
	tests := []struct {
		name         string
		cardsList    []cards.Card
		wantTotal    int
		wantSoft     bool
		wantBJ       bool
		wantBust     bool
		wantCanSplit bool
	}{
		{
			name:         "Natural Blackjack A + K",
			cardsList:    []cards.Card{cards.NewCard(cards.Ace, cards.Spades), cards.NewCard(cards.King, cards.Hearts)},
			wantTotal:    21,
			wantSoft:     true,
			wantBJ:       true,
			wantBust:     false,
			wantCanSplit: false,
		},
		{
			name:         "Pair of 8s",
			cardsList:    []cards.Card{cards.NewCard(cards.Eight, cards.Clubs), cards.NewCard(cards.Eight, cards.Diamonds)},
			wantTotal:    16,
			wantSoft:     false,
			wantBJ:       false,
			wantBust:     false,
			wantCanSplit: true,
		},
		{
			name:         "Ten and Jack pair value split",
			cardsList:    []cards.Card{cards.NewCard(cards.Ten, cards.Clubs), cards.NewCard(cards.Jack, cards.Diamonds)},
			wantTotal:    20,
			wantSoft:     false,
			wantBJ:       false,
			wantBust:     false,
			wantCanSplit: true,
		},
		{
			name:         "Soft 18 (A + 7)",
			cardsList:    []cards.Card{cards.NewCard(cards.Ace, cards.Hearts), cards.NewCard(cards.Seven, cards.Spades)},
			wantTotal:    18,
			wantSoft:     true,
			wantBJ:       false,
			wantBust:     false,
			wantCanSplit: false,
		},
		{
			name:         "Multiple Aces (A + A + 9)",
			cardsList:    []cards.Card{cards.NewCard(cards.Ace, cards.Hearts), cards.NewCard(cards.Ace, cards.Diamonds), cards.NewCard(cards.Nine, cards.Spades)},
			wantTotal:    21,
			wantSoft:     true,
			wantBJ:       false,
			wantBust:     false,
			wantCanSplit: false,
		},
		{
			name:         "Hard Bust (10 + 6 + 7)",
			cardsList:    []cards.Card{cards.NewCard(cards.Ten, cards.Hearts), cards.NewCard(cards.Six, cards.Diamonds), cards.NewCard(cards.Seven, cards.Spades)},
			wantTotal:    23,
			wantSoft:     false,
			wantBJ:       false,
			wantBust:     true,
			wantCanSplit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHand(10)
			for _, c := range tt.cardsList {
				h.AddCard(c)
			}
			tot, soft := h.Score()
			if tot != tt.wantTotal {
				t.Errorf("Score() total = %d, want %d", tot, tt.wantTotal)
			}
			if soft != tt.wantSoft {
				t.Errorf("Score() soft = %v, want %v", soft, tt.wantSoft)
			}
			if h.IsBlackjack() != tt.wantBJ {
				t.Errorf("IsBlackjack() = %v, want %v", h.IsBlackjack(), tt.wantBJ)
			}
			if h.IsBusted() != tt.wantBust {
				t.Errorf("IsBusted() = %v, want %v", h.IsBusted(), tt.wantBust)
			}
			if h.CanSplit() != tt.wantCanSplit {
				t.Errorf("CanSplit() = %v, want %v", h.CanSplit(), tt.wantCanSplit)
			}
			_ = h.String() // coverage
		})
	}
}

func TestGameEngine_BasicRoundFlows(t *testing.T) {
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)

	if engine.State != StateWaitingBet {
		t.Fatalf("Expected initial state StateWaitingBet, got %v", engine.State)
	}

	// Invalid Bet
	if err := engine.StartRound(0); err == nil {
		t.Errorf("Expected error starting round with 0 bet")
	}

	// Start valid round
	err := engine.StartRound(100)
	if err != nil {
		t.Fatalf("StartRound failed: %v", err)
	}

	// Active hand
	if engine.ActiveHand() == nil {
		t.Fatalf("ActiveHand should not be nil")
	}

	// If in StateInsuranceOffered, test declining insurance
	if engine.State == StateInsuranceOffered {
		actions := engine.AvailableActions()
		if len(actions) != 2 {
			t.Errorf("Expected 2 insurance actions, got %d", len(actions))
		}
		err = engine.Step(ActionInsuranceDecline)
		if err != nil {
			t.Fatalf("Declining insurance failed: %v", err)
		}
	}

	// If player is in turn
	if engine.State == StatePlayerTurn {
		actions := engine.AvailableActions()
		if len(actions) == 0 {
			t.Errorf("Expected at least 1 action during player turn")
		}
		// Stand to conclude round
		err = engine.Step(ActionStand)
		if err != nil {
			t.Fatalf("Stand failed: %v", err)
		}
	}

	if engine.State != StateRoundResolved {
		t.Fatalf("Expected round to be resolved, got %v", engine.State)
	}

	_ = engine.NetRoundProfit()
}

func TestGameEngine_HitUntilBustOr21(t *testing.T) {
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)

	engine.State = StatePlayerTurn
	engine.PlayerHands = []*Hand{NewHand(10)}
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Ten, cards.Hearts))
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Nine, cards.Diamonds)) // 19
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Ten, cards.Spades))
	engine.DealerHoleCard = cards.NewCard(cards.Eight, cards.Clubs)
	engine.HasHoleCard = true

	// Hit -> will likely bust or hit 21
	err := engine.Step(ActionHit)
	if err != nil {
		t.Fatalf("Hit failed: %v", err)
	}
}

func TestGameEngine_DoubleDownAndSurrender(t *testing.T) {
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)

	// Simulate a hand setup manually
	engine.State = StatePlayerTurn
	engine.PlayerHands = []*Hand{NewHand(50)}
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Five, cards.Hearts))
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Six, cards.Diamonds)) // 11
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Seven, cards.Spades))
	engine.DealerHoleCard = cards.NewCard(cards.Ten, cards.Clubs)
	engine.HasHoleCard = true

	// Double Down
	err := engine.Step(ActionDouble)
	if err != nil {
		t.Fatalf("Double down failed: %v", err)
	}

	if !engine.PlayerHands[0].Doubled {
		t.Errorf("Expected hand to be marked doubled")
	}
	if engine.PlayerHands[0].Bet != 100 {
		t.Errorf("Expected doubled bet 100, got %f", engine.PlayerHands[0].Bet)
	}
	if engine.State != StateRoundResolved {
		t.Errorf("Expected round to be resolved after double down, got %v", engine.State)
	}

	// Test Surrender
	engine2 := NewGameEngine(rule, nil)
	engine2.State = StatePlayerTurn
	engine2.PlayerHands = []*Hand{NewHand(50)}
	engine2.PlayerHands[0].AddCard(cards.NewCard(cards.Ten, cards.Hearts))
	engine2.PlayerHands[0].AddCard(cards.NewCard(cards.Six, cards.Diamonds)) // 16
	engine2.DealerHand = NewHand(0)
	engine2.DealerHand.AddCard(cards.NewCard(cards.Ten, cards.Spades))
	engine2.DealerHoleCard = cards.NewCard(cards.Nine, cards.Clubs)
	engine2.HasHoleCard = true

	err = engine2.Step(ActionSurrender)
	if err != nil {
		t.Fatalf("Surrender failed: %v", err)
	}
	if engine2.PlayerHands[0].Status != StatusSurrendered {
		t.Errorf("Expected hand to be surrendered")
	}
	if engine2.PlayerHands[0].NetProfit != -25 {
		t.Errorf("Expected -$25 net profit for surrender, got %f", engine2.PlayerHands[0].NetProfit)
	}
}

func TestGameEngine_SplitHands(t *testing.T) {
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)

	engine.State = StatePlayerTurn
	engine.PlayerHands = []*Hand{NewHand(50)}
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Eight, cards.Hearts))
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Eight, cards.Diamonds))
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Six, cards.Spades))
	engine.DealerHoleCard = cards.NewCard(cards.Ten, cards.Clubs)
	engine.HasHoleCard = true

	err := engine.Step(ActionSplit)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	if len(engine.PlayerHands) != 2 {
		t.Fatalf("Expected 2 hands after split, got %d", len(engine.PlayerHands))
	}

	// Play both split hands with Stand
	if engine.State == StatePlayerTurn {
		_ = engine.Step(ActionStand)
	}
	if engine.State == StatePlayerTurn {
		_ = engine.Step(ActionStand)
	}

	if engine.State != StateRoundResolved {
		t.Errorf("Expected round resolved, got %v", engine.State)
	}
}

func TestGameEngine_SplitAcesNoHit(t *testing.T) {
	rule := rules.VegasStrip()
	rule.HitSplitAces = false
	engine := NewGameEngine(rule, nil)

	engine.State = StatePlayerTurn
	engine.PlayerHands = []*Hand{NewHand(50)}
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Ace, cards.Diamonds))
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Seven, cards.Spades))
	engine.DealerHoleCard = cards.NewCard(cards.Ten, cards.Clubs)
	engine.HasHoleCard = true

	err := engine.Step(ActionSplit)
	if err != nil {
		t.Fatalf("Split Aces failed: %v", err)
	}

	// Both hands should auto stand
	if engine.State != StateRoundResolved {
		t.Errorf("Expected round resolved automatically when splitting Aces with HitSplitAces=false, got %v", engine.State)
	}
}

func TestGameEngine_StepInvalidState(t *testing.T) {
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)
	engine.State = StateRoundResolved

	if err := engine.Step(ActionHit); err == nil {
		t.Errorf("Expected error stepping when round is already resolved")
	}
}

func TestGameEngine_DealerUpcard(t *testing.T) {
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)

	// No cards
	c := engine.DealerUpcard()
	if c.Rank != 0 {
		t.Errorf("Expected empty dealer card when no cards dealt")
	}
}

func TestGameEngine_EuropeanENHCDealerPlay(t *testing.T) {
	rule := rules.European()
	engine := NewGameEngine(rule, nil)

	_ = engine.StartRound(50)
	if engine.HasHoleCard {
		t.Errorf("European ENHC should not deal hole card at start")
	}

	if engine.State == StatePlayerTurn {
		_ = engine.Step(ActionStand)
	}

	if len(engine.DealerHand.Cards) < 2 {
		t.Errorf("Dealer should draw at least 2 cards in European ENHC, got %d", len(engine.DealerHand.Cards))
	}
}

func TestGameEngine_ResolutionsAndOutcomes(t *testing.T) {
	// 1. Natural Blackjack vs normal dealer hand
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)
	engine.State = StatePlayerTurn
	h1 := NewHand(100)
	h1.AddCard(cards.NewCard(cards.Ace, cards.Spades))
	h1.AddCard(cards.NewCard(cards.King, cards.Spades)) // BJ
	h1.Status = StatusBlackjack
	engine.PlayerHands = []*Hand{h1}
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Ten, cards.Diamonds))
	engine.DealerHand.AddCard(cards.NewCard(cards.Eight, cards.Clubs)) // 18
	engine.resolveRound()

	if h1.NetProfit != 150 { // 3:2 on $100 = +150
		t.Errorf("Expected $150 net profit on BJ, got %f", h1.NetProfit)
	}

	// 2. Natural BJ vs Dealer BJ -> Push
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Ace, cards.Hearts))
	engine.DealerHand.AddCard(cards.NewCard(cards.Queen, cards.Hearts))
	engine.resolveRound()
	if h1.NetProfit != 0 {
		t.Errorf("Expected 0 net profit on BJ vs BJ push, got %f", h1.NetProfit)
	}

	// 3. Dealer Busted
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Ten, cards.Hearts))
	engine.DealerHand.AddCard(cards.NewCard(cards.Six, cards.Hearts))
	engine.DealerHand.AddCard(cards.NewCard(cards.Seven, cards.Hearts)) // 23 Bust
	h2 := NewHand(100)
	h2.AddCard(cards.NewCard(cards.Ten, cards.Spades))
	h2.AddCard(cards.NewCard(cards.Nine, cards.Spades)) // 19
	engine.PlayerHands = []*Hand{h2}
	engine.resolveRound()
	if h2.NetProfit != 100 {
		t.Errorf("Expected +100 when dealer busts, got %f", h2.NetProfit)
	}

	// 4. Dealer Higher Total
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Ten, cards.Hearts))
	engine.DealerHand.AddCard(cards.NewCard(cards.Ten, cards.Clubs)) // 20
	engine.resolveRound()
	if h2.NetProfit != -100 {
		t.Errorf("Expected -100 when dealer wins, got %f", h2.NetProfit)
	}

	// 5. Tie / Push
	h3 := NewHand(100)
	h3.AddCard(cards.NewCard(cards.Ten, cards.Spades))
	h3.AddCard(cards.NewCard(cards.Ten, cards.Diamonds)) // 20
	engine.PlayerHands = []*Hand{h3}
	engine.resolveRound()
	if h3.NetProfit != 0 {
		t.Errorf("Expected 0 on push, got %f", h3.NetProfit)
	}
}

func TestGameEngine_InsuranceAcceptedAndWon(t *testing.T) {
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)

	engine.State = StateInsuranceOffered
	engine.InitialBet = 100
	engine.PlayerHands = []*Hand{NewHand(100)}
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Ten, cards.Hearts))
	engine.PlayerHands[0].AddCard(cards.NewCard(cards.Eight, cards.Diamonds))
	engine.DealerHand = NewHand(0)
	engine.DealerHand.AddCard(cards.NewCard(cards.Ace, cards.Spades))
	engine.DealerHoleCard = cards.NewCard(cards.King, cards.Clubs) // Dealer has BJ!
	engine.HasHoleCard = true

	err := engine.Step(ActionInsuranceAccept)
	if err != nil {
		t.Fatalf("Insurance accept failed: %v", err)
	}

	if !engine.PlayerHands[0].InsuranceWon {
		t.Errorf("Expected insurance won")
	}
	// Hand bet lost (-100), insurance won (+100) -> net 0
	if engine.PlayerHands[0].NetProfit != 0 {
		t.Errorf("Expected net profit 0 on insurance win, got %f", engine.PlayerHands[0].NetProfit)
	}
}

func TestGameEngine_InvalidActionErrors(t *testing.T) {
	rule := rules.VegasStrip()
	engine := NewGameEngine(rule, nil)

	// Step while waiting for bet
	if err := engine.Step(ActionHit); err == nil {
		t.Errorf("Expected error stepping while waiting for bet")
	}

	// Insurance while not offered
	if err := engine.Insurance(true); err == nil {
		t.Errorf("Expected error insurance when not in insurance state")
	}

	// Active hand when empty
	if h := engine.ActiveHand(); h != nil {
		t.Errorf("Expected nil active hand when empty")
	}

	// Available actions when waiting bet
	if acts := engine.AvailableActions(); acts != nil {
		t.Errorf("Expected nil actions when waiting bet")
	}

	// Test double restriction rule validation in AvailableActions
	engine.State = StatePlayerTurn
	h := NewHand(10)
	h.AddCard(cards.NewCard(cards.Two, cards.Hearts))
	h.AddCard(cards.NewCard(cards.Three, cards.Diamonds)) // Total 5
	engine.PlayerHands = []*Hand{h}
	engine.Rules.DoubleRestriction = rules.Double9To11Only
	actions := engine.AvailableActions()
	for _, a := range actions {
		if a == ActionDouble {
			t.Errorf("ActionDouble should not be available with total 5 under 9-11 restriction")
		}
	}

	// Test DAS restriction in AvailableActions
	h.IsSplitHand = true
	engine.Rules.DoubleAfterSplit = false
	actions2 := engine.AvailableActions()
	for _, a := range actions2 {
		if a == ActionDouble {
			t.Errorf("ActionDouble should not be available for split hand when DAS=false")
		}
	}

	// Test Double restriction 10-11
	h.IsSplitHand = false
	engine.Rules.DoubleRestriction = rules.Double10Or11Only
	actions3 := engine.AvailableActions()
	for _, a := range actions3 {
		if a == ActionDouble {
			t.Errorf("ActionDouble should not be available with total 5 under 10-11 restriction")
		}
	}

	// Invalid step action
	err := engine.Step(PlayerAction(99))
	if err == nil {
		t.Errorf("Expected error for invalid player action")
	}

	// Double invalid hand
	h.AddCard(cards.NewCard(cards.Five, cards.Spades)) // 3 cards now
	err = engine.actionDouble(h)
	if err == nil {
		t.Errorf("Expected error doubling 3-card hand")
	}

	// Split invalid hand
	err = engine.actionSplit(h)
	if err == nil {
		t.Errorf("Expected error splitting non-pair")
	}

	// Surrender invalid
	engine.Rules.Surrender = rules.SurrenderNone
	err = engine.actionSurrender(h)
	if err == nil {
		t.Errorf("Expected error surrendering when surrender rule is None")
	}
}

func TestEnumsAndStrings(t *testing.T) {
	if StateWaitingBet.String() != "Waiting For Bet" ||
		StateInsuranceOffered.String() != "Insurance Offered" ||
		StatePlayerTurn.String() != "Player Turn" ||
		StateDealerTurn.String() != "Dealer Turn" ||
		StateRoundResolved.String() != "Round Resolved" ||
		GameState(99).String() != "Unknown State" {
		t.Errorf("Invalid GameState string")
	}

	if ActionHit.String() != "Hit" ||
		ActionStand.String() != "Stand" ||
		ActionDouble.String() != "Double Down" ||
		ActionSplit.String() != "Split" ||
		ActionSurrender.String() != "Surrender" ||
		ActionInsuranceAccept.String() != "Insurance Accept" ||
		ActionInsuranceDecline.String() != "Insurance Decline" ||
		PlayerAction(99).String() != "Unknown Action" {
		t.Errorf("Invalid PlayerAction string")
	}

	if StatusInPlay.String() != "In Play" ||
		StatusStood.String() != "Stood" ||
		StatusDoubled.String() != "Doubled" ||
		StatusBusted.String() != "Busted" ||
		StatusBlackjack.String() != "Blackjack" ||
		StatusSurrendered.String() != "Surrendered" ||
		HandStatus(99).String() != "Unknown" {
		t.Errorf("Invalid HandStatus string")
	}
}

func BenchmarkHandScore(b *testing.B) {
	h := NewHand(10)
	h.AddCard(cards.NewCard(cards.Ace, cards.Spades))
	h.AddCard(cards.NewCard(cards.Six, cards.Hearts))
	h.AddCard(cards.NewCard(cards.Four, cards.Diamonds))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = h.Score()
	}
}
