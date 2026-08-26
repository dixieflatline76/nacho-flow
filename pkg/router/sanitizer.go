package router

import (
	"encoding/json"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

type HistorySanitizer struct{}

func NewSanitizer() contract.Sanitizer {
	return &HistorySanitizer{}
}

// SanitizePayload strips image_url objects from message history if the target model lacks vision capabilities,
// and strips @nacho: directive tags from message contents.
func (s *HistorySanitizer) SanitizePayload(body []byte, targetHasVision bool) ([]byte, bool) {
	if targetHasVision && !HasDirective(string(body)) {
		return body, false
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return body, false
	}

	messages, ok := raw["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		return body, false
	}

	modified := false

	for i, m := range messages {
		msgMap, ok := m.(map[string]interface{})
		if !ok {
			continue
		}

		content, hasContent := msgMap["content"]
		if !hasContent {
			continue
		}

		// Handle string content
		if strContent, isStr := content.(string); isStr {
			if HasDirective(strContent) {
				msgMap["content"] = StripDirective(strContent)
				messages[i] = msgMap
				modified = true
			}
			continue
		}

		// Content can be an array of parts
		parts, isArray := content.([]interface{})
		if isArray {
			newParts := make([]interface{}, 0, len(parts))
			for _, part := range parts {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					newParts = append(newParts, part)
					continue
				}

				partType, _ := partMap["type"].(string)
				if partType == "image_url" && !targetHasVision {
					// Strip image payload to prevent 400 Bad Request on text models
					modified = true
					continue
				} else if partType == "text" {
					if textStr, hasText := partMap["text"].(string); hasText && HasDirective(textStr) {
						partMap["text"] = StripDirective(textStr)
						modified = true
					}
				}
				newParts = append(newParts, part)
			}

			// If only text parts remain, flatten or keep array
			if len(newParts) == 0 {
				msgMap["content"] = ""
			} else {
				msgMap["content"] = newParts
			}
			messages[i] = msgMap
		}
	}

	if !modified {
		return body, false
	}

	sanitized, err := json.Marshal(raw)
	if err != nil {
		return body, false
	}

	return sanitized, true
}
