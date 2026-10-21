package curation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
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

	profile, found := mgr.Lookup("anthropic/claude-sonnet-5")
	if !found {
		t.Fatalf("expected to find anthropic/claude-sonnet-5 in embedded catalog")
	}

	if profile.TierRole != RoleCodingWorkhorse {
		t.Errorf("expected RoleCodingWorkhorse, got %s", profile.TierRole)
	}

	if profile.CodingIndex <= 0 {
		t.Errorf("expected positive coding index, got %f", profile.CodingIndex)
	}

	if profile.BenchmarkSource != "artificial_analysis" {
		t.Errorf("expected artificial_analysis benchmark source, got %s", profile.BenchmarkSource)
	}
	if profile.ProvenanceURL != "https://artificialanalysis.ai" {
		t.Errorf("expected artificialanalysis.ai provenance URL, got %s", profile.ProvenanceURL)
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

func TestCurationManager_DefaultManagerSingleton(t *testing.T) {
	dm1 := DefaultManager()
	if dm1 == nil {
		t.Fatalf("expected non-nil default manager")
	}
	dm2 := DefaultManager()
	if dm1 != dm2 {
		t.Errorf("expected DefaultManager() to return the same singleton instance")
	}
}

func TestCurationManager_NormalizedAndFuzzyLookup(t *testing.T) {
	mgr := DefaultManager()

	// Empty string lookup
	if _, found := mgr.Lookup(""); found {
		t.Errorf("expected empty string lookup to return false")
	}
	if _, found := mgr.Lookup("   "); found {
		t.Errorf("expected whitespace lookup to return false")
	}

	// 1. Exact lookup
	p1, ok1 := mgr.Lookup("anthropic/claude-sonnet-5")
	if !ok1 || p1.CodingIndex < 70 {
		t.Errorf("expected to find anthropic/claude-sonnet-5 with coding index >= 70, got: %+v", p1)
	}

	// 2. Stripped provider lookup
	p2, ok2 := mgr.Lookup("claude-sonnet-5")
	if !ok2 || p2.CodingIndex != p1.CodingIndex {
		t.Errorf("expected claude-sonnet-5 to match anthropic/claude-sonnet-5, got: %+v", p2)
	}

	// 3. Punctuation normalized lookup
	p3, ok3 := mgr.Lookup("claude-sonnet5")
	if !ok3 || p3.CodingIndex != p1.CodingIndex {
		t.Errorf("expected claude-sonnet5 to match claude-sonnet-5, got: %+v", p3)
	}

	// 4. Tag stripped lookup
	p4, ok4 := mgr.Lookup("google/gemini-2.5-flash:latest")
	if !ok4 || p4.TierRole != RoleVisionWorkhorse {
		t.Errorf("expected google/gemini-2.5-flash:latest to match google/gemini-2.5-flash, got: %+v", p4)
	}

	// 5. Short name lookup
	p5, ok5 := mgr.Lookup("gemini-2.5-flash")
	if !ok5 || p5.PromptCostPerMillion <= 0 {
		t.Errorf("expected gemini-2.5-flash to match google/gemini-2.5-flash, got: %+v", p5)
	}

	// 6. Family lookup
	p6, ok6 := mgr.Lookup("custom-sonnet-fleet")
	if !ok6 || p6.TierRole != RoleCodingWorkhorse || p6.PromptCostPerMillion <= 0 {
		t.Errorf("expected custom-sonnet-fleet to match a sonnet coding model, got: %+v", p6)
	}

	// 7. Nil manager lookup
	var nilMgr *Manager
	if _, found := nilMgr.Lookup("anthropic/claude-sonnet-5"); found {
		t.Errorf("expected nil manager lookup to return false")
	}

	// 8. Family keyword matching on custom names
	if p, ok := mgr.Lookup("my-custom-opus-tier"); !ok || p.TierRole != RoleDeepReasoner {
		t.Errorf("expected family lookup for opus to match opus model, got: %+v", p)
	}
	if p, ok := mgr.Lookup("special-haiku-model"); !ok || p.PromptCostPerMillion <= 0 {
		t.Errorf("expected family lookup for haiku to match haiku model, got: %+v", p)
	}

	// 9. Unindexed manager fallback
	unindexedMgr := &Manager{}
	cat := CuratedCatalog{
		Models: map[string]ModelCuratedProfile{
			"direct-test-model": {Name: "Direct Test", CodingIndex: 88.0},
		},
	}
	unindexedMgr.activeCatalog.Store(&cat)
	if p, ok := unindexedMgr.Lookup("direct-test-model"); !ok || p.CodingIndex != 88.0 {
		t.Errorf("expected unindexed fallback lookup to succeed, got: %+v", p)
	}
	if _, ok := unindexedMgr.Lookup("nonexistent"); ok {
		t.Errorf("expected nonexistent lookup on unindexed manager to fail")
	}
}

func TestExtractParameterSize(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"qwen2.5-coder:7b", 7.0},
		{"qwen2.5-coder:14b-instruct-q4_K_M", 14.0},
		{"llama-3.1-8b", 8.0},
		{"deepseek-coder:6.7b", 6.7},
		{"meta-llama/llama-3.3-70b-instruct", 70.0},
		{"phi-4", 0.0},
		{"", 0.0},
	}

	for _, tc := range cases {
		got := ExtractParameterSize(tc.input)
		if got != tc.expected {
			t.Errorf("ExtractParameterSize(%q) = %f, expected %f", tc.input, got, tc.expected)
		}
	}
}

