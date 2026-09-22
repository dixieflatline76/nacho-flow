package tuner

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

var (
	extractTokenRegex   = regexp.MustCompile(`(?i)tokens\s*<\s*(\d+)`)
	extractRetriesRegex = regexp.MustCompile(`(?i)retries\s*<\s*(\d+)`)
	extractKwRegex      = regexp.MustCompile(`'([^']+)'|"([^"]+)"`)
)

// ResolveModelRates resolves realistic prompt and completion rates per million tokens.
// The comprehensive rate represents a blended rate: prompt + 0.25 * completion.
func ResolveModelRates(model string, isLocal bool) (promptCost, compCost, compRate float64) {
	if isLocal {
		return 0, 0, 0
	}
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return 15.00, 75.00, 33.75
	case strings.Contains(m, "sonnet"):
		return 3.00, 15.00, 6.75
	case strings.Contains(m, "haiku"):
		return 0.80, 4.00, 1.80
	case strings.Contains(m, "deepseek"):
		return 0.27, 1.10, 0.545
	case strings.Contains(m, "gemini") && strings.Contains(m, "flash"):
		return 0.15, 0.60, 0.30
	case strings.Contains(m, "gemini") && strings.Contains(m, "pro"):
		return 1.25, 5.00, 2.50
	case strings.Contains(m, "qwen"):
		return 0.20, 0.60, 0.35
	default:
		return 2.50, 10.00, 5.00
	}
}

// ResolveModelBenchmark resolves verified coding index and tool reliability from the curated catalog.
func ResolveModelBenchmark(model string) (codingIndex float64, toolReliability float64) {
	cat := curation.NewManager("", "")
	if p, ok := cat.Lookup(model); ok {
		return p.CodingIndex, p.ToolReliability
	}
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "sonnet-5") || strings.Contains(m, "opus"):
		return 97.4, 99.0
	case strings.Contains(m, "sonnet-3.7") || strings.Contains(m, "3.7-sonnet"):
		return 95.8, 99.0
	case strings.Contains(m, "sonnet"):
		return 92.0, 99.0
	case strings.Contains(m, "deepseek") || strings.Contains(m, "r1"):
		return 82.5, 90.0
	case strings.Contains(m, "qwen"):
		return 78.4, 88.0
	case strings.Contains(m, "flash"):
		return 74.0, 85.0
	default:
		return 70.0, 80.0
	}
}

// ExtractRoutingState converts a contract.Config into an editable MultiTierConfig
// for the Min-Conflicts optimizer.
func ExtractRoutingState(cfg *contract.Config, monitoredKeywords []string) MultiTierConfig {
	if cfg == nil || len(cfg.Tiers) == 0 {
		return MultiTierConfig{}
	}

	result := MultiTierConfig{}

	// Tiers 0 .. len(cfg.Tiers)-1 are cascade tiers
	for _, tier := range cfg.Tiers {
		isLocal := IsLocalTier(tier, cfg.Providers)
		promptCost, compCost, compRate := ResolveModelRates(tier.Model, isLocal)
		codingIndex, toolReliability := ResolveModelBenchmark(tier.Model)
		isDisabled := strings.TrimSpace(tier.When) == "false"

		threshold := 8000
		if m := extractTokenRegex.FindStringSubmatch(tier.When); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
				threshold = v
			}
		}

		retryBound := 2
		if m := extractRetriesRegex.FindStringSubmatch(tier.When); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
				retryBound = v
			}
		}

		restrictImages := strings.Contains(strings.ToLower(tier.When), "!hasimages")
		restrictTools := strings.Contains(strings.ToLower(tier.When), "!hastools")

		var excludedKws []string
		if strings.Contains(strings.ToLower(tier.When), "keywords") {
			matches := extractKwRegex.FindAllStringSubmatch(tier.When, -1)
			for _, m := range matches {
				if len(m) > 1 && m[1] != "" {
					excludedKws = append(excludedKws, m[1])
				} else if len(m) > 2 && m[2] != "" {
					excludedKws = append(excludedKws, m[2])
				}
			}
		}

		result.Tiers = append(result.Tiers, TierReplayConfig{
			TierName:                 tier.Name,
			Provider:                 tier.Provider,
			Model:                    tier.Model,
			IsLocal:                  isLocal,
			IsDisabled:               isDisabled,
			CodingIndex:              codingIndex,
			ToolReliability:          toolReliability,
			PromptCostPerMillion:     promptCost,
			CompletionCostPerMillion: compCost,
			ComprehensiveRate:        compRate,
			CostPerMillion:           compRate,
			TokenThreshold:           threshold,
			MaxContext:               tier.MaxContext,
			RetryBound:               retryBound,
			RestrictImages:           restrictImages,
			RestrictTools:            restrictTools,
			ExcludedKeywords:         excludedKws,
		})
	}

	// Default fallback tier (unconditional sink)
	defLocal := IsLocalTier(cfg.DefaultTier, cfg.Providers)
	defPrompt, defComp, defRate := ResolveModelRates(cfg.DefaultTier.Model, defLocal)
	defCoding, defTool := ResolveModelBenchmark(cfg.DefaultTier.Model)

	result.DefaultTier = TierReplayConfig{
		TierName:                 cfg.DefaultTier.Name,
		Provider:                 cfg.DefaultTier.Provider,
		Model:                    cfg.DefaultTier.Model,
		IsLocal:                  defLocal,
		CodingIndex:              defCoding,
		ToolReliability:          defTool,
		PromptCostPerMillion:     defPrompt,
		CompletionCostPerMillion: defComp,
		ComprehensiveRate:        defRate,
		CostPerMillion:           defRate,
	}

	return result
}
