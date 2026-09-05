// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// DirectiveFile defines the on-disk command envelope for cold startup maintenance.
type DirectiveFile struct {
	Action    string           `json:"action"`
	CreatedAt string           `json:"created_at"`
	Targets   DirectiveTargets `json:"targets,omitempty"`
}

// DirectiveTargets defines the target files and directories for directive execution.
type DirectiveTargets struct {
	StatsPath string   `json:"stats_path,omitempty"`
	LogDir    string   `json:"log_dir,omitempty"`
	LogFiles  []string `json:"log_files,omitempty"`
}

// executeStartupDirectives checks for a pending directive file, executes maintenance
// operations on inert files prior to logger and store initialization, and unconditionally
// wipes the directive file. It never returns a fatal error that blocks gateway startup.
func executeStartupDirectives(directivePath string) error {
	if directivePath == "" {
		resolved, err := contract.GetDirectiveFilePath()
		if err != nil {
			return nil
		}
		directivePath = resolved
	}

	if _, err := os.Stat(directivePath); os.IsNotExist(err) {
		return nil
	}

	// Unconditionally wipe directive file on function exit to prevent boot loops
	defer func() {
		if rmErr := os.Remove(directivePath); rmErr != nil && !os.IsNotExist(rmErr) {
			fmt.Fprintf(os.Stderr, "[nacho-flow directive] Warning: failed to wipe directive file %s: %v\n", directivePath, rmErr)
		} else {
			fmt.Fprintf(os.Stderr, "[nacho-flow directive] Successfully wiped directive file: %s\n", directivePath)
		}
	}()

	fmt.Fprintf(os.Stderr, "[nacho-flow directive] 🌮 Found startup directive file: %s\n", directivePath)

	data, err := os.ReadFile(filepath.Clean(directivePath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[nacho-flow directive] Failed to read directive file: %v\n", err)
		return nil
	}

	var directive DirectiveFile
	if err := json.Unmarshal(data, &directive); err != nil {
		fmt.Fprintf(os.Stderr, "[nacho-flow directive] Failed to parse directive JSON: %v\n", err)
		return nil
	}

	fmt.Fprintf(os.Stderr, "[nacho-flow directive] Executing directive action: %s\n", directive.Action)

	switch directive.Action {
	case contract.DirectiveActionPurgeAllLogs:
		executePurgeAllLogs(directive.Targets)
	default:
		fmt.Fprintf(os.Stderr, "[nacho-flow directive] Unknown action '%s' ignored.\n", directive.Action)
	}

	fmt.Fprintf(os.Stderr, "[nacho-flow directive] Maintenance completed. Proceeding to normal boot.\n")
	return nil
}

func executePurgeAllLogs(targets DirectiveTargets) {
	timestamp := time.Now().Format("20060102-150405")

	logDir := targets.LogDir
	if logDir == "" {
		logDir = "logs"
	}

	logFiles := targets.LogFiles
	if len(logFiles) == 0 {
		logFiles = []string{contract.DefaultTrafficLogFileName, contract.DefaultRouterLogFileName}
	}

	// 1. Rotate active log files to *.bak.YYYYMMDD-HHMMSS
	for _, lf := range logFiles {
		src := filepath.Join(logDir, lf)
		if _, err := os.Stat(src); err == nil {
			dest := fmt.Sprintf("%s.bak.%s", src, timestamp)
			if rErr := os.Rename(src, dest); rErr != nil {
				fmt.Fprintf(os.Stderr, "[nacho-flow directive] Error rotating %s -> %s: %v (exit 1)\n", src, dest, rErr)
			} else {
				fmt.Fprintf(os.Stderr, "[nacho-flow directive] Rotated %s -> %s (exit 0)\n", src, dest)
			}
		} else {
			fmt.Fprintf(os.Stderr, "[nacho-flow directive] File %s not present, skipping rotation\n", src)
		}
	}

	// 2. Remove/purge stats.json
	statsPath := targets.StatsPath
	if statsPath == "" {
		userConfigDir, err := contract.GetUserConfigDir()
		if err != nil || userConfigDir == "" {
			userConfigDir = "."
		}
		statsPath = filepath.Join(userConfigDir, contract.AppName, contract.DefaultStatsFileName)
	}

	if _, err := os.Stat(statsPath); err == nil {
		if rmErr := os.Remove(statsPath); rmErr != nil {
			fmt.Fprintf(os.Stderr, "[nacho-flow directive] Error removing stats %s: %v (exit 1)\n", statsPath, rmErr)
		} else {
			fmt.Fprintf(os.Stderr, "[nacho-flow directive] Purged stats file %s (exit 0)\n", statsPath)
		}
	} else {
		fmt.Fprintf(os.Stderr, "[nacho-flow directive] Stats file %s not present, skipping purge\n", statsPath)
	}
}
