// Package task provides task orchestration for AI-driven development.
package task

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/darklang/dark-multi/config"
)

// Phase represents the current phase of a task.
type Phase string

const (
	PhaseNone           Phase = ""                    // No task assigned
	PhasePlanning       Phase = "planning"            // AI creating plan, todos
	PhaseReady          Phase = "ready"               // Planning done, waiting for user to start Ralph
	PhaseExecuting      Phase = "executing"           // Ralph loop running
	PhaseDone           Phase = "done"                // Complete
	PhaseAuthError      Phase = "auth-error"          // Authentication failed
	PhaseError          Phase = "error"               // General error
	PhaseMaxIterations  Phase = "max-iterations-reached" // Hit iteration limit
	PhaseAwaitingAnswers Phase = "awaiting-answers"   // Needs human input
	PhaseReadyForReview Phase = "ready-for-review"    // Completed, needs review
)

// PhaseDisplay returns a human-readable display string for a phase.
func (p Phase) Display() string {
	switch p {
	case PhaseNone:
		return "no task"
	case PhasePlanning:
		return "planning"
	case PhaseReady:
		return "ready"
	case PhaseExecuting:
		return "executing"
	case PhaseDone:
		return "done"
	case PhaseAuthError:
		return "auth error"
	case PhaseError:
		return "error"
	case PhaseMaxIterations:
		return "max iterations"
	case PhaseAwaitingAnswers:
		return "needs input"
	case PhaseReadyForReview:
		return "review"
	default:
		return string(p)
	}
}

// PhaseIcon returns an icon for the phase.
func (p Phase) Icon() string {
	switch p {
	case PhaseNone:
		return "📝"
	case PhasePlanning:
		return "🔍"
	case PhaseReady:
		return "✋"
	case PhaseExecuting:
		return "⚡"
	case PhaseDone:
		return "✅"
	case PhaseAuthError:
		return "🔑"
	case PhaseError:
		return "❌"
	case PhaseMaxIterations:
		return "🔄"
	case PhaseAwaitingAnswers:
		return "❓"
	case PhaseReadyForReview:
		return "👀"
	default:
		return "?"
	}
}

// Task represents an AI-driven development task for a branch.
type Task struct {
	BranchName string
	TaskDir    string // ~/.config/dark-multi/tasks/<branch>/
	BranchPath string // ~/code/dark/<branch>/
}

// New creates a new Task for a branch.
func New(branchName, branchPath string) *Task {
	taskDir := filepath.Join(config.ConfigDir, "tasks", branchName)
	return &Task{
		BranchName: branchName,
		TaskDir:    taskDir,
		BranchPath: branchPath,
	}
}

// Exists returns true if a task exists for this branch.
func (t *Task) Exists() bool {
	_, err := os.Stat(t.prePromptPath())
	return err == nil
}

// Phase returns the current phase of the task.
func (t *Task) Phase() Phase {
	if !t.Exists() {
		return PhaseNone
	}

	// Check branch's .claude-task/phase (Claude/loop writes here)
	branchPhaseFile := filepath.Join(t.BranchPath, ".claude-task", "phase")
	if data, err := os.ReadFile(branchPhaseFile); err == nil {
		phase := Phase(strings.TrimSpace(string(data)))
		if phase != "" {
			return phase
		}
	}

	// Fall back to task dir phase file
	data, err := os.ReadFile(t.phasePath())
	if err != nil {
		return PhasePlanning // Has pre-prompt but no phase = planning
	}

	phase := Phase(strings.TrimSpace(string(data)))
	if phase == "" {
		return PhasePlanning
	}
	return phase
}

