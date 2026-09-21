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

	b.WriteString("\n🔍 FRICTION & BOTTLENECK SIGNALS DETECTED:\n")
	b.WriteString(fmt.Sprintf("  • Optimal Local Context Threshold: %d tokens\n", res.OptimalThreshold))
	if res.OptimalRetries > 0 {
		b.WriteString(fmt.Sprintf("  • Retry Bound:                    %d max retries before cloud escalation\n", res.OptimalRetries))
	}

	if res.RestrictImages {
		b.WriteString("  • Multimodal Vision:              High Friction (Spikes local retries — restricted)\n")
	} else {
		b.WriteString("  • Multimodal Vision:              Clean (0% retry rate — enabled locally)\n")
	}

	if res.RestrictTools {
		b.WriteString("  • Agentic Tool Calls:             High Friction (Spikes local retries — restricted)\n")
	} else {
		b.WriteString("  • Agentic Tool Calls:             Clean (0% retry rate — enabled locally)\n")
	}

	if len(res.FrictionKeywords) > 0 {
		b.WriteString(fmt.Sprintf("  • High-Friction Domain Keywords:  %v (Spikes local retry probability)\n", res.FrictionKeywords))
	} else {
		b.WriteString("  • High-Friction Domain Keywords:  None (Clean token progression across all domains)\n")
	}

	b.WriteString("\n📈 PROJECTED MONTHLY IMPACT:\n")
	b.WriteString(fmt.Sprintf("  • Developer Retries Avoided: ~%d retries eliminated\n", res.RetriesEliminated))
	if res.ProjectedSavingsUSD > 0 {
		b.WriteString(fmt.Sprintf("  • Net Monthly Cost Optimization: $%.2f USD saved\n", res.ProjectedSavingsUSD))
	}

	// Find the local tier in current config to show diff
	var oldRule string
	localTierName := res.TargetTierName
	if localTierName == "" {
		localTierName = "Local ROCm GPU"
	}

	if cfg != nil {
		for _, tier := range cfg.Tiers {
			if IsLocalTier(tier, cfg.Providers) {
				oldRule = tier.When
				if tier.Name != "" {
					localTierName = tier.Name
				}
				break
			}
		}
	}
	if oldRule == "" {
		oldRule = "Tokens < 16000 && !HasImages && !HasTools"
	}

	b.WriteString("\n🛠️ RECOMMENDED CONFIGURATION DIFF:\n")
	b.WriteString("----------------------------------------------------------------------------------------\n")
	b.WriteString(fmt.Sprintf("  Tier: %q\n", localTierName))
	b.WriteString(fmt.Sprintf("  - when: %q\n", oldRule))
	b.WriteString(fmt.Sprintf("  + when: %q\n", res.SynthesizedRule))
	b.WriteString("----------------------------------------------------------------------------------------\n\n")

	b.WriteString("To apply this recommendation with automatic backup:\n")
	b.WriteString("  $ nacho-flow tune --apply\n")
	b.WriteString("========================================================================================\n")

	return b.String()
}
