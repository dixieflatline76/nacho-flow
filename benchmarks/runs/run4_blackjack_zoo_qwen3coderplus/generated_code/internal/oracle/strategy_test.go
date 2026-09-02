package oracle

import (
	"testing"

	"blackjack/internal/game"
	"blackjack/pkg/cards"
)

func TestNewStrategyAdvisor(t *testing.T) {
	ruleSet := &game.RuleSet{}
	counter := NewCardCounter(HiLoCount, 6)

	advisor := NewStrategyAdvisor(ruleSet, counter)

	if advisor.RuleSet != ruleSet {
		t.Errorf("Expected RuleSet to match, but they don't")
	}

	if advisor.Counter != counter {
		t.Errorf("Expected Counter to match, but they don't")
	}
}

func TestGetCardValue(t *testing.T) {
	advisor := &StrategyAdvisor{}

	card := cards.Card{Rank: cards.Ten, Suit: cards.Spades, Value: 10}
	value := advisor.getCardValue(card)

	if value != 10 {
		t.Errorf("Expected card value to be 10, got %d", value)
	}

	card = cards.Card{Rank: cards.Ace, Suit: cards.Spades, Value: 1}
	value = advisor.getCardValue(card)

	if value != 1 {
		t.Errorf("Expected card value to be 1, got %d", value)
	}
}

func TestGetHandValue(t *testing.T) {
	advisor := &StrategyAdvisor{}

	// Test hard hand
	hand := []cards.Card{
		{Rank: cards.Ten, Suit: cards.Spades, Value: 10},
		{Rank: cards.Five, Suit: cards.Hearts, Value: 5},
	}
	value := advisor.getHandValue(hand)

	if value != 15 {
		t.Errorf("Expected hand value to be 15, got %d", value)
	}

	// Test soft hand (A+6 = soft 17)
	hand = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
		{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
	}
	value = advisor.getHandValue(hand)

	if value != 17 {
		t.Errorf("Expected hand value to be 17, got %d", value)
	}

	// Test hard hand with ace as 1 (A+6+10 = 17)
	hand = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
		{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
		{Rank: cards.Ten, Suit: cards.Clubs, Value: 10},
	}
	value = advisor.getHandValue(hand)

	if value != 17 {
		t.Errorf("Expected hand value to be 17, got %d", value)
	}

	// Test blackjack
	hand = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
		{Rank: cards.King, Suit: cards.Hearts, Value: 10},
	}
	value = advisor.getHandValue(hand)

	if value != 21 {
		t.Errorf("Expected hand value to be 21 (blackjack), got %d", value)
	}
}

func TestIsSoftHand(t *testing.T) {
	advisor := &StrategyAdvisor{}

	// Test hard hand (should return false)
	hand := []cards.Card{
		{Rank: cards.Ten, Suit: cards.Spades, Value: 10},
		{Rank: cards.Five, Suit: cards.Hearts, Value: 5},
	}
	isSoft := advisor.isSoftHand(hand)

	if isSoft {
		t.Errorf("Expected hand to be hard, got soft")
	}

	// Test soft hand (A+6 = soft 17)
	hand = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
		{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
	}
	isSoft = advisor.isSoftHand(hand)

	if !isSoft {
		t.Errorf("Expected hand to be soft, got hard")
	}

	// Test hard hand with ace as 1 (A+6+10 = hard 17)
	hand = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
		{Rank: cards.Six, Suit: cards.Hearts, Value: 6},
		{Rank: cards.Ten, Suit: cards.Clubs, Value: 10},
	}
	isSoft = advisor.isSoftHand(hand)

	if isSoft {
		t.Errorf("Expected hand to be hard (ace as 1), got soft")
	}

	// Test soft hand (A+A+5 = soft 17)
	hand = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
		{Rank: cards.Ace, Suit: cards.Hearts, Value: 1},
		{Rank: cards.Five, Suit: cards.Clubs, Value: 5},
	}
	isSoft = advisor.isSoftHand(hand)

	if !isSoft {
		t.Errorf("Expected hand to be soft, got hard")
	}
}

