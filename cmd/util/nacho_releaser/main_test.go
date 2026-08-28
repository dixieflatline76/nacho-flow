package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v63/github"
)

func TestResolveToken(t *testing.T) {
	// 1. NACHO_RELEASER_TOKEN
	tok, err := resolveToken(func(k string) string {
		if k == "NACHO_RELEASER_TOKEN" {
			return "tok1"
		}
		return ""
	})
	if err != nil || tok != "tok1" {
		t.Fatalf("expected tok1, got %s (%v)", tok, err)
	}

	// 2. GORELEASER_GITHUB_TOKEN
	tok, err = resolveToken(func(k string) string {
		if k == "GORELEASER_GITHUB_TOKEN" {
			return "tok2"
		}
		return ""
	})
	if err != nil || tok != "tok2" {
		t.Fatalf("expected tok2, got %s (%v)", tok, err)
	}

	// 3. GITHUB_TOKEN
	tok, err = resolveToken(func(k string) string {
		if k == "GITHUB_TOKEN" {
			return "tok3"
		}
		return ""
	})
	if err != nil || tok != "tok3" {
		t.Fatalf("expected tok3, got %s (%v)", tok, err)
	}

	// 4. Missing token error
	_, err = resolveToken(func(k string) string { return "" })
	if err == nil {
		t.Fatalf("expected error when no token present, got nil")
	}
}

func TestResolveTag(t *testing.T) {
	// 1. Tag from GITHUB_REF_NAME
	tag, ver, err := resolveTag(func(k string) string {
		if k == "GITHUB_REF_NAME" {
			return "v0.5.2"
		}
		return ""
	}, nil)
	if err != nil || tag != "v0.5.2" || ver != "0.5.2" {
		t.Fatalf("expected v0.5.2 / 0.5.2, got %s / %s (%v)", tag, ver, err)
	}

	// 2. Tag from version.txt
	tag, ver, err = resolveTag(func(k string) string { return "" }, func(p string) ([]byte, error) {
		return []byte("0.5.2\n"), nil
	})
	if err != nil || tag != "v0.5.2" || ver != "0.5.2" {
		t.Fatalf("expected v0.5.2 / 0.5.2 from version.txt, got %s / %s (%v)", tag, ver, err)
	}

	// 3. Error when version.txt fails to read
	_, _, err = resolveTag(func(k string) string { return "" }, func(p string) ([]byte, error) {
		return nil, errors.New("read error")
	})
	if err == nil {
		t.Fatalf("expected error on read failure")
	}

	// 4. Error when tag does not start with v
	_, _, err = resolveTag(func(k string) string {
		if k == "GITHUB_REF_NAME" {
			return "invalid-tag"
		}
		return ""
	}, nil)
	if err == nil {
		t.Fatalf("expected error for tag not starting with 'v'")
	}
}

func TestCollectArtifactPaths(t *testing.T) {
	paths := collectArtifactPaths("dist", "0.5.2")
	if len(paths) != 11 {
		t.Fatalf("expected 11 artifact paths, got %d", len(paths))
	}
	expectedWindows := filepath.Join("dist", "nacho-flow-0.5.2-windows-amd64.exe")
	if paths[0] != expectedWindows {
		t.Errorf("expected %s, got %s", expectedWindows, paths[0])
	}
	expectedVsix := filepath.Join("dist", "nacho-flow-0.5.2-win32-x64.vsix")
	if paths[5] != expectedVsix {
		t.Errorf("expected %s, got %s", expectedVsix, paths[5])
	}
}

func TestGenerateChecksums(t *testing.T) {
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "file1.bin")
	f2 := filepath.Join(tmpDir, "file2.bin")
	missing := filepath.Join(tmpDir, "missing.bin")

	if err := os.WriteFile(f1, []byte("data1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("data2"), 0600); err != nil {
		t.Fatal(err)
	}

	hashes, buf, err := generateChecksums([]string{f1, f2, missing})
	if err != nil {
		t.Fatalf("generateChecksums failed: %v", err)
	}

	if len(hashes) != 2 {
		t.Errorf("expected 2 hashes (ignoring missing), got %d", len(hashes))
	}

	if !strings.Contains(string(buf), "file1.bin") || !strings.Contains(string(buf), "file2.bin") {
		t.Errorf("missing checksum entries in buffer: %s", string(buf))
	}
}

