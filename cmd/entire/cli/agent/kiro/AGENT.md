# Kiro Agent Integration

Implementation one-pager for the Kiro (Amazon AI coding CLI) agent integration.

## Identity

| Field | Value |
|-------|-------|
| Package | `kiro` |
| Registry Key | `kiro` |
| Agent Type | `Kiro` |
| Binary | `kiro-cli` |
| Preview | Yes |
| Protected Dir | `.kiro` |

## Hook Events (5 CLI + 4 IDE)

| Kiro Hook (camelCase) | CLI Subcommand (kebab-case) | EventType | Notes |
|----------------------|----------------------------|-----------|-------|
| `agentSpawn` | `agent-spawn` | `SessionStart` | Agent initializes |
| `userPromptSubmit` | `user-prompt-submit` | `TurnStart` | stdin includes `prompt` field |
| `preToolUse` | `pre-tool-use` | `nil, nil` | Pass-through |
| `postToolUse` | `post-tool-use` | `nil, nil` | Pass-through |
| `stop` | `stop` | `TurnEnd` | Checkpoint trigger |

No `SessionEnd` hook exists — sessions end implicitly (similar to Cursor).

## Hook Configuration

### CLI Agent Hooks

**File:** `.kiro/agents/entire.json`

We own the entire file — no round-trip preservation needed (unlike Cursor's shared `hooks.json`).

**Format:**
```json
{
  "name": "entire",
  "tools": ["read", "write", "shell", "grep", "glob", "aws", "report",
            "introspect", "knowledge", "thinking", "todo", "delegate"],
  "hooks": {
    "agentSpawn": [{"command": "entire hooks kiro agent-spawn"}],
    "userPromptSubmit": [{"command": "entire hooks kiro user-prompt-submit"}],
    "preToolUse": [{"command": "entire hooks kiro pre-tool-use"}],
    "postToolUse": [{"command": "entire hooks kiro post-tool-use"}],
    "stop": [{"command": "entire hooks kiro stop"}]
  }
}
```

Note: The file is a Kiro agent definition. Hooks must be nested under the `hooks` field.
Required top-level fields: `name`. Optional: `$schema`, `description`, `prompt`, `mcpServers`,
`tools`, `toolAliases`, `allowedTools`, `resources`, `hooks`, `toolsSettings`, `model`, etc.

**Important:** The `tools` array must include all default Kiro tools. Without it, `--agent entire`
restricts the model to zero tools. The tool names come from `~/.kiro/agents/agent_config.json.example`.

### IDE Hook Configuration

**Directory:** `.kiro/hooks/`

The Kiro IDE (VS Code extension) reads hooks from individual `.kiro/hooks/*.kiro.hook` files.
Unlike CLI hooks (nested in `entire.json`), each IDE hook is a standalone JSON file.

**IDE hook files installed (4 total):**

| File | `when.type` | Command |
|------|-------------|---------|
| `entire-prompt-submit.kiro.hook` | `promptSubmit` | `entire hooks kiro user-prompt-submit` |
| `entire-stop.kiro.hook` | `agentStop` | `entire hooks kiro stop` |
| `entire-pre-tool-use.kiro.hook` | `preToolUse` | `entire hooks kiro pre-tool-use` |
| `entire-post-tool-use.kiro.hook` | `postToolUse` | `entire hooks kiro post-tool-use` |

No `agentSpawn` IDE hook — the IDE has no such trigger. The first `promptSubmit` serves as session start.

**Format:**
```json
{
  "enabled": true,
  "name": "entire-prompt-submit",
  "description": "Entire CLI promptSubmit hook",
  "version": "1",
  "when": {
    "type": "promptSubmit"
  },
  "then": {
    "type": "runCommand",
    "command": "entire hooks kiro user-prompt-submit"
  }
}
```

**Key difference from CLI hooks:** IDE hooks deliver data via environment variables (e.g.,
`USER_PROMPT` for the user's prompt) rather than JSON on stdin. The lifecycle parsers handle
empty stdin gracefully by reading from environment variables as a fallback.

## Agent Activation

Hooks only fire when `--agent entire` is passed to `kiro-cli chat`. Without this flag,
`.kiro/agents/entire.json` is not loaded and hooks do not execute.

IDE hooks in `.kiro/hooks/` are loaded automatically by the Kiro IDE without requiring
an explicit agent flag.

**`--no-interactive` mode:** Does not fire agent hooks. All E2E tests use interactive (tmux) mode.

**TUI prompt indicator:** `!>` in trust-all mode (with `-a` flag). The `Credits:` line
appears after each agent response and serves as a reliable completion marker.

## Hook Data Delivery

### CLI Mode (stdin JSON)

CLI hooks (`kiro-cli chat --agent entire`) receive a JSON structure on stdin:
```json
{
  "hook_event_name": "userPromptSubmit",
  "cwd": "/path/to/repo",
  "prompt": "user message",
  "tool_name": "fs_write",
  "tool_input": "...",
  "tool_response": "..."
}
```

Fields are populated based on the hook event — `prompt` only for `userPromptSubmit`, tool fields only for tool hooks.

### IDE Mode (environment variables)

IDE hooks (`.kiro/hooks/*.kiro.hook`) receive **no stdin**. Data is delivered via environment variables:
- `USER_PROMPT` — the user's prompt text (for `promptSubmit` hooks)

The lifecycle parsers handle empty stdin gracefully: if stdin is empty, they fall back to reading
environment variables for prompt data and use `paths.WorktreeRoot()` for CWD.

## Transcript Storage

**Source:** SQLite database at `~/Library/Application Support/kiro-cli/data.sqlite3` (macOS)
or `~/.local/share/kiro-cli/data.sqlite3` (Linux).

**Table:** `conversations_v2`
- `key` column: CWD path (used for lookup)
- `value` column: JSON blob with conversation data
- `updated_at` column: timestamp for ordering

**Conversation JSON structure:**
```json
{
  "conversation_id": "uuid-v4",
  "history": [
    {"role": "user", "content": [{"type": "text", "text": "..."}]},
    {"role": "assistant", "content": [{"type": "text", "text": "..."}, {"type": "tool_use", "name": "fs_write", "input": {...}}]},
    {"role": "request_metadata", "input_tokens": 150, "output_tokens": 80}
  ]
}
```

## Session ID Discovery

Hook stdin does not include session ID. We query SQLite by CWD:
```sql
SELECT json_extract(value, '$.conversation_id')
FROM conversations_v2
WHERE key = '<cwd>'
ORDER BY updated_at DESC LIMIT 1
```

## SQLite Access Strategy

Uses `sqlite3` CLI (pre-installed on macOS/Linux) rather than a Go library to avoid CGO dependencies. Same approach as OpenCode's `opencode export` CLI.

**Test mock:** `ENTIRE_TEST_KIRO_MOCK_DB=1` env var causes the agent to skip SQLite queries and use pre-written mock files.

## File Modification Tools

Tools that modify files on disk:
- `fs_write` — write file content
- `str_replace` — string replacement in files
- `create_file` — create new files
- `write_file` — write/overwrite files
- `edit_file` — edit existing files

## Caching

Transcript data is cached to `.entire/tmp/<sessionID>.json` (same pattern as OpenCode).
