package simulation

import (
	"math/rand"
	"sync"
	"time"

	"blackjack/internal/game"
	"blackjack/internal/oracle"
	"blackjack/pkg/cards"
)

// SimulationResult holds the results of a simulation run
type SimulationResult struct {
	TotalHands       int
	PlayerWins       int
	DealerWins       int
	Pushes           int
	PlayerBlackjacks int
	NetProfit        float64
	ExpectedValue    float64
	HouseEdge        float64
	AvgBetSize       float64
	StdDeviation     float64
	WinRate          float64
}

// Simulator runs Monte Carlo simulations of blackjack games
type Simulator struct {
	RuleSet          *game.RuleSet
	Runs             int
	BaseBet          float64
	UseBasicStrategy bool
	UseCardCounting  bool
	CountSystem      oracle.CountSystem
	TrueCountCutoff  float64 // Minimum true count to deviate from basic strategy
}

// NewSimulator creates a new simulator with the specified parameters
func NewSimulator(ruleSet *game.RuleSet, runs int, baseBet float64) *Simulator {
	return &Simulator{
		RuleSet:          ruleSet,
		Runs:             runs,
		BaseBet:          baseBet,
		UseBasicStrategy: true,
		UseCardCounting:  false,
		CountSystem:      oracle.HiLoCount,
		TrueCountCutoff:  2.0,
	}
}

// EnableCardCounting enables card counting in the simulation
func (s *Simulator) EnableCardCounting(countSystem oracle.CountSystem, cutoff float64) {
	s.UseCardCounting = true
	s.CountSystem = countSystem
	s.TrueCountCutoff = cutoff
}

// DisableBasicStrategy disables basic strategy (plays randomly)
func (s *Simulator) DisableBasicStrategy() {
	s.UseBasicStrategy = false
}

// Outcome is the fully resolved result of a single simulated hand. Carrying the
// resolution (rather than a bare *game.Hand) is what makes EV/house-edge
// measurement possible: the dealer comparison has already been applied.
type Outcome struct {
	Result      string // "win", "lose", "push", "blackjack"
	AmountBet   float64
	NetProfit   float64
	IsBlackjack bool
}

// Run executes the simulation with the specified number of runs
func (s *Simulator) Run() *SimulationResult {
	// For better performance with large numbers of runs, we'll use goroutines
	// Split the total runs across multiple workers
	numWorkers := 4
	runsPerWorker := s.Runs / numWorkers

	var wg sync.WaitGroup
	resultsChan := make(chan *SimulationResult, numWorkers)

	startTime := time.Now()

	// Launch worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			workerResult := s.runWorker(runsPerWorker)
			resultsChan <- workerResult
		}(i)
	}

	// Close the results channel when all workers are done
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results from all workers
	finalResult := &SimulationResult{}
	for workerResult := range resultsChan {
		finalResult.TotalHands += workerResult.TotalHands
		finalResult.PlayerWins += workerResult.PlayerWins
		finalResult.DealerWins += workerResult.DealerWins
		finalResult.Pushes += workerResult.Pushes
		finalResult.PlayerBlackjacks += workerResult.PlayerBlackjacks
		finalResult.NetProfit += workerResult.NetProfit
		finalResult.AvgBetSize += workerResult.AvgBetSize * float64(workerResult.TotalHands) // Weighted average
	}

	// Calculate weighted average bet size
	if finalResult.TotalHands > 0 {
		finalResult.AvgBetSize /= float64(finalResult.TotalHands)
	}

	// Calculate derived statistics
	finalResult.WinRate = float64(finalResult.PlayerWins) / float64(finalResult.TotalHands)
	finalResult.ExpectedValue = finalResult.NetProfit / float64(finalResult.TotalHands)
	finalResult.HouseEdge = -finalResult.ExpectedValue / finalResult.AvgBetSize

	duration := time.Since(startTime)
	_ = duration // For potential logging if needed

	return finalResult
}

