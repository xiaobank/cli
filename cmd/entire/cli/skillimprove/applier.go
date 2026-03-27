package skillimprove

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Hunk represents a single hunk in a unified diff.
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []DiffLine
}

// DiffLine is a single line within a diff hunk.
type DiffLine struct {
	Kind rune // ' ' for context, '+' for addition, '-' for removal
	Text string
}

// ApplyDiff reads the file at filePath, applies the unified diff, and writes
// the result back. It returns a descriptive error if context lines do not match.
func ApplyDiff(filePath, diffText string) error {
	content, err := os.ReadFile(filePath) //nolint:gosec // filePath is caller-controlled, not user input
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	hunks, err := ParseHunks(diffText)
	if err != nil {
		return fmt.Errorf("parsing diff: %w", err)
	}

	if len(hunks) == 0 {
		return nil // nothing to apply
	}

	lines := splitLines(string(content))
	result, err := applyHunks(lines, hunks)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filePath, []byte(joinLines(result)), 0o600); err != nil { //nolint:gosec // filePath is caller-controlled, not user input
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

// ParseHunks extracts hunks from a unified diff string.
func ParseHunks(diffText string) ([]Hunk, error) {
	rawLines := strings.Split(diffText, "\n")

	var hunks []Hunk
	var current *Hunk

	for _, line := range rawLines {
		// Skip file headers.
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "diff ") {
			continue
		}

		// Hunk header.
		if strings.HasPrefix(line, "@@ ") {
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, h)
			current = &hunks[len(hunks)-1]
			continue
		}

		if current == nil {
			continue // skip lines before first hunk
		}

		if len(line) == 0 {
			// Empty line in a diff is treated as a context line with empty text.
			current.Lines = append(current.Lines, DiffLine{Kind: ' ', Text: ""})
			continue
		}

		kind := rune(line[0])
		text := line[1:]

		switch kind {
		case ' ', '+', '-':
			current.Lines = append(current.Lines, DiffLine{Kind: kind, Text: text})
		case '\\':
			// "\ No newline at end of file" — skip.
			continue
		default:
			// Treat unrecognised lines as context (some diffs omit the leading space).
			current.Lines = append(current.Lines, DiffLine{Kind: ' ', Text: line})
		}
	}

	return hunks, nil
}

// parseHunkHeader parses a line like "@@ -1,3 +1,4 @@" or "@@ -1 +1,2 @@".
func parseHunkHeader(line string) (Hunk, error) {
	// Strip the @@ markers and any trailing section heading.
	line = strings.TrimPrefix(line, "@@ ")
	if idx := strings.Index(line, " @@"); idx != -1 {
		line = line[:idx]
	}

	parts := strings.Fields(line)
	if len(parts) < 2 {
		return Hunk{}, fmt.Errorf("invalid hunk header: %q", line)
	}

	oldStart, oldCount, err := parseRange(parts[0])
	if err != nil {
		return Hunk{}, fmt.Errorf("invalid old range %q: %w", parts[0], err)
	}

	newStart, newCount, err := parseRange(parts[1])
	if err != nil {
		return Hunk{}, fmt.Errorf("invalid new range %q: %w", parts[1], err)
	}

	return Hunk{OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount}, nil
}

// parseRange parses "-1,3" or "+1,3" or "-1" into (start, count).
func parseRange(s string) (int, int, error) {
	s = strings.TrimLeft(s, "+-")
	if idx := strings.Index(s, ","); idx != -1 {
		start, err := strconv.Atoi(s[:idx])
		if err != nil {
			return 0, 0, fmt.Errorf("parsing start: %w", err)
		}
		count, err := strconv.Atoi(s[idx+1:])
		if err != nil {
			return 0, 0, fmt.Errorf("parsing count: %w", err)
		}
		return start, count, nil
	}

	start, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing range: %w", err)
	}
	return start, 1, nil
}

// applyHunks applies parsed hunks to the source lines.
// Hunks must be in order of ascending OldStart.
func applyHunks(lines []string, hunks []Hunk) ([]string, error) {
	var result []string
	srcIdx := 0 // 0-based index into lines

	for _, h := range hunks {
		hunkStart := h.OldStart - 1 // convert to 0-based

		// Copy lines before this hunk.
		if hunkStart > srcIdx {
			result = append(result, lines[srcIdx:hunkStart]...)
		}
		srcIdx = hunkStart

		for _, dl := range h.Lines {
			switch dl.Kind {
			case ' ':
				// Context line: verify and copy.
				if srcIdx >= len(lines) {
					return nil, fmt.Errorf("diff context mismatch at line %d: expected %q, got end of file", srcIdx+1, dl.Text)
				}
				if lines[srcIdx] != dl.Text {
					return nil, fmt.Errorf("diff context mismatch at line %d: expected %q, got %q", srcIdx+1, dl.Text, lines[srcIdx])
				}
				result = append(result, lines[srcIdx])
				srcIdx++

			case '-':
				// Removal: verify and skip.
				if srcIdx >= len(lines) {
					return nil, fmt.Errorf("diff context mismatch at line %d: expected %q, got end of file", srcIdx+1, dl.Text)
				}
				if lines[srcIdx] != dl.Text {
					return nil, fmt.Errorf("diff context mismatch at line %d: expected %q, got %q", srcIdx+1, dl.Text, lines[srcIdx])
				}
				srcIdx++

			case '+':
				// Addition: insert.
				result = append(result, dl.Text)
			}
		}
	}

	// Copy remaining lines after the last hunk.
	if srcIdx < len(lines) {
		result = append(result, lines[srcIdx:]...)
	}

	return result, nil
}

// splitLines splits content into lines. A trailing newline does not produce an
// extra empty element.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// joinLines joins lines back together with newlines, adding a trailing newline.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
