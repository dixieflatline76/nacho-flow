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
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/safeio"
	"golang.org/x/mod/semver"
)

// DefaultRemoteCatalogURL is the canonical remote endpoint on GitHub for Over-The-Air catalog updates.
const DefaultRemoteCatalogURL = contract.DefaultRemoteCatalogURL

var (
	defaultManagerInstance *Manager
	defaultManagerOnce     sync.Once
)

// DefaultManager returns the process-wide singleton Manager loading the canonical catalog.
func DefaultManager() *Manager {
	defaultManagerOnce.Do(func() {
		defaultManagerInstance = NewManager("", "")
	})
	return defaultManagerInstance
}

type catalogIndex struct {
	exactMap      map[string]ModelCuratedProfile
	normalizedMap map[string]ModelCuratedProfile
	modelList     []catalogModelEntry
}

type catalogModelEntry struct {
	key           string
	normalizedKey string
	profile       ModelCuratedProfile
}

// Manager orchestrates loading, semver resolution, and background OTA sync for curated model intelligence.
type Manager struct {
	activeCatalog atomic.Pointer[CuratedCatalog]
	activeIndex   atomic.Pointer[catalogIndex]
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

func buildCatalogIndex(cat *CuratedCatalog) *catalogIndex {
	if cat == nil || len(cat.Models) == 0 {
		return &catalogIndex{
			exactMap:      make(map[string]ModelCuratedProfile),
			normalizedMap: make(map[string]ModelCuratedProfile),
		}
	}

	idx := &catalogIndex{
		exactMap:      make(map[string]ModelCuratedProfile, len(cat.Models)*3),
		normalizedMap: make(map[string]ModelCuratedProfile, len(cat.Models)*3),
		modelList:     make([]catalogModelEntry, 0, len(cat.Models)),
	}

	putIfBetter := func(m map[string]ModelCuratedProfile, k string, p ModelCuratedProfile, id string) {
		if k == "" {
			return
		}
		if existing, exists := m[k]; exists {
			if strings.Contains(id, ":batch") || strings.Contains(id, "-batch") {
				return
			}
			if existing.CodingIndex > 0 && p.CodingIndex == 0 {
				return
			}
			if existing.CodingIndex > p.CodingIndex {
				return
			}
		}
		m[k] = p
	}

	for id, profile := range cat.Models {
		if profile.ModelID == "" {
			profile.ModelID = id
		}
		if !profile.SupportsVision {
			profile.SupportsVision = profile.TierRole == RoleVisionWorkhorse || slices.Contains(profile.RecommendedTiers, contract.TierIDVision)
		}
		if !profile.SupportsTools {
			profile.SupportsTools = profile.TierRole == RoleCodingWorkhorse || profile.ToolReliability > 0 || slices.Contains(profile.RecommendedTiers, contract.TierIDWorkhorse)
		}
		cat.Models[id] = profile
		lowerID := strings.ToLower(id)
		idx.exactMap[id] = profile
		idx.exactMap[lowerID] = profile

		baseID := id
		if slash := strings.LastIndex(id, "/"); slash >= 0 {
			baseID = id[slash+1:]
		}
		putIfBetter(idx.exactMap, baseID, profile, id)
		putIfBetter(idx.exactMap, strings.ToLower(baseID), profile, id)

		if colon := strings.Index(baseID, ":"); colon >= 0 {
			untagged := baseID[:colon]
			putIfBetter(idx.exactMap, untagged, profile, id)
			putIfBetter(idx.exactMap, strings.ToLower(untagged), profile, id)
		}

		normID := normalizeModelKey(id)
		normBase := normalizeModelKey(baseID)
		putIfBetter(idx.normalizedMap, normID, profile, id)
		putIfBetter(idx.normalizedMap, normBase, profile, id)
		if profile.Name != "" {
			putIfBetter(idx.normalizedMap, normalizeModelKey(profile.Name), profile, id)
		}

		idx.modelList = append(idx.modelList, catalogModelEntry{
			key:           lowerID,
			normalizedKey: normID,
			profile:       profile,
		})
	}

	return idx
}

func normalizeModelKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if slash := strings.LastIndex(s, "/"); slash >= 0 {
		s = s[slash+1:]
	}
	if colon := strings.Index(s, ":"); colon >= 0 {
		s = s[:colon]
	}
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// loadInitialCatalog compares embedded and cached catalogs and stores whichever is newer according to semver.
func (m *Manager) loadInitialCatalog() {
	var embedCat CuratedCatalog
	_ = json.Unmarshal(embeddedCatalogBytes, &embedCat)

	setCatalog := func(cat *CuratedCatalog) {
		m.activeCatalog.Store(cat)
		m.activeIndex.Store(buildCatalogIndex(cat))
	}

	var cacheCat CuratedCatalog
	cacheValid := false
	if sbd, err := safeio.NewSafeBoundedDir(m.cacheDir); err == nil {
		if data, readErr := sbd.ReadFile(contract.DefaultCatalogFileName); readErr == nil {
			if json.Unmarshal(data, &cacheCat) == nil && cacheCat.Version != "" {
				cacheValid = true
			}
		}
	}

	if cacheValid {
		if semver.IsValid(cacheCat.Version) && semver.IsValid(embedCat.Version) {
			if semver.Compare(cacheCat.Version, embedCat.Version) >= 0 {
				setCatalog(&cacheCat)
				return
			}
			setCatalog(&embedCat)
			return
		}
		setCatalog(&cacheCat)
		return
	}

	setCatalog(&embedCat)
}

// Lookup retrieves a curated model profile lock-free from the active catalog.
// It performs exact, case-insensitive, normalized, and data-driven family lookups.
func (m *Manager) Lookup(modelID string) (ModelCuratedProfile, bool) {
	if m == nil {
		return ModelCuratedProfile{}, false
	}
	trimmed := strings.TrimSpace(modelID)
	if trimmed == "" {
		return ModelCuratedProfile{}, false
	}

	idx := m.activeIndex.Load()
	if idx == nil {
		cat := m.activeCatalog.Load()
		if cat == nil || cat.Models == nil {
			return ModelCuratedProfile{}, false
		}
		p, ok := cat.Models[trimmed]
		return p, ok
	}

	// 1. Exact match (as supplied or lowercase)
	if p, ok := idx.exactMap[trimmed]; ok {
		return p, true
	}
	lower := strings.ToLower(trimmed)
	if p, ok := idx.exactMap[lower]; ok {
		return p, true
	}

	// 2. Base model without provider prefix (e.g. claude-3.5-sonnet)
	base := trimmed
	if slash := strings.LastIndex(base, "/"); slash >= 0 {
		base = base[slash+1:]
	}
	if p, ok := idx.exactMap[base]; ok {
		return p, true
	}
	if p, ok := idx.exactMap[strings.ToLower(base)]; ok {
		return p, true
	}

	// 3. Strip tags if present
	if colon := strings.Index(base, ":"); colon >= 0 {
		untagged := base[:colon]
		if p, ok := idx.exactMap[untagged]; ok {
			return p, true
		}
		if p, ok := idx.exactMap[strings.ToLower(untagged)]; ok {
			return p, true
		}
	}

	// 4. Normalized key match (punctuation stripped)
	norm := normalizeModelKey(trimmed)
	if p, ok := idx.normalizedMap[norm]; ok {
		return p, true
	}
	if p, ok := idx.normalizedMap[normalizeModelKey(base)]; ok {
		return p, true
	}

	// 5. Data-driven catalog substring matching against catalog entries
	for _, entry := range idx.modelList {
		if strings.Contains(norm, entry.normalizedKey) || strings.Contains(entry.normalizedKey, norm) {
			return entry.profile, true
		}
	}

	// 6. Data-driven token overlap: match significant words (length >= 4) from modelID against catalog keys
	tokens := strings.FieldsFunc(lower, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, tok := range tokens {
		if len(tok) < 4 || tok == "custom" || tok == "tier" || tok == "model" || tok == "fleet" {
			continue
		}
		for _, entry := range idx.modelList {
			if strings.Contains(entry.key, tok) {
				return entry.profile, true
			}
		}
	}

	return ModelCuratedProfile{}, false
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

	// Persist to disk cache using SafeBoundedDir
	if sbd, sbdErr := safeio.NewSafeBoundedDir(m.cacheDir); sbdErr == nil {
		if data, marshalErr := json.MarshalIndent(remoteCat, "", "  "); marshalErr == nil {
			_ = sbd.AtomicWrite(contract.DefaultCatalogFileName, data, 0600)
		}
	}

	// Atomically swap in-memory catalog pointer and index
	m.activeCatalog.Store(&remoteCat)
	m.activeIndex.Store(buildCatalogIndex(&remoteCat))
	slog.Info("OTA Curation Catalog updated successfully", "version", remoteCat.Version, "models_count", len(remoteCat.Models))
	return true, nil
}

var paramRegex = regexp.MustCompile(`(?i)(?:^|[-_: /])(\d+(?:\.\d+)?)\s*[bB](?:[-_: /]|$)`)

// ExtractParameterSize extracts the parameter count in billions from a model identifier or name.
// Returns 0.0 if parameter size is unspecified.
func ExtractParameterSize(modelID string) float64 {
	m := paramRegex.FindStringSubmatch(modelID)
	if len(m) > 1 {
		if val, err := strconv.ParseFloat(m[1], 64); err == nil {
			return val
		}
	}
	return 0.0
}

// CandidateModels returns filtered candidate models matching the specified tier role and deployment mode.
func (m *Manager) CandidateModels(role TierRole, isLocal bool) []ModelCuratedProfile {
	if m == nil {
		return nil
	}
	cat := m.activeCatalog.Load()
	if cat == nil || len(cat.Models) == 0 {
		return nil
	}

	var candidates []ModelCuratedProfile
	for id, p := range cat.Models {
		if p.ModelID == "" {
			p.ModelID = id
		}
		// Never recommend batch models
		if strings.Contains(id, ":batch") || strings.Contains(id, "-batch") {
			continue
		}

		if isLocal {
			// Local tier: candidate must be an open-weights model capable of local inference
			if !p.IsOpenWeights {
				continue
			}
			// For local tiers, token costs are 0
			p.PromptCostPerMillion = 0
			p.CompletionCostPerMillion = 0
		} else {
			// Cloud tier: ignore zero-benchmark models
			if p.CodingIndex <= 0 {
				continue
			}
		}

		// Role matching
		if role != "" && role != RoleGeneral {
			if role == RoleVisionWorkhorse && !p.SupportsVision && p.TierRole != RoleVisionWorkhorse {
				continue
			}
			if role == RoleCodingWorkhorse && p.TierRole != RoleCodingWorkhorse && p.CodingIndex < 65.0 {
				continue
			}
			if role == RoleDeepReasoner && !p.IsFrontier() {
				continue
			}
		}

		candidates = append(candidates, p)
	}

	return candidates
}

// ParetoCandidates filters candidate models down to the Pareto-optimal frontier.
// For local models (isLocal == true), candidates are bounded by localVRAMGB or currentModelID's parameter class.
// For cloud models, candidates are pruned to the non-dominated set across (BlendedCost, CodingIndex).
func (m *Manager) ParetoCandidates(role TierRole, isLocal bool, currentModelID string, localVRAMGB int) []ModelCuratedProfile {
	allCandidates := m.CandidateModels(role, isLocal)
	if len(allCandidates) == 0 {
		return nil
	}

	if isLocal {
		// Determine maximum parameter ceiling
		maxParams := 8.0 // default baseline ceiling
		if localVRAMGB > 0 {
			switch {
			case localVRAMGB <= 8:
				maxParams = 8.0
			case localVRAMGB <= 12:
				maxParams = 14.5
			case localVRAMGB <= 16:
				maxParams = 16.5
			case localVRAMGB <= 24:
				maxParams = 33.0
			case localVRAMGB <= 48:
				maxParams = 72.0
			default:
				maxParams = 1000.0 // unlimited
			}
		} else if currentModelID != "" {
			if currP := ExtractParameterSize(currentModelID); currP > 0 {
				maxParams = currP
			}
		}

		// Filter candidates by parameter ceiling
		var bounded []ModelCuratedProfile
		for _, cand := range allCandidates {
			candParams := ExtractParameterSize(cand.ModelID)
			if candParams == 0 {
				candParams = ExtractParameterSize(cand.Name)
			}
			if candParams <= 0 || candParams > maxParams {
				continue
			}
			bounded = append(bounded, cand)
		}

		if len(bounded) == 0 {
			return nil
		}

		// Group by parameter class and pick highest CodingIndex and ToolReliability
		sort.Slice(bounded, func(i, j int) bool {
			if bounded[i].CodingIndex != bounded[j].CodingIndex {
				return bounded[i].CodingIndex > bounded[j].CodingIndex
			}
			return bounded[i].ToolReliability > bounded[j].ToolReliability
		})

		seenParams := make(map[int]bool)
		var result []ModelCuratedProfile
		for _, c := range bounded {
			pClass := int(ExtractParameterSize(c.ModelID))
			if !seenParams[pClass] {
				seenParams[pClass] = true
				result = append(result, c)
				if len(result) >= 5 {
					break
				}
			}
		}
		if len(result) == 0 {
			result = bounded
		}
		return result
	}

	// Cloud: Multi-objective Pareto frontier over (BlendedRate, CodingIndex)
	// BlendedRate = PromptCost + 0.25 * CompletionCost
	type modelWithRate struct {
		profile ModelCuratedProfile
		rate    float64
	}

	withRates := make([]modelWithRate, 0, len(allCandidates))
	for _, c := range allCandidates {
		rate := c.PromptCostPerMillion + 0.25*c.CompletionCostPerMillion
		withRates = append(withRates, modelWithRate{profile: c, rate: rate})
	}

	var pareto []ModelCuratedProfile
	for i := range withRates {
		dominated := false
		for j := range withRates {
			if i == j {
				continue
			}
			// Model j dominates model i if:
			// Rate_j <= Rate_i AND CodingIndex_j >= CodingIndex_i
			// and at least one is strictly better.
			if withRates[j].rate <= withRates[i].rate &&
				withRates[j].profile.CodingIndex >= withRates[i].profile.CodingIndex {
				if withRates[j].rate < withRates[i].rate ||
					withRates[j].profile.CodingIndex > withRates[i].profile.CodingIndex {
					dominated = true
					break
				}
			}
		}
		if !dominated {
			pareto = append(pareto, withRates[i].profile)
		}
	}

	// Sort Pareto frontier by BlendedRate ascending (cheapest to most capable)
	sort.Slice(pareto, func(i, j int) bool {
		rateI := pareto[i].PromptCostPerMillion + 0.25*pareto[i].CompletionCostPerMillion
		rateJ := pareto[j].PromptCostPerMillion + 0.25*pareto[j].CompletionCostPerMillion
		if rateI != rateJ {
			return rateI < rateJ
		}
		return pareto[i].CodingIndex < pareto[j].CodingIndex
	})

	// Quality preservation invariant: do not recommend models that degrade coding index by more than 5 points
	if currentModelID != "" {
		if activeProfile, found := m.Lookup(currentModelID); found && activeProfile.CodingIndex > 0 {
			minCodingIndex := activeProfile.CodingIndex - 5.0
			var qualityFiltered []ModelCuratedProfile
			for _, p := range pareto {
				if p.CodingIndex >= minCodingIndex {
					qualityFiltered = append(qualityFiltered, p)
				}
			}
			if len(qualityFiltered) > 0 {
				pareto = qualityFiltered
			}
		}
	}

	return pareto
}
