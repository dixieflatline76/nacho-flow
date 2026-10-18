package router

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/router/shield"
)

type RequestClassifier struct {
	estimator        atomic.Pointer[TokenEstimator]
	errorSignatures  atomic.Pointer[[]string]
	writeToolsLookup atomic.Pointer[map[string]bool]
	initMu           sync.Mutex
}

// defaultAgentErrorSignatures are fallback error patterns loaded from agent profiles in agentregistry.
var defaultAgentErrorSignatures = func() []string {
	if reg := agentregistry.DefaultRegistry(); reg != nil {
		if list := reg.ErrorSignaturesList(); len(list) > 0 {
			return list
		}
	}
	return []string{
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
}()

// NewClassifier initializes a default RequestClassifier with an adaptive TokenEstimator.
func NewClassifier() contract.Classifier {
	return NewClassifierWithEstimator(NewTokenEstimator())
}

// NewClassifierWithEstimator initializes a RequestClassifier with a specific TokenEstimator.
func NewClassifierWithEstimator(e *TokenEstimator) contract.Classifier {
	if e == nil {
		e = NewTokenEstimator()
	}
	c := &RequestClassifier{}
	c.estimator.Store(e)
	return c
}

// SetErrorSignatures configures custom error patterns from config.yaml.
func (c *RequestClassifier) SetErrorSignatures(signatures []string) {
	if len(signatures) == 0 {
		c.errorSignatures.Store(nil)
		return
	}
	copied := make([]string, len(signatures))
	copy(copied, signatures)
	c.errorSignatures.Store(&copied)
}

// GetErrorSignatures returns active error signatures or default fallback.
// Completely lock-free using RCU atomic pointer load.
func (c *RequestClassifier) GetErrorSignatures() []string {
	ptr := c.errorSignatures.Load()
	if ptr == nil || len(*ptr) == 0 {
		return defaultAgentErrorSignatures
	}
	return *ptr
}

// SetKickstartWriteTools configures custom write-tool names from config.yaml.
func (c *RequestClassifier) SetKickstartWriteTools(tools []string) {
	if tools == nil {
		c.writeToolsLookup.Store(nil)
		return
	}
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
// Completely lock-free on the hot path via atomic pointer load.
func (c *RequestClassifier) GetEstimator() *TokenEstimator {
	if est := c.estimator.Load(); est != nil {
		return est
	}
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if est := c.estimator.Load(); est != nil {
		return est
	}
	est := NewTokenEstimator()
	c.estimator.Store(est)
	return est
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
			switch trimmed[0] {
			case '"':
				var s string
				if err := json.Unmarshal(trimmed, &s); err == nil {
					if p.Text == "" {
						p.Text = s
					} else {
						p.Text = p.Text + " " + s
					}
				}
			case '[':
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

	c.inspectToolSchemas(payload.Tools, &reqCtx)

	if len(payload.Messages) == 0 {
		return reqCtx, nil
	}
	reqCtx.MessageCount = len(payload.Messages)
	reqCtx.RootPromptHash = extractRootPromptHash(payload.Messages)

	latestUserPrompt, latestUserMsg, hasNonEmptyContent := scanMessagesForPrompts(payload.Messages, &reqCtx)

	reqCtx.HistoryErrors, reqCtx.HasToolProgress, reqCtx.HasWriteProgress, reqCtx.HasShellWrite, reqCtx.HasTestPass, reqCtx.HasTestFail, reqCtx.HasActiveRepair = c.scanTrailingTyped(payload.Messages)
	reqCtx.HasTestProgress = reqCtx.HasTestPass && !reqCtx.HasTestFail

	if hasNonEmptyContent || reqCtx.HasTools {
		estimator := c.GetEstimator()
		reqCtx.Tokens = estimator.Estimate(len(body))
	}
	reqCtx.Prompt = latestUserPrompt
	reqCtx.CleanPrompt = latestUserPrompt
	if latestUserPrompt != "" {
		reqCtx.PromptHash = HashPrompt(latestUserPrompt)
	} else if latestUserMsg != nil {
		reqCtx.PromptHash = hashMessageContent(latestUserMsg)
	}
	if reqCtx.RootPromptHash == 0 {
		reqCtx.RootPromptHash = reqCtx.PromptHash
	}

	applyPromptDirectives(latestUserPrompt, &reqCtx)
	reqCtx.Keywords = resolvePromptKeywords(reqCtx.CleanPrompt, payload.Messages)

	return reqCtx, nil
}

func (c *RequestClassifier) inspectToolSchemas(tools []classifyToolSchema, reqCtx *contract.RequestContext) {
	if len(tools) == 0 {
		return
	}
	reqCtx.HasTools = true
	writeLookup := c.GetKickstartWriteTools()

	for _, t := range tools {
		fnName := t.Name
		if t.Function != nil && t.Function.Name != "" {
			fnName = t.Function.Name
		}

		lower := strings.ToLower(fnName)
		if reqCtx.InteractiveTool == "" && (agentregistry.DefaultRegistry().IsInteractiveTool(lower) || lower == "interactive_input") {
			reqCtx.InteractiveTool = fnName
		}

		if !reqCtx.HasWriteCapability {
			if writeLookup != nil {
				if writeLookup[strings.ToLower(strings.TrimSpace(fnName))] {
					reqCtx.HasWriteCapability = true
				}
			} else if IsBuiltinWriteTool(fnName) {
				reqCtx.HasWriteCapability = true
			}
		}
	}
}

func extractRootPromptHash(messages []classifyMessage) uint64 {
	if len(messages) == 0 {
		return 0
	}
	if messages[0].Role == "user" {
		return hashMessageContent(&messages[0])
	}
	if len(messages) > 1 && messages[1].Role == "user" {
		return hashMessageContent(&messages[1])
	}
	for i := 0; i < len(messages) && i < 4; i++ {
		if messages[i].Role == "user" {
			return hashMessageContent(&messages[i])
		}
	}
	return hashMessageContent(&messages[0])
}

func scanMessagesForPrompts(messages []classifyMessage, reqCtx *contract.RequestContext) (latestUserPrompt string, latestUserMsg *classifyMessage, hasNonEmptyContent bool) {
	for i := range messages {
		msg := &messages[i]
		if msg.Content.Text != "" {
			hasNonEmptyContent = true
			if msg.Role == "user" {
				latestUserPrompt = msg.Content.Text
				latestUserMsg = msg
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
					if msg.Role == "user" {
						latestUserPrompt = part.Text
						latestUserMsg = msg
					}
				}
			}
		}
	}
	return latestUserPrompt, latestUserMsg, hasNonEmptyContent
}

func applyPromptDirectives(latestUserPrompt string, reqCtx *contract.RequestContext) {
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
}

func resolvePromptKeywords(cleanPrompt string, messages []classifyMessage) []string {
	keywordSource := cleanPrompt
	if keywordSource == "" {
		var fallbackText strings.Builder
		for i := range messages {
			if t := messages[i].Text(); t != "" {
				if fallbackText.Len() > 0 {
					fallbackText.WriteString(" ")
				}
				fallbackText.WriteString(t)
			}
		}
		keywordSource = fallbackText.String()
	}
	return extractKeywords(keywordSource)
}

func hashMessageContent(msg *classifyMessage) uint64 {
	if msg == nil {
		return 0
	}
	if msg.Content.Text != "" {
		return HashPrompt(msg.Content.Text)
	}
	if len(msg.Content.Parts) == 0 {
		return 0
	}
	if len(msg.Content.Parts) == 1 {
		return HashPrompt(msg.Content.Parts[0].Text)
	}
	var h uint64 = fnvOffset64
	hasText := false
	for _, p := range msg.Content.Parts {
		if p.Text != "" {
			if hasText {
				h ^= uint64(' ')
				h *= fnvPrime64
			}
			hasText = true
			for i := 0; i < len(p.Text); i++ {
				h ^= uint64(p.Text[i])
				h *= fnvPrime64
			}
		}
	}
	if !hasText {
		return 0
	}
	return h
}

type trailingScanner struct {
	signatures             []string
	writeTools             map[string]bool
	start                  int
	lastAssistantIdx       int
	stackWriteIDs          [8]string
	writeCallIDs           []string
	hasAnyWriteCall        bool
	stackShellIDs          [8]string
	shellWriteCallIDs      []string
	hasAnyShellWriteCall   bool
	assistantXMLWrite      [8]bool
	assistantXMLShellWrite [8]bool
	lastWriteIdx           int
	lastTestPassIdx        int
	lastTestFailIdx        int
	immediateWrite         bool
	hasToolProgress        bool
	hasWriteProgress       bool
	hasShellWrite          bool
}

func (ts *trailingScanner) isWriteTool(fnLower string) bool {
	if ts.writeTools != nil {
		return ts.writeTools[fnLower]
	}
	return IsBuiltinWriteTool(fnLower)
}

func (ts *trailingScanner) recordStructuredCall(fnName, callID string, rawArgs json.RawMessage, checkBuiltinWhenCustomNil bool) {
	fnLower := strings.ToLower(strings.TrimSpace(fnName))
	if fnLower == "" {
		return
	}
	isWrite := false
	if checkBuiltinWhenCustomNil {
		isWrite = ts.isWriteTool(fnLower)
	} else if ts.writeTools != nil {
		isWrite = ts.writeTools[fnLower]
	}
	if isWrite {
		if callID != "" {
			ts.writeCallIDs = append(ts.writeCallIDs, callID)
		}
		ts.hasAnyWriteCall = true
	}
	if isShellTool(fnLower) {
		if cmd := extractCommandFromRaw(rawArgs); cmd != "" && detectShellWrite(cmd) {
			if callID != "" {
				ts.shellWriteCallIDs = append(ts.shellWriteCallIDs, callID)
			}
			ts.hasAnyShellWriteCall = true
		}
	}
}

func (ts *trailingScanner) scanAssistantXML(msgIdx int, text string) {
	if text == "" {
		return
	}
	lowerText := strings.ToLower(text)
	relIdx := msgIdx - ts.start

	if len(ts.writeTools) > 0 {
		for toolName := range ts.writeTools {
			if containsXMLToolTag(lowerText, toolName) {
				ts.hasAnyWriteCall = true
				if relIdx >= 0 && relIdx < 8 {
					ts.assistantXMLWrite[relIdx] = true
				}
				break
			}
		}
	} else if reg := agentregistry.DefaultRegistry(); reg != nil {
		for _, toolName := range reg.WriteToolsList() {
			if containsXMLToolTag(lowerText, strings.ToLower(toolName)) {
				ts.hasAnyWriteCall = true
				if relIdx >= 0 && relIdx < 8 {
					ts.assistantXMLWrite[relIdx] = true
				}
				break
			}
		}
	}

	for _, shellName := range []string{"execute_command", "bash", "terminal", "run_command"} {
		if containsXMLToolTag(lowerText, shellName) {
			if cmd := extractXMLCommand(text); cmd != "" && detectShellWrite(cmd) {
				ts.hasAnyShellWriteCall = true
				if relIdx >= 0 && relIdx < 8 {
					ts.assistantXMLShellWrite[relIdx] = true
				}
				break
			}
		}
	}
}

func (ts *trailingScanner) scanAssistantMessage(msgIdx int, msg *classifyMessage) {
	if msg.Role != "assistant" {
		return
	}
	for _, tc := range msg.ToolCalls {
		fnName := tc.Name
		if tc.Function != nil && tc.Function.Name != "" {
			fnName = tc.Function.Name
		}
		var rawArgs json.RawMessage
		if tc.Function != nil && len(tc.Function.Arguments) > 0 {
			rawArgs = tc.Function.Arguments
		} else if len(tc.Arguments) > 0 {
			rawArgs = tc.Arguments
		}
		ts.recordStructuredCall(fnName, tc.ID, rawArgs, true)
	}
	if msg.FunctionCall != nil && msg.FunctionCall.Name != "" {
		ts.recordStructuredCall(msg.FunctionCall.Name, "", msg.FunctionCall.Arguments, false)
	}
	for _, p := range msg.Content.Parts {
		if p.Type == "tool_use" {
			ts.recordStructuredCall(p.Name, p.ID, p.Input, true)
		}
	}
	ts.scanAssistantXML(msgIdx, msg.Text())
}

func (ts *trailingScanner) recordTestSignals(msgIdx int, text string) {
	p, f := detectTestSignals(text)
	if p {
		ts.lastTestPassIdx = msgIdx
	}
	if f {
		ts.lastTestFailIdx = msgIdx
	}
}

func (ts *trailingScanner) recordStructuredResult(msgIdx int, callID, text string) (hadWrite bool) {
	if !isErrorText(text, ts.signatures) {
		ts.hasToolProgress = true
		if (callID != "" && writeCallIDsContains(ts.writeCallIDs, callID)) || (callID == "" && ts.hasAnyWriteCall) {
			ts.hasWriteProgress = true
			hadWrite = true
		}
		if (callID != "" && writeCallIDsContains(ts.shellWriteCallIDs, callID)) || (callID == "" && ts.hasAnyShellWriteCall) {
			ts.hasShellWrite = true
			hadWrite = true
		}
	}
	ts.recordTestSignals(msgIdx, text)
	return hadWrite
}

func (ts *trailingScanner) scanXMLUserResult(msgIdx int, msg *classifyMessage) (hadWrite bool) {
	if msg.Role != "user" {
		return false
	}
	prevIdx := msgIdx - 1
	prevHadXMLWrite := false
	prevHadXMLShellWrite := false
	if prevIdx >= ts.start && prevIdx-ts.start < 8 {
		prevHadXMLWrite = ts.assistantXMLWrite[prevIdx-ts.start]
		prevHadXMLShellWrite = ts.assistantXMLShellWrite[prevIdx-ts.start]
	}
	if !prevHadXMLWrite && !prevHadXMLShellWrite {
		return false
	}

	text := msg.Text()
	if !isErrorText(text, ts.signatures) {
		ts.hasToolProgress = true
		if prevHadXMLWrite {
			ts.hasWriteProgress = true
			hadWrite = true
		}
		if prevHadXMLShellWrite {
			ts.hasShellWrite = true
			hadWrite = true
		}
	}
	ts.recordTestSignals(msgIdx, text)
	return hadWrite
}

func (ts *trailingScanner) scanResultMessage(msgIdx int, msg *classifyMessage) {
	msgHadWrite := false
	hasStructuredToolResults := false

	if msg.Role == "tool" {
		hasStructuredToolResults = true
		if ts.recordStructuredResult(msgIdx, msg.ToolCallID, msg.Text()) {
			msgHadWrite = true
		}
	}

	for _, part := range msg.Content.Parts {
		if part.Type == "tool_result" {
			hasStructuredToolResults = true
			if part.IsError {
				ts.recordTestSignals(msgIdx, part.Text)
			} else {
				if ts.recordStructuredResult(msgIdx, part.ToolUseID, part.Text) {
					msgHadWrite = true
				}
			}
		}
	}

	if !hasStructuredToolResults {
		if ts.scanXMLUserResult(msgIdx, msg) {
			msgHadWrite = true
		}
	}

	if msgHadWrite {
		ts.lastWriteIdx = msgIdx
		if msgIdx > ts.lastAssistantIdx {
			ts.immediateWrite = true
		}
	}
}

func countTrailingErrors(messages []classifyMessage, start int, signatures []string) int {
	errors := 0
	for i := len(messages) - 1; i >= start; i-- {
		msg := &messages[i]
		if msg.Role != "user" && msg.Role != "tool" {
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
			errors++
		} else {
			break
		}
	}
	return errors
}

func (c *RequestClassifier) scanTrailingTyped(messages []classifyMessage) (historyErrors int, hasToolProgress bool, hasWriteProgress bool, hasShellWrite bool, hasTestPass bool, hasTestFail bool, hasActiveRepair bool) {
	start := len(messages) - 8
	if start < 0 {
		start = 0
	}

	lastAssistantIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			lastAssistantIdx = i
			break
		}
	}

	ts := trailingScanner{
		signatures:       c.GetErrorSignatures(),
		writeTools:       c.GetKickstartWriteTools(),
		start:            start,
		lastAssistantIdx: lastAssistantIdx,
		lastWriteIdx:     -1,
		lastTestPassIdx:  -1,
		lastTestFailIdx:  -1,
	}
	ts.writeCallIDs = ts.stackWriteIDs[:0]
	ts.shellWriteCallIDs = ts.stackShellIDs[:0]

	for i := ts.start; i < len(messages); i++ {
		ts.scanAssistantMessage(i, &messages[i])
	}

	for i := ts.start; i < len(messages); i++ {
		ts.scanResultMessage(i, &messages[i])
	}

	hasTestPass = ts.lastTestPassIdx != -1 && ts.lastTestPassIdx >= ts.lastTestFailIdx
	hasTestFail = ts.lastTestFailIdx != -1 && ts.lastTestFailIdx >= ts.lastTestPassIdx

	if hasTestFail && ts.lastWriteIdx > ts.lastTestFailIdx && ts.immediateWrite {
		hasActiveRepair = true
	}

	historyErrors = countTrailingErrors(messages, ts.start, ts.signatures)

	return historyErrors, ts.hasToolProgress, ts.hasWriteProgress, ts.hasShellWrite, hasTestPass, hasTestFail, hasActiveRepair
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
		if agentregistry.DefaultRegistry().IsInteractiveTool(lower) || lower == "interactive_input" {
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

// IsBuiltinWriteTool checks if the tool name represents a structured file writing or editing tool.
func IsBuiltinWriteTool(name string) bool {
	return agentregistry.DefaultRegistry().IsWriteTool(name)
}

// IsBuiltinCycleKillerExempt checks if the tool is registered as exempt from Cycle Killer repetition detection.
func IsBuiltinCycleKillerExempt(name string) bool {
	return agentregistry.DefaultRegistry().IsCycleKillerExempt(name)
}

// IsBuiltinReadTool checks if the tool name represents a known file reading tool.
func IsBuiltinReadTool(name string) bool {
	return agentregistry.DefaultRegistry().IsFileReadTool(name)
}

// ClassifyToolCategory maps a tool name to its cycle-breaker enforcement lane.
func ClassifyToolCategory(name string) shield.ToolActivityCategory {
	if IsBuiltinWriteTool(name) || IsBuiltinCycleKillerExempt(name) {
		return shield.ToolCategoryFileWrite
	}
	if IsBuiltinReadTool(name) {
		return shield.ToolCategoryFileRead
	}
	return shield.ToolCategoryCommand
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

// DetectShellWrite inspects shell command lines for file-writing operations using zero-alloc string parsing.
func DetectShellWrite(cmd string) bool {
	return agentregistry.DefaultRegistry().DetectShellWrite(cmd)
}

// detectShellWrite inspects shell command lines for file-writing operations using zero-alloc string parsing.
func detectShellWrite(cmd string) bool {
	return agentregistry.DefaultRegistry().DetectShellWrite(cmd)
}
