# Plan: Evolve Agent Interface for Contributor-Ready Plugin Surface

## Context

The CLI is now open source and receiving contributions. The current agent integration surface area requires contributors to:
1. Implement a 17-method `Agent` interface with unclear required-vs-optional boundaries
2. Write per-agent handler functions in the `cli` package (due to import cycles)
3. Register handlers via stringly-typed `(agentName, hookName)` pairs in `hook_registry.go`
4. Understand implicit mappings from agent-native hooks to lifecycle semantics
5. Duplicate framework logic (state transitions, strategy calls, file detection) in each handler

This plan redesigns the agent interface around explicit lifecycle events with a generic framework dispatcher, making it compile-time safe and contributor-friendly.

## Design Rationale

### The core insight: agents are data providers, not orchestrators

The current architecture grew organically as we added agents. Each agent's handler file (`hooks_claudecode_handlers.go`, `hooks_geminicli_handlers.go`) contains a mix of agent-specific parsing and framework orchestration logic — state transitions, strategy calls, file change detection, context generation. When a contributor wants to add a new agent, they have to understand and reproduce all of that framework machinery, which is both error-prone and creates maintenance burden as the framework evolves.

The key realization is that agents should be **passive data providers**. They know how to parse their own hook inputs and transcript formats, but they should never drive the checkpoint/step lifecycle themselves. The framework should call *into* the agent for data (transcript bytes, modified file lists, prompts, summaries) and handle all the orchestration — state machine transitions, strategy method calls, file change detection, metadata generation — in one place.

This inverts the current flow from "agent handler calls framework functions" to "framework dispatcher calls agent methods."

### Separating Events from Actions

We initially conflated what happens (events) with what the framework does about it (actions). For example, "StepSave" was initially proposed as an event type, but it's actually a framework *response* to a TurnEnd event. Getting this distinction right was critical:

- **Events** are inputs — things that happen in the outside world. They come from agents (SessionStart, TurnStart, TurnEnd, Compaction, SessionEnd, SubagentStart, SubagentEnd) or from git (GitCommit, PrePush). An agent's only job is to translate its native hooks into these normalized events.
- **Actions** are outputs — what the framework does in response. StepSave, Checkpoint, Condense, PhaseTransition, ResetTranscriptOffset. These are entirely internal to the framework and invisible to agent implementers.

This means a contributor adding a new agent only needs to think about: "which of my hooks correspond to which lifecycle events?" They never need to understand the action dispatch machinery.

### ParseHookEvent as the contribution surface

The most important method in the new interface is `ParseHookEvent(hookName string, stdin io.Reader) (*Event, error)`. This is where 90% of agent-specific work lives. The agent reads its native hook payload from stdin, extracts the session ID, transcript path, and any other relevant data, and returns a normalized `Event` — or `nil` if the hook has no lifecycle significance.

This design handles the "pass-through hook" problem elegantly. Agents like Gemini have hooks (before-tool, after-tool, before-model, etc.) that we acknowledge but don't act on. Rather than requiring dummy handler registrations for each, the agent simply returns `nil` from `ParseHookEvent` and the dispatcher does nothing.

### Why transcript operations are core, not optional

We initially considered making transcript reading/chunking/analysis optional interfaces, since not all hypothetical future agents might have accessible transcripts. But transcript data is fundamental to the product — it's what gets stored in checkpoints, what drives the "files modified" detection, and what enables session logs and summaries. An agent that can't provide transcript access can't meaningfully participate in the checkpoint lifecycle. Making these methods core forces contributors to think about transcript access upfront rather than discovering the gap later.

### Step vs Checkpoint terminology

The codebase had been using "checkpoint" loosely for two distinct concepts: the temporary per-turn saves on shadow branches and the permanent persisted saves on `entire/checkpoints/v1`. We established precise terminology:
- A **Step** is saved to the shadow branch — it's temporary, per-turn, and gets cleaned up on condensation
- A **Checkpoint** is persisted to `entire/checkpoints/v1` — it's permanent and survives branch operations

This distinction matters because both happen on TurnEnd (the framework saves a step *and* writes a checkpoint), but they have different lifecycles and storage mechanisms. The strategy interface rename (`SaveChanges` → `SaveStep`, `SaveTaskCheckpoint` → `SaveTaskStep`) makes the code self-documenting.

### Compaction and session handoff

Two lifecycle events we identified during design that weren't previously modeled:

