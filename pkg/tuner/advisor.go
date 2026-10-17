package tuner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// GenerateAdvisoryReport creates a human-readable CLI report from TuningResult using the AdvisoryReportBuilder.
func GenerateAdvisoryReport(res *TuningResult, cfg *contract.Config) string {
	return NewAdvisoryReportBuilder(res, cfg).
		WithBanner().
		WithSampleSize().
		WithSessionDynamics().
		WithContextInflationAudit().
		WithModelRecovery().
		WithDominanceConflicts().
		WithTierPolicies().
		WithFallbackPolicy().
		WithFleetImpact().
		WithConfigDiff().
		WithApplyInstructions().
		Build()
}

// AdvisoryReportBuilder constructs human-readable advisory CLI reports.
type AdvisoryReportBuilder struct {
	res *TuningResult
	cfg *contract.Config
	sb  strings.Builder
}

// NewAdvisoryReportBuilder creates a new fluent builder for tuning reports.
func NewAdvisoryReportBuilder(res *TuningResult, cfg *contract.Config) *AdvisoryReportBuilder {
	return &AdvisoryReportBuilder{
		res: res,
		cfg: cfg,
	}
}

// WithBanner renders the main decorative header.
func (b *AdvisoryReportBuilder) WithBanner() *AdvisoryReportBuilder {
	b.sb.WriteString("========================================================================================\n")
	if b.res.TotalSessions > 0 {
		b.sb.WriteString("🌮 NACHO FLOW ADVISORY TUNING REPORT (v2 — Session Replay)\n")
	} else {
		b.sb.WriteString("🌮 NACHO FLOW ADVISORY TUNING REPORT\n")
	}
	b.sb.WriteString("========================================================================================\n\n")
	return b
}

// WithSampleSize renders the number of historical prompt turns analyzed.
func (b *AdvisoryReportBuilder) WithSampleSize() *AdvisoryReportBuilder {
	fmt.Fprintf(&b.sb, "📊 Sample Size: %d historical prompt turns evaluated\n", b.res.TotalSampleTurns)
	return b
}

// WithSessionDynamics renders multi-turn session trajectory statistics.
func (b *AdvisoryReportBuilder) WithSessionDynamics() *AdvisoryReportBuilder {
	if b.res.TotalSessions > 0 {
		fmt.Fprintf(&b.sb, "\n🔄 SESSION DYNAMICS:\n"+
			"  • Total Sessions Evaluated:       %d\n"+
			"  • Average Turns per Session:      %.1f\n"+
			"  • Cloud Escalation Rate:          %.1f%%\n",
			b.res.TotalSessions, b.res.AvgTurnsPerSession, b.res.EscalationRate*100)
	} else {
		b.sb.WriteString("\n")
	}
	return b
}

// WithContextInflationAudit renders late-stage context cost inflation and read-burst escalation metrics.
func (b *AdvisoryReportBuilder) WithContextInflationAudit() *AdvisoryReportBuilder {
	if b.res.HighContextEscalationTurns == 0 && b.res.ReadBurstEscalations == 0 {
		return b
	}

	b.sb.WriteString("\n🔥 LATE-CONTEXT COST INFLATION AUDIT:\n")
	if b.res.HighContextEscalationTurns > 0 {
		fmt.Fprintf(&b.sb, "  • High-Context Escalations (>100k tok): %d turns\n", b.res.HighContextEscalationTurns)
		fmt.Fprintf(&b.sb, "  • High-Context Cloud Spend:            $%.2f / $%.2f (%.1f%% of total run cost)\n",
			b.res.HighContextCostUSD, b.res.CurrentCostUSD, b.res.HighContextCostPct)
	}
	if b.res.ReadBurstEscalations > 0 {
		fmt.Fprintf(&b.sb, "  • Read-Burst Escalations:               %d turns triggered by read-only tool sequences\n", b.res.ReadBurstEscalations)
	}
	b.sb.WriteString("  • Replay Engine Fidelity:               write_only progress simulation active\n")
	if b.res.NTSProjectedSavingsUSD > 0 {
		fmt.Fprintf(&b.sb, "  • NTS Context Compaction:               Projected savings ~$%.2f if enabled\n", b.res.NTSProjectedSavingsUSD)
	}
	return b
}

// WithModelRecovery renders the self-recovery rates for models under transient retries.
func (b *AdvisoryReportBuilder) WithModelRecovery() *AdvisoryReportBuilder {
	if len(b.res.RecoveryStats) == 0 {
		return b
	}

	b.sb.WriteString("\n🩺 MODEL SELF-RECOVERY ANALYSIS:\n")
	models := make([]string, 0, len(b.res.RecoveryStats))
	for m := range b.res.RecoveryStats {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		stat := b.res.RecoveryStats[m]
		fmt.Fprintf(&b.sb, "  • %-24s Self-Recovery: %.1f%% (avg %.1f turns to recover)\n",
			stat.Model+":", stat.SelfRecoveryRate*100, stat.AvgTurnsToRecover)
	}
	return b
}