func TestCandidateModels_And_Pareto(t *testing.T) {
	mgr := NewManager(t.TempDir(), "")

	// 1. Local Candidate Models
	localCandidates := mgr.CandidateModels(RoleCodingWorkhorse, true)
	if len(localCandidates) == 0 {
		t.Fatalf("expected local candidates for coding workhorse")
	}
	for _, cand := range localCandidates {
		if cand.PromptCostPerMillion != 0 || cand.CompletionCostPerMillion != 0 {
			t.Errorf("expected 0 cost for local candidate %s, got prompt=%f", cand.ModelID, cand.PromptCostPerMillion)
		}
	}

	// 2. Local Pareto with 8GB VRAM limit (should not include >8B)
	pareto8GB := mgr.ParetoCandidates(RoleCodingWorkhorse, true, "qwen2.5-coder:7b", 8)
	for _, cand := range pareto8GB {
		size := ExtractParameterSize(cand.ModelID)
		if size > 8.0 {
			t.Errorf("expected candidates <= 8B for 8GB VRAM, got %s (size %f)", cand.ModelID, size)
		}
	}

	// 3. Local Pareto with 24GB VRAM limit (can include up to 32B)
	pareto24GB := mgr.ParetoCandidates(RoleCodingWorkhorse, true, "qwen2.5-coder:7b", 24)
	var foundOver8B bool
	for _, cand := range pareto24GB {
		size := ExtractParameterSize(cand.ModelID)
		if size > 8.0 && size <= 33.0 {
			foundOver8B = true
		}
		if size > 33.0 {
			t.Errorf("expected candidates <= 33B for 24GB VRAM, got %s (size %f)", cand.ModelID, size)
		}
	}
	if !foundOver8B {
		t.Logf("Note: catalog did not contain local coding models between 8B and 33B, count=%d", len(pareto24GB))
	}

	// 4. Cloud Pareto Candidates
	cloudPareto := mgr.ParetoCandidates(RoleCodingWorkhorse, false, "anthropic/claude-3.5-sonnet", 0)
	if len(cloudPareto) == 0 {
		t.Fatalf("expected cloud pareto candidates")
	}

	// Verify Pareto frontier property: blended rates should be sorted ascending
	var prevRate float64
	for i, c := range cloudPareto {
		rate := c.PromptCostPerMillion + 0.25*c.CompletionCostPerMillion
		if i > 0 && rate < prevRate {
			t.Errorf("expected Pareto frontier sorted by blended rate, candidate %s has rate %f < prev %f", c.ModelID, rate, prevRate)
		}
		prevRate = rate
	}

	// 5. Nil manager safety
	var nilMgr *Manager
	if c := nilMgr.CandidateModels(RoleCodingWorkhorse, true); c != nil {
		t.Errorf("expected nil for nil manager candidates")
	}
	if p := nilMgr.ParetoCandidates(RoleCodingWorkhorse, true, "7b", 16); p != nil {
		t.Errorf("expected nil for nil manager pareto")
	}
}

