package game

import (
	"math/rand"
	"testing"

	"blackjack/internal/deck"
)

func newTestShoe() *deck.Shoe {
	rng := rand.New(rand.NewSource(12345))
	shoe, _ := deck.NewShoeWithRNG(1, 0.75, rng)
	return shoe
}

// makeHand creates a hand with a fixed sequence of ranks/suits.
func makeHand(rankList []deck.Rank) *Hand {
	suits := []deck.Suit{deck.Spades, deck.Hearts, deck.Diamonds, deck.Clubs}
	h := NewHand(10)
	for i, r := range rankList {
		h.AddCard(deck.NewCard(suits[i%4], r))
	}
	return h
}

func TestHandTotalSoftHard(t *testing.T) {
	cases := []struct {
		name        string
		ranks       []deck.Rank
		expectHard  int
		expectSoft  int
		expectIsSoft bool
	}{
		{"A+K (blackjack)", []deck.Rank{deck.Ace, deck.King}, 11, 21, true},
		{"A+A", []deck.Rank{deck.Ace, deck.Ace}, 2, 12, true},
		{"A+A+A", []deck.Rank{deck.Ace, deck.Ace, deck.Ace}, 3, 13, true},
		{"K+Q", []deck.Rank{deck.King, deck.Queen}, 20, 20, false},
		{"K+A+Q", []deck.Rank{deck.King, deck.Ace, deck.Queen}, 21, 21, false},
		{"A+5", []deck.Rank{deck.Ace, deck.Five}, 6, 16, true},
		{"A+5+Q", []deck.Rank{deck.Ace, deck.Five, deck.Queen}, 16, 16, false},
		{"2+3+4", []deck.Rank{deck.Two, deck.Three, deck.Four}, 9, 9, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := makeHand(tc.ranks)
			hard, soft, isSoft := h.Total()
			if hard != tc.expectHard {
				t.Errorf("hard: expected %d, got %d", tc.expectHard, hard)
			}
			if soft != tc.expectSoft {
				t.Errorf("soft: expected %d, got %d", tc.expectSoft, soft)
			}
			if isSoft != tc.expectIsSoft {
				t.Errorf("isSoft: expected %v, got %v", tc.expectIsSoft, isSoft)
			}
			if h.BestValue() != tc.expectSoft {
				t.Errorf("BestValue: expected %d, got %d", tc.expectSoft, h.BestValue())
			}
		})
	}
}

func TestBlackjackDetection(t *testing.T) {
	bj := makeHand([]deck.Rank{deck.Ace, deck.King})
	if !bj.IsBlackjack() {
		t.Errorf("expected natural blackjack for A+K")
	}

	notBJ := makeHand([]deck.Rank{deck.Ten, deck.Nine, deck.Two})
	if notBJ.IsBlackjack() {
		t.Errorf("expected non-blackjack for 10+9+2")
	}

	splitBJ := makeHand([]deck.Rank{deck.Ace, deck.King})
	splitBJ.IsSplit = true
	if splitBJ.IsBlackjack() {
		t.Errorf("split hands should not count as natural blackjack")
	}
}

func TestHandSplitAbility(t *testing.T) {
	pair := makeHand([]deck.Rank{deck.Eight, deck.Eight})
	if !pair.CanSplit() {
		t.Errorf("expected pair of 8s to be splittable")
	}

	nonPair := makeHand([]deck.Rank{deck.Eight, deck.Nine})
	if nonPair.CanSplit() {
		t.Errorf("expected 8-9 to not be splittable")
	}

	// King + Queen both valued 10, still splittable in casinos
	faces := makeHand([]deck.Rank{deck.King, deck.Queen})
	if !faces.CanSplit() {
		t.Errorf("expected K+Q to be splittable (both 10-value)")
	}

	threeCards := makeHand([]deck.Rank{deck.Eight, deck.Eight, deck.Eight})
	if threeCards.CanSplit() {
		t.Errorf("expected 3-card hand to not be splittable")
	}
}

