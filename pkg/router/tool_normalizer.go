package router

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// RawToolCall represents an extracted tool call structure from markdown, XML, or control tokens.
type RawToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

var (
	// Tag patterns
	xmlToolCallOpenRegex    = regexp.MustCompile(`(?i)<tool_call>`)
	mistralToolCallRegex    = regexp.MustCompile(`(?i)\[TOOL_CALLS\]`)
	llamaFunctionTagRegex   = regexp.MustCompile(`(?s)<function=([a-zA-Z0-9_.-]+)>\s*`)
	llamaPythonTagRegex     = regexp.MustCompile(`(?s)<\|python_tag\|>\s*([a-zA-Z0-9_.-]+?)(?:\.call)?\((.*?)\)\s*(?:<\|eom_id\|>|<\|eot_id\|>)?`)
	claudeInvokeRegex       = regexp.MustCompile(`(?s)<invoke\s+name=["']([^"']+)["']>\s*(.*?)\s*</invoke>`)
	claudeParamRegex        = regexp.MustCompile(`(?s)<parameter\s+name=["']([^"']+)["']>\s*(.*?)\s*</parameter>`)
	reActActionRegex        = regexp.MustCompile(`(?i)Action:\s*([a-zA-Z0-9_.-]+)\s*\n\s*Action Input:\s*`)
	markdownFenceStartRegex = regexp.MustCompile("(?s)`{3,4}(?:json|tool|tool_code)?\\s*")
)

