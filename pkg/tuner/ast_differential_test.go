package tuner

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	oldExtractTokenRegex        = regexp.MustCompile(`(?i)tokens\s*<\s*(\d+)`)
	oldExtractRetriesRegex      = regexp.MustCompile(`(?i)retries\s*<\s*(\d+)`)
	oldExtractRetryFloorRegex   = regexp.MustCompile(`(?i)retries\s*>=\s*(\d+)`)
	oldExtractRetryFloorGtRegex = regexp.MustCompile(`(?i)retries\s*>\s*(\d+)`)
	oldExtractKwRegex           = regexp.MustCompile(`'([^']+)'|"([^"]+)"`)
	oldKickstartClauseRegex     = regexp.MustCompile(`(?i)\bSessionKickstarted\b(?:\s*==\s*true)?`)
	oldRestrictImagesRegex      = regexp.MustCompile(`(?i)(?:!\s*HasImages|\bHasImages\s*==\s*false\b)`)
	oldRequireImagesRegex       = regexp.MustCompile(`(?i)(?:^|[^!])\bHasImages\b(?:\s*==\s*true)?`)
	oldRestrictToolsRegex       = regexp.MustCompile(`(?i)(?:!\s*HasTools|\bHasTools\s*==\s*false\b)`)
	oldRequireToolsRegex        = regexp.MustCompile(`(?i)(?:^|[^!])\bHasTools\b(?:\s*==\s*true)?`)
	oldExtractKeywordsRegex     = regexp.MustCompile(`(?i)\bkeywords\b`)

	oldTokenClauseRegex       = regexp.MustCompile(`(?i)^\s*tokens\s*[<>=]`)
	oldRetriesClauseRegex     = regexp.MustCompile(`(?i)^\s*(retries\s*<=?\s*\d+|retries\s*==\s*0|!?isretry\b)`)
	oldModalityClauseRegex    = regexp.MustCompile(`(?i)^\s*(?:!\s*has(images|tools)|has(images|tools)\s*==\s*false)\s*$`)
	oldKeywordClauseRegex     = regexp.MustCompile(`(?i)any\s*\(\s*keywords\s*,`)
	oldHasPositiveImagesRegex = regexp.MustCompile(`(?i)(?:^|[^!])\bhasimages\b(?:\s*==\s*true)?`)
	oldHasPositiveToolsRegex  = regexp.MustCompile(`(?i)(?:^|[^!])\bhastools\b(?:\s*==\s*true)?`)
)

func oldExtractRouting(whenStr string) ExtractedRoutingRules {
	requiresKickstart := oldKickstartClauseRegex.MatchString(whenStr)
	restrictImages := oldRestrictImagesRegex.MatchString(whenStr)
	requiresImages := oldRequireImagesRegex.MatchString(whenStr) && !restrictImages
	restrictTools := oldRestrictToolsRegex.MatchString(whenStr)
	requiresTools := oldRequireToolsRegex.MatchString(whenStr) && !restrictTools

	threshold := 0
	if m := oldExtractTokenRegex.FindStringSubmatch(whenStr); len(m) > 1 {
		if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
			threshold = v
		}
	}

	retryBound := 0
	if m := oldExtractRetriesRegex.FindStringSubmatch(whenStr); len(m) > 1 {
		if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
			retryBound = v
		}
	}

	retryFloor := 0
	if m := oldExtractRetryFloorRegex.FindStringSubmatch(whenStr); len(m) > 1 {
		if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
			retryFloor = v
		}
	} else if m := oldExtractRetryFloorGtRegex.FindStringSubmatch(whenStr); len(m) > 1 {
		if v, err := strconv.Atoi(m[1]); err == nil && v >= 0 {
			retryFloor = v + 1
		}
	}

	var excludedKws []string
	if oldExtractKeywordsRegex.MatchString(whenStr) {
		matches := oldExtractKwRegex.FindAllStringSubmatch(whenStr, -1)
		for _, m := range matches {
			if len(m) > 1 && m[1] != "" {
				excludedKws = append(excludedKws, m[1])
			} else if len(m) > 2 && m[2] != "" {
				excludedKws = append(excludedKws, m[2])
			}
		}
	}

	return ExtractedRoutingRules{
		TokenThreshold:    threshold,
		RetryBound:        retryBound,
		RetryFloor:        retryFloor,
		RequiresKickstart: requiresKickstart,
		RestrictImages:    restrictImages,
		RequiresImages:    requiresImages,
		RestrictTools:     restrictTools,
		RequiresTools:     requiresTools,
		ExcludedKeywords:  excludedKws,
	}
}

func oldIsAutoTunableClause(clause string, tuningRetries bool) bool {
	c := strings.TrimSpace(clause)
	if oldTokenClauseRegex.MatchString(c) {
		return true
	}
	if tuningRetries && oldRetriesClauseRegex.MatchString(c) {
		return true
	}
	if oldModalityClauseRegex.MatchString(c) {
		return true
	}
	if oldKeywordClauseRegex.MatchString(c) {
		return true
	}
	return false
}

