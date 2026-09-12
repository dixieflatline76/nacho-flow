package nts

import (
	"bytes"
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
	return t.transformCommon(body)
}

// TransformOpenAI parses an OpenAI Chat Completions payload, compacts any
// role: "tool" messages or tool_result blocks in-place, and re-encodes the JSON.
func (t *Transformer) TransformOpenAI(body []byte) ([]byte, ReductionResult, error) {
	return t.transformCommon(body)
}

func (t *Transformer) transformCommon(body []byte) ([]byte, ReductionResult, error) {
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

	toolNames := indexToolNames(msgs)
	modified, totalResult := t.processMessages(msgs, toolNames)

	if !modified {
		return body, totalResult, nil
	}

	newBody, err := json.Marshal(req)
	if err != nil {
		return body, totalResult, err
	}

	return newBody, totalResult, nil
}

// indexToolNames extracts tool call IDs to tool names from both OpenAI tool_calls
// and Anthropic tool_use content blocks across all assistant messages.
func indexToolNames(msgs []interface{}) map[string]string {
	toolNames := make(map[string]string)
	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}

		// 1. OpenAI tool_calls
		if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
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
						toolNames[id] = name
					}
				}
			}
		}

		// 2. Anthropic content blocks: tool_use
		if contentList, ok := msg["content"].([]interface{}); ok {
			for _, partItem := range contentList {
				part, ok := partItem.(map[string]interface{})
				if !ok {
					continue
				}
				if part["type"] == "tool_use" {
					id, _ := part["id"].(string)
					name, _ := part["name"].(string)
					if id != "" && name != "" {
						toolNames[id] = name
					}
				}
			}
		}
	}
	return toolNames
}

