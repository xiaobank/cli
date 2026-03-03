# Plugin Architecture Research

Research into making the Entire CLI more modular and plugin-oriented, with two goals:

1. **External agents** can be used without merging into the core repo (and forever maintaining them)
2. **Internal commands** that aren't released publicly can be baked into internal CLI builds easily

## Current Architecture

### How Agents Work Today

Agents use a **factory-based registry with `init()` self-registration**:

```
cmd/entire/cli/agent/
├── agent.go              # Agent interface (19 methods) + 7 optional interfaces
├── registry.go           # Factory registry: Register(), Get(), List(), Detect()
├── types/agent.go        # AgentName, AgentType type aliases
├── claudecode/            # Each agent is a subpackage
│   ├── claude.go         #   init() { agent.Register("claude-code", NewClaudeCodeAgent) }
│   ├── hooks.go
│   ├── lifecycle.go
│   └── transcript.go
├── cursor/
├── geminicli/
├── opencode/
└── factoryaidroid/
```

**Registration flow:**
1. Each agent package has `init()` calling `agent.Register(name, factory)`
2. `hooks_cmd.go` has blank imports (`_ "...agent/claudecode"`) to trigger `init()`
3. At runtime, `agent.List()` / `agent.Get()` / `agent.Detect()` query the registry
4. `newHooksCmd()` dynamically creates subcommands from registered agents

**Architecture enforcement:**
- `architecture_test.go` verifies each agent calls `Register()` in `init()`
- Agents are forbidden from importing `strategy/`, `checkpoint/`, `session/`, or `cli/`
- Agents are "passive data providers" — framework calls them, never vice versa

### How Commands Work Today

Commands are hardcoded in `root.go`:

```go
func NewRootCmd() *cobra.Command {
    cmd.AddCommand(newRewindCmd())
    cmd.AddCommand(newResumeCmd())
    cmd.AddCommand(newCleanCmd())
    // ... 10+ commands explicitly wired
}
```

### Build System

- **goreleaser** for cross-platform builds with `CGO_ENABLED=0`
- **ldflags** inject version, commit hash, telemetry keys
- Build tags already used: `integration`, `e2e`, `unix`
- No existing plugin or conditional compilation mechanism

---

## Analysis: What's Already Good

The agent architecture is **already 80% of the way to a plugin system**:

- Clean `Agent` interface with optional interfaces via type assertions
- Factory registry pattern with name-based lookup
- Self-registration via `init()` — no central "list of all agents"
- Architecture tests enforce decoupling — agents can't import framework internals
- Dynamic hook subcommand generation from registered agents

The main coupling points that prevent external agents today:
1. Agent packages must live inside the `cmd/entire/cli/agent/` directory
2. Blank imports in `hooks_cmd.go` are the only way to activate an agent
3. `AgentName` constants are defined in `registry.go` — adding a new agent requires modifying core code
4. The binary is statically compiled — no runtime loading

---

## Option 1: External Agent Executables (kubectl/gh-style)

**Pattern:** Agents are standalone executables discovered on `$PATH`.

### How It Would Work

```
# External agent is a standalone binary
$ entire-agent-aider --capabilities    # Reports what it implements
$ entire-agent-aider --parse-hook ...  # Translates hook into Event JSON
$ entire-agent-aider --detect          # Reports if present in repo

# Discovery
$ ls ~/.entire/agents/                 # Or scan PATH for entire-agent-* prefix
entire-agent-aider
entire-agent-windsurf
```

**Protocol:** JSON over stdin/stdout (similar to how hooks already work):

```json
// entire-agent-aider --capabilities
{
  "name": "aider",
  "type": "Aider",
  "description": "Aider AI pair programmer",
  "is_preview": true,
  "protected_dirs": [".aider"],
  "supports": ["hook_support", "transcript_analyzer", "file_watcher"],
  "hook_names": ["session-start", "session-end"]
}

// entire-agent-aider --parse-hook session-start < stdin_data
{
  "type": "session_start",
  "session_id": "abc123",
  "session_ref": "/path/to/transcript",
  "timestamp": "2026-03-03T12:00:00Z"
}
```

