package tuner

import (
	"fmt"
	"os"
	"path/filepath"
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

	// 2. Mutate Tier rules and models matching result.Tiers
	updated := false
	for _, tunedTier := range result.Tiers {
		if tunedTier.SynthesizedRule == "" && tunedTier.RecommendedModel == "" {
			continue
		}
		for i := range cfg.Tiers {
			if cfg.Tiers[i].Name == tunedTier.TierName || (len(result.Tiers) == 1 && IsLocalTier(cfg.Tiers[i], cfg.Providers)) {
				if tunedTier.SynthesizedRule != "" {
					cfg.Tiers[i].When = tunedTier.SynthesizedRule
				}
				if tunedTier.RecommendedModel != "" && tunedTier.RecommendedModel != tunedTier.OriginalModel {
					cfg.Tiers[i].Model = tunedTier.RecommendedModel
				}
				updated = true
				break
			}
		}
	}

	if result.DefaultTier != nil && result.DefaultTier.RecommendedModel != "" && result.DefaultTier.RecommendedModel != result.DefaultTier.OriginalModel {
		cfg.DefaultTier.Model = result.DefaultTier.RecommendedModel
		updated = true
	}

	if !updated {
		return backupPath, fmt.Errorf("no matching tiers found in config to apply tuned rules")
	}

	// 3. Serialize updated config
	// #nosec G117 - applier safely serializes existing config fields including auth_token
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
		_ = os.Remove(tmpFile)
		return backupPath, fmt.Errorf("failed to atomically replace config file: %w", err)
	}

	return backupPath, nil
}