func TestRenderTemplate_WingetVersion(t *testing.T) {
	data := struct {
		Version string
	}{Version: "0.5.2"}

	b, err := renderTemplate("templates/winget_version.yaml.tmpl", data)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	content := string(b)
	if !strings.Contains(content, "PackageIdentifier: dixieflatline76.NachoFlow") {
		t.Errorf("expected PackageIdentifier in version manifest, got: %s", content)
	}
	if !strings.Contains(content, "PackageVersion: 0.5.2") {
		t.Errorf("expected PackageVersion: 0.5.2, got: %s", content)
	}
	if !strings.Contains(content, "DefaultLocale: en-US") {
		t.Errorf("expected DefaultLocale: en-US, got: %s", content)
	}
}

func TestRenderTemplate_WingetInstaller(t *testing.T) {
	data := struct {
		Version string
		WinHash string
	}{
		Version: "0.5.2",
		WinHash: "0DD8AB8148A2AECFDC06714AA3ACD1F28C7EB3C514303CCEAFC8EB3E2F335F53",
	}

	b, err := renderTemplate("templates/winget_installer.yaml.tmpl", data)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	content := string(b)
	if !strings.Contains(content, "PackageVersion: 0.5.2") {
		t.Errorf("missing version: %s", content)
	}
	if !strings.Contains(content, "Commands:") || !strings.Contains(content, "- nacho-flow") {
		t.Errorf("missing Commands alias in installer manifest: %s", content)
	}
	if !strings.Contains(content, "0DD8AB8148A2AECFDC06714AA3ACD1F28C7EB3C514303CCEAFC8EB3E2F335F53") {
		t.Errorf("missing SHA256 hash in installer manifest: %s", content)
	}
	if !strings.Contains(content, "InstallerType: portable") {
		t.Errorf("expected InstallerType: portable, got: %s", content)
	}
}

func TestRenderTemplate_WingetLocale(t *testing.T) {
	data := struct {
		Version string
	}{Version: "0.5.2"}

	b, err := renderTemplate("templates/winget_locale.yaml.tmpl", data)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	content := string(b)
	if !strings.Contains(content, "PackageUrl: https://spicebox.dev/nacho-flow/") {
		t.Errorf("expected PackageUrl: https://spicebox.dev/nacho-flow/, got: %s", content)
	}
	if !strings.Contains(content, "PublisherUrl: https://spicebox.dev") {
		t.Errorf("expected PublisherUrl: https://spicebox.dev, got: %s", content)
	}
	if !strings.Contains(content, "PublisherSupportUrl: https://github.com/dixieflatline76/nacho-flow/issues") {
		t.Errorf("expected PublisherSupportUrl, got: %s", content)
	}
	if !strings.Contains(content, "License: AGPL-3.0-or-later") {
		t.Errorf("expected License: AGPL-3.0-or-later, got: %s", content)
	}
}

func TestRenderTemplate_HomebrewFormula(t *testing.T) {
	data := struct {
		Version     string
		DarwinArm64 string
		DarwinAmd64 string
		LinuxAmd64  string
		LinuxArm64  string
	}{
		Version:     "0.5.2",
		DarwinArm64: "arm64_mac_hash",
		DarwinAmd64: "amd64_mac_hash",
		LinuxAmd64:  "amd64_linux_hash",
		LinuxArm64:  "arm64_linux_hash",
	}

	b, err := renderTemplate("templates/homebrew_formula.rb.tmpl", data)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	content := string(b)
	if !strings.Contains(content, `homepage "https://spicebox.dev/nacho-flow/"`) {
		t.Errorf("expected homepage https://spicebox.dev/nacho-flow/, got: %s", content)
	}
	if !strings.Contains(content, `version "0.5.2"`) {
		t.Errorf("expected version 0.5.2, got: %s", content)
	}
	if !strings.Contains(content, `sha256 "arm64_mac_hash"`) || !strings.Contains(content, `sha256 "amd64_mac_hash"`) {
		t.Errorf("missing mac hashes in formula: %s", content)
	}
	if !strings.Contains(content, `sha256 "amd64_linux_hash"`) || !strings.Contains(content, `sha256 "arm64_linux_hash"`) {
		t.Errorf("missing linux hashes in formula: %s", content)
	}
}

func TestHashFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "sample.bin")
	sampleData := []byte("hello nacho-flow releaser test")
	if err := os.WriteFile(testFile, sampleData, 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	expectedHasher := sha256.New()
	expectedHasher.Write(sampleData)
	expectedHash := hex.EncodeToString(expectedHasher.Sum(nil))

	actualHash, err := hashFile(testFile)
	if err != nil {
		t.Fatalf("hashFile failed: %v", err)
	}
	if actualHash != expectedHash {
		t.Errorf("expected hash %s, got %s", expectedHash, actualHash)
	}

	// Test non-existent file returns error
	_, err = hashFile(filepath.Join(tmpDir, "does-not-exist.bin"))
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func TestRenderTemplate_MissingTemplate_ReturnsError(t *testing.T) {
	_, err := renderTemplate("templates/non_existent.tmpl", nil)
	if err == nil {
		t.Errorf("expected error for non-existent template, got nil")
	}
}

func TestPushHomebrewFormula_MissingHashes(t *testing.T) {
	pushHomebrewFormula(context.Background(), nil, "0.5.2", map[string]string{})
}

func TestPushHomebrewFormula_CreateAndExisting(t *testing.T) {
	hashes := map[string]string{
		"nacho-flow-0.5.2-darwin-arm64": "hash1",
		"nacho-flow-0.5.2-darwin-amd64": "hash2",
		"nacho-flow-0.5.2-linux-amd64":  "hash3",
		"nacho-flow-0.5.2-linux-arm64":  "hash4",
	}

	// 1. Create file path (404 on GET)
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == "PUT" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(github.RepositoryContentResponse{})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts1.Close()

	client1 := github.NewClient(nil)
	u1, _ := url.Parse(ts1.URL + "/")
	client1.BaseURL = u1
	pushHomebrewFormula(context.Background(), client1, "0.5.2", hashes)

	// 2. Update existing file path (200 on GET)
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			fileContent := github.RepositoryContent{SHA: github.String("oldsha")}
			_ = json.NewEncoder(w).Encode(fileContent)
			return
		}
		if r.Method == "PUT" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(github.RepositoryContentResponse{})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts2.Close()

	client2 := github.NewClient(nil)
	u2, _ := url.Parse(ts2.URL + "/")
	client2.BaseURL = u2
	pushHomebrewFormula(context.Background(), client2, "0.5.2", hashes)
}

func TestPushWingetManifests(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// GET base branch ref
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/git/ref/heads/master") {
			_ = json.NewEncoder(w).Encode(github.Reference{Object: &github.GitObject{SHA: github.String("base_sha")}})
			return
		}
		// DELETE old branch ref
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// POST new branch ref
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/git/refs") {
			_ = json.NewEncoder(w).Encode(github.Reference{Object: &github.GitObject{SHA: github.String("base_sha")}})
			return
		}
		// POST tree
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/git/trees") {
			_ = json.NewEncoder(w).Encode(github.Tree{SHA: github.String("tree_sha")})
			return
		}
		// POST commit
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/git/commits") {
			_ = json.NewEncoder(w).Encode(github.Commit{SHA: github.String("commit_sha")})
			return
		}
		// PATCH update branch ref
		if r.Method == "PATCH" && strings.Contains(r.URL.Path, "/git/refs/heads/") {
			_ = json.NewEncoder(w).Encode(github.Reference{Object: &github.GitObject{SHA: github.String("commit_sha")}})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	u, _ := url.Parse(ts.URL + "/")
	client.BaseURL = u

	pushWingetManifests(context.Background(), client, "0.5.2", "ABCD1234SHA")
}

func TestEnsureRelease_FoundExisting(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/tags/v0.5.2") {
			w.Header().Set("Content-Type", "application/json")
			rel := github.RepositoryRelease{
				ID:      github.Int64(12345),
				TagName: github.String("v0.5.2"),
			}
			_ = json.NewEncoder(w).Encode(rel)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	serverURL, _ := url.Parse(ts.URL + "/")
	client.BaseURL = serverURL

	rel, err := ensureRelease(context.Background(), client, "owner", "repo", "v0.5.2")
	if err != nil {
		t.Fatalf("ensureRelease failed: %v", err)
	}
	if rel.GetID() != 12345 {
		t.Errorf("expected release ID 12345, got %d", rel.GetID())
	}
}

func TestEnsureRelease_CreateWhenNotFound(t *testing.T) {
	created := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/releases/tags/v0.5.2") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/releases") {
			created = true
			w.Header().Set("Content-Type", "application/json")
			rel := github.RepositoryRelease{
				ID:      github.Int64(67890),
				TagName: github.String("v0.5.2"),
			}
			_ = json.NewEncoder(w).Encode(rel)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	serverURL, _ := url.Parse(ts.URL + "/")
	client.BaseURL = serverURL

	rel, err := ensureRelease(context.Background(), client, "owner", "repo", "v0.5.2")
	if err != nil {
		t.Fatalf("ensureRelease failed: %v", err)
	}
	if !created || rel.GetID() != 67890 {
		t.Errorf("expected newly created release ID 67890, got %d", rel.GetID())
	}
}