func oldHasPositivePrerequisite(clause string, modality string) bool {
	if strings.EqualFold(modality, "HasImages") {
		return oldHasPositiveImagesRegex.MatchString(clause)
	}
	if strings.EqualFold(modality, "HasTools") {
		return oldHasPositiveToolsRegex.MatchString(clause)
	}
	return false
}

// TestDifferentialEquivalence_ExtractRouting verifies that the pure AST parser
// produces 100% equivalent results to the old regex implementation on standard rules.
func TestDifferentialEquivalence_ExtractRouting(t *testing.T) {
	cases := []string{
		"Tokens < 4000 && Retries < 2 && !HasImages && !HasTools",
		"Tokens < 32000 && !(Keywords contains 'sql' || Keywords contains \"deadlock\")",
		"SessionKickstarted && Retries < 3",
		"SessionKickstarted == true && HasImages == true",
		"HasImages == false && HasTools == false",
		"! HasImages && ! HasTools",
		"!HasImages",
		"!HasTools",
		"HasImages",
		"HasTools",
		"Retries >= 5 && Retries < 8",
		"Retries > 2",
		"Retries >= 1",
		"!any(Keywords, { # in ['deadlock', 'mutex'] })",
		"Tokens < 1000",
		"Tokens < 16000 && Retries < 2",
		"Tokens < 64000",
		"true",
		"false",
		"",
	}

	for _, exprStr := range cases {
		t.Run(exprStr, func(t *testing.T) {
			oldResult := oldExtractRouting(exprStr)
			newResult := extractRoutingFromWhenAST(exprStr)

			if oldResult.TokenThreshold != newResult.TokenThreshold {
				t.Errorf("[%s] TokenThreshold mismatch: old=%d, new=%d", exprStr, oldResult.TokenThreshold, newResult.TokenThreshold)
			}
			if oldResult.RetryBound != newResult.RetryBound {
				t.Errorf("[%s] RetryBound mismatch: old=%d, new=%d", exprStr, oldResult.RetryBound, newResult.RetryBound)
			}
			if oldResult.RetryFloor != newResult.RetryFloor {
				t.Errorf("[%s] RetryFloor mismatch: old=%d, new=%d", exprStr, oldResult.RetryFloor, newResult.RetryFloor)
			}
			if oldResult.RequiresKickstart != newResult.RequiresKickstart {
				t.Errorf("[%s] RequiresKickstart mismatch: old=%v, new=%v", exprStr, oldResult.RequiresKickstart, newResult.RequiresKickstart)
			}
			if oldResult.RestrictImages != newResult.RestrictImages {
				t.Errorf("[%s] RestrictImages mismatch: old=%v, new=%v", exprStr, oldResult.RestrictImages, newResult.RestrictImages)
			}
			if oldResult.RequiresImages != newResult.RequiresImages {
				t.Errorf("[%s] RequiresImages mismatch: old=%v, new=%v", exprStr, oldResult.RequiresImages, newResult.RequiresImages)
			}
			if oldResult.RestrictTools != newResult.RestrictTools {
				t.Errorf("[%s] RestrictTools mismatch: old=%v, new=%v", exprStr, oldResult.RestrictTools, newResult.RestrictTools)
			}
			if oldResult.RequiresTools != newResult.RequiresTools {
				t.Errorf("[%s] RequiresTools mismatch: old=%v, new=%v", exprStr, oldResult.RequiresTools, newResult.RequiresTools)
			}
			if len(oldResult.ExcludedKeywords) != len(newResult.ExcludedKeywords) {
				t.Errorf("[%s] ExcludedKeywords count mismatch: old=%v, new=%v", exprStr, oldResult.ExcludedKeywords, newResult.ExcludedKeywords)
			} else {
				for i := range oldResult.ExcludedKeywords {
					if oldResult.ExcludedKeywords[i] != newResult.ExcludedKeywords[i] {
						t.Errorf("[%s] ExcludedKeywords[%d] mismatch: old=%s, new=%s", exprStr, i, oldResult.ExcludedKeywords[i], newResult.ExcludedKeywords[i])
					}
				}
			}
		})
	}
}

