package insights

import (
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent/types"
)

const (
	dirStable    = "stable"
	dirImproving = "improving"
	dirDeclining = "declining"
)

func TestComputeTrends_Empty(t *testing.T) {
	t.Parallel()

	trends := ComputeTrends(nil)
	if len(trends) != 4 {
		t.Fatalf("expected 4 trends, got %d", len(trends))
	}
	for _, tr := range trends {
		if tr.Direction != dirStable {
			t.Errorf("trend %q direction = %q, want stable", tr.Metric, tr.Direction)
		}
		if len(tr.DataPoints) != 0 {
			t.Errorf("trend %q data points = %d, want 0", tr.Metric, len(tr.DataPoints))
		}
	}
}

func TestComputeTrends_SingleDataPoint(t *testing.T) {
	t.Parallel()

	scores := []SessionScore{makeScore(1, 75, 1000, 3, 0)}
	trends := ComputeTrends(scores)

	if len(trends) != 4 {
		t.Fatalf("expected 4 trends, got %d", len(trends))
	}
	for _, tr := range trends {
		if tr.Direction != dirStable {
			t.Errorf("trend %q direction = %q, want stable for single point", tr.Metric, tr.Direction)
		}
	}
}

func TestComputeTrends_StableScore(t *testing.T) {
	t.Parallel()

	// All scores the same → stable
	scores := []SessionScore{
		makeScore(1, 75, 1000, 3, 0),
		makeScore(2, 75, 1000, 3, 0),
		makeScore(3, 75, 1000, 3, 0),
		makeScore(4, 75, 1000, 3, 0),
	}
	trends := ComputeTrends(scores)
	overall := findTrend(trends, "overall_score")
	if overall == nil {
		t.Fatal("overall_score trend not found")
	}
	if overall.Direction != dirStable {
		t.Errorf("overall_score direction = %q, want stable", overall.Direction)
	}
	// change percent < 5%
	if overall.ChangePercent >= 5.0 {
		t.Errorf("ChangePercent = %v, want < 5", overall.ChangePercent)
	}
}

func TestComputeTrends_ImprovingScore(t *testing.T) {
	t.Parallel()

	// Score improves significantly from first half to second half
	scores := []SessionScore{
		makeScore(1, 40, 5000, 5, 2),
		makeScore(2, 45, 5000, 5, 2),
		makeScore(3, 75, 2000, 3, 0),
		makeScore(4, 80, 2000, 3, 0),
	}
	trends := ComputeTrends(scores)
	overall := findTrend(trends, "overall_score")
	if overall == nil {
		t.Fatal("overall_score trend not found")
	}
	if overall.Direction != dirImproving {
		t.Errorf("overall_score direction = %q, want improving", overall.Direction)
	}
}

func TestComputeTrends_DecliningScore(t *testing.T) {
	t.Parallel()

	// Score declines
	scores := []SessionScore{
		makeScore(1, 80, 2000, 3, 0),
		makeScore(2, 75, 2000, 3, 0),
		makeScore(3, 45, 5000, 5, 2),
		makeScore(4, 40, 5000, 5, 2),
	}
	trends := ComputeTrends(scores)
	overall := findTrend(trends, "overall_score")
	if overall == nil {
		t.Fatal("overall_score trend not found")
	}
	if overall.Direction != dirDeclining {
		t.Errorf("overall_score direction = %q, want declining", overall.Direction)
	}
}

func TestComputeTrends_TokenUsageInverted(t *testing.T) {
	t.Parallel()

	// Token usage decreasing (lower is better) → improving
	scores := []SessionScore{
		makeScore(1, 60, 8000, 4, 1),
		makeScore(2, 60, 8000, 4, 1),
		makeScore(3, 60, 1000, 4, 1),
		makeScore(4, 60, 1000, 4, 1),
	}
	trends := ComputeTrends(scores)
	tokenTrend := findTrend(trends, "token_usage")
	if tokenTrend == nil {
		t.Fatal("token_usage trend not found")
	}
	if tokenTrend.Direction != dirImproving {
		t.Errorf("token_usage direction = %q, want improving (lower tokens = better)", tokenTrend.Direction)
	}
}

func TestComputeTrends_FrictionInverted(t *testing.T) {
	t.Parallel()

	// Friction decreasing (lower is better) → improving
	scores := []SessionScore{
		makeScoreWithFriction(1, 2),
		makeScoreWithFriction(2, 2),
		makeScoreWithFriction(3, 0),
		makeScoreWithFriction(4, 0),
	}
	trends := ComputeTrends(scores)
	frictionTrend := findTrend(trends, "friction")
	if frictionTrend == nil {
		t.Fatal("friction trend not found")
	}
	if frictionTrend.Direction != dirImproving {
		t.Errorf("friction direction = %q, want improving (lower friction = better)", frictionTrend.Direction)
	}
}

func TestComputeTrends_DataPointsPopulated(t *testing.T) {
	t.Parallel()

	scores := []SessionScore{
		makeScore(1, 60, 1000, 3, 0),
		makeScore(2, 70, 1000, 3, 0),
		makeScore(3, 80, 1000, 3, 0),
	}
	trends := ComputeTrends(scores)
	overall := findTrend(trends, "overall_score")
	if overall == nil {
		t.Fatal("overall_score trend not found")
	}
	if len(overall.DataPoints) != 3 {
		t.Errorf("DataPoints count = %d, want 3", len(overall.DataPoints))
	}
}

func TestComputeTrends_AllMetricsPresent(t *testing.T) {
	t.Parallel()

	scores := []SessionScore{makeScore(1, 70, 2000, 4, 0)}
	trends := ComputeTrends(scores)

	expectedMetrics := []string{"overall_score", "token_usage", "friction", "turns_per_session"}
	for _, metric := range expectedMetrics {
		if findTrend(trends, metric) == nil {
			t.Errorf("metric %q not found in trends", metric)
		}
	}
}

