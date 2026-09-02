package game

import (
	"errors"
	"math"

	"blackjack/pkg/cards"
)

// GameState represents the current state of the game
type GameState string

const (
	BettingState    GameState = "betting"
	DealingState    GameState = "dealing"
	PlayerTurnState GameState = "player_turn"
	DealerTurnState GameState = "dealer_turn"
	EndRoundState   GameState = "end_round"
	GameOverState   GameState = "game_over"
)

// Hand represents a player's or dealer's hand
type Hand struct {
	Cards       []cards.Card
	Bet         float64
	Active      bool
	IsSplit     bool
	IsDouble    bool
	IsBusted    bool
	IsBlackjack bool
}

// Value calculates the best possible value of a hand (with aces as 1 or 11)
func (h *Hand) Value() int {
	minVal := 0
	aces := 0

	for _, card := range h.Cards {
		if card.Rank == cards.Ace {
			aces++
			minVal += 1
		} else {
			minVal += card.Value
		}
	}

	if aces > 0 && minVal+10 <= 21 {
		return minVal + 10
	}
	return minVal
}

// IsSoft returns true if the hand contains an ace counted as 11
func (h *Hand) IsSoft() bool {
	minVal := 0
	hasAce := false

	for _, card := range h.Cards {
		if card.Rank == cards.Ace {
			hasAce = true
			minVal += 1
		} else {
			minVal += card.Value
		}
	}

	return hasAce && (minVal+10 <= 21)
}

// CanSplit returns true if the hand can be split (same rank cards, not already split beyond max)
func (h *Hand) CanSplit(rules *RuleSet) bool {
	if len(h.Cards) != 2 {
		return false
	}

	// Cards must be of the same rank (or both worth 10 for face cards)
	card1, card2 := h.Cards[0], h.Cards[1]

	// If both are aces, special handling
	if card1.Rank == cards.Ace && card2.Rank == cards.Ace {
		if rules.SplitAcesOnceOnly {
			return true // Can split aces once regardless of other rules
		}
		return !rules.SplitAcesOnceOnly // If not limited to one split, allow more
	}

	// Same rank (A-A, K-K, etc.) or both 10-value cards (K-Q, J-10, etc.)
	return card1.Rank == card2.Rank || (card1.Value == card2.Value && card1.Value == 10)
}

// CanDoubleDown returns true if the hand can be doubled down
func (h *Hand) CanDoubleDown(rules *RuleSet) bool {
	// Typically can double down on any first two cards
	// Some games restrict to 9, 10, or 11 only
	return len(h.Cards) == 2 && !h.IsDouble
}

// Player represents a player in the game
type Player struct {
	ID         int
	Name       string
	Chips      float64
	Hands      []*Hand
	CurrentBet float64
}

// GameEngine manages the state and flow of the blackjack game
type GameEngine struct {
	State              GameState
	Rules              *RuleSet
	Shoe               *cards.Shoe
	Players            []*Player
	Dealer             *Hand
	CurrentPlayerIndex int
	CurrentHandIndex   int
}

// NewGameEngine creates a new game engine with the specified rules
func NewGameEngine(rules *RuleSet) *GameEngine {
	engine := &GameEngine{
		State:   BettingState,
		Rules:   rules,
		Shoe:    cards.NewShoe(rules.NumDecks),
		Players: make([]*Player, 0),
		Dealer:  &Hand{Cards: make([]cards.Card, 0)},
	}

	// Shuffle the shoe initially
	engine.Shoe.Shuffle()

	return engine
}

// AddPlayer adds a player to the game
func (g *GameEngine) AddPlayer(id int, name string, chips float64) {
	player := &Player{
		ID:    id,
		Name:  name,
		Chips: chips,
		Hands: make([]*Hand, 0),
	}
	g.Players = append(g.Players, player)
}

