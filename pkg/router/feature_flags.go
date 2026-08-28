// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package router

// FeatureFlag encodes the active feature set as a 16-bit bitmask.
// Internal implementation detail — never exposed directly to users.
type FeatureFlag uint16

const (
	// --- 🛡️ Shield Subsystem ---
	// Directive-reachable: @nacho:no-shield clears FeatureShieldEnabled (and its children).
	// Tier-reachable: shield: false in config.yaml.
	FeatureShieldEnabled    FeatureFlag = 1 << 0 // Master: disabling clears all shield sub-strategies
	FeatureShieldFollowup   FeatureFlag = 1 << 1 // Tier-only: synthesize ask_followup_question schema
	FeatureShieldModeSwitch FeatureFlag = 1 << 2 // Tier-only: synthesize switch_mode schema

	// --- 🔧 Tool & Stream Normalizer Subsystem ---
	// Directive-reachable: @nacho:raw clears all (via FeatureRawPassThrough = 0).
	// Tier-reachable: normalizers: react: false, etc.
	FeatureToolNormalizer FeatureFlag = 1 << 4 // Master: disabling skips all normalizer parsers
	FeatureNormMarkdown   FeatureFlag = 1 << 5 // Tier-only: strip ```json code fences
	FeatureNormBareJSON   FeatureFlag = 1 << 6 // Tier-only: wrap raw JSON payloads
	FeatureNormReAct      FeatureFlag = 1 << 7 // Tier-only: parse Action: tool syntax

	// --- 🧠 Think/Reasoning Stream Subsystem ---
	// Directive-reachable: @nacho:raw clears all.
	// Tier-reachable: normalizers: think: false.
	FeatureThinkNormalizer FeatureFlag = 1 << 8 // Master: disabling skips reasoning stream handling
	FeatureThinkSanitize   FeatureFlag = 1 << 9 // Tier-only: prevent double-wrapping native <think>

	// --- 🌟 Composite Presets ---
	FeatureDefaultAll = FeatureShieldEnabled | FeatureShieldFollowup | FeatureShieldModeSwitch |
		FeatureToolNormalizer | FeatureNormMarkdown | FeatureNormBareJSON | FeatureNormReAct |
		FeatureThinkNormalizer | FeatureThinkSanitize

	// FeatureRawPassThrough: all bits zero — complete transparent pass-through.
	// Reached via @nacho:raw directive or tier.raw = true in config.yaml.
	FeatureRawPassThrough FeatureFlag = 0
)

// Has returns true if the specified flag is enabled. O(1), 0 B/op.
func (f FeatureFlag) Has(flag FeatureFlag) bool {
	return f&flag != 0
}

// MaskOut returns a new FeatureFlag with the specified bits cleared. O(1), 0 B/op.
func (f FeatureFlag) MaskOut(flag FeatureFlag) FeatureFlag {
	return f &^ flag
}
