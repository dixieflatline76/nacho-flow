package router

import (
	"bytes"
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
	c.mu.RLock()
	if c.estimator != nil {
		defer c.mu.RUnlock()
		return c.estimator
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
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
	Role         string              `json:"role"`
	Content      classifyContent     `json:"content"`
	ToolCalls    []classifyToolCall  `json:"tool_calls,omitempty"`
	ToolCallID   string              `json:"tool_call_id,omitempty"`
	FunctionCall *classifyToolCallFn `json:"function_call,omitempty"`
}

func (m *classifyMessage) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	type rawMsg classifyMessage
	return json.Unmarshal(data, (*rawMsg)(m))
}

func (m *classifyMessage) Text() string {
	if m.Content.Text != "" {
		return m.Content.Text
	}
	if len(m.Content.Parts) == 0 {
		return ""
	}
	if len(m.Content.Parts) == 1 {
		return m.Content.Parts[0].Text
	}
	var sb strings.Builder
	for _, p := range m.Content.Parts {
		if p.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
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
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

func (p *classifyContentPart) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	type rawPart classifyContentPart
	if err := json.Unmarshal(data, (*rawPart)(p)); err != nil {
		return err
	}
	if len(p.Content) > 0 {
		trimmed := bytes.TrimSpace(p.Content)
		if len(trimmed) > 0 {
			if trimmed[0] == '"' {
				var s string
				if err := json.Unmarshal(trimmed, &s); err == nil {
					if p.Text == "" {
						p.Text = s
					} else {
						p.Text = p.Text + " " + s
					}
				}
			} else if trimmed[0] == '[' {
				var blocks []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if err := json.Unmarshal(trimmed, &blocks); err == nil {
					var sb strings.Builder
					for _, b := range blocks {
						if b.Text != "" {
							if sb.Len() > 0 {
								sb.WriteString(" ")
							}
							sb.WriteString(b.Text)
						}
					}
					if sb.Len() > 0 {
						if p.Text == "" {
							p.Text = sb.String()
						} else {
							p.Text = p.Text + " " + sb.String()
						}
					}
				}
			}
		}
		p.Content = nil
	}
	return nil
}

type classifyToolCall struct {
	ID        string              `json:"id"`
	Type      string              `json:"type,omitempty"`
	Name      string              `json:"name,omitempty"`
	Function  *classifyToolCallFn `json:"function,omitempty"`
	Arguments json.RawMessage     `json:"arguments,omitempty"`
}

type classifyToolCallFn struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
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
	reqCtx.HistoryErrors, reqCtx.HasToolProgress, reqCtx.HasWriteProgress, reqCtx.HasShellWrite, reqCtx.HasTestPass, reqCtx.HasTestFail = c.scanTrailingTyped(payload.Messages)
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

func (c *RequestClassifier) scanTrailingTyped(messages []classifyMessage) (historyErrors int, hasToolProgress bool, hasWriteProgress bool, hasShellWrite bool, hasTestPass bool, hasTestFail bool) {
	signatures := c.GetErrorSignatures()
	writeTools := c.GetKickstartWriteTools()

	start := len(messages) - 8
	if start < 0 {
		start = 0
	}

	var stackIDs [8]string
	writeCallIDs := stackIDs[:0]
	hasAnyWriteCall := false

	var stackShellIDs [8]string
	shellWriteCallIDs := stackShellIDs[:0]
	hasAnyShellWriteCall := false

	// Pass 1: assistant tool calls
	for i := start; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				fnName := tc.Name
				if tc.Function != nil && tc.Function.Name != "" {
					fnName = tc.Function.Name
				}
				fnLower := strings.ToLower(strings.TrimSpace(fnName))
				if writeTools != nil && writeTools[fnLower] {
					if tc.ID != "" {
						writeCallIDs = append(writeCallIDs, tc.ID)
					}
					hasAnyWriteCall = true
				}
				if isShellTool(fnLower) {
					var rawArgs json.RawMessage
					if tc.Function != nil && len(tc.Function.Arguments) > 0 {
						rawArgs = tc.Function.Arguments
					} else if len(tc.Arguments) > 0 {
						rawArgs = tc.Arguments
					}
					if cmd := extractCommandFromRaw(rawArgs); cmd != "" && detectShellWrite(cmd) {
						if tc.ID != "" {
							shellWriteCallIDs = append(shellWriteCallIDs, tc.ID)
						}
						hasAnyShellWriteCall = true
					}
				}
			}
			if msg.FunctionCall != nil && msg.FunctionCall.Name != "" {
				fnLower := strings.ToLower(strings.TrimSpace(msg.FunctionCall.Name))
				if writeTools != nil && writeTools[fnLower] {
					hasAnyWriteCall = true
				}
				if isShellTool(fnLower) {
					if cmd := extractCommandFromRaw(msg.FunctionCall.Arguments); cmd != "" && detectShellWrite(cmd) {
						hasAnyShellWriteCall = true
					}
				}
			}
			for _, p := range msg.Content.Parts {
				if p.Type == "tool_use" {
					pLower := strings.ToLower(strings.TrimSpace(p.Name))
					if writeTools != nil && writeTools[pLower] {
						if p.ID != "" {
							writeCallIDs = append(writeCallIDs, p.ID)
						}
						hasAnyWriteCall = true
					}
					if isShellTool(pLower) {
						if cmd := extractCommandFromRaw(p.Input); cmd != "" && detectShellWrite(cmd) {
							if p.ID != "" {
								shellWriteCallIDs = append(shellWriteCallIDs, p.ID)
							}
							hasAnyShellWriteCall = true
						}
					}
				}
			}
			assistantText := msg.Text()
			if assistantText != "" {
				lowerText := strings.ToLower(assistantText)
				if len(writeTools) > 0 {
					for toolName := range writeTools {
						if containsXMLToolTag(lowerText, toolName) {
							hasAnyWriteCall = true
							break
						}
					}
				}
				for _, shellName := range []string{"execute_command", "bash", "terminal", "run_command"} {
					if containsXMLToolTag(lowerText, shellName) {
						if cmd := extractXMLCommand(assistantText); cmd != "" && detectShellWrite(cmd) {
							hasAnyShellWriteCall = true
							break
						}
					}
				}
			}
		}
	}

	// Pass 2: tool results
	for i := start; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "tool" {
			text := msg.Text()
			if !isErrorText(text, signatures) {
				hasToolProgress = true
				if msg.ToolCallID != "" && writeCallIDsContains(writeCallIDs, msg.ToolCallID) {
					hasWriteProgress = true
				} else if msg.ToolCallID == "" && hasAnyWriteCall {
					hasWriteProgress = true
				}
				if msg.ToolCallID != "" && writeCallIDsContains(shellWriteCallIDs, msg.ToolCallID) {
					hasShellWrite = true
				} else if msg.ToolCallID == "" && hasAnyShellWriteCall {
					hasShellWrite = true
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
						if part.ToolUseID != "" && writeCallIDsContains(shellWriteCallIDs, part.ToolUseID) {
							hasShellWrite = true
						} else if part.ToolUseID == "" && hasAnyShellWriteCall {
							hasShellWrite = true
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
		// Cline-style: user message following an assistant with XML tool calls.
		// In Cline's protocol, every user message after a tool call IS the tool result.
		if msg.Role == "user" && (hasAnyWriteCall || hasAnyShellWriteCall) {
			text := msg.Text()
			if !isErrorText(text, signatures) {
				hasToolProgress = true
				if hasAnyWriteCall {
					hasWriteProgress = true
				}
				if hasAnyShellWriteCall {
					hasShellWrite = true
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

	// Pass 3: trailing consecutive error turns (from the end, backwards)
	for i := len(messages) - 1; i >= start; i-- {
		msg := messages[i]
		if msg.Role != "user" && msg.Role != "tool" {
			// Skip assistant or other turns in backwards scan
			continue
		}

		text := msg.Text()
		isErr := isErrorText(text, signatures)
		if !isErr {
			for _, part := range msg.Content.Parts {
				if part.Type == "tool_result" && part.IsError {
					isErr = true
					break
				}
			}
		}

		if isErr {
			historyErrors++
		} else {
			// Break consecutive error chain on a non-error message
			break
		}
	}

	return historyErrors, hasToolProgress, hasWriteProgress, hasShellWrite, hasTestPass, hasTestFail
}

// containsXMLToolTag checks whether text contains an XML tool tag (e.g. <write_to_file> or <write_to_file )
func containsXMLToolTag(text, toolName string) bool {
	prefix := "<" + toolName
	idx := 0
	for {
		pos := strings.Index(text[idx:], prefix)
		if pos == -1 {
			return false
		}
		matchStart := idx + pos
		after := matchStart + len(prefix)
		if after < len(text) {
			b := text[after]
			if b == '>' || b == ' ' || b == '\n' || b == '\r' || b == '\t' || b == '/' {
				return true
			}
		}
		idx = matchStart + 1
	}
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

// isShellTool checks if the given tool name represents a shell/terminal execution tool.
func isShellTool(name string) bool {
	switch name {
	case "bash", "execute_command", "run_command", "terminal", "run_shell_command",
		"execute_bash", "powershell", "pwsh", "cmd", "sh", "zsh":
		return true
	default:
		return false
	}
}

// extractCommandFromRaw pulls the shell command string from tool call argument payloads.
func extractCommandFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var unquoted string
		if err := json.Unmarshal(trimmed, &unquoted); err == nil {
			trimmed = bytes.TrimSpace([]byte(unquoted))
		}
	}
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ""
	}
	var fastArgs struct {
		Command     string `json:"command"`
		CommandLine string `json:"CommandLine"`
		Cmd         string `json:"cmd"`
		Script      string `json:"script"`
	}
	if err := json.Unmarshal(trimmed, &fastArgs); err == nil {
		if fastArgs.Command != "" {
			return fastArgs.Command
		}
		if fastArgs.CommandLine != "" {
			return fastArgs.CommandLine
		}
		if fastArgs.Cmd != "" {
			return fastArgs.Cmd
		}
		if fastArgs.Script != "" {
			return fastArgs.Script
		}
	}
	return ""
}

// extractXMLCommand extracts shell commands from Cline-style XML tool blocks.
func extractXMLCommand(text string) string {
	for _, tag := range []string{"command", "CommandLine", "cmd", "script"} {
		openTag := "<" + tag + ">"
		closeTag := "</" + tag + ">"
		start := strings.Index(text, openTag)
		if start != -1 {
			end := strings.Index(text[start+len(openTag):], closeTag)
			if end != -1 {
				return strings.TrimSpace(text[start+len(openTag) : start+len(openTag)+end])
			}
		}
	}
	return ""
}

// detectShellWrite inspects shell command lines for file-writing operations using zero-alloc string parsing.
func detectShellWrite(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return false
	}

	// 1. Redirection checks: > and >> (ignoring comparisons/scripts inside quotes)
	if strings.Contains(trimmed, ">") {
		var inSingleQuote, inDoubleQuote bool
		for i := 0; i < len(trimmed); i++ {
			ch := trimmed[i]
			if ch == '\\' && i+1 < len(trimmed) {
				i++
				continue
			}
			if ch == '\'' && !inDoubleQuote {
				inSingleQuote = !inSingleQuote
				continue
			}
			if ch == '"' && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
				continue
			}
			if inSingleQuote || inDoubleQuote {
				continue
			}
			if ch == '>' {
				// Guard: process substitution <( or redirection descriptor &>
				if i > 0 && (trimmed[i-1] == '&' || trimmed[i-1] == '<') {
					continue
				}
				// Guard: >&2 (stdout to stderr)
				if i+1 < len(trimmed) && trimmed[i+1] == '&' {
					continue
				}
				// Skip second '>' if '>>'
				targetIdx := i + 1
				if targetIdx < len(trimmed) && trimmed[targetIdx] == '>' {
					targetIdx++
				}
				// Guard: >= comparison operator
				if targetIdx < len(trimmed) && trimmed[targetIdx] == '=' {
					continue
				}
				target := strings.TrimSpace(trimmed[targetIdx:])
				if endIdx := strings.IndexAny(target, " \t\r\n|;&"); endIdx != -1 {
					target = target[:endIdx]
				}
				target = strings.TrimSpace(target)
				if isNullTarget(target) {
					continue
				}
				if target != "" {
					return true
				}
			}
		}
	}

	lower := strings.ToLower(trimmed)

	// 2. Pipe to file-writing tools
	if strings.Contains(lower, "| tee") ||
		strings.Contains(lower, "|tee") ||
		strings.Contains(lower, "| dd of=") ||
		strings.Contains(lower, "| out-file") ||
		strings.Contains(lower, "| set-content") ||
		strings.Contains(lower, "| add-content") {
		return true
	}

	// 3. Heredocs combined with file writing
	if strings.Contains(trimmed, "<<") {
		if strings.Contains(trimmed, ">") || strings.Contains(lower, "| tee") {
			return true
		}
	}

	// 4. File-modifying CLI tools
	if containsCommandWord(lower, "sed") && strings.Contains(lower, "-i") {
		return true
	}

	modifyingTools := [...]string{
		"patch", "touch", "mkdir", "rm", "rmdir", "cp", "mv",
		"truncate", "install", "unzip", "gunzip",
		"copy", "move", "del", "erase", "ren", "rename", "md", "rd",
		"new-item", "copy-item", "move-item", "remove-item", "set-content", "add-content", "out-file",
	}
	for _, tool := range modifyingTools {
		if containsCommandWord(lower, tool) {
			return true
		}
	}

	// tar extraction
	if containsCommandWord(lower, "tar") && (strings.Contains(lower, "-x") || strings.Contains(lower, " x")) {
		return true
	}

	// git file-modifying commands (narrowed to concrete file writes; avoids merge/rebase false positives)
	if containsCommandWord(lower, "git") {
		if strings.Contains(lower, "checkout --") ||
			strings.Contains(lower, "checkout .") ||
			strings.Contains(lower, "restore ") ||
			strings.Contains(lower, "restore\t") ||
			strings.Contains(lower, "apply ") ||
			strings.Contains(lower, "apply\t") {
			return true
		}
	}

	// Project and package scaffolding commands that generate/modify configuration and source files
	if (containsCommandWord(lower, "go") && (strings.Contains(lower, "mod init") || strings.Contains(lower, "mod tidy"))) ||
		(containsCommandWord(lower, "npm") && (strings.Contains(lower, "init") || strings.Contains(lower, "create "))) ||
		(containsCommandWord(lower, "cargo") && (strings.Contains(lower, "new ") || strings.Contains(lower, "init"))) {
		return true
	}

	return false
}

func isNullTarget(target string) bool {
	lower := strings.ToLower(target)
	switch lower {
	case "/dev/null", "/dev/zero", "nul", "$null", "&1", "&2":
		return true
	default:
		return false
	}
}

func containsCommandWord(s, word string) bool {
	idx := 0
	for {
		pos := strings.Index(s[idx:], word)
		if pos == -1 {
			return false
		}
		actualPos := idx + pos
		prefixOK := false
		if actualPos == 0 {
			prefixOK = true
		} else {
			prev := s[actualPos-1]
			if prev == ' ' || prev == '\t' || prev == ';' || prev == '|' || prev == '&' || prev == '`' || prev == '(' || prev == '\n' {
				prefixOK = true
			}
		}

		afterPos := actualPos + len(word)
		suffixOK := false
		if afterPos >= len(s) {
			suffixOK = true
		} else {
			next := s[afterPos]
			if next == ' ' || next == '\t' || next == ';' || next == '|' || next == '&' || next == '`' || next == ')' || next == '\n' || next == '\r' {
				suffixOK = true
			}
		}

		if prefixOK && suffixOK {
			return true
		}
		idx = actualPos + 1
	}
}
