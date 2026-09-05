package router

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

type RequestClassifier struct {
	mu                  sync.RWMutex
	estimator           *TokenEstimator
	errorSignatures     []string
	kickstartWriteTools []string
	writeToolsLookup    atomic.Pointer[map[string]bool]
}

// defaultAgentErrorSignatures are fallback error patterns injected by agent clients
// (Cline, Zoo Code, OpenCode) when no custom error_signatures are specified in config.yaml.
var defaultAgentErrorSignatures = []string{
	"[ERROR] You did not use a tool",
	"Missing value for required parameter",
	"The tool execution failed",
	"<error_details>",
	"No sufficiently similar match found",
	"Command failed with exit code",
	"Please retry with complete response",
	"Editor operation failed",
	"Parameter `old_text` is required",
	"Parameter old_text is required",
	"Command not executed:",
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

// SetErrorSignatures configures custom error patterns from config.yaml.
func (c *RequestClassifier) SetErrorSignatures(signatures []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(signatures) == 0 {
		c.errorSignatures = nil
		return
	}
	c.errorSignatures = make([]string, len(signatures))
	copy(c.errorSignatures, signatures)
}

// GetErrorSignatures returns active error signatures or default fallback.
func (c *RequestClassifier) GetErrorSignatures() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.errorSignatures) == 0 {
		return defaultAgentErrorSignatures
	}
	return c.errorSignatures
}

// SetKickstartWriteTools configures custom write-tool names from config.yaml.
func (c *RequestClassifier) SetKickstartWriteTools(tools []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(tools) == 0 {
		c.kickstartWriteTools = nil
		c.writeToolsLookup.Store(nil)
		return
	}
	c.kickstartWriteTools = make([]string, len(tools))
	copy(c.kickstartWriteTools, tools)

	lookup := make(map[string]bool, len(tools))
	for _, t := range tools {
		lookup[strings.ToLower(strings.TrimSpace(t))] = true
	}
	c.writeToolsLookup.Store(&lookup)
}

// GetKickstartWriteTools returns the configured write-tool names as a lookup map.
// Returns an empty map if no tools are configured — kickstart_write_only will
// effectively never detect write progress unless the list is specified in config.
func (c *RequestClassifier) GetKickstartWriteTools() map[string]bool {
	ptr := c.writeToolsLookup.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// GetEstimator returns the active TokenEstimator instance for dynamic calibration.
func (c *RequestClassifier) GetEstimator() *TokenEstimator {
	if c.estimator == nil {
		c.estimator = NewTokenEstimator()
	}
	return c.estimator
}

type classifyPayload struct {
	Messages []classifyMessage    `json:"messages"`
	Tools    []classifyToolSchema `json:"tools,omitempty"`
}

type classifyMessage struct {
	Role       string             `json:"role"`
	Content    classifyContent    `json:"content"`
	ToolCalls  []classifyToolCall `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

func (m *classifyMessage) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	type rawMsg classifyMessage
	return json.Unmarshal(data, (*rawMsg)(m))
}

type classifyContent struct {
	Text  string
	Parts []classifyContentPart
}

func (c *classifyContent) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if data[0] == '"' {
		return json.Unmarshal(data, &c.Text)
	}
	if data[0] == '[' {
		return json.Unmarshal(data, &c.Parts)
	}
	return nil
}

type classifyContentPart struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

func (p *classifyContentPart) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	type rawPart classifyContentPart
	return json.Unmarshal(data, (*rawPart)(p))
}

type classifyToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type,omitempty"`
	Name     string              `json:"name,omitempty"`
	Function *classifyToolCallFn `json:"function,omitempty"`
}

type classifyToolCallFn struct {
	Name string `json:"name"`
}

type classifyToolSchema struct {
	Type     string                `json:"type,omitempty"`
	Name     string                `json:"name,omitempty"`
	Function *classifyToolSchemaFn `json:"function,omitempty"`
}

func (t *classifyToolSchema) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	type rawTool classifyToolSchema
	return json.Unmarshal(data, (*rawTool)(t))
}

type classifyToolSchemaFn struct {
	Name string `json:"name"`
}

