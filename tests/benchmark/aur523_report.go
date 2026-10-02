package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// MLLanguageRow is the per-language line of the report.
type MLLanguageRow struct {
	Language             string   `json:"language"`
	Cases                int      `json:"cases"`
	DefectCases          int      `json:"defect_cases"`
	CleanCases           int      `json:"clean_cases"`
	Defects              int      `json:"defects"`
	Detected             int      `json:"detected"`
	Missed               int      `json:"missed"`
	Findings             int      `json:"findings"`
	FalsePositives       int      `json:"false_positives"`
	Recall               float64  `json:"recall"`
	RecallInterval       Interval `json:"recall_interval_95"`
	Precision            float64  `json:"precision"`
	PrecisionInterval    Interval `json:"precision_interval_95"`
	ApprovedWithDefect   int      `json:"approved_with_defect"`
	ApprovedWithDefectID []string `json:"approved_with_defect_cases"`
}

// MLReport is the whole report. It carries no timestamp, path or duration, so
// the same corpus digest and policy digest give the same bytes.
type MLReport struct {
	Schema       string          `json:"schema"`
	Mode         string          `json:"mode"`
	Note         string          `json:"note"`
	CorpusSHA256 string          `json:"corpus_sha256"`
	PolicySHA256 string          `json:"policy_sha256"`
	Languages    []MLLanguageRow `json:"languages"`
	Total        MLLanguageRow   `json:"total"`
}

const mlNote = "Model stand-in is a deterministic fake provider (fixture derived from each case label); " +
	"verdict and findings are read from the real binary's SARIF and audit output. " +
	"A run with a real model is the owner's decision and spend, not part of this report."

func ruleClass(id string) string {
	if i := strings.LastIndexAny(id, "#/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func round4(v float64) float64 { return math.Round(v*1e4) / 1e4 }

func roundInterval(i Interval) Interval { return Interval{Low: round4(i.Low), High: round4(i.High)} }

// CaseResult is one scored case.
type CaseResult struct {
	Case    MLCase
	Outcome Outcome
}

func (r *MLLanguageRow) add(res CaseResult) {
	c, o := res.Case, res.Outcome
	r.Cases++
	var defects []Defect
	if c.Label == LabelDefect {
		r.DefectCases++
		defects = []Defect{{ID: c.ID, File: c.File, Line: c.Line, Category: c.Rule}}
	} else {
		r.CleanCases++
	}
	// The rule class of a finding is the id after its namespace
	// ("seguranca#sql-injection" and the built-in "analysis/sql-injection"
	// are the same class). A finding that sits on the defect but is of
	// another class is not a detection of this defect: it is scored as
	// noise. Findings of the same class on the same line, from the policy
	// skill and from the built-in analysis, are one finding, not a duplicate.
	findings := make([]Finding, 0, len(o.Findings))
	seen := map[string]bool{}
	for _, f := range o.Findings {
		class := ruleClass(f.RuleID)
		key := fmt.Sprintf("%s:%d:%s", f.File, f.Line, class)
		if seen[key] {
			continue
		}
		seen[key] = true
		if c.Label == LabelDefect && class != c.Rule {
			f.File = "(wrong-rule)"
		}
		findings = append(findings, f)
	}
	m := Evaluate(findings, defects, DefaultLineTolerance)
	r.Defects += m.TotalDefects
	r.Detected += m.Matched
	r.Missed += m.Missed
	r.Findings += m.TotalFindings
	r.FalsePositives += m.Noise + m.Duplicates
	if c.Label == LabelDefect && o.Approved() {
		r.ApprovedWithDefect++
		r.ApprovedWithDefectID = append(r.ApprovedWithDefectID, c.ID)
	}
}

func (r *MLLanguageRow) finish() {
	tp := r.Detected
	r.Recall = round4(ratio(tp, r.Defects))
	r.RecallInterval = roundInterval(Wilson(tp, r.Defects))
	r.Precision = round4(ratio(tp, tp+r.FalsePositives))
	r.PrecisionInterval = roundInterval(Wilson(tp, tp+r.FalsePositives))
	sort.Strings(r.ApprovedWithDefectID)
	if r.ApprovedWithDefectID == nil {
		r.ApprovedWithDefectID = []string{}
	}
}

// BuildMLReport aggregates scored cases per language (sorted) and in total.
func BuildMLReport(c *MLCorpus, results []CaseResult) MLReport {
	rep := MLReport{Schema: "aurum.benchmark-multilang-report/1", Mode: MultilangMode, Note: mlNote,
		CorpusSHA256: c.CorpusSHA256, PolicySHA256: c.PolicySHA256, Total: MLLanguageRow{Language: "total"}}
	rows := map[string]*MLLanguageRow{}
	for _, l := range c.Languages() {
		rows[l] = &MLLanguageRow{Language: l}
	}
	sorted := append([]CaseResult(nil), results...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Case.ID < sorted[j].Case.ID })
	for _, res := range sorted {
		rows[res.Case.Language].add(res)
		rep.Total.add(res)
	}
	for _, l := range c.Languages() {
		rows[l].finish()
		rep.Languages = append(rep.Languages, *rows[l])
	}
	rep.Total.finish()
	return rep
}

// JSON renders the report deterministically.
func (r MLReport) JSON() ([]byte, error) {
	raw, err := json.MarshalIndent(r, "", "  ")
	return append(raw, '\n'), err
}

// Markdown renders the per-language table, digests in the header.
func (r MLReport) Markdown() []byte {
	var b strings.Builder
	b.WriteString("<!-- Generated by the AUR-523 harness. Never edit by hand: regenerate it. -->\n")
	b.WriteString("# Multi-language gate benchmark (AUR-523)\n\n")
	fmt.Fprintf(&b, "- mode: `%s`\n- corpus sha256: `%s`\n- policy sha256: `%s`\n\n%s\n\n", r.Mode, r.CorpusSHA256, r.PolicySHA256, r.Note)
	b.WriteString("| language | cases | defects | detected | recall | recall 95% CI | precision | precision 95% CI | false positives | approved with defect |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	row := func(x MLLanguageRow) {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %.4f | [%.4f, %.4f] | %.4f | [%.4f, %.4f] | %d | %d |\n",
			x.Language, x.Cases, x.Defects, x.Detected, x.Recall, x.RecallInterval.Low, x.RecallInterval.High,
			x.Precision, x.PrecisionInterval.Low, x.PrecisionInterval.High, x.FalsePositives, x.ApprovedWithDefect)
	}
	for _, x := range r.Languages {
		row(x)
	}
	row(r.Total)
	return []byte(b.String())
}
