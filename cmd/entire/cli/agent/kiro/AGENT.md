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

## Hook Events (5 total)

| Kiro Hook (camelCase) | CLI Subcommand (kebab-case) | EventType | Notes |
|----------------------|----------------------------|-----------|-------|
| `agentSpawn` | `agent-spawn` | `SessionStart` | Agent initializes |
| `userPromptSubmit` | `user-prompt-submit` | `TurnStart` | stdin includes `prompt` field |
| `preToolUse` | `pre-tool-use` | `nil, nil` | Pass-through |
| `postToolUse` | `post-tool-use` | `nil, nil` | Pass-through |
| `stop` | `stop` | `TurnEnd` | Checkpoint trigger |

No `SessionEnd` hook exists — sessions end implicitly (similar to Cursor).

## Hook Configuration

**File:** `.kiro/agents/entire.json`

We own the entire file — no round-trip preservation needed (unlike Cursor's shared `hooks.json`).

**Format:**
```json
{
  "agentSpawn": [{"command": "entire hooks kiro agent-spawn"}],
  "userPromptSubmit": [{"command": "entire hooks kiro user-prompt-submit"}],
  "preToolUse": [{"command": "entire hooks kiro pre-tool-use"}],
  "postToolUse": [{"command": "entire hooks kiro post-tool-use"}],
  "stop": [{"command": "entire hooks kiro stop"}]
}
```

## Hook Stdin Format

All hooks receive the same JSON structure on stdin:
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
