# External Agent Plugin Protocol

## Overview

The Entire CLI supports external agent plugins — standalone binaries that implement the Agent interface via a subcommand-based protocol over stdin/stdout. This allows third-party agents to integrate with the CLI without modifying the main repository.

## Discovery

The CLI discovers external agents by scanning `$PATH` for executables matching the pattern `entire-agent-<name>`. For example, `entire-agent-cursor` would register as the "cursor" agent.

- Binaries whose `<name>` conflicts with an already-registered built-in agent are skipped.
- Discovery runs once during CLI initialization (before building the hooks command tree).
- The binary must be executable and respond to the `info` subcommand.

## Environment

Every subcommand invocation sets:

| Variable | Description |
|---|---|
| `ENTIRE_REPO_ROOT` | Absolute path to the git repository root |
| `ENTIRE_PROTOCOL_VERSION` | Protocol version (`1`) |

The working directory is set to the repository root.

## Communication Model

- **Subcommand-based:** Each Agent interface method maps to a CLI subcommand.
- **JSON over stdin/stdout:** Structured data uses JSON. Transcripts use raw bytes.
- **Stateless:** Each invocation is independent — no persistent connection.
- **Exit codes:** `0` = success, non-zero = error. Error messages go to stderr.

## Subcommands

### Always Required

#### `info`

Returns agent metadata and declared capabilities.

**Arguments:** None

**Output (stdout):** JSON

```json
{
  "protocol_version": 1,
  "name": "cursor",
  "type": "Cursor",
  "description": "Cursor - AI-powered code editor",
  "is_preview": true,
  "protected_dirs": [".cursor"],
  "hook_names": ["session-start", "session-end", "stop"],
  "capabilities": {
    "hooks": true,
    "transcript_analyzer": true,
    "transcript_preparer": false,
    "token_calculator": false,
    "text_generator": false,
    "hook_response_writer": false,
    "subagent_aware_extractor": false
  }
}
```

The `capabilities` object determines which optional subcommands the CLI will call. If a capability is `false` or missing, the CLI will never invoke the corresponding subcommands.

#### `detect`

Checks whether the agent is present/usable in the current environment.

**Arguments:** None

**Output (stdout):** JSON

```json
{"present": true}
```

#### `get-session-id`

Extracts the session ID from a hook input event.

**Input (stdin):** JSON — the HookInput object

**Output (stdout):** JSON

```json
{"session_id": "abc123"}
```

#### `get-session-dir --repo-path <path>`

Returns the directory where agent sessions are stored.

**Arguments:**
- `--repo-path` — Absolute path to the repository

**Output (stdout):** JSON

```json
{"session_dir": "/path/to/sessions"}
```

#### `resolve-session-file --session-dir <dir> --session-id <id>`

Resolves the session file path from a session directory and ID.

**Arguments:**
- `--session-dir` — Session directory path
- `--session-id` — Session identifier

**Output (stdout):** JSON

```json
{"session_file": "/path/to/session/file.jsonl"}
```

#### `read-session`

Reads session data from a hook input event.

**Input (stdin):** JSON — the HookInput object

**Output (stdout):** JSON — AgentSession object

```json
{
  "session_id": "abc123",
  "session_file": "/path/to/file.jsonl"
}
```

#### `write-session`

Writes/updates session data.

**Input (stdin):** JSON — the AgentSession object

**Output:** Exit 0 on success.

#### `read-transcript --session-ref <path>`

Reads a transcript file and returns its raw bytes.

**Arguments:**
- `--session-ref` — Path to the transcript file

**Output (stdout):** Raw transcript bytes.

#### `chunk-transcript --max-size <n>`

Splits a transcript into chunks for storage.

**Input (stdin):** Raw transcript bytes.

**Arguments:**
- `--max-size` — Maximum chunk size in bytes

**Output (stdout):** JSON

```json
{"chunks": ["<base64-encoded-chunk>", "..."]}
```

#### `reassemble-transcript`

Reassembles transcript chunks back into a full transcript.

**Input (stdin):** JSON

```json
{"chunks": ["<base64-encoded-chunk>", "..."]}
```

**Output (stdout):** Raw transcript bytes.

#### `format-resume-command --session-id <id>`

Returns the command a user can run to resume a session.

**Arguments:**
- `--session-id` — Session identifier

**Output (stdout):** JSON

```json
{"command": "cursor --resume abc123"}
```

### Capability: `hooks`

These subcommands are required when `capabilities.hooks` is `true`.

#### `parse-hook --hook <name>`

Parses a raw agent hook payload into a structured event.

**Arguments:**
- `--hook` — Hook name (e.g., "stop", "session-start")

**Input (stdin):** Raw agent hook payload bytes.

**Output (stdout):** JSON — the parsed event, or `null` if the payload is not relevant.

