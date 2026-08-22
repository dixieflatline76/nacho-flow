package main

// Nacho Releaser is the automated release and distribution utility for Nacho Flow.
//
// ⚠️ CI/CD ARCHITECTURAL LIFECYCLE (2-Stage Release Workflow):
//
// 1. Stage 1: Pre-Release Asset Upload (publish-release in .github/workflows/ci.yml)
//    - Trigger: Release is published as a Pre-release (or draft published).
//    - Auth: Runs with standard repository GITHUB_TOKEN (scoped ONLY to dixieflatline76/nacho-flow).
//    - Action: Verifies local build artifacts, calculates SHA-256 hashes, generates checksums.txt,
//      and attaches all binaries to the GitHub Release.
//    - Invariant: MUST have SKIP_DISTRIBUTION=true. It MUST NOT attempt cross-repo pushes to
//      Homebrew or WinGet, because GITHUB_TOKEN does not have write permissions to external repos.
//
// 2. Stage 2: Public Distribution & Manifest Sync (distribute-release in .github/workflows/ci.yml)
//    - Trigger: Release is promoted to Latest (pre-release checkbox unchecked).
//    - Auth: Runs with GORELEASER_GITHUB_TOKEN / NACHO_RELEASER_TOKEN (elevated cross-repo PAT).
//    - Action: Pushes updated Formula to dixieflatline76/homebrew-nacho-flow and manifest branch
//      to dixieflatline76/winget-pkgs for the 1-click Microsoft winget-pkgs PR.
//    - Invariant: Runs with SKIP_DISTRIBUTION=false.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/google/go-github/v63/github"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

