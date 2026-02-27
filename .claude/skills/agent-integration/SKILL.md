---
name: agent-integration
description: >
  Run all three agent integration phases sequentially: research, write-tests,
  and implement. For individual phases, use /agent-integration:research,
  /agent-integration:write-tests, or /agent-integration:implement.
  Use when the user says "integrate agent", "add agent support", or wants
  to run the full agent integration pipeline end-to-end.
---

# Agent Integration — Full Pipeline

Run all three phases of agent integration in a single session. Parameters are collected once and reused across all phases.

## Parameters

Collect these before starting (ask the user if not provided):

| Parameter | Description | How to derive |
|-----------|-------------|---------------|
| `AGENT_NAME` | Human-readable name (e.g., "Gemini CLI") | User provides |
| `AGENT_PACKAGE` | Go package dir name — **no hyphens** | Lowercase, remove hyphens/spaces |
| `AGENT_KEY` | Registry key for `agent.Register()` and `entire enable` | Check existing patterns in `cmd/entire/cli/agent/registry.go` |
| `AGENT_BIN` | CLI binary name | `command -v <binary>` |
| `LIVE_COMMAND` | Full command to launch agent | User provides |
| `EVENTS_OR_UNKNOWN` | Known hook event names, or "unknown" | From agent docs or "unknown" |

**Note:** These identifiers can differ. Run `grep -r 'AgentName\|func.*Name()' cmd/entire/cli/agent/*/` and `e2e/agents/` to see how existing agents handle the split.

## Architecture References

These documents define the agent integration contract:

- **Implementation guide**: `docs/architecture/agent-guide.md` — Step-by-step code templates, event mapping, testing patterns
- **Integration checklist**: `docs/architecture/agent-integration-checklist.md` — Design principles and validation criteria

## Scope

This skill targets **hook-capable agents** — those that support lifecycle hooks
(implementing `HookSupport` from `agent.go`). Agents that use file-based detection
(implementing `FileWatcher`) require a different integration approach not covered here.
Check `agent.go` for the current interface definitions.

## Pipeline

Run these three phases in order. Each phase builds on the previous phase's output.

### Phase 1: Research

Discover the agent's hook mechanism, transcript format, and configuration through binary probing and documentation research. Produces an implementation one-pager at `cmd/entire/cli/agent/$AGENT_PACKAGE/AGENT.md` that the other phases use as their single source of agent-specific information.

Read and follow the research procedure from `.claude/skills/agent-integration/researcher.md`.

**Expected output:** Implementation one-pager at `cmd/entire/cli/agent/$AGENT_PACKAGE/AGENT.md` and a test script at `scripts/test-$AGENT_SLUG-agent-integration.sh`.

**Gate:** If the verdict is INCOMPATIBLE, stop and discuss with the user before proceeding.

### Phase 2: Write Tests

Generate the E2E test suite using the one-pager for agent-specific information (binary name, CLI flags, interactive mode support).

Read and follow the write-tests procedure from `.claude/skills/agent-integration/test-writer.md`.

**Expected output:** E2E agent runner at `e2e/agents/$AGENT_SLUG.go` and any agent-specific test scenarios.

### Phase 3: Implement

Build the Go agent package using E2E-driven development. Reads internal Entire docs (agent-guide, checklist, interfaces) and uses the one-pager for all agent-specific details (hook format, transcript location, config structure).

Read and follow the implement procedure from `.claude/skills/agent-integration/implementer.md`.

**Expected output:** Complete agent package at `cmd/entire/cli/agent/$AGENT_PACKAGE/` with all tests passing.

**Note:** `AGENT.md` is a living document — Phases 2 and 3 update it when they discover new information during testing or implementation.

## Final Validation

After all three phases, run the complete validation:

```bash
mise run fmt      # Format
mise run lint     # Lint
mise run test:ci  # All tests (unit + integration)
```

Summarize:
- Compatibility verdict from Phase 1
- Files created in Phases 2 and 3
- Test coverage
- Any remaining TODOs or gaps
