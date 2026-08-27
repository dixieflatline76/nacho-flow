package safeio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeBoundedDir_BasicReadWriteAtomic(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "safe_io_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sbd, err := NewSafeBoundedDir(tempDir)
	if err != nil {
		t.Fatalf("failed to create SafeBoundedDir: %v", err)
	}

	if sbd.RootDir() == "" {
		t.Errorf("expected non-empty RootDir")
	}

	// 1. Write and Read legitimate file
	testData := []byte("hello safe bounded world")
	if err := sbd.WriteFile("test.txt", testData, 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	readData, err := sbd.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(readData) != string(testData) {
		t.Errorf("expected %q, got %q", string(testData), string(readData))
	}

	// 2. Subdirectory legitimate write and read
	subData := []byte("nested safe data")
	if err := sbd.WriteFile(filepath.Join("subdir", "nested.txt"), subData, 0600); err != nil {
		t.Fatalf("nested WriteFile failed: %v", err)
	}
	readSub, err := sbd.ReadFile(filepath.Join("subdir", "nested.txt"))
	if err != nil {
		t.Fatalf("nested ReadFile failed: %v", err)
	}
	if string(readSub) != string(subData) {
		t.Errorf("expected %q, got %q", string(subData), string(readSub))
	}

	// 3. Atomic Write
	atomicData := []byte("atomic payload content")
	if err := sbd.AtomicWrite("config.yaml", atomicData, 0600); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}
	readAtomic, err := sbd.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("ReadFile on atomic written file failed: %v", err)
	}
	if string(readAtomic) != string(atomicData) {
		t.Errorf("expected %q, got %q", string(atomicData), string(readAtomic))
	}
}

func TestSafeBoundedDir_PathTraversalRejections(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "safe_io_traversal_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sbd, err := NewSafeBoundedDir(tempDir)
	if err != nil {
		t.Fatalf("failed to create SafeBoundedDir: %v", err)
	}

	hostilePaths := []string{
		"../escaped.txt",
		"../../escaped.txt",
		filepath.Join("subdir", "..", "..", "escaped.txt"),
		"/etc/passwd",
		"\\Windows\\System32\\cmd.exe",
		"C:\\Windows\\System32\\cmd.exe",
	}

	for _, hostile := range hostilePaths {
		t.Run("Reject_"+hostile, func(t *testing.T) {
			_, err := sbd.ResolveSafePath(hostile)
			if err == nil {
				t.Fatalf("expected error resolving hostile path %q, got nil", hostile)
			}
			if !strings.Contains(err.Error(), "security violation") {
				t.Errorf("expected security violation error, got: %v", err)
			}

			// ReadFile should also fail
			if _, err := sbd.ReadFile(hostile); err == nil {
				t.Errorf("expected ReadFile to fail for hostile path %q", hostile)
			}

			// WriteFile should also fail
			if err := sbd.WriteFile(hostile, []byte("evil"), 0600); err == nil {
				t.Errorf("expected WriteFile to fail for hostile path %q", hostile)
			}

			// AtomicWrite should also fail
			if err := sbd.AtomicWrite(hostile, []byte("evil"), 0600); err == nil {
				t.Errorf("expected AtomicWrite to fail for hostile path %q", hostile)
			}
		})
	}
}

func TestSafeBoundedDir_ConstructorAndEdgeCases(t *testing.T) {
	// Empty directory
	if _, err := NewSafeBoundedDir(""); err == nil {
		t.Errorf("expected error for empty base directory")
	}

	tempDir, err := os.MkdirTemp("", "safe_io_edges_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sbd, err := NewSafeBoundedDir(tempDir)
	if err != nil {
		t.Fatalf("failed to create SafeBoundedDir: %v", err)
	}

	// Empty filename
	if _, err := sbd.ResolveSafePath(""); err == nil {
		t.Errorf("expected error for empty filename")
	}

	// Read non-existent file
	if _, err := sbd.ReadFile("missing.txt"); err == nil {
		t.Errorf("expected error reading missing file")
	}
}
