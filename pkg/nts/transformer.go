package nts

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
	"github.com/dixieflatline76/nacho-flow/pkg/wirejson"
	"github.com/dixieflatline76/nacho-flow/pkg/zeroalloc"
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
	// Zero-allocation fast bypass: if payload contains no candidate tool outputs or reasoning markers
	if !t.hasCandidateCompaction(body) && isCleanMessagesArray(body) {
		return body, ReductionResult{Bypassed: true, BypassReason: "no_candidates"}, nil
	}

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
	modified, totalResult, changedIndices := t.processMessages(msgs, toolNames)
	if !modified {
		totalResult.Bypassed = true
		totalResult.BypassReason = "no_modifications"
		return body, totalResult, nil
	}

	// Message-level targeted splicing: locate "messages" array in the raw input body
	_, valStart, valEnd, ok := zeroalloc.ObjectField(body, 0, len(body), "messages")
	if !ok {
		newBody, err := wirejson.Marshal(req)
		return newBody, totalResult, err
	}

	type msgSpan struct {
		start, end int
	}
	var spans []msgSpan
	pos := valStart + 1
	for {
		start, end, next, ok := zeroalloc.NextElement(body, pos, valEnd)
		if !ok {
			break
		}
		spans = append(spans, msgSpan{start: start, end: end})
		pos = next
	}

	if len(spans) != len(msgs) {
		slog.Warn("NTS message count mismatch between scanner and JSON parser, falling back to full marshal",
			"scanner_count", len(spans), "json_count", len(msgs))
		newBody, err := wirejson.Marshal(req)
		return newBody, totalResult, err
	}

	var edits []zeroalloc.Edit
	for idx, sp := range spans {
		if _, changed := changedIndices[idx]; !changed {
			continue
		}
		msgBytes, err := wirejson.Marshal(msgs[idx])
		if err != nil {
			newBody, mErr := wirejson.Marshal(req)
			return newBody, totalResult, mErr
		}
		edits = append(edits, zeroalloc.Edit{
			Start: sp.start,
			End:   sp.end,
			Repl:  msgBytes,
		})
	}

	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Start < edits[j].Start
	})

	fits := true
	extraCap := 0
	for _, e := range edits {
		spanLen := e.End - e.Start
		if len(e.Repl) > spanLen {
			fits = false
			extraCap += len(e.Repl) - spanLen
		}
	}

	var newBody []byte
	if fits {
		// In-place edit when all replacements fit (Safety Rule 1 & 5)
		if out, ok := zeroalloc.SpliceInPlace(body, edits); ok {
			newBody = out
		}
	}

	if newBody == nil {
		// Growth fallback into fresh buffer
		dst := make([]byte, 0, len(body)+extraCap)
		if out, ok := zeroalloc.SpliceInto(dst, body, edits); ok {
			newBody = out
		} else {
			full, err := wirejson.Marshal(req)
			return full, totalResult, err
		}
	}

	totalResult.BytesSaved = len(body) - len(newBody)
	totalResult.TokensSaved = totalResult.BytesSaved / 4
	if totalResult.TokensSaved < 0 {
		totalResult.TokensSaved = 0
	}

	return newBody, totalResult, nil
}

// indexToolNames indexes tool call IDs to tool names across all assistant messages,
// supporting both OpenAI tool_calls and Anthropic tool_use content blocks.
func indexToolNames(msgs []interface{}) map[string]string {
	var names map[string]string

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
						if names == nil {
							names = make(map[string]string)
						}
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
					if names == nil {
						names = make(map[string]string)
					}
					names[id] = name
				}
			}
		}
	}

	return names
}

