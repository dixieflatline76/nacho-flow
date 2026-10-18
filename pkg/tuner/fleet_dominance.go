package tuner

import (
	"fmt"
	"math"

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
	reasoningEffort   string
	retryBound        int
	retryFloor        int
	codingIndex       float64
	hasVision         bool
	hasTools          bool
	isFrontier        bool
	requiresKickstart bool
	requiresImages    bool
	isKickoff         bool
}

func (t *activeTierInfo) isLateralOrKickoff() bool {
	return t.requiresKickstart || t.requiresImages || t.isKickoff
}

func effectiveTierCoding(t *TierReplayConfig) float64 {
	if t == nil {
		return 0
	}
	if t.ReasoningEffort == "" {
		return t.CodingIndex
	}
	if base, _ := ResolveModelBenchmark(t.Model); base > 0 && math.Abs(t.CodingIndex-base) > 0.01 {
		// Already scaled by EffectiveCodingIndex in config_bridge / builder
		return t.CodingIndex
	}
	return EffectiveCodingIndex(t.CodingIndex, t.ReasoningEffort)
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
			reasoningEffort:   t.ReasoningEffort,
			retryBound:        t.RetryBound,
			retryFloor:        t.RetryFloor,
			codingIndex:       effectiveTierCoding(t),
			hasVision:         t.SupportsVision,
			hasTools:          t.SupportsTools,
			isFrontier:        t.IsFrontier || IsFrontierModel(t.Model),
			requiresKickstart: t.RequiresKickstart,
			requiresImages:    t.RequiresImages,
			isKickoff:         t.MessageCountMax > 0,
		}

		var immediatePred *activeTierInfo
		for j := activeCount - 1; j >= 0; j-- {
			pred := &activeTiers[j]
			if pred.isLateralOrKickoff() {
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
				if info.reasoningEffort != immediatePred.reasoningEffort && info.codingIndex > immediatePred.codingIndex {
					// Valid reasoning escalation chain (e.g. low -> high -> max)
				} else {
					conflicts++
				}
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
	defaultCoding := effectiveTierCoding(&cfg.DefaultTier)
	if !defaultIsFrontier && defaultCoding > 0 {
		var highestPred *activeTierInfo
		for i := 0; i < activeCount; i++ {
			pred := &activeTiers[i]
			if pred.isLateralOrKickoff() || pred.retryFloor > 0 {
				continue
			}
			if highestPred == nil || pred.codingIndex > highestPred.codingIndex {
				highestPred = pred
			}
		}

		if highestPred != nil {
			if cfg.DefaultTier.Model != "" && cfg.DefaultTier.Model == highestPred.model && (highestPred.reasoningEffort != "" || cfg.DefaultTier.ReasoningEffort != "") {
				if cfg.DefaultTier.ReasoningEffort != highestPred.reasoningEffort && defaultCoding > highestPred.codingIndex {
					// Valid reasoning escalation chain (e.g. low/med -> high -> max)
				} else {
					conflicts++
				}
			} else if defaultCoding < highestPred.codingIndex {
				conflicts++
			}
		}
	}

	return conflicts
}

// HighestPredecessorCodingIndex returns the highest effective coding index among active predecessor tiers
// that cascade down to the fallback tier (excluding lateral, kickoff, and retry-escalation tiers).
func HighestPredecessorCodingIndex(cfg *MultiTierConfig) float64 {
	if cfg == nil || len(cfg.Tiers) == 0 {
		return 0
	}
	var highest float64
	for i := range cfg.Tiers {
		t := &cfg.Tiers[i]
		if t.IsDisabled || t.RequiresKickstart || t.RequiresImages || t.MessageCountMax > 0 || t.RetryFloor > 0 {
			continue
		}
		effCoding := effectiveTierCoding(t)
		if effCoding > highest {
			highest = effCoding
		}
	}
	return highest
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
			reasoningEffort:   t.ReasoningEffort,
			retryBound:        t.RetryBound,
			retryFloor:        t.RetryFloor,
			codingIndex:       effectiveTierCoding(t),
			hasVision:         t.SupportsVision,
			hasTools:          t.SupportsTools,
			isFrontier:        t.IsFrontier || IsFrontierModel(t.Model),
			requiresKickstart: t.RequiresKickstart,
			requiresImages:    t.RequiresImages,
			isKickoff:         t.MessageCountMax > 0,
		}

		// Find the immediate escalation predecessor in the retry hierarchy.
		var immediatePred *activeTierInfo
		for j := activeCount - 1; j >= 0; j-- {
			pred := &activeTiers[j]
			if pred.isLateralOrKickoff() {
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
				if info.reasoningEffort != immediatePred.reasoningEffort && info.codingIndex > immediatePred.codingIndex {
					// Valid reasoning escalation chain (e.g. low -> high -> max)
				} else if info.reasoningEffort != immediatePred.reasoningEffort && info.codingIndex < immediatePred.codingIndex {
					conflicts = append(conflicts, StaticDominanceConflict{
						TierIndex:         i,
						TierName:          t.TierName,
						PredecessorIndex:  immediatePred.index,
						PredecessorName:   immediatePred.name,
						CurrentBenchmark:  info.codingIndex,
						RequiredBenchmark: immediatePred.codingIndex,
						Reason: fmt.Sprintf(
							"Escalation tier %q (CodingIndex %.1f, effort: %s) regresses capability compared to prerequisite tier %q (CodingIndex %.1f, effort: %s)",
							t.TierName, info.codingIndex, info.reasoningEffort, immediatePred.name, immediatePred.codingIndex, immediatePred.reasoningEffort,
						),
					})
				} else {
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
				}
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
	defaultCoding := effectiveTierCoding(&cfg.DefaultTier)
	if !defaultIsFrontier && defaultCoding > 0 {
		var highestPred *activeTierInfo
		for i := 0; i < activeCount; i++ {
			pred := &activeTiers[i]
			if pred.isLateralOrKickoff() || pred.retryFloor > 0 {
				continue
			}
			if highestPred == nil || pred.codingIndex > highestPred.codingIndex {
				highestPred = pred
			}
		}

		if highestPred != nil {
			if cfg.DefaultTier.Model != "" && cfg.DefaultTier.Model == highestPred.model && (highestPred.reasoningEffort != "" || cfg.DefaultTier.ReasoningEffort != "") {
				if cfg.DefaultTier.ReasoningEffort != highestPred.reasoningEffort && defaultCoding > highestPred.codingIndex {
					// Valid reasoning escalation chain (e.g. low/med -> high -> max)
				} else if cfg.DefaultTier.ReasoningEffort != highestPred.reasoningEffort && defaultCoding < highestPred.codingIndex {
					conflicts = append(conflicts, StaticDominanceConflict{
						TierIndex:         len(cfg.Tiers),
						TierName:          cfg.DefaultTier.TierName,
						PredecessorIndex:  highestPred.index,
						PredecessorName:   highestPred.name,
						CurrentBenchmark:  defaultCoding,
						RequiredBenchmark: highestPred.codingIndex,
						Reason: fmt.Sprintf(
							"Fallback catch-all tier %q (CodingIndex %.1f, effort: %s) regresses capability compared to prerequisite tier %q (CodingIndex %.1f, effort: %s)",
							cfg.DefaultTier.TierName, defaultCoding, cfg.DefaultTier.ReasoningEffort, highestPred.name, highestPred.codingIndex, highestPred.reasoningEffort,
						),
					})
				} else {
					conflicts = append(conflicts, StaticDominanceConflict{
						TierIndex:         len(cfg.Tiers),
						TierName:          cfg.DefaultTier.TierName,
						PredecessorIndex:  highestPred.index,
						PredecessorName:   highestPred.name,
						CurrentBenchmark:  defaultCoding,
						RequiredBenchmark: highestPred.codingIndex,
						Reason: fmt.Sprintf(
							"Fallback catch-all tier %q duplicates predecessor model %q at same reasoning effort %q (degenerate retry loop)",
							cfg.DefaultTier.TierName, highestPred.model, highestPred.reasoningEffort,
						),
					})
				}
			} else if defaultCoding < highestPred.codingIndex {
				conflicts = append(conflicts, StaticDominanceConflict{
					TierIndex:         len(cfg.Tiers),
					TierName:          cfg.DefaultTier.TierName,
					PredecessorIndex:  highestPred.index,
					PredecessorName:   highestPred.name,
					CurrentBenchmark:  defaultCoding,
					RequiredBenchmark: highestPred.codingIndex,
					Reason: fmt.Sprintf(
						"Fallback catch-all tier %q (CodingIndex %.1f) is inferior to prerequisite tier %q (CodingIndex %.1f)",
						cfg.DefaultTier.TierName, defaultCoding, highestPred.name, highestPred.codingIndex,
					),
				})
			}
		}
	}

	return conflicts
}
