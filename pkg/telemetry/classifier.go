package telemetry

import (
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

// Capability thresholds for heuristic & live benchmark classification.
const (
	LiveBenchmarkCodingThreshold = 70.0
	VisionWorkhorseMaxPromptCost = 0.50
)

var (
	coderKeywords    = []string{"coder", "codestral", "starcoder", "deepseek-coder"}
	reasonerKeywords = []string{"r1", "reason", "thinking"}
	fastProseKeywords = []string{"flash", "lite", "mini"}
)

// Classifier evaluates model metadata against the Curated Gallery, live API benchmarks, and heuristics.
type Classifier struct {
	gallery *curation.Manager
}

// NewClassifier creates a new 3-tier capability classifier.
func NewClassifier(gallery *curation.Manager) *Classifier {
	return &Classifier{gallery: gallery}
}

// ClassifyModel evaluates a model through Tier 1 (Gallery), Tier 2 (Live API), and Tier 3 (Heuristics).
// Returns (TierRole, CodingIndex, RecommendedTiers).
func (c *Classifier) ClassifyModel(m ModelMetadata) (curation.TierRole, float64, []string) {
	// 1. Tier 1: Check Curated Gallery (Highest Fidelity)
	if c.gallery != nil {
		if profile, found := c.gallery.Lookup(m.ModelID); found {
			codingScore := profile.CodingIndex
			if m.CodingIndex > 0 {
				codingScore = m.CodingIndex // Live API benchmark takes priority if available
			}
			return profile.TierRole, codingScore, profile.RecommendedTiers
		}
	}

	// 2. Tier 2: Live API Benchmark
	if m.CodingIndex >= LiveBenchmarkCodingThreshold {
		return curation.RoleCodingWorkhorse, m.CodingIndex, []string{contract.TierIDWorkhorse}
	}

	// 3. Tier 3: Heuristic Fallback
	name := strings.ToLower(m.ModelID)
	if m.SupportsVision && m.PromptCostPerMillion < VisionWorkhorseMaxPromptCost {
		return curation.RoleVisionWorkhorse, m.CodingIndex, []string{contract.TierIDVision}
	}
	if (matchesAny(name, coderKeywords) || strings.HasSuffix(name, "-dev")) && m.SupportsTools {
		return curation.RoleCodingWorkhorse, m.CodingIndex, []string{contract.TierIDWorkhorse}
	}
	if matchesAny(name, reasonerKeywords) {
		return curation.RoleDeepReasoner, m.CodingIndex, []string{contract.TierIDFrontier}
	}
	if matchesAny(name, fastProseKeywords) {
		return curation.RoleFastProse, m.CodingIndex, []string{contract.TierIDVision}
	}

	return curation.RoleGeneral, m.CodingIndex, nil
}

func matchesAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
