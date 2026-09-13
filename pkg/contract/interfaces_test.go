package contract_test

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestProviderConfig_IsLocal(t *testing.T) {
	tests := []struct {
		name     string
		p        contract.ProviderConfig
		expected bool
	}{
		{
			name:     "local provider",
			p:        contract.ProviderConfig{Type: contract.ProviderTypeLocal},
			expected: true,
		},
		{
			name:     "cloud provider",
			p:        contract.ProviderConfig{Type: contract.ProviderTypeCloud},
			expected: false,
		},
		{
			name:     "empty type",
			p:        contract.ProviderConfig{Type: ""},
			expected: false,
		},
		{
			name:     "arbitrary unknown type",
			p:        contract.ProviderConfig{Type: "other"},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.IsLocal(); got != tc.expected {
				t.Errorf("ProviderConfig.IsLocal() = %v, expected %v", got, tc.expected)
			}
		})
	}
}

func TestRequestContext_IsModelCoolingDown(t *testing.T) {
	rc := contract.RequestContext{
		CoolingDownModels: []string{"openai/gpt-4o", "anthropic/claude-3-5-sonnet"},
	}

	if rc.IsModelCoolingDown("") {
		t.Errorf("expected empty model to not be cooling down")
	}

	emptyRC := contract.RequestContext{}
	if emptyRC.IsModelCoolingDown("openai/gpt-4o") {
		t.Errorf("expected model to not be cooling down on empty RC")
	}

	if !rc.IsModelCoolingDown("OPENAI/GPT-4O") {
		t.Errorf("expected case-insensitive match for cooling down model")
	}

	if rc.IsModelCoolingDown("google/gemini-pro") {
		t.Errorf("expected non-cooling down model to return false")
	}
}

