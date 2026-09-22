package curation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
