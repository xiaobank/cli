# Heavyweight Memory Loop Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Turn the current memory-loop snapshot into a heavyweight memory manager with lifecycle states, layered personal and repo memory, explicit promotion for shared memory, history, pruning, and better operator controls.

**Architecture:** Extend the existing `memoryloop` JSON state into a unified memory store rather than a single snapshot. Keep generation and Claude injection in the current command and lifecycle flow, but add lifecycle-aware record reconciliation, richer commands, and outcome metadata so the store can support candidate review, manual memories, shared repo governance, and pruning.

**Tech Stack:** Go 1.26, Cobra CLI, local JSON state in `.entire/memory-loop.json`, existing insights cache and Claude hook injection path.

---

### Task 1: Expand the memory store model

**Files:**
- Modify: `cmd/entire/cli/memoryloop/memoryloop.go`
- Test: `cmd/entire/cli/memoryloop/memoryloop_test.go`

**Step 1: Add the new enums and state types**

Add lifecycle and scope types to `cmd/entire/cli/memoryloop/memoryloop.go`:

```go
type Mode string

const (
	ModeOff    Mode = "off"
	ModeManual Mode = "manual"
	ModeAuto   Mode = "auto"
)

type ActivationPolicy string

const (
	ActivationPolicyReview ActivationPolicy = "review"
	ActivationPolicyAuto   ActivationPolicy = "auto"
)

type Status string

const (
	StatusCandidate  Status = "candidate"
	StatusActive     Status = "active"
	StatusSuppressed Status = "suppressed"
	StatusArchived   Status = "archived"
)
```

**Step 2: Expand record and store fields**

Add store and record fields for:

- `Mode`
- `ActivationPolicy`
- `RefreshHistory`
- `Fingerprint`
- `ScopeKind`
- `ScopeValue`
- `Origin`
- `OwnerEmail`
- `LastReviewedAt`
- `LastInjectedAt`
- `LastMatchedAt`
- `InjectCount`
- `MatchCount`
- `Outcome`
- `History`

**Step 3: Preserve backward compatibility**

Update `normalizeState(...)` so older snapshot-only files still load with sane defaults:

- `mode=auto` if existing snapshot had injection enabled
- `activation_policy=review`
- missing records default to `active`

**Step 4: Run focused tests**

Run: `go test ./cmd/entire/cli/memoryloop -run 'Test.*State|Test.*Select|Test.*Format'`

Expected: existing state loading still works and new defaults normalize correctly.

**Step 5: Commit**

```bash
git add cmd/entire/cli/memoryloop/memoryloop.go cmd/entire/cli/memoryloop/memoryloop_test.go
git commit -m "feat: expand memory-loop state model"
```

### Task 2: Rework generation into reconcile-plus-history

**Files:**
- Modify: `cmd/entire/cli/memoryloop/generator.go`
- Modify: `cmd/entire/cli/memoryloop/memoryloop.go`
- Modify: `cmd/entire/cli/memory_loop_cmd.go`
- Test: `cmd/entire/cli/memoryloop/memoryloop_test.go`
- Test: `cmd/entire/cli/memory_loop_cmd_test.go`

**Step 1: Stop generating only active records**

Change generator output mapping so generated records are raw candidates with:

- stable `fingerprint`
- `origin=generated`
- `status` decided later during reconciliation

**Step 2: Add reconciliation helpers**

Implement helpers like:

```go
func ReconcileGeneratedRecords(existing []MemoryRecord, generated []MemoryRecord, mode Mode, policy ActivationPolicy) []MemoryRecord
func FindByFingerprint(records []MemoryRecord, fingerprint string) *MemoryRecord
```

Rules:

- dedupe against all statuses
- do not resurrect suppressed fingerprints as active
- keep archived history
- for repo-scoped generated records, keep as `candidate`
- for personal generated records, apply `review|auto`

**Step 3: Add refresh history entries**

Record:

- refresh time
- source window
- scope
- number generated
- number activated
- number left as candidate

**Step 4: Run focused tests**

Run: `go test ./cmd/entire/cli/... -run 'Test.*MemoryLoop.*Refresh|Test.*Reconcile'`

Expected: new generated records reconcile correctly against existing active, suppressed, and archived memory.

**Step 5: Commit**

```bash
git add cmd/entire/cli/memoryloop/generator.go cmd/entire/cli/memoryloop/memoryloop.go cmd/entire/cli/memory_loop_cmd.go cmd/entire/cli/memoryloop/memoryloop_test.go cmd/entire/cli/memory_loop_cmd_test.go
git commit -m "feat: reconcile generated memories with history"
```

### Task 3: Add mode and activation policy controls