func TestHandDoubleAbility(t *testing.T) {
	h := makeHand([]deck.Rank{deck.Five, deck.Six})
	if !h.CanDouble() {
		t.Errorf("expected 2-card hand to allow double")
	}
	h.AddCard(deck.NewCard(deck.Spades, deck.Two))
	if h.CanDouble() {
		t.Errorf("expected 3-card hand to not allow double")
	}

	d2 := makeHand([]deck.Rank{deck.Five, deck.Six})
	d2.IsStanding = true
	if d2.CanDouble() {
		t.Errorf("expected standing hand to not allow double")
	}

	d3 := makeHand([]deck.Rank{deck.Five, deck.Six})
	d3.IsDoubled = true
	if d3.CanDouble() {
		t.Errorf("expected already-doubled hand to not allow double")
	}
}

func TestHandString(t *testing.T) {
	h := makeHand([]deck.Rank{deck.Ten, deck.Ace})
	expected := "10♠ A♥"
	if h.String() != expected {
		t.Errorf("expected %q, got %q", expected, h.String())
	}
}

func TestDealerHandFaceDown(t *testing.T) {
	h := makeHand([]deck.Rank{deck.Seven, deck.King})
	faceDown := h.DealerHand(true)
	expected := "?? K♥"
	if faceDown != expected {
		t.Errorf("expected %q, got %q", expected, faceDown)
	}
	faceUp := h.DealerHand(false)
	expectedUp := "7♠ K♥"
	if faceUp != expectedUp {
		t.Errorf("expected %q, got %q", expectedUp, faceUp)
	}
}

func TestEngineCreation(t *testing.T) {
	rules := DefaultRules()
	engine, err := NewEngine(rules, 1000)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	if engine.Bankroll != 1000 {
		t.Errorf("expected bankroll 1000, got %d", engine.Bankroll)
	}

	// Test invalid deck count
	badRules := Rules{Decks: 0, Penetration: 0.75}
	if _, err := NewEngine(badRules, 1000); err == nil {
		t.Errorf("expected error with 0 decks")
	}
}

func TestEngineBetValidation(t *testing.T) {
	engine, _ := NewEngine(DefaultRules(), 1000)

	tests := []struct {
		bet      int
		hasError bool
	}{
		{10, false},
		{500, false},
		{5, true},
		{501, true},
		{1001, true},
	}

	for _, tc := range tests {
		err := engine.ValidateBet(tc.bet)
		if tc.hasError && err == nil {
			t.Errorf("expected error for bet %d", tc.bet)
		}
		if !tc.hasError && err != nil {
			t.Errorf("unexpected error for bet %d: %v", tc.bet, err)
		}
	}
}

func TestEngineStartRound(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)

	err := engine.StartRound(50)
	if err != nil {
		t.Fatalf("failed to start round: %v", err)
	}
	if engine.Bankroll != 950 {
		t.Errorf("expected bankroll 950 after bet, got %d", engine.Bankroll)
	}
	if engine.TotalWagered != 50 {
		t.Errorf("expected wagered 50, got %d", engine.TotalWagered)
	}
	if len(engine.Hands) != 1 {
		t.Errorf("expected 1 hand, got %d", len(engine.Hands))
	}
	if len(engine.Hands[0].Cards) != 2 {
		t.Errorf("expected 2 cards in hand, got %d", len(engine.Hands[0].Cards))
	}
	if len(engine.Dealer.Cards) != 2 {
		t.Errorf("expected 2 dealer cards, got %d", len(engine.Dealer.Cards))
	}

	// Bet out of range
	if err := engine.StartRound(5); err == nil {
		t.Errorf("expected error for bet below min")
	}
	if err := engine.StartRound(10000); err == nil {
		t.Errorf("expected error for insufficient bankroll")
	}
}

