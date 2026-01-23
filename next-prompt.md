# Preable

I have some feedback on multi, and a bunch of tasks to work on.
I'd like some major changes to multi, such that we can start automatically queueing up, executing, and finishing tasks in a queue.

# Changes to Multi

- We need to be more automatic
- Rather than me creating branches manually, providing the prompt manually, and you doing some hand-holding along the way, I want to create a repository of tasks to complete, and you process that set as a _queue_. as you have availability, queue up the 'ready' tasks, start the containers, give them ralph and the initial prompt, and have the containers go to town until 'done' or stuck.
- No questions until you're stuck or done
- Prepend all you need to the CLAUDE.md to get the whole ralph setup working
- create TODOs folder with original-prompt.md
- todos.md
  - create "real" prompt
  - commit along the way
- I provide you prompt, and all else done until agent is stuck or done
	(needs prompt -> ready -> running -> waiting)
    (also: paused-container stopped)
- allow up to 10 branches running at a time - later we might increase this if we think resources can handle such
- list view shows all tasks. some (up to ten) will have a Container currently running, processing/completing. but most will be either Waiting FOr Review or Needs Prompt or Waiting for Container
- the Grid view should be  filterable by status. can multi-sepect statuses in cool modal that pops up. by default, probably should only include the Running containers, and (like we have now) display the summaries of what's happening
- early work (for this 'multi' prompt, as well as for in each container) should be focused on making a plan
- ensure multi can revover from crashes at any point
- if we've somehow lost it, bring back single Container page (focus) with whatever's relevant
- separately, some focused Task page might make sense
- we need to switch to a different kind of auth for claude stuff. Let's use the anthropic API key. it's really arduous to manually auth, or copy/mount the claude auth stuff. so stop mounting that, and instead copy the API key that the host has
- turn this document into a TODOs for yourself, somehow, and slowly iterate on them...
- here's an initial wave of tasks to queue up
- (include the already-running branches)
- queue up the _easy_ tasks first.
- I should have a way to manually spin up a container, for the sake of testing, hours/whenever after the container was running (and then stopped) for codewriting.

# Tasks to queue up

## Clone Charm TUI stuff
Research tools for TUIs: Charm, FsSpectre, gui.cs, ratatui, etc.
Review some initial TUI building blocks we've started in CLI 'experiments'.

Build a more thorough set of UI components for building TUIs in Darklang, all following our Ethos in the CLAUDE.md

Darklang.CLI.UI is a good root namespace to work in. Might be good to migrate whatever we have to that place.

Write .dark tests in backend/testfiles/execution around how tese components render. Don't get too carried away writing too many tests, though.


## Sqlite Spike -- Access, DSL

We embed Sqlite in our CLI/Runtime.
Internally, we access a data.db to store various data, in our host language, F#, using (library).

We'd like to avail access to this DB, as well as to user .db files, in our language Darklang, with a minimal set of Builtins. Something like (library).

Separately, a DSL for querying the DB might be cool.
Hack on that, but keep it separate/'above' the main solution, so I can remove it in case it's ugly.

Test what you can, ideally in .dark tests.
Update CLI and VS Code editing experience if relevant.


## Darklang VS Code _Theme_ Included in Extension
(-classic style)

Find screenshots of and source code for Darklang Classic.
In this repo, create a VS Code theme in our existing extension, matching the color scheme of Darklang Classic.
Make usage optional.


## Implement Wildcard Match Pattern

- see issue #5460
- add LPWildcard
 add general tests
- close issue via text in committing


## Fix/flip Pipes (|>)

How does/will JS/TS do pipes?
If that's the same as F#, then Darklang is doing it wrong (strangely). Fix it.


## AI Spike

Do a spike on "AI Support" in Darklang
- study/clone langchain
- a few vendor-specific packages (especially Claude Code)
- some generic packages for general use in Darklang.AI
- PT-style primitives: Prompt, Agent, Session
- any specs for us to create packages for?
- tests and dmos where possible
- cool CLI UI for composing prompts? (just a sketch)
- VS Code impact, if any (minimal)
- create demo coding agents
- CLI UI and commands for usage?
  (just UI/feel, no deep impl.)
