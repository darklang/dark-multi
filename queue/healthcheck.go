package queue

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/task"
	"github.com/darklang/dark-multi/tmux"
)

// HealthIssue represents a detected issue with a task.
type HealthIssue struct {
	TaskID   string
	Severity string // "warning", "error", "info"
	Message  string
	Action   string // Suggested action
}

// RunHealthCheck evaluates the state of all tasks and returns any issues found.
func RunHealthCheck() []HealthIssue {
	var issues []HealthIssue
	q := Get()

	for _, t := range q.GetAll() {
		taskIssues := checkTask(t)
		issues = append(issues, taskIssues...)
	}

	return issues
}

func checkTask(t *Task) []HealthIssue {
	var issues []HealthIssue
	branchPath := filepath.Join(config.DarkRoot, t.ID)
	b := branch.New(t.ID)
	taskObj := task.New(t.ID, branchPath)

	// Check 1: Container state vs queue state mismatch
	containerRunning := b.Exists() && b.IsRunning()
	if t.Status == StatusRunning && !containerRunning {
		issues = append(issues, HealthIssue{
			TaskID:   t.ID,
			Severity: "warning",
			Message:  "Marked as running but container is stopped",
			Action:   "restart",
		})
	}

	// Check 2: Task marked done but has uncommitted work
	if t.Status == StatusDone {
		uncommitted := countUncommittedFiles(branchPath)
		if uncommitted > 0 {
			issues = append(issues, HealthIssue{
				TaskID:   t.ID,
				Severity: "warning",
				Message:  fmt.Sprintf("Marked done but has %d uncommitted files", uncommitted),
				Action:   "commit",
			})
		}
	}

	// Check 3: Check for stuck Claude (running but no output change)
	if t.Status == StatusRunning && tmux.ClaudeSessionExists(t.ID) {
		paneContent := tmux.CapturePaneContent(t.ID, 50)
		if isClaudeStuck(paneContent) {
			issues = append(issues, HealthIssue{
				TaskID:   t.ID,
				Severity: "warning",
				Message:  "Claude may be stuck (auth issue or waiting for input)",
				Action:   "check",
			})
		}
	}

	// Check 4: Phase file says done but queue doesn't reflect it
	if taskObj.Exists() {
		phase := taskObj.Phase()
		if phase == task.PhaseDone && t.Status != StatusDone {
			issues = append(issues, HealthIssue{
				TaskID:   t.ID,
				Severity: "info",
				Message:  "Phase is done, updating queue status",
				Action:   "sync",
			})
			// Auto-fix: update queue status
			q := Get()
			q.UpdateStatus(t.ID, StatusDone)
			q.Save()
		}

		// Check for error phases
		if phase == task.PhaseAuthError || phase == task.PhaseError || phase == task.PhaseMaxIterations {
			issues = append(issues, HealthIssue{
				TaskID:   t.ID,
				Severity: "error",
				Message:  fmt.Sprintf("Task in error state: %s", phase),
				Action:   "fix",
			})
		}
	}

	// Check 5: Running for too long without progress
	if t.Status == StatusRunning && !t.StartedAt.IsZero() {
		runningFor := time.Since(t.StartedAt)
		if runningFor > 4*time.Hour {
			issues = append(issues, HealthIssue{
				TaskID:   t.ID,
				Severity: "warning",
				Message:  fmt.Sprintf("Running for %s without completion", runningFor.Round(time.Minute)),
				Action:   "check",
			})
		}
	}

	return issues
}

func countUncommittedFiles(branchPath string) int {
	// Use git status --porcelain to count uncommitted files
	cmd := fmt.Sprintf("git -C %s status --porcelain 2>/dev/null | wc -l", branchPath)
	out, err := runCommand(cmd)
	if err != nil {
		return 0
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &count)
	return count
}

func isClaudeStuck(paneContent string) bool {
	// Check for signs that Claude is stuck
	lower := strings.ToLower(paneContent)

	// Auth issues
	if strings.Contains(lower, "invalid api key") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "auth conflict") {
		return true
	}

	// Waiting for OAuth
	if strings.Contains(lower, "platform.claude.com/oauth") ||
		strings.Contains(lower, "paste code here") {
		return true
	}

	// Rate limiting
	if strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") {
		return true
	}

	return false
}

func runCommand(cmd string) (string, error) {
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// AutoFix attempts to automatically fix detected issues.
func AutoFix(issues []HealthIssue) []string {
	var actions []string
	q := Get()

	for _, issue := range issues {
		switch issue.Action {
		case "sync":
			// Already handled in checkTask
			actions = append(actions, fmt.Sprintf("%s: synced status", issue.TaskID))

		case "commit":
			// Commit uncommitted work
			branchPath := filepath.Join(config.DarkRoot, issue.TaskID)
			exec.Command("git", "-C", branchPath, "add", "-A").Run()
			exec.Command("git", "-C", branchPath, "commit", "-m", "wip: uncommitted work").Run()
			actions = append(actions, fmt.Sprintf("%s: committed uncommitted work", issue.TaskID))

		case "restart":
			// Mark as ready so it can be restarted
			q.UpdateStatus(issue.TaskID, StatusReady)
			actions = append(actions, fmt.Sprintf("%s: marked ready for restart", issue.TaskID))
		}
	}

	q.Save()
	return actions
}