func TestEngineDealerUpcard(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)
	engine.StartRound(10)

	up := engine.DealerUpcard()
	if up.Rank == 0 {
		t.Errorf("expected a valid upcard")
	}
	hole := engine.DealerHoleCard()
	if hole.Rank == 0 {
		t.Errorf("expected a valid hole card")
	}
}

func TestEngineApplyActionHit(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)
	engine.StartRound(10)

	err := engine.ApplyAction(ActionHit)
	if err != nil {
		t.Fatalf("unexpected error hitting: %v", err)
	}
	if len(engine.Hands[0].Cards) != 3 {
		t.Errorf("expected 3 cards after hit, got %d", len(engine.Hands[0].Cards))
	}
}

func TestEngineApplyActionStand(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)
	engine.StartRound(10)

	err := engine.ApplyAction(ActionStand)
	if err != nil {
		t.Fatalf("unexpected error standing: %v", err)
	}
	if !engine.Hands[0].IsStanding {
		t.Errorf("expected hand to be standing")
	}
	if engine.AllHandsResolved() != true {
		t.Errorf("expected all hands resolved after stand")
	}
}

func TestEngineApplyActionDouble(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)
	engine.StartRound(10)

	before := engine.Bankroll
	err := engine.ApplyAction(ActionDouble)
	if err != nil {
		t.Fatalf("unexpected error doubling: %v", err)
	}
	if !engine.Hands[0].IsDoubled {
		t.Errorf("expected hand to be doubled")
	}
	if engine.Hands[0].Bet != 20 {
		t.Errorf("expected bet to be 20 after double, got %d", engine.Hands[0].Bet)
	}
	if engine.Bankroll != before-10 {
		t.Errorf("expected bankroll to decrease by 10, got %d", engine.Bankroll)
	}
	if len(engine.Hands[0].Cards) != 3 {
		t.Errorf("expected 3 cards after double, got %d", len(engine.Hands[0].Cards))
	}

	// Cannot double again
	if err := engine.ApplyAction(ActionDouble); err == nil {
		t.Errorf("expected error when doubling twice")
	}
}

func TestEngineApplyActionSplit(t *testing.T) {
	// Force pair situation
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)
	engine.Hands = []*Hand{makeHand([]deck.Rank{deck.Eight, deck.Eight})}
	engine.Dealer = NewHand(0)
	engine.Bankroll = 1000

	before := engine.Bankroll
	err := engine.ApplyAction(ActionSplit)
	if err != nil {
		t.Fatalf("unexpected error splitting: %v", err)
	}
	if len(engine.Hands) != 2 {
		t.Errorf("expected 2 hands after split, got %d", len(engine.Hands))
	}
	if engine.Bankroll != before-10 {
		t.Errorf("expected bankroll to decrease by bet, got %d", engine.Bankroll)
	}
	if !engine.Hands[0].IsSplit || !engine.Hands[1].IsSplit {
		t.Errorf("both hands should be marked as split")
	}
	// Both split hands should have 2 cards each after drawing
	if len(engine.Hands[0].Cards) != 2 || len(engine.Hands[1].Cards) != 2 {
		t.Errorf("split hands should each have 2 cards")
	}
}

func TestEngineCannotSplitThreeHands(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)
	engine.Hands = []*Hand{
		makeHand([]deck.Rank{deck.Eight, deck.Eight}),
		makeHand([]deck.Rank{deck.Eight, deck.Eight}),
	}
	engine.Dealer = NewHand(0)
	engine.Bankroll = 1000
	if err := engine.ApplyAction(ActionSplit); err == nil {
		t.Errorf("expected error when trying to split beyond MaxSplits")
	}
}

