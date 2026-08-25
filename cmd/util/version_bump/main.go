package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version with major, minor, and patch components.
type Version struct {
	Major  int
	Minor  int
	Patch  int
	Raw    string
	Prefix string // "v" or ""
}

// GitRunner executes git commands.
type GitRunner func(args ...string) error

// OutputRunner executes git commands and captures stdout.
type OutputRunner func(args ...string) (string, error)

func defaultGitRunner(args ...string) error {
	// #nosec G204 - version_bump executes trusted git commands
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func defaultOutputRunner(args ...string) (string, error) {
	// #nosec G204 - version_bump executes trusted git commands
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

var exitFunc = os.Exit

func main() {
	if err := runCLI(os.Args); err != nil {
		fmt.Println("Error:", err)
		exitFunc(1)
	}
}

func runCLI(args []string) error {
	return runWithRunners(args, "version.txt", "site/index.html", "extension/package.json", "extension/package-lock.json", defaultGitRunner, defaultOutputRunner)
}

func runWithRunners(args []string, versionFile, siteFile, pkgJsonFile, pkgLockFile string, git GitRunner, outRunner OutputRunner) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: go run cmd/util/version_bump/main.go <bump-type>\nWhere <bump-type> is one of: patch, minor, major (or -type=patch, etc.)")
	}

	bumpType := args[1]
	bumpType = strings.TrimPrefix(bumpType, "-type=")
	bumpType = strings.TrimPrefix(bumpType, "--type=")

	// 1. Ensure we are on 'main', switching if necessary
	currentBranch, err := outRunner("branch", "--show-current")
	if err != nil {
		return fmt.Errorf("error determining current branch: %w", err)
	}
	if currentBranch != "main" {
		fmt.Printf("Currently on '%s', switching to 'main'...\n", currentBranch)
		if err := git("checkout", "main"); err != nil {
			return fmt.Errorf("error checking out main: %w", err)
		}
	}

	// 2. Always pull latest from origin
	fmt.Println("Pulling latest from origin/main...")
	if err := git("pull", "origin", "main"); err != nil {
		return fmt.Errorf("error pulling from origin: %w", err)
	}

	// 3. Read and parse version.txt
	version, err := readVersionFromFile(versionFile)
	if err != nil {
		return fmt.Errorf("error reading version from file: %w", err)
	}

	// 4. Bump version
	newVersion, err := bumpVersion(version, bumpType)
	if err != nil {
		return fmt.Errorf("error incrementing version: %w", err)
	}

	// 5. Write new version to version.txt, site/index.html, and extension/package.json
	err = writeVersionToFile(versionFile, newVersion)
	if err != nil {
		return fmt.Errorf("error writing new version to file: %w", err)
	}

	filesToCommit := []string{versionFile}
	if err := updateSiteVersion(siteFile, newVersion); err == nil {
		filesToCommit = append(filesToCommit, siteFile)
	}
	if pkgJsonFile != "" {
		if err := updatePackageJSON(pkgJsonFile, newVersion); err == nil {
			filesToCommit = append(filesToCommit, pkgJsonFile)
		}
	}
	if pkgLockFile != "" {
		if err := updatePackageJSON(pkgLockFile, newVersion); err == nil {
			filesToCommit = append(filesToCommit, pkgLockFile)
		}
	}

	// 6. Commit version files
	err = commitVersionFiles(filesToCommit, newVersion.String(), git)
	if err != nil {
		return fmt.Errorf("error committing version file: %w", err)
	}

	// 7. Create Git tag
	err = createGitTag(newVersion.String(), git)
	if err != nil {
		return fmt.Errorf("error creating Git tag: %w", err)
	}

	// 8. Push commit to main
	fmt.Println("Pushing commit to origin/main...")
	if err := git("push", "origin", "main"); err != nil {
		return fmt.Errorf("error pushing to origin: %w", err)
	}

	fmt.Printf("Successfully bumped to %s, tagged, and pushed to origin.\n", newVersion.String())
	return nil
}

// readVersionFromFile reads the version string from the specified file.
func readVersionFromFile(filename string) (Version, error) {
	data, err := os.ReadFile(filepath.Clean(filename))
	if err != nil {
		return Version{}, err
	}

	versionString := strings.TrimSpace(string(data))
	return parseVersion(versionString)
}

// parseVersion parses a version string into a Version struct.
func parseVersion(versionString string) (Version, error) {
	re := regexp.MustCompile(`^(v?)(\d+)\.(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(versionString)

	if matches == nil {
		return Version{}, fmt.Errorf("invalid version format: %s", versionString)
	}

	major, _ := strconv.Atoi(matches[2])
	minor, _ := strconv.Atoi(matches[3])
	patch, _ := strconv.Atoi(matches[4])

	return Version{
		Major:  major,
		Minor:  minor,
		Patch:  patch,
		Prefix: matches[1],
		Raw:    versionString,
	}, nil
}

// bumpVersion increments the version based on the bump type.
func bumpVersion(v Version, bumpType string) (Version, error) {
	switch bumpType {
	case "patch":
		v.Patch++
	case "minor":
		v.Minor++
		v.Patch = 0
	case "major":
		v.Major++
		v.Minor = 0
		v.Patch = 0
	default:
		return v, fmt.Errorf("invalid bump type: %s", bumpType)
	}

	v.Raw = v.String()
	return v, nil
}

// writeVersionToFile writes the new version string to the specified file.
func writeVersionToFile(filename string, v Version) error {
	// G306: Expect WriteFile permissions to be 0600
	versionStr := fmt.Sprintf("%d.%d.%d\n", v.Major, v.Minor, v.Patch)
	return os.WriteFile(filepath.Clean(filename), []byte(versionStr), 0600)
}

// String returns the formatted version string (e.g., "v1.2.4").
func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// createGitTag creates a Git tag with the given version string and pushes it.
func createGitTag(versionString string, git GitRunner) error {
	if err := git("tag", "-a", versionString, "-m", fmt.Sprintf("Release %s", versionString)); err != nil {
		return err
	}
	return git("push", "origin", versionString)
}

// commitVersionFiles commits the version files to git.
func commitVersionFiles(filenames []string, version string, git GitRunner) error {
	args := append([]string{"add"}, filenames...)
	if err := git(args...); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	commitMsg := fmt.Sprintf("Bump version to %s", version)
	return git("commit", "-m", commitMsg)
}

// updateSiteVersion updates the logo badge in site/index.html with the new version.
func updateSiteVersion(filename string, v Version) error {
	data, err := os.ReadFile(filepath.Clean(filename))
	if err != nil {
		return err
	}

	re := regexp.MustCompile(`(class="logo-badge[^"]*" id="version-badge">)[^<]*(</span>)`)
	updated := re.ReplaceAll(data, fmt.Appendf(nil, "${1}%s${2}", v.String()))

	return os.WriteFile(filepath.Clean(filename), updated, 0600)
}

// updatePackageJSON updates the "version" field in package.json/package-lock.json.
func updatePackageJSON(filename string, v Version) error {
	data, err := os.ReadFile(filepath.Clean(filename))
	if err != nil {
		return err
	}

	versionStr := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	re := regexp.MustCompile(`("version"\s*:\s*)"[^"]+"`)
	updated := re.ReplaceAll(data, fmt.Appendf(nil, `${1}"%s"`, versionStr))

	return os.WriteFile(filepath.Clean(filename), updated, 0600)
}

