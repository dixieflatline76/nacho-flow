// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package agentregistry

// AgentProfile defines the tool capabilities and tool names of a specific AI coding agent.
type AgentProfile struct {
	ID                            string                         `json:"id"`
	Name                          string                         `json:"name"`
	Version                       string                         `json:"version"`
	UserAgentPatterns             []string                       `json:"user_agent_patterns,omitempty"`
	HeaderSignatures              map[string]string              `json:"header_signatures,omitempty"`
	WriteTools                    []string                       `json:"write_tools"`
	FileReadTools                 []string                       `json:"file_read_tools,omitempty"`
	ReadTools                     []string                       `json:"read_tools"`
	CommandTools                  []string                       `json:"command_tools"`
	InteractiveTools              []string                       `json:"interactive_tools,omitempty"`
	CycleKillerExemptTools        []string                       `json:"cycle_killer_exempt_tools,omitempty"`
	ModeTool                      string                         `json:"mode_tool,omitempty"`
	ModeHeuristics                []string                       `json:"mode_heuristics,omitempty"`
	QuestionHeuristics            []string                       `json:"question_heuristics,omitempty"`
	ErrorSignatures               []string                       `json:"error_signatures,omitempty"`
	ParameterSanitizers           []ParameterSanitizer           `json:"parameter_sanitizers,omitempty"`
	RepetitiveEnvironmentSections []RepetitiveEnvironmentSection `json:"repetitive_environment_sections,omitempty"`
	RecommendedRetentionTurns     int                            `json:"recommended_retention_turns,omitempty"`
}

// RepetitiveEnvironmentSection defines metadata for identifying repetitive context wrappers (e.g. todo lists, reminders).
type RepetitiveEnvironmentSection struct {
	WrapperTag   string `json:"wrapper_tag"`
	HeaderMarker string `json:"header_marker"`
}

// ParameterSanitizer defines argument pattern replacements to heal model-generated parameter errors in-place.
type ParameterSanitizer struct {
	Tool            string   `json:"tool,omitempty"`
	Parameter       string   `json:"parameter"`
	InvalidPatterns []string `json:"invalid_patterns"`
	Replacements    []string `json:"replacements"`
}

// Manifest defines the versioned index of all agent profiles, shell catalog, and reasoning catalog.
type Manifest struct {
	Version   string            `json:"version"`
	Agents    map[string]string `json:"agents"`
	Shell     string            `json:"shell"`
	Reasoning string            `json:"reasoning,omitempty"`
}

// ResuscitationCatalog defines prompt and message text to synthesize when a model turn concludes with reasoning only.
type ResuscitationCatalog struct {
	SilentTurnMessage string `json:"silent_turn_message,omitempty"`
}

// ReasoningCompactionCatalog defines data-driven thresholds for semantic reasoning compaction and hysteresis.
type ReasoningCompactionCatalog struct {
	MinKickoffBytes            int      `json:"min_kickoff_bytes"`
	KickoffScanTurns           int      `json:"kickoff_scan_turns"`
	MinSubstantialBytes        int      `json:"min_substantial_bytes"`
	HysteresisHighWatermark    int      `json:"hysteresis_high_watermark"`
	HysteresisLowWatermark     int      `json:"hysteresis_low_watermark"`
	UniversalToolErrorPrefixes []string `json:"universal_tool_error_prefixes"`
}

// ReasoningCatalog defines model chat-template and thought delimiters to canonicalize or strip.
type ReasoningCatalog struct {
	Version          string                     `json:"version"`
	CanonicalOpen    map[string][]string        `json:"canonical_open"`
	CanonicalClose   map[string][]string        `json:"canonical_close"`
	StripDelimiters  []string                   `json:"strip_delimiters"`
	ReasoningFields  []string                   `json:"reasoning_fields"`
	ExtraByteMarkers []string                   `json:"extra_byte_markers"`
	Resuscitation    ResuscitationCatalog       `json:"resuscitation,omitempty"`
	Compaction       ReasoningCompactionCatalog `json:"compaction,omitempty"`
}

// ShellCatalog defines the platform-specific commands and pipes that perform file writes.
type ShellCatalog struct {
	Version string           `json:"version"`
	Unix    ShellEnvironment `json:"unix"`
	Windows ShellEnvironment `json:"windows"`
}

// CompositePattern defines a multi-token shell write rule requiring at least one token from RequireAny
// and at least one token from WithAny.
type CompositePattern struct {
	RequireAny []string `json:"require_any"`
	WithAny    []string `json:"with_any"`
}

// ShellEnvironment contains OS-specific shell write indicators.
type ShellEnvironment struct {
	WriteCommands      []string            `json:"write_commands"`
	WritePipes         []string            `json:"write_pipes"`
	WritePatterns      []string            `json:"write_patterns,omitempty"`
	CompositePatterns  []CompositePattern  `json:"composite_patterns,omitempty"`
	ContextualCommands map[string][]string `json:"contextual_commands"`
	Redirections       []string            `json:"redirections"`
}