// WithDominanceConflicts renders static routing defects if any were detected.
func (b *AdvisoryReportBuilder) WithDominanceConflicts() *AdvisoryReportBuilder {
	if len(b.res.StaticDominanceConflicts) == 0 {
		return b
	}

	b.sb.WriteString("\n⚠️ STATIC ROUTING DOMINANCE DEFECTS DETECTED:\n")
	for _, c := range b.res.StaticDominanceConflicts {
		fmt.Fprintf(&b.sb, "  • %s\n", c.Reason)
	}
	return b
}

// WithTierPolicies renders the configured and recommended tier guardrails.
func (b *AdvisoryReportBuilder) WithTierPolicies() *AdvisoryReportBuilder {
	b.sb.WriteString("\n🔍 MULTI-TIER ROUTING POLICIES:\n")
	for i, tier := range b.res.Tiers {
		fmt.Fprintf(&b.sb, "\n  [Tier %d: %s]\n", i+1, tier.TierName)
		if tier.OptimalThreshold > 0 {
			fmt.Fprintf(&b.sb, "  • Context Threshold:   %d tokens\n", tier.OptimalThreshold)
		} else {
			b.sb.WriteString("  • Context Threshold:   Unlimited\n")
		}
		if tier.OptimalRetries > 0 {
			fmt.Fprintf(&b.sb, "  • Retry Bound:         %d max retries before escalation\n", tier.OptimalRetries)
		}
		if tier.IsDisabled && strings.TrimSpace(tier.OriginalRule) != "false" {
			b.sb.WriteString("  • Routing Policy:      PRUNED / BYPASSED (when: \"false\")\n")
			if tier.ModelBenefit != "" {
				fmt.Fprintf(&b.sb, "    Benefit:             %s\n", tier.ModelBenefit)
			}
		} else if tier.RecommendedModel != "" && tier.RecommendedModel != tier.OriginalModel {
			fmt.Fprintf(&b.sb, "  • Model Substitution:  %s -> %s\n", tier.OriginalModel, tier.RecommendedModel)
			if tier.ModelBenefit != "" {
				fmt.Fprintf(&b.sb, "    Benefit:             %s\n", tier.ModelBenefit)
			}
		}
		if tier.RecommendedReasoningEffort != tier.OriginalReasoningEffort && (tier.OriginalReasoningEffort != "" || tier.RecommendedReasoningEffort != "") {
			origDisplay := tier.OriginalReasoningEffort
			if origDisplay == "" {
				origDisplay = "standard"
			}
			fmt.Fprintf(&b.sb, "  • Reasoning Effort:    %s -> %s\n", origDisplay, tier.RecommendedReasoningEffort)
		}
		if tier.RestrictImages {
			b.sb.WriteString("  • Multimodal Vision:   Restricted\n")
		} else {
			b.sb.WriteString("  • Multimodal Vision:   Allowed\n")
		}
		if tier.RestrictTools {
			b.sb.WriteString("  • Agentic Tool Calls:  Restricted\n")
		} else {
			b.sb.WriteString("  • Agentic Tool Calls:  Allowed\n")
		}
		if len(tier.FrictionKeywords) > 0 {
			fmt.Fprintf(&b.sb, "  • Excluded Keywords:   %v\n", tier.FrictionKeywords)
		}
	}
	return b
}

// WithFallbackPolicy renders the fallback tier recommendation.
func (b *AdvisoryReportBuilder) WithFallbackPolicy() *AdvisoryReportBuilder {
	if b.res.DefaultTier != nil {
		dt := b.res.DefaultTier
		hasModelDiff := dt.RecommendedModel != "" && dt.RecommendedModel != dt.OriginalModel
		hasReasoningDiff := dt.RecommendedReasoningEffort != dt.OriginalReasoningEffort && (dt.OriginalReasoningEffort != "" || dt.RecommendedReasoningEffort != "")
		if hasModelDiff || hasReasoningDiff {
			fmt.Fprintf(&b.sb, "\n  [Fallback Tier: %s]\n", dt.TierName)
			if hasModelDiff {
				fmt.Fprintf(&b.sb, "  • Model Substitution:  %s -> %s\n", dt.OriginalModel, dt.RecommendedModel)
			}
			if hasReasoningDiff {
				origDisplay := dt.OriginalReasoningEffort
				if origDisplay == "" {
					origDisplay = "standard"
				}
				fmt.Fprintf(&b.sb, "  • Reasoning Effort:    %s -> %s\n", origDisplay, dt.RecommendedReasoningEffort)
			}
			if dt.ModelBenefit != "" {
				fmt.Fprintf(&b.sb, "    Benefit:             %s\n", dt.ModelBenefit)
			}
		}
	}
	return b
}