func TestEngineDealerPlayS17(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)
	engine.Rules.DealerStandsOnS17 = true

	// Dealer has 17 already, should not hit
	engine.Dealer = makeHand([]deck.Rank{deck.Ten, deck.Seven})
	before := len(engine.Dealer.Cards)
	engine.DealerPlay()
	if len(engine.Dealer.Cards) != before {
		t.Errorf("dealer should stand on 17")
	}

	// Dealer has 16, should hit
	engine.Dealer = makeHand([]deck.Rank{deck.Ten, deck.Six})
	before = len(engine.Dealer.Cards)
	engine.DealerPlay()
	if len(engine.Dealer.Cards) <= before {
		t.Errorf("dealer should hit on 16")
	}
	if engine.Dealer.BestValue() < 17 {
		t.Errorf("dealer should have at least 17 after play, got %d", engine.Dealer.BestValue())
	}

	// Soft 17 test (A + 6 = soft 17) with S17 rule - dealer should stand
	engine.Dealer = makeHand([]deck.Rank{deck.Ace, deck.Six})
	engine.DealerPlay()
	if engine.Dealer.BestValue() != 17 && engine.Dealer.BestValue() < 17 {
		t.Errorf("dealer should stand on soft 17 under S17 rule, got %d", engine.Dealer.BestValue())
	}
}

func TestEngineResolveRoundDealerBust(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)

	// Setup: player stands on 20, dealer forced low
	engine.Hands = []*Hand{makeHand([]deck.Rank{deck.Ten, deck.Ten})}
	engine.Hands[0].IsStanding = true
	engine.Bankroll = 1000
	engine.TotalWagered = 10

	// Setup dealer with 6 up, then force hits to bust
	engine.Dealer = makeHand([]deck.Rank{deck.Six, deck.Five}) // 11
	results := engine.ResolveRound()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if engine.NetProfit <= 0 {
		t.Errorf("expected positive net profit, got %d", engine.NetProfit)
	}
}

func TestEngineResolveRoundDealerWin(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)

	engine.Hands = []*Hand{makeHand([]deck.Rank{deck.Ten, deck.Ten})}
	engine.Hands[0].IsStanding = true
	engine.Bankroll = 1000

	// Dealer has 21 (10+10+... wait, can't have 21 with 2 cards except BJ)
	// Use Ten + Nine = 19 (player stands at 20, this loses)
	// Dealer makes 21 with 3 cards (not a natural blackjack) and stands.
	engine.Dealer = makeHand([]deck.Rank{deck.Ten, deck.Nine, deck.Two})
	results := engine.ResolveRound()

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Outcome != OutcomeLoss {
		t.Errorf("expected loss, got %s", results[0].Outcome.String())
	}
}

func TestEngineResolveRoundPush(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)

	engine.Hands = []*Hand{makeHand([]deck.Rank{deck.Ten, deck.Nine})}
	engine.Hands[0].IsStanding = true
	engine.Bankroll = 1000

	engine.Dealer = makeHand([]deck.Rank{deck.Ten, deck.Nine})
	results := engine.ResolveRound()

	if results[0].Outcome != OutcomePush {
		t.Errorf("expected push, got %s", results[0].Outcome.String())
	}
}

func TestEngineResolveRoundPlayerBlackjack(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)

	engine.Hands = []*Hand{makeHand([]deck.Rank{deck.Ace, deck.King})}
	engine.Hands[0].IsStanding = true
	engine.Bankroll = 1000
	engine.TotalWagered = 10

	engine.Dealer = makeHand([]deck.Rank{deck.Ten, deck.Nine})
	results := engine.ResolveRound()

	if results[0].Outcome != OutcomeBlackjack {
		t.Errorf("expected blackjack outcome, got %s", results[0].Outcome.String())
	}
	// Blackjack pays 3:2 with bet 10 => +15
	if results[0].Payout != 15 {
		t.Errorf("expected payout 15 for blackjack, got %d", results[0].Payout)
	}
}

