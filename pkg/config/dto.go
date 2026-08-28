package config

import (
	"encoding/json"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"gopkg.in/yaml.v3"
)

// SanitizedProviderDTO represents an LLM provider configuration with masked credentials.
type SanitizedProviderDTO struct {
	BaseURL             string
	Key                 string
	Type                contract.ProviderType
	Headers             map[string]string
	PricingSyncInterval string
}

// SanitizedConfigDTO represents a top-level configuration payload with all credentials masked for safe serialization.
type SanitizedConfigDTO struct {
	Port        int
	ClientAuth  string
	Router      contract.RouterConfig
	Deals       contract.DealsConfig
	Providers   map[string]SanitizedProviderDTO
	Tiers       []contract.Tier
	DefaultTier contract.Tier
}

// ToPublicDTO transforms an internal Config entity into a SanitizedConfigDTO with masked secrets.
func ToPublicDTO(cfg *contract.Config) *SanitizedConfigDTO {
	if cfg == nil {
		return nil
	}

	dto := &SanitizedConfigDTO{
		Port:        cfg.Port,
		ClientAuth:  MaskSecret(cfg.AuthToken),
		Router:      cfg.Router,
		Deals:       cfg.Deals,
		Providers:   make(map[string]SanitizedProviderDTO, len(cfg.Providers)),
		Tiers:       make([]contract.Tier, len(cfg.Tiers)),
		DefaultTier: cfg.DefaultTier,
	}

	for id, p := range cfg.Providers {
		dto.Providers[id] = SanitizedProviderDTO{
			BaseURL:             p.BaseURL,
			Key:                 MaskSecret(p.APIKey),
			Type:                p.Type,
			Headers:             p.Headers,
			PricingSyncInterval: p.PricingSyncInterval,
		}
	}

	copy(dto.Tiers, cfg.Tiers)
	return dto
}

// ToMap converts the SanitizedConfigDTO into a clean map for safe wire serialization.
func (d *SanitizedConfigDTO) ToMap() map[string]any {
	if d == nil {
		return nil
	}
	providers := make(map[string]any, len(d.Providers))
	for id, p := range d.Providers {
		pMap := map[string]any{
			"base_url": p.BaseURL,
		}
		if p.Key != "" {
			pMap["api_key"] = p.Key
		}
		if p.Type != "" {
			pMap["type"] = p.Type
		}
		if len(p.Headers) > 0 {
			pMap["headers"] = p.Headers
		}
		if p.PricingSyncInterval != "" {
			pMap["pricing_sync_interval"] = p.PricingSyncInterval
		}
		providers[id] = pMap
	}

	res := map[string]any{
		"port":         d.Port,
		"providers":    providers,
		"tiers":        d.Tiers,
		"default_tier": d.DefaultTier,
	}
	if d.ClientAuth != "" {
		res["auth_token"] = d.ClientAuth
	}
	if d.Router.EnableInPromptDirectives != nil {
		res["router"] = d.Router
	}
	if d.Deals.Enabled {
		res["deals"] = d.Deals
	}
	return res
}

// MarshalJSON implements json.Marshaler.
func (d *SanitizedConfigDTO) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.ToMap())
}

// MarshalYAML implements yaml.Marshaler.
func (d *SanitizedConfigDTO) MarshalYAML() (any, error) {
	return d.ToMap(), nil
}

// SerializeConfigYAML converts a Config into YAML bytes for disk persistence.
func SerializeConfigYAML(cfg *contract.Config) ([]byte, error) {
	if cfg == nil {
		return nil, nil
	}

	providers := make(map[string]any, len(cfg.Providers))
	for id, p := range cfg.Providers {
		pMap := map[string]any{
			"base_url": p.BaseURL,
		}
		if p.APIKey != "" {
			pMap["api_key"] = p.APIKey
		}
		if p.Type != "" {
			pMap["type"] = p.Type
		}
		if len(p.Headers) > 0 {
			pMap["headers"] = p.Headers
		}
		if p.PricingSyncInterval != "" {
			pMap["pricing_sync_interval"] = p.PricingSyncInterval
		}
		providers[id] = pMap
	}

	res := map[string]any{
		"port":         cfg.Port,
		"providers":    providers,
		"tiers":        cfg.Tiers,
		"default_tier": cfg.DefaultTier,
	}
	if cfg.AuthToken != "" {
		res["auth_token"] = cfg.AuthToken
	}
	if cfg.Router.EnableInPromptDirectives != nil {
		res["router"] = cfg.Router
	}
	if cfg.Deals.Enabled {
		res["deals"] = cfg.Deals
	}
	return yaml.Marshal(res)
}
