package tuner

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
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

// TabuMemory implements a circular FIFO tabu queue and set lookup.
type TabuMemory struct {
	queue []uint64
	set   map[uint64]bool
	size  int
}

// NewTabuMemory creates a tabu memory buffer of the specified capacity.
func NewTabuMemory(size int) *TabuMemory {
	return &TabuMemory{
		queue: make([]uint64, 0, size),
		set:   make(map[uint64]bool, size),
		size:  size,
	}
}

// Push adds a state hash, evicting the oldest element if at capacity.
func (t *TabuMemory) Push(h uint64) {
	if len(t.queue) >= t.size {
		oldest := t.queue[0]
		t.queue = t.queue[1:]
		delete(t.set, oldest)
	}
	t.queue = append(t.queue, h)
	t.set[h] = true
}

// Contains returns true if the state hash is in the tabu set.
func (t *TabuMemory) Contains(h uint64) bool {
	return t.set[h]
}

// CandidateConfigBuilder provides a fluent builder for generating speculative MultiTierConfig mutations.
type CandidateConfigBuilder struct {
	cfg MultiTierConfig
}

// NewCandidateBuilder initializes a builder starting from a base configuration deep copy.
func NewCandidateBuilder(base *MultiTierConfig) *CandidateConfigBuilder {
	return &CandidateConfigBuilder{
		cfg: copyMultiTierConfig(base),
	}
}

// WithTokenThreshold updates a tier's token threshold.
func (b *CandidateConfigBuilder) WithTokenThreshold(tierIdx, val int) *CandidateConfigBuilder {
	b.cfg.Tiers[tierIdx].TokenThreshold = val
	return b
}

// WithRetryBound updates a tier's retry bound limit.
func (b *CandidateConfigBuilder) WithRetryBound(tierIdx, val int) *CandidateConfigBuilder {
	b.cfg.Tiers[tierIdx].RetryBound = val
	return b
}

// WithToolRestriction updates a tier's tool restriction gate.
func (b *CandidateConfigBuilder) WithToolRestriction(tierIdx int, restrict bool) *CandidateConfigBuilder {
	b.cfg.Tiers[tierIdx].RestrictTools = restrict
	return b
}

// WithImageRestriction updates a tier's vision/image restriction gate.
func (b *CandidateConfigBuilder) WithImageRestriction(tierIdx int, restrict bool) *CandidateConfigBuilder {
	b.cfg.Tiers[tierIdx].RestrictImages = restrict
	return b
}

// WithPrunedTier disables an intermediate escalation tier.
func (b *CandidateConfigBuilder) WithPrunedTier(tierIdx int) *CandidateConfigBuilder {
	b.cfg.Tiers[tierIdx].IsDisabled = true
	b.cfg.Tiers[tierIdx].Model = ""
	return b
}

// WithCandidateModel substitutes a tier's model with an evaluated CandidateModel.
func (b *CandidateConfigBuilder) WithCandidateModel(tierIdx int, cand CandidateModel) *CandidateConfigBuilder {
	tier := &b.cfg.Tiers[tierIdx]
	tier.Model = cand.ModelID
	tier.CodingIndex = cand.CodingIndex
	tier.ToolReliability = cand.ToolReliability
	tier.IsFrontier = cand.IsFrontier()

	candVision, candTools := ResolveModelCapabilities(cand.ModelID)
	if cand.SupportsVision {
		candVision = true
	}
	if cand.SupportsTools {
		candTools = true
	}
	tier.SupportsVision = candVision
	tier.SupportsTools = candTools

	if !tier.IsLocal {
		tier.PromptCostPerMillion = cand.PromptCostPerMillion
		tier.CompletionCostPerMillion = cand.CompletionCostPerMillion
		compRate := cand.ComprehensiveRate
		if compRate <= 0 {
			compRate = cand.PromptCostPerMillion + 0.25*cand.CompletionCostPerMillion
		}
		tier.ComprehensiveRate = compRate
		tier.CostPerMillion = compRate
	} else {
		tier.PromptCostPerMillion = 0
		tier.CompletionCostPerMillion = 0
		tier.ComprehensiveRate = 0
		tier.CostPerMillion = 0
	}
	return b
}

// WithModelCandidate substitutes a tier's model with a curated candidate.
func (b *CandidateConfigBuilder) WithModelCandidate(tierIdx int, cand curation.ModelCuratedProfile) *CandidateConfigBuilder {
	return b.WithCandidateModel(tierIdx, ConvertProfileToCandidate(cand))
}

