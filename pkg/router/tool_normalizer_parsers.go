package router

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	xmlToolCallOpenRegex    = regexp.MustCompile(`(?i)<tool_call>`)
	mistralToolCallRegex    = regexp.MustCompile(`(?i)\[TOOL_CALLS\]`)
	llamaFunctionTagRegex   = regexp.MustCompile(`(?s)<function=([a-zA-Z0-9_.-]+)>\s*`)
	llamaPythonTagRegex     = regexp.MustCompile(`(?s)<\|python_tag\|>\s*([a-zA-Z0-9_.-]+?)(?:\.call)?\((.*?)\)\s*(?:<\|eom_id\|>|<\|eot_id\|>)?`)
	claudeInvokeRegex       = regexp.MustCompile(`(?s)<invoke\s+name=["']([^"']+)["']>\s*(.*?)\s*</invoke>`)
	claudeParamRegex        = regexp.MustCompile(`(?s)<parameter\s+name=["']([^"']+)["']>\s*(.*?)\s*</parameter>`)
	reActActionRegex        = regexp.MustCompile(`(?i)Action:\s*([a-zA-Z0-9_.-]+)\s*\n\s*Action Input:\s*`)
	markdownFenceStartRegex = regexp.MustCompile("(?s)`{3,4}(?:json|tool|tool_code)?\\s*")
	functionCallsTagRegex   = regexp.MustCompile(`(?s)</?function_calls>`)
)

// ToolParser defines the strategy interface for an individual model tool format.
type ToolParser interface {
	Name() string
	Parse(content string, nextID int) (remainingContent string, calls []RawToolCall, matched bool)
}

// 1. HermesXMLParser parses Hermes / Nous / Qwen <tool_call>...</tool_call> tags.
type HermesXMLParser struct{}

func (p *HermesXMLParser) Name() string { return "Hermes/Qwen XML" }

func (p *HermesXMLParser) Parse(content string, nextID int) (string, []RawToolCall, bool) {
	if !strings.Contains(content, "<tool_call>") && !strings.Contains(content, "<TOOL_CALL>") {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content

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

		if tc, ok := parseSingleToolCall(inner, nextID+len(extracted)); ok {
			extracted = append(extracted, tc)
			remainingContent = remainingContent[:loc[0]] + remainingContent[fullEnd:]
		} else {
			break
		}
	}

	if len(extracted) > 0 {
		return remainingContent, extracted, true
	}
	return content, nil, false
}

// 2. MistralTokenParser parses Mistral [TOOL_CALLS] control tokens.
type MistralTokenParser struct{}

func (p *MistralTokenParser) Name() string { return "Mistral Control Token" }