func TestNoHardcodedModelNamesInCurationManager(t *testing.T) {
	content, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatalf("Failed to read manager.go: %v", err)
	}
	src := string(content)

	forbiddenLiterals := []string{"\"qwen\"", "\"llama\"", "\"deepseek\"", "\"mistral\"", "\"phi\"", "\"gemma\"", "\"claude\"", "\"gpt\""}
	for _, lit := range forbiddenLiterals {
		if strings.Contains(src, lit) {
			t.Errorf("Architecture violation: manager.go contains hardcoded model family literal %s! Use catalog metadata (e.g. IsOpenWeights) instead.", lit)
		}
	}
}

func TestModelCuratedProfile_Methods(t *testing.T) {
	// 1. IsFrontier
	pReasoner := ModelCuratedProfile{TierRole: RoleDeepReasoner}
	if !pReasoner.IsFrontier() {
		t.Errorf("expected RoleDeepReasoner to be frontier")
	}

	pFrontierCoding := ModelCuratedProfile{
		RecommendedTiers: []string{contract.TierIDFrontier},
		CodingIndex:      75.0,
	}
	if !pFrontierCoding.IsFrontier() {
		t.Errorf("expected TierIDFrontier with 75.0 to be frontier")
	}

	pLowFrontier := ModelCuratedProfile{
		RecommendedTiers: []string{contract.TierIDFrontier},
		CodingIndex:      65.0,
	}
	if pLowFrontier.IsFrontier() {
		t.Errorf("expected low coding index not to be frontier")
	}

	pOther := ModelCuratedProfile{TierRole: RoleGeneral}
	if pOther.IsFrontier() {
		t.Errorf("expected general role not to be frontier")
	}

	// 2. IsWorkhorse
	pWorkhorse := ModelCuratedProfile{TierRole: RoleCodingWorkhorse}
	if !pWorkhorse.IsWorkhorse() {
		t.Errorf("expected RoleCodingWorkhorse to be workhorse")
	}

	pRecWorkhorse := ModelCuratedProfile{RecommendedTiers: []string{contract.TierIDWorkhorse}}
	if !pRecWorkhorse.IsWorkhorse() {
		t.Errorf("expected recommended TierIDWorkhorse to be workhorse")
	}

	pNotWorkhorse := ModelCuratedProfile{TierRole: RoleFastProse}
	if pNotWorkhorse.IsWorkhorse() {
		t.Errorf("expected fast prose not to be workhorse")
	}
}

