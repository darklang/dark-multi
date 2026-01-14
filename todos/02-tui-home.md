# Phase 2: TUI Home Screen

## Design

```
┌─ dark-multi ──────────────────────────────────────────────────────┐
│                                                                    │
│  BRANCHES                                                          │
│  ────────────────────────────────────────────────────────────────  │
│  ● main         [3 modified]     ⏳ waiting        localhost:11101 │
│  ● test         [clean]          🔄 working...     localhost:11201 │
│  ○ feature-x    [stopped]                                          │
│                                                                    │
│  System: 8 cores, 31GB RAM • 2/4 running • Proxy: ●                │
│                                                                    │
│  [n]ew  [s]tart  [S]top  [r]emove  [d]ns  [p]roxy  [q]uit         │
│  ↑↓ navigate • enter select • ? help                               │
└────────────────────────────────────────────────────────────────────┘
```

## Bubbletea Model

```go
// internal/tui/home.go
package tui

import (
    "github.com/charmbracelet/bubbles/list"
    tea "github.com/charmbracelet/bubbletea"
)

type HomeModel struct {
    branches     []*branch.Branch
    list         list.Model
    proxyRunning bool
    width        int
    height       int
    err          error
}

// Messages
type branchesLoadedMsg []*branch.Branch
type proxyStatusMsg bool
type errMsg error

func NewHomeModel() HomeModel { ... }
func (m HomeModel) Init() tea.Cmd { ... }
func (m HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { ... }
func (m HomeModel) View() string { ... }
```

## Key Bindings

```go
// internal/tui/keys.go
package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
    Up       key.Binding
    Down     key.Binding
    Enter    key.Binding
    New      key.Binding
    Start    key.Binding
    Stop     key.Binding
    Remove   key.Binding
    DNS      key.Binding
    Proxy    key.Binding
    Quit     key.Binding
    Help     key.Binding
}

var keys = keyMap{
    Up:     key.NewBinding(key.WithKeys("up", "k")),
    Down:   key.NewBinding(key.WithKeys("down", "j")),
    Enter:  key.NewBinding(key.WithKeys("enter")),
    New:    key.NewBinding(key.WithKeys("n")),
    Start:  key.NewBinding(key.WithKeys("s")),
    Stop:   key.NewBinding(key.WithKeys("S")),
    Remove: key.NewBinding(key.WithKeys("r")),
    DNS:    key.NewBinding(key.WithKeys("d")),
    Proxy:  key.NewBinding(key.WithKeys("p")),
    Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c")),
    Help:   key.NewBinding(key.WithKeys("?")),
}
```

## Styles

```go
// internal/tui/styles.go
package tui

import "github.com/charmbracelet/lipgloss"

var (
    titleStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("170"))

    runningStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("42"))  // green

    stoppedStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("241"))  // gray

    modifiedStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("214"))  // yellow

    waitingStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("214"))  // yellow

    workingStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("42"))  // green

    statusBarStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("241")).
        Background(lipgloss.Color("236"))

    helpStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("241"))
)
```

## Branch List Item

```go
// internal/tui/home.go

type branchItem struct {
    branch      *branch.Branch
    claudeState string  // "waiting", "working", "idle", ""
}

func (i branchItem) Title() string {
    indicator := "○"
    if i.branch.IsRunning() {
        indicator = "●"
    }
    return fmt.Sprintf("%s %s", indicator, i.branch.Name)
}

func (i branchItem) Description() string {
    parts := []string{}

    // Git status
    if i.branch.HasChanges() {
        parts = append(parts, modifiedStyle.Render("[modified]"))
    } else if i.branch.IsRunning() {
        parts = append(parts, "[clean]")
    }

    // Claude status
    switch i.claudeState {
    case "waiting":
        parts = append(parts, waitingStyle.Render("⏳ waiting"))
    case "working":
        parts = append(parts, workingStyle.Render("🔄 working..."))
    }

    // Port
    if i.branch.IsRunning() {
        parts = append(parts, fmt.Sprintf(":%d", i.branch.BwdPortBase()))
    }

    return strings.Join(parts, "  ")
}

func (i branchItem) FilterValue() string {
    return i.branch.Name
}
```

## Update Logic

```go
func (m HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch {
        case key.Matches(msg, keys.Quit):
            return m, tea.Quit

        case key.Matches(msg, keys.Enter):
            // Go to branch detail
            selected := m.list.SelectedItem().(branchItem)
            return NewBranchModel(selected.branch), nil

        case key.Matches(msg, keys.New):
            // Prompt for branch name, then create
            return m, m.promptNewBranch()

        case key.Matches(msg, keys.Start):
            selected := m.list.SelectedItem().(branchItem)
            return m, m.startBranch(selected.branch)

        case key.Matches(msg, keys.Stop):
            selected := m.list.SelectedItem().(branchItem)
            return m, m.stopBranch(selected.branch)

        case key.Matches(msg, keys.DNS):
            return m, m.setupDNS()

        case key.Matches(msg, keys.Proxy):
            return m, m.toggleProxy()
        }

    case branchesLoadedMsg:
        items := make([]list.Item, len(msg))
        for i, b := range msg {
            items[i] = branchItem{branch: b}
        }
        m.list.SetItems(items)
        return m, nil

    case proxyStatusMsg:
        m.proxyRunning = bool(msg)
        return m, nil

    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.list.SetSize(msg.Width-4, msg.Height-8)
        return m, nil
    }

    var cmd tea.Cmd
    m.list, cmd = m.list.Update(msg)
    return m, cmd
}
```

## View Rendering

```go
func (m HomeModel) View() string {
    // Title
    title := titleStyle.Render("dark-multi")

    // Branch list
    branchList := m.list.View()

    // Status bar
    cpuCores, ramGB := config.GetSystemResources()
    running := countRunning(m.branches)
    maxSuggested := config.SuggestMaxInstances()
    proxyIndicator := "○"
    if m.proxyRunning {
        proxyIndicator = "●"
    }
    statusBar := statusBarStyle.Render(fmt.Sprintf(
        "System: %d cores, %dGB RAM • %d/%d running • Proxy: %s",
        cpuCores, ramGB, running, maxSuggested, proxyIndicator,
    ))

    // Help
    help := helpStyle.Render("[n]ew [s]tart [S]top [r]emove [d]ns [p]roxy [q]uit • ↑↓ navigate • enter select")

    return lipgloss.JoinVertical(
        lipgloss.Left,
        title,
        "",
        branchList,
        "",
        statusBar,
        help,
    )
}
```

## App Entry Point

```go
// internal/tui/app.go
package tui

import (
    tea "github.com/charmbracelet/bubbletea"
)

func Run() error {
    p := tea.NewProgram(
        NewHomeModel(),
        tea.WithAltScreen(),
        tea.WithMouseCellMotion(),
    )
    _, err := p.Run()
    return err
}
```

## Checklist

- [ ] HomeModel struct defined
- [ ] Key bindings configured
- [ ] Styles defined
- [ ] Branch list rendering
- [ ] Status bar rendering
- [ ] Navigation working (up/down)
- [ ] Enter → branch detail
- [ ] 'n' → new branch prompt
- [ ] 's' → start selected branch
- [ ] 'S' → stop selected branch
- [ ] 'd' → DNS setup
- [ ] 'p' → toggle proxy
- [ ] 'q' → quit
- [ ] Window resize handling
- [ ] Async branch loading
