// Package claude provides Claude status detection for dark-multi.
package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Status represents Claude's current state for a branch.
type Status struct {
	State      string    // "waiting", "working", "idle"
	LastMsg    string    // Truncated last message/activity
	LastUpdate time.Time // When the conversation was last updated
}

// Message represents a conversation message from Claude's JSONL files.
type Message struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// GetStatus returns Claude's status for a given branch path.
func GetStatus(branchPath string) *Status {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return &Status{State: "idle"}
	}

	// Claude encodes paths: /home/stachu/code/dark/main -> -home-stachu-code-dark-main
	encodedPath := strings.ReplaceAll(branchPath, "/", "-")

	projectsDir := filepath.Join(homeDir, ".claude", "projects")
	projectDir := filepath.Join(projectsDir, encodedPath)

	// Find .jsonl conversation files
	files, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil || len(files) == 0 {
		return &Status{State: "idle"}
	}

	// Find most recent file by modification time
	var mostRecent string
	var mostRecentTime time.Time
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().After(mostRecentTime) {
			mostRecent = f
			mostRecentTime = info.ModTime()
		}
	}

	if mostRecent == "" {
		return &Status{State: "idle"}
	}

	// Read last message from file
	lastMsg, lastRole := readLastMessage(mostRecent)

	status := &Status{
		LastUpdate: mostRecentTime,
		LastMsg:    truncate(lastMsg, 40),
	}

	// Determine state based on timing and last role
	timeSinceUpdate := time.Since(mostRecentTime)

	if timeSinceUpdate > 30*time.Minute {
		status.State = "idle"
	} else if timeSinceUpdate < 10*time.Second {
		// Very recent activity - likely working
		status.State = "working"
	} else if lastRole == "assistant" {
		// Claude sent last message, waiting for user
		status.State = "waiting"
	} else if lastRole == "user" {
		// User sent last, Claude should be working (or done)
		if timeSinceUpdate < 2*time.Minute {
			status.State = "working"
		} else {
			status.State = "idle"
		}
	} else {
		status.State = "idle"
	}

	return status
}

// readLastMessage reads the last assistant message from a JSONL file.
func readLastMessage(filepath string) (content string, role string) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", ""
	}
	defer file.Close()

	// Read all lines to find the last meaningful message
	var lastAssistantMsg string
	var lastRole string

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large messages
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		// Handle different message formats
		if msg.Type == "assistant" && msg.Message.Role == "assistant" {
			lastRole = "assistant"
			// Extract text from content blocks
			for _, block := range msg.Message.Content {
				if block.Type == "text" && block.Text != "" {
					lastAssistantMsg = block.Text
					break
				}
			}
		} else if msg.Type == "user" || msg.Role == "user" {
			lastRole = "user"
		}
	}

	return lastAssistantMsg, lastRole
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	// Try to break at a word boundary
	truncated := s[:maxLen-3]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxLen/2 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// GetStatusForBranches returns status for multiple branches efficiently.
func GetStatusForBranches(branchPaths []string) map[string]*Status {
	result := make(map[string]*Status)
	for _, path := range branchPaths {
		result[path] = GetStatus(path)
	}
	return result
}