**Files:**
- Modify: `cmd/entire/cli/memory_loop_cmd.go`
- Modify: `cmd/entire/cli/settings/settings.go`
- Test: `cmd/entire/cli/memory_loop_settings_test.go`
- Test: `cmd/entire/cli/memory_loop_cmd_test.go`

**Step 1: Replace enable/disable with mode**

Add:

- `entire memory-loop mode off|manual|auto`
- `entire memory-loop policy review|auto`

Remove the old binary mental model from command output even if you keep compatibility wrappers.

**Step 2: Wire defaults from settings/state**

Persist:

- mode
- activation policy
- max injected

Keep state authoritative once the memory store exists.

**Step 3: Run focused tests**

Run: `go test ./cmd/entire/cli/... -run 'Test.*Mode|Test.*Policy'`

Expected: commands update stored state and status output reflects the new controls.

**Step 4: Commit**

```bash
git add cmd/entire/cli/memory_loop_cmd.go cmd/entire/cli/settings/settings.go cmd/entire/cli/memory_loop_settings_test.go cmd/entire/cli/memory_loop_cmd_test.go
git commit -m "feat: add memory-loop mode and activation policy"
```

### Task 4: Add progress output and richer refresh summaries

**Files:**
- Modify: `cmd/entire/cli/memory_loop_cmd.go`
- Test: `cmd/entire/cli/memory_loop_cmd_test.go`

**Step 1: Print explicit refresh progress**

Emit lines before each major step:

```text
Refreshing cache...
Backfilling summaries...
Backfilling facets...
Loading scoped sessions...
Distilling memories...
Reconciling with existing memory history...
Saving memory store...
```

**Step 2: Print counts by lifecycle**

At refresh completion, print:

- active count
- candidate count
- suppressed count
- archived count
- generated and activated counts for this refresh

**Step 3: Run focused tests**

Run: `go test ./cmd/entire/cli/... -run 'Test.*Refresh.*Output'`

Expected: progress steps and final counts render predictably.

**Step 4: Commit**

```bash
git add cmd/entire/cli/memory_loop_cmd.go cmd/entire/cli/memory_loop_cmd_test.go
git commit -m "feat: add memory-loop refresh progress output"
```

### Task 5: Add lifecycle management commands

**Files:**
- Modify: `cmd/entire/cli/memory_loop_cmd.go`
- Modify: `cmd/entire/cli/memoryloop/memoryloop.go`
- Test: `cmd/entire/cli/memory_loop_cmd_test.go`
- Test: `cmd/entire/cli/memoryloop/memoryloop_test.go`

**Step 1: Add explicit commands**

Implement:

- `activate <id>`
- `promote <id>`
- `suppress <id>`
- `unsuppress <id>`
- `archive <id>`

**Step 2: Encode governance rules**

Rules:

- `activate` should not turn a repo candidate into shared-active
- `promote` is required for repo-scoped generated candidates
- `unsuppress` returns to `candidate`
- `archive` preserves history

**Step 3: Record history events**

Append record history entries on each lifecycle transition.

**Step 4: Run focused tests**

Run: `go test ./cmd/entire/cli/... -run 'Test.*Activate|Test.*Promote|Test.*Suppress|Test.*Archive'`

Expected: lifecycle transitions behave correctly for personal and repo-scoped records.

**Step 5: Commit**

```bash
git add cmd/entire/cli/memory_loop_cmd.go cmd/entire/cli/memoryloop/memoryloop.go cmd/entire/cli/memory_loop_cmd_test.go cmd/entire/cli/memoryloop/memoryloop_test.go
git commit -m "feat: add memory-loop lifecycle commands"
```

### Task 6: Add manual memory entry

**Files:**
- Modify: `cmd/entire/cli/memory_loop_cmd.go`
- Modify: `cmd/entire/cli/memoryloop/memoryloop.go`
- Test: `cmd/entire/cli/memory_loop_cmd_test.go`
- Test: `cmd/entire/cli/memoryloop/memoryloop_test.go`

**Step 1: Implement `add`**

Add:

```text
entire memory-loop add --kind repo_rule --title "..." --body "..." --scope me|repo
```

Defaults:

- `scope=me`
- `origin=manual`
- `status=active`

**Step 2: Validate scope and IDs**

Ensure:

- repo-scoped manual adds are explicit
- fingerprints dedupe against existing records
- manual records are clearly marked for pruning exemptions

**Step 3: Run focused tests**

Run: `go test ./cmd/entire/cli/... -run 'Test.*Add'`

Expected: manual memories become active immediately and preserve scope and origin metadata.

**Step 4: Commit**

```bash
git add cmd/entire/cli/memory_loop_cmd.go cmd/entire/cli/memoryloop/memoryloop.go cmd/entire/cli/memory_loop_cmd_test.go cmd/entire/cli/memoryloop/memoryloop_test.go
git commit -m "feat: add manual memory entries"
```

