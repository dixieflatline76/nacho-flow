package tuner

import (
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
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
	Providers     map[string]contract.ProviderConfig
}

// CandidateFilter defines a single-responsibility specification for candidate model qualification.
type CandidateFilter interface {
	Name() string
	Passes(ctx *FilterContext) (bool, string)
}

// ProviderCompatibilityFilter guarantees that the candidate model is natively supported
// by the target tier's configured provider, preventing invalid cross-provider dispatches.
type ProviderCompatibilityFilter struct{}

func (f *ProviderCompatibilityFilter) Name() string { return "provider_compatibility" }

func (f *ProviderCompatibilityFilter) Passes(ctx *FilterContext) (bool, string) {
	if ctx.TargetTier == nil || ctx.Candidate == nil {
		return false, "nil_context"
	}
	if !IsModelCompatibleWithProvider(ctx.Candidate.ModelID, ctx.TargetTier.Provider, ctx.TargetTier.IsLocal, ctx.Providers) {
		return false, "provider_model_incompatible:" + ctx.TargetTier.Provider
	}
	return true, ""
}

// IsModelCompatibleWithProvider checks whether a model ID is valid for dispatch on a given provider.
func IsModelCompatibleWithProvider(modelID, providerName string, isLocal bool, providers map[string]contract.ProviderConfig) bool {
	if modelID == "" {
		return false
	}
	if providerName == "" {
		return true // Unconstrained test fixture
	}

	normProv := strings.ToLower(strings.TrimSpace(providerName))
	lowerModel := strings.ToLower(strings.TrimSpace(modelID))

	// 1. If providers map is provided and non-empty, verify provider exists in configuration
	var provConfig *contract.ProviderConfig
	if len(providers) > 0 {
		cfg, exists := providers[providerName]
		if !exists {
			for k, v := range providers {
				if strings.EqualFold(k, providerName) {
					cfg = v
					exists = true
					break
				}
			}
		}
		if !exists {
			return false
		}
		provConfig = &cfg
	}

	// 2. Local provider check
	isLocalProvider := isLocal
	if provConfig != nil {
		isLocalProvider = isLocalProvider || provConfig.IsLocal()
	}
	if normProv == "ollama" || normProv == "llama_cluster" || normProv == "lmstudio" ||
		normProv == "vllm" || normProv == "llamacpp" || normProv == "llama.cpp" || normProv == "local" {
		isLocalProvider = true
	}
	if provConfig != nil && (strings.Contains(provConfig.BaseURL, "127.0.0.1") || strings.Contains(provConfig.BaseURL, "localhost")) {
		isLocalProvider = true
	}

	if isLocalProvider {
		// Local providers can only run local/open-weights models, not proprietary cloud endpoints
		if strings.HasPrefix(lowerModel, "anthropic/") || strings.HasPrefix(lowerModel, "claude-") ||
			strings.HasPrefix(lowerModel, "openai/") || strings.HasPrefix(lowerModel, "gpt-") ||
			strings.HasPrefix(lowerModel, "o1") || strings.HasPrefix(lowerModel, "o3") ||
			strings.HasPrefix(lowerModel, "google/") || strings.HasPrefix(lowerModel, "gemini-") {
			return false
		}
		return true
	}

	var resolvedPlat contract.Platform
	if provConfig != nil {
		resolvedPlat, _ = provider.ResolvePlatform(*provConfig)
	}

	// 3. Direct vendor provider checks
	// Direct Anthropic (e.g. provider: "anthropic" or platform: anthropic)
	isAnthropicDirect := normProv == "anthropic" || resolvedPlat == contract.PlatformAnthropic
	if isAnthropicDirect {
		return strings.HasPrefix(lowerModel, "claude-") ||
			strings.HasPrefix(lowerModel, "anthropic/claude-") ||
			strings.HasPrefix(lowerModel, "anthropic/")
	}

	// Direct OpenAI (e.g. provider: "openai" or baseURL contains api.openai.com)
	isOpenAIDirect := normProv == "openai"
	if provConfig != nil && strings.Contains(strings.ToLower(provConfig.BaseURL), "api.openai.com") {
		isOpenAIDirect = true
	}
	if isOpenAIDirect {
		return strings.HasPrefix(lowerModel, "gpt-") ||
			strings.HasPrefix(lowerModel, "o1") ||
			strings.HasPrefix(lowerModel, "o3") ||
			strings.HasPrefix(lowerModel, "o4") ||
			strings.HasPrefix(lowerModel, "openai/")
	}

	// Direct Google / Gemini (e.g. provider: "google" or "gemini" or baseURL contains googleapis.com)
	isGoogleDirect := normProv == "google" || normProv == "gemini"
	if provConfig != nil && (strings.Contains(strings.ToLower(provConfig.BaseURL), "googleapis.com") || strings.Contains(strings.ToLower(provConfig.BaseURL), "generativelanguage.googleapis.com")) {
		isGoogleDirect = true
	}
	if isGoogleDirect {
		return strings.HasPrefix(lowerModel, "gemini-") ||
			strings.HasPrefix(lowerModel, "google/gemini-") ||
			strings.HasPrefix(lowerModel, "google/")
	}

	// Direct DeepSeek (e.g. provider: "deepseek" or baseURL contains api.deepseek.com)
	isDeepSeekDirect := normProv == "deepseek"
	if provConfig != nil && strings.Contains(strings.ToLower(provConfig.BaseURL), "api.deepseek.com") {
		isDeepSeekDirect = true
	}
	if isDeepSeekDirect {
		return strings.HasPrefix(lowerModel, "deepseek-") ||
			strings.HasPrefix(lowerModel, "deepseek/")
	}

	// 4. Aggregator providers (openrouter, litellm, etc.)
	isAggregator := normProv == "openrouter" || normProv == "litellm" || resolvedPlat == contract.PlatformOpenRouter
	if isAggregator {
		return true
	}

	// Generic cloud provider with configured entry
	if resolvedPlat == contract.PlatformOpenAICompatible {
		return true
	}

	return true
}

