package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"blackjack/internal/game"
	"blackjack/internal/oracle"
	"blackjack/internal/simulation"
	"blackjack/pkg/cards"
	"blackjack/pkg/ui"
)

func main() {
	fmt.Println("Welcome to the Professional Blackjack Casino Engine!")
	fmt.Println("===============================================")

	reader := bufio.NewReader(os.Stdin)

	// Select game mode
	fmt.Println("Select mode:")
	fmt.Println("1. Play Blackjack")
	fmt.Println("2. Card Counting Practice")
	fmt.Println("3. Strategy Training")
	fmt.Println("4. Monte Carlo Simulation")
	fmt.Print("Enter choice (1-4): ")

	modeInput, _ := reader.ReadString('\n')
	mode, err := strconv.Atoi(strings.TrimSpace(modeInput))
	if err != nil || mode < 1 || mode > 4 {
		fmt.Println("Invalid choice. Exiting.")
		return
	}

	switch mode {
	case 1:
		playBlackjack(reader)
	case 2:
		cardCountingPractice(reader)
	case 3:
		strategyTraining(reader)
	case 4:
		runMonteCarloSimulation(reader)
	}
}

func playBlackjack(reader *bufio.Reader) {
	fmt.Println("\n--- Blackjack Game ---")

	// Select rule set
	fmt.Println("Select rule set:")
	fmt.Println("1. Vegas Strip (H17, DAS, 3:2)")
	fmt.Println("2. Atlantic City (H17, No DAS, 3:2)")
	fmt.Println("3. European (S17, No DAS, 3:2)")
	fmt.Println("4. High-Low Reno (H17, No DAS, 6:5)")
	fmt.Print("Enter choice (1-4): ")

	ruleInput, _ := reader.ReadString('\n')
	ruleChoice, err := strconv.Atoi(strings.TrimSpace(ruleInput))
	if err != nil || ruleChoice < 1 || ruleChoice > 4 {
		fmt.Println("Invalid choice. Using Vegas Strip rules.")
		ruleChoice = 1
	}

	var ruleSet *game.RuleSet
	switch ruleChoice {
	case 1:
		ruleSet = &game.VegasStrip
	case 2:
		ruleSet = &game.AtlanticCity
	case 3:
		ruleSet = &game.European
	case 4:
		ruleSet = &game.HighLowReno
	default:
		ruleSet = &game.VegasStrip
	}

	fmt.Printf("Selected rule set: %s\n", ruleSet.Name)

	// Initialize game engine
	engine := game.NewGameEngine(ruleSet)

	// Add player
	fmt.Print("Enter your name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Player"
	}

	fmt.Print("Enter starting chips: ")
	chipsInput, _ := reader.ReadString('\n')
	chips, err := strconv.ParseFloat(strings.TrimSpace(chipsInput), 64)
	if err != nil || chips <= 0 {
		chips = 1000.0
		fmt.Printf("Invalid amount. Starting with %.2f chips.\n", chips)
	}

	playerID := 1
	engine.AddPlayer(playerID, name, chips)

	// Game loop
	for {
		ui.ClearScreen()

		// Betting phase
		fmt.Printf("\n--- %s's Turn ---\n", name)
		fmt.Printf("Your chips: %.2f\n", engine.Players[0].Chips)

		if engine.Players[0].Chips <= 0 {
			fmt.Println("You're out of chips! Game over.")
			break
		}

		fmt.Print("Enter bet amount: ")
		betInput, _ := reader.ReadString('\n')
		bet, err := strconv.ParseFloat(strings.TrimSpace(betInput), 64)
		if err != nil || bet <= 0 || bet > engine.Players[0].Chips {
			fmt.Printf("Invalid bet amount. Minimum: 1.0, Maximum: %.2f\n", engine.Players[0].Chips)
			continue
		}

		// Place bet and deal cards
		engine.PlaceBet(playerID, bet)
		engine.DealInitialCards()

		// Display initial hands
		playerHand := engine.Players[0].Hands[0]

		fmt.Println(ui.PrintStatus(engine.Dealer.Cards, playerHand.Cards, name, bet, false))

		// Check for blackjack
		if playerHand.IsBlackjack {
			fmt.Println("Blackjack! You win!")
			engine.ResolveRound()
			fmt.Printf("Press Enter to continue...")
			reader.ReadString('\n')
			continue
		}

		// Player's turn
		for engine.State == game.PlayerTurnState {
			// Show available actions
			actions, _ := engine.GetAvailableActions(playerID)
			fmt.Printf("\nAvailable actions: ")
			for i, action := range actions {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Printf("%s", strings.ToUpper(action))
			}
			fmt.Println()

			fmt.Print("Choose action: ")
			actionInput, _ := reader.ReadString('\n')
			action := strings.ToLower(strings.TrimSpace(actionInput))

			// Validate action
			validAction := false
			for _, valid := range actions {
				if action == valid {
					validAction = true
					break
				}
			}

			if !validAction {
				fmt.Println("Invalid action. Please try again.")
				continue
			}

			// Execute action
			switch action {
			case "hit":
				engine.Hit(playerID)
			case "stand":
				engine.Stand(playerID)
			case "double":
				err := engine.DoubleDown(playerID)
				if err != nil {
					fmt.Printf("Cannot double down: %s\n", err.Error())
					continue
				}
			case "split":
				err := engine.Split(playerID)
				if err != nil {
					fmt.Printf("Cannot split: %s\n", err.Error())
					continue
				}
			case "surrender":
				err := engine.LateSurrender(playerID)
				if err != nil {
					fmt.Printf("Cannot surrender: %s\n", err.Error())
					continue
				}
			}

			// Refresh display
			ui.ClearScreen()
			fmt.Printf("\n--- %s's Turn ---\n", name)
			fmt.Printf("Your chips: %.2f\n", engine.Players[0].Chips)
			fmt.Println(ui.PrintStatus(engine.Dealer.Cards, playerHand.Cards, name, bet, false))
		}

		// Dealer's turn
		if engine.State == game.PlayerTurnState {
			engine.StartDealerTurn()
		}

		// Show dealer's full hand
		ui.ClearScreen()
		fmt.Printf("\n--- %s's Turn ---\n", name)
		fmt.Printf("Your chips: %.2f\n", engine.Players[0].Chips)
		fmt.Println(ui.PrintStatus(engine.Dealer.Cards, playerHand.Cards, name, bet, true))

		// Resolve round
		engine.ResolveRound()

		fmt.Printf("Round completed. Press Enter to continue...")
		reader.ReadString('\n')

		// Ask if player wants to continue
		fmt.Print("Play another hand? (y/n): ")
		continueInput, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(continueInput)) != "y" {
			break
		}
	}

	fmt.Println("Thanks for playing!")
}