// runWorker performs a portion of the total simulation runs
func (s *Simulator) runWorker(runs int) *SimulationResult {
	result := &SimulationResult{}

	// Initialize a new random seed for this worker to avoid correlation
	rand.Seed(time.Now().UnixNano() + int64(rand.Intn(10000)))

	for i := 0; i < runs; i++ {
		handOutcome := s.simulateSingleGame()
		s.updateResult(result, handOutcome)
	}

	return result
}

// simulateSingleGame simulates a single game of blackjack and returns both hands
func (s *Simulator) simulateSingleGame() HandOutcome {
	// Create a simplified simulation that doesn't rely on the internal methods of the game engine
	// We'll simulate the game directly based on the rules

	// Create a new shoe
	shoe := cards.NewShoe(s.RuleSet.NumDecks)
	shoe.Shuffle()

	// Deal initial cards
	playerHand := &game.Hand{
		Cards:  make([]cards.Card, 2),
		Bet:    s.BaseBet,
		Active: true,
	}
	dealerHand := &game.Hand{
		Cards: make([]cards.Card, 2),
	}

	playerHand.Cards[0], _ = shoe.DrawCard()
	dealerHand.Cards[0], _ = shoe.DrawCard()
	playerHand.Cards[1], _ = shoe.DrawCard()
	dealerHand.Cards[1], _ = shoe.DrawCard()

	// Check for blackjacks
	playerHasBlackjack := s.isBlackjackHand(playerHand)
	dealerHasBlackjack := s.isBlackjackHand(dealerHand)

	if playerHasBlackjack || dealerHasBlackjack {
		// If both have blackjack, it's a push
		// If only player has blackjack, player wins
		// If only dealer has blackjack, dealer wins
		playerHand.IsBlackjack = playerHasBlackjack
		dealerHand.IsBlackjack = dealerHasBlackjack
		return HandOutcome{PlayerHand: playerHand, DealerHand: dealerHand}
	}

	// Initialize card counter if needed
	var counter *oracle.CardCounter
	var advisor *oracle.StrategyAdvisor

	if s.UseCardCounting {
		counter = oracle.NewCardCounter(s.CountSystem, s.RuleSet.NumDecks)
		advisor = oracle.NewStrategyAdvisor(s.RuleSet, counter)

		// Count the initial cards dealt
		for _, c := range playerHand.Cards {
			counter.UpdateCount(c)
		}
		for _, c := range dealerHand.Cards {
			counter.UpdateCount(c)
		}
	}

	// Player's turn
	if s.UseBasicStrategy {
		playerHand = s.playPlayerTurn(playerHand, dealerHand.Cards[0], shoe, advisor, counter)
	} else {
		// Random strategy
		playerHand = s.playRandomPlayerTurn(playerHand, dealerHand.Cards[0], shoe)
	}

	// Dealer's turn - implement dealer strategy directly
	dealerHand = s.playDealerTurn(dealerHand, shoe, s.RuleSet)

	// Update the hands with the final results
	if s.getHandValue(playerHand.Cards) > 21 {
		playerHand.IsBusted = true
	}

	// Return both hands for outcome determination
	return HandOutcome{PlayerHand: playerHand, DealerHand: dealerHand}
}