func TestCheckPairStrategy(t *testing.T) {
	ruleSet := &game.RuleSet{DoubleAfterSplit: true}
	advisor := &StrategyAdvisor{RuleSet: ruleSet}

	// Test pair of aces (should split)
	action := advisor.checkPairStrategy(cards.Ace, 6, false)
	if action != SplitAction {
		t.Errorf("Expected pair of aces to split, got %s", action)
	}

	// Test pair of eights (should split)
	action = advisor.checkPairStrategy(cards.Eight, 6, false)
	if action != SplitAction {
		t.Errorf("Expected pair of eights to split, got %s", action)
	}

	// Test pair of tens (should not split)
	action = advisor.checkPairStrategy(cards.Ten, 6, false)
	if action != StandAction {
		t.Errorf("Expected pair of tens to stand, got %s", action)
	}

	// Test pair of nines (should split against 2-6, 8-9; stand against 7, 10, A)
	action = advisor.checkPairStrategy(cards.Nine, 6, false)
	if action != SplitAction {
		t.Errorf("Expected pair of nines to split against 6, got %s", action)
	}

	action = advisor.checkPairStrategy(cards.Nine, 7, false)
	if action != StandAction {
		t.Errorf("Expected pair of nines to stand against 7, got %s", action)
	}
}

func TestGetSoftHandStrategy(t *testing.T) {
	ruleSet := &game.RuleSet{DoubleAfterSplit: true}
	advisor := &StrategyAdvisor{RuleSet: ruleSet}

	// Test A+8 (soft 19) - should double against dealer 6 according to basic strategy
	action := advisor.getSoftHandStrategy(19, 6)
	if action != DoubleAction {
		t.Errorf("Expected A+8 to double against dealer 6, got %s", action)
	}

	// Test A+8 (soft 19) - should stand against dealer 2, 7, 8
	action = advisor.getSoftHandStrategy(19, 7)
	if action != StandAction {
		t.Errorf("Expected A+8 to stand against dealer 7, got %s", action)
	}

	// Test A+7 (soft 18) - should double against 3-6, stand against 2,7,8, hit against 9,10,A
	action = advisor.getSoftHandStrategy(18, 6)
	if action != DoubleAction {
		t.Errorf("Expected A+7 to double against 6, got %s", action)
	}

	action = advisor.getSoftHandStrategy(18, 7)
	if action != StandAction {
		t.Errorf("Expected A+7 to stand against 7, got %s", action)
	}

	action = advisor.getSoftHandStrategy(18, 9)
	if action != HitAction {
		t.Errorf("Expected A+7 to hit against 9, got %s", action)
	}
}

func TestGetHardHandStrategy(t *testing.T) {
	ruleSet := &game.RuleSet{DoubleAfterSplit: true}
	advisor := &StrategyAdvisor{RuleSet: ruleSet}

	// Test 20 - should stand
	action := advisor.getHardHandStrategy(20, 6)
	if action != StandAction {
		t.Errorf("Expected 20 to stand, got %s", action)
	}

	// Test 11 - should double
	action = advisor.getHardHandStrategy(11, 6)
	if action != DoubleAction {
		t.Errorf("Expected 11 to double, got %s", action)
	}

	// Test 16 against dealer 6 - should stand
	action = advisor.getHardHandStrategy(16, 6)
	if action != StandAction {
		t.Errorf("Expected 16 to stand against dealer 6, got %s", action)
	}

	// Test 16 against dealer 10 - should hit
	action = advisor.getHardHandStrategy(16, 10)
	if action != HitAction {
		t.Errorf("Expected 16 to hit against dealer 10, got %s", action)
	}
}

