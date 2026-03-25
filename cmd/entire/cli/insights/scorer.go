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

// scoreTokenEfficiency returns 100*exp(-tokensPerFile/8000).
// Returns 50 (neutral) when filesCount==0 or totalTokens==0.
func scoreTokenEfficiency(data SessionData) float64 {
	if data.FilesCount == 0 || data.TotalTokens == 0 {
		return 50
	}
	tokensPerFile := float64(data.TotalTokens) / float64(data.FilesCount)
	return 100 * math.Exp(-tokensPerFile/8000)
}

// scoreFirstPassSuccess returns a score starting at 80, deducting for friction,
// extra turns, and open items. Returns 50 (neutral) when HasSummary is false.
func scoreFirstPassSuccess(data SessionData) float64 {
	if !data.HasSummary {
		return 50
	}
	score := 80.0
	score -= float64(data.FrictionCount) * 10
	if data.TurnCount > 5 {
		score -= float64(data.TurnCount-5) * 3
	}
	score -= float64(data.OpenItemCount) * 5
	return clampScore(score)
}

// scoreFriction returns 100 - frictionCount*20, clamped to [0,100].
func scoreFriction(data SessionData) float64 {
	return clampScore(100 - float64(data.FrictionCount)*20)
}

// scoreFocus returns a score based on the file-to-turn ratio.
// Returns 50 (neutral) when turns==0 or files==0.
func scoreFocus(data SessionData) float64 {
	if data.TurnCount == 0 || data.FilesCount == 0 {
		return 50
	}
	ratio := float64(data.FilesCount) / float64(data.TurnCount)
	var score float64
	switch {
	case ratio >= 0.5 && ratio <= 3.0:
		score = 90
	case ratio < 0.5:
		score = 50 + ratio*80
	default: // ratio > 3.0
		score = 90 - (ratio-3.0)*10
	}
	return clampScore(score)
}
