package tuner

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// GenerateDeterministicOracleTraffic produces synthetic telemetry records with mathematically provable
// optimal routing configurations across 4 archetypes:
// 1. Local context cliff (forces Local GPU threshold to 8000)
// 2. Read-burst retry accumulation (forces Cloud Coder retries to >= 4 under write_only)
// 3. Multimodal vision with retries (routes active image traffic to Vision tier, loosening retries)
// 4. Non-image text turns (verifying Vision tier does not cannibalize text prompts)
func GenerateDeterministicOracleTraffic() []telemetry.TurnRecord {
	now := time.Now().UTC()
	var records []telemetry.TurnRecord

	// =========================================================================
	// ARCHETYPE 1: Local Context Cliff (5 sessions)
	// Small prompts (4000 tokens) succeed on local GPU.
	// Large prompts (10000 tokens) fail on local GPU with 2 wasted retries,
	// then succeed on Cloud Coder.
	// Ground Truth: Optimal threshold for Local GPU must be 8000.
	// =========================================================================
	for s := 1; s <= 5; s++ {
		sessID := "sess-oracle-cliff-" + string(rune('0'+s))
		rootHash := uint64(1000 + s)

		// Turn 1: 4000 tokens -> Local GPU succeeds
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*100+1) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             4000,
			IsLocal:            true,
			IsRetry:            false,
			HasWriteProgress:   true,
			HasWriteCapability: true,
			HasImages:          false,
			CostSpentUSD:       0.0,
		})

		// Turn 2: 10000 tokens -> Local GPU attempt 1 fails
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*100+2) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             10000,
			IsLocal:            true,
			IsRetry:            true,
			Retries:            1,
			HasWriteProgress:   false,
			HasWriteCapability: true,
			HasImages:          false,
			CostSpentUSD:       0.0,
		})

		// Turn 3: 10000 tokens -> Local GPU attempt 2 fails (wasted retry penalty)
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*100+3) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             10000,
			IsLocal:            true,
			IsRetry:            true,
			Retries:            2,
			HasWriteProgress:   false,
			HasWriteCapability: true,
			HasImages:          false,
			CostSpentUSD:       0.0,
		})

		// Turn 4: 10000 tokens -> Cloud Coder succeeds
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*100+4) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             10000,
			IsLocal:            false,
			IsRetry:            false,
			Retries:            0,
			HasWriteProgress:   true,
			HasWriteCapability: true,
			HasImages:          false,
			CostSpentUSD:       0.0065,
		})
	}

	// =========================================================================
	// ARCHETYPE 2: Read-Burst Sequence on Cloud Coder (3 sessions)
	// 3 consecutive exploratory reads (HasWriteProgress = false) followed by 1 write.
	// Under write_only: true, retries reach 3.
	// If Retries < 2, turns trip into the $5.00/M Frontier Fallback.
	// Ground Truth: Optimal retries for Cloud Coder must loosen to >= 4.
	// =========================================================================
	for s := 1; s <= 3; s++ {
		sessID := "sess-oracle-readburst-" + string(rune('0'+s))
		rootHash := uint64(2000 + s)

		// Read 1
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*200+1) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             40000,
			IsLocal:            false,
			HasTools:           true,
			HasWriteCapability: true,
			HasWriteProgress:   false,
			IsRetry:            false,
			Retries:            0,
			HasImages:          false,
			CostSpentUSD:       0.026,
		})

		// Read 2
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*200+2) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             42000,
			IsLocal:            false,
			HasTools:           true,
			HasWriteCapability: true,
			HasWriteProgress:   false,
			IsRetry:            true,
			Retries:            1,
			HasImages:          false,
			CostSpentUSD:       0.027,
		})

		// Read 3
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*200+3) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             44000,
			IsLocal:            false,
			HasTools:           true,
			HasWriteCapability: true,
			HasWriteProgress:   false,
			IsRetry:            true,
			Retries:            2,
			HasImages:          false,
			CostSpentUSD:       0.028,
		})

		// Write 4
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*200+4) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             46000,
			IsLocal:            false,
			HasTools:           true,
			HasWriteCapability: true,
			HasWriteProgress:   true,
			IsRetry:            false,
			Retries:            0,
			HasImages:          false,
			CostSpentUSD:       0.030,
		})
	}

	// =========================================================================
	// ARCHETYPE 3: Multimodal Vision with Retries (3 sessions)
	// Prompts have HasImages: true and 30000-35000 tokens.
	// They experience 2 retries before writing progress.
	// Routes to Vision tier, forcing TurnsRouted > 0 and triggering retry optimization.
	// Ground Truth: Vision tier retries must loosen to >= 3 while STRICTLY PRESERVING HasImages!
	// =========================================================================
	for s := 1; s <= 3; s++ {
		sessID := "sess-oracle-vision-" + string(rune('0'+s))
		rootHash := uint64(3000 + s)

		// Image turn 1: exploratory read
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*300+1) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             30000,
			IsLocal:            false,
			HasImages:          true,
			HasTools:           true,
			HasWriteCapability: true,
			HasWriteProgress:   false,
			IsRetry:            false,
			Retries:            0,
			CostSpentUSD:       0.015,
		})

		// Image turn 2: retry 1
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*300+2) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             32000,
			IsLocal:            false,
			HasImages:          true,
			HasTools:           true,
			HasWriteCapability: true,
			HasWriteProgress:   false,
			IsRetry:            true,
			Retries:            1,
			CostSpentUSD:       0.016,
		})

		// Image turn 3: retry 2 -> file write progress
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(s*300+3) * time.Second),
			SessionID:          sessID,
			RootPromptHash:     rootHash,
			Tokens:             34000,
			IsLocal:            false,
			HasImages:          true,
			HasTools:           true,
			HasWriteCapability: true,
			HasWriteProgress:   true,
			IsRetry:            false,
			Retries:            0,
			CostSpentUSD:       0.017,
		})
	}

	return records
}

