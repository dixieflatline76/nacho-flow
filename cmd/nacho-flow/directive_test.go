package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestExecuteStartupDirectives_NoFile(t *testing.T) {
	nonExistent := filepath.Join(t.TempDir(), "non-existent-directive.json")
	err := executeStartupDirectives(nonExistent)
	if err != nil {
		t.Fatalf("expected nil error for missing directive, got: %v", err)
	}
}

func TestExecuteStartupDirectives_EmptyPath(t *testing.T) {
	// Should resolve default path without error
	err := executeStartupDirectives("")
	if err != nil {
		t.Fatalf("expected nil error for empty directive path, got: %v", err)
	}
}

func TestExecuteStartupDirectives_PurgeAllLogs_Full(t *testing.T) {
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0750); err != nil {
		t.Fatalf("failed to create logDir: %v", err)
	}

	trafficLog := filepath.Join(logDir, contract.DefaultTrafficLogFileName)
	if err := os.WriteFile(trafficLog, []byte(`{"turn": 1, "cost": 0.05}`+"\n"), 0600); err != nil {
		t.Fatalf("failed to write traffic log: %v", err)
	}

	routerLog := filepath.Join(logDir, contract.DefaultRouterLogFileName)
	if err := os.WriteFile(routerLog, []byte("router log line\n"), 0600); err != nil {
		t.Fatalf("failed to write router log: %v", err)
	}

	statsFile := filepath.Join(tempDir, "stats.json")
	if err := os.WriteFile(statsFile, []byte(`{"total_requests": 42}`), 0600); err != nil {
		t.Fatalf("failed to write stats file: %v", err)
	}

	directivePath := filepath.Join(tempDir, "directive.json")
	directiveData := DirectiveFile{
		Action:    contract.DirectiveActionPurgeAllLogs,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Targets: DirectiveTargets{
			StatsPath: statsFile,
			LogDir:    logDir,
			LogFiles:  []string{contract.DefaultTrafficLogFileName, contract.DefaultRouterLogFileName},
		},
	}
	raw, err := json.Marshal(directiveData)
	if err != nil {
		t.Fatalf("failed to marshal directive: %v", err)
	}
	if err := os.WriteFile(directivePath, raw, 0600); err != nil {
		t.Fatalf("failed to write directive file: %v", err)
	}

	// Execute startup directives
	if err := executeStartupDirectives(directivePath); err != nil {
		t.Fatalf("executeStartupDirectives returned error: %v", err)
	}

	// 1. Directive file MUST be wiped
	if _, err := os.Stat(directivePath); !os.IsNotExist(err) {
		t.Errorf("directive file was not wiped after execution")
	}

	// 2. Active logs must no longer exist at original paths
	if _, err := os.Stat(trafficLog); !os.IsNotExist(err) {
		t.Errorf("original traffic log still exists")
	}
	if _, err := os.Stat(routerLog); !os.IsNotExist(err) {
		t.Errorf("original router log still exists")
	}

	// 3. Backup files must exist
	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("failed to read log dir: %v", err)
	}

	foundTrafficBak := false
	foundRouterBak := false
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, contract.DefaultTrafficLogFileName+".bak.") {
			foundTrafficBak = true
			content, _ := os.ReadFile(filepath.Join(logDir, name))
			if !strings.Contains(string(content), "turn") {
				t.Errorf("traffic backup content corrupted")
			}
		}
		if strings.HasPrefix(name, contract.DefaultRouterLogFileName+".bak.") {
			foundRouterBak = true
			content, _ := os.ReadFile(filepath.Join(logDir, name))
			if !strings.Contains(string(content), "router log line") {
				t.Errorf("router backup content corrupted")
			}
		}
	}
	if !foundTrafficBak {
		t.Errorf("traffic backup file was not found in log dir")
	}
	if !foundRouterBak {
		t.Errorf("router backup file was not found in log dir")
	}

	// 4. stats.json must be removed
	if _, err := os.Stat(statsFile); !os.IsNotExist(err) {
		t.Errorf("stats.json was not removed")
	}
}

func TestExecuteStartupDirectives_CorruptFile(t *testing.T) {
	tempDir := t.TempDir()
	directivePath := filepath.Join(tempDir, "directive.json")
	if err := os.WriteFile(directivePath, []byte("NOT_VALID_JSON{{{"), 0600); err != nil {
		t.Fatalf("failed to write corrupt directive: %v", err)
	}

	err := executeStartupDirectives(directivePath)
	if err != nil {
		t.Fatalf("expected nil error on corrupt file, got: %v", err)
	}

	// Corrupt file must still be wiped
	if _, err := os.Stat(directivePath); !os.IsNotExist(err) {
		t.Errorf("corrupt directive file was not wiped")
	}
}