const (
	repoOwner   = "dixieflatline76"
	repoName    = "nacho-flow"
	homebrewTap = "homebrew-nacho-flow"
	wingetRepo  = "winget-pkgs"
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)

	// Resolve GitHub authentication token.
	// - For Stage 1 (Assets only): GITHUB_TOKEN is sufficient.
	// - For Stage 2 (Distribution): NACHO_RELEASER_TOKEN / GORELEASER_GITHUB_TOKEN with cross-repo write access is required.
	token := os.Getenv("NACHO_RELEASER_TOKEN")
	if token == "" {
		token = os.Getenv("GORELEASER_GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		log.Fatal("No GitHub token found in NACHO_RELEASER_TOKEN, GORELEASER_GITHUB_TOKEN, or GITHUB_TOKEN")
	}

	tag := os.Getenv("GITHUB_REF_NAME")
	if tag == "" {
		data, err := os.ReadFile("version.txt")
		if err == nil {
			tag = "v" + strings.TrimSpace(string(data))
		}
	}
	if !strings.HasPrefix(tag, "v") {
		// #nosec G706 - trusted build tag from GITHUB_REF_NAME or version.txt
		log.Fatalf("Tag %s does not start with 'v'", tag)
	}

	version := strings.TrimPrefix(tag, "v")
	// #nosec G706 - trusted build version
	log.Printf("[nacho-releaser] Starting release process for version: %s (tag: %s)", version, tag)

	client := github.NewClient(nil).WithAuthToken(token)
	ctx := context.Background()

	// 1. Artifact Verification & Hashing
	artifacts := []struct {
		Path     string
		BaseName string
	}{
		{Path: fmt.Sprintf("dist/nacho-flow-%s-windows-amd64.exe", version)},
		{Path: fmt.Sprintf("dist/nacho-flow-%s-linux-amd64", version)},
		{Path: fmt.Sprintf("dist/nacho-flow-%s-linux-arm64", version)},
		{Path: fmt.Sprintf("dist/nacho-flow-%s-darwin-amd64", version)},
		{Path: fmt.Sprintf("dist/nacho-flow-%s-darwin-arm64", version)},
	}

	hashes := make(map[string]string)
	var checksumData bytes.Buffer

	for i, a := range artifacts {
		artifacts[i].BaseName = filepath.Base(a.Path)
		if _, err := os.Stat(a.Path); os.IsNotExist(err) {
			log.Printf("[nacho-releaser] Warning: Artifact missing (skipping): %s", a.Path)
			continue
		}

		hash, err := hashFile(a.Path)
		if err != nil {
			log.Fatalf("Failed to hash %s: %v", a.Path, err)
		}
		hashes[artifacts[i].BaseName] = hash
		checksumData.WriteString(fmt.Sprintf("%s  %s\n", hash, artifacts[i].BaseName))
		log.Printf("[nacho-releaser] Hashed %s -> %s", artifacts[i].BaseName, hash)
	}

	checksumBytes := checksumData.Bytes()
	if err := os.WriteFile("checksums.txt", checksumBytes, 0600); err != nil {
		log.Fatalf("Failed to write checksums.txt: %v", err)
	}
	log.Println("[nacho-releaser] Successfully generated checksums.txt")

	winHash := hashes[fmt.Sprintf("nacho-flow-%s-windows-amd64.exe", version)]

	// 2. GitHub Release Management
	release, resp, err := client.Repositories.GetReleaseByTag(ctx, repoOwner, repoName, tag)
	if err != nil && resp != nil && resp.StatusCode == 404 {
		log.Println("[nacho-releaser] Release not found, creating new release...")
		newRelease := &github.RepositoryRelease{
			TagName:         github.String(tag),
			TargetCommitish: github.String("main"),
			Name:            github.String(fmt.Sprintf("Nacho Flow %s", tag)),
			Body:            github.String(fmt.Sprintf("Automated release for version %s", tag)),
			Draft:           github.Bool(false),
			Prerelease:      github.Bool(false),
		}
		var createErr error
		release, _, createErr = client.Repositories.CreateRelease(ctx, repoOwner, repoName, newRelease)
		if createErr != nil {
			log.Fatalf("Failed to create GitHub release: %v", createErr)
		}
		// #nosec G706 - trusted release tag
		log.Printf("[nacho-releaser] Created release %s (ID: %d)", tag, release.GetID())
	} else if err != nil {
		log.Fatalf("Failed to check existing release: %v", err)
	} else {
		// #nosec G706 - trusted release tag
		log.Printf("[nacho-releaser] Found existing release %s (ID: %d)", tag, release.GetID())
	}

	// 3. Upload Artifacts to Release (Always executed in Stage 1 & Stage 2)
	for _, a := range artifacts {
		uploadAsset(ctx, client, release.GetID(), a.Path)
	}
	uploadAsset(ctx, client, release.GetID(), "checksums.txt")

	// ⚠️ CRITICAL STAGE GATE:
	// If SKIP_DISTRIBUTION=true (set during publish-release on pre-releases), stop here.
	// We MUST NOT attempt cross-repo pushes (Homebrew/WinGet) during pre-releases because:
	//   1. Pre-releases are meant for local/manual testing before public distribution.
	//   2. The GITHUB_TOKEN present during publish-release lacks write access to external repositories.
	// Cross-repo distribution only occurs in Stage 2 (distribute-release) when promoted to Latest.
	skipDistribution := os.Getenv("SKIP_DISTRIBUTION") == "true"
	if skipDistribution {
		log.Println("[nacho-releaser] SKIP_DISTRIBUTION=true (Stage 1: Pre-release). Asset upload complete. Skipping Homebrew/WinGet distribution.")
	} else {
		// Stage 2: Push Homebrew Formula to dixieflatline76/homebrew-nacho-flow
		if token != "" {
			log.Println("[nacho-releaser] (Stage 2: Latest) Generating and pushing Homebrew formula...")
			pushHomebrewFormula(ctx, client, version, hashes)
		}

		// Stage 2: Push Winget Manifests to dixieflatline76/winget-pkgs
		if token != "" {
			log.Println("[nacho-releaser] (Stage 2: Latest) Generating and pushing winget manifests...")
			pushWingetManifests(ctx, client, version, winHash)
		}
	}

	// #nosec G706 - trusted release tag
	log.Printf("🎉 Release %s completed successfully!", tag)
}

