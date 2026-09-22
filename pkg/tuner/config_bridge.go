package tuner

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

var (
	extractTokenRegex        = regexp.MustCompile(`(?i)tokens\s*<\s*(\d+)`)
	extractRetriesRegex      = regexp.MustCompile(`(?i)retries\s*<\s*(\d+)`)
	extractRetryFloorRegex   = regexp.MustCompile(`(?i)retries\s*>=\s*(\d+)`)
	extractRetryFloorGtRegex = regexp.MustCompile(`(?i)retries\s*>\s*(\d+)`)
	extractKwRegex           = regexp.MustCompile(`'([^']+)'|"([^"]+)"`)

	kickstartClauseRegex = regexp.MustCompile(`(?i)\bSessionKickstarted\b(?:\s*==\s*true)?`)
	restrictImagesRegex  = regexp.MustCompile(`(?i)(?:!\s*HasImages|\bHasImages\s*==\s*false\b)`)
	requireImagesRegex   = regexp.MustCompile(`(?i)(?:^|[^!])\bHasImages\b(?:\s*==\s*true)?`)
	restrictToolsRegex   = regexp.MustCompile(`(?i)(?:!\s*HasTools|\bHasTools\s*==\s*false\b)`)
	extractKeywordsRegex = regexp.MustCompile(`(?i)\bkeywords\b`)
)

// ResolveModelRates resolves realistic prompt and completion rates per million tokens.
// It queries the canonical curated catalog dynamically without hardcoded switches.
// The comprehensive rate represents a blended rate: prompt + 0.25 * completion.
func ResolveModelRates(model string, isLocal bool) (promptCost, compCost, compRate float64) {
	if isLocal {
		return 0, 0, 0
	}
	if p, ok := curation.DefaultManager().Lookup(model); ok && p.PromptCostPerMillion > 0 {
		promptCost = p.PromptCostPerMillion
		compCost = p.CompletionCostPerMillion
		compRate = promptCost + 0.25*compCost
		return promptCost, compCost, compRate
	}
	// Fallback to standard cloud baseline rate from DefaultTuningPolicy for uncatalogued models
	defRate := DefaultTuningPolicy().CostPerMillionCloud
	return defRate, defRate * 4.0, defRate * 2.0
}

// ResolveModelBenchmark resolves verified coding index and tool reliability from the curated catalog.
func ResolveModelBenchmark(model string) (codingIndex float64, toolReliability float64) {
	if p, ok := curation.DefaultManager().Lookup(model); ok && p.CodingIndex > 0 {
		return p.CodingIndex, p.ToolReliability
	}
	// Default neutral baseline benchmarks for uncatalogued models
	return 70.0, 80.0
}

// ResolveModelCapabilities resolves verified vision and tool calling support from the curated catalog.
func ResolveModelCapabilities(model string) (supportsVision bool, supportsTools bool) {
	if p, ok := curation.DefaultManager().Lookup(model); ok {
		return p.SupportsVision, p.SupportsTools
	}
	// Default to no-vision, tool-capable for uncatalogued models
	return false, true
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

		requiresKickstart := kickstartClauseRegex.MatchString(tier.When)
		restrictImages := restrictImagesRegex.MatchString(tier.When)
		requiresImages := requireImagesRegex.MatchString(tier.When) && !restrictImages
		restrictTools := restrictToolsRegex.MatchString(tier.When)

		supportsVision, supportsTools := ResolveModelCapabilities(tier.Model)
		if tier.HasVision != nil {
			supportsVision = *tier.HasVision
		} else if tier.ResolvedHasVision || requiresImages {
			supportsVision = true
		}
		if tier.StripImages {
			supportsVision = false
		}
		if tier.HasTools != nil {
			supportsTools = *tier.HasTools
		}
		if tier.StripTools {
			supportsTools = false
		}

		isFrontier := false
		if p, ok := curation.DefaultManager().Lookup(tier.Model); ok {
			isFrontier = p.IsFrontier()
		}

		isDisabled := strings.TrimSpace(tier.When) == "false"

		threshold := 0
		if m := extractTokenRegex.FindStringSubmatch(tier.When); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
				threshold = v
			}
		} else if isLocal {
			threshold = 16000
			if tier.MaxContext > 0 && tier.MaxContext < threshold {
				threshold = tier.MaxContext
			}
		}

		retryBound := 0
		if m := extractRetriesRegex.FindStringSubmatch(tier.When); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
				retryBound = v
			}
		} else if isLocal {
			retryBound = 2
		}

		retryFloor := 0
		if m := extractRetryFloorRegex.FindStringSubmatch(tier.When); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
				retryFloor = v
			}
		} else if m := extractRetryFloorGtRegex.FindStringSubmatch(tier.When); len(m) > 1 {
			if v, err := strconv.Atoi(m[1]); err == nil && v >= 0 {
				retryFloor = v + 1
			}
		}

		var excludedKws []string
		if extractKeywordsRegex.MatchString(tier.When) {
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
			IsFrontier:               isFrontier,
			CodingIndex:              codingIndex,
			ToolReliability:          toolReliability,
			PromptCostPerMillion:     promptCost,
			CompletionCostPerMillion: compCost,
			ComprehensiveRate:        compRate,
			CostPerMillion:           compRate,
			SupportsVision:           supportsVision,
			SupportsTools:            supportsTools,
			RequiresKickstart:        requiresKickstart,
			RequiresImages:           requiresImages,
			RetryFloor:               retryFloor,
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
	defVision, defTools := ResolveModelCapabilities(cfg.DefaultTier.Model)
	if cfg.DefaultTier.HasVision != nil {
		defVision = *cfg.DefaultTier.HasVision
	} else if cfg.DefaultTier.ResolvedHasVision {
		defVision = true
	}
	if cfg.DefaultTier.StripImages {
		defVision = false
	}
	if cfg.DefaultTier.HasTools != nil {
		defTools = *cfg.DefaultTier.HasTools
	}
	if cfg.DefaultTier.StripTools {
		defTools = false
	}
	defIsFrontier := false
	if p, ok := curation.DefaultManager().Lookup(cfg.DefaultTier.Model); ok {
		defIsFrontier = p.IsFrontier()
	}

	result.DefaultTier = TierReplayConfig{
		TierName:                 cfg.DefaultTier.Name,
		Provider:                 cfg.DefaultTier.Provider,
		Model:                    cfg.DefaultTier.Model,
		IsLocal:                  defLocal,
		IsFrontier:               defIsFrontier,
		CodingIndex:              defCoding,
		ToolReliability:          defTool,
		PromptCostPerMillion:     defPrompt,
		CompletionCostPerMillion: defComp,
		ComprehensiveRate:        defRate,
		CostPerMillion:           defRate,
		SupportsVision:           defVision,
		SupportsTools:            defTools,
	}

	return result
}
