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

	t.Run("DefaultSuffix", func(t *testing.T) {
		path, err := contract.GetDirectiveFilePath()
		if err != nil {
			t.Fatalf("GetDirectiveFilePath failed: %v", err)
		}
		if !strings.HasSuffix(path, filepath.Join(contract.AppName, contract.DefaultDirectiveFileName)) {
			t.Fatalf("GetDirectiveFilePath() = %s, expected suffix %s", path, filepath.Join(contract.AppName, contract.DefaultDirectiveFileName))
		}
	})

	t.Run("FallbackWhenConfigDirEmpty", func(t *testing.T) {
		t.Setenv("APPDATA", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		t.Setenv("NACHO_DIRECTIVE_FILE", "")
		t.Setenv("NACHO_CONFIG_DIR", "")

		fallbackPath, err := contract.GetDirectiveFilePath()
		if err != nil {
			t.Fatalf("GetDirectiveFilePath fallback failed: %v", err)
		}
		if !strings.HasSuffix(fallbackPath, filepath.Join(contract.AppName, contract.DefaultDirectiveFileName)) {
			t.Fatalf("unexpected fallbackPath: %s", fallbackPath)
		}
	})

	t.Run("CustomDirectiveFileOverride", func(t *testing.T) {
		customPath := filepath.Join(t.TempDir(), "custom-dir", "custom-directive.json")
		t.Setenv("NACHO_DIRECTIVE_FILE", customPath)
		resPath, err := contract.GetDirectiveFilePath()
		if err != nil {
			t.Fatalf("GetDirectiveFilePath with NACHO_DIRECTIVE_FILE failed: %v", err)
		}
		if resPath != customPath {
			t.Fatalf("expected customPath %s, got %s", customPath, resPath)
		}
	})

	t.Run("MkdirAllError", func(t *testing.T) {
		tempDir := t.TempDir()
		blockerFile := filepath.Join(tempDir, "blocker_file")
		if err := os.WriteFile(blockerFile, []byte("blocker"), 0600); err != nil {
			t.Fatalf("failed to create blocker file: %v", err)
		}
		t.Setenv("NACHO_CONFIG_DIR", blockerFile)
		_, err := contract.GetDirectiveFilePath()
		if err == nil {
			t.Errorf("expected error when NACHO_CONFIG_DIR is a regular file")
		}
	})
}

func TestGetUserConfigDir(t *testing.T) {
	t.Run("NachoConfigDirEnvSet", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("NACHO_CONFIG_DIR", tempDir)
		dir, err := contract.GetUserConfigDir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dir != tempDir {
			t.Fatalf("expected %s, got %s", tempDir, dir)
		}
	})

	t.Run("FallbackToOSUserConfigDir", func(t *testing.T) {
		t.Setenv("NACHO_CONFIG_DIR", "")
		dir, err := contract.GetUserConfigDir()
		expectedDir, expectedErr := os.UserConfigDir()
		if (err != nil) != (expectedErr != nil) {
			t.Fatalf("expected error match, got err=%v, expectedErr=%v", err, expectedErr)
		}
		if dir != expectedDir {
			t.Fatalf("expected %s, got %s", expectedDir, dir)
		}
	})
}