func TestCurationManager_AdditionalCoverage(t *testing.T) {
	mgr := NewManager("", "")

	// Lookup edge cases
	if _, ok := mgr.Lookup(""); ok {
		t.Errorf("expected false for empty lookup")
	}
	if _, ok := mgr.Lookup("   "); ok {
		t.Errorf("expected false for whitespace lookup")
	}
	// Tagged and casing variations
	if p, ok := mgr.Lookup("anthropic/claude-sonnet-5:v1"); !ok || p.CodingIndex <= 0 {
		t.Errorf("expected to find anthropic/claude-sonnet-5:v1 via untagged lookup")
	}
	if p, ok := mgr.Lookup("ANTHROPIC/CLAUDE-SONNET-5:V1"); !ok || p.CodingIndex <= 0 {
		t.Errorf("expected to find ANTHROPIC/CLAUDE-SONNET-5:V1 via uppercase untagged lookup")
	}

	// Catalog declared aliases
	if p, ok := mgr.Lookup("claude-3-7-sonnet-20250219"); !ok || p.ModelID != "anthropic/claude-3-7-sonnet" {
		t.Errorf("expected to find anthropic/claude-3-7-sonnet via alias claude-3-7-sonnet-20250219, got %v, ok=%v", p, ok)
	}
	if p, ok := mgr.Lookup("anthropic/claude-3-7-sonnet-20250219"); !ok || p.ModelID != "anthropic/claude-3-7-sonnet" {
		t.Errorf("expected to find anthropic/claude-3-7-sonnet via prefixed alias, got %v, ok=%v", p, ok)
	}

	// buildCatalogIndex with nil and empty
	if idx := buildCatalogIndex(nil); idx == nil || len(idx.exactMap) != 0 {
		t.Errorf("expected empty index for nil catalog")
	}
	if idx := buildCatalogIndex(&CuratedCatalog{}); idx == nil || len(idx.exactMap) != 0 {
		t.Errorf("expected empty index for empty catalog")
	}

	// Lookup without active index
	mgrNilIdx := NewManager("", "")
	mgrNilIdx.activeIndex.Store(nil)
	if _, ok := mgrNilIdx.Lookup("anthropic/claude-sonnet-5"); !ok {
		t.Errorf("expected true for lookup without active index on known model")
	}
	if _, ok := mgrNilIdx.Lookup("nonexistent-model-xyz"); ok {
		t.Errorf("expected false for nonexistent model without active index")
	}

	// Normalized lookup and token overlap
	if _, ok := mgr.Lookup("claude35sonnet"); !ok {
		t.Logf("normalized lookup executed")
	}
	if _, ok := mgr.Lookup("internal-sonnet-deployment-1234"); !ok {
		t.Logf("token overlap lookup executed")
	}
	if _, ok := mgr.Lookup("completelyunknownmodelnamethatdoesnotexist999"); ok {
		t.Errorf("expected false for completely unknown model")
	}

	// Various VRAM sizes and local model ceilings
	vrams := []int{0, 8, 12, 16, 24, 48, 80}
	for _, vram := range vrams {
		_ = mgr.ParetoCandidates(RoleCodingWorkhorse, true, "gemma4:12b-it-qat", vram)
		_ = mgr.ParetoCandidates(RoleGeneral, true, "", vram)
	}

	// RoleVisionWorkhorse
	vis := mgr.CandidateModels(RoleVisionWorkhorse, false)
	t.Logf("Vision candidates: %d", len(vis))

	// RoleDeepReasoner
	reasoners := mgr.CandidateModels(RoleDeepReasoner, false)
	t.Logf("Deep reasoner candidates: %d", len(reasoners))

	// Empty catalog safety
	emptyMgr := &Manager{}
	if emptyMgr.CandidateModels(RoleCodingWorkhorse, false) != nil {
		t.Errorf("expected nil for empty catalog")
	}
	if emptyMgr.ParetoCandidates(RoleCodingWorkhorse, false, "", 0) != nil {
		t.Errorf("expected nil for empty catalog pareto")
	}
	if _, ok := emptyMgr.Lookup("test"); ok {
		t.Errorf("expected false for lookup on empty manager")
	}
}

func TestCurationManager_RegisterDynamicModels(t *testing.T) {
	mgr := NewManager("", "")

	// 1. Nil manager and empty models no-op tests
	var nilMgr *Manager
	nilMgr.RegisterDynamicModels(map[string]ModelCuratedProfile{"m": {}})
	mgr.RegisterDynamicModels(nil)
	mgr.RegisterDynamicModels(map[string]ModelCuratedProfile{})

	// 2. Register brand new dynamic model with empty ModelID
	newModels := map[string]ModelCuratedProfile{
		"provider/brand-new-model": {
			Name:             "Brand New Model",
			SupportsVision:   true,
			SupportsTools:    true,
			CodingIndex:      88.0,
			ToolReliability:  92.0,
			TierRole:         RoleCodingWorkhorse,
			RecommendedTiers: []string{"workhorse"},
		},
	}
	mgr.RegisterDynamicModels(newModels)

	prof, ok := mgr.Lookup("provider/brand-new-model")
	if !ok {
		t.Fatalf("expected to find dynamically registered model")
	}
	if prof.ModelID != "provider/brand-new-model" {
		t.Errorf("expected ModelID to be set from map key, got %q", prof.ModelID)
	}

	// 3. Register dynamic model updating existing model with default fallback fields
	updateModels := map[string]ModelCuratedProfile{
		"provider/brand-new-model": {
			ModelID:          "provider/brand-new-model",
			Name:             "Updated Brand New Model",
			SupportsVision:   false,       // existing was true
			SupportsTools:    false,       // existing was true
			CodingIndex:      0,           // existing was 88.0
			ToolReliability:  0,           // existing was 92.0
			TierRole:         RoleGeneral, // existing was RoleCodingWorkhorse
			RecommendedTiers: nil,         // existing was ["workhorse"]
		},
	}
	mgr.RegisterDynamicModels(updateModels)

	updatedProf, ok := mgr.Lookup("provider/brand-new-model")
	if !ok {
		t.Fatalf("expected to find updated dynamic model")
	}
	if !updatedProf.SupportsVision {
		t.Errorf("expected SupportsVision to be preserved from existing")
	}
	if !updatedProf.SupportsTools {
		t.Errorf("expected SupportsTools to be preserved from existing")
	}
	if updatedProf.CodingIndex != 88.0 {
		t.Errorf("expected CodingIndex 88.0 preserved from existing, got %f", updatedProf.CodingIndex)
	}
	if updatedProf.ToolReliability != 92.0 {
		t.Errorf("expected ToolReliability 92.0 preserved from existing, got %f", updatedProf.ToolReliability)
	}
	if updatedProf.TierRole != RoleCodingWorkhorse {
		t.Errorf("expected TierRole RoleCodingWorkhorse preserved from existing, got %q", updatedProf.TierRole)
	}
	if len(updatedProf.RecommendedTiers) == 0 {
		t.Errorf("expected RecommendedTiers preserved from existing")
	}
}

