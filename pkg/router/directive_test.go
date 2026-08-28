package router

import (
	"testing"
)

func TestHasDirective_FastBailout(t *testing.T) {
	// Plain text without directive
	if HasDirective("Please refactor this Go function to use sync.Pool") {
		t.Errorf("expected false for plain text prompt")
	}

	// Plain text with '@' but not '@nacho:'
	if HasDirective("Hey @alice check this out") {
		t.Errorf("expected false for @alice")
	}

	// Valid directive prefix
	if !HasDirective("Can you @nacho:local refactor this?") {
		t.Errorf("expected true for @nacho:local")
	}

	// Empty string
	if HasDirective("") {
		t.Errorf("expected false for empty string")
	}
}

func TestExtractDirective_RoutingVariants(t *testing.T) {
	tests := []struct {
		name          string
		prompt        string
		wantDirective string
		wantArg       string
		wantClean     string
		wantForced    string
		wantModel     string
		wantIsMeta    bool
	}{
		{
			name:          "local shorthand",
			prompt:        "@nacho:local optimize this loop",
			wantDirective: "local",
			wantClean:     "optimize this loop",
			wantForced:    "local",
			wantIsMeta:    false,
		},
		{
			name:          "cloud shorthand",
			prompt:        "fix memory leak @nacho:cloud in worker",
			wantDirective: "cloud",
			wantClean:     "fix memory leak in worker",
			wantForced:    "cloud",
			wantIsMeta:    false,
		},
		{
			name:          "frontier shorthand",
			prompt:        "@nacho:frontier architect our auth flow",
			wantDirective: "frontier",
			wantClean:     "architect our auth flow",
			wantForced:    "cloud",
			wantIsMeta:    false,
		},
		{
			name:          "reasoning shorthand",
			prompt:        "@nacho:reasoning solve this mathematical proof",
			wantDirective: "reasoning",
			wantClean:     "solve this mathematical proof",
			wantForced:    "reasoning",
			wantIsMeta:    false,
		},
		{
			name:          "named tier with quotes",
			prompt:        `@nacho:tier="Local ROCm GPU" write a unit test`,
			wantDirective: "tier",
			wantArg:       "Local ROCm GPU",
			wantClean:     "write a unit test",
			wantForced:    "Local ROCm GPU",
			wantIsMeta:    false,
		},
		{
			name:          "named tier unquoted",
			prompt:        `@nacho:tier=LocalGPU write a unit test`,
			wantDirective: "tier",
			wantArg:       "LocalGPU",
			wantClean:     "write a unit test",
			wantForced:    "LocalGPU",
			wantIsMeta:    false,
		},
		{
			name:          "model override with quotes",
			prompt:        `@nacho:model="anthropic/claude-3.7-sonnet" review PR`,
			wantDirective: "model",
			wantArg:       "anthropic/claude-3.7-sonnet",
			wantClean:     "review PR",
			wantModel:     "anthropic/claude-3.7-sonnet",
			wantIsMeta:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, clean := ExtractDirective(tt.prompt)
			if info.Directive != tt.wantDirective {
				t.Errorf("directive: got %q, want %q", info.Directive, tt.wantDirective)
			}
			if info.Arg != tt.wantArg {
				t.Errorf("arg: got %q, want %q", info.Arg, tt.wantArg)
			}
			if clean != tt.wantClean {
				t.Errorf("clean prompt: got %q, want %q", clean, tt.wantClean)
			}
			if info.ForcedTier != tt.wantForced {
				t.Errorf("forced tier: got %q, want %q", info.ForcedTier, tt.wantForced)
			}
			if info.ForcedModel != tt.wantModel {
				t.Errorf("forced model: got %q, want %q", info.ForcedModel, tt.wantModel)
			}
			if info.IsMeta != tt.wantIsMeta {
				t.Errorf("isMeta: got %v, want %v", info.IsMeta, tt.wantIsMeta)
			}
		})
	}
}

func TestExtractDirective_MetaVariants(t *testing.T) {
	tests := []struct {
		name          string
		prompt        string
		wantDirective string
		wantIsMeta    bool
	}{
		{"help", "@nacho:help", "help", true},
		{"tiers", "@nacho:tiers", "tiers", true},
		{"status", "@nacho:status", "status", true},
		{"deals", "@nacho:deals", "deals", true},
		{"unknown typo", "@nacho:fast", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, _ := ExtractDirective(tt.prompt)
			if info.Directive != tt.wantDirective {
				t.Errorf("got directive %q, want %q", info.Directive, tt.wantDirective)
			}
			if !info.IsMeta {
				t.Errorf("expected IsMeta = true for %s", tt.name)
			}
		})
	}
}

func TestExtractDirective_MultiTagConflict_FirstWins(t *testing.T) {
	prompt := "@nacho:local @nacho:cloud @nacho:reasoning refactor this codebase"
	info, clean := ExtractDirective(prompt)

	if info.Directive != "local" {
		t.Errorf("expected first directive 'local', got %q", info.Directive)
	}
	if info.ForcedTier != "local" {
		t.Errorf("expected forced tier 'local', got %q", info.ForcedTier)
	}
	if clean != "refactor this codebase" {
		t.Errorf("expected all tags stripped, got %q", clean)
	}
}

