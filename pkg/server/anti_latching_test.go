package server

import (
	"log/slog"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router"
)

func TestEnforceAntiLatchingBreaker_Unit(t *testing.T) {
	defaultTier := contract.Tier{Name: "expensive-frontier", Model: "gpt-4o"}
	workhorseTier := contract.Tier{Name: "workhorse-coder", Model: "claude-3-5-sonnet"}

	cfg := &contract.Config{
		DefaultTier: defaultTier,
		Tiers: []contract.Tier{
			defaultTier,
			workhorseTier,
		},
	}

	sessionKey := "test-anti-latch-sess"
	sessionTracker := router.NewSessionTracker(10 * time.Minute)
	sessionTracker.RecordTurn(sessionKey, 1234, false)
	srv := &Server{
		sessionTracker: sessionTracker,
	}
	logger := slog.Default()

	// Turns 1, 2, 3: Escalation budget not exhausted -> remains defaultTier
	for i := 1; i <= 3; i++ {
		res := srv.enforceAntiLatchingBreaker(sessionKey, defaultTier, cfg, logger)
		if res.Name != defaultTier.Name {
			t.Fatalf("Turn %d: expected %s, got %s", i, defaultTier.Name, res.Name)
		}
	}

	// Turn 4: Budget exhausted -> de-escalate to workhorseTier
	resTurn4 := srv.enforceAntiLatchingBreaker(sessionKey, defaultTier, cfg, logger)
	if resTurn4.Name != workhorseTier.Name {
		t.Fatalf("Turn 4: expected de-escalation to %s, got %s", workhorseTier.Name, resTurn4.Name)
	}

	// Turn 5: Request resolves to workhorseTier -> resets escalation counter
	resTurn5 := srv.enforceAntiLatchingBreaker(sessionKey, workhorseTier, cfg, logger)
	if resTurn5.Name != workhorseTier.Name {
		t.Fatalf("Turn 5: expected %s, got %s", workhorseTier.Name, resTurn5.Name)
	}

	// Turn 6: Next escalation to default tier starts counter over (Turn 1 of new cycle)
	resTurn6 := srv.enforceAntiLatchingBreaker(sessionKey, defaultTier, cfg, logger)
	if resTurn6.Name != defaultTier.Name {
		t.Fatalf("Turn 6 after reset: expected %s, got %s", defaultTier.Name, resTurn6.Name)
	}
}

func TestEnforceAntiLatchingBreaker_NoAlternativeTier(t *testing.T) {
	defaultTier := contract.Tier{Name: "only-tier", Model: "gpt-4o"}

	cfg := &contract.Config{
		DefaultTier: defaultTier,
		Tiers: []contract.Tier{
			defaultTier,
		},
	}

	sessionKey := "test-anti-latch-single-tier"
	sessionTracker := router.NewSessionTracker(10 * time.Minute)
	sessionTracker.RecordTurn(sessionKey, 1234, false)
	srv := &Server{
		sessionTracker: sessionTracker,
	}
	logger := slog.Default()

	// Exhaust budget
	for i := 1; i <= 4; i++ {
		res := srv.enforceAntiLatchingBreaker(sessionKey, defaultTier, cfg, logger)
		// Since no alternative tier exists in Tiers, remains defaultTier without panic
		if res.Name != defaultTier.Name {
			t.Fatalf("Turn %d: expected %s, got %s", i, defaultTier.Name, res.Name)
		}
	}
}