// WithFleetImpact renders projected savings and retries eliminated.
func (b *AdvisoryReportBuilder) WithFleetImpact() *AdvisoryReportBuilder {
	b.sb.WriteString("\n📈 PROJECTED FLEET IMPACT:\n")
	fmt.Fprintf(&b.sb, "  • Developer Retries Avoided: ~%d retries eliminated\n", b.res.RetriesEliminated)
	if b.res.ProjectedSavingsUSD > 0 {
		fmt.Fprintf(&b.sb, "  • Net Monthly Cost Optimization: $%.2f USD saved\n", b.res.ProjectedSavingsUSD)
	}
	return b
}

// WithConfigDiff renders the unified diff recommendation.
func (b *AdvisoryReportBuilder) WithConfigDiff() *AdvisoryReportBuilder {
	b.sb.WriteString("\n🛠️ RECOMMENDED CONFIGURATION DIFF:\n" +
		"----------------------------------------------------------------------------------------\n")
	var diffCount int
	for _, tier := range b.res.Tiers {
		hasModelDiff := tier.RecommendedModel != "" && tier.RecommendedModel != tier.OriginalModel
		hasReasoningDiff := tier.RecommendedReasoningEffort != tier.OriginalReasoningEffort && (tier.OriginalReasoningEffort != "" || tier.RecommendedReasoningEffort != "")
		hasRuleDiff := tier.SynthesizedRule != "" && tier.SynthesizedRule != tier.OriginalRule
		if !hasModelDiff && !hasReasoningDiff && !hasRuleDiff {
			continue
		}
		diffCount++
		fmt.Fprintf(&b.sb, "  Tier: %q\n", tier.TierName)
		if hasModelDiff {
			fmt.Fprintf(&b.sb, "  - model: %q\n  + model: %q\n", tier.OriginalModel, tier.RecommendedModel)
		}
		if hasReasoningDiff {
			if tier.OriginalReasoningEffort != "" {
				fmt.Fprintf(&b.sb, "  - reasoning_effort: %q\n", tier.OriginalReasoningEffort)
			}
			if tier.RecommendedReasoningEffort != "" {
				fmt.Fprintf(&b.sb, "  + reasoning_effort: %q\n", tier.RecommendedReasoningEffort)
			}
		}
		if hasRuleDiff {
			if tier.OriginalRule != "" {
				fmt.Fprintf(&b.sb, "  - when: %q\n", tier.OriginalRule)
			}
			fmt.Fprintf(&b.sb, "  + when: %q\n", tier.SynthesizedRule)
		}
		b.sb.WriteString("\n")
	}
	if b.res.DefaultTier != nil {
		dt := b.res.DefaultTier
		hasModelDiff := dt.RecommendedModel != "" && dt.RecommendedModel != dt.OriginalModel
		hasReasoningDiff := dt.RecommendedReasoningEffort != dt.OriginalReasoningEffort && (dt.OriginalReasoningEffort != "" || dt.RecommendedReasoningEffort != "")
		if hasModelDiff || hasReasoningDiff {
			diffCount++
			fmt.Fprintf(&b.sb, "  Fallback Tier: %q\n", dt.TierName)
			if hasModelDiff {
				fmt.Fprintf(&b.sb, "  - model: %q\n  + model: %q\n", dt.OriginalModel, dt.RecommendedModel)
			}
			if hasReasoningDiff {
				if dt.OriginalReasoningEffort != "" {
					fmt.Fprintf(&b.sb, "  - reasoning_effort: %q\n", dt.OriginalReasoningEffort)
				}
				if dt.RecommendedReasoningEffort != "" {
					fmt.Fprintf(&b.sb, "  + reasoning_effort: %q\n", dt.RecommendedReasoningEffort)
				}
			}
			b.sb.WriteString("\n")
		}
	}
	if diffCount == 0 {
		b.sb.WriteString("  (No configuration changes recommended — active fleet policy is optimal)\n\n")
	}
	b.sb.WriteString("----------------------------------------------------------------------------------------\n\n")
	return b
}

// WithApplyInstructions renders the command for applying the generated recommendations.
func (b *AdvisoryReportBuilder) WithApplyInstructions() *AdvisoryReportBuilder {
	b.sb.WriteString("To apply this recommendation with automatic backup:\n" +
		"  $ nacho-flow tune --apply\n" +
		"========================================================================================\n")
	return b
}

// Build finalizes the report string.
func (b *AdvisoryReportBuilder) Build() string {
	return b.sb.String()
}