func TestComputeAgentComparisons_Empty(t *testing.T) {
	t.Parallel()

	comparisons := ComputeAgentComparisons(nil)
	if len(comparisons) != 0 {
		t.Errorf("expected 0 comparisons, got %d", len(comparisons))
	}
}

func TestComputeAgentComparisons_SingleAgent(t *testing.T) {
	t.Parallel()

	scores := []SessionScore{
		{Agent: "Claude Code", Overall: 80, TokensUsed: 2000, TurnCount: 4, FrictionCount: 1,
			Breakdown: ScoreBreakdown{TokenEfficiency: 90, FirstPassSuccess: 70, FrictionScore: 80, FocusScore: 60}},
		{Agent: "Claude Code", Overall: 70, TokensUsed: 3000, TurnCount: 6, FrictionCount: 0,
			Breakdown: ScoreBreakdown{TokenEfficiency: 80, FirstPassSuccess: 80, FrictionScore: 100, FocusScore: 70}},
	}

	comparisons := ComputeAgentComparisons(scores)
	if len(comparisons) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(comparisons))
	}

	c := comparisons[0]
	if c.Agent != "Claude Code" {
		t.Errorf("Agent = %q, want Claude Code", c.Agent)
	}
	if c.SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", c.SessionCount)
	}
	if c.AvgScore != 75.0 {
		t.Errorf("AvgScore = %v, want 75.0", c.AvgScore)
	}
	if c.AvgTokens != 2500 {
		t.Errorf("AvgTokens = %d, want 2500", c.AvgTokens)
	}
	if c.AvgTurns != 5.0 {
		t.Errorf("AvgTurns = %v, want 5.0", c.AvgTurns)
	}
	if c.AvgFriction != 0.5 {
		t.Errorf("AvgFriction = %v, want 0.5", c.AvgFriction)
	}
}

func TestComputeAgentComparisons_TopStrengthWeakness(t *testing.T) {
	t.Parallel()

	scores := []SessionScore{
		{
			Agent:   "Claude Code",
			Overall: 75,
			Breakdown: ScoreBreakdown{
				TokenEfficiency:  90, // highest
				FirstPassSuccess: 60,
				FrictionScore:    40, // lowest
				FocusScore:       70,
			},
		},
	}

	comparisons := ComputeAgentComparisons(scores)
	if len(comparisons) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(comparisons))
	}
	c := comparisons[0]
	if c.TopStrength != "token_efficiency" {
		t.Errorf("TopStrength = %q, want token_efficiency", c.TopStrength)
	}
	if c.TopWeakness != "friction_score" {
		t.Errorf("TopWeakness = %q, want friction_score", c.TopWeakness)
	}
}

func TestComputeAgentComparisons_MultipleAgentsSortedByScore(t *testing.T) {
	t.Parallel()

	scores := []SessionScore{
		{Agent: "Gemini CLI", Overall: 60, Breakdown: ScoreBreakdown{TokenEfficiency: 60, FirstPassSuccess: 60, FrictionScore: 60, FocusScore: 60}},
		{Agent: "Claude Code", Overall: 80, Breakdown: ScoreBreakdown{TokenEfficiency: 80, FirstPassSuccess: 80, FrictionScore: 80, FocusScore: 80}},
		{Agent: "OpenCode", Overall: 70, Breakdown: ScoreBreakdown{TokenEfficiency: 70, FirstPassSuccess: 70, FrictionScore: 70, FocusScore: 70}},
	}

	comparisons := ComputeAgentComparisons(scores)
	if len(comparisons) != 3 {
		t.Fatalf("expected 3 comparisons, got %d", len(comparisons))
	}
	// Should be sorted by avg score descending
	if comparisons[0].Agent != "Claude Code" {
		t.Errorf("first agent = %q, want Claude Code", comparisons[0].Agent)
	}
	if comparisons[1].Agent != "OpenCode" {
		t.Errorf("second agent = %q, want OpenCode", comparisons[1].Agent)
	}
	if comparisons[2].Agent != "Gemini CLI" {
		t.Errorf("third agent = %q, want Gemini CLI", comparisons[2].Agent)
	}
}

// helpers

func makeScore(dayOffset int, overall float64, tokens, turns, friction int) SessionScore {
	return SessionScore{
		CreatedAt:     time.Now().AddDate(0, 0, dayOffset),
		Overall:       overall,
		TokensUsed:    tokens,
		TurnCount:     turns,
		FrictionCount: friction,
		Agent:         types.AgentType("Claude Code"),
		Breakdown:     ScoreBreakdown{TokenEfficiency: overall, FirstPassSuccess: overall, FrictionScore: overall, FocusScore: overall},
	}
}

func makeScoreWithFriction(dayOffset int, friction int) SessionScore {
	const fixedOverall = 60.0
	const fixedTurns = 5
	return SessionScore{
		CreatedAt:     time.Now().AddDate(0, 0, dayOffset),
		Overall:       fixedOverall,
		TokensUsed:    2000,
		TurnCount:     fixedTurns,
		FrictionCount: friction,
		Agent:         types.AgentType("Claude Code"),
		Breakdown: ScoreBreakdown{
			TokenEfficiency:  fixedOverall,
			FirstPassSuccess: fixedOverall,
			FrictionScore:    fixedOverall,
			FocusScore:       fixedOverall,
		},
	}
}

func findTrend(trends []Trend, metric string) *Trend {
	for i := range trends {
		if trends[i].Metric == metric {
			return &trends[i]
		}
	}
	return nil
}