- how do we take advantage of Darklang's strengths to support distributed AI usage, agentes, execution, etc.?
- see wip.darklang.com to determine our strengths, as well as any recent blog posts.


## Tidy VS Code extension page

We have a vs code extension that's wip, very alpha
it requires the CLI has already been downloaded (from GH Releases) and instealled (run 'install' in the exe)
Please update the extension's "landing page" to note such, and include some brief text explaining what Darklang is, and the fact that it's a WIP, with the extension (at this point) meant for internal testing/usage)

tangentially, somehow demand the vscode lockfile to be in sync with the package.json. we keep running into a dumb issue in CIU.


## Find TODOs, CLeanups, and create report

This codebase has a ton of TODO and CLEANUP marked inline. some of them are involved and should be further punted, but some are likely low-hanging fruit, doable fully without human review/feedback.
Review all of these items.
For the latter group, write a document full of the TODOs/CLEANUPs, referenced by file and text, ## title and brief body with how to resolve (like this wave of TODOs I'm giving you)


## HTML Spike

Somewhere in ./packages, we have started some helper types and functions for creating HTML documents/pages.

Please review and expand up on this.
There are tests in backend/testfiles
One problem to solve for: weird whitespace issue
(meaningful whitespace in between tags)
run tests with ./scripts/run-backend-tests --filter-test-list html
see source in WIP branch of darklang/website, and rewrite _matter_ page(s) in the packages canvas with this html functionality.


## Uncomment/backfill HttpClient tests

In backend/testfiles/httpclient, many old test files have been marked to be ignored, with a _ prepended to the file name.
I suspect many of these tests can/should be brought back with relatively little pain.
Review and bring those tests back.Where appropriate, delete dupes.
I'd recommend starting with just one test a a time, until you've gained some confidence.
The tests are run by ./scripts/run-backend-tests --filter-test-list HttpClient


## Generally backfill tests

Review all F# tests in backend/tests/Tests
and .dark tests in backend/testfiles
Identify and fill in any, as you see fit
(ignore httpclient tests - another thread is uncommenting those files)


## Make test-running faster
See how long ./scripts/run-backend-tests takes.
See if it can be improved.
Are we allowing for as much concurrency as we safely can?
Ignore the package-reload time; another thread is working on that.


## Find and report dependencies to upgrade
The benefits, rough steps, etc.
The goal here is to just create one or more .md file I can review, committed to the repo.


## Remove Ply
A long time ago, we added Ply to our F# solution.
At the time, there were alleged perf benefits.
In hindsight, it made the codebase noisier than the time save might be, and we'd like to rip it out in favor of async/task, whichever would be appropriate for our needs.


## RegEx Support Spike
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
- any VS COde updates?

Constraints:
- minimal builtins
- minimal impact to ProgramTypes and RuntimeTypes
- be ready to change syntax


## Review Stdlib Issue

Find and read issue #5329
Identify things that could be done with relative ease.
Do them
Test them


## Upgrade to .NET 10


## More Reflection
Review the minimal reflection we have.
What else can/should we do?
Add few clean builtins, and package fns, and demo/test things.
I thnk right now we just have Builtin.reflect or something
How might this relate to the CLI experience/app? Create a .md report iwth ideas
Really keep F# code impact to be relatively minimal.


## Only expose Builtins to one package fn each, where possible.
Darklang's set of available code is _mostly_ written in Darklang, but some core thngs are "builtins" written in F#.
Currently, any Dark fn may reference/use any builtin.
But we'd like to restrict things such that only ONE package fn may reference each builtin. we can do this gradually.

Commits should be:
- initial infrastructure with 
	- type usageRestriction = AllowAny | AllowOne of Location
- Do all oe easy restrictions (AllowOne)
- futher commits for the harder restrictions

run-backend-tests at end to see if things work

Unfortunately, the runtime doesn't know about the Locations, so this will need to SOMEHOW be a restriction set at parse-time