func hashFile(path string) (string, error) {
	path = filepath.Clean(path)
	// #nosec G304 - path is from vetted local release artifacts
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func uploadAsset(ctx context.Context, client *github.Client, releaseID int64, path string) {
	path = filepath.Clean(path)
	// #nosec G304 - path is from vetted local release artifacts
	file, err := os.Open(path)
	if err != nil {
		log.Printf("Failed to open %s: %v", path, err)
		return
	}
	defer func() { _ = file.Close() }()

	baseName := filepath.Base(path)
	assets, _, _ := client.Repositories.ListReleaseAssets(ctx, repoOwner, repoName, releaseID, nil)
	for _, a := range assets {
		if a.GetName() == baseName {
			_, _ = client.Repositories.DeleteReleaseAsset(ctx, repoOwner, repoName, a.GetID())
		}
	}

	opts := &github.UploadOptions{Name: baseName}
	log.Printf("[nacho-releaser] Uploading asset: %s", baseName)
	_, _, _ = client.Repositories.UploadReleaseAsset(ctx, repoOwner, repoName, releaseID, opts, file)
}

func renderTemplate(tmplPath string, data any) ([]byte, error) {
	tmplContent, err := templateFS.ReadFile(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded template %s: %w", tmplPath, err)
	}

	t, err := template.New(filepath.Base(tmplPath)).Parse(string(tmplContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", tmplPath, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template %s: %w", tmplPath, err)
	}
	return buf.Bytes(), nil
}

func pushWingetManifests(ctx context.Context, client *github.Client, version, winHash string) {
	baseBranch := "master"
	branchName := fmt.Sprintf("nacho-flow-v%s", version)
	commitMsg := fmt.Sprintf("New version: dixieflatline76.NachoFlow version %s", version)

	data := struct {
		Version string
		WinHash string
	}{version, strings.ToUpper(winHash)}

	baseManifestPath := fmt.Sprintf("manifests/d/dixieflatline76/NachoFlow/%s", version)
	files := []struct {
		Path         string
		TemplateName string
	}{
		{fmt.Sprintf("%s/dixieflatline76.NachoFlow.yaml", baseManifestPath), "templates/winget_version.yaml.tmpl"},
		{fmt.Sprintf("%s/dixieflatline76.NachoFlow.installer.yaml", baseManifestPath), "templates/winget_installer.yaml.tmpl"},
		{fmt.Sprintf("%s/dixieflatline76.NachoFlow.locale.en-US.yaml", baseManifestPath), "templates/winget_locale.yaml.tmpl"},
	}

	baseRef, _, err := client.Git.GetRef(ctx, repoOwner, wingetRepo, "refs/heads/"+baseBranch)
	if err != nil {
		log.Printf("[nacho-releaser] Could not fetch winget branch: %v", err)
		return
	}
	baseSHA := baseRef.Object.GetSHA()

	_, _ = client.Git.DeleteRef(ctx, repoOwner, wingetRepo, "refs/heads/"+branchName)
	newRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branchName),
		Object: &github.GitObject{SHA: github.String(baseSHA)},
	}
	_, _, _ = client.Git.CreateRef(ctx, repoOwner, wingetRepo, newRef)

	var treeEntries []*github.TreeEntry
	for _, f := range files {
		rendered, err := renderTemplate(f.TemplateName, data)
		if err != nil {
			log.Printf("[nacho-releaser] %v", err)
			return
		}
		treeEntries = append(treeEntries, &github.TreeEntry{
			Path:    github.String(f.Path),
			Mode:    github.String("100644"),
			Type:    github.String("blob"),
			Content: github.String(string(rendered)),
		})
	}

	tree, _, err := client.Git.CreateTree(ctx, repoOwner, wingetRepo, baseSHA, treeEntries)
	if err != nil || tree == nil {
		log.Printf("[nacho-releaser] Failed to create git tree for winget: %v", err)
		return
	}
	commit := &github.Commit{
		Message: github.String(commitMsg),
		Tree:    tree,
		Parents: []*github.Commit{{SHA: github.String(baseSHA)}},
	}
	newCommit, _, err := client.Git.CreateCommit(ctx, repoOwner, wingetRepo, commit, nil)
	if err != nil || newCommit == nil {
		log.Printf("[nacho-releaser] Failed to create git commit for winget: %v", err)
		return
	}
	branchRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branchName),
		Object: &github.GitObject{SHA: newCommit.SHA},
	}
	_, _, err = client.Git.UpdateRef(ctx, repoOwner, wingetRepo, branchRef, true)
	if err != nil {
		log.Printf("[nacho-releaser] Failed to update branch ref for winget: %v", err)
		return
	}

	// #nosec G706 - trusted calculated branch name
	log.Printf("✅ Winget manifests pushed to branch '%s' for 1-click PR!", branchName)
}