// processMessages traverses messages, identifies tool outputs, and applies compaction in-place.
func (t *Transformer) processMessages(msgs []interface{}, toolNames map[string]string) (bool, ReductionResult, map[int]struct{}) {
	var total ReductionResult
	modified := false
	changedIndices := make(map[int]struct{})

	var staleIDs map[string]struct{}
	if t.pipeline.config.CompactStaleFileReads {
		depth := t.pipeline.config.StaleReadDepth
		if depth <= 0 {
			depth = 3
		}
		staleIDs = IdentifyStaleToolCallIDs(msgs, depth)
	}

	var historicalAssistantIndices map[int]struct{}
	if t.pipeline.config.StripHistoricalReasoning {
		var assistantIndices []int
		for idx, msgItem := range msgs {
			if msg, ok := msgItem.(map[string]interface{}); ok {
				if role, _ := msg["role"].(string); role == "assistant" {
					assistantIndices = append(assistantIndices, idx)
				}
			}
		}
		keepTurns := t.pipeline.config.KeepRecentReasoningTurns
		if keepTurns < 0 {
			keepTurns = 0
		}
		cutoff := len(assistantIndices) - keepTurns
		if cutoff > 0 {
			historicalAssistantIndices = make(map[int]struct{}, cutoff)
			for i := 0; i < cutoff; i++ {
				historicalAssistantIndices[assistantIndices[i]] = struct{}{}
			}
		}
	}

	var remindersCtx *DeduplicateRemindersContext
	if t.pipeline.config.DeduplicateReminders {
		remindersCtx = NewDeduplicateRemindersContext(t.pipeline.config.RemindersRefreshInterval)
	}

	var toolDedupCtx *ToolDedupContext
	if t.pipeline.config.DeduplicateToolOutputs {
		toolDedupCtx = NewToolDedupContext(t.pipeline.config.MinDuplicateOutputBytes)
	}

	for idx, msgItem := range msgs {
		msg, ok := msgItem.(map[string]interface{})
		if !ok {
			continue
		}

		// Historical assistant reasoning compaction
		if historicalAssistantIndices != nil {
			if _, isHistorical := historicalAssistantIndices[idx]; isHistorical {
				if t.pruneAssistantReasoning(msg, &total) {
					modified = true
					changedIndices[idx] = struct{}{}
				}
			}
		}

		// Format A: OpenAI role == "tool"
		if role, _ := msg["role"].(string); role == "tool" {
			toolCallID, _ := msg["tool_call_id"].(string)
			toolName := resolveToolName(msg["name"], toolCallID, toolNames)
			hasCache := hasCacheControl(msg)
			isErr, _ := msg["is_error"].(bool)

			// Evict stale file reads before standard compaction (with tool error immunity)
			if res, evicted := t.tryEvictStaleRead(msg["content"], toolCallID, isErr, hasCache, staleIDs); evicted {
				msg["content"] = StaleFileReadNotice
				total.merge(res)
				modified = true
				changedIndices[idx] = struct{}{}
				continue
			}

			newContent, res, changed := t.compactToolContent(msg["content"], toolName)
			if changed {
				msg["content"] = newContent
				total.merge(res)
				modified = true
				changedIndices[idx] = struct{}{}
			}

			if toolDedupCtx != nil {
				if dedupContent, dedupRes, dedupChanged := toolDedupCtx.Deduplicate(msg["content"], toolName, isErr); dedupChanged {
					msg["content"] = dedupContent
					total.merge(dedupRes)
					modified = true
					changedIndices[idx] = struct{}{}
				}
			}
			continue
		}

		// Format C: Messages with <environment_details> REMINDERS table (User messages & Tool results)
		if remindersCtx != nil {
			if contentStr, ok := msg["content"].(string); ok && strings.Contains(contentStr, "environment_details") {
				if newContent, res, remChanged := remindersCtx.CompactReminders(contentStr); remChanged {
					msg["content"] = newContent
					total.merge(res)
					modified = true
					changedIndices[idx] = struct{}{}
				}
			} else if contentList, ok := msg["content"].([]interface{}); ok {
				for _, partItem := range contentList {
					if part, ok := partItem.(map[string]interface{}); ok {
						partType, _ := part["type"].(string)
						switch partType {
						case "text":
							if textStr, ok := part["text"].(string); ok && strings.Contains(textStr, "environment_details") {
								if newText, res, remChanged := remindersCtx.CompactReminders(textStr); remChanged {
									part["text"] = newText
									total.merge(res)
									modified = true
									changedIndices[idx] = struct{}{}
								}
							}
						case "tool_result":
							if textStr, ok := part["content"].(string); ok && strings.Contains(textStr, "environment_details") {
								if newText, res, remChanged := remindersCtx.CompactReminders(textStr); remChanged {
									part["content"] = newText
									total.merge(res)
									modified = true
									changedIndices[idx] = struct{}{}
								}
							}
						}
					}
				}
			}
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
			isErr, _ := part["is_error"].(bool)

			// Evict stale file reads before standard compaction (with tool error immunity)
			if res, evicted := t.tryEvictStaleRead(part["content"], toolUseID, isErr, hasCache, staleIDs); evicted {
				part["content"] = StaleFileReadNotice
				total.merge(res)
				modified = true
				changedIndices[idx] = struct{}{}
				continue
			}

			newContent, res, changed := t.compactToolContent(part["content"], toolName)
			if changed {
				part["content"] = newContent
				total.merge(res)
				modified = true
				changedIndices[idx] = struct{}{}
			}

			if toolDedupCtx != nil {
				if dedupContent, dedupRes, dedupChanged := toolDedupCtx.Deduplicate(part["content"], toolName, isErr); dedupChanged {
					part["content"] = dedupContent
					total.merge(dedupRes)
					modified = true
					changedIndices[idx] = struct{}{}
				}
			}
		}
	}

	return modified, total, changedIndices
}