### Pros
- **Zero coupling** — agents developed in any language, any repo
- **Independent release cycles** — agent updates don't require CLI release
- **Battle-tested pattern** — kubectl, gh, git all use this successfully
- **No CGO needed** — stays `CGO_ENABLED=0`
- **Community friendly** — anyone can write an agent

### Cons
- **Performance overhead** — process spawn per hook invocation (10-50ms each)
- **Protocol versioning** — must maintain backward-compatible JSON schema
- **Error handling complexity** — stderr parsing, exit codes, timeouts
- **Distribution burden** — each agent needs its own install/update mechanism
- **Testing complexity** — integration tests need the agent binary available
- **Loss of type safety** — JSON serialization/deserialization vs. Go interfaces

### Implementation Effort
**Medium-High.** Requires:
- Defining a stable JSON protocol for all 7+ interfaces
- Writing a `ProcessAgent` adapter that implements `agent.Agent` by shelling out
- Agent discovery logic (PATH scan, `~/.entire/agents/` directory)
- Protocol version negotiation
- Good error messages when agent binary is missing/broken

---

## Option 2: Private Wrapper Repo (Internal Commands & Agents)

**Pattern:** A private repo imports the public CLI as a Go library, adds its own commands and agents, and builds a custom binary. The public repo code is never modified.

### Why Not Build Tags in a Public Repo?

Build tags (`//go:build internal`) in a public repo don't actually protect anything — anyone can run `go build -tags=internal` and get the internal commands. The internal code source is also visible to anyone browsing the repo. For truly private features, the code must live in a private repository.

### How It Would Work

**The public repo (this repo) exposes extension points:**

```go
// root.go — add a single exported function
func NewRootCmdWithExtensions(extensions ...Extension) *cobra.Command {
    cmd := NewRootCmd()
    for _, ext := range extensions {
        for _, sub := range ext.Commands {
            cmd.AddCommand(sub)
        }
    }
    return cmd
}

// extension.go — minimal extension type
type Extension struct {
    Commands []*cobra.Command
}
```

**The private repo wraps the public CLI:**

```
github.com/entireio/cli-internal/   # PRIVATE repo
├── go.mod                           # requires github.com/entireio/cli
├── cmd/entire/main.go               # Custom entrypoint
├── agent/windsurf/                   # Private agent implementations
│   └── windsurf.go
└── commands/
    ├── billing.go                   # Internal-only commands
    └── admin.go
```

```go
// github.com/entireio/cli-internal/go.mod
module github.com/entireio/cli-internal

require github.com/entireio/cli v0.15.0

go 1.26.0
```

```go
// github.com/entireio/cli-internal/cmd/entire/main.go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/entireio/cli/cmd/entire/cli"
    "github.com/entireio/cli/cmd/entire/cli/agent"

    // Private agents register via init()
    _ "github.com/entireio/cli-internal/agent/windsurf"

    // Private commands
    "github.com/entireio/cli-internal/commands"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    go func() { <-sigChan; cancel() }()

    rootCmd := cli.NewRootCmdWithExtensions(
        commands.BillingExtension(),
        commands.AdminExtension(),
    )
    if err := rootCmd.ExecuteContext(ctx); err != nil {
        os.Exit(1)
    }
}
```

```go
// github.com/entireio/cli-internal/agent/windsurf/windsurf.go
package windsurf

import "github.com/entireio/cli/cmd/entire/cli/agent"

func init() {
    agent.Register("windsurf", NewWindsurfAgent)
}

// ... implements agent.Agent interface ...
```

```go
// github.com/entireio/cli-internal/commands/billing.go
package commands

import (
    "github.com/entireio/cli/cmd/entire/cli"
    "github.com/spf13/cobra"
)

func BillingExtension() cli.Extension {
    return cli.Extension{
        Commands: []*cobra.Command{newBillingCmd()},
    }
}

func newBillingCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "billing",
        Short: "Manage billing (internal)",
        RunE:  func(cmd *cobra.Command, args []string) error { ... },
    }
}
```

