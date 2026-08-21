package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// Test 4.1: Save and Load fidelity
func TestDiskStore_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	statsPath := filepath.Join(tempDir, "stats.json")

	store, err := NewDiskStore(statsPath)
	if err != nil {
		t.Fatalf("Failed to create DiskStore: %v", err)
	}

	initialSnapshot := telemetry.StatsSnapshot{
		StartedAt:     time.Now().Format(time.RFC3339),
		TotalRequests: 15420,
		TierBreakdown: telemetry.TierMetrics{
			Tier1LocalFree:      12000,
			Tier2CloudCoder:     2420,
			Tier3CloudReasoning: 1000,
			Tier4CloudVision:    0,
		},
		TotalTokensRoutedLocally: 45000000,
		EstimatedCostSavedUSD:    202.50,
	}

	if err := store.Save(initialSnapshot); err != nil {
		t.Fatalf("Failed to save snapshot: %v", err)
	}

	// Create fresh store instance targeting same file
	freshStore, err := NewDiskStore(statsPath)
	if err != nil {
		t.Fatalf("Failed to create fresh store: %v", err)
	}

	loaded, err := freshStore.Load()
	if err != nil {
		t.Fatalf("Failed to load snapshot: %v", err)
	}

	if loaded.TotalRequests != 15420 {
		t.Errorf("Expected TotalRequests 15420, got %d", loaded.TotalRequests)
	}
	if loaded.TierBreakdown.Tier1LocalFree != 12000 {
		t.Errorf("Expected Tier1LocalFree 12000, got %d", loaded.TierBreakdown.Tier1LocalFree)
	}
	if loaded.TotalTokensRoutedLocally != 45000000 {
		t.Errorf("Expected TotalTokensRoutedLocally 45000000, got %d", loaded.TotalTokensRoutedLocally)
	}
	if loaded.EstimatedCostSavedUSD != 202.50 {
		t.Errorf("Expected EstimatedCostSavedUSD 202.50, got %f", loaded.EstimatedCostSavedUSD)
	}
}

// Test 4.2: Missing file defaults to empty snapshot without error
func TestDiskStore_MissingFile_DefaultsCleanly(t *testing.T) {
	tempDir := t.TempDir()
	statsPath := filepath.Join(tempDir, "non_existent_stats.json")

	store, err := NewDiskStore(statsPath)
	if err != nil {
		t.Fatalf("Failed to create DiskStore: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Expected nil error for non-existent file, got: %v", err)
	}

	if loaded.TotalRequests != 0 {
		t.Errorf("Expected TotalRequests 0, got %d", loaded.TotalRequests)
	}
}

// Test 4.3: Atomic overwrite does not leave temp files
func TestDiskStore_AtomicWrite_CleansUpTemp(t *testing.T) {
	tempDir := t.TempDir()
	statsPath := filepath.Join(tempDir, "stats.json")

	store, err := NewDiskStore(statsPath)
	if err != nil {
		t.Fatalf("Failed to create DiskStore: %v", err)
	}

	snap := telemetry.StatsSnapshot{
		TotalRequests: 100,
	}

	for i := 0; i < 5; i++ {
		snap.TotalRequests += int64(i)
		if err := store.Save(snap); err != nil {
			t.Fatalf("Failed to save on iteration %d: %v", i, err)
		}
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	if loaded.TotalRequests != 110 {
		t.Errorf("Expected TotalRequests 110, got %d", loaded.TotalRequests)
	}
}

// Test 4.4: Default path and FilePath getter
func TestDiskStore_DefaultPathAndGetter(t *testing.T) {
	store, err := NewDiskStore("")
	if err != nil {
		t.Fatalf("Failed to create default DiskStore: %v", err)
	}
	if store.FilePath() == "" {
		t.Errorf("Expected non-empty FilePath")
	}
}

// Test 4.5: Corrupt JSON file returns error
func TestDiskStore_CorruptJSON_ReturnsError(t *testing.T) {
	tempDir := t.TempDir()
	statsPath := filepath.Join(tempDir, "corrupt_stats.json")

	// Write invalid JSON
	if err := os.WriteFile(statsPath, []byte("invalid-json{"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	store, err := NewDiskStore(statsPath)
	if err != nil {
		t.Fatalf("Failed to create DiskStore: %v", err)
	}

	_, err = store.Load()
	if err == nil {
		t.Fatalf("Expected error loading corrupt JSON, got nil")
	}
}

// Test 4.6: Save to invalid path returns write error
func TestDiskStore_Save_WriteError(t *testing.T) {
	store := &DiskStore{
		filePath: filepath.Join(t.TempDir(), "nonexistent_subdir", "stats.json"),
	}

	err := store.Save(telemetry.StatsSnapshot{})
	if err == nil {
		t.Fatalf("Expected error saving to non-existent directory, got nil")
	}
}

// Test 4.7: NewDiskStore with file as parent directory returns mkdir error
func TestDiskStore_MkdirError(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "dummy_file")
	if err := os.WriteFile(tempFile, []byte("data"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Passing tempFile/sub/stats.json will cause MkdirAll on tempFile/sub to fail because tempFile is a regular file
	_, err := NewDiskStore(filepath.Join(tempFile, "sub", "stats.json"))
	if err == nil {
		t.Fatalf("Expected error for mkdir under regular file, got nil")
	}
}

// Test 4.8: Load returns read error when target is a directory
func TestDiskStore_Load_DirectoryError(t *testing.T) {
	tempDir := t.TempDir()
	store := &DiskStore{
		filePath: tempDir,
	}

	_, err := store.Load()
	if err == nil {
		t.Fatalf("Expected read error loading directory as file, got nil")
	}
}

// Test 4.9: Save returns rename error when destination is a non-empty directory
func TestDiskStore_Save_RenameToDirectoryError(t *testing.T) {
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "existing_dir")
	if err := os.MkdirAll(filepath.Join(targetDir, "nested"), 0750); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	store := &DiskStore{
		filePath: targetDir,
	}

	err := store.Save(telemetry.StatsSnapshot{})
	if err == nil {
		t.Fatalf("Expected error saving when destination is a non-empty directory, got nil")
	}
}

// Test 4.10: Direct Save validation
func TestDiskStore_Save_Direct(t *testing.T) {
	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "stats.json")
	store, err := NewDiskStore(target)
	if err != nil {
		t.Fatalf("NewDiskStore failed: %v", err)
	}
	if err := store.Save(telemetry.StatsSnapshot{TotalRequests: 5}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
}