// playPlayerTurn simulates the player's turn using strategy
func (s *Simulator) playPlayerTurn(playerHand *game.Hand, dealerUpcard cards.Card, shoe *cards.Shoe, advisor *oracle.StrategyAdvisor, counter *oracle.CardCounter) *game.Hand {
	// Continue player's turn until they stand or bust
	for playerHand.Active && s.getHandValue(playerHand.Cards) <= 21 && !playerHand.IsBusted && !playerHand.IsBlackjack {
		var action oracle.Action
		if advisor != nil {
			if s.UseCardCounting {
				action = advisor.GetAdvancedAction(playerHand.Cards, dealerUpcard, playerHand.Bet)
			} else {
				action = advisor.GetRecommendedAction(playerHand.Cards, dealerUpcard, playerHand.Bet)
			}
		} else {
			// Fallback to basic strategy
			action = s.getBasicStrategyAction(playerHand.Cards, dealerUpcard)
		}

		switch action {
		case oracle.HitAction:
			card, err := shoe.DrawCard()
			if err != nil {
				break // No more cards
			}
			playerHand.Cards = append(playerHand.Cards, card)

			// Update counter if applicable
			if counter != nil {
				counter.UpdateCount(card)
			}

			// Check if the hand busted
			if s.getHandValue(playerHand.Cards) > 21 {
				playerHand.IsBusted = true
				playerHand.Active = false
			}
		case oracle.StandAction:
			playerHand.Active = false
		case oracle.DoubleAction:
			// For simulation, just take one more card and stop
			card, err := shoe.DrawCard()
			if err != nil {
				break // No more cards
			}
			playerHand.Cards = append(playerHand.Cards, card)

			// Update counter if applicable
			if counter != nil {
				counter.UpdateCount(card)
			}

			if s.getHandValue(playerHand.Cards) > 21 {
				playerHand.IsBusted = true
			}
			playerHand.Active = false
		case oracle.SplitAction:
			// For simulation purposes, we'll just continue with the current hand
			playerHand.Active = false
		default:
			// Default to hit
			card, err := shoe.DrawCard()
			if err != nil {
				break // No more cards
			}
			playerHand.Cards = append(playerHand.Cards, card)

			// Update counter if applicable
			if counter != nil {
				counter.UpdateCount(card)
			}

			if s.getHandValue(playerHand.Cards) > 21 {
				playerHand.IsBusted = true
				playerHand.Active = false
			}
		}
	}

	return playerHand
}

// playRandomPlayerTurn simulates the player's turn with random actions
func (s *Simulator) playRandomPlayerTurn(playerHand *game.Hand, dealerUpcard cards.Card, shoe *cards.Shoe) *game.Hand {
	// Continue player's turn until they stand or bust
	for playerHand.Active && s.getHandValue(playerHand.Cards) <= 21 && !playerHand.IsBusted {
		// Randomly select an action
		actions := []oracle.Action{oracle.HitAction, oracle.StandAction, oracle.DoubleAction}
		randomAction := actions[rand.Intn(len(actions))]

		switch randomAction {
		case oracle.HitAction:
			card, err := shoe.DrawCard()
			if err != nil {
				break // No more cards
			}
			playerHand.Cards = append(playerHand.Cards, card)

			// Check if the hand busted
			if s.getHandValue(playerHand.Cards) > 21 {
				playerHand.IsBusted = true
				playerHand.Active = false
			}
		case oracle.StandAction:
			playerHand.Active = false
		case oracle.DoubleAction:
			// For simulation, just take one more card and stop
			card, err := shoe.DrawCard()
			if err != nil {
				break // No more cards
			}
			playerHand.Cards = append(playerHand.Cards, card)

			if s.getHandValue(playerHand.Cards) > 21 {
				playerHand.IsBusted = true
			}
			playerHand.Active = false
		}
	}

	return playerHand
}

// playDealerTurn simulates the dealer's turn according to fixed rules
func (s *Simulator) playDealerTurn(dealerHand *game.Hand, shoe *cards.Shoe, ruleSet *game.RuleSet) *game.Hand {
	for {
		dealerValue := s.getHandValue(dealerHand.Cards)

		// Dealer hits on soft 17 if configured, otherwise stands on hard 17+
		if dealerValue < 17 || (dealerValue == 17 && ruleSet.DealerHitsSoft17 && s.isSoftHand(dealerHand.Cards)) {
			card, err := shoe.DrawCard()
			if err != nil {
				break // No more cards
			}
			dealerHand.Cards = append(dealerHand.Cards, card)

			if s.getHandValue(dealerHand.Cards) > 21 {
				dealerHand.IsBusted = true
				break
			}
		} else {
			break // Dealer stands
		}
	}

	return dealerHand
}

