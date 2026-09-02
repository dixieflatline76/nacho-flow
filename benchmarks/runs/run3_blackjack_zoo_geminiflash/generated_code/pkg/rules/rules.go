package rules

import "fmt"

// BlackjackPayout represents the payout multiplier on a winning natural Blackjack.
type BlackjackPayout float64

const (
	Payout3to2 BlackjackPayout = 1.5 // 3:2 payout (standard/favorable)
	Payout6to5 BlackjackPayout = 1.2 // 6:5 payout (high house edge)
	Payout1to1 BlackjackPayout = 1.0 // 1:1 payout (single deck / variant)
)

func (p BlackjackPayout) String() string {
	switch p {
	case Payout3to2:
		return "3:2"
	case Payout6to5:
		return "6:5"
	case Payout1to1:
		return "1:1"
	default:
		return fmt.Sprintf("%.2f:1", float64(p))
	}
}

// SurrenderRule specifies the surrender availability.
type SurrenderRule int

const (
	SurrenderNone  SurrenderRule = iota
	SurrenderLate                // Surrender after dealer peeks for BJ
	SurrenderEarly               // Surrender before dealer peeks for BJ
)

func (s SurrenderRule) String() string {
	switch s {
	case SurrenderNone:
		return "None"
	case SurrenderLate:
		return "Late Surrender"
	case SurrenderEarly:
		return "Early Surrender"
	default:
		return "Unknown"
	}
}

// DoubleRestriction specifies on what hand totals doubling is permitted.
type DoubleRestriction int

const (
	DoubleAny2Cards DoubleRestriction = iota
	Double9To11Only
	Double10Or11Only
)

func (d DoubleRestriction) String() string {
	switch d {
	case DoubleAny2Cards:
		return "Any 2 Cards"
	case Double9To11Only:
		return "9, 10, or 11 only"
	case Double10Or11Only:
		return "10 or 11 only"
	default:
		return "Unknown"
	}
}

// RuleSet encapsulates all casino table rules and parameters.
type RuleSet struct {
	Name                string
	Decks               int
	DealerHitsSoft17    bool              // true = H17, false = S17
	BlackjackPayout     BlackjackPayout   // 3:2 or 6:5
	DoubleAfterSplit    bool              // DAS
	DoubleRestriction   DoubleRestriction // Any 2, 9-11, 10-11
	MaxSplitHands       int               // Max resulting hands (e.g., 4 = split up to 3 times)
	ResplitAces         bool              // Can split Aces more than once
	HitSplitAces        bool              // Can draw multiple cards to split Aces
	Surrender           SurrenderRule     // Late / Early / None
	DealerPeeksHoleCard bool              // US Peek on Ace/10 vs European No-Hole-Card (ENHC)
	InsuranceOffered    bool              // Pays 2:1 on dealer Ace
	DeckPenetration     float64           // Shoe penetration cut card (e.g., 0.75)
}

// VegasStrip returns the standard Vegas Strip casino ruleset (4 decks, S17, 3:2, DAS, US Peek, Late Surrender).
func VegasStrip() RuleSet {
	return RuleSet{
		Name:                "Vegas Strip",
		Decks:               4,
		DealerHitsSoft17:    false, // S17
		BlackjackPayout:     Payout3to2,
		DoubleAfterSplit:    true,
		DoubleRestriction:   DoubleAny2Cards,
		MaxSplitHands:       4,
		ResplitAces:         false,
		HitSplitAces:        false,
		Surrender:           SurrenderLate,
		DealerPeeksHoleCard: true,
		InsuranceOffered:    true,
		DeckPenetration:     0.75,
	}
}

// AtlanticCity returns Atlantic City ruleset (8 decks, S17, 3:2, DAS, US Peek, Late Surrender, Resplit to 4 hands).
func AtlanticCity() RuleSet {
	return RuleSet{
		Name:                "Atlantic City",
		Decks:               8,
		DealerHitsSoft17:    false, // S17
		BlackjackPayout:     Payout3to2,
		DoubleAfterSplit:    true,
		DoubleRestriction:   DoubleAny2Cards,
		MaxSplitHands:       4,
		ResplitAces:         true,
		HitSplitAces:        false,
		Surrender:           SurrenderLate,
		DealerPeeksHoleCard: true,
		InsuranceOffered:    true,
		DeckPenetration:     0.75,
	}
}

// European returns European Blackjack ruleset (2 decks, S17, 3:2, ENHC - No Hole Card Peek, Double on 9-11 only, No Surrender).
func European() RuleSet {
	return RuleSet{
		Name:                "European",
		Decks:               2,
		DealerHitsSoft17:    false, // S17
		BlackjackPayout:     Payout3to2,
		DoubleAfterSplit:    false,
		DoubleRestriction:   Double9To11Only,
		MaxSplitHands:       2,
		ResplitAces:         false,
		HitSplitAces:        false,
		Surrender:           SurrenderNone,
		DealerPeeksHoleCard: false, // ENHC
		InsuranceOffered:    true,
		DeckPenetration:     0.65,
	}
}

// SingleDeckVegas returns Single Deck Vegas Downtown ruleset (1 deck, H17, 3:2 or 6:5, No DAS).
func SingleDeckVegas(payout BlackjackPayout) RuleSet {
	return RuleSet{
		Name:                "Single Deck Vegas",
		Decks:               1,
		DealerHitsSoft17:    true, // H17
		BlackjackPayout:     payout,
		DoubleAfterSplit:    false,
		DoubleRestriction:   DoubleAny2Cards,
		MaxSplitHands:       2,
		ResplitAces:         false,
		HitSplitAces:        false,
		Surrender:           SurrenderNone,
		DealerPeeksHoleCard: true,
		InsuranceOffered:    true,
		DeckPenetration:     0.60,
	}
}

// Validate ensures rules are physically and logically consistent.
func (r *RuleSet) Validate() error {
	if r.Decks < 1 || r.Decks > 8 {
		return fmt.Errorf("deck count must be between 1 and 8, got %d", r.Decks)
	}
	if r.BlackjackPayout <= 0 {
		return fmt.Errorf("blackjack payout must be positive, got %v", r.BlackjackPayout)
	}
	if r.MaxSplitHands < 1 {
		return fmt.Errorf("max split hands must be >= 1, got %d", r.MaxSplitHands)
	}
	if r.DeckPenetration <= 0 || r.DeckPenetration >= 1.0 {
		r.DeckPenetration = 0.75
	}
	return nil
}
