package skilldb

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoveredSkill represents a skill file found in an agent's config directory.
type DiscoveredSkill struct {
	Name        string // skill name (e.g., "e2e", "dev")
	SourceAgent string // "claude-code" or "gemini-cli"
	Path        string // relative path from repo root (e.g., ".claude/skills/e2e/SKILL.md")
	Kind        string // "skill", "command", or "agent-def"
}

// DiscoverSkills scans repoRoot for skill files across agent config directories.
// Missing directories are silently skipped.
func DiscoverSkills(repoRoot string) ([]DiscoveredSkill, error) {
	var skills []DiscoveredSkill

	collectors := []struct {
		pattern     string
		sourceAgent string
		kind        string
		nameFunc    func(match string) string
		readContent bool
	}{
		{
			pattern:     filepath.Join(repoRoot, ".claude", "skills", "*", "SKILL.md"),
			sourceAgent: "claude-code",
			kind:        "skill",
			nameFunc:    func(match string) string { return filepath.Base(filepath.Dir(match)) },
		},
		{
			pattern:     filepath.Join(repoRoot, ".claude", "commands", "*.md"),
			sourceAgent: "claude-code",
			kind:        "command",
			nameFunc:    func(match string) string { return strings.TrimSuffix(filepath.Base(match), ".md") },
		},
		{
			pattern:     filepath.Join(repoRoot, ".gemini", "agents", "*.md"),
			sourceAgent: "gemini-cli",
			kind:        "agent-def",
			readContent: true,
			nameFunc:    func(match string) string { return strings.TrimSuffix(filepath.Base(match), ".md") },
		},
		{
			pattern:     filepath.Join(repoRoot, ".gemini", "commands", "*.md"),
			sourceAgent: "gemini-cli",
			kind:        "command",
			readContent: true,
			nameFunc:    func(match string) string { return strings.TrimSuffix(filepath.Base(match), ".md") },
		},
	}

	for _, c := range collectors {
		matches, err := filepath.Glob(c.pattern)
		if err != nil {
			return nil, fmt.Errorf("globbing %s: %w", c.pattern, err)
		}

		for _, match := range matches {
			name := c.nameFunc(match)

			if c.readContent {
				content, err := os.ReadFile(match) //nolint:gosec // match comes from filepath.Glob, not user input
				if err != nil {
					return nil, fmt.Errorf("reading %s: %w", match, err)
				}
				if yamlName := extractYAMLName(string(content)); yamlName != "" {
					name = yamlName
				}
			}

			relPath, err := filepath.Rel(repoRoot, match)
			if err != nil {
				return nil, fmt.Errorf("computing relative path for %s: %w", match, err)
			}

			skills = append(skills, DiscoveredSkill{
				Name:        name,
				SourceAgent: c.sourceAgent,
				Path:        relPath,
				Kind:        c.kind,
			})
		}
	}

	sort.Slice(skills, func(i, j int) bool {
		if skills[i].SourceAgent != skills[j].SourceAgent {
			return skills[i].SourceAgent < skills[j].SourceAgent
		}
		return skills[i].Name < skills[j].Name
	})

	return skills, nil
}

// extractYAMLName looks for a name field in YAML frontmatter delimited by "---".
func extractYAMLName(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}

	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if strings.HasPrefix(trimmed, "name:") {
			value := strings.TrimPrefix(trimmed, "name:")
			return strings.TrimSpace(value)
		}
	}

	return ""
}
