// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package agentregistry provides an in-memory, zero-allocation registry of AI agent
// tool capabilities, shell command write signatures, and model reasoning tags.
package agentregistry

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/dixieflatline76/nacho-flow/data"
	"github.com/dixieflatline76/nacho-flow/pkg/zeroalloc"
)

// Registry maintains an in-memory index of agent profiles, tool categories, and shell commands.
type Registry struct {
	mu                     sync.RWMutex
	writeTools             map[string]struct{}
	writeToolsList         []string
	fileReadTools          map[string]struct{}
	fileReadToolsList      []string
	allKnownTools          map[string]struct{}
	shellWriteCommands     map[string]struct{}
	shellWritePipes        []string
	contextualCommands     map[string][]string
	redirections           []string
	writeTagByteMarkers    [][]byte
	interactiveToolsLookup map[string]struct{}
	interactiveToolsList   []string
	modeHeuristicsList     []string
	questionHeuristicsList []string
	errorSignaturesList    []string
	errorSignaturesLower   []string
	manifest               Manifest
	reasoningCatalog       ReasoningCatalog
	tagReplacer            *strings.Replacer
	reasoningByteMarkers   [][]byte
	controlTokensBytes     [][]byte
	delimiterPrefixes      [][]byte
}

var (
	defaultRegistry *Registry
	once            sync.Once
)

// DefaultRegistry returns the process-wide default Registry loaded from embedded catalogs.
func DefaultRegistry() *Registry {
	once.Do(func() {
		reg, err := NewRegistryFromFS(data.CatalogFS)
		if err != nil {
			panic("agentregistry: failed to initialize embedded catalog: " + err.Error())
		}
		defaultRegistry = reg
	})
	return defaultRegistry
}

// NewRegistryFromFS loads agent profiles, shell catalog, and reasoning catalog from a filesystem.
func NewRegistryFromFS(sysFS fs.FS) (*Registry, error) {
	reg := &Registry{
		writeTools:         make(map[string]struct{}),
		fileReadTools:      make(map[string]struct{}),
		allKnownTools:      make(map[string]struct{}),
		shellWriteCommands: make(map[string]struct{}),
		contextualCommands: make(map[string][]string),
	}

	if err := reg.loadAgents(sysFS); err != nil {
		return nil, err
	}
	if err := reg.loadShell(sysFS); err != nil {
		return nil, err
	}
	if err := reg.loadReasoning(sysFS); err != nil {
		return nil, err
	}

	// Pre-sort and freeze immutable write tools slice at startup for lock-free zero-alloc reads
	tools := make([]string, 0, len(reg.writeTools))
	for tool := range reg.writeTools {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	reg.writeToolsList = tools

	reg.writeTagByteMarkers = compileWriteByteMarkers(reg.writeToolsList)

	return reg, nil
}

func addNormalized(target map[string]struct{}, items ...string) {
	for _, item := range items {
		if lower := strings.ToLower(strings.TrimSpace(item)); lower != "" {
			target[lower] = struct{}{}
		}
	}
}

func (r *Registry) loadAgents(sysFS fs.FS) error {
	entries, err := fs.ReadDir(sysFS, "agents")
	if err != nil {
		return fmt.Errorf("failed to read agents directory: %w", err)
	}

	interactiveToolsMap := make(map[string]struct{})
	modeHeuristicsMap := make(map[string]struct{})
	questionHeuristicsMap := make(map[string]struct{})
	errorSignaturesMap := make(map[string]struct{})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if entry.Name() == "manifest.json" {
			dataBytes, readErr := fs.ReadFile(sysFS, "agents/"+entry.Name())
			if readErr != nil {
				return fmt.Errorf("failed to read manifest: %w", readErr)
			}
			if err := json.Unmarshal(dataBytes, &r.manifest); err != nil {
				return fmt.Errorf("failed to parse manifest.json: %w", err)
			}
			continue
		}

		dataBytes, readErr := fs.ReadFile(sysFS, "agents/"+entry.Name())
		if readErr != nil {
			return fmt.Errorf("failed to read agent file %s: %w", entry.Name(), readErr)
		}

		var profile AgentProfile
		if err := json.Unmarshal(dataBytes, &profile); err != nil {
			return fmt.Errorf("failed to parse agent profile %s: %w", entry.Name(), err)
		}

		addNormalized(r.writeTools, profile.WriteTools...)
		addNormalized(r.fileReadTools, profile.FileReadTools...)
		addNormalized(r.allKnownTools, profile.WriteTools...)
		addNormalized(r.allKnownTools, profile.FileReadTools...)
		addNormalized(r.allKnownTools, profile.ReadTools...)
		addNormalized(r.allKnownTools, profile.CommandTools...)
		addNormalized(interactiveToolsMap, profile.InteractiveTools...)
		addNormalized(r.allKnownTools, profile.InteractiveTools...)
		if profile.ModeTool != "" {
			addNormalized(interactiveToolsMap, profile.ModeTool)
			addNormalized(r.allKnownTools, profile.ModeTool)
		}
		addNormalized(modeHeuristicsMap, profile.ModeHeuristics...)
		addNormalized(questionHeuristicsMap, profile.QuestionHeuristics...)
		for _, s := range profile.ErrorSignatures {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				errorSignaturesMap[trimmed] = struct{}{}
			}
		}
	}

	r.fileReadToolsList = mapToSortedSlice(r.fileReadTools)
	r.interactiveToolsLookup = interactiveToolsMap
	r.interactiveToolsList = mapToSortedSlice(interactiveToolsMap)
	r.modeHeuristicsList = mapToSortedSlice(modeHeuristicsMap)
	r.questionHeuristicsList = mapToSortedSlice(questionHeuristicsMap)
	r.errorSignaturesList = mapToSortedSlice(errorSignaturesMap)
	r.errorSignaturesLower = make([]string, len(r.errorSignaturesList))
	for i, s := range r.errorSignaturesList {
		r.errorSignaturesLower[i] = strings.ToLower(s)
	}

	return nil
}