// processMessages walks message structures and compacts tool output payloads.
func (t *Transformer) processMessages(msgs []interface{}, toolNames map[string]string) (bool, ReductionResult) {
	var totalResult ReductionResult
	modified := false

	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}

		// 1. OpenAI style: role == "tool"
		role, _ := msg["role"].(string)
		if role == "tool" {
			if t.pipeline.config.PreserveCacheControl {
				if _, hasCache := msg["cache_control"]; hasCache {
					continue
				}
			}

			toolName, _ := msg["name"].(string)
			if toolName == "" {
				toolCallID, _ := msg["tool_call_id"].(string)
				toolName = toolNames[toolCallID]
			}
			cat := categorizeToolName(toolName)

			// Content can be string or array of text parts
			if textVal, isStr := msg["content"].(string); isStr && len(textVal) > 0 {
				newText, res := t.processContentText(textVal, cat)
				if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
					msg["content"] = newText
					totalResult.OriginalBytes += res.OriginalBytes
					totalResult.ReducedBytes += res.ReducedBytes
					totalResult.BytesSaved += res.BytesSaved
					totalResult.TokensSaved += res.TokensSaved
					totalResult.Duration += res.Duration
					modified = true
				}
			} else if parts, isList := msg["content"].([]interface{}); isList {
				for _, pItem := range parts {
					part, ok := pItem.(map[string]interface{})
					if !ok {
						continue
					}
					if part["type"] == "text" {
						textVal, _ := part["text"].(string)
						if len(textVal) > 0 {
							newText, res := t.processContentText(textVal, cat)
							if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
								part["text"] = newText
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
			}
		}

		// 2. Anthropic style content blocks: type == "tool_result" (can be in user role)
		if contentRaw, ok := msg["content"]; ok {
			if contentList, ok := contentRaw.([]interface{}); ok {
				for _, partItem := range contentList {
					part, ok := partItem.(map[string]interface{})
					if !ok {
						continue
					}

					if part["type"] != "tool_result" {
						continue
					}

					if t.pipeline.config.PreserveCacheControl {
						if _, hasCache := part["cache_control"]; hasCache {
							continue
						}
					}

					toolName, _ := part["tool_name"].(string)
					if toolName == "" {
						toolUseID, _ := part["tool_use_id"].(string)
						toolName = toolNames[toolUseID]
					}
					cat := categorizeToolName(toolName)

					if textVal, isStr := part["content"].(string); isStr && len(textVal) > 0 {
						newText, res := t.processContentText(textVal, cat)
						if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
							part["content"] = newText
							totalResult.OriginalBytes += res.OriginalBytes
							totalResult.ReducedBytes += res.ReducedBytes
							totalResult.BytesSaved += res.BytesSaved
							totalResult.TokensSaved += res.TokensSaved
							totalResult.Duration += res.Duration
							modified = true
						}
					} else if subList, isList := part["content"].([]interface{}); isList {
						for _, subItem := range subList {
							subPart, ok := subItem.(map[string]interface{})
							if !ok {
								continue
							}
							if subPart["type"] == "text" {
								textVal, _ := subPart["text"].(string)
								if len(textVal) > 0 {
									newText, res := t.processContentText(textVal, cat)
									if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
										subPart["text"] = newText
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
					}
				}
			}
		}
	}

	return modified, totalResult
}

// processContentText compacts tool text output, unwrapping and re-wrapping inner JSON
// payloads emitted by clients like Cline / Vercel AI SDK (e.g. [{"query":...,"result":"..."}]).
func (t *Transformer) processContentText(text string, cat ToolCategory) (string, ReductionResult) {
	trimmed := bytes.TrimSpace([]byte(text))
	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		if trimmed[0] == '[' {
			var items []map[string]interface{}
			if err := json.Unmarshal(trimmed, &items); err == nil {
				var totalRes ReductionResult
				modified := false
				for _, item := range items {
					for _, field := range []string{"result", "output", "text", "content"} {
						if val, ok := item[field].(string); ok && len(val) > 0 {
							buf := []byte(val)
							res := t.pipeline.Process(buf, cat)
							if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
								item[field] = string(buf[:res.ReducedBytes])
								totalRes.OriginalBytes += res.OriginalBytes
								totalRes.ReducedBytes += res.ReducedBytes
								totalRes.BytesSaved += res.BytesSaved
								totalRes.TokensSaved += res.TokensSaved
								totalRes.Duration += res.Duration
								modified = true
							}
						}
					}
				}
				if modified {
					if newBytes, err := json.Marshal(items); err == nil {
						return string(newBytes), totalRes
					}
				}
			}
		} else {
			var item map[string]interface{}
			if err := json.Unmarshal(trimmed, &item); err == nil {
				var totalRes ReductionResult
				modified := false
				for _, field := range []string{"result", "output", "text", "content"} {
					if val, ok := item[field].(string); ok && len(val) > 0 {
						buf := []byte(val)
						res := t.pipeline.Process(buf, cat)
						if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
							item[field] = string(buf[:res.ReducedBytes])
							totalRes.OriginalBytes += res.OriginalBytes
							totalRes.ReducedBytes += res.ReducedBytes
							totalRes.BytesSaved += res.BytesSaved
							totalRes.TokensSaved += res.TokensSaved
							totalRes.Duration += res.Duration
							modified = true
						}
					}
				}
				if modified {
					if newBytes, err := json.Marshal(item); err == nil {
						return string(newBytes), totalRes
					}
				}
			}
		}
	}

	// Plain text processing
	buf := []byte(text)
	res := t.pipeline.Process(buf, cat)
	if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
		return string(buf[:res.ReducedBytes]), res
	}
	return text, res
}

func categorizeToolName(name string) ToolCategory {
	nameLower := strings.ToLower(name)
	if strings.Contains(nameLower, "read") || strings.Contains(nameLower, "view") || nameLower == "cat" {
		return CategoryFileRead
	}
	if strings.Contains(nameLower, "write") || strings.Contains(nameLower, "replace") || strings.Contains(nameLower, "patch") || strings.Contains(nameLower, "diff") || strings.Contains(nameLower, "edit") {
		return CategoryFileWrite
	}
	return CategoryGeneric
}
