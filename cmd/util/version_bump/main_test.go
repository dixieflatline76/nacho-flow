package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input  string
		valid  bool
		major  int
		minor  int
		patch  int
		prefix string
	}{
		{"v1.2.3", true, 1, 2, 3, "v"},
		{"0.5.2", true, 0, 5, 2, ""},
		{"v0.1.0", true, 0, 1, 0, "v"},
		{"invalid", false, 0, 0, 0, ""},
		{"v1.2", false, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		v, err := parseVersion(tt.input)
		if tt.valid && err != nil {
			t.Fatalf("expected %s to parse, got err: %v", tt.input, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("expected %s to fail parsing", tt.input)
		}
		if tt.valid {
			if v.Major != tt.major || v.Minor != tt.minor || v.Patch != tt.patch || v.Prefix != tt.prefix {
				t.Fatalf("parsed version mismatch for %s: %+v", tt.input, v)
			}
		}
	}
}

func TestBumpVersion(t *testing.T) {
	v := Version{Major: 0, Minor: 5, Patch: 2}

	patch, err := bumpVersion(v, "patch")
	if err != nil || patch.String() != "v0.5.3" {
		t.Fatalf("expected v0.5.3, got %s (%v)", patch.String(), err)
	}

	minor, err := bumpVersion(v, "minor")
	if err != nil || minor.String() != "v0.6.0" {
		t.Fatalf("expected v0.6.0, got %s (%v)", minor.String(), err)
	}

	major, err := bumpVersion(v, "major")
	if err != nil || major.String() != "v1.0.0" {
		t.Fatalf("expected v1.0.0, got %s (%v)", major.String(), err)
	}

	_, err = bumpVersion(v, "invalid")
	if err == nil {
		t.Fatalf("expected error on invalid bump type")
	}
}

func TestReadWriteVersionFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "version.txt")

	v := Version{Major: 1, Minor: 0, Patch: 0}
	if err := writeVersionToFile(path, v); err != nil {
		t.Fatalf("writeVersionToFile failed: %v", err)
	}

	read, err := readVersionFromFile(path)
	if err != nil {
		t.Fatalf("readVersionFromFile failed: %v", err)
	}
	if read.Major != 1 || read.Minor != 0 || read.Patch != 0 {
		t.Fatalf("version mismatch: %+v", read)
	}

	// Missing file error
	_, err = readVersionFromFile(filepath.Join(tmpDir, "missing.txt"))
	if err == nil {
		t.Fatalf("expected error reading missing file")
	}

	// Write error to nonexistent directory
	if err := writeVersionToFile(filepath.Join(tmpDir, "missing_subdir", "version.txt"), v); err == nil {
		t.Fatalf("expected error writing to nonexistent subdir")
	}
}

func TestUpdateSiteVersion(t *testing.T) {
	tmpDir := t.TempDir()
	sitePath := filepath.Join(tmpDir, "index.html")

	html := `<!DOCTYPE html>
<html>
<body>
    <span class="logo-badge logo-badge-version" id="version-badge">v0.5.1</span>
</body>
</html>`

	if err := os.WriteFile(sitePath, []byte(html), 0600); err != nil {
		t.Fatalf("failed to write test html: %v", err)
	}

	v := Version{Major: 0, Minor: 5, Patch: 2}
	if err := updateSiteVersion(sitePath, v); err != nil {
		t.Fatalf("updateSiteVersion failed: %v", err)
	}

	updated, err := os.ReadFile(sitePath)
	if err != nil {
		t.Fatalf("failed to read updated site: %v", err)
	}

	expected := `<span class="logo-badge logo-badge-version" id="version-badge">v0.5.2</span>`
	if string(updated) != `<!DOCTYPE html>
<html>
<body>
    `+expected+`
</body>
</html>` {
		t.Fatalf("site content mismatch: %s", string(updated))
	}

	// Error path: file missing
	if err := updateSiteVersion(filepath.Join(tmpDir, "nonexistent.html"), v); err == nil {
		t.Fatalf("expected error on nonexistent file")
	}
}

