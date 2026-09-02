package main

import (
	"blackjack/pkg/cards"
	"blackjack/pkg/engine"
	"blackjack/pkg/oracle"
	"blackjack/pkg/rules"
	"blackjack/pkg/simulation"
	"blackjack/pkg/ui"
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "sim" {
		runSimulation()
		return
	}

	runInteractiveLoop()
}

func runSimulation() {
	fmt.Println("=== 🔮 Blackjack Monte Carlo Oracle Simulator ===")

	// Create simulation configuration
	config := simulation.SimConfig{
		TotalRounds:     100_000,
		Workers:         8,
		Rules:           rules.VegasStrip(),
		CountingSystem:  oracle.HiLo,
		BaseBet:         10.0,
		UseCardCounting: true,
	}

	fmt.Printf("Rules: %s\n", config.Rules.Name)
	fmt.Printf("Hands: %d\n", config.TotalRounds)
	fmt.Printf("Counting System: %s\n", config.CountingSystem)
	fmt.Printf("Card Counting Active: %v\n", config.UseCardCounting)
	fmt.Println("Starting simulation...")

	sim := simulation.NewSimulator(config)

	startTime := time.Now()
	stats, err := sim.Run(context.Background(), nil)
	if err != nil {
		fmt.Printf("Simulation failed: %v\n", err)
		return
	}
	elapsed := time.Since(startTime)

	fmt.Printf("=====================================\n")
	fmt.Printf("Simulation completed in %v\n", elapsed)
	fmt.Printf("Speed: %.0f hands/sec\n", float64(stats.RoundsPlayed)/elapsed.Seconds())
	fmt.Printf("Hands Played: %d\n", stats.TotalHandsPlayed)
	fmt.Printf("Hands Won: %d (%.2f%%)\n", stats.PlayerWins, float64(stats.PlayerWins)/float64(stats.TotalHandsPlayed)*100)
	fmt.Printf("Hands Lost: %d (%.2f%%)\n", stats.DealerWins, float64(stats.DealerWins)/float64(stats.TotalHandsPlayed)*100)
	fmt.Printf("Hands Pushed: %d (%.2f%%)\n", stats.Pushes, float64(stats.Pushes)/float64(stats.TotalHandsPlayed)*100)
	fmt.Printf("Total Amount Wagered: $%.2f\n", stats.TotalAmountWagered)
	fmt.Printf("Net Profit/Loss: $%.2f\n", stats.TotalNetProfit)
	fmt.Printf("Player Edge/EV: %.4f%%\n", stats.ExpectedValue*100)
	fmt.Printf("=====================================\n")
}

func runInteractiveLoop() {
	scanner := bufio.NewScanner(os.Stdin)
	renderer := ui.NewCardRenderer(true)

	ruleSet := rules.VegasStrip()
	counter := oracle.NewCardCounter(oracle.HiLo, ruleSet.Decks)
	advisor := oracle.NewStrategyAdvisor(ruleSet)

	shoe := cards.NewShoe(
		ruleSet.Decks,
		cards.WithCutCardPenetration(ruleSet.DeckPenetration),
		cards.WithDealListener(func(c cards.Card) {
			counter.ObserveCard(c)
		}),
	)
	game := engine.NewGameEngine(ruleSet, shoe)

	fmt.Printf("%sWelcome to Blackjack Oracle CLI Engine%s\n", ui.Bold+ui.Cyan, ui.Reset)

	bankroll := 1000.0

	for {
		if game.State == engine.StateWaitingBet {
			if bankroll <= 0 {
				fmt.Printf("%sYou are out of money! Game Over.%s\n", ui.Bold+ui.Red, ui.Reset)
				break
			}

			if shoe.NeedsShuffle() {
				fmt.Printf("%s[Shoe reached cut card. Shuffling full %d decks...]%s\n", ui.Yellow, ruleSet.Decks, ui.Reset)
				shoe.ResetAndShuffle()
				counter.Reset()
			}

			fmt.Printf("\nBankroll: $%s%.2f%s | Base Bet: $10.0\n", ui.Green, bankroll, ui.Reset)

			fmt.Print("Press ENTER to deal next hand (or type 'q' to quit): ")
			if !scanner.Scan() {
				break
			}
			input := strings.TrimSpace(scanner.Text())
			if input == "q" {
				break
			}

			betSize := 10.0
			if counter.TrueCount() >= 2.0 {
				betSize = float64(counter.RecommendedBetMultiplier()) * 10.0
				if betSize > bankroll {
					betSize = bankroll
				}
				fmt.Printf("Oracle detects advantage! Increasing bet to $%.2f\n", betSize)
			}

			err := game.StartRound(betSize)
			if err != nil {
				fmt.Printf("Error starting round: %v\n", err)
				continue
			}
		}

		// Print Table State
		fmt.Println(ui.FormatTableState(renderer, game, counter, advisor, true))

		// Check resolution
		if game.State == engine.StateRoundResolved {
			profit := game.NetRoundProfit()
			bankroll += profit
			if profit > 0 {
				fmt.Printf("%s*** YOU WON $%.2f! ***%s\n", ui.Bold+ui.Green, profit, ui.Reset)
			} else if profit < 0 {
				fmt.Printf("%s*** YOU LOST $%.2f ***%s\n", ui.Bold+ui.Red, -profit, ui.Reset)
			} else {
				fmt.Printf("%s*** PUSH ***%s\n", ui.Bold+ui.Yellow, ui.Reset)
			}

			// Transition back to waiting bet
			game.State = engine.StateWaitingBet
			continue
		}

		// Handle player turn
		if game.State == engine.StateInsuranceOffered {
			fmt.Print("Insurance? (y/n): ")
			scanner.Scan()
			ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if ans == "y" {
				game.Step(engine.ActionInsuranceAccept)
			} else {
				game.Step(engine.ActionInsuranceDecline)
			}
			continue
		}

		if game.State == engine.StatePlayerTurn {
			avail := game.AvailableActions()
			fmt.Print("Actions: ")
			var keys []string
			actionMap := make(map[string]engine.PlayerAction)

			for _, a := range avail {
				key := ""
				switch a {
				case engine.ActionHit:
					key = "h"
					fmt.Print("[h]it ")
				case engine.ActionStand:
					key = "s"
					fmt.Print("[s]tand ")
				case engine.ActionDouble:
					key = "d"
					fmt.Print("[d]ouble ")
				case engine.ActionSplit:
					key = "p"
					fmt.Print("s[p]lit ")
				case engine.ActionSurrender:
					key = "x"
					fmt.Print("surrender[x] ")
				}
				if key != "" {
					keys = append(keys, key)
					actionMap[key] = a
				}
			}
			fmt.Print("\nChoose action: ")
			scanner.Scan()
			input := strings.ToLower(strings.TrimSpace(scanner.Text()))

			if action, ok := actionMap[input]; ok {
				err := game.Step(action)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				}
			} else if input == "q" {
				break
			} else {
				fmt.Println("Invalid input.")
			}
		}
	}
	fmt.Printf("\nFinal Bankroll: $%.2f\n", bankroll)
}
