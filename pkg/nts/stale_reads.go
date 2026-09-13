package nts

import (
	"strconv"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
)

// StaleFileReadNotice is the constant replacement string for superseded historical file reads.
const StaleFileReadNotice = "[NTS: File content superseded by later read/write]"

var (
	rawPathCandidates = []string{
		"path", "filePath", "file_path", "target_file", "TargetFile",
		"file", "filename", "file_name", "uri", "AbsolutePath",
	}
	mapStartLineKeys = []string{"startLine", "start_line", "StartLine", "offset", "start"}
	mapEndLineKeys   = []string{"endLine", "end_line", "EndLine", "end"}
	mapLimitKeys     = []string{"limit", "count", "lines"}

	jsonQuotedPathCandidates = []string{
		`"path":`, `"filePath":`, `"file_path":`, `"target_file":`,
		`"TargetFile":`, `"file":`, `"filename":`, `"file_name":`,
		`"uri":`, `"AbsolutePath":`,
	}
	jsonQuotedStartLineKeys = []string{`"startLine":`, `"start_line":`, `"StartLine":`, `"offset":`, `"start":`}
	jsonQuotedEndLineKeys   = []string{`"endLine":`, `"end_line":`, `"EndLine":`, `"end":`}
	jsonQuotedLimitKeys     = []string{`"limit":`, `"count":`, `"lines":`}
)

// fnv64 computes a 64-bit FNV-1a hash of a string with zero heap allocation.
func fnv64(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// fileToolCall captures the metadata of a single file read or write invocation.
type fileToolCall struct {
	id       string
	basePath string
	pathKey  string
	isRead   bool
	isWrite  bool
}

// IdentifyStaleToolCallIDs inspects conversation messages in chronological order,
// then scans backwards to identify tool call IDs of file reads that were superseded
// by later reads or writes of the same file. It preserves up to depth recent reads (default: 1).
func IdentifyStaleToolCallIDs(msgs []interface{}, depth ...int) map[string]struct{} {
	if len(msgs) == 0 {
		return nil
	}

	retentionDepth := 1
	if len(depth) > 0 && depth[0] > 0 {
		retentionDepth = depth[0]
	}

	calls := collectFileToolCalls(msgs)
	if len(calls) <= 1 {
		return nil
	}

	return identifySupersededCalls(calls, retentionDepth)
}

func collectFileToolCalls(msgs []interface{}) []fileToolCall {
	var calls []fileToolCall

	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}

		// Format A: OpenAI tool_calls
		if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
			for _, tcItem := range toolCalls {
				tc, ok := tcItem.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := tc["id"].(string)
				fn, ok := tc["function"].(map[string]interface{})
				if !ok || id == "" {
					continue
				}
				name, _ := fn["name"].(string)
				isRead := isFileReadTool(name)
				isWrite := isFileWriteTool(name)
				if !isRead && !isWrite {
					continue
				}

				basePath, pathKey := extractFilePaths(fn["arguments"])
				if basePath != "" {
					calls = append(calls, fileToolCall{
						id:       id,
						basePath: basePath,
						pathKey:  pathKey,
						isRead:   isRead,
						isWrite:  isWrite,
					})
				}
			}
		}

		// Format B: Anthropic content blocks with type == "tool_use"
		if contentList, ok := msg["content"].([]interface{}); ok {
			for _, partItem := range contentList {
				part, ok := partItem.(map[string]interface{})
				if !ok || part["type"] != "tool_use" {
					continue
				}
				id, _ := part["id"].(string)
				name, _ := part["name"].(string)
				if id == "" {
					continue
				}

				isRead := isFileReadTool(name)
				isWrite := isFileWriteTool(name)
				if !isRead && !isWrite {
					continue
				}

				basePath, pathKey := extractFilePaths(part["input"])
				if basePath != "" {
					calls = append(calls, fileToolCall{
						id:       id,
						basePath: basePath,
						pathKey:  pathKey,
						isRead:   isRead,
						isWrite:  isWrite,
					})
				}
			}
		}
	}

	return calls
}

func identifySupersededCalls(calls []fileToolCall, retentionDepth int) map[string]struct{} {
	var seenWrites [256]uint64
	seenWritesCount := 0

	var seenFullFiles [256]uint64
	var seenFullCounts [256]uint8
	seenFullCount := 0

	var seenRangedFiles [256]uint64
	var seenRangedCounts [256]uint8
	seenRangedCount := 0

	staleIDs := make(map[string]struct{})

	for i := len(calls) - 1; i >= 0; i-- {
		c := calls[i]
		baseHash := fnv64(c.basePath)
		keyHash := fnv64(c.pathKey)

		if c.isWrite {
			// A write modifies the file on disk. Any reads prior to this write are obsolete.
			if !containsHash(seenWrites[:seenWritesCount], baseHash) {
				if seenWritesCount < len(seenWrites) {
					seenWrites[seenWritesCount] = baseHash
					seenWritesCount++
				}
			}
			continue
		}

		if c.isRead {
			// Check if a later write modified this file
			if containsHash(seenWrites[:seenWritesCount], baseHash) {
				staleIDs[c.id] = struct{}{}
				continue
			}

			if c.basePath == c.pathKey {
				// Full file read
				idx := findHashIndex(seenFullFiles[:seenFullCount], baseHash)
				if idx >= 0 {
					seenFullCounts[idx]++
					if int(seenFullCounts[idx]) > retentionDepth {
						staleIDs[c.id] = struct{}{}
					}
				} else {
					if seenFullCount < len(seenFullFiles) {
						seenFullFiles[seenFullCount] = baseHash
						seenFullCounts[seenFullCount] = 1
						seenFullCount++
					}
				}
			} else {
				// Specific line range read
				// If full reads of this file already exceeded retention depth, range is stale
				idxFull := findHashIndex(seenFullFiles[:seenFullCount], baseHash)
				if idxFull >= 0 && int(seenFullCounts[idxFull]) >= retentionDepth {
					staleIDs[c.id] = struct{}{}
					continue
				}

				idx := findHashIndex(seenRangedFiles[:seenRangedCount], keyHash)
				if idx >= 0 {
					seenRangedCounts[idx]++
					if int(seenRangedCounts[idx]) > retentionDepth {
						staleIDs[c.id] = struct{}{}
					}
				} else {
					if seenRangedCount < len(seenRangedFiles) {
						seenRangedFiles[seenRangedCount] = keyHash
						seenRangedCounts[seenRangedCount] = 1
						seenRangedCount++
					}
				}
			}
		}
	}

	if len(staleIDs) == 0 {
		return nil
	}
	return staleIDs
}

