package tuner

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
	"github.com/expr-lang/expr/parser"
)

// MinConflictsOptimizer implements the 6D Min-Conflicts CSP local search optimizer.
type MinConflictsOptimizer struct {
	policy TuningPolicy
}

// NewMinConflictsOptimizer creates an optimizer configured with the given tuning policy.
func NewMinConflictsOptimizer(policy TuningPolicy) *MinConflictsOptimizer {
	return &MinConflictsOptimizer{
		policy: policy,
	}
}

// Name returns the strategy identifier.
func (opt *MinConflictsOptimizer) Name() string {
	return "min_conflicts"
}

// TokenCandidateValues defines the discrete context search domain.
var TokenCandidateValues = []int{1000, 2000, 4000, 8000, 12000, 16000, 20000, 24000, 32000, 64000}

// RetryCandidateValues defines the discrete retry bound search domain.
var RetryCandidateValues = []int{1, 2, 3, 4, 5, 6}

// Optimize executes the Min-Conflicts local repair loop over historical turn records.
func (opt *MinConflictsOptimizer) Optimize(records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error) {
	if currentConfig == nil || len(currentConfig.Tiers) == 0 {
		return &TuningResult{
			Tiers: []TierTuningResult{},
		}, nil
	}

	if len(records) == 0 {
		var tierResults []TierTuningResult
		for _, tier := range currentConfig.Tiers {
			isLocal := IsLocalTier(tier, currentConfig.Providers)
			promptCost, compCost, compRate := ResolveModelRates(tier.Model, isLocal)
			codingIndex, toolReliability := ResolveModelBenchmark(tier.Model)
			isDisabled := strings.TrimSpace(tier.When) == "false"
			if isDisabled {
				tierResults = append(tierResults, TierTuningResult{
					TierName:                 tier.Name,
					Model:                    tier.Model,
					OriginalModel:            tier.Model,
					RecommendedModel:         tier.Model,
					CodingIndex:              codingIndex,
					ToolReliability:          toolReliability,
					PromptCostPerMillion:     promptCost,
					CompletionCostPerMillion: compCost,
					ComprehensiveRate:        compRate,
					IsDisabled:               true,
					OptimalThreshold:         0,
					OptimalRetries:           0,
					OriginalRule:             tier.When,
					SynthesizedRule:          "false",
				})
				continue
			}
			defaultThreshold := 16000
			if tier.MaxContext > 0 && tier.MaxContext < defaultThreshold {
				defaultThreshold = tier.MaxContext
			}
			rule, err := RewriteRuleAST(tier.When, defaultThreshold, 0, nil, false, false)
			if err != nil {
				rule = tier.When
			}
			tierResults = append(tierResults, TierTuningResult{
				TierName:                 tier.Name,
				Model:                    tier.Model,
				OriginalModel:            tier.Model,
				RecommendedModel:         tier.Model,
				CodingIndex:              codingIndex,
				ToolReliability:          toolReliability,
				PromptCostPerMillion:     promptCost,
				CompletionCostPerMillion: compCost,
				ComprehensiveRate:        compRate,
				OptimalThreshold:         defaultThreshold,
				OptimalRetries:           0,
				OriginalRule:             tier.When,
				SynthesizedRule:          rule,
			})
		}
		return &TuningResult{
			Tiers: tierResults,
		}, nil
	}

	trajectories := GroupBySession(records)
	if len(trajectories) == 0 {
		trajectories = []SessionTrajectory{
			{
				SessionID:  "legacy-session",
				Turns:      records,
				TotalTurns: len(records),
			},
		}
	}

	// 1. Analyze high-friction candidate keywords
	keywordAnalyzer := NewKeywordRiskAnalyzer(opt.policy)
	candidateKeywords := keywordAnalyzer.Analyze(records)
	if len(candidateKeywords) > 8 {
		candidateKeywords = candidateKeywords[:8]
	}

	// 2. Extract initial state from active config
	currentCfg := ExtractRoutingState(currentConfig, candidateKeywords)
	if len(currentCfg.Tiers) == 0 {
		return nil, fmt.Errorf("no tunable tiers found in active configuration")
	}
	initialCfg := copyMultiTierConfig(&currentCfg)
	initialDominance := AnalyzeFleetDominance(&initialCfg)

	// 3. Evaluate initial baseline
	var baselineBuf MultiTierReplayResult
	ReplayMultiTierFleet(trajectories, &currentCfg, &opt.policy, &baselineBuf)
	baselineConflict := EvaluateFleetConflict(trajectories, &currentCfg, &opt.policy, &baselineBuf)

	bestCfg := copyMultiTierConfig(&currentCfg)
	bestConflict := baselineConflict

	// 4. Tabu Memory (circular FIFO queue of 10 state hashes)
	const tabuSize = 10
	tabuQueue := make([]uint64, 0, tabuSize)
	tabuSet := make(map[uint64]bool)

	pushTabu := func(h uint64) {
		if len(tabuQueue) >= tabuSize {
			oldest := tabuQueue[0]
			tabuQueue = tabuQueue[1:]
			delete(tabuSet, oldest)
		}
		tabuQueue = append(tabuQueue, h)
		tabuSet[h] = true
	}

	// 5. Min-Conflicts Local Repair Loop
	const maxIterations = 150
	const epsilon = 0.05
	var evalBuf MultiTierReplayResult
	var consecutiveStagnations int
	lastBest := bestConflict
	// #nosec G404 - pseudo-random epsilon perturbation for heuristic search
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for iter := 0; iter < maxIterations; iter++ {
		// Evaluate conflict ledger with fractional attribution
		report := EvaluateFleetWithAttribution(trajectories, &currentCfg, &opt.policy, candidateKeywords)
		if report.TotalConflict == 0 {
			break
		}

		// Relative convergence check: (lastBest - current) / baseline < 0.001
		if baselineConflict > 0 && math.Abs(lastBest-report.TotalConflict)/baselineConflict < 0.001 {
			consecutiveStagnations++
			if consecutiveStagnations >= 10 {
				break
			}
		} else {
			consecutiveStagnations = 0
			lastBest = report.TotalConflict
		}

		// Pick variable to repair
		targetVar := report.MaxConflictVar
		if len(report.VariableConflicts) == 0 && targetVar == "" {
			break
		}

		// Epsilon-greedy perturbation to escape local optima
		if rng.Float64() < epsilon && len(report.VariableConflicts) > 0 {
			var keys []string
			for k := range report.VariableConflicts {
				keys = append(keys, k)
			}
			targetVar = keys[rng.Intn(len(keys))]
		}

		if targetVar == "" {
			break
		}

		// Greedily reassign targetVar to minimize fleet conflict
		bestValCfg := copyMultiTierConfig(&currentCfg)
		minValConflict := report.TotalConflict

		if strings.HasPrefix(targetVar, "tier_") {
			// Variable format: "tier_<idx>:<attr>"
			parts := strings.Split(targetVar, ":")
			if len(parts) == 2 {
				tierIdxStr := strings.TrimPrefix(parts[0], "tier_")
				attr := parts[1]
				tierIdx, err := strconv.Atoi(tierIdxStr)
				if err == nil && tierIdx >= 0 && tierIdx < len(currentCfg.Tiers) && !currentCfg.Tiers[tierIdx].IsDisabled {
					switch attr {
					case "tokens":
						for _, candT := range TokenCandidateValues {
							if currentCfg.Tiers[tierIdx].MaxContext > 0 && candT > currentCfg.Tiers[tierIdx].MaxContext {
								continue
							}
							candCfg := copyMultiTierConfig(&currentCfg)
							candCfg.Tiers[tierIdx].TokenThreshold = candT
							if !CheckHardConstraints(&candCfg) {
								continue
							}
							h := hashState(&candCfg)
							c := EvaluateFleetConflict(trajectories, &candCfg, &opt.policy, &evalBuf)
							if tabuSet[h] && c >= bestConflict {
								continue
							}
							if c < minValConflict {
								minValConflict = c
								bestValCfg = candCfg
							}
						}

					case "retries":
						for _, candR := range RetryCandidateValues {
							candCfg := copyMultiTierConfig(&currentCfg)
							candCfg.Tiers[tierIdx].RetryBound = candR
							h := hashState(&candCfg)
							c := EvaluateFleetConflict(trajectories, &candCfg, &opt.policy, &evalBuf)
							if tabuSet[h] && c >= bestConflict {
								continue
							}
							if c < minValConflict {
								minValConflict = c
								bestValCfg = candCfg
							}
						}

					case "tools":
						for _, candTools := range []bool{true, false} {
							candCfg := copyMultiTierConfig(&currentCfg)
							candCfg.Tiers[tierIdx].RestrictTools = candTools
							h := hashState(&candCfg)
							c := EvaluateFleetConflict(trajectories, &candCfg, &opt.policy, &evalBuf)
							if tabuSet[h] && c >= bestConflict {
								continue
							}
							if c < minValConflict {
								minValConflict = c
								bestValCfg = candCfg
							}
						}

					case "images":
						for _, candImages := range []bool{true, false} {
							candCfg := copyMultiTierConfig(&currentCfg)
							candCfg.Tiers[tierIdx].RestrictImages = candImages
							h := hashState(&candCfg)
							c := EvaluateFleetConflict(trajectories, &candCfg, &opt.policy, &evalBuf)
							if tabuSet[h] && c >= bestConflict {
								continue
							}
							if c < minValConflict {
								minValConflict = c
								bestValCfg = candCfg
							}
						}

					case "model":
						// Preserve local model unless user explicitly requests local VRAM model substitution
						if currentCfg.Tiers[tierIdx].IsLocal && opt.policy.LocalVRAMGB == 0 {
							continue
						}

						// Identify immediate escalation predecessor in the retry hierarchy
						var immediatePred *TierReplayConfig
						for j := tierIdx - 1; j >= 0; j-- {
							pred := &currentCfg.Tiers[j]
							if pred.IsDisabled || pred.RequiresKickstart || pred.RequiresImages {
								continue
							}
							isEscalation := false
							if currentCfg.Tiers[tierIdx].RetryFloor > 0 && pred.RetryBound > 0 && currentCfg.Tiers[tierIdx].RetryFloor >= pred.RetryBound {
								isEscalation = true
							} else if pred.RetryBound > 0 && currentCfg.Tiers[tierIdx].RetryBound > pred.RetryBound {
								isEscalation = true
							} else if pred.RetryBound > 0 && currentCfg.Tiers[tierIdx].RetryBound == 0 {
								isEscalation = true
							}
							if isEscalation {
								immediatePred = pred
								break
							}
						}

						isFrontier := currentCfg.Tiers[tierIdx].IsFrontier || IsFrontierModel(currentCfg.Tiers[tierIdx].Model)
						isEscalationTier := currentCfg.Tiers[tierIdx].RetryFloor > 0 || (immediatePred != nil && immediatePred.CodingIndex >= 70.0)

						tierRole := curation.RoleCodingWorkhorse
						if currentCfg.Tiers[tierIdx].RequiresImages || (currentCfg.Tiers[tierIdx].SupportsVision && !currentCfg.Tiers[tierIdx].SupportsTools) {
							tierRole = curation.RoleVisionWorkhorse
						} else if isFrontier || isEscalationTier {
							tierRole = curation.RoleGeneral
						}
						candidates := curation.DefaultManager().ParetoCandidates(
							tierRole,
							currentCfg.Tiers[tierIdx].IsLocal,
							currentCfg.Tiers[tierIdx].Model,
							opt.policy.LocalVRAMGB,
						)
						for _, cand := range candidates {
							if cand.ModelID == currentCfg.Tiers[tierIdx].Model {
								continue
							}
							// Anti-Duplication: Never duplicate immediate escalation predecessor
							if immediatePred != nil && cand.ModelID == immediatePred.Model {
								continue
							}
							// Escalation Progression: Require strictly higher capability or verified frontier class
							if isEscalationTier && immediatePred != nil {
								if !cand.IsFrontier() && cand.CodingIndex <= immediatePred.CodingIndex {
									continue
								}
							}
							// Frontier tier protection: never downgrade a frontier tier to a non-frontier or lower-benchmark model
							if isFrontier {
								if !cand.IsFrontier() && cand.CodingIndex < 80.0 {
									continue
								}
								if currentCfg.Tiers[tierIdx].CodingIndex > 0 && cand.CodingIndex < currentCfg.Tiers[tierIdx].CodingIndex {
									continue
								}
								if currentCfg.Tiers[tierIdx].ToolReliability > 0 && cand.ToolReliability < currentCfg.Tiers[tierIdx].ToolReliability {
									continue
								}
							}
							candCfg := copyMultiTierConfig(&currentCfg)
							candCfg.Tiers[tierIdx].Model = cand.ModelID
							candCfg.Tiers[tierIdx].CodingIndex = cand.CodingIndex
							candCfg.Tiers[tierIdx].ToolReliability = cand.ToolReliability
							candCfg.Tiers[tierIdx].IsFrontier = cand.IsFrontier()
							candVision, candTools := ResolveModelCapabilities(cand.ModelID)
							if cand.SupportsVision {
								candVision = true
							}
							if cand.SupportsTools {
								candTools = true
							}
							candCfg.Tiers[tierIdx].SupportsVision = candVision
							candCfg.Tiers[tierIdx].SupportsTools = candTools
							if !candCfg.Tiers[tierIdx].IsLocal {
								candCfg.Tiers[tierIdx].PromptCostPerMillion = cand.PromptCostPerMillion
								candCfg.Tiers[tierIdx].CompletionCostPerMillion = cand.CompletionCostPerMillion
								compRate := cand.PromptCostPerMillion + 0.25*cand.CompletionCostPerMillion
								candCfg.Tiers[tierIdx].ComprehensiveRate = compRate
								candCfg.Tiers[tierIdx].CostPerMillion = compRate
							} else {
								candCfg.Tiers[tierIdx].PromptCostPerMillion = 0
								candCfg.Tiers[tierIdx].CompletionCostPerMillion = 0
								candCfg.Tiers[tierIdx].ComprehensiveRate = 0
								candCfg.Tiers[tierIdx].CostPerMillion = 0
							}

							if !CheckHardConstraints(&candCfg) {
								continue
							}
							h := hashState(&candCfg)
							c := EvaluateFleetConflict(trajectories, &candCfg, &opt.policy, &evalBuf)
							if tabuSet[h] && c >= bestConflict {
								continue
							}
							if c < minValConflict {
								minValConflict = c
								bestValCfg = candCfg
							}
						}
					}
				}
			}

		} else if strings.HasPrefix(targetVar, "keyword:") {
			// Variable format: "keyword:<kw>"
			kw := strings.TrimPrefix(targetVar, "keyword:")
			// Test allocating exclusion to each tier
			for tIdx := 0; tIdx < len(currentCfg.Tiers); tIdx++ {
				candCfg := copyMultiTierConfig(&currentCfg)
				if !hasKeyword([]string{kw}, candCfg.Tiers[tIdx].ExcludedKeywords) {
					candCfg.Tiers[tIdx].ExcludedKeywords = append(candCfg.Tiers[tIdx].ExcludedKeywords, kw)
				}
				h := hashState(&candCfg)
				c := EvaluateFleetConflict(trajectories, &candCfg, &opt.policy, &evalBuf)
				if tabuSet[h] && c >= bestConflict {
					continue
				}
				if c < minValConflict {
					minValConflict = c
					bestValCfg = candCfg
				}
			}
		} else if targetVar == "default_tier:model" {
			isFrontierFallback := currentCfg.DefaultTier.IsFrontier || IsFrontierModel(currentConfig.DefaultTier.Model)

			candidates := curation.DefaultManager().ParetoCandidates(
				curation.RoleGeneral,
				currentCfg.DefaultTier.IsLocal,
				currentCfg.DefaultTier.Model,
				opt.policy.LocalVRAMGB,
			)
			for _, cand := range candidates {
				if cand.ModelID == currentCfg.DefaultTier.Model {
					continue
				}
				// Frontier Safety Net Protection:
				// Fallback catch-all tier must not be downgraded from Frontier to Budget Workhorse.
				if isFrontierFallback {
					if !cand.IsFrontier() && cand.CodingIndex < 80.0 {
						continue
					}
					// Must not degrade coding capability or tool reliability on frontier fallback
					if currentCfg.DefaultTier.CodingIndex > 0 && cand.CodingIndex < currentCfg.DefaultTier.CodingIndex {
						continue
					}
					if currentCfg.DefaultTier.ToolReliability > 0 && cand.ToolReliability < currentCfg.DefaultTier.ToolReliability {
						continue
					}
				} else {
					// Workhorse fallback: enforce tool capability and baseline tool reliability
					if !cand.SupportsTools || cand.ToolReliability < 30.0 {
						continue
					}
				}
				candCfg := copyMultiTierConfig(&currentCfg)
				candCfg.DefaultTier.Model = cand.ModelID
				candCfg.DefaultTier.CodingIndex = cand.CodingIndex
				candCfg.DefaultTier.ToolReliability = cand.ToolReliability
				candCfg.DefaultTier.IsFrontier = cand.IsFrontier()
				candVision, candTools := ResolveModelCapabilities(cand.ModelID)
				if cand.SupportsVision {
					candVision = true
				}
				if cand.SupportsTools {
					candTools = true
				}
				candCfg.DefaultTier.SupportsVision = candVision
				candCfg.DefaultTier.SupportsTools = candTools
				if !candCfg.DefaultTier.IsLocal {
					candCfg.DefaultTier.PromptCostPerMillion = cand.PromptCostPerMillion
					candCfg.DefaultTier.CompletionCostPerMillion = cand.CompletionCostPerMillion
					compRate := cand.PromptCostPerMillion + 0.25*cand.CompletionCostPerMillion
					candCfg.DefaultTier.ComprehensiveRate = compRate
					candCfg.DefaultTier.CostPerMillion = compRate
				} else {
					candCfg.DefaultTier.PromptCostPerMillion = 0
					candCfg.DefaultTier.CompletionCostPerMillion = 0
					candCfg.DefaultTier.ComprehensiveRate = 0
					candCfg.DefaultTier.CostPerMillion = 0
				}

				if !CheckHardConstraints(&candCfg) {
					continue
				}
				h := hashState(&candCfg)
				c := EvaluateFleetConflict(trajectories, &candCfg, &opt.policy, &evalBuf)
				if tabuSet[h] && c >= bestConflict {
					continue
				}
				if c < minValConflict {
					minValConflict = c
					bestValCfg = candCfg
				}
			}
		}

		currentCfg = bestValCfg
		pushTabu(hashState(&currentCfg))

		if minValConflict < bestConflict {
			bestConflict = minValConflict
			bestCfg = copyMultiTierConfig(&currentCfg)
		}
	}

	// 6. Synthesize final multi-tier rules and impact metrics
	var finalBuf MultiTierReplayResult
	ReplayMultiTierFleet(trajectories, &bestCfg, &opt.policy, &finalBuf)

	var tierResults []TierTuningResult
	for i, tier := range bestCfg.Tiers {
		var origWhen, origModel string
		if i < len(currentConfig.Tiers) {
			origWhen = currentConfig.Tiers[i].When
			origModel = currentConfig.Tiers[i].Model
		}

		isDisabled := tier.IsDisabled || strings.TrimSpace(origWhen) == "false"
		if clean := strings.TrimSpace(origWhen); clean != "" && clean != "false" {
			if _, err := parser.Parse(clean); err != nil {
				return nil, fmt.Errorf("failed to rewrite AST for tier %q: %w", tier.TierName, err)
			}
		}

		var routingUnchanged bool
		if i < len(initialCfg.Tiers) {
			origTier := initialCfg.Tiers[i]
			routingUnchanged = (tier.TokenThreshold == origTier.TokenThreshold &&
				tier.RetryBound == origTier.RetryBound &&
				tier.RestrictImages == origTier.RestrictImages &&
				tier.RestrictTools == origTier.RestrictTools &&
				equalStringSlices(tier.ExcludedKeywords, origTier.ExcludedKeywords))
		}

		var synthRule string
		if isDisabled {
			synthRule = "false"
			tier.TokenThreshold = 0
			tier.RetryBound = 0
		} else if routingUnchanged {
			synthRule = origWhen
		} else if i < len(finalBuf.TierStats) && finalBuf.TierStats[i].TurnsRouted == 0 {
			// Zero traffic reached this tier during replay -> preserve original rule strictly without dummy threshold additions!
			synthRule = origWhen
		} else {
			var err error
			synthRule, err = RewriteRuleAST(
				origWhen,
				tier.TokenThreshold,
				tier.RetryBound,
				tier.ExcludedKeywords,
				tier.RestrictImages,
				tier.RestrictTools,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to rewrite AST for tier %q: %w", tier.TierName, err)
			}
		}

		recModel := tier.Model
		var modelBenefit string
		if recModel != "" && recModel != origModel {
			origCoding, _ := ResolveModelBenchmark(origModel)
			origPrompt, origComp, _ := ResolveModelRates(origModel, tier.IsLocal)
			if tier.CodingIndex > origCoding {
				modelBenefit = fmt.Sprintf("Improves coding benchmark from %.1f to %.1f", origCoding, tier.CodingIndex)
			} else if !tier.IsLocal && (tier.PromptCostPerMillion < origPrompt || tier.CompletionCostPerMillion < origComp) {
				modelBenefit = fmt.Sprintf("Reduces cloud token pricing ($%.2f/M prompt vs $%.2f/M)", tier.PromptCostPerMillion, origPrompt)
			} else {
				modelBenefit = "Optimizes fleet cost-to-performance frontier"
			}
		}

		tierResults = append(tierResults, TierTuningResult{
			TierName:                 tier.TierName,
			Model:                    recModel,
			OriginalModel:            origModel,
			RecommendedModel:         recModel,
			ModelBenefit:             modelBenefit,
			CodingIndex:              tier.CodingIndex,
			ToolReliability:          tier.ToolReliability,
			PromptCostPerMillion:     tier.PromptCostPerMillion,
			CompletionCostPerMillion: tier.CompletionCostPerMillion,
			ComprehensiveRate:        tier.ComprehensiveRate,
			IsDisabled:               isDisabled,
			OptimalThreshold:         tier.TokenThreshold,
			OptimalRetries:           tier.RetryBound,
			FrictionKeywords:         tier.ExcludedKeywords,
			RestrictImages:           tier.RestrictImages,
			RestrictTools:            tier.RestrictTools,
			OriginalRule:             origWhen,
			SynthesizedRule:          synthRule,
		})
	}

	var defaultTierResult *TierTuningResult
	origDefModel := currentConfig.DefaultTier.Model
	recDefModel := bestCfg.DefaultTier.Model
	if recDefModel != "" && recDefModel != origDefModel {
		origDefCoding, _ := ResolveModelBenchmark(origDefModel)
		var defBenefit string
		if bestCfg.DefaultTier.CodingIndex > origDefCoding {
			defBenefit = fmt.Sprintf("Improves fallback benchmark from %.1f to %.1f", origDefCoding, bestCfg.DefaultTier.CodingIndex)
		} else {
			defBenefit = "Reduces fallback cloud cost while preserving reasoning capability"
		}
		defaultTierResult = &TierTuningResult{
			TierName:                 bestCfg.DefaultTier.TierName,
			Model:                    recDefModel,
			OriginalModel:            origDefModel,
			RecommendedModel:         recDefModel,
			ModelBenefit:             defBenefit,
			CodingIndex:              bestCfg.DefaultTier.CodingIndex,
			ToolReliability:          bestCfg.DefaultTier.ToolReliability,
			PromptCostPerMillion:     bestCfg.DefaultTier.PromptCostPerMillion,
			CompletionCostPerMillion: bestCfg.DefaultTier.CompletionCostPerMillion,
			ComprehensiveRate:        bestCfg.DefaultTier.ComprehensiveRate,
		}
	}

	retriesAvoided := baselineBuf.TotalWastedRetries - finalBuf.TotalWastedRetries
	if retriesAvoided < 0 {
		retriesAvoided = 0
	}

	savingsUSD := baselineBuf.TotalCostUSD - finalBuf.TotalCostUSD
	if savingsUSD < 0 {
		savingsUSD = 0
	}

	// Invariant: If optimization achieves zero savings and zero retries avoided,
	// and no static fleet dominance conflicts were resolved,
	// the active configuration is already optimal. Preserve original rules and models for all tiers!
	hasDominanceResolution := len(AnalyzeFleetDominance(&initialCfg)) > len(AnalyzeFleetDominance(&bestCfg))
	if retriesAvoided <= 0 && savingsUSD <= 0.001 && !hasDominanceResolution {
		defaultTierResult = nil
		for i := range tierResults {
			if !tierResults[i].IsDisabled {
				tierResults[i].SynthesizedRule = tierResults[i].OriginalRule
			}
			tierResults[i].RecommendedModel = tierResults[i].OriginalModel
			tierResults[i].ModelBenefit = ""
		}
	}

	escalationRate := 0.0
	if len(trajectories) > 0 {
		escalationRate = float64(finalBuf.EscalatedSessions) / float64(len(trajectories))
	}

	avgTurns := 0.0
	if len(trajectories) > 0 {
		avgTurns = float64(finalBuf.TotalTurns) / float64(len(trajectories))
	}

	return &TuningResult{
		Tiers:                    tierResults,
		DefaultTier:              defaultTierResult,
		CurrentCostUSD:           baselineBuf.TotalCostUSD,
		ProjectedCostUSD:         finalBuf.TotalCostUSD,
		ProjectedSavingsUSD:      savingsUSD,
		RetriesEliminated:        retriesAvoided,
		TotalSampleTurns:         finalBuf.TotalTurns,
		TotalSessions:            len(trajectories),
		AvgTurnsPerSession:       avgTurns,
		EscalationRate:           escalationRate,
		StaticDominanceConflicts: initialDominance,
	}, nil
}

