# Carry-Forward Improvements

## Status: Design

## Problem Statement

When an agent modifies files during a session, users may commit those changes across
multiple `git commit` invocations (split commits). The system needs to track which
parts of the agent's work have been committed and which remain, so that:

1. Each commit gets its own checkpoint with accurate metadata
2. The system knows when all agent work has been committed (carry-forward complete)
3. No git refs (shadow branches) need to linger after the session ends

### Current Behavior (File-Level)

Today, carry-forward operates at **file granularity**:

- Agent modifies files A, B, C
- User commits A, B → carry-forward tracks file C
- User commits C → carry-forward complete

For partial commits (`git add -p`), the system detects "file still has changes" via
whole-file blob hash comparison + working tree dirty check, but doesn't know WHICH
hunks were committed. The shadow branch must persist for the entire carry-forward
period to hold the agent's file blobs for comparison.

### Desired Behavior (Hunk-Level)

The system should track agent changes at the **hunk/diff level**:

- Agent modifies `auth.go` (lines 20-24 and lines 100-120)
- User commits lines 20-24 via `git add -p` → system knows lines 100-120 remain
- User commits lines 100-120 → carry-forward complete for this file
- Accurate per-commit attribution: commit 1 gets credit for lines 20-24, commit 2 for 100-120
- Shadow branch deleted after first commit; carry-forward uses lightweight local ref

## Design

### Algorithm: Three-Way Diff (diff3)

The core algorithm uses **three-way diffing** to classify every chunk of a file:

Given three versions:
- **O** (origin/base) — file state before the agent (`state.AttributionBaseCommit`)
- **A** (agent) — what the agent wrote (blob stored in carry-forward ref)
- **B** (committed) — what the user committed (in HEAD tree)

diff3 classifies each region:

| O→A | O→B | Classification | Action |
|-----|-----|---------------|--------|
| unchanged | unchanged | Untouched | Skip |
| **changed** | unchanged | Agent change NOT committed | **Remaining** — keep in carry-forward |
| unchanged | **changed** | User's own edit | Skip — not from agent |
| **same change** | **same change** | Agent change committed | **Consumed** — remove from carry-forward |
| **different changes** | **different changes** | Agent change modified by user | **Consumed** (user took ownership) |

This naturally handles:
- **Partial staging** (`git add -p`): consumed hunks show as "same change in both"
- **User edits between commits**: user-only regions classified correctly
- **Reverts**: if user reverts an agent hunk, O→B shows unchanged → "remaining" is cleared
- **User modifications of agent work**: both-changed-differently → consumed (user took ownership)

#### Why diff3 over alternatives

| Approach | Pros | Cons |
|----------|------|------|
| **diff3 (chosen)** | Exact classification, handles all edge cases, proven algorithm | Needs all 3 versions available |
| Patch commutation (git-absorb) | Elegant, well-founded in theory | Only detects overlap, not consumption direction |
| Reverse-apply check | Simple concept | Fragile with context changes, requires shelling out |
| Hunk line-range overlap | Fast | Breaks with line drift, can't handle user edits |

### Storage: `refs/entire/carry_forward`

A single custom git ref holds all carry-forward data across all sessions:

```
refs/entire/carry_forward → commit → tree:
  <session-id-1>/
    src/auth.go          ← agent's file blob (version A)
    src/main.go
  <session-id-2>/
    lib/utils.py
```

**Properties:**
- **Not pushed**: Custom refs outside `refs/heads/` and `refs/tags/` are not pushed
  by default. The pre-push hook that pushes `entire/checkpoints/v1` ignores it.
- **Not fetched**: Not included in default clone/fetch refspecs.
- **Prevents GC**: Git treats all refs as reachability roots. Blobs referenced by
  this tree are protected from garbage collection.
- **Invisible**: Not shown by `git branch` or `git tag`.
- **Single ref**: One ref for all sessions. Adding/removing sessions mutates the tree.
  When the last session completes, the ref is deleted entirely.
