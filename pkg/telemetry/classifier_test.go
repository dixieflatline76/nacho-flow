package telemetry

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

func TestClassifier_Tier1_CuratedGallery(t *testing.T) {
	gallery := curation.NewManager(t.TempDir(), "")
	classifier := NewClassifier(gallery)

	// Case 1: Curated match without live benchmark override
	meta := ModelMetadata{
		ModelPricing: ModelPricing{PromptCostPerMillion: 0.30},
		ModelID:      "google/gemini-2.5-flash",
		Name:         "Gemini 2.5 Flash",
		CodingIndex:  0, // no live API benchmark
	}

	role, codingIdx, recTiers := classifier.ClassifyModel(meta)
	if role != curation.RoleCodingWorkhorse {
		t.Errorf("expected RoleCodingWorkhorse from curated gallery, got %s", role)
	}
	if codingIdx != 78.4 {
		t.Errorf("expected coding index 78.4 from gallery, got %f", codingIdx)
	}
	if len(recTiers) != 2 {
		t.Errorf("expected 2 recommended tiers, got %d", len(recTiers))
	}

	// Case 2: Curated match WITH live benchmark override
	metaWithLiveBench := meta
	metaWithLiveBench.CodingIndex = 85.0
	_, codingIdxOverridden, _ := classifier.ClassifyModel(metaWithLiveBench)
	if codingIdxOverridden != 85.0 {
		t.Errorf("expected live API benchmark 85.0 to override curated 78.4, got %f", codingIdxOverridden)
	}
}

func TestClassifier_Tier2_LiveAPIBenchmark(t *testing.T) {
	// Nil gallery or unknown model with high API benchmark
	classifier := NewClassifier(nil)

	meta := ModelMetadata{
		ModelID:     "uncatalogued/frontier-expert",
		Name:        "Frontier Expert",
		CodingIndex: 88.0, // >= 70.0
	}

	role, codingIdx, recTiers := classifier.ClassifyModel(meta)
	if role != curation.RoleCodingWorkhorse {
		t.Errorf("expected RoleCodingWorkhorse from Tier 2, got %s", role)
	}
	if codingIdx != 88.0 {
		t.Errorf("expected coding index 88.0, got %f", codingIdx)
	}
	if len(recTiers) != 1 || recTiers[0] != "tier_3_workhorse" {
		t.Errorf("expected tier_3_workhorse recommendation, got %v", recTiers)
	}
}

func TestClassifier_Tier3_Heuristics(t *testing.T) {
	classifier := NewClassifier(nil)

	// 1. Vision Workhorse heuristic: vision support + prompt < 0.50
	visionModel := ModelMetadata{
		ModelPricing:   ModelPricing{PromptCostPerMillion: 0.15},
		ModelID:        "some-org/custom-vision-v1",
		SupportsVision: true,
	}
	role, _, recTiers := classifier.ClassifyModel(visionModel)
	if role != curation.RoleVisionWorkhorse {
		t.Errorf("expected RoleVisionWorkhorse, got %s", role)
	}
	if len(recTiers) != 1 || recTiers[0] != "tier_1_vision" {
		t.Errorf("expected tier_1_vision recommendation, got %v", recTiers)
	}

	// 2. Coding Workhorse heuristic: name contains 'coder' + supports tools
	coderModel := ModelMetadata{
		ModelID:       "mistral/codestral-2501",
		SupportsTools: true,
	}
	role, _, _ = classifier.ClassifyModel(coderModel)
	if role != curation.RoleCodingWorkhorse {
		t.Errorf("expected RoleCodingWorkhorse, got %s", role)
	}

	// Coder name WITHOUT tools -> should not be coding workhorse
	coderNoTools := ModelMetadata{
		ModelID:       "mistral/codestral-2501",
		SupportsTools: false,
	}
	role, _, _ = classifier.ClassifyModel(coderNoTools)
	if role == curation.RoleCodingWorkhorse {
		t.Errorf("expected model without tools not to be RoleCodingWorkhorse")
	}

	// 3. Deep Reasoner heuristic: name contains 'r1' / 'reason'
	r1Model := ModelMetadata{
		ModelID: "custom/deepseek-r1-distill",
	}
	role, _, recTiers = classifier.ClassifyModel(r1Model)
	if role != curation.RoleDeepReasoner {
		t.Errorf("expected RoleDeepReasoner, got %s", role)
	}
	if len(recTiers) != 1 || recTiers[0] != "tier_4_frontier" {
		t.Errorf("expected tier_4_frontier recommendation, got %v", recTiers)
	}

	// 4. Fast Prose heuristic: name contains 'flash' / 'lite' / 'mini'
	liteModel := ModelMetadata{
		ModelPricing: ModelPricing{PromptCostPerMillion: 2.0}, // expensive prompt, so vision check won't fire
		ModelID:      "openai/gpt-4o-mini",
	}
	role, _, _ = classifier.ClassifyModel(liteModel)
	if role != curation.RoleFastProse {
		t.Errorf("expected RoleFastProse, got %s", role)
	}

	// 5. General fallback
	generalModel := ModelMetadata{
		ModelID: "meta-llama/llama-3.1-70b-instruct",
	}
	role, _, _ = classifier.ClassifyModel(generalModel)
	if role != curation.RoleGeneral {
		t.Errorf("expected RoleGeneral fallback, got %s", role)
	}
}
