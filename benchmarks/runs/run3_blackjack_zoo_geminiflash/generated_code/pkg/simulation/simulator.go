package simulation

import (
	"blackjack/pkg/cards"
	"blackjack/pkg/engine"
	"blackjack/pkg/oracle"
	"blackjack/pkg/rules"
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// SimConfig configures the Monte Carlo simulation.
type SimConfig struct {
	Rules             rules.RuleSet
	TotalRounds       int64
	Workers           int
	CountingSystem    oracle.CountingSystem
	UseCardCounting   bool
	BaseBet           float64
	Seed              int64
	ProgressFrequency int64 // Update progress every N rounds (0 = disabled)
}

// SimStats aggregates the outcome statistics of the simulation.
type SimStats struct {
	RoundsPlayed       int64         `json:"rounds_played"`
	TotalHandsPlayed   int64         `json:"total_hands_played"`
	TotalAmountWagered float64       `json:"total_amount_wagered"`
	TotalNetProfit     float64       `json:"total_net_profit"`
	ExpectedValue      float64       `json:"expected_value"` // EV per unit wagered (net / wagered)
	HouseEdge          float64       `json:"house_edge"`     // House edge percentage (-EV %)
	PlayerWins         int64         `json:"player_wins"`
	DealerWins         int64         `json:"dealer_wins"`
	Pushes             int64         `json:"pushes"`
	Blackjacks         int64         `json:"blackjacks"`
	Busts              int64         `json:"busts"`
	Surrenders         int64         `json:"surrenders"`
	DoublesWon         int64         `json:"doubles_won"`
	DoublesLost        int64         `json:"doubles_lost"`
	SplitsPlayed       int64         `json:"splits_played"`
	InsurancesWon      int64         `json:"insurances_won"`
	InsurancesLost     int64         `json:"insurances_lost"`
	Duration           time.Duration `json:"duration"`
	RoundsPerSecond    float64       `json:"rounds_per_second"`
}

// ProgressCallback is invoked periodically during simulation runs.
type ProgressCallback func(completedRounds int64, totalRounds int64, currentEV float64)

// Simulator manages concurrent Monte Carlo simulations.
type Simulator struct {
	config SimConfig
}

// NewSimulator creates a new Monte Carlo simulator instance.
func NewSimulator(config SimConfig) *Simulator {
	if config.Workers <= 0 {
		config.Workers = runtime.NumCPU()
	}
	if config.BaseBet <= 0 {
		config.BaseBet = 10.0
	}
	if config.Seed == 0 {
		config.Seed = time.Now().UnixNano()
	}
	return &Simulator{config: config}
}

// workerResult contains stats accumulated by a single worker goroutine.
type workerResult struct {
	roundsPlayed   int64
	handsPlayed    int64
	totalWagered   float64
	netProfit      float64
	playerWins     int64
	dealerWins     int64
	pushes         int64
	blackjacks     int64
	busts          int64
	surrenders     int64
	doublesWon     int64
	doublesLost    int64
	splitsPlayed   int64
	insurancesWon  int64
	insurancesLost int64
}

// Run executes the Monte Carlo simulation concurrently across worker goroutines.
func (s *Simulator) Run(ctx context.Context, progress ProgressCallback) (*SimStats, error) {
	if err := s.config.Rules.Validate(); err != nil {
		return nil, fmt.Errorf("invalid rules: %w", err)
	}

	startTime := time.Now()
	var globalCompleted int64

	roundsPerWorker := s.config.TotalRounds / int64(s.config.Workers)
	remainderRounds := s.config.TotalRounds % int64(s.config.Workers)

	resultsChan := make(chan workerResult, s.config.Workers)
	var wg sync.WaitGroup

	for w := 0; w < s.config.Workers; w++ {
		workerRounds := roundsPerWorker
		if w == 0 {
			workerRounds += remainderRounds
		}

		workerSeed := s.config.Seed + int64(w)*7919 + 17
		wg.Add(1)
		go func(rounds int64, seed int64) {
			defer wg.Done()
			res := s.runWorker(ctx, rounds, seed, &globalCompleted, progress)
			resultsChan <- res
		}(workerRounds, workerSeed)
	}

	wg.Wait()
	close(resultsChan)

	// Aggregate all worker results
	stats := &SimStats{}
	for r := range resultsChan {
		stats.RoundsPlayed += r.roundsPlayed
		stats.TotalHandsPlayed += r.handsPlayed
		stats.TotalAmountWagered += r.totalWagered
		stats.TotalNetProfit += r.netProfit
		stats.PlayerWins += r.playerWins
		stats.DealerWins += r.dealerWins
		stats.Pushes += r.pushes
		stats.Blackjacks += r.blackjacks
		stats.Busts += r.busts
		stats.Surrenders += r.surrenders
		stats.DoublesWon += r.doublesWon
		stats.DoublesLost += r.doublesLost
		stats.SplitsPlayed += r.splitsPlayed
		stats.InsurancesWon += r.insurancesWon
		stats.InsurancesLost += r.insurancesLost
	}

	stats.Duration = time.Since(startTime)
	if stats.Duration > 0 {
		stats.RoundsPerSecond = float64(stats.RoundsPlayed) / stats.Duration.Seconds()
	}

	if stats.TotalAmountWagered > 0 {
		stats.ExpectedValue = stats.TotalNetProfit / stats.TotalAmountWagered
		stats.HouseEdge = -stats.ExpectedValue * 100.0 // House edge is negative player EV %
	}

	return stats, nil
}

// runWorker executes a slice of rounds in an isolated goroutine without locks.
func (s *Simulator) runWorker(
	ctx context.Context,
	rounds int64,
	seed int64,
	globalCompleted *int64,
	progress ProgressCallback,
) workerResult {
	res := workerResult{}
	rng := rand.New(rand.NewSource(seed))

	counter := oracle.NewCardCounter(s.config.CountingSystem, s.config.Rules.Decks)

	shoe := cards.NewShoe(
		s.config.Rules.Decks,
		cards.WithCutCardPenetration(s.config.Rules.DeckPenetration),
		cards.WithCustomRNG(rng),
		cards.WithDealListener(func(c cards.Card) {
			counter.ObserveCard(c)
		}),
	)

	advisor := oracle.NewStrategyAdvisor(s.config.Rules)

	for i := int64(0); i < rounds; i++ {
		select {
		case <-ctx.Done():
			return res
		default:
		}

		if shoe.NeedsShuffle() {
			shoe.ResetAndShuffle()
			counter.Reset()
		}

		// Calculate bet size
		bet := s.config.BaseBet
		tc := counter.TrueCount()
		if s.config.UseCardCounting {
			mult := counter.RecommendedBetMultiplier()
			bet = s.config.BaseBet * float64(mult)
		}

		game := engine.NewGameEngine(s.config.Rules, shoe)

		err := game.StartRound(bet)
		if err != nil {
			continue
		}

		// Handle Insurance offer
		if game.State == engine.StateInsuranceOffered {
			accept, _ := advisor.AdviseInsurance(tc)
			_ = game.Insurance(accept)
		}

		// Player decision loop
		for game.State == engine.StatePlayerTurn {
			hand := game.ActiveHand()
			if hand == nil {
				break
			}

			dealerUpcard := game.DealerUpcard()
			rec := advisor.Advise(hand, dealerUpcard, tc)

			// Try to execute recommended action, falling back to basic alternatives if not permitted
			var actErr error
			switch rec.Action {
			case engine.ActionSurrender:
				actErr = game.Step(engine.ActionSurrender)
				if actErr != nil {
					actErr = game.Step(engine.ActionHit)
				}
			case engine.ActionSplit:
				actErr = game.Step(engine.ActionSplit)
				if actErr != nil {
					if hand.Total() >= 17 {
						actErr = game.Step(engine.ActionStand)
					} else {
						actErr = game.Step(engine.ActionHit)
					}
				}
			case engine.ActionDouble:
				actErr = game.Step(engine.ActionDouble)
				if actErr != nil {
					actErr = game.Step(engine.ActionHit)
				}
			case engine.ActionHit:
				actErr = game.Step(engine.ActionHit)
			case engine.ActionStand:
				actErr = game.Step(engine.ActionStand)
			default:
				actErr = game.Step(engine.ActionStand)
			}

			if actErr != nil {
				_ = game.Step(engine.ActionStand)
			}
		}

		// Tally round statistics
		res.roundsPlayed++
		res.netProfit += game.NetRoundProfit()

		for _, h := range game.PlayerHands {
			res.handsPlayed++
			res.totalWagered += h.Bet

			if h.Status == engine.StatusBusted {
				res.busts++
			}
			if h.Status == engine.StatusSurrendered {
				res.surrenders++
			}
			if h.IsSplitHand {
				res.splitsPlayed++
			}
			if h.Doubled {
				if h.NetProfit > 0 {
					res.doublesWon++
				} else if h.NetProfit < 0 {
					res.doublesLost++
				}
			}

			if h.NetProfit > 0 {
				res.playerWins++
				if h.Status == engine.StatusBlackjack {
					res.blackjacks++
				}
			} else if h.NetProfit < 0 {
				res.dealerWins++
			} else {
				res.pushes++
			}

			if h.InsuranceBet > 0 {
				res.totalWagered += h.InsuranceBet
				if h.InsuranceWon {
					res.insurancesWon++
				} else {
					res.insurancesLost++
				}
			}
		}

		// Progress update
		if progress != nil && s.config.ProgressFrequency > 0 && i%s.config.ProgressFrequency == 0 {
			completed := atomic.AddInt64(globalCompleted, s.config.ProgressFrequency)
			currentEV := 0.0
			if res.totalWagered > 0 {
				currentEV = res.netProfit / res.totalWagered
			}
			progress(completed, s.config.TotalRounds, currentEV)
		}
	}

	return res
}