// getBasicStrategyAction returns the basic strategy action for a given hand
func (s *Simulator) getBasicStrategyAction(hand []cards.Card, dealerUpcard cards.Card) oracle.Action {
	playerSum := s.getHandValue(hand)
	dealerValue := s.getCardValue(dealerUpcard)

	// Check for pairs first
	if len(hand) == 2 && hand[0].Rank == hand[1].Rank {
		switch hand[0].Rank {
		case cards.Ace:
			return oracle.SplitAction
		case cards.Eight:
			return oracle.SplitAction
		case cards.Nine:
			if dealerValue == 7 || dealerValue >= 10 {
				return oracle.StandAction
			} else if dealerValue >= 2 && dealerValue <= 6 || dealerValue == 8 || dealerValue == 9 {
				return oracle.SplitAction
			}
		case cards.Six:
			if dealerValue >= 2 && dealerValue <= 6 {
				return oracle.SplitAction
			}
		case cards.Five:
			if dealerValue >= 2 && dealerValue <= 9 {
				return oracle.DoubleAction
			}
		case cards.Four:
			if dealerValue >= 5 && dealerValue <= 6 && s.RuleSet.DoubleAfterSplit {
				return oracle.SplitAction
			}
		case cards.Three, cards.Two:
			if dealerValue >= 2 && dealerValue <= 7 {
				return oracle.SplitAction
			}
		}
	}

	// Check for soft hands (hands with an ace counted as 11)
	if s.isSoftHand(hand) {
		switch playerSum {
		case 20: // A-9
			return oracle.StandAction
		case 19: // A-8
			if dealerValue == 6 {
				return oracle.DoubleAction
			}
			return oracle.StandAction
		case 18: // A-7
			if dealerValue >= 3 && dealerValue <= 6 {
				return oracle.DoubleAction
			} else if dealerValue == 2 || dealerValue == 7 || dealerValue == 8 {
				return oracle.StandAction
			}
			return oracle.HitAction
		case 17: // A-6
			if dealerValue >= 3 && dealerValue <= 6 {
				return oracle.DoubleAction
			}
			return oracle.HitAction
		case 16, 15: // A-5, A-4
			if dealerValue >= 4 && dealerValue <= 6 {
				return oracle.DoubleAction
			}
			return oracle.HitAction
		case 14, 13: // A-3, A-2
			if dealerValue == 5 || dealerValue == 6 {
				return oracle.DoubleAction
			}
			return oracle.HitAction
		}
		return oracle.HitAction
	}

	// Hard hands
	switch playerSum {
	case 20, 19, 18, 17:
		return oracle.StandAction
	case 16:
		if dealerValue >= 2 && dealerValue <= 6 {
			return oracle.StandAction
		}
		return oracle.HitAction
	case 15:
		if dealerValue >= 2 && dealerValue <= 6 {
			return oracle.StandAction
		}
		return oracle.HitAction
	case 14, 13:
		if dealerValue >= 2 && dealerValue <= 6 {
			return oracle.StandAction
		}
		return oracle.HitAction
	case 12:
		if dealerValue >= 4 && dealerValue <= 6 {
			return oracle.StandAction
		}
		return oracle.HitAction
	case 11:
		return oracle.DoubleAction
	case 10:
		if dealerValue >= 2 && dealerValue <= 9 {
			return oracle.DoubleAction
		}
		return oracle.HitAction
	case 9:
		if dealerValue >= 3 && dealerValue <= 6 {
			return oracle.DoubleAction
		}
		return oracle.HitAction
	}

	// Default to hit for low hands or if no other condition is met
	return oracle.HitAction
}

