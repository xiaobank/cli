package insights

import (
	"math"
	"sort"

	"github.com/entireio/cli/cmd/entire/cli/agent/types"
)

// ComputeTrends analyzes score history to detect improvement or decline.
// It computes trends for: "overall_score", "token_usage", "friction", "turns_per_session".
// Method: compare first-half average to second-half average.
// |change| < 5% → "stable"; otherwise "improving" or "declining"
// (inverted for token_usage, friction, turns where lower is better).
// If < 2 data points, all trends are "stable".
func ComputeTrends(scores []SessionScore) []Trend {
	metrics := []struct {
		name        string
		extract     func(SessionScore) float64
		lowerBetter bool
	}{
		{"overall_score", func(s SessionScore) float64 { return s.Overall }, false},
		{"token_usage", func(s SessionScore) float64 { return float64(s.TokensUsed) }, true},
		{"friction", func(s SessionScore) float64 { return float64(s.FrictionCount) }, true},
		{"turns_per_session", func(s SessionScore) float64 { return float64(s.TurnCount) }, true},
	}

	trends := make([]Trend, 0, len(metrics))
	for _, m := range metrics {
		trends = append(trends, computeTrendForMetric(scores, m.name, m.extract, m.lowerBetter))
	}
	return trends
}

// computeTrendForMetric computes a single trend for one metric.
func computeTrendForMetric(scores []SessionScore, metric string, extract func(SessionScore) float64, lowerBetter bool) Trend {
	dataPoints := make([]DataPoint, 0, len(scores))
	for _, s := range scores {
		dataPoints = append(dataPoints, DataPoint{
			Date:  s.CreatedAt,
			Value: extract(s),
			Label: s.SessionID,
		})
	}

	trend := Trend{
		Metric:     metric,
		Direction:  "stable",
		DataPoints: dataPoints,
	}

	if len(scores) < 2 {
		return trend
	}

	// Split into first and second halves.
	mid := len(scores) / 2
	firstHalf := scores[:mid]
	secondHalf := scores[mid:]

	firstAvg := average(firstHalf, extract)
	secondAvg := average(secondHalf, extract)

	if firstAvg == 0 {
		return trend
	}

	changePercent := (secondAvg - firstAvg) / math.Abs(firstAvg) * 100
	trend.ChangePercent = math.Round(math.Abs(changePercent)*10) / 10

	if math.Abs(changePercent) < 5 {
		return trend
	}

	// Determine direction: for lower-is-better metrics, a decrease is "improving".
	increased := changePercent > 0
	if lowerBetter {
		if increased {
			trend.Direction = "declining"
		} else {
			trend.Direction = "improving"
		}
	} else {
		if increased {
			trend.Direction = "improving"
		} else {
			trend.Direction = "declining"
		}
	}

	return trend
}

// average computes the mean of the extracted value from a slice of scores.
func average(scores []SessionScore, extract func(SessionScore) float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range scores {
		sum += extract(s)
	}
	return sum / float64(len(scores))
}

// ComputeAgentComparisons groups scores by agent and computes averages.
// For each agent: avg score, avg tokens, avg turns, avg friction.
// TopStrength/TopWeakness: compare breakdown dimension averages, pick highest/lowest.
// Results are sorted by avg score descending.
func ComputeAgentComparisons(scores []SessionScore) []AgentComparison {
	if len(scores) == 0 {
		return nil
	}

	type accumulator struct {
		totalScore      float64
		totalTokens     int
		totalTurns      float64
		totalFriction   float64
		totalTokenEff   float64
		totalFirstPass  float64
		totalFrictionSc float64
		totalFocus      float64
		totalToolCalls  int
		toolCounts      map[string]int
		count           int
	}

	agentMap := make(map[types.AgentType]*accumulator)
	for _, s := range scores {
		acc, ok := agentMap[s.Agent]
		if !ok {
			acc = &accumulator{toolCounts: make(map[string]int)}
			agentMap[s.Agent] = acc
		}
		acc.totalScore += s.Overall
		acc.totalTokens += s.TokensUsed
		acc.totalTurns += float64(s.TurnCount)
		acc.totalFriction += float64(s.FrictionCount)
		acc.totalTokenEff += s.Breakdown.TokenEfficiency
		acc.totalFirstPass += s.Breakdown.FirstPassSuccess
		acc.totalFrictionSc += s.Breakdown.FrictionScore
		acc.totalFocus += s.Breakdown.FocusScore
		acc.totalToolCalls += s.ToolCallCount
		for _, tool := range s.TopTools {
			acc.toolCounts[tool]++
		}
		acc.count++
	}

	comparisons := make([]AgentComparison, 0, len(agentMap))
	for agent, acc := range agentMap {
		n := float64(acc.count)
		avgTokenEff := acc.totalTokenEff / n
		avgFirstPass := acc.totalFirstPass / n
		avgFrictionSc := acc.totalFrictionSc / n
		avgFocus := acc.totalFocus / n

		topStrength, topWeakness := strengthAndWeakness(avgTokenEff, avgFirstPass, avgFrictionSc, avgFocus)

		comparisons = append(comparisons, AgentComparison{
			Agent:        agent,
			SessionCount: acc.count,
			AvgScore:     math.Round(acc.totalScore/n*10) / 10,
			AvgTokens:    int(math.Round(float64(acc.totalTokens) / n)),
			AvgTurns:     math.Round(acc.totalTurns/n*10) / 10,
			AvgFriction:  math.Round(acc.totalFriction/n*10) / 10,
			TopStrength:  topStrength,
			TopWeakness:  topWeakness,
			TopTools:     topNFromCounts(acc.toolCounts, 3),
			AvgToolCalls: math.Round(float64(acc.totalToolCalls)/n*10) / 10,
		})
	}

	// Sort by avg score descending.
	sort.Slice(comparisons, func(i, j int) bool {
		return comparisons[i].AvgScore > comparisons[j].AvgScore
	})

	return comparisons
}

// strengthAndWeakness returns the dimension name with the highest and lowest average.
func strengthAndWeakness(tokenEff, firstPass, frictionSc, focus float64) (strength, weakness string) {
	dims := []struct {
		name  string
		value float64
	}{
		{"token_efficiency", tokenEff},
		{"first_pass_success", firstPass},
		{"friction_score", frictionSc},
		{"focus_score", focus},
	}

	maxVal := dims[0].value
	minVal := dims[0].value
	strength = dims[0].name
	weakness = dims[0].name

	for _, d := range dims[1:] {
		if d.value > maxVal {
			maxVal = d.value
			strength = d.name
		}
		if d.value < minVal {
			minVal = d.value
			weakness = d.name
		}
	}
	return strength, weakness
}

// topNFromCounts returns the top N keys from a count map, sorted by count descending.
func topNFromCounts(counts map[string]int, n int) []string {
	if len(counts) == 0 {
		return nil
	}
	type kv struct {
		key   string
		count int
	}
	sorted := make([]kv, 0, len(counts))
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})
	limit := min(n, len(sorted))
	result := make([]string, limit)
	for i := range limit {
		result[i] = sorted[i].key
	}
	return result
}
