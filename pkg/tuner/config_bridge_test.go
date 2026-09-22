package tuner

import (
	"os"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestResolveModelRates_DataDriven(t *testing.T) {
	// 1. Local models MUST always resolve to zero cost regardless of model ID
	localModels := []string{
		"anthropic/claude-sonnet-5",
		"ollama/gemma4:12b-it-qat",
		"ollama/qwen3-coder:next",
		"local/deepseek-v3",
	}
	for _, lm := range localModels {
		pCost, cCost, cRate := ResolveModelRates(lm, true)
		if pCost != 0 || cCost != 0 || cRate != 0 {
			t.Errorf("Expected local model %s to have 0 rates, got: %f, %f, %f", lm, pCost, cCost, cRate)
		}
	}

	// 2. Cloud models dynamically resolve prompt and completion rates from live catalog
	cloudModels := []string{
		"anthropic/claude-sonnet-5",
		"anthropic/claude-opus-5",
		"google/gemini-2.5-flash",
		"deepseek/deepseek-chat-v3-0324",
	}

	for _, cm := range cloudModels {
		p, c, r := ResolveModelRates(cm, false)
		if p <= 0 {
			t.Errorf("Model %s: expected positive prompt cost, got %f", cm, p)
		}
		if c <= 0 {
			t.Errorf("Model %s: expected positive completion cost, got %f", cm, c)
		}
		expectedRate := p + 0.25*c
		if r != expectedRate {
			t.Errorf("Model %s: expected blended rate %f, got %f", cm, expectedRate, r)
		}
	}

	// 3. Uncatalogued model fallback
	defRate := DefaultTuningPolicy().CostPerMillionCloud
	pUnk, cUnk, rUnk := ResolveModelRates("custom-org/my-unseen-model-v1", false)
	if pUnk != defRate {
		t.Errorf("Expected uncatalogued model prompt to equal defRate %f, got %f", defRate, pUnk)
	}
	if cUnk != defRate*4.0 {
		t.Errorf("Expected uncatalogued model comp to equal %f, got %f", defRate*4.0, cUnk)
	}
	if rUnk != defRate*2.0 {
		t.Errorf("Expected uncatalogued model blended rate to equal %f, got %f", defRate*2.0, rUnk)
	}
}

func TestResolveModelBenchmark_DataDriven(t *testing.T) {
	// 1. Known catalog models retrieve verified benchmarks
	coding, tool := ResolveModelBenchmark("anthropic/claude-sonnet-5")
	if coding <= 50.0 {
		t.Errorf("Expected Claude Sonnet 5 coding index > 50, got %f", coding)
	}
	if tool <= 30.0 {
		t.Errorf("Expected Claude Sonnet 5 tool reliability > 30, got %f", tool)
	}

	// 2. Normalized local model matches catalog architecture without local-specific list
	gemmaCoding, _ := ResolveModelBenchmark("ollama/gemma-4-31b-it:latest")
	if gemmaCoding <= 0 {
		t.Errorf("Expected normalized local gemma4 lookup to resolve positive coding index, got %f", gemmaCoding)
	}

	// 3. Uncatalogued model returns neutral baseline
	codingUnk, toolUnk := ResolveModelBenchmark("completely-unknown-model-xyz")
	if codingUnk != 70.0 || toolUnk != 80.0 {
		t.Errorf("Expected neutral baseline (70.0, 80.0) for uncatalogued model, got: %f, %f", codingUnk, toolUnk)
	}
}

func TestExtractRoutingState_Comprehensive(t *testing.T) {
	// Nil and empty config
	if res := ExtractRoutingState(nil, nil); len(res.Tiers) != 0 {
		t.Errorf("Expected empty result for nil config")
	}
	if res := ExtractRoutingState(&contract.Config{}, nil); len(res.Tiers) != 0 {
		t.Errorf("Expected empty result for config with no tiers")
	}

	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
		Tiers: []contract.Tier{
			{
				Name:       "Tier 1: Local",
				Provider:   "ollama",
				Model:      "qwen2.5-coder:7b",
				MaxContext: 8192,
				When:       "Tokens < 4000 && Retries < 2 && !HasImages && !HasTools",
			},
			{
				Name:     "Tier 2: Cloud Flash",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "Tokens < 32000 && !(Keywords contains 'sql' || Keywords contains \"deadlock\")",
			},
			{
				Name:     "Tier 3: Disabled",
				Provider: "openrouter",
				Model:    "anthropic/claude-3-opus",
				When:     "false",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 4: Default",
			Provider: "openrouter",
			Model:    "anthropic/claude-sonnet-5",
			When:     "true",
		},
	}

	state := ExtractRoutingState(cfg, []string{"sql", "deadlock"})
	if len(state.Tiers) != 3 {
		t.Fatalf("Expected 3 cascade tiers, got %d", len(state.Tiers))
	}

	// Tier 1 checks
	t1 := state.Tiers[0]
	if !t1.IsLocal {
		t.Errorf("Expected Tier 1 to be local")
	}
	if t1.TokenThreshold != 4000 {
		t.Errorf("Expected Tier 1 threshold 4000, got %d", t1.TokenThreshold)
	}
	if t1.RetryBound != 2 {
		t.Errorf("Expected Tier 1 retry bound 2, got %d", t1.RetryBound)
	}
	if !t1.RestrictImages || !t1.RestrictTools {
		t.Errorf("Expected Tier 1 to restrict images and tools")
	}
	if t1.ComprehensiveRate != 0 {
		t.Errorf("Expected Tier 1 local cost to be 0")
	}

	// Tier 2 checks
	t2 := state.Tiers[1]
	if t2.IsLocal {
		t.Errorf("Expected Tier 2 to be cloud")
	}
	if t2.TokenThreshold != 32000 {
		t.Errorf("Expected Tier 2 threshold 32000, got %d", t2.TokenThreshold)
	}
	if len(t2.ExcludedKeywords) != 2 {
		t.Errorf("Expected 2 excluded keywords ('sql', 'deadlock'), got: %v", t2.ExcludedKeywords)
	}
	if t2.ComprehensiveRate <= 0 {
		t.Errorf("Expected positive ComprehensiveRate for Tier 2")
	}

	// Tier 3 checks (disabled)
	t3 := state.Tiers[2]
	if !t3.IsDisabled {
		t.Errorf("Expected Tier 3 to be marked disabled")
	}

	// Default Tier checks
	def := state.DefaultTier
	if def.IsLocal {
		t.Errorf("Expected Default Tier to be cloud")
	}
	if def.CodingIndex < 50.0 {
		t.Errorf("Expected Default Tier Claude Sonnet 5 coding index >= 50.0, got %f", def.CodingIndex)
	}
}

func TestExtractRoutingState_LocalFallbacks(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama": {Type: contract.ProviderTypeLocal},
		},
		Tiers: []contract.Tier{
			{
				Name:       "Local Tier With MaxContext",
				Provider:   "ollama",
				Model:      "qwen2.5-coder:7b",
				MaxContext: 8000,
				When:       "true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Provider: "ollama",
			Model:    "qwen2.5-coder:7b",
			When:     "true",
		},
	}
	state := ExtractRoutingState(cfg, nil)
	if len(state.Tiers) != 1 {
		t.Fatalf("Expected 1 tier")
	}
	t1 := state.Tiers[0]
	if t1.TokenThreshold != 8000 {
		t.Errorf("Expected fallback threshold 8000 from MaxContext, got %d", t1.TokenThreshold)
	}
	if t1.RetryBound != 2 {
		t.Errorf("Expected fallback retry bound 2, got %d", t1.RetryBound)
	}
}

