package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"gopkg.in/yaml.v3"
)

// LoadConfig reads config.yaml from specified path or standard OS-agnostic locations.
func LoadConfig(customPath string) (*contract.Config, error) {
	pathsToTry := []string{}

	if customPath != "" {
		pathsToTry = append(pathsToTry, customPath)
	}

	// Local directory fallback
	pathsToTry = append(pathsToTry, "config.yaml", "./config.yaml")

	// Cross-platform user config directory
	userConfigDir, err := os.UserConfigDir()
	if err == nil {
		pathsToTry = append(pathsToTry, filepath.Join(userConfigDir, "nacho-flow", "config.yaml"))
	}

	var data []byte
	var loadedPath string

	for _, p := range pathsToTry {
		cleanPath := filepath.Clean(p)
		// #nosec G304 - pathsToTry contains trusted search locations
		b, err := os.ReadFile(cleanPath)
		if err == nil {
			data = b
			loadedPath = cleanPath
			break
		}
	}

	if data == nil {
		return nil, fmt.Errorf("could not find config.yaml in any standard location: %v", pathsToTry)
	}

	var cfg contract.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config at %s: %w", loadedPath, err)
	}

	if cfg.Port == 0 {
		cfg.Port = 8000
	}

	// Resolve ENV variables for auth_token
	if strings.HasPrefix(cfg.AuthToken, "ENV_") {
		envVarName := strings.TrimPrefix(cfg.AuthToken, "ENV_")
		if envVal := os.Getenv(envVarName); envVal != "" {
			cfg.AuthToken = envVal
		}
	} else if cfg.AuthToken == "" {
		if envVal := os.Getenv("NACHO_AUTH_TOKEN"); envVal != "" {
			cfg.AuthToken = envVal
		}
	}

	// Validate providers and resolve ENV variables for API keys
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("config error: at least one provider must be defined in 'providers'")
	}

	for id, p := range cfg.Providers {
		if strings.TrimSpace(p.BaseURL) == "" {
			return nil, fmt.Errorf("config error: provider '%s' is missing required 'base_url'", id)
		}

		// Resolve ENV_... format
		if strings.HasPrefix(p.APIKey, "ENV_") {
			envVarName := strings.TrimPrefix(p.APIKey, "ENV_")
			if envVal := os.Getenv(envVarName); envVal != "" {
				p.APIKey = envVal
			}
		}
		cfg.Providers[id] = p
	}

	// Validate that all tiers reference existing providers
	for _, tier := range cfg.Tiers {
		if _, exists := cfg.Providers[tier.Provider]; !exists {
			return nil, fmt.Errorf("config error: tier '%s' references unknown provider '%s'", tier.Name, tier.Provider)
		}
	}

	if cfg.DefaultTier.Provider != "" {
		if _, exists := cfg.Providers[cfg.DefaultTier.Provider]; !exists {
			return nil, fmt.Errorf("config error: default_tier references unknown provider '%s'", cfg.DefaultTier.Provider)
		}
	}

	return &cfg, nil
}
