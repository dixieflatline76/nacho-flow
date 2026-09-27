package tuner

import (
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

// CandidateModel encapsulates all operational, cognitive, and financial dimensions of a potential replacement model.
type CandidateModel struct {
	ModelID                  string
	Name                     string
	CodingIndex              float64
	ToolReliability          float64
	PromptCostPerMillion     float64
	CompletionCostPerMillion float64
	ComprehensiveRate        float64
	ContextLength            int
	SupportsVision           bool
	SupportsTools            bool
	IsOpenWeights            bool
	IsFree                   bool
	TierRole                 string
	RecommendedTiers         []string
}

// FilterContext provides the complete decision environment for evaluating a candidate replacement model.
type FilterContext struct {
	TargetTier    *TierReplayConfig
	Candidate     *CandidateModel
	ImmediatePred *TierReplayConfig
	ImmediateSucc *TierReplayConfig
	Policy        *TuningPolicy
}

// CandidateFilter defines a single-responsibility specification for candidate model qualification.
type CandidateFilter interface {
	Name() string
	Passes(ctx *FilterContext) (bool, string)
}

// LocalTierImmunityFilter protects zero-cost local inference engines ($0.00 Ollama/Gemma) from cloud replacement.
type LocalTierImmunityFilter struct{}

func (f *LocalTierImmunityFilter) Name() string { return "local_tier_immunity" }

func (f *LocalTierImmunityFilter) Passes(ctx *FilterContext) (bool, string) {
	if ctx.TargetTier == nil || ctx.Candidate == nil {
		return false, "nil_context"
	}
	if ctx.TargetTier.IsLocal && !ctx.Candidate.IsOpenWeights {
		return false, "local_tier_protected"
	}
	return true, ""
}

// ContextParityFilter guarantees candidate context window is equal to or larger than the tier's token threshold.
type ContextParityFilter struct{}

func (f *ContextParityFilter) Name() string { return "context_parity" }

func (f *ContextParityFilter) Passes(ctx *FilterContext) (bool, string) {
	if ctx.TargetTier == nil || ctx.Candidate == nil {
		return false, "nil_context"
	}
	requiredContext := ctx.TargetTier.TokenThreshold
	if requiredContext <= 0 {
		requiredContext = DefaultContextWindowFloor
		if ctx.Policy != nil && ctx.Policy.ContextWindowFloor > 0 {
			requiredContext = ctx.Policy.ContextWindowFloor
		}
	}
	if ctx.Candidate.ContextLength > 0 && ctx.Candidate.ContextLength < requiredContext {
		return false, "insufficient_context_length"
	}
	return true, ""
}

// ModalityFilter enforces that tool-calling and multimodal image capabilities are preserved.
type ModalityFilter struct{}

func (f *ModalityFilter) Name() string { return "modality_preservation" }

func (f *ModalityFilter) Passes(ctx *FilterContext) (bool, string) {
	if ctx.TargetTier == nil || ctx.Candidate == nil {
		return false, "nil_context"
	}
	if ctx.TargetTier.SupportsTools && !ctx.Candidate.SupportsTools {
		return false, "missing_required_tools"
	}
	if ctx.TargetTier.RequiresImages && !ctx.Candidate.SupportsVision {
		return false, "missing_required_vision"
	}
	return true, ""
}

// CognitiveParityFilter guarantees the candidate matches or exceeds the target tier's coding index.
type CognitiveParityFilter struct{}

func (f *CognitiveParityFilter) Name() string { return "cognitive_parity" }

func (f *CognitiveParityFilter) Passes(ctx *FilterContext) (bool, string) {
	if ctx.TargetTier == nil || ctx.Candidate == nil {
		return false, "nil_context"
	}
	targetCoding := ctx.TargetTier.CodingIndex
	if targetCoding <= 0 {
		targetCoding, _ = ResolveModelBenchmark(ctx.TargetTier.Model)
	}
	tolerance := DefaultCodingParityTolerance
	if ctx.Policy != nil && ctx.Policy.CodingParityTolerance > 0 {
		tolerance = ctx.Policy.CodingParityTolerance
	}
	if targetCoding > 0 && ctx.Candidate.CodingIndex < (targetCoding-tolerance) {
		return false, "sub_parity_coding_index"
	}
	return true, ""
}

// CostReductionFilter ensures candidate provides significant cost savings (default >= 20%).
type CostReductionFilter struct{}

func (f *CostReductionFilter) Name() string { return "cost_reduction" }

func (f *CostReductionFilter) Passes(ctx *FilterContext) (bool, string) {
	if ctx.TargetTier == nil || ctx.Candidate == nil {
		return false, "nil_context"
	}
	origRate := ctx.TargetTier.ComprehensiveRate
	if origRate <= 0 {
		_, _, origRate = ResolveModelRates(ctx.TargetTier.Model, ctx.TargetTier.IsLocal)
	}
	candRate := ctx.Candidate.ComprehensiveRate
	if candRate <= 0 {
		candRate = ctx.Candidate.PromptCostPerMillion + 0.25*ctx.Candidate.CompletionCostPerMillion
	}
	if origRate <= 0 {
		return true, ""
	}
	minSavingsPct := DefaultMinSavingsPct
	if ctx.Policy != nil && ctx.Policy.MinSavingsPct > 0 {
		minSavingsPct = ctx.Policy.MinSavingsPct
	}
	savingsPct := ((origRate - candRate) / origRate) * 100.0
	if savingsPct < minSavingsPct {
		return false, "insufficient_cost_savings"
	}
	return true, ""
}

// DominanceOrderFilter ensures the candidate does not invert cognitive escalation ordering.
type DominanceOrderFilter struct{}

func (f *DominanceOrderFilter) Name() string { return "dominance_order" }

func (f *DominanceOrderFilter) Passes(ctx *FilterContext) (bool, string) {
	if ctx.TargetTier == nil || ctx.Candidate == nil {
		return false, "nil_context"
	}
	// Never duplicate immediate predecessor model
	if ctx.ImmediatePred != nil && ctx.Candidate.ModelID == ctx.ImmediatePred.Model {
		return false, "duplicates_predecessor_model"
	}
	// Escalation Progression: Candidate must not drop below predecessor benchmark
	if ctx.ImmediatePred != nil && !ctx.ImmediatePred.IsDisabled && ctx.ImmediatePred.CodingIndex > 0 {
		if ctx.Candidate.CodingIndex < ctx.ImmediatePred.CodingIndex {
			return false, "violates_predecessor_escalation"
		}
	}
	// Successor Inversion Guard: Candidate must not leapfrog successor benchmark
	if ctx.ImmediateSucc != nil && !ctx.ImmediateSucc.IsDisabled && ctx.ImmediateSucc.CodingIndex > 0 {
		if ctx.Candidate.CodingIndex > ctx.ImmediateSucc.CodingIndex {
			return false, "violates_successor_escalation"
		}
	}
	return true, ""
}

// CandidateFilterPipeline executes an ordered sequence of rule specifications.
type CandidateFilterPipeline struct {
	filters []CandidateFilter
}

// NewDefaultFilterPipeline instantiates the standard production candidate qualification pipeline.
func NewDefaultFilterPipeline() *CandidateFilterPipeline {
	return &CandidateFilterPipeline{
		filters: []CandidateFilter{
			&LocalTierImmunityFilter{},
			&ContextParityFilter{},
			&ModalityFilter{},
			&CognitiveParityFilter{},
			&DominanceOrderFilter{},
			&CostReductionFilter{},
		},
	}
}

// Evaluate evaluates a candidate against all filters in the pipeline sequentially.
func (p *CandidateFilterPipeline) Evaluate(ctx *FilterContext) (bool, string) {
	if p == nil || len(p.filters) == 0 {
		return true, ""
	}
	for _, f := range p.filters {
		if passed, reason := f.Passes(ctx); !passed {
			return false, reason
		}
	}
	return true, ""
}

// CandidateScorer scores and ranks qualifying replacement candidates.
type CandidateScorer interface {
	Score(target *TierReplayConfig, candidate *CandidateModel) float64
}

// CompositeValueScorer ranks candidates by their Quality-to-Price ValueScore:
// ValueScore = CodingIndex * (1.0 + DiscountPct / 100.0)
type CompositeValueScorer struct{}

func (s *CompositeValueScorer) Score(target *TierReplayConfig, candidate *CandidateModel) float64 {
	if target == nil || candidate == nil {
		return 0.0
	}
	origRate := target.ComprehensiveRate
	if origRate <= 0 {
		_, _, origRate = ResolveModelRates(target.Model, target.IsLocal)
	}
	candRate := candidate.ComprehensiveRate
	if candRate <= 0 {
		candRate = candidate.PromptCostPerMillion + 0.25*candidate.CompletionCostPerMillion
	}
	discountPct := 0.0
	if origRate > 0 && origRate > candRate {
		discountPct = ((origRate - candRate) / origRate) * 100.0
	}
	return candidate.CodingIndex * (1.0 + discountPct/100.0)
}

// ConvertDealToCandidate maps a contract.DealInfo into an operational CandidateModel.
func ConvertDealToCandidate(d contract.DealInfo) CandidateModel {
	compRate := d.PromptCostPerM + 0.25*d.CompletionCostPerM
	return CandidateModel{
		ModelID:                  d.ModelID,
		Name:                     d.Name,
		CodingIndex:              d.CodingIndex,
		PromptCostPerMillion:     d.PromptCostPerM,
		CompletionCostPerMillion: d.CompletionCostPerM,
		ComprehensiveRate:        compRate,
		ContextLength:            d.ContextLength,
		SupportsVision:           d.SupportsVision,
		SupportsTools:            d.SupportsTools,
		IsOpenWeights:            false,
		IsFree:                   d.IsFree,
		TierRole:                 d.TierRole,
		RecommendedTiers:         d.RecommendedTiers,
	}
}

// ConvertProfileToCandidate maps a curation.ModelCuratedProfile into an operational CandidateModel.
func ConvertProfileToCandidate(p curation.ModelCuratedProfile) CandidateModel {
	compRate := p.PromptCostPerMillion + 0.25*p.CompletionCostPerMillion
	return CandidateModel{
		ModelID:                  p.ModelID,
		Name:                     p.Name,
		CodingIndex:              p.CodingIndex,
		ToolReliability:          p.ToolReliability,
		PromptCostPerMillion:     p.PromptCostPerMillion,
		CompletionCostPerMillion: p.CompletionCostPerMillion,
		ComprehensiveRate:        compRate,
		ContextLength:            0, // Default unconstrained in static profile
		SupportsVision:           p.SupportsVision,
		SupportsTools:            p.SupportsTools,
		IsOpenWeights:            p.IsOpenWeights,
		IsFree:                   p.PromptCostPerMillion == 0 && p.CompletionCostPerMillion == 0,
		TierRole:                 string(p.TierRole),
		RecommendedTiers:         p.RecommendedTiers,
	}
}

// IsFrontier reports whether the candidate model represents a deep reasoner or frontier class.
func (c CandidateModel) IsFrontier() bool {
	if c.TierRole == string(curation.RoleDeepReasoner) {
		return true
	}
	for _, rTier := range c.RecommendedTiers {
		if rTier == contract.TierIDFrontier && c.CodingIndex >= 70.0 {
			return true
		}
	}
	return false
}

// IsDisallowedEndpoint reports whether an endpoint is unsuitable for agentic coding (e.g. batch endpoints).
func IsDisallowedEndpoint(modelID, name string) bool {
	lowerID := strings.ToLower(modelID)
	lowerName := strings.ToLower(name)
	if strings.Contains(lowerID, ":batch") || strings.Contains(lowerID, "-batch") || strings.Contains(lowerName, "(batch)") {
		return true
	}
	return false
}

// CollectCandidateModels aggregates candidates from policy deals and curated profiles,
// deduplicated by ModelID (deals taking precedence), excluding disallowed endpoints.
func CollectCandidateModels(tierRole curation.TierRole, isLocal bool, currentModel string, localVRAMGB int, deals []contract.DealInfo) []CandidateModel {
	seen := make(map[string]bool)
	var result []CandidateModel

	for _, d := range deals {
		if d.ModelID == "" || d.ModelID == currentModel || seen[d.ModelID] {
			continue
		}
		if IsDisallowedEndpoint(d.ModelID, d.Name) {
			continue
		}
		cand := ConvertDealToCandidate(d)
		seen[d.ModelID] = true
		result = append(result, cand)
	}

	curated := curation.DefaultManager().ParetoCandidates(tierRole, isLocal, currentModel, localVRAMGB)
	for _, p := range curated {
		if p.ModelID == "" || p.ModelID == currentModel || seen[p.ModelID] {
			continue
		}
		if IsDisallowedEndpoint(p.ModelID, p.Name) {
			continue
		}
		cand := ConvertProfileToCandidate(p)
		seen[p.ModelID] = true
		result = append(result, cand)
	}

	return result
}

// ConvertTierToCandidate maps a TierReplayConfig into an operational CandidateModel.
func ConvertTierToCandidate(t *TierReplayConfig) CandidateModel {
	if t == nil {
		return CandidateModel{}
	}
	return CandidateModel{
		ModelID:                  t.Model,
		CodingIndex:              t.CodingIndex,
		ToolReliability:          t.ToolReliability,
		PromptCostPerMillion:     t.PromptCostPerMillion,
		CompletionCostPerMillion: t.CompletionCostPerMillion,
		ComprehensiveRate:        t.ComprehensiveRate,
		SupportsVision:           t.SupportsVision,
		SupportsTools:            t.SupportsTools,
		IsOpenWeights:            t.IsLocal,
	}
}

