package nts

import (
	"strconv"
	"strings"
)

// StaleFileReadNotice is the constant replacement string for superseded historical file reads.
const StaleFileReadNotice = "[NTS: File content superseded by later read/write in conversation history]"

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
// by later reads or writes of the same file.
func IdentifyStaleToolCallIDs(msgs []interface{}) map[string]struct{} {
	if len(msgs) == 0 {
		return nil
	}

	// 1. Collect all file read/write tool calls across assistant messages
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

	if len(calls) <= 1 {
		return nil
	}

	// 2. Scan backwards to identify superseded reads
	var seenFullFiles [64]uint64
	seenFullCount := 0

	var seenRangedFiles [64]uint64
	seenRangedCount := 0

	staleIDs := make(map[string]struct{})

	for i := len(calls) - 1; i >= 0; i-- {
		c := calls[i]
		baseHash := fnv64(c.basePath)
		keyHash := fnv64(c.pathKey)

		if c.isWrite {
			// A write modifies the underlying file, superseding all previous reads
			if !containsHash(seenFullFiles[:seenFullCount], baseHash) {
				if seenFullCount < len(seenFullFiles) {
					seenFullFiles[seenFullCount] = baseHash
					seenFullCount++
				}
			}
			continue
		}

		if c.isRead {
			// Check if this read was superseded by:
			// 1. A later write or full read of the same file
			isSupersededByFull := containsHash(seenFullFiles[:seenFullCount], baseHash)
			// 2. A later read of the exact same line range
			isSupersededByRange := containsHash(seenRangedFiles[:seenRangedCount], keyHash)

			if isSupersededByFull || isSupersededByRange {
				staleIDs[c.id] = struct{}{}
			} else {
				// This is the active/latest read of this file or range
				if c.basePath == c.pathKey {
					// Full read (no line range specified)
					if seenFullCount < len(seenFullFiles) {
						seenFullFiles[seenFullCount] = baseHash
						seenFullCount++
					}
				} else {
					// Specific line range
					if seenRangedCount < len(seenRangedFiles) {
						seenRangedFiles[seenRangedCount] = keyHash
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

func isFileReadTool(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "read") || strings.Contains(n, "view") || n == "cat" || strings.Contains(n, "open")
}

func isFileWriteTool(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "write") || strings.Contains(n, "edit") ||
		strings.Contains(n, "replace") || strings.Contains(n, "patch") ||
		strings.Contains(n, "diff") || strings.Contains(n, "insert") ||
		strings.Contains(n, "create")
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
	candidates := []string{
		"path", "filePath", "file_path", "target_file", "TargetFile",
		"file", "filename", "file_name", "uri", "AbsolutePath",
	}

	for _, k := range candidates {
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
	startLine := getIntFromMap(m, "startLine", "start_line", "StartLine")
	endLine := getIntFromMap(m, "endLine", "end_line", "EndLine")

	if startLine > 0 || endLine > 0 {
		pathKey := basePath + "#L" + strconv.Itoa(startLine) + "-" + strconv.Itoa(endLine)
		return basePath, pathKey
	}

	return basePath, basePath
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

	candidates := []string{
		`"path":`, `"filePath":`, `"file_path":`, `"target_file":`,
		`"TargetFile":`, `"file":`, `"filename":`, `"file_name":`,
		`"uri":`, `"AbsolutePath":`,
	}

	var rawPath string
	for _, key := range candidates {
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
	startLine := extractIntField(s, `"startLine":`, `"start_line":`, `"StartLine":`)
	endLine := extractIntField(s, `"endLine":`, `"end_line":`, `"EndLine":`)

	if startLine > 0 || endLine > 0 {
		pathKey := basePath + "#L" + strconv.Itoa(startLine) + "-" + strconv.Itoa(endLine)
		return basePath, pathKey
	}

	return basePath, basePath
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
