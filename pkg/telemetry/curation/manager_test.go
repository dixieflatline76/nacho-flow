package curation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCurationManager_InitialLoad_Embedded(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir, "")

	cat := mgr.GetActiveCatalog()
	if cat == nil {
		t.Fatalf("expected active catalog, got nil")
	}

	if cat.Version != "v1.0.0" {
		t.Errorf("expected version v1.0.0, got %s", cat.Version)
	}

	profile, found := mgr.Lookup("google/gemini-2.5-flash")
	if !found {
		t.Fatalf("expected to find google/gemini-2.5-flash in embedded catalog")
	}

	if profile.TierRole != RoleCodingWorkhorse {
		t.Errorf("expected RoleCodingWorkhorse, got %s", profile.TierRole)
	}

	if profile.CodingIndex <= 0 {
		t.Errorf("expected positive coding index, got %f", profile.CodingIndex)
	}

	// Lookup non-existent
	_, notFound := mgr.Lookup("unknown/non-existent-model")
	if notFound {
		t.Errorf("expected unknown model not to be found")
	}
}

func TestCurationManager_DefaultConstructorPaths(t *testing.T) {
	// Test constructor with empty cacheDir and default URL
	mgr := NewManager("", "")
	if mgr == nil {
		t.Fatalf("expected manager, got nil")
	}
	if mgr.GetActiveCatalog() == nil {
		t.Errorf("expected non-nil catalog")
	}
}

func TestCurationManager_CachePrecedence(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a newer version in the cache dir (v1.5.0 vs embedded v1.0.0)
	newerCat := CuratedCatalog{
		Version:     "v1.5.0",
		UpdatedAt:   time.Now().UTC(),
		Description: "Cached catalog update",
		Models: map[string]ModelCuratedProfile{
			"custom/super-model": {
				Name:            "Custom Super Model",
				TierRole:        RoleDeepReasoner,
				CodingIndex:     95.5,
				ToolReliability: 98.0,
			},
		},
	}
	cacheBytes, err := json.Marshal(newerCat)
	if err != nil {
		t.Fatalf("failed to marshal newer catalog: %v", err)
	}
	cacheFile := filepath.Join(tempDir, "models.json")
	if err := os.WriteFile(cacheFile, cacheBytes, 0600); err != nil {
		t.Fatalf("failed to write cache file: %v", err)
	}

	mgr := NewManager(tempDir, "")
	active := mgr.GetActiveCatalog()
	if active.Version != "v1.5.0" {
		t.Errorf("expected cache version v1.5.0 to take precedence, got %s", active.Version)
	}

	profile, found := mgr.Lookup("custom/super-model")
	if !found {
		t.Fatalf("expected custom/super-model to be found from cache")
	}
	if profile.CodingIndex != 95.5 {
		t.Errorf("expected coding index 95.5, got %f", profile.CodingIndex)
	}
}

func TestCurationManager_NonSemverCacheFallback(t *testing.T) {
	tempDir := t.TempDir()

	// Non-standard semver in cache (e.g. "custom-build-1")
	customCat := CuratedCatalog{
		Version: "custom-build-1",
		Models: map[string]ModelCuratedProfile{
			"custom/model": {Name: "Custom Model", TierRole: RoleGeneral},
		},
	}
	cacheBytes, _ := json.Marshal(customCat)
	cacheFile := filepath.Join(tempDir, "models.json")
	_ = os.WriteFile(cacheFile, cacheBytes, 0600)

	mgr := NewManager(tempDir, "")
	active := mgr.GetActiveCatalog()
	if active.Version != "custom-build-1" {
		t.Errorf("expected custom-build-1 version, got %s", active.Version)
	}
}

func TestCurationManager_EmbeddedPrecedenceOverOlderCache(t *testing.T) {
	tempDir := t.TempDir()

	// Create an older version in cache (v0.5.0 vs embedded v1.0.0)
	olderCat := CuratedCatalog{
		Version: "v0.5.0",
		Models: map[string]ModelCuratedProfile{
			"old/model": {Name: "Old Model", TierRole: RoleFastProse},
		},
	}
	cacheBytes, _ := json.Marshal(olderCat)
	cacheFile := filepath.Join(tempDir, "models.json")
	_ = os.WriteFile(cacheFile, cacheBytes, 0600)

	mgr := NewManager(tempDir, "")
	active := mgr.GetActiveCatalog()
	if active.Version != "v1.0.0" {
		t.Errorf("expected embedded v1.0.0 to take precedence over older cache v0.5.0, got %s", active.Version)
	}
}

