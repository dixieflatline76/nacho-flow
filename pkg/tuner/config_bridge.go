package tuner

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

var (
	extractTokenRegex   = regexp.MustCompile(`(?i)tokens\s*<\s*(\d+)`)
	extractRetriesRegex = regexp.MustCompile(`(?i)retries\s*<\s*(\d+)`)
	extractKwRegex      = regexp.MustCompile(`'([^']+)'|"([^"]+)"`)
)

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
		costRate := 0.0
		if !isLocal {
			costRate = 2.50 // Default cloud rate ($2.50 / M tokens)
		}

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
			TierName:         tier.Name,
			Provider:         tier.Provider,
			IsLocal:          isLocal,
			CostPerMillion:   costRate,
			TokenThreshold:   threshold,
			MaxContext:       tier.MaxContext,
			RetryBound:       retryBound,
			RestrictImages:   restrictImages,
			RestrictTools:    restrictTools,
			ExcludedKeywords: excludedKws,
		})
	}

	// Default fallback tier (unconditional sink)
	defLocal := IsLocalTier(cfg.DefaultTier, cfg.Providers)
	defCost := 0.0
	if !defLocal {
		defCost = 15.00 // Default frontier rate ($15.00 / M tokens)
	}

	result.DefaultTier = TierReplayConfig{
		TierName:       cfg.DefaultTier.Name,
		Provider:       cfg.DefaultTier.Provider,
		IsLocal:        defLocal,
		CostPerMillion: defCost,
	}

	return result
}
