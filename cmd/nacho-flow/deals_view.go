package main

import (
	"fmt"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

const (
	// Badges & visual indicator constants.
	BadgeHotDeal           = "🔥"
	BadgeFreeTier          = "🆓"
	PlaceholderCodingScore = "--"
	TokensKiloThreshold    = 1000
	TokensMegaThreshold    = 1_000_000
	TokensMegaDivisor      = 1_000_000.0
)

// DealRowView represents a presentation-ready row for tabular and human display.
type DealRowView struct {
	ModelID     string
	Role        string
	Context     string
	PromptPrice string
	CompPrice   string
	CodingScore string
	Discount    string
	Badge       string
	Why         string
}

// FormatTokenCount formats token counts into human-readable shorthand (e.g. 512k, 1.0M).
func FormatTokenCount(tokens int) string {
	if tokens >= TokensMegaThreshold {
		return fmt.Sprintf("%.1fM", float64(tokens)/TokensMegaDivisor)
	}
	if tokens >= TokensKiloThreshold {
		return fmt.Sprintf("%dk", tokens/TokensKiloThreshold)
	}
	return fmt.Sprintf("%d", tokens)
}

// FormatCodingScore formats a benchmark score into a decimal string or placeholder.
func FormatCodingScore(score float64) string {
	if score > 0 {
		return fmt.Sprintf("%.1f", score)
	}
	return PlaceholderCodingScore
}

// FormatCost formats USD cost per 1M tokens.
func FormatCost(cost float64) string {
	return fmt.Sprintf("$%.2f", cost)
}

// FormatDiscount formats a percentage discount string.
func FormatDiscount(pct float64) string {
	return fmt.Sprintf("%.1f%%", pct)
}

// FormatDealWhy generates a clean explanation of why a deal is surfaced.
func FormatDealWhy(d contract.DealInfo) string {
	if len(d.RecommendedTiers) > 0 {
		tiersStr := strings.Join(d.RecommendedTiers, ", ")
		return fmt.Sprintf("Recommended for %s (Replaces benchmark at %.1f%% discount)", tiersStr, d.DiscountPct)
	}
	return fmt.Sprintf("Discovery scouted high value model at %.1f%% discount", d.DiscountPct)
}

// ToDealRowView transforms a raw domain DealInfo struct into a presentation View Model.
func ToDealRowView(d contract.DealInfo) DealRowView {
	badge := BadgeHotDeal
	if d.IsFree {
		badge = BadgeFreeTier
	}

	return DealRowView{
		ModelID:     d.ModelID,
		Role:        d.TierRole,
		Context:     FormatTokenCount(d.ContextLength),
		PromptPrice: FormatCost(d.PromptCostPerM),
		CompPrice:   FormatCost(d.CompletionCostPerM),
		CodingScore: FormatCodingScore(d.CodingIndex),
		Discount:    FormatDiscount(d.DiscountPct),
		Badge:       badge,
		Why:         FormatDealWhy(d),
	}
}
