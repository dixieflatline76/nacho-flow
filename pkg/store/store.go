package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// DiskStore provides atomic, corruption-proof persistence for telemetry metrics on disk.
type DiskStore struct {
	mu       sync.Mutex
	filePath string
}

// NewDiskStore creates a DiskStore targeting the specified file path.
func NewDiskStore(filePath string) (*DiskStore, error) {
	if filePath == "" {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			userConfigDir = "."
		}
		filePath = filepath.Join(userConfigDir, "nacho-flow", "stats.json")
	}

	filePath = filepath.Clean(filePath)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	return &DiskStore{
		filePath: filePath,
	}, nil
}

// Load reads and unmarshals the stats snapshot from disk.
// Returns an empty snapshot if the file does not exist.
func (s *DiskStore) Load() (telemetry.StatsSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(filepath.Clean(s.filePath))
	if err != nil {
		if os.IsNotExist(err) {
			return telemetry.StatsSnapshot{}, nil
		}
		return telemetry.StatsSnapshot{}, fmt.Errorf("failed to read stats file: %w", err)
	}

	var snapshot telemetry.StatsSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return telemetry.StatsSnapshot{}, fmt.Errorf("failed to parse stats file: %w", err)
	}

	return snapshot, nil
}

// Save writes the stats snapshot to disk atomically using write-to-temp-then-rename.
func (s *DiskStore) Save(snapshot telemetry.StatsSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats snapshot: %w", err)
	}

	// Write to temporary file in the same directory to ensure atomic same-filesystem rename
	tmpFile := filepath.Clean(fmt.Sprintf("%s.tmp.%d", s.filePath, os.Getpid()))
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temporary stats file: %w", err)
	}

	// Atomically replace the destination file
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		// On Windows, rename over an existing file may require removal first
		_ = os.Remove(s.filePath)
		if retryErr := os.Rename(tmpFile, s.filePath); retryErr != nil {
			_ = os.Remove(tmpFile)
			return fmt.Errorf("failed to atomically rename stats file: %w", retryErr)
		}
	}

	return nil
}

// FilePath returns the target disk path.
func (s *DiskStore) FilePath() string {
	return s.filePath
}
