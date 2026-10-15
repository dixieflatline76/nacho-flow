package nts

import (
	"github.com/dixieflatline76/nacho-flow/pkg/agentregistry"
	"github.com/dixieflatline76/nacho-flow/pkg/zeroalloc"
)

// IsToolErrorMessage exports isToolErrorMessage for testing.
func (t *Transformer) IsToolErrorMessage(
	msg map[string]interface{},
	agentProfile *agentregistry.AgentProfile,
	universalPrefixes []string,
) bool {
	return t.isToolErrorMessage(msg, agentProfile, universalPrefixes)
}

// MatchesToolError exports matchesToolError for testing.
func (t *Transformer) MatchesToolError(
	s string,
	agentProfile *agentregistry.AgentProfile,
	universalPrefixes []string,
	allowUniversalPrefixes bool,
) bool {
	return t.matchesToolError(s, agentProfile, universalPrefixes, allowUniversalPrefixes)
}

// PrecededByToolError exports precededByToolError for testing.
func (t *Transformer) PrecededByToolError(
	msgs []interface{},
	assistantIdx int,
	agentProfile *agentregistry.AgentProfile,
	universalPrefixes []string,
) bool {
	return t.precededByToolError(msgs, assistantIdx, agentProfile, universalPrefixes)
}

// IdentifyHistoricalReasoningIndices exports identifyHistoricalReasoningIndices for testing.
func (t *Transformer) IdentifyHistoricalReasoningIndices(
	msgs []interface{},
	agentProfile *agentregistry.AgentProfile,
) (zeroalloc.BitSet512, bool) {
	return t.identifyHistoricalReasoningIndices(msgs, agentProfile)
}