func cardCountingPractice(reader *bufio.Reader) {
	fmt.Println("\n--- Card Counting Practice ---")

	// Select counting system
	fmt.Println("Select counting system:")
	fmt.Println("1. Hi-Lo")
	fmt.Println("2. KO")
	fmt.Println("3. Omega II")
	fmt.Print("Enter choice (1-3): ")

	countInput, _ := reader.ReadString('\n')
	countChoice, err := strconv.Atoi(strings.TrimSpace(countInput))
	if err != nil || countChoice < 1 || countChoice > 3 {
		fmt.Println("Invalid choice. Using Hi-Lo system.")
		countChoice = 1
	}

	var countSystem oracle.CountSystem
	switch countChoice {
	case 1:
		countSystem = oracle.HiLoCount
	case 2:
		countSystem = oracle.KOCount
	case 3:
		countSystem = oracle.OmegaIICount
	default:
		countSystem = oracle.HiLoCount
	}

	fmt.Printf("Selected counting system: %s\n", countSystem)

	// Create a counter
	counter := oracle.NewCardCounter(countSystem, 6) // Assume 6-deck shoe

	// Simulate dealing cards
	fmt.Println("\nCard counting practice. Enter cards one by one (e.g., 'Ah' for Ace of Hearts):")
	fmt.Println("Format: [Rank][Suit] where Suit is h/d/c/s for hearts/diamonds/clubs/spades")
	fmt.Println("Examples: Ah, 10d, Kc, 2s")
	fmt.Println("Type 'done' to finish practice and see results.")

	cardMap := map[string]string{
		"a": "A", "2": "2", "3": "3", "4": "4", "5": "5", "6": "6", "7": "7", "8": "8", "9": "9", "10": "10", "j": "J", "q": "Q", "k": "K",
	}

	for {
		fmt.Printf("\nCurrent running count: %.2f, True count: %.2f\n", counter.RunningCount, counter.TrueCount())
		fmt.Print("Enter card (or 'done' to finish): ")
		cardInput, _ := reader.ReadString('\n')
		cardStr := strings.ToLower(strings.TrimSpace(cardInput))

		if cardStr == "done" {
			break
		}

		// Parse card input
		if len(cardStr) < 2 {
			fmt.Println("Invalid card format. Try again.")
			continue
		}

		// Extract rank and suit
		var rankStr string
		suitStr := string(cardStr[len(cardStr)-1]) // Last character is suit

		// Handle 10 specially (two characters)
		if len(cardStr) >= 3 && cardStr[:2] == "10" {
			rankStr = "10"
		} else {
			rankStr = string(cardStr[0])
		}

		// Validate suit
		if suitStr != "h" && suitStr != "d" && suitStr != "c" && suitStr != "s" {
			fmt.Println("Invalid suit. Use h/d/c/s for hearts/diamonds/clubs/spades.")
			continue
		}

		// Convert rank to proper format
		properRank, exists := cardMap[rankStr]
		if !exists {
			fmt.Println("Invalid rank. Use A,2-9,J,Q,K or 10.")
			continue
		}

		// Create a temporary card for counting purposes
		var tempCardRank cards.Rank
		switch properRank {
		case "A":
			tempCardRank = cards.Ace
		case "2":
			tempCardRank = cards.Two
		case "3":
			tempCardRank = cards.Three
		case "4":
			tempCardRank = cards.Four
		case "5":
			tempCardRank = cards.Five
		case "6":
			tempCardRank = cards.Six
		case "7":
			tempCardRank = cards.Seven
		case "8":
			tempCardRank = cards.Eight
		case "9":
			tempCardRank = cards.Nine
		case "10":
			tempCardRank = cards.Ten
		case "J":
			tempCardRank = cards.Jack
		case "Q":
			tempCardRank = cards.Queen
		case "K":
			tempCardRank = cards.King
		}

		// Create a temporary card for counting
		tempCard := cards.Card{
			Rank:  tempCardRank,
			Suit:  cards.Hearts, // Suit doesn't matter for counting
			Value: getValueForRank(tempCardRank),
		}

		// Update the counter
		counter.UpdateCount(tempCard)

		fmt.Printf("Card %s counted. Running count: %.2f, True count: %.2f\n",
			fmt.Sprintf("%s%c", properRank, map[string]rune{"h": '♥', "d": '♦', "c": '♣', "s": '♠'}[suitStr]),
			counter.RunningCount, counter.TrueCount())
	}

	fmt.Printf("\nPractice finished!\nFinal running count: %.2f\nTrue count: %.2f\n",
		counter.RunningCount, counter.TrueCount())
}