func TestGetRecommendedAction(t *testing.T) {
	ruleSet := &game.RuleSet{DoubleAfterSplit: true}
	advisor := &StrategyAdvisor{RuleSet: ruleSet}

	// Test pair of aces
	playerHand := []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
		{Rank: cards.Ace, Suit: cards.Hearts, Value: 1},
	}
	dealerUpcard := cards.Card{Rank: cards.Six, Suit: cards.Spades, Value: 6}
	action := advisor.GetRecommendedAction(playerHand, dealerUpcard, 10.0)

	if action != SplitAction {
		t.Errorf("Expected pair of aces to split, got %s", action)
	}

	// Test soft 18 (A+7)
	playerHand = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades, Value: 1},
		{Rank: cards.Seven, Suit: cards.Hearts, Value: 7},
	}
	action = advisor.GetRecommendedAction(playerHand, dealerUpcard, 10.0)

	if action != DoubleAction {
		t.Errorf("Expected soft 18 to double, got %s", action)
	}

	// Test hard 20 (10+10)
	playerHand = []cards.Card{
		{Rank: cards.Ten, Suit: cards.Spades, Value: 10},
		{Rank: cards.Ten, Suit: cards.Hearts, Value: 10},
	}
	action = advisor.GetRecommendedAction(playerHand, dealerUpcard, 10.0)

	if action != StandAction {
		t.Errorf("Expected hard 20 to stand, got %s", action)
	}
}

func TestShouldTakeInsurance(t *testing.T) {
	ruleSet := &game.RuleSet{}
	counter := NewCardCounter(HiLoCount, 6)
	advisor := NewStrategyAdvisor(ruleSet, counter)

	// Create an ace card for dealer
	dealerUpcard := cards.Card{Rank: cards.Ace, Suit: cards.Spades, Value: 1}

	// Initially, the count is neutral, so insurance should not be taken
	shouldTake := advisor.ShouldTakeInsurance(dealerUpcard, 10.0)
	if shouldTake {
		t.Error("Expected not to take insurance with neutral count")
	}

	// Increase the count to make insurance favorable by adding low-value cards (which are +1 in Hi-Lo)
	// To reach TC >= 3 with 6 decks, we need RC >= 3*6 = 18, so let's add 20 low cards to ensure TC > 3
	for i := 0; i < 20; i++ {
		counter.UpdateCount(cards.Card{Rank: cards.Two, Suit: cards.Spades, Value: 2})
	}

	// Now insurance should be favorable (TC should be > 3 after adding 20 low cards)
	shouldTake = advisor.ShouldTakeInsurance(dealerUpcard, 10.0)
	if !shouldTake {
		t.Errorf("Expected to take insurance with high positive count, but TC is %.2f", counter.TrueCount())
	}

	// Test with non-ace upcard (should never take insurance)
	nonAceCard := cards.Card{Rank: cards.Ten, Suit: cards.Spades, Value: 10}
	shouldTake = advisor.ShouldTakeInsurance(nonAceCard, 10.0)
	if shouldTake {
		t.Error("Expected not to take insurance when dealer doesn't show ace")
	}
}

func TestShouldSurrender(t *testing.T) {
	ruleSet := &game.RuleSet{LateSurrender: true}
	advisor := &StrategyAdvisor{RuleSet: ruleSet}

	// Test 16 vs 10 (should surrender)
	playerHand := []cards.Card{
		{Rank: cards.Six, Suit: cards.Spades, Value: 6},
		{Rank: cards.Ten, Suit: cards.Hearts, Value: 10},
	}
	dealerUpcard := cards.Card{Rank: cards.Ten, Suit: cards.Spades, Value: 10}
	shouldSurr := advisor.ShouldSurrender(playerHand, dealerUpcard)

	if !shouldSurr {
		t.Error("Expected to surrender 16 vs 10")
	}

	// Test 15 vs 10 (should surrender)
	playerHand = []cards.Card{
		{Rank: cards.Five, Suit: cards.Spades, Value: 5},
		{Rank: cards.Ten, Suit: cards.Hearts, Value: 10},
	}
	shouldSurr = advisor.ShouldSurrender(playerHand, dealerUpcard)

	if !shouldSurr {
		t.Error("Expected to surrender 15 vs 10")
	}

	// Test 16 vs 6 (should not surrender)
	dealerUpcard = cards.Card{Rank: cards.Six, Suit: cards.Spades, Value: 6}
	shouldSurr = advisor.ShouldSurrender(playerHand, dealerUpcard)

	if shouldSurr {
		t.Error("Expected not to surrender 15 vs 6")
	}

	// Test with surrender disabled
	ruleSet.LateSurrender = false
	shouldSurr = advisor.ShouldSurrender(playerHand, dealerUpcard)

	if shouldSurr {
		t.Error("Expected not to surrender when surrender is disabled")
	}
}