// Classify extracts token count, tools presence, image presence, and prompt keywords from request JSON.
func (c *RequestClassifier) Classify(body []byte) (contract.RequestContext, error) {
	reqCtx := contract.RequestContext{}

	var payload classifyPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return reqCtx, err
	}

	// 1. Check tools and extract supported interactive tool for agent fallback shield
	if len(payload.Tools) > 0 {
		reqCtx.HasTools = true
		writeLookup := c.GetKickstartWriteTools()

		for _, t := range payload.Tools {
			fnName := t.Name
			if t.Function != nil && t.Function.Name != "" {
				fnName = t.Function.Name
			}

			lower := strings.ToLower(fnName)
			if reqCtx.InteractiveTool == "" && (lower == "ask_followup_question" || lower == "ask_question" || lower == "user_prompt" || lower == "interactive_input") {
				reqCtx.InteractiveTool = fnName
			}

			if !reqCtx.HasWriteCapability && writeLookup != nil && writeLookup[strings.ToLower(strings.TrimSpace(fnName))] {
				reqCtx.HasWriteCapability = true
			}
		}
	}

	// 2. Parse messages to extract prompt, keywords, image flags, and history errors
	if len(payload.Messages) == 0 {
		return reqCtx, nil
	}

	var latestUserPrompt string
	var fallbackText strings.Builder
	hasNonEmptyContent := false

	for _, msg := range payload.Messages {
		if msg.Content.Text != "" {
			hasNonEmptyContent = true
			fallbackText.WriteString(msg.Content.Text)
			fallbackText.WriteString(" ")
			if msg.Role == "user" {
				latestUserPrompt = msg.Content.Text
			}
		}

		for _, part := range msg.Content.Parts {
			switch part.Type {
			case "image_url":
				reqCtx.HasImages = true
				hasNonEmptyContent = true
			case "text":
				if part.Text != "" {
					hasNonEmptyContent = true
					fallbackText.WriteString(part.Text)
					fallbackText.WriteString(" ")
					if msg.Role == "user" {
						latestUserPrompt = part.Text
					}
				}
			}
		}
	}

	// 2.5 Scan trailing messages for error patterns and tool progress
	reqCtx.HistoryErrors, reqCtx.HasToolProgress, reqCtx.HasWriteProgress, reqCtx.HasTestPass, reqCtx.HasTestFail = c.scanTrailingTyped(payload.Messages)
	reqCtx.HasTestProgress = reqCtx.HasTestPass && !reqCtx.HasTestFail

	// 3. Approximate total token count using zero-allocation len(body) estimator
	if hasNonEmptyContent || reqCtx.HasTools {
		estimator := c.GetEstimator()
		reqCtx.Tokens = estimator.Estimate(len(body))
	}
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

	// 5. Extract clean lowercased keywords strictly from latest user prompt (fallback to fallbackText if no user prompt)
	keywordSource := reqCtx.CleanPrompt
	if keywordSource == "" {
		keywordSource = fallbackText.String()
	}
	reqCtx.Keywords = extractKeywords(keywordSource)

	return reqCtx, nil
}

