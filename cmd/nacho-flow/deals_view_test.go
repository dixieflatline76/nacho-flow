package main

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		tokens   int
		expected string
	}{
		{tokens: 0, expected: "0"},
		{tokens: 500, expected: "500"},
		{tokens: 1000, expected: "1k"},
		{tokens: 32000, expected: "32k"},
		{tokens: 512000, expected: "512k"},
		{tokens: 1000000, expected: "1.0M"},
		{tokens: 1048576, expected: "1.0M"},
		{tokens: 2000000, expected: "2.0M"},
	}

	for _, tc := range tests {
		got := FormatTokenCount(tc.tokens)
		if got != tc.expected {
			t.Errorf("FormatTokenCount(%d) = %s; want %s", tc.tokens, got, tc.expected)
		}
	}
}

func TestFormatCodingScore(t *testing.T) {
	if got := FormatCodingScore(0); got != PlaceholderCodingScore {
		t.Errorf("FormatCodingScore(0) = %s; want %s", got, PlaceholderCodingScore)
	}
	if got := FormatCodingScore(-1.0); got != PlaceholderCodingScore {
		t.Errorf("FormatCodingScore(-1) = %s; want %s", got, PlaceholderCodingScore)
	}
	if got := FormatCodingScore(78.43); got != "78.4" {
		t.Errorf("FormatCodingScore(78.43) = %s; want 78.4", got)
	}
}

func TestFormatCost(t *testing.T) {
	if got := FormatCost(0.1); got != "$0.10" {
		t.Errorf("FormatCost(0.1) = %s; want $0.10", got)
	}
	if got := FormatCost(0.0); got != "$0.00" {
		t.Errorf("FormatCost(0.0) = %s; want $0.00", got)
	}
}

func TestFormatDiscount(t *testing.T) {
	if got := FormatDiscount(96.67); got != "96.7%" {
		t.Errorf("FormatDiscount(96.67) = %s; want 96.7%%", got)
	}
}

func TestFormatDealWhy(t *testing.T) {
	dealWithTiers := contract.DealInfo{
		RecommendedTiers: []string{"tier_1_vision", "tier_3_workhorse"},
		DiscountPct:      85.0,
	}
	expectedWith := "Recommended for tier_1_vision, tier_3_workhorse (Replaces benchmark at 85.0% discount)"
	if got := FormatDealWhy(dealWithTiers); got != expectedWith {
		t.Errorf("FormatDealWhy(dealWithTiers) = %q; want %q", got, expectedWith)
	}

	dealWithoutTiers := contract.DealInfo{
		DiscountPct: 90.0,
	}
	expectedWithout := "Discovery scouted high value model at 90.0% discount"
	if got := FormatDealWhy(dealWithoutTiers); got != expectedWithout {
		t.Errorf("FormatDealWhy(dealWithoutTiers) = %q; want %q", got, expectedWithout)
	}
}

func TestToDealRowView(t *testing.T) {
	// 1. Paid deal
	paidDeal := contract.DealInfo{
		ModelID:            "google/gemini-2.5-flash-lite",
		TierRole:           "vision_workhorse",
		ContextLength:      1048576,
		PromptCostPerM:     0.10,
		CompletionCostPerM: 0.40,
		CodingIndex:        68.1,
		DiscountPct:        96.7,
		IsFree:             false,
		RecommendedTiers:   []string{"tier_1_vision"},
	}

	viewPaid := ToDealRowView(paidDeal)
	if viewPaid.Badge != BadgeHotDeal {
		t.Errorf("expected BadgeHotDeal, got %s", viewPaid.Badge)
	}
	if viewPaid.Context != "1.0M" {
		t.Errorf("expected Context '1.0M', got %s", viewPaid.Context)
	}
	if viewPaid.PromptPrice != "$0.10" {
		t.Errorf("expected PromptPrice '$0.10', got %s", viewPaid.PromptPrice)
	}
	if viewPaid.CompPrice != "$0.40" {
		t.Errorf("expected CompPrice '$0.40', got %s", viewPaid.CompPrice)
	}
	if viewPaid.CodingScore != "68.1" {
		t.Errorf("expected CodingScore '68.1', got %s", viewPaid.CodingScore)
	}

	// 2. Free deal
	freeDeal := contract.DealInfo{
		ModelID:            "dots-studio/dots-3-note:free",
		TierRole:           "coding_workhorse",
		ContextLength:      512000,
		PromptCostPerM:     0.0,
		CompletionCostPerM: 0.0,
		CodingIndex:        0.0,
		DiscountPct:        100.0,
		IsFree:             true,
	}

	viewFree := ToDealRowView(freeDeal)
	if viewFree.Badge != BadgeFreeTier {
		t.Errorf("expected BadgeFreeTier, got %s", viewFree.Badge)
	}
	if viewFree.CodingScore != PlaceholderCodingScore {
		t.Errorf("expected PlaceholderCodingScore, got %s", viewFree.CodingScore)
	}
}
