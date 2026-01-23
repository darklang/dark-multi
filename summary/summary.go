// Package summary provides AI-powered summarization of Claude sessions.
package summary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/darklang/dark-multi/tmux"
)

// Cache stores summaries for branches
var (
	cache       = make(map[string]*CachedSummary)
	cacheMu     sync.RWMutex
	summarizing = make(map[string]bool)
	sumMu       sync.Mutex
)

// CachedSummary holds a cached summary with timestamp
type CachedSummary struct {
	Summary   string
	Iteration int
	UpdatedAt time.Time
}

// GetSummary returns the cached summary for a branch, or triggers generation.
func GetSummary(branchName string) string {
	cacheMu.RLock()
	cached, ok := cache[branchName]
	cacheMu.RUnlock()

	if ok && time.Since(cached.UpdatedAt) < 60*time.Second {
		return formatSummary(cached)
	}

	// Trigger async summarization if not already running
	go triggerSummarization(branchName)

	if ok {
		return formatSummary(cached) // Return stale while updating
	}

	// Return fallback immediately while waiting for first summary
	iter := getIteration(branchName)
	summary := getFallbackSummary(branchName)
	if iter > 0 || summary != "" {
		return formatResult(iter, summary)
	}
	return ""
}

func formatSummary(c *CachedSummary) string {
	return formatResult(c.Iteration, c.Summary)
}

func formatResult(iter int, summary string) string {
	if iter > 0 && summary != "" {
		return fmt.Sprintf("#%d • %s", iter, summary)
	}
	if iter > 0 {
		return fmt.Sprintf("#%d", iter)
	}
	if summary != "" {
		return summary
	}
	return ""
}

func triggerSummarization(branchName string) {
	sumMu.Lock()
	if summarizing[branchName] {
		sumMu.Unlock()
		return
	}
	summarizing[branchName] = true
	sumMu.Unlock()

	defer func() {
		sumMu.Lock()
		delete(summarizing, branchName)
		sumMu.Unlock()
	}()

	iter := getIteration(branchName)
	sum := generateSummary(branchName)
	if sum != "" || iter > 0 {
		cacheMu.Lock()
		cache[branchName] = &CachedSummary{
			Summary:   sum,
			Iteration: iter,
			UpdatedAt: time.Now(),
		}
		cacheMu.Unlock()
	}
}

// getIteration extracts the current iteration number from the ralph log
func getIteration(branchName string) int {
	logPath := tmux.GetOutputLogPath(branchName)
	content, err := readTail(logPath, 2048)
	if err != nil {
		return 0
	}

	// Look for "[ralph] Iteration N" pattern
	re := regexp.MustCompile(`\[ralph\] Iteration (\d+)`)
	matches := re.FindAllStringSubmatch(content, -1)
	if len(matches) > 0 {
		// Get the last match (most recent iteration)
		lastMatch := matches[len(matches)-1]
		var iter int
		fmt.Sscanf(lastMatch[1], "%d", &iter)
		return iter
	}
	return 0
}

func generateSummary(branchName string) string {
	logPath := tmux.GetOutputLogPath(branchName)

	// Read last ~4KB of log
	content, err := readTail(logPath, 4096)
	if err != nil || content == "" {
		return getFallbackSummary(branchName)
	}

	// Clean up ANSI escape codes and control characters
	content = cleanTerminalOutput(content)
	if len(content) < 50 {
		return getFallbackSummary(branchName)
	}

	// Check for API key - if not set, use fallback
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return getFallbackSummary(branchName)
	}

	// Call Haiku for summarization
	return callHaiku(content)
}

// getFallbackSummary extracts useful info from the log without AI
func getFallbackSummary(branchName string) string {
	logPath := tmux.GetOutputLogPath(branchName)
	content, err := readTail(logPath, 2048)
	if err != nil || content == "" {
		return ""
	}

	content = cleanTerminalOutput(content)
	lines := strings.Split(content, "\n")

	// Look for interesting patterns in reverse order
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-20; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Skip common noise
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "[ralph]") ||
			strings.Contains(lower, "iteration") ||
			strings.Contains(lower, "───") ||
			strings.Contains(lower, "╭") ||
			strings.Contains(lower, "╰") ||
			len(line) < 5 {
			continue
		}

		// Look for file operations
		if strings.Contains(line, "Reading") || strings.Contains(line, "Writing") ||
			strings.Contains(line, "Editing") || strings.Contains(line, "Created") {
			return truncate(line, 80)
		}

		// Look for tool usage
		if strings.Contains(line, "Read(") || strings.Contains(line, "Edit(") ||
			strings.Contains(line, "Write(") || strings.Contains(line, "Bash(") {
			return truncate(line, 80)
		}

		// Return first non-noise line
		return truncate(line, 80)
	}

	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func readTail(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	start := stat.Size() - maxBytes
	if start < 0 {
		start = 0
	}

	f.Seek(start, 0)
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func cleanTerminalOutput(s string) string {
	// Remove ANSI escape sequences
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		// Skip other control characters except newline/tab
		if r < 32 && r != '\n' && r != '\t' {
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

type claudeRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func callHaiku(content string) string {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return ""
	}

	prompt := `What is Claude doing RIGHT NOW? One short fragment, max 80 chars. No bullet, no period.

Good: editing auth.go to fix login timeout
Good: running pytest, 3 failures so far
Good: reading codebase to understand user model
Bad: Claude is currently working on implementing the authentication system for users

Output ONLY the fragment, nothing else.

Terminal output:
` + content

	reqBody := claudeRequest{
		Model:     "claude-3-5-haiku-20241022",
		MaxTokens: 100,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var result claudeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if len(result.Content) > 0 {
		text := strings.TrimSpace(result.Content[0].Text)
		// Remove any bullet or period Haiku might add
		text = strings.TrimPrefix(text, "•")
		text = strings.TrimPrefix(text, "-")
		text = strings.TrimSpace(text)
		text = strings.TrimSuffix(text, ".")
		return truncate(text, 80)
	}
	return ""
}

// ClearCache removes the cached summary for a branch.
func ClearCache(branchName string) {
	cacheMu.Lock()
	delete(cache, branchName)
	cacheMu.Unlock()
}
