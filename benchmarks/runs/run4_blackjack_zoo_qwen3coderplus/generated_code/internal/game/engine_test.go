package game

import (
	"testing"

	"blackjack/pkg/cards"
)

func TestNewGameEngine(t *testing.T) {
	rules := &RuleSet{
		NumDecks:          6,
		DealerHitsSoft17:  true,
		DoubleAfterSplit:  true,
		SplitAcesOnceOnly: true,
		LateSurrender:     true,
		InsuranceAllowed:  true,
		BlackjackPayout:   1.5,
	}

	engine := NewGameEngine(rules)

	if engine.State != BettingState {
		t.Errorf("Expected state to be BettingState, got %s", engine.State)
	}

	if engine.Rules != rules {
		t.Errorf("Expected rules to match, but they don't")
	}

	if engine.Shoe == nil {
		t.Error("Expected shoe to be initialized")
	}

	if len(engine.Players) != 0 {
		t.Errorf("Expected no players initially, got %d", len(engine.Players))
	}
}

func TestAddPlayer(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})

	engine.AddPlayer(1, "Test Player", 100.0)

	if len(engine.Players) != 1 {
		t.Fatalf("Expected 1 player, got %d", len(engine.Players))
	}

	player := engine.Players[0]
	if player.ID != 1 {
		t.Errorf("Expected player ID to be 1, got %d", player.ID)
	}

	if player.Name != "Test Player" {
		t.Errorf("Expected player name to be 'Test Player', got '%s'", player.Name)
	}

	if player.Chips != 100.0 {
		t.Errorf("Expected player chips to be 100.0, got %f", player.Chips)
	}
}

func TestPlaceBet(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})
	engine.AddPlayer(1, "Test Player", 100.0)

	err := engine.PlaceBet(1, 20.0)
	if err != nil {
		t.Fatalf("Unexpected error placing bet: %v", err)
	}

	player := engine.Players[0]
	if player.Chips != 80.0 {
		t.Errorf("Expected player chips to be 80.0 after bet, got %f", player.Chips)
	}

	if player.CurrentBet != 20.0 {
		t.Errorf("Expected current bet to be 20.0, got %f", player.CurrentBet)
	}
}

func TestPlaceBetInsufficientChips(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})
	engine.AddPlayer(1, "Test Player", 10.0)

	err := engine.PlaceBet(1, 20.0)
	if err == nil {
		t.Fatal("Expected error for insufficient chips, got none")
	}

	expected := "insufficient chips"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestPlaceBetWrongGameState(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})
	engine.AddPlayer(1, "Test Player", 100.0)
	engine.State = DealingState

	err := engine.PlaceBet(1, 20.0)
	if err == nil {
		t.Fatal("Expected error when not in betting state, got none")
	}

	expected := "cannot place bet when not in betting state"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestDealInitialCards(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})
	engine.AddPlayer(1, "Test Player", 100.0)

	// Place a bet first
	err := engine.PlaceBet(1, 20.0)
	if err != nil {
		t.Fatalf("Unexpected error placing bet: %v", err)
	}

	err = engine.DealInitialCards()
	if err != nil {
		t.Fatalf("Unexpected error dealing cards: %v", err)
	}

	player := engine.Players[0]
	if len(player.Hands) != 1 {
		t.Fatalf("Expected 1 hand, got %d", len(player.Hands))
	}

	hand := player.Hands[0]
	if len(hand.Cards) != 2 {
		t.Fatalf("Expected 2 cards in hand, got %d", len(hand.Cards))
	}

	if len(engine.Dealer.Cards) != 2 {
		t.Fatalf("Expected 2 cards in dealer hand, got %d", len(engine.Dealer.Cards))
	}

	if engine.State != PlayerTurnState {
		t.Errorf("Expected state to be PlayerTurnState, got %s", engine.State)
	}
}