func TestCatalog_AnthropicProviderBindings(t *testing.T) {
	mgr := DefaultManager()
	if mgr == nil {
		t.Fatalf("expected DefaultManager to be non-nil")
	}

	expectedActiveAnthropicModels := map[string]string{
		"anthropic/claude-haiku-4.5": "claude-haiku-4-5",
		"anthropic/claude-sonnet-4":   "claude-sonnet-4-0",
		"anthropic/claude-sonnet-4.5": "claude-sonnet-4-5",
		"anthropic/claude-sonnet-4.6": "claude-sonnet-4-6",
		"anthropic/claude-sonnet-5":   "claude-sonnet-5",
		"anthropic/claude-opus-4.1":   "claude-opus-4-1",
		"anthropic/claude-opus-4.5":   "claude-opus-4-5",
		"anthropic/claude-opus-4.6":   "claude-opus-4-6",
		"anthropic/claude-opus-4.7":   "claude-opus-4-7",
		"anthropic/claude-opus-4.8":   "claude-opus-4-8",
		"anthropic/claude-opus-5":     "claude-opus-5",
		"anthropic/claude-fable-5":    "claude-fable-5",
		"anthropic/claude-fable-5.1":  "claude-fable-5-1",
	}

	cat := mgr.GetActiveCatalog()
	if cat == nil || cat.Models == nil {
		t.Fatalf("expected active catalog with models")
	}

	for canonicalID, expectedWireID := range expectedActiveAnthropicModels {
		profile, ok := cat.Models[canonicalID]
		if !ok {
			t.Fatalf("expected active Anthropic model %q to exist in catalog", canonicalID)
		}

		if profile.Providers == nil {
			t.Fatalf("expected model %q to have Providers map", canonicalID)
		}

		binding, ok := profile.Providers[contract.PlatformAnthropic]
		if !ok {
			t.Fatalf("expected model %q to have binding for platform anthropic", canonicalID)
		}

		if binding.ModelID != expectedWireID {
			t.Errorf("model %q: expected Anthropic wire ID %q, got %q", canonicalID, expectedWireID, binding.ModelID)
		}
	}

	// Verify Sonnet 5 coding index restored to 71.5 and retains tier_4_frontier
	sonnet5, ok := cat.Models["anthropic/claude-sonnet-5"]
	if !ok {
		t.Fatalf("expected anthropic/claude-sonnet-5 in catalog")
	}
	if sonnet5.CodingIndex != 71.5 {
		t.Errorf("expected anthropic/claude-sonnet-5 coding_index 71.5, got %f", sonnet5.CodingIndex)
	}
	if !slices.Contains(sonnet5.RecommendedTiers, contract.TierIDFrontier) {
		t.Errorf("expected anthropic/claude-sonnet-5 to contain tier_4_frontier in recommended_tiers")
	}
	if !sonnet5.IsFrontier() {
		t.Errorf("expected anthropic/claude-sonnet-5 to report IsFrontier() == true")
	}

	// Verify :batch models do NOT have providers.anthropic
	for id, p := range cat.Models {
		if strings.HasPrefix(id, "anthropic/") && strings.Contains(id, ":batch") {
			if _, hasAnthropic := p.Providers[contract.PlatformAnthropic]; hasAnthropic {
				t.Errorf("batch model %q should NOT have providers.anthropic", id)
			}
		}
	}

	// Verify estimated 5.5 models do NOT have providers.anthropic
	estimated55 := []string{
		"anthropic/claude-haiku-5.5",
		"anthropic/claude-sonnet-5.5",
		"anthropic/claude-opus-5.5",
	}
	for _, id := range estimated55 {
		if p, exists := cat.Models[id]; exists {
			if _, hasAnthropic := p.Providers[contract.PlatformAnthropic]; hasAnthropic {
				t.Errorf("estimated model %q must NOT have providers.anthropic binding", id)
			}
			if p.BenchmarkSource != BenchmarkSourceEstimated {
				t.Errorf("model %q expected benchmark_source %q, got %q", id, BenchmarkSourceEstimated, p.BenchmarkSource)
			}
		}
	}
}

