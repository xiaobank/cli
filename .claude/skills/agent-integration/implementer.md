# Implement Command

Build the agent Go package using E2E-driven development. E2E tests are the primary spec — unit tests are written *after* each E2E test passes to lock in behavior.

## Prerequisites

- The research command's one-pager at `cmd/entire/cli/agent/$AGENT_PACKAGE/AGENT.md`
- The E2E test runner already added (from `write-tests` command)
- If no one-pager exists, read the agent's docs and ask the user about hook events, transcript format, and config

## Procedure

### Step 1: Read Implementation Guide

Read these files thoroughly before writing any code:

1. `docs/architecture/agent-guide.md` — Authoritative implementation guide with code templates. Read thoroughly.
2. `docs/architecture/agent-integration-checklist.md` — Validation criteria for completeness.
3. `cmd/entire/cli/agent/agent.go` — Read to find the exact `Agent` interface and all optional interfaces.
4. `cmd/entire/cli/agent/event.go` — Read to find `EventType` constants and shared parsing helpers.

### Step 2: Read Reference Implementation

Read `cmd/entire/cli/agent/$AGENT_PACKAGE/AGENT.md` (the one-pager from the research phase) for the agent's hook mechanism, transcript format, and config structure.

Run `Glob("cmd/entire/cli/agent/*/")` to find all existing agent packages. Check the one-pager's "Hook Mechanism" and "Gaps & Limitations" sections to pick the best reference — choose an agent with a similar hook mechanism to your target. Read all `*.go` files (skip `*_test.go` on first pass) in the chosen reference.

### Step 3: Create Bare-Minimum Compiling Package

Create the agent package directory and stub out every required interface method so the project compiles.

```
cmd/entire/cli/agent/$AGENT_PACKAGE/
```

**What to create:**

1. **`${AGENT_PACKAGE}.go`** — Struct definition, `init()` with `agent.Register(agent.AgentName("$AGENT_KEY"), New)`, and stub implementations for every method in the `Agent` interface — refer to `agent.go` from Step 1. Include `HookSupport` methods in `lifecycle.go` and `hooks.go`.
2. **`types.go`** — Hook input struct(s) with JSON tags matching the one-pager's "Hook input (stdin JSON)" section.
3. **`lifecycle.go`** — Stub `ParseHookEvent()` that returns `nil, nil` for all inputs. Use the one-pager's "Hook names" table for the native hook name → Entire EventType mapping.
4. **`hooks.go`** — Stub `InstallHooks()`, `UninstallHooks()`, `AreHooksInstalled()` that return nil/false. Use the one-pager's "Config file" and "Hook registration" sections for the config path and format.
5. **`transcript.go`** — Stub `TranscriptAnalyzer` methods if the one-pager's "Transcript" section indicates the agent supports transcript analysis. Use the one-pager for transcript location and format.

**Wire up blank imports:**

- Ensure the blank import `_ "github.com/entireio/cli/cmd/entire/cli/agent/$AGENT_PACKAGE"` exists in `cmd/entire/cli/hooks_cmd.go`

**Verify compilation:**

```bash
mise run fmt && mise run lint && mise run test
```

Everything must pass before proceeding. Fix any issues.

**Standing instruction for Steps 4-12:** If you need agent-specific information (hook format, transcript location, config structure), check `AGENT.md` first. If `AGENT.md` doesn't cover what you need, you may search external docs — but always update `AGENT.md` with anything new you discover so future steps don't need to re-search.

### Step 4: E2E Tier 1 — `TestHumanOnlyChangesAndCommits`

This test requires no agent prompts — it only exercises hooks, so it's the fastest feedback loop.

**What it exercises:**
- `InstallHooks()` — real hook installation in the agent's config
- `AreHooksInstalled()` — detection that hooks are present
- `ParseHookEvent()` — at minimum, the event types needed for session start and turn end (see `EventType` constants in `event.go`)
- Basic hook invocation flow (the test calls hooks directly via the CLI)

**Cycle:**

1. Run: `mise run test:e2e:$AGENT_SLUG TestHumanOnlyChangesAndCommits`
2. Read the failure output carefully
3. If there are artifact dirs, use `/debug-e2e {artifact-dir}` to understand what happened
4. Implement the minimum code to fix the first failure
5. Repeat until the test passes

**After passing, write unit tests:**