func (p *MistralTokenParser) Parse(content string, nextID int) (string, []RawToolCall, bool) {
	if !strings.Contains(content, "[TOOL_CALLS]") && !strings.Contains(content, "[tool_calls]") {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content

	for {
		loc := mistralToolCallRegex.FindStringIndex(remainingContent)
		if loc == nil {
			break
		}
		rawJSON, _, endPos, ok := extractBalancedJSON(remainingContent, loc[1])
		if ok {
			if tcs, parsed := parseToolCallsOrArray(rawJSON, nextID+len(extracted)); parsed {
				extracted = append(extracted, tcs...)
				remainingContent = remainingContent[:loc[0]] + remainingContent[endPos:]
				continue
			}
		}
		break
	}

	if len(extracted) > 0 {
		return remainingContent, extracted, true
	}
	return content, nil, false
}

// 3. LlamaTagParser parses Llama 3 <function=name>{...}</function> tags.
type LlamaTagParser struct{}

func (p *LlamaTagParser) Name() string { return "Llama 3 Function Tag" }

func (p *LlamaTagParser) Parse(content string, nextID int) (string, []RawToolCall, bool) {
	if !strings.Contains(content, "<function=") {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content

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
				ID:   fmt.Sprintf("call_llama_%d", nextID+len(extracted)),
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

	if len(extracted) > 0 {
		return remainingContent, extracted, true
	}
	return content, nil, false
}

// 4. LlamaPythonParser parses Llama 3 <|python_tag|>name(args) calls.
type LlamaPythonParser struct{}

func (p *LlamaPythonParser) Name() string { return "Llama 3 Python Tag" }

func (p *LlamaPythonParser) Parse(content string, nextID int) (string, []RawToolCall, bool) {
	if !strings.Contains(content, "<|python_tag|>") {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content
	pyMatches := llamaPythonTagRegex.FindAllStringSubmatch(remainingContent, -1)
	for idx, m := range pyMatches {
		if len(m) > 2 {
			fnName := strings.TrimSpace(m[1])
			argsRaw := strings.TrimSpace(m[2])
			argsJSON := pythonArgsToJSON(argsRaw)
			tc := RawToolCall{
				ID:   fmt.Sprintf("call_py_%d", nextID+idx),
				Type: "function",
			}
			tc.Function.Name = fnName
			tc.Function.Arguments = argsJSON
			extracted = append(extracted, tc)
			remainingContent = strings.Replace(remainingContent, m[0], "", 1)
		}
	}

	if len(extracted) > 0 {
		return remainingContent, extracted, true
	}
	return content, nil, false
}

// 5. ClaudeXMLParser parses Claude XML <invoke name="...">...<parameter name="...">...</invoke>.
type ClaudeXMLParser struct{}

func (p *ClaudeXMLParser) Name() string { return "Claude XML Invoke" }

func (p *ClaudeXMLParser) Parse(content string, nextID int) (string, []RawToolCall, bool) {
	if !strings.Contains(content, "<invoke") {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content
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
				ID:   fmt.Sprintf("call_claude_%d", nextID+idx),
				Type: "function",
			}
			tc.Function.Name = fnName
			tc.Function.Arguments = string(argsBytes)
			extracted = append(extracted, tc)
			remainingContent = strings.Replace(remainingContent, m[0], "", 1)
		}
	}

	if len(extracted) > 0 {
		remainingContent = functionCallsTagRegex.ReplaceAllString(remainingContent, "")
		return remainingContent, extracted, true
	}
	return content, nil, false
}

// 6. ReActParser parses ReAct format Action: name\nAction Input: {...}.
type ReActParser struct{}

func (p *ReActParser) Name() string { return "ReAct Action/Input" }

func (p *ReActParser) Parse(content string, nextID int) (string, []RawToolCall, bool) {
	if !strings.Contains(content, "Action:") && !strings.Contains(content, "action:") {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content

	for {
		m := reActActionRegex.FindStringSubmatchIndex(remainingContent)
		if m == nil {
			break
		}
		fnName := strings.TrimSpace(remainingContent[m[2]:m[3]])
		rawJSON, _, endPos, ok := extractBalancedJSON(remainingContent, m[1])
		if ok {
			tc := RawToolCall{
				ID:   fmt.Sprintf("call_react_%d", nextID+len(extracted)),
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
				ID:   fmt.Sprintf("call_react_%d", nextID+len(extracted)),
				Type: "function",
			}
			tc.Function.Name = fnName
			tc.Function.Arguments = string(bytes)
			extracted = append(extracted, tc)
			remainingContent = remainingContent[:m[0]] + remainingContent[endIdx:]
			continue
		}
	}

	if len(extracted) > 0 {
		return remainingContent, extracted, true
	}
	return content, nil, false
}

// 7. MarkdownFenceParser parses Markdown ```json code fences containing balanced tool call JSON.
type MarkdownFenceParser struct{}

func (p *MarkdownFenceParser) Name() string { return "Markdown Code Fence" }

func (p *MarkdownFenceParser) Parse(content string, nextID int) (string, []RawToolCall, bool) {
	if !strings.Contains(content, "```") {
		return content, nil, false
	}

	var extracted []RawToolCall
	var finalContent strings.Builder
	cursor := 0

	for cursor < len(content) {
		loc := markdownFenceStartRegex.FindStringIndex(content[cursor:])
		if loc == nil {
			finalContent.WriteString(content[cursor:])
			break
		}

		startFence := cursor + loc[0]
		afterFenceTag := cursor + loc[1]

		rawJSON, _, jsonEnd, ok := extractBalancedJSON(content, afterFenceTag)
		if ok {
			rest := content[jsonEnd:]
			trimmedRest := strings.TrimLeft(rest, " \t\r\n")
			closingFenceLen := 0
			if strings.HasPrefix(trimmedRest, "````") {
				closingFenceLen = 4
			} else if strings.HasPrefix(trimmedRest, "```") {
				closingFenceLen = 3
			}

			if closingFenceLen > 0 {
				fenceEnd := jsonEnd + (len(rest) - len(trimmedRest)) + closingFenceLen
				if tcs, parsed := parseToolCallsOrArray(rawJSON, nextID+len(extracted)); parsed {
					finalContent.WriteString(content[cursor:startFence])
					extracted = append(extracted, tcs...)
					cursor = fenceEnd
					continue
				}
			}
		}

		// Not a tool call fence: preserve up through opening fence tag and continue scanning
		finalContent.WriteString(content[cursor:afterFenceTag])
		cursor = afterFenceTag
	}

	if len(extracted) > 0 {
		return finalContent.String(), extracted, true
	}
	return content, nil, false
}

// 8. BareJSONParser parses direct JSON object or array tool calls (e.g. Qwen / Ollama direct output).
type BareJSONParser struct{}

func (p *BareJSONParser) Name() string { return "Bare JSON Object/Array" }

func (p *BareJSONParser) Parse(content string, nextID int) (string, []RawToolCall, bool) {
	if !strings.Contains(content, "{") && !strings.Contains(content, "[") {
		return content, nil, false
	}

	// If explicit tool tags are present, defer to specialized parsers
	if strings.Contains(content, "<tool_call>") || strings.Contains(content, "<function=") {
		return content, nil, false
	}

	var extracted []RawToolCall
	remainingContent := content

	for i := 0; i < len(remainingContent); i++ {
		if remainingContent[i] == '{' || remainingContent[i] == '[' {
			rawJSON, startPos, endPos, ok := extractBalancedJSON(remainingContent, i)
			if ok {
				if tcs, parsed := parseBareJSONToolCall(rawJSON, nextID+len(extracted)); parsed {
					extracted = append(extracted, tcs...)
					remainingContent = remainingContent[:startPos] + remainingContent[endPos:]
					i = startPos - 1
					continue
				}
			}
		}
	}

	if len(extracted) > 0 {
		return remainingContent, extracted, true
	}
	return content, nil, false
}