func TestHasBlackjack(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})

	// Create a hand with an Ace and a 10-value card
	hand := &Hand{
		Cards: []cards.Card{
			{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
			{Rank: cards.King, Suit: cards.Hearts, Value: 10},
		},
	}

	if !engine.hasBlackjack(hand) {
		t.Error("Expected hand to be blackjack, but it wasn't")
	}

	// Create a hand with an Ace and a 9-value card (not blackjack)
	hand2 := &Hand{
		Cards: []cards.Card{
			{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
			{Rank: cards.Nine, Suit: cards.Hearts, Value: 9},
		},
	}

	if engine.hasBlackjack(hand2) {
		t.Error("Expected hand not to be blackjack, but it was")
	}
}

func TestHit(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})
	engine.AddPlayer(1, "Test Player", 100.0)

	// Place a bet and deal initial cards
	err := engine.PlaceBet(1, 20.0)
	if err != nil {
		t.Fatalf("Unexpected error placing bet: %v", err)
	}

	err = engine.DealInitialCards()
	if err != nil {
		t.Fatalf("Unexpected error dealing cards: %v", err)
	}

	// Store initial hand value
	initialHandValue := engine.Players[0].Hands[0].Value()

	err = engine.Hit(1)
	if err != nil {
		t.Fatalf("Unexpected error hitting: %v", err)
	}

	hand := engine.Players[0].Hands[0]
	if len(hand.Cards) != 3 {
		t.Errorf("Expected 3 cards after hit, got %d", len(hand.Cards))
	}

	// The hand value should have increased
	newHandValue := hand.Value()
	if newHandValue < initialHandValue && newHandValue <= 21 {
		t.Errorf("Expected hand value to possibly increase after hit, old: %d, new: %d", initialHandValue, newHandValue)
	}
}

func TestStand(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})
	engine.AddPlayer(1, "Test Player", 100.0)

	// Place a bet and deal initial cards
	err := engine.PlaceBet(1, 20.0)
	if err != nil {
		t.Fatalf("Unexpected error placing bet: %v", err)
	}

	err = engine.DealInitialCards()
	if err != nil {
		t.Fatalf("Unexpected error dealing cards: %v", err)
	}

	err = engine.Stand(1)
	if err != nil {
		t.Fatalf("Unexpected error standing: %v", err)
	}

	hand := engine.Players[0].Hands[0]
	if hand.Active {
		t.Error("Expected hand to be inactive after standing")
	}
}

func TestDoubleDown(t *testing.T) {
	engine := NewGameEngine(&RuleSet{})
	engine.AddPlayer(1, "Test Player", 100.0)

	// Place a bet and deal initial cards
	err := engine.PlaceBet(1, 20.0)
	if err != nil {
		t.Fatalf("Unexpected error placing bet: %v", err)
	}

	err = engine.DealInitialCards()
	if err != nil {
		t.Fatalf("Unexpected error dealing cards: %v", err)
	}

	player := engine.Players[0]
	initialChips := player.Chips

	err = engine.DoubleDown(1)
	if err != nil {
		t.Fatalf("Unexpected error doubling down: %v", err)
	}

	hand := player.Hands[0]
	if hand.Bet != 40.0 {
		t.Errorf("Expected bet to be doubled to 40.0, got %f", hand.Bet)
	}

	if player.Chips != initialChips-20.0 {
		t.Errorf("Expected chips to decrease by 20.0 (additional bet), had %f, now %f", initialChips, player.Chips)
	}

	if !hand.IsDouble {
		t.Error("Expected hand to be marked as doubled")
	}

	// After double down, the hand becomes inactive (no more actions allowed)
	if hand.Active {
		t.Error("Expected hand to become inactive after double down")
	}
}

