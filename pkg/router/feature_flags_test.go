// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package router_test

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/router"
)

func TestFeatureFlags_BitmaskOperations(t *testing.T) {
	tests := []struct {
		name     string
		flags    router.FeatureFlag
		check    router.FeatureFlag
		expected bool
	}{
		{
			name:     "DefaultAll has ShieldEnabled",
			flags:    router.FeatureDefaultAll,
			check:    router.FeatureShieldEnabled,
			expected: true,
		},
		{
			name:     "DefaultAll has ToolNormalizer",
			flags:    router.FeatureDefaultAll,
			check:    router.FeatureToolNormalizer,
			expected: true,
		},
		{
			name:     "DefaultAll has ThinkNormalizer",
			flags:    router.FeatureDefaultAll,
			check:    router.FeatureThinkNormalizer,
			expected: true,
		},
		{
			name:     "RawPassThrough has no flags",
			flags:    router.FeatureRawPassThrough,
			check:    router.FeatureShieldEnabled,
			expected: false,
		},
		{
			name:     "RawPassThrough has no tool normalizer",
			flags:    router.FeatureRawPassThrough,
			check:    router.FeatureToolNormalizer,
			expected: false,
		},
		{
			name:     "MaskOut Shield removes ShieldEnabled",
			flags:    router.FeatureDefaultAll.MaskOut(router.FeatureShieldEnabled),
			check:    router.FeatureShieldEnabled,
			expected: false,
		},
		{
			name:     "MaskOut Shield preserves ToolNormalizer",
			flags:    router.FeatureDefaultAll.MaskOut(router.FeatureShieldEnabled),
			check:    router.FeatureToolNormalizer,
			expected: true,
		},
		{
			name:     "MaskOut multiple bits",
			flags:    router.FeatureDefaultAll.MaskOut(router.FeatureShieldEnabled | router.FeatureShieldFollowup | router.FeatureShieldModeSwitch),
			check:    router.FeatureShieldFollowup,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.flags.Has(tt.check)
			if got != tt.expected {
				t.Errorf("flags.Has(%v) = %v; want %v", tt.check, got, tt.expected)
			}
		})
	}
}

func BenchmarkHas(b *testing.B) {
	flags := router.FeatureDefaultAll
	target := router.FeatureToolNormalizer
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		if !flags.Has(target) {
			b.Fatal("unexpected false")
		}
	}
}

func BenchmarkMaskOut(b *testing.B) {
	flags := router.FeatureDefaultAll
	mask := router.FeatureShieldEnabled | router.FeatureShieldFollowup
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		res := flags.MaskOut(mask)
		if res.Has(router.FeatureShieldEnabled) {
			b.Fatal("unexpected true")
		}
	}
}
