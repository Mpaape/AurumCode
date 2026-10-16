package plugin

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strconv"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// IssueFinding is one result of the SARIF a conclusive ingest wrote, as the
// issues destination reads it. Every text field is redacted again on read:
// the SARIF file is the boundary, not the ingest that wrote it.
type IssueFinding struct {
	Fingerprint string
	Rule        string
	Path        string
	Line        int
	Score       float64
	Text        string
}

// ErrSARIFInconclusive is a SARIF the issues destination must not act on:
// missing (the ingest was inconclusive and wrote nothing), unreadable, not
// a successful run or not this plugin's category. No issue is touched.
var ErrSARIFInconclusive = errors.New("varredura inconclusiva: nenhuma issue foi tocada")

type issuesSARIF struct {
	Runs []struct {
		AutomationDetails struct {
			ID string `json:"id"`
		} `json:"automationDetails"`
		Invocations []sarifInvocation `json:"invocations"`
		Results     []struct {
			RuleID              string            `json:"ruleId"`
			Message             map[string]string `json:"message"`
			Locations           []sarifLocation   `json:"locations"`
			PartialFingerprints map[string]string `json:"partialFingerprints"`
			Properties          map[string]string `json:"properties"`
		} `json:"results"`
	} `json:"runs"`
}

// LoadIssueFindings reads the plugin's SARIF. It reports conclusive=false
// (and no findings) whenever the document is absent or is not a successful
// run of this plugin: only a conclusive scan may close issues. Findings
// come back deduplicated by fingerprint and sorted, so the plan is stable.
func LoadIssueFindings(path, name string, filter *redaction.Filter) ([]IssueFinding, bool, error) {
	if filter == nil {
		filter = redaction.NewFilter()
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var doc issuesSARIF
	if json.Unmarshal(data, &doc) != nil || len(doc.Runs) != 1 {
		return nil, false, nil
	}
	run := doc.Runs[0]
	if run.AutomationDetails.ID != Category(name)+"/" || len(run.Invocations) == 0 || !run.Invocations[0].ExecutionSuccessful {
		return nil, false, nil
	}
	seen := map[string]bool{}
	out := make([]IssueFinding, 0, len(run.Results))
	for _, r := range run.Results {
		fp := issueFingerprint(r.PartialFingerprints[FingerprintKey])
		if fp == "" || seen[fp] {
			continue
		}
		seen[fp] = true
		f := IssueFinding{Fingerprint: fp, Rule: filter.Redact(r.RuleID), Text: filter.Redact(r.Message["text"])}
		if len(r.Locations) > 0 {
			f.Path = filter.Redact(r.Locations[0].PhysicalLocation.ArtifactLocation.URI)
			if reg := r.Locations[0].PhysicalLocation.Region; reg != nil {
				f.Line = reg.StartLine
			}
		}
		f.Score, _ = strconv.ParseFloat(r.Properties["security-severity"], 64)
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out, true, nil
}