func (t *Transformer) tryEvictStaleRead(content interface{}, id string, isError, hasCache bool, staleIDs map[string]struct{}) (ReductionResult, bool) {
	if staleIDs == nil || id == "" {
		return ReductionResult{}, false
	}
	if _, isStale := staleIDs[id]; !isStale {
		return ReductionResult{}, false
	}
	if t.pipeline.config.PreserveCacheControl && hasCache {
		return ReductionResult{}, false
	}
	contentStr := extractContentString(content)
	if IsToolError(contentStr, isError) {
		return ReductionResult{}, false
	}

	origBytes := calculateContentLength(content)
	reducedBytes := len(StaleFileReadNotice)
	saved := origBytes - reducedBytes
	if saved < 0 {
		saved = 0
	}
	return ReductionResult{
		OriginalBytes: origBytes,
		ReducedBytes:  reducedBytes,
		BytesSaved:    saved,
		TokensSaved:   (saved + 3) / 4,
	}, true
}

// compactToolContent handles tool content in both string form and content-block list form.
// Deterministic per-message passes execute unconditionally so sliding cache breakpoints
// never cause retroactive byte mutations on subsequent turns.
func (t *Transformer) compactToolContent(content interface{}, toolName string) (interface{}, ReductionResult, bool) {
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
// (emitted by agents like Cline / Zoo Code), it unpacks, compacts the inner fields,
// and re-marshals the JSON. Otherwise, it processes the raw text directly.
func (t *Transformer) compactText(text string, cat ToolCategory) (string, ReductionResult, bool) {
	trimmed := strings.TrimSpace(text)

	// Attempt inner-JSON compaction if text resembles an array or object
	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		if newJSON, res, ok := t.compactInnerJSON([]byte(trimmed), cat); ok {
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
			if reencoded, err := wirejson.Marshal(items); err == nil {
				return string(reencoded), total, true
			}
		}
	} else {
		var item map[string]interface{}
		if err := json.Unmarshal(raw, &item); err != nil {
			return "", ReductionResult{}, false
		}
		if t.compactItemFields(item, cat, &total) {
			if reencoded, err := wirejson.Marshal(item); err == nil {
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
	if id, ok := callID.(string); ok && id != "" && toolNames != nil {
		return toolNames[id]
	}
	return ""
}

func hasCacheControl(m map[string]interface{}) bool {
	_, ok := m["cache_control"]
	return ok
}

func categorizeToolName(name string) ToolCategory {
	reg := agentregistry.DefaultRegistry()
	if reg.IsWriteTool(name) {
		return CategoryFileWrite
	}
	if reg.IsFileReadTool(name) {
		return CategoryFileRead
	}
	return CategoryGeneric
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

func extractContentString(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case []interface{}:
		if len(v) == 1 {
			if part, ok := v[0].(map[string]interface{}); ok {
				if text, ok := part["text"].(string); ok {
					return text
				}
			}
		}
		var sb strings.Builder
		for _, item := range v {
			if part, ok := item.(map[string]interface{}); ok {
				if text, ok := part["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		return sb.String()
	default:
		return ""
	}
}

// DefaultSilentTurnMessage is injected into historical assistant messages that become empty
// after reasoning pruning, preventing Anthropic RFC 400 "content cannot be empty" violations.
const DefaultSilentTurnMessage = "Proceeding with next step."

// pruneAssistantReasoning strips historical reasoning blocks, OpenAI reasoning fields,
// and inline thinking tags from an assistant message while preserving surviving content,
// tool calls, and cache control breakpoints.
func (t *Transformer) pruneAssistantReasoning(msg map[string]interface{}, total *ReductionResult) bool {
	modified := false

	// 1. Delete OpenAI top-level reasoning fields
	reasoningFields := []string{"reasoning_content", "reasoning", "reason"}
	for _, field := range reasoningFields {
		if val, exists := msg[field]; exists {
			var fieldBytes int
			switch v := val.(type) {
			case string:
				fieldBytes = len(v)
			case []byte:
				fieldBytes = len(v)
			default:
				if b, err := json.Marshal(v); err == nil {
					fieldBytes = len(b)
				}
			}
			savedBytes := fieldBytes + len(field) + 4
			savedTokens := (savedBytes + 3) / 4
			total.merge(ReductionResult{
				OriginalBytes: savedBytes,
				ReducedBytes:  0,
				BytesSaved:    savedBytes,
				TokensSaved:   savedTokens,
			})
			delete(msg, field)
			modified = true
		}
	}

	// 2. Handle assistant content
	switch contentVal := msg["content"].(type) {
	case []interface{}:
		var surviving []interface{}
		var migratedCacheControl interface{}
		blocksModified := false

		for _, item := range contentVal {
			part, ok := item.(map[string]interface{})
			if !ok {
				surviving = append(surviving, item)
				continue
			}

			partType, _ := part["type"].(string)
			if isReasoningBlockType(partType) {
				blocksModified = true
				if cc, ok := part["cache_control"]; ok && cc != nil {
					migratedCacheControl = cc
				}
				partBytes := calculateBlockLength(part)
				savedTokens := (partBytes + 3) / 4
				total.merge(ReductionResult{
					OriginalBytes: partBytes,
					ReducedBytes:  0,
					BytesSaved:    partBytes,
					TokensSaved:   savedTokens,
				})
				continue
			}

			// If text block, strip any inline thinking tags
			if partType == "text" {
				if text, ok := part["text"].(string); ok && len(text) > 0 {
					strippedText, saved := stripInlineThinking(text)
					if saved > 0 {
						blocksModified = true
						part["text"] = strippedText
						total.merge(ReductionResult{
							OriginalBytes: len(text),
							ReducedBytes:  len(strippedText),
							BytesSaved:    saved,
							TokensSaved:   (saved + 3) / 4,
						})
					}
				}
			}

			surviving = append(surviving, part)
		}

		if blocksModified {
			modified = true
			// Anthropic RFC 400 guard: content array cannot be empty
			if len(surviving) == 0 {
				fallback := map[string]interface{}{
					"type": "text",
					"text": DefaultSilentTurnMessage,
				}
				if t.pipeline.config.PreserveCacheControl && migratedCacheControl != nil {
					fallback["cache_control"] = migratedCacheControl
					migratedCacheControl = nil
				}
				surviving = append(surviving, fallback)
			} else if t.pipeline.config.PreserveCacheControl && migratedCacheControl != nil {
				// Migrate cache control to the first surviving block if it lacks one
				if firstBlock, ok := surviving[0].(map[string]interface{}); ok {
					if _, hasCC := firstBlock["cache_control"]; !hasCC {
						firstBlock["cache_control"] = migratedCacheControl
						migratedCacheControl = nil
					}
				}
			}
			msg["content"] = surviving
		}

	case string:
		if len(contentVal) > 0 {
			strippedText, saved := stripInlineThinking(contentVal)
			if saved > 0 {
				modified = true
				total.merge(ReductionResult{
					OriginalBytes: len(contentVal),
					ReducedBytes:  len(strippedText),
					BytesSaved:    saved,
					TokensSaved:   (saved + 3) / 4,
				})
				if strings.TrimSpace(strippedText) == "" {
					hasToolCalls := false
					if tc, ok := msg["tool_calls"].([]interface{}); ok && len(tc) > 0 {
						hasToolCalls = true
					}
					if !hasToolCalls {
						msg["content"] = DefaultSilentTurnMessage
					} else {
						msg["content"] = ""
					}
				} else {
					msg["content"] = strippedText
				}
			}
		}

	default:
		hasToolCalls := false
		if tc, ok := msg["tool_calls"].([]interface{}); ok && len(tc) > 0 {
			hasToolCalls = true
		}
		if !hasToolCalls && modified {
			msg["content"] = DefaultSilentTurnMessage
		}
	}

	return modified
}

func isReasoningBlockType(t string) bool {
	switch t {
	case "thinking", "redacted_thinking", "thought", "reasoning":
		return true
	default:
		return false
	}
}

func calculateBlockLength(part map[string]interface{}) int {
	for _, f := range []string{"thinking", "data", "thought", "reasoning", "text"} {
		if s, ok := part[f].(string); ok {
			return len(s)
		}
	}
	b, _ := json.Marshal(part)
	return len(b)
}

func stripInlineThinking(s string) (string, int) {
	if s == "" {
		return "", 0
	}
	if strings.IndexByte(s, '<') == -1 {
		return s, 0
	}

	reg := agentregistry.DefaultRegistry()
	replacer := reg.TagReplacer()
	canonical := replacer.Replace(s)

	if !strings.Contains(canonical, "<think>") && !strings.Contains(canonical, "</think>") {
		if canonical == s {
			return s, 0
		}
		saved := len(s) - len(canonical)
		if saved < 0 {
			saved = 0
		}
		return canonical, saved
	}

	var sb strings.Builder
	cur := canonical
	for {
		openIdx := strings.Index(cur, "<think>")
		if openIdx == -1 {
			rest := cur
			if strings.Contains(rest, "</think>") {
				rest = strings.ReplaceAll(rest, "</think>", "")
			}
			sb.WriteString(rest)
			break
		}

		prefix := cur[:openIdx]
		if strings.Contains(prefix, "</think>") {
			prefix = strings.ReplaceAll(prefix, "</think>", "")
		}
		sb.WriteString(prefix)

		afterOpen := cur[openIdx+len("<think>"):]
		closeIdx := strings.Index(afterOpen, "</think>")
		if closeIdx == -1 {
			break
		}

		cur = afterOpen[closeIdx+len("</think>"):]
	}

	result := sb.String()
	saved := len(s) - len(result)
	if saved < 0 {
		saved = 0
	}
	return result, saved
}

func (t *Transformer) hasCandidateCompaction(body []byte) bool {
	if bytes.Contains(body, []byte(`"tool`)) {
		return true
	}
	if t.pipeline.config.DeduplicateReminders && bytes.Contains(body, []byte(`environment_details`)) {
		return true
	}
	if t.pipeline.config.StripHistoricalReasoning && bytes.Contains(body, []byte(`"assistant"`)) {
		if bytes.IndexByte(body, '<') != -1 ||
			bytes.Contains(body, []byte(`"reason`)) ||
			bytes.Contains(body, []byte(`"thinking"`)) ||
			bytes.Contains(body, []byte(`"thought"`)) ||
			bytes.Contains(body, []byte(`\u003c`)) ||
			bytes.Contains(body, []byte(`\u003C`)) {
			return true
		}
	}
	return false
}

func isCleanMessagesArray(body []byte) bool {
	idx := bytes.Index(body, []byte(`"messages"`))
	if idx == -1 {
		return false
	}
	rest := body[idx+len(`"messages"`):]
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ':' {
			continue
		}
		return c == '['
	}
	return false
}
