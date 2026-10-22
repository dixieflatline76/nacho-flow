package curation

import (
	"encoding/json"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// TierRole defines the capability tier archetype for an LLM.
type TierRole string

const (
	RoleVisionWorkhorse TierRole = "vision_workhorse" // Supports image input + fast throughput
	RoleCodingWorkhorse TierRole = "coding_workhorse" // High coding score + tools + large context
	RoleDeepReasoner    TierRole = "deep_reasoner"    // R1 / O1 / thinking tokens
	RoleFastProse       TierRole = "fast_prose"       // Flash / Lite / Haiku / small parameter
	RoleGeneral         TierRole = "general"
)

const BenchmarkSourceEstimated = "estimated"

// ProviderBinding holds platform-specific overrides for a logical model when a
// direct vendor platform uses a different wire ID or price than the canonical (OpenRouter) entry.
type ProviderBinding struct {
	ModelID                  string   `json:"model_id"`
	PromptCostPerMillion     *float64 `json:"prompt_cost_per_million,omitempty"`
	CompletionCostPerMillion *float64 `json:"completion_cost_per_million,omitempty"`
}

// UnmarshalJSON supports both model_id and wire_model_id for provider bindings.
func (b *ProviderBinding) UnmarshalJSON(data []byte) error {
	type Alias ProviderBinding
	aux := struct {
		WireModelID string `json:"wire_model_id"`
		*Alias
	}{
		Alias: (*Alias)(b),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if b.ModelID == "" && aux.WireModelID != "" {
		b.ModelID = aux.WireModelID
	}
	return nil
}

// ModelCuratedProfile stores verified benchmarks, tier recommendations, and tool reliability metrics.
type ModelCuratedProfile struct {
	ModelID                  string                                `json:"model_id,omitempty"`
	CanonicalID              string                                `json:"-"`
	Name                     string                                `json:"name"`
	Aliases                  []string                              `json:"aliases,omitempty"`
	TierRole                 TierRole                              `json:"tier_role"`
	CodingIndex              float64                               `json:"coding_index"`     // Artificial Analysis / SWE-bench index
	ToolReliability          float64                               `json:"tool_reliability"` // 0.0 - 100.0%
	PromptCostPerMillion     float64                               `json:"prompt_cost_per_million,omitempty"`
	CompletionCostPerMillion float64                               `json:"completion_cost_per_million,omitempty"`
	RecommendedTiers         []string                              `json:"recommended_tiers"` // e.g. ["tier_1_vision", "tier_3_workhorse"]
	SupportsVision           bool                                  `json:"supports_vision,omitempty"`
	SupportsTools            bool                                  `json:"supports_tools,omitempty"`
	IsOpenWeights            bool                                  `json:"is_open_weights,omitempty"` // Capable of local execution (Ollama, vLLM, llama.cpp)
	Notes                    string                                `json:"notes,omitempty"`
	BenchmarkSource          string                                `json:"benchmark_source,omitempty"` // e.g. "artificial_analysis"
	ProvenanceURL            string                                `json:"provenance_url,omitempty"`   // e.g. "https://artificialanalysis.ai"
	Providers                map[contract.Platform]ProviderBinding `json:"providers,omitempty"`
}

// IsFrontier reports whether the curated profile represents a true frontier capability class
// based on either deep reasoning role classification or top-tier coding index (>= 70.0).
func (p ModelCuratedProfile) IsFrontier() bool {
	if p.TierRole == RoleDeepReasoner {
		return true
	}
	for _, rTier := range p.RecommendedTiers {
		if rTier == contract.TierIDFrontier && p.CodingIndex >= 70.0 {
			return true
		}
	}
	return false
}

// IsWorkhorse reports whether the model is suitable as an autonomous coding workhorse.
func (p ModelCuratedProfile) IsWorkhorse() bool {
	if p.TierRole == RoleCodingWorkhorse {
		return true
	}
	for _, rTier := range p.RecommendedTiers {
		if rTier == contract.TierIDWorkhorse {
			return true
		}
	}
	return false
}

// MergeDynamicOverlay returns a copy of the curated base profile with live-discovered
// capabilities and prices overlaid, preserving all curated fields (Providers, Aliases,
// Notes, IsOpenWeights, RecommendedTiers, TierRole, ProvenanceURL, and estimated CodingIndex).
func (base ModelCuratedProfile) MergeDynamicOverlay(live ModelCuratedProfile) ModelCuratedProfile {
	base.SupportsVision = base.SupportsVision || live.SupportsVision
	base.SupportsTools = base.SupportsTools || live.SupportsTools
	if live.PromptCostPerMillion > 0 {
		base.PromptCostPerMillion = live.PromptCostPerMillion
	}
	if live.CompletionCostPerMillion > 0 {
		base.CompletionCostPerMillion = live.CompletionCostPerMillion
	}
	if live.CodingIndex > 0 && base.BenchmarkSource != BenchmarkSourceEstimated {
		base.CodingIndex = live.CodingIndex
		if live.BenchmarkSource != "" {
			base.BenchmarkSource = live.BenchmarkSource
		}
	}
	if live.ToolReliability > 0 && base.BenchmarkSource != BenchmarkSourceEstimated {
		base.ToolReliability = live.ToolReliability
	}
	if len(base.RecommendedTiers) == 0 && len(live.RecommendedTiers) > 0 {
		base.RecommendedTiers = live.RecommendedTiers
	}
	if (base.TierRole == "" || base.TierRole == RoleGeneral) && live.TierRole != "" && live.TierRole != RoleGeneral {
		base.TierRole = live.TierRole
	}
	return base
}

// CuratedCatalog represents the canonical JSON structure for all curated model intelligence.
type CuratedCatalog struct {
	Version     string                         `json:"version"` // e.g. "v1.0.0" (Semver)
	UpdatedAt   time.Time                      `json:"updated_at"`
	Description string                         `json:"description"`
	Models      map[string]ModelCuratedProfile `json:"models"`
}