func (c *RequestClassifier) scanTrailingTyped(messages []classifyMessage) (historyErrors int, hasToolProgress bool, hasWriteProgress bool, hasTestPass bool, hasTestFail bool) {
	signatures := c.GetErrorSignatures()
	writeTools := c.GetKickstartWriteTools()

	start := len(messages) - 8
	if start < 0 {
		start = 0
	}

	var stackIDs [8]string
	writeCallIDs := stackIDs[:0]
	hasAnyWriteCall := false

	// Pass 1: assistant tool calls
	for i := start; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				fnName := tc.Name
				if tc.Function != nil && tc.Function.Name != "" {
					fnName = tc.Function.Name
				}
				if writeTools != nil && writeTools[strings.ToLower(strings.TrimSpace(fnName))] {
					if tc.ID != "" {
						writeCallIDs = append(writeCallIDs, tc.ID)
					}
					hasAnyWriteCall = true
				}
			}
			for _, p := range msg.Content.Parts {
				if p.Type == "tool_use" {
					if writeTools != nil && writeTools[strings.ToLower(strings.TrimSpace(p.Name))] {
						if p.ID != "" {
							writeCallIDs = append(writeCallIDs, p.ID)
						}
						hasAnyWriteCall = true
					}
				}
			}
			if msg.Content.Text != "" && len(writeTools) > 0 {
				lowerText := strings.ToLower(msg.Content.Text)
				for toolName := range writeTools {
					if strings.Contains(lowerText, "<"+toolName+">") {
						hasAnyWriteCall = true
						break
					}
				}
			}
		}
	}

	// Pass 2: tool results
	for i := start; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "tool" {
			text := msg.Content.Text
			if text == "" && len(msg.Content.Parts) > 0 {
				var sb strings.Builder
				for _, p := range msg.Content.Parts {
					if p.Text != "" {
						sb.WriteString(p.Text)
						sb.WriteString(" ")
					}
				}
				text = sb.String()
			}
			if !isErrorText(text, signatures) {
				hasToolProgress = true
				if msg.ToolCallID != "" && writeCallIDsContains(writeCallIDs, msg.ToolCallID) {
					hasWriteProgress = true
				} else if msg.ToolCallID == "" && hasAnyWriteCall {
					hasWriteProgress = true
				}
			}
			p, f := detectTestSignals(text)
			if p {
				hasTestPass = true
			}
			if f {
				hasTestFail = true
			}
		}
		// Anthropic tool_result parts
		for _, part := range msg.Content.Parts {
			if part.Type == "tool_result" {
				text := part.Text
				if !part.IsError {
					if !isErrorText(text, signatures) {
						hasToolProgress = true
						if part.ToolUseID != "" && writeCallIDsContains(writeCallIDs, part.ToolUseID) {
							hasWriteProgress = true
						} else if part.ToolUseID == "" && hasAnyWriteCall {
							hasWriteProgress = true
						}
					}
				}
				p, f := detectTestSignals(text)
				if p {
					hasTestPass = true
				}
				if f {
					hasTestFail = true
				}
			}
		}
	}

	// Pass 3: trailing consecutive error turns
	for i := len(messages) - 1; i >= start; i-- {
		msg := messages[i]
		if msg.Role == "tool" {
			text := msg.Content.Text
			if text == "" && len(msg.Content.Parts) > 0 {
				var sb strings.Builder
				for _, p := range msg.Content.Parts {
					if p.Text != "" {
						sb.WriteString(p.Text)
						sb.WriteString(" ")
					}
				}
				text = sb.String()
			}
			if isErrorText(text, signatures) {
				historyErrors++
			} else {
				break
			}
		} else if msg.Role == "assistant" {
			continue
		} else {
			break
		}
	}

	return historyErrors, hasToolProgress, hasWriteProgress, hasTestPass, hasTestFail
}

var (
	testFailSignatures = []string{
		// Go test failure signatures
		"--- FAIL:", "FAIL\t", "FAIL\n",
		// Go compiler & linker errors
		"undefined:", "cannot use", "syntax error", "build failed",
		"compilation failed", "does not implement", "too many arguments",
		"not enough arguments", "declared and not used",
		// Jest / Vitest / Mocha
		"FAIL ",
		// Pytest / Unittest
		"=== FAILURES ===", "FAILED ", "FAIL: test_", "Traceback (most recent call last):",
		// Cargo / Rust
		"test result: FAILED", "error[E",
		// General CLI exit failure
		"Command failed with exit code", "exit status 1", "exit status 2",
	}

	testPassSignatures = []string{
		// Go test pass signatures
		"--- PASS:", "ok  \t", "\tok\t", "PASS\n",
		// Jest / Vitest
		"PASS ", "passed, ", "passed\n", "0 failed",
		// Pytest
		"=== 1 passed", "=== 2 passed", " passed in ",
		// Cargo
		"test result: ok",
	}
)

