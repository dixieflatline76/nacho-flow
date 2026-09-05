package contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestDirectiveConstantsAndPath(t *testing.T) {
	if contract.PathAPIDirective != "/api/v1/directive" {
		t.Fatalf("unexpected PathAPIDirective: %s", contract.PathAPIDirective)
	}
	if contract.DefaultDirectiveFileName != "directive.json" {
		t.Fatalf("unexpected DefaultDirectiveFileName: %s", contract.DefaultDirectiveFileName)
	}
	if contract.DirectiveActionPurgeAllLogs != "PURGE_ALL_LOGS" {
		t.Fatalf("unexpected DirectiveActionPurgeAllLogs: %s", contract.DirectiveActionPurgeAllLogs)
	}
	if contract.DirectiveActionResetCircuits != "RESET_CIRCUITS" {
		t.Fatalf("unexpected DirectiveActionResetCircuits: %s", contract.DirectiveActionResetCircuits)
	}
	if contract.DirectiveActionRecalculateStats != "RECALCULATE_STATS" {
		t.Fatalf("unexpected DirectiveActionRecalculateStats: %s", contract.DirectiveActionRecalculateStats)
	}

	path, err := contract.GetDirectiveFilePath()
	if err != nil {
		t.Fatalf("GetDirectiveFilePath failed: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join(contract.AppName, contract.DefaultDirectiveFileName)) {
		t.Fatalf("GetDirectiveFilePath() = %s, expected suffix %s", path, filepath.Join(contract.AppName, contract.DefaultDirectiveFileName))
	}

	// Test fallback when user config dir is empty/unavailable
	origConfigDir := os.Getenv("APPDATA")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	origDirectiveFile := os.Getenv("NACHO_DIRECTIVE_FILE")
	origNachoConfig := os.Getenv("NACHO_CONFIG_DIR")
	defer func() {
		os.Setenv("APPDATA", origConfigDir)
		os.Setenv("XDG_CONFIG_HOME", origXDG)
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
		os.Setenv("NACHO_DIRECTIVE_FILE", origDirectiveFile)
		os.Setenv("NACHO_CONFIG_DIR", origNachoConfig)
	}()

	os.Unsetenv("APPDATA")
	os.Unsetenv("XDG_CONFIG_HOME")
	os.Unsetenv("HOME")
	os.Unsetenv("USERPROFILE")
	os.Unsetenv("NACHO_DIRECTIVE_FILE")
	os.Unsetenv("NACHO_CONFIG_DIR")

	fallbackPath, err := contract.GetDirectiveFilePath()
	if err != nil {
		t.Fatalf("GetDirectiveFilePath fallback failed: %v", err)
	}
	if !strings.HasSuffix(fallbackPath, filepath.Join(contract.AppName, contract.DefaultDirectiveFileName)) {
		t.Fatalf("unexpected fallbackPath: %s", fallbackPath)
	}

	// Test custom NACHO_DIRECTIVE_FILE override
	customPath := filepath.Join(t.TempDir(), "custom-dir", "custom-directive.json")
	os.Setenv("NACHO_DIRECTIVE_FILE", customPath)
	resPath, err := contract.GetDirectiveFilePath()
	if err != nil {
		t.Fatalf("GetDirectiveFilePath with NACHO_DIRECTIVE_FILE failed: %v", err)
	}
	if resPath != customPath {
		t.Fatalf("expected customPath %s, got %s", customPath, resPath)
	}
	os.Unsetenv("NACHO_DIRECTIVE_FILE")

	// Test MkdirAll error by setting NACHO_CONFIG_DIR to an existing file
	tempFile, tfErr := os.CreateTemp("", "nacho_file_blocker_*")
	if tfErr == nil {
		defer os.Remove(tempFile.Name())
		tempFile.Close()
		os.Setenv("NACHO_CONFIG_DIR", tempFile.Name())
		_, err = contract.GetDirectiveFilePath()
		if err == nil {
			t.Errorf("expected error when NACHO_CONFIG_DIR is a regular file")
		}
	}
}
