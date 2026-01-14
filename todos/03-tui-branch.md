# Phase 3: TUI Branch Detail View

## Design

```
┌─ main ────────────────────────────────────────────────────────────┐
│                                                                    │
│  Container: dark-main (up 2h 34m)                                  │
│  Git: 3 files modified, 1 untracked                               │
│  Claude: "Refactoring the auth module to use JWT..."              │
│                                                                    │
│  URLS                                                              │
│  ────────────────────────────────────────────────────────────────  │
│  > dark-packages.main.dlio.localhost:9000                         │
│    dark-stdlib.main.dlio.localhost:9000                           │
│    (direct) localhost:11101                                       │
│                                                                    │
│  QUICK ACTIONS                                                     │
│  ────────────────────────────────────────────────────────────────  │
│    [g]it status   [l]ogs   [c]ode   [t]mux   [o]pen url           │
│                                                                    │
│  ← back  q quit                                                    │
└────────────────────────────────────────────────────────────────────┘
```

## Model

```go
// internal/tui/branch_detail.go
package tui

import (
    "github.com/charmbracelet/bubbles/list"
    tea "github.com/charmbracelet/bubbletea"
)

type BranchDetailModel struct {
    branch       *branch.Branch
    urlList      list.Model
    containerInfo string
    gitStatus    string
    claudeStatus string
    width        int
    height       int
}

func NewBranchDetailModel(b *branch.Branch) BranchDetailModel {
    // Create URL list
    urls := []list.Item{
        urlItem{
            url:  fmt.Sprintf("dark-packages.%s.dlio.localhost:%d", b.Name, config.ProxyPort),
            kind: "proxy",
        },
        urlItem{
            url:  fmt.Sprintf("localhost:%d", b.BwdPortBase()),
            kind: "direct",
        },
    }

    l := list.New(urls, urlDelegate{}, 0, 0)
    l.SetShowTitle(false)
    l.SetShowStatusBar(false)
    l.SetFilteringEnabled(false)

    return BranchDetailModel{
        branch:  b,
        urlList: l,
    }
}
```

## URL Item

```go
type urlItem struct {
    url  string
    kind string // "proxy", "direct"
}

func (i urlItem) Title() string {
    if i.kind == "direct" {
        return fmt.Sprintf("(direct) %s", i.url)
    }
    return i.url
}

func (i urlItem) Description() string { return "" }
func (i urlItem) FilterValue() string { return i.url }
```

## Key Bindings

```go
var branchKeys = struct {
    Back    key.Binding
    Open    key.Binding
    Git     key.Binding
    Logs    key.Binding
    Code    key.Binding
    Tmux    key.Binding
    Quit    key.Binding
}{
    Back:   key.NewBinding(key.WithKeys("esc", "backspace", "left")),
    Open:   key.NewBinding(key.WithKeys("enter", "o")),
    Git:    key.NewBinding(key.WithKeys("g")),
    Logs:   key.NewBinding(key.WithKeys("l")),
    Code:   key.NewBinding(key.WithKeys("c")),
    Tmux:   key.NewBinding(key.WithKeys("t")),
    Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c")),
}
```

## Update Logic

```go
func (m BranchDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch {
        case key.Matches(msg, branchKeys.Quit):
            return m, tea.Quit

        case key.Matches(msg, branchKeys.Back):
            // Return to home screen
            return NewHomeModel(), loadBranches

        case key.Matches(msg, branchKeys.Open):
            // Open selected URL in browser
            selected := m.urlList.SelectedItem().(urlItem)
            return m, openInBrowser(selected.url)

        case key.Matches(msg, branchKeys.Git):
            // Show git status in overlay or new view
            return m, showGitStatus(m.branch)

        case key.Matches(msg, branchKeys.Logs):
            // Show container logs
            return m, showLogs(m.branch)

        case key.Matches(msg, branchKeys.Code):
            // Open VS Code
            return m, openVSCode(m.branch)

        case key.Matches(msg, branchKeys.Tmux):
            // Attach to tmux (exits TUI)
            return m, attachTmux(m.branch)
        }

    case containerInfoMsg:
        m.containerInfo = string(msg)
        return m, nil

    case gitStatusMsg:
        m.gitStatus = string(msg)
        return m, nil

    case claudeStatusMsg:
        m.claudeStatus = string(msg)
        return m, nil

    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.urlList.SetSize(msg.Width-4, 5)
        return m, nil
    }

    var cmd tea.Cmd
    m.urlList, cmd = m.urlList.Update(msg)
    return m, cmd
}
```

