// Package improve provides context file detection, friction analysis, and
// AI-powered improvement suggestions for project context files.
package improve

import "time"

// ContextFileType identifies the type of context file.
type ContextFileType string

const (
	// ContextFileCLAUDEMD represents a CLAUDE.md context file.
	ContextFileCLAUDEMD ContextFileType = "CLAUDE.md"
	// ContextFileAGENTSMD represents an AGENTS.md context file.
	ContextFileAGENTSMD ContextFileType = "AGENTS.md"
	// ContextFileCursorRules represents a .cursorrules context file.
	ContextFileCursorRules ContextFileType = ".cursorrules"
	// ContextFileGemini represents a .gemini/settings.json context file.
	ContextFileGemini ContextFileType = ".gemini/settings.json"
)

// ContextFile represents a detected context file in the project.
type ContextFile struct {
	Type      ContextFileType `json:"type"`
	Path      string          `json:"path"`
	Exists    bool            `json:"exists"`
	Content   string          `json:"content,omitempty"`
	SizeBytes int             `json:"size_bytes"`
}

// Suggestion represents a proposed change to a context file.
type Suggestion struct {
	ID          string          `json:"id"`
	FileType    ContextFileType `json:"file_type"`
	FilePath    string          `json:"file_path"`
	Category    string          `json:"category"` // "fix_friction", "add_rule", "add_pattern"
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Evidence    []string        `json:"evidence"`
	Priority    string          `json:"priority"` // "high", "medium", "low"
	Diff        string          `json:"diff"`
	CreatedAt   time.Time       `json:"created_at"`
	Status      string          `json:"status"` // "pending", "accepted", "rejected"
}

// ImprovementReport is the output of `entire improve`.
type ImprovementReport struct {
	ContextFiles  []ContextFile `json:"context_files"`
	Suggestions   []Suggestion  `json:"suggestions"`
	SessionsUsed  int           `json:"sessions_used"`
	FrictionTotal int           `json:"friction_total"`
	PatternsFound int           `json:"patterns_found"`
}

// FrictionPattern represents a recurring friction theme with evidence.
type FrictionPattern struct {
	Theme             string   // Normalized theme
	Count             int      // Occurrences across sessions
	Examples          []string // Raw friction text from summaries
	AffectedSessions  []string // Checkpoint IDs
	TranscriptExcerpt string   // Condensed transcript excerpt around friction (from deep-read)
}

// PatternAnalysis contains extracted patterns from multiple sessions.
type PatternAnalysis struct {
	RepeatedFriction  []FrictionPattern
	RepoLearnings     []string
	WorkflowLearnings []string
	OpenItems         []string
	SessionCount      int
}
