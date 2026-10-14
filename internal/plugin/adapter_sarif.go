package plugin

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// sarifIn is the part of a plugin's SARIF 2.1.0 document the ingest reads.
type sarifIn struct {
	Version *string    `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Properties  map[string]any `json:"properties"`
	Invocations []struct {
		ExecutionSuccessful *bool `json:"executionSuccessful"`
	} `json:"invocations"`
	Results []sarifResultIn `json:"results"`
}

type sarifResultIn struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine int `json:"startLine"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties          map[string]any    `json:"properties"`
}

// parseSARIF reads a generic SARIF 2.1.0 document. A run that reports
// executionSuccessful: false, hollow or coverageDegraded is inconclusive, as
// is a document without runs or of another version.
func parseSARIF(data []byte) (Parsed, error) { return parseSARIFTolerating(data, nil) }

// parseSARIFTolerating is parseSARIF where a coverageDegraded run whose
// coverageDegradedReasons are ALL in tolerated (and not empty) stays
// conclusive.
func parseSARIFTolerating(data []byte, tolerated map[string]bool) (Parsed, error) {
	var doc sarifIn
	if err := json.Unmarshal(data, &doc); err != nil {
		return Parsed{}, inconclusive(ReasonOutputInvalid)
	}
	if doc.Version == nil || *doc.Version != "2.1.0" {
		return Parsed{}, inconclusive(ReasonSchemaVersion)
	}
	if len(doc.Runs) == 0 {
		return Parsed{}, inconclusive(ReasonOutputInvalid)
	}
	var out Parsed
	for _, run := range doc.Runs {
		if reason := runInconclusive(run, tolerated); reason != "" {
			return Parsed{}, inconclusive(reason)
		}
		for _, r := range run.Results {
			out.Findings = append(out.Findings, sarifToFinding(r))
		}
	}
	return out, nil
}

func runInconclusive(run sarifRun, tolerated map[string]bool) string {
	for _, inv := range run.Invocations {
		if inv.ExecutionSuccessful != nil && !*inv.ExecutionSuccessful {
			return ReasonExecutionFailed
		}
	}
	if truthy(run.Properties["hollow"]) {
		return ReasonHollow
	}
	if truthy(run.Properties["coverageDegraded"]) && !allTolerated(run.Properties["coverageDegradedReasons"], tolerated) {
		return ReasonHollow
	}
	return ""
}

// allTolerated: reasons is a non-empty list of strings, each in tolerated.
func allTolerated(reasons any, tolerated map[string]bool) bool {
	list, ok := reasons.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, r := range list {
		s, ok := r.(string)
		if !ok || !tolerated[s] {
			return false
		}
	}
	return true
}

func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func sarifToFinding(r sarifResultIn) Finding {
	f := Finding{Class: normalizeClass("", r.RuleID), Title: r.RuleID, Message: r.Message.Text}
	if len(r.Locations) > 0 {
		f.Path = r.Locations[0].PhysicalLocation.ArtifactLocation.URI
		f.Line = r.Locations[0].PhysicalLocation.Region.StartLine
	}
	switch strings.ToLower(r.Level) {
	case "error":
		f.Severity = "high"
	case "note", "none":
		f.Severity = "low"
	default:
		f.Severity = "medium"
	}
	f.Score = defaultScore(f.Severity)
	if s, ok := r.Properties["security-severity"].(string); ok {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v >= 0 && v <= 10 {
			f.Score = v
			f.Severity = severityOfScore(v)
		}
	}
	keys := make([]string, 0, len(r.PartialFingerprints))
	for k := range r.PartialFingerprints {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		f.Fingerprint = r.PartialFingerprints[keys[0]]
	}
	return f
}

// severityOfScore follows code scanning's security-severity bands.
func severityOfScore(v float64) string {
	switch {
	case v >= 9.0:
		return "critical"
	case v >= 7.0:
		return "high"
	case v >= 4.0:
		return "medium"
	case v > 0:
		return "low"
	default:
		return "info"
	}
}
