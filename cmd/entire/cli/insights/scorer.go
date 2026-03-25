package insights

import (
	"math"
)

// Weights for composite score.
const (
	WeightTokenEfficiency  = 0.30
	WeightFirstPassSuccess = 0.30
	WeightFriction         = 0.25
	WeightFocus            = 0.15
)

// ScoreSession computes a score breakdown from session data.
// This is a pure function — no I/O.
// The Overall score is NOT computed here — call ComputeOverall with the result.
func ScoreSession(data SessionData) ScoreBreakdown {
	return ScoreBreakdown{
		TokenEfficiency:  scoreTokenEfficiency(data),
		FirstPassSuccess: scoreFirstPassSuccess(data),
		FrictionScore:    scoreFriction(data),
		FocusScore:       scoreFocus(data),
	}
}

// ComputeOverall applies weights to a breakdown and returns the composite 0-100 score
// rounded to 1 decimal place.
func ComputeOverall(b ScoreBreakdown) float64 {
	raw := b.TokenEfficiency*WeightTokenEfficiency +
		b.FirstPassSuccess*WeightFirstPassSuccess +
		b.FrictionScore*WeightFriction +
		b.FocusScore*WeightFocus
	return math.Round(raw*10) / 10
}

// clampScore clamps s to the range [0, 100].
func clampScore(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 100 {
		return 100
	}
	return s
}

// scoreTokenEfficiency uses a sigmoid centered at 500k tokens/turn.
// Lower tokens per turn = higher efficiency.
// ~100k/turn → ~85, ~500k → 50, ~2M → ~12.
// Returns 50 (neutral) when turns==0 or totalTokens==0.
func scoreTokenEfficiency(data SessionData) float64 {
	if data.TurnCount == 0 || data.TotalTokens == 0 {
		return 50
	}
	tokensPerTurn := float64(data.TotalTokens) / float64(data.TurnCount)
	return clampScore(100 / (1 + math.Pow(tokensPerTurn/500000, 1.5)))
}

// scoreFirstPassSuccess starts at 90, deducting for friction, extra turns,
// and open items. Returns 50 (neutral) when HasSummary is false.
func scoreFirstPassSuccess(data SessionData) float64 {
	if !data.HasSummary {
		return 50
	}
	score := 90.0
	score -= float64(data.FrictionCount) * 5
	if data.TurnCount > 5 {
		score -= float64(data.TurnCount-5) * 2
	}
	score -= float64(data.OpenItemCount) * 3
	return clampScore(score)
}

// scoreFriction returns 100 - frictionCount*15, clamped to [0,100].
// 0 friction → 100, 3 → 55, 5 → 25, 7+ → 0.
func scoreFriction(data SessionData) float64 {
	return clampScore(100 - float64(data.FrictionCount)*15)
}

// scoreFocus uses a gaussian curve on turns-per-file ratio.
// Peak at ratio=1 (1 turn per file). Gradual falloff for scattered or over-focused sessions.
// Returns 50 (neutral) when turns==0 or files==0.
func scoreFocus(data SessionData) float64 {
	if data.TurnCount == 0 || data.FilesCount == 0 {
		return 50
	}
	ratio := float64(data.TurnCount) / float64(data.FilesCount)
	deviation := math.Log2(math.Max(ratio, 0.1))
	return clampScore(95 * math.Exp(-deviation*deviation/4))
}