func (r *Registry) loadShell(sysFS fs.FS) error {
	dataBytes, err := fs.ReadFile(sysFS, "shell.json")
	if err != nil {
		return fmt.Errorf("failed to read shell.json: %w", err)
	}

	var catalog ShellCatalog
	if err := json.Unmarshal(dataBytes, &catalog); err != nil {
		return fmt.Errorf("failed to parse shell.json: %w", err)
	}

	for _, env := range []ShellEnvironment{catalog.Unix, catalog.Windows} {
		for _, cmd := range env.WriteCommands {
			lower := strings.ToLower(strings.TrimSpace(cmd))
			if lower != "" {
				r.shellWriteCommands[lower] = struct{}{}
			}
		}
		r.shellWritePipes = append(r.shellWritePipes, env.WritePipes...)
		for k, v := range env.ContextualCommands {
			r.contextualCommands[strings.ToLower(k)] = append(r.contextualCommands[strings.ToLower(k)], v...)
		}
		r.redirections = append(r.redirections, env.Redirections...)
	}
	return nil
}

// Reload reloads the registry from a filesystem.
func (r *Registry) Reload(sysFS fs.FS) error {
	newReg, err := NewRegistryFromFS(sysFS)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.writeTools = newReg.writeTools
	r.writeToolsList = newReg.writeToolsList
	r.fileReadTools = newReg.fileReadTools
	r.fileReadToolsList = newReg.fileReadToolsList
	r.allKnownTools = newReg.allKnownTools
	r.shellWriteCommands = newReg.shellWriteCommands
	r.shellWritePipes = newReg.shellWritePipes
	r.contextualCommands = newReg.contextualCommands
	r.redirections = newReg.redirections
	r.writeTagByteMarkers = newReg.writeTagByteMarkers
	r.interactiveToolsLookup = newReg.interactiveToolsLookup
	r.interactiveToolsList = newReg.interactiveToolsList
	r.modeHeuristicsList = newReg.modeHeuristicsList
	r.questionHeuristicsList = newReg.questionHeuristicsList
	r.errorSignaturesList = newReg.errorSignaturesList
	r.errorSignaturesLower = newReg.errorSignaturesLower
	r.manifest = newReg.manifest
	r.reasoningCatalog = newReg.reasoningCatalog
	r.tagReplacer = newReg.tagReplacer
	r.reasoningByteMarkers = newReg.reasoningByteMarkers
	r.controlTokensBytes = newReg.controlTokensBytes
	r.delimiterPrefixes = newReg.delimiterPrefixes
	return nil
}