// WithDefaultTierCandidateModel substitutes the fallback default tier's model with an evaluated CandidateModel.
func (b *CandidateConfigBuilder) WithDefaultTierCandidateModel(cand CandidateModel) *CandidateConfigBuilder {
	dt := &b.cfg.DefaultTier
	dt.Model = cand.ModelID
	dt.CodingIndex = cand.CodingIndex
	dt.ToolReliability = cand.ToolReliability
	dt.IsFrontier = cand.IsFrontier()

	candVision, candTools := ResolveModelCapabilities(cand.ModelID)
	if cand.SupportsVision {
		candVision = true
	}
	if cand.SupportsTools {
		candTools = true
	}
	dt.SupportsVision = candVision
	dt.SupportsTools = candTools

	if !dt.IsLocal {
		dt.PromptCostPerMillion = cand.PromptCostPerMillion
		dt.CompletionCostPerMillion = cand.CompletionCostPerMillion
		compRate := cand.ComprehensiveRate
		if compRate <= 0 {
			compRate = cand.PromptCostPerMillion + 0.25*cand.CompletionCostPerMillion
		}
		dt.ComprehensiveRate = compRate
		dt.CostPerMillion = compRate
	} else {
		dt.PromptCostPerMillion = 0
		dt.CompletionCostPerMillion = 0
		dt.ComprehensiveRate = 0
		dt.CostPerMillion = 0
	}
	return b
}

// WithDefaultTierModel substitutes the fallback default tier's model.
func (b *CandidateConfigBuilder) WithDefaultTierModel(cand curation.ModelCuratedProfile) *CandidateConfigBuilder {
	return b.WithDefaultTierCandidateModel(ConvertProfileToCandidate(cand))
}

// WithExcludedKeyword adds a keyword exclusion to the tier.
func (b *CandidateConfigBuilder) WithExcludedKeyword(tierIdx int, kw string) *CandidateConfigBuilder {
	if !hasKeyword([]string{kw}, b.cfg.Tiers[tierIdx].ExcludedKeywords) {
		b.cfg.Tiers[tierIdx].ExcludedKeywords = append(b.cfg.Tiers[tierIdx].ExcludedKeywords, kw)
	}
	return b
}

// Build finalizes the candidate configuration copy.
func (b *CandidateConfigBuilder) Build() MultiTierConfig {
	return b.cfg
}

// repairContext holds intermediate state and buffers during local search repair moves.
type repairContext struct {
	opt            *MinConflictsOptimizer
	trajectories   []SessionTrajectory
	bestConflict   float64
	minValConflict float64
	bestValCfg     MultiTierConfig
	tabu           *TabuMemory
	evalBuf        *MultiTierReplayResult
}

func (rc *repairContext) evalCandidate(candCfg *MultiTierConfig) {
	if !CheckHardConstraints(candCfg) {
		return
	}
	h := hashState(candCfg)
	c := EvaluateFleetConflict(rc.trajectories, candCfg, &rc.opt.policy, rc.evalBuf)
	if rc.tabu.Contains(h) && c >= rc.bestConflict {
		return
	}
	if c < rc.minValConflict {
		rc.minValConflict = c
		rc.bestValCfg = *candCfg
	}
}

