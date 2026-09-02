package engine

import (
	"blackjack/pkg/cards"
	"blackjack/pkg/rules"
	"errors"
	"fmt"
)

// GameState represents the current step in a Blackjack round.
type GameState int

const (
	StateWaitingBet GameState = iota
	StateInsuranceOffered
	StatePlayerTurn
	StateDealerTurn
	StateRoundResolved
)

func (s GameState) String() string {
	switch s {
	case StateWaitingBet:
		return "Waiting For Bet"
	case StateInsuranceOffered:
		return "Insurance Offered"
	case StatePlayerTurn:
		return "Player Turn"
	case StateDealerTurn:
		return "Dealer Turn"
	case StateRoundResolved:
		return "Round Resolved"
	default:
		return "Unknown State"
	}
}

// PlayerAction represents an action chosen by the player.
type PlayerAction int

const (
	ActionHit PlayerAction = iota
	ActionStand
	ActionDouble
	ActionSplit
	ActionSurrender
	ActionInsuranceAccept
	ActionInsuranceDecline
)

func (a PlayerAction) String() string {
	switch a {
	case ActionHit:
		return "Hit"
	case ActionStand:
		return "Stand"
	case ActionDouble:
		return "Double Down"
	case ActionSplit:
		return "Split"
	case ActionSurrender:
		return "Surrender"
	case ActionInsuranceAccept:
		return "Insurance Accept"
	case ActionInsuranceDecline:
		return "Insurance Decline"
	default:
		return "Unknown Action"
	}
}

// GameEngine orchestrates a Blackjack table with deterministic state transitions.
type GameEngine struct {
	Rules          rules.RuleSet
	Shoe           *cards.Shoe
	State          GameState
	DealerHand     *Hand
	PlayerHands    []*Hand
	ActiveHandIdx  int
	DealerHoleCard cards.Card
	HasHoleCard    bool
	InitialBet     float64
}

// NewGameEngine creates a new game engine instance with the specified rules and shoe.
func NewGameEngine(ruleSet rules.RuleSet, shoe *cards.Shoe) *GameEngine {
	if shoe == nil {
		shoe = cards.NewShoe(ruleSet.Decks, cards.WithCutCardPenetration(ruleSet.DeckPenetration))
	}
	return &GameEngine{
		Rules:         ruleSet,
		Shoe:          shoe,
		State:         StateWaitingBet,
		PlayerHands:   make([]*Hand, 0, 4),
		DealerHand:    NewHand(0),
		ActiveHandIdx: 0,
	}
}

// StartRound places initial bet and deals the opening cards.
func (g *GameEngine) StartRound(bet float64) error {
	if bet <= 0 {
		return errors.New("bet must be greater than 0")
	}
	if g.State != StateWaitingBet && g.State != StateRoundResolved {
		return fmt.Errorf("cannot start round in state: %s", g.State)
	}

	if g.Shoe.NeedsShuffle() {
		g.Shoe.ResetAndShuffle()
	}

	g.InitialBet = bet
	g.PlayerHands = []*Hand{NewHand(bet)}
	g.ActiveHandIdx = 0
	g.DealerHand = NewHand(0)
	g.HasHoleCard = false

	// Initial deal: Player, Dealer Upcard, Player, Dealer Hole Card (if US Peek / standard)
	pCard1 := g.Shoe.Deal()
	g.PlayerHands[0].AddCard(pCard1)

	dCard1 := g.Shoe.Deal()
	g.DealerHand.AddCard(dCard1)

	pCard2 := g.Shoe.Deal()
	g.PlayerHands[0].AddCard(pCard2)

	if g.Rules.DealerPeeksHoleCard {
		g.DealerHoleCard = g.Shoe.Deal()
		g.HasHoleCard = true
	}

	// Check for Dealer Ace (Insurance offer)
	if dCard1.IsAce() && g.Rules.InsuranceOffered {
		g.State = StateInsuranceOffered
		return nil
	}

	// If no insurance, check dealer peek / immediate Blackjack
	return g.proceedAfterInsurance()
}

