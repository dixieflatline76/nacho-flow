package provider

import (
	"sync"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// Registry manages the collection of active LLM providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]LLMProvider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]LLMProvider),
	}
}

// NewRegistryFromConfig populates a registry from contract.Config.
func NewRegistryFromConfig(cfg *contract.Config) *Registry {
	r := NewRegistry()
	if cfg == nil {
		return r
	}

	for id, pCfg := range cfg.Providers {
		provider := NewGenericLLMProvider(id, pCfg)
		r.Register(provider)
	}

	return r
}

// Register adds or replaces a provider in the registry.
func (r *Registry) Register(p LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

// Get retrieves a provider by ID in a thread-safe manner.
func (r *Registry) Get(id string) (LLMProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// All returns a slice of all registered providers.
func (r *Registry) All() []LLMProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]LLMProvider, 0, len(r.providers))
	for _, p := range r.providers {
		list = append(list, p)
	}
	return list
}

// CircuitInfo captures the operational health of a provider circuit breaker.
type CircuitInfo struct {
	Provider         string `json:"provider"`
	Name             string `json:"name"`
	State            string `json:"state"`
	Failures         int    `json:"failures"`
	FailureThreshold int    `json:"failure_threshold"`
	CooldownSeconds  int    `json:"cooldown_seconds"`
	IsAvailable      bool   `json:"is_available"`
}

// GetCircuitsStatus returns the health and circuit breaker state of all registered providers.
func (r *Registry) GetCircuitsStatus() []CircuitInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	circuits := make([]CircuitInfo, 0, len(r.providers))
	for _, p := range r.providers {
		info := CircuitInfo{
			Provider:         p.ID(),
			Name:             p.Name(),
			State:            "closed",
			Failures:         0,
			FailureThreshold: DefaultFailureThreshold,
			CooldownSeconds:  int(DefaultCooldownDuration.Seconds()),
			IsAvailable:      true,
		}
		if cbProvider, ok := p.(CircuitBreakerProvider); ok {
			cb := cbProvider.CircuitBreaker()
			if cb != nil {
				info.State = cb.State().String()
				info.Failures = int(cb.failures.Load())
				info.FailureThreshold = int(cb.failureThreshold)
				info.CooldownSeconds = int(cb.cooldown.Seconds())
				info.IsAvailable = cb.AllowRequest()
			}
		}
		circuits = append(circuits, info)
	}
	return circuits
}

// ResetCircuit resets the circuit breaker for a specific provider ID or all providers if id is "all" or empty.
func (r *Registry) ResetCircuit(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if id == "" || id == "all" {
		for _, p := range r.providers {
			if cbProvider, ok := p.(CircuitBreakerProvider); ok {
				if cb := cbProvider.CircuitBreaker(); cb != nil {
					cb.Reset()
				}
			}
		}
		return true
	}

	if p, ok := r.providers[id]; ok {
		if cbProvider, ok := p.(CircuitBreakerProvider); ok {
			if cb := cbProvider.CircuitBreaker(); cb != nil {
				cb.Reset()
				return true
			}
		}
	}
	return false
}
