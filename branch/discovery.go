package branch

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/darklang/dark-multi/config"
)

// FindNextInstanceID finds the next available instance ID.
func FindNextInstanceID() int {
	maxID := 0
	entries, err := os.ReadDir(config.OverridesDir)
	if err != nil {
		return 1
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(config.OverridesDir, entry.Name(), "metadata")
		content, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "ID=") {
				if id, err := strconv.Atoi(line[3:]); err == nil {
					if id > maxID {
						maxID = id
					}
				}
			}
		}
	}

	return maxID + 1
}

// FindSourceRepo finds a repo to clone from.
// Always clones from 'main' or upstream - never from other branches to avoid inheriting changes.
func FindSourceRepo() string {
	// Check DARK_SOURCE (explicit override)
	if config.DarkSource != config.DarkRoot {
		gitPath := filepath.Join(config.DarkSource, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return config.DarkSource
		}
	}

	// Check for 'main' branch (must be fully cloned - has devcontainer.json)
	// Only use 'main' - never use other branches as source
	mainPath := filepath.Join(config.DarkRoot, "main")
	devcontainerPath := filepath.Join(mainPath, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(devcontainerPath); err == nil {
		return mainPath
	}

	// Fall back to GitHub upstream - will clone fresh from origin/main
	return "git@github.com:darklang/dark.git"
}

// GetManagedBranches returns all managed branches, sorted by name.
func GetManagedBranches() []*Branch {
	var branches []*Branch

	entries, err := os.ReadDir(config.OverridesDir)
	if err != nil {
		return branches
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(config.OverridesDir, entry.Name(), "metadata")
		if _, err := os.Stat(metaPath); err == nil {
			b := New(entry.Name())
			branches = append(branches, b)
		}
	}

	sort.Slice(branches, func(i, j int) bool {
		return branches[i].Name < branches[j].Name
	})

	return branches
}
