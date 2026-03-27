# Memory Loop TUI Restyle Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Restyle the existing memory-loop TUI so it feels polished and terminal-native without changing the four-tab information architecture.

**Architecture:** Keep the current tab structure and state flow, and limit changes to the TUI presentation layer. Improve the shared style primitives and tab rendering so the shell, cards, and selected states feel closer to `gh-dash` and `posting` while still adapting to the user’s terminal.

**Tech Stack:** Go, Bubble Tea, Bubbles, Lip Gloss, testify/require

---

### Task 1: Add failing render tests for shell chrome

**Files:**
- Create: `cmd/entire/cli/memorylooptui/render_test.go`
- Modify: `cmd/entire/cli/memorylooptui/render.go`

**Step 1: Write the failing test**

```go
func TestRenderTabBar_UsesStyledNavigationLabels(t *testing.T) {
	t.Parallel()

	out := renderTabBar(newStyles(), tabMemories, 100, memoryloop.ModeAuto, memoryloop.ActivationPolicyReview)

	require.Contains(t, out, "1 Memories")
	require.Contains(t, out, "auto")
	require.Contains(t, out, "review")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/entire/cli/memorylooptui -run TestRenderTabBar_UsesStyledNavigationLabels`

Expected: FAIL once the test expects the new chrome markers or panel labels.

**Step 3: Write minimal implementation**

Restyle the shared shell renderers in `render.go` and style primitives in `styles.go`.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/entire/cli/memorylooptui -run TestRenderTabBar_UsesStyledNavigationLabels`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/entire/cli/memorylooptui/render_test.go cmd/entire/cli/memorylooptui/render.go cmd/entire/cli/memorylooptui/styles.go
git commit -m "test: add memory-loop TUI shell restyle coverage"
```

### Task 2: Restyle memories tab presentation

**Files:**
- Modify: `cmd/entire/cli/memorylooptui/tab_memories.go`
- Modify: `cmd/entire/cli/memorylooptui/styles.go`
- Test: `cmd/entire/cli/memorylooptui/render_test.go`

**Step 1: Write the failing test**

```go
func TestMemoriesView_UsesPanelLabelsAndStyledDetailCard(t *testing.T) {
	t.Parallel()

	out := newRootModelForTest().View()

	require.Contains(t, out, "MEMORY LIST")
	require.Contains(t, out, "WHY")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/entire/cli/memorylooptui -run TestMemoriesView_UsesPanelLabelsAndStyledDetailCard`

Expected: FAIL before the memory tab gets the new styling treatment.

**Step 3: Write minimal implementation**

Tighten memory table styling, selected row treatment, filter chips, and detail card presentation.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/entire/cli/memorylooptui -run TestMemoriesView_UsesPanelLabelsAndStyledDetailCard`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/entire/cli/memorylooptui/tab_memories.go cmd/entire/cli/memorylooptui/styles.go cmd/entire/cli/memorylooptui/render_test.go
git commit -m "feat: restyle memory-loop memories tab"
```

### Task 3: Restyle injection, history, and settings panels

**Files:**
- Modify: `cmd/entire/cli/memorylooptui/tab_injection.go`
- Modify: `cmd/entire/cli/memorylooptui/tab_history.go`
- Modify: `cmd/entire/cli/memorylooptui/tab_settings.go`
- Modify: `cmd/entire/cli/memorylooptui/styles.go`
- Test: `cmd/entire/cli/memorylooptui/render_test.go`

**Step 1: Write the failing test**

```go
func TestSecondaryTabs_UseSharedPanelChrome(t *testing.T) {
	t.Parallel()

	root := newRootModelForTest()
	root.activeTab = tabInjection

	require.Contains(t, root.View(), "PROMPT TESTER")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/entire/cli/memorylooptui -run TestSecondaryTabs_UseSharedPanelChrome`

Expected: FAIL until secondary tabs use the improved panel styling.

**Step 3: Write minimal implementation**

Apply the shared visual language to injection/history/settings without changing their behavior.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/entire/cli/memorylooptui -run TestSecondaryTabs_UseSharedPanelChrome`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/entire/cli/memorylooptui/tab_injection.go cmd/entire/cli/memorylooptui/tab_history.go cmd/entire/cli/memorylooptui/tab_settings.go cmd/entire/cli/memorylooptui/styles.go cmd/entire/cli/memorylooptui/render_test.go
git commit -m "feat: restyle memory-loop secondary tabs"
```

### Task 4: Final verification

**Files:**
- Modify: `cmd/entire/cli/memorylooptui/*.go`
- Test: `cmd/entire/cli/memorylooptui/render_test.go`

**Step 1: Format**

Run: `gofmt -w cmd/entire/cli/memorylooptui/*.go`

Expected: formatted files only

**Step 2: Run package tests**

Run: `go test ./cmd/entire/cli/memorylooptui`

Expected: PASS

**Step 3: Run related CLI tests**

Run: `go test ./cmd/entire/cli -run 'Test(RunMemoryLoop|SetMemoryLoop|RunMemoryLoopShow)'`

Expected: PASS

**Step 4: Commit**

```bash
git add cmd/entire/cli/memorylooptui/*.go
git commit -m "fix: polish memory-loop TUI restyle"
```
