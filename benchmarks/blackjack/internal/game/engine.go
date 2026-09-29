package game

import (
	"blackjack/internal/deck"
	"errors"
	"fmt"
)

// Action represents a player decision.
type Action int

const (
	ActionHit Action = iota
	ActionStand
	ActionDouble
	ActionSplit
)

func (a Action) String() string {
	switch a {
	case ActionHit:
		return "Hit"
	case ActionStand:
		return "Stand"
	case ActionDouble:
		return "Double"
	case ActionSplit:
		return "Split"
	default:
		return "Unknown"
	}
}

// Outcome represents a resolved hand result.
type Outcome int

const (
	OutcomeWin Outcome = iota
	OutcomeLoss
	OutcomePush
	OutcomeBlackjack
	OutcomeBust
	OutcomeSurrender
)

// Rules represents the casino rules configuration.
type Rules struct {
	Decks             int
	Penetration       float64
	DealerStandsOnS17 bool
	BlackjackPays3To2 bool
	DoubleAfterSplit  bool
	MaxSplits         int
	MinBet            int
	MaxBet            int
}

// DefaultRules returns the standard casino rules used by this simulator.
func DefaultRules() Rules {
	return Rules{
		Decks:             6,
		Penetration:       0.75,
		DealerStandsOnS17: true,
		BlackjackPays3To2: true,
		DoubleAfterSplit:  true,
		MaxSplits:         1, // split into 2 active hands
		MinBet:            10,
		MaxBet:            500,
	}
}

// RoundResult captures a summary of a resolved round for analysis.
type RoundResult struct {
	Outcome        Outcome
	Bet            int
	Payout         int // net profit/loss (negative for loss)
	IsBlackjackWin bool
}

// Engine encapsulates game state and rules.
type Engine struct {
	Shoe     *deck.Shoe
	Rules    Rules
	Dealer   *Hand
	Hands    []*Hand
	Bankroll int
	// Stats
	RoundsPlayed int
	Wins         int
	Losses       int
	Pushes       int
	Blackjacks   int
	TotalWagered int
	NetProfit    int
}

var (
	ErrInsufficientBankroll = errors.New("insufficient bankroll for bet")
	ErrBetOutOfRange        = errors.New("bet out of allowed range")
	ErrCannotDouble         = errors.New("cannot double with more than 2 cards")
	ErrCannotSplit          = errors.New("cannot split this hand")
	ErrInvalidAction        = errors.New("invalid action")
)

// NewEngine creates a new blackjack engine with the specified rules and bankroll.
func NewEngine(rules Rules, bankroll int) (*Engine, error) {
	shoe, err := deck.NewShoe(rules.Decks, rules.Penetration)
	if err != nil {
		return nil, err
	}
	return &Engine{
		Shoe:     shoe,
		Rules:    rules,
		Bankroll: bankroll,
	}, nil
}

// NewEngineWithShoe creates an engine using an existing (pre-configured) shoe.
func NewEngineWithShoe(rules Rules, bankroll int, shoe *deck.Shoe) *Engine {
	return &Engine{
		Shoe:     shoe,
		Rules:    rules,
		Bankroll: bankroll,
	}
}

// ValidateBet ensures the bet is within the min/max range and affordable.
func (e *Engine) ValidateBet(bet int) error {
	if bet < e.Rules.MinBet || bet > e.Rules.MaxBet {
		return ErrBetOutOfRange
	}
	if bet > e.Bankroll {
		return ErrInsufficientBankroll
	}
	return nil
}

// EnsureShoeReady reshuffles the shoe if not enough cards remain for a round.
func (e *Engine) EnsureShoeReady() {
	if e.Shoe.Remaining() < 10 {
		e.Shoe.Reset()
	}
}