// Optimize executes the Min-Conflicts local repair loop over historical turn records.
func (opt *MinConflictsOptimizer) Optimize(records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error) {
	if currentConfig == nil || len(currentConfig.Tiers) == 0 {
		return &TuningResult{
			Tiers: []TierTuningResult{},
		}, nil
	}

	if len(records) == 0 {
		return opt.handleEmptyRecords(currentConfig), nil
	}

	// if currentConfig.Kickstart.WriteOnly || currentConfig.CycleKiller.KickstartWriteOnly || currentConfig.CycleBreaker.KickstartWriteOnly {
	// 	opt.policy.WriteOnly = true
	// }

	// 1. Group records into session trajectories
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
	tabu := NewTabuMemory(tabuSize)

	// 5. Min-Conflicts Local Repair Loop
	const maxIterations = 150
	const epsilon = 0.05
	var evalBuf MultiTierReplayResult
	var consecutiveStagnations int
	lastBest := bestConflict
	// #nosec G404 - pseudo-random epsilon perturbation for heuristic search
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	rc := repairContext{
		opt:          opt,
		trajectories: trajectories,
		tabu:         tabu,
		evalBuf:      &evalBuf,
	}

	for iter := 0; iter < maxIterations; iter++ {
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

		// Collect candidate variables sorted by conflict descending
		type varConflict struct {
			name     string
			conflict float64
		}
		var candidateVars []varConflict
		for k, v := range report.VariableConflicts {
			if v > 0 {
				candidateVars = append(candidateVars, varConflict{name: k, conflict: v})
			}
		}
		if len(candidateVars) == 0 && report.MaxConflictVar != "" {
			candidateVars = append(candidateVars, varConflict{name: report.MaxConflictVar, conflict: report.TotalConflict})
		}
		if len(candidateVars) == 0 {
			break
		}

		sort.Slice(candidateVars, func(i, j int) bool {
			if candidateVars[i].conflict == candidateVars[j].conflict {
				return candidateVars[i].name < candidateVars[j].name
			}
			return candidateVars[i].conflict > candidateVars[j].conflict
		})

		// Epsilon-greedy perturbation to escape local optima
		if rng.Float64() < epsilon && len(candidateVars) > 1 {
			randIdx := 1 + rng.Intn(len(candidateVars)-1)
			candidateVars[0], candidateVars[randIdx] = candidateVars[randIdx], candidateVars[0]
		}

		// Greedily reassign targetVar to minimize fleet conflict
		rc.bestConflict = bestConflict
		rc.minValConflict = report.TotalConflict
		rc.bestValCfg = copyMultiTierConfig(&currentCfg)

		for _, cv := range candidateVars {
			targetVar := cv.name

			if strings.HasPrefix(targetVar, "tier_") {
				parts := strings.Split(targetVar, ":")
				if len(parts) == 2 {
					tierIdxStr := strings.TrimPrefix(parts[0], "tier_")
					attr := parts[1]
					tierIdx, err := strconv.Atoi(tierIdxStr)
					if err == nil && tierIdx >= 0 && tierIdx < len(currentCfg.Tiers) && !currentCfg.Tiers[tierIdx].IsDisabled {
						switch attr {
						case "tokens":
							opt.repairTokens(&currentCfg, tierIdx, &rc)
						case "retries":
							opt.repairRetries(&currentCfg, tierIdx, &rc)
						case "tools", "images":
							opt.repairModalityGates(&currentCfg, tierIdx, attr, &rc)
						case "model":
							opt.repairModelOrPrune(&currentCfg, tierIdx, &rc)
						}
					}
				}
			} else if strings.HasPrefix(targetVar, "keyword:") {
				kw := strings.TrimPrefix(targetVar, "keyword:")
				opt.repairKeyword(&currentCfg, kw, &rc)
			} else if targetVar == "default_tier:model" {
				opt.repairDefaultTierModel(&currentCfg, currentConfig, &rc)
			}

			if rc.minValConflict < report.TotalConflict {
				break
			}
		}

		currentCfg = rc.bestValCfg
		tabu.Push(hashState(&currentCfg))

		if rc.minValConflict < bestConflict {
			bestConflict = rc.minValConflict
			bestCfg = copyMultiTierConfig(&currentCfg)
		}
	}

	// 5b. Post-CSP model substitution sweep for active cloud tiers.
	opt.sweepModelReplacements(trajectories, &bestCfg, &evalBuf, &bestConflict)

	// 6. Synthesize final multi-tier rules and impact metrics
	var finalBuf MultiTierReplayResult
	ReplayMultiTierFleet(trajectories, &bestCfg, &opt.policy, &finalBuf)

	return opt.synthesizeTuningResult(currentConfig, initialCfg, bestCfg, baselineBuf, finalBuf, trajectories, initialDominance)
}

