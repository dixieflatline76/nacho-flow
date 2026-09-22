package tuner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// GenerateAdvisoryReport creates a human-readable CLI report from TuningResult.
func GenerateAdvisoryReport(res *TuningResult, cfg *contract.Config) string {
	var b strings.Builder

	b.WriteString("========================================================================================\n")
	if res.TotalSessions > 0 {
		b.WriteString("🌮 NACHO FLOW ADVISORY TUNING REPORT (v2 — Session Replay)\n")
	} else {
		b.WriteString("🌮 NACHO FLOW ADVISORY TUNING REPORT\n")
	}
	b.WriteString("========================================================================================\n\n")

	b.WriteString(fmt.Sprintf("📊 Sample Size: %d historical prompt turns evaluated\n", res.TotalSampleTurns))

	if res.TotalSessions > 0 {
		b.WriteString("\n🔄 SESSION DYNAMICS:\n")
		b.WriteString(fmt.Sprintf("  • Total Sessions Evaluated:       %d\n", res.TotalSessions))
		b.WriteString(fmt.Sprintf("  • Average Turns per Session:      %.1f\n", res.AvgTurnsPerSession))
		b.WriteString(fmt.Sprintf("  • Cloud Escalation Rate:          %.1f%%\n", res.EscalationRate*100))
	} else {
		b.WriteString("\n")
	}

	if len(res.RecoveryStats) > 0 {
		b.WriteString("\n🩺 MODEL SELF-RECOVERY ANALYSIS:\n")
		var models []string
		for m := range res.RecoveryStats {
			models = append(models, m)
		}
		sort.Strings(models)
		for _, m := range models {
			stat := res.RecoveryStats[m]
			b.WriteString(fmt.Sprintf("  • %-24s Self-Recovery: %.1f%% (avg %.1f turns to recover)\n",
				stat.Model+":", stat.SelfRecoveryRate*100, stat.AvgTurnsToRecover))
		}
	}

	b.WriteString("\n🔍 MULTI-TIER ROUTING POLICIES:\n")
	for i, tier := range res.Tiers {
		b.WriteString(fmt.Sprintf("\n  [Tier %d: %s]\n", i+1, tier.TierName))
		b.WriteString(fmt.Sprintf("  • Context Threshold:   %d tokens\n", tier.OptimalThreshold))
		if tier.OptimalRetries > 0 {
			b.WriteString(fmt.Sprintf("  • Retry Bound:         %d max retries before escalation\n", tier.OptimalRetries))
		}
		if tier.RestrictImages {
			b.WriteString("  • Multimodal Vision:   Restricted\n")
		} else {
			b.WriteString("  • Multimodal Vision:   Allowed\n")
		}
		if tier.RestrictTools {
			b.WriteString("  • Agentic Tool Calls:  Restricted\n")
		} else {
			b.WriteString("  • Agentic Tool Calls:  Allowed\n")
		}
		if len(tier.FrictionKeywords) > 0 {
			b.WriteString(fmt.Sprintf("  • Excluded Keywords:   %v\n", tier.FrictionKeywords))
		}
	}

	b.WriteString("\n📈 PROJECTED FLEET IMPACT:\n")
	b.WriteString(fmt.Sprintf("  • Developer Retries Avoided: ~%d retries eliminated\n", res.RetriesEliminated))
	if res.ProjectedSavingsUSD > 0 {
		b.WriteString(fmt.Sprintf("  • Net Monthly Cost Optimization: $%.2f USD saved\n", res.ProjectedSavingsUSD))
	}

	b.WriteString("\n🛠️ RECOMMENDED CONFIGURATION DIFF:\n")
	b.WriteString("----------------------------------------------------------------------------------------\n")
	for _, tier := range res.Tiers {
		b.WriteString(fmt.Sprintf("  Tier: %q\n", tier.TierName))
		if tier.OriginalRule != "" {
			b.WriteString(fmt.Sprintf("  - when: %q\n", tier.OriginalRule))
		}
		b.WriteString(fmt.Sprintf("  + when: %q\n", tier.SynthesizedRule))
		b.WriteString("\n")
	}
	b.WriteString("----------------------------------------------------------------------------------------\n\n")

	b.WriteString("To apply this recommendation with automatic backup:\n")
	b.WriteString("  $ nacho-flow tune --apply\n")
	b.WriteString("========================================================================================\n")

	return b.String()
}
