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
	changelogFile := filepath.Join(tmpDir, "CHANGELOG.md")

	_ = os.WriteFile(versionFile, []byte("0.5.2\n"), 0600)
	_ = os.WriteFile(siteFile, []byte(`<span class="logo-badge logo-badge-version" id="version-badge">v0.5.2</span>`), 0600)
	_ = os.WriteFile(pkgJsonFile, []byte(`{"name": "nacho-flow", "version": "0.5.2"}`), 0600)
	_ = os.WriteFile(pkgLockFile, []byte(`{"name": "nacho-flow", "version": "0.5.2"}`), 0600)
	_ = os.WriteFile(changelogFile, []byte("# Change Log\n\n## [Unreleased]\n\n### Added\n- Test feature\n"), 0600)

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

	err := runWithRunners([]string{"cmd", "-type=minor"}, versionFile, siteFile, pkgJsonFile, pkgLockFile, changelogFile, mockGit, mockOut)
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

	// Verify CHANGELOG.md updated to 0.6.0
	clContent, err := os.ReadFile(changelogFile)
	if err != nil || !strings.Contains(string(clContent), "## [0.6.0] - ") {
		t.Fatalf("CHANGELOG.md not updated properly: %s", string(clContent))
	}
	if !strings.Contains(string(clContent), "## [Unreleased]") {
		t.Fatalf("CHANGELOG.md missing [Unreleased] header: %s", string(clContent))
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
	if err := runWithRunners([]string{"cmd"}, versionFile, siteFile, "", "", "", mockGit, mockOut); err == nil {
		t.Fatal("expected error on missing bump type")
	}

	// 2. Branch error
	branchErrOut := func(args ...string) (string, error) { return "", errors.New("branch failure") }
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", "", mockGit, branchErrOut); err == nil {
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
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", "", checkoutErrGit, featureBranchOut); err == nil {
		t.Fatal("expected error on checkout failure")
	}

	// 4. Pull error
	pullErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "pull" {
			return errors.New("pull failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", "", pullErrGit, mockOut); err == nil {
		t.Fatal("expected error on pull failure")
	}

	// 5. Version file read error
	if err := runWithRunners([]string{"cmd", "patch"}, filepath.Join(tmpDir, "missing.txt"), siteFile, "", "", "", mockGit, mockOut); err == nil {
		t.Fatal("expected error on missing version file")
	}

	// 6. Invalid bump type
	if err := runWithRunners([]string{"cmd", "invalid-type"}, versionFile, siteFile, "", "", "", mockGit, mockOut); err == nil {
		t.Fatal("expected error on invalid bump type")
	}

	// 7. Git add / commit error
	commitErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "add" {
			return errors.New("add failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", "", commitErrGit, mockOut); err == nil {
		t.Fatal("expected error on commit failure")
	}

	// 8. Git tag error
	tagErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "tag" {
			return errors.New("tag failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", "", tagErrGit, mockOut); err == nil {
		t.Fatal("expected error on tag failure")
	}

	// 9. Git push error
	pushErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "push" && args[1] == "origin" && args[2] == "main" {
			return errors.New("push failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", "", pushErrGit, mockOut); err == nil {
		t.Fatal("expected error on push failure")
	}
}

func TestUpdateChangelog(t *testing.T) {
	tmpDir := t.TempDir()
	changelogPath := filepath.Join(tmpDir, "CHANGELOG.md")

	initial := `# Change Log

## [Unreleased]

### Added
- Feature A
`
	if err := os.WriteFile(changelogPath, []byte(initial), 0600); err != nil {
		t.Fatalf("failed to write test changelog: %v", err)
	}

	v := Version{Major: 1, Minor: 1, Patch: 0}
	if err := updateChangelog(changelogPath, v); err != nil {
		t.Fatalf("updateChangelog failed: %v", err)
	}

	data, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("failed to read changelog: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "## [1.1.0] - ") {
		t.Fatalf("expected version header for 1.1.0, got: %s", content)
	}
	if !strings.Contains(content, "## [Unreleased]") {
		t.Fatalf("expected Unreleased header retained, got: %s", content)
	}

	// Idempotency check: running again with the same version should not duplicate
	if err := updateChangelog(changelogPath, v); err != nil {
		t.Fatalf("second updateChangelog failed: %v", err)
	}
	data2, _ := os.ReadFile(changelogPath)
	if strings.Count(string(data2), "## [1.1.0] - ") != 1 {
		t.Fatalf("expected exactly 1 instance of 1.1.0 header, got: %s", string(data2))
	}

	// Fallback check: when ## [Unreleased] is missing
	noUnreleasedPath := filepath.Join(tmpDir, "CHANGELOG_NO_UNRELEASED.md")
	_ = os.WriteFile(noUnreleasedPath, []byte("# Change Log\n\n## [1.0.0] - 2026-09-01\n"), 0600)
	v2 := Version{Major: 1, Minor: 2, Patch: 0}
	if err := updateChangelog(noUnreleasedPath, v2); err != nil {
		t.Fatalf("updateChangelog without Unreleased failed: %v", err)
	}
	data3, _ := os.ReadFile(noUnreleasedPath)
	if !strings.Contains(string(data3), "## [1.2.0] - ") || !strings.Contains(string(data3), "## [Unreleased]") {
		t.Fatalf("expected fallback insertion for 1.2.0, got: %s", string(data3))
	}

	// Error check: missing file
	if err := updateChangelog(filepath.Join(tmpDir, "nonexistent.md"), v); err == nil {
		t.Fatal("expected error on nonexistent file")
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

	// defaultWinresRunner error and success paths
	_, _ = defaultWinresRunner("nonexistent-file.json", "out")
	tmpDir := t.TempDir()
	outPrefix := filepath.Join(tmpDir, "rsrc")
	if files, wErr := defaultWinresRunner(filepath.Join("..", "..", "nacho-flow", "winres", "winres.json"), outPrefix); wErr == nil {
		for _, f := range files {
			_ = os.Remove(f)
		}
	}

	// updateSiteVersion, updatePackageJSON, and updateChangelog error paths on invalid files
	v := Version{Major: 1, Minor: 0, Patch: 0}
	if err := updateSiteVersion("/nonexistent_dir_12345/index.html", v); err == nil {
		t.Errorf("expected error updating nonexistent site file")
	}
	if err := updatePackageJSON("/nonexistent_dir_12345/package.json", v); err == nil {
		t.Errorf("expected error updating nonexistent package file")
	}
	if err := updateChangelog("/nonexistent_dir_12345/CHANGELOG.md", v); err == nil {
		t.Errorf("expected error updating nonexistent changelog file")
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
	if err := runWithRunners([]string{"cmd", "patch"}, writeErrFile, siteFile, "", "", "", mockGit, mockOut); err == nil {
		t.Fatal("expected error on write failure or read failure")
	}

	// Trigger tag push error
	tagPushErrGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "push" && args[1] == "origin" && args[2] == "v0.5.3" {
			return errors.New("tag push failed")
		}
		return nil
	}
	if err := runWithRunners([]string{"cmd", "patch"}, versionFile, siteFile, "", "", "", tagPushErrGit, mockOut); err == nil {
		t.Fatal("expected error on tag push failure")
	}
}

func TestUpdateWinresVersion(t *testing.T) {
	tmpDir := t.TempDir()
	winresPath := filepath.Join(tmpDir, "winres.json")

	content := `{
  "RT_MANIFEST": {
    "#1": {
      "0409": {
        "identity": {
          "name": "Spicebox.NachoFlow",
          "version": "1.0.2.0"
        }
      }
    }
  },
  "RT_VERSION": {
    "#1": {
      "0409": {
        "fixed": {
          "file_version": "1.0.2.0",
          "product_version": "1.0.2.0"
        },
        "info": {
          "0409": {
            "FileVersion": "1.0.2.0",
            "ProductVersion": "1.0.2.0"
          }
        }
      }
    }
  }
}`
	if err := os.WriteFile(winresPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test winres.json: %v", err)
	}

	v := Version{Major: 1, Minor: 1, Patch: 0}
	if err := updateWinresVersion(winresPath, v); err != nil {
		t.Fatalf("updateWinresVersion failed: %v", err)
	}

	data, err := os.ReadFile(winresPath)
	if err != nil {
		t.Fatalf("failed to read updated winres.json: %v", err)
	}
	updatedStr := string(data)
	if !strings.Contains(updatedStr, `"version": "1.1.0.0"`) {
		t.Errorf("expected identity version 1.1.0.0, got: %s", updatedStr)
	}
	if !strings.Contains(updatedStr, `"file_version": "1.1.0.0"`) {
		t.Errorf("expected file_version 1.1.0.0, got: %s", updatedStr)
	}
	if !strings.Contains(updatedStr, `"product_version": "1.1.0.0"`) {
		t.Errorf("expected product_version 1.1.0.0, got: %s", updatedStr)
	}
	if !strings.Contains(updatedStr, `"FileVersion": "1.1.0.0"`) {
		t.Errorf("expected FileVersion 1.1.0.0, got: %s", updatedStr)
	}
	if !strings.Contains(updatedStr, `"ProductVersion": "1.1.0.0"`) {
		t.Errorf("expected ProductVersion 1.1.0.0, got: %s", updatedStr)
	}

	// Nonexistent file error path
	if err := updateWinresVersion(filepath.Join(tmpDir, "nonexistent.json"), v); err == nil {
		t.Errorf("expected error on nonexistent winres.json")
	}
}

func TestRunWithRunners_WithWinres(t *testing.T) {
	tmpDir := t.TempDir()
	versionFile := filepath.Join(tmpDir, "version.txt")
	winresDir := filepath.Join(tmpDir, "cmd", "nacho-flow", "winres")
	_ = os.MkdirAll(winresDir, 0750)
	winresPath := filepath.Join(winresDir, "winres.json")

	_ = os.WriteFile(versionFile, []byte("1.0.2\n"), 0600)
	_ = os.WriteFile(winresPath, []byte(`{"version":"1.0.2.0"}`), 0600)

	origWinresPath := winresJSONPath
	origWinresRunner := winresRunner
	defer func() {
		winresJSONPath = origWinresPath
		winresRunner = origWinresRunner
	}()

	winresJSONPath = winresPath
	dummySyso := filepath.Join(tmpDir, "cmd", "nacho-flow", "rsrc_windows_amd64.syso")
	_ = os.WriteFile(dummySyso, []byte("fake-syso"), 0600)

	winresRunner = func(inJson, outPrefix string) ([]string, error) {
		return []string{dummySyso}, nil
	}

	var committedFiles []string
	mockGit := func(args ...string) error {
		if len(args) > 0 && args[0] == "add" {
			committedFiles = append(committedFiles, args[1:]...)
		}
		return nil
	}
	mockOut := func(args ...string) (string, error) { return "main", nil }

	err := runWithRunners([]string{"cmd", "patch"}, versionFile, "", "", "", "", mockGit, mockOut)
	if err != nil {
		t.Fatalf("expected success with winres, got: %v", err)
	}

	foundWinres := false
	foundSyso := false
	for _, f := range committedFiles {
		if f == winresPath {
			foundWinres = true
		}
		if f == dummySyso {
			foundSyso = true
		}
	}
	if !foundWinres {
		t.Errorf("expected winres.json to be committed")
	}
	if !foundSyso {
		t.Errorf("expected syso file to be committed")
	}
}