func TestSplit(t *testing.T) {
	engine := NewGameEngine(&RuleSet{DoubleAfterSplit: true})
	engine.AddPlayer(1, "Test Player", 200.0)

	// Place a bet and deal initial cards with a pair
	player := engine.Players[0]
	player.Hands = append(player.Hands, &Hand{
		Cards: []cards.Card{
			{Rank: cards.Eight, Suit: cards.Spades, Value: 8},
			{Rank: cards.Eight, Suit: cards.Hearts, Value: 8},
		},
		Bet:    20.0,
		Active: true,
	})
	player.CurrentBet = 20.0
	engine.State = PlayerTurnState

	initialChips := player.Chips
	err := engine.Split(1)
	if err != nil {
		t.Fatalf("Unexpected error splitting: %v", err)
	}

	if len(player.Hands) != 2 {
		t.Fatalf("Expected 2 hands after split, got %d", len(player.Hands))
	}

	if player.Chips != initialChips-20.0 {
		t.Errorf("Expected chips to decrease by 20.0 (additional bet), had %f, now %f", initialChips, player.Chips)
	}

	for i, hand := range player.Hands {
		if len(hand.Cards) != 2 {
			t.Errorf("Expected hand %d to have 2 cards after split, got %d", i, len(hand.Cards))
		}

		if hand.Bet != 20.0 {
			t.Errorf("Expected hand %d bet to be 20.0, got %f", i, hand.Bet)
		}
	}
}

