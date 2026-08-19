package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	bumpType := flag.String("type", "patch", "Bump type: patch, minor, or major")
	flag.Parse()

	data, err := os.ReadFile("version.txt")
	if err != nil {
		log.Fatalf("Failed to read version.txt: %v", err)
	}

	currentVersion := strings.TrimSpace(string(data))
	currentVersion = strings.TrimPrefix(currentVersion, "v")

	parts := strings.Split(currentVersion, ".")
	if len(parts) != 3 {
		log.Fatalf("Invalid version format in version.txt: %s", currentVersion)
	}

	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])

	switch *bumpType {
	case "patch":
		patch++
	case "minor":
		minor++
		patch = 0
	case "major":
		major++
		minor = 0
		patch = 0
	default:
		log.Fatalf("Unknown bump type: %s", *bumpType)
	}

	newVersion := fmt.Sprintf("%d.%d.%d", major, minor, patch)
	newTag := fmt.Sprintf("v%s", newVersion)

	if err := os.WriteFile("version.txt", []byte(newVersion+"\n"), 0600); err != nil {
		log.Fatalf("Failed to write version.txt: %v", err)
	}

	log.Printf("Bumped version: %s -> %s (tag: %s)", currentVersion, newVersion, newTag)

	// Git commit, tag, and push
	execCmd("git", "add", "version.txt")
	execCmd("git", "commit", "-m", fmt.Sprintf("Bump version to %s", newTag))
	execCmd("git", "tag", "-a", newTag, "-m", fmt.Sprintf("Release %s", newTag))
	execCmd("git", "push", "origin", "main")
	execCmd("git", "push", "origin", newTag)

	fmt.Printf("Successfully bumped to %s, created git tag %s, and pushed to origin\n", newVersion, newTag)
}

func execCmd(name string, args ...string) {
	// #nosec G204 - version_bump executes trusted git commands
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Command failed (%s %v): %v", name, args, err)
	}
}
