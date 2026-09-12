package nts

import (
	"encoding/json"
	"strings"
)

// Transformer inspects incoming HTTP request JSON payloads (Anthropic / OpenAI)
// and applies NTS compaction to tool responses with full immunity guards.
type Transformer struct {
	pipeline *Pipeline
}

// NewTransformer creates a new payload Transformer backed by the given NTS config.
func NewTransformer(cfg Config) *Transformer {
	return &Transformer{
		pipeline: NewPipeline(cfg),
	}
}

// Pipeline returns the underlying compaction pipeline.
func (t *Transformer) Pipeline() *Pipeline {
	return t.pipeline
}

// TransformAnthropic parses an Anthropic Messages API payload, compacts any
// tool_result content blocks in-place, and re-encodes the JSON.
func (t *Transformer) TransformAnthropic(body []byte) ([]byte, ReductionResult, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return body, ReductionResult{Bypassed: true, BypassReason: "unmarshal_error"}, err
	}

	msgsRaw, ok := req["messages"]
	if !ok {
		return body, ReductionResult{Bypassed: true, BypassReason: "no_messages"}, nil
	}

	msgs, ok := msgsRaw.([]interface{})
	if !ok {
		return body, ReductionResult{Bypassed: true, BypassReason: "invalid_messages"}, nil
	}

	// Index tool_use ids to tool names for resolution
	toolUseNames := make(map[string]string)
	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}
		contentList, ok := msg["content"].([]interface{})
		if !ok {
			continue
		}
		for _, partItem := range contentList {
			part, ok := partItem.(map[string]interface{})
			if !ok {
				continue
			}
			if part["type"] == "tool_use" {
				id, _ := part["id"].(string)
				name, _ := part["name"].(string)
				if id != "" && name != "" {
					toolUseNames[id] = name
				}
			}
		}
	}

	var totalResult ReductionResult
	modified := false

	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}

		contentRaw, ok := msg["content"]
		if !ok {
			continue
		}

		contentList, ok := contentRaw.([]interface{})
		if !ok {
			continue
		}

		for _, partItem := range contentList {
			part, ok := partItem.(map[string]interface{})
			if !ok {
				continue
			}

			if part["type"] != "tool_result" {
				continue
			}

			// Check prompt caching immunity
			if t.pipeline.config.PreserveCacheControl {
				if _, hasCache := part["cache_control"]; hasCache {
					continue
				}
			}

			// Determine tool category for immunity
			toolName, _ := part["tool_name"].(string)
			if toolName == "" {
				toolUseID, _ := part["tool_use_id"].(string)
				toolName = toolUseNames[toolUseID]
			}
			category := categorizeToolName(toolName)

			textVal, isStr := part["content"].(string)
			if isStr && len(textVal) > 0 {
				buf := []byte(textVal)
				res := t.pipeline.Process(buf, category)
				if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
					part["content"] = string(buf[:res.ReducedBytes])
					totalResult.OriginalBytes += res.OriginalBytes
					totalResult.ReducedBytes += res.ReducedBytes
					totalResult.BytesSaved += res.BytesSaved
					totalResult.TokensSaved += res.TokensSaved
					totalResult.Duration += res.Duration
					modified = true
				}
			}
		}
	}

	if !modified {
		return body, totalResult, nil
	}

	newBody, err := json.Marshal(req)
	if err != nil {
		return body, totalResult, err
	}

	return newBody, totalResult, nil
}

// TransformOpenAI parses an OpenAI Chat Completions payload, compacts any
// role: "tool" messages in-place, and re-encodes the JSON.
func (t *Transformer) TransformOpenAI(body []byte) ([]byte, ReductionResult, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return body, ReductionResult{Bypassed: true, BypassReason: "unmarshal_error"}, err
	}

	msgsRaw, ok := req["messages"]
	if !ok {
		return body, ReductionResult{Bypassed: true, BypassReason: "no_messages"}, nil
	}

	msgs, ok := msgsRaw.([]interface{})
	if !ok {
		return body, ReductionResult{Bypassed: true, BypassReason: "invalid_messages"}, nil
	}

	// Index tool_call ids to tool names from assistant messages
	toolCallNames := make(map[string]string)
	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}
		toolCalls, ok := msg["tool_calls"].([]interface{})
		if !ok {
			continue
		}
		for _, tcItem := range toolCalls {
			tc, ok := tcItem.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := tc["id"].(string)
			fn, ok := tc["function"].(map[string]interface{})
			if ok {
				name, _ := fn["name"].(string)
				if id != "" && name != "" {
					toolCallNames[id] = name
				}
			}
		}
	}

	var totalResult ReductionResult
	modified := false

	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}

		role, _ := msg["role"].(string)
		if role != "tool" {
			continue
		}

		toolName, _ := msg["name"].(string)
		if toolName == "" {
			toolCallID, _ := msg["tool_call_id"].(string)
			toolName = toolCallNames[toolCallID]
		}
		category := categorizeToolName(toolName)

		textVal, isStr := msg["content"].(string)
		if isStr && len(textVal) > 0 {
			buf := []byte(textVal)
			res := t.pipeline.Process(buf, category)
			if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
				msg["content"] = string(buf[:res.ReducedBytes])
				totalResult.OriginalBytes += res.OriginalBytes
				totalResult.ReducedBytes += res.ReducedBytes
				totalResult.BytesSaved += res.BytesSaved
				totalResult.TokensSaved += res.TokensSaved
				totalResult.Duration += res.Duration
				modified = true
			}
		}
	}

	if !modified {
		return body, totalResult, nil
	}

	newBody, err := json.Marshal(req)
	if err != nil {
		return body, totalResult, err
	}

	return newBody, totalResult, nil
}

func categorizeToolName(name string) ToolCategory {
	nameLower := strings.ToLower(name)
	if strings.Contains(nameLower, "read") || strings.Contains(nameLower, "view") || nameLower == "cat" {
		return CategoryFileRead
	}
	if strings.Contains(nameLower, "write") || strings.Contains(nameLower, "replace") || strings.Contains(nameLower, "patch") {
		return CategoryFileWrite
	}
	return CategoryGeneric
}
