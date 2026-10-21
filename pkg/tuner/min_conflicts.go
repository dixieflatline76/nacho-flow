package tuner

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
	"github.com/expr-lang/expr/parser"
)

// MinConflictsOptimizer implements the 6D Min-Conflicts CSP local search optimizer.
type MinConflictsOptimizer struct {
	policy    TuningPolicy
	platforms contract.PlatformResolver
}

// NewMinConflictsOptimizer creates an optimizer configured with the given tuning policy.
func NewMinConflictsOptimizer(policy TuningPolicy, platforms ...contract.PlatformResolver) *MinConflictsOptimizer {
	var pr contract.PlatformResolver
	if len(platforms) > 0 {
		pr = platforms[0]
	}
	return &MinConflictsOptimizer{
		policy:    policy,
		platforms: pr,
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

// applyCandidateToTier applies CandidateModel attributes to an arbitrary tier configuration.
func applyCandidateToTier(tier *TierReplayConfig, cand CandidateModel) {
	newModel, ok := NormalizeModelForProvider(cand.ModelID, tier.EffectivePlatform())
	if !ok || newModel == "" {
		newModel = cand.ModelID
	}
	if !curation.DefaultManager().IsSameModel(newModel, tier.Model) {
		tier.ReasoningEffort = ""
	}
	tier.Model = newModel
	tier.CodingIndex = EffectiveCodingIndex(cand.CodingIndex, tier.ReasoningEffort)
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
		tier.ComprehensiveRate = ResolveComprehensiveRate(cand.PromptCostPerMillion, cand.CompletionCostPerMillion, cand.ComprehensiveRate)
		tier.CostPerMillion = tier.ComprehensiveRate
	} else {
		tier.PromptCostPerMillion = 0
		tier.CompletionCostPerMillion = 0
		tier.ComprehensiveRate = 0
		tier.CostPerMillion = 0
	}
}

// WithCandidateModel substitutes a tier's model with an evaluated CandidateModel.
func (b *CandidateConfigBuilder) WithCandidateModel(tierIdx int, cand CandidateModel) *CandidateConfigBuilder {
	applyCandidateToTier(&b.cfg.Tiers[tierIdx], cand)
	return b
}

// WithModelCandidate substitutes a tier's model with a curated candidate.
func (b *CandidateConfigBuilder) WithModelCandidate(tierIdx int, cand curation.ModelCuratedProfile) *CandidateConfigBuilder {
	return b.WithCandidateModel(tierIdx, ConvertProfileToCandidate(cand))
}

// WithDefaultTierCandidateModel substitutes the fallback default tier's model with an evaluated CandidateModel.
func (b *CandidateConfigBuilder) WithDefaultTierCandidateModel(cand CandidateModel) *CandidateConfigBuilder {
	applyCandidateToTier(&b.cfg.DefaultTier, cand)
	return b
}

// WithDefaultTierModel substitutes the fallback default tier's model.
func (b *CandidateConfigBuilder) WithDefaultTierModel(cand curation.ModelCuratedProfile) *CandidateConfigBuilder {
	return b.WithDefaultTierCandidateModel(ConvertProfileToCandidate(cand))
}

// WithReasoningEffort updates a tier's reasoning effort level and recomputes effective coding index.
func (b *CandidateConfigBuilder) WithReasoningEffort(tierIdx int, effort string) *CandidateConfigBuilder {
	t := &b.cfg.Tiers[tierIdx]
	t.ReasoningEffort = effort
	baseCoding, _ := ResolveModelBenchmark(t.Model)
	if baseCoding <= 0 {
		baseCoding = t.CodingIndex
	}
	t.CodingIndex = EffectiveCodingIndex(baseCoding, effort)
	return b
}

// WithDefaultTierReasoningEffort updates the fallback default tier's reasoning effort level.
func (b *CandidateConfigBuilder) WithDefaultTierReasoningEffort(effort string) *CandidateConfigBuilder {
	t := &b.cfg.DefaultTier
	t.ReasoningEffort = effort
	baseCoding, _ := ResolveModelBenchmark(t.Model)
	if baseCoding <= 0 {
		baseCoding = t.CodingIndex
	}
	t.CodingIndex = EffectiveCodingIndex(baseCoding, effort)
	return b
}

// getHigherReasoningEfforts returns laddered escalation levels higher than current.
func getHigherReasoningEfforts(current string) []string {
	normalized := strings.ToLower(strings.TrimSpace(current))
	switch normalized {
	case "low":
		return []string{"medium", "high", "max"}
	case "med", "medium":
		return []string{"high", "max"}
	case "high":
		return []string{"max"}
	default:
		return nil
	}
}

// containsKeyword checks if target string is in keywords slice using case-insensitive ASCII comparison without allocation.
func containsKeyword(keywords []string, target string) bool {
	for _, kw := range keywords {
		if equalFoldASCII(kw, target) {
			return true
		}
	}
	return false
}

// WithExcludedKeyword adds a keyword exclusion to the tier.
func (b *CandidateConfigBuilder) WithExcludedKeyword(tierIdx int, kw string) *CandidateConfigBuilder {
	if !containsKeyword(b.cfg.Tiers[tierIdx].ExcludedKeywords, kw) {
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
	policy         TuningPolicy
	trajectories   []SessionTrajectory
	recoveryStats  map[string]RecoveryStats
	bestConflict   float64
	minValConflict float64
	bestValCfg     MultiTierConfig
	tabu           *TabuMemory
	evalBuf        *MultiTierReplayResult
	providers      map[string]contract.ProviderConfig
}

func (rc *repairContext) evalCandidate(candCfg *MultiTierConfig) {
	if !CheckHardConstraints(candCfg) {
		return
	}
	h := hashState(candCfg)
	c := EvaluateFleetConflict(rc.trajectories, candCfg, &rc.policy, rc.evalBuf)
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

	// Propagate WriteOnly policy from active config without mutating shared receiver
	policy := opt.policy
	if currentConfig.Kickstart.ReadSlap.WriteOnly {
		policy.WriteOnly = true
	}
	wOpt := *opt
	wOpt.policy = policy

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
	keywordAnalyzer := NewKeywordRiskAnalyzer(policy)
	candidateKeywords := keywordAnalyzer.Analyze(records)
	if len(candidateKeywords) > 8 {
		candidateKeywords = candidateKeywords[:8]
	}

	// 2. Extract initial state from active config
	currentCfg := ExtractRoutingState(currentConfig, candidateKeywords, opt.platforms)
	if len(currentCfg.Tiers) == 0 {
		return nil, fmt.Errorf("no tunable tiers found in active configuration")
	}
	initialCfg := copyMultiTierConfig(&currentCfg)
	initialDominance := AnalyzeFleetDominance(&initialCfg)

	// 3. Evaluate initial baseline
	var baselineBuf MultiTierReplayResult
	ReplayMultiTierFleet(trajectories, &currentCfg, &policy, &baselineBuf)
	baselineConflict := EvaluateFleetConflict(trajectories, &currentCfg, &policy, &baselineBuf)

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
	// #nosec G404 - deterministic pseudo-random epsilon perturbation for reproducible heuristic search
	rng := rand.New(rand.NewSource(42))

	recoveryStats := AnalyzeRecovery(trajectories)
	rc := repairContext{
		opt:           &wOpt,
		policy:        policy,
		trajectories:  trajectories,
		recoveryStats: recoveryStats,
		tabu:          tabu,
		evalBuf:       &evalBuf,
		providers:     currentConfig.Providers,
	}

	for iter := 0; iter < maxIterations; iter++ {
		report := EvaluateFleetWithAttribution(trajectories, &currentCfg, &policy, candidateKeywords)
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
			wOpt.dispatchRepairMove(cv.name, &currentCfg, currentConfig, &rc)
			if rc.minValConflict < report.TotalConflict {
				break
			}
		}

		currentCfg = rc.bestValCfg
		tabu.Push(hashState(&currentCfg))

		if rc.minValConflict < bestConflict {
			bestConflict = rc.minValConflict
			bestCfg = copyMultiTierConfig(&currentCfg)
			consecutiveStagnations = 0
		}
	}

	// 5b. Post-CSP model substitution sweep for active cloud tiers.
	wOpt.sweepModelReplacements(trajectories, &bestCfg, &evalBuf, &bestConflict, currentConfig.Providers)

	// 6. Synthesize final multi-tier rules and impact metrics
	var finalBuf MultiTierReplayResult
	ReplayMultiTierFleet(trajectories, &bestCfg, &policy, &finalBuf)

	return wOpt.synthesizeTuningResult(currentConfig, initialCfg, bestCfg, baselineBuf, finalBuf, trajectories, initialDominance, recoveryStats)
}

// dispatchRepairMove dispatches a single variable repair step based on targetVar name.
func (opt *MinConflictsOptimizer) dispatchRepairMove(
	targetVar string,
	currentCfg *MultiTierConfig,
	currentConfig *contract.Config,
	rc *repairContext,
) {
	if strings.HasPrefix(targetVar, "tier_") {
		parts := strings.Split(targetVar, ":")
		if len(parts) == 2 {
			tierIdxStr := strings.TrimPrefix(parts[0], "tier_")
			attr := parts[1]
			tierIdx, err := strconv.Atoi(tierIdxStr)
			if err == nil && tierIdx >= 0 && tierIdx < len(currentCfg.Tiers) && !currentCfg.Tiers[tierIdx].IsDisabled {
				switch attr {
				case "tokens":
					opt.repairTokens(currentCfg, tierIdx, rc)
				case "retries":
					opt.repairRetries(currentCfg, tierIdx, rc)
				case "tools", "images":
					opt.repairModalityGates(currentCfg, tierIdx, attr, rc)
				case "model":
					opt.repairModelOrPrune(currentCfg, tierIdx, rc)
				}
			}
		}
		return
	}
	if strings.HasPrefix(targetVar, "keyword:") {
		kw := strings.TrimPrefix(targetVar, "keyword:")
		opt.repairKeyword(currentCfg, kw, rc)
		return
	}
	if targetVar == "default_tier:model" {
		opt.repairDefaultTierModel(currentCfg, currentConfig, rc)
	}
}

// findImmediateNeighbors returns the non-disabled predecessor and successor tiers for the given index.
func findImmediateNeighbors(tiers []TierReplayConfig, idx int) (pred, succ *TierReplayConfig) {
	for j := idx - 1; j >= 0; j-- {
		if !tiers[j].IsDisabled {
			pred = &tiers[j]
			break
		}
	}
	for j := idx + 1; j < len(tiers); j++ {
		if !tiers[j].IsDisabled {
			succ = &tiers[j]
			break
		}
	}
	return pred, succ
}

// resolveTierRole determines the operational role for candidate matching based on tier capabilities and escalation status.
func resolveTierRole(t *TierReplayConfig, isEscalation bool) curation.TierRole {
	if t.RequiresImages || (t.SupportsVision && !t.SupportsTools) {
		return curation.RoleVisionWorkhorse
	}
	if t.IsFrontier || IsFrontierModel(t.Model) || isEscalation {
		return curation.RoleGeneral
	}
	return curation.RoleCodingWorkhorse
}

// sweepModelReplacements performs a post-CSP pass over active cloud tiers with spend,
// evaluating qualifying replacement candidates via CandidateFilterPipeline and CompositeValueScorer.
func (opt *MinConflictsOptimizer) sweepModelReplacements(
	trajectories []SessionTrajectory,
	bestCfg *MultiTierConfig,
	evalBuf *MultiTierReplayResult,
	bestConflict *float64,
	providers map[string]contract.ProviderConfig,
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

		immediatePred, immediateSucc := findImmediateNeighbors(bestCfg.Tiers, i)

		// Invariance: Do not perform single-tier model replacement if this tier shares its model
		// with an adjacent predecessor or successor tier with differential reasoning effort.
		// Doing so would shatter KV prompt cache stickiness.
		isReasoningEscalationChain := (immediatePred != nil && immediatePred.Model == t.Model && immediatePred.ReasoningEffort != t.ReasoningEffort && t.ReasoningEffort != "") ||
			(immediateSucc != nil && immediateSucc.Model == t.Model && immediateSucc.ReasoningEffort != t.ReasoningEffort && t.ReasoningEffort != "")
		if isReasoningEscalationChain {
			continue
		}

		tierRole := resolveTierRole(t, false)

		candidates := CollectCandidateModels(tierRole, t.EffectivePlatform(), t.Model, opt.policy.LocalVRAMGB, opt.policy.CandidateDeals)
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
				Providers:     providers,
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
	// Only mutate token cliffs for local GPU tiers (to prevent VRAM OOM).
	// Never artificially clamp cloud workhorses with 1M context down to 64k.
	if !currentCfg.Tiers[tierIdx].IsLocal {
		return
	}
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
	cands := RetryCandidateValues
	model := currentCfg.Tiers[tierIdx].Model
	if rc.recoveryStats != nil && model != "" {
		if stat, ok := rc.recoveryStats[model]; ok && stat.TotalFailures > 0 {
			if stat.SelfRecoveryRate < 0.20 {
				cands = []int{1, 2}
			} else if stat.AvgTurnsToRecover <= 2.0 {
				cands = []int{2, 3, 4}
			} else {
				cands = []int{3, 4, 6}
			}
		}
	}
	for _, candR := range cands {
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
	if currentCfg.Tiers[tierIdx].IsLocal && rc.policy.LocalVRAMGB == 0 {
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

	// Invariance Guard: Do NOT prune an escalation tier if it steps reasoning effort
	// over its predecessor (e.g. glm-5.3-flash low -> high -> max).
	isReasoningEscalation := immediatePred != nil &&
		immediatePred.Model == currentCfg.Tiers[tierIdx].Model &&
		immediatePred.ReasoningEffort != currentCfg.Tiers[tierIdx].ReasoningEffort &&
		currentCfg.Tiers[tierIdx].ReasoningEffort != ""

	// Autonomous Tier Pruning Candidate:
	// Try disabling this intermediate escalation tier entirely.
	// Whichever produces lower total fleet conflict C(X) wins.
	if isEscalationTier && !isReasoningEscalation && tierIdx > 0 && tierIdx < len(currentCfg.Tiers)-1 {
		candCfg := NewCandidateBuilder(currentCfg).
			WithPrunedTier(tierIdx).
			Build()
		rc.evalCandidate(&candCfg)
	}

	// Invariance Guard: Do not perform single-tier model candidate replacement
	// on an intermediate tier in a same-model reasoning chain
	if isReasoningEscalation {
		return
	}

	initialTierConflicts := CountFleetDominanceConflicts(currentCfg)

	// Try laddering reasoning effort on the existing tier model first before model substitution
	higherEfforts := getHigherReasoningEfforts(currentCfg.Tiers[tierIdx].ReasoningEffort)
	solvedViaTierReasoning := false
	for _, effort := range higherEfforts {
		candCfg := NewCandidateBuilder(currentCfg).
			WithReasoningEffort(tierIdx, effort).
			Build()
		if initialTierConflicts > 0 && CountFleetDominanceConflicts(&candCfg) == 0 {
			rc.evalCandidate(&candCfg)
			solvedViaTierReasoning = true
			break
		}
		rc.evalCandidate(&candCfg)
	}

	if solvedViaTierReasoning {
		// Prefer same-model reasoning escalation over external model swap
		return
	}

	tierRole := resolveTierRole(&currentCfg.Tiers[tierIdx], isEscalationTier)
	candidates := CollectCandidateModels(
		tierRole,
		currentCfg.Tiers[tierIdx].EffectivePlatform(),
		currentCfg.Tiers[tierIdx].Model,
		rc.policy.LocalVRAMGB,
		rc.policy.CandidateDeals,
	)

	pipeline := NewDefaultFilterPipeline()
	scorer := &CompositeValueScorer{}

	_, immediateSucc := findImmediateNeighbors(currentCfg.Tiers, tierIdx)

	for _, cand := range candidates {
		if curation.DefaultManager().IsSameModel(cand.ModelID, currentCfg.Tiers[tierIdx].Model) {
			continue
		}
		// Anti-Duplication: Never duplicate immediate escalation predecessor
		if immediatePred != nil && curation.DefaultManager().IsSameModel(cand.ModelID, immediatePred.Model) {
			continue
		}

		// Evaluate candidate against specification filter pipeline
		fCtx := &FilterContext{
			TargetTier:    &currentCfg.Tiers[tierIdx],
			Candidate:     &cand,
			ImmediatePred: immediatePred,
			ImmediateSucc: immediateSucc,
			Policy:        &rc.policy,
			Providers:     rc.providers,
		}
		candEffCoding := cand.CodingIndex
		if passed, reason := pipeline.Evaluate(fCtx); !passed {
			// Hard filters: provider incompatibility, local tier protection, and nil context must NEVER be bypassed
			if strings.HasPrefix(reason, "provider_") || reason == "local_tier_protected" || reason == "nil_context" {
				continue
			}
			// For escalation tiers resolving capability failures, allow benchmark leap if frontier or meets escalation gain
			minGain := rc.policy.MinEscalationGainPct
			if minGain <= 0 {
				minGain = 0.05
			}
			isEscalationUpgrade := isEscalationTier && immediatePred != nil &&
				(cand.IsFrontier() || candEffCoding >= immediatePred.CodingIndex*(1.0+minGain))
			if !isEscalationUpgrade {
				continue
			}
		}

		// Frontier tier protection: never downgrade a frontier tier to a non-frontier or lower-benchmark model
		if isFrontier {
			if !cand.IsFrontier() && candEffCoding < 80.0 {
				continue
			}
			if currentCfg.Tiers[tierIdx].CodingIndex > 0 && candEffCoding < currentCfg.Tiers[tierIdx].CodingIndex {
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
		c := EvaluateFleetConflict(rc.trajectories, &candCfg, &rc.policy, rc.evalBuf)
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

// repairDefaultTierModel explores Pareto candidate substitutions and reasoning escalation on the fallback tier.
func (opt *MinConflictsOptimizer) repairDefaultTierModel(currentCfg *MultiTierConfig, currentConfig *contract.Config, rc *repairContext) {
	initialDefConflicts := CountFleetDominanceConflicts(currentCfg)

	// Try laddering reasoning effort on the existing fallback model first before model substitution
	higherEfforts := getHigherReasoningEfforts(currentCfg.DefaultTier.ReasoningEffort)
	solvedViaReasoning := false
	for _, effort := range higherEfforts {
		candCfg := NewCandidateBuilder(currentCfg).
			WithDefaultTierReasoningEffort(effort).
			Build()
		if initialDefConflicts > 0 && CountFleetDominanceConflicts(&candCfg) == 0 {
			rc.evalCandidate(&candCfg)
			solvedViaReasoning = true
			break
		}
		rc.evalCandidate(&candCfg)
	}

	if solvedViaReasoning {
		// Prefer same-model reasoning escalation over external model swap to preserve prompt cache stickiness
		return
	}

	isFrontierFallback := currentCfg.DefaultTier.IsFrontier || IsFrontierModel(currentConfig.DefaultTier.Model)
	highestPredCoding := HighestPredecessorCodingIndex(currentCfg)

	// Collect models already utilized as early kickoff / utility tiers (lateral, kickoff, vision)
	earlyUtilityModels := make(map[string]bool)
	for i := range currentCfg.Tiers {
		t := &currentCfg.Tiers[i]
		if t.IsDisabled {
			continue
		}
		if t.RequiresKickstart || t.RequiresImages || t.MessageCountMax > 0 {
			earlyUtilityModels[t.Model] = true
		}
	}

	candidates := curation.DefaultManager().ParetoCandidates(
		curation.RoleGeneral,
		currentCfg.DefaultTier.EffectivePlatform(),
		currentCfg.DefaultTier.Model,
		rc.policy.LocalVRAMGB,
	)
	for _, cand := range candidates {
		if curation.DefaultManager().IsSameModel(cand.ModelID, currentCfg.DefaultTier.Model) {
			continue
		}
		if !IsModelCompatibleWithProvider(cand.ModelID, currentCfg.DefaultTier.Provider, currentCfg.DefaultTier.IsLocal, rc.providers, currentCfg.DefaultTier.Platform) {
			continue
		}
		// A tier already used as an early kickoff/utility tier with lower capability than the cascade
		// must never replace the cascade/fallback tier
		if earlyUtilityModels[cand.ModelID] && highestPredCoding > 0 && cand.CodingIndex <= highestPredCoding {
			continue
		}
		if highestPredCoding > 0 && cand.CodingIndex < highestPredCoding {
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
		plat := ResolveProviderPlatform(tier.Provider, isLocal, currentConfig.Providers, opt.platforms)
		promptCost, compCost, compRate := ResolveModelRates(plat, tier.Model)
		codingIndex, toolReliability := ResolveModelBenchmark(tier.Model)
		isDisabled := strings.TrimSpace(tier.When) == "false"
		if isDisabled {
			tierResults = append(tierResults, TierTuningResult{
				TierName:                 tier.Name,
				Model:                    tier.Model,
				OriginalModel:            tier.Model,
				RecommendedModel:         tier.Model,
				CodingIndex:              math.Round(codingIndex*100) / 100,
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
			CodingIndex:              math.Round(codingIndex*100) / 100,
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

// isTierRoutingUnchanged checks if candidate tier routing limits match the original baseline tier.
func isTierRoutingUnchanged(tier, origTier *TierReplayConfig) bool {
	if tier == nil || origTier == nil {
		return false
	}
	return tier.TokenThreshold == origTier.TokenThreshold &&
		tier.RetryBound == origTier.RetryBound &&
		tier.RestrictImages == origTier.RestrictImages &&
		tier.RestrictTools == origTier.RestrictTools &&
		tier.MessageCountMax == origTier.MessageCountMax &&
		tier.RequiresTestFail == origTier.RequiresTestFail &&
		tier.ProhibitsTestFail == origTier.ProhibitsTestFail &&
		equalStringSlices(tier.ExcludedKeywords, origTier.ExcludedKeywords)
}

// synthesizeTierRule generates the new AST when expression for a tuned tier.
func synthesizeTierRule(
	origWhen string,
	tier *TierReplayConfig,
	origTier *TierReplayConfig,
	hasOrig bool,
	turnsRouted int,
	isDisabled bool,
) (string, error) {
	if isDisabled {
		tier.TokenThreshold = 0
		tier.RetryBound = 0
		return "false", nil
	}
	if turnsRouted == 0 {
		// Zero traffic reached this tier during replay -> preserve original rule and thresholds strictly
		if hasOrig {
			tier.TokenThreshold = origTier.TokenThreshold
			tier.RetryBound = origTier.RetryBound
			tier.RestrictImages = origTier.RestrictImages
			tier.RestrictTools = origTier.RestrictTools
			tier.ExcludedKeywords = origTier.ExcludedKeywords
		}
		return origWhen, nil
	}
	if hasOrig && tier.Model != origTier.Model {
		if origTier.SupportsVision && !tier.SupportsVision && !tier.RequiresImages {
			tier.RestrictImages = true
		}
		if origTier.SupportsTools && !tier.SupportsTools && !tier.RequiresTools {
			tier.RestrictTools = true
		}
	}
	if hasOrig && isTierRoutingUnchanged(tier, origTier) {
		return origWhen, nil
	}

	if tier.Model != "" && !tier.SupportsVision && !tier.RequiresImages {
		tier.RestrictImages = true
	}
	if tier.Model != "" && !tier.SupportsTools && !tier.RequiresTools {
		tier.RestrictTools = true
	}
	synthRule, err := RewriteRuleAST(
		origWhen,
		tier.TokenThreshold,
		tier.RetryBound,
		tier.ExcludedKeywords,
		tier.RestrictImages,
		tier.RestrictTools,
	)
	if err != nil {
		return "", fmt.Errorf("failed to rewrite AST for tier %q: %w", tier.TierName, err)
	}
	return synthRule, nil
}

// formatModelBenefit computes human-readable explanation of why a model or pruning was recommended.
func formatModelBenefit(
	tier, origTier *TierReplayConfig,
	bestCfg *MultiTierConfig,
	tierIdx int,
	isDisabled bool,
	origModel, origWhen string,
) (recModel string, benefit string) {
	recModel = tier.Model
	if isDisabled && origModel != "" && strings.TrimSpace(origWhen) != "false" {
		origCoding, _ := ResolveModelBenchmark(origModel)
		if origTier != nil {
			origCoding = EffectiveCodingIndex(origCoding, origTier.ReasoningEffort)
		}
		var predCoding float64
		pred, _ := findImmediateNeighbors(bestCfg.Tiers, tierIdx)
		if pred != nil {
			predCoding = pred.CodingIndex
		}
		gainPct := 0.0
		if predCoding > 0 && origCoding > 0 {
			gainPct = ((origCoding - predCoding) / predCoding) * 100.0
		}
		sign := "+"
		if gainPct < 0 {
			sign = ""
		}
		benefit = fmt.Sprintf(
			"Redundant Escalation Tier Bypassed: Capability gain over predecessor (%s%.1f%%) does not justify intermediate retry burn; bypassing saves wasted turns",
			sign, gainPct,
		)
		return origModel, benefit
	}

	if recModel == "" || recModel == origModel {
		return recModel, ""
	}

	origCoding, _ := ResolveModelBenchmark(origModel)
	if origTier != nil {
		origCoding = EffectiveCodingIndex(origCoding, origTier.ReasoningEffort)
	}
	origPrompt, origComp, origCompRate := ResolveModelRates(tier.EffectivePlatform(), origModel)
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
			benefit = fmt.Sprintf(BenefitMsgWithCognitiveGain, tier.CodingIndex, origCoding, gainPct, savingsPct, tier.ComprehensiveRate, origCompRate)
		} else {
			benefit = fmt.Sprintf(BenefitMsgAtCognitiveParity, tier.CodingIndex, savingsPct, tier.ComprehensiveRate, origCompRate)
		}
	} else if tier.CodingIndex > origCoding {
		deltaCoding := tier.CodingIndex - origCoding
		deltaCostPerM := tier.ComprehensiveRate - origCompRate
		gainPct := (deltaCoding / origCoding) * 100.0
		if deltaCostPerM > 0.01 {
			efficiency := deltaCoding / deltaCostPerM
			benefit = fmt.Sprintf("Improves coding benchmark from %.1f to %.1f (+%.1f%%, %.1f pts/$M)", origCoding, tier.CodingIndex, gainPct, efficiency)
		} else {
			benefit = fmt.Sprintf("Improves coding benchmark from %.1f to %.1f (+%.1f%%) at no additional cost", origCoding, tier.CodingIndex, gainPct)
		}
	} else {
		benefit = "Optimizes fleet cost-to-performance frontier"
	}
	return recModel, benefit
}

// buildDefaultTierResult synthesizes recommendations for the fallback default tier if upgraded.
func buildDefaultTierResult(currentConfig *contract.Config, bestCfg *MultiTierConfig) *TierTuningResult {
	origDefModel := currentConfig.DefaultTier.Model
	recDefModel := bestCfg.DefaultTier.Model
	origDefReasoning := currentConfig.DefaultTier.ReasoningEffort
	recDefReasoning := bestCfg.DefaultTier.ReasoningEffort

	modelChanged := recDefModel != "" && recDefModel != origDefModel
	reasoningChanged := recDefReasoning != "" && recDefReasoning != origDefReasoning

	if !modelChanged && !reasoningChanged {
		return nil
	}
	origDefCoding, _ := ResolveModelBenchmark(origDefModel)
	origDefCoding = EffectiveCodingIndex(origDefCoding, origDefReasoning)
	var defBenefit string
	if bestCfg.DefaultTier.CodingIndex > origDefCoding {
		if reasoningChanged && !modelChanged {
			origDisplay := origDefReasoning
			if origDisplay == "" {
				origDisplay = "standard"
			}
			defBenefit = fmt.Sprintf("Improves fallback benchmark from %.1f to %.1f (escalates reasoning: %s -> %s)", origDefCoding, bestCfg.DefaultTier.CodingIndex, origDisplay, recDefReasoning)
		} else {
			defBenefit = fmt.Sprintf("Improves fallback benchmark from %.1f to %.1f", origDefCoding, bestCfg.DefaultTier.CodingIndex)
		}
	} else {
		defBenefit = "Reduces fallback cloud cost while preserving reasoning capability"
	}
	return &TierTuningResult{
		TierName:                   bestCfg.DefaultTier.TierName,
		Model:                      recDefModel,
		OriginalModel:              origDefModel,
		RecommendedModel:           recDefModel,
		ReasoningEffort:            recDefReasoning,
		OriginalReasoningEffort:    origDefReasoning,
		RecommendedReasoningEffort: recDefReasoning,
		ModelBenefit:               defBenefit,
		CodingIndex:                math.Round(bestCfg.DefaultTier.CodingIndex*100) / 100,
		ToolReliability:            bestCfg.DefaultTier.ToolReliability,
		PromptCostPerMillion:       bestCfg.DefaultTier.PromptCostPerMillion,
		CompletionCostPerMillion:   bestCfg.DefaultTier.CompletionCostPerMillion,
		ComprehensiveRate:          bestCfg.DefaultTier.ComprehensiveRate,
	}
}

// applyRuleInvarianceGuard rolls back syntax modifications when fleet metrics did not improve.
func applyRuleInvarianceGuard(
	tierResults []TierTuningResult,
	defaultTierResult **TierTuningResult,
	retriesAvoided int,
	savingsUSD float64,
	initialCfg, bestCfg *MultiTierConfig,
) {
	hasDominanceResolution := len(AnalyzeFleetDominance(initialCfg)) > len(AnalyzeFleetDominance(bestCfg))
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
	if *defaultTierResult != nil && (*defaultTierResult).RecommendedModel != (*defaultTierResult).OriginalModel {
		hasModelChange = true
	}
	hasReasoningChange := false
	for _, t := range tierResults {
		if t.RecommendedReasoningEffort != "" && t.RecommendedReasoningEffort != t.OriginalReasoningEffort {
			hasReasoningChange = true
			break
		}
	}
	if *defaultTierResult != nil && (*defaultTierResult).RecommendedReasoningEffort != (*defaultTierResult).OriginalReasoningEffort {
		hasReasoningChange = true
	}

	if retriesAvoided <= 0 && savingsUSD <= 0.001 && !hasDominanceResolution && !hasPruning {
		if !hasModelChange && !hasReasoningChange {
			*defaultTierResult = nil
			for i := range tierResults {
				if !tierResults[i].IsDisabled {
					tierResults[i].SynthesizedRule = tierResults[i].OriginalRule
				}
				tierResults[i].RecommendedModel = tierResults[i].OriginalModel
				tierResults[i].RecommendedReasoningEffort = tierResults[i].OriginalReasoningEffort
				tierResults[i].ModelBenefit = ""
			}
		} else {
			// Rule Invariance: No retry or dominance pressure to mutate syntax,
			// preserve AST rules while recommending qualifying replacement models or reasoning
			for i := range tierResults {
				if !tierResults[i].IsDisabled {
					tierResults[i].SynthesizedRule = tierResults[i].OriginalRule
				}
			}
		}
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
	recoveryStats map[string]RecoveryStats,
) (*TuningResult, error) {
	var tierResults []TierTuningResult
	for i := range bestCfg.Tiers {
		tier := &bestCfg.Tiers[i]
		var origWhen, origModel, origReasoning string
		if i < len(currentConfig.Tiers) {
			origWhen = currentConfig.Tiers[i].When
			origModel = currentConfig.Tiers[i].Model
			origReasoning = currentConfig.Tiers[i].ReasoningEffort
		}
		recReasoning := tier.ReasoningEffort

		isDisabled := tier.IsDisabled || strings.TrimSpace(origWhen) == "false"
		if clean := strings.TrimSpace(origWhen); clean != "" && clean != "false" {
			if _, err := parser.Parse(clean); err != nil {
				return nil, fmt.Errorf("failed to rewrite AST for tier %q: %w", tier.TierName, err)
			}
		}

		var origTier *TierReplayConfig
		hasOrig := i < len(initialCfg.Tiers)
		if hasOrig {
			origTier = &initialCfg.Tiers[i]
		}

		turnsRouted := 0
		if i < len(finalBuf.TierStats) {
			turnsRouted = finalBuf.TierStats[i].TurnsRouted
		}

		synthRule, err := synthesizeTierRule(origWhen, tier, origTier, hasOrig, turnsRouted, isDisabled)
		if err != nil {
			return nil, err
		}

		recModel, modelBenefit := formatModelBenefit(tier, origTier, &bestCfg, i, isDisabled, origModel, origWhen)

		tierResults = append(tierResults, TierTuningResult{
			TierName:                   tier.TierName,
			Model:                      recModel,
			ReasoningEffort:            recReasoning,
			OriginalModel:              origModel,
			RecommendedModel:           recModel,
			OriginalReasoningEffort:    origReasoning,
			RecommendedReasoningEffort: recReasoning,
			ModelBenefit:               modelBenefit,
			CodingIndex:                math.Round(tier.CodingIndex*100) / 100,
			ToolReliability:            tier.ToolReliability,
			PromptCostPerMillion:       tier.PromptCostPerMillion,
			CompletionCostPerMillion:   tier.CompletionCostPerMillion,
			ComprehensiveRate:          tier.ComprehensiveRate,
			IsDisabled:                 isDisabled,
			OptimalThreshold:           tier.TokenThreshold,
			OptimalRetries:             tier.RetryBound,
			FrictionKeywords:           tier.ExcludedKeywords,
			RestrictImages:             tier.RestrictImages,
			RestrictTools:              tier.RestrictTools,
			OriginalRule:               origWhen,
			SynthesizedRule:            synthRule,
		})
	}

	defaultTierResult := buildDefaultTierResult(currentConfig, &bestCfg)

	retriesAvoided := baselineBuf.TotalWastedRetries - finalBuf.TotalWastedRetries
	if retriesAvoided < 0 {
		retriesAvoided = 0
	}
	savingsUSD := baselineBuf.TotalCostUSD - finalBuf.TotalCostUSD
	if savingsUSD < 0 {
		savingsUSD = 0
	}

	applyRuleInvarianceGuard(tierResults, &defaultTierResult, retriesAvoided, savingsUSD, &initialCfg, &bestCfg)

	escalationRate := 0.0
	avgTurns := 0.0
	if len(trajectories) > 0 {
		escalationRate = float64(finalBuf.EscalatedSessions) / float64(len(trajectories))
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
		RecoveryStats:              recoveryStats,
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
		for i := 0; i < len(t.ReasoningEffort); i++ {
			h = (h ^ uint64(t.ReasoningEffort[i])) * prime
		}
		h = (h ^ uint64(t.MessageCountMax)) * prime
		if t.RequiresTestFail {
			h = (h ^ 0x08) * prime
		}
		if t.ProhibitsTestFail {
			h = (h ^ 0x10) * prime
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
	for i := 0; i < len(cfg.DefaultTier.ReasoningEffort); i++ {
		h = (h ^ uint64(cfg.DefaultTier.ReasoningEffort[i])) * prime
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
