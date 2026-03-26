# Heavyweight Memory Loop Design

## Summary

Turn the current snapshot-style memory loop into a durable learning system that supports:

- layered memory for personal and shared repo knowledge
- explicit review and promotion workflows
- recommendation history and lifecycle tracking
- outcome-aware pruning instead of snapshot overwrite

This design keeps `insights` and `improve` unchanged. The memory loop remains a separate subsystem that reads from existing session and facet data, but its local state becomes a unified memory store rather than a single active snapshot.

## Goals

- Reduce repeated developer babysitting across sessions.
- Preserve repo-specific and workflow-specific lessons over time.
- Support multi-contributor repos without letting one developer's generated memory silently affect everyone else.
- Make memory state inspectable, reviewable, suppressible, and pruneable.

## Non-Goals

- No changes to `entire insights` or `entire improve`.
- No new remote or shared backend.
- No attempt to infer perfect causality for whether a memory "worked".
- No requirement for interactive TUI review in v1.

## Current State

Today the memory loop:

- builds one snapshot from recent cached sessions
- stores active memories in `.entire/memory-loop.json`
- injects top-ranked active memories into Claude turn start
- tracks recent injection logs

This is useful as a retrieval experiment, but it has important gaps:

- no candidate review stage
- no durable recommendation history
- no manual memory entry
- no suppression or archive workflow
- no distinction between personal and shared repo memory
- no outcome-based pruning

## Design Options Considered

### Option 1: Keep a Lightweight Snapshot and Bolt On Small Controls

Pros:

- smallest code change
- minimal migration cost

Cons:

- weak model for history and pruning
- awkward fit for multi-contributor governance
- would accumulate flags without a coherent lifecycle

### Option 2: Unified Heavyweight Memory Store

Pros:

- one record model supports candidate, active, suppressed, and archived states
- history, review, pruning, and dedupe work against the same store
- simpler than separate current and history stores

Cons:

- larger schema change inside `.entire/memory-loop.json`
- more command surface

### Option 3: Separate Current Store and History Store

Pros:

- conceptually separates live state from audit trail

Cons:

- more bookkeeping and sync risk
- every lifecycle transition becomes a cross-store move
- overbuilt for a PoC

## Recommendation

Implement **Option 2: a unified heavyweight memory store**.

Each memory record remains in one local JSON file and changes lifecycle state over time. This preserves recommendation history without introducing a second storage system.

## Storage Model

Use one local file at `.entire/memory-loop.json` with a store shape like:

- `mode`: `off|manual|auto`
- `activation_policy`: `review|auto`
- `generated_at`
- `scope`
- `scope_value`
- `records`
- `injection_logs`
- `refresh_history`

Each memory record contains:

- identity: `id`, `fingerprint`
- classification: `kind`, `scope_kind`, `scope_value`, `origin`
- content: `title`, `body`, `why`, `evidence`
- provenance: `source_session_ids`, `owner_email`
- lifecycle: `status`
- scoring: `confidence`, `strength`
- activity: `inject_count`, `match_count`, `last_injected_at`, `last_matched_at`
- review: `created_at`, `updated_at`, `last_reviewed_at`
- outcome: `outcome`
- audit trail: `history`

### Unified File Model

The same record moves through statuses instead of being copied between separate stores:

- `candidate`
- `active`
- `suppressed`
- `archived`

This means:

- `show` can group by status without merging multiple stores
- `refresh` can dedupe against accepted, suppressed, and archived history
- `prune` changes statuses instead of moving records around

## Lifecycle

### Statuses

- `candidate`
  Generated and pending review. Never injects.
- `active`
  Eligible for ranking and injection.
- `suppressed`
  Explicitly rejected. Never injects and strongly resists regeneration.
- `archived`
  Historical only. Not active, but kept for history and dedupe.

### State Transitions

- generated memory becomes `candidate` or `active` depending on activation policy and scope rules
- `activate` promotes a `candidate` to `active`
- `suppress` moves a `candidate` or `active` memory to `suppressed`
- `unsuppress` moves a `suppressed` memory back to `candidate`
- `archive` retires a memory from active circulation
- `prune` archives or demotes stale/ineffective records

## Personal and Repo Layers

The system should support layered memory:

- personal memory
- shared repo memory

Each record carries scope:

- `scope_kind = me|repo`
- `scope_value`

Branch scope remains useful as a refresh filter, but the long-term retrieval model should be personal plus repo layers.

### Defaults

- manual add defaults to personal scope
- generated personal memories follow normal activation policy
- repo-scoped generated memories never become shared-active automatically
- repo-scoped manual memories can be active immediately because they are already explicit user intent