// Insurance responds to insurance offer.
func (g *GameEngine) Insurance(accept bool) error {
	if g.State != StateInsuranceOffered {
		return errors.New("insurance is not currently offered")
	}

	if accept {
		// Insurance costs 0.5x original bet
		insBet := g.InitialBet * 0.5
		g.PlayerHands[0].InsuranceBet = insBet
	}

	return g.proceedAfterInsurance()
}

func (g *GameEngine) proceedAfterInsurance() error {
	dealerHasBJ := false
	if g.HasHoleCard {
		// Test if dealer has Blackjack with hole card
		dScore := g.DealerHand.Cards[0].Rank.BlackjackValue() + g.DealerHoleCard.Rank.BlackjackValue()
		if dScore == 21 {
			dealerHasBJ = true
		}
	}

	// Process insurance win/loss if dealer has BJ
	if g.PlayerHands[0].InsuranceBet > 0 {
		if dealerHasBJ {
			g.PlayerHands[0].InsuranceWon = true
		}
	}

	if dealerHasBJ {
		// Reveal hole card and resolve immediately
		g.DealerHand.AddCard(g.DealerHoleCard)
		g.HasHoleCard = false
		g.DealerHand.Status = StatusBlackjack

		if g.PlayerHands[0].IsBlackjack() {
			g.PlayerHands[0].Status = StatusBlackjack
		} else {
			g.PlayerHands[0].Status = StatusStood
		}
		g.resolveRound()
		return nil
	}

	// Dealer does not have BJ (or ENHC mode where dealer hasn't drawn 2nd card)
	if g.PlayerHands[0].IsBlackjack() {
		g.PlayerHands[0].Status = StatusBlackjack
		// Dealer plays out or round resolves
		g.State = StateDealerTurn
		return g.playDealerTurn()
	}

	g.State = StatePlayerTurn
	g.ActiveHandIdx = 0
	return nil
}

// AvailableActions returns the list of valid actions for the active player hand.
func (g *GameEngine) AvailableActions() []PlayerAction {
	if g.State != StatePlayerTurn {
		if g.State == StateInsuranceOffered {
			return []PlayerAction{ActionInsuranceAccept, ActionInsuranceDecline}
		}
		return nil
	}

	hand := g.ActiveHand()
	if hand == nil || hand.Status != StatusInPlay {
		return nil
	}

	actions := []PlayerAction{ActionHit, ActionStand}

	// Double Down
	if hand.CanDouble() {
		canDoubleRule := true
		if hand.IsSplitHand && !g.Rules.DoubleAfterSplit {
			canDoubleRule = false
		}
		if canDoubleRule {
			tot := hand.Total()
			switch g.Rules.DoubleRestriction {
			case rules.Double9To11Only:
				if tot < 9 || tot > 11 {
					canDoubleRule = false
				}
			case rules.Double10Or11Only:
				if tot < 10 || tot > 11 {
					canDoubleRule = false
				}
			}
		}
		if canDoubleRule {
			actions = append(actions, ActionDouble)
		}
	}

	// Split
	if hand.CanSplit() && len(g.PlayerHands) < g.Rules.MaxSplitHands {
		if hand.Cards[0].IsAce() {
			// Ace splitting restrictions
			if !hand.IsSplitHand || g.Rules.ResplitAces {
				actions = append(actions, ActionSplit)
			}
		} else {
			actions = append(actions, ActionSplit)
		}
	}

	// Surrender
	if hand.CanSurrender() && g.Rules.Surrender == rules.SurrenderLate {
		actions = append(actions, ActionSurrender)
	}

	return actions
}

// ActiveHand returns the pointer to currently active player hand.
func (g *GameEngine) ActiveHand() *Hand {
	if g.ActiveHandIdx >= 0 && g.ActiveHandIdx < len(g.PlayerHands) {
		return g.PlayerHands[g.ActiveHandIdx]
	}
	return nil
}