// Helper function to get the value for a rank
func getValueForRank(rank cards.Rank) int {
	switch rank {
	case cards.Ace:
		return 11
	case cards.Two:
		return 2
	case cards.Three:
		return 3
	case cards.Four:
		return 4
	case cards.Five:
		return 5
	case cards.Six:
		return 6
	case cards.Seven:
		return 7
	case cards.Eight:
		return 8
	case cards.Nine:
		return 9
	case cards.Ten, cards.Jack, cards.Queen, cards.King:
		return 10
	default:
		return 0
	}
}

func strategyTraining(reader *bufio.Reader) {
	fmt.Println("\n--- Strategy Training ---")
	fmt.Println("Learn basic strategy with real-time recommendations!")

	// Select rule set
	fmt.Println("Select rule set:")
	fmt.Println("1. Vegas Strip (H17, DAS, 3:2)")
	fmt.Println("2. Atlantic City (H17, No DAS, 3:2)")
	fmt.Println("3. European (S17, No DAS, 3:2)")
	fmt.Print("Enter choice (1-3): ")

	ruleInput, _ := reader.ReadString('\n')
	ruleChoice, err := strconv.Atoi(strings.TrimSpace(ruleInput))
	if err != nil || ruleChoice < 1 || ruleChoice > 3 {
		fmt.Println("Invalid choice. Using Vegas Strip rules.")
		ruleChoice = 1
	}

	var ruleSet *game.RuleSet
	switch ruleChoice {
	case 1:
		ruleSet = &game.VegasStrip
	case 2:
		ruleSet = &game.AtlanticCity
	case 3:
		ruleSet = &game.European
	default:
		ruleSet = &game.VegasStrip
	}

	// Create a dummy counter and advisor for training
	counter := oracle.NewCardCounter(oracle.HiLoCount, ruleSet.NumDecks)
	advisor := oracle.NewStrategyAdvisor(ruleSet, counter)

	// Training loop
	fmt.Println("\nStrategy training mode. Enter your hand and dealer's upcard:")
	fmt.Println("Format: Enter your cards separated by space (e.g., 'A 10' or '8 8')")
	fmt.Println("Then enter dealer's upcard (e.g., '10')")

	for {
		fmt.Print("\nEnter your hand (or 'quit' to exit): ")
		handInput, _ := reader.ReadString('\n')
		handStr := strings.TrimSpace(handInput)

		if strings.ToLower(handStr) == "quit" {
			break
		}

		handCards := parseCards(handStr)
		if len(handCards) < 2 {
			fmt.Println("Please enter at least 2 cards for your hand.")
			continue
		}

		fmt.Print("Enter dealer's upcard: ")
		dealerInput, _ := reader.ReadString('\n')
		dealerStr := strings.TrimSpace(dealerInput)
		dealerCard := parseSingleCard(dealerStr)

		if dealerCard.Rank == "" {
			fmt.Println("Invalid dealer card.")
			continue
		}

		// Get recommendation
		recommendation := advisor.GetRecommendedAction(handCards, dealerCard, 10.0) // Base bet of 10
		advancedRec := advisor.GetAdvancedAction(handCards, dealerCard, 10.0)

		fmt.Printf("\nYour hand: %v\n", handCards)
		fmt.Printf("Dealer's upcard: %s\n", dealerCard)
		fmt.Printf("Basic strategy recommends: %s\n", strings.ToUpper(string(recommendation)))
		if recommendation != advancedRec {
			fmt.Printf("With card counting (%.2f): %s\n", counter.TrueCount(), strings.ToUpper(string(advancedRec)))
		}

		fmt.Print("\nPress Enter to continue...")
		reader.ReadString('\n')
	}

	fmt.Println("Strategy training finished!")
}