// SetPhase updates the task phase.
func (t *Task) SetPhase(phase Phase) error {
	if err := os.MkdirAll(t.TaskDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(t.phasePath(), []byte(string(phase)), 0644)
}

// PrePrompt returns the pre-prompt content.
func (t *Task) PrePrompt() string {
	data, err := os.ReadFile(t.prePromptPath())
	if err != nil {
		return ""
	}
	return string(data)
}

// SetPrePrompt saves the pre-prompt and initializes the task.
func (t *Task) SetPrePrompt(content string) error {
	if err := os.MkdirAll(t.TaskDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(t.prePromptPath(), []byte(content), 0644)
}

// TodoProgress returns (completed, total) todo counts from .claude-task/todos.md.
func (t *Task) TodoProgress() (int, int) {
	todosPath := filepath.Join(t.BranchPath, ".claude-task", "todos.md")
	data, err := os.ReadFile(todosPath)
	if err != nil {
		return 0, 0
	}

	content := string(data)
	total := strings.Count(content, "- [ ]") + strings.Count(content, "- [x]") + strings.Count(content, "- [X]")
	completed := strings.Count(content, "- [x]") + strings.Count(content, "- [X]")
	return completed, total
}

// StatusLine returns a short status for TUI display.
func (t *Task) StatusLine() string {
	phase := t.Phase()
	// Show todo progress for planning, ready, and executing phases
	if phase == PhasePlanning || phase == PhaseReady || phase == PhaseExecuting {
		done, total := t.TodoProgress()
		if total > 0 {
			return fmt.Sprintf("%d/%d", done, total)
		}
	}
	return ""
}

// PrePromptTemplate returns a template for a new pre-prompt.
func PrePromptTemplate(branchName string) string {
	return fmt.Sprintf(`# Task: %s

## Goal
[What should this accomplish?]

## Context
[Relevant background, files to look at, constraints]

## Success Criteria
[How do we know it's done? What tests should pass?]
`, branchName)
}

// File paths
func (t *Task) prePromptPath() string { return filepath.Join(t.TaskDir, "pre-prompt.md") }
func (t *Task) phasePath() string     { return filepath.Join(t.TaskDir, "phase") }

// PrePromptPath returns the path to pre-prompt.md for external editing.
func (t *Task) PrePromptPath() string { return t.prePromptPath() }

// ClaudeTaskDir returns the path to .claude-task/ in the branch.
func (t *Task) ClaudeTaskDir() string {
	return filepath.Join(t.BranchPath, ".claude-task")
}

// EnsureClaudeTaskDir creates the .claude-task directory in the branch.
func (t *Task) EnsureClaudeTaskDir() error {
	return os.MkdirAll(t.ClaudeTaskDir(), 0755)
}

// CopyLoopScript copies the Ralph loop script into the branch's .claude-task/.
func (t *Task) CopyLoopScript() error {
	if err := t.EnsureClaudeTaskDir(); err != nil {
		return err
	}

	// The loop script content
	script := `#!/bin/bash
# Ralph Wiggum loop - runs Claude until task complete
set -e

TASK_DIR=".claude-task"
PHASE_FILE="$TASK_DIR/phase"
MAX_ITERATIONS=${MAX_ITERATIONS:-100}
ITERATION=0

mkdir -p "$TASK_DIR"
echo "executing" > "$PHASE_FILE"

log() {
    echo "[ralph] $1"
    echo "$(date '+%H:%M:%S') $1" >> "$TASK_DIR/loop.log"
}

log "Starting Ralph loop (max $MAX_ITERATIONS iterations)"

while [ $ITERATION -lt $MAX_ITERATIONS ]; do
    ITERATION=$((ITERATION + 1))

    phase=$(cat "$PHASE_FILE" 2>/dev/null || echo "executing")
    if [ "$phase" = "done" ]; then
        log "Task complete!"
        break
    fi

    log "Iteration $ITERATION - running Claude"

    # Run Claude with a prompt - it reads CLAUDE.md which has the task context
    claude --dangerously-skip-permissions -p "Continue working on the task. Read CLAUDE.md for context and .claude-task/todos.md for the checklist. Complete the next unchecked todo." || true

    # Check phase after Claude exits
    phase=$(cat "$PHASE_FILE" 2>/dev/null || echo "executing")
    if [ "$phase" = "done" ]; then
        log "Task complete!"
        break
    fi

    log "Claude exited, restarting in 2s..."
    sleep 2
done

if [ $ITERATION -ge $MAX_ITERATIONS ]; then
    log "Max iterations reached"
fi

log "Loop finished"
`

	scriptPath := filepath.Join(t.ClaudeTaskDir(), "ralph.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return err
	}

	return nil
}

// WriteInitialSetup creates the initial .claude-task/todos.md and phase file.
func (t *Task) WriteInitialSetup() error {
	if err := t.EnsureClaudeTaskDir(); err != nil {
		return err
	}

	// Create initial todos.md
	todos := `# Task Todos

<!-- Claude will populate this during planning -->

- [ ] (Planning) Research and understand the codebase
- [ ] (Planning) Create detailed implementation plan
- [ ] (Planning) Update this todo list with specific tasks
`
	todosPath := filepath.Join(t.ClaudeTaskDir(), "todos.md")
	if err := os.WriteFile(todosPath, []byte(todos), 0644); err != nil {
		return err
	}

	// Set phase to planning
	phasePath := filepath.Join(t.ClaudeTaskDir(), "phase")
	if err := os.WriteFile(phasePath, []byte("planning"), 0644); err != nil {
		return err
	}

	return nil
}

// GetCreatedTime returns when the task was created.
func (t *Task) GetCreatedTime() time.Time {
	info, err := os.Stat(t.prePromptPath())
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// Cleanup removes task-related files for a clean PR.
// This commits any uncommitted work first, then removes .claude-task/ and cleans CLAUDE.md.
func (t *Task) Cleanup() error {
	gitDir := t.BranchPath

	// First, commit any uncommitted work (excluding task files)
	// Check if there's uncommitted work
	out, _ := exec.Command("git", "-C", gitDir, "status", "--porcelain").Output()
	hasUncommitted := len(strings.TrimSpace(string(out))) > 0

	if hasUncommitted {
		// Stage and commit the actual work first
		exec.Command("git", "-C", gitDir, "add", "-A").Run()
		exec.Command("git", "-C", gitDir, "commit", "-m", fmt.Sprintf("wip: %s", t.BranchName)).Run()
	}

	// Now remove task files
	claudeTaskDir := t.ClaudeTaskDir()
	if _, err := os.Stat(claudeTaskDir); err == nil {
		os.RemoveAll(claudeTaskDir)
	}

	// Remove injected task context from CLAUDE.md
	t.RemoveTaskContext()

	// Commit the cleanup separately (if there are changes)
	out, _ = exec.Command("git", "-C", gitDir, "status", "--porcelain").Output()
	if len(strings.TrimSpace(string(out))) > 0 {
		exec.Command("git", "-C", gitDir, "add", "-A").Run()
		exec.Command("git", "-C", gitDir, "commit", "-m", "cleanup: remove task management files").Run()
	}

	return nil
}