func TestNoHardcodedModelSwitchesInTunerSource(t *testing.T) {
	files := []string{"config_bridge.go", "fleet_dominance.go", "min_conflicts.go"}
	forbiddenLiterals := []string{
		"\"opus\"", "\"sonnet\"", "\"haiku\"", "\"deepseek\"", "\"qwen\"",
		"\"gemini\"", "\"gpt-4\"", "\"gpt-5\"", "\"o1\"", "\"o3\"", "\"r1\"",
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", file, err)
		}
		src := string(content)

		if file == "config_bridge.go" {
			if strings.Contains(src, "switch ") || strings.Contains(src, "switch{") {
				t.Errorf("Maintenance nightmare detected: %s still contains hardcoded switch statements!", file)
			}
		}

		for _, lit := range forbiddenLiterals {
			if strings.Contains(src, lit) {
				t.Errorf("Maintenance nightmare detected: %s contains hardcoded model literal %s! Use catalog metadata (IsFrontier, SupportsVision, SupportsTools) instead.", file, lit)
			}
		}
	}
}

func TestExtractRoutingState_PositivePrerequisites(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: "ollama"},
			"openrouter": {Type: "openrouter"},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 0: Kickstart",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "SessionKickstarted && Retries < 3",
			},
			{
				Name:     "Tier 1: Multimodal Vision",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "HasImages && Retries < 2",
			},
			{
				Name:     "Tier 2: Escalation",
				Provider: "openrouter",
				Model:    "deepseek/deepseek-chat",
				When:     "Retries >= 5 && Retries < 8",
			},
			{
				Name:     "Tier 3: Local GPU",
				Provider: "ollama",
				Model:    "qwen2.5-coder:7b",
				When:     "Tokens < 20000 && Retries < 2",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Provider: "openrouter",
			Model:    "google/gemini-2.5-flash",
			When:     "true",
		},
	}

	state := ExtractRoutingState(cfg, nil)
	if len(state.Tiers) != 4 {
		t.Fatalf("Expected 4 tiers, got %d", len(state.Tiers))
	}

	// Tier 0: Kickstart
	t0 := state.Tiers[0]
	if !t0.RequiresKickstart {
		t.Errorf("Expected Tier 0 RequiresKickstart to be true")
	}
	if t0.RequiresImages {
		t.Errorf("Expected Tier 0 RequiresImages to be false")
	}
	if t0.RetryBound != 3 {
		t.Errorf("Expected Tier 0 RetryBound 3, got %d", t0.RetryBound)
	}

	// Tier 1: Multimodal Vision
	t1 := state.Tiers[1]
	if !t1.RequiresImages {
		t.Errorf("Expected Tier 1 RequiresImages to be true")
	}
	if t1.RequiresKickstart {
		t.Errorf("Expected Tier 1 RequiresKickstart to be false")
	}
	if t1.RetryBound != 2 {
		t.Errorf("Expected Tier 1 RetryBound 2, got %d", t1.RetryBound)
	}

	// Tier 2: Escalation with RetryFloor
	t2 := state.Tiers[2]
	if t2.RetryFloor != 5 {
		t.Errorf("Expected Tier 2 RetryFloor 5, got %d", t2.RetryFloor)
	}
	if t2.RetryBound != 8 {
		t.Errorf("Expected Tier 2 RetryBound 8, got %d", t2.RetryBound)
	}

	// Tier 3: Local GPU
	t3 := state.Tiers[3]
	if t3.RequiresKickstart || t3.RequiresImages || t3.RetryFloor != 0 {
		t.Errorf("Expected Tier 3 standard guards, got Kickstart=%v, Images=%v, Floor=%d", t3.RequiresKickstart, t3.RequiresImages, t3.RetryFloor)
	}
	if t3.TokenThreshold != 20000 {
		t.Errorf("Expected Tier 3 TokenThreshold 20000, got %d", t3.TokenThreshold)
	}
}