// isFailingTestOutput checks if text contains actual test/build failure indicators.
// It specifically distinguishes "0 failed" (a passing summary) from actual failures (e.g. "1 failed", "build failed").
func isFailingTestOutput(text string) bool {
	for _, sig := range testFailSignatures {
		if strings.Contains(text, sig) {
			return true
		}
	}

	// Inspect occurrences of "failed" to catch framework summaries (e.g. "1 failed", "10 failed", "build failed")
	// while ignoring "0 failed".
	remaining := text
	for {
		idx := strings.Index(remaining, "failed")
		if idx == -1 {
			break
		}

		// Look backwards from idx for the preceding word / number
		j := idx - 1
		for j >= 0 && (remaining[j] == ' ' || remaining[j] == '\t') {
			j--
		}

		if j >= 0 && remaining[j] >= '0' && remaining[j] <= '9' {
			// Extract all consecutive digits backwards
			digitEnd := j + 1
			for j >= 0 && remaining[j] >= '0' && remaining[j] <= '9' {
				j--
			}
			digits := remaining[j+1 : digitEnd]
			if digits != "0" {
				return true // e.g. "1 failed", "10 failed"
			}
			// If digits == "0", this specific occurrence is "0 failed"
		} else {
			// Non-digit preceding "failed", e.g. "tests failed", "run failed", "build failed"
			return true
		}

		remaining = remaining[idx+len("failed"):]
	}

	return false
}

// detectTestSignals scans tool result text for test and compiler output.
// Operates directly on the immutable string using SIMD strings.Contains without heap allocations.
// Both pass and fail are evaluated independently to prevent short-circuit false negatives on mixed runs.
func detectTestSignals(text string) (pass bool, fail bool) {
	fail = isFailingTestOutput(text)
	for _, sig := range testPassSignatures {
		if strings.Contains(text, sig) {
			pass = true
			break
		}
	}
	return pass, fail
}

// isErrorText checks if a message text contains known error signatures or failure indicators.
func isErrorText(text string, signatures []string) bool {
	if strings.Contains(text, `"success":false`) || strings.Contains(text, `"success": false`) {
		return true
	}
	if len(signatures) == 0 {
		signatures = defaultAgentErrorSignatures
	}
	for _, sig := range signatures {
		if strings.Contains(text, sig) {
			return true
		}
	}
	return false
}

