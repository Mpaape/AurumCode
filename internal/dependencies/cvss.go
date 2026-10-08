package dependencies

import (
	"errors"
	"math"
	"strings"
)

// cvss3Weights are the CVSS v3.x base metric weights of the FIRST
// specification (https://www.first.org/cvss/v3.1/specification-document,
// section 7.4). They are the standard's own constants, not a policy.
var cvss3Weights = map[string]map[string]float64{
	"AV": {"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2},
	"AC": {"L": 0.77, "H": 0.44},
	"UI": {"N": 0.85, "R": 0.62},
	"C":  {"H": 0.56, "L": 0.22, "N": 0},
	"I":  {"H": 0.56, "L": 0.22, "N": 0},
	"A":  {"H": 0.56, "L": 0.22, "N": 0},
}

// cvss3Privileges is PR's weight, which depends on the scope.
var cvss3Privileges = map[bool]map[string]float64{
	false: {"N": 0.85, "L": 0.62, "H": 0.27},
	true:  {"N": 0.85, "L": 0.68, "H": 0.5},
}

var errCVSS = errors.New("dependencies: unreadable CVSS v3 vector")

// CVSS3Score computes the base score of a CVSS v3.0/v3.1 vector
// ("CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"). A vector missing a base
// metric or carrying an unknown value is an error, never a guessed score.
func CVSS3Score(vector string) (float64, error) {
	parts := strings.Split(strings.TrimSpace(vector), "/")
	if len(parts) < 9 || !strings.HasPrefix(parts[0], "CVSS:3.") {
		return 0, errCVSS
	}
	metrics := map[string]string{}
	for _, p := range parts[1:] {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			return 0, errCVSS
		}
		metrics[kv[0]] = kv[1]
	}
	scope, ok := map[string]bool{"U": false, "C": true}[metrics["S"]]
	if !ok {
		return 0, errCVSS
	}
	w := map[string]float64{}
	for metric, values := range cvss3Weights {
		v, ok := values[metrics[metric]]
		if !ok {
			return 0, errCVSS
		}
		w[metric] = v
	}
	pr, ok := cvss3Privileges[scope][metrics["PR"]]
	if !ok {
		return 0, errCVSS
	}
	iss := 1 - (1-w["C"])*(1-w["I"])*(1-w["A"])
	impact := 6.42 * iss
	if scope {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	}
	if impact <= 0 {
		return 0, nil
	}
	exploitability := 8.22 * w["AV"] * w["AC"] * pr * w["UI"]
	if scope {
		return roundUp(math.Min(1.08*(impact+exploitability), 10)), nil
	}
	return roundUp(math.Min(impact+exploitability, 10)), nil
}

// roundUp is the specification's Roundup (appendix A): the smallest number
// with one decimal at or above x, computed on integers to avoid floating
// point surprises.
func roundUp(x float64) float64 {
	n := int64(math.Round(x * 100000))
	if n%10000 == 0 {
		return float64(n) / 100000
	}
	return float64(n/10000+1) / 10
}

// SeverityOfScore maps a CVSS base score onto the qualitative rating of the
// specification (section 5): 0.1-3.9 low, 4.0-6.9 medium, 7.0-8.9 high,
// 9.0-10.0 critical; 0.0 (none) reads as low.
func SeverityOfScore(score float64) string {
	switch {
	case score >= 9:
		return "critical"
	case score >= 7:
		return "high"
	case score >= 4:
		return "medium"
	default:
		return "low"
	}
}

// severityFromCVSS reads the highest severity of the CVSS v3 entries. CVSS
// v4 vectors are not scored here (the v4 score needs the specification's
// macro-vector table); an advisory with only a v4 vector stays unknown,
// which the gate counts as failing.
func severityFromCVSS(entries []osvSeverity) string {
	best := -1.0
	for _, e := range entries {
		if !strings.EqualFold(e.Type, "CVSS_V3") {
			continue
		}
		if score, err := CVSS3Score(e.Score); err == nil && score > best {
			best = score
		}
	}
	if best < 0 {
		return SeverityUnknown
	}
	return SeverityOfScore(best)
}
