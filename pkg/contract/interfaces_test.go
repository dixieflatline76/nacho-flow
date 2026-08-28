package contract_test

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestProviderConfig_IsLocal(t *testing.T) {
	tests := []struct {
		name     string
		p        contract.ProviderConfig
		expected bool
	}{
		{
			name:     "local provider",
			p:        contract.ProviderConfig{Type: contract.ProviderTypeLocal},
			expected: true,
		},
		{
			name:     "cloud provider",
			p:        contract.ProviderConfig{Type: contract.ProviderTypeCloud},
			expected: false,
		},
		{
			name:     "empty type",
			p:        contract.ProviderConfig{Type: ""},
			expected: false,
		},
		{
			name:     "arbitrary unknown type",
			p:        contract.ProviderConfig{Type: "other"},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.IsLocal(); got != tc.expected {
				t.Errorf("ProviderConfig.IsLocal() = %v, expected %v", got, tc.expected)
			}
		})
	}
}