func TestExtractRoutingState_SyntaxAndWhitespaceVariants(t *testing.T) {
	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {Type: "openrouter"},
		},
		Tiers: []contract.Tier{
			{
				Name:     "Tier 0: Spaced Negations",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "Tokens < 5000 && ! HasImages && ! HasTools",
			},
			{
				Name:     "Tier 1: Equality Negations",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "HasImages == false && HasTools == false",
			},
			{
				Name:     "Tier 2: Explicit True Equalities",
				Provider: "openrouter",
				Model:    "google/gemini-2.5-flash",
				When:     "SessionKickstarted == true && HasImages == true",
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default",
			Provider: "openrouter",
			Model:    "google/gemini-2.5-flash",
			When:     "true",
		},
	}

	state := ExtractRoutingState(cfg, nil)
	if len(state.Tiers) != 3 {
		t.Fatalf("Expected 3 tiers, got %d", len(state.Tiers))
	}

	// Tier 0: Spaced Negations (! HasImages, ! HasTools)
	t0 := state.Tiers[0]
	if !t0.RestrictImages {
		t.Errorf("Expected Tier 0 RestrictImages to be true for '! HasImages'")
	}
	if t0.RequiresImages {
		t.Errorf("Expected Tier 0 RequiresImages to be false for '! HasImages'")
	}
	if !t0.RestrictTools {
		t.Errorf("Expected Tier 0 RestrictTools to be true for '! HasTools'")
	}

	// Tier 1: Equality Negations (HasImages == false, HasTools == false)
	t1 := state.Tiers[1]
	if !t1.RestrictImages {
		t.Errorf("Expected Tier 1 RestrictImages to be true for 'HasImages == false'")
	}
	if t1.RequiresImages {
		t.Errorf("Expected Tier 1 RequiresImages to be false for 'HasImages == false'")
	}
	if !t1.RestrictTools {
		t.Errorf("Expected Tier 1 RestrictTools to be true for 'HasTools == false'")
	}

	// Tier 2: Explicit True Equalities (SessionKickstarted == true, HasImages == true)
	t2 := state.Tiers[2]
	if !t2.RequiresKickstart {
		t.Errorf("Expected Tier 2 RequiresKickstart to be true for 'SessionKickstarted == true'")
	}
	if !t2.RequiresImages {
		t.Errorf("Expected Tier 2 RequiresImages to be true for 'HasImages == true'")
	}
	if t2.RestrictImages {
		t.Errorf("Expected Tier 2 RestrictImages to be false for 'HasImages == true'")
	}
}