// PlaceBet places a bet for the current player
func (g *GameEngine) PlaceBet(playerID int, betAmount float64) error {
	if g.State != BettingState {
		return errors.New("cannot place bet when not in betting state")
	}

	player := g.getPlayerByID(playerID)
	if player == nil {
		return errors.New("player not found")
	}

	if betAmount <= 0 {
		return errors.New("bet amount must be positive")
	}

	if player.Chips < betAmount {
		return errors.New("insufficient chips")
	}

	player.Chips -= betAmount
	player.CurrentBet = betAmount
	return nil
}

// DealInitialCards deals two cards to each player and the dealer
func (g *GameEngine) DealInitialCards() error {
	if g.State != BettingState {
		return errors.New("cannot deal cards when not in betting state")
	}

	// Verify all players have placed bets
	for _, player := range g.Players {
		if player.CurrentBet <= 0 {
			return errors.New("all players must place bets before dealing")
		}
	}

	// Deal two cards to each player and the dealer
	for i := 0; i < 2; i++ {
		for _, player := range g.Players {
			card, err := g.Shoe.DrawCard()
			if err != nil {
				return err
			}

			// Create the initial hand for the player
			if len(player.Hands) == 0 {
				hand := &Hand{
					Cards:  []cards.Card{card},
					Bet:    player.CurrentBet,
					Active: true,
				}
				player.Hands = append(player.Hands, hand)
			} else {
				player.Hands[0].Cards = append(player.Hands[0].Cards, card)
			}
		}

		// Deal to dealer
		card, err := g.Shoe.DrawCard()
		if err != nil {
			return err
		}
		g.Dealer.Cards = append(g.Dealer.Cards, card)
	}

	// Check for blackjacks
	g.checkForBlackjacks()

	// Transition to next state
	if g.anyActivePlayers() {
		g.State = PlayerTurnState
		g.CurrentPlayerIndex = 0
		g.CurrentHandIndex = 0
	} else {
		g.State = DealerTurnState
	}

	return nil
}

// checkForBlackjacks checks if any player or the dealer has a blackjack
func (g *GameEngine) checkForBlackjacks() {
	// Check dealer first
	if g.hasBlackjack(g.Dealer) {
		g.Dealer.IsBlackjack = true
	}

	// Check each player
	for _, player := range g.Players {
		if len(player.Hands) > 0 && g.hasBlackjack(player.Hands[0]) {
			player.Hands[0].IsBlackjack = true
		}
	}
}

// hasBlackjack checks if a hand has a blackjack (ace + 10-value card)
func (g *GameEngine) hasBlackjack(hand *Hand) bool {
	if len(hand.Cards) != 2 {
		return false
	}

	card1, card2 := hand.Cards[0], hand.Cards[1]

	// Check if one card is an ace and the other is a 10-value card
	isAceFirst := card1.Rank == cards.Ace
	isAceSecond := card2.Rank == cards.Ace
	isTenValuedFirst := card1.Value == 10
	isTenValuedSecond := card2.Value == 10

	return (isAceFirst && isTenValuedSecond) || (isAceSecond && isTenValuedFirst)
}

// Hit draws a card for the current player's current hand
func (g *GameEngine) Hit(playerID int) error {
	if g.State != PlayerTurnState {
		return errors.New("cannot hit when not in player turn state")
	}

	player := g.getPlayerByID(playerID)
	if player == nil {
		return errors.New("player not found")
	}

	currentHand := g.getCurrentHand(player)
	if currentHand == nil {
		return errors.New("no active hand found")
	}

	card, err := g.Shoe.DrawCard()
	if err != nil {
		return err
	}

	currentHand.Cards = append(currentHand.Cards, card)

	// Check if the hand busted
	if currentHand.Value() > 21 {
		currentHand.IsBusted = true
		currentHand.Active = false

		// Move to next hand or next player
		if !g.moveToNextHand() {
			g.moveToNextPlayer()
		}
	} else if g.hasBlackjack(currentHand) {
		// If the player gets a blackjack after hitting, they're done with this hand
		currentHand.Active = false
		if !g.moveToNextHand() {
			g.moveToNextPlayer()
		}
	}

	return nil
}

