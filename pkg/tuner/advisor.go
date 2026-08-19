package tuner

import (
	"fmt"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// GenerateAdvisoryReport creates a human-readable CLI report from TuningResult.
func GenerateAdvisoryReport(res *TuningResult, cfg *contract.Config) string {
	var b strings.Builder

	b.WriteString("========================================================================================\n")
	b.WriteString("🌮 NACHO FLOW ADVISORY TUNING REPORT\n")
	b.WriteString("========================================================================================\n\n")

	b.WriteString(fmt.Sprintf("📊 Sample Size: %d historical prompt turns evaluated\n\n", res.TotalSampleTurns))

	b.WriteString("🔍 FRICTION & BOTTLENECK SIGNALS DETECTED:\n")
	b.WriteString(fmt.Sprintf("  • Optimal Local Context Threshold: %d tokens\n", res.OptimalThreshold))
	if len(res.FrictionKeywords) > 0 {
		b.WriteString(fmt.Sprintf("  • High-Friction Domain Keywords:  %v (Spikes local retry probability)\n", res.FrictionKeywords))
	} else {
		b.WriteString("  • High-Friction Domain Keywords:  None (Clean token progression across all domains)\n")
	}

	b.WriteString("\n📈 PROJECTED MONTHLY IMPACT:\n")
	b.WriteString(fmt.Sprintf("  • Developer Retries Avoided: ~%d retries eliminated\n", res.RetriesEliminated))
	if res.ProjectedSavingsUSD > 0 {
		b.WriteString(fmt.Sprintf("  • Net Monthly Cost Optimization: +$%.2f USD saved\n", res.ProjectedSavingsUSD))
	}

	// Find the local tier in current config to show diff
	var oldRule string
	var localTierName string
	if cfg != nil {
		for _, tier := range cfg.Tiers {
			if tier.Provider == "ollama" || strings.Contains(strings.ToLower(tier.Name), "local") {
				oldRule = tier.When
				localTierName = tier.Name
				break
			}
		}
	}
	if oldRule == "" {
		oldRule = "Tokens < 16000 && !HasImages && !HasTools"
		localTierName = "Local ROCm GPU"
	}

	b.WriteString("\n🛠️ RECOMMENDED CONFIGURATION DIFF:\n")
	b.WriteString("----------------------------------------------------------------------------------------\n")
	b.WriteString(fmt.Sprintf("  Tier: \"%s\"\n", localTierName))
	b.WriteString(fmt.Sprintf("  - when: \"%s\"\n", oldRule))
	b.WriteString(fmt.Sprintf("  + when: \"%s\"\n", res.SynthesizedRule))
	b.WriteString("----------------------------------------------------------------------------------------\n\n")

	b.WriteString("To apply this recommendation with automatic backup:\n")
	b.WriteString("  $ nacho-flow tune --apply\n")
	b.WriteString("========================================================================================\n")

	return b.String()
}
