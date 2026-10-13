package nts

import (
	"fmt"
)

// DefaultMinDuplicateOutputBytes is the default minimum byte length for a generic
// tool output to be considered for duplicate replacement.
const DefaultMinDuplicateOutputBytes = 200

// ToolDedupContext tracks previously observed outputs of CategoryGeneric tools
// in a single request execution to replace identical subsequent outputs with references.
type ToolDedupContext struct {
	minBytes int
	seen     map[uint64]string
}

// NewToolDedupContext creates a new ToolDedupContext with the given minimum output threshold.
func NewToolDedupContext(minBytes int) *ToolDedupContext {
	if minBytes <= 0 {
		minBytes = DefaultMinDuplicateOutputBytes
	}
	return &ToolDedupContext{
		minBytes: minBytes,
		seen:     make(map[uint64]string),
	}
}

// Deduplicate checks if content represents a repeated CategoryGeneric tool result.
// If repeated, returns tombstone replacement and ReductionResult.
func (ctx *ToolDedupContext) Deduplicate(content interface{}, toolName string, isError bool) (interface{}, ReductionResult, bool) {
	if ctx == nil {
		return content, ReductionResult{}, false
	}

	// 1. Strict Category Restriction: Only generic tools (e.g. bash/terminal) are candidates.
	// File reads and writes are 100% immune from command output deduplication.
	if toolName == "" || categorizeToolName(toolName) != CategoryGeneric {
		return content, ReductionResult{}, false
	}

	switch v := content.(type) {
	case string:
		return ctx.dedupString(v, isError)

	case []interface{}:
		var total ReductionResult
		modified := false
		for _, item := range v {
			if part, ok := item.(map[string]interface{}); ok && part["type"] == "text" {
				if textStr, ok := part["text"].(string); ok {
					newText, res, changed := ctx.dedupString(textStr, isError)
					if changed {
						part["text"] = newText
						total.merge(res)
						modified = true
					}
				}
			}
		}
		if modified {
			return v, total, true
		}
		return content, ReductionResult{}, false

	default:
		return content, ReductionResult{}, false
	}
}

func (ctx *ToolDedupContext) dedupString(s string, isError bool) (string, ReductionResult, bool) {
	if len(s) < ctx.minBytes {
		return s, ReductionResult{}, false
	}

	// 2. Error Immunity: Errors and failure outputs must always be preserved verbatim.
	if IsToolError(s, isError) {
		return s, ReductionResult{}, false
	}

	h := fnv64String(s)
	if prev, exists := ctx.seen[h]; exists && prev == s {
		origBytes := len(s)
		replacement := fmt.Sprintf("[Output identical to previous command execution above (%d bytes omitted)]", origBytes)
		reducedBytes := len(replacement)
		saved := origBytes - reducedBytes
		if saved < 0 {
			saved = 0
		}
		return replacement, ReductionResult{
			OriginalBytes: origBytes,
			ReducedBytes:  reducedBytes,
			BytesSaved:    saved,
			TokensSaved:   (saved + 3) / 4,
		}, true
	}

	ctx.seen[h] = s
	return s, ReductionResult{}, false
}

func fnv64String(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