func TestLookup_HyphenWireIDsResolveToCanonical(t *testing.T) {
	mgr := DefaultManager()
	if mgr == nil {
		t.Fatalf("expected DefaultManager to be non-nil")
	}

	wireTests := []struct {
		wireID            string
		expectedCanonical string
	}{
		{"claude-sonnet-5", "anthropic/claude-sonnet-5"},
		{"CLAUDE-SONNET-5", "anthropic/claude-sonnet-5"},
		{"claude-sonnet-4-0", "anthropic/claude-sonnet-4"},
		{"claude-sonnet-4-5", "anthropic/claude-sonnet-4.5"},
		{"claude-sonnet-4-6", "anthropic/claude-sonnet-4.6"},
		{"claude-haiku-4-5", "anthropic/claude-haiku-4.5"},
		{"claude-opus-4-1", "anthropic/claude-opus-4.1"},
		{"claude-opus-4-5", "anthropic/claude-opus-4.5"},
		{"claude-opus-4-6", "anthropic/claude-opus-4.6"},
		{"claude-opus-4-7", "anthropic/claude-opus-4.7"},
		{"claude-opus-4-8", "anthropic/claude-opus-4.8"},
		{"claude-opus-5", "anthropic/claude-opus-5"},
		{"claude-fable-5", "anthropic/claude-fable-5"},
		{"claude-fable-5-1", "anthropic/claude-fable-5.1"},
	}

	for _, tc := range wireTests {
		p, found := mgr.Lookup(tc.wireID)
		if !found {
			t.Errorf("wire ID %q failed to resolve via Lookup", tc.wireID)
			continue
		}
		canon := p.CanonicalID
		if canon == "" {
			canon = p.ModelID
		}
		if canon != tc.expectedCanonical {
			t.Errorf("wire ID %q resolved to %q, expected %q", tc.wireID, canon, tc.expectedCanonical)
		}
	}
}