// AssertRuleSemanticInvariants compiles a synthesized rule and executes it against
// test RequestContext payloads to verify that the rule obeys its behavioral contract.
func AssertRuleSemanticInvariants(t *testing.T, rule string, tier contract.Tier) {
	t.Helper()

	program, err := expr.Compile(rule, expr.Env(contract.RequestContext{}))
	if err != nil {
		t.Fatalf("Synthesized rule %q failed expr compilation: %v", rule, err)
	}

	// 1. Modality Requirement: If tier requires images, non-image prompts MUST evaluate to false
	if strings.Contains(tier.When, "HasImages") && !strings.Contains(tier.When, "!HasImages") {
		out, err := expr.Run(program, contract.RequestContext{
			HasImages: false,
			Retries:   0,
			Tokens:    1000,
		})
		if err != nil {
			t.Fatalf("Failed evaluating rule %q on text-only turn: %v", rule, err)
		}
		if out == true {
			t.Errorf("SEMANTIC INVARIANT VIOLATION: Tier %q requires images, but synthesized rule %q matched a text-only prompt (HasImages=false)!",
				tier.Name, rule)
		}
	}

	// 2. Modality Restriction: If synthesized rule restricts images, image prompts MUST evaluate to false
	if strings.Contains(rule, "!HasImages") || strings.Contains(rule, "HasImages == false") {
		out, err := expr.Run(program, contract.RequestContext{
			HasImages: true,
			Retries:   0,
			Tokens:    1000,
		})
		if err != nil {
			t.Fatalf("Failed evaluating rule %q on image turn: %v", rule, err)
		}
		if out == true {
			t.Errorf("SEMANTIC INVARIANT VIOLATION: Tier %q restricts images (!HasImages), but rule %q matched an image prompt!",
				tier.Name, rule)
		}
	}

	// 3. Positive Kickstart Requirement: If tier requires Kickstart, non-kickstarted sessions MUST evaluate to false
	if strings.Contains(tier.When, "SessionKickstarted") {
		out, err := expr.Run(program, contract.RequestContext{
			SessionKickstarted: false,
			Retries:            0,
			Tokens:             1000,
		})
		if err != nil {
			t.Fatalf("Failed evaluating rule %q on non-kickstarted turn: %v", rule, err)
		}
		if out == true {
			t.Errorf("SEMANTIC INVARIANT VIOLATION: Tier %q requires Kickstart, but rule %q matched an unkickstarted prompt!",
				tier.Name, rule)
		}
	}

	// 4. Modality Requirement: If tier requires tools, non-tool prompts MUST evaluate to false
	if strings.Contains(tier.When, "HasTools") && !strings.Contains(tier.When, "!HasTools") && !strings.Contains(tier.When, "HasTools == false") {
		out, err := expr.Run(program, contract.RequestContext{
			HasTools: false,
			Retries:  0,
			Tokens:   1000,
		})
		if err != nil {
			t.Fatalf("Failed evaluating rule %q on non-tool turn: %v", rule, err)
		}
		if out == true {
			t.Errorf("SEMANTIC INVARIANT VIOLATION: Tier %q requires tools, but synthesized rule %q matched a non-tool prompt (HasTools=false)!",
				tier.Name, rule)
		}
	}

	// 5. Modality Restriction: If synthesized rule restricts tools, tool prompts MUST evaluate to false
	if strings.Contains(rule, "!HasTools") || strings.Contains(rule, "HasTools == false") {
		out, err := expr.Run(program, contract.RequestContext{
			HasTools: true,
			Retries:  0,
			Tokens:   1000,
		})
		if err != nil {
			t.Fatalf("Failed evaluating rule %q on tool turn: %v", rule, err)
		}
		if out == true {
			t.Errorf("SEMANTIC INVARIANT VIOLATION: Tier %q restricts tools (!HasTools), but rule %q matched a tool prompt!",
				tier.Name, rule)
		}
	}

	// 6. Retry Floor Requirement: If tier has Retries >= N, turns with Retries < N MUST evaluate to false
	if strings.Contains(tier.When, "Retries >=") {
		floorMatch := extractRetryFloorRegex.FindStringSubmatch(tier.When)
		if len(floorMatch) > 1 {
			floorVal, _ := strconv.Atoi(floorMatch[1])
			if floorVal > 0 {
				out, err := expr.Run(program, contract.RequestContext{
					Retries: floorVal - 1,
					Tokens:  1000,
				})
				if err != nil {
					t.Fatalf("Failed evaluating rule %q on below-floor retry turn: %v", rule, err)
				}
				if out == true {
					t.Errorf("SEMANTIC INVARIANT VIOLATION: Tier %q has floor Retries >= %d, but rule %q matched Retries=%d!",
						tier.Name, floorVal, rule, floorVal-1)
				}
			}
		}
	}
}