## View Rendering

```go
func (m BranchDetailModel) View() string {
    // Title
    title := titleStyle.Render(m.branch.Name)

    // Info section
    info := lipgloss.JoinVertical(lipgloss.Left,
        fmt.Sprintf("Container: %s", m.containerInfo),
        fmt.Sprintf("Git: %s", m.gitStatus),
        fmt.Sprintf("Claude: %s", m.claudeStatus),
    )

    // URLs section
    urlsTitle := subtitleStyle.Render("URLS")
    urlsList := m.urlList.View()

    // Quick actions
    actionsTitle := subtitleStyle.Render("QUICK ACTIONS")
    actions := helpStyle.Render("[g]it status  [l]ogs  [c]ode  [t]mux  [o]pen url")

    // Footer
    footer := helpStyle.Render("← back  q quit")

    return lipgloss.JoinVertical(lipgloss.Left,
        title,
        "",
        info,
        "",
        urlsTitle,
        urlsList,
        "",
        actionsTitle,
        actions,
        "",
        footer,
    )
}
```

## Commands (Side Effects)

```go
// Open URL in browser
func openInBrowser(url string) tea.Cmd {
    return func() tea.Msg {
        fullURL := "http://" + url
        var cmd *exec.Cmd
        switch runtime.GOOS {
        case "darwin":
            cmd = exec.Command("open", fullURL)
        case "linux":
            cmd = exec.Command("xdg-open", fullURL)
        }
        cmd.Run()
        return nil
    }
}

// Open VS Code
func openVSCode(b *branch.Branch) tea.Cmd {
    return func() tea.Msg {
        exec.Command("devcontainer", "open", b.Path).Run()
        return nil
    }
}

// Attach tmux (exits TUI first)
func attachTmux(b *branch.Branch) tea.Cmd {
    return tea.ExecProcess(
        exec.Command("tmux", "attach", "-t", config.TmuxSession),
        nil,
    )
}

// Show git status
func showGitStatus(b *branch.Branch) tea.Cmd {
    return func() tea.Msg {
        out, _ := exec.Command("git", "-C", b.Path, "status", "--short").Output()
        return gitStatusMsg(out)
    }
}

// Get container info
func getContainerInfo(b *branch.Branch) tea.Cmd {
    return func() tea.Msg {
        id, err := b.ContainerID()
        if err != nil || id == "" {
            return containerInfoMsg("not running")
        }
        // Get uptime
        out, _ := exec.Command("docker", "inspect", "-f",
            "{{.State.StartedAt}}", id).Output()
        // Parse and format as "up Xh Ym"
        return containerInfoMsg(fmt.Sprintf("dark-%s (%s)", b.Name, formatUptime(out)))
    }
}
```

## Async Loading

```go
func (m BranchDetailModel) Init() tea.Cmd {
    return tea.Batch(
        getContainerInfo(m.branch),
        getGitStatus(m.branch),
        getClaudeStatus(m.branch),
    )
}

func getGitStatus(b *branch.Branch) tea.Cmd {
    return func() tea.Msg {
        out, _ := exec.Command("git", "-C", b.Path, "status", "--porcelain").Output()
        lines := strings.Split(strings.TrimSpace(string(out)), "\n")
        if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
            return gitStatusMsg("clean")
        }
        modified := 0
        untracked := 0
        for _, line := range lines {
            if strings.HasPrefix(line, "??") {
                untracked++
            } else {
                modified++
            }
        }
        return gitStatusMsg(fmt.Sprintf("%d modified, %d untracked", modified, untracked))
    }
}
```

## Checklist

- [ ] BranchDetailModel struct defined
- [ ] URL list rendering
- [ ] Container info display
- [ ] Git status display
- [ ] Claude status display (placeholder for phase 4)
- [ ] 'o'/enter → open URL in browser
- [ ] 'g' → show git status
- [ ] 'l' → show logs
- [ ] 'c' → open VS Code
- [ ] 't' → attach tmux
- [ ] esc/backspace → back to home
- [ ] Async info loading
