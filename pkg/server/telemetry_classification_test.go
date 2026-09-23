package server

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestClassifyTurnFailure(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		reqCtx     contract.RequestContext
		targetTier contract.Tier
		expected   telemetry.FailureCategory
	}{
		{
			name:       "Clean successful request",
			statusCode: 200,
			reqCtx:     contract.RequestContext{Tokens: 1000},
			targetTier: contract.Tier{Name: "Tier 1", MaxContext: 8000},
			expected:   telemetry.FailureNone,
		},
		{
			name:       "HTTP 429 Rate Limit",
			statusCode: 429,
			reqCtx:     contract.RequestContext{Tokens: 1000, IsRetry: true},
			targetTier: contract.Tier{Name: "Tier 2"},
			expected:   telemetry.FailureUpstream,
		},
		{
			name:       "HTTP 503 Service Unavailable",
			statusCode: 503,
			reqCtx:     contract.RequestContext{Tokens: 1000},
			targetTier: contract.Tier{Name: "Tier 2"},
			expected:   telemetry.FailureUpstream,
		},
		{
			name:       "HTTP 502 Bad Gateway",
			statusCode: 502,
			reqCtx:     contract.RequestContext{Tokens: 1000},
			targetTier: contract.Tier{Name: "Tier 2"},
			expected:   telemetry.FailureUpstream,
		},
		{
			name:       "Context Limit Exceeded",
			statusCode: 200,
			reqCtx:     contract.RequestContext{Tokens: 16000},
			targetTier: contract.Tier{Name: "Tier 1: Local", MaxContext: 8192},
			expected:   telemetry.FailureContextLimit,
		},
		{
			name:       "Reasoning Loop / Cycle Breaker Triggered",
			statusCode: 200,
			reqCtx: contract.RequestContext{
				Tokens:                2000,
				CycleBreakerTriggered: true,
				CycleBreakerReason:    "repeated ngrams",
			},
			targetTier: contract.Tier{Name: "Tier 1"},
			expected:   telemetry.FailureReasoningLoop,
		},
		{
			name:       "Execution Failure / Test Failed",
			statusCode: 200,
			reqCtx: contract.RequestContext{
				Tokens:      3000,
				HasTestFail: true,
			},
			targetTier: contract.Tier{Name: "Tier 2"},
			expected:   telemetry.FailureExecution,
		},
		{
			name:       "Tool Schema Failure (tools present with zero tool or write progress)",
			statusCode: 200,
			reqCtx: contract.RequestContext{
				Tokens:           2500,
				IsRetry:          true,
				HasTools:         true,
				HasToolProgress:  false,
				HasWriteProgress: false,
			},
			targetTier: contract.Tier{Name: "Tier 2"},
			expected:   telemetry.FailureToolSchema,
		},
		{
			name:       "Generic In-History Retry",
			statusCode: 200,
			reqCtx: contract.RequestContext{
				Tokens:           2500,
				IsRetry:          true,
				HasToolProgress:  true,
				HasWriteProgress: true,
			},
			targetTier: contract.Tier{Name: "Tier 2"},
			expected:   telemetry.FailureExecution,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyTurnFailure(tc.statusCode, tc.reqCtx, tc.targetTier)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