### What Changes in This Repo

**Minimal.** The existing architecture already supports most of this:

1. **Agents**: Already work via `init()` + `agent.Register()`. An external package blank-importing a private agent package will register it. The `hooks_cmd.go` already iterates `agent.List()` dynamically. **No changes needed for agents.**

2. **Commands**: Need a small extension point. Two options:

   **Option A — `NewRootCmdWithExtensions` (cleanest):**
   Add ~15 lines to `root.go`. The private repo calls this instead of `NewRootCmd()`.

   **Option B — Post-construction `AddCommand` (zero changes):**
   The private repo's `main.go` calls `cli.NewRootCmd()` then does `rootCmd.AddCommand(...)` directly. Works today with zero public repo changes, but the private repo has to duplicate the signal handling and error printing from `main.go`.

### Seam Analysis: What the Private Repo Can Import Today

| Package | Importable? | Notes |
|---|---|---|
| `cmd/entire/cli` | Yes | `NewRootCmd()` is exported |
| `cmd/entire/cli/agent` | Yes | `Register()`, `Get()`, `Agent` interface all exported |
| `cmd/entire/cli/agent/types` | Yes | `AgentName`, `AgentType` are exported types |
| `cmd/entire/cli/settings` | Yes | `Load()`, `EntireSettings` exported |
| `cmd/entire/cli/strategy` | Yes | `ManualCommitStrategy` exported |
| `cmd/entire/cli/versioninfo` | Yes | `Version`, `Commit` variables for ldflags |

The private repo gets full access to the framework. Its agents are first-class citizens — indistinguishable from built-in agents at runtime.

### Private Repo Build & Release

The private repo has its own `.goreleaser.yaml`:

```yaml
builds:
  - main: ./cmd/entire
    binary: entire
    env: [CGO_ENABLED=0]
    ldflags:
      # Same ldflags as public, can add more
      - -X github.com/entireio/cli/cmd/entire/cli/versioninfo.Version={{.Version}}
      - -X github.com/entireio/cli/cmd/entire/cli/versioninfo.Commit={{.ShortCommit}}
```

Upgrading the public CLI: just bump the version in `go.mod`:
```bash
cd cli-internal
go get github.com/entireio/cli@v0.16.0
go mod tidy
# Run tests to verify compatibility
```

### Pros
- **True privacy** — internal code never touches the public repo
- **Zero runtime overhead** — single static binary, identical to today
- **Full type safety** — compile-time verification, IDE support, Go tooling
- **Minimal public repo changes** — agents need zero changes; commands need ~15 lines
- **Independent release cadence** — private repo can release on its own schedule
- **Standard Go patterns** — just modules, imports, and interfaces. Nothing exotic
- **Testable** — private repo runs its own test suite against the public CLI interfaces
- **No protocol design** — reuses Go interfaces directly

### Cons
- **Must be Go** — private commands/agents must be written in Go
- **Version coupling** — private repo must compile against a specific public CLI version
- **Requires rebuilding** — can't hot-add features to an existing binary
- **API stability pressure** — changing exported types in the public repo can break the private repo
- **Two repos to maintain** — release process is slightly more complex

### Implementation Effort
**Very Low.** For agents: zero changes (already works). For commands: add `Extension` type + `NewRootCmdWithExtensions()` (~15 lines to `root.go`). Or: zero changes if using Option B (post-construction AddCommand).

### Compile-Time Agents from Other External Repos

The same pattern works for anyone, not just internal teams:

```go
// In github.com/someone/entire-agent-aider (any external repo):
package aider

import "github.com/entireio/cli/cmd/entire/cli/agent"

func init() {
    agent.Register("aider", NewAiderAgent)
}

type AiderAgent struct{}
func (a *AiderAgent) Name() types.AgentName { return "aider" }
// ... implement Agent interface ...
```

