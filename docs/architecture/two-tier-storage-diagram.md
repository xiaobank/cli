# Two-Tier Storage Model

## Overview

Entire uses a two-tier storage model to balance mid-session rewind capability with clean commit history.

## The Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  WHILE YOU WORK                                                             │
│  ══════════════                                                             │
│                                                                             │
│  Your branch:     A ─── B ─── C (HEAD)      ◄── no checkpoint commits here  │
│                                                                             │
│                                                                             │
│  Shadow branch:   entire/c4f8a2b-d3e1a9     ◄── out-of-band scratch space   │
│                   ┌─────────────────────┐                                   │
│                   │ checkpoint 1        │ ◄── rewindable                    │
│                   │ checkpoint 2        │ ◄── rewindable                    │
│                   │ checkpoint 3        │ ◄── rewindable                    │
│                   └─────────────────────┘                                   │
│                   (full code snapshots + transcripts)                       │
│                                                                             │
│  • Rewind anytime if agent goes sideways                                    │
│  • Never appears in git log or PRs                                          │
│  • Purely scratch space for the session                                     │
└─────────────────────────────────────────────────────────────────────────────┘

                                    │
                                    │  git commit
                                    ▼

┌─────────────────────────────────────────────────────────────────────────────┐
│  ONCE YOU COMMIT                                                            │
│  ══════════════                                                             │
│                                                                             │
│  Your branch:     A ─── B ─── C ─── D (HEAD)                                │
│                                     │                                       │
│                                     └── Entire-Checkpoint: a3b2c4d5e6f7     │
│                                         (trailer links to permanent record) │
│                                                                             │
│                                                                             │
│  Shadow branch:   (deleted)                                                 │
│                                                                             │
│                                                                             │
│  Checkpoints branch: entire/checkpoints/v1                                  │
│                      ┌─────────────────────┐                                │
│                      │ a3/b2c4d5e6f7/      │                                │
│                      │ ├── metadata.json   │                                │
│                      │ ├── full.jsonl      │ ◄── transcript                 │
│                      │ ├── prompt.txt      │ ◄── user prompts               │
│                      │ └── (token usage,   │                                │
│                      │      attribution,   │                                │
│                      │      summary...)    │                                │
│                      └─────────────────────┘                                │
│                                                                             │
│  • Temporary state condensed into permanent record                          │
│  • Shadow branch cleaned up                                                 │
│  • Your commit links to the checkpoint via trailer                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Shadow Branch Naming

Shadow branches follow a specific naming convention to stay isolated and organized:

```
entire/<commit-hash>-<worktree-hash>
       ─────┬─────   ──────┬──────
            │              │
            │              └─ 6-char hash of worktree path
            │                 (allows concurrent worktrees)
            │
            └─ 7-char prefix of base commit
               (ties checkpoints to your HEAD)

Example: entire/c4f8a2b-d3e1a9
```

## What Gets Stored Where

| Data | Shadow Branch (pre-commit) | Checkpoints Branch (post-commit) |
|------|----------------------------|----------------------------------|
| Code snapshots | ✓ Full worktree | ✗ |
| Transcript | ✓ | ✓ |
| User prompts | ✓ | ✓ |
| Token usage | ✗ | ✓ |
| File attribution | ✗ | ✓ |
| AI-generated summary | ✗ | ✓ |

## Key Properties

**Shadow branches (while you work):**
- Enable mid-session rewind without polluting your branch
- Completely invisible to normal git workflows
- Automatically cleaned up after commit
- Named per base-commit + worktree for isolation

**Checkpoints branch (after you commit):**
- Permanent, auditable record of AI sessions
- Linked to your commits via `Entire-Checkpoint` trailer
- Can be pushed to remote for team visibility
- Sharded storage (256 directories) for scale