func TestCycleBreakerConfig_ResolveMethods(t *testing.T) {
	// 1. Defaults when all fields are empty
	empty := &contract.CycleBreakerConfig{}
	if empty.ResolvePhraseLength() != 6 {
		t.Errorf("expected default phrase length 6, got %d", empty.ResolvePhraseLength())
	}
	if empty.ResolveBudgetMaxRepeats() != 5 {
		t.Errorf("expected default budget max repeats 5, got %d", empty.ResolveBudgetMaxRepeats())
	}
	if empty.ResolveThinkingMaxTokens() != 4096 {
		t.Errorf("expected default thinking max tokens 4096, got %d", empty.ResolveThinkingMaxTokens())
	}
	if empty.ResolveThinkingMaxRepeats() != 6 {
		t.Errorf("expected default thinking max repeats 6, got %d", empty.ResolveThinkingMaxRepeats())
	}
	if empty.ResolveThinkingPhraseLength() != 6 {
		t.Errorf("expected thinking phrase length 6, got %d", empty.ResolveThinkingPhraseLength())
	}
	if empty.ResolveThinkingBudgetMaxRepeats() != 5 {
		t.Errorf("expected thinking budget max repeats 5, got %d", empty.ResolveThinkingBudgetMaxRepeats())
	}

	if empty.ResolveContentMaxTokens() != 6144 {
		t.Errorf("expected default content max tokens 6144, got %d", empty.ResolveContentMaxTokens())
	}
	if empty.ResolveContentMaxRepeats() != 8 {
		t.Errorf("expected default content max repeats 8, got %d", empty.ResolveContentMaxRepeats())
	}
	if empty.ResolveContentPhraseLength() != 6 {
		t.Errorf("expected content phrase length 6, got %d", empty.ResolveContentPhraseLength())
	}
	if empty.ResolveContentBudgetMaxRepeats() != 5 {
		t.Errorf("expected content budget max repeats 5, got %d", empty.ResolveContentBudgetMaxRepeats())
	}

	if empty.ResolveToolMaxTokens() != 8192 {
		t.Errorf("expected default tool max tokens 8192, got %d", empty.ResolveToolMaxTokens())
	}
	if empty.ResolveToolMaxWriteTokens() != 32768 {
		t.Errorf("expected default tool max write tokens 32768, got %d", empty.ResolveToolMaxWriteTokens())
	}
	if empty.ResolveToolMaxRepeats() != 8 {
		t.Errorf("expected default tool max repeats 8, got %d", empty.ResolveToolMaxRepeats())
	}
	if empty.ResolveToolPhraseLength() != 6 {
		t.Errorf("expected tool phrase length 6, got %d", empty.ResolveToolPhraseLength())
	}
	if empty.ResolveToolBudgetMaxRepeats() != 5 {
		t.Errorf("expected tool budget max repeats 5, got %d", empty.ResolveToolBudgetMaxRepeats())
	}

	// 2. Global fallback overrides
	global := &contract.CycleBreakerConfig{
		PhraseLength:                       4,
		BudgetMaxRepeats:                   3,
		MaxThinkingTokens:                  2048,
		ThinkingRepetitionThreshold:        4,
		ThinkingBudgetRepetitionThreshold: 2,
		MaxContentTokens:                     3072,
		RepetitionThreshold:                5,
		MaxToolTokens:                      4096,
		MaxWriteTokens:                     16384,
	}
	if global.ResolvePhraseLength() != 4 {
		t.Errorf("expected phrase length 4, got %d", global.ResolvePhraseLength())
	}
	if global.ResolveBudgetMaxRepeats() != 3 {
		t.Errorf("expected budget max repeats 3, got %d", global.ResolveBudgetMaxRepeats())
	}
	if global.ResolveThinkingMaxTokens() != 2048 {
		t.Errorf("expected 2048, got %d", global.ResolveThinkingMaxTokens())
	}
	if global.ResolveThinkingMaxRepeats() != 4 {
		t.Errorf("expected 4, got %d", global.ResolveThinkingMaxRepeats())
	}
	if global.ResolveThinkingBudgetMaxRepeats() != 2 {
		t.Errorf("expected 2, got %d", global.ResolveThinkingBudgetMaxRepeats())
	}
	if global.ResolveContentMaxTokens() != 3072 {
		t.Errorf("expected 3072, got %d", global.ResolveContentMaxTokens())
	}
	if global.ResolveContentMaxRepeats() != 5 {
		t.Errorf("expected 5, got %d", global.ResolveContentMaxRepeats())
	}
	if global.ResolveToolMaxTokens() != 4096 {
		t.Errorf("expected 4096, got %d", global.ResolveToolMaxTokens())
	}
	if global.ResolveToolMaxWriteTokens() != 16384 {
		t.Errorf("expected 16384, got %d", global.ResolveToolMaxWriteTokens())
	}
	if global.ResolveToolMaxRepeats() != 5 {
		t.Errorf("expected 5, got %d", global.ResolveToolMaxRepeats())
	}

	// 3. Lane-specific overrides taking precedence
	laneConfig := &contract.CycleBreakerConfig{
		PhraseLength:     4,
		BudgetMaxRepeats: 3,
		ThinkingLane: contract.LaneConfig{
			MaxTokens:                 1024,
			MaxRepeats:                2,
			PhraseLength:              5,
			BudgetMaxRepeats:          4,
			RepetitionThreshold:       3,
			BudgetRepetitionThreshold: 2,
		},
		ContentLane: contract.LaneConfig{
			MaxTokens:                 512,
			MaxRepeats:                3,
			PhraseLength:              7,
			BudgetMaxRepeats:          6,
			RepetitionThreshold:       4,
			BudgetRepetitionThreshold: 3,
		},
		ToolLane: contract.ToolLaneConfig{
			LaneConfig: contract.LaneConfig{
				MaxTokens:                 256,
				MaxRepeats:                4,
				PhraseLength:              3,
				BudgetMaxRepeats:          2,
				RepetitionThreshold:       5,
				BudgetRepetitionThreshold: 4,
			},
			MaxWriteTokens: 8192,
		},
	}
	if laneConfig.ResolveThinkingMaxTokens() != 1024 {
		t.Errorf("expected 1024, got %d", laneConfig.ResolveThinkingMaxTokens())
	}
	if laneConfig.ResolveThinkingMaxRepeats() != 2 {
		t.Errorf("expected 2, got %d", laneConfig.ResolveThinkingMaxRepeats())
	}
	if laneConfig.ResolveThinkingPhraseLength() != 5 {
		t.Errorf("expected 5, got %d", laneConfig.ResolveThinkingPhraseLength())
	}
	if laneConfig.ResolveThinkingBudgetMaxRepeats() != 4 {
		t.Errorf("expected 4, got %d", laneConfig.ResolveThinkingBudgetMaxRepeats())
	}

	if laneConfig.ResolveContentMaxTokens() != 512 {
		t.Errorf("expected 512, got %d", laneConfig.ResolveContentMaxTokens())
	}
	if laneConfig.ResolveContentMaxRepeats() != 3 {
		t.Errorf("expected 3, got %d", laneConfig.ResolveContentMaxRepeats())
	}
	if laneConfig.ResolveContentPhraseLength() != 7 {
		t.Errorf("expected 7, got %d", laneConfig.ResolveContentPhraseLength())
	}
	if laneConfig.ResolveContentBudgetMaxRepeats() != 6 {
		t.Errorf("expected 6, got %d", laneConfig.ResolveContentBudgetMaxRepeats())
	}

	if laneConfig.ResolveToolMaxTokens() != 256 {
		t.Errorf("expected 256, got %d", laneConfig.ResolveToolMaxTokens())
	}
	if laneConfig.ResolveToolMaxWriteTokens() != 8192 {
		t.Errorf("expected 8192, got %d", laneConfig.ResolveToolMaxWriteTokens())
	}
	if laneConfig.ResolveToolMaxRepeats() != 4 {
		t.Errorf("expected 4, got %d", laneConfig.ResolveToolMaxRepeats())
	}
	if laneConfig.ResolveToolPhraseLength() != 3 {
		t.Errorf("expected 3, got %d", laneConfig.ResolveToolPhraseLength())
	}
	if laneConfig.ResolveToolBudgetMaxRepeats() != 2 {
		t.Errorf("expected 2, got %d", laneConfig.ResolveToolBudgetMaxRepeats())
	}

	// Test fallback legacy fields inside lane config
	legacyLane := &contract.CycleBreakerConfig{
		RepetitionWindow:          8,
		BudgetRepetitionThreshold: 7,
	}
	legacyLaneArg := &contract.LaneConfig{
		RepetitionWindow:          9,
		BudgetRepetitionThreshold: 8,
	}
	if legacyLane.ResolvePhraseLength(legacyLaneArg) != 9 {
		t.Errorf("expected 9, got %d", legacyLane.ResolvePhraseLength(legacyLaneArg))
	}
	if legacyLane.ResolveBudgetMaxRepeats(legacyLaneArg) != 8 {
		t.Errorf("expected 8, got %d", legacyLane.ResolveBudgetMaxRepeats(legacyLaneArg))
	}
	if legacyLane.ResolvePhraseLength() != 8 {
		t.Errorf("expected 8, got %d", legacyLane.ResolvePhraseLength())
	}
	if legacyLane.ResolveBudgetMaxRepeats() != 7 {
		t.Errorf("expected 7, got %d", legacyLane.ResolveBudgetMaxRepeats())
	}

	// Test ReasoningLane aliases and ContentLane threshold overrides
	reasoningCfg := &contract.CycleBreakerConfig{
		ReasoningLane: contract.LaneConfig{
			MaxTokens:           3000,
			MaxRepeats:          4,
			PhraseLength:        5,
			BudgetMaxRepeats:    3,
			RepetitionThreshold: 4,
		},
		ContentLane: contract.LaneConfig{
			RepetitionThreshold: 7,
		},
		ToolLane: contract.ToolLaneConfig{
			LaneConfig: contract.LaneConfig{
				RepetitionThreshold: 6,
			},
		},
	}
	if reasoningCfg.ResolveThinkingMaxTokens() != 3000 {
		t.Errorf("expected 3000, got %d", reasoningCfg.ResolveThinkingMaxTokens())
	}
	if reasoningCfg.ResolveThinkingMaxRepeats() != 4 {
		t.Errorf("expected 4, got %d", reasoningCfg.ResolveThinkingMaxRepeats())
	}
	if reasoningCfg.ResolveThinkingPhraseLength() != 5 {
		t.Errorf("expected 5, got %d", reasoningCfg.ResolveThinkingPhraseLength())
	}
	if reasoningCfg.ResolveThinkingBudgetMaxRepeats() != 3 {
		t.Errorf("expected 3, got %d", reasoningCfg.ResolveThinkingBudgetMaxRepeats())
	}
	if reasoningCfg.ResolveContentMaxRepeats() != 7 {
		t.Errorf("expected 7, got %d", reasoningCfg.ResolveContentMaxRepeats())
	}
	if reasoningCfg.ResolveToolMaxRepeats() != 6 {
		t.Errorf("expected 6, got %d", reasoningCfg.ResolveToolMaxRepeats())
	}

	// Test RepetitionThreshold fallback in ReasoningLane
	reasoningFallback := &contract.CycleBreakerConfig{
		ReasoningLane: contract.LaneConfig{
			RepetitionThreshold:       5,
			RepetitionWindow:          7,
			BudgetRepetitionThreshold: 4,
		},
	}
	if reasoningFallback.ResolveThinkingMaxRepeats() != 5 {
		t.Errorf("expected 5, got %d", reasoningFallback.ResolveThinkingMaxRepeats())
	}
	if reasoningFallback.ResolveThinkingPhraseLength() != 7 {
		t.Errorf("expected 7, got %d", reasoningFallback.ResolveThinkingPhraseLength())
	}
	if reasoningFallback.ResolveThinkingBudgetMaxRepeats() != 4 {
		t.Errorf("expected 4, got %d", reasoningFallback.ResolveThinkingBudgetMaxRepeats())
	}
}