// Stand ends the current player's turn for the current hand
func (g *GameEngine) Stand(playerID int) error {
	if g.State != PlayerTurnState {
		return errors.New("cannot stand when not in player turn state")
	}

	player := g.getPlayerByID(playerID)
	if player == nil {
		return errors.New("player not found")
	}

	currentHand := g.getCurrentHand(player)
	if currentHand == nil {
		return errors.New("no active hand found")
	}

	currentHand.Active = false

	// Move to next hand or next player
	if !g.moveToNextHand() {
		g.moveToNextPlayer()
	}

	return nil
}

// DoubleDown doubles the bet and gives one more card
func (g *GameEngine) DoubleDown(playerID int) error {
	if g.State != PlayerTurnState {
		return errors.New("cannot double down when not in player turn state")
	}

	player := g.getPlayerByID(playerID)
	if player == nil {
		return errors.New("player not found")
	}

	currentHand := g.getCurrentHand(player)
	if currentHand == nil {
		return errors.New("no active hand found")
	}

	if !currentHand.CanDoubleDown(g.Rules) {
		return errors.New("cannot double down on this hand")
	}

	// Double the bet
	player.Chips -= currentHand.Bet
	currentHand.Bet *= 2
	currentHand.IsDouble = true

	// Give one more card
	card, err := g.Shoe.DrawCard()
	if err != nil {
		return err
	}

	currentHand.Cards = append(currentHand.Cards, card)

	// Check if the hand busted
	if currentHand.Value() > 21 {
		currentHand.IsBusted = true
	}

	currentHand.Active = false

	// Move to next hand or next player
	if !g.moveToNextHand() {
		g.moveToNextPlayer()
	}

	return nil
}

// Split splits the current hand into two hands
func (g *GameEngine) Split(playerID int) error {
	if g.State != PlayerTurnState {
		return errors.New("cannot split when not in player turn state")
	}

	player := g.getPlayerByID(playerID)
	if player == nil {
		return errors.New("player not found")
	}

	currentHand := g.getCurrentHand(player)
	if currentHand == nil {
		return errors.New("no active hand found")
	}

	if !currentHand.CanSplit(g.Rules) {
		return errors.New("cannot split this hand")
	}

	// Check if player has enough chips to split
	if player.Chips < currentHand.Bet {
		return errors.New("insufficient chips to split")
	}

	// Deduct the additional bet amount
	player.Chips -= currentHand.Bet

	// Split the hand
	cardToSplit := currentHand.Cards[1]
	currentHand.Cards = currentHand.Cards[:1] // Remove the second card

	newHand := &Hand{
		Cards:   []cards.Card{cardToSplit},
		Bet:     currentHand.Bet,
		Active:  true,
		IsSplit: true,
	}

	player.Hands = append(player.Hands, newHand)

	// Draw a card for each hand
	card1, err := g.Shoe.DrawCard()
	if err != nil {
		return err
	}
	currentHand.Cards = append(currentHand.Cards, card1)

	card2, err := g.Shoe.DrawCard()
	if err != nil {
		return err
	}
	newHand.Cards = append(newHand.Cards, card2)

	// Check for blackjacks after split
	if g.hasBlackjack(currentHand) {
		currentHand.IsBlackjack = true
		currentHand.Active = false
	}
	if g.hasBlackjack(newHand) {
		newHand.IsBlackjack = true
		newHand.Active = false
	}

	// If splitting aces, both hands get one card only
	if currentHand.Cards[0].Rank == cards.Ace && g.Rules.SplitAcesOnceOnly {
		currentHand.Active = false
		newHand.Active = false
	}

	// Move to the next hand
	g.CurrentHandIndex++

	return nil
}