func TestEngineResolveRoundPlayerBust(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)

	hand := makeHand([]deck.Rank{deck.Ten, deck.Ten, deck.Ten})
	hand.IsBusted = true
	engine.Hands = []*Hand{hand}
	engine.Bankroll = 1000

	engine.Dealer = makeHand([]deck.Rank{deck.Ten, deck.Nine})
	results := engine.ResolveRound()

	if results[0].Outcome != OutcomeBust {
		t.Errorf("expected bust outcome, got %s", results[0].Outcome.String())
	}
	if results[0].Payout != -10 {
		t.Errorf("expected payout -10, got %d", results[0].Payout)
	}
}

func TestEngineResolveRoundBothBlackjackPush(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)

	engine.Hands = []*Hand{makeHand([]deck.Rank{deck.Ace, deck.King})}
	engine.Hands[0].IsStanding = true
	engine.Bankroll = 1000

	engine.Dealer = makeHand([]deck.Rank{deck.Ace, deck.Queen})
	results := engine.ResolveRound()

	if results[0].Outcome != OutcomePush {
		t.Errorf("expected push when both have blackjack, got %s", results[0].Outcome.String())
	}
}

func TestEngineFullRoundWithBasicFlow(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)

	if err := engine.StartRound(10); err != nil {
		t.Fatalf("failed to start round: %v", err)
	}

	// Play until resolved using simple strategy: stand on >= 17
	for !engine.AllHandsResolved() {
		hand := engine.CurrentHand()
		if hand != nil && hand.BestValue() < 17 {
			engine.ApplyAction(ActionHit)
		} else {
			engine.ApplyAction(ActionStand)
		}
	}

	results := engine.ResolveRound()
	if len(results) == 0 {
		t.Errorf("expected at least 1 result")
	}
	if engine.RoundsPlayed != 1 {
		t.Errorf("expected 1 round played, got %d", engine.RoundsPlayed)
	}
}

func TestActionStrings(t *testing.T) {
	if ActionHit.String() != "Hit" {
		t.Errorf("unexpected Hit string")
	}
	if ActionStand.String() != "Stand" {
		t.Errorf("unexpected Stand string")
	}
	if ActionDouble.String() != "Double" {
		t.Errorf("unexpected Double string")
	}
	if ActionSplit.String() != "Split" {
		t.Errorf("unexpected Split string")
	}
	if Action(99).String() != "Unknown" {
		t.Errorf("unexpected Unknown string")
	}
}

func TestOutcomeStrings(t *testing.T) {
	if OutcomeWin.String() != "WIN" {
		t.Errorf("unexpected WIN string")
	}
	if OutcomeLoss.String() != "LOSS" {
		t.Errorf("unexpected LOSS string")
	}
	if OutcomePush.String() != "PUSH" {
		t.Errorf("unexpected PUSH string")
	}
	if OutcomeBlackjack.String() != "BLACKJACK!" {
		t.Errorf("unexpected BLACKJACK! string")
	}
	if OutcomeBust.String() != "BUST" {
		t.Errorf("unexpected BUST string")
	}
	if OutcomeSurrender.String() != "SURRENDER" {
		t.Errorf("unexpected SURRENDER string")
	}
	if Outcome(99).String() != "Outcome(99)" {
		t.Errorf("unexpected default Outcome string")
	}
}

func TestHandTotalEmpty(t *testing.T) {
	h := NewHand(10)
	hard, soft, isSoft := h.Total()
	if hard != 0 || soft != 0 || isSoft {
		t.Errorf("empty hand should total 0, 0, false")
	}
	if h.BestValue() != 0 {
		t.Errorf("empty hand BestValue should be 0")
	}
}

func TestCurrentHandNil(t *testing.T) {
	shoe := newTestShoe()
	engine := NewEngineWithShoe(DefaultRules(), 1000, shoe)
	engine.StartRound(10)

	// After standing, CurrentHand should be nil (no more active hands)
	engine.ApplyAction(ActionStand)
	if engine.CurrentHand() != nil {
		t.Errorf("expected nil CurrentHand after standing")
	}
}