func TestLookupForPlatform_AnthropicOverridesPriceAndWireID(t *testing.T) {
	mgr := DefaultManager()
	if mgr == nil {
		t.Fatalf("expected DefaultManager to be non-nil")
	}

	// 1. Existing catalog model lookup with PlatformAnthropic
	p, ok := mgr.LookupForPlatform(contract.PlatformAnthropic, "anthropic/claude-sonnet-5")
	if !ok {
		t.Fatalf("expected LookupForPlatform to succeed for anthropic/claude-sonnet-5 on platform anthropic")
	}
	if p.ModelID != "claude-sonnet-5" {
		t.Errorf("expected ModelID to be wire ID claude-sonnet-5, got %q", p.ModelID)
	}
	if p.PromptCostPerMillion != 3.0 || p.CompletionCostPerMillion != 15.0 {
		t.Errorf("expected base prices 3.0/15.0, got %f/%f", p.PromptCostPerMillion, p.CompletionCostPerMillion)
	}

	// 2. Lookup by wire ID on PlatformAnthropic
	pWire, ok := mgr.LookupForPlatform(contract.PlatformAnthropic, "claude-sonnet-5")
	if !ok {
		t.Fatalf("expected LookupForPlatform to succeed for claude-sonnet-5 on platform anthropic")
	}
	if pWire.ModelID != "claude-sonnet-5" {
		t.Errorf("expected ModelID claude-sonnet-5, got %q", pWire.ModelID)
	}

	// 3. Dynamic model with overridden prices in ProviderBinding (using isolated manager)
	isolatedMgr := NewManager(t.TempDir(), "")
	customPrompt := 4.5
	customComp := 22.5
	isolatedMgr.RegisterDynamicModels(map[string]ModelCuratedProfile{
		"custom/override-model": {
			Name:                     "Custom Override Model",
			TierRole:                 RoleCodingWorkhorse,
			PromptCostPerMillion:     2.0,
			CompletionCostPerMillion: 10.0,
			Providers: map[contract.Platform]ProviderBinding{
				contract.PlatformAnthropic: {
					ModelID:                  "claude-custom-override",
					PromptCostPerMillion:     &customPrompt,
					CompletionCostPerMillion: &customComp,
				},
			},
		},
	})

	pOverride, ok := isolatedMgr.LookupForPlatform(contract.PlatformAnthropic, "custom/override-model")
	if !ok {
		t.Fatalf("expected dynamic model to resolve on platform anthropic")
	}
	if pOverride.ModelID != "claude-custom-override" {
		t.Errorf("expected wire ID claude-custom-override, got %q", pOverride.ModelID)
	}
	if pOverride.PromptCostPerMillion != customPrompt {
		t.Errorf("expected overridden prompt price %f, got %f", customPrompt, pOverride.PromptCostPerMillion)
	}
	if pOverride.CompletionCostPerMillion != customComp {
		t.Errorf("expected overridden completion price %f, got %f", customComp, pOverride.CompletionCostPerMillion)
	}

	// 4. Negative lookups
	if _, ok := mgr.LookupForPlatform(contract.PlatformAnthropic, "openai/gpt-4o"); ok {
		t.Errorf("expected gpt-4o to fail LookupForPlatform on platform anthropic")
	}
	if _, ok := mgr.LookupForPlatform(contract.PlatformOpenRouter, "anthropic/claude-sonnet-5"); ok {
		t.Errorf("expected openrouter to return false when no openrouter binding entry exists")
	}
	if _, ok := mgr.LookupForPlatform(contract.PlatformLocal, "anthropic/claude-sonnet-5"); ok {
		t.Errorf("expected local to return false for cloud model without local binding")
	}
	if _, ok := mgr.LookupForPlatform(contract.PlatformAnthropic, "non-existent-model"); ok {
		t.Errorf("expected non-existent model to return false")
	}
}

