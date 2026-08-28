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
	} else {
		// Local directory fallback
		pathsToTry = append(pathsToTry, contract.DefaultConfigFileName, "./"+contract.DefaultConfigFileName)

		// Cross-platform user config directory
		userConfigDir, err := os.UserConfigDir()
		if err == nil {
			pathsToTry = append(pathsToTry, filepath.Join(userConfigDir, contract.AppName, contract.DefaultConfigFileName))
		}
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
		if customPath != "" {
			return nil, fmt.Errorf("could not find %s in any standard location: %v", customPath, pathsToTry)
		}

		// Auto-bootstrap: When running without an explicit config and none is found,
		// initialize the default starter configuration template into the user config directory.
		data = []byte(DefaultStarterConfigTemplate)
		loadedPath = "embedded:default"

		if uDir, uErr := os.UserConfigDir(); uErr == nil {
			targetDir := filepath.Join(uDir, contract.AppName)
			targetFile := filepath.Join(targetDir, contract.DefaultConfigFileName)
			if mkErr := os.MkdirAll(targetDir, 0750); mkErr == nil {
				// #nosec G306 - User configuration file with restricted permissions
				if writeErr := os.WriteFile(targetFile, data, 0600); writeErr == nil {
					loadedPath = targetFile
				}
			}
		}
	}

	var cfg contract.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config at %s: %w", loadedPath, err)
	}

	if cfg.Port == 0 {
		cfg.Port = contract.DefaultServerPort
	}

	// Resolve ENV variables for auth_token
	if strings.HasPrefix(cfg.AuthToken, contract.EnvVarPrefix) {
		envVarName := strings.TrimPrefix(cfg.AuthToken, contract.EnvVarPrefix)
		if envVal := os.Getenv(envVarName); envVal != "" {
			cfg.AuthToken = envVal
		}
	} else if cfg.AuthToken == "" {
		if envVal := os.Getenv(contract.GlobalAuthTokenEnv); envVal != "" {
			cfg.AuthToken = envVal
		}
	}

	// Resolve ENV variables for provider API keys
	for id, p := range cfg.Providers {
		// Resolve ENV_... format
		if strings.HasPrefix(p.APIKey, "ENV_") {
			envVarName := strings.TrimPrefix(p.APIKey, "ENV_")
			if envVal := os.Getenv(envVarName); envVal != "" {
				p.APIKey = envVal
			}
		}
		cfg.Providers[id] = p
	}

	// Boundary Schema Validation: Enforce mandatory provider types, base URLs, and tier references
	if err := ValidateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
