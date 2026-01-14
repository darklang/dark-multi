# Phase 5: Polish & Extras

## Error Handling

### User-Friendly Errors

```go
// internal/tui/errors.go
package tui

type errorModel struct {
    err     error
    context string
}

func showError(err error, context string) tea.Cmd {
    return func() tea.Msg {
        return errorModel{err: err, context: context}
    }
}

func (m HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case errorModel:
        m.lastError = msg
        // Auto-clear after 5 seconds
        return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
            return clearErrorMsg{}
        })
    }
}
```

### Error Display

```go
func (m HomeModel) View() string {
    // ... normal view ...

    if m.lastError.err != nil {
        errorBox := errorStyle.Render(fmt.Sprintf(
            "Error: %s\n%v",
            m.lastError.context,
            m.lastError.err,
        ))
        return lipgloss.JoinVertical(lipgloss.Left, view, errorBox)
    }

    return view
}
```

## Input Prompts

### New Branch Dialog

```go
// internal/tui/prompt.go
package tui

import (
    "github.com/charmbracelet/bubbles/textinput"
)

type newBranchPrompt struct {
    nameInput textinput.Model
    baseInput textinput.Model
    focused   int
}

func NewBranchPrompt() newBranchPrompt {
    name := textinput.New()
    name.Placeholder = "branch-name"
    name.Focus()

    base := textinput.New()
    base.Placeholder = "main"
    base.SetValue("main")

    return newBranchPrompt{
        nameInput: name,
        baseInput: base,
    }
}

func (p newBranchPrompt) View() string {
    return lipgloss.JoinVertical(lipgloss.Left,
        "Create new branch",
        "",
        "Name: " + p.nameInput.View(),
        "Base: " + p.baseInput.View(),
        "",
        helpStyle.Render("enter: create • esc: cancel • tab: switch field"),
    )
}
```

### Confirmation Dialog

```go
type confirmPrompt struct {
    message string
    onYes   tea.Cmd
}

func (p confirmPrompt) View() string {
    return lipgloss.JoinVertical(lipgloss.Left,
        p.message,
        "",
        helpStyle.Render("y: yes • n: no • esc: cancel"),
    )
}
```

## Help Screen

```go
type helpModel struct{}

func (m helpModel) View() string {
    return lipgloss.JoinVertical(lipgloss.Left,
        titleStyle.Render("Help"),
        "",
        sectionStyle.Render("Navigation"),
        "  ↑/k     Move up",
        "  ↓/j     Move down",
        "  enter   Select / Open",
        "  esc     Back",
        "",
        sectionStyle.Render("Branch Actions"),
        "  n       New branch",
        "  s       Start selected",
        "  S       Stop selected",
        "  r       Remove selected",
        "",
        sectionStyle.Render("System"),
        "  d       Setup DNS",
        "  p       Toggle proxy",
        "  q       Quit",
        "",
        helpStyle.Render("Press any key to close"),
    )
}
```

## Progress Indicators

### Spinner for Long Operations

```go
import "github.com/charmbracelet/bubbles/spinner"

type HomeModel struct {
    // ...
    spinner   spinner.Model
    loading   bool
    loadingOp string
}

func NewHomeModel() HomeModel {
    s := spinner.New()
    s.Spinner = spinner.Dot
    s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

    return HomeModel{
        spinner: s,
        // ...
    }
}

func (m HomeModel) View() string {
    if m.loading {
        return fmt.Sprintf("%s %s...", m.spinner.View(), m.loadingOp)
    }
    // ... normal view
}
```

### Progress for Container Start

```go
func (m HomeModel) startBranch(b *branch.Branch) tea.Cmd {
    return tea.Batch(
        m.spinner.Tick,
        func() tea.Msg {
            m.loading = true
            m.loadingOp = fmt.Sprintf("Starting %s", b.Name)
            return nil
        },
        func() tea.Msg {
            err := container.Start(b)
            return branchStartedMsg{branch: b, err: err}
        },
    )
}
```

## Logging

### Structured Logging with Charm Log

```go
// internal/logging/log.go
package logging

import (
    "os"
    "github.com/charmbracelet/log"
)

var Logger *log.Logger

func Init() {
    Logger = log.NewWithOptions(os.Stderr, log.Options{
        ReportTimestamp: true,
        Prefix:          "multi",
    })
}

// Usage
logging.Logger.Info("Starting branch", "name", b.Name)
logging.Logger.Error("Failed to start", "error", err)
```

## Notifications

### Desktop Notifications

```go
// internal/notify/notify.go
package notify

import "os/exec"

func Send(title, message string) {
    switch runtime.GOOS {
    case "darwin":
        exec.Command("osascript", "-e",
            fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)).Run()
    case "linux":
        exec.Command("notify-send", title, message).Run()
    }
}

// Usage: when Claude starts waiting
notify.Send("Claude waiting", fmt.Sprintf("Branch %s needs your input", b.Name))
```

## Config File (Optional)

```go
// internal/config/file.go
package config

import (
    "os"
    "gopkg.in/yaml.v3"
)

type UserConfig struct {
    ProxyPort        int      `yaml:"proxy_port"`
    AutoStartProxy   bool     `yaml:"auto_start_proxy"`
    NotifyOnWaiting  bool     `yaml:"notify_on_waiting"`
    RefreshInterval  int      `yaml:"refresh_interval_seconds"`
    FavoriteCanvases []string `yaml:"favorite_canvases"`
}

func LoadUserConfig() (*UserConfig, error) {
    path := filepath.Join(ConfigDir, "config.yaml")
    data, err := os.ReadFile(path)
    if err != nil {
        return &UserConfig{
            ProxyPort:       9000,
            AutoStartProxy:  true,
            RefreshInterval: 5,
        }, nil
    }

    var cfg UserConfig
    yaml.Unmarshal(data, &cfg)
    return &cfg, nil
}
```

## Final Checklist

### Error Handling
- [ ] User-friendly error messages
- [ ] Error display in TUI
- [ ] Auto-clear errors
- [ ] Graceful degradation

### Input
- [ ] New branch prompt
- [ ] Confirmation dialogs
- [ ] Text input validation

### Help
- [ ] Help screen ('?')
- [ ] Contextual hints

### Progress
- [ ] Spinner for async ops
- [ ] Loading state display

### Polish
- [ ] Consistent styling
- [ ] Responsive layout
- [ ] Smooth transitions
- [ ] Keyboard shortcuts help

### Optional
- [ ] Desktop notifications
- [ ] Config file support
- [ ] Structured logging
- [ ] Debug mode

## Testing

```bash
# Manual test checklist
./multi                     # TUI mode
./multi ls                  # CLI ls
./multi new test-branch     # CLI new
./multi start test-branch   # CLI start
./multi stop test-branch    # CLI stop
./multi urls                # CLI urls
./multi proxy start         # CLI proxy
./multi setup-dns           # CLI dns

# TUI tests
# - Navigate branches
# - Start/stop from TUI
# - View branch detail
# - Open URL in browser
# - Back navigation
# - Help screen
# - Error display
# - Window resize
```

## Build & Release

```bash
# Build for current platform
go build -o multi ./cmd/multi

# Cross-compile
GOOS=darwin GOARCH=arm64 go build -o multi-darwin-arm64 ./cmd/multi
GOOS=linux GOARCH=amd64 go build -o multi-linux-amd64 ./cmd/multi

# Install locally
ln -sf $(pwd)/multi ~/.local/bin/multi
```