func TestCycleBreakerConfig_Normalize(t *testing.T) {
	var nilConfig *contract.CycleBreakerConfig
	nilConfig.Normalize() // should not panic

	// 1. Flat fields to structured lane fields
	cfg1 := &contract.CycleBreakerConfig{
		MaxThinkingTokens:           2048,
		MaxContentTokens:              4096,
		MaxToolTokens:               8192,
		MaxWriteTokens:              16384,
		PhraseLength:                5,
		ThinkingRepetitionThreshold: 4,
		RepetitionThreshold:         7,
		BudgetMaxRepeats:            3,
	}
	cfg1.Normalize()
	if cfg1.ThinkingLane.MaxTokens != 2048 {
		t.Errorf("expected ThinkingLane.MaxTokens 2048, got %d", cfg1.ThinkingLane.MaxTokens)
	}
	if cfg1.ContentLane.MaxTokens != 4096 {
		t.Errorf("expected ContentLane.MaxTokens 4096, got %d", cfg1.ContentLane.MaxTokens)
	}
	if cfg1.ToolLane.MaxTokens != 8192 {
		t.Errorf("expected ToolLane.MaxTokens 8192, got %d", cfg1.ToolLane.MaxTokens)
	}
	if cfg1.ToolLane.MaxWriteTokens != 16384 {
		t.Errorf("expected ToolLane.MaxWriteTokens 16384, got %d", cfg1.ToolLane.MaxWriteTokens)
	}
	if cfg1.RepetitionWindow != 5 {
		t.Errorf("expected RepetitionWindow 5, got %d", cfg1.RepetitionWindow)
	}
	if cfg1.ThinkingLane.MaxRepeats != 4 {
		t.Errorf("expected ThinkingLane.MaxRepeats 4, got %d", cfg1.ThinkingLane.MaxRepeats)
	}
	if cfg1.ContentLane.MaxRepeats != 7 {
		t.Errorf("expected ContentLane.MaxRepeats 7, got %d", cfg1.ContentLane.MaxRepeats)
	}
	if cfg1.BudgetRepetitionThreshold != 3 {
		t.Errorf("expected BudgetRepetitionThreshold 3, got %d", cfg1.BudgetRepetitionThreshold)
	}

	// 2. Structured lane fields to flat fields
	cfg2 := &contract.CycleBreakerConfig{
		ThinkingLane: contract.LaneConfig{
			MaxTokens:  1024,
			MaxRepeats: 3,
		},
		ContentLane: contract.LaneConfig{
			MaxTokens:  2048,
			MaxRepeats: 5,
		},
		ToolLane: contract.ToolLaneConfig{
			LaneConfig: contract.LaneConfig{
				MaxTokens: 4096,
			},
			MaxWriteTokens: 8192,
		},
		RepetitionWindow:          6,
		BudgetRepetitionThreshold: 4,
	}
	cfg2.Normalize()
	if cfg2.MaxThinkingTokens != 1024 {
		t.Errorf("expected MaxThinkingTokens 1024, got %d", cfg2.MaxThinkingTokens)
	}
	if cfg2.MaxContentTokens != 2048 {
		t.Errorf("expected MaxContentTokens 2048, got %d", cfg2.MaxContentTokens)
	}
	if cfg2.MaxToolTokens != 4096 {
		t.Errorf("expected MaxToolTokens 4096, got %d", cfg2.MaxToolTokens)
	}
	if cfg2.MaxWriteTokens != 8192 {
		t.Errorf("expected MaxWriteTokens 8192, got %d", cfg2.MaxWriteTokens)
	}
	if cfg2.PhraseLength != 6 {
		t.Errorf("expected PhraseLength 6, got %d", cfg2.PhraseLength)
	}
	if cfg2.ThinkingRepetitionThreshold != 3 {
		t.Errorf("expected ThinkingRepetitionThreshold 3, got %d", cfg2.ThinkingRepetitionThreshold)
	}
	if cfg2.RepetitionThreshold != 5 {
		t.Errorf("expected RepetitionThreshold 5, got %d", cfg2.RepetitionThreshold)
	}
	if cfg2.BudgetMaxRepeats != 4 {
		t.Errorf("expected BudgetMaxRepeats 4, got %d", cfg2.BudgetMaxRepeats)
	}

	// 3. ReasoningLane normalization
	cfg3 := &contract.CycleBreakerConfig{
		ReasoningLane: contract.LaneConfig{
			MaxTokens: 1234,
		},
	}
	cfg3.Normalize()
	if cfg3.ThinkingLane.MaxTokens != 1234 {
		t.Errorf("expected ThinkingLane.MaxTokens 1234, got %d", cfg3.ThinkingLane.MaxTokens)
	}
}