Users wanting this agent create a one-file `main.go` that blank-imports it alongside the standard CLI. This "custom build" pattern is well-established in the Go ecosystem (e.g., Caddy's xcaddy, CoreDNS's plugin system).

---

## Option 3: HashiCorp-Style gRPC Plugins

**Pattern:** Agents are separate processes communicating via gRPC, managed by `hashicorp/go-plugin`.

### How It Would Work

```protobuf
// agent.proto
service AgentPlugin {
  rpc GetCapabilities(Empty) returns (Capabilities);
  rpc DetectPresence(DetectRequest) returns (DetectResponse);
  rpc ParseHookEvent(HookEventRequest) returns (Event);
  rpc ReadTranscript(ReadRequest) returns (TranscriptData);
  rpc ExtractModifiedFiles(ExtractRequest) returns (FileList);
  // ...
}
```

```go
// Plugin side (agent binary)
func main() {
    plugin.Serve(&plugin.ServeConfig{
        HandshakeConfig: handshake,
        Plugins: map[string]plugin.Plugin{
            "agent": &AgentGRPCPlugin{Impl: &AiderAgent{}},
        },
        GRPCServer: plugin.DefaultGRPCServer,
    })
}
```

```go
// Host side (CLI)
client := plugin.NewClient(&plugin.ClientConfig{
    HandshakeConfig: handshake,
    Plugins:         pluginMap,
    Cmd:             exec.Command("entire-agent-aider"),
    AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
})
defer client.Kill()

rpcClient, _ := client.Client()
raw, _ := rpcClient.Dispense("agent")
agent := raw.(Agent)
```

### Pros
- **Process isolation** — plugin crash doesn't take down CLI
- **Language agnostic** — any language with gRPC support
- **Versioned protocol** — protobuf handles schema evolution well
- **Connection reuse** — single process for multiple calls per session
- **Battle-tested** — Terraform has thousands of providers using this
- **Health checking** — built-in keepalive and restart

### Cons
- **Heavy dependency** — pulls in gRPC, protobuf, `hashicorp/go-plugin`
- **Complexity** — significant boilerplate for plugin authors
- **Startup latency** — gRPC handshake adds ~100-200ms per plugin
- **Overkill for this use case** — agents are called infrequently (hook events)
- **CGO concerns** — some gRPC features may want CGO (though pure-Go works)
- **Developer friction** — writing a Terraform provider is notoriously painful

### Implementation Effort
**High.** Requires:
- Protobuf schema design for all agent interfaces
- gRPC server/client boilerplate
- Plugin discovery and lifecycle management
- SDK package for plugin authors
- Significant documentation

---

## Option 4: Hybrid Approach (Recommended)

Combine **private wrapper repo for internal commands** with **executable-based external agents**, leveraging the existing architecture.

### Design

```
┌───────────────────────────────────────────────────────────────┐
│                                                               │
│  github.com/entireio/cli (PUBLIC)                             │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  Agent Registry  (init() + Register() pattern)          │  │
│  │  ┌──────────┐ ┌────────┐ ┌────────┐ ┌─────────┐        │  │
│  │  │Claude    │ │Cursor  │ │Gemini  │ │OpenCode │        │  │
│  │  │Code      │ │        │ │        │ │         │        │  │
│  │  └──────────┘ └────────┘ └────────┘ └─────────┘        │  │
│  │  ┌────────────────────────┐                             │  │
│  │  │ ExternalAgentAdapter   │── JSON ──► entire-agent-*   │  │
│  │  └────────────────────────┘  protocol   (any language)  │  │
│  ├─────────────────────────────────────────────────────────┤  │
│  │  Commands: rewind, resume, enable, disable, ...         │  │
│  ├─────────────────────────────────────────────────────────┤  │
│  │  Extension point:                                       │  │
│  │  NewRootCmdWithExtensions(...Extension) *cobra.Command  │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                               │
└───────────────────────────┬───────────────────────────────────┘
                            │ go module import
┌───────────────────────────▼───────────────────────────────────┐
│                                                               │
│  github.com/entireio/cli-internal (PRIVATE)                   │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  cmd/entire/main.go  (custom entrypoint)                │  │
│  │    imports cli.NewRootCmdWithExtensions(...)             │  │
│  │    blank-imports private agent packages                  │  │
│  ├─────────────────────────────────────────────────────────┤  │
│  │  Private agents:  windsurf/, internal-tool/             │  │
│  │  Private commands: billing, admin, debug-internal       │  │
│  │  Own .goreleaser.yaml, own CI, own release cadence      │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

### Part A: Private Wrapper Repo for Internal Commands

**Minimal public repo changes, true code privacy.**

The public repo adds a small extension point (~15 lines):

```go
// root.go — extension type and builder
type Extension struct {
    Commands []*cobra.Command
}