// Helper function to parse cards from string input
func parseCards(input string) []cards.Card {
	parts := strings.Fields(input)
	var cardsList []cards.Card

	rankMap := map[string]cards.Rank{
		"A": cards.Ace, "a": cards.Ace, "2": cards.Two, "3": cards.Three, "4": cards.Four, "5": cards.Five, "6": cards.Six, "7": cards.Seven, "8": cards.Eight, "9": cards.Nine, "10": cards.Ten,
		"J": cards.Jack, "j": cards.Jack, "Q": cards.Queen, "q": cards.Queen, "K": cards.King, "k": cards.King,
	}

	for _, part := range parts {
		if rank, ok := rankMap[part]; ok {
			card := cards.Card{
				Rank:  rank,
				Suit:  cards.Hearts, // Suit doesn't matter for strategy
				Value: getValueForRank(rank),
			}
			cardsList = append(cardsList, card)
		}
	}

	return cardsList
}

// Helper function to parse a single card from string input
func parseSingleCard(input string) cards.Card {
	rankMap := map[string]cards.Rank{
		"A": cards.Ace, "a": cards.Ace, "2": cards.Two, "3": cards.Three, "4": cards.Four, "5": cards.Five, "6": cards.Six, "7": cards.Seven, "8": cards.Eight, "9": cards.Nine, "10": cards.Ten,
		"J": cards.Jack, "j": cards.Jack, "Q": cards.Queen, "q": cards.Queen, "K": cards.King, "k": cards.King,
	}

	if rank, ok := rankMap[input]; ok {
		return cards.Card{
			Rank:  rank,
			Suit:  cards.Hearts, // Suit doesn't matter for strategy
			Value: getValueForRank(rank),
		}
	}

	return cards.Card{Rank: "", Value: 0}
}

