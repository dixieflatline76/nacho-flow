package router

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

type RequestClassifier struct {
	estimator *TokenEstimator
}

// NewClassifier initializes a default RequestClassifier with an adaptive TokenEstimator.
func NewClassifier() contract.Classifier {
	return NewClassifierWithEstimator(NewTokenEstimator())
}

// NewClassifierWithEstimator initializes a RequestClassifier with a specific TokenEstimator.
func NewClassifierWithEstimator(e *TokenEstimator) contract.Classifier {
	if e == nil {
		e = NewTokenEstimator()
	}
	return &RequestClassifier{
		estimator: e,
	}
}

// GetEstimator returns the active TokenEstimator instance for dynamic calibration.
func (c *RequestClassifier) GetEstimator() *TokenEstimator {
	if c.estimator == nil {
		c.estimator = NewTokenEstimator()
	}
	return c.estimator
}

// Classify extracts token count, tools presence, image presence, and prompt keywords from request JSON.
func (c *RequestClassifier) Classify(body []byte) (contract.RequestContext, error) {
	reqCtx := contract.RequestContext{}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return reqCtx, err
	}

	// 1. Check tools and extract supported interactive tool for agent fallback shield
	if tools, ok := raw["tools"].([]interface{}); ok && len(tools) > 0 {
		reqCtx.HasTools = true
		reqCtx.InteractiveTool = ExtractSupportedInteractiveTool(tools)
	}

	// 2. Parse messages to count tokens, detect images, and extract keywords
	messages, ok := raw["messages"].([]interface{})
	if !ok {
		return reqCtx, nil
	}

	var fullText strings.Builder
	var latestUserPrompt string

	for _, m := range messages {
		msgMap, ok := m.(map[string]interface{})
		if !ok {
			continue
		}

		role, _ := msgMap["role"].(string)
		content, hasContent := msgMap["content"]
		if !hasContent {
			continue
		}

		// Handle text string content
		if strContent, ok := content.(string); ok {
			fullText.WriteString(strContent)
			fullText.WriteString(" ")
			if role == "user" {
				latestUserPrompt = strContent
			}
			continue
		}

		// Handle array content (multimodal / multi-part)
		if parts, ok := content.([]interface{}); ok {
			for _, part := range parts {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					continue
				}

				partType, _ := partMap["type"].(string)
				switch partType {
				case "image_url":
					reqCtx.HasImages = true
				case "text":
					textStr, _ := partMap["text"].(string)
					fullText.WriteString(textStr)
					fullText.WriteString(" ")
					if role == "user" {
						latestUserPrompt = textStr
					}
				}
			}
		}
	}

	// 2.5 Scan trailing messages for error patterns and tool progress
	reqCtx.HistoryErrors, reqCtx.HasToolProgress = scanTrailingMessages(messages)

	// 3. Approximate token count using adaptive TokenEstimator
	allText := fullText.String()
	estimator := c.GetEstimator()
	reqCtx.Tokens = estimator.Estimate(len(allText))
	reqCtx.Prompt = latestUserPrompt
	reqCtx.CleanPrompt = latestUserPrompt

	// 4. Parse @nacho: in-prompt directives if present
	if HasDirective(latestUserPrompt) {
		info, cleanPrompt := ExtractDirective(latestUserPrompt)
		reqCtx.CleanPrompt = cleanPrompt
		reqCtx.ForcedTier = info.ForcedTier
		reqCtx.ForcedModel = info.ForcedModel
		reqCtx.IsMetaDirective = info.IsMeta
		reqCtx.MetaDirective = info.Directive
		reqCtx.MetaDirectiveRaw = info.Raw

		flags, _ := ScanDirectives(latestUserPrompt)
		reqCtx.Features = uint16(flags)
	} else {
		reqCtx.Features = uint16(FeatureDefaultAll)
	}

	// 5. Extract clean lowercased keywords strictly from latest user prompt (fallback to allText if no user prompt)
	keywordSource := reqCtx.CleanPrompt
	if keywordSource == "" {
		keywordSource = allText
	}
	reqCtx.Keywords = extractKeywords(keywordSource)

	return reqCtx, nil
}