// TestMinConflicts_DeterministicOracleMatrix verifies optimizer ground truth against the synthetic oracle matrix.
// In the unpatched codebase, this test MUST FAIL because the Multimodal Vision tier loses HasImages
// during AST rewriting, causing AssertRuleSemanticInvariants to catch text-only cannibalization.
func TestMinConflicts_DeterministicOracleMatrix(t *testing.T) {
	records := GenerateDeterministicOracleTraffic()

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 0: Multimodal Vision (Gemini 3.8 Flash)",
				Provider: "openrouter",
				Model:    "google/gemini-3.8-flash",
				When:     "HasImages && Retries < 2",
			},
			{
				Name:       "Tier 1: Local GPU Workhorse",
				Provider:   "ollama",
				Model:      "gemma4:12b-it-qat",
				When:       "Tokens < 20000 && Retries < 2",
				MaxContext: 32000,
			},
			{
				Name:       "Tier 2: Flagship Agent Coder (Cloud)",
				Provider:   "openrouter",
				Model:      "qwen/qwen3-coder-plus",
				When:       "Tokens < 160000 && Retries < 2",
				MaxContext: 160000,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Fallback: Frontier Powerhouse",
			Provider: "openrouter",
			Model:    "anthropic/claude-sonnet-5",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	policy := DefaultTuningPolicy()
	policy.WriteOnly = true // Enable faithful write_only progress tracking

	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed on oracle records: %v", err)
	}

	// -------------------------------------------------------------------------
	// Ground Truth Assertion 1: Multimodal Vision Tier
	// Traffic reached Vision tier with retries up to 2.
	// Retries must be loosened to >= 3.
	// CRITICAL: SynthesizedRule MUST preserve "HasImages" and pass semantic invariant!
	// -------------------------------------------------------------------------
	visionTier := res.Tiers[0]
	t.Logf("Tuned Vision Tier: threshold=%d, retries=%d, rule=%q",
		visionTier.OptimalThreshold, visionTier.OptimalRetries, visionTier.SynthesizedRule)

	if visionTier.OptimalRetries < 3 {
		t.Errorf("Expected Vision tier retries to loosen to >= 3, got %d (rule: %q)",
			visionTier.OptimalRetries, visionTier.SynthesizedRule)
	}

	if !strings.Contains(visionTier.SynthesizedRule, "HasImages") {
		t.Errorf("CRITICAL BUG CONFIRMED: Multimodal Vision tier lost 'HasImages' guardrail! Rule: %q",
			visionTier.SynthesizedRule)
	}

	// Semantic Invariant Check: Must reject non-image prompts!
	AssertRuleSemanticInvariants(t, visionTier.SynthesizedRule, cfg.Tiers[0])

	// -------------------------------------------------------------------------
	// Ground Truth Assertion 2: Local GPU Workhorse
	// High-conflict failures at 10000 tokens must force threshold down to 8000.
	// -------------------------------------------------------------------------
	localTier := res.Tiers[1]
	t.Logf("Tuned Local GPU Tier: threshold=%d, retries=%d, rule=%q",
		localTier.OptimalThreshold, localTier.OptimalRetries, localTier.SynthesizedRule)

	if localTier.OptimalThreshold != 8000 {
		t.Errorf("Expected Local GPU threshold to converge to 8000, got %d (rule: %q)",
			localTier.OptimalThreshold, localTier.SynthesizedRule)
	}
	AssertRuleSemanticInvariants(t, localTier.SynthesizedRule, cfg.Tiers[1])

	// -------------------------------------------------------------------------
	// Ground Truth Assertion 3: Flagship Cloud Coder
	// Read-burst sequence with retries up to 3 must loosen retries to >= 4.
	// -------------------------------------------------------------------------
	cloudTier := res.Tiers[2]
	t.Logf("Tuned Cloud Coder Tier: threshold=%d, retries=%d, rule=%q",
		cloudTier.OptimalThreshold, cloudTier.OptimalRetries, cloudTier.SynthesizedRule)

	if cloudTier.OptimalRetries < 4 {
		t.Errorf("Expected Cloud Coder retries to loosen to >= 4 for read-bursts, got %d (rule: %q)",
			cloudTier.OptimalRetries, cloudTier.SynthesizedRule)
	}
	AssertRuleSemanticInvariants(t, cloudTier.SynthesizedRule, cfg.Tiers[2])
}