func TestCurationManager_CorruptCacheFallback(t *testing.T) {
	tempDir := t.TempDir()
	cacheFile := filepath.Join(tempDir, "models.json")
	_ = os.WriteFile(cacheFile, []byte("{corrupted json"), 0600)

	mgr := NewManager(tempDir, "")
	active := mgr.GetActiveCatalog()
	if active.Version != "v1.0.0" {
		t.Errorf("expected fallback to embedded v1.0.0 when cache is corrupt, got %s", active.Version)
	}
}

func TestCurationManager_NilCatalogLookup(t *testing.T) {
	mgr := &Manager{}
	_, found := mgr.Lookup("any")
	if found {
		t.Errorf("expected false for nil catalog lookup")
	}

	emptyCat := &CuratedCatalog{Models: nil}
	mgr.activeCatalog.Store(emptyCat)
	_, foundEmpty := mgr.Lookup("any")
	if foundEmpty {
		t.Errorf("expected false for nil models map lookup")
	}
}

func TestCurationManager_SyncOTA_SuccessAndEdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	var remotePayload CuratedCatalog
	var statusCode int = http.StatusOK

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			_ = json.NewEncoder(w).Encode(remotePayload)
		}
	}))
	defer server.Close()

	mgr := NewManager(tempDir, server.URL)
	if mgr.GetActiveCatalog().Version != "v1.0.0" {
		t.Fatalf("expected initial version v1.0.0")
	}

	// 1. Remote has newer version v2.0.0
	remotePayload = CuratedCatalog{
		Version:     "v2.0.0",
		UpdatedAt:   time.Now().UTC(),
		Description: "OTA version 2",
		Models: map[string]ModelCuratedProfile{
			"future/model-x": {
				Name:        "Future Model X",
				TierRole:    RoleCodingWorkhorse,
				CodingIndex: 99.0,
			},
		},
	}

	updated, err := mgr.SyncOTA(context.Background())
	if err != nil {
		t.Fatalf("SyncOTA failed: %v", err)
	}
	if !updated {
		t.Errorf("expected SyncOTA to return updated=true")
	}

	if mgr.GetActiveCatalog().Version != "v2.0.0" {
		t.Errorf("expected active catalog to be upgraded to v2.0.0, got %s", mgr.GetActiveCatalog().Version)
	}

	// Verify it was cached to disk
	cachedData, err := os.ReadFile(filepath.Join(tempDir, "models.json"))
	if err != nil {
		t.Fatalf("expected cache file on disk: %v", err)
	}
	var reloaded CuratedCatalog
	_ = json.Unmarshal(cachedData, &reloaded)
	if reloaded.Version != "v2.0.0" {
		t.Errorf("expected cached disk file to be v2.0.0")
	}

	// 2. Second sync with same version -> returns updated=false
	updated2, err := mgr.SyncOTA(context.Background())
	if err != nil {
		t.Fatalf("second SyncOTA failed: %v", err)
	}
	if updated2 {
		t.Errorf("expected updated=false for same version")
	}

	// 3. Remote missing version
	remotePayload = CuratedCatalog{
		Version: "",
	}
	_, errNoVersion := mgr.SyncOTA(context.Background())
	if errNoVersion == nil {
		t.Errorf("expected error when remote catalog lacks version")
	}

	// 4. Remote with HTTP 500 error -> returns error, active catalog untouched
	statusCode = http.StatusInternalServerError
	_, err500 := mgr.SyncOTA(context.Background())
	if err500 == nil {
		t.Errorf("expected error on HTTP 500")
	}
	if mgr.GetActiveCatalog().Version != "v2.0.0" {
		t.Errorf("expected catalog version to remain v2.0.0 after failure")
	}

	// 5. Remote with invalid JSON -> returns error
	statusCode = http.StatusOK
	invalidServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{invalid json"))
	}))
	defer invalidServer.Close()

	mgrInvalid := NewManager(tempDir, invalidServer.URL)
	_, errInvalid := mgrInvalid.SyncOTA(context.Background())
	if errInvalid == nil {
		t.Errorf("expected error for invalid JSON")
	}

	// 6. Invalid URL / network error
	mgrBadURL := NewManager(tempDir, "http://127.0.0.1:99999/invalid")
	_, errBadURL := mgrBadURL.SyncOTA(context.Background())
	if errBadURL == nil {
		t.Errorf("expected error for unreachable URL")
	}
}

func TestCurationManager_ConcurrentLookups(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir, "")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = mgr.Lookup("google/gemini-2.5-flash")
				_, _ = mgr.Lookup("unknown/model")
			}
		}()
	}
	wg.Wait()
}
