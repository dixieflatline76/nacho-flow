package curation

import "time"

// TierRole defines the capability tier archetype for an LLM.
type TierRole string

const (
	RoleVisionWorkhorse TierRole = "vision_workhorse" // Supports image input + fast throughput
	RoleCodingWorkhorse TierRole = "coding_workhorse" // High coding score + tools + large context
	RoleDeepReasoner    TierRole = "deep_reasoner"    // R1 / O1 / thinking tokens
	RoleFastProse       TierRole = "fast_prose"       // Flash / Lite / Haiku / small parameter
	RoleGeneral         TierRole = "general"
)

// ModelCuratedProfile stores verified benchmarks, tier recommendations, and tool reliability metrics.
type ModelCuratedProfile struct {
	Name             string   `json:"name"`
	TierRole         TierRole `json:"tier_role"`
	CodingIndex      float64  `json:"coding_index"`      // Artificial Analysis / SWE-bench index
	ToolReliability  float64  `json:"tool_reliability"`  // 0.0 - 100.0%
	RecommendedTiers []string `json:"recommended_tiers"` // e.g. ["tier_1_vision", "tier_3_workhorse"]
	Notes            string   `json:"notes,omitempty"`
}

// CuratedCatalog represents the canonical JSON structure for all curated model intelligence.
type CuratedCatalog struct {
	Version     string                          `json:"version"` // e.g. "v1.0.0" (Semver)
	UpdatedAt   time.Time                       `json:"updated_at"`
	Description string                          `json:"description"`
	Models      map[string]ModelCuratedProfile `json:"models"`
}