func NewRootCmdWithExtensions(extensions ...Extension) *cobra.Command {
    cmd := NewRootCmd()
    for _, ext := range extensions {
        for _, sub := range ext.Commands {
            cmd.AddCommand(sub)
        }
    }
    return cmd
}
```

The private repo has its own `main.go` that uses this:

```go
// github.com/entireio/cli-internal/cmd/entire/main.go
package main

import (
    "github.com/entireio/cli/cmd/entire/cli"
    _ "github.com/entireio/cli-internal/agent/windsurf"  // Private agent
    "github.com/entireio/cli-internal/commands"           // Private commands
)

func main() {
    rootCmd := cli.NewRootCmdWithExtensions(
        commands.BillingExtension(),
        commands.AdminExtension(),
    )
    rootCmd.ExecuteContext(ctx)
}
```

**Key properties:**
- Internal code is *never* in the public repo — not even behind build tags
- Private agents use the same `init()` + `Register()` pattern as built-in agents
- Upgrading the public CLI = `go get github.com/entireio/cli@v0.16.0`
- Private repo has its own goreleaser, CI, and release process
- The produced binary is indistinguishable from the public one (single static binary)

### Part B: External Agent Protocol

**Moderate changes, enables community agents in any language.**

1. Define a JSON-over-stdin/stdout protocol covering the `Agent` interface
2. Create an `ExternalAgent` adapter struct that implements `Agent` by executing the external binary
3. Discovery: scan `~/.entire/agents/` and `$PATH` for `entire-agent-*` binaries
4. External agents register themselves into the existing registry at startup
5. Keep the process alive for the duration of a hook invocation (not persistent daemon)

**Protocol design (minimal viable):**

An external agent binary must support these subcommands:

| Subcommand | Maps to | Required |
|---|---|---|
| `capabilities` | Identity methods | Yes |
| `detect` | `DetectPresence()` | Yes |
| `parse-hook <name>` | `ParseHookEvent()` (stdin: hook data, stdout: Event JSON) | If HookSupport |
| `install-hooks` | `InstallHooks()` | If HookSupport |
| `uninstall-hooks` | `UninstallHooks()` | If HookSupport |
| `read-transcript <ref>` | `ReadTranscript()` | Yes |
| `extract-files <ref> --offset N` | `ExtractModifiedFilesFromOffset()` | Optional |
| `extract-prompts <ref> --offset N` | `ExtractPrompts()` | Optional |

**Capabilities response declares what's supported:**

```json
{
  "protocol_version": 1,
  "name": "aider",
  "type": "Aider",
  "description": "Aider AI pair programmer",
  "is_preview": true,
  "protected_dirs": [".aider"],
  "interfaces": {
    "hook_support": {
      "hook_names": ["session-start", "session-end"]
    },
    "transcript_analyzer": true,
    "file_watcher": {
      "watch_paths": [".aider/sessions/*.json"]
    }
  }
}
```

**The adapter:**

```go
// cmd/entire/cli/agent/external/adapter.go
type ExternalAgent struct {
    BinaryPath   string
    caps         *Capabilities  // cached from capabilities call
}

