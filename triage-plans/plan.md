## Fix Plan

### Change 1 — `e2e/tests/interactive_test.go`

**File:** `e2e/tests/interactive_test.go`

The second `Send` call uses `"now commit it"` with no confirmation-suppression instruction. Gemini CLI asks for approval before running `git commit`, returns to the input prompt without committing, and `AssertNewCommits` times out.

**Current code (approx line 24):**
```go
s.Send(t, session, "now commit it")
```

**Change to:**
```go
s.Send(t, session, "now commit it. Do not ask for confirmation, just commit directly.")
```

This follows the same pattern as the first prompt (`"Do not ask for confirmation, just make the change."`) which successfully suppressed confirmation for file creation. No other test files need to change.

### Verification

After the fix:
1. Run the canary to confirm no Vogon regex breakage: `mise run test:e2e:canary TestInteractiveMultiStep`
   - If Vogon parses prompts via regex and the new wording doesn't match, update `e2e/vogon/main.go` to handle the new phrasing.
2. No unit/integration test changes needed — this is a pure prompt wording fix.
