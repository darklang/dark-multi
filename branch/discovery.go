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
func FindSourceRepo() string {
	// Check DARK_SOURCE
	if config.DarkSource != config.DarkRoot {
		gitPath := filepath.Join(config.DarkSource, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return config.DarkSource
		}
	}

	// Check for 'main' branch
	mainPath := filepath.Join(config.DarkRoot, "main")
	gitPath := filepath.Join(mainPath, ".git")
	if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
		return mainPath
	}

	// Check any existing managed branch (via overrides dir)
	for _, b := range GetManagedBranches() {
		if b.Exists() {
			return b.Path
		}
	}

	// Fall back to GitHub
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