// OptimizeWithContext executes the optimization algorithm respecting context cancellation/timeout.
func (opt *MinConflictsOptimizer) OptimizeWithContext(ctx context.Context, records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error) {
	type resultPair struct {
		res *TuningResult
		err error
	}
	done := make(chan resultPair, 1)

	go func() {
		res, err := opt.Optimize(records, currentConfig)
		done <- resultPair{res: res, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.res, r.err
	}
}

// copyMultiTierConfig creates an isolated deep copy of a candidate configuration.
func copyMultiTierConfig(src *MultiTierConfig) MultiTierConfig {
	if src == nil {
		return MultiTierConfig{}
	}
	dst := MultiTierConfig{
		DefaultTier: src.DefaultTier,
	}
	for _, t := range src.Tiers {
		tCopy := t
		if len(t.ExcludedKeywords) > 0 {
			tCopy.ExcludedKeywords = make([]string, len(t.ExcludedKeywords))
			copy(tCopy.ExcludedKeywords, t.ExcludedKeywords)
		}
		dst.Tiers = append(dst.Tiers, tCopy)
	}
	return dst
}

// hashState computes a lightweight 64-bit fingerprint of a candidate state for Tabu lookup.
func hashState(cfg *MultiTierConfig) uint64 {
	var h uint64 = 14695981039346656037
	const prime uint64 = 1099511628211

	for _, t := range cfg.Tiers {
		// #nosec G115 - TokenThreshold and RetryBound are positive configuration limits
		h = (h ^ uint64(t.TokenThreshold)) * prime
		// #nosec G115 - TokenThreshold and RetryBound are positive configuration limits
		h = (h ^ uint64(t.RetryBound)) * prime
		if t.RestrictImages {
			h = (h ^ 0x01) * prime
		}
		if t.RestrictTools {
			h = (h ^ 0x02) * prime
		}
		for i := 0; i < len(t.Model); i++ {
			h = (h ^ uint64(t.Model[i])) * prime
		}
		for _, kw := range t.ExcludedKeywords {
			for i := 0; i < len(kw); i++ {
				h = (h ^ uint64(kw[i])) * prime
			}
		}
	}
	for i := 0; i < len(cfg.DefaultTier.Model); i++ {
		h = (h ^ uint64(cfg.DefaultTier.Model[i])) * prime
	}
	return h
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
