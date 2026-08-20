package router

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// RawToolCall represents an extracted tool call structure from markdown or XML blocks.
type RawToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

var (
	// Matches ```json ... ``` code blocks
	jsonBlockRegex = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\}|\\[.*?\\])\\s*```")

	// Matches <tool_call> ... </tool_call> tags
	xmlToolCallRegex = regexp.MustCompile("(?s)<tool_call>\\s*(.*?)\\s*</tool_call>")
)

// NormalizeMarkdownToolCalls extracts markdown/XML code fences from text and converts them to OpenAI tool_calls.
// Returns the remaining cleaned message text, the extracted tool calls array, and true if any tool calls were parsed.
func NormalizeMarkdownToolCalls(content string) (string, []RawToolCall, bool) {
	if strings.TrimSpace(content) == "" {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content

	// 1. Check XML <tool_call> blocks
	xmlMatches := xmlToolCallRegex.FindAllStringSubmatch(content, -1)
	for idx, m := range xmlMatches {
		if len(m) > 1 {
			rawJSON := strings.TrimSpace(m[1])
			if tc, ok := parseSingleToolCall(rawJSON, idx+1); ok {
				extracted = append(extracted, tc)
				remainingContent = strings.Replace(remainingContent, m[0], "", 1)
			}
		}
	}

	// 2. Check markdown ```json code blocks if no XML matches found
	if len(extracted) == 0 {
		codeMatches := jsonBlockRegex.FindAllStringSubmatch(content, -1)
		for idx, m := range codeMatches {
			if len(m) > 1 {
				rawJSON := strings.TrimSpace(m[1])
				// Could be single object or array of tool calls
				if tcs, ok := parseToolCallsOrArray(rawJSON, idx+1); ok {
					extracted = append(extracted, tcs...)
					remainingContent = strings.Replace(remainingContent, m[0], "", 1)
				}
			}
		}
	}

	if len(extracted) == 0 {
		return content, nil, false
	}

	return strings.TrimSpace(remainingContent), extracted, true
}

func parseToolCallsOrArray(rawJSON string, baseIndex int) ([]RawToolCall, bool) {
	// Try parsing as array first: [{"name": ...}, ...]
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

	// Try parsing as single object
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
		return str
	}
	bytes, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