- **go-git compatible**: `Storer.SetReference()` accepts arbitrary ref names. The
  codebase already uses this pattern for shadow branches.

#### Why not other storage options

| Option | Problem |
|--------|---------|
| Shadow branch (`refs/heads/entire/...`) | Visible in `git branch`, one ref per session |
| `entire/checkpoints/v1` tree | Pushed to remote, creates unnecessary commits |
| Session state JSON | Can't hold git blob references (GC-unsafe) |
| Filesystem (`.entire/snapshots/`) | No dedup, no GC integration, uses disk space |

### Session State Additions

The session state file (`.git/entire-sessions/<id>.json`) gets minimal new fields:

```json
{
  "carry_forward_base": "abc123def456...",
  "carry_forward_files": ["src/auth.go", "src/main.go"]
}
```

- `carry_forward_base`: The base commit hash (O in the diff3 triple). This is the
  commit the agent's changes were relative to. Needed to retrieve version O.
- `carry_forward_files`: Which files still have remaining agent hunks. Used for quick
  checks without reading the carry-forward ref tree.

The actual agent file blobs (version A) live in the carry-forward ref, not in the
state file. The state file only has lightweight metadata.

### Lifecycle

```
                    ┌─────────────────────────────────────────────┐
                    │              ACTIVE SESSION                  │
                    │                                             │
                    │  Shadow branch: entire/<hash>-<worktree>    │
                    │  Stores: file snapshots + transcript        │
                    │  (unchanged from today)                     │
                    └──────────────────┬──────────────────────────┘
                                       │
                              First commit
                                       │
                    ┌──────────────────▼──────────────────────────┐
                    │            CONDENSATION                      │
                    │                                             │
                    │  1. Condense transcript → checkpoints/v1    │
                    │  2. Compute remaining files (as today)      │
                    │  3. If remaining files:                     │
                    │     a. Write agent blobs to carry-forward   │
                    │        ref under <session-id>/              │
                    │     b. Set carry_forward_base in state      │
                    │  4. Delete shadow branch                    │
                    └──────────────────┬──────────────────────────┘
                                       │
                           ┌───────────┴───────────┐
                           │                       │
                    No remaining files      Has remaining files
                           │                       │
                    ┌──────▼──────┐    ┌───────────▼──────────────┐
                    │  DONE       │    │    CARRY-FORWARD          │
                    │  FullyCond. │    │                           │
                    │  = true     │    │  On each subsequent       │
                    └─────────────┘    │  commit (PostCommit):     │
                                       │                           │
                                       │  1. For each file in      │
                                       │     carry_forward_files:  │
                                       │     a. Get O from base    │
                                       │     b. Get A from ref     │
                                       │     c. Get B from HEAD    │
                                       │     d. Run diff3(O, A, B) │
                                       │     e. If no "remaining"  │
                                       │        chunks → consumed  │
                                       │  2. Update ref tree       │
                                       │     (remove consumed)     │
                                       │  3. Update state file     │
                                       │  4. If all consumed →     │
                                       │     delete ref subtree,   │
                                       │     FullyCondensed=true   │
                                       └──────────────────────────┘
```

### Detailed Flow: PostCommit with Hunk-Level Carry-Forward

For each session in `postCommitProcessSession`:

1. **Check carry-forward state**: If `carry_forward_files` is empty, skip (no
   carry-forward). Fall through to existing file-level logic for active sessions.

2. **Load the three versions** for each carry-forward file:
   - **O** (base): `repo.CommitObject(carry_forward_base).Tree().File(path)`
   - **A** (agent): read from `refs/entire/carry_forward` tree at `<session-id>/<path>`
   - **B** (committed): `HEAD.Tree().File(path)` (the just-committed version)

3. **Run diff3** per file:
   - Use `epiclabs-io/diff3` or equivalent Go library
   - Classify each chunk as consumed, remaining, or user-only

4. **Determine file status**:
   - If no "remaining" chunks → file is fully consumed
   - If "remaining" chunks exist → file stays in carry-forward
   - If file was deleted in B → consumed (user removed it, agent work is moot)
   - If file not in committed files set → skip (not part of this commit)