// IsWriteTool checks if the tool name represents a known structured file writing or editing tool.
// Lock-free, zero heap allocation.
func (r *Registry) IsWriteTool(toolName string) bool {
	if r == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}

	if _, exists := r.writeTools[lower]; exists {
		return true
	}

	// Dynamic suffix/prefix heuristics for unregistered custom tools
	return strings.HasSuffix(lower, "_editor") || strings.HasPrefix(lower, "edit_") ||
		strings.Contains(lower, "write") || strings.Contains(lower, "patch") || strings.Contains(lower, "diff") ||
		strings.Contains(lower, "edit") || strings.Contains(lower, "replace") || strings.Contains(lower, "insert") ||
		strings.Contains(lower, "create")
}

// IsFileReadTool checks if the tool name represents a known file-content reading tool.
// Lock-free, zero heap allocation.
func (r *Registry) IsFileReadTool(toolName string) bool {
	if r == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}

	if _, exists := r.fileReadTools[lower]; exists {
		return true
	}

	// Dynamic heuristics for unregistered custom tools (excluding directory/search tools)
	if strings.Contains(lower, "dir") || strings.Contains(lower, "list") ||
		strings.Contains(lower, "search") || strings.Contains(lower, "find") {
		return false
	}
	return strings.Contains(lower, "read") || strings.Contains(lower, "view") ||
		lower == "cat" || strings.Contains(lower, "open")
}

// IsInteractiveTool checks if the tool name represents a known conversational or mode switching tool.
// Lock-free, zero heap allocation.
func (r *Registry) IsInteractiveTool(toolName string) bool {
	if r == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}

	if r.interactiveToolsLookup == nil {
		return false
	}
	_, exists := r.interactiveToolsLookup[lower]
	return exists
}

// IsKnownTool checks if a tool is recognized in any category across registered agent profiles.
// Lock-free, zero heap allocation.
func (r *Registry) IsKnownTool(toolName string) bool {
	if r == nil {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}

	_, exists := r.allKnownTools[lower]
	return exists
}

// WriteToolsList returns an immutable, pre-sorted slice of all registered file write tool names.
// Lock-free, zero heap allocation.
func (r *Registry) WriteToolsList() []string {
	if r == nil {
		return nil
	}
	return r.writeToolsList
}

// FileReadToolsList returns an immutable, pre-sorted slice of all registered file read tool names.
// Lock-free, zero heap allocation.
func (r *Registry) FileReadToolsList() []string {
	if r == nil {
		return nil
	}
	return r.fileReadToolsList
}

// WriteTagByteMarkers returns the precompiled byte signatures for in-flight XML write tool demuxing.
// Lock-free, zero heap allocation.
func (r *Registry) WriteTagByteMarkers() [][]byte {
	if r == nil {
		return nil
	}
	return r.writeTagByteMarkers
}

// TagReplacer returns the precompiled string replacer for canonicalizing thinking/turn tags.
// Lock-free, zero heap allocation.
func (r *Registry) TagReplacer() *strings.Replacer {
	if r == nil {
		return nil
	}
	return r.tagReplacer
}

// ReasoningByteMarkers returns the precompiled byte signatures for identifying reasoning chunks.
// Lock-free, zero heap allocation.
func (r *Registry) ReasoningByteMarkers() [][]byte {
	if r == nil {
		return nil
	}
	return r.reasoningByteMarkers
}

// ControlTokensByteList returns the precompiled immutable slice of control token byte markers.
// Pre-sorted descending by length for maximal greedy matching.
// Lock-free, zero heap allocation.
func (r *Registry) ControlTokensByteList() [][]byte {
	if r == nil {
		return nil
	}
	return r.controlTokensBytes
}