// Insurance handles an insurance bet
func (g *GameEngine) Insurance(playerID int, amount float64) error {
	if !g.Rules.InsuranceAllowed {
		return errors.New("insurance not allowed with current rules")
	}

	// Insurance is only offered when dealer's upcard is an Ace
	dealerUpcard := g.Dealer.Cards[0]
	if dealerUpcard.Rank != cards.Ace {
		return errors.New("insurance only available when dealer shows an Ace")
	}

	player := g.getPlayerByID(playerID)
	if player == nil {
		return errors.New("player not found")
	}

	if player.Chips < amount {
		return errors.New("insufficient chips for insurance")
	}

	// Process insurance bet (stored separately from main bet)
	// For now, we just acknowledge the bet was made
	// Actual resolution happens after dealer reveals hole card

	return nil
}

// LateSurrender allows a player to surrender after seeing dealer's upcard
func (g *GameEngine) LateSurrender(playerID int) error {
	if !g.Rules.LateSurrender {
		return errors.New("late surrender not allowed with current rules")
	}

	// Check if it's a valid time for surrender
	// (after initial deal but before taking any action)
	player := g.getPlayerByID(playerID)
	if player == nil {
		return errors.New("player not found")
	}

	currentHand := g.getCurrentHand(player)
	if currentHand == nil {
		return errors.New("no active hand found")
	}

	// Surrender: player loses half their bet
	player.Chips += currentHand.Bet / 2 // Return half the bet
	currentHand.Active = false
	currentHand.Bet = 0 // Mark as surrendered

	// Move to next hand or next player
	if !g.moveToNextHand() {
		g.moveToNextPlayer()
	}

	return nil
}

// StartDealerTurn begins the dealer's turn
func (g *GameEngine) StartDealerTurn() error {
	if g.State != PlayerTurnState {
		return errors.New("cannot start dealer turn when not in player turn state")
	}

	// Check if there are any non-busted players
	if g.anyActivePlayers() {
		g.State = DealerTurnState
		g.playDealerTurn()
	}

	return nil
}

// playDealerTurn implements the dealer's fixed strategy
func (g *GameEngine) playDealerTurn() {
	// Dealer reveals hole card
	// The dealer's hole card is already in the hand, so no action needed here

	// Dealer plays according to fixed rules
	for {
		value := g.Dealer.Value()

		// Dealer hits on soft 17 if configured, otherwise stands on hard 17+
		if value < 17 || (value == 17 && g.Rules.DealerHitsSoft17 && g.Dealer.IsSoft()) {
			card, err := g.Shoe.DrawCard()
			if err != nil {
				break // No more cards
			}
			g.Dealer.Cards = append(g.Dealer.Cards, card)

			if g.Dealer.Value() > 21 {
				g.Dealer.IsBusted = true
				break
			}
		} else {
			break // Dealer stands
		}
	}

	g.State = EndRoundState
}

// ResolveRound resolves all player hands against the dealer
func (g *GameEngine) ResolveRound() error {
	if g.State != EndRoundState {
		return errors.New("round not ready to be resolved")
	}

	for _, player := range g.Players {
		for _, hand := range player.Hands {
			result := g.calculateHandResult(hand)

			switch result {
			case "win":
				player.Chips += hand.Bet * 2 // Original bet back plus winnings (equal to bet amount)
			case "lose":
				// Player loses their bet (already deducted in PlaceBet)
			case "push":
				player.Chips += hand.Bet // Return original bet
			case "blackjack":
				payout := math.Round(hand.Bet*g.Rules.BlackjackPayout*100) / 100 // Round to 2 decimal places
				player.Chips += hand.Bet + payout                                // Original bet back plus payout
			}
		}

		// Reset player hands for next round
		player.Hands = make([]*Hand, 0)
		player.CurrentBet = 0
	}

	// Reset dealer hand
	g.Dealer = &Hand{Cards: make([]cards.Card, 0)}

	g.State = BettingState

	return nil
}