- `hooks_test.go` — Test `InstallHooks` (creates config, idempotent), `UninstallHooks` (removes hooks), `AreHooksInstalled` (detects presence). Use a temp directory to avoid touching real config.
- `lifecycle_test.go` (initial) — Test `ParseHookEvent` for the event types exercised so far. Include nil return for unknown hook names and malformed JSON input. **Important:** Test against `EventType` constants from `event.go`, not native hook names — the agent's native hook verbs (e.g., "stop") map to normalized EventTypes (e.g., `TurnEnd`).

Run: `mise run fmt && mise run lint && mise run test`

### Step 5: E2E Tier 2 — `TestSingleSessionManualCommit`

The foundational test. This exercises the full agent lifecycle: start session → agent prompt → agent produces files → user commits → session ends.

**What it exercises:**
- Complete `ParseHookEvent()` for all lifecycle event types from `event.go`. Use the one-pager's hook mapping table to translate native hook names to `EventType` constants.
- `GetSessionDir` / `ResolveSessionFile` — finding the agent's session/transcript files
- `ReadTranscript` / `ChunkTranscript` / `ReassembleTranscript` — reading native transcript format
- `TranscriptAnalyzer` methods (see `agent.go` for current method signatures)

**Cycle:**

1. Run: `mise run test:e2e:$AGENT_SLUG TestSingleSessionManualCommit`
2. Read the failure output carefully
3. Use `/debug-e2e {artifact-dir}` to understand what happened
4. Implement the minimum code to fix the first failure
5. Repeat until the test passes

**After passing, write unit tests:**

- `types_test.go` — Test hook input struct parsing with actual JSON payloads from `AGENT.md` examples or captured payloads.
- `lifecycle_test.go` (complete) — Test `ParseHookEvent` for all 4 event types. Use actual JSON payloads. Test every `EventType` mapping, nil returns for pass-through hooks, empty input, and malformed JSON.
- `transcript_test.go` — Test `ReadTranscript`, `ChunkTranscript`, `ReassembleTranscript` with sample data in the agent's native format. Test all `TranscriptAnalyzer` methods (from `agent.go`) if implemented.

Run: `mise run fmt && mise run lint && mise run test`

### Step 6: E2E Tier 2b — `TestCheckpointMetadataDeepValidation`

Validates transcript quality: JSONL validity, content hash correctness, prompt extraction accuracy.

**What it exercises:**
- Transcript content stored at checkpoints is valid JSONL
- Content hash matches the stored transcript
- User prompts are correctly extracted
- Metadata fields are populated

**Cycle:**

1. Run: `mise run test:e2e:$AGENT_SLUG TestCheckpointMetadataDeepValidation`
2. Use `/debug-e2e {artifact-dir}` on any failures — this test often exposes subtle transcript formatting bugs
3. Fix and repeat

**After passing:** Update `transcript_test.go` if any edge cases were discovered.

Run: `mise run fmt && mise run lint && mise run test`

### Step 7: E2E Tier 3 — `TestSingleSessionAgentCommitInTurn`

Agent creates files and commits them within a single prompt turn. Tests the in-turn commit path.

**What it exercises:**
- Hook events firing during an agent's commit (post-commit hooks while agent is active)
- Checkpoint creation when agent commits mid-turn
- Usually no new agent-specific code needed — this tests the strategy's handling of agent commits

**Cycle:**

1. Run: `mise run test:e2e:$AGENT_SLUG TestSingleSessionAgentCommitInTurn`
2. Use `/debug-e2e {artifact-dir}` on failures
3. Fix and repeat — if the agent doesn't support committing, skip this test

**After passing:** Add any new edge cases to existing unit tests if bugs were found.

Run: `mise run fmt && mise run lint && mise run test`

### Step 8: E2E Tier 4 — Multi-Session Tests

Run these tests to validate multi-session behavior:

- `TestMultiSessionManualCommit` — Two sessions, both produce files, user commits
- `TestMultiSessionSequential` — Sessions run one after another
- `TestEndedSessionUserCommitsAfterExit` — User commits after session ends

**Cycle (for each test):**

1. Run: `mise run test:e2e:$AGENT_SLUG TestMultiSessionManualCommit`
2. Use `/debug-e2e {artifact-dir}` on failures
3. Fix and repeat
4. Move to next test

**After all pass:** These tests rarely need new agent code — they exercise the strategy layer. Update unit tests only if agent-specific bugs were found.

Run: `mise run fmt && mise run lint && mise run test`

### Step 9: E2E Tier 5 — File Operation Edge Cases

Run these tests for file operation correctness:

- `TestModifyExistingTrackedFile` — Agent modifies (not creates) a file
- `TestUserSplitsAgentChanges` — User stages only some of the agent's changes
- `TestDeletedFilesCommitDeletion` — Agent deletes a file, user commits the deletion
- `TestMixedNewAndModifiedFiles` — Agent both creates and modifies files

