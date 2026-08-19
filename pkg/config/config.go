package config

import (
	"fmt"
	"os"
	"path/filepath"

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
		b, err := os.ReadFile(p)
		if err == nil {
			data = b
			loadedPath = p
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

	return &cfg, nil
}