func (e *ExternalAgent) Name() types.AgentName { return types.AgentName(e.caps.Name) }
func (e *ExternalAgent) Type() types.AgentType  { return types.AgentType(e.caps.Type) }
func (e *ExternalAgent) DetectPresence(ctx context.Context) (bool, error) {
    out, err := e.exec(ctx, "detect")
    // parse JSON bool response
}
func (e *ExternalAgent) ParseHookEvent(ctx context.Context, hookName string, stdin io.Reader) (*Event, error) {
    out, err := e.execWithStdin(ctx, stdin, "parse-hook", hookName)
    // parse JSON Event response
}
// ... etc
```

### Part C: Agent Interface SDK (Future)

For compile-time external agents, extract the agent interfaces into a separate Go module:

```
github.com/entireio/agent-sdk/     # Stable, semver'd
├── agent.go                        # Agent interface
├── types.go                        # AgentName, AgentType, Event, etc.
├── registry.go                     # Register(), Get(), List()
└── testing/                        # Test helpers for agent authors
```

This is a future enhancement that provides compile-time type safety for Go agent authors who want tighter integration than the JSON protocol. It also decouples external agents from importing the full `cli` module (which pulls in cobra, huh, go-git, etc.).

---

## Recommendation: Phased Rollout

### Phase 1: Private Wrapper Repo Support (1-2 days)

Changes to the **public repo** (this repo):
- Add `Extension` type and `NewRootCmdWithExtensions()` to `root.go` (~15 lines)
- That's it. Agents already work via `init()` + `Register()` — zero changes needed

Setting up the **private repo**:
- Create `github.com/entireio/cli-internal` with its own `go.mod`
- Add `cmd/entire/main.go` that calls `NewRootCmdWithExtensions()`
- Add `.goreleaser.yaml` for internal builds
- Add private commands/agents as needed
- **Result:** Internal team ships private commands/agents with full type safety, zero public repo exposure

### Phase 2: External Agent Protocol (1-2 weeks)

- Design JSON protocol (subcommand-based, as described above)
- Implement `ExternalAgent` adapter in `cmd/entire/cli/agent/external/`
- Add agent discovery (scan `~/.entire/agents/` + PATH)
- Register discovered external agents in the existing registry
- Write a reference external agent (e.g., re-implement a simple agent as external)
- Document "How to write an external agent"
- **Result:** Anyone can write an agent in any language without touching this repo

### Phase 3: Agent SDK Module (future, if needed)

- Extract agent interfaces into `github.com/entireio/agent-sdk`
- Version the interfaces with semver guarantees
- Provide `cmd/entire-custom/main.go` template for custom builds
- **Result:** External Go agent authors don't need to depend on the full CLI module

---

## Risk Assessment

| Risk | Mitigation |
|---|---|
| Protocol changes break external agents | Version field + backward-compat commitment |
| External agents are slow (process spawn) | Cache capabilities; most hooks are infrequent |
| Agent interface changes | Phase 3 SDK with semver; Phase 2 protocol versioning |
| Build tag complexity | Keep it simple: one tag (`internal`), one extra goreleaser config |
| External agent security | Document trust model; agents run with user's permissions (same as any CLI tool) |
| Discovery confusion | Clear precedence: built-in > `~/.entire/agents/` > PATH |

---

## Comparable Systems

| System | Plugin Mechanism | Lesson for Us |
|---|---|---|
| **kubectl** | `kubectl-*` executables on PATH | Simple, community-loved, but no structured protocol |
| **gh (GitHub CLI)** | `gh-*` extensions with manifest | Added `gh extension` manager for install/update |
| **Terraform** | hashicorp/go-plugin (gRPC) | Powerful but heavy; good for complex providers, overkill for agents |
| **VS Code** | JavaScript extensions with typed API | SDK approach works at scale; versioned API is key |
| **Git** | Subcommands on PATH (`git-*`) | Simplest possible; has stood the test of time |
| **Docker** | CLI plugins in `~/.docker/cli-plugins/` | JSON metadata + executable; good middle ground |

The Docker CLI plugin model is closest to what makes sense here: lightweight JSON metadata + executable, with a well-defined discovery path.