func TestExtractRoutingState_ToolsAndVisionFlags(t *testing.T) {
	bTrue := true
	bFalse := false

	cfg := &contract.Config{
		Providers: map[string]contract.ProviderConfig{
			"openrouter": {Type: "openrouter"},
		},
		Tiers: []contract.Tier{
			{
				Name:      "Tier 0: Vision and Tools explicitly enabled",
				Provider:  "openrouter",
				Model:     "google/gemini-2.5-flash",
				HasVision: &bTrue,
				HasTools:  &bTrue,
				When:      "Retries > 2", // Tests extractRetryFloorGtRegex
			},
			{
				Name:        "Tier 1: Stripped images and tools",
				Provider:    "openrouter",
				Model:       "google/gemini-2.5-flash",
				StripImages: true,
				StripTools:  true,
				When:        "Tokens < 1000",
			},
			{
				Name:      "Tier 2: False flags",
				Provider:  "openrouter",
				Model:     "google/gemini-2.5-flash",
				HasVision: &bFalse,
				HasTools:  &bFalse,
				When:      "Tokens < 2000",
			},
		},
		DefaultTier: contract.Tier{
			Name:        "Default",
			Provider:    "openrouter",
			Model:       "google/gemini-2.5-flash",
			StripImages: true,
			StripTools:  true,
			When:        "true",
		},
	}

	state := ExtractRoutingState(cfg, nil)
	if len(state.Tiers) != 3 {
		t.Fatalf("expected 3 tiers")
	}

	// Tier 0
	if !state.Tiers[0].SupportsVision || !state.Tiers[0].SupportsTools {
		t.Errorf("expected Tier 0 to support vision and tools")
	}
	if state.Tiers[0].RetryFloor != 3 {
		t.Errorf("expected RetryFloor 3 from 'Retries > 2', got %d", state.Tiers[0].RetryFloor)
	}

	// Tier 1
	if state.Tiers[1].SupportsVision || state.Tiers[1].SupportsTools {
		t.Errorf("expected Tier 1 to have vision and tools stripped")
	}

	// Tier 2
	if state.Tiers[2].SupportsVision || state.Tiers[2].SupportsTools {
		t.Errorf("expected Tier 2 to have false vision and tools")
	}

	// Default tier
	if state.DefaultTier.SupportsVision || state.DefaultTier.SupportsTools {
		t.Errorf("expected Default tier to have vision and tools stripped")
	}
}

func TestCheckHardConstraints_EdgeCases(t *testing.T) {
	if CheckHardConstraints(nil) {
		t.Errorf("expected CheckHardConstraints(nil) == false")
	}
	validCfg := &MultiTierConfig{
		Tiers: []TierReplayConfig{
			{TierName: "T1", TokenThreshold: 4000, MaxContext: 8000},
			{TierName: "T2", TokenThreshold: 8000, MaxContext: 16000},
		},
		DefaultTier: TierReplayConfig{TierName: "T3"},
	}
	if !CheckHardConstraints(validCfg) {
		t.Errorf("expected valid config to pass CheckHardConstraints")
	}
}