func TestUpdatePackageJSON(t *testing.T) {
	tmpDir := t.TempDir()
	pkgPath := filepath.Join(tmpDir, "package.json")

	content := `{
  "name": "nacho-flow",
  "version": "0.5.2",
  "description": "test"
}`
	_ = os.WriteFile(pkgPath, []byte(content), 0600)

	v := Version{Major: 0, Minor: 6, Patch: 0}
	if err := updatePackageJSON(pkgPath, v); err != nil {
		t.Fatalf("updatePackageJSON failed: %v", err)
	}

	data, err := os.ReadFile(pkgPath)
	if err != nil || !strings.Contains(string(data), `"version": "0.6.0"`) {
		t.Fatalf("package.json not updated properly: %s", string(data))
	}

	// Missing file error
	if err := updatePackageJSON(filepath.Join(tmpDir, "missing.json"), v); err == nil {
		t.Fatalf("expected error on missing package.json")
	}
}

func TestRunWithRunners_Success(t *testing.T) {
	tmpDir := t.TempDir()
	versionFile := filepath.Join(tmpDir, "version.txt")
	siteFile := filepath.Join(tmpDir, "index.html")
	pkgJsonFile := filepath.Join(tmpDir, "package.json")
	pkgLockFile := filepath.Join(tmpDir, "package-lock.json")

	_ = os.WriteFile(versionFile, []byte("0.5.2\n"), 0600)
	_ = os.WriteFile(siteFile, []byte(`<span class="logo-badge logo-badge-version" id="version-badge">v0.5.2</span>`), 0600)
	_ = os.WriteFile(pkgJsonFile, []byte(`{"name": "nacho-flow", "version": "0.5.2"}`), 0600)
	_ = os.WriteFile(pkgLockFile, []byte(`{"name": "nacho-flow", "version": "0.5.2"}`), 0600)

	var gitCalls [][]string
	mockGit := func(args ...string) error {
		gitCalls = append(gitCalls, args)
		return nil
	}

	mockOut := func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "branch" {
			return "feature/test", nil
		}
		return "", nil
	}

	err := runWithRunners([]string{"cmd", "-type=minor"}, versionFile, siteFile, pkgJsonFile, pkgLockFile, mockGit, mockOut)
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}

	// Verify version.txt updated to 0.6.0
	ver, err := readVersionFromFile(versionFile)
	if err != nil || ver.String() != "v0.6.0" {
		t.Fatalf("expected v0.6.0 in version.txt, got %s (%v)", ver.String(), err)
	}

	// Verify site/index.html updated to v0.6.0
	siteContent, err := os.ReadFile(siteFile)
	if err != nil || string(siteContent) != `<span class="logo-badge logo-badge-version" id="version-badge">v0.6.0</span>` {
		t.Fatalf("site file not updated properly: %s", string(siteContent))
	}

	// Verify package.json updated to 0.6.0
	pkgContent, err := os.ReadFile(pkgJsonFile)
	if err != nil || !strings.Contains(string(pkgContent), `"version": "0.6.0"`) {
		t.Fatalf("package.json not updated properly: %s", string(pkgContent))
	}
}