**Cycle:** Same as above — run each test, use `/debug-e2e` on failures, fix, repeat.

**After all pass:** Update unit tests if any transcript parsing or file-touched extraction bugs were discovered.

Run: `mise run fmt && mise run lint && mise run test`

### Step 10: Optional Interfaces

Read `cmd/entire/cli/agent/agent.go` for all optional interfaces. For each one the one-pager's "Gaps & Limitations" or "Transcript" sections suggest is feasible:

- **`TranscriptPreparer`** — If the agent needs pre-processing before transcript storage
- **`TokenCalculator`** — If the agent provides token usage data
- **`SubagentAwareExtractor`** — If the agent has subagent/tool-use patterns

For each optional interface:

1. Implement the methods based on `AGENT.md` and reference implementation
2. Write unit tests for the new methods
3. Run relevant E2E tests to verify integration

Run: `mise run fmt && mise run lint && mise run test`

### Step 11: E2E Tier 6 — Interactive and Rewind Tests

Run these if the agent supports interactive multi-step sessions:

- `TestInteractiveMultiStep` — Multiple prompts in one session
- `TestRewindPreCommit` — Rewind to a checkpoint before committing
- `TestRewindAfterCommit` — Rewind to a checkpoint after committing
- `TestRewindMultipleFiles` — Rewind with multiple files changed

**Cycle:** Same pattern — run, `/debug-e2e` on failures, fix, repeat.

Run: `mise run fmt && mise run lint && mise run test`

### Step 12: E2E Tier 7 — Complex Scenarios

Run the remaining edge case and stress tests:

- `TestPartialCommitStashNewPrompt` — Partial commit, stash, new prompt
- `TestStashSecondPromptUnstashCommitAll` — Stash workflow across prompts
- `TestRapidSequentialCommits` — Multiple commits in quick succession
- `TestAgentContinuesAfterCommit` — Agent keeps working after a commit
- `TestSubagentCommitFlow` — If the agent has subagent support
- `TestSingleSessionSubagentCommitInTurn` — Subagent commits during a turn

**Cycle:** Same pattern. Many of these require no new agent code — they exercise strategy-layer behavior.

Run: `mise run fmt && mise run lint && mise run test`

### Step 13: Verify Registration

Verify that registration from Step 3 is correct and complete:

1. The `init()` function in `${AGENT_PACKAGE}.go` calls `agent.Register(agent.AgentName("$AGENT_KEY"), New)`
2. The blank import in `cmd/entire/cli/hooks_cmd.go` is present
3. Run the full test suite: `mise run test:ci`

### Step 14: Final Validation

Run the complete validation:

```bash
mise run fmt      # Format
mise run lint     # Lint
mise run test:ci  # All tests (unit + integration)
```

Check against the integration checklist (`docs/architecture/agent-integration-checklist.md`):

- [ ] Full transcript stored at every checkpoint
- [ ] Native format preserved
- [ ] All mappable hook events implemented
- [ ] Session storage working
- [ ] Hook installation/uninstallation working
- [ ] Tests pass with `t.Parallel()`

## E2E Debugging Protocol

At every E2E failure, follow this protocol:

1. **Read the test output** — the assertion message often tells you exactly what's wrong
2. **Find the artifact directory** — E2E tests save artifacts (logs, transcripts, git state) to a temp dir printed in the output
3. **Run `/debug-e2e {artifact-dir}`** — this skill analyzes artifacts and diagnoses the root cause
4. **Implement the minimum fix** — don't over-engineer; fix only what the test demands
5. **Re-run the failing test** — not the whole suite, just the one test

## Key Patterns to Follow

- **Use `agent.ReadAndParseHookInput[T]`** for parsing hook stdin JSON
- **Use `paths.WorktreeRoot()`** not `os.Getwd()` for git-relative paths
- **Preserve unknown config keys** when modifying agent config files (don't clobber user settings)
- **Use `logging.Debug/Info/Warn/Error`** for internal logging, not `fmt.Print`
- **Keep interface implementations minimal** — only implement what's needed
- **Follow Go idioms** from `.golangci.yml` — check before writing code

## Output

Summarize what was implemented:
- Package directory and files created
- Interfaces implemented (core + optional)
- Hook names registered
- Test coverage (number of test functions, what they cover)
- Any gaps or TODOs remaining
- E2E tests passing (list which ones pass)
- Commands to run full validation