func TestEnsureRelease_ErrorPaths(t *testing.T) {
	// Server returns 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	u, _ := url.Parse(ts.URL + "/")
	client.BaseURL = u

	_, err := ensureRelease(context.Background(), client, "owner", "repo", "v0.5.2")
	if err == nil {
		t.Errorf("expected error on 500 response, got nil")
	}
}

func TestUploadAsset(t *testing.T) {
	tmpDir := t.TempDir()
	assetFile := filepath.Join(tmpDir, "nacho-flow.bin")
	_ = os.WriteFile(assetFile, []byte("asset-content"), 0600)

	uploaded := false
	deletedOld := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// List assets
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/releases/123/assets") {
			w.Header().Set("Content-Type", "application/json")
			assets := []*github.ReleaseAsset{
				{ID: github.Int64(999), Name: github.String("nacho-flow.bin")},
			}
			_ = json.NewEncoder(w).Encode(assets)
			return
		}
		// Delete asset
		if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/releases/assets/999") {
			deletedOld = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Upload asset
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/releases/123/assets") {
			uploaded = true
			w.Header().Set("Content-Type", "application/json")
			asset := github.ReleaseAsset{ID: github.Int64(1001), Name: github.String("nacho-flow.bin")}
			_ = json.NewEncoder(w).Encode(asset)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	serverURL, _ := url.Parse(ts.URL + "/")
	client.BaseURL = serverURL
	client.UploadURL = serverURL

	// 1. Success path
	uploadAsset(context.Background(), client, "owner", "repo", 123, assetFile)
	if !deletedOld || !uploaded {
		t.Errorf("expected old asset deleted (%v) and new uploaded (%v)", deletedOld, uploaded)
	}

	// 2. Non-existent file error path
	uploadAsset(context.Background(), client, "owner", "repo", 123, filepath.Join(tmpDir, "missing.bin"))
}

func TestRun_Stage1_Prerelease(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "nacho-flow-0.5.2-windows-amd64.exe")
	_ = os.WriteFile(binPath, []byte("fake-win-binary"), 0600)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/releases/tags/v0.5.2") {
			rel := github.RepositoryRelease{ID: github.Int64(555), TagName: github.String("v0.5.2")}
			_ = json.NewEncoder(w).Encode(rel)
			return
		}
		if strings.Contains(r.URL.Path, "/releases/555/assets") {
			if r.Method == "GET" {
				_ = json.NewEncoder(w).Encode([]*github.ReleaseAsset{})
				return
			}
			if r.Method == "POST" {
				_ = json.NewEncoder(w).Encode(github.ReleaseAsset{ID: github.Int64(777)})
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	u, _ := url.Parse(ts.URL + "/")
	client.BaseURL = u
	client.UploadURL = u

	// In Stage 1: skipDistribution=true (only upload assets, no external git calls)
	err := run(context.Background(), client, "tok", "v0.5.2", "0.5.2", tmpDir, true)
	if err != nil {
		t.Fatalf("run failed in Stage 1: %v", err)
	}
}

func TestRun_Stage2_Latest(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "nacho-flow-0.5.2-windows-amd64.exe")
	_ = os.WriteFile(binPath, []byte("fake-win-binary"), 0600)

	// Create linux and mac binaries so homebrew formula has all hashes
	_ = os.WriteFile(filepath.Join(tmpDir, "nacho-flow-0.5.2-darwin-arm64"), []byte("mac-arm"), 0600)
	_ = os.WriteFile(filepath.Join(tmpDir, "nacho-flow-0.5.2-darwin-amd64"), []byte("mac-intel"), 0600)
	_ = os.WriteFile(filepath.Join(tmpDir, "nacho-flow-0.5.2-linux-amd64"), []byte("linux-amd"), 0600)
	_ = os.WriteFile(filepath.Join(tmpDir, "nacho-flow-0.5.2-linux-arm64"), []byte("linux-arm"), 0600)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/releases/tags/v0.5.2") {
			rel := github.RepositoryRelease{ID: github.Int64(555), TagName: github.String("v0.5.2")}
			_ = json.NewEncoder(w).Encode(rel)
			return
		}
		if strings.Contains(r.URL.Path, "/releases/555/assets") {
			if r.Method == "GET" {
				_ = json.NewEncoder(w).Encode([]*github.ReleaseAsset{})
				return
			}
			if r.Method == "POST" {
				_ = json.NewEncoder(w).Encode(github.ReleaseAsset{ID: github.Int64(777)})
				return
			}
		}
		// Homebrew formula endpoints
		if strings.Contains(r.URL.Path, "/contents/Formula/nacho-flow.rb") {
			if r.Method == "GET" {
				_ = json.NewEncoder(w).Encode(github.RepositoryContent{SHA: github.String("old_rb_sha")})
				return
			}
			if r.Method == "PUT" {
				_ = json.NewEncoder(w).Encode(github.RepositoryContentResponse{})
				return
			}
		}
		// Winget git endpoints
		if strings.Contains(r.URL.Path, "/git/ref/heads/master") {
			_ = json.NewEncoder(w).Encode(github.Reference{Object: &github.GitObject{SHA: github.String("base_sha")}})
			return
		}
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.Contains(r.URL.Path, "/git/refs") {
			_ = json.NewEncoder(w).Encode(github.Reference{Object: &github.GitObject{SHA: github.String("base_sha")}})
			return
		}
		if strings.Contains(r.URL.Path, "/git/trees") {
			_ = json.NewEncoder(w).Encode(github.Tree{SHA: github.String("tree_sha")})
			return
		}
		if strings.Contains(r.URL.Path, "/git/commits") {
			_ = json.NewEncoder(w).Encode(github.Commit{SHA: github.String("commit_sha")})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	u, _ := url.Parse(ts.URL + "/")
	client.BaseURL = u
	client.UploadURL = u

	// In Stage 2: skipDistribution=false (runs full asset upload + Homebrew + Winget distribution)
	err := run(context.Background(), client, "tok", "v0.5.2", "0.5.2", tmpDir, false)
	if err != nil {
		t.Fatalf("run failed in Stage 2: %v", err)
	}
}

