// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package agentregistry

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/dixieflatline76/nacho-flow/data"
)

// Registry maintains an in-memory index of agent profiles, tool categories, and shell commands.
type Registry struct {
	mu                  sync.RWMutex
	writeTools          map[string]struct{}
	writeToolsList      []string
	allKnownTools       map[string]struct{}
	shellWriteCommands  map[string]struct{}
	shellWritePipes     []string
	contextualCommands  map[string][]string
	redirections        []string
	writeTagByteMarkers [][]byte
	manifest            Manifest
	reasoningCatalog    ReasoningCatalog
	tagReplacer         *strings.Replacer
	reasoningByteMarkers [][]byte
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

func (r *Registry) loadAgents(sysFS fs.FS) error {
	entries, err := fs.ReadDir(sysFS, "agents")
	if err != nil {
		return fmt.Errorf("failed to read agents directory: %w", err)
	}

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

		for _, tool := range profile.WriteTools {
			lower := strings.ToLower(strings.TrimSpace(tool))
			if lower != "" {
				r.writeTools[lower] = struct{}{}
				r.allKnownTools[lower] = struct{}{}
			}
		}
		for _, tool := range profile.ReadTools {
			lower := strings.ToLower(strings.TrimSpace(tool))
			if lower != "" {
				r.allKnownTools[lower] = struct{}{}
			}
		}
		for _, tool := range profile.CommandTools {
			lower := strings.ToLower(strings.TrimSpace(tool))
			if lower != "" {
				r.allKnownTools[lower] = struct{}{}
			}
		}
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
	r.allKnownTools = newReg.allKnownTools
	r.shellWriteCommands = newReg.shellWriteCommands
	r.shellWritePipes = newReg.shellWritePipes
	r.contextualCommands = newReg.contextualCommands
	r.redirections = newReg.redirections
	r.writeTagByteMarkers = newReg.writeTagByteMarkers
	r.manifest = newReg.manifest
	r.reasoningCatalog = newReg.reasoningCatalog
	r.tagReplacer = newReg.tagReplacer
	r.reasoningByteMarkers = newReg.reasoningByteMarkers
	return nil
}

// IsWriteTool checks if the tool name represents a known structured file writing or editing tool.
func (r *Registry) IsWriteTool(toolName string) bool {
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}

	r.mu.RLock()
	_, exists := r.writeTools[lower]
	r.mu.RUnlock()
	if exists {
		return true
	}

	// Dynamic suffix/prefix heuristics for unregistered custom tools
	return strings.HasSuffix(lower, "_editor") || strings.HasPrefix(lower, "edit_") ||
		strings.Contains(lower, "write") || strings.Contains(lower, "patch") || strings.Contains(lower, "diff")
}

// IsKnownTool checks if a tool is recognized in any category across registered agent profiles.
func (r *Registry) IsKnownTool(toolName string) bool {
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.allKnownTools[lower]
	return exists
}

// WriteToolsList returns an immutable, pre-sorted slice of all registered file write tool names.
// Lock-free, zero heap allocation.
func (r *Registry) WriteToolsList() []string {
	return r.writeToolsList
}

// WriteTagByteMarkers returns the precompiled byte signatures for in-flight XML write tool demuxing.
// Lock-free, zero heap allocation.
func (r *Registry) WriteTagByteMarkers() [][]byte {
	return r.writeTagByteMarkers
}

// TagReplacer returns the precompiled string replacer for canonicalizing thinking/turn tags.
// Lock-free, zero heap allocation.
func (r *Registry) TagReplacer() *strings.Replacer {
	return r.tagReplacer
}

// ReasoningByteMarkers returns the precompiled byte signatures for identifying reasoning chunks.
// Lock-free, zero heap allocation.
func (r *Registry) ReasoningByteMarkers() [][]byte {
	return r.reasoningByteMarkers
}

// DetectShellWrite inspects shell command lines for file-writing operations using zero-alloc string parsing.
func (r *Registry) DetectShellWrite(cmd string) bool {
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

	r.mu.RLock()
	defer r.mu.RUnlock()

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
	r.tagReplacer, r.reasoningByteMarkers = compileReasoning(catalog)
	return nil
}

func compileReasoning(cat ReasoningCatalog) (*strings.Replacer, [][]byte) {
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

	return replacer, markers
}

