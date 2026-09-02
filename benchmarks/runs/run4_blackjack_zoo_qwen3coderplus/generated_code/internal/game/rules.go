package game

// RuleSet defines the configurable rules for a Blackjack game
type RuleSet struct {
	Name              string  // Name of the rule set (e.g. "Vegas Strip", "Atlantic City", "European")
	NumDecks          int     // Number of decks in the shoe (1-8)
	BlackjackPayout   float64 // Payout ratio for a natural blackjack (typically 3:2 = 1.5 or 6:5 = 1.2)
	DealerHitsSoft17  bool    // True if dealer hits on soft 17 (H17), False if stands (S17)
	DoubleAfterSplit  bool    // True if players can double down after splitting
	SplitAcesOnceOnly bool    // True if aces can only be split once
	ResplitAces       bool    // True if aces can be resplit after an initial split
	LateSurrender     bool    // True if late surrender is allowed
	InsuranceAllowed  bool    // True if insurance bets are allowed
	MaxSplits         int     // Maximum number of hands a player can have after splitting (usually 1-3 splits = 2-4 hands)
}

// Predefined rule sets
var (
	VegasStrip = RuleSet{
		Name:              "Vegas Strip",
		NumDecks:          6,
		BlackjackPayout:   1.5,  // 3:2
		DealerHitsSoft17:  true, // H17
		DoubleAfterSplit:  true,
		SplitAcesOnceOnly: true,
		ResplitAces:       false,
		LateSurrender:     true,
		InsuranceAllowed:  true,
		MaxSplits:         3, // Up to 4 hands
	}

	AtlanticCity = RuleSet{
		Name:              "Atlantic City",
		NumDecks:          8,
		BlackjackPayout:   1.5,   // 3:2
		DealerHitsSoft17:  true,  // H17
		DoubleAfterSplit:  false, // No DAS
		SplitAcesOnceOnly: true,
		ResplitAces:       false,
		LateSurrender:     true,
		InsuranceAllowed:  true,
		MaxSplits:         3, // Up to 4 hands
	}

	European = RuleSet{
		Name:              "European",
		NumDecks:          6,
		BlackjackPayout:   1.5,   // 3:2
		DealerHitsSoft17:  false, // S17
		DoubleAfterSplit:  false, // No DAS
		SplitAcesOnceOnly: true,
		ResplitAces:       false,
		LateSurrender:     false,
		InsuranceAllowed:  false,
		MaxSplits:         1, // Up to 2 hands
	}

	// HighLowReno is a common variant that pays 6:5
	HighLowReno = RuleSet{
		Name:              "High-Low Reno",
		NumDecks:          6,
		BlackjackPayout:   1.2,   // 6:5
		DealerHitsSoft17:  true,  // H17
		DoubleAfterSplit:  false, // No DAS
		SplitAcesOnceOnly: true,
		ResplitAces:       false,
		LateSurrender:     false,
		InsuranceAllowed:  true,
		MaxSplits:         1, // Up to 2 hands
	}
)

// GetRuleSetByName returns a predefined rule set by name
func GetRuleSetByName(name string) *RuleSet {
	switch name {
	case "Vegas Strip":
		return &VegasStrip
	case "Atlantic City":
		return &AtlanticCity
	case "European":
		return &European
	case "High-Low Reno":
		return &HighLowReno
	default:
		return &VegasStrip // Default fallback
	}
}