func TestStripDirective_WhitespaceCollapsing(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"@nacho:local hello world", "hello world"},
		{"hello @nacho:local world", "hello world"},
		{"hello world @nacho:local", "hello world"},
		{"   @nacho:local    multiple    spaces   ", "multiple spaces"},
		{"@nacho:tier=\"My Tier\" check", "check"},
		{"no directive here", "no directive here"},
		{"", ""},
	}

	for _, tt := range tests {
		got := StripDirective(tt.input)
		if got != tt.want {
			t.Errorf("StripDirective(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLevenshteinFuzzyMatching(t *testing.T) {
	tests := []struct {
		input       string
		wantClosest string
	}{
		{"lcoal", "local"},
		{"lacol", "local"},
		{"cloudd", "cloud"},
		{"statuss", "status"},
		{"tierr", "tiers"},
		{"halp", "help"},
		{"dealz", "deals"},
		{"completely_unrelated_word_xyz", ""},
	}

	registered := []string{"local", "cloud", "frontier", "reasoning", "help", "tiers", "status", "deals"}

	for _, tt := range tests {
		got := FindClosestDirective(tt.input, registered, 2)
		if got != tt.wantClosest {
			t.Errorf("FindClosestDirective(%q) = %q, want %q", tt.input, got, tt.wantClosest)
		}
	}

	// Edge case tests: empty string
	if d := LevenshteinDistance("", "test"); d != 4 {
		t.Errorf("expected 4, got %d", d)
	}
	if d := LevenshteinDistance("test", ""); d != 4 {
		t.Errorf("expected 4, got %d", d)
	}
	if d := LevenshteinDistance("", ""); d != 0 {
		t.Errorf("expected 0, got %d", d)
	}

	// IsValidDirectiveChar tests
	if !IsValidDirectiveChar('a') || !IsValidDirectiveChar('1') || !IsValidDirectiveChar('_') || !IsValidDirectiveChar('-') {
		t.Errorf("valid chars failed")
	}
	if IsValidDirectiveChar(' ') || IsValidDirectiveChar('!') || IsValidDirectiveChar('@') {
		t.Errorf("invalid chars returned true")
	}
}

func BenchmarkHasDirective_Bailout(b *testing.B) {
	plainPrompt := "Please refactor the user management handler in auth.go to use context timeouts and structured logging."
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = HasDirective(plainPrompt)
	}
}

func TestScanDirectives_FeatureFlags(t *testing.T) {
	tests := []struct {
		name          string
		prompt        string
		wantFlags     FeatureFlag
		wantClean     string
		checkHas      FeatureFlag
		checkNotHas   FeatureFlag
	}{
		{
			name:        "plain prompt defaults to FeatureDefaultAll",
			prompt:      "Write a binary search in Go",
			wantFlags:   FeatureDefaultAll,
			wantClean:   "Write a binary search in Go",
			checkHas:    FeatureShieldEnabled | FeatureToolNormalizer | FeatureThinkNormalizer,
			checkNotHas: 0,
		},
		{
			name:        "empty prompt defaults to FeatureDefaultAll",
			prompt:      "",
			wantFlags:   FeatureDefaultAll,
			wantClean:   "",
			checkHas:    FeatureShieldEnabled | FeatureToolNormalizer,
			checkNotHas: 0,
		},
		{
			name:        "@nacho:raw directive clears all flags",
			prompt:      "@nacho:raw Stream this verbatim without tools",
			wantFlags:   FeatureRawPassThrough,
			wantClean:   "Stream this verbatim without tools",
			checkHas:    0,
			checkNotHas: FeatureShieldEnabled | FeatureToolNormalizer | FeatureThinkNormalizer,
		},
		{
			name:        "@Nacho:RAW uppercase case-insensitive",
			prompt:      "Please @Nacho:RAW return raw JSON payload",
			wantFlags:   FeatureRawPassThrough,
			wantClean:   "Please return raw JSON payload",
			checkHas:    0,
			checkNotHas: FeatureShieldEnabled | FeatureToolNormalizer,
		},
		{
			name:        "@nacho:no-shield disables shield but preserves tool/think normalizers",
			prompt:      "@nacho:no-shield Ask me any clarifying question",
			wantFlags:   FeatureDefaultAll.MaskOut(FeatureShieldEnabled | FeatureShieldFollowup | FeatureShieldModeSwitch),
			wantClean:   "Ask me any clarifying question",
			checkHas:    FeatureToolNormalizer | FeatureThinkNormalizer,
			checkNotHas: FeatureShieldEnabled | FeatureShieldFollowup | FeatureShieldModeSwitch,
		},
		{
			name:        "@Nacho:No-Shield mixed case",
			prompt:      "@Nacho:No-Shield plan the migration",
			wantFlags:   FeatureDefaultAll.MaskOut(FeatureShieldEnabled | FeatureShieldFollowup | FeatureShieldModeSwitch),
			wantClean:   "plan the migration",
			checkHas:    FeatureToolNormalizer,
			checkNotHas: FeatureShieldEnabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFlags, gotClean := ScanDirectives(tt.prompt)
			if gotClean != tt.wantClean {
				t.Errorf("ScanDirectives(%q) cleanPrompt = %q, want %q", tt.prompt, gotClean, tt.wantClean)
			}
			if gotFlags != tt.wantFlags {
				t.Errorf("ScanDirectives(%q) flags = %v, want %v", tt.prompt, gotFlags, tt.wantFlags)
			}
			if tt.checkHas != 0 && !gotFlags.Has(tt.checkHas) {
				t.Errorf("ScanDirectives(%q) expected flags to have %v", tt.prompt, tt.checkHas)
			}
			if tt.checkNotHas != 0 && gotFlags.Has(tt.checkNotHas) {
				t.Errorf("ScanDirectives(%q) expected flags NOT to have %v", tt.prompt, tt.checkNotHas)
			}
		})
	}
}

func BenchmarkScanDirectives(b *testing.B) {
	prompt := "@nacho:raw Stream this unformatted response without synthetic tools"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ScanDirectives(prompt)
	}
}

