package router

import (
	"regexp"
	"strings"
	"unicode"
)

const DirectivePrefix = "@nacho:"

var (
	directiveRegex  = regexp.MustCompile(`(?i)@nacho:([a-zA-Z0-9_\-]+)(?:=(?:"([^"]+)"|([^\s]+)))?`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// DirectiveInfo represents the parsed details of a @nacho: directive.
type DirectiveInfo struct {
	Directive   string // "local", "cloud", "reasoning", "tier", "model", "kickstart-off", "kickstart-on", "cyclekiller-off", "cyclekiller-on", "shield-off", "shield-on", "raw-on", "raw-off", "fairydust-off", "fairydust-on", "help", "tiers", "status", "toggles", "reset", "unknown"
	Arg         string // Argument value for tier="..." or model="..."
	ForcedTier  string // Resolved forced tier identifier
	ForcedModel string // Resolved forced model ID
	IsMeta      bool   // True if this directive is handled locally ($0.00 / 0ms)
	Raw         string // The raw matching token (e.g. "@nacho:fast")
}

// HasDirective performs a fast, zero-allocation byte pre-filter on the prompt.
func HasDirective(text string) bool {
	if len(text) < len(DirectivePrefix) {
		return false
	}
	prefixLen := len(DirectivePrefix)
	for i := 0; i <= len(text)-prefixLen; i++ {
		if text[i] == '@' {
			if strings.EqualFold(text[i:i+prefixLen], DirectivePrefix) {
				return true
			}
		}
	}
	return false
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
	key := strings.ToLower(first[1])

	val := first[2]
	if val == "" {
		val = first[3]
	}

	info := DirectiveInfo{
		Raw: rawToken,
	}

	clean := StripDirective(prompt)

	switch key {
	case "tier":
		info.Directive = "tier"
		info.Arg = val
		info.ForcedTier = val
	case "model":
		info.Directive = "model"
		info.Arg = val
		info.ForcedModel = val
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
	case "kickstart":
		vLower := strings.ToLower(val)
		if vLower == "off" || vLower == "false" || vLower == "0" {
			info.Directive = "kickstart-off"
		} else if vLower == "on" || vLower == "true" || vLower == "1" {
			info.Directive = "kickstart-on"
		} else {
			info.Directive = "unknown"
			info.Arg = key + "=" + val
			info.IsMeta = true
		}
	case "kickstart-off", "kickstart_off", "no-kick", "nokick", "no_kick", "disable-kick", "disable_kick":
		info.Directive = "kickstart-off"
	case "kickstart-on", "kickstart_on", "kick", "enable-kick", "enable_kick":
		info.Directive = "kickstart-on"
	case "cyclekiller", "cycle-killer", "cycle_killer":
		vLower := strings.ToLower(val)
		if vLower == "off" || vLower == "false" || vLower == "0" {
			info.Directive = "cyclekiller-off"
		} else if vLower == "on" || vLower == "true" || vLower == "1" {
			info.Directive = "cyclekiller-on"
		} else {
			info.Directive = "unknown"
			info.Arg = key + "=" + val
			info.IsMeta = true
		}
	case "cyclekiller-off", "cyclekiller_off", "cycle-killer-off", "cycle-killer_off", "disable-cyclekiller":
		info.Directive = "cyclekiller-off"
	case "cyclekiller-on", "cyclekiller_on", "cycle-killer-on", "cycle-killer_on", "enable-cyclekiller":
		info.Directive = "cyclekiller-on"
	case "shield":
		vLower := strings.ToLower(val)
		if vLower == "off" || vLower == "false" || vLower == "0" {
			info.Directive = "shield-off"
		} else if vLower == "on" || vLower == "true" || vLower == "1" {
			info.Directive = "shield-on"
		} else {
			info.Directive = "unknown"
			info.Arg = key + "=" + val
			info.IsMeta = true
		}
	case "shield-off", "shield_off", "no-shield", "noshield", "no_shield", "disable-shield":
		info.Directive = "shield-off"
	case "shield-on", "shield_on", "enable-shield":
		info.Directive = "shield-on"
	case "raw":
		vLower := strings.ToLower(val)
		if vLower == "off" || vLower == "false" {
			info.Directive = "raw-off"
		} else if vLower == "on" || vLower == "true" {
			info.Directive = "raw-on"
		} else if val == "" {
			info.Directive = "raw"
		} else {
			info.Directive = "unknown"
			info.Arg = key + "=" + val
			info.IsMeta = true
		}
	case "raw-on", "raw_on":
		info.Directive = "raw-on"
	case "raw-off", "raw_off":
		info.Directive = "raw-off"
	case "fairydust", "fairy-dust", "fairy_dust":
		vLower := strings.ToLower(val)
		if vLower == "off" || vLower == "false" {
			info.Directive = "fairydust-off"
		} else if vLower == "on" || vLower == "true" {
			info.Directive = "fairydust-on"
		} else {
			info.Directive = "unknown"
			info.Arg = key + "=" + val
			info.IsMeta = true
		}
	case "fairydust-off", "fairydust_off", "fairy-dust-off", "fairy_dust_off", "disable-fairydust":
		info.Directive = "fairydust-off"
	case "fairydust-on", "fairydust_on", "fairy-dust-on", "fairy_dust_on", "enable-fairydust":
		info.Directive = "fairydust-on"
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
	case "toggles", "toggle", "guardrails", "guardrail", "features", "feature":
		info.Directive = "toggles"
		info.IsMeta = true
	case "reset", "clear":
		info.Directive = "reset"
		info.IsMeta = true
	default:
		info.Directive = "unknown"
		if val != "" {
			info.Arg = key + "=" + val
		} else {
			info.Arg = key
		}
		info.IsMeta = true
	}

	// Standalone toggle check: if clean is empty (the prompt was solely the directive),
	// mark toggles as meta directives for instant local ($0.00 / 0ms) acknowledgment.
	if clean == "" && !info.IsMeta {
		switch info.Directive {
		case "kickstart-off", "kickstart-on",
			"cyclekiller-off", "cyclekiller-on",
			"shield-off", "shield-on",
			"raw-on", "raw-off",
			"fairydust-off", "fairydust-on":
			info.IsMeta = true
		}
	}

	return info, clean
}

// ScanDirectives scans the prompt for spicy feature directives (@nacho:raw, @nacho:no-shield)
// and returns the resolved FeatureFlag bitmask and the stripped clean prompt.
// If no feature directive is present, it returns FeatureDefaultAll.
func ScanDirectives(prompt string) (FeatureFlag, string) {
	if prompt == "" || !HasDirective(prompt) {
		return FeatureDefaultAll, prompt
	}

	flags := FeatureDefaultAll
	lower := strings.ToLower(prompt)

	if strings.Contains(lower, "@nacho:raw") {
		flags = FeatureRawPassThrough
	} else if strings.Contains(lower, "@nacho:no-shield") || strings.Contains(lower, "@nacho:noshield") ||
		strings.Contains(lower, "@nacho:shield-off") || strings.Contains(lower, "@nacho:shield=off") {
		flags = flags.MaskOut(FeatureShieldEnabled | FeatureShieldFollowup | FeatureShieldModeSwitch)
	}

	clean := StripDirective(prompt)
	return flags, clean
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
