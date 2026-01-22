package task

import (
	"os"
	"path/filepath"
	"strings"
)

// InjectTaskContext writes the task context to CLAUDE.md in the branch.
func (t *Task) InjectTaskContext() error {
	claudeMdPath := filepath.Join(t.BranchPath, "CLAUDE.md")

	// Read existing CLAUDE.md
	existing := ""
	if data, err := os.ReadFile(claudeMdPath); err == nil {
		existing = string(data)
	}

	// Remove old task context if present
	if strings.Contains(existing, "<!-- TASK CONTEXT START -->") {
		existing = removeTaskContext(existing)
	}

	// Build task context based on phase
	var taskContext string
	phase := t.Phase()

	switch phase {
	case PhasePlanning:
		taskContext = t.planningContext()
	case PhaseReady:
		taskContext = t.readyContext()
	case PhaseExecuting:
		taskContext = t.executingContext()
	default:
		return nil // No context to inject
	}

	// Prepend to CLAUDE.md
	newContent := taskContext + existing
	return os.WriteFile(claudeMdPath, []byte(newContent), 0644)
}

func (t *Task) planningContext() string {
	prePrompt := t.PrePrompt()

	return "<!-- TASK CONTEXT START -->\n" +
		"# Active Task - Planning Phase\n\n" +
		"You have been given a task to complete. Your job is to:\n\n" +
		"1. **Research** the codebase to understand what needs to be done\n" +
		"2. **Create a plan** with specific, actionable todos\n" +
		"3. **Signal** when planning is complete\n\n" +
		"## The Task\n\n" +
		prePrompt + "\n\n" +
		"## Your Instructions\n\n" +
		"1. Read and understand the relevant code\n" +
		"2. Create `.claude-task/todos.md` with a detailed checklist of specific tasks\n" +
		"3. **When planning is complete**, write \"ready\" to `.claude-task/phase`:\n" +
		"   ```bash\n" +
		"   echo \"ready\" > .claude-task/phase\n" +
		"   ```\n" +
		"   This signals the TUI that you're done planning.\n\n" +
		"4. Tell the user: \"Planning complete. Review .claude-task/todos.md, then press 'r' in multi to start the Ralph loop.\"\n\n" +
		"## What happens next\n\n" +
		"After user approves, the Ralph loop will:\n" +
		"- Run you repeatedly until all todos are done\n" +
		"- You read CLAUDE.md and .claude-task/todos.md each iteration\n" +
		"- Mark todos as [x] when complete\n" +
		"- Write \"done\" to .claude-task/phase when ALL todos complete\n\n" +
		"## Important\n\n" +
		"- Be thorough in research before creating the plan\n" +
		"- Keep todos specific and actionable\n" +
		"- Include testing in the plan\n" +
		"- You can interact with the user now during planning\n\n" +
		"<!-- TASK CONTEXT END -->\n\n"
}

func (t *Task) readyContext() string {
	prePrompt := t.PrePrompt()

	// Read current todos
	todosPath := filepath.Join(t.ClaudeTaskDir(), "todos.md")
	todosContent := ""
	if data, err := os.ReadFile(todosPath); err == nil {
		todosContent = string(data)
	}

	return "<!-- TASK CONTEXT START -->\n" +
		"# Active Task - Ready for Review\n\n" +
		"Planning is complete. The user is reviewing the plan.\n\n" +
		"## The Task\n\n" +
		prePrompt + "\n\n" +
		"## Current Todos\n\n" +
		todosContent + "\n\n" +
		"## Status\n\n" +
		"Waiting for user to:\n" +
		"1. Review `.claude-task/todos.md`\n" +
		"2. Give feedback or approve\n" +
		"3. Press 'r' in multi to start the Ralph loop\n\n" +
		"You can discuss the plan with the user now.\n\n" +
		"<!-- TASK CONTEXT END -->\n\n"
}

func (t *Task) executingContext() string {
	prePrompt := t.PrePrompt()

	// Read current todos
	todosPath := filepath.Join(t.ClaudeTaskDir(), "todos.md")
	todosContent := ""
	if data, err := os.ReadFile(todosPath); err == nil {
		todosContent = string(data)
	}

	return "<!-- TASK CONTEXT START -->\n" +
		"# Active Task - Executing Phase (Ralph Loop)\n\n" +
		"You are in a Ralph Wiggum loop. Work through the todos systematically.\n\n" +
		"## The Task\n\n" +
		prePrompt + "\n\n" +
		"## Current Todos\n\n" +
		todosContent + "\n\n" +
		"## Instructions\n\n" +
		"1. Find the next uncompleted todo (marked with [ ])\n" +
		"2. Complete it\n" +
		"3. Mark it done in .claude-task/todos.md (change [ ] to [x])\n" +
		"4. Run tests to verify\n" +
		"5. Continue to next todo\n\n" +
		"## Commits\n\n" +
		"Commit early and often as you make progress:\n" +
		"- Short casual commit messages (e.g., \"add user auth\", \"fix login bug\")\n" +
		"- No attribution/co-author needed\n" +
		"- Commit after completing each logical chunk of work\n" +
		"- Don't wait until the end to commit everything\n\n" +
		"## When Done\n\n" +
		"When ALL todos are complete and tests pass:\n" +
		"- Make a final commit if there are uncommitted changes\n" +
		"- Write \"done\" to .claude-task/phase\n" +
		"- The loop will exit\n\n" +
		"## If Stuck\n\n" +
		"If stuck, just exit - the loop will restart you.\n" +
		"Leave notes in .claude-task/todos.md about what's blocking.\n\n" +
		"<!-- TASK CONTEXT END -->\n\n"
}

// RemoveTaskContext removes the injected task context from CLAUDE.md.
func (t *Task) RemoveTaskContext() error {
	claudeMdPath := filepath.Join(t.BranchPath, "CLAUDE.md")

	data, err := os.ReadFile(claudeMdPath)
	if err != nil {
		return nil
	}

	content := removeTaskContext(string(data))
	return os.WriteFile(claudeMdPath, []byte(content), 0644)
}

func removeTaskContext(content string) string {
	startMarker := "<!-- TASK CONTEXT START -->"
	endMarker := "<!-- TASK CONTEXT END -->"

	startIdx := strings.Index(content, startMarker)
	endIdx := strings.Index(content, endMarker)

	if startIdx == -1 || endIdx == -1 {
		return content
	}

	endIdx += len(endMarker)
	for endIdx < len(content) && (content[endIdx] == '\n' || content[endIdx] == '\r') {
		endIdx++
	}

	return content[:startIdx] + content[endIdx:]
}