func runMonteCarloSimulation(reader *bufio.Reader) {
	fmt.Println("\n--- Monte Carlo Simulation ---")

	// Select rule set
	fmt.Println("Select rule set:")
	fmt.Println("1. Vegas Strip (H17, DAS, 3:2)")
	fmt.Println("2. Atlantic City (H17, No DAS, 3:2)")
	fmt.Println("3. European (S17, No DAS, 3:2)")
	fmt.Println("4. High-Low Reno (H17, No DAS, 6:5)")
	fmt.Print("Enter choice (1-4): ")

	ruleInput, _ := reader.ReadString('\n')
	ruleChoice, err := strconv.Atoi(strings.TrimSpace(ruleInput))
	if err != nil || ruleChoice < 1 || ruleChoice > 4 {
		fmt.Println("Invalid choice. Using Vegas Strip rules.")
		ruleChoice = 1
	}

	var ruleSet *game.RuleSet
	switch ruleChoice {
	case 1:
		ruleSet = &game.VegasStrip
	case 2:
		ruleSet = &game.AtlanticCity
	case 3:
		ruleSet = &game.European
	case 4:
		ruleSet = &game.HighLowReno
	default:
		ruleSet = &game.VegasStrip
	}

	fmt.Print("Enter number of hands to simulate (e.g., 10000): ")
	handsInput, _ := reader.ReadString('\n')
	hands, err := strconv.Atoi(strings.TrimSpace(handsInput))
	if err != nil || hands <= 0 {
		hands = 10000
		fmt.Printf("Invalid number. Simulating %d hands.\n", hands)
	}

	fmt.Print("Enter base bet amount: ")
	betInput, _ := reader.ReadString('\n')
	baseBet, err := strconv.ParseFloat(strings.TrimSpace(betInput), 64)
	if err != nil || baseBet <= 0 {
		baseBet = 10.0
		fmt.Printf("Invalid amount. Using %.2f as base bet.\n", baseBet)
	}

	// Create simulator
	simulator := simulation.NewSimulator(ruleSet, hands, baseBet)

	fmt.Println("\nRunning simulation...")
	fmt.Printf("Rule Set: %s\n", ruleSet.Name)
	fmt.Printf("Number of Hands: %d\n", hands)
	fmt.Printf("Base Bet: %.2f\n", baseBet)

	// Run simulation
	result := simulator.Run()

	// Display results
	fmt.Println("\n--- Simulation Results ---")
	fmt.Printf("Total Hands Played: %d\n", result.TotalHands)
	fmt.Printf("Player Wins: %d (%.2f%%)\n", result.PlayerWins, float64(result.PlayerWins)/float64(result.TotalHands)*100)
	fmt.Printf("Dealer Wins: %d (%.2f%%)\n", result.DealerWins, float64(result.DealerWins)/float64(result.TotalHands)*100)
	fmt.Printf("Pushes: %d (%.2f%%)\n", result.Pushes, float64(result.Pushes)/float64(result.TotalHands)*100)
	fmt.Printf("Player Blackjacks: %d\n", result.PlayerBlackjacks)
	fmt.Printf("Net Profit/Loss: %.2f\n", result.NetProfit)
	fmt.Printf("Average Bet Size: %.2f\n", result.AvgBetSize)
	fmt.Printf("Expected Value per Hand: %.4f\n", result.ExpectedValue)
	fmt.Printf("House Edge: %.4f%%\n", result.HouseEdge*100)

	// Ask if user wants to run comparison
	fmt.Print("\nRun strategy comparison? (y/n): ")
	compareInput, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(compareInput)) == "y" {
		fmt.Println("\nRunning strategy comparison...")
		comparisonResults := simulator.RunComparison()

		fmt.Println("\n--- Strategy Comparison ---")
		for strategy, res := range comparisonResults {
			fmt.Printf("\nStrategy: %s\n", strategy)
			fmt.Printf("  Win Rate: %.2f%%\n", res.WinRate*100)
			fmt.Printf("  Expected Value: %.4f\n", res.ExpectedValue)
			fmt.Printf("  House Edge: %.4f%%\n", res.HouseEdge*100)
			fmt.Printf("  Net Profit: %.2f\n", res.NetProfit)
		}
	}
}
