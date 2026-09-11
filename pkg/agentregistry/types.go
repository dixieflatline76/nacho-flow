// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package agentregistry

// AgentProfile defines the tool capabilities and tool names of a specific AI coding agent.
type AgentProfile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	WriteTools   []string `json:"write_tools"`
	ReadTools    []string `json:"read_tools"`
	CommandTools []string `json:"command_tools"`
}

// Manifest defines the versioned index of all agent profiles, shell catalog, and reasoning catalog.
type Manifest struct {
	Version   string            `json:"version"`
	Agents    map[string]string `json:"agents"`
	Shell     string            `json:"shell"`
	Reasoning string            `json:"reasoning,omitempty"`
}

// ReasoningCatalog defines model chat-template and thought delimiters to canonicalize or strip.
type ReasoningCatalog struct {
	Version          string              `json:"version"`
	CanonicalOpen    map[string][]string `json:"canonical_open"`
	CanonicalClose   map[string][]string `json:"canonical_close"`
	StripDelimiters  []string            `json:"strip_delimiters"`
	ReasoningFields  []string            `json:"reasoning_fields"`
	ExtraByteMarkers []string            `json:"extra_byte_markers"`
}


// ShellCatalog defines the platform-specific commands and pipes that perform file writes.
type ShellCatalog struct {
	Version string           `json:"version"`
	Unix    ShellEnvironment `json:"unix"`
	Windows ShellEnvironment `json:"windows"`
}

// ShellEnvironment contains OS-specific shell write indicators.
type ShellEnvironment struct {
	WriteCommands      []string            `json:"write_commands"`
	WritePipes         []string            `json:"write_pipes"`
	ContextualCommands map[string][]string `json:"contextual_commands"`
	Redirections       []string            `json:"redirections"`
}
