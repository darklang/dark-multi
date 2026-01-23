package queue

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/darklang/dark-multi/branch"
	"github.com/darklang/dark-multi/config"
	"github.com/darklang/dark-multi/task"
	"github.com/darklang/dark-multi/tmux"
)

var (
	processorRunning bool
	processorMu      sync.Mutex
)

// StartProcessor starts the background queue processor.
func StartProcessor() {
	processorMu.Lock()
	if processorRunning {
		processorMu.Unlock()
		return
	}
	processorRunning = true
	processorMu.Unlock()

	go runProcessor()
}

// StopProcessor stops the background queue processor.
func StopProcessor() {
	processorMu.Lock()
	processorRunning = false
	processorMu.Unlock()
}

// IsProcessorRunning returns true if the processor is running.
func IsProcessorRunning() bool {
	processorMu.Lock()
	defer processorMu.Unlock()
	return processorRunning
}

func runProcessor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Process immediately on start
	processQueue()

	for {
		processorMu.Lock()
		if !processorRunning {
			processorMu.Unlock()
			return
		}
		processorMu.Unlock()

		select {
		case <-ticker.C:
			processQueue()
		}
	}
}

// processQueue checks for tasks to start and monitors running tasks.
func processQueue() {
	q := Get()

	// Sync with actual container state (handles manually started containers)
	syncRunningContainers(q)

	// Sync task phases with queue status
	syncTaskPhases(q)

	// Start all ready tasks up to capacity (no waiting between starts)
	maxConcurrent := config.GetMaxConcurrent()
	for {
		running := q.CountRunning()
		if running >= maxConcurrent {
			break
		}

		task := q.NextReady()
		if task == nil {
			break
		}

		// Start the task
		if err := startTask(task); err != nil {
			q.SetError(task.ID, err.Error())
			q.Save()
			continue
		}

		q.UpdateStatus(task.ID, StatusRunning)
		q.Save()
	}
}

// syncTaskPhases updates queue status based on task phase files.
func syncTaskPhases(q *Queue) {
	for _, t := range q.GetByStatus(StatusRunning) {
		branchPath := filepath.Join(config.DarkRoot, t.ID)
		taskObj := task.New(t.ID, branchPath)

		phase := taskObj.Phase()
		switch phase {
		case task.PhaseDone:
			q.UpdateStatus(t.ID, StatusDone)
			// Clean up task files for a clean PR
			go func(taskObj *task.Task, branchPath string) {
				taskObj.Cleanup() // Removes .claude-task/ and cleans CLAUDE.md
			}(taskObj, branchPath)
			// Stop container when task is done to free resources
			b := branch.New(t.ID)
			if b.IsRunning() {
				go branch.Stop(b) // Stop in background to not block sync
			}
		case task.PhaseAuthError, task.PhaseError, task.PhaseMaxIterations:
			// Error states - mark as waiting for human intervention
			q.UpdateStatus(t.ID, StatusWaiting)
			q.SetError(t.ID, string(phase))
		case task.PhaseAwaitingAnswers, task.PhaseReadyForReview:
			// Needs human input
			q.UpdateStatus(t.ID, StatusWaiting)
		case task.PhaseNone:
			// Task was reset or deleted
			if t.Prompt != "" {
				q.UpdateStatus(t.ID, StatusReady)
			} else {
				q.UpdateStatus(t.ID, StatusNeedsPrompt)
			}
		}
	}
	q.Save()
}

// syncRunningContainers updates queue status based on actual running containers.
// This handles the case where containers were started manually before the queue existed.
func syncRunningContainers(q *Queue) {
	// Check tasks that are ready or needs-prompt - if their container is running, mark as running
	for _, t := range q.GetAll() {
		if t.Status == StatusRunning || t.Status == StatusDone {
			continue // Already in terminal state
		}

		b := branch.New(t.ID)
		if b.Exists() && b.IsRunning() {
			// Container is running, update queue status
			q.UpdateStatus(t.ID, StatusRunning)
		}
	}
	q.Save()
}

// startTask creates the branch if needed, sets up the task, and starts the ralph loop.
func startTask(t *Task) error {
	branchPath := filepath.Join(config.DarkRoot, t.ID)

	// Create branch if it doesn't exist
	b := branch.New(t.ID)
	if !b.Exists() {
		var err error
		b, err = branch.Create(t.ID)
		if err != nil {
			return fmt.Errorf("failed to create branch: %w", err)
		}
	}

	// Start container if not running
	if !b.IsRunning() {
		if err := branch.Start(b); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}
		// Wait for container to be ready
		time.Sleep(5 * time.Second)
	}

	// Set up task
	taskObj := task.New(t.ID, branchPath)

	// Set the prompt
	if err := taskObj.SetPrePrompt(t.Prompt); err != nil {
		return fmt.Errorf("failed to set pre-prompt: %w", err)
	}

	// Set up task infrastructure
	if err := taskObj.EnsureClaudeTaskDir(); err != nil {
		return fmt.Errorf("failed to create task dir: %w", err)
	}

	if err := taskObj.WriteInitialSetup(); err != nil {
		return fmt.Errorf("failed to write initial setup: %w", err)
	}

	// Set phase to executing
	taskObj.SetPhase(task.PhaseExecuting)

	// Copy ralph script
	if err := taskObj.CopyLoopScript(); err != nil {
		return fmt.Errorf("failed to copy loop script: %w", err)
	}

	// Inject task context into CLAUDE.md
	if err := taskObj.InjectTaskContext(); err != nil {
		return fmt.Errorf("failed to inject context: %w", err)
	}

	// Get container ID
	containerID, err := b.ContainerID()
	if err != nil {
		return fmt.Errorf("failed to get container ID: %w", err)
	}

	// Start ralph loop
	if err := tmux.StartRalphLoop(t.ID, containerID); err != nil {
		return fmt.Errorf("failed to start ralph loop: %w", err)
	}

	return nil
}

// ProcessOnce runs a single processing cycle (for CLI usage).
func ProcessOnce() error {
	processQueue()
	return nil
}
