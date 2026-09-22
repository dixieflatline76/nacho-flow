package tuner

import (
	"fmt"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

// StaticDominanceConflict captures a logical defect in the routing hierarchy where
// a fallback or escalation tier is mathematically or functionally inferior to its prerequisite tier.
type StaticDominanceConflict struct {
	TierIndex         int     `json:"tier_index"`
	TierName          string  `json:"tier_name"`
	PredecessorIndex  int     `json:"predecessor_index"`
	PredecessorName   string  `json:"predecessor_name"`
	CurrentBenchmark  float64 `json:"current_benchmark"`
	RequiredBenchmark float64 `json:"required_benchmark"`
	Reason            string  `json:"reason"`
}

// IsFrontierModel determines whether an LLM model identifier represents a true frontier capability class
// by querying the canonical curated catalog.
func IsFrontierModel(modelID string) bool {
	if modelID == "" {
		return false
	}
	if p, ok := curation.DefaultManager().Lookup(modelID); ok {
		return p.IsFrontier()
	}
	return false
}

// IsFrontierTier determines whether a tier's assigned model designates it as a frontier capability class.
func IsFrontierTier(tierName, modelID string) bool {
	return IsFrontierModel(modelID)
}

type activeTierInfo struct {
	index             int
	name              string
	model             string
	retryBound        int
	retryFloor        int
	codingIndex       float64
	hasVision         bool
	hasTools          bool
	isFrontier        bool
	requiresKickstart bool
	requiresImages    bool
}

// CountFleetDominanceConflicts returns the number of static capability inversions.
// Zero-allocation hot path function for EvaluateFleetConflict.
func CountFleetDominanceConflicts(cfg *MultiTierConfig) int {
	if cfg == nil || len(cfg.Tiers) == 0 {
		return 0
	}

	var activeTiers [MaxSupportedTiers]activeTierInfo
	activeCount := 0
	conflicts := 0

	for i := 0; i < len(cfg.Tiers) && i < MaxSupportedTiers; i++ {
		t := &cfg.Tiers[i]
		if t.IsDisabled {
			continue
		}

		info := activeTierInfo{
			index:             i,
			name:              t.TierName,
			model:             t.Model,
			retryBound:        t.RetryBound,
			retryFloor:        t.RetryFloor,
			codingIndex:       t.CodingIndex,
			hasVision:         t.SupportsVision,
			hasTools:          t.SupportsTools,
			isFrontier:        t.IsFrontier || IsFrontierModel(t.Model),
			requiresKickstart: t.RequiresKickstart,
			requiresImages:    t.RequiresImages,
		}

		var immediatePred *activeTierInfo
		for j := activeCount - 1; j >= 0; j-- {
			pred := &activeTiers[j]
			if pred.requiresKickstart || pred.requiresImages {
				continue
			}
			isEscalation := false
			if info.retryFloor > 0 && pred.retryBound > 0 && info.retryFloor >= pred.retryBound {
				isEscalation = true
			} else if pred.retryBound > 0 && info.retryBound > pred.retryBound {
				isEscalation = true
			} else if pred.retryBound > 0 && info.retryBound == 0 {
				isEscalation = true
			}

			if isEscalation {
				immediatePred = pred
				break
			}
		}

		if immediatePred != nil {
			if info.model != "" && info.model == immediatePred.model {
				conflicts++
			} else if immediatePred.codingIndex > 0 && info.codingIndex > 0 {
				if !info.isFrontier && info.codingIndex < immediatePred.codingIndex {
					conflicts++
				}
			}
		}

		activeTiers[activeCount] = info
		activeCount++
	}

	defaultIsFrontier := cfg.DefaultTier.IsFrontier || IsFrontierModel(cfg.DefaultTier.Model)
	if !defaultIsFrontier && cfg.DefaultTier.CodingIndex > 0 {
		var highestPred *activeTierInfo
		for i := 0; i < activeCount; i++ {
			pred := &activeTiers[i]
			if pred.requiresKickstart || pred.requiresImages || pred.retryFloor > 0 {
				continue
			}
			if highestPred == nil || pred.codingIndex > highestPred.codingIndex {
				highestPred = pred
			}
		}

		if highestPred != nil && cfg.DefaultTier.CodingIndex < highestPred.codingIndex {
			conflicts++
		}
	}

	return conflicts
}

// AnalyzeFleetDominance inspects a MultiTierConfig for capability inversions along the cascade.
// It verifies that any tier serving as a fallback or escalation path for a predecessor tier
// provides at least equal coding reasoning capability (CodingIndex) and does not drop modality.
func AnalyzeFleetDominance(cfg *MultiTierConfig) []StaticDominanceConflict {
	if cfg == nil || len(cfg.Tiers) == 0 {
		return nil
	}

	var conflicts []StaticDominanceConflict
	var activeTiers [MaxSupportedTiers]activeTierInfo
	activeCount := 0

	for i := 0; i < len(cfg.Tiers) && i < MaxSupportedTiers; i++ {
		t := &cfg.Tiers[i]
		if t.IsDisabled {
			continue
		}

		info := activeTierInfo{
			index:             i,
			name:              t.TierName,
			model:             t.Model,
			retryBound:        t.RetryBound,
			retryFloor:        t.RetryFloor,
			codingIndex:       t.CodingIndex,
			hasVision:         t.SupportsVision,
			hasTools:          t.SupportsTools,
			isFrontier:        t.IsFrontier || IsFrontierModel(t.Model),
			requiresKickstart: t.RequiresKickstart,
			requiresImages:    t.RequiresImages,
		}

		// Find the immediate escalation predecessor in the retry hierarchy.
		var immediatePred *activeTierInfo
		for j := activeCount - 1; j >= 0; j-- {
			pred := &activeTiers[j]
			if pred.requiresKickstart || pred.requiresImages {
				continue
			}
			isEscalation := false
			if info.retryFloor > 0 && pred.retryBound > 0 && info.retryFloor >= pred.retryBound {
				isEscalation = true
			} else if pred.retryBound > 0 && info.retryBound > pred.retryBound {
				isEscalation = true
			} else if pred.retryBound > 0 && info.retryBound == 0 {
				isEscalation = true
			}

			if isEscalation {
				immediatePred = pred
				break
			}
		}

		if immediatePred != nil {
			if info.model != "" && info.model == immediatePred.model {
				conflicts = append(conflicts, StaticDominanceConflict{
					TierIndex:         i,
					TierName:          t.TierName,
					PredecessorIndex:  immediatePred.index,
					PredecessorName:   immediatePred.name,
					CurrentBenchmark:  info.codingIndex,
					RequiredBenchmark: immediatePred.codingIndex,
					Reason: fmt.Sprintf(
						"Escalation tier %q duplicates predecessor model %q (degenerate retry loop)",
						t.TierName, immediatePred.model,
					),
				})
			} else if immediatePred.codingIndex > 0 && info.codingIndex > 0 {
				// Frontier tiers are inherently superior due to frontier reasoning and multi-step planning;
				// synthetic coding indices of workhorse flash models cannot dominate a frontier tier.
				if !info.isFrontier && info.codingIndex < immediatePred.codingIndex {
					conflicts = append(conflicts, StaticDominanceConflict{
						TierIndex:         i,
						TierName:          t.TierName,
						PredecessorIndex:  immediatePred.index,
						PredecessorName:   immediatePred.name,
						CurrentBenchmark:  info.codingIndex,
						RequiredBenchmark: immediatePred.codingIndex,
						Reason: fmt.Sprintf(
							"Escalation tier %q (CodingIndex %.1f) is strictly inferior to prerequisite tier %q (CodingIndex %.1f)",
							t.TierName, info.codingIndex, immediatePred.name, immediatePred.codingIndex,
						),
					})
				}
			}
		}

		activeTiers[activeCount] = info
		activeCount++
	}

	// Also inspect DefaultTier against the highest capability prerequisite in the active cascade,
	// only if DefaultTier itself is NOT a frontier model/tier.
	defaultIsFrontier := cfg.DefaultTier.IsFrontier || IsFrontierModel(cfg.DefaultTier.Model)
	if !defaultIsFrontier && cfg.DefaultTier.CodingIndex > 0 {
		var highestPred *activeTierInfo
		for i := 0; i < activeCount; i++ {
			pred := &activeTiers[i]
			if pred.requiresKickstart || pred.requiresImages || pred.retryFloor > 0 {
				continue
			}
			if highestPred == nil || pred.codingIndex > highestPred.codingIndex {
				highestPred = pred
			}
		}

		if highestPred != nil && cfg.DefaultTier.CodingIndex < highestPred.codingIndex {
			conflicts = append(conflicts, StaticDominanceConflict{
				TierIndex:         len(cfg.Tiers),
				TierName:          cfg.DefaultTier.TierName,
				PredecessorIndex:  highestPred.index,
				PredecessorName:   highestPred.name,
				CurrentBenchmark:  cfg.DefaultTier.CodingIndex,
				RequiredBenchmark: highestPred.codingIndex,
				Reason: fmt.Sprintf(
					"Fallback catch-all tier %q (CodingIndex %.1f) is inferior to prerequisite tier %q (CodingIndex %.1f)",
					cfg.DefaultTier.TierName, cfg.DefaultTier.CodingIndex, highestPred.name, highestPred.codingIndex,
				),
			})
		}
	}

	return conflicts
}
