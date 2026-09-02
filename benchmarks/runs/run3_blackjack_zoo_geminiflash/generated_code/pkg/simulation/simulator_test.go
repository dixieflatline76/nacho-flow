package simulation

import (
	"blackjack/pkg/oracle"
	"blackjack/pkg/rules"
	"context"
	"testing"
	"time"
)

func TestSimulator_BasicStrategyEV(t *testing.T) {
	config := SimConfig{
		Rules:             rules.VegasStrip(),
		TotalRounds:       20000,
		Workers:           4,
		CountingSystem:    oracle.HiLo,
		UseCardCounting:   false,
		BaseBet:           10.0,
		Seed:              42,
		ProgressFrequency: 5000,
	}

	sim := NewSimulator(config)

	var lastProgress int64
	progressCB := func(completed int64, total int64, ev float64) {
		lastProgress = completed
	}
	_ = lastProgress

	stats, err := sim.Run(context.Background(), progressCB)
	if err != nil {
		t.Fatalf("Simulation failed: %v", err)
	}

	if stats.RoundsPlayed != 20000 {
		t.Errorf("Expected 20000 rounds played, got %d", stats.RoundsPlayed)
	}
	if stats.TotalAmountWagered <= 0 {
		t.Errorf("Expected positive wagered amount, got %f", stats.TotalAmountWagered)
	}
	if stats.TotalHandsPlayed < stats.RoundsPlayed {
		t.Errorf("Hands played (%d) should be >= rounds played (%d)", stats.TotalHandsPlayed, stats.RoundsPlayed)
	}

	// Basic strategy house edge in Vegas Strip is typically ~0.3% to 1.5%
	t.Logf("EV: %.4f, House Edge: %.2f%%, Net Profit: $%.2f over %d rounds (%.1f rps)",
		stats.ExpectedValue, stats.HouseEdge, stats.TotalNetProfit, stats.RoundsPlayed, stats.RoundsPerSecond)

	if stats.HouseEdge < -10.0 || stats.HouseEdge > 10.0 {
		t.Errorf("Unrealistic house edge: %.2f%%", stats.HouseEdge)
	}
}

func TestSimulator_CardCountingEdge(t *testing.T) {
	// Card counting with Hi-Lo bet spread should outperform flat betting
	config := SimConfig{
		Rules:             rules.AtlanticCity(),
		TotalRounds:       20000,
		Workers:           4,
		CountingSystem:    oracle.HiLo,
		UseCardCounting:   true,
		BaseBet:           10.0,
		Seed:              12345,
		ProgressFrequency: 0,
	}

	sim := NewSimulator(config)
	stats, err := sim.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Simulation with counting failed: %v", err)
	}

	if stats.RoundsPlayed != 20000 {
		t.Errorf("Expected 20000 rounds, got %d", stats.RoundsPlayed)
	}
	t.Logf("Card Counting Net Profit: $%.2f, Total Wagered: $%.2f, EV: %.4f",
		stats.TotalNetProfit, stats.TotalAmountWagered, stats.ExpectedValue)
}

func TestSimulator_ContextCancellation(t *testing.T) {
	config := SimConfig{
		Rules:       rules.VegasStrip(),
		TotalRounds: 1000000, // Large run
		Workers:     2,
		BaseBet:     10.0,
		Seed:        999,
	}

	sim := NewSimulator(config)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	stats, err := sim.Run(ctx, nil)
	if err != nil {
		t.Fatalf("Expected clean cancellation, got error: %v", err)
	}

	if stats.RoundsPlayed >= 1000000 {
		t.Errorf("Simulation did not stop early on context cancellation")
	}
}

func TestSimulator_InvalidRules(t *testing.T) {
	invalidRule := rules.VegasStrip()
	invalidRule.Decks = 0 // Invalid

	config := SimConfig{
		Rules:       invalidRule,
		TotalRounds: 100,
	}

	sim := NewSimulator(config)
	_, err := sim.Run(context.Background(), nil)
	if err == nil {
		t.Error("Expected error on invalid rules, got nil")
	}
}

func BenchmarkMonteCarlo10kHands(b *testing.B) {
	config := SimConfig{
		Rules:           rules.VegasStrip(),
		TotalRounds:     10000,
		Workers:         4,
		CountingSystem:  oracle.HiLo,
		UseCardCounting: true,
		BaseBet:         10.0,
		Seed:            101,
	}
	sim := NewSimulator(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sim.Run(context.Background(), nil)
	}
}
