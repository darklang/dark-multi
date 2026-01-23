package queue

import "time"

// InitialTasks returns the initial set of tasks to queue.
// Priority: lower = start first. Easy tasks get lower priority numbers.
func InitialTasks() []Task {
	return []Task{
		// === Existing branches (priority 0-9, will be reviewed) ===
		{
			ID:       "ai-basics",
			Name:     "AI Basics (existing)",
			Prompt:   "", // Needs review
			Priority: 5,
		},
		{
			ID:       "bring-back-wasm",
			Name:     "Bring Back WASM (existing)",
			Prompt:   "", // Needs review
			Priority: 5,
		},
		{
			ID:       "extensible-cli",
			Name:     "Extensible CLI (existing)",
			Prompt:   "", // Needs review
			Priority: 5,
		},
		{
			ID:       "faster-package-reloading",
			Name:     "Faster Package Reloading (existing)",
			Prompt:   "", // Needs review
			Priority: 5,
		},

		// === Easy tasks (priority 10-19) ===
		{
			ID:       "wildcard-match-pattern",
			Name:     "Implement Wildcard Match Pattern",
			Priority: 10,
			Prompt: `Implement Wildcard Match Pattern

- See issue #5460
- Add LPWildcard
- Add general tests
- Close issue via text in commit message`,
		},
		{
			ID:       "fix-pipes",
			Name:     "Fix/flip Pipes (|>)",
			Priority: 11,
			Prompt: `Fix/flip Pipes (|>)

How does/will JS/TS do pipes?
If that's the same as F#, then Darklang is doing it wrong (strangely). Fix it.`,
		},
		{
			ID:       "vscode-extension-page",
			Name:     "Tidy VS Code Extension Page",
			Priority: 12,
			Prompt: `Tidy VS Code extension page

We have a VS Code extension that's WIP, very alpha.
It requires the CLI has already been downloaded (from GH Releases) and installed (run 'install' in the exe).

Please update the extension's "landing page" to note such, and include some brief text explaining what Darklang is, and the fact that it's a WIP, with the extension (at this point) meant for internal testing/usage.

Tangentially, somehow demand the vscode lockfile to be in sync with the package.json. We keep running into a dumb issue in CI.`,
		},
		{
			ID:       "vscode-theme",
			Name:     "VS Code Theme (Classic Style)",
			Priority: 13,
			Prompt: `Darklang VS Code Theme (Classic Style)

Find screenshots of and source code for Darklang Classic.
In this repo, create a VS Code theme in our existing extension, matching the color scheme of Darklang Classic.
Make usage optional.`,
		},
		{
			ID:       "find-todos-report",
			Name:     "Find TODOs/Cleanups Report",
			Priority: 14,
			Prompt: `Find TODOs, Cleanups, and create report

This codebase has a ton of TODO and CLEANUP marked inline. Some of them are involved and should be further punted, but some are likely low-hanging fruit, doable fully without human review/feedback.

Review all of these items.
For the latter group, write a document full of the TODOs/CLEANUPs, referenced by file and text, ## title and brief body with how to resolve.`,
		},
		{
			ID:       "find-deps-upgrade",
			Name:     "Find Dependencies to Upgrade",
			Priority: 15,
			Prompt: `Find and report dependencies to upgrade

The benefits, rough steps, etc.
The goal here is to just create one or more .md file I can review, committed to the repo.`,
		},
		{
			ID:       "review-stdlib-issue",
			Name:     "Review Stdlib Issue #5329",
			Priority: 16,
			Prompt: `Review Stdlib Issue

Find and read issue #5329
Identify things that could be done with relative ease.
Do them.
Test them.`,
		},

		// === Medium tasks (priority 20-29) ===
		{
			ID:       "httpclient-tests",
			Name:     "Uncomment HttpClient Tests",
			Priority: 20,
			Prompt: `Uncomment/backfill HttpClient tests

In backend/testfiles/httpclient, many old test files have been marked to be ignored, with a _ prepended to the file name.
I suspect many of these tests can/should be brought back with relatively little pain.

Review and bring those tests back. Where appropriate, delete dupes.
I'd recommend starting with just one test at a time, until you've gained some confidence.
The tests are run by ./scripts/run-backend-tests --filter-test-list HttpClient`,
		},
		{
			ID:       "backfill-tests",
			Name:     "Generally Backfill Tests",
			Priority: 21,
			Prompt: `Generally backfill tests

Review all F# tests in backend/tests/Tests
and .dark tests in backend/testfiles
Identify and fill in any, as you see fit
(ignore httpclient tests - another thread is uncommenting those files)`,
		},
		{
			ID:       "faster-tests",
			Name:     "Make Test-Running Faster",
			Priority: 22,
			Prompt: `Make test-running faster

See how long ./scripts/run-backend-tests takes.
See if it can be improved.
Are we allowing for as much concurrency as we safely can?
Ignore the package-reload time; another thread is working on that.`,
		},
		{
			ID:       "remove-ply",
			Name:     "Remove Ply",
			Priority: 23,
			Prompt: `Remove Ply

A long time ago, we added Ply to our F# solution.
At the time, there were alleged perf benefits.
In hindsight, it made the codebase noisier than the time save might be, and we'd like to rip it out in favor of async/task, whichever would be appropriate for our needs.`,
		},
		{
			ID:       "dotnet-10",
			Name:     "Upgrade to .NET 10",
			Priority: 24,
			Prompt:   `Upgrade to .NET 10`,
		},

		// === Larger tasks (priority 30-39) ===
		{
			ID:       "charm-tui",
			Name:     "Clone Charm TUI Stuff",
			Priority: 30,
			Prompt: `Clone Charm TUI stuff

Research tools for TUIs: Charm, FsSpectre, gui.cs, ratatui, etc.
Review some initial TUI building blocks we've started in CLI 'experiments'.

Build a more thorough set of UI components for building TUIs in Darklang, all following our Ethos in the CLAUDE.md.

Darklang.CLI.UI is a good root namespace to work in. Might be good to migrate whatever we have to that place.

Write .dark tests in backend/testfiles/execution around how these components render. Don't get too carried away writing too many tests, though.`,
		},
		{
			ID:       "sqlite-spike",
			Name:     "Sqlite Spike - Access, DSL",
			Priority: 31,
			Prompt: `Sqlite Spike -- Access, DSL

We embed Sqlite in our CLI/Runtime.
Internally, we access a data.db to store various data, in our host language, F#, using (library).

We'd like to avail access to this DB, as well as to user .db files, in our language Darklang, with a minimal set of Builtins.

Separately, a DSL for querying the DB might be cool.
Hack on that, but keep it separate/'above' the main solution, so I can remove it in case it's ugly.

Test what you can, ideally in .dark tests.
Update CLI and VS Code editing experience if relevant.`,
		},
		{
			ID:       "html-spike",
			Name:     "HTML Spike",
			Priority: 32,
			Prompt: `HTML Spike

Somewhere in ./packages, we have started some helper types and functions for creating HTML documents/pages.

Please review and expand upon this.
There are tests in backend/testfiles.
One problem to solve for: weird whitespace issue (meaningful whitespace in between tags).

Run tests with ./scripts/run-backend-tests --filter-test-list html
See source in WIP branch of darklang/website, and rewrite _matter_ page(s) in the packages canvas with this html functionality.`,
		},
		{
			ID:       "regex-spike",
			Name:     "RegEx Support Spike",
			Priority: 33,
			Prompt: `RegEx Support Spike

Initial steps:
- start with LibExe and F# tests
- then F# parser
- more tests (F#)
- .dark tests
- treesitter grammar
- .dark side of consuming that tree sitter tree
- .dark tests
- LSP updates
- any CLI updates?
- any VS Code updates?

Constraints:
- minimal builtins
- minimal impact to ProgramTypes and RuntimeTypes
- be ready to change syntax`,
		},
		{
			ID:       "reflection",
			Name:     "More Reflection",
			Priority: 34,
			Prompt: `More Reflection

Review the minimal reflection we have.
What else can/should we do?
Add few clean builtins, and package fns, and demo/test things.
I think right now we just have Builtin.reflect or something.

How might this relate to the CLI experience/app? Create a .md report with ideas.
Really keep F# code impact to be relatively minimal.`,
		},
		{
			ID:       "builtins-restrict",
			Name:     "Restrict Builtins to One Package Fn",
			Priority: 35,
			Prompt: `Only expose Builtins to one package fn each, where possible.

Darklang's set of available code is _mostly_ written in Darklang, but some core things are "builtins" written in F#.
Currently, any Dark fn may reference/use any builtin.
But we'd like to restrict things such that only ONE package fn may reference each builtin. We can do this gradually.

Commits should be:
- initial infrastructure with
	- type usageRestriction = AllowAny | AllowOne of Location
- Do all the easy restrictions (AllowOne)
- further commits for the harder restrictions

run-backend-tests at end to see if things work

Unfortunately, the runtime doesn't know about the Locations, so this will need to SOMEHOW be a restriction set at parse-time`,
		},

		// === AI Spike (priority 40, larger exploratory task) ===
		{
			ID:       "ai-spike",
			Name:     "AI Spike",
			Priority: 40,
			Prompt: `AI Spike

Do a spike on "AI Support" in Darklang:
- study/clone langchain
- a few vendor-specific packages (especially Claude Code)
- some generic packages for general use in Darklang.AI
- PT-style primitives: Prompt, Agent, Session
- any specs for us to create packages for?
- tests and demos where possible
- cool CLI UI for composing prompts? (just a sketch)
- VS Code impact, if any (minimal)
- create demo coding agents
- CLI UI and commands for usage? (just UI/feel, no deep impl.)
- how do we take advantage of Darklang's strengths to support distributed AI usage, agents, execution, etc.?
- see wip.darklang.com to determine our strengths, as well as any recent blog posts.`,
		},
	}
}

// PopulateInitialQueue adds initial tasks to the queue.
func PopulateInitialQueue() error {
	q := Get()

	for _, task := range InitialTasks() {
		// Don't overwrite existing tasks
		if q.Get(task.ID) == nil {
			status := StatusReady
			if task.Prompt == "" {
				status = StatusNeedsPrompt
			}
			q.Tasks[task.ID] = &Task{
				ID:        task.ID,
				Name:      task.Name,
				Prompt:    task.Prompt,
				Status:    status,
				Priority:  task.Priority,
				CreatedAt: task.CreatedAt,
			}
			if q.Tasks[task.ID].CreatedAt.IsZero() {
				q.Tasks[task.ID].CreatedAt = time.Now()
			}
		}
	}

	return q.Save()
}
