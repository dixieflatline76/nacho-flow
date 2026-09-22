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
var TokenCandidateValues = []int{1000, 2000, 4000, 8000, 12000, 16000, 24000, 32000, 64000}

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
		var origWhen string
		if i < len(currentConfig.Tiers) {
			origWhen = currentConfig.Tiers[i].When
		}

		isDisabled := tier.IsDisabled || strings.TrimSpace(origWhen) == "false"

		var synthRule string
		if isDisabled {
			synthRule = "false"
			tier.TokenThreshold = 0
			tier.RetryBound = 0
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

		tierResults = append(tierResults, TierTuningResult{
			TierName:                 tier.TierName,
			Model:                    tier.Model,
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

	retriesAvoided := baselineBuf.TotalWastedRetries - finalBuf.TotalWastedRetries
	if retriesAvoided < 0 {
		retriesAvoided = 0
	}

	savingsUSD := baselineBuf.TotalCostUSD - finalBuf.TotalCostUSD
	if savingsUSD < 0 {
		savingsUSD = 0
	}

	// Invariant: If optimization achieves zero savings and zero retries avoided,
	// the active configuration is already optimal. Preserve original rules for all tiers!
	if retriesAvoided <= 0 && savingsUSD <= 0.001 {
		for i := range tierResults {
			if !tierResults[i].IsDisabled {
				tierResults[i].SynthesizedRule = tierResults[i].OriginalRule
			}
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
		Tiers:               tierResults,
		CurrentCostUSD:      baselineBuf.TotalCostUSD,
		ProjectedCostUSD:    finalBuf.TotalCostUSD,
		ProjectedSavingsUSD: savingsUSD,
		RetriesEliminated:   retriesAvoided,
		TotalSampleTurns:    finalBuf.TotalTurns,
		TotalSessions:       len(trajectories),
		AvgTurnsPerSession:  avgTurns,
		EscalationRate:      escalationRate,
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
		h = (h ^ uint64(t.TokenThreshold)) * prime
		h = (h ^ uint64(t.RetryBound)) * prime
		if t.RestrictImages {
			h = (h ^ 0x01) * prime
		}
		if t.RestrictTools {
			h = (h ^ 0x02) * prime
		}
		for _, kw := range t.ExcludedKeywords {
			for i := 0; i < len(kw); i++ {
				h = (h ^ uint64(kw[i])) * prime
			}
		}
	}
	return h
}
