// AUR-521: the SARIF 2.1.0 document a policy-governed review writes
// alongside its audit record (audit.go), for a workflow to upload to
// GitHub's code scanning. The structs below carry only the fields AC-002
// requires; no schema is downloaded at runtime -- TestAUR521SARIF* in
// sarif_test.go validates the required 2.1.0 shape directly against this
// package's own encoding.
package render

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// SARIFVersion is the single SARIF version this writer ever produces.
const SARIFVersion = "2.1.0"

// sarifSchema is the canonical schema URI named in every SARIF 2.1.0
// document; it is descriptive metadata only -- this package never fetches
// it.
const sarifSchema = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"

// FindingFingerprintKey is the stable key this writer uses inside a SARIF
// result's partialFingerprints object.
const FindingFingerprintKey = "aurumcode/findingId/v1"

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Invocations []sarifInvocation `json:"invocations"`
	Results     []sarifResult     `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name    string      `json:"name"`
	Version string      `json:"version"`
	Rules   []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name,omitempty"`
	ShortDescription sarifMessage `json:"shortDescription"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifInvocation struct {
	ExecutionSuccessful        bool                `json:"executionSuccessful"`
	ToolExecutionNotifications []sarifNotification `json:"toolExecutionNotifications,omitempty"`
}

type sarifNotification struct {
	Message sarifMessage `json:"message"`
	Level   string       `json:"level,omitempty"`
}

type sarifResult struct {
	RuleID              string             `json:"ruleId"`
	Level               string             `json:"level"`
	Message             sarifMessage       `json:"message"`
	Locations           []sarifLocation    `json:"locations"`
	PartialFingerprints map[string]string  `json:"partialFingerprints,omitempty"`
	Suppressions        []sarifSuppression `json:"suppressions,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	// Region is a pointer so a finding with no usable line number (Line <
	// 1 -- a general/file-level finding) omits it entirely: SARIF 2.1.0
	// requires region.startLine >= 1 when region is present at all, and
	// github/codeql-action/upload-sarif rejects a document that violates
	// that (AC-002).
	Region *sarifRegion `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// sarifSuppression's Kind is always "external": an AUR-520 exception is
// decided by policy configuration outside the reviewed source tree, never
// by an in-source suppression comment (SARIF's own "inSource" kind).
type sarifSuppression struct {
	Kind          string `json:"kind"`
	Justification string `json:"justification,omitempty"`
}

// SARIFFinding is one finding to render into the SARIF document. Context is
// the normalized code line/hunk content at Line -- the same material
// FindingFingerprint hashes -- never the free-text Message alone.
// Suppressed/Justification are AUR-520's own, synthetic until that card
// lands: this writer already renders them into SARIF's "suppressions" shape
// whenever a caller sets them (AC-003).
type SARIFFinding struct {
	RuleID        string
	RuleTitle     string
	Path          string
	Line          int
	Severity      string
	Message       string
	Context       string
	Suppressed    bool
	Justification string
}

// severityToSARIFLevel maps this system's three issue severities onto
// SARIF's result level vocabulary (AC-002: "regra, severidade, arquivo,
// linha"). Unrecognized input maps to "warning" rather than silently
// dropping the result.
func severityToSARIFLevel(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "error":
		return "error"
	case "warning":
		return "warning"
	case "info":
		return "note"
	default:
		return "warning"
	}
}

// BuildSARIFLog assembles the complete SARIF document. executionSuccessful
// is AC-004's own invocation flag: false marks the run inconclusive, and
// notificationReason (non-empty exactly when executionSuccessful is false)
// becomes the one toolExecutionNotification naming why.
func BuildSARIFLog(toolVersion string, findings []SARIFFinding, executionSuccessful bool, notificationReason string) sarifLog {
	rules := make([]sarifRule, 0, len(findings))
	seenRules := map[string]bool{}
	results := make([]sarifResult, 0, len(findings))

	for _, f := range findings {
		if f.RuleID != "" && !seenRules[f.RuleID] {
			seenRules[f.RuleID] = true
			title := strings.TrimSpace(f.RuleTitle)
			if title == "" {
				title = f.RuleID
			}
			rules = append(rules, sarifRule{
				ID:               f.RuleID,
				Name:             f.RuleID,
				ShortDescription: sarifMessage{Text: title},
			})
		}

		fingerprint := FindingFingerprint(FindingIdentity{
			RuleID:  f.RuleID,
			Path:    f.Path,
			Line:    f.Line,
			Context: f.Context,
		})

		physical := sarifPhysicalLocation{
			ArtifactLocation: sarifArtifactLocation{URI: normalizeFindingPath(f.Path)},
		}
		if f.Line >= 1 {
			physical.Region = &sarifRegion{StartLine: f.Line}
		}
		result := sarifResult{
			RuleID:    f.RuleID,
			Level:     severityToSARIFLevel(f.Severity),
			Message:   sarifMessage{Text: f.Message},
			Locations: []sarifLocation{{PhysicalLocation: physical}},
			PartialFingerprints: map[string]string{
				FindingFingerprintKey: fingerprint,
			},
		}
		if f.Suppressed {
			result.Suppressions = []sarifSuppression{{
				Kind:          "external",
				Justification: f.Justification,
			}}
		}
		results = append(results, result)
	}

	invocation := sarifInvocation{ExecutionSuccessful: executionSuccessful}
	if !executionSuccessful && strings.TrimSpace(notificationReason) != "" {
		invocation.ToolExecutionNotifications = []sarifNotification{{
			Message: sarifMessage{Text: notificationReason},
			Level:   "error",
		}}
	}

	return sarifLog{
		Schema:  sarifSchema,
		Version: SARIFVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:    "AurumCode",
				Version: toolVersion,
				Rules:   rules,
			}},
			Invocations: []sarifInvocation{invocation},
			Results:     results,
		}},
	}
}

// redactSARIFLog runs every individual string field of log through filter
// BEFORE it is ever marshaled to JSON -- the same fix, and for the same
// reason, as redactAuditRecord in audit.go: redacting only the final,
// marshaled JSON text misses a secret whose quote, backslash or embedded
// newline json.Marshal escaped into a different byte sequence than the one
// the redaction regexes match. Builds entirely new slices/structs; never
// mutates the caller's own log in place.
func redactSARIFLog(filter *redaction.Filter, log sarifLog) sarifLog {
	out := log
	runs := make([]sarifRun, len(log.Runs))
	for ri, run := range log.Runs {
		driver := run.Tool.Driver
		driver.Name = filter.Redact(driver.Name)
		driver.Version = filter.Redact(driver.Version)
		rules := make([]sarifRule, len(driver.Rules))
		for i, r := range driver.Rules {
			rules[i] = sarifRule{
				ID:               filter.Redact(r.ID),
				Name:             filter.Redact(r.Name),
				ShortDescription: sarifMessage{Text: filter.Redact(r.ShortDescription.Text)},
			}
		}
		driver.Rules = rules
		run.Tool.Driver = driver

		invocations := make([]sarifInvocation, len(run.Invocations))
		for i, inv := range run.Invocations {
			notifications := make([]sarifNotification, len(inv.ToolExecutionNotifications))
			for j, n := range inv.ToolExecutionNotifications {
				notifications[j] = sarifNotification{
					Message: sarifMessage{Text: filter.Redact(n.Message.Text)},
					Level:   filter.Redact(n.Level),
				}
			}
			invocations[i] = sarifInvocation{
				ExecutionSuccessful:        inv.ExecutionSuccessful,
				ToolExecutionNotifications: notifications,
			}
		}
		run.Invocations = invocations

		results := make([]sarifResult, len(run.Results))
		for i, res := range run.Results {
			locations := make([]sarifLocation, len(res.Locations))
			for j, loc := range res.Locations {
				physical := loc.PhysicalLocation
				physical.ArtifactLocation = sarifArtifactLocation{URI: filter.Redact(physical.ArtifactLocation.URI)}
				if physical.Region != nil {
					region := *physical.Region
					physical.Region = &region
				}
				locations[j] = sarifLocation{PhysicalLocation: physical}
			}
			var fingerprints map[string]string
			if res.PartialFingerprints != nil {
				fingerprints = make(map[string]string, len(res.PartialFingerprints))
				for k, v := range res.PartialFingerprints {
					fingerprints[filter.Redact(k)] = v
				}
			}
			var suppressions []sarifSuppression
			if res.Suppressions != nil {
				suppressions = make([]sarifSuppression, len(res.Suppressions))
				for j, s := range res.Suppressions {
					suppressions[j] = sarifSuppression{
						Kind:          filter.Redact(s.Kind),
						Justification: filter.Redact(s.Justification),
					}
				}
			}
			results[i] = sarifResult{
				RuleID:              filter.Redact(res.RuleID),
				Level:               filter.Redact(res.Level),
				Message:             sarifMessage{Text: filter.Redact(res.Message.Text)},
				Locations:           locations,
				PartialFingerprints: fingerprints,
				Suppressions:        suppressions,
			}
		}
		run.Results = results
		runs[ri] = run
	}
	out.Runs = runs
	return out
}

// redactSARIFFindings redacts every string field of each finding BEFORE
// BuildSARIFLog ever sees them. This matters for Path specifically:
// BuildSARIFLog derives artifactLocation.uri via normalizeFindingPath,
// which replaces every literal backslash in the path with a forward slash
// -- so a secret containing a backslash, if redacted only AFTER that
// normalization (as redactSARIFLog alone would), no longer matches the
// filter's registered pattern (the backslash it was registered with is
// already gone) and leaks into artifactLocation.uri unredacted. Redacting
// the raw Path here, first, means normalizeFindingPath only ever sees the
// filter's own marker text, which contains no backslash to mangle.
func redactSARIFFindings(filter *redaction.Filter, findings []SARIFFinding) []SARIFFinding {
	out := make([]SARIFFinding, len(findings))
	for i, f := range findings {
		out[i] = SARIFFinding{
			RuleID:        filter.Redact(f.RuleID),
			RuleTitle:     filter.Redact(f.RuleTitle),
			Path:          filter.Redact(f.Path),
			Line:          f.Line,
			Severity:      filter.Redact(f.Severity),
			Message:       filter.Redact(f.Message),
			Context:       filter.Redact(f.Context),
			Suppressed:    f.Suppressed,
			Justification: filter.Redact(f.Justification),
		}
	}
	return out
}

// WriteSARIF redacts every finding's own fields first (redactSARIFFindings
// -- before BuildSARIFLog's own path normalization ever runs), redacts
// every string field of the built document a second time
// (redactSARIFLog), marshals the result as indented JSON, then runs the
// complete text through filter a THIRD time (AUR-009, AC-005 -- the single
// redaction filter every sink in this system writes through) before
// writing it to path. Three passes, not one: each catches a shape the
// others cannot (pre-normalization, post-build structured fields,
// post-marshal escaped text).
func WriteSARIF(path, toolVersion string, findings []SARIFFinding, executionSuccessful bool, notificationReason string, filter *redaction.Filter) error {
	log := BuildSARIFLog(toolVersion, redactSARIFFindings(filter, findings), executionSuccessful, notificationReason)
	data, err := json.MarshalIndent(redactSARIFLog(filter, log), "", "  ")
	if err != nil {
		return err
	}
	redacted := filter.Redact(string(data))
	return os.WriteFile(path, []byte(redacted+"\n"), 0o600)
}