// sweepModelReplacements performs a post-CSP pass over active cloud tiers with spend,
// evaluating qualifying replacement candidates via CandidateFilterPipeline and CompositeValueScorer.
func (opt *MinConflictsOptimizer) sweepModelReplacements(
	trajectories []SessionTrajectory,
	bestCfg *MultiTierConfig,
	evalBuf *MultiTierReplayResult,
	bestConflict *float64,
) {
	ReplayMultiTierFleet(trajectories, bestCfg, &opt.policy, evalBuf)
	for i := 0; i < len(bestCfg.Tiers); i++ {
		t := &bestCfg.Tiers[i]
		if t.IsDisabled || t.IsLocal || t.Model == "" || t.IsFrontier {
			continue
		}
		minSpend := DefaultMinCloudSpendUSD
		if opt.policy.MinCloudSpendUSD > 0 {
			minSpend = opt.policy.MinCloudSpendUSD
		}
		if i < len(evalBuf.TierStats) && (evalBuf.TierStats[i].TurnsRouted == 0 || evalBuf.TierStats[i].CostUSD < minSpend) {
			continue
		}

		var immediatePred *TierReplayConfig
		for j := i - 1; j >= 0; j-- {
			if !bestCfg.Tiers[j].IsDisabled {
				immediatePred = &bestCfg.Tiers[j]
				break
			}
		}
		var immediateSucc *TierReplayConfig
		for j := i + 1; j < len(bestCfg.Tiers); j++ {
			if !bestCfg.Tiers[j].IsDisabled {
				immediateSucc = &bestCfg.Tiers[j]
				break
			}
		}

		tierRole := curation.RoleCodingWorkhorse
		if t.RequiresImages || (t.SupportsVision && !t.SupportsTools) {
			tierRole = curation.RoleVisionWorkhorse
		} else if t.IsFrontier {
			tierRole = curation.RoleGeneral
		}

		candidates := CollectCandidateModels(tierRole, t.IsLocal, t.Model, opt.policy.LocalVRAMGB, opt.policy.CandidateDeals)
		pipeline := NewDefaultFilterPipeline()
		scorer := &CompositeValueScorer{}

		var bestCand *CandidateModel
		var bestScore float64

		for _, cand := range candidates {
			fCtx := &FilterContext{
				TargetTier:    t,
				Candidate:     &cand,
				ImmediatePred: immediatePred,
				ImmediateSucc: immediateSucc,
				Policy:        &opt.policy,
			}
			if passed, _ := pipeline.Evaluate(fCtx); !passed {
				continue
			}
			s := scorer.Score(t, &cand)
			if bestCand == nil || s > bestScore {
				candCopy := cand
				bestCand = &candCopy
				bestScore = s
			}
		}

		if bestCand != nil {
			candCfg := NewCandidateBuilder(bestCfg).
				WithCandidateModel(i, *bestCand).
				Build()
			if CheckHardConstraints(&candCfg) {
				c := EvaluateFleetConflict(trajectories, &candCfg, &opt.policy, evalBuf)
				if c <= *bestConflict {
					*bestConflict = c
					*bestCfg = candCfg
				}
			}
		}
	}
}

// repairTokens explores candidate context thresholds for a tier.
func (opt *MinConflictsOptimizer) repairTokens(currentCfg *MultiTierConfig, tierIdx int, rc *repairContext) {
	for _, candT := range TokenCandidateValues {
		if currentCfg.Tiers[tierIdx].MaxContext > 0 && candT > currentCfg.Tiers[tierIdx].MaxContext {
			continue
		}
		candCfg := NewCandidateBuilder(currentCfg).
			WithTokenThreshold(tierIdx, candT).
			Build()
		rc.evalCandidate(&candCfg)
	}
}

// repairRetries explores candidate retry bounds for a tier.
func (opt *MinConflictsOptimizer) repairRetries(currentCfg *MultiTierConfig, tierIdx int, rc *repairContext) {
	for _, candR := range RetryCandidateValues {
		candCfg := NewCandidateBuilder(currentCfg).
			WithRetryBound(tierIdx, candR).
			Build()
		rc.evalCandidate(&candCfg)
	}
}

// repairModalityGates explores restricting tools or images on a tier.
func (opt *MinConflictsOptimizer) repairModalityGates(currentCfg *MultiTierConfig, tierIdx int, attr string, rc *repairContext) {
	for _, val := range []bool{true, false} {
		builder := NewCandidateBuilder(currentCfg)
		if attr == "tools" {
			builder.WithToolRestriction(tierIdx, val)
		} else {
			builder.WithImageRestriction(tierIdx, val)
		}
		candCfg := builder.Build()
		rc.evalCandidate(&candCfg)
	}
}