func containsHash(slice []uint64, target uint64) bool {
	for _, h := range slice {
		if h == target {
			return true
		}
	}
	return false
}

func findHashIndex(slice []uint64, target uint64) int {
	for i, h := range slice {
		if h == target {
			return i
		}
	}
	return -1
}

// IsToolError checks whether a tool output represents an execution error or failure.
// Tool errors are immune to stale eviction and must always remain visible to the agent.
func IsToolError(content string, isError bool) bool {
	return agentregistry.DefaultRegistry().IsToolError(content, isError)
}

func isFileReadTool(name string) bool {
	return agentregistry.DefaultRegistry().IsFileReadTool(name)
}

func isFileWriteTool(name string) bool {
	return agentregistry.DefaultRegistry().IsWriteTool(name)
}

func formatPathKey(basePath string, startLine, endLine int) string {
	if startLine > 0 || endLine > 0 {
		return basePath + "#L" + strconv.Itoa(startLine) + "-" + strconv.Itoa(endLine)
	}
	return basePath
}

// extractFilePaths extracts the normalized basePath and pathKey (with optional range) from tool arguments.
func extractFilePaths(args interface{}) (string, string) {
	switch v := args.(type) {
	case string:
		return extractPathsFromString(v)
	case map[string]interface{}:
		return extractPathsFromMap(v)
	default:
		return "", ""
	}
}

func extractPathsFromMap(m map[string]interface{}) (string, string) {
	var rawPath string
	for _, k := range rawPathCandidates {
		if val, ok := m[k].(string); ok && val != "" {
			rawPath = val
			break
		}
	}

	if rawPath == "" {
		return "", ""
	}

	basePath := normalizePath(rawPath)

	// Extract optional line range
	startLine := getIntFromMap(m, mapStartLineKeys...)
	endLine := getIntFromMap(m, mapEndLineKeys...)
	if endLine == 0 {
		limit := getIntFromMap(m, mapLimitKeys...)
		if limit > 0 && startLine > 0 {
			endLine = startLine + limit - 1
		}
	}

	return basePath, formatPathKey(basePath, startLine, endLine)
}

func getIntFromMap(m map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			case string:
				if parsed, err := strconv.Atoi(n); err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func extractPathsFromString(s string) (string, string) {
	if len(s) == 0 {
		return "", ""
	}

	var rawPath string
	for _, key := range jsonQuotedPathCandidates {
		idx := strings.Index(s, key)
		if idx == -1 {
			continue
		}
		afterKey := s[idx+len(key):]
		// Find opening quote
		quoteStart := strings.IndexByte(afterKey, '"')
		if quoteStart == -1 {
			continue
		}
		valStart := quoteStart + 1
		quoteEnd := strings.IndexByte(afterKey[valStart:], '"')
		if quoteEnd == -1 {
			continue
		}
		rawPath = afterKey[valStart : valStart+quoteEnd]
		break
	}

	if rawPath == "" {
		return "", ""
	}

	basePath := normalizePath(rawPath)

	// Quick check for startLine and endLine
	startLine := extractIntField(s, jsonQuotedStartLineKeys...)
	endLine := extractIntField(s, jsonQuotedEndLineKeys...)
	if endLine == 0 {
		limit := extractIntField(s, jsonQuotedLimitKeys...)
		if limit > 0 && startLine > 0 {
			endLine = startLine + limit - 1
		}
	}

	return basePath, formatPathKey(basePath, startLine, endLine)
}

func extractIntField(s string, keys ...string) int {
	for _, key := range keys {
		idx := strings.Index(s, key)
		if idx == -1 {
			continue
		}
		afterKey := strings.TrimSpace(s[idx+len(key):])
		endIdx := 0
		for endIdx < len(afterKey) && (afterKey[endIdx] >= '0' && afterKey[endIdx] <= '9') {
			endIdx++
		}
		if endIdx > 0 {
			if n, err := strconv.Atoi(afterKey[:endIdx]); err == nil {
				return n
			}
		}
	}
	return 0
}

func normalizePath(p string) string {
	cleaned := strings.TrimSpace(p)
	cleaned = strings.ReplaceAll(cleaned, `\`, `/`)
	cleaned = strings.TrimPrefix(cleaned, "./")
	if len(cleaned) >= 2 && (cleaned[0] >= 'A' && cleaned[0] <= 'Z') && cleaned[1] == ':' {
		cleaned = string(cleaned[0]+('a'-'A')) + cleaned[1:]
	}
	return cleaned
}