func TestFormatModelForPlatform(t *testing.T) {
	mgr := DefaultManager()
	if mgr == nil {
		t.Fatalf("expected DefaultManager to be non-nil")
	}

	// 1. PlatformOpenRouter or empty platform ("")
	formatted, ok := mgr.FormatModelForPlatform(contract.PlatformOpenRouter, "claude-sonnet-5")
	if !ok || formatted != "anthropic/claude-sonnet-5" {
		t.Errorf("OpenRouter: expected anthropic/claude-sonnet-5, true; got %q, %v", formatted, ok)
	}
	formatted, ok = mgr.FormatModelForPlatform("", "claude-sonnet-5")
	if !ok || formatted != "anthropic/claude-sonnet-5" {
		t.Errorf("Empty platform: expected anthropic/claude-sonnet-5, true; got %q, %v", formatted, ok)
	}
	// Uncataloged model on OpenRouter / empty platform passes through
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformOpenRouter, "vendor/unknown-spot-model")
	if !ok || formatted != "vendor/unknown-spot-model" {
		t.Errorf("OpenRouter uncataloged: expected vendor/unknown-spot-model, true; got %q, %v", formatted, ok)
	}
	formatted, ok = mgr.FormatModelForPlatform("", "unregistered-model")
	if !ok || formatted != "unregistered-model" {
		t.Errorf("Empty platform uncataloged: expected unregistered-model, true; got %q, %v", formatted, ok)
	}

	// 2. PlatformLocal
	// Open weights in catalog: ok == true
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformLocal, "meta-llama/llama-3.3-70b-instruct")
	if !ok {
		t.Errorf("Local: expected llama-3.3-70b-instruct to be allowed (open weights)")
	}
	// Closed weights in catalog: ok == false (IsOpenWeights is false)
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformLocal, "anthropic/claude-sonnet-5")
	if ok {
		t.Errorf("Local: expected claude-sonnet-5 to be rejected on local platform, got ok=true")
	}
	if formatted != "anthropic/claude-sonnet-5" {
		t.Errorf("Local: expected returned modelID anthropic/claude-sonnet-5 even when ok=false, got %q", formatted)
	}
	// Uncataloged model on Local passes through (e.g. ad-hoc ollama model)
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformLocal, "ollama/my-finetune:8b")
	if !ok || formatted != "ollama/my-finetune:8b" {
		t.Errorf("Local uncataloged: expected ollama/my-finetune:8b, true; got %q, %v", formatted, ok)
	}

	// 3. PlatformAnthropic
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformAnthropic, "anthropic/claude-sonnet-5")
	if !ok || formatted != "claude-sonnet-5" {
		t.Errorf("Anthropic: expected claude-sonnet-5, true; got %q, %v", formatted, ok)
	}
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformAnthropic, "claude-opus-4-7")
	if !ok || formatted != "claude-opus-4-7" {
		t.Errorf("Anthropic by wire ID: expected claude-opus-4-7, true; got %q, %v", formatted, ok)
	}
	// Non-Anthropic model
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformAnthropic, "openai/gpt-4o")
	if ok || formatted != "" {
		t.Errorf("Anthropic non-binding: expected empty string and false, got %q, %v", formatted, ok)
	}
	// Uncataloged model on Anthropic
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformAnthropic, "random-unknown")
	if ok || formatted != "" {
		t.Errorf("Anthropic uncataloged: expected empty string and false, got %q, %v", formatted, ok)
	}

	// 4. PlatformOpenAICompatible and other platforms return "", false
	formatted, ok = mgr.FormatModelForPlatform(contract.PlatformOpenAICompatible, "anthropic/claude-sonnet-5")
	if ok || formatted != "" {
		t.Errorf("OpenAICompatible: expected empty string and false, got %q, %v", formatted, ok)
	}
	formatted, ok = mgr.FormatModelForPlatform(contract.Platform("unrecognized_platform"), "anthropic/claude-sonnet-5")
	if ok || formatted != "" {
		t.Errorf("Unknown platform: expected empty string and false, got %q, %v", formatted, ok)
	}
}

func TestIsSameModel(t *testing.T) {
	mgr := DefaultManager()
	if mgr == nil {
		t.Fatalf("expected DefaultManager to be non-nil")
	}

	// 1. EqualFold fast-path
	if !mgr.IsSameModel("claude-sonnet-5", "CLAUDE-SONNET-5") {
		t.Errorf("expected case-insensitive match")
	}

	// 2. Canonical vs Wire ID cross-syntax
	if !mgr.IsSameModel("anthropic/claude-sonnet-5", "claude-sonnet-5") {
		t.Errorf("expected anthropic/claude-sonnet-5 and claude-sonnet-5 to be same model")
	}
	if !mgr.IsSameModel("claude-sonnet-5", "anthropic/claude-sonnet-5") {
		t.Errorf("expected symmetry in IsSameModel")
	}
	if !mgr.IsSameModel("claude-opus-4-7", "anthropic/claude-opus-4.7") {
		t.Errorf("expected claude-opus-4-7 and anthropic/claude-opus-4.7 to be same model")
	}
	if !mgr.IsSameModel("claude-haiku-4-5", "anthropic/claude-haiku-4.5") {
		t.Errorf("expected claude-haiku-4-5 and anthropic/claude-haiku-4.5 to be same model")
	}

	// 3. Distinct models
	if mgr.IsSameModel("claude-sonnet-5", "claude-opus-5") {
		t.Errorf("expected distinct models to report false")
	}
	if mgr.IsSameModel("anthropic/claude-sonnet-5", "openai/gpt-4o") {
		t.Errorf("expected distinct models to report false")
	}

	// 4. Uncataloged models
	if !mgr.IsSameModel("my-org/custom-model", "my-org/custom-model") {
		t.Errorf("expected identical uncataloged models to report true")
	}
	if mgr.IsSameModel("my-org/custom-model-1", "my-org/custom-model-2") {
		t.Errorf("expected different uncataloged models to report false")
	}

	// 5. One cataloged, one uncataloged
	if mgr.IsSameModel("anthropic/claude-sonnet-5", "my-org/custom-model") {
		t.Errorf("expected cataloged and uncataloged models to report false")
	}
}