func TestExecuteStartupDirectives_UnknownAction(t *testing.T) {
	tempDir := t.TempDir()
	directivePath := filepath.Join(tempDir, "directive.json")
	directiveData := DirectiveFile{
		Action:    "UNKNOWN_ACTION",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(directiveData)
	_ = os.WriteFile(directivePath, raw, 0600)

	err := executeStartupDirectives(directivePath)
	if err != nil {
		t.Fatalf("expected nil error on unknown action, got: %v", err)
	}

	if _, err := os.Stat(directivePath); !os.IsNotExist(err) {
		t.Errorf("unknown action directive file was not wiped")
	}
}

func TestExecuteStartupDirectives_PurgeDefaultsAndMissingFiles(t *testing.T) {
	tempDir := t.TempDir()
	directivePath := filepath.Join(tempDir, "directive.json")
	// Targets empty, files do not exist
	directiveData := DirectiveFile{
		Action:    contract.DirectiveActionPurgeAllLogs,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Targets: DirectiveTargets{
			LogDir:    filepath.Join(tempDir, "empty_logs"),
			StatsPath: filepath.Join(tempDir, "nonexistent_stats.json"),
		},
	}
	raw, _ := json.Marshal(directiveData)
	_ = os.WriteFile(directivePath, raw, 0600)

	err := executeStartupDirectives(directivePath)
	if err != nil {
		t.Fatalf("expected nil error when targets do not exist, got: %v", err)
	}

	if _, err := os.Stat(directivePath); !os.IsNotExist(err) {
		t.Errorf("directive file was not wiped")
	}
}

func TestExecuteStartupDirectives_ReadFileError(t *testing.T) {
	tempDir := t.TempDir()
	// Create a subdirectory with the directive name so os.ReadFile fails
	dirAsFile := filepath.Join(tempDir, "directive.json")
	if err := os.MkdirAll(dirAsFile, 0750); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	err := executeStartupDirectives(dirAsFile)
	if err != nil {
		t.Fatalf("expected nil error when reading directory, got: %v", err)
	}
}

func TestExecuteStartupDirectives_PurgeErrors(t *testing.T) {
	tempDir := t.TempDir()
	directivePath := filepath.Join(tempDir, "directive.json")

	// Create non-empty directory at statsPath so os.Remove fails
	nonEmptyStatsDir := filepath.Join(tempDir, "stats_dir")
	_ = os.MkdirAll(filepath.Join(nonEmptyStatsDir, "child"), 0750)

	directiveData := DirectiveFile{
		Action:    contract.DirectiveActionPurgeAllLogs,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Targets: DirectiveTargets{
			LogDir:    tempDir,
			StatsPath: nonEmptyStatsDir,
			LogFiles:  []string{"fake_log.log"},
		},
	}
	raw, _ := json.Marshal(directiveData)
	_ = os.WriteFile(directivePath, raw, 0600)

	err := executeStartupDirectives(directivePath)
	if err != nil {
		t.Fatalf("expected nil error on purge errors, got: %v", err)
	}
}

func TestExecutePurgeAllLogs_EmptyTargets_Defaults(t *testing.T) {
	tempDir := t.TempDir()
	directivePath := filepath.Join(tempDir, "directive.json")
	directiveData := DirectiveFile{
		Action:    contract.DirectiveActionPurgeAllLogs,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Targets:   DirectiveTargets{}, // All empty to trigger defaults
	}
	raw, _ := json.Marshal(directiveData)
	_ = os.WriteFile(directivePath, raw, 0600)

	if err := executeStartupDirectives(directivePath); err != nil {
		t.Fatalf("expected nil error with empty targets, got: %v", err)
	}
}

func TestExecutePurgeAllLogs_RenameFailure(t *testing.T) {
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	_ = os.MkdirAll(logDir, 0750)

	logFile := filepath.Join(logDir, "test.log")
	_ = os.WriteFile(logFile, []byte("some log"), 0600)

	// Pre-create destination as non-empty directory with current timestamp to force rename failure
	ts := time.Now().Format("20060102-150405")
	destDir := fmt.Sprintf("%s.bak.%s", logFile, ts)
	_ = os.MkdirAll(filepath.Join(destDir, "nested"), 0750)

	targets := DirectiveTargets{
		LogDir:   logDir,
		LogFiles: []string{"test.log"},
	}
	executePurgeAllLogs(targets)
}
