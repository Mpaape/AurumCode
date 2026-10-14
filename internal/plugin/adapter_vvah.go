package plugin

import (
	"encoding/json"
	"strings"
)

// vvahReport is the part of the agentic harness's findings.json
// (pydantic FinalReport, schema_version 1.x) the ingest reads.
type vvahReport struct {
	SchemaVersion *string           `json:"schema_version"`
	Findings      []vvahRanked      `json:"findings"`
	Degraded      *bool             `json:"degraded"`
	Unreachable   []json.RawMessage `json:"unreachable_files"`
}

type vvahRanked struct {
	Finding  *vvahFinding `json:"finding"`
	Severity string       `json:"severity"`
}

type vvahFinding struct {
	Title          string   `json:"title"`
	File           string   `json:"file"`
	LineStart      int      `json:"line_start"`
	VulnClass      string   `json:"vuln_class"`
	Severity       string   `json:"severity"`
	CWE            *string  `json:"cwe"`
	Impact         string   `json:"impact"`
	Description    string   `json:"description"`
	Recommendation string   `json:"recommendation"`
	CaseID         string   `json:"case_id"`
	Verdict        *string  `json:"verdict"`
	CVSSScore      *float64 `json:"cvss_score"`
	Channel        string   `json:"channel"`
}

// parseVVAH reads findings.json. A missing or other-major schema_version,
// a report that says it is degraded (or does not say whether it is) and a
// malformed document are inconclusive. Only TRUE_POSITIVE findings of the
// main channel become results; every other one is counted.
func parseVVAH(data []byte) (Parsed, error) {
	var r vvahReport
	if err := json.Unmarshal(data, &r); err != nil {
		return Parsed{}, inconclusive(ReasonOutputInvalid)
	}
	if r.SchemaVersion == nil {
		return Parsed{}, inconclusive(ReasonSchemaVersion)
	}
	if major, _, _ := strings.Cut(strings.TrimSpace(*r.SchemaVersion), "."); major != "1" {
		return Parsed{}, inconclusive(ReasonSchemaVersion)
	}
	if r.Degraded == nil || *r.Degraded {
		return Parsed{}, inconclusive(ReasonDegraded)
	}
	var out Parsed
	for _, rf := range r.Findings {
		f := rf.Finding
		if f == nil {
			return Parsed{}, inconclusive(ReasonOutputInvalid)
		}
		if ch := strings.ToLower(strings.TrimSpace(f.Channel)); ch != "" && ch != "main" {
			out.Counts.OtherChannel++
			continue
		}
		switch verdict(f.Verdict) {
		case "TRUE_POSITIVE":
		case "FALSE_POSITIVE":
			out.Counts.FalsePositive++
			continue
		default:
			out.Counts.NoVerdict++
			continue
		}
		out.Findings = append(out.Findings, vvahToFinding(rf.Severity, f))
	}
	return out, nil
}

func verdict(v *string) string {
	if v == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(*v))
}

func vvahToFinding(rankedSeverity string, f *vvahFinding) Finding {
	sev := rankedSeverity
	if strings.TrimSpace(sev) == "" {
		sev = f.Severity
	}
	severity := normalizeSeverity(sev)
	score := defaultScore(severity)
	if f.CVSSScore != nil && *f.CVSSScore >= 0 && *f.CVSSScore <= 10 {
		score = *f.CVSSScore
	}
	cwe := ""
	if f.CWE != nil {
		cwe = *f.CWE
	}
	msg := strings.TrimSpace(f.Description)
	if impact := strings.TrimSpace(f.Impact); impact != "" {
		msg += "\n\nImpacto: " + impact
	}
	if rec := strings.TrimSpace(f.Recommendation); rec != "" {
		msg += "\n\nRecomendação: " + rec
	}
	if strings.TrimSpace(msg) == "" {
		msg = f.Title
	}
	return Finding{
		Class:       normalizeClass(cwe, f.VulnClass),
		Title:       f.Title,
		Message:     strings.TrimSpace(msg),
		Path:        f.File,
		Line:        f.LineStart,
		Severity:    severity,
		Score:       score,
		Fingerprint: strings.TrimSpace(f.CaseID),
	}
}
