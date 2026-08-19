package tuner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"gopkg.in/yaml.v3"
)

// ApplyTuning creates a backup of the target config.yaml, updates the local tier rule,
// and atomically replaces the config file on disk.
func ApplyTuning(configPath string, result *TuningResult) (string, error) {
	if configPath == "" {
		configPath = "config.yaml"
	}
	configPath = filepath.Clean(configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file at %s: %w", configPath, err)
	}

	var cfg contract.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// 1. Create Timestamped Backup
	timestamp := time.Now().Format("20060102T150405")
	backupPath := filepath.Clean(fmt.Sprintf("%s.bak.%s", configPath, timestamp))
	// #nosec G703 - backup path is strictly scoped with clean path and timestamp
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to create backup file at %s: %w", backupPath, err)
	}

	// 2. Mutate Local Tier rule
	updated := false
	for i, tier := range cfg.Tiers {
		if tier.Provider == "ollama" || strings.Contains(strings.ToLower(tier.Name), "local") {
			cfg.Tiers[i].When = result.SynthesizedRule
			updated = true
			break
		}
	}

	if !updated && len(cfg.Tiers) > 0 {
		cfg.Tiers[0].When = result.SynthesizedRule
	}

	// 3. Serialize updated config
	updatedData, err := yaml.Marshal(&cfg)
	if err != nil {
		return backupPath, fmt.Errorf("failed to marshal updated config YAML: %w", err)
	}

	// 4. Atomic Write (write to temp in same dir, then rename)
	dir := filepath.Dir(configPath)
	tmpFile := filepath.Clean(filepath.Join(dir, fmt.Sprintf("config.tmp.%d.yaml", os.Getpid())))
	if err := os.WriteFile(tmpFile, updatedData, 0600); err != nil {
		return backupPath, fmt.Errorf("failed to write temporary config: %w", err)
	}

	if err := os.Rename(tmpFile, configPath); err != nil {
		_ = os.Remove(configPath)
		if retryErr := os.Rename(tmpFile, configPath); retryErr != nil {
			_ = os.Remove(tmpFile)
			return backupPath, fmt.Errorf("failed to atomically replace config file: %w", retryErr)
		}
	}

	return backupPath, nil
}