// scanTrailingMessages inspects the last N messages in the conversation history
// to detect: (a) consecutive trailing error turns, (b) successful tool progress,
// (c) write-specific tool progress (e.g. file writes, terminal executions), and
// (d) test/debug progress (running tests, compiler errors, reading test files).
func (c *RequestClassifier) scanTrailingMessages(messages []interface{}) (historyErrors int, hasToolProgress bool, hasWriteProgress bool, hasTestPass bool, hasTestFail bool) {
	signatures := c.GetErrorSignatures()
	writeTools := c.GetKickstartWriteTools()

	// Scan backwards from the end, up to 8 messages to capture assistant calls + tool responses
	start := len(messages) - 8
	if start < 0 {
		start = 0
	}

	// First pass: collect call IDs for write/execute tool invocations in assistant turns.
	// Hybrid stack/heap: zero allocs for ≤8 IDs (common case), transparent heap spillover on overflow.
	var stackIDs [8]string
	writeCallIDs := stackIDs[:0]
	hasAnyWriteCall := false

	for i := start; i < len(messages); i++ {
		msgMap, ok := messages[i].(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)

		if role == "assistant" {
			// Check OpenAI-style tool_calls
			if toolCalls, ok := msgMap["tool_calls"].([]interface{}); ok {
				for _, tc := range toolCalls {
					tcMap, ok := tc.(map[string]interface{})
					if !ok {
						continue
					}
					id, _ := tcMap["id"].(string)
					fnName := ""
					if fnMap, ok := tcMap["function"].(map[string]interface{}); ok {
						fnName, _ = fnMap["name"].(string)
					}
					if fnName == "" {
						fnName, _ = tcMap["name"].(string)
					}
					if writeTools[strings.ToLower(strings.TrimSpace(fnName))] {
						if id != "" {
							writeCallIDs = append(writeCallIDs, id)
						}
						hasAnyWriteCall = true
					}
				}
			}
			// Check legacy OpenAI function_call
			if fnCall, ok := msgMap["function_call"].(map[string]interface{}); ok {
				fnName, _ := fnCall["name"].(string)
				if writeTools[strings.ToLower(strings.TrimSpace(fnName))] {
					hasAnyWriteCall = true
				}
			}
			// Check Anthropic-style tool_use content blocks
			if parts, ok := msgMap["content"].([]interface{}); ok {
				for _, part := range parts {
					partMap, ok := part.(map[string]interface{})
					if !ok {
						continue
					}
					partType, _ := partMap["type"].(string)
					if partType == "tool_use" {
						id, _ := partMap["id"].(string)
						name, _ := partMap["name"].(string)
						if writeTools[strings.ToLower(strings.TrimSpace(name))] {
							if id != "" {
								writeCallIDs = append(writeCallIDs, id)
							}
							hasAnyWriteCall = true
						}
					}
				}
			}
			// Check Cline-style XML tool calls embedded in text content
			// Cline models emit <write_to_file>, <replace_in_file>, etc. as XML tags in prose
			if textContent, ok := msgMap["content"].(string); ok && len(writeTools) > 0 {
				lowerText := strings.ToLower(textContent)
				for toolName := range writeTools {
					if strings.Contains(lowerText, "<"+toolName+">") {
						hasAnyWriteCall = true
						break
					}
				}
			}
		}
	}

	// Second pass: detect tool progress and write-specific progress on tool results
	for i := start; i < len(messages); i++ {
		msgMap, ok := messages[i].(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)

		// Check OpenAI-style role: tool
		if role == "tool" {
			text := extractAllTextFromContent(msgMap["content"])
			if !isErrorText(text, signatures) {
				hasToolProgress = true
				toolCallID, _ := msgMap["tool_call_id"].(string)
				if toolCallID != "" && writeCallIDsContains(writeCallIDs, toolCallID) {
					hasWriteProgress = true
				} else if toolCallID == "" && hasAnyWriteCall {
					hasWriteProgress = true
				}
			}
			// Detect test signals independently (success OR error)
			p, f := detectTestSignals(text)
			if p {
				hasTestPass = true
			}
			if f {
				hasTestFail = true
			}
		}

		// Check Anthropic-style multi-part content with tool_result type
		if parts, ok := msgMap["content"].([]interface{}); ok {
			for _, part := range parts {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					continue
				}
				partType, _ := partMap["type"].(string)
				if partType == "tool_result" {
					text := extractAllTextFromContent(partMap["content"])
					isError, _ := partMap["is_error"].(bool)
					if !isError {
						if !isErrorText(text, signatures) {
							hasToolProgress = true
							toolUseID, _ := partMap["tool_use_id"].(string)
							if toolUseID != "" && writeCallIDsContains(writeCallIDs, toolUseID) {
								hasWriteProgress = true
							} else if toolUseID == "" && hasAnyWriteCall {
								hasWriteProgress = true
							}
						}
					}
					p, f := detectTestSignals(text)
					if p {
						hasTestPass = true
					}
					if f {
						hasTestFail = true
					}
				}
			}
		}

		// Check Cline-style: user message following an assistant with XML tool calls.
		// In Cline's protocol, every user message after a tool call IS the tool result.
		if role == "user" && hasAnyWriteCall {
			text := extractAllTextFromContent(msgMap["content"])
			if !isErrorText(text, signatures) {
				hasToolProgress = true
				hasWriteProgress = true
			}
			p, f := detectTestSignals(text)
			if p {
				hasTestPass = true
			}
			if f {
				hasTestFail = true
			}
		}
	}

	// Third pass: count consecutive trailing errors (from the end, backwards)
	for i := len(messages) - 1; i >= start; i-- {
		msgMap, ok := messages[i].(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)

		// User and tool role messages carry error feedback from agent clients
		if role != "user" && role != "tool" {
			continue
		}

		text := extractAllTextFromContent(msgMap["content"])
		if isErrorText(text, signatures) {
			historyErrors++
		} else {
			// Break consecutive error chain on a non-error message
			break
		}
	}

	return historyErrors, hasToolProgress, hasWriteProgress, hasTestPass, hasTestFail
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

// writeCallIDsContains checks if a tool call ID exists in the write call ID slice.
// Linear scan over ≤8 items on the stack is faster than map hashing.
func writeCallIDsContains(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