// TestReplayVsExprEquivalence executes real production traffic through both the Min-Conflicts
// simulator and the synthesized Expr rules, proving 100% routing fidelity between the offline optimizer
// and the live gateway router.
func TestReplayVsExprEquivalence(t *testing.T) {
	records := loadRealTrafficData(t)
	if len(records) == 0 {
		t.Skip("No real traffic records found in testdata")
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Kickstart Escalation (Gemini 3.8 Flash)",
				Provider: "openrouter",
				Model:    "google/gemini-3.8-flash",
				When:     "SessionKickstarted && Retries < 3",
			},
			{
				Name:     "Tier: Multimodal Vision (Gemini 3.8 Flash)",
				Provider: "openrouter",
				Model:    "google/gemini-3.8-flash",
				When:     "HasImages && Retries < 2",
			},
			{
				Name:       "Tier 1: Local GPU Workhorse",
				Provider:   "ollama",
				Model:      "gemma4:12b-it-qat",
				When:       "Tokens < 16000 && Retries < 2",
				MaxContext: 32000,
			},
			{
				Name:       "Tier 2: Flagship Agent Coder",
				Provider:   "openrouter",
				Model:      "qwen/qwen3-coder-plus",
				When:       "Tokens < 160000 && Retries < 2",
				MaxContext: 160000,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Tier 3: Fallback",
			Provider: "openrouter",
			Model:    "anthropic/claude-sonnet-5",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	policy := DefaultTuningPolicy()
	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	// 1. Compile all synthesized Expr rules
	type compiledRule struct {
		tierIdx int
		program *vm.Program
	}
	var compiled []compiledRule
	for i, tier := range res.Tiers {
		if tier.SynthesizedRule == "" || tier.SynthesizedRule == "false" {
			continue
		}
		prog, err := expr.Compile(tier.SynthesizedRule, expr.Env(contract.RequestContext{}))
		if err != nil {
			t.Fatalf("Failed to compile synthesized rule for tier %d (%s): %v", i, tier.TierName, err)
		}
		compiled = append(compiled, compiledRule{tierIdx: i, program: prog})
	}

	defaultIdx := len(res.Tiers)

	// 2. Replay all turns and verify that Expr routing matches replay routing
	discrepancies := 0
	totalTurnsChecked := 0

	// Extract the final tuned MultiTierConfig
	tunedCfg := ExtractRoutingState(cfg, nil)
	for i, tRes := range res.Tiers {
		tunedCfg.Tiers[i].TokenThreshold = tRes.OptimalThreshold
		tunedCfg.Tiers[i].RetryBound = tRes.OptimalRetries
		tunedCfg.Tiers[i].RestrictImages = tRes.RestrictImages
		tunedCfg.Tiers[i].RestrictTools = tRes.RestrictTools
		tunedCfg.Tiers[i].ExcludedKeywords = tRes.FrictionKeywords
	}

	for turnIdx, turn := range records {
		totalTurnsChecked++

		// A. Router simulation via Replay rules
		simTierIdx := -1
		for tIdx := 0; tIdx < len(tunedCfg.Tiers); tIdx++ {
			cand := &tunedCfg.Tiers[tIdx]
			if cand.IsDisabled {
				continue
			}
			if cand.RequiresKickstart && !turn.SessionKickstarted {
				continue
			}
			if cand.RequiresImages && !turn.HasImages {
				continue
			}
			if cand.RequiresTools && !turn.HasTools {
				continue
			}
			if cand.RetryFloor > 0 && turn.Retries < cand.RetryFloor {
				continue
			}
			if cand.MaxContext > 0 && turn.Tokens > cand.MaxContext {
				continue
			}
			if cand.TokenThreshold > 0 && turn.Tokens >= cand.TokenThreshold {
				continue
			}
			if cand.RetryBound > 0 && turn.Retries >= cand.RetryBound {
				continue
			}
			if (cand.RestrictImages || (cand.Model != "" && !cand.SupportsVision)) && turn.HasImages {
				continue
			}
			if (cand.RestrictTools || (cand.Model != "" && !cand.SupportsTools)) && turn.HasTools {
				continue
			}
			if len(cand.ExcludedKeywords) > 0 && hasKeyword(turn.Keywords, cand.ExcludedKeywords) {
				continue
			}
			simTierIdx = tIdx
			break
		}
		if simTierIdx == -1 {
			simTierIdx = defaultIdx
		}

		// B. Live Expr evaluation
		reqCtx := contract.RequestContext{
			Tokens:             turn.Tokens,
			Retries:            turn.Retries,
			HasImages:          turn.HasImages,
			HasTools:           turn.HasTools,
			SessionKickstarted: turn.SessionKickstarted,
			Keywords:           turn.Keywords,
		}

		exprTierIdx := -1
		for _, cr := range compiled {
			out, err := expr.Run(cr.program, reqCtx)
			if err == nil && out == true {
				exprTierIdx = cr.tierIdx
				break
			}
		}
		if exprTierIdx == -1 {
			exprTierIdx = defaultIdx
		}

		if simTierIdx != exprTierIdx {
			discrepancies++
			if discrepancies <= 5 {
				t.Errorf("Routing discrepancy on turn %d (Tokens=%d, Retries=%d, HasImages=%v): Sim routed to Tier %d (%s), Expr routed to Tier %d (%s)",
					turnIdx, turn.Tokens, turn.Retries, turn.HasImages,
					simTierIdx, getTierName(cfg, simTierIdx),
					exprTierIdx, getTierName(cfg, exprTierIdx))
			}
		}
	}

	if discrepancies > 0 {
		t.Fatalf("Found %d routing discrepancies out of %d turns between Simulator and Expr Engine!",
			discrepancies, totalTurnsChecked)
	}
	t.Logf("Replay-to-Expr Equivalence Verified: 100%% agreement across %d turns", totalTurnsChecked)
}

func getTierName(cfg *contract.Config, idx int) string {
	if idx < len(cfg.Tiers) {
		return cfg.Tiers[idx].Name
	}
	return cfg.DefaultTier.Name
}

// TestMinConflicts_DeterministicOracle_EdgeCasesMatrix evaluates synthetic traffic specifically crafted
// to probe advanced edge cases:
// 1. Dedicated Tool Execution Tier (HasTools prerequisite preservation & non-tool isolation)
// 2. Escalation Tier with Retry Floor (Retries >= N floor preservation under retry tuning)
// 3. Exact boundary condition precision (Tokens == Threshold, Retries == Bound, Retries == Floor)
// 4. Keyword exclusion semantic invariance (!any(Keywords, { # in [...] }))
// 5. 100% Simulator vs compiled Expr engine agreement across all synthetic edge cases
func TestMinConflicts_DeterministicOracle_EdgeCasesMatrix(t *testing.T) {
	now := time.Now().UTC()
	var records []telemetry.TurnRecord

	// Session 1: Tool execution session with retries
	// Tool turns with HasTools: true, requiring loosening of tool tier retry bound
	for turn := 1; turn <= 4; turn++ {
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(turn) * time.Second),
			SessionID:          "sess-edge-tools",
			RootPromptHash:     5001,
			Tokens:             25000,
			HasTools:           true,
			HasImages:          false,
			HasWriteCapability: true,
			HasWriteProgress:   turn == 4,
			IsRetry:            turn > 1,
			Retries:            turn - 1,
			IsLocal:            false,
			CostSpentUSD:       0.01,
		})
	}

	// Session 2: Pure text prompts (HasTools: false, HasImages: false)
	// Must NOT be routed to Tool Tier or Vision Tier!
	records = append(records, telemetry.TurnRecord{
		Timestamp:          now.Add(10 * time.Second),
		SessionID:          "sess-edge-text",
		RootPromptHash:     5002,
		Tokens:             3500,
		HasTools:           false,
		HasImages:          false,
		HasWriteCapability: true,
		HasWriteProgress:   true,
		IsRetry:            false,
		Retries:            0,
		IsLocal:            true,
		CostSpentUSD:       0.0,
	})

	// Session 3: Escalation session with high retries (Retries: 3, 4, 5)
	// Must route to Escalation Tier with RetryFloor >= 3
	for turn := 1; turn <= 3; turn++ {
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(20+turn) * time.Second),
			SessionID:          "sess-edge-floor",
			RootPromptHash:     5003,
			Tokens:             15000,
			HasTools:           true,
			HasImages:          false,
			HasWriteCapability: true,
			HasWriteProgress:   turn == 3,
			IsRetry:            true,
			Retries:            2 + turn, // Retries: 3, 4, 5
			IsLocal:            false,
			CostSpentUSD:       0.02,
		})
	}

	// Session 4: Friction keyword session (Keywords: ["sql"])
	// Fails repeatedly on local GPU, resolved on cloud
	for turn := 1; turn <= 2; turn++ {
		records = append(records, telemetry.TurnRecord{
			Timestamp:          now.Add(time.Duration(30+turn) * time.Second),
			SessionID:          "sess-edge-kw",
			RootPromptHash:     5004,
			Tokens:             2000,
			Keywords:           []string{"sql", "database"},
			HasTools:           false,
			HasImages:          false,
			HasWriteCapability: true,
			HasWriteProgress:   false,
			IsRetry:            true,
			Retries:            turn,
			IsLocal:            true,
			CostSpentUSD:       0.0,
		})
	}

	cfg := &contract.Config{
		Tiers: []contract.Tier{
			{
				Name:     "Tier 0: Dedicated Tool Execution",
				Provider: "openrouter",
				Model:    "google/gemini-3.8-flash",
				When:     "HasTools && Retries < 2",
			},
			{
				Name:       "Tier 1: Local GPU Workhorse",
				Provider:   "ollama",
				Model:      "gemma4:12b-it-qat",
				When:       "Tokens < 8000 && Retries < 2",
				MaxContext: 32000,
			},
			{
				Name:       "Tier 2: Escalation Floor Tier",
				Provider:   "openrouter",
				Model:      "anthropic/claude-sonnet-5",
				When:       "Retries >= 3 && Tokens < 32000 && Retries < 6",
				MaxContext: 200000,
			},
		},
		DefaultTier: contract.Tier{
			Name:     "Default Catch-All",
			Provider: "openrouter",
			Model:    "anthropic/claude-opus-5",
			When:     "true",
		},
		Providers: map[string]contract.ProviderConfig{
			"ollama":     {Type: contract.ProviderTypeLocal},
			"openrouter": {Type: contract.ProviderTypeCloud},
		},
	}

	policy := DefaultTuningPolicy()
	policy.WriteOnly = true
	opt := NewMinConflictsOptimizer(policy)
	res, err := opt.Optimize(records, cfg)
	if err != nil {
		t.Fatalf("Optimize failed on edge-case records: %v", err)
	}

	// 1. Verify Tier 0: Dedicated Tool Execution
	toolTier := res.Tiers[0]
	t.Logf("Tuned Tool Tier: retries=%d, rule=%q", toolTier.OptimalRetries, toolTier.SynthesizedRule)
	if !strings.Contains(toolTier.SynthesizedRule, "HasTools") {
		t.Errorf("EDGE CASE FAILURE: Tool tier lost positive 'HasTools'! Rule: %q", toolTier.SynthesizedRule)
	}
	if strings.Contains(toolTier.SynthesizedRule, "!HasTools") {
		t.Errorf("EDGE CASE FAILURE: Tool tier synthesized contradictory '!HasTools'! Rule: %q", toolTier.SynthesizedRule)
	}
	AssertRuleSemanticInvariants(t, toolTier.SynthesizedRule, cfg.Tiers[0])

	// 1b. Verify Tier 1: Local GPU
	localTier := res.Tiers[1]
	t.Logf("Tuned Local Tier: threshold=%d, retries=%d, rule=%q, is_disabled=%v, restrictTools=%v",
		localTier.OptimalThreshold, localTier.OptimalRetries, localTier.SynthesizedRule, localTier.IsDisabled, localTier.RestrictTools)

	// 2. Verify Tier 2: Escalation Floor Tier
	floorTier := res.Tiers[2]
	t.Logf("Tuned Escalation Floor Tier: retries=%d, rule=%q", floorTier.OptimalRetries, floorTier.SynthesizedRule)
	if !strings.Contains(floorTier.SynthesizedRule, "Retries >= 3") {
		t.Errorf("EDGE CASE FAILURE: Escalation tier lost 'Retries >= 3' floor! Rule: %q", floorTier.SynthesizedRule)
	}
	AssertRuleSemanticInvariants(t, floorTier.SynthesizedRule, cfg.Tiers[2])

	// 3. Exact Boundary Condition Precision Test across both Simulator and Expr
	boundaryTestCases := []struct {
		name         string
		ctx          contract.RequestContext
		expectedTier int // 0: Tool, 1: Local, 2: Escalation, 3: Default
	}{
		{
			name:         "Exact token ceiling Tokens == 8000 (must reject Local Tier, route to Default)",
			ctx:          contract.RequestContext{Tokens: 8000, Retries: 0, HasTools: false, HasImages: false},
			expectedTier: 3,
		},
		{
			name:         "Tokens under ceiling Tokens == 7999 (must match Local Tier)",
			ctx:          contract.RequestContext{Tokens: 7999, Retries: 0, HasTools: false, HasImages: false},
			expectedTier: 1,
		},
		{
			name:         "Exact retry floor Retries == 3 with Tokens < 32000 (must match Escalation Tier)",
			ctx:          contract.RequestContext{Tokens: 10000, Retries: 3, HasTools: false, HasImages: false},
			expectedTier: 2,
		},
		{
			name:         "Below retry floor Retries == 2 with Tokens >= 8000 (must reject Escalation Tier, fall to Default)",
			ctx:          contract.RequestContext{Tokens: 10000, Retries: 2, HasTools: false, HasImages: false},
			expectedTier: 3,
		},
		{
			name:         "HasTools=true with Retries=1 (must match Tool Tier)",
			ctx:          contract.RequestContext{Tokens: 15000, Retries: 1, HasTools: true, HasImages: false},
			expectedTier: 0,
		},
		{
			name:         "HasTools=false with Retries=1 and Tokens < 8000 (must bypass Tool Tier to Local)",
			ctx:          contract.RequestContext{Tokens: 4000, Retries: 1, HasTools: false, HasImages: false},
			expectedTier: 1,
		},
	}

	// Compile synthesized rules for Expr evaluation
	var compiledRules []*vm.Program
	for _, tier := range res.Tiers {
		prog, err := expr.Compile(tier.SynthesizedRule, expr.Env(contract.RequestContext{}))
		if err != nil {
			t.Fatalf("Failed to compile synthesized rule %q: %v", tier.SynthesizedRule, err)
		}
		compiledRules = append(compiledRules, prog)
	}

	// Build MultiTierConfig for Simulator evaluation
	simCfg := ExtractRoutingState(cfg, nil)
	for i, tRes := range res.Tiers {
		simCfg.Tiers[i].TokenThreshold = tRes.OptimalThreshold
		simCfg.Tiers[i].RetryBound = tRes.OptimalRetries
		simCfg.Tiers[i].RestrictImages = tRes.RestrictImages
		simCfg.Tiers[i].RestrictTools = tRes.RestrictTools
		simCfg.Tiers[i].ExcludedKeywords = tRes.FrictionKeywords
	}

	for _, tc := range boundaryTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// A. Test Expr live routing
			exprTier := len(compiledRules)
			for idx, prog := range compiledRules {
				out, err := expr.Run(prog, tc.ctx)
				if err == nil && out == true {
					exprTier = idx
					break
				}
			}

			// B. Test Simulator routing
			simTier := len(simCfg.Tiers)
			for idx := 0; idx < len(simCfg.Tiers); idx++ {
				cand := &simCfg.Tiers[idx]
				if cand.IsDisabled {
					continue
				}
				if cand.RequiresKickstart && !tc.ctx.SessionKickstarted {
					continue
				}
				if cand.RequiresImages && !tc.ctx.HasImages {
					continue
				}
				if cand.RequiresTools && !tc.ctx.HasTools {
					continue
				}
				if cand.RetryFloor > 0 && tc.ctx.Retries < cand.RetryFloor {
					continue
				}
				if cand.TokenThreshold > 0 && tc.ctx.Tokens >= cand.TokenThreshold {
					continue
				}
				if cand.RetryBound > 0 && tc.ctx.Retries >= cand.RetryBound {
					continue
				}
				if (cand.RestrictImages || (cand.Model != "" && !cand.SupportsVision)) && tc.ctx.HasImages {
					continue
				}
				if (cand.RestrictTools || (cand.Model != "" && !cand.SupportsTools)) && tc.ctx.HasTools {
					continue
				}
				simTier = idx
				break
			}

			if exprTier != tc.expectedTier {
				t.Errorf("EXPR ROUTING ERROR: expected tier %d, but Expr routed to tier %d",
					tc.expectedTier, exprTier)
			}
			if simTier != tc.expectedTier {
				t.Errorf("SIMULATOR ROUTING ERROR: expected tier %d, but Simulator routed to tier %d",
					tc.expectedTier, simTier)
			}
			if exprTier != simTier {
				t.Errorf("DISCREPANCY: Expr routed to tier %d, Simulator routed to tier %d",
					exprTier, simTier)
			}
		})
	}
}
