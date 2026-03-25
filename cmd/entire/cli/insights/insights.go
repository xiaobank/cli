package insights

import (
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent/types"
)

// SessionScore represents a quality assessment of a single session.
type SessionScore struct {
	CheckpointID  string          `json:"checkpoint_id"`
	SessionID     string          `json:"session_id"`
	Agent         types.AgentType `json:"agent"`
	Model         string          `json:"model"`
	CreatedAt     time.Time       `json:"created_at"`
	Overall       float64         `json:"overall"` // 0-100 composite
	Breakdown     ScoreBreakdown  `json:"breakdown"`
	TokensUsed    int             `json:"tokens_used"`
	TurnCount     int             `json:"turn_count"`
	FilesCount    int             `json:"files_count"`
	FrictionCount int             `json:"friction_count"`
}

// ScoreBreakdown shows individual scoring dimensions (each 0-100).
type ScoreBreakdown struct {
	TokenEfficiency  float64 `json:"token_efficiency"`
	FirstPassSuccess float64 `json:"first_pass_success"`
	FrictionScore    float64 `json:"friction_score"`
	FocusScore       float64 `json:"focus_score"`
}

// SessionData is the input to ScoreSession — populated from CommittedMetadata.
// This decouples scoring from checkpoint types.
type SessionData struct {
	TotalTokens   int
	FilesCount    int
	FrictionCount int
	TurnCount     int
	OpenItemCount int
	HasSummary    bool
}

// Trend represents a metric tracked over time.
type Trend struct {
	Metric        string      `json:"metric"`
	Direction     string      `json:"direction"` // "improving", "declining", "stable"
	ChangePercent float64     `json:"change_percent"`
	DataPoints    []DataPoint `json:"data_points"`
}

// DataPoint is a single observation in a trend.
type DataPoint struct {
	Date  time.Time `json:"date"`
	Value float64   `json:"value"`
	Label string    `json:"label,omitempty"`
}

// AgentComparison summarizes performance differences between agents.
type AgentComparison struct {
	Agent        types.AgentType `json:"agent"`
	SessionCount int             `json:"session_count"`
	AvgScore     float64         `json:"avg_score"`
	AvgTokens    int             `json:"avg_tokens"`
	AvgTurns     float64         `json:"avg_turns"`
	AvgFriction  float64         `json:"avg_friction"`
	TopStrength  string          `json:"top_strength"`
	TopWeakness  string          `json:"top_weakness"`
}

// Report is the full output of `entire insights`.
type Report struct {
	GeneratedAt  time.Time         `json:"generated_at"`
	Period       string            `json:"period"`
	Sessions     []SessionScore    `json:"sessions"`
	Trends       []Trend           `json:"trends"`
	Comparisons  []AgentComparison `json:"comparisons"`
	SessionCount int               `json:"session_count"`
}
