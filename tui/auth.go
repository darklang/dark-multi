package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/darklang/dark-multi/branch"
)

// AuthModel handles Claude authentication for a branch.
type AuthModel struct {
	branch    *branch.Branch
	status    string
	authURL   string
	codeInput string
	done      bool
	err       error
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	width     int
	height    int
}

// Auth messages
type authURLFoundMsg struct {
	url    string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader
}
type authDoneMsg struct{}
type authErrMsg struct{ err error }
type authNeededMsg struct {
	branch *branch.Branch
	needed bool
}

// CheckAuthNeeded checks if a branch needs Claude authentication.
func CheckAuthNeeded(b *branch.Branch) tea.Cmd {
	return func() tea.Msg {
		containerID, err := b.ContainerID()
		if err != nil {
			// Can't get container ID - assume auth is needed so we don't silently skip
			return authNeededMsg{b, true}
		}

		// Wait a moment for container to be ready for exec
		time.Sleep(2 * time.Second)

		// Check if credentials file exists
		cmd := exec.Command("docker", "exec", containerID, "test", "-f", "/home/dark/.claude/.credentials.json")
		err = cmd.Run()
		return authNeededMsg{b, err != nil} // needed if file doesn't exist
	}
}

// NewAuthModel creates an auth view for a branch.
func NewAuthModel(b *branch.Branch) AuthModel {
	return AuthModel{
		branch: b,
		status: "Starting Claude auth...",
	}
}

// Init starts the auth process.
func (m AuthModel) Init() tea.Cmd {
	return m.startAuth()
}

func (m AuthModel) startAuth() tea.Cmd {
	return func() tea.Msg {
		containerID, err := m.branch.ContainerID()
		if err != nil {
			return authErrMsg{fmt.Errorf("container not running: %w", err)}
		}

		// Don't short-circuit on credentials check - always run Claude to verify
		// it's properly configured (theme, auth, etc.)

		// Open log file for debugging
		logFile, _ := os.OpenFile("/tmp/dark-multi-auth.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		defer func() {
			if logFile != nil {
				logFile.Close()
			}
		}()
		logLine := func(s string) {
			if logFile != nil {
				logFile.WriteString(s + "\n")
			}
		}

		logLine("Starting auth for container: " + containerID)

		// Run claude with stdin/stdout pipes - use script to fake a TTY
		cmd := exec.Command("docker", "exec", "-i", containerID,
			"script", "-q", "-c", "claude", "/dev/null")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return authErrMsg{err}
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return authErrMsg{err}
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			return authErrMsg{err}
		}

		logLine("Started claude process")

		// Scan for OAuth URL - look for anthropic auth URLs
		urlRegex := regexp.MustCompile(`https://[^\s]*(anthropic|claude)[^\s]*`)
		scanner := bufio.NewScanner(stdout)

		themeSent := false
		for scanner.Scan() {
			line := scanner.Text()
			logLine("OUTPUT: " + line)

			// Handle theme selection prompt - send "1" for dark mode
			lower := strings.ToLower(line)
			if !themeSent && (strings.Contains(lower, "choose the text style") || strings.Contains(lower, "dark mode") && strings.Contains(lower, "light mode")) {
				logLine("Detected theme prompt, sending '1'")
				time.Sleep(200 * time.Millisecond)
				stdin.Write([]byte("1\n"))
				themeSent = true
				continue
			}

			// Look for OAuth URL
			if match := urlRegex.FindString(line); match != "" {
				logLine("Found OAuth URL: " + match)
				// Found URL - return it along with process handles
				return authURLFoundMsg{
					url:    match,
					cmd:    cmd,
					stdin:  stdin,
					stdout: stdout,
				}
			}

			// Claude started successfully - look for the actual interactive prompt
			// NOT "Welcome to Claude Code" which appears before theme selection
			if strings.Contains(line, "What would you like") ||
			   strings.Contains(line, "How can I help") ||
			   strings.Contains(lower, "successfully authenticated") {
				logLine("Detected Claude ready/authenticated")
				cmd.Process.Kill()
				return authDoneMsg{}
			}
		}

		logLine("Scanner finished, no URL found")
		cmd.Wait()
		return authErrMsg{fmt.Errorf("auth flow ended without URL - check /tmp/dark-multi-auth.log")}
	}
}

// waitForAuthComplete continues scanning stdout for success message
func waitForAuthComplete(stdout io.Reader) tea.Cmd {
	return func() tea.Msg {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Successfully") || strings.Contains(line, "authenticated") || strings.Contains(line, "Welcome") {
				return authDoneMsg{}
			}
		}
		// Stream ended - either success or failure
		return authDoneMsg{}
	}
}

// Update handles messages.
func (m AuthModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			// Cancel auth
			if m.cmd != nil && m.cmd.Process != nil {
				m.cmd.Process.Kill()
			}
			grid := NewGridModel()
			return grid, grid.Init()

		case "enter":
			// Submit code
			if m.authURL != "" && m.codeInput != "" && m.stdin != nil {
				// Send code to claude's stdin
				m.stdin.Write([]byte(m.codeInput + "\n"))
				m.codeInput = ""
				m.status = "Authenticating..."
				return m, nil
			}

		case "backspace":
			if len(m.codeInput) > 0 {
				m.codeInput = m.codeInput[:len(m.codeInput)-1]
			}
			return m, nil

		default:
			// Accept alphanumeric input for code
			key := msg.String()
			if len(key) == 1 && isValidCodeChar(key[0]) {
				m.codeInput += key
			}
			return m, nil
		}

	case authURLFoundMsg:
		m.authURL = msg.url
		m.cmd = msg.cmd
		m.stdin = msg.stdin
		m.status = "Opening browser... enter the code from the page:"
		// Auto-open URL in browser
		openInBrowser(msg.url)
		// Start background goroutine to wait for auth completion
		return m, waitForAuthComplete(msg.stdout)

	case authDoneMsg:
		m.done = true
		m.status = "Authentication complete!"
		if m.cmd != nil && m.cmd.Process != nil {
			m.cmd.Process.Kill()
		}
		// Auto-return to grid after short delay
		grid := NewGridModel()
		return grid, grid.Init()

	case authErrMsg:
		m.err = msg.err
		m.status = fmt.Sprintf("Error: %v", msg.err)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// View renders the auth screen.
func (m AuthModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("── Authenticate Claude for %s ──", m.branch.Name)))
	b.WriteString("\n\n")

	b.WriteString("  " + m.status)
	b.WriteString("\n\n")

	if m.authURL != "" && !m.done {
		// Code input field
		b.WriteString("  Code: ")
		b.WriteString(m.codeInput)
		b.WriteString("█")
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  1. Authorize in the browser window that opened"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  2. Copy the code shown after authorization"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  3. Type code above and press Enter"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  If browser didn't open:"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  " + m.authURL))
		b.WriteString("\n")
	}

	if m.done {
		b.WriteString("\n")
		b.WriteString(runningStyle.Render("  Ready to use Claude"))
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  [esc] cancel"))
	b.WriteString("\n")

	return b.String()
}

// isValidCodeChar returns true for characters valid in auth codes
func isValidCodeChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}