### Task 7: Improve status and show output for review and retrieval visibility

**Files:**
- Modify: `cmd/entire/cli/memory_loop_cmd.go`
- Modify: `cmd/entire/cli/memoryloop/memoryloop.go`
- Test: `cmd/entire/cli/memory_loop_cmd_test.go`

**Step 1: Separate `status` and `show` responsibilities**

- `status`
  - mode
  - activation policy
  - scope
  - counts by status
  - last refresh
  - optional prompt preview
- `show`
  - grouped detailed inventory
  - recent refresh history
  - recent injection logs

**Step 2: Add verbose retrieval preview**

For `status --prompt --verbose`, show:

- memory id
- score
- reason
- scope
- status

**Step 3: Run focused tests**

Run: `go test ./cmd/entire/cli/... -run 'Test.*Show|Test.*Status|Test.*PromptPreview'`

Expected: status is concise, show is detailed, and prompt preview includes reasons.

**Step 4: Commit**

```bash
git add cmd/entire/cli/memory_loop_cmd.go cmd/entire/cli/memoryloop/memoryloop.go cmd/entire/cli/memory_loop_cmd_test.go
git commit -m "feat: improve memory-loop inspection output"
```

### Task 8: Add outcome tracking and pruning

**Files:**
- Modify: `cmd/entire/cli/lifecycle.go`
- Modify: `cmd/entire/cli/memoryloop/memoryloop.go`
- Modify: `cmd/entire/cli/memory_loop_cmd.go`
- Test: `cmd/entire/cli/lifecycle_test.go`
- Test: `cmd/entire/cli/memory_loop_cmd_test.go`
- Test: `cmd/entire/cli/memoryloop/memoryloop_test.go`

**Step 1: Record match and injection activity**

Update retrieval and injection paths to persist:

- `match_count`
- `last_matched_at`
- `inject_count`
- `last_injected_at`

**Step 2: Derive simple outcomes during refresh**

Use recent sessions and facets to mark records as:

- `neutral`
- `reinforced`
- `ineffective`

**Step 3: Add `prune` command**

Apply default rules:

- archive stale candidates after 30 days
- archive active memories with zero matches after 60 days
- demote or archive ineffective active memories after repeated injection
- never auto-prune manual memories

**Step 4: Run focused tests**

Run: `go test ./cmd/entire/cli/... -run 'Test.*Prune|Test.*Injection|Test.*Outcome'`

Expected: activity metadata updates during injection and pruning changes only eligible generated records.

**Step 5: Commit**

```bash
git add cmd/entire/cli/lifecycle.go cmd/entire/cli/memoryloop/memoryloop.go cmd/entire/cli/memory_loop_cmd.go cmd/entire/cli/lifecycle_test.go cmd/entire/cli/memory_loop_cmd_test.go cmd/entire/cli/memoryloop/memoryloop_test.go
git commit -m "feat: add memory-loop outcomes and pruning"
```

### Task 9: Manual verification pass

**Files:**
- Modify: `cmd/entire/cli/memory_loop_cmd.go`
- Modify: `cmd/entire/cli/memoryloop/memoryloop.go`
- Modify: `cmd/entire/cli/lifecycle.go`

**Step 1: Run formatter**

Run: `mise run fmt`

Expected: no formatting diffs remain.

**Step 2: Run targeted package tests if present**

Run: `go test ./cmd/entire/cli/...`

Expected: passing, if tests were kept current during implementation.

**Step 3: Run lint**

Run: `mise run lint`

Expected: no lint findings.

**Step 4: Manual operator flow**

Run:

```bash
entire memory-loop refresh --last 20 --scope me
entire memory-loop show
entire memory-loop status --prompt "fix lint in capabilities.go" --verbose
entire memory-loop add --kind repo_rule --title "Run lint before final answer" --body "Run lint before concluding changes." --scope me
entire memory-loop prune
```

Expected:

- progress output appears
- candidate and active records are separated
- prompt preview shows reasons and scores
- manual memory is active immediately
- prune only affects eligible generated records

**Step 5: Commit**

```bash
git add cmd/entire/cli/memory_loop_cmd.go cmd/entire/cli/memoryloop/memoryloop.go cmd/entire/cli/lifecycle.go
git commit -m "chore: verify heavyweight memory-loop workflow"
```

## Notes

- The user explicitly said this is a PoC and not to prioritize writing tests. Keep manual verification as the main acceptance path.
- If lightweight command tests already exist, update only the ones directly touched instead of adding broad new coverage.
- Do not change `insights` or `improve` behavior while implementing this plan.
