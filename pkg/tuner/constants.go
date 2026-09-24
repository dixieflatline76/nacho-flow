package tuner

// Default algorithmic thresholds for coding index matching and model replacement.
const (
	// DefaultCodingParityTolerance is the maximum allowed coding index delta below current tier benchmark.
	DefaultCodingParityTolerance = 0.50

	// DefaultMinSavingsPct is the minimum cost reduction percentage required to justify recommending a model change.
	DefaultMinSavingsPct = 20.0

	// DefaultContextWindowFloor is the fallback context ceiling (32k) when a tier does not define an explicit token constraint.
	DefaultContextWindowFloor = 32000

	// DefaultMinCloudSpendUSD is the minimum dollar spend required on a cloud tier before evaluating model replacement.
	DefaultMinCloudSpendUSD = 0.10
)

// Advisory diff format strings for plain-English benefit presentation.
const (
	BenefitMsgWithCognitiveGain = "Recommended Model Replacement: Matches coding index (%.1f vs %.1f, +%.1f%%) with %.1f%% cost savings ($%.2f/1M vs $%.2f/1M)"
	BenefitMsgAtCognitiveParity = "Recommended Model Replacement: Matches coding index (%.1f) with %.1f%% cost savings ($%.2f/1M vs $%.2f/1M)"
)