// StripControlTokensInPlace removes all cataloged reasoning tags and control tokens in-place.
// Lock-free, zero heap allocation.
func (r *Registry) StripControlTokensInPlace(b []byte) []byte {
	if r == nil || len(b) == 0 {
		return b
	}
	return zeroalloc.StripSubslicesInPlace(b, r.controlTokensBytes)
}

// FindTrailingDelimiterPrefix checks if the tail of b (up to maxLen bytes) matches
// any valid proper prefix of a cataloged reasoning/control delimiter.
// Returns the byte offset in b where the earliest matching prefix begins, or -1 if none.
// Lock-free, zero heap allocation.
func (r *Registry) FindTrailingDelimiterPrefix(b []byte, maxLen int) int {
	if r == nil || len(b) == 0 {
		return -1
	}
	return zeroalloc.FindTrailingPrefix(b, maxLen, r.delimiterPrefixes)
}

// IsKnownDelimiterPrefix checks whether b matches any cataloged delimiter prefix.
// Lock-free, zero heap allocation.
func (r *Registry) IsKnownDelimiterPrefix(b []byte) bool {
	if r == nil || len(b) == 0 {
		return false
	}
	for _, p := range r.delimiterPrefixes {
		if string(p) == string(b) {
			return true
		}
	}
	return false
}

// InteractiveToolsList returns an immutable, pre-sorted slice of known interactive/followup tool names.
// Lock-free, zero heap allocation.
func (r *Registry) InteractiveToolsList() []string {
	if r == nil {
		return nil
	}
	return r.interactiveToolsList
}

// ModeHeuristicsList returns an immutable, pre-sorted slice of mode-switching phrases.
// Lock-free, zero heap allocation.
func (r *Registry) ModeHeuristicsList() []string {
	if r == nil {
		return nil
	}
	return r.modeHeuristicsList
}

// QuestionHeuristicsList returns an immutable, pre-sorted slice of conversational question heuristics.
// Lock-free, zero heap allocation.
func (r *Registry) QuestionHeuristicsList() []string {
	if r == nil {
		return nil
	}
	return r.questionHeuristicsList
}

// ErrorSignaturesList returns an immutable, pre-sorted slice of known agent error signatures.
// Lock-free, zero heap allocation.
func (r *Registry) ErrorSignaturesList() []string {
	if r == nil {
		return nil
	}
	return r.errorSignaturesList
}

// IsToolError checks whether a tool output represents an execution error or failure.
// It inspects boolean flags, cataloged agent error signatures, and common failure prefixes.
// Lock-free, zero heap allocation.
func (r *Registry) IsToolError(content string, isError bool) bool {
	if isError {
		return true
	}
	if r == nil {
		return false
	}
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		return false
	}

	// 1. Catalog signatures lookup (zero-alloc case folding)
	for _, sig := range r.errorSignaturesList {
		if containsFold(trimmed, sig) {
			return true
		}
	}

	// 2. Generic heuristics fallback for unregistered agents / uncataloged errors
	if hasPrefixFold(trimmed, "error:") ||
		hasPrefixFold(trimmed, "fatal:") ||
		hasPrefixFold(trimmed, "[syntax error]") ||
		hasPrefixFold(trimmed, "exit code:") ||
		containsFold(trimmed, "command execution was not successful") ||
		containsFold(trimmed, "not recognized as an internal or external command") ||
		(containsFold(trimmed, "the process") && containsFold(trimmed, "not found")) ||
		containsFold(trimmed, "unable to apply diff") {
		return true
	}

	return false
}

func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return strings.EqualFold(s[:len(prefix)], prefix)
}

func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	b0 := substr[0]
	var b0Alt byte
	if b0 >= 'a' && b0 <= 'z' {
		b0Alt = b0 - ('a' - 'A')
	} else if b0 >= 'A' && b0 <= 'Z' {
		b0Alt = b0 + ('a' - 'A')
	} else {
		b0Alt = b0
	}

	limit := len(s) - len(substr)
	for i := 0; i <= limit; i++ {
		c := s[i]
		if c == b0 || c == b0Alt {
			if strings.EqualFold(s[i:i+len(substr)], substr) {
				return true
			}
		}
	}
	return false
}