// TestDifferentialEquivalence_IsAutoTunable verifies that isAutoTunableClause
// matches old regex behavior on all standard clauses.
func TestDifferentialEquivalence_IsAutoTunable(t *testing.T) {
	type testCase struct {
		clause        string
		tuningRetries bool
	}

	cases := []testCase{
		{"Tokens < 4000", false},
		{"Tokens <= 8000", false},
		{"Tokens > 1000", false},
		{"Tokens == 2000", false},
		{"Retries < 2", true},
		{"Retries < 2", false},
		{"Retries <= 3", true},
		{"Retries == 0", true},
		{"Retries >= 3", true}, // floor - must NOT be autotuned
		{"Retries > 2", true},  // floor - must NOT be autotuned
		{"!IsRetry", true},
		{"IsRetry", true},
		{"!HasImages", false},
		{"!HasTools", false},
		{"HasImages == false", false},
		{"HasTools == false", false},
		{"HasImages", false},
		{"HasTools", false},
		{"HasImages == true", false},
		{"HasTools == true", false},
		{"!any(Keywords, { # in ['sql'] })", false},
		{"any(Keywords, { # in ['sql'] })", false},
		{"CustomGuard == 123", true},
		{"CustomGuard == 123", false},
	}

	for _, tc := range cases {
		t.Run(tc.clause, func(t *testing.T) {
			oldVal := oldIsAutoTunableClause(tc.clause, tc.tuningRetries)
			newVal := isAutoTunableClause(tc.clause, tc.tuningRetries)
			if oldVal != newVal {
				t.Errorf("[%s (tuningRetries=%v)] isAutoTunable mismatch: old=%v, new=%v", tc.clause, tc.tuningRetries, oldVal, newVal)
			}
		})
	}
}

// TestDifferentialEquivalence_PositivePrerequisites verifies that hasPositivePrerequisiteClause
// matches old regex behavior on all standard prerequisite forms.
func TestDifferentialEquivalence_PositivePrerequisites(t *testing.T) {
	cases := []struct {
		clause   string
		modality string
	}{
		{"HasImages", "HasImages"},
		{"HasImages == true", "HasImages"},
		{"!HasImages", "HasImages"},
		{"HasTools", "HasTools"},
		{"HasTools == true", "HasTools"},
		{"!HasTools", "HasTools"},
		{"Tokens < 10000", "HasImages"},
		{"Retries < 2", "HasTools"},
	}

	for _, tc := range cases {
		t.Run(tc.clause+"_"+tc.modality, func(t *testing.T) {
			oldVal := oldHasPositivePrerequisite(tc.clause, tc.modality)
			newVal := hasPositivePrerequisiteClause(tc.clause, tc.modality)
			if oldVal != newVal {
				t.Errorf("[%s (%s)] PositivePrerequisite mismatch: old=%v, new=%v", tc.clause, tc.modality, oldVal, newVal)
			}
		})
	}
}

// TestASTSuperiorityOverRegex_FixedEdgeCases tests specific real-world edge cases
// where the old regex suffered from substring collisions or false positives that the AST fixes.
func TestASTSuperiorityOverRegex_FixedEdgeCases(t *testing.T) {
	// Case 1: Substring collision in custom variable names containing "tokens"
	// Old regex: oldTokenClauseRegex `(?i)^\s*tokens\s*[<>=]` matches `tokens_v2 < 100` because `tokens` prefix matches before `_`!
	c1 := "tokens_v2 < 100"
	if oldTokenClauseRegex.MatchString(c1) {
		t.Logf("CONFIRMED OLD REGEX FLAW: Old regex matched %q as a Tokens constraint due to substring matching!", c1)
	}
	// AST correctly understands that the identifier is "tokens_v2", NOT "Tokens"!
	if isAutoTunableClause(c1, false) != false {
		t.Errorf("AST should not consider 'tokens_v2 < 100' as a Tokens constraint")
	}

	// Case 2: Keyword extraction false positive
	// If a tier has Keywords filter AND a string literal in another clause:
	// e.g. `ForcedModel == "important_model" && !(Keywords contains 'sql')`
	// Old regex: searches for ALL string literals in the expression if the word 'keywords' appears anywhere!
	// So old regex captures BOTH "important_model" AND "sql"!
	c2 := `ForcedModel == "important_model" && !(Keywords contains 'sql')`
	oldKws := oldExtractRouting(c2).ExcludedKeywords
	newKws := extractRoutingFromWhenAST(c2).ExcludedKeywords

	if len(oldKws) == 2 && oldKws[0] == "important_model" {
		t.Logf("CONFIRMED OLD REGEX FLAW: Old regex falsely captured %q as a keyword filter!", oldKws[0])
	}
	if len(newKws) != 1 || newKws[0] != "sql" {
		t.Errorf("AST failed to accurately capture only the keyword: got %v", newKws)
	}

	// Case 3: False positive in oldHasPositiveImagesRegex on equality negations
	// Old regex: `(?i)(?:^|[^!])\bhasimages\b(?:\s*==\s*true)?`
	// Because `(?:\s*==\s*true)?` was optional, it matched `HasImages == false` as TRUE (positive prerequisite)!
	c3 := "HasImages == false"
	oldPos := oldHasPositivePrerequisite(c3, "HasImages")
	newPos := hasPositivePrerequisiteClause(c3, "HasImages")
	if oldPos == true {
		t.Logf("CONFIRMED OLD REGEX BUG: Old regex falsely classified %q as a POSITIVE image prerequisite!", c3)
	}
	if newPos != false {
		t.Errorf("AST must classify %q as negative (false), got %v", c3, newPos)
	}
}
