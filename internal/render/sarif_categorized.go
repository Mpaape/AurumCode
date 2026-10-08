package render

import (
	"encoding/json"
	"os"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// A categorized SARIF document names its analysis category inside the run
// (automationDetails.id): code scanning keeps the alerts of each category
// apart, so a scheduled scan's results never close or reopen the pull
// request review's, and a result absent from the category's next upload is
// closed by code scanning itself.

type sarifCategorizedLog struct {
	Schema  string                `json:"$schema"`
	Version string                `json:"version"`
	Runs    []sarifCategorizedRun `json:"runs"`
}

type sarifCategorizedRun struct {
	Tool              sarifTool              `json:"tool"`
	AutomationDetails sarifAutomationDetails `json:"automationDetails"`
	Invocations       []sarifInvocation      `json:"invocations"`
	Results           []sarifResult          `json:"results"`
}

type sarifAutomationDetails struct {
	ID string `json:"id"`
}

// BuildCategorizedSARIF is the redacted SARIF document of a conclusive run
// whose results belong to category. The results and their fingerprints are
// the same as BuildSARIFLog's: a finding's partialFingerprints derive from
// rule, path, line and Context only, so a caller that wants an identity
// stable across runs keeps those stable.
func BuildCategorizedSARIF(toolVersion, category string, findings []SARIFFinding, filter *redaction.Filter) ([]byte, error) {
	if filter == nil {
		filter = redaction.NewFilter()
	}
	log := redactSARIFLog(filter, BuildSARIFLog(toolVersion, redactSARIFFindings(filter, findings), ""))
	out := sarifCategorizedLog{Schema: log.Schema, Version: log.Version}
	for _, run := range log.Runs {
		results := run.Results
		if results == nil {
			results = []sarifResult{}
		}
		out.Runs = append(out.Runs, sarifCategorizedRun{
			Tool:              run.Tool,
			AutomationDetails: sarifAutomationDetails{ID: category + "/"},
			Invocations:       run.Invocations,
			Results:           results,
		})
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return []byte(filter.Redact(string(data)) + "\n"), nil
}

// WriteCategorizedSARIF writes BuildCategorizedSARIF's document to path.
func WriteCategorizedSARIF(path, toolVersion, category string, findings []SARIFFinding, filter *redaction.Filter) error {
	data, err := BuildCategorizedSARIF(toolVersion, category, findings, filter)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
