package branch

import (
	"os"
	"path/filepath"
	"strings"
)

// StartupPhase represents a container startup milestone.
type StartupPhase int

const (
	PhaseNotStarted  StartupPhase = iota
	PhaseContainer                // Container starting
	PhaseTreeSitter               // Tree-sitter building
	PhaseFSharpBuild              // F# build in progress
	PhaseBwdServer                // BwdServer starting
	PhasePackages                 // Packages reloading
	PhaseReady                    // Fully ready
)

// StartupStatus represents the container's startup progress.
type StartupStatus struct {
	Phase       StartupPhase
	Description string
}

// GetStartupStatus checks the container's startup progress by parsing log files.
func (b *Branch) GetStartupStatus() StartupStatus {
	logsDir := filepath.Join(b.Path, "rundir", "logs")

	// Check build-server.log for progress
	buildLog := filepath.Join(logsDir, "build-server.log")
	buildContent, err := os.ReadFile(buildLog)
	if err != nil {
		// No log file yet - container is still starting
		return StartupStatus{PhaseContainer, "starting container"}
	}

	content := string(buildContent)

	// Empty log file means container just started
	if len(strings.TrimSpace(content)) == 0 {
		return StartupStatus{PhaseContainer, "starting container"}
	}

	// Check milestones in order (most complete first)
	if strings.Contains(content, "-- Initial compile succeeded --") {
		return StartupStatus{PhaseReady, "ready"}
	}

	if strings.Contains(content, "Done reloading packages") {
		return StartupStatus{PhaseReady, "ready"}
	}

	// Check bwdserver.log for server startup
	bwdLog := filepath.Join(logsDir, "bwdserver.log")
	if bwdContent, err := os.ReadFile(bwdLog); err == nil {
		if strings.Contains(string(bwdContent), "Now listening on:") {
			// BwdServer is up, waiting for packages
			if strings.Contains(content, "reload-packages") {
				return StartupStatus{PhasePackages, "loading packages"}
			}
			return StartupStatus{PhaseBwdServer, "bwdserver running"}
		}
	}

	// Check F# build progress
	if strings.Contains(content, "Build succeeded.") {
		return StartupStatus{PhaseBwdServer, "starting bwdserver"}
	}

	if strings.Contains(content, "dotnet build") || strings.Contains(content, "Restoring") {
		return StartupStatus{PhaseFSharpBuild, "building F#"}
	}

	// Check tree-sitter
	if strings.Contains(content, "tree-sitter") {
		if strings.Contains(content, ">> Success") && strings.Contains(content, "tree-sitter") {
			return StartupStatus{PhaseFSharpBuild, "building F#"}
		}
		return StartupStatus{PhaseTreeSitter, "building tree-sitter"}
	}

	return StartupStatus{PhaseNotStarted, "starting"}
}

// StartupProgress returns a progress indicator string (e.g., "[3/6]").
func (s StartupStatus) Progress() string {
	switch s.Phase {
	case PhaseNotStarted:
		return "[0/6]"
	case PhaseContainer:
		return "[1/6]"
	case PhaseTreeSitter:
		return "[2/6]"
	case PhaseFSharpBuild:
		return "[3/6]"
	case PhaseBwdServer:
		return "[4/6]"
	case PhasePackages:
		return "[5/6]"
	case PhaseReady:
		return "[6/6]"
	default:
		return "[?/6]"
	}
}
