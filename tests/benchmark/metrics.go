package benchmark

import "math"

const DefaultLineTolerance = 2

type Finding struct {
	CaseID   string   `json:"case_id"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Severity Severity `json:"severity"`
	RuleID   string   `json:"rule_id"`
	Message  string   `json:"message"`
	Patch    string   `json:"patch,omitempty"`
}

type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

type Metrics struct {
	TotalDefects     int      `json:"total_defects"`
	Matched          int      `json:"matched"`
	Missed           int      `json:"missed"`
	Noise            int      `json:"noise"`
	Duplicates       int      `json:"duplicates"`
	TotalFindings    int      `json:"total_findings"`
	ExactLocation    int      `json:"exact_location"`
	Precision        float64  `json:"precision"`
	Recall           float64  `json:"recall"`
	LocationAccuracy float64  `json:"location_accuracy"`
	PrecisionRange   Interval `json:"precision_interval"`
	RecallRange      Interval `json:"recall_interval"`
}

// Evaluate scores findings against a case's frozen defects. A finding matches
// a defect on the same file within the line tolerance. The first finding to
// match a defect is the true positive; a later finding on the same defect is a
// duplicate and a finding that matches nothing is noise. Both duplicates and
// noise count against precision, so padding a review with repeats or invented
// findings can never raise the score.
func Evaluate(findings []Finding, defects []Defect, tolerance int) Metrics {
	if tolerance < 0 {
		tolerance = 0
	}
	m := Metrics{TotalDefects: len(defects), TotalFindings: len(findings)}
	matched := make([]bool, len(defects))
	exact := 0
	for _, f := range findings {
		idx := -1
		for j := range defects {
			if defects[j].File != f.File || abs(defects[j].Line-f.Line) > tolerance {
				continue
			}
			if !matched[j] {
				idx = j
				break
			}
		}
		if idx == -1 {
			duplicate := false
			for j := range defects {
				if defects[j].File == f.File && abs(defects[j].Line-f.Line) <= tolerance && matched[j] {
					duplicate = true
					break
				}
			}
			if duplicate {
				m.Duplicates++
			} else {
				m.Noise++
			}
			continue
		}
		matched[idx] = true
		m.Matched++
		if defects[idx].Line == f.Line {
			exact++
		}
	}
	for _, ok := range matched {
		if !ok {
			m.Missed++
		}
	}
	m.ExactLocation = exact
	tp := m.Matched
	fp := m.Noise + m.Duplicates
	m.Precision = ratio(tp, tp+fp)
	m.Recall = ratio(tp, m.TotalDefects)
	m.LocationAccuracy = ratio(exact, m.Matched)
	m.PrecisionRange = Wilson(tp, tp+fp)
	m.RecallRange = Wilson(tp, m.TotalDefects)
	return m
}

func Aggregate(metrics []Metrics) Metrics {
	sum := Metrics{}
	for _, m := range metrics {
		sum.TotalDefects += m.TotalDefects
		sum.Matched += m.Matched
		sum.Missed += m.Missed
		sum.Noise += m.Noise
		sum.Duplicates += m.Duplicates
		sum.TotalFindings += m.TotalFindings
		sum.ExactLocation += m.ExactLocation
	}
	tp := sum.Matched
	fp := sum.Noise + sum.Duplicates
	sum.Precision = ratio(tp, tp+fp)
	sum.Recall = ratio(tp, sum.TotalDefects)
	sum.LocationAccuracy = ratio(sum.ExactLocation, sum.Matched)
	sum.PrecisionRange = Wilson(tp, tp+fp)
	sum.RecallRange = Wilson(tp, sum.TotalDefects)
	return sum
}

// Wilson returns the 95% Wilson score interval for a proportion. It is
// reported alongside every sample so a small run is never read as a precise
// estimate.
func Wilson(success, n int) Interval {
	if n <= 0 {
		return Interval{}
	}
	const z = 1.959963984540054
	p := float64(success) / float64(n)
	denom := 1 + z*z/float64(n)
	centre := p + z*z/(2*float64(n))
	margin := z * math.Sqrt((p*(1-p)+z*z/(4*float64(n)))/float64(n))
	low := (centre - margin) / denom
	high := (centre + margin) / denom
	if low < 0 {
		low = 0
	}
	if high > 1 {
		high = 1
	}
	return Interval{Low: low, High: high}
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