5. **Update carry-forward ref**:
   - Remove consumed files from the `<session-id>/` subtree
   - Write a new commit on the ref with the updated tree
   - If subtree is empty, remove the session's directory entirely

6. **Update session state**:
   - Update `carry_forward_files` to only remaining files
   - If empty → set `FullyCondensed = true`, clean up ref subtree

7. **For checkpoint metadata**: The consumed hunks can be recorded in the checkpoint
   on `entire/checkpoints/v1` for per-commit attribution.

### Detailed Flow: PrepareCommitMsg with Hunk-Level Carry-Forward

For sessions with `carry_forward_files`:

1. **Check staged files overlap**: Intersect staged files with `carry_forward_files`
2. **For overlapping files, run diff3** with staged content (from git index) as B:
   - If any chunks classify as "consumed" → agent work is being committed → add trailer
   - If no consumed chunks → staged content is user-only or reverted → skip trailer
3. This replaces the current `stagedFilesOverlapWithContent` for carry-forward sessions

### Edge Cases

#### User rebases between split commits
- `carry_forward_base` points to a commit that may no longer be in the branch history
- But the commit object still exists in the repo (not GC'd — still reachable via reflog)
- diff3 still works: O is read from the old base commit's tree, A from carry-forward ref
- If the base commit IS GC'd (unlikely within 90-day reflog window): fall back to
  file-level comparison

#### User amends the first commit
- PostCommit fires again for the amend
- diff3 runs with the amended commit as B
- If amend included more agent hunks → those are now consumed
- carry_forward_base doesn't change (still the original pre-agent base)

#### Agent session resumes (ENDED → ACTIVE via new TurnStart)
- `ActionClearEndedAt` already clears `FullyCondensed`
- New SaveStep creates a new shadow branch (normal active session flow)
- On next commit, fresh condensation + carry-forward computation
- Any leftover carry-forward ref data from the previous ended session is overwritten

#### Binary files
- diff3 doesn't work on binary files
- Fall back to file-level blob hash comparison (current behavior)
- Detection: check if file content contains null bytes or use go-git's `IsBinary()`

#### File renamed between commits
- If user renames a carry-forward file, the old path won't match
- Detection: use go-git's `DiffTreeWithOptions` with `DetectRenames: true`
- If rename detected, update the path in carry-forward ref and state

## Go Libraries

| Library | Purpose | Notes |
|---------|---------|-------|
| `epiclabs-io/diff3` | Three-way merge/diff | Go port, returns classified chunks |
| `github.com/sergi/go-diff` | Low-level Myers diff | Already a go-git dependency |
| `github.com/bluekeyes/go-gitdiff` | Parse unified diff output | Structured `TextFragment` (hunk) types |
| `go-git/go-git/v5` | Git operations, tree diff | `DiffTree()` → `Patch()` → `Chunks()` |

## Performance Budget

| Operation | Expected Time | Notes |
|-----------|--------------|-------|
| Read 3 file versions from git | 1-3ms | go-git blob reads |
| diff3 per file | 2-10ms | Depends on file size |
| 5 carry-forward files | 15-65ms total | Parallelizable |
| Update carry-forward ref | 5-10ms | Single tree mutation + commit |
| **Total PostCommit overhead** | **20-75ms** | Within <200ms budget |

Current PostCommit is <100ms. Adding hunk tracking stays within 200ms for typical cases.

## Migration Path

1. **Phase 1**: Implement `refs/entire/carry_forward` storage alongside existing shadow
   branch carry-forward. Both paths active. New path used when carry-forward ref exists,
   old path as fallback.

2. **Phase 2**: At condensation time, always write to carry-forward ref instead of
   creating a new shadow branch. Shadow branch deleted immediately after first commit.

3. **Phase 3**: Remove shadow-branch carry-forward code. Add diff3-based hunk
   consumption detection. File-level comparison becomes the fallback for binary files
   and error cases.

## Files to Modify

| File | Change |
|------|--------|
| `strategy/content_overlap.go` | Add `hunksWithRemainingAgentChanges()` using diff3 |
| `strategy/manual_commit_hooks.go` | PostCommit: add carry-forward ref read/write; PrepareCommitMsg: diff3-based overlap |
| `strategy/manual_commit_hooks.go` | Replace `carryForwardToNewShadowBranch` with `carryForwardToRef` |
| `session/state.go` | Add `CarryForwardBase`, `CarryForwardFiles` fields |
| `strategy/manual_commit_condensation.go` | At condensation: write agent blobs to carry-forward ref |
| `checkpoint/` (new file) | `carry_forward.go`: ref management (read/write/update/cleanup) |
| `strategy/cleanup.go` | Add carry-forward ref cleanup to `ListOrphanedItems` |

## Open Questions (Option A)

1. Should we store the unified diff text alongside the blob in the carry-forward ref
   (for debugging/inspection), or just the blob?
2. Should carry-forward ref commits have meaningful messages (for `git log refs/entire/carry_forward`)?
3. What's the right stale timeout for carry-forward data? (Currently 7 days for session state)
4. Should `entire status` show carry-forward hunk counts per file?

---

# Option B: Session-Bounded Carry-Forward

## Overview

A simpler alternative to hunk-level tracking. Instead of tracking which hunks were
consumed across sessions, **carry-forward only persists until the next session produces
changes**. When a new session's first checkpoint captures the working tree, any ENDED
sessions with remaining carry-forward are cleaned up and their uncommitted changes
are absorbed into the new session's shadow branch.

This accepts a tradeoff: uncommitted changes from Session 1 get attributed to
Session 2. In exchange, it eliminates the need for diff3, custom refs, and new
libraries entirely.

## Design

### Core Rule

> Carry-forward within a session works as today (file-level, shadow branch).
> Cross-session carry-forward does not exist. When a new session produces its first
> checkpoint, all ENDED sessions with carry-forward on the same worktree are cleaned up.

### Why This Works

When a new session's `SaveStep` runs with `StepCount == 0`, `WriteTemporary` uses
`IsFirstCheckpoint: true`, which calls `collectChangedFiles()` (`git status --porcelain -z`).
This captures **everything** dirty in the working tree — including leftover uncommitted
changes from old sessions. The new shadow branch naturally absorbs those changes.

No diff computation needed. No new storage needed. The existing shadow branch mechanics
handle everything.

### Lifecycle

```
Session 1: Agent changes lines 20-50, 100-120 in auth.go
     │
     ▼
User commits lines 20-50 (git add -p)
     │  PostCommit: condense, carry-forward with auth.go (100-120 still in working tree)
     │  Shadow branch updated: base=new HEAD, auth.go from working tree
     │
     ▼
Session 1 ends (Phase=ENDED, shadow branch persists for carry-forward)
     │
     │  ┌─── If user commits lines 100-120 before new session ───┐
     │  │  PostCommit: condense ENDED session, FullyCondensed=true │
     │  │  Shadow branch deleted. Clean end.                       │
     │  └──────────────────────────────────────────────────────────┘
     │
     ▼
Session 2 starts, first turn produces changes
     │  SaveStep (StepCount=0, IsFirstCheckpoint=true):
     │    1. collectChangedFiles() captures full working tree
     │       (includes auth.go with lines 100-120 from Session 1)
     │    2. New shadow branch created with ALL current changes
     │    3. ENDED sessions with carry-forward on this worktree:
     │       → Mark FullyCondensed=true
     │       → Delete their shadow branches
     │
     ▼
Lines 100-120 are now part of Session 2's shadow branch.
When committed, they are attributed to Session 2.
```

### Attribution Tradeoff

Lines 100-120 were written by Session 1's agent but get attributed to Session 2.

This is acceptable because:

- **Session 1's checkpoint for commit 1 is correct**: lines 20-50 are properly
  attributed to Session 1 with the right transcript and metadata.
- **The user made a choice**: they started a new session without committing the rest.
  The remaining changes become part of the new working context.
- **When the lines ARE committed**: they are attributed to whatever session is active.
  This is accurate — the session that "owns" the working tree at commit time gets credit.
- **When the lines are NOT committed in Session 2 either**: they carry forward again
  within Session 2 (same mechanism), and get cleaned up when Session 3 starts, or when
  they're eventually committed, or via the 7-day stale timeout.
- **The alternative is worse**: keeping Session 1's shadow branch alive indefinitely
  for attribution precision that the user likely doesn't care about.

### Implementation

The change is small — roughly 20-30 lines of new code.

#### Where: `SaveStep` in `manual_commit_git.go`

Before creating the first checkpoint (`StepCount == 0`), find and clean up ENDED
sessions with carry-forward on the same worktree:

```go
// In SaveStep, before WriteTemporary for first checkpoint:
if state.StepCount == 0 {
    s.cleanupEndedSessionsWithCarryForward(ctx, repo, state.SessionID, state.WorktreePath)
}
```

#### New method: `cleanupEndedSessionsWithCarryForward`

```go
func (s *ManualCommitStrategy) cleanupEndedSessionsWithCarryForward(
    ctx context.Context,
    repo *git.Repository,
    currentSessionID string,
    worktreePath string,
) {
    sessions, err := s.findSessionsForWorktree(ctx, worktreePath)
    if err != nil {
        return // Best-effort cleanup
    }

    for _, old := range sessions {
        if old.SessionID == currentSessionID {
            continue
        }
        if old.Phase != session.PhaseEnded {
            continue
        }
        if len(old.FilesTouched) == 0 && old.FullyCondensed {
            continue // Already fully condensed
        }

        // Clean up: mark fully condensed and delete shadow branch
        old.FullyCondensed = true
        old.FilesTouched = nil
        _ = s.saveSessionState(ctx, old)

        shadowBranch := getShadowBranchNameForCommit(old.BaseCommit, old.WorktreeID)
        _ = deleteShadowBranch(ctx, repo, shadowBranch)

        logging.Info(logging.WithComponent(ctx, "checkpoint"),
            "cleaned up ended session carry-forward (new session taking over)",
            slog.String("old_session_id", old.SessionID),
            slog.String("new_session_id", currentSessionID),
        )
    }
}
```

#### What stays the same

- Shadow branch creation during active sessions: unchanged
- `carryForwardToNewShadowBranch`: unchanged (within-session split commits still work)
- `filesWithRemainingAgentChanges`: unchanged
- `filesOverlapWithContent` / `stagedFilesOverlapWithContent`: unchanged
- PostCommit flow: unchanged
- Session state machine: unchanged
- `entire/checkpoints/v1` format: unchanged

### Comparison: Option A vs Option B

| Aspect | Option A (Hunk-Level) | Option B (Session-Bounded) |
|--------|----------------------|---------------------------|
| **Complexity** | New algorithm (diff3), new ref namespace, new library | ~30 lines of cleanup code |
| **Cross-session attribution** | Precise (each hunk attributed to originating session) | Approximate (leftover hunks attributed to new session) |
| **Within-session split commits** | Hunk-level precision | File-level precision (same as today) |
| **Shadow branch lifetime** | Deleted at first commit (carry-forward moves to ref) | Deleted at first commit OR next session start |
| **New dependencies** | `epiclabs-io/diff3` | None |
| **New git concepts** | `refs/entire/carry_forward` custom ref | None |
| **Risk** | Medium (new algorithm, new storage, new lifecycle) | Low (small change to existing cleanup logic) |
| **Migration** | Multi-phase rollout | Single change, backward compatible |
| **Handles user never commits** | Carry-forward persists in ref (still needs timeout) | Same as today (7-day stale timeout) |

### Recommendation

**Start with Option B.** It solves the main problem (lingering shadow branches for
ENDED sessions) with minimal risk and code change. The attribution tradeoff is
acceptable for most users.

Option A can be pursued later if precise cross-session hunk attribution becomes a
user-visible requirement (e.g., for billing, compliance, or detailed contribution
tracking).
