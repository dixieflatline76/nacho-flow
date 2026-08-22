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
	"errors"
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

func resolveToken(getEnv func(string) string) (string, error) {
	for _, k := range []string{"NACHO_RELEASER_TOKEN", "GORELEASER_GITHUB_TOKEN", "GITHUB_TOKEN"} {
		if val := getEnv(k); val != "" {
			return val, nil
		}
	}
	return "", errors.New("no GitHub token found in NACHO_RELEASER_TOKEN, GORELEASER_GITHUB_TOKEN, or GITHUB_TOKEN")
}

func resolveTag(getEnv func(string) string, readFile func(string) ([]byte, error)) (string, string, error) {
	tag := getEnv("GITHUB_REF_NAME")
	if tag == "" {
		data, err := readFile("version.txt")
		if err != nil {
			return "", "", fmt.Errorf("failed to read version.txt: %w", err)
		}
		tag = "v" + strings.TrimSpace(string(data))
	}
	if !strings.HasPrefix(tag, "v") {
		return "", "", fmt.Errorf("tag %s does not start with 'v'", tag)
	}
	version := strings.TrimPrefix(tag, "v")
	return tag, version, nil
}

func collectArtifactPaths(distDir, version string) []string {
	return []string{
		filepath.Join(distDir, fmt.Sprintf("nacho-flow-%s-windows-amd64.exe", version)),
		filepath.Join(distDir, fmt.Sprintf("nacho-flow-%s-linux-amd64", version)),
		filepath.Join(distDir, fmt.Sprintf("nacho-flow-%s-linux-arm64", version)),
		filepath.Join(distDir, fmt.Sprintf("nacho-flow-%s-darwin-amd64", version)),
		filepath.Join(distDir, fmt.Sprintf("nacho-flow-%s-darwin-arm64", version)),
	}
}

func generateChecksums(artifactPaths []string) (map[string]string, []byte, error) {
	hashes := make(map[string]string)
	var buf bytes.Buffer

	for _, p := range artifactPaths {
		baseName := filepath.Base(p)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			log.Printf("[nacho-releaser] Warning: Artifact missing (skipping): %s", p)
			continue
		}

		hash, err := hashFile(p)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to hash %s: %w", p, err)
		}
		hashes[baseName] = hash
		buf.WriteString(fmt.Sprintf("%s  %s\n", hash, baseName))
		log.Printf("[nacho-releaser] Hashed %s -> %s", baseName, hash)
	}

	return hashes, buf.Bytes(), nil
}

func run(ctx context.Context, client *github.Client, token, tag, version, distDir string, skipDistribution bool) error {
	// 1. Artifact Verification & Hashing
	artifactPaths := collectArtifactPaths(distDir, version)
	hashes, checksumBytes, err := generateChecksums(artifactPaths)
	if err != nil {
		return fmt.Errorf("checksum generation failed: %w", err)
	}

	if err := os.WriteFile("checksums.txt", checksumBytes, 0600); err != nil {
		return fmt.Errorf("failed to write checksums.txt: %w", err)
	}
	log.Println("[nacho-releaser] Successfully generated checksums.txt")

	winHash := hashes[fmt.Sprintf("nacho-flow-%s-windows-amd64.exe", version)]

	// 2. GitHub Release Management
	release, err := ensureRelease(ctx, client, repoOwner, repoName, tag, version)
	if err != nil {
		return fmt.Errorf("failed to ensure release: %w", err)
	}

	// 3. Upload Artifacts to Release (Always executed in Stage 1 & Stage 2)
	for _, p := range artifactPaths {
		if _, err := os.Stat(p); err == nil {
			uploadAsset(ctx, client, repoOwner, repoName, release.GetID(), p)
		}
	}
	uploadAsset(ctx, client, repoOwner, repoName, release.GetID(), "checksums.txt")

	// ⚠️ CRITICAL STAGE GATE:
	// If SKIP_DISTRIBUTION=true (set during publish-release on pre-releases), stop here.
	// We MUST NOT attempt cross-repo pushes (Homebrew/WinGet) during pre-releases because:
	//   1. Pre-releases are meant for local/manual testing before public distribution.
	//   2. The GITHUB_TOKEN present during publish-release lacks write access to external repositories.
	// Cross-repo distribution only occurs in Stage 2 (distribute-release) when promoted to Latest.
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
	return nil
}

func runCLI(getEnv func(string) string, readFile func(string) ([]byte, error)) error {
	token, err := resolveToken(getEnv)
	if err != nil {
		return err
	}
	client := github.NewClient(nil).WithAuthToken(token)
	return runCLIWithClient(client, token, getEnv, readFile)
}

func runCLIWithClient(client *github.Client, token string, getEnv func(string) string, readFile func(string) ([]byte, error)) error {
	tag, version, err := resolveTag(getEnv, readFile)
	if err != nil {
		return fmt.Errorf("invalid release tag: %w", err)
	}

	// #nosec G706 - trusted build version
	log.Printf("[nacho-releaser] Starting release process for version: %s (tag: %s)", version, tag)

	ctx := context.Background()
	skipDistribution := getEnv("SKIP_DISTRIBUTION") == "true"
	distDir := getEnv("NACHO_DIST_DIR")
	if distDir == "" {
		distDir = "dist"
	}

	return run(ctx, client, token, tag, version, distDir, skipDistribution)
}

var exitFunc = log.Fatal

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)

	if err := runCLI(os.Getenv, os.ReadFile); err != nil {
		exitFunc(err)
	}
}

func ensureRelease(ctx context.Context, client *github.Client, owner, repo, tag, version string) (*github.RepositoryRelease, error) {
	release, resp, err := client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
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
		rel, _, createErr := client.Repositories.CreateRelease(ctx, owner, repo, newRelease)
		if createErr != nil {
			return nil, fmt.Errorf("failed to create release %s: %w", tag, createErr)
		}
		// #nosec G706 - trusted release tag
		log.Printf("[nacho-releaser] Created release %s (ID: %d)", tag, rel.GetID())
		return rel, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to check existing release %s: %w", tag, err)
	}
	// #nosec G706 - trusted release tag
	log.Printf("[nacho-releaser] Found existing release %s (ID: %d)", tag, release.GetID())
	return release, nil
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

func uploadAsset(ctx context.Context, client *github.Client, owner, repo string, releaseID int64, path string) {
	path = filepath.Clean(path)
	// #nosec G304 - path is from vetted local release artifacts
	file, err := os.Open(path)
	if err != nil {
		log.Printf("Failed to open %s: %v", path, err)
		return
	}
	defer func() { _ = file.Close() }()

	baseName := filepath.Base(path)
	assets, _, _ := client.Repositories.ListReleaseAssets(ctx, owner, repo, releaseID, nil)
	for _, a := range assets {
		if a.GetName() == baseName {
			_, _ = client.Repositories.DeleteReleaseAsset(ctx, owner, repo, a.GetID())
		}
	}

	opts := &github.UploadOptions{Name: baseName}
	log.Printf("[nacho-releaser] Uploading asset: %s", baseName)
	_, _, _ = client.Repositories.UploadReleaseAsset(ctx, owner, repo, releaseID, opts, file)
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
