# How Entire Works: Building a Time Machine for AI Coding Sessions

*A deep dive into the architecture behind git-native AI session tracking*

---

If you've ever used an AI coding assistant like Claude Code, Cursor, or GitHub Copilot, you've probably experienced this: you're three prompts deep into a refactoring session, the agent makes a change that breaks everything, and suddenly you're playing detective—trying to remember what worked, what didn't, and how to get back to that moment 15 minutes ago when things were fine.

Entire is a CLI tool that solves this problem. It hooks into your git workflow to capture AI agent sessions as you work, creating checkpoints you can rewind to at any moment. Think of it as Time Machine for your AI coding sessions.

But how does it actually work? Let's pop the hood.

---

## The Problem: AI Sessions Are Ephemeral

When you work with an AI coding assistant, a lot happens:

- You submit prompts describing what you want
- The agent reads files, reasons about your codebase, and makes changes
- You iterate, refine, and eventually commit

But once you close that session, most of that context evaporates. The prompts, the reasoning, the intermediate states—gone. If you need to understand *why* code changed (not just *what* changed), you're left reconstructing it from memory.

This creates real problems:

1. **Recovery is painful** — When an agent goes off the rails, undoing its work means manual `git checkout` commands and hoping you remember which files changed
2. **Onboarding is harder** — New team members see commits but not the intent behind them
3. **Auditing is incomplete** — You can trace code to commits, but not to the AI interactions that produced them

Entire addresses all three by linking your git commits to the full session transcript—prompts, responses, files touched, and token usage.

---

## The Core Insight: Git Is Already a Time Machine

Here's the key architectural insight: **git is already a content-addressed storage system with branching, merging, and history traversal built in**. Rather than build a separate database, Entire stores everything in git itself.

This means:

- **No external services** — Everything lives in your repository
- **Collaboration works automatically** — Push your branch, and teammates get your session history
- **Existing tools work** — `git log`, `git diff`, and your IDE all understand the data

The challenge is doing this without polluting your commit history. Nobody wants their clean `main` branch cluttered with "AI checkpoint: step 47" commits.

---

## Architecture Overview

Entire has four main layers:

```
┌─────────────────────────────────────────────────────────────┐
│                         ENTIRE CLI                           │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│   Agents          Lifecycle        Strategy       Storage    │
│   (Claude,        Dispatcher       (Manual-       (Git       │
│    Gemini,        (Events)         Commit)        Trees)     │
│    Cursor)                                                   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

Let's walk through each one.

---

## Layer 1: The Agent Abstraction

Entire supports multiple AI coding assistants: Claude Code, Gemini CLI, Cursor, OpenCode, GitHub Copilot CLI, and Factory AI Droid. Each has its own hook format, transcript structure, and session storage.

Rather than writing separate logic for each, Entire defines a common `Agent` interface:

```go
type Agent interface {
    Name() string                              // "claude-code", "cursor"
    Type() string                              // "Claude Code", "Cursor"
    DetectPresence(ctx) (bool, error)          // Is this agent configured?
    ReadTranscript(sessionRef) ([]byte, error) // Read the session log
    // ... more methods
}
```

Each agent implements this interface by translating its native format to Entire's normalized model. For example, Claude Code stores transcripts as JSONL files in `.claude/projects/`, while Cursor uses a different structure in `.cursor/`.

This abstraction means the rest of the system doesn't care which agent you're using—it just works with normalized events and transcripts.

### Lifecycle Hooks

Agents that support hooks (most do) implement a second interface:

```go
type HookSupport interface {
    Agent
    HookNames() []string                                    // ["session-start", "stop", ...]
    ParseHookEvent(ctx, hookName, stdin) (*Event, error)    // Translate to normalized event
    InstallHooks(ctx, localDev, force) (int, error)         // Write hook config
}
```

When you run `entire enable --agent claude-code`, Entire writes to `.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [{"command": "entire hooks claude-code session-start"}],
    "Stop": [{"command": "entire hooks claude-code stop"}],
    "UserPromptSubmit": [{"command": "entire hooks claude-code user-prompt-submit"}]
  }
}
```

Now whenever Claude Code starts, stops, or receives a prompt, it invokes Entire. The agent doesn't know or care what Entire does—it just fires the hook and continues.

---

## Layer 2: The Lifecycle Dispatcher

When a hook fires, Entire needs to decide what to do. This is the lifecycle dispatcher's job.

First, it parses the agent-specific input into a normalized event:

```go
type Event struct {
    Type       EventType   // SessionStart, TurnStart, TurnEnd, etc.
    SessionID  string      // "2026-03-19-abc123..."
    SessionRef string      // Path to transcript file
    Prompt     string      // User's prompt (for TurnStart)
    Timestamp  time.Time
}
```

Then it routes the event to the appropriate handler:

```go
func DispatchLifecycleEvent(ctx, agent, event) error {
    switch event.Type {
    case SessionStart:
        return handleLifecycleSessionStart(ctx, agent, event)
    case TurnStart:
        return handleLifecycleTurnStart(ctx, agent, event)
    case TurnEnd:
        return handleLifecycleTurnEnd(ctx, agent, event)
    // ...
    }
}
```

Each handler performs bookkeeping: updating session state, recording file changes, creating checkpoints.

### The State Machine

Sessions have a lifecycle: they start, the user submits prompts, the agent works, and eventually the session ends. Entire models this as a state machine:

```
     TurnStart          TurnEnd
         │                 │
         ▼                 ▼
    ┌─────────┐       ┌─────────┐
    │  IDLE   │ ◀───▶ │ ACTIVE  │
    └─────────┘       └─────────┘
         │                 │
         └────────┬────────┘
                  │ SessionStop
                  ▼
             ┌─────────┐
             │  ENDED  │
             └─────────┘