// calculateHandResult determines the outcome of a player hand vs dealer
func (g *GameEngine) calculateHandResult(hand *Hand) string {
	playerValue := hand.Value()
	dealerValue := g.Dealer.Value()

	// If player busted, they lose regardless of dealer
	if hand.IsBusted {
		return "lose"
	}

	// If dealer busted and player didn't, player wins
	if g.Dealer.IsBusted {
		return "win"
	}

	// Natural blackjack pays differently
	if hand.IsBlackjack {
		// If dealer also has blackjack, it's a push
		if g.Dealer.IsBlackjack {
			return "push"
		}
		return "blackjack"
	}

	// Regular comparison
	if playerValue > dealerValue {
		return "win"
	} else if playerValue < dealerValue {
		return "lose"
	} else {
		return "push" // Tie
	}
}

// moveToNextHand moves to the next active hand for the current player
// Returns true if moved to next hand, false if no more hands
func (g *GameEngine) moveToNextHand() bool {
	player := g.Players[g.CurrentPlayerIndex]

	// Find next active hand
	for i := g.CurrentHandIndex + 1; i < len(player.Hands); i++ {
		if player.Hands[i].Active {
			g.CurrentHandIndex = i
			return true
		}
	}

	return false
}

// moveToNextPlayer moves to the next player with active hands
func (g *GameEngine) moveToNextPlayer() {
	// Find next player with active hands
	for i := g.CurrentPlayerIndex + 1; i < len(g.Players); i++ {
		player := g.Players[i]
		for _, hand := range player.Hands {
			if hand.Active {
				g.CurrentPlayerIndex = i
				g.CurrentHandIndex = 0
				// Find the first active hand for this player
				for j, h := range player.Hands {
					if h.Active {
						g.CurrentHandIndex = j
						return
					}
				}
				break
			}
		}
	}

	// If no more players with active hands, move to dealer turn
	if !g.anyActivePlayers() {
		g.StartDealerTurn()
	}
}

// anyActivePlayers checks if any player has any active hands
func (g *GameEngine) anyActivePlayers() bool {
	for _, player := range g.Players {
		for _, hand := range player.Hands {
			if hand.Active {
				return true
			}
		}
	}
	return false
}

// getCurrentHand returns the current active hand for the specified player
func (g *GameEngine) getCurrentHand(player *Player) *Hand {
	if g.CurrentHandIndex < len(player.Hands) {
		return player.Hands[g.CurrentHandIndex]
	}
	return nil
}

// getPlayerByID returns the player with the specified ID
func (g *GameEngine) getPlayerByID(id int) *Player {
	for _, player := range g.Players {
		if player.ID == id {
			return player
		}
	}
	return nil
}

// GetAvailableActions returns the list of valid actions for the current player/hand
func (g *GameEngine) GetAvailableActions(playerID int) ([]string, error) {
	player := g.getPlayerByID(playerID)
	if player == nil {
		return nil, errors.New("player not found")
	}

	currentHand := g.getCurrentHand(player)
	if currentHand == nil {
		return nil, errors.New("no active hand found")
	}

	actions := []string{"hit", "stand"}

	if currentHand.CanDoubleDown(g.Rules) {
		actions = append(actions, "double")
	}

	if currentHand.CanSplit(g.Rules) {
		actions = append(actions, "split")
	}

	// Check for surrender
	if g.Rules.LateSurrender && len(currentHand.Cards) == 2 {
		actions = append(actions, "surrender")
	}

	// Check for insurance (only when dealer shows Ace)
	dealerUpcard := g.Dealer.Cards[0]
	if g.Rules.InsuranceAllowed && dealerUpcard.Rank == cards.Ace && len(currentHand.Cards) == 2 {
		actions = append(actions, "insurance")
	}

	return actions, nil
}
