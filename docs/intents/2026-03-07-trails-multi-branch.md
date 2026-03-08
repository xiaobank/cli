# Trails: Multi-Branch & Multi-PR Support

**Date:** 2026-03-07
**Status:** Draft

## Problem

Trails currently model a 1:1 relationship between a trail and a branch. Real work
often spans multiple branches and PRs: stacked PRs for large features, iterative
follow-up PRs, or parallel refactor-then-feature splits. The trail should represent
the *intent*, with branches/PRs as execution artifacts serving that intent.

## Design Principles

- **Trail = intent.** Branches and PRs are mutable execution details.
- **Flat list of branches.** No dependency graph — git already encodes branch topology.
- **Stateless lookup.** Active trail resolved from current branch, not stored context.
- **CAS for concurrency.** Optimistic retry on ref updates, not file splitting.
- **Extensible verification.** Unified event log for all verification types.

## Data Model

### Trail (top-level)

```
Trail
+-- title              (human-friendly label for display)
+-- status             (draft | active | validating | done | abandoned)
+-- intent             (typed reference — the "what and why")
+--   kind             (file, url, issue, inline)
+--   value            (path, URL, issue ID, inline text)
+--   content          (resolved/cached text)
+--   amendments[]     (spec changes with reasoning)
+--     description
+--     reasoning
+--     timestamp
+-- branches[]         (execution artifacts — the "how")
+--   id               (stable UUID)
+--   name             (branch ref, informational — may go stale)
+--   base_branch      (target branch name, e.g. "main")
+--   base_commit      (fork point SHA, updated on rebase)
+--   status           (open | merged | discarded)
+--   pr               (optional, replaceable)
+--     number
+--     url
+-- checkpoints[]      (the spine — the "when")
+--   checkpoint_id
+--   commit_sha
+--   summary
+--   branch_id        (links to BranchEntry.ID)
+--   sessions[]       (contributing agent sessions)
+-- verification[]     (event log — unified across all verification types)
+--   kind             (pr_checks, review, security, perf, canary,
+--                     rollout_stage, trail_review, ...)
+--   branch_id        (optional — per-branch or per-trail)
+--   status           (pass, fail, pending, requested, approved,
+--                     changes_requested)
+--   timestamp
+--   details
+-- discussion[]       (comments/threads — separate file)
+-- summary            (aggregated trail-level summary)
```

### Intent (typed reference)

```go
type Intent struct {
    Kind       string      `json:"kind"`                // file, url, issue, inline
    Value      string      `json:"value"`               // path, URL, issue ID, inline text
    Content    string      `json:"content,omitempty"`    // resolved intent text (cached)
    Amendments []Amendment `json:"amendments,omitempty"`
}

type Amendment struct {
    Description string    `json:"description"`
    Reasoning   string    `json:"reasoning"`
    Timestamp   time.Time `json:"timestamp"`
}
```

### BranchEntry

```go
type BranchEntry struct {
    ID         string       `json:"id"`                  // stable UUID
    Name       string       `json:"name"`                // branch name (informational, may go stale)
    BaseBranch string       `json:"base_branch"`         // target branch name
    BaseCommit string       `json:"base_commit"`         // fork point SHA
    Status     BranchStatus `json:"status"`              // open, merged, discarded
    PR         *PRRef       `json:"pr,omitempty"`        // optional, replaceable
    AddedAt    time.Time    `json:"added_at"`
}

type BranchStatus string
const (
    BranchOpen      BranchStatus = "open"
    BranchMerged    BranchStatus = "merged"
    BranchDiscarded BranchStatus = "discarded"
)

type PRRef struct {
    Number int    `json:"number"`
    URL    string `json:"url,omitempty"`
}
```

### CheckpointRef (extended)

```go
type CheckpointRef struct {
    CheckpointID string    `json:"checkpoint_id"`
    CommitSHA    string    `json:"commit_sha"`
    Summary      string    `json:"summary,omitempty"`
    BranchID     string    `json:"branch_id,omitempty"` // links to BranchEntry.ID
    CreatedAt    time.Time `json:"created_at"`
}
```

### VerificationEvent

```go
type VerificationEvent struct {
    Kind      string    `json:"kind"`                 // pr_checks, review, security, perf, canary, trail_review, ...
    BranchID  string    `json:"branch_id,omitempty"`  // optional — per-branch or per-trail
    Status    string    `json:"status"`               // pass, fail, pending, requested, approved, changes_requested
    Timestamp time.Time `json:"timestamp"`
    Details   string    `json:"details,omitempty"`
}
```

### Trail Status

```
draft -> active -> validating -> done
                       |
                   abandoned
```

- **draft** — intent defined, work hasn't started
- **active** — branches exist, work in progress
- **validating** — PRs in review, canary running, rollout in progress — all "is this good?"
- **done** — intent fully realized and verified (all branches merged/discarded, verifications pass)
- **abandoned** — intent dropped