// DetectShellWrite inspects shell command lines for file-writing operations using zero-alloc string parsing.
func (r *Registry) DetectShellWrite(cmd string) bool {
	if r == nil {
		return false
	}
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
				if i > 0 && (trimmed[i-1] == '&' || trimmed[i-1] == '<') {
					continue
				}
				if i+1 < len(trimmed) && trimmed[i+1] == '&' {
					continue
				}
				targetIdx := i + 1
				if targetIdx < len(trimmed) && trimmed[targetIdx] == '>' {
					targetIdx++
				}
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

	// 2. Check shell write pipes
	for _, pipe := range r.shellWritePipes {
		if strings.Contains(lower, strings.ToLower(pipe)) {
			return true
		}
	}

	// 3. Heredocs combined with file writing
	if strings.Contains(trimmed, "<<") {
		if strings.Contains(trimmed, ">") || strings.Contains(lower, "| tee") {
			return true
		}
	}

	// 4. File-modifying commands
	for cmdWord := range r.shellWriteCommands {
		if containsCommandWord(lower, cmdWord) {
			return true
		}
	}

	// 5. Contextual commands
	for baseCmd, subFlags := range r.contextualCommands {
		if containsCommandWord(lower, baseCmd) {
			for _, flag := range subFlags {
				if strings.Contains(lower, flag) {
					return true
				}
			}
		}
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

func isCmdPrefixBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', ';', '|', '&', '`', '(':
		return true
	default:
		return false
	}
}

func isCmdSuffixBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', ';', '|', '&', '`', ')':
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
		prefixOK := actualPos == 0 || isCmdPrefixBoundary(s[actualPos-1])

		afterPos := actualPos + len(word)
		suffixOK := afterPos >= len(s) || isCmdSuffixBoundary(s[afterPos])

		if prefixOK && suffixOK {
			return true
		}
		idx = actualPos + 1
	}
}

func compileWriteByteMarkers(tools []string) [][]byte {
	markers := [][]byte{
		[]byte("</"),
		[]byte(`\u003c/`),
		[]byte(`\u003C/`),
	}

	seen := make(map[string]struct{})
	addMarker := func(s string) {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			markers = append(markers, []byte(s))
		}
	}

	for _, tool := range tools {
		if tool == "" {
			continue
		}
		// 1. Raw XML open and close
		addMarker("<" + tool)
		addMarker("</" + tool)

		// 2. Unicode JSON escapes (\u003c and \u003C)
		addMarker(`\u003c` + tool)
		addMarker(`\u003C` + tool)
		addMarker(`\u003c/` + tool)
		addMarker(`\u003C/` + tool)

		// 3. Prefix splits if tool has underscores (e.g. write_, replace_, create_, edit_, patch_)
		if idx := strings.IndexByte(tool, '_'); idx != -1 {
			prefix := tool[:idx+1]
			addMarker("<" + prefix)
			addMarker("</" + prefix)
			addMarker(`\u003c` + prefix)
			addMarker(`\u003C` + prefix)
			addMarker(`\u003c/` + prefix)
			addMarker(`\u003C/` + prefix)
		}
	}

	return markers
}

func (r *Registry) loadReasoning(sysFS fs.FS) error {
	dataBytes, err := fs.ReadFile(sysFS, "reasoning.json")
	if err != nil {
		return fmt.Errorf("failed to read reasoning.json: %w", err)
	}

	var catalog ReasoningCatalog
	if err := json.Unmarshal(dataBytes, &catalog); err != nil {
		return fmt.Errorf("failed to parse reasoning.json: %w", err)
	}

	r.reasoningCatalog = catalog
	r.tagReplacer, r.reasoningByteMarkers, r.controlTokensBytes, r.delimiterPrefixes = compileReasoning(catalog)
	return nil
}