func TestConfig_Normalize(t *testing.T) {
	var nilCfg *contract.Config
	nilCfg.Normalize() // should not panic

	fullCfg := &contract.Config{
		CycleKiller: contract.CycleBreakerConfig{
			MaxThinkingTokens: 2048,
		},
		CycleBreaker: contract.CycleBreakerConfig{
			MaxContentTokens: 4096,
		},
		Tiers: []contract.Tier{
			{
				Name: "Tier1",
				CycleKiller: &contract.CycleBreakerConfig{
					MaxToolTokens: 8192,
				},
				CycleBreaker: &contract.CycleBreakerConfig{
					MaxWriteTokens: 16384,
				},
			},
		},
	}
	fullCfg.Normalize()
	if fullCfg.CycleKiller.ThinkingLane.MaxTokens != 2048 {
		t.Errorf("expected normalized CycleKiller")
	}
	if fullCfg.CycleBreaker.ContentLane.MaxTokens != 4096 {
		t.Errorf("expected normalized CycleBreaker")
	}
	if fullCfg.Tiers[0].CycleKiller.ToolLane.MaxTokens != 8192 {
		t.Errorf("expected normalized Tier CycleKiller")
	}
	if fullCfg.Tiers[0].CycleBreaker.ToolLane.MaxWriteTokens != 16384 {
		t.Errorf("expected normalized Tier CycleBreaker")
	}
}