#### `install-hooks [--local-dev] [--force]`

Installs agent hooks for Entire integration.

**Arguments:**
- `--local-dev` — Use local development binary path (optional)
- `--force` — Overwrite existing hooks (optional)

**Output (stdout):** JSON

```json
{"hooks_installed": 3}
```

#### `uninstall-hooks`

Removes installed agent hooks.

**Arguments:** None

**Output:** Exit 0 on success.

#### `are-hooks-installed`

Checks whether hooks are currently installed.

**Arguments:** None

**Output (stdout):** JSON

```json
{"installed": true}
```

### Capability: `transcript_analyzer`

Required when `capabilities.transcript_analyzer` is `true`.

#### `get-transcript-position --path <path>`

Returns the current byte position/size of a transcript file.

**Arguments:**
- `--path` — Path to the transcript file

**Output (stdout):** JSON

```json
{"position": 12345}
```

#### `extract-modified-files --path <path> --offset <n>`

Extracts the list of files modified by the agent from a transcript.

**Arguments:**
- `--path` — Path to the transcript file
- `--offset` — Byte offset to start reading from

**Output (stdout):** JSON

```json
{"files": ["path/to/file1.go", "path/to/file2.go"]}
```

#### `extract-prompts --session-ref <path> --offset <n>`

Extracts user prompts from a transcript.

**Arguments:**
- `--session-ref` — Path to the transcript file
- `--offset` — Byte offset to start reading from

**Output (stdout):** JSON

```json
{"prompts": ["first prompt text", "second prompt text"]}
```

#### `extract-summary --session-ref <path>`

Extracts an AI-generated summary from a transcript.

**Arguments:**
- `--session-ref` — Path to the transcript file

**Output (stdout):** JSON

```json
{"summary": "Summary text here", "has_summary": true}
```

### Capability: `transcript_preparer`

Required when `capabilities.transcript_preparer` is `true`.

#### `prepare-transcript --session-ref <path>`

Prepares/processes a transcript file (e.g., converting from raw format).

**Arguments:**
- `--session-ref` — Path to the transcript file

**Output:** Exit 0 on success.

### Capability: `token_calculator`

Required when `capabilities.token_calculator` is `true`.

#### `calculate-tokens --offset <n>`

Calculates token usage from a transcript.

**Input (stdin):** Raw transcript bytes.

**Arguments:**
- `--offset` — Byte offset to start calculating from

**Output (stdout):** JSON

```json
{"input_tokens": 1500, "output_tokens": 500}
```

### Capability: `text_generator`

Required when `capabilities.text_generator` is `true`.

#### `generate-text --model <model>`

Generates text using the agent's underlying LLM.

**Input (stdin):** Prompt text.

**Arguments:**
- `--model` — Model to use for generation

**Output (stdout):** JSON

```json
{"text": "Generated response text"}
```

### Capability: `hook_response_writer`

Required when `capabilities.hook_response_writer` is `true`.

#### `write-hook-response --message <message>`

Writes a message in the agent's native hook response format.

**Arguments:**
- `--message` — Message to write

**Output (stdout):** Agent-native format bytes (e.g., JSONL for Claude Code).

### Capability: `subagent_aware_extractor`

Required when `capabilities.subagent_aware_extractor` is `true`.

#### `extract-all-modified-files --offset <n> --subagents-dir <dir>`

Extracts modified files from both the main transcript and any subagent transcripts.

**Input (stdin):** Raw main transcript bytes.

**Arguments:**
- `--offset` — Byte offset for the main transcript
- `--subagents-dir` — Directory containing subagent transcripts

**Output (stdout):** JSON

```json
{"files": ["file1.go", "file2.go"]}
```

#### `calculate-total-tokens --offset <n> --subagents-dir <dir>`

Calculates total token usage across main transcript and subagent transcripts.

**Input (stdin):** Raw main transcript bytes.

**Arguments:**
- `--offset` — Byte offset for the main transcript
- `--subagents-dir` — Directory containing subagent transcripts

**Output (stdout):** JSON

```json
{"input_tokens": 5000, "output_tokens": 2000}
```

## Error Handling

- Exit code `0` indicates success.
- Any non-zero exit code indicates an error.
- Error messages should be written to stderr.
- The CLI captures stderr and wraps it in a Go error.
- If the binary is not found in PATH, the agent is simply not registered.
- If `info` fails or returns invalid JSON, the binary is skipped during discovery.

## Versioning

The protocol version is declared in the `info` response (`protocol_version` field) and set via `ENTIRE_PROTOCOL_VERSION` environment variable. The CLI checks that the binary's protocol version matches its expected version before registering the agent.

Future protocol versions may add new subcommands or capabilities. Existing subcommands will maintain backwards compatibility within a major version.