func compileReasoning(cat ReasoningCatalog) (*strings.Replacer, [][]byte, [][]byte, [][]byte) {
	type pair struct {
		from string
		to   string
	}
	var pairs []pair

	for target, sources := range cat.CanonicalOpen {
		for _, src := range sources {
			if src != "" {
				pairs = append(pairs, pair{from: src, to: target})
			}
		}
	}
	for target, sources := range cat.CanonicalClose {
		for _, src := range sources {
			if src != "" {
				pairs = append(pairs, pair{from: src, to: target})
			}
		}
	}
	for _, delimiter := range cat.StripDelimiters {
		if delimiter != "" {
			pairs = append(pairs, pair{from: delimiter, to: ""})
		}
	}

	// Sort descending by length of 'from', tie-break alphabetically for deterministic order
	sort.Slice(pairs, func(i, j int) bool {
		if len(pairs[i].from) != len(pairs[j].from) {
			return len(pairs[i].from) > len(pairs[j].from)
		}
		return pairs[i].from < pairs[j].from
	})

	var replacerArgs []string
	for _, p := range pairs {
		replacerArgs = append(replacerArgs, p.from, p.to)
	}
	replacer := strings.NewReplacer(replacerArgs...)

	// Build byte markers
	seen := make(map[string]struct{})
	var markers [][]byte

	addMarker := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, exists := seen[s]; !exists {
			seen[s] = struct{}{}
			markers = append(markers, []byte(s))
		}
	}

	for _, field := range cat.ReasoningFields {
		addMarker(field)
	}
	for target := range cat.CanonicalOpen {
		addMarker(target)
	}
	for target := range cat.CanonicalClose {
		addMarker(target)
	}
	for _, p := range pairs {
		addMarker(p.from)
	}
	for _, extra := range cat.ExtraByteMarkers {
		addMarker(extra)
	}

	// Expand unicode JSON escapes (\u003c and \u003C)
	snapshot := make([]string, 0, len(seen))
	for s := range seen {
		snapshot = append(snapshot, s)
	}

	for _, s := range snapshot {
		if strings.HasPrefix(s, "<") {
			base := strings.TrimRight(s, ">|")
			if len(base) > 1 {
				addMarker(`\u003c` + base[1:])
				addMarker(`\u003C` + base[1:])
			}
		}
	}

	// Build complete control token slice for zero-allocation in-place stripping
	seenControl := make(map[string]struct{})
	addControlToken := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			seenControl[s] = struct{}{}
		}
	}

	for _, p := range pairs {
		addControlToken(p.from)
	}

	// Expand unicode escapes for tokens starting with '<'
	controlSnapshot := make([]string, 0, len(seenControl))
	for s := range seenControl {
		controlSnapshot = append(controlSnapshot, s)
	}
	for _, s := range controlSnapshot {
		if strings.HasPrefix(s, "<") {
			base := s[1:]
			addControlToken(`\u003c` + base)
			addControlToken(`\u003C` + base)
		}
	}

	var controlTokensList []string
	for s := range seenControl {
		controlTokensList = append(controlTokensList, s)
	}
	sort.Slice(controlTokensList, func(i, j int) bool {
		if len(controlTokensList[i]) != len(controlTokensList[j]) {
			return len(controlTokensList[i]) > len(controlTokensList[j])
		}
		return controlTokensList[i] < controlTokensList[j]
	})

	var controlTokensBytes [][]byte
	for _, s := range controlTokensList {
		controlTokensBytes = append(controlTokensBytes, []byte(s))
	}

	// Build delimiter proper prefixes (length strictly < len(tok)) for chunk-boundary split matching
	seenPrefixes := make(map[string]struct{})
	for _, tok := range controlTokensList {
		if strings.HasPrefix(tok, "<") {
			maxP := len(tok) - 1
			if maxP > 24 {
				maxP = 24
			}
			for l := 1; l <= maxP; l++ {
				seenPrefixes[tok[:l]] = struct{}{}
			}
		}
	}

	var delimiterPrefixes [][]byte
	for p := range seenPrefixes {
		delimiterPrefixes = append(delimiterPrefixes, []byte(p))
	}
	sort.Slice(delimiterPrefixes, func(i, j int) bool {
		if len(delimiterPrefixes[i]) != len(delimiterPrefixes[j]) {
			return len(delimiterPrefixes[i]) > len(delimiterPrefixes[j])
		}
		return string(delimiterPrefixes[i]) < string(delimiterPrefixes[j])
	})

	return replacer, markers, controlTokensBytes, delimiterPrefixes
}

func mapToSortedSlice(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}
