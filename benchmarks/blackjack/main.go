package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"

	"blackjack/internal/deck"
	"blackjack/internal/game"
	"blackjack/internal/strategy"
	"blackjack/internal/ui"
)

// Version string.
const version = "1.0.0"

// simulate runs the Monte Carlo simulation of N rounds using basic strategy.
func simulate(rounds int, decks int) {
	rules := game.DefaultRules()
	rules.Decks = decks
	if rounds < 1 {
		rounds = 1
	}

	start := time.Now()

	engine, err := game.NewEngine(rules, 1_000_000)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init engine: %v\n", err)
		os.Exit(1)
	}

	simRounds := 0
	handsResolved := 0
	wins := 0
	losses := 0
	pushes := 0
	blackjacks := 0
	totalWagered := 0
	netProfit := 0
	const bet = 10

	for i := 0; i < rounds; i++ {
		// If bankroll is too low, refill to keep simulation going (long-run EV)
		if engine.Bankroll < bet*10 {
			engine.Bankroll += 10_000
		}

		if err := engine.StartRound(bet); err != nil {
			continue
		}

		// Play all hands with basic strategy
		for !engine.AllHandsResolved() {
			hand := engine.CurrentHand()
			if hand == nil {
				break
			}

			// If player has natural blackjack, stand
			if hand.IsBlackjack() {
				_ = engine.ApplyAction(game.ActionStand)
				continue
			}

			action := strategy.OptimalAction(hand, engine.DealerUpcard(), hand.CanDouble(), hand.CanSplit())
			if err := engine.ApplyAction(action); err != nil {
				// Fallback when primary action is not legal
				fallback := strategy.OptimalAction(hand, engine.DealerUpcard(), false, false)
				if err2 := engine.ApplyAction(fallback); err2 != nil {
					_ = engine.ApplyAction(game.ActionStand)
				}
			}
		}

		results := engine.ResolveRound()
		simRounds++
		for _, r := range results {
			handsResolved++
			totalWagered += r.Bet
			switch r.Outcome {
			case game.OutcomeWin:
				wins++
			case game.OutcomeBlackjack:
				wins++
				blackjacks++
			case game.OutcomeLoss, game.OutcomeBust:
				losses++
			case game.OutcomePush:
				pushes++
			}
			netProfit += r.Payout
		}
	}

	duration := time.Since(start)

	// Print summary
	fmt.Println()
	fmt.Println("==========================================")
	fmt.Println("    MONTE CARLO SIMULATION RESULTS")
	fmt.Println("==========================================")
	fmt.Printf("Rounds simulated:        %d\n", simRounds)
	fmt.Printf("Hands resolved:          %d (splits create extra hands)\n", handsResolved)
	fmt.Printf("Shoe:                    %d decks (S17, 3:2, DAS)\n", decks)
	fmt.Printf("Duration:                %v\n", duration)
	fmt.Println("------------------------------------------")
	if simRounds > 0 {
		winPct := float64(wins) * 100 / float64(simRounds)
		lossPct := float64(losses) * 100 / float64(simRounds)
		pushPct := float64(pushes) * 100 / float64(simRounds)
		bjPct := float64(blackjacks) * 100 / float64(simRounds)
		evPct := float64(netProfit) * 100 / float64(totalWagered)

		fmt.Printf("Wins:                    %d (%.2f%%)\n", wins, winPct)
		fmt.Printf("Losses:                  %d (%.2f%%)\n", losses, lossPct)
		fmt.Printf("Pushes:                  %d (%.2f%%)\n", pushes, pushPct)
		fmt.Printf("Blackjacks:              %d (%.2f%%)\n", blackjacks, bjPct)
		fmt.Println("------------------------------------------")
		fmt.Printf("Total wagered:           $%d\n", totalWagered)
		fmt.Printf("Net result:              %+d\n", netProfit)
		fmt.Printf("House Edge / EV:         %.3f%%\n", evPct)
	}
	fmt.Println("==========================================")
}

