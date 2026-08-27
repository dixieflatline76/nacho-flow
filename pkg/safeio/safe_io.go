package safeio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SafeBoundedDir represents a directory root that restricts all file I/O operations strictly
// within its path boundary using os.Root (Go >= 1.24) and lexical path validation.
type SafeBoundedDir struct {
	rootDir string
}

// NewSafeBoundedDir creates a new SafeBoundedDir instance, resolving the base path to an absolute, cleaned path.
func NewSafeBoundedDir(baseDir string) (*SafeBoundedDir, error) {
	if strings.TrimSpace(baseDir) == "" {
		return nil, fmt.Errorf("base directory cannot be empty")
	}

	absPath, err := filepath.Abs(filepath.Clean(baseDir))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for %q: %w", baseDir, err)
	}

	return &SafeBoundedDir{rootDir: absPath}, nil
}

// RootDir returns the absolute root directory path.
func (s *SafeBoundedDir) RootDir() string {
	return s.rootDir
}

// ResolveSafePath validates that the target relative path does not escape the root directory boundary.
func (s *SafeBoundedDir) ResolveSafePath(relPath string) (string, error) {
	trimmed := strings.TrimSpace(relPath)
	if trimmed == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	// Reject absolute paths or rooted paths immediately
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "\\") || (len(trimmed) >= 2 && trimmed[1] == ':') {
		return "", fmt.Errorf("security violation: absolute/rooted path rejected (%q)", relPath)
	}

	// Clean and combine
	targetPath := filepath.Clean(filepath.Join(s.rootDir, trimmed))

	// Ensure the resolved target starts strictly within rootDir
	rel, err := filepath.Rel(s.rootDir, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("security violation: path traversal outside root directory (%q)", relPath)
	}

	return targetPath, nil
}

// ReadFile safely reads file contents strictly within the bounded directory using os.Root.
func (s *SafeBoundedDir) ReadFile(relPath string) ([]byte, error) {
	safePath, err := s.ResolveSafePath(relPath)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(s.rootDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	rel, err := filepath.Rel(s.rootDir, safePath)
	if err != nil {
		return nil, err
	}

	f, err := root.Open(filepath.ToSlash(rel))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}

// WriteFile safely writes data to a file strictly within the bounded directory.
func (s *SafeBoundedDir) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	safePath, err := s.ResolveSafePath(relPath)
	if err != nil {
		return err
	}

	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	root, err := os.OpenRoot(s.rootDir)
	if err != nil {
		return err
	}
	defer root.Close()

	rel, err := filepath.Rel(s.rootDir, safePath)
	if err != nil {
		return err
	}

	f, err := root.OpenFile(filepath.ToSlash(rel), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// AtomicWrite safely writes data via a temporary file and atomic rename within the bounded directory.
func (s *SafeBoundedDir) AtomicWrite(relPath string, data []byte, perm os.FileMode) error {
	safePath, err := s.ResolveSafePath(relPath)
	if err != nil {
		return err
	}

	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	tmpName := fmt.Sprintf(".tmp.%d.%d.%s", os.Getpid(), time.Now().UnixNano(), filepath.Base(safePath))
	tmpPath := filepath.Join(dir, tmpName)

	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, safePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file to target: %w", err)
	}

	return nil
}