// Step applies a player action to the active hand.
func (g *GameEngine) Step(action PlayerAction) error {
	if g.State != StatePlayerTurn {
		if g.State == StateInsuranceOffered {
			if action == ActionInsuranceAccept {
				return g.Insurance(true)
			} else if action == ActionInsuranceDecline {
				return g.Insurance(false)
			}
		}
		return fmt.Errorf("action %s not allowed in state %s", action, g.State)
	}

	hand := g.ActiveHand()
	if hand == nil || hand.Status != StatusInPlay {
		return errors.New("no active hand in play")
	}

	switch action {
	case ActionHit:
		return g.actionHit(hand)
	case ActionStand:
		return g.actionStand(hand)
	case ActionDouble:
		return g.actionDouble(hand)
	case ActionSplit:
		return g.actionSplit(hand)
	case ActionSurrender:
		return g.actionSurrender(hand)
	default:
		return fmt.Errorf("invalid action: %s", action)
	}
}

func (g *GameEngine) actionHit(hand *Hand) error {
	card := g.Shoe.Deal()
	hand.AddCard(card)

	if hand.IsBusted() {
		hand.Status = StatusBusted
		return g.advanceToNextHand()
	}

	if hand.Total() == 21 {
		hand.Status = StatusStood
		return g.advanceToNextHand()
	}

	return nil
}

func (g *GameEngine) actionStand(hand *Hand) error {
	hand.Status = StatusStood
	return g.advanceToNextHand()
}

func (g *GameEngine) actionDouble(hand *Hand) error {
	if !hand.CanDouble() {
		return errors.New("cannot double this hand")
	}

	hand.Bet *= 2
	hand.Doubled = true
	card := g.Shoe.Deal()
	hand.AddCard(card)

	if hand.IsBusted() {
		hand.Status = StatusBusted
	} else {
		hand.Status = StatusDoubled
	}

	return g.advanceToNextHand()
}

func (g *GameEngine) actionSplit(hand *Hand) error {
	if !hand.CanSplit() || len(g.PlayerHands) >= g.Rules.MaxSplitHands {
		return errors.New("cannot split this hand")
	}

	isAceSplit := hand.Cards[0].IsAce()
	card1 := hand.Cards[0]
	card2 := hand.Cards[1]

	// First hand keeps card1, draws 1 new card
	hand.Cards = []cards.Card{card1}
	hand.IsSplitHand = true
	if isAceSplit {
		hand.FromSplitAces = true
	}
	newCard1 := g.Shoe.Deal()
	hand.AddCard(newCard1)

	// Create second hand with card2, draws 1 new card
	newHand := NewHand(hand.Bet)
	newHand.Cards = []cards.Card{card2}
	newHand.IsSplitHand = true
	if isAceSplit {
		newHand.FromSplitAces = true
	}
	newCard2 := g.Shoe.Deal()
	newHand.AddCard(newCard2)

	// Insert second hand right after current hand
	g.PlayerHands = append(g.PlayerHands[:g.ActiveHandIdx+1], append([]*Hand{newHand}, g.PlayerHands[g.ActiveHandIdx+1:]...)...)

	// If splitting Aces and rule prohibits hitting split Aces, both stand automatically
	if isAceSplit && !g.Rules.HitSplitAces {
		hand.Status = StatusStood
		newHand.Status = StatusStood
		return g.advanceToNextHand()
	}

	// If hand total is 21 after receiving card, auto stand
	if hand.Total() == 21 {
		hand.Status = StatusStood
		return g.advanceToNextHand()
	}

	return nil
}

func (g *GameEngine) actionSurrender(hand *Hand) error {
	if !hand.CanSurrender() || g.Rules.Surrender != rules.SurrenderLate {
		return errors.New("surrender is not permitted")
	}

	hand.Status = StatusSurrendered
	return g.advanceToNextHand()
}

func (g *GameEngine) advanceToNextHand() error {
	for i := g.ActiveHandIdx; i < len(g.PlayerHands); i++ {
		if g.PlayerHands[i].Status == StatusInPlay {
			g.ActiveHandIdx = i
			return nil
		}
	}

	// All player hands resolved
	g.State = StateDealerTurn
	return g.playDealerTurn()
}