// repairModelOrPrune explores autonomous tier pruning and Pareto candidate model substitutions.
func (opt *MinConflictsOptimizer) repairModelOrPrune(currentCfg *MultiTierConfig, tierIdx int, rc *repairContext) {
	// Preserve local model unless user explicitly requests local VRAM model substitution
	if currentCfg.Tiers[tierIdx].IsLocal && opt.policy.LocalVRAMGB == 0 {
		return
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

	// Autonomous Tier Pruning Candidate:
	// Try disabling this intermediate escalation tier entirely.
	// Whichever produces lower total fleet conflict C(X) wins.
	if isEscalationTier && tierIdx > 0 && tierIdx < len(currentCfg.Tiers)-1 {
		candCfg := NewCandidateBuilder(currentCfg).
			WithPrunedTier(tierIdx).
			Build()
		if CheckHardConstraints(&candCfg) {
			h := hashState(&candCfg)
			c := EvaluateFleetConflict(rc.trajectories, &candCfg, &rc.opt.policy, rc.evalBuf)
			if !(rc.tabu.Contains(h) && c >= rc.bestConflict) && c < rc.minValConflict {
				rc.minValConflict = c
				rc.bestValCfg = candCfg
			}
		}
	}

	tierRole := curation.RoleCodingWorkhorse
	if currentCfg.Tiers[tierIdx].RequiresImages || (currentCfg.Tiers[tierIdx].SupportsVision && !currentCfg.Tiers[tierIdx].SupportsTools) {
		tierRole = curation.RoleVisionWorkhorse
	} else if isFrontier || isEscalationTier {
		tierRole = curation.RoleGeneral
	}
	candidates := CollectCandidateModels(
		tierRole,
		currentCfg.Tiers[tierIdx].IsLocal,
		currentCfg.Tiers[tierIdx].Model,
		opt.policy.LocalVRAMGB,
		opt.policy.CandidateDeals,
	)

	pipeline := NewDefaultFilterPipeline()
	scorer := &CompositeValueScorer{}

	var immediateSucc *TierReplayConfig
	for j := tierIdx + 1; j < len(currentCfg.Tiers); j++ {
		if !currentCfg.Tiers[j].IsDisabled {
			immediateSucc = &currentCfg.Tiers[j]
			break
		}
	}

	for _, cand := range candidates {
		if cand.ModelID == currentCfg.Tiers[tierIdx].Model {
			continue
		}
		// Anti-Duplication: Never duplicate immediate escalation predecessor
		if immediatePred != nil && cand.ModelID == immediatePred.Model {
			continue
		}

		// Evaluate candidate against specification filter pipeline
		fCtx := &FilterContext{
			TargetTier:    &currentCfg.Tiers[tierIdx],
			Candidate:     &cand,
			ImmediatePred: immediatePred,
			ImmediateSucc: immediateSucc,
			Policy:        &opt.policy,
		}
		if passed, _ := pipeline.Evaluate(fCtx); !passed {
			// For escalation tiers resolving capability failures, allow benchmark leap if frontier or meets escalation gain
			minGain := opt.policy.MinEscalationGainPct
			if minGain <= 0 {
				minGain = 0.05
			}
			isEscalationUpgrade := isEscalationTier && immediatePred != nil &&
				(cand.IsFrontier() || cand.CodingIndex >= immediatePred.CodingIndex*(1.0+minGain))
			if !isEscalationUpgrade {
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

		candCfg := NewCandidateBuilder(currentCfg).
			WithCandidateModel(tierIdx, cand).
			Build()

		if !CheckHardConstraints(&candCfg) {
			continue
		}
		h := hashState(&candCfg)
		c := EvaluateFleetConflict(rc.trajectories, &candCfg, &rc.opt.policy, rc.evalBuf)
		if rc.tabu.Contains(h) && c >= rc.bestConflict {
			continue
		}
		if c < rc.minValConflict {
			rc.minValConflict = c
			rc.bestValCfg = candCfg
		} else if math.Abs(c-rc.minValConflict) < 0.01 && !candCfg.Tiers[tierIdx].IsDisabled && !rc.bestValCfg.Tiers[tierIdx].IsDisabled {
			// Near-identical conflict tiebreaker: rank by CompositeValueScorer
			candScore := scorer.Score(&currentCfg.Tiers[tierIdx], &cand)
			bestCand := ConvertTierToCandidate(&rc.bestValCfg.Tiers[tierIdx])
			bestScore := scorer.Score(&currentCfg.Tiers[tierIdx], &bestCand)
			if candScore > bestScore {
				rc.minValConflict = c
				rc.bestValCfg = candCfg
			}
		}
	}
}

// repairKeyword explores assigning domain keywords to individual tiers.
func (opt *MinConflictsOptimizer) repairKeyword(currentCfg *MultiTierConfig, kw string, rc *repairContext) {
	for tIdx := 0; tIdx < len(currentCfg.Tiers); tIdx++ {
		candCfg := NewCandidateBuilder(currentCfg).
			WithExcludedKeyword(tIdx, kw).
			Build()
		rc.evalCandidate(&candCfg)
	}
}

// repairDefaultTierModel explores Pareto candidate substitutions on the fallback tier.
func (opt *MinConflictsOptimizer) repairDefaultTierModel(currentCfg *MultiTierConfig, currentConfig *contract.Config, rc *repairContext) {
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
		if isFrontierFallback {
			if !cand.IsFrontier() && cand.CodingIndex < 80.0 {
				continue
			}
			if currentCfg.DefaultTier.CodingIndex > 0 && cand.CodingIndex < currentCfg.DefaultTier.CodingIndex {
				continue
			}
			if currentCfg.DefaultTier.ToolReliability > 0 && cand.ToolReliability < currentCfg.DefaultTier.ToolReliability {
				continue
			}
		} else {
			if !cand.SupportsTools || cand.ToolReliability < 30.0 {
				continue
			}
		}

		candCfg := NewCandidateBuilder(currentCfg).
			WithDefaultTierModel(cand).
			Build()
		rc.evalCandidate(&candCfg)
	}
}

// handleEmptyRecords builds a default pass-through TuningResult when no historical telemetry is available.
func (opt *MinConflictsOptimizer) handleEmptyRecords(currentConfig *contract.Config) *TuningResult {
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
	}
}

// synthesizeTuningResult transforms the winning CSP configuration into human-readable TuningResults and AST rules.
func (opt *MinConflictsOptimizer) synthesizeTuningResult(
	currentConfig *contract.Config,
	initialCfg MultiTierConfig,
	bestCfg MultiTierConfig,
	baselineBuf MultiTierReplayResult,
	finalBuf MultiTierReplayResult,
	trajectories []SessionTrajectory,
	initialDominance []StaticDominanceConflict,
) (*TuningResult, error) {
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
		var origTier TierReplayConfig
		var hasOrigTier bool
		if i < len(initialCfg.Tiers) {
			origTier = initialCfg.Tiers[i]
			hasOrigTier = true
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
			// Zero traffic reached this tier during replay -> preserve original rule and thresholds strictly
			synthRule = origWhen
			if hasOrigTier {
				tier.TokenThreshold = origTier.TokenThreshold
				tier.RetryBound = origTier.RetryBound
				tier.RestrictImages = origTier.RestrictImages
				tier.RestrictTools = origTier.RestrictTools
				tier.ExcludedKeywords = origTier.ExcludedKeywords
			}
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
		if isDisabled && origModel != "" && strings.TrimSpace(origWhen) != "false" {
			origCoding, _ := ResolveModelBenchmark(origModel)
			var predCoding float64
			for j := i - 1; j >= 0; j-- {
				if !bestCfg.Tiers[j].IsDisabled {
					predCoding = bestCfg.Tiers[j].CodingIndex
					break
				}
			}
			gainPct := 0.0
			if predCoding > 0 && origCoding > 0 {
				gainPct = ((origCoding - predCoding) / predCoding) * 100.0
			}
			sign := "+"
			if gainPct < 0 {
				sign = ""
			}
			modelBenefit = fmt.Sprintf(
				"Redundant Escalation Tier Bypassed: Capability gain over predecessor (%s%.1f%%) does not justify intermediate retry burn; bypassing saves wasted turns",
				sign, gainPct,
			)
			recModel = origModel
		} else if recModel != "" && recModel != origModel {
			origCoding, _ := ResolveModelBenchmark(origModel)
			origPrompt, origComp, origCompRate := ResolveModelRates(origModel, tier.IsLocal)
			if !tier.IsLocal && (tier.ComprehensiveRate < origCompRate || tier.PromptCostPerMillion < origPrompt || tier.CompletionCostPerMillion < origComp) {
				savingsPct := 0.0
				if origCompRate > 0 {
					savingsPct = ((origCompRate - tier.ComprehensiveRate) / origCompRate) * 100.0
				}
				if tier.CodingIndex > origCoding {
					gainPct := 0.0
					if origCoding > 0 {
						gainPct = ((tier.CodingIndex - origCoding) / origCoding) * 100.0
					}
					modelBenefit = fmt.Sprintf(BenefitMsgWithCognitiveGain, tier.CodingIndex, origCoding, gainPct, savingsPct, tier.ComprehensiveRate, origCompRate)
				} else {
					modelBenefit = fmt.Sprintf(BenefitMsgAtCognitiveParity, tier.CodingIndex, savingsPct, tier.ComprehensiveRate, origCompRate)
				}
			} else if tier.CodingIndex > origCoding {
				deltaCoding := tier.CodingIndex - origCoding
				deltaCostPerM := tier.ComprehensiveRate - origCompRate
				gainPct := (deltaCoding / origCoding) * 100.0
				if deltaCostPerM > 0.01 {
					efficiency := deltaCoding / deltaCostPerM
					modelBenefit = fmt.Sprintf("Improves coding benchmark from %.1f to %.1f (+%.1f%%, %.1f pts/$M)", origCoding, tier.CodingIndex, gainPct, efficiency)
				} else {
					modelBenefit = fmt.Sprintf("Improves coding benchmark from %.1f to %.1f (+%.1f%%) at no additional cost", origCoding, tier.CodingIndex, gainPct)
				}
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

	hasDominanceResolution := len(AnalyzeFleetDominance(&initialCfg)) > len(AnalyzeFleetDominance(&bestCfg))
	hasPruning := false
	for _, t := range tierResults {
		if t.IsDisabled && strings.TrimSpace(t.OriginalRule) != "false" {
			hasPruning = true
			break
		}
	}
	hasModelChange := false
	for _, t := range tierResults {
		if t.RecommendedModel != "" && t.RecommendedModel != t.OriginalModel {
			hasModelChange = true
			break
		}
	}
	if defaultTierResult != nil && defaultTierResult.RecommendedModel != defaultTierResult.OriginalModel {
		hasModelChange = true
	}

	if retriesAvoided <= 0 && savingsUSD <= 0.001 && !hasDominanceResolution && !hasPruning {
		if !hasModelChange {
			defaultTierResult = nil
			for i := range tierResults {
				if !tierResults[i].IsDisabled {
					tierResults[i].SynthesizedRule = tierResults[i].OriginalRule
				}
				tierResults[i].RecommendedModel = tierResults[i].OriginalModel
				tierResults[i].ModelBenefit = ""
			}
		} else {
			// Rule Invariance: No retry or dominance pressure to mutate syntax,
			// preserve AST rules while recommending qualifying replacement models
			for i := range tierResults {
				if !tierResults[i].IsDisabled {
					tierResults[i].SynthesizedRule = tierResults[i].OriginalRule
				}
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

	highContextCostPct := 0.0
	if baselineBuf.TotalCostUSD > 0 {
		highContextCostPct = (baselineBuf.HighContextCostUSD / baselineBuf.TotalCostUSD) * 100.0
	}

	return &TuningResult{
		Tiers:                      tierResults,
		DefaultTier:                defaultTierResult,
		CurrentCostUSD:             baselineBuf.TotalCostUSD,
		ProjectedCostUSD:           finalBuf.TotalCostUSD,
		ProjectedSavingsUSD:        savingsUSD,
		RetriesEliminated:          retriesAvoided,
		TotalSampleTurns:           finalBuf.TotalTurns,
		TotalSessions:              len(trajectories),
		AvgTurnsPerSession:         avgTurns,
		EscalationRate:             escalationRate,
		StaticDominanceConflicts:   initialDominance,
		HighContextEscalationTurns: baselineBuf.HighContextEscalationTurns,
		HighContextCostUSD:         baselineBuf.HighContextCostUSD,
		HighContextCostPct:         highContextCostPct,
		ReadBurstEscalations:       baselineBuf.ReadBurstEscalations,
		NTSProjectedSavingsUSD:     baselineBuf.NTSProjectedSavingsUSD,
	}, nil
}

// OptimizeWithContext executes the optimization algorithm respecting context cancellation/timeout.
func (opt *MinConflictsOptimizer) OptimizeWithContext(ctx context.Context, records []telemetry.TurnRecord, currentConfig *contract.Config) (*TuningResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		WriteOnly:   src.WriteOnly,
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
		if t.IsDisabled {
			h = (h ^ 0x04) * prime
		}
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
