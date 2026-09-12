package nts

import (
	"bytes"
	"encoding/json"
	"strings"
)

// targetJSONFields are field names within inner-JSON payloads that carry tool output text.
var targetJSONFields = []string{"result", "output", "text", "content"}

// Transformer inspects incoming HTTP request JSON payloads (Anthropic Messages / OpenAI Chat)
// and applies wire-speed NTS compaction to tool outputs with dual-lane immunity guards.
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

// TransformAnthropic compacts tool responses in an Anthropic Messages API payload.
func (t *Transformer) TransformAnthropic(body []byte) ([]byte, ReductionResult, error) {
	return t.transformPayload(body)
}

// TransformOpenAI compacts tool responses in an OpenAI Chat Completions payload.
func (t *Transformer) TransformOpenAI(body []byte) ([]byte, ReductionResult, error) {
	return t.transformPayload(body)
}

func (t *Transformer) transformPayload(body []byte) ([]byte, ReductionResult, error) {
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

// indexToolNames indexes tool call IDs to tool names across all assistant messages,
// supporting both OpenAI tool_calls and Anthropic tool_use content blocks.
func indexToolNames(msgs []interface{}) map[string]string {
	names := make(map[string]string)

	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}

		// 1. OpenAI function calls: assistant.tool_calls[].function.name
		if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
			for _, tcItem := range toolCalls {
				tc, ok := tcItem.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := tc["id"].(string)
				fn, ok := tc["function"].(map[string]interface{})
				if ok && id != "" {
					if name, ok := fn["name"].(string); ok && name != "" {
						names[id] = name
					}
				}
			}
		}

		// 2. Anthropic content blocks: assistant.content[type == "tool_use"].name
		if contentList, ok := msg["content"].([]interface{}); ok {
			for _, partItem := range contentList {
				part, ok := partItem.(map[string]interface{})
				if !ok || part["type"] != "tool_use" {
					continue
				}
				id, _ := part["id"].(string)
				name, _ := part["name"].(string)
				if id != "" && name != "" {
					names[id] = name
				}
			}
		}
	}

	return names
}

// processMessages traverses messages, identifies tool outputs, and applies compaction in-place.
func (t *Transformer) processMessages(msgs []interface{}, toolNames map[string]string) (bool, ReductionResult) {
	var total ReductionResult
	modified := false

	var staleIDs map[string]struct{}
	if t.pipeline.config.CompactStaleFileReads {
		staleIDs = IdentifyStaleToolCallIDs(msgs)
	}

	for _, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}

		// Format A: OpenAI role == "tool"
		if role, _ := msg["role"].(string); role == "tool" {
			toolCallID, _ := msg["tool_call_id"].(string)
			toolName := resolveToolName(msg["name"], toolCallID, toolNames)
			hasCache := hasCacheControl(msg)

			// Evict stale file reads before standard compaction
			if staleIDs != nil && toolCallID != "" {
				if _, isStale := staleIDs[toolCallID]; isStale {
					if !t.pipeline.config.PreserveCacheControl || !hasCache {
						origBytes := calculateContentLength(msg["content"])
						msg["content"] = StaleFileReadNotice
						reducedBytes := len(StaleFileReadNotice)
						saved := origBytes - reducedBytes
						if saved > 0 {
							total.merge(ReductionResult{
								OriginalBytes: origBytes,
								ReducedBytes:  reducedBytes,
								BytesSaved:    saved,
								TokensSaved:   (saved + 3) / 4,
							})
							modified = true
							continue
						}
					}
				}
			}

			newContent, res, changed := t.compactToolContent(msg["content"], toolName, hasCache)
			if changed {
				msg["content"] = newContent
				total.merge(res)
				modified = true
			}
			continue
		}

		// Format B: Content blocks with type == "tool_result" (Anthropic / Zoo Code / Vercel AI)
		contentList, ok := msg["content"].([]interface{})
		if !ok {
			continue
		}

		for _, partItem := range contentList {
			part, ok := partItem.(map[string]interface{})
			if !ok || part["type"] != "tool_result" {
				continue
			}

			toolUseID, _ := part["tool_use_id"].(string)
			toolName := resolveToolName(part["tool_name"], toolUseID, toolNames)
			hasCache := hasCacheControl(part)

			// Evict stale file reads before standard compaction
			if staleIDs != nil && toolUseID != "" {
				if _, isStale := staleIDs[toolUseID]; isStale {
					if !t.pipeline.config.PreserveCacheControl || !hasCache {
						origBytes := calculateContentLength(part["content"])
						part["content"] = StaleFileReadNotice
						reducedBytes := len(StaleFileReadNotice)
						saved := origBytes - reducedBytes
						if saved > 0 {
							total.merge(ReductionResult{
								OriginalBytes: origBytes,
								ReducedBytes:  reducedBytes,
								BytesSaved:    saved,
								TokensSaved:   (saved + 3) / 4,
							})
							modified = true
							continue
						}
					}
				}
			}

			newContent, res, changed := t.compactToolContent(part["content"], toolName, hasCache)
			if changed {
				part["content"] = newContent
				total.merge(res)
				modified = true
			}
		}
	}

	return modified, total
}