func TestPushWingetManifests_Errors(t *testing.T) {
	// 1. GetRef fails
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts1.Close()
	c1 := github.NewClient(nil)
	u1, _ := url.Parse(ts1.URL + "/")
	c1.BaseURL = u1
	pushWingetManifests(context.Background(), c1, "0.5.2", "HASH")

	// 2. CreateTree fails
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/git/ref/heads/master") {
			_ = json.NewEncoder(w).Encode(github.Reference{Object: &github.GitObject{SHA: github.String("base_sha")}})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/git/trees") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}))
	defer ts2.Close()
	c2 := github.NewClient(nil)
	u2, _ := url.Parse(ts2.URL + "/")
	c2.BaseURL = u2
	pushWingetManifests(context.Background(), c2, "0.5.2", "HASH")

	// 3. CreateCommit fails
	ts3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/git/ref/heads/master") {
			_ = json.NewEncoder(w).Encode(github.Reference{Object: &github.GitObject{SHA: github.String("base_sha")}})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/git/trees") {
			_ = json.NewEncoder(w).Encode(github.Tree{SHA: github.String("tree_sha")})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/git/commits") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}))
	defer ts3.Close()
	c3 := github.NewClient(nil)
	u3, _ := url.Parse(ts3.URL + "/")
	c3.BaseURL = u3
	pushWingetManifests(context.Background(), c3, "0.5.2", "HASH")

	// 4. UpdateRef fails
	ts4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/git/ref/heads/master") {
			_ = json.NewEncoder(w).Encode(github.Reference{Object: &github.GitObject{SHA: github.String("base_sha")}})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/git/trees") {
			_ = json.NewEncoder(w).Encode(github.Tree{SHA: github.String("tree_sha")})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/git/commits") {
			_ = json.NewEncoder(w).Encode(github.Commit{SHA: github.String("commit_sha")})
			return
		}
		if r.Method == "PATCH" && strings.Contains(r.URL.Path, "/git/refs/heads/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}))
	defer ts4.Close()
	c4 := github.NewClient(nil)
	u4, _ := url.Parse(ts4.URL + "/")
	c4.BaseURL = u4
	pushWingetManifests(context.Background(), c4, "0.5.2", "HASH")
}

