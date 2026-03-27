# Memory Loop TUI Restyle Design

## Summary

Restyle the existing four-tab memory-loop TUI without changing its information architecture. Keep `Memories`, `Injection`, `History`, and `Settings` as they are, but make the shell and panels feel more intentional and terminal-native, drawing visual cues from `gh-dash` and `posting`.

## Goals

- Keep the current tab layout and workflows intact.
- Make the TUI feel polished in both dark and light terminals.
- Use terminal-native styling instead of fixed backgrounds.
- Improve hierarchy, spacing, borders, and selection treatment.
- Reduce the spreadsheet feel of the data-heavy screens.

## Non-Goals

- No dashboard or inspector layout experiment.
- No state or navigation changes.
- No changes to memory-loop commands, actions, or storage.
- No theme that fights the user’s terminal background.

## Visual Direction

The UI should use the terminal as the base canvas and add structure on top of it.

### Principles

- default terminal background
- muted gray chrome
- one restrained warm accent for focus
- semantic colors only for statuses
- stronger spacing and quieter borders

### Reference Traits

From `gh-dash`:

- compact, legible tab chrome
- strong contrast between active and inactive navigation
- crisp panel framing without heavy fill colors

From `posting`:

- calm typography hierarchy
- compact uppercase labels
- deliberate spacing that makes dense screens feel readable

## Design Changes

### Shell

- restyle the top tab bar so it looks like app navigation rather than plain text tabs
- tighten the top status indicators for mode and policy
- improve the footer hint bar so it reads like secondary chrome

### Panels

- use a shared panel style for cards and bordered blocks
- use uppercase micro-labels for section headers
- use more padding inside cards so content is easier to scan

### Tables and Lists

- soften table headers
- make selected rows clearer with accent treatment and stronger foreground emphasis
- reduce visual noise in non-selected rows
- keep backgrounds transparent or minimal

### Detail Cards

- use badges more cleanly for kind/status/scope
- strengthen heading hierarchy
- reduce border clutter so content reads first

## Implementation Boundaries

This is a presentation-only change.

Primary files:

- `cmd/entire/cli/memorylooptui/styles.go`
- `cmd/entire/cli/memorylooptui/render.go`
- `cmd/entire/cli/memorylooptui/tab_memories.go`
- `cmd/entire/cli/memorylooptui/tab_injection.go`
- `cmd/entire/cli/memorylooptui/tab_history.go`
- `cmd/entire/cli/memorylooptui/tab_settings.go`

## Testing Strategy

Add focused render tests that verify:

- improved tab bar formatting
- section labels and shell chrome
- restyled summary cards or panel labels
- existing tab content still renders under the same navigation model