// play runs the interactive blackjack session.
func play(decks int, showHints bool) {
	rules := game.DefaultRules()
	rules.Decks = decks

	startingBankroll := 1000
	engine, err := game.NewEngine(rules, startingBankroll)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init engine: %v\n", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("==========================================")
	fmt.Printf("    BLACKJACK SIMULATOR v%s\n", version)
	fmt.Printf("    S17 | 3:2 BJ | DAS | %d decks\n", decks)
	fmt.Println("==========================================")
	fmt.Println("Actions: h=hit s=stand d=double l=split q=quit")
	if showHints {
		fmt.Println("Basic strategy hints: ENABLED")
	}
	fmt.Println()

	for engine.Bankroll >= engine.Rules.MinBet {
		fmt.Println("------------------------------------------")

		// Bet prompt
		bet, ok := ui.PromptBet(reader, os.Stdout, engine.Rules.MinBet, engine.Rules.MaxBet, engine.Bankroll)
		if !ok {
			break
		}

		if err := engine.StartRound(bet); err != nil {
			fmt.Println("Error starting round:", err)
			continue
		}

		// Display initial state (dealer hole card face down)
		fmt.Println()
		fmt.Println(ui.FormatStatus(engine.Bankroll+bet, engine.Hands, engine.Dealer, true))
		fmt.Println()

		// Handle immediate natural blackjack
		if engine.PlayerHasBlackjack() {
			fmt.Println("Player has NATURAL BLACKJACK!")
			if engine.DealerHasBlackjack() {
				fmt.Println("Dealer also has blackjack - PUSH")
			}
		}

		// Player turns
		for !engine.AllHandsResolved() {
			hand := engine.CurrentHand()
			if hand == nil {
				break
			}

			if engine.PlayerHasBlackjack() {
				// Stand on blackjack
				_ = engine.ApplyAction(game.ActionStand)
				break
			}

			if showHints {
				hint := strategy.OptimalAction(hand, engine.DealerUpcard(), hand.CanDouble(), hand.CanSplit())
				fmt.Println(ui.FormatHint(hint))
			}

			action := ui.PromptAction(reader, os.Stdout, hand.CanDouble(), hand.CanSplit())
			if int(action) == -1 {
				fmt.Println("\nFinal bankroll:", engine.Bankroll)
				fmt.Println("Goodbye!")
				return
			}

			if err := engine.ApplyAction(action); err != nil {
				fmt.Println("Invalid action:", err)
			} else {
				fmt.Println("You chose:", action.String())
				fmt.Println(ui.FormatStatus(engine.Bankroll+totalBets(engine), engine.Hands, engine.Dealer, true))
				fmt.Println()
			}
		}

		// Dealer plays and hands resolve
		results := engine.ResolveRound()

		fmt.Println()
		fmt.Println("Dealer reveals:", ui.FormatHand(engine.Dealer))

		for i, r := range results {
			hand := engine.Hands[i]
			label := "Hand"
			if len(results) > 1 {
				fmt.Printf("Hand %d: ", i+1)
				label = ""
			}
			_ = label
			fmt.Printf("%s -> %s\n", ui.FormatHand(hand), ui.FormatOutcome(r.Outcome, r.Payout))
		}
		fmt.Println()
		fmt.Printf("Bankroll: $%d\n", engine.Bankroll)
	}

	fmt.Println()
	fmt.Println("==========================================")
	fmt.Println("    SESSION COMPLETE")
	fmt.Println("==========================================")
	fmt.Printf("Rounds played:    %d\n", engine.RoundsPlayed)
	fmt.Printf("Wins:             %d\n", engine.Wins)
	fmt.Printf("Losses:           %d\n", engine.Losses)
	fmt.Printf("Pushes:           %d\n", engine.Pushes)
	fmt.Printf("Blackjacks:       %d\n", engine.Blackjacks)
	fmt.Printf("Total wagered:    $%d\n", engine.TotalWagered)
	fmt.Printf("Net result:       %+d\n", engine.NetProfit)
	fmt.Printf("Final bankroll:   $%d\n", engine.Bankroll)
}

// totalBets sums the current bets of all active hands (for status display).
func totalBets(e *game.Engine) int {
	total := 0
	for _, h := range e.Hands {
		total += h.Bet
	}
	return total
}

func main() {
	simMode := flag.Bool("sim", false, "Run Monte Carlo simulation mode")
	numRounds := flag.Int("n", 10000, "Number of rounds to simulate (sim mode)")
	numDecks := flag.Int("decks", 6, "Number of decks in shoe (1-8)")
	showHints := flag.Bool("hint", false, "Show basic strategy hints in play mode")
	showVersion := flag.Bool("version", false, "Print version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("blackjack v%s\n", version)
		return
	}

	if *numDecks < 1 || *numDecks > 8 {
		fmt.Fprintln(os.Stderr, "Error: -decks must be between 1 and 8")
		os.Exit(1)
	}

	// Validate shoe construction early
	if _, err := deck.NewShoe(*numDecks, 0.75); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid shoe configuration: %v\n", err)
		os.Exit(1)
	}

	if *simMode {
		simulate(*numRounds, *numDecks)
		return
	}

	play(*numDecks, *showHints)
}
