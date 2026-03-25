package insights

import (
	"math"
	"testing"
)

func TestScoreSession_TokenEfficiency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      SessionData
		wantMin   float64
		wantMax   float64
		wantExact *float64
	}{
		{
			name:      "zero files returns neutral 50",
			data:      SessionData{TotalTokens: 1000, FilesCount: 0},
			wantExact: ptr(50.0),
		},
		{
			name:      "zero tokens returns neutral 50",
			data:      SessionData{TotalTokens: 0, FilesCount: 5},
			wantExact: ptr(50.0),
		},
		{
			name:      "both zero returns neutral 50",
			data:      SessionData{TotalTokens: 0, FilesCount: 0},
			wantExact: ptr(50.0),
		},
		{
			name:    "low tokens per file scores high",
			data:    SessionData{TotalTokens: 1000, FilesCount: 10},
			wantMin: 88.0, // 100 * exp(-100/8000) ≈ 98.8
			wantMax: 100.0,
		},
		{
			name:    "8000 tokens per file scores ~36.8",
			data:    SessionData{TotalTokens: 8000, FilesCount: 1},
			wantMin: 36.0, // 100 * exp(-1) ≈ 36.79
			wantMax: 38.0,
		},
		{
			name:    "very high tokens per file scores low",
			data:    SessionData{TotalTokens: 100000, FilesCount: 1},
			wantMin: 0.0,
			wantMax: 1.0, // near zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScoreSession(tt.data)
			if tt.wantExact != nil {
				if got.TokenEfficiency != *tt.wantExact {
					t.Errorf("TokenEfficiency = %v, want %v", got.TokenEfficiency, *tt.wantExact)
				}
				return
			}
			if got.TokenEfficiency < tt.wantMin || got.TokenEfficiency > tt.wantMax {
				t.Errorf("TokenEfficiency = %v, want [%v, %v]", got.TokenEfficiency, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestScoreSession_FirstPassSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      SessionData
		wantExact *float64
		wantMin   float64
		wantMax   float64
	}{
		{
			name:      "no summary returns neutral 50",
			data:      SessionData{HasSummary: false, FrictionCount: 0, TurnCount: 3, OpenItemCount: 0},
			wantExact: ptr(50.0),
		},
		{
			name:    "perfect session scores 80",
			data:    SessionData{HasSummary: true, FrictionCount: 0, TurnCount: 5, OpenItemCount: 0},
			wantMin: 80.0,
			wantMax: 80.0,
		},
		{
			name:    "friction deducts 10 per count",
			data:    SessionData{HasSummary: true, FrictionCount: 2, TurnCount: 5, OpenItemCount: 0},
			wantMin: 60.0, // 80 - 2*10 = 60
			wantMax: 60.0,
		},
		{
			name:    "extra turns deduct 3 per turn over 5",
			data:    SessionData{HasSummary: true, FrictionCount: 0, TurnCount: 8, OpenItemCount: 0},
			wantMin: 71.0, // 80 - 3*3 = 71
			wantMax: 71.0,
		},
		{
			name:    "open items deduct 5 each",
			data:    SessionData{HasSummary: true, FrictionCount: 0, TurnCount: 5, OpenItemCount: 2},
			wantMin: 70.0, // 80 - 2*5 = 70
			wantMax: 70.0,
		},
		{
			name:    "clamped at 0 for severe friction",
			data:    SessionData{HasSummary: true, FrictionCount: 10, TurnCount: 5, OpenItemCount: 0},
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "turns <= 5 do not deduct extra",
			data:    SessionData{HasSummary: true, FrictionCount: 0, TurnCount: 1, OpenItemCount: 0},
			wantMin: 80.0,
			wantMax: 80.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScoreSession(tt.data)
			if tt.wantExact != nil {
				if got.FirstPassSuccess != *tt.wantExact {
					t.Errorf("FirstPassSuccess = %v, want %v", got.FirstPassSuccess, *tt.wantExact)
				}
				return
			}
			if got.FirstPassSuccess < tt.wantMin || got.FirstPassSuccess > tt.wantMax {
				t.Errorf("FirstPassSuccess = %v, want [%v, %v]", got.FirstPassSuccess, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestScoreSession_FrictionScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data SessionData
		want float64
	}{
		{"zero friction is 100", SessionData{FrictionCount: 0}, 100.0},
		{"one friction is 80", SessionData{FrictionCount: 1}, 80.0},
		{"two friction is 60", SessionData{FrictionCount: 2}, 60.0},
		{"five friction is 0", SessionData{FrictionCount: 5}, 0.0},
		{"six friction clamped to 0", SessionData{FrictionCount: 6}, 0.0},
		{"ten friction clamped to 0", SessionData{FrictionCount: 10}, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScoreSession(tt.data)
			if got.FrictionScore != tt.want {
				t.Errorf("FrictionScore = %v, want %v", got.FrictionScore, tt.want)
			}
		})
	}
}

func TestScoreSession_FocusScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    SessionData
		wantMin float64
		wantMax float64
	}{
		{
			name:    "zero turns returns neutral 50",
			data:    SessionData{FilesCount: 5, TurnCount: 0},
			wantMin: 50.0,
			wantMax: 50.0,
		},
		{
			name:    "zero files returns neutral 50",
			data:    SessionData{FilesCount: 0, TurnCount: 5},
			wantMin: 50.0,
			wantMax: 50.0,
		},
		{
			name:    "both zero returns neutral 50",
			data:    SessionData{FilesCount: 0, TurnCount: 0},
			wantMin: 50.0,
			wantMax: 50.0,
		},
		{
			name:    "ratio 1.0 (in 0.5-3.0 range) scores 90",
			data:    SessionData{FilesCount: 5, TurnCount: 5},
			wantMin: 90.0,
			wantMax: 90.0,
		},
		{
			name:    "ratio 2.0 (in 0.5-3.0 range) scores 90",
			data:    SessionData{FilesCount: 10, TurnCount: 5},
			wantMin: 90.0,
			wantMax: 90.0,
		},
		{
			name:    "ratio 0.5 (boundary) scores 90",
			data:    SessionData{FilesCount: 1, TurnCount: 2},
			wantMin: 90.0,
			wantMax: 90.0,
		},
		{
			name:    "ratio below 0.5 scores lower",
			data:    SessionData{FilesCount: 1, TurnCount: 10}, // ratio = 0.1
			wantMin: 50.0,                                      // 50 + 0.1*80 = 58
			wantMax: 60.0,
		},
		{
			name:    "ratio above 3.0 scores lower",
			data:    SessionData{FilesCount: 40, TurnCount: 5}, // ratio = 8.0
			wantMin: 30.0,                                      // 90 - (8-3)*10 = 40, but clamped
			wantMax: 42.0,
		},
		{
			name:    "very high ratio clamped to 0",
			data:    SessionData{FilesCount: 1000, TurnCount: 1}, // ratio = 1000
			wantMin: 0.0,
			wantMax: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScoreSession(tt.data)
			if got.FocusScore < tt.wantMin || got.FocusScore > tt.wantMax {
				t.Errorf("FocusScore = %v, want [%v, %v]", got.FocusScore, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestComputeOverall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		b    ScoreBreakdown
		want float64
	}{
		{
			name: "all 100 gives 100",
			b:    ScoreBreakdown{TokenEfficiency: 100, FirstPassSuccess: 100, FrictionScore: 100, FocusScore: 100},
			want: 100.0,
		},
		{
			name: "all 0 gives 0",
			b:    ScoreBreakdown{TokenEfficiency: 0, FirstPassSuccess: 0, FrictionScore: 0, FocusScore: 0},
			want: 0.0,
		},
		{
			name: "weighted sum: token=100 only",
			// 100*0.30 + 0*0.30 + 0*0.25 + 0*0.15 = 30.0
			b:    ScoreBreakdown{TokenEfficiency: 100, FirstPassSuccess: 0, FrictionScore: 0, FocusScore: 0},
			want: 30.0,
		},
		{
			name: "weighted sum: first pass=100 only",
			// 0*0.30 + 100*0.30 + 0*0.25 + 0*0.15 = 30.0
			b:    ScoreBreakdown{TokenEfficiency: 0, FirstPassSuccess: 100, FrictionScore: 0, FocusScore: 0},
			want: 30.0,
		},
		{
			name: "weighted sum: friction=100 only",
			// 0*0.30 + 0*0.30 + 100*0.25 + 0*0.15 = 25.0
			b:    ScoreBreakdown{TokenEfficiency: 0, FirstPassSuccess: 0, FrictionScore: 100, FocusScore: 0},
			want: 25.0,
		},
		{
			name: "weighted sum: focus=100 only",
			// 0*0.30 + 0*0.30 + 0*0.25 + 100*0.15 = 15.0
			b:    ScoreBreakdown{TokenEfficiency: 0, FirstPassSuccess: 0, FrictionScore: 0, FocusScore: 100},
			want: 15.0,
		},
		{
			name: "mixed values rounded to 1 decimal",
			// 80*0.30 + 70*0.30 + 90*0.25 + 60*0.15
			// = 24 + 21 + 22.5 + 9 = 76.5
			b:    ScoreBreakdown{TokenEfficiency: 80, FirstPassSuccess: 70, FrictionScore: 90, FocusScore: 60},
			want: 76.5,
		},
		{
			name: "rounding: result with fractional part",
			// 75*0.30 + 75*0.30 + 75*0.25 + 75*0.15 = 75.0
			b:    ScoreBreakdown{TokenEfficiency: 75, FirstPassSuccess: 75, FrictionScore: 75, FocusScore: 75},
			want: 75.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ComputeOverall(tt.b)
			if math.Abs(got-tt.want) > 0.05 {
				t.Errorf("ComputeOverall = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClampScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input float64
		want  float64
	}{
		{-10, 0},
		{0, 0},
		{50, 50},
		{100, 100},
		{110, 100},
	}

	for _, tt := range tests {
		got := clampScore(tt.input)
		if got != tt.want {
			t.Errorf("clampScore(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func ptr(f float64) *float64 { return &f }