func TestPushHomebrewFormula_Errors(t *testing.T) {
	hashes := map[string]string{
		"nacho-flow-0.5.2-darwin-arm64": "h1",
		"nacho-flow-0.5.2-darwin-amd64": "h2",
		"nacho-flow-0.5.2-linux-amd64":  "h3",
		"nacho-flow-0.5.2-linux-arm64":  "h4",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := github.NewClient(nil)
	u, _ := url.Parse(ts.URL + "/")
	c.BaseURL = u
	pushHomebrewFormula(context.Background(), c, "0.5.2", hashes)
}

func TestRun_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Release creation error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := github.NewClient(nil)
	u, _ := url.Parse(ts.URL + "/")
	c.BaseURL = u

	err := run(context.Background(), c, "tok", "v0.5.2", "0.5.2", tmpDir, true)
	if err == nil {
		t.Errorf("expected error on release failure, got nil")
	}

	// 2. Checksum generation error (e.g., directory instead of file)
	badDir := filepath.Join(tmpDir, "nacho-flow-0.5.2-windows-amd64.exe")
	_ = os.MkdirAll(badDir, 0755)
	err = run(context.Background(), c, "tok", "v0.5.2", "0.5.2", tmpDir, true)
	if err == nil {
		t.Errorf("expected error on checksum generation failure, got nil")
	}
}

func TestEnsureRelease_CreateFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == "POST" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	u, _ := url.Parse(ts.URL + "/")
	client.BaseURL = u

	_, err := ensureRelease(context.Background(), client, "owner", "repo", "v0.5.2")
	if err == nil {
		t.Errorf("expected error when creation fails, got nil")
	}
}

func TestGenerateChecksums_UnreadableFileError(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "directory.bin")
	_ = os.MkdirAll(subDir, 0755)

	_, _, err := generateChecksums([]string{subDir})
	if err == nil {
		t.Errorf("expected error when hashing a directory path, got nil")
	}
}

func TestRunCLI(t *testing.T) {
	// 1. Missing token error
	err := runCLI(func(k string) string { return "" }, nil)
	if err == nil {
		t.Errorf("expected error on missing token, got nil")
	}

	// 2. Invalid tag error
	err = runCLI(func(k string) string {
		if k == "NACHO_RELEASER_TOKEN" {
			return "tok"
		}
		if k == "GITHUB_REF_NAME" {
			return "badtag"
		}
		return ""
	}, nil)
	if err == nil {
		t.Errorf("expected error on invalid tag, got nil")
	}

	// 3. Success path via runCLIWithClient (custom distDir)
	tmpDir := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/releases/tags/v0.5.2") {
			rel := github.RepositoryRelease{ID: github.Int64(555), TagName: github.String("v0.5.2")}
			_ = json.NewEncoder(w).Encode(rel)
			return
		}
		if strings.Contains(r.URL.Path, "/releases/555/assets") {
			if r.Method == "GET" {
				_ = json.NewEncoder(w).Encode([]*github.ReleaseAsset{})
				return
			}
			if r.Method == "POST" {
				_ = json.NewEncoder(w).Encode(github.ReleaseAsset{ID: github.Int64(777)})
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := github.NewClient(nil)
	u, _ := url.Parse(ts.URL + "/")
	client.BaseURL = u
	client.UploadURL = u

	getEnv := func(k string) string {
		if k == "GITHUB_REF_NAME" {
			return "v0.5.2"
		}
		if k == "SKIP_DISTRIBUTION" {
			return "true"
		}
		if k == "NACHO_DIST_DIR" {
			return tmpDir
		}
		return ""
	}

	err = runCLIWithClient(client, "tok", getEnv, nil)
	if err != nil {
		t.Fatalf("runCLIWithClient failed: %v", err)
	}

	// 4. Default distDir branch (NACHO_DIST_DIR empty)
	getEnvDefault := func(k string) string {
		if k == "GITHUB_REF_NAME" {
			return "v0.5.2"
		}
		if k == "SKIP_DISTRIBUTION" {
			return "true"
		}
		return ""
	}
	err = runCLIWithClient(client, "tok", getEnvDefault, nil)
	if err != nil {
		t.Fatalf("runCLIWithClient default distDir failed: %v", err)
	}
}

func TestEnsureRelease_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := github.NewClient(nil)
	_, err := ensureRelease(ctx, client, "owner", "repo", "v0.5.2")
	if err == nil {
		t.Errorf("expected error on canceled context, got nil")
	}
}

func TestMainFunc(t *testing.T) {
	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()

	var called bool
	exitFunc = func(v ...any) {
		called = true
	}

	main()
	if !called {
		t.Errorf("expected exitFunc to be called when no token is present")
	}
}
