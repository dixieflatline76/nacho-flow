package router

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	pyKVRegex = regexp.MustCompile(`([a-zA-Z0-9_]+)\s*=\s*(".*?"|'.*?'|[0-9.]+|true|false|null)`)
)

// extractBalancedJSON finds the balanced JSON object or array starting at or after startIdx in s.
func extractBalancedJSON(s string, startIdx int) (string, int, int, bool) {
	if startIdx >= len(s) {
		return "", 0, 0, false
	}

	// Find first '{' or '['
	openIdx := -1
	var openChar, closeChar byte
	for i := startIdx; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n' {
			continue
		}
		if s[i] == '{' {
			openIdx = i
			openChar, closeChar = '{', '}'
			break
		} else if s[i] == '[' {
			openIdx = i
			openChar, closeChar = '[', ']'
			break
		} else {
			return "", 0, 0, false
		}
	}
	if openIdx == -1 {
		return "", 0, 0, false
	}

	depth := 0
	inString := false
	escaped := false

	for i := openIdx; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if !inString {
			switch c {
			case openChar:
				depth++
			case closeChar:
				depth--
				if depth == 0 {
					return s[openIdx : i+1], openIdx, i + 1, true
				}
			}
		}
	}
	return "", 0, 0, false
}

// parseBareJSONToolCall validates and extracts tool calls from a bare JSON block.
func parseBareJSONToolCall(rawJSON string, index int) ([]RawToolCall, bool) {
	// For bare JSON in content, require that it explicitly contains "arguments" or "parameters" or nested "function"
	// to avoid false-matching regular prose JSON dictionaries
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &obj); err == nil {
		if _, hasArgs := obj["arguments"]; hasArgs {
			return parseToolCallsOrArray(rawJSON, index)
		}
		if _, hasParams := obj["parameters"]; hasParams {
			return parseToolCallsOrArray(rawJSON, index)
		}
		if _, hasFn := obj["function"]; hasFn {
			return parseToolCallsOrArray(rawJSON, index)
		}
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &arr); err == nil && len(arr) > 0 {
		first := arr[0]
		if _, hasArgs := first["arguments"]; hasArgs {
			return parseToolCallsOrArray(rawJSON, index)
		}
		if _, hasParams := first["parameters"]; hasParams {
			return parseToolCallsOrArray(rawJSON, index)
		}
		if _, hasFn := first["function"]; hasFn {
			return parseToolCallsOrArray(rawJSON, index)
		}
	}
	return nil, false
}

// parseToolCallsOrArray parses raw JSON into one or more RawToolCall structs.
func parseToolCallsOrArray(rawJSON string, baseIndex int) ([]RawToolCall, bool) {
	// 1. Try parsing as array first: [{"name": ...}, ...]
	var rawArray []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &rawArray); err == nil && len(rawArray) > 0 {
		var list []RawToolCall
		for i, obj := range rawArray {
			if tc, ok := parseMapToToolCall(obj, baseIndex+i); ok {
				list = append(list, tc)
			}
		}
		if len(list) > 0 {
			return list, true
		}
	}

	// 2. Try parsing as single object: {"name": ...}
	if tc, ok := parseSingleToolCall(rawJSON, baseIndex); ok {
		return []RawToolCall{tc}, true
	}

	return nil, false
}

// parseSingleToolCall unmarshals a single JSON object string into a RawToolCall.
func parseSingleToolCall(rawJSON string, index int) (RawToolCall, bool) {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &obj); err != nil {
		return RawToolCall{}, false
	}
	return parseMapToToolCall(obj, index)
}

// parseMapToToolCall extracts a RawToolCall from generic map representations.
func parseMapToToolCall(obj map[string]interface{}, index int) (RawToolCall, bool) {
	var name string
	var argsStr string

	// Format A: {"name": "func_name", "arguments": {...}} or {"name": "func_name", "parameters": {...}}
	if n, ok := obj["name"].(string); ok && n != "" {
		name = n
		if argsObj, hasArgs := obj["arguments"]; hasArgs {
			argsStr = serializeArgs(argsObj)
		} else if paramsObj, hasParams := obj["parameters"]; hasParams {
			argsStr = serializeArgs(paramsObj)
		} else {
			argsStr = "{}"
		}
	} else if fnObj, ok := obj["function"].(map[string]interface{}); ok {
		// Format B: {"function": {"name": "func_name", "arguments": ...}}
		if n, ok := fnObj["name"].(string); ok && n != "" {
			name = n
			if argsObj, hasArgs := fnObj["arguments"]; hasArgs {
				argsStr = serializeArgs(argsObj)
			} else {
				argsStr = "{}"
			}
		}
	} else if callObj, ok := obj["tool_call"].(map[string]interface{}); ok {
		// Format C: {"tool_call": {"name": ...}}
		return parseMapToToolCall(callObj, index)
	}

	if name == "" {
		return RawToolCall{}, false
	}

	tc := RawToolCall{
		ID:   fmt.Sprintf("call_norm_%d", index),
		Type: "function",
	}
	tc.Function.Name = name
	tc.Function.Arguments = argsStr
	return tc, true
}

func serializeArgs(args interface{}) string {
	if str, ok := args.(string); ok {
		if isValidJSON(str) {
			return str
		}
		// Wrap string in JSON string
		bytes, _ := json.Marshal(str)
		return string(bytes)
	}
	bytes, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func isValidJSON(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	var js interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}

// pythonArgsToJSON converts python keyword arguments like `query="test", limit=5` into valid JSON `{"query":"test","limit":5}`
func pythonArgsToJSON(rawArgs string) string {
	rawArgs = strings.TrimSpace(rawArgs)
	if rawArgs == "" {
		return "{}"
	}
	if isValidJSON(rawArgs) {
		return rawArgs
	}

	resultMap := make(map[string]interface{})
	matches := pyKVRegex.FindAllStringSubmatch(rawArgs, -1)
	for _, m := range matches {
		if len(m) > 2 {
			k := m[1]
			vStr := m[2]
			if strings.HasPrefix(vStr, "\"") && strings.HasSuffix(vStr, "\"") {
				resultMap[k] = strings.Trim(vStr, "\"")
			} else if strings.HasPrefix(vStr, "'") && strings.HasSuffix(vStr, "'") {
				resultMap[k] = strings.Trim(vStr, "'")
			} else if vStr == "true" {
				resultMap[k] = true
			} else if vStr == "false" {
				resultMap[k] = false
			} else if vStr == "null" {
				resultMap[k] = nil
			} else {
				resultMap[k] = vStr
			}
		}
	}

	if len(resultMap) == 0 {
		return fmt.Sprintf(`{"raw_args": %q}`, rawArgs)
	}

	bytes, err := json.Marshal(resultMap)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
