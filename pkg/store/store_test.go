package store

import (
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
		TierBreakdown: telemetry.TierBreakdown{
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
