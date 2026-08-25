package router

import (
	"regexp"
	"strings"
	"unicode"
)

const DirectivePrefix = "@nacho:"

var (
	directiveRegex  = regexp.MustCompile(`(?i)@nacho:(tier="([^"]+)"|tier=([^\s]+)|model="([^"]+)"|model=([^\s]+)|([a-zA-Z0-9_\-]+))`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// DirectiveInfo represents the parsed details of a @nacho: directive.
type DirectiveInfo struct {
	Directive   string // "local", "cloud", "reasoning", "tier", "model", "help", "tiers", "status", "deals", "unknown"
	Arg         string // Argument value for tier="..." or model="..."
	ForcedTier  string // Resolved forced tier identifier
	ForcedModel string // Resolved forced model ID
	IsMeta      bool   // True if this directive is handled locally ($0.00 / 0ms)
	Raw         string // The raw matching token (e.g. "@nacho:fast")
}

// HasDirective performs a < 4ns zero-allocation byte pre-filter on the prompt.
func HasDirective(text string) bool {
	return strings.Contains(text, DirectivePrefix)
}

// ExtractDirective parses the first @nacho: directive in prompt and strips all directives from clean prompt.
func ExtractDirective(prompt string) (DirectiveInfo, string) {
	if !HasDirective(prompt) {
		return DirectiveInfo{}, prompt
	}

	matches := directiveRegex.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return DirectiveInfo{}, prompt
	}

	first := matches[0]
	rawToken := first[0]

	info := DirectiveInfo{
		Raw: rawToken,
	}

	// Capture groups:
	// first[1] = full directive argument after "@nacho:"
	// first[2] = tier quoted value
	// first[3] = tier unquoted value
	// first[4] = model quoted value
	// first[5] = model unquoted value
	// first[6] = keyword token (e.g. "local", "cloud", "help", "fast")

	if first[2] != "" {
		info.Directive = "tier"
		info.Arg = first[2]
		info.ForcedTier = first[2]
	} else if first[3] != "" {
		info.Directive = "tier"
		info.Arg = first[3]
		info.ForcedTier = first[3]
	} else if first[4] != "" {
		info.Directive = "model"
		info.Arg = first[4]
		info.ForcedModel = first[4]
	} else if first[5] != "" {
		info.Directive = "model"
		info.Arg = first[5]
		info.ForcedModel = first[5]
	} else {
		keyword := strings.ToLower(first[6])
		switch keyword {
		case "local":
			info.Directive = "local"
			info.ForcedTier = "local"
		case "cloud":
			info.Directive = "cloud"
			info.ForcedTier = "cloud"
		case "frontier":
			info.Directive = "frontier"
			info.ForcedTier = "cloud"
		case "reasoning":
			info.Directive = "reasoning"
			info.ForcedTier = "reasoning"
		case "help":
			info.Directive = "help"
			info.IsMeta = true
		case "tiers":
			info.Directive = "tiers"
			info.IsMeta = true
		case "status":
			info.Directive = "status"
			info.IsMeta = true
		case "deals":
			info.Directive = "deals"
			info.IsMeta = true
		default:
			info.Directive = "unknown"
			info.Arg = keyword
			info.IsMeta = true
		}
	}

	clean := StripDirective(prompt)
	return info, clean
}

// StripDirective removes all @nacho:... directives and collapses excess whitespace.
func StripDirective(text string) string {
	if text == "" || !HasDirective(text) {
		return text
	}

	stripped := directiveRegex.ReplaceAllString(text, " ")
	collapsed := whitespaceRegex.ReplaceAllString(stripped, " ")
	return strings.TrimSpace(collapsed)
}

// LevenshteinDistance calculates the minimum edit operations between two strings.
func LevenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(strings.ToLower(s1)), []rune(strings.ToLower(s2))
	n, m := len(r1), len(r2)

	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	matrix := make([][]int, n+1)
	for i := range matrix {
		matrix[i] = make([]int, m+1)
		matrix[i][0] = i
	}
	for j := 0; j <= m; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			del := matrix[i-1][j] + 1
			ins := matrix[i][j-1] + 1
			sub := matrix[i-1][j-1] + cost

			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			matrix[i][j] = min
		}
	}

	return matrix[n][m]
}

// FindClosestDirective searches candidates for the string with minimal Levenshtein distance within maxDist.
func FindClosestDirective(input string, candidates []string, maxDist int) string {
	input = strings.ToLower(input)
	bestMatch := ""
	bestDistance := maxDist + 1

	for _, cand := range candidates {
		d := LevenshteinDistance(input, cand)
		if d < bestDistance {
			bestDistance = d
			bestMatch = cand
		}
	}

	if bestDistance <= maxDist {
		return bestMatch
	}
	return ""
}

// IsValidDirectiveChar checks if character is allowed in unquoted directive keywords.
func IsValidDirectiveChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-'
}
