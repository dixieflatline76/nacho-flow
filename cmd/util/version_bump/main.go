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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/util/version_bump/main.go <bump-type>")
		fmt.Println("Where <bump-type> is one of: patch, minor, major (or -type=patch, etc.)")
		os.Exit(1)
	}

	bumpType := os.Args[1]
	bumpType = strings.TrimPrefix(bumpType, "-type=")
	bumpType = strings.TrimPrefix(bumpType, "--type=")

	// 1. Ensure we are on 'main', switching if necessary
	currentBranch, err := getCurrentBranch()
	if err != nil {
		fmt.Println("Error determining current branch:", err)
		os.Exit(1)
	}
	if currentBranch != "main" {
		fmt.Printf("Currently on '%s', switching to 'main'...\n", currentBranch)
		if err := runGit("checkout", "main"); err != nil {
			fmt.Println("Error checking out main:", err)
			os.Exit(1)
		}
	}

	// 2. Always pull latest from origin
	fmt.Println("Pulling latest from origin/main...")
	if err := runGit("pull", "origin", "main"); err != nil {
		fmt.Println("Error pulling from origin:", err)
		os.Exit(1)
	}

	// 3. Read and parse version.txt
	version, err := readVersionFromFile("version.txt")
	if err != nil {
		fmt.Println("Error reading version from file:", err)
		os.Exit(1)
	}

	// 4. Bump version
	newVersion, err := bumpVersion(version, bumpType)
	if err != nil {
		fmt.Println("Error incrementing version:", err)
		os.Exit(1)
	}

	// 5. Write new version to version.txt
	err = writeVersionToFile("version.txt", newVersion)
	if err != nil {
		fmt.Println("Error writing new version to file:", err)
		os.Exit(1)
	}

	// 6. Commit version.txt
	filesToCommit := []string{"version.txt"}
	err = commitVersionFiles(filesToCommit, newVersion.String())
	if err != nil {
		fmt.Println("Error committing version file:", err)
		os.Exit(1)
	}

	// 7. Create Git tag
	err = createGitTag(newVersion.String())
	if err != nil {
		fmt.Println("Error creating Git tag:", err)
		os.Exit(1)
	}

	// 8. Push commit to main
	fmt.Println("Pushing commit to origin/main...")
	if err := runGit("push", "origin", "main"); err != nil {
		fmt.Println("Error pushing to origin:", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully bumped to %s, tagged, and pushed to origin.\n", newVersion.String())
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
func createGitTag(versionString string) error {
	// #nosec G204 - version_bump executes trusted git commands
	cmd := exec.Command("git", "tag", "-a", versionString, "-m", fmt.Sprintf("Release %s", versionString))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// #nosec G204 - version_bump executes trusted git commands
	cmd = exec.Command("git", "push", "origin", versionString)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// commitVersionFiles commits the version files to git.
func commitVersionFiles(filenames []string, version string) error {
	args := append([]string{"add"}, filenames...)
	// #nosec G204 - version_bump executes trusted git commands
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	commitMsg := fmt.Sprintf("Bump version to %s", version)
	// #nosec G204 - version_bump executes trusted git commands
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getCurrentBranch returns the name of the current git branch.
func getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git branch failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// runGit executes a git command with the given arguments.
func runGit(args ...string) error {
	// #nosec G204 - version_bump executes trusted git commands
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