func TestResolveRound(t *testing.T) {
	engine := NewGameEngine(&RuleSet{BlackjackPayout: 1.5})
	engine.AddPlayer(1, "Test Player", 100.0)

	// Simulate a completed round
	player := engine.Players[0]
	player.Hands = append(player.Hands, &Hand{
		Cards: []cards.Card{
			{Rank: cards.Five, Suit: cards.Spades, Value: 5},
			{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
			{Rank: cards.Five, Suit: cards.Clubs, Value: 5}, // Total 16
		},
		Bet:    20.0,
		Active: false,
	})

	// Set dealer to bust
	engine.Dealer = &Hand{
		Cards: []cards.Card{
			{Rank: cards.King, Suit: cards.Spades, Value: 10},
			{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
			{Rank: cards.Seven, Suit: cards.Clubs, Value: 7}, // Total 23 (bust)
		},
		IsBusted: true,
	}

	initialChips := player.Chips
	engine.State = EndRoundState

	err := engine.ResolveRound()
	if err != nil {
		t.Fatalf("Unexpected error resolving round: %v", err)
	}

	// Player should win because dealer busted
	expectedChips := initialChips + 40.0 // Original bet (20) + winnings (20)
	if player.Chips != expectedChips {
		t.Errorf("Expected chips to be %f after winning, got %f", expectedChips, player.Chips)
	}

	// Hands should be reset
	if len(player.Hands) != 0 {
		t.Errorf("Expected hands to be reset to 0, got %d", len(player.Hands))
	}

	if engine.State != BettingState {
		t.Errorf("Expected state to be BettingState after resolving, got %s", engine.State)
	}
}

func TestCalculateHandResult(t *testing.T) {
	engine := NewGameEngine(&RuleSet{BlackjackPayout: 1.5})

	// Create dealer hand with a value of 15
	dealerHand := &Hand{
		Cards: []cards.Card{
			{Rank: cards.Eight, Suit: cards.Spades, Value: 8},
			{Rank: cards.Seven, Suit: cards.Hearts, Value: 7},
		},
		IsBusted: false,
	}
	engine.Dealer = dealerHand

	// Player wins with 18 vs dealer 15
	playerHand := &Hand{
		Cards: []cards.Card{
			{Rank: cards.Ten, Suit: cards.Spades, Value: 10},
			{Rank: cards.Eight, Suit: cards.Hearts, Value: 8},
		},
		IsBusted:    false,
		IsBlackjack: false,
	}

	result := engine.calculateHandResult(playerHand)
	if result != "win" {
		t.Errorf("Expected result to be 'win', got '%s'", result)
	}

	// Player loses with 14 vs dealer 15
	playerHand = &Hand{
		Cards: []cards.Card{
			{Rank: cards.Eight, Suit: cards.Spades, Value: 8},
			{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
		},
		IsBusted:    false,
		IsBlackjack: false,
	}

	result = engine.calculateHandResult(playerHand)
	if result != "lose" {
		t.Errorf("Expected result to be 'lose', got '%s'", result)
	}

	// Push (tie) with 15 vs dealer 15
	playerHand = &Hand{
		Cards: []cards.Card{
			{Rank: cards.Nine, Suit: cards.Spades, Value: 9},
			{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
		},
		IsBusted:    false,
		IsBlackjack: false,
	}

	result = engine.calculateHandResult(playerHand)
	if result != "push" {
		t.Errorf("Expected result to be 'push', got '%s'", result)
	}

	// Player busts
	playerHand = &Hand{
		Cards: []cards.Card{
			{Rank: cards.Ten, Suit: cards.Spades, Value: 10},
			{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
			{Rank: cards.Eight, Suit: cards.Clubs, Value: 8}, // Total 24 (bust)
		},
		IsBusted:    true,
		IsBlackjack: false,
	}

	result = engine.calculateHandResult(playerHand)
	if result != "lose" {
		t.Errorf("Expected result to be 'lose' when player busts, got '%s'", result)
	}

	// Reset dealer to not bust
	engine.Dealer = &Hand{
		Cards: []cards.Card{
			{Rank: cards.Ten, Suit: cards.Spades, Value: 10},
			{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
			{Rank: cards.Eight, Suit: cards.Clubs, Value: 8}, // Total 24 (bust)
		},
		IsBusted: true,
	}

	// Player wins because dealer busts
	playerHand = &Hand{
		Cards: []cards.Card{
			{Rank: cards.Ten, Suit: cards.Spades, Value: 10},
			{Rank: cards.Seven, Suit: cards.Hearts, Value: 7}, // Total 17
		},
		IsBusted:    false,
		IsBlackjack: false,
	}

	result = engine.calculateHandResult(playerHand)
	if result != "win" {
		t.Errorf("Expected result to be 'win' when dealer busts, got '%s'", result)
	}

	// Player blackjack
	playerHand = &Hand{
		Cards: []cards.Card{
			{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
			{Rank: cards.King, Suit: cards.Hearts, Value: 10},
		},
		IsBlackjack: true,
		IsBusted:    false,
	}
	// Reset dealer to not have blackjack
	engine.Dealer = &Hand{
		Cards: []cards.Card{
			{Rank: cards.Ten, Suit: cards.Spades, Value: 10},
			{Rank: cards.Seven, Suit: cards.Hearts, Value: 7},
		},
		IsBusted:    false,
		IsBlackjack: false,
	}

	result = engine.calculateHandResult(playerHand)
	if result != "blackjack" {
		t.Errorf("Expected result to be 'blackjack', got '%s'", result)
	}
}

func TestGetAvailableActions(t *testing.T) {
	engine := NewGameEngine(&RuleSet{DoubleAfterSplit: true})
	engine.AddPlayer(1, "Test Player", 100.0)

	// Place a bet and deal initial cards with a pair
	player := engine.Players[0]
	player.Hands = append(player.Hands, &Hand{
		Cards: []cards.Card{
			{Rank: cards.Eight, Suit: cards.Spades, Value: 8},
			{Rank: cards.Eight, Suit: cards.Hearts, Value: 8},
		},
		Bet:    20.0,
		Active: true,
	})
	player.CurrentBet = 20.0
	// Dealer must have an upcard for GetAvailableActions to check insurance eligibility
	engine.Dealer = &Hand{
		Cards: []cards.Card{
			{Rank: cards.Nine, Suit: cards.Spades, Value: 9},
			{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
		},
	}
	engine.State = PlayerTurnState

	actions, err := engine.GetAvailableActions(1)
	if err != nil {
		t.Fatalf("Unexpected error getting actions: %v", err)
	}

	// Should have hit, stand, double, split
	expectedActions := map[string]bool{
		"hit":    true,
		"stand":  true,
		"double": true,
		"split":  true,
	}

	for _, action := range actions {
		if !expectedActions[action] {
			t.Errorf("Unexpected action '%s' in available actions", action)
		}
		delete(expectedActions, action)
	}

	if len(expectedActions) > 0 {
		t.Errorf("Missing expected actions: %v", expectedActions)
	}
}