func (g *GameEngine) playDealerTurn() error {
	// Reveal hole card if present, or deal dealer's 2nd card in European (ENHC) mode
	if g.HasHoleCard {
		g.DealerHand.AddCard(g.DealerHoleCard)
		g.HasHoleCard = false
	} else if len(g.DealerHand.Cards) == 1 {
		// In ENHC mode dealer draws 2nd card now
		g.DealerHand.AddCard(g.Shoe.Deal())
	}

	// Check if all player hands busted or surrendered - if so, dealer doesn't need to draw further
	allDead := true
	for _, h := range g.PlayerHands {
		if h.Status != StatusBusted && h.Status != StatusSurrendered {
			allDead = false
			break
		}
	}

	if !allDead {
		for {
			tot, isSoft := g.DealerHand.Score()
			if tot < 17 {
				c := g.Shoe.Deal()
				g.DealerHand.AddCard(c)
				continue
			}
			if tot == 17 && isSoft && g.Rules.DealerHitsSoft17 {
				c := g.Shoe.Deal()
				g.DealerHand.AddCard(c)
				continue
			}
			break
		}
	}

	if g.DealerHand.IsBusted() {
		g.DealerHand.Status = StatusBusted
	} else if g.DealerHand.IsBlackjack() {
		g.DealerHand.Status = StatusBlackjack
	} else {
		g.DealerHand.Status = StatusStood
	}

	g.resolveRound()
	return nil
}

func (g *GameEngine) resolveRound() {
	g.State = StateRoundResolved
	dealerTotal := g.DealerHand.Total()
	dealerBJ := g.DealerHand.IsBlackjack()
	dealerBust := g.DealerHand.IsBusted()

	for _, h := range g.PlayerHands {
		pTotal := h.Total()
		pBJ := h.IsBlackjack()

		var netProfit float64
		var payout float64

		// Process Insurance
		if h.InsuranceBet > 0 {
			if h.InsuranceWon {
				netProfit += h.InsuranceBet * 2.0 // Insurance pays 2:1
			} else {
				netProfit -= h.InsuranceBet
			}
		}

		if h.Status == StatusSurrendered {
			payout = -0.5
			netProfit += -0.5 * h.Bet
		} else if h.Status == StatusBusted {
			payout = -1.0
			netProfit += -1.0 * h.Bet
		} else if dealerBJ {
			if pBJ {
				payout = 0.0 // Push
				netProfit += 0.0
			} else {
				payout = -1.0
				netProfit += -1.0 * h.Bet
			}
		} else if pBJ {
			// Natural player Blackjack wins payout ratio (e.g. 1.5x)
			bjMultiplier := float64(g.Rules.BlackjackPayout)
			payout = bjMultiplier
			netProfit += bjMultiplier * h.Bet
		} else if dealerBust {
			// Dealer busted, player not busted
			payout = 1.0
			netProfit += 1.0 * h.Bet
		} else {
			// Compare totals
			if pTotal > dealerTotal {
				payout = 1.0
				netProfit += 1.0 * h.Bet
			} else if pTotal < dealerTotal {
				payout = -1.0
				netProfit += -1.0 * h.Bet
			} else {
				payout = 0.0 // Push
				netProfit += 0.0
			}
		}

		h.Payout = payout
		h.NetProfit = netProfit
	}
}

// NetRoundProfit returns the aggregate profit/loss across all split hands and insurance.
func (g *GameEngine) NetRoundProfit() float64 {
	var total float64
	for _, h := range g.PlayerHands {
		total += h.NetProfit
	}
	return total
}

// DealerUpcard returns the visible dealer upcard (first card dealt to dealer).
func (g *GameEngine) DealerUpcard() cards.Card {
	if g.DealerHand != nil && len(g.DealerHand.Cards) > 0 {
		return g.DealerHand.Cards[0]
	}
	return cards.Card{}
}
