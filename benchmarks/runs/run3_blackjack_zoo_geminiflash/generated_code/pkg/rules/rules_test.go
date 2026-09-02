package rules

import "testing"

func TestRulePresetsAndStrings(t *testing.T) {
	vegas := VegasStrip()
	if err := vegas.Validate(); err != nil {
		t.Fatalf("VegasStrip validation failed: %v", err)
	}
	if vegas.DealerHitsSoft17 {
		t.Errorf("VegasStrip should be S17")
	}
	if vegas.Decks != 4 {
		t.Errorf("VegasStrip decks expected 4, got %d", vegas.Decks)
	}

	ac := AtlanticCity()
	if err := ac.Validate(); err != nil {
		t.Fatalf("AtlanticCity validation failed: %v", err)
	}
	if !ac.ResplitAces {
		t.Errorf("AtlanticCity should allow ResplitAces")
	}

	euro := European()
	if err := euro.Validate(); err != nil {
		t.Fatalf("European validation failed: %v", err)
	}
	if euro.DealerPeeksHoleCard {
		t.Errorf("European should be ENHC (no hole card peek)")
	}
	if euro.DoubleRestriction != Double9To11Only {
		t.Errorf("European should restrict double to 9-11")
	}

	single32 := SingleDeckVegas(Payout3to2)
	if err := single32.Validate(); err != nil {
		t.Fatalf("SingleDeckVegas 3:2 validation failed: %v", err)
	}
	if !single32.DealerHitsSoft17 {
		t.Errorf("SingleDeckVegas should be H17")
	}

	single65 := SingleDeckVegas(Payout6to5)
	if single65.BlackjackPayout != Payout6to5 {
		t.Errorf("Expected 6:5 payout")
	}

	// Test String() methods
	if Payout3to2.String() != "3:2" || Payout6to5.String() != "6:5" || Payout1to1.String() != "1:1" || BlackjackPayout(2.0).String() != "2.00:1" {
		t.Errorf("Unexpected Payout string representation")
	}

	if SurrenderNone.String() != "None" || SurrenderLate.String() != "Late Surrender" || SurrenderEarly.String() != "Early Surrender" || SurrenderRule(99).String() != "Unknown" {
		t.Errorf("Unexpected Surrender string representation")
	}

	if DoubleAny2Cards.String() != "Any 2 Cards" || Double9To11Only.String() != "9, 10, or 11 only" || Double10Or11Only.String() != "10 or 11 only" || DoubleRestriction(99).String() != "Unknown" {
		t.Errorf("Unexpected DoubleRestriction string representation")
	}
}

func TestRuleSet_ValidationErrors(t *testing.T) {
	r := VegasStrip()

	r.Decks = 0
	if err := r.Validate(); err == nil {
		t.Errorf("Expected error for 0 decks")
	}

	r.Decks = 9
	if err := r.Validate(); err == nil {
		t.Errorf("Expected error for 9 decks")
	}

	r.Decks = 6
	r.BlackjackPayout = 0
	if err := r.Validate(); err == nil {
		t.Errorf("Expected error for 0 payout")
	}

	r.BlackjackPayout = Payout3to2
	r.MaxSplitHands = 0
	if err := r.Validate(); err == nil {
		t.Errorf("Expected error for 0 max split hands")
	}

	r.MaxSplitHands = 4
	r.DeckPenetration = 0
	_ = r.Validate()
	if r.DeckPenetration != 0.75 {
		t.Errorf("Expected penetration default fallback to 0.75, got %f", r.DeckPenetration)
	}
}