**Compaction** occurs when an agent compresses its context window (e.g., Gemini's pre-compress hook). This triggers the same save logic as TurnEnd but also resets the transcript offset, since the transcript may be truncated or reorganized. We also added defensive detection: if `GetTranscriptPosition()` returns a value smaller than the stored offset, the framework treats it as an implicit compaction even without an explicit event.

**Session handoff** occurs when Claude starts a new session ID (observed after exiting plan mode). Rather than building complex session-linking machinery now, we added a `PreviousSessionID` field on `Event` for future use. The current shadow branch architecture already handles this gracefully — multiple session IDs naturally share the same shadow branch and condense together.

### Terminology (established in this plan)
- **Step** = saved to shadow branch (`entire/<hash>-<worktreeHash>`) — temporary, per-turn
- **Checkpoint** = persisted to `entire/checkpoints/v1` — permanent, on turn end and git commit

### Prerequisites
- **ENT-250** (ActionHandler interface for compile-time exhaustive action dispatch) should be completed first. It's self-contained and establishes the pattern we'll extend here.

---

## Phase 1: Define Event Types and New Agent Interface

### 1a. Event types (`cmd/entire/cli/agent/event.go` — new file)

```go
type EventType int
const (
    SessionStart  EventType = iota  // Agent session begins
    TurnStart                       // User submitted a prompt
    TurnEnd                         // Agent finished responding
    Compaction                      // Agent about to compress context
    SessionEnd                      // Session terminated
    SubagentStart                   // Subagent spawned
    SubagentEnd                     // Subagent completed
)

type Event struct {
    Type              EventType
    SessionID         string
    PreviousSessionID string     // Non-empty = continuation/handoff
    SessionRef        string     // Path to transcript file
    Prompt            string     // For TurnStart: user's prompt
    Timestamp         time.Time

    // Subagent-specific
    ToolUseID         string
    SubagentID        string
    ToolInput         json.RawMessage  // Raw tool input (for subagent type/description extraction)

    // Response — message to show the user via the agent
    ResponseMessage   string
}
```

### 1b. New `Agent` interface (`cmd/entire/cli/agent/agent.go` — modify)

Replace the current 17-method interface with a 14-method interface in three clear groups:

```go
type Agent interface {
    // --- Identity (5 methods, all trivial one-liners) ---
    Name() AgentName
    Type() AgentType
    Description() string
    DetectPresence() (bool, error)
    ProtectedDirs() []string

    // --- Event Mapping (2 methods, the core contribution surface) ---
    HookNames() []string
    ParseHookEvent(hookName string, stdin io.Reader) (*Event, error)

    // --- Transcript Operations (7 methods, called by framework during actions) ---
    ReadTranscript(sessionRef string) ([]byte, error)
    GetTranscriptPosition(sessionRef string) (int, error)
    ExtractModifiedFiles(sessionRef string, fromOffset int) (files []string, newOffset int, err error)
    ExtractPrompts(sessionRef string, fromOffset int) ([]string, error)
    ExtractSummary(sessionRef string) (string, error)
    ChunkTranscript(content []byte, maxSize int) ([][]byte, error)
    ReassembleTranscript(chunks [][]byte) ([]byte, error)
}
```

Methods removed from core interface (moved to optional interfaces or framework):
- `GetHookConfigPath()` → move to `HookInstaller`
- `SupportsHooks()` → inferred from `HookInstaller` implementation
- `ParseHookInput()` → replaced by `ParseHookEvent()`
- `GetSessionID()` → extracted inside `ParseHookEvent()`
- `TransformSessionID()` / `ExtractAgentSessionID()` → internal to agent, used inside `ParseHookEvent()`
- `GetSessionDir()` / `ResolveSessionFile()` / `ReadSession()` / `WriteSession()` / `FormatResumeCommand()` → move to `SessionResumer`

### 1c. Optional interfaces (keep existing, reorganize)

```go
// For agents where we can auto-install hooks
type HookInstaller interface {
    Agent
    GetHookConfigPath() string
    InstallHooks(localDev, force bool) (int, error)
    UninstallHooks() error
    AreHooksInstalled() bool
    GetSupportedHooks() []HookType  // Keep for setup validation
}

// For agents that support session resumption (rewind --resume)
type SessionResumer interface {
    Agent
    GetSessionDir(repoPath string) (string, error)
    ResolveSessionFile(sessionDir, agentSessionID string) string
    ReadSession(sessionRef string) (*AgentSession, error)
    WriteSession(session *AgentSession) error
    FormatResumeCommand(sessionID string) string
}

// For agents without hooks — file-watching based
type FileWatcher interface {
    Agent
    WatchPaths() ([]string, error)
    OnFileChange(path string) (*Event, error)
}
```

Remove `TranscriptAnalyzer` and `TranscriptChunker` as separate optional interfaces — their methods are now in the core `Agent` interface.

### 1d. Agent-specific concerns

**Transcript flush (Claude-specific):** Add an optional interface:
```go
// TranscriptPreparer is called before ReadTranscript to handle agent-specific
// flush/sync requirements (e.g., Claude Code's async transcript writing).
type TranscriptPreparer interface {
    Agent
    PrepareTranscript(sessionRef string) error
}
```
The framework calls this before `ReadTranscript()` if implemented. Claude implements it with the sentinel polling logic from `waitForTranscriptFlush()`.

**Token usage calculation:** Add an optional interface:
```go
// TokenCalculator provides token usage for a session.
type TokenCalculator interface {
    Agent
    CalculateTokenUsage(sessionRef string, fromOffset int) (*TokenUsage, error)
}
```
Both agents implement this with their format-specific logic. The framework calls it during StepSave/Checkpoint if available.

### Files to modify/create:
- `cmd/entire/cli/agent/event.go` — **new**: Event types
- `cmd/entire/cli/agent/agent.go` — **modify**: new interface, keep old as `LegacyAgent` temporarily
- `cmd/entire/cli/agent/claudecode/claude.go` — **modify**: implement new interface
- `cmd/entire/cli/agent/geminicli/gemini.go` — **modify**: implement new interface

---

## Phase 2: Build Generic Lifecycle Dispatcher

### 2a. Pre-prompt state unification

Currently there are two separate functions: `CapturePrePromptState()` (Claude, JSONL line counting) and `CaptureGeminiPrePromptState()` (Gemini, JSON message counting). Both capture untracked files + transcript position.

Unify into a single function that uses the Agent interface:
```go
func CapturePrePromptState(ag agent.Agent, sessionID, sessionRef string) error {
    position, err := ag.GetTranscriptPosition(sessionRef)
    // ... store position + untracked files
}
```

The `PrePromptState` struct already has both `StepTranscriptStart` (used by Claude) and `StartMessageIndex` (used by Gemini) — unify these into a single `TranscriptOffset int` field.

### 2b. Generic dispatcher (`cmd/entire/cli/lifecycle.go` — new file)

Single entry point replacing all per-agent handler logic:

```go
func DispatchLifecycleEvent(ag agent.Agent, event *agent.Event) error {
    switch event.Type {
    case agent.SessionStart:
        return handleSessionStart(ag, event)
    case agent.TurnStart:
        return handleTurnStart(ag, event)
    case agent.TurnEnd:
        return handleTurnEnd(ag, event)
    case agent.Compaction:
        return handleCompaction(ag, event)
    case agent.SessionEnd:
        return handleSessionEnd(ag, event)
    case agent.SubagentStart:
        return handleSubagentStart(ag, event)
    case agent.SubagentEnd:
        return handleSubagentEnd(ag, event)
    }
}
```

Each handler function encapsulates the framework logic that's currently duplicated across `commitWithMetadata()` and `handleGeminiAfterAgent()`:

**`handleTurnEnd(ag, event)`** — the main flow (framework orchestrates, agent provides data):
1. Validate transcript exists
2. If agent implements `TranscriptPreparer`, call `ag.PrepareTranscript()` *(Claude: flush wait)*
3. Create session metadata directory
4. Read transcript via `ag.ReadTranscript(event.SessionRef)` *(agent provides bytes)*
5. Load pre-prompt state (captured on TurnStart)
6. Extract modified files via `ag.ExtractModifiedFiles(sessionRef, offset)` *(agent parses its format)*
7. Extract prompts via `ag.ExtractPrompts(sessionRef, offset)` *(agent parses its format)*
8. Extract summary via `ag.ExtractSummary(sessionRef)` *(agent parses its format)*
9. Detect file changes via `DetectFileChanges()` *(framework, git status)*
10. Filter and normalize paths *(framework)*
11. If no changes: transition phase, clean up, return
12. Create context file *(framework, agent-agnostic)*
13. If agent implements `TokenCalculator`, call `ag.CalculateTokenUsage()` *(optional)*
14. Build `strategy.StepContext`, call `strat.SaveStep()` — **StepSave action**
15. Persist checkpoint to `entire/checkpoints/v1` — **Checkpoint action**
16. Transition session phase (TurnEnd) *(framework, state machine)*
17. Clean up pre-prompt state *(framework)*

**`handleTurnStart(ag, event)`:**
1. Capture pre-prompt state using `ag.GetTranscriptPosition()`
2. If strategy implements `SessionInitializer`, call `InitializeSession()`

**`handleSessionStart(ag, event)`:**
1. Check concurrent sessions
2. Output hook response message
3. Fire `EventSessionStart` on state machine

**`handleSessionEnd(ag, event)`:**
1. Mark session ended via `markSessionEnded()`

**`handleCompaction(ag, event)`:**
1. Same as `handleTurnEnd()` but also resets transcript offset in session state

**`handleSubagentStart(ag, event)`:**
1. Capture pre-task state

**`handleSubagentEnd(ag, event)`:**
1. Extract modified files from subagent transcript
2. Detect file changes
3. Build `TaskStepContext`, call `strat.SaveTaskStep()`
4. Clean up pre-task state

### 2c. Hook command integration

Modify `newAgentHookVerbCmdWithLogging()` in `hook_registry.go` to use the generic dispatcher:

```go
RunE: func(_ *cobra.Command, _ []string) error {
    // ... existing setup (repo check, logging, agent resolution) ...

    event, err := ag.ParseHookEvent(hookName, os.Stdin)
    if err != nil {
        return err
    }
    if event == nil {
        return nil  // Agent says: no lifecycle action needed
    }
    return DispatchLifecycleEvent(ag, event)
}
```

This replaces the entire `hookRegistry` map + `GetHookHandler()` lookup + per-agent `init()` registration.

### Files to modify/create:
- `cmd/entire/cli/lifecycle.go` — **new**: generic dispatcher
- `cmd/entire/cli/hook_registry.go` — **modify**: simplify to use dispatcher
- `cmd/entire/cli/state.go` — **modify**: unify `CapturePrePromptState` / `CaptureGeminiPrePromptState`

---

## Phase 3: Migrate Agents

### 3a. Migrate Claude Code

Implement `ParseHookEvent()` on `ClaudeCodeAgent`:

```go
func (a *ClaudeCodeAgent) ParseHookEvent(hookName string, stdin io.Reader) (*Event, error) {
    switch hookName {
    case HookNameSessionStart:
        return parseClaudeSessionStart(stdin)
    case HookNameUserPromptSubmit:
        return parseClaudeTurnStart(stdin)
    case HookNameStop:
        return parseClaudeTurnEnd(stdin)
    case HookNameSessionEnd:
        return parseClaudeSessionEnd(stdin)
    case HookNamePreTask:
        return parseClaudeSubagentStart(stdin)
    case HookNamePostTask:
        return parseClaudeSubagentEnd(stdin)
    case HookNamePostTodo:
        return nil, nil  // Agent-specific: handled outside generic dispatcher
    }
    return nil, nil
}
```

Move transcript methods to satisfy new interface:
- `GetTranscriptPosition()` — already exists on `ClaudeCodeAgent`
- `ExtractModifiedFiles()` — already exists, adjust signature
- `ReadTranscript()` — wrap existing file read
- `ExtractPrompts()` / `ExtractSummary()` — wrap existing transcript parsing functions
- `ChunkTranscript()` / `ReassembleTranscript()` — already exist

Implement `TranscriptPreparer` for the flush-wait logic.
Implement `TokenCalculator` wrapping `CalculateTotalTokenUsage()`.

### 3b. Migrate Gemini CLI

Implement `ParseHookEvent()` on `GeminiCLIAgent`:

```go
func (a *GeminiCLIAgent) ParseHookEvent(hookName string, stdin io.Reader) (*Event, error) {
    switch hookName {
    case HookNameSessionStart:
        return parseGeminiSessionStart(stdin)
    case HookNameBeforeAgent:
        return parseGeminiTurnStart(stdin)
    case HookNameAfterAgent:
        return parseGeminiTurnEnd(stdin)
    case HookNameSessionEnd:
        return parseGeminiSessionEnd(stdin)
    case HookNamePreCompress:
        return parseGeminiCompaction(stdin)
    case HookNameBeforeTool, HookNameAfterTool, HookNameBeforeModel,
         HookNameAfterModel, HookNameBeforeToolSelection, HookNameNotification:
        return nil, nil  // Acknowledged, no lifecycle action
    }
    return nil, nil
}
```

### Files to modify:
- `cmd/entire/cli/agent/claudecode/claude.go` — **modify**: add `ParseHookEvent`, `ReadTranscript`, `PrepareTranscript`, etc.
- `cmd/entire/cli/agent/geminicli/gemini.go` — **modify**: add `ParseHookEvent`, `ReadTranscript`, etc.

---

## Phase 4: Clean Up and Rename

### 4a. Remove old code
- **Delete** `cmd/entire/cli/hooks_claudecode_handlers.go`
- **Delete** `cmd/entire/cli/hooks_geminicli_handlers.go`
- **Remove** `hookRegistry` map and string-based registration from `hook_registry.go`
- **Remove** `HookHandlerFunc` type
- **Remove** the `init()` that registers 20+ handlers
- **Remove** `CaptureGeminiPrePromptState()` (merged into unified version)
- **Remove** old `Agent` interface methods that moved to optional interfaces

### 4b. Update state machine events

Add `Compaction` to session phase events (`cmd/entire/cli/session/phase.go`):
- New event: `EventCompaction`
- Transition: from any active phase, save + reset offset, no phase change
- Framework defensively detects compaction: if `GetTranscriptPosition()` returns value < stored offset, treat as compaction

### 4c. Step/Checkpoint terminology rename

Rename strategy interface methods to align with Step/Checkpoint terminology:

| Current | New | Notes |
|---------|-----|-------|
| `SaveChanges(SaveContext)` | `SaveStep(StepContext)` | Core strategy method |
| `SaveTaskCheckpoint(TaskCheckpointContext)` | `SaveTaskStep(TaskStepContext)` | Core strategy method |
| `SaveContext` struct | `StepContext` | Rename type |
| `TaskCheckpointContext` struct | `TaskStepContext` | Rename type |
| `GetRewindPoints()` | Keep as-is | Rewind works from both steps and checkpoints |
| `checkpoint.WriteTemporary()` | Keep as-is | Already semantically "step write" |
| `checkpoint.WriteCommitted()` | Keep as-is | Already semantically "checkpoint write" |

This rename propagates through:
- `cmd/entire/cli/strategy/strategy.go` — interface definition, `StepContext`/`TaskStepContext` structs
- `cmd/entire/cli/strategy/manual_commit.go` — `SaveStep()` implementation
- `cmd/entire/cli/strategy/auto_commit.go` — `SaveStep()` implementation
- `cmd/entire/cli/strategy/manual_commit_git.go` — uses `SaveContext` → `StepContext`
- `cmd/entire/cli/lifecycle.go` — builds `StepContext` instead of `SaveContext`
- All test files referencing these types/methods
- CLAUDE.md documentation

### 4d. PostTodo pass-through

Claude Code's PostTodo hook stays as a **direct handler** outside the generic dispatcher. In `hook_registry.go`, register it as a special case:

```go
// PostTodo is Claude-specific: incremental subagent checkpoints.
// Not generalized to the lifecycle dispatcher until other agents need it.
```

The dispatcher handles the `nil` return from `ParseHookEvent("post-todo", ...)` by doing nothing. The actual PostTodo logic remains in a Claude-specific handler function (moved from `hooks_claudecode_handlers.go` to a smaller `hooks_claudecode_posttodo.go` file).

### Files to delete/modify:
- `cmd/entire/cli/hooks_claudecode_handlers.go` — **delete** (most logic moves to `lifecycle.go`)
- `cmd/entire/cli/hooks_claudecode_posttodo.go` — **new**: PostTodo handler (extracted from above)
- `cmd/entire/cli/hooks_geminicli_handlers.go` — **delete**
- `cmd/entire/cli/hook_registry.go` — **modify**: simplify significantly, keep PostTodo registration
- `cmd/entire/cli/session/phase.go` — **modify**: add EventCompaction
- `cmd/entire/cli/state.go` — **modify**: remove Gemini-specific state capture
- `cmd/entire/cli/strategy/strategy.go` — **modify**: rename SaveChanges→SaveStep, SaveContext→StepContext, etc.
- `cmd/entire/cli/strategy/manual_commit.go` — **modify**: rename method
- `cmd/entire/cli/strategy/auto_commit.go` — **modify**: rename method
- `cmd/entire/cli/strategy/manual_commit_git.go` — **modify**: update type references
- All `*_test.go` files referencing renamed types

---

## Phase 5: Documentation and Contributor Guide

- Update CLAUDE.md with new agent interface documentation
- Add `cmd/entire/cli/agent/CONTRIBUTING.md` with step-by-step guide for adding a new agent
- Add compile-time interface assertions in each agent package

---

## Migration Strategy

Phases 1-2 are additive — the new interface and dispatcher can coexist with the old handler system. During Phase 3, we migrate one agent at a time:

1. Add `ParseHookEvent()` to Claude Code agent, update dispatcher to use it
2. Verify all Claude Code tests pass
3. Delete `hooks_claudecode_handlers.go`
4. Repeat for Gemini CLI
5. Delete old registry code

This ensures we never have a broken intermediate state.

---

## Verification

After each phase:
```bash
mise run fmt && mise run lint && mise run test:ci
```

End-to-end validation:
1. `entire enable --agent claude-code` → hooks install correctly
2. Start a Claude Code session → SessionStart event dispatched, banner shown
3. Submit a prompt → TurnStart captures pre-prompt state
4. Agent completes → TurnEnd saves step + checkpoint
5. `git commit` → condensation works
6. `entire rewind` → rewind points listed correctly
7. Repeat steps 1-6 for Gemini CLI

Integration tests in `cmd/entire/cli/integration_test/` cover the strategy-level flows. Add new integration tests for the lifecycle dispatcher.

---

## Addendum: Validation Against Third-Party Agent Contributions

We reviewed open PRs for four third-party agents (OpenCode, Codex CLI, Pi, Cursor) and cross-referenced each against official documentation and source code. This section documents what we found and what adjustments the plan needs.

### Validation confirms the core design

The strongest signal from the PR review is the **massive duplication of framework logic**. Every contributed agent re-implements the same ~300-line handler pattern: pre-prompt state capture, transcript copying, file change detection, path normalization, context file creation, `strategy.SaveChanges()`, session state transitions. PR #257 (Codex + OpenCode) recognized this independently and extracted a shared `commitAgentSession()` function — which is essentially a partial version of Phase 2's `DispatchLifecycleEvent`. The plan's inversion ("framework calls agent for data") would eliminate ~1500 lines of duplicated handler code across these agents.

The 7 event types (SessionStart, TurnStart, TurnEnd, Compaction, SessionEnd, SubagentStart, SubagentEnd) are sufficient for all four agents. No agent needs events outside this set. Agents with fewer hooks (e.g., Codex with only TurnEnd today) naturally get degraded functionality — no pre-prompt state capture means git-status-only file detection — which is an acceptable trade-off.

### Adjustment 1: Framework-level hook input normalization

**Problem**: Codex CLI delivers its `notify` payload as the **last argv element**, not via stdin. The Rust source (`codex-rs/hooks/src/user_notification.rs`) explicitly sets `stdin(Stdio::null())` and appends JSON via `command.arg(notify_payload)`. PR #337 assumes stdin delivery and would silently fail. PR #257 discovered this and added `GetCurrentHookArgs()` infrastructure.

**Note**: The Codex team has confirmed that proper hook support (with stdin-based payload delivery and richer lifecycle events beyond just `agent-turn-complete`) is in active development. Once available, this normalization becomes unnecessary for Codex. However, other future agents may have similar delivery mechanisms.

**Adjustment**: Add input normalization in `newAgentHookVerbCmdWithLogging()` before calling `ParseHookEvent`. If stdin is empty and positional args contain a JSON payload, wrap the last arg into a `bytes.Reader`. This keeps the `Agent` interface clean (`ParseHookEvent` always receives an `io.Reader`) while handling delivery variation at the framework layer:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    // ... existing setup ...

    input := normalizeHookInput(os.Stdin, args)
    event, err := ag.ParseHookEvent(hookName, input)
    // ...
}