// agentErrorSignatures are known error patterns injected by agent clients
// (Zoo Code, Cline, Roo Code) into conversation history when a tool call fails.
var agentErrorSignatures = []string{
	"[ERROR] You did not use a tool",
	"Missing value for required parameter",
	"The tool execution failed",
	"<error_details>",
	"No sufficiently similar match found",
	"Command failed with exit code",
	"Please retry with complete response",
}

// scanTrailingMessages inspects the last N messages in the conversation history
// to detect: (a) consecutive trailing error turns, and (b) successful tool progress.
//
// Returns:
//   - historyErrors: count of consecutive trailing error messages (from the end)
//   - hasToolProgress: true if any recent message contains successful tool_result content
func scanTrailingMessages(messages []interface{}) (historyErrors int, hasToolProgress bool) {
	// Scan backwards from the end, up to 6 messages
	start := len(messages) - 6
	if start < 0 {
		start = 0
	}

	// First pass: detect tool progress (any successful tool result in trailing messages)
	for i := start; i < len(messages); i++ {
		msgMap, ok := messages[i].(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)

		// Successful tool results indicate forward progress
		if role == "tool" {
			hasToolProgress = true
		}

		// Check for multi-part content with tool_result type (Anthropic format)
		if parts, ok := msgMap["content"].([]interface{}); ok {
			for _, part := range parts {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					continue
				}
				partType, _ := partMap["type"].(string)
				if partType == "tool_result" {
					// Check if this tool result is an error
					isError, _ := partMap["is_error"].(bool)
					if !isError {
						hasToolProgress = true
					}
				}
			}
		}
	}

	// Second pass: count consecutive trailing errors (from the end, backwards)
	for i := len(messages) - 1; i >= start; i-- {
		msgMap, ok := messages[i].(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)

		// Only user-role messages carry error feedback from agent clients
		if role != "user" {
			continue
		}

		text := extractAllTextFromContent(msgMap["content"])
		foundError := false
		for _, sig := range agentErrorSignatures {
			if strings.Contains(text, sig) {
				foundError = true
				break
			}
		}

		if foundError {
			historyErrors++
		} else {
			// Break consecutive error chain on a non-error user message
			break
		}
	}

	return historyErrors, hasToolProgress
}

// extractAllTextFromContent extracts all text from a content field,
// handling both plain string and multi-part array formats.
func extractAllTextFromContent(content interface{}) string {
	if s, ok := content.(string); ok {
		return s
	}
	if parts, ok := content.([]interface{}); ok {
		var sb strings.Builder
		for _, part := range parts {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
			if contentStr, ok := partMap["content"].(string); ok {
				sb.WriteString(contentStr)
				sb.WriteString(" ")
			}
		}
		return sb.String()
	}
	return ""
}

func extractKeywords(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	seen := make(map[string]bool)
	keywords := make([]string, 0, len(words))

	for _, w := range words {
		if len(w) > 2 && !seen[w] {
			seen[w] = true
			keywords = append(keywords, w)
		}
	}

	return keywords
}

// ExtractSupportedInteractiveTool inspects tools to find supported conversational tool schemas.
func ExtractSupportedInteractiveTool(tools []interface{}) string {
	for _, t := range tools {
		tMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		var name string
		if fnMap, ok := tMap["function"].(map[string]interface{}); ok {
			name, _ = fnMap["name"].(string)
		} else if n, ok := tMap["name"].(string); ok {
			name = n
		}

		lower := strings.ToLower(name)
		if lower == "ask_followup_question" || lower == "ask_question" || lower == "user_prompt" || lower == "interactive_input" {
			return name
		}
	}
	return ""
}
