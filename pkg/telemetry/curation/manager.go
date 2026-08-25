package curation

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"golang.org/x/mod/semver"
)

// DefaultRemoteCatalogURL is the canonical remote endpoint on GitHub for Over-The-Air catalog updates.
const DefaultRemoteCatalogURL = contract.DefaultRemoteCatalogURL

// Manager orchestrates loading, semver resolution, and background OTA sync for curated model intelligence.
type Manager struct {
	activeCatalog atomic.Pointer[CuratedCatalog]
	remoteURL     string
	cacheDir      string
	httpClient    *http.Client
	mu            sync.Mutex
}

// NewManager initializes a new Curation Manager, loading the latest catalog between embedded and cached versions.
func NewManager(cacheDir, remoteURL string) *Manager {
	if remoteURL == "" {
		remoteURL = contract.DefaultRemoteCatalogURL
	}
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cacheDir = filepath.Join(home, filepath.FromSlash(contract.DefaultCatalogCacheDir))
		} else {
			cacheDir = filepath.Join(os.TempDir(), filepath.FromSlash(contract.DefaultCatalogCacheDir))
		}
	}

	m := &Manager{
		remoteURL:  remoteURL,
		cacheDir:   cacheDir,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	m.loadInitialCatalog()
	return m
}

// loadInitialCatalog compares embedded and cached catalogs and stores whichever is newer according to semver.
func (m *Manager) loadInitialCatalog() {
	var embedCat CuratedCatalog
	_ = json.Unmarshal(embeddedCatalogBytes, &embedCat)

	var cacheCat CuratedCatalog
	cacheValid := false
	cachePath := filepath.Join(m.cacheDir, contract.DefaultCatalogFileName)
	if data, err := os.ReadFile(cachePath); err == nil {
		if json.Unmarshal(data, &cacheCat) == nil && cacheCat.Version != "" {
			cacheValid = true
		}
	}

	if cacheValid {
		if semver.IsValid(cacheCat.Version) && semver.IsValid(embedCat.Version) {
			if semver.Compare(cacheCat.Version, embedCat.Version) >= 0 {
				m.activeCatalog.Store(&cacheCat)
				return
			}
			m.activeCatalog.Store(&embedCat)
			return
		}
		m.activeCatalog.Store(&cacheCat)
		return
	}

	m.activeCatalog.Store(&embedCat)
}

// Lookup retrieves a curated model profile lock-free from the active catalog.
func (m *Manager) Lookup(modelID string) (ModelCuratedProfile, bool) {
	cat := m.activeCatalog.Load()
	if cat == nil || cat.Models == nil {
		return ModelCuratedProfile{}, false
	}
	profile, ok := cat.Models[modelID]
	return profile, ok
}

// GetActiveCatalog returns a pointer to the currently loaded CuratedCatalog.
func (m *Manager) GetActiveCatalog() *CuratedCatalog {
	return m.activeCatalog.Load()
}

// SyncOTA checks the remote catalog URL, compares semver, and atomically upgrades active intelligence if newer.
func (m *Manager) SyncOTA(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.remoteURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create OTA request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("OTA fetch network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("OTA fetch returned HTTP %d", resp.StatusCode)
	}

	var remoteCat CuratedCatalog
	if err := json.NewDecoder(resp.Body).Decode(&remoteCat); err != nil {
		return false, fmt.Errorf("failed to decode remote catalog JSON: %w", err)
	}

	if remoteCat.Version == "" {
		return false, fmt.Errorf("remote catalog is missing required version string")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.activeCatalog.Load()
	if current != nil && semver.IsValid(remoteCat.Version) && semver.IsValid(current.Version) {
		if semver.Compare(remoteCat.Version, current.Version) <= 0 {
			return false, nil // Active version is already newer or equal
		}
	}

	// Persist to disk cache
	_ = os.MkdirAll(m.cacheDir, 0750)
	cachePath := filepath.Join(m.cacheDir, contract.DefaultCatalogFileName)
	if data, err := json.MarshalIndent(remoteCat, "", "  "); err == nil {
		_ = os.WriteFile(cachePath, data, 0600)
	}

	// Atomically swap in-memory catalog pointer
	m.activeCatalog.Store(&remoteCat)
	slog.Info("OTA Curation Catalog updated successfully", "version", remoteCat.Version, "models_count", len(remoteCat.Models))
	return true, nil
}