func TestRunWithRunners_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	versionFile := filepath.Join(tmpDir, "version.txt")
	siteFile := filepath.Join(tmpDir, "index.html")
	_ = os.WriteFile(versionFile, []byte("0.5.2\n"), 0600)

	mockGit := func(args ...string) error { return nil }
	mockOut := func(args ...string) (string, error) { return "main", nil }

	// 1. Missing args
	if err := runWithRunners([]string{"cmd"}, versionFile, siteFile, "", "", mockGit, mockOut); err == nil {
		t.Fatal("expected error on missing bump type")
	}

	// 2. Branch error
	branchErrOut := func(args ...string) (string, error) { return "", errors.New("branch failure") }
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", mockGit, branchErrOut); err == nil {
		t.Fatal("expected error on branch failure")
	}

	// 3. Checkout error
	checkoutErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "checkout" {
			return errors.New("checkout failed")
		}
		return nil
	}
	featureBranchOut := func(args ...string) (string, error) { return "feature", nil }
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", checkoutErrGit, featureBranchOut); err == nil {
		t.Fatal("expected error on checkout failure")
	}

	// 4. Pull error
	pullErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "pull" {
			return errors.New("pull failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", pullErrGit, mockOut); err == nil {
		t.Fatal("expected error on pull failure")
	}

	// 5. Version file read error
	if err := runWithRunners([]string{"cmd", "patch"}, filepath.Join(tmpDir, "missing.txt"), siteFile, "", "", mockGit, mockOut); err == nil {
		t.Fatal("expected error on missing version file")
	}

	// 6. Invalid bump type
	if err := runWithRunners([]string{"cmd", "invalid-type"}, versionFile, siteFile, "", "", mockGit, mockOut); err == nil {
		t.Fatal("expected error on invalid bump type")
	}

	// 7. Git add / commit error
	commitErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "add" {
			return errors.New("add failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", commitErrGit, mockOut); err == nil {
		t.Fatal("expected error on commit failure")
	}

	// 8. Git tag error
	tagErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "tag" {
			return errors.New("tag failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", tagErrGit, mockOut); err == nil {
		t.Fatal("expected error on tag failure")
	}

	// 9. Git push error
	pushErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "push" && args[1] == "origin" && args[2] == "main" {
			return errors.New("push failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", pushErrGit, mockOut); err == nil {
		t.Fatal("expected error on push failure")
	}
}

func TestDefaultRunners(t *testing.T) {
	// Simple invocation of default runners for coverage
	_ = defaultGitRunner("version")
	_, _ = defaultOutputRunner("version")

	// Error path for defaultOutputRunner
	_, err := defaultOutputRunner("invalid-command-that-fails-12345")
	if err == nil {
		t.Errorf("expected error running invalid git command")
	}

	// updateSiteVersion and updatePackageJSON error paths on invalid files
	v := Version{Major: 1, Minor: 0, Patch: 0}
	if err := updateSiteVersion("/nonexistent_dir_12345/index.html", v); err == nil {
		t.Errorf("expected error updating nonexistent site file")
	}
	if err := updatePackageJSON("/nonexistent_dir_12345/package.json", v); err == nil {
		t.Errorf("expected error updating nonexistent package file")
	}
}

func TestRunCLI_Errors(t *testing.T) {
	// Calling runCLI with insufficient args returns usage error
	if err := runCLI([]string{"cmd"}); err == nil {
		t.Fatal("expected error with no bump type")
	}
}

func TestMainFunc(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
	}()

	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	os.Args = []string{"cmd"}
	main()
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

func TestRunWithRunners_WriteFileError(t *testing.T) {
	tmpDir := t.TempDir()
	versionFile := filepath.Join(tmpDir, "version.txt")
	siteFile := filepath.Join(tmpDir, "index.html")
	_ = os.WriteFile(versionFile, []byte("0.5.2\n"), 0600)

	mockGit := func(args ...string) error { return nil }
	mockOut := func(args ...string) (string, error) { return "main", nil }

	// Trigger write failure
	writeErrFile := filepath.Join(tmpDir, "nonexistent_dir", "version.txt")
	if err := runWithRunners([]string{"cmd", "patch"}, writeErrFile, siteFile, "", "", mockGit, mockOut); err == nil {
		t.Fatal("expected error on write failure or read failure")
	}

	// Trigger tag push error
	tagPushErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "push" && args[1] == "origin" && args[2] == "v0.5.3" {
			return errors.New("tag push failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", tagPushErrGit, mockOut); err == nil {
		t.Fatal("expected error on tag push failure")
	}
}