// normalizeHookInput handles agents that deliver payloads via argv instead of stdin.
func normalizeHookInput(stdin io.Reader, args []string) io.Reader {
    // Check if stdin has data
    if f, ok := stdin.(*os.File); ok {
        if stat, err := f.Stat(); err == nil && stat.Size() > 0 {
            return stdin
        }
    }
    // Fall back to last positional arg if it looks like JSON
    if len(args) > 0 {
        last := args[len(args)-1]
        if len(last) > 0 && (last[0] == '{' || last[0] == '[') {
            return strings.NewReader(last)
        }
    }
    return stdin
}
```

### Adjustment 2: Transcript analysis methods — core vs optional

**Problem**: The plan states "An agent that can't provide transcript access can't meaningfully participate in the checkpoint lifecycle" and makes all 7 transcript methods core. Evidence from PRs and official docs shows this is *almost* true but not quite:

- **Cursor** passes `transcript_path` in every hook payload (confirmed in [official docs](https://cursor.com/docs/agent/hooks)), but the transcript file format is not publicly documented. Implementers would need to stub the analysis methods until the format is known.
- **OpenCode** (PR #315, #257) treats transcripts as opaque and relies entirely on git status — yet still provides useful checkpoint functionality.
- **Codex** has full structured transcripts (JSONL RolloutItems) but is transitioning its hook system, meaning the transcript path delivery mechanism may change.

All agents can provide `ReadTranscript` (raw bytes), `ChunkTranscript`, and `ReassembleTranscript` (storage operations). The divergence is in the 4 analysis methods that require format-specific parsing.

**Adjustment**: Split transcript operations into core (3 methods) and optional (4 methods):

```go
type Agent interface {
    // ... Identity (5) + Event Mapping (2) unchanged ...

    // --- Transcript Storage (3 methods, core) ---
    ReadTranscript(sessionRef string) ([]byte, error)
    ChunkTranscript(content []byte, maxSize int) ([][]byte, error)
    ReassembleTranscript(chunks [][]byte) ([]byte, error)
}

