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

	// 1. Check tools
	if tools, ok := raw["tools"].([]interface{}); ok && len(tools) > 0 {
		reqCtx.HasTools = true
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

	// 3. Approximate token count using adaptive TokenEstimator
	allText := fullText.String()
	estimator := c.GetEstimator()
	reqCtx.Tokens = estimator.Estimate(len(allText))
	reqCtx.Prompt = latestUserPrompt

	// 4. Extract clean lowercased keywords strictly from latest user prompt (fallback to allText if no user prompt)
	keywordSource := latestUserPrompt
	if keywordSource == "" {
		keywordSource = allText
	}
	reqCtx.Keywords = extractKeywords(keywordSource)

	return reqCtx, nil
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
