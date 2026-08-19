package main

import (
	"bytes"
	"context"
	"crypto/sha256"
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

const (
	repoOwner   = "dixieflatline76"
	repoName    = "nacho-flow"
	homebrewTap = "homebrew-spice"
	wingetRepo  = "winget-pkgs"
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)

	token := os.Getenv("NACHO_RELEASER_TOKEN")
	if token == "" {
		token = os.Getenv("GORELEASER_GITHUB_TOKEN")
	}
	if token == "" {
		log.Fatal("NACHO_RELEASER_TOKEN or GORELEASER_GITHUB_TOKEN environment variable is not set")
	}

	tag := os.Getenv("GITHUB_REF_NAME")
	if tag == "" {
		data, err := os.ReadFile("version.txt")
		if err == nil {
			tag = "v" + strings.TrimSpace(string(data))
		}
	}
	if !strings.HasPrefix(tag, "v") {
		log.Fatalf("Tag %s does not start with 'v'", tag)
	}

	version := strings.TrimPrefix(tag, "v")
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
	if err := os.WriteFile("checksums.txt", checksumBytes, 0644); err != nil {
		log.Fatalf("Failed to write checksums.txt: %v", err)
	}
	log.Println("[nacho-releaser] Successfully generated checksums.txt")

	winHash := hashes[fmt.Sprintf("nacho-flow-%s-windows-amd64.exe", version)]

	// 2. GitHub Release Management
	release, resp, err := client.Repositories.GetReleaseByTag(ctx, repoOwner, repoName, tag)
	if err != nil && resp != nil && resp.StatusCode == 404 {
		log.Println("[nacho-releaser] Release not found, creating new release...")
		newRelease := &github.RepositoryRelease{
			TagName: github.String(tag),
			Name:    github.String("Nacho Flow " + tag),
			Body:    github.String("Automated release for Nacho Flow " + tag + " (spicerack.dev)"),
			Draft:   github.Bool(false),
		}
		release, _, err = client.Repositories.CreateRelease(ctx, repoOwner, repoName, newRelease)
		if err != nil {
			log.Fatalf("Failed to create release: %v", err)
		}
	} else if err != nil {
		log.Fatalf("Failed to fetch release: %v", err)
	}

	// Upload artifacts + checksums
	uploadFiles := []string{"checksums.txt"}
	for _, a := range artifacts {
		if _, err := os.Stat(a.Path); err == nil {
			uploadFiles = append(uploadFiles, a.Path)
		}
	}

	for _, file := range uploadFiles {
		uploadAsset(ctx, client, release.GetID(), file)
	}

	if strings.EqualFold(os.Getenv("SKIP_DISTRIBUTION"), "true") {
		log.Println("[nacho-releaser] SKIP_DISTRIBUTION=true — skipping Homebrew/Winget sync.")
		return
	}

	// 3. Winget Manifest Automation via Git Trees API
	if winHash != "" {
		log.Printf("[nacho-releaser] Syncing %s/%s for Winget manifest...", repoOwner, wingetRepo)
		pushWingetManifests(ctx, client, version, winHash)
	}

	log.Println("[nacho-releaser] Release process completed successfully! 🚀")
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func uploadAsset(ctx context.Context, client *github.Client, releaseID int64, path string) {
	file, err := os.Open(path)
	if err != nil {
		log.Printf("Failed to open %s: %v", path, err)
		return
	}
	defer file.Close()

	baseName := filepath.Base(path)
	assets, _, _ := client.Repositories.ListReleaseAssets(ctx, repoOwner, repoName, releaseID, nil)
	for _, a := range assets {
		if a.GetName() == baseName {
			client.Repositories.DeleteReleaseAsset(ctx, repoOwner, repoName, a.GetID())
		}
	}

	opts := &github.UploadOptions{Name: baseName}
	log.Printf("[nacho-releaser] Uploading asset: %s", baseName)
	client.Repositories.UploadReleaseAsset(ctx, repoOwner, repoName, releaseID, opts, file)
}

func pushWingetManifests(ctx context.Context, client *github.Client, version, winHash string) {
	baseBranch := "master"
	branchName := fmt.Sprintf("nacho-flow-v%s", version)
	commitMsg := fmt.Sprintf("New version: dixieflatline76.NachoFlow version %s", version)

	wingetVersionTmpl := `PackageIdentifier: dixieflatline76.NachoFlow
PackageVersion: {{.Version}}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.5.0
`

	wingetInstallerTmpl := `PackageIdentifier: dixieflatline76.NachoFlow
PackageVersion: {{.Version}}
Installers:
  - Architecture: x64
    InstallerType: portable
    InstallerUrl: https://github.com/dixieflatline76/nacho-flow/releases/download/v{{.Version}}/nacho-flow-{{.Version}}-windows-amd64.exe
    InstallerSha256: {{.WinHash}}
ManifestType: installer
ManifestVersion: 1.5.0
`

	wingetLocaleTmpl := `PackageIdentifier: dixieflatline76.NachoFlow
PackageVersion: {{.Version}}
PackageLocale: en-US
Publisher: dixieflatline76
PackageName: Nacho Flow
License: MIT
ShortDescription: High-performance OpenAI-compatible hybrid AI gateway for local GPUs and cloud APIs (spicerack.dev).
Moniker: nacho-flow
Tags:
  - ai
  - llm
  - proxy
  - gateway
  - ollama
  - openrouter
ManifestType: defaultLocale
ManifestVersion: 1.5.0
`

	data := struct {
		Version string
		WinHash string
	}{version, strings.ToUpper(winHash)}

	baseManifestPath := fmt.Sprintf("manifests/d/dixieflatline76/NachoFlow/%s", version)
	files := []struct {
		Path     string
		Template string
	}{
		{fmt.Sprintf("%s/dixieflatline76.NachoFlow.yaml", baseManifestPath), wingetVersionTmpl},
		{fmt.Sprintf("%s/dixieflatline76.NachoFlow.installer.yaml", baseManifestPath), wingetInstallerTmpl},
		{fmt.Sprintf("%s/dixieflatline76.NachoFlow.locale.en-US.yaml", baseManifestPath), wingetLocaleTmpl},
	}

	baseRef, _, err := client.Git.GetRef(ctx, repoOwner, wingetRepo, "refs/heads/"+baseBranch)
	if err != nil {
		log.Printf("[nacho-releaser] Could not fetch winget branch: %v", err)
		return
	}
	baseSHA := baseRef.Object.GetSHA()

	client.Git.DeleteRef(ctx, repoOwner, wingetRepo, "refs/heads/"+branchName)
	newRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branchName),
		Object: &github.GitObject{SHA: github.String(baseSHA)},
	}
	client.Git.CreateRef(ctx, repoOwner, wingetRepo, newRef)

	var treeEntries []*github.TreeEntry
	for _, f := range files {
		t, _ := template.New("t").Parse(f.Template)
		var buf bytes.Buffer
		t.Execute(&buf, data)
		treeEntries = append(treeEntries, &github.TreeEntry{
			Path:    github.String(f.Path),
			Mode:    github.String("100644"),
			Type:    github.String("blob"),
			Content: github.String(buf.String()),
		})
	}

	tree, _, _ := client.Git.CreateTree(ctx, repoOwner, wingetRepo, baseSHA, treeEntries)
	commit := &github.Commit{
		Message: github.String(commitMsg),
		Tree:    tree,
		Parents: []*github.Commit{{SHA: github.String(baseSHA)}},
	}
	newCommit, _, _ := client.Git.CreateCommit(ctx, repoOwner, wingetRepo, commit, nil)
	branchRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branchName),
		Object: &github.GitObject{SHA: newCommit.SHA},
	}
	client.Git.UpdateRef(ctx, repoOwner, wingetRepo, branchRef, true)

	log.Printf("✅ Winget manifests pushed to branch '%s' for 1-click PR!", branchName)
}
