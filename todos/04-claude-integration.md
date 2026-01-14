# Phase 4: Claude Status Integration

## Goal

Detect Claude's state for each branch:
- **waiting** - Claude sent a message, waiting for user response
- **working** - Claude is currently processing/generating
- **idle** - No active conversation or completed

Also extract recent activity summary.

## Claude Data Locations

```
~/.claude/
├── projects/
│   └── -home-stachu-code-dark-<branch>/
│       ├── *.jsonl                    # Conversation transcripts
│       └── ...
├── todos/
│   └── *.json                         # Per-conversation todo lists
└── ...
```

## Approach 1: Parse Conversation Files

```go
// internal/claude/status.go
package claude

import (
    "bufio"
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "time"
)

type Status struct {
    State      string    // "waiting", "working", "idle"
    LastMsg    string    // Truncated last message
    LastUpdate time.Time
}

type ConversationMessage struct {
    Role      string `json:"role"`      // "user", "assistant"
    Content   string `json:"content"`
    Timestamp string `json:"timestamp"`
}

func GetStatus(branchPath string) (*Status, error) {
    // Find Claude project directory for this branch
    homeDir, _ := os.UserHomeDir()
    projectsDir := filepath.Join(homeDir, ".claude", "projects")

    // Claude encodes paths with dashes
    encodedPath := strings.ReplaceAll(branchPath, "/", "-")
    projectDir := filepath.Join(projectsDir, encodedPath)

    // Find most recent .jsonl file
    files, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
    if err != nil || len(files) == 0 {
        return &Status{State: "idle"}, nil
    }

    // Get most recent by mtime
    var mostRecent string
    var mostRecentTime time.Time
    for _, f := range files {
        info, _ := os.Stat(f)
        if info.ModTime().After(mostRecentTime) {
            mostRecent = f
            mostRecentTime = info.ModTime()
        }
    }

    // Read last few lines
    lastMsg, lastRole := readLastMessage(mostRecent)

    status := &Status{
        LastUpdate: mostRecentTime,
        LastMsg:    truncate(lastMsg, 50),
    }

    // Determine state
    if time.Since(mostRecentTime) > 30*time.Minute {
        status.State = "idle"
    } else if lastRole == "assistant" {
        status.State = "waiting"  // Claude sent last, waiting for user
    } else {
        status.State = "working"  // User sent last, Claude is working
    }

    return status, nil
}

func readLastMessage(filepath string) (content string, role string) {
    file, err := os.Open(filepath)
    if err != nil {
        return "", ""
    }
    defer file.Close()

    var lastLine string
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        lastLine = scanner.Text()
    }

    var msg ConversationMessage
    if err := json.Unmarshal([]byte(lastLine), &msg); err != nil {
        return "", ""
    }

    return msg.Content, msg.Role
}
```

## Approach 2: Check tmux Pane Content

More real-time but hackier - check what's visible in the Claude tmux pane:

```go
func GetStatusFromTmux(branchName string) (*Status, error) {
    // Capture pane content
    cmd := exec.Command("tmux", "capture-pane", "-t",
        fmt.Sprintf("dark:%s.1", branchName), "-p")
    out, err := cmd.Output()
    if err != nil {
        return &Status{State: "idle"}, nil
    }

    content := string(out)

    // Look for indicators
    if strings.Contains(content, "Waiting for input") ||
       strings.Contains(content, ">") {  // prompt
        return &Status{State: "waiting"}, nil
    }

    if strings.Contains(content, "...") ||
       strings.Contains(content, "Thinking") {
        return &Status{State: "working"}, nil
    }

    return &Status{State: "idle"}, nil
}
```

## Approach 3: Hybrid

Best of both:

```go
func GetStatus(branchName, branchPath string) *Status {
    // First try conversation file for recent activity
    fileStatus, _ := getStatusFromFiles(branchPath)

    // Then check tmux for real-time state
    tmuxStatus, _ := getStatusFromTmux(branchName)

    // Combine: prefer tmux for state, files for message
    return &Status{
        State:      tmuxStatus.State,
        LastMsg:    fileStatus.LastMsg,
        LastUpdate: fileStatus.LastUpdate,
    }
}
```

## Activity Summary

For the home screen, show what Claude was recently doing:

```go
func GetRecentActivity(branchPath string) string {
    status, err := GetStatus(branchPath)
    if err != nil || status.LastMsg == "" {
        return ""
    }

    // Extract action from message
    // Look for patterns like "I'll...", "Let me...", "Creating...", etc.
    msg := status.LastMsg

    // Truncate intelligently
    if len(msg) > 60 {
        // Try to break at word boundary
        msg = msg[:57] + "..."
    }

    return msg
}
```

## Integration with TUI

```go
// internal/tui/home.go

// Add to branchItem
type branchItem struct {
    branch       *branch.Branch
    claudeStatus *claude.Status
}

func (i branchItem) Description() string {
    parts := []string{}

    // Git status
    if i.branch.HasChanges() {
        parts = append(parts, modifiedStyle.Render("[modified]"))
    }

    // Claude status
    if i.claudeStatus != nil {
        switch i.claudeStatus.State {
        case "waiting":
            parts = append(parts, waitingStyle.Render("⏳ waiting"))
        case "working":
            parts = append(parts, workingStyle.Render("🔄 working"))
        }
        if i.claudeStatus.LastMsg != "" {
            parts = append(parts, dimStyle.Render(i.claudeStatus.LastMsg))
        }
    }

    return strings.Join(parts, "  ")
}

// Load Claude status async
func loadClaudeStatuses(branches []*branch.Branch) tea.Cmd {
    return func() tea.Msg {
        statuses := make(map[string]*claude.Status)
        for _, b := range branches {
            status, _ := claude.GetStatus(b.Name, b.Path)
            statuses[b.Name] = status
        }
        return claudeStatusesMsg(statuses)
    }
}
```

## Periodic Refresh

Update Claude status periodically:

```go
func (m HomeModel) Init() tea.Cmd {
    return tea.Batch(
        loadBranches,
        loadProxyStatus,
        tickEvery(5 * time.Second),  // Refresh every 5s
    )
}

func tickEvery(d time.Duration) tea.Cmd {
    return tea.Tick(d, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}

func (m HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tickMsg:
        // Refresh Claude statuses
        return m, loadClaudeStatuses(m.branches)
    // ...
    }
}
```

## Checklist

- [ ] claude package created
- [ ] Conversation file parsing
- [ ] Status detection from files
- [ ] Status detection from tmux (optional)
- [ ] Last message extraction
- [ ] Activity summary truncation
- [ ] Integration with HomeModel
- [ ] Integration with BranchDetailModel
- [ ] Periodic refresh (5s)
- [ ] Handle missing/empty Claude data gracefully
