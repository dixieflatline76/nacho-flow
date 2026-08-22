package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if !strings.Contains(content, "License: MIT") {
		t.Errorf("expected License: MIT, got: %s", content)
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