// NormalizeMarkdownToolCalls extracts tool calls across 7 open-source format families and converts them to OpenAI tool_calls.
// Returns the remaining cleaned message text, the extracted tool calls array, and true if any tool calls were parsed.
func NormalizeMarkdownToolCalls(content string) (string, []RawToolCall, bool) {
	if strings.TrimSpace(content) == "" {
		return content, nil, false
	}

	// Fast pre-filter: if content has no candidate tokens, bail out in 1 CPU cycle without regex/parsing
	if !strings.Contains(content, "<") && !strings.Contains(content, "[") && !strings.Contains(content, "`") && !strings.Contains(content, "Action:") {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content

	// --- Parser 1: Hermes / Qwen <tool_call> tags ---
	for {
		loc := xmlToolCallOpenRegex.FindStringIndex(remainingContent)
		if loc == nil {
			break
		}
		closeTag := "</tool_call>"
		closeIdx := strings.Index(remainingContent[loc[1]:], closeTag)
		if closeIdx == -1 {
			break
		}
		inner := strings.TrimSpace(remainingContent[loc[1] : loc[1]+closeIdx])
		fullEnd := loc[1] + closeIdx + len(closeTag)

		if tc, ok := parseSingleToolCall(inner, len(extracted)+1); ok {
			extracted = append(extracted, tc)
			remainingContent = remainingContent[:loc[0]] + remainingContent[fullEnd:]
		} else {
			break
		}
	}

	// --- Parser 2: Mistral [TOOL_CALLS] control tokens ---
	if len(extracted) == 0 {
		for {
			loc := mistralToolCallRegex.FindStringIndex(remainingContent)
			if loc == nil {
				break
			}
			rawJSON, _, endPos, ok := extractBalancedJSON(remainingContent, loc[1])
			if ok {
				if tcs, parsed := parseToolCallsOrArray(rawJSON, len(extracted)+1); parsed {
					extracted = append(extracted, tcs...)
					remainingContent = remainingContent[:loc[0]] + remainingContent[endPos:]
					continue
				}
			}
			break
		}
	}

	// --- Parser 3: Llama 3 <function=name>{...}</function> ---
	if len(extracted) == 0 {
		for {
			m := llamaFunctionTagRegex.FindStringSubmatchIndex(remainingContent)
			if m == nil {
				break
			}
			fnName := remainingContent[m[2]:m[3]]
			rawJSON, _, endPos, ok := extractBalancedJSON(remainingContent, m[1])
			closeTag := "</function>"
			if ok && strings.HasPrefix(remainingContent[endPos:], closeTag) {
				tc := RawToolCall{
					ID:   fmt.Sprintf("call_llama_%d", len(extracted)+1),
					Type: "function",
				}
				tc.Function.Name = fnName
				tc.Function.Arguments = rawJSON
				extracted = append(extracted, tc)
				remainingContent = remainingContent[:m[0]] + remainingContent[endPos+len(closeTag):]
				continue
			}
			break
		}
	}

	// --- Parser 4: Llama 3 <|python_tag|>name(args) ---
	if len(extracted) == 0 {
		pyMatches := llamaPythonTagRegex.FindAllStringSubmatch(remainingContent, -1)
		for idx, m := range pyMatches {
			if len(m) > 2 {
				fnName := strings.TrimSpace(m[1])
				argsRaw := strings.TrimSpace(m[2])
				argsJSON := pythonArgsToJSON(argsRaw)
				tc := RawToolCall{
					ID:   fmt.Sprintf("call_py_%d", idx+1),
					Type: "function",
				}
				tc.Function.Name = fnName
				tc.Function.Arguments = argsJSON
				extracted = append(extracted, tc)
				remainingContent = strings.Replace(remainingContent, m[0], "", 1)
			}
		}
	}

	// --- Parser 5: Claude XML <invoke name="..."> ... </invoke> ---
	if len(extracted) == 0 {
		invokeMatches := claudeInvokeRegex.FindAllStringSubmatch(remainingContent, -1)
		for idx, m := range invokeMatches {
			if len(m) > 2 {
				fnName := strings.TrimSpace(m[1])
				paramsBody := m[2]
				argsMap := make(map[string]interface{})
				paramMatches := claudeParamRegex.FindAllStringSubmatch(paramsBody, -1)
				for _, pm := range paramMatches {
					if len(pm) > 2 {
						pName := strings.TrimSpace(pm[1])
						pVal := strings.TrimSpace(pm[2])
						argsMap[pName] = pVal
					}
				}
				argsBytes, _ := json.Marshal(argsMap)
				tc := RawToolCall{
					ID:   fmt.Sprintf("call_claude_%d", idx+1),
					Type: "function",
				}
				tc.Function.Name = fnName
				tc.Function.Arguments = string(argsBytes)
				extracted = append(extracted, tc)
				remainingContent = strings.Replace(remainingContent, m[0], "", 1)
			}
		}
		if len(extracted) > 0 {
			remainingContent = regexp.MustCompile(`(?s)</?function_calls>`).ReplaceAllString(remainingContent, "")
		}
	}

	// --- Parser 6: ReAct Action: name\nAction Input: {...} ---
	if len(extracted) == 0 {
		for {
			m := reActActionRegex.FindStringSubmatchIndex(remainingContent)
			if m == nil {
				break
			}
			fnName := strings.TrimSpace(remainingContent[m[2]:m[3]])
			rawJSON, _, endPos, ok := extractBalancedJSON(remainingContent, m[1])
			if ok {
				tc := RawToolCall{
					ID:   fmt.Sprintf("call_react_%d", len(extracted)+1),
					Type: "function",
				}
				tc.Function.Name = fnName
				tc.Function.Arguments = rawJSON
				extracted = append(extracted, tc)
				remainingContent = remainingContent[:m[0]] + remainingContent[endPos:]
				continue
			} else {
				// Single line string input
				lineEnd := strings.Index(remainingContent[m[1]:], "\n")
				var rawInput string
				var endIdx int
				if lineEnd == -1 {
					rawInput = strings.TrimSpace(remainingContent[m[1]:])
					endIdx = len(remainingContent)
				} else {
					rawInput = strings.TrimSpace(remainingContent[m[1] : m[1]+lineEnd])
					endIdx = m[1] + lineEnd
				}
				argObj := map[string]string{"input": rawInput}
				bytes, _ := json.Marshal(argObj)
				tc := RawToolCall{
					ID:   fmt.Sprintf("call_react_%d", len(extracted)+1),
					Type: "function",
				}
				tc.Function.Name = fnName
				tc.Function.Arguments = string(bytes)
				extracted = append(extracted, tc)
				remainingContent = remainingContent[:m[0]] + remainingContent[endIdx:]
				continue
			}
		}
	}

	// --- Parser 7: Markdown ```json code fences with balanced JSON ---
	if len(extracted) == 0 {
		for {
			m := markdownFenceStartRegex.FindStringIndex(remainingContent)
			if m == nil {
				break
			}
			rawJSON, _, jsonEnd, ok := extractBalancedJSON(remainingContent, m[1])
			if ok {
				// Find trailing backticks
				rest := remainingContent[jsonEnd:]
				trimmedRest := strings.TrimLeft(rest, " \t\r\n")
				if strings.HasPrefix(trimmedRest, "```") {
					closingFenceLen := 3
					if strings.HasPrefix(trimmedRest, "````") {
						closingFenceLen = 4
					}
					fenceEnd := jsonEnd + (len(rest) - len(trimmedRest)) + closingFenceLen

					if tcs, parsed := parseToolCallsOrArray(rawJSON, len(extracted)+1); parsed {
						extracted = append(extracted, tcs...)
						remainingContent = remainingContent[:m[0]] + remainingContent[fenceEnd:]
						continue
					}
				}
			}
			// Skip this fence if not tool calls
			remainingContent = remainingContent[:m[0]] + remainingContent[m[1]:]
			break
		}
	}

	if len(extracted) == 0 {
		return content, nil, false
	}

	return strings.TrimSpace(remainingContent), extracted, true
}

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

func parseSingleToolCall(rawJSON string, index int) (RawToolCall, bool) {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &obj); err != nil {
		return RawToolCall{}, false
	}
	return parseMapToToolCall(obj, index)
}

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
	kvRegex := regexp.MustCompile(`([a-zA-Z0-9_]+)\s*=\s*(".*?"|'.*?'|[0-9.]+|true|false|null)`)
	matches := kvRegex.FindAllStringSubmatch(rawArgs, -1)
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