```

The interesting part is what happens on `GitCommit`. If you commit while a session is active (or idle with uncommitted checkpoints), the state machine triggers a "condense" action—more on that below.

---

## Layer 3: The Manual-Commit Strategy

This is where the magic happens. The strategy determines *how* checkpoints are stored and *when* they're made permanent.

Entire uses a "manual-commit" strategy with two key principles:

1. **Never create commits on the user's branch** — Your commit history stays clean
2. **Store checkpoints on shadow branches** — Temporary state that disappears after you commit

### Shadow Branches

When you're working with an AI agent, Entire creates a shadow branch named after your current commit:

```
entire/abc1234-def567
       ───────  ──────
          │       │
          │       └── Worktree hash (for concurrent worktrees)
          └── First 7 chars of HEAD commit
```

Every time the agent finishes a turn, Entire writes a checkpoint to this branch:

```
Shadow branch tree:
├── <your working directory files>
└── .entire/
    └── metadata/
        └── 2026-03-19-session-id/
            ├── full.jsonl     # Complete transcript
            └── prompt.txt     # User prompts
```

This branch exists only in your local `.git`—it's never pushed. It's a scratch space for in-progress work.

### Condensation: Shadow → Permanent

When you run `git commit`, something interesting happens. Entire installs git hooks that intercept the commit process:

**prepare-commit-msg**: Before you edit the commit message, Entire adds a trailer:

```
Implement user authentication

Entire-Checkpoint: a3b2c4d5e6f7
```

This 12-character ID is the stable link between your commit and the session metadata.

**post-commit**: After git creates the commit, Entire "condenses" the shadow branch:

1. Read the transcript and metadata from the shadow branch
2. Calculate attribution (which lines came from the agent vs. human edits)
3. Write everything to a permanent branch: `entire/checkpoints/v1`
4. Delete the shadow branch

The permanent branch uses a sharded structure to avoid directory bloat:

```
entire/checkpoints/v1
├── a3/
│   └── b2c4d5e6f7/           # Checkpoint a3b2c4d5e6f7
│       ├── metadata.json      # Summary stats
│       └── 0/                 # Session data
│           ├── full.jsonl     # Transcript
│           ├── prompt.txt     # Prompts
│           └── metadata.json  # Token usage, attribution
```

Now you have bidirectional linking:

- **Commit → Metadata**: Parse the `Entire-Checkpoint` trailer, look up `a3/b2c4d5e6f7/`
- **Metadata → Commit**: Search branch history for commits with that trailer

---

## Layer 4: Git Tree Building

Here's where things get technically interesting. Entire needs to create git commits containing both your code *and* metadata, without ever touching your working directory.

It does this using go-git's plumbing APIs to build trees in memory:

```go
func (s *GitStore) WriteTemporary(ctx, opts) (WriteTemporaryResult, error) {
    // 1. Get the base tree (from HEAD)
    baseTreeHash := getHeadTree()
    
    // 2. Build a new tree with modifications
    treeHash := s.buildTreeWithChanges(ctx, baseTreeHash,
        opts.ModifiedFiles,    // Files the agent changed
        opts.DeletedFiles,     // Files the agent removed
        opts.MetadataDir,      // ".entire/metadata/<session-id>"
        opts.MetadataDirAbs,   // Absolute path to read metadata from
    )
    
    // 3. Create a commit pointing to this tree
    commitHash := s.createCommit(treeHash, parentHash, commitMsg, ...)
    
    // 4. Update the branch reference
    s.repo.Storer.SetReference(plumbing.NewHashReference(refName, commitHash))
    
    return WriteTemporaryResult{CommitHash: commitHash}, nil
}
```

The key insight: **git trees are content-addressed**. If you build a tree with the same contents, you get the same hash. This enables efficient deduplication—if nothing changed since the last checkpoint, Entire skips creating a new commit.

---

## Rewind: Restoring Previous States

All this machinery exists to enable one killer feature: **rewind**.

When you run `entire rewind`, Entire shows you available checkpoints:

```
$ entire rewind