"Done" is derived: all branch statuses are terminal (merged or discarded) and at least
one is merged, plus verification events are satisfactory. The status field captures the
current state; the verification event log captures the evidence.

### Trail Review

Trail review is a verification event, not a status. It can happen at any point in the
lifecycle (design review before coding, checkpoint review mid-feature, final review
before rollout). Signaling mechanism TBD — rules/policies on top of the verification
event log.

```
verification event:
  kind: "trail_review"
  status: "requested" | "approved" | "changes_requested"
```

## Storage Layout

On `entire/trails/v1` orphan branch, sharded by trail ID:

```
<trail-id[:2]>/<trail-id[2:]>/
+-- metadata.json       (title, status, intent, branches, summary)
+-- checkpoints.json    (checkpoint refs with branch_id)
+-- verification.json   (verification event log)
+-- discussion.json     (comments/threads)
```

Single `metadata.json` per trail. CAS retry handles concurrent writes — no need for
file splitting or subdirectories.

## Concurrency: CAS Ref Updates

The codebase currently uses unconditional `SetReference` for all ref updates. This
is a race condition when two sessions write concurrently to the same orphan branch
(e.g., two worktrees appending checkpoints to the same trail).

**Fix:** Use go-git's `CheckAndSetReference(new, old)` for CAS semantics.

```go
// Write loop with optimistic retry
for {
    tipHash, rootTree := readBranchTip()    // step 1: read
    newTree := applyChanges(rootTree)        // step 2: modify
    newCommit := createCommit(newTree, tipHash)
    err := repo.Storer.CheckAndSetReference(
        plumbing.NewHashReference(refName, newCommit),
        plumbing.NewHashReference(refName, tipHash),  // expected old value
    )
    if err == nil {
        break // success
    }
    // ref moved — retry from step 1
}
```

This is a foundational improvement that benefits all orphan branch writes (trails,
checkpoints, metadata), not just multi-branch trails.

## Trail Context Resolution (Stateless)

No stored "active trail" context. Resolved fresh on every command:

1. **`--trail <id>` flag** — explicit, always wins
2. **Branch lookup** — current branch found in exactly one trail's `Branches[]`
3. **No match / ambiguous** — prompt or error

This avoids stale context when switching branches within a worktree. The cost is a
branch-to-trail scan on every command, which is acceptable at expected scale.

## CLI Commands

### Trail creation (modified)

```bash
entire trail create "Add auth system"
entire trail create "Add auth system" --intent-file docs/spec.md
entire trail create "Add auth system" --intent-issue LIN-123
entire trail create "Add auth system" --intent "inline description"
```

Creates the trail and auto-adds the current branch as the first branch entry.

### Branch management (new)

```bash
entire trail branch add                           # add current branch to active trail
entire trail branch add feature/auth-api          # add named branch
entire trail branch add --base feature/auth-core  # explicit base (stacking)
entire trail branch set-pr 43                     # set/replace PR on current branch
entire trail branch set-pr 43 --branch feature/x  # set PR on named branch
entire trail branch discard                       # mark current branch as discarded
```

Trail resolution uses stateless lookup (see above). If current branch isn't in a
trail, `--trail <id>` is required or an interactive picker is shown.

### Viewing (modified)

```bash
entire trail show                                 # full trail view with all branches
entire trail list                                 # list trails
```

### Verification (future)

```bash
entire trail verify request                       # request trail review
entire trail verify approve                       # approve trail review
```

## Hook Integration

### Post-commit checkpoint linking

Current flow:
1. Post-commit fires
2. Get current branch name
3. `store.FindByBranch(branchName)` -> finds trail
4. Append checkpoint to trail

New flow:
1. Post-commit fires
2. Get current branch name
3. `store.FindByBranch(branchName)` -> returns `(trail, branchEntry)`
4. Append checkpoint with `branch_id: branchEntry.ID`
5. Write with CAS retry

`FindByBranch` scans all trails' `Branches[]` fields. Returns matched
`BranchEntry` alongside the trail. Warns (logs) if branch appears in multiple
trails; uses first match.

Branches not in any trail: checkpoint still saved to shadow branch and condensed
to `entire/checkpoints/v1`. Trail linkage is optional/additive.

## Migration

No migration of existing trails. Old trails stay as-is on `entire/trails/v1`.
New trails use the new schema. Read path handles both formats (presence/absence
of `branches` field).

## Not In Scope

- Automatic branch detection (no git checkout hook available)
- Dependency graph between branches (git topology suffices)
- GitHub/Linear PR integration (future — PR number is stored, sync is separate)
- Deployment verification events (canary, rollout — structure supports it, implementation later)
- Trail review signaling UX (policy layer TBD)