// compactToolContent handles tool content in both string form and content-block list form.
func (t *Transformer) compactToolContent(content interface{}, toolName string, hasCache bool) (interface{}, ReductionResult, bool) {
	if t.pipeline.config.PreserveCacheControl && hasCache {
		return content, ReductionResult{}, false
	}

	category := categorizeToolName(toolName)

	switch val := content.(type) {
	case string:
		if len(val) == 0 {
			return content, ReductionResult{}, false
		}
		newText, res, changed := t.compactText(val, category)
		return newText, res, changed

	case []interface{}:
		var total ReductionResult
		modified := false
		for _, item := range val {
			part, ok := item.(map[string]interface{})
			if !ok || part["type"] != "text" {
				continue
			}
			text, ok := part["text"].(string)
			if !ok || len(text) == 0 {
				continue
			}
			newText, res, changed := t.compactText(text, category)
			if changed {
				part["text"] = newText
				total.merge(res)
				modified = true
			}
		}
		return val, total, modified

	default:
		return content, ReductionResult{}, false
	}
}

// compactText compacts a tool text string. If the string is serialized inner-JSON
// (emitted by agents like Cline / Roo Code), it unpacks, compacts the inner fields,
// and re-marshals the JSON. Otherwise, it processes the raw text directly.
func (t *Transformer) compactText(text string, cat ToolCategory) (string, ReductionResult, bool) {
	trimmed := bytes.TrimSpace([]byte(text))

	// Attempt inner-JSON compaction if text resembles an array or object
	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		if newJSON, res, ok := t.compactInnerJSON(trimmed, cat); ok {
			return newJSON, res, true
		}
	}

	// Direct string compaction
	buf := []byte(text)
	res := t.pipeline.Process(buf, cat)
	if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
		return string(buf[:res.ReducedBytes]), res, true
	}

	return text, res, false
}

// compactInnerJSON parses and compacts text fields inside JSON strings.
func (t *Transformer) compactInnerJSON(raw []byte, cat ToolCategory) (string, ReductionResult, bool) {
	var total ReductionResult
	modified := false

	if raw[0] == '[' {
		var items []map[string]interface{}
		if err := json.Unmarshal(raw, &items); err != nil {
			return "", ReductionResult{}, false
		}
		for _, item := range items {
			if t.compactItemFields(item, cat, &total) {
				modified = true
			}
		}
		if modified {
			if reencoded, err := json.Marshal(items); err == nil {
				return string(reencoded), total, true
			}
		}
	} else {
		var item map[string]interface{}
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", ReductionResult{}, false
		}
		if t.compactItemFields(item, cat, &total) {
			if reencoded, err := json.Marshal(item); err == nil {
				return string(reencoded), total, true
			}
		}
	}

	return "", ReductionResult{}, false
}

// compactItemFields scans known output fields in an object map and compacts them.
func (t *Transformer) compactItemFields(item map[string]interface{}, cat ToolCategory, total *ReductionResult) bool {
	itemModified := false
	for _, field := range targetJSONFields {
		val, ok := item[field].(string)
		if !ok || len(val) == 0 {
			continue
		}
		buf := []byte(val)
		res := t.pipeline.Process(buf, cat)
		if !res.Bypassed && res.ReducedBytes < res.OriginalBytes {
			item[field] = string(buf[:res.ReducedBytes])
			total.merge(res)
			itemModified = true
		}
	}
	return itemModified
}

func resolveToolName(explicitName, callID interface{}, toolNames map[string]string) string {
	if name, ok := explicitName.(string); ok && name != "" {
		return name
	}
	if id, ok := callID.(string); ok && id != "" {
		return toolNames[id]
	}
	return ""
}

func hasCacheControl(m map[string]interface{}) bool {
	_, ok := m["cache_control"]
	return ok
}

func categorizeToolName(name string) ToolCategory {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "read") || strings.Contains(n, "view") || n == "cat":
		return CategoryFileRead
	case strings.Contains(n, "write") || strings.Contains(n, "edit") ||
		strings.Contains(n, "replace") || strings.Contains(n, "patch") || strings.Contains(n, "diff"):
		return CategoryFileWrite
	default:
		return CategoryGeneric
	}
}

func (r *ReductionResult) merge(other ReductionResult) {
	r.OriginalBytes += other.OriginalBytes
	r.ReducedBytes += other.ReducedBytes
	r.BytesSaved += other.BytesSaved
	r.TokensSaved += other.TokensSaved
	r.Duration += other.Duration
}

func calculateContentLength(content interface{}) int {
	switch v := content.(type) {
	case string:
		return len(v)
	case []byte:
		return len(v)
	case []interface{}:
		total := 0
		for _, item := range v {
			if part, ok := item.(map[string]interface{}); ok {
				if text, ok := part["text"].(string); ok {
					total += len(text)
				}
			}
		}
		return total
	default:
		return 0
	}
}
