package router

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	// reGutter matches line numbers followed by a pipe: "  168 | " or "168 |"
	reGutter = regexp.MustCompile(`^\s*\d+\s*\|\s?`)
	// reColonIndicator matches standalone line indicators: ":168:"
	reColonIndicator = regexp.MustCompile(`^:\d+:\s*$`)
	// reLinePrefix matches standard prefixed line numbers: "168: " while preserving tabs/indentation
	reLinePrefix = regexp.MustCompile(`^\s*\d+:\s?`)
)

// SanitizeDiff cleans LLM-hallucinated line number indicators inside <<<<<<< SEARCH blocks.
// It leaves the replacement block (======= to >>>>>>>) strictly untouched.
func SanitizeDiff(diff string) string {
	if !strings.Contains(diff, "<<<<<<< SEARCH") {
		return diff
	}

	lines := strings.Split(diff, "\n")
	var result []string
	inSearchBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "<<<<<<< SEARCH") {
			inSearchBlock = true
			result = append(result, line)
			continue
		}

		if strings.HasPrefix(trimmed, "=======") || strings.HasPrefix(trimmed, ">>>>>>>") {
			inSearchBlock = false
			result = append(result, line)
			continue
		}

		if inSearchBlock {
			// If the line is purely an indicator like ":168:", drop it completely
			if reColonIndicator.MatchString(trimmed) {
				continue
			}

			// If the line starts with gutter like "168 | const x = 1;", strip the gutter
			if reGutter.MatchString(line) {
				cleaned := reGutter.ReplaceAllString(line, "")
				result = append(result, cleaned)
				continue
			}

			// If the line starts with standard line prefix "168: const x = 1;", strip prefix
			if reLinePrefix.MatchString(line) {
				cleaned := reLinePrefix.ReplaceAllString(line, "")
				result = append(result, cleaned)
				continue
			}
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// SanitizeToolCallArguments parses tool call JSON and sanitizes any embedded diff argument.
func SanitizeToolCallArguments(toolName, argsJSON string) string {
	lowerName := strings.ToLower(toolName)
	if !strings.Contains(lowerName, "diff") && !strings.Contains(lowerName, "patch") && !strings.Contains(lowerName, "edit") {
		if !strings.Contains(argsJSON, "<<<<<<< SEARCH") {
			return argsJSON
		}
	}

	var argsMap map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &argsMap); err != nil {
		if strings.Contains(argsJSON, "<<<<<<< SEARCH") {
			return SanitizeDiff(argsJSON)
		}
		return argsJSON
	}

	modified := false
	for k, v := range argsMap {
		if strVal, ok := v.(string); ok && strings.Contains(strVal, "<<<<<<< SEARCH") {
			sanitized := SanitizeDiff(strVal)
			if sanitized != strVal {
				argsMap[k] = sanitized
				modified = true
			}
		}
	}

	if modified {
		sanitizedBytes, err := json.Marshal(argsMap)
		if err == nil {
			return string(sanitizedBytes)
		}
	}

	return argsJSON
}
