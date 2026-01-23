// Package queue manages the task queue for automated processing.
package queue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/darklang/dark-multi/config"
)

// Status represents task status in the queue.
type Status string

const (
	StatusNeedsPrompt Status = "needs-prompt" // Waiting for prompt to be written
	StatusReady       Status = "ready"        // Has prompt, waiting for container slot
	StatusRunning     Status = "running"      // Container running, claude working
	StatusWaiting     Status = "waiting"      // Stuck or needs human input
	StatusDone        Status = "done"         // Completed
	StatusPaused      Status = "paused"       // Manually paused
)

// MaxConcurrent returns the configured max concurrent containers.
// Use config.GetMaxConcurrent() for the actual value.
var MaxConcurrent = 10 // Deprecated: use config.GetMaxConcurrent()

// Task represents a queued task.
type Task struct {
	ID          string    `json:"id"`           // Unique ID (usually branch name)
	Name        string    `json:"name"`         // Display name
	Prompt      string    `json:"prompt"`       // Task description/prompt
	Status      Status    `json:"status"`       // Current status
	Priority    int       `json:"priority"`     // Lower = higher priority
	CreatedAt   time.Time `json:"created_at"`   // When task was created
	StartedAt   time.Time `json:"started_at"`   // When container started
	CompletedAt time.Time `json:"completed_at"` // When task completed
	Error       string    `json:"error"`        // Error message if stuck
}

// Queue manages the task queue.
type Queue struct {
	Tasks map[string]*Task `json:"tasks"`
	mu    sync.RWMutex
}

var (
	instance *Queue
	once     sync.Once
)

// Get returns the singleton queue instance.
func Get() *Queue {
	once.Do(func() {
		instance = &Queue{
			Tasks: make(map[string]*Task),
		}
		instance.Load()
	})
	return instance
}

// queuePath returns the path to the queue file.
func queuePath() string {
	return filepath.Join(config.ConfigDir, "queue.json")
}

// Load loads the queue from disk.
func (q *Queue) Load() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := os.ReadFile(queuePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &q.Tasks)
}

// Save persists the queue to disk.
func (q *Queue) Save() error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if err := os.MkdirAll(config.ConfigDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(q.Tasks, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(queuePath(), data, 0644)
}

// Add adds a new task to the queue.
func (q *Queue) Add(id, name, prompt string, priority int) *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	status := StatusReady
	if prompt == "" {
		status = StatusNeedsPrompt
	}

	task := &Task{
		ID:        id,
		Name:      name,
		Prompt:    prompt,
		Status:    status,
		Priority:  priority,
		CreatedAt: time.Now(),
	}

	q.Tasks[id] = task
	return task
}

// Get returns a task by ID.
func (q *Queue) Get(id string) *Task {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.Tasks[id]
}

// UpdateStatus updates a task's status.
func (q *Queue) UpdateStatus(id string, status Status) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if task, ok := q.Tasks[id]; ok {
		task.Status = status
		if status == StatusRunning && task.StartedAt.IsZero() {
			task.StartedAt = time.Now()
		}
		if status == StatusDone && task.CompletedAt.IsZero() {
			task.CompletedAt = time.Now()
		}
	}
}

// SetPrompt sets the prompt for a task.
func (q *Queue) SetPrompt(id, prompt string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if task, ok := q.Tasks[id]; ok {
		task.Prompt = prompt
		if task.Status == StatusNeedsPrompt {
			task.Status = StatusReady
		}
	}
}

// SetError sets an error message and marks task as waiting.
func (q *Queue) SetError(id, err string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if task, ok := q.Tasks[id]; ok {
		task.Error = err
		task.Status = StatusWaiting
	}
}

// GetByStatus returns all tasks with a given status.
func (q *Queue) GetByStatus(statuses ...Status) []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	statusSet := make(map[Status]bool)
	for _, s := range statuses {
		statusSet[s] = true
	}

	var result []*Task
	for _, task := range q.Tasks {
		if statusSet[task.Status] {
			result = append(result, task)
		}
	}

	// Sort by priority, then by created time
	sort.Slice(result, func(i, j int) bool {
		if result[i].Priority != result[j].Priority {
			return result[i].Priority < result[j].Priority
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result
}

// statusOrder returns the sort order for a status (lower = first).
func statusOrder(s Status) int {
	switch s {
	case StatusDone:
		return 0
	case StatusRunning:
		return 1
	case StatusWaiting:
		return 2
	case StatusReady:
		return 3
	case StatusNeedsPrompt:
		return 4
	case StatusPaused:
		return 5
	default:
		return 9
	}
}

// GetAll returns all tasks sorted by status (done first), then priority.
func (q *Queue) GetAll() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var result []*Task
	for _, task := range q.Tasks {
		result = append(result, task)
	}

	sort.Slice(result, func(i, j int) bool {
		// First sort by status
		orderI, orderJ := statusOrder(result[i].Status), statusOrder(result[j].Status)
		if orderI != orderJ {
			return orderI < orderJ
		}
		// Then by priority
		if result[i].Priority != result[j].Priority {
			return result[i].Priority < result[j].Priority
		}
		// Then by creation time
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result
}

// CountRunning returns the number of running tasks.
func (q *Queue) CountRunning() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	count := 0
	for _, task := range q.Tasks {
		if task.Status == StatusRunning {
			count++
		}
	}
	return count
}

// NextReady returns the next ready task, or nil if none or at capacity.
func (q *Queue) NextReady() *Task {
	if q.CountRunning() >= MaxConcurrent {
		return nil
	}

	tasks := q.GetByStatus(StatusReady)
	if len(tasks) == 0 {
		return nil
	}

	return tasks[0]
}

// Remove removes a task from the queue.
func (q *Queue) Remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.Tasks, id)
}

// StatusIcon returns an icon for a status.
func (s Status) Icon() string {
	switch s {
	case StatusNeedsPrompt:
		return "📝"
	case StatusReady:
		return "⏳"
	case StatusRunning:
		return "🔄"
	case StatusWaiting:
		return "⏸️"
	case StatusDone:
		return "✅"
	case StatusPaused:
		return "⏹️"
	default:
		return "?"
	}
}

// StatusDisplay returns a display string for a status.
func (s Status) Display() string {
	switch s {
	case StatusNeedsPrompt:
		return "needs prompt"
	case StatusReady:
		return "ready"
	case StatusRunning:
		return "running"
	case StatusWaiting:
		return "waiting"
	case StatusDone:
		return "done"
	case StatusPaused:
		return "paused"
	default:
		return string(s)
	}
}