## Governance for Shared Repo Memory

For multi-contributor repos, shared repo memory uses explicit promotion.

### Approved Governance Model

- repo-scoped generated memories remain `candidate`
- they do not inject until explicitly promoted
- only shared repo `active` records can affect everyone

This avoids one developer's noisy generated memory silently steering other contributors' prompts.

## Modes and Policies

### Injection Mode

- `off`
  Memory loop is inert for injection.
- `manual`
  No automatic injection; preview commands still work.
- `auto`
  Relevant active memories inject at Claude turn start.

### Activation Policy

- `review`
  Generated memories remain pending review.
- `auto`
  Eligible generated memories can become active automatically.

These are separate controls:

- mode answers "should active memories inject?"
- activation policy answers "what happens to newly generated memories?"

## Refresh Flow

`entire memory-loop refresh` should:

1. print step-based progress
2. refresh the insights cache if needed
3. backfill summaries and facets if needed
4. load scoped sessions
5. generate new memory candidates
6. dedupe them against existing history using fingerprints
7. apply activation policy and scope rules
8. save the updated memory store
9. print a summary of active and pending candidates

### Progress Output

Use explicit progress steps, not a spinner:

- `Refreshing cache...`
- `Backfilling summaries...`
- `Backfilling facets...`
- `Loading scoped sessions...`
- `Distilling memories...`
- `Reconciling with existing memory history...`
- `Saving memory store...`

## Command Surface

The heavyweight command surface should include:

- `entire memory-loop mode off|manual|auto`
- `entire memory-loop policy review|auto`
- `entire memory-loop refresh [--last N] [--scope ...] [--review] [--auto-activate]`
- `entire memory-loop show`
- `entire memory-loop status [--prompt "..."] [--verbose]`
- `entire memory-loop add --kind ... --title ... --body ... [--scope me|repo]`
- `entire memory-loop activate <id>`
- `entire memory-loop promote <id>`
- `entire memory-loop suppress <id>`
- `entire memory-loop unsuppress <id>`
- `entire memory-loop archive <id>`
- `entire memory-loop prune`

### Command Semantics

- `activate`
  Promotes a personal or local candidate to active.
- `promote`
  Explicitly promotes a repo candidate into shared repo-active state.
- `suppress`
  Rejects a memory and discourages regeneration.
- `unsuppress`
  Returns a suppressed memory to candidate state.
- `archive`
  Retires a memory while preserving history.

## Retrieval

Retrieval should only consider `active` records.

Default retrieval composition:

- personal active records
- shared repo active records

Personal records should outrank repo records when scores are otherwise similar.

### Manual Visibility

`status --prompt` should preview:

- ranked memory matches
- score
- reason
- whether a memory came from the personal or repo layer

`--verbose` should reveal score components and matched evidence.

## Outcome Tracking

The memory loop does not need perfect causality, but it should track enough to support pruning.

### Record Activity Fields

- `inject_count`
- `match_count`
- `last_injected_at`
- `last_matched_at`

### Injection Logs

Each log entry stores:

- session id
- prompt preview
- injected memory ids
- timestamp
- reason

### Derived Outcome

Use a simple derived field:

- `neutral`
- `reinforced`
- `ineffective`

This should be updated during refresh using later sessions and their facets:

- repeated supporting evidence reinforces a memory
- recurring matching friction after repeated injection marks it ineffective

## Pruning Policy

Recommended defaults:

- archive stale candidates after 30 days
- archive active memories with no matches after 60 days
- demote or archive active memories marked ineffective after repeated injections
- retain suppressed memories longer than others to preserve rejection history
- never auto-prune manual memories unless explicitly archived

Suppression history should remain effective even if a record is archived later. Fingerprints should continue to block easy regeneration of the same rejected idea.

## Multi-Contributor Behavior

In shared repos:

- personal memories are private to the developer's local store
- shared repo memories are explicit shared knowledge
- generated repo candidates do not become shared-active automatically

This allows:

- local experimentation with personal memory
- shared institutional memory for real repo rules
- safer governance for team environments

## Manual Verification

This PoC does not prioritize automated tests. Verification should focus on manual end-to-end checks:

1. refresh with personal and repo scopes
2. inspect pending and active records
3. manually add, activate, suppress, promote, archive, and prune
4. preview retrieval with `status --prompt`
5. confirm only active records inject in `auto` mode
6. confirm repo candidates do not inject before promotion

## Open Follow-Ups

- whether to eventually sync shared repo-active memory across collaborators rather than keeping it purely local
- whether repo-level promotion should later require stronger governance
- whether a future migration from JSON to SQLite is warranted after the workflow proves itself
