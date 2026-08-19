package router

import (
	"encoding/json"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

type HistorySanitizer struct{}

func NewSanitizer() contract.Sanitizer {
	return &HistorySanitizer{}
}

// SanitizePayload strips image_url objects from message history if the target model lacks vision capabilities.
func (s *HistorySanitizer) SanitizePayload(body []byte, targetHasVision bool) ([]byte, bool) {
	if targetHasVision {
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

		// Content can be a string or an array of parts
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
				if partType == "image_url" {
					// Strip image payload to prevent 400 Bad Request on text models
					modified = true
					continue
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