func pushHomebrewFormula(ctx context.Context, client *github.Client, version string, hashes map[string]string) {
	formulaPath := "Formula/nacho-flow.rb"
	branch := "main"

	darwinArm64Hash := hashes[fmt.Sprintf("nacho-flow-%s-darwin-arm64", version)]
	darwinAmd64Hash := hashes[fmt.Sprintf("nacho-flow-%s-darwin-amd64", version)]
	linuxAmd64Hash := hashes[fmt.Sprintf("nacho-flow-%s-linux-amd64", version)]
	linuxArm64Hash := hashes[fmt.Sprintf("nacho-flow-%s-linux-arm64", version)]

	if darwinArm64Hash == "" || darwinAmd64Hash == "" || linuxAmd64Hash == "" || linuxArm64Hash == "" {
		log.Println("[nacho-releaser] Warning: Missing one or more binary hashes for Homebrew formula update")
		return
	}

	data := struct {
		Version     string
		DarwinArm64 string
		DarwinAmd64 string
		LinuxAmd64  string
		LinuxArm64  string
	}{
		Version:     version,
		DarwinArm64: darwinArm64Hash,
		DarwinAmd64: darwinAmd64Hash,
		LinuxAmd64:  linuxAmd64Hash,
		LinuxArm64:  linuxArm64Hash,
	}

	rendered, err := renderTemplate("templates/homebrew_formula.rb.tmpl", data)
	if err != nil {
		log.Printf("[nacho-releaser] %v", err)
		return
	}

	fileContent, _, _, err := client.Repositories.GetContents(ctx, repoOwner, homebrewTap, formulaPath, &github.RepositoryContentGetOptions{Ref: branch})
	commitMsg := fmt.Sprintf("chore: update Formula for v%s", version)

	opts := &github.RepositoryContentFileOptions{
		Message: github.String(commitMsg),
		Content: rendered,
		Branch:  github.String(branch),
	}
	if err == nil && fileContent != nil {
		opts.SHA = fileContent.SHA
	}

	if fileContent != nil {
		_, _, err = client.Repositories.UpdateFile(ctx, repoOwner, homebrewTap, formulaPath, opts)
	} else {
		_, _, err = client.Repositories.CreateFile(ctx, repoOwner, homebrewTap, formulaPath, opts)
	}

	if err != nil {
		log.Printf("[nacho-releaser] Failed to push Homebrew formula to %s/%s: %v", repoOwner, homebrewTap, err)
		return
	}

	// #nosec G706 - trusted calculated repository name
	log.Printf("✅ Homebrew formula updated successfully in %s/%s!", repoOwner, homebrewTap)
}
