package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// FingerprintKey names the stable identity in partialFingerprints.
const FingerprintKey = "aurumcodePlugin/v1"

// Category is the code scanning category of a plugin's results; the same
// value is the SARIF automationDetails.id prefix.
func Category(name string) string { return "aurumcode/plugin/" + name }

// RuleID is the stable rule of a finding: plugin/<name>/<cwe or class>.
func RuleID(name, class string) string { return "plugin/" + name + "/" + class }

type sarifOut struct {
	Schema  string        `json:"$schema"`
	Version string        `json:"version"`
	Runs    []sarifOutRun `json:"runs"`
}

type sarifOutRun struct {
	Tool struct {
		Driver sarifDriver `json:"driver"`
	} `json:"tool"`
	AutomationDetails struct {
		ID string `json:"id"`
	} `json:"automationDetails"`
	Invocations []sarifInvocation `json:"invocations"`
	Results     []sarifOutResult  `json:"results"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri,omitempty"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string            `json:"id"`
	ShortDescription map[string]string `json:"shortDescription"`
	Properties       map[string]string `json:"properties"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool `json:"executionSuccessful"`
}

type sarifOutResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             map[string]string `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties          map[string]string `json:"properties"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region *struct {
			StartLine int `json:"startLine"`
		} `json:"region,omitempty"`
	} `json:"physicalLocation"`
}

func levelOf(severity string) string {
	switch severity {
	case "critical", "high":
		return "error"
	case "medium":
		return "warning"
	default:
		return "note"
	}
}

// BuildSARIF is the SARIF 2.1.0 document of a conclusive ingest. Every
// string that came from the plugin is redacted field by field.
func BuildSARIF(name, toolVersion string, findings []Finding, filter *redaction.Filter) ([]byte, error) {
	if filter == nil {
		filter = redaction.NewFilter()
	}
	run := sarifOutRun{Invocations: []sarifInvocation{{ExecutionSuccessful: true}}, Results: []sarifOutResult{}}
	run.Tool.Driver = sarifDriver{Name: "AurumCode plugin " + name, Version: toolVersion, Rules: []sarifRule{}}
	run.AutomationDetails.ID = Category(name) + "/"
	rules := map[string]sarifRule{}
	for _, f := range findings {
		rule := RuleID(name, f.Class)
		score := strconv.FormatFloat(f.Score, 'f', 1, 64)
		if _, ok := rules[rule]; !ok {
			rules[rule] = sarifRule{ID: rule, ShortDescription: map[string]string{"text": filter.Redact(f.Class)}, Properties: map[string]string{"security-severity": score}}
		}
		fp := f.Fingerprint
		if fp == "" {
			fp = fallbackFingerprint(rule, f.Path, f.Title)
		}
		var loc sarifLocation
		loc.PhysicalLocation.ArtifactLocation.URI = filter.Redact(f.Path)
		if f.Line > 0 {
			loc.PhysicalLocation.Region = &struct {
				StartLine int `json:"startLine"`
			}{StartLine: f.Line}
		}
		text := filter.Redact(f.Title)
		if f.Message != "" && f.Message != f.Title {
			text += "\n\n" + filter.Redact(f.Message)
		}
		run.Results = append(run.Results, sarifOutResult{
			RuleID:              rule,
			Level:               levelOf(f.Severity),
			Message:             map[string]string{"text": text},
			Locations:           []sarifLocation{loc},
			PartialFingerprints: map[string]string{FingerprintKey: filter.Redact(fp)},
			Properties:          map[string]string{"security-severity": score},
		})
	}
	ids := make([]string, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		run.Tool.Driver.Rules = append(run.Tool.Driver.Rules, rules[id])
	}
	doc := sarifOut{Schema: "https://json.schemastore.org/sarif-2.1.0.json", Version: "2.1.0", Runs: []sarifOutRun{run}}
	return json.MarshalIndent(doc, "", "  ")
}

// writeAtomic writes data to path through a temporary file in the same
// directory, so a failed write never leaves a partial document behind.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".aurumcode-plugin-*.sarif")
	if err != nil {
		return fmt.Errorf("creating SARIF: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("writing SARIF: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("writing SARIF: %w", err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return fmt.Errorf("writing SARIF: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("writing SARIF: %w", err)
	}
	return nil
}