// StartRound begins a new round by placing bet and dealing initial cards.
func (e *Engine) StartRound(bet int) error {
	if err := e.ValidateBet(bet); err != nil {
		return err
	}

	e.EnsureShoeReady()

	e.Hands = []*Hand{NewHand(bet)}
	e.Dealer = NewHand(0)
	e.Bankroll -= bet
	e.TotalWagered += bet

	// Deal initial cards: Player, Dealer, Player, Dealer
	p1, _ := e.Shoe.Draw()
	d1, _ := e.Shoe.Draw()
	p2, _ := e.Shoe.Draw()
	d2, _ := e.Shoe.Draw()

	e.Hands[0].AddCard(p1)
	e.Dealer.AddCard(d1)
	e.Hands[0].AddCard(p2)
	e.Dealer.AddCard(d2)

	return nil
}

// CurrentHand returns the active hand (or nil if none active).
func (e *Engine) CurrentHand() *Hand {
	for _, h := range e.Hands {
		if !h.IsStanding && !h.IsBusted {
			return h
		}
	}
	return nil
}

// PlayerHasBlackjack returns true if the player's hand is a natural blackjack.
func (e *Engine) PlayerHasBlackjack() bool {
	return e.Hands != nil && len(e.Hands) > 0 && e.Hands[0].IsBlackjack()
}

// DealerHasBlackjack returns true if the dealer has a natural blackjack (upcard + hole card).
func (e *Engine) DealerHasBlackjack() bool {
	if e.Dealer == nil || len(e.Dealer.Cards) != 2 {
		return false
	}
	_, soft, _ := e.Dealer.Total()
	return soft == 21
}

// DealerUpcard returns the dealer's face-up card.
func (e *Engine) DealerUpcard() deck.Card {
	if e.Dealer == nil || len(e.Dealer.Cards) < 2 {
		return deck.Card{}
	}
	return e.Dealer.Cards[0]
}

// DealerHoleCard returns the dealer's face-down card.
func (e *Engine) DealerHoleCard() deck.Card {
	if e.Dealer == nil || len(e.Dealer.Cards) < 2 {
		return deck.Card{}
	}
	return e.Dealer.Cards[1]
}

// ApplyAction processes the requested action against the active hand.
func (e *Engine) ApplyAction(action Action) error {
	hand := e.CurrentHand()
	if hand == nil {
		return ErrInvalidAction
	}

	switch action {
	case ActionHit:
		card, err := e.Shoe.Draw()
		if err != nil {
			return err
		}
		hand.AddCard(card)
		if hand.BestValue() > 21 {
			hand.IsBusted = true
		}

	case ActionStand:
		hand.IsStanding = true

	case ActionDouble:
		if !hand.CanDouble() {
			return ErrCannotDouble
		}
		if hand.Bet > e.Bankroll {
			return ErrInsufficientBankroll
		}
		e.Bankroll -= hand.Bet
		e.TotalWagered += hand.Bet
		hand.Bet *= 2
		hand.IsDoubled = true
		card, err := e.Shoe.Draw()
		if err != nil {
			return err
		}
		hand.AddCard(card)
		if hand.BestValue() > 21 {
			hand.IsBusted = true
		}
		hand.IsStanding = true

	case ActionSplit:
		if !hand.CanSplit() {
			return ErrCannotSplit
		}
		if hand.Bet > e.Bankroll {
			return ErrInsufficientBankroll
		}
		if len(e.Hands) >= e.Rules.MaxSplits+1 {
			return ErrCannotSplit
		}

		// Take second card for the new hand
		secondCard := hand.Cards[1]
		hand.Cards = hand.Cards[:1]
		hand.IsSplit = true

		newHand := NewHand(hand.Bet)
		newHand.AddCard(secondCard)
		newHand.IsSplit = true

		e.Bankroll -= hand.Bet
		e.TotalWagered += hand.Bet

		// Register the new hand before drawing so both hands receive one card each.
		e.Hands = append(e.Hands, newHand)

		// Draw one new card for each split hand
		for _, h := range e.Hands {
			card, err := e.Shoe.Draw()
			if err != nil {
				return err
			}
			h.AddCard(card)
		}

	default:
		return ErrInvalidAction
	}

	return nil
}