// NormalizeModelForProvider ensures model IDs match the format expected by the provider
// (e.g., stripping vendor prefix for direct Anthropic/OpenAI/Google/DeepSeek endpoints).
func NormalizeModelForProvider(modelID, providerName string) string {
	normProv := strings.ToLower(strings.TrimSpace(providerName))
	switch normProv {
	case "anthropic":
		if strings.HasPrefix(strings.ToLower(modelID), "anthropic/") {
			return modelID[len("anthropic/"):]
		}
	case "openai":
		if strings.HasPrefix(strings.ToLower(modelID), "openai/") {
			return modelID[len("openai/"):]
		}
	case "google", "gemini":
		if strings.HasPrefix(strings.ToLower(modelID), "google/") {
			return modelID[len("google/"):]
		}
	case "deepseek":
		if strings.HasPrefix(strings.ToLower(modelID), "deepseek/") {
			return modelID[len("deepseek/"):]
		}
	}
	return modelID
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
		targetCoding = EffectiveCodingIndex(targetCoding, ctx.TargetTier.ReasoningEffort)
	}
	tolerance := DefaultCodingParityTolerance
	if ctx.Policy != nil && ctx.Policy.CodingParityTolerance > 0 {
		tolerance = ctx.Policy.CodingParityTolerance
	}
	candCoding := EffectiveCodingIndex(ctx.Candidate.CodingIndex, ctx.TargetTier.ReasoningEffort)
	if targetCoding > 0 && candCoding < (targetCoding-tolerance) {
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
	candRate := ResolveComprehensiveRate(ctx.Candidate.PromptCostPerMillion, ctx.Candidate.CompletionCostPerMillion, ctx.Candidate.ComprehensiveRate)
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
	candCoding := EffectiveCodingIndex(ctx.Candidate.CodingIndex, ctx.TargetTier.ReasoningEffort)
	// Escalation Progression: Candidate must not drop below predecessor benchmark
	if ctx.ImmediatePred != nil && !ctx.ImmediatePred.IsDisabled && ctx.ImmediatePred.CodingIndex > 0 {
		if candCoding < ctx.ImmediatePred.CodingIndex {
			return false, "violates_predecessor_escalation"
		}
	}
	// Successor Inversion Guard: Candidate must not leapfrog successor benchmark
	if ctx.ImmediateSucc != nil && !ctx.ImmediateSucc.IsDisabled && ctx.ImmediateSucc.CodingIndex > 0 {
		if candCoding > ctx.ImmediateSucc.CodingIndex {
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
			&ProviderCompatibilityFilter{},
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
	candRate := ResolveComprehensiveRate(candidate.PromptCostPerMillion, candidate.CompletionCostPerMillion, candidate.ComprehensiveRate)
	discountPct := 0.0
	if origRate > 0 && origRate > candRate {
		discountPct = ((origRate - candRate) / origRate) * 100.0
	}
	candCoding := EffectiveCodingIndex(candidate.CodingIndex, target.ReasoningEffort)
	return candCoding * (1.0 + discountPct/100.0)
}

// ResolveComprehensiveRate returns currentRate if positive, or computes blended rate from prompt and completion costs.
func ResolveComprehensiveRate(promptCost, compCost, currentRate float64) float64 {
	if currentRate > 0 {
		return currentRate
	}
	return promptCost + 0.25*compCost
}

// ConvertDealToCandidate maps a contract.DealInfo into an operational CandidateModel.
func ConvertDealToCandidate(d contract.DealInfo) CandidateModel {
	compRate := ResolveComprehensiveRate(d.PromptCostPerM, d.CompletionCostPerM, 0)
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
	compRate := ResolveComprehensiveRate(p.PromptCostPerMillion, p.CompletionCostPerMillion, 0)
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

	addCandidate := func(cand CandidateModel, name string) {
		if cand.ModelID == "" || cand.ModelID == currentModel || seen[cand.ModelID] {
			return
		}
		if IsDisallowedEndpoint(cand.ModelID, name) {
			return
		}
		seen[cand.ModelID] = true
		result = append(result, cand)
	}

	for _, d := range deals {
		addCandidate(ConvertDealToCandidate(d), d.Name)
	}

	curated := curation.DefaultManager().ParetoCandidates(tierRole, isLocal, currentModel, localVRAMGB)
	for _, p := range curated {
		addCandidate(ConvertProfileToCandidate(p), p.Name)
	}

	return result
}

// ConvertTierToCandidate maps a TierReplayConfig into an operational CandidateModel.
func ConvertTierToCandidate(t *TierReplayConfig) CandidateModel {
	if t == nil {
		return CandidateModel{}
	}
	compRate := ResolveComprehensiveRate(t.PromptCostPerMillion, t.CompletionCostPerMillion, t.ComprehensiveRate)
	return CandidateModel{
		ModelID:                  t.Model,
		CodingIndex:              t.CodingIndex,
		ToolReliability:          t.ToolReliability,
		PromptCostPerMillion:     t.PromptCostPerMillion,
		CompletionCostPerMillion: t.CompletionCostPerMillion,
		ComprehensiveRate:        compRate,
		SupportsVision:           t.SupportsVision,
		SupportsTools:            t.SupportsTools,
		IsOpenWeights:            t.IsLocal,
	}
}

// EffectiveCodingIndex scales a model's base coding benchmark by its reasoning effort level.
func EffectiveCodingIndex(baseCoding float64, effort string) float64 {
	if baseCoding <= 0 {
		return baseCoding
	}
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return baseCoding * 1.02
	case "medium", "med":
		return baseCoding * 1.06
	case "high":
		return baseCoding * 1.10
	case "max":
		return baseCoding * 1.14
	default:
		return baseCoding
	}
}