// getHandValue calculates the value of a hand, preferring the highest value <= 21
func (s *Simulator) getHandValue(hand []cards.Card) int {
	value := 0
	aces := 0

	for _, card := range hand {
		if card.Rank == cards.Ace {
			aces++
			value += 11
		} else {
			value += s.getCardValue(card)
		}
	}

	// Adjust for aces if value exceeds 21
	for aces > 0 && value > 21 {
		value -= 10 // Change ace from 11 to 1
		aces--
	}

	return value
}

// getCardValue returns the numeric value of a card
func (s *Simulator) getCardValue(card cards.Card) int {
	return card.Value
}

// isSoftHand returns true if the hand contains an ace counted as 11
func (s *Simulator) isSoftHand(hand []cards.Card) bool {
	minVal := 0
	hasAce := false

	for _, card := range hand {
		if card.Rank == cards.Ace {
			hasAce = true
			minVal += 1
		} else {
			minVal += s.getCardValue(card)
		}
	}

	return hasAce && (minVal+10 <= 21)
}

// isBlackjackHand checks if a hand is a blackjack (21 with an ace and a 10-value card)
func (s *Simulator) isBlackjackHand(hand *game.Hand) bool {
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

// HandOutcome represents the outcome of a single hand with both player and dealer hands
type HandOutcome struct {
	PlayerHand *game.Hand
	DealerHand *game.Hand
}

// updateResult updates the cumulative result with a single game result
func (s *Simulator) updateResult(result *SimulationResult, outcome HandOutcome) {
	result.TotalHands++

	playerHand := outcome.PlayerHand
	dealerHand := outcome.DealerHand

	playerValue := s.getHandValue(playerHand.Cards)
	dealerValue := s.getHandValue(dealerHand.Cards)

	playerBusted := playerHand.IsBusted || playerValue > 21
	dealerBusted := dealerHand.IsBusted || dealerValue > 21

	// Determine the outcome
	var outcomeType string
	netProfit := 0.0

	if playerHand.IsBlackjack && dealerHand.IsBlackjack {
		outcomeType = "push"
		netProfit = 0
	} else if playerHand.IsBlackjack {
		outcomeType = "blackjack"
		netProfit = playerHand.Bet * s.RuleSet.BlackjackPayout
	} else if dealerHand.IsBlackjack {
		outcomeType = "lose"
		netProfit = -playerHand.Bet
	} else if playerBusted {
		outcomeType = "lose"
		netProfit = -playerHand.Bet
	} else if dealerBusted {
		outcomeType = "win"
		netProfit = playerHand.Bet
	} else if playerValue > dealerValue {
		outcomeType = "win"
		netProfit = playerHand.Bet
	} else if playerValue < dealerValue {
		outcomeType = "lose"
		netProfit = -playerHand.Bet
	} else {
		outcomeType = "push"
		netProfit = 0
	}

	// Update statistics
	result.NetProfit += netProfit
	result.AvgBetSize += playerHand.Bet // Will be divided by TotalHands later

	if playerHand.IsBlackjack {
		result.PlayerBlackjacks++
	}

	switch outcomeType {
	case "win", "blackjack":
		result.PlayerWins++
	case "lose":
		result.DealerWins++
	case "push":
		result.Pushes++
	}
}

// RunComparison runs simulations comparing different strategies
func (s *Simulator) RunComparison() map[string]*SimulationResult {
	results := make(map[string]*SimulationResult)

	// Run with basic strategy only
	s.UseCardCounting = false
	s.UseBasicStrategy = true
	results["basic_strategy"] = s.Run()

	// Run with card counting (Hi-Lo)
	s.EnableCardCounting(oracle.HiLoCount, 2.0)
	results["hi_lo_counting"] = s.Run()

	// Run with KO counting
	s.EnableCardCounting(oracle.KOCount, 2.0)
	results["ko_counting"] = s.Run()

	// Run with Omega II counting
	s.EnableCardCounting(oracle.OmegaIICount, 2.0)
	results["omega_ii_counting"] = s.Run()

	return results
}
