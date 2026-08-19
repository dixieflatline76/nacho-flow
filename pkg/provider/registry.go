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