Select a checkpoint to rewind to:
> a3b2c4d (2026-03-19 14:30) Implement login endpoint
         (2026-03-19 14:25) Add user model  
         (2026-03-19 14:20) Set up database connection
```

When you select one, Entire:

1. **Reads the checkpoint tree** from the shadow branch (uncommitted) or metadata branch (committed)
2. **Calculates the diff** between that tree and your current working directory
3. **Restores files** from the checkpoint
4. **Deletes orphaned files** (files created after the checkpoint)
5. **Truncates the transcript** so the agent's context matches the restored state

That last point is crucial. If you rewind to step 3, you don't want the agent's context to include steps 4 and 5—that would be confusing. Entire truncates the transcript to maintain consistency.

---

## Concurrent Sessions & Worktrees

Real development isn't always linear. You might:

- Have two terminal windows working on different features
- Use git worktrees for parallel development
- Start a new session before the old one finished

Entire handles all of these.

**Concurrent sessions** on the same commit share a shadow branch. Each session's checkpoints are tagged with their session ID, so rewind knows which checkpoint belongs to which session.

**Worktrees** get their own shadow branch namespace. The worktree hash in `entire/abc1234-def567` ensures sessions in different worktrees don't collide.

**Shadow branch migration**: If you do `git stash && git pull && git stash pop` (changing HEAD without committing), the shadow branch is automatically renamed to track the new base commit.

---

## The Value Proposition

So why does all this matter? What can you actually *do* with Entire?

### 1. Fearless Experimentation

With traditional version control, you commit when you're confident code works. With Entire, you can ask the AI to try risky refactors knowing you can rewind in seconds if it goes wrong.

This changes how you work. Instead of carefully constraining prompts to avoid mistakes, you can be ambitious: "Refactor this entire module to use the new API." If it works, great. If not, rewind and try a different approach.

### 2. Understanding Code History

Git blame tells you *who* changed a line and *when*. Entire tells you *why*:

```
$ entire explain abc1234

Commit: abc1234
Session: 2026-03-19-def456

Prompt: "The login endpoint is timing out under load. Can you optimize 
the database queries?"

Summary:
- Added database connection pooling
- Replaced N+1 query with JOIN
- Added index on users.email

Files: src/auth/login.go, migrations/add_email_index.sql
Tokens: 12,450 input, 3,200 output
```

This is invaluable for:

- **Code review**: Understanding the intent behind changes
- **Debugging**: Tracing a bug back to the prompt that introduced it
- **Onboarding**: Showing new team members how features were built

### 3. Collaboration & Handoffs

Sessions are stored in git, which means they're shareable:

```
$ entire resume feature-branch

Restoring session from feature-branch...
To continue: claude "Let's pick up where we left off"
```

Your teammate can literally resume your AI session, with full context of what was tried, what worked, and what didn't.

### 4. Audit & Compliance

For regulated industries, being able to trace code back to specific AI interactions is increasingly important. Entire provides that audit trail automatically—no extra process needed.

---

## Design Philosophy

A few principles guided Entire's design:

**Git-native**: Everything is stored in git. No external databases, no cloud dependencies. This makes it robust, portable, and familiar.

**Non-invasive**: Your commit history stays clean. Entire never creates commits on your working branch without your explicit action.

**Agent-agnostic**: The same architecture works with any AI coding assistant. Add a new agent by implementing an interface—the strategy and storage layers don't change.

**Opt-out friendly**: Don't want a commit linked to session data? Just delete the `Entire-Checkpoint` trailer before committing. Entire respects your choice.

**Fail-safe**: If anything goes wrong, Entire fails silently rather than blocking your work. Git hooks return success even on errors—your commits always go through.

---

## Conclusion

Entire exists because AI coding assistants are genuinely useful, but their ephemeral nature creates real friction. By treating session data as a first-class artifact—versioned, linked to commits, and rewindable—it removes that friction.

The technical approach is deliberately conservative: use git's existing primitives, don't modify the user's working tree, store everything as content-addressed blobs. This makes the system predictable and debuggable.

If you're using AI assistants for serious development work, give Entire a try:

```bash
brew install entireio/tap/entire
cd your-project && entire enable
```

Your future self—the one trying to understand why the code looks the way it does—will thank you.

---

*Entire is open source. Check out the [GitHub repository](https://github.com/entireio/cli) to learn more or contribute.*