// TranscriptAnalyzer provides format-specific transcript parsing.
// Agents that implement this get richer checkpoints (transcript-derived file lists,
// prompts, summaries). Agents that don't still participate in the checkpoint lifecycle
// via git-status-based file detection and raw transcript storage.
type TranscriptAnalyzer interface {
    Agent
    GetTranscriptPosition(sessionRef string) (int, error)
    ExtractModifiedFiles(sessionRef string, fromOffset int) (files []string, newOffset int, err error)
    ExtractPrompts(sessionRef string, fromOffset int) ([]string, error)
    ExtractSummary(sessionRef string) (string, error)
}
```

The dispatcher gracefully degrades:
- With `TranscriptAnalyzer`: incremental transcript parsing, transcript-derived file lists, prompt/summary extraction
- Without: git-status-only file detection, no prompt/summary metadata, transcript stored as opaque blob

This reduces the core `Agent` interface from 14 to 10 methods while keeping the same functionality for agents that implement the full set.

### Adjustment 3: Event.Metadata for agent-specific state

**Problem**: Pi has tree-structured conversations with an `activeLeafId` that determines which branch is "active." This is runtime state (not persisted in the JSONL, confirmed via [Pi source](https://github.com/badlogic/pi-mono)). The Pi agent needs to pass this through the framework so it's available on subsequent calls. Cursor passes `is_background_agent` and `composer_mode` that could be useful for checkpoint metadata.

**Adjustment**: Add a generic metadata field to `Event`:

```go
type Event struct {
    // ... existing fields ...

    // Agent-specific metadata the framework stores and passes back on subsequent events.
    // Examples: Pi's activeLeafId, Cursor's is_background_agent.
    Metadata map[string]string
}
```

The framework stores this in session state and makes it available to subsequent `ParseHookEvent` calls (or transcript method calls) via context. This avoids widening the core interface for agent-specific concerns.

### Agent-specific notes

#### OpenCode

Verified against [official docs](https://opencode.ai/docs/plugins/) and [source](https://github.com/anomalyco/opencode).

- **Plugin system**: `.opencode/plugins/entire.ts` with named export receiving `{ project, client, $, directory, worktree }` — well-documented, stable API
- **Events**: `session.created`, `session.idle` (deprecated, use `session.status`), `tool.execute.before`, `tool.execute.after` — all confirmed
- **Transcript access**: Via `client.session.messages()` SDK call, exported to temp JSONL — works but adds indirection (OpenCode stores sessions in SQLite, not JSONL files)
- **Event mapping**: SessionStart (`session.created`), TurnEnd (`session.status` type=idle), SubagentStart/End (`tool.execute.before/after`). No TurnStart event exists — pre-prompt state capture not possible, git-status-only file detection
- **Resume**: `opencode --session <id>` (not `--resume`)
- **HookInstaller**: generates plugin file — fits cleanly

#### Codex CLI

Verified against [source](https://github.com/openai/codex) and [docs](https://developers.openai.com/codex/config-reference/).

- **Current hook system**: Single `notify` event (`agent-turn-complete`) delivered via argv with `stdin(Stdio::null())`. Payload is legacy JSON with kebab-case fields (`thread-id`, `turn-id`, `input-messages`, `last-assistant-message`)
- **Proper hook support in development**: The Codex team has confirmed that a richer hook system with stdin-based payload delivery and more lifecycle events is actively being built. Once shipped, Codex would support TurnStart, TurnEnd, SessionStart/End, etc. — mapping cleanly to our event model without the argv normalization workaround
- **Transcript**: JSONL at `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` with typed `RolloutItem` discriminants (`session_meta`, `response_item`, `event_msg`, `compacted`, `turn_context`). Structured `file_change` events exist in the exec stream, and `patch_apply_begin`/`patch_apply_end` in internal EventMsg — no need for heuristic shell command parsing
- **Config**: Both `~/.codex/config.toml` (global) and `.codex/config.toml` (project-local, requires trust) are supported
- **Resume**: `codex resume <id>` (subcommand, not `--resume` flag)
- **HookInstaller**: Modifies TOML `notify` array — fits cleanly

#### Pi

Verified against [source](https://github.com/badlogic/pi-mono) and [extension docs](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md).

- **Extension system**: `.pi/extensions/entire/index.ts` — auto-discovered, hot-reloaded, TypeScript. 26 total lifecycle events available.
- **Events**: All 9 events used in PR #251 are confirmed real and documented. Richest event coverage of any contributed agent — maps to 6/7 of our event types (everything except Compaction, though `session_compact` exists and could be added)
- **Tree-structured transcripts**: JSONL with `id`/`parentId` pointers forming a tree. `activeLeafId` is runtime state from `SessionManager.getLeafId()`, not persisted. Agent must reconstruct active branch via tree traversal — handled internally in `ExtractModifiedFiles`/`ExtractPrompts`, transparent to framework
- **Resume**: `pi -c` (continue last), `pi -r` (interactive picker), `pi --session <path>` (specific session), or `/resume` in-TUI
- **HookInstaller**: generates extension scaffold — fits cleanly
- **Best-fit agent**: Pi maps most naturally to the TASK.md model. Full lifecycle events, accessible JSONL transcripts, clean extension API.

#### Cursor

Verified against [official hooks docs](https://cursor.com/docs/agent/hooks) and [third-party hooks docs](https://cursor.com/docs/agent/third-party-hooks).

- **Hooks API**: `.cursor/hooks.json` with 20+ event types — richest hook surface of any agent. Includes `sessionStart`, `sessionEnd`, `beforeSubmitPrompt`, `stop`, `preToolUse`, `postToolUse`, `subagentStart`, `subagentStop`, `preCompact`, `afterFileEdit`, and more. Maps to all 7 of our event types including Compaction and Subagent events.
- **Payload delivery**: JSON on stdin, with universal fields on every hook (`conversation_id`, `generation_id`, `model`, `transcript_path`, `workspace_roots`, `user_email`). The `transcript_path` field is significant — Cursor provides the transcript file path in every hook payload.
- **Transcript**: Format of the file at `transcript_path` is not publicly documented. Primary storage is SQLite (`state.vscdb`). The hook-provided transcript file is likely JSONL but this needs verification.
- **Native Claude Code hook loading**: Cursor auto-discovers hooks from `.claude/settings.json` and translates event names (PascalCase → camelCase) and tool names (`Bash` → `Shell`, `Edit` → `Write`). This means `entire enable --agent claude-code` may already work in Cursor without a separate agent — **needs testing**. If the stdin payload format matches Claude's, no Cursor-specific agent is needed. If Cursor sends its own payload format (with `conversation_id`, `cursor_version`, etc.), a Cursor agent or detection layer is required.
- **CLI**: `cursor-agent --resume <id>` (not `cursor --resume`)
- **HookInstaller**: writes `.cursor/hooks.json` — fits cleanly
- **Recommendation**: Before building a Cursor agent, test whether `entire enable --agent claude-code` works in Cursor via the native hook translation. If it does, document it. If not, build a Cursor agent that leverages the full 20+ event hook surface.

---

## Caveats

### Cursor payload format ambiguity

Cursor's third-party hooks documentation says it loads Claude Code hooks from `.claude/settings.json`, but it's unclear whether the **stdin payload** sent to those hooks matches Claude Code's format or Cursor's native format (which includes extra fields like `conversation_id`, `cursor_version`, `workspace_roots`). If Cursor sends its own format, the Claude Code agent's `ParseHookEvent` would fail to unmarshal the payload. This needs empirical testing before deciding whether a separate Cursor agent is necessary.

### Codex hook system in transition

The current Codex `notify` system is minimal (single event, argv delivery, fire-and-forget). A proper hook system with stdin payloads and richer lifecycle events is in active development by the Codex team. Our implementation should target the new hook system rather than building elaborate workarounds for the legacy `notify` mechanism. The argv normalization in Adjustment 1 is a stopgap — once the new hooks ship, Codex would use standard stdin delivery like every other agent.

### OpenCode `session.idle` deprecation

OpenCode's `session.idle` event is marked as deprecated in the source code (`packages/opencode/src/session/status.ts`). It still fires alongside `session.status`, but new integrations should use `session.status` and check `event.properties.status.type === "idle"`. The plugin should be updated before merging any OpenCode PR.

### OpenCode transcript indirection

OpenCode stores sessions in SQLite, not as filesystem JSONL files. The plugin must export transcripts via the SDK (`client.session.messages()`) to a temp JSONL file. This adds a layer of indirection and means the transcript is a point-in-time snapshot, not a live file. `GetTranscriptPosition` based on JSONL line offsets works, but the offset resets if the plugin re-exports the full transcript (which it does on every hook invocation in PR #341). Incremental offset tracking requires the plugin to append to the existing JSONL file rather than overwriting it.

### Pi tree-structured transcripts

Pi's `id`/`parentId` tree model means that naive line-based transcript parsing returns entries from **all branches**, not just the active one. The agent must reconstruct the active branch path by walking `parentId` pointers from the leaf. The `activeLeafId` is runtime state from `SessionManager.getLeafId()` and must be passed from the extension to the CLI (PR #251 does this via the hook payload's `active_leaf_id` field). If the leaf ID is lost (e.g., between sessions), the agent falls back to the last entry in the JSONL, which may be on a stale branch.

### `TranscriptAnalyzer` degradation path

When an agent doesn't implement `TranscriptAnalyzer`, the dispatcher must handle:
- **File detection**: git-status-only (compare worktree against pre-prompt untracked baseline). This works well for new/deleted files but can't distinguish "files the agent modified" from "files modified by other processes."
- **Prompts**: Empty — no prompt metadata in checkpoints. The `prompt.txt` file would be omitted or contain only the TurnStart event's `Prompt` field if available.
- **Summary**: Empty — no summary in checkpoint metadata.
- **Transcript position**: No incremental parsing. The full transcript is stored on every step. This increases storage but is functionally correct.
- **Pre-prompt state**: `CapturePrePromptState` can still capture untracked files even without `GetTranscriptPosition`. The `TranscriptOffset` field would be 0, meaning the next `ExtractModifiedFiles` call (if the agent later implements `TranscriptAnalyzer`) processes the full transcript.