// AllHandsResolved checks if all player hands have finished (bust, stand, blackjack).
func (e *Engine) AllHandsResolved() bool {
	if e.Hands == nil || len(e.Hands) == 0 {
		return true
	}
	for _, h := range e.Hands {
		if !h.IsStanding && !h.IsBusted {
			return false
		}
	}
	return true
}

// DealerShouldHit checks if dealer should hit (hits until 17, stands on soft 17).
func (e *Engine) DealerShouldHit() bool {
	if e.Dealer == nil {
		return false
	}
	hard, soft, isSoft := e.Dealer.Total()
	if e.Rules.DealerStandsOnS17 {
		// S17: dealer stands on all 17s (including soft 17)
		if isSoft {
			return soft < 17
		}
		return hard < 17
	}
	// H17: dealer hits on soft 17
	if isSoft {
		return soft < 18
	}
	return hard < 17
}

// DealerPlay runs the dealer's turn until they stand or bust.
func (e *Engine) DealerPlay() {
	for e.DealerShouldHit() {
		card, err := e.Shoe.Draw()
		if err != nil {
			// auto reshuffle if out of cards
			e.Shoe.Reset()
			continue
		}
		e.Dealer.AddCard(card)
	}
}

// ResolveRound resolves all hands against dealer and returns round results.
func (e *Engine) ResolveRound() []RoundResult {
	results := make([]RoundResult, 0, len(e.Hands))

	dealerBJ := e.DealerHasBlackjack()
	dealerVal := 0
	dealerBusted := false
	if e.Dealer != nil {
		e.DealerPlay()
		dealerVal = e.Dealer.BestValue()
		dealerBusted = dealerVal > 21
	}

	for _, hand := range e.Hands {
		playerVal := hand.BestValue()
		var outcome Outcome
		var payout int

		if hand.IsBusted {
			outcome = OutcomeBust
			payout = -hand.Bet
		} else if hand.IsSurrender {
			outcome = OutcomeSurrender
			payout = -hand.Bet / 2
		} else if hand.IsBlackjack() && !dealerBJ {
			outcome = OutcomeBlackjack
			if e.Rules.BlackjackPays3To2 {
				payout = hand.Bet * 3 / 2
			} else {
				payout = hand.Bet
			}
		} else if hand.IsBlackjack() && dealerBJ {
			outcome = OutcomePush
			payout = 0
		} else if dealerBJ {
			outcome = OutcomeLoss
			payout = -hand.Bet
		} else if dealerBusted {
			outcome = OutcomeWin
			payout = hand.Bet
		} else if playerVal > dealerVal {
			outcome = OutcomeWin
			payout = hand.Bet
		} else if playerVal < dealerVal {
			outcome = OutcomeLoss
			payout = -hand.Bet
		} else {
			outcome = OutcomePush
			payout = 0
		}

		// Payout is net profit/loss; bankroll gets original stake back plus/minus payout.
		e.Bankroll += hand.Bet + payout
		e.NetProfit += payout

		switch outcome {
		case OutcomeWin:
			e.Wins++
		case OutcomeBlackjack:
			e.Wins++
			e.Blackjacks++
		case OutcomeLoss, OutcomeBust:
			e.Losses++
		case OutcomePush:
			e.Pushes++
		case OutcomeSurrender:
			e.Losses++
		}

		results = append(results, RoundResult{
			Outcome:        outcome,
			Bet:            hand.Bet,
			Payout:         payout,
			IsBlackjackWin: outcome == OutcomeBlackjack,
		})
	}

	e.RoundsPlayed++

	return results
}

// String describes the outcome.
func (o Outcome) String() string {
	switch o {
	case OutcomeWin:
		return "WIN"
	case OutcomeLoss:
		return "LOSS"
	case OutcomePush:
		return "PUSH"
	case OutcomeBlackjack:
		return "BLACKJACK!"
	case OutcomeBust:
		return "BUST"
	case OutcomeSurrender:
		return "SURRENDER"
	default:
		return fmt.Sprintf("Outcome(%d)", int(o))
	}
}
