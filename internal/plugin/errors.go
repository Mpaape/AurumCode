package plugin

import "fmt"

// The reasons of an inconclusive ingest. They are stable identifiers, safe
// to print in a public log: none carries text from the plugin's output.
const (
	ReasonPluginExit      = "plugin_exit"
	ReasonOutputMissing   = "output_missing"
	ReasonOutputInvalid   = "output_invalid"
	ReasonSchemaVersion   = "schema_version"
	ReasonDegraded        = "degraded"
	ReasonExecutionFailed = "execution_failed"
	ReasonHollow          = "coverage_lost"
	ReasonAnalysisErrors  = "analysis_errors"
)

// InconclusiveError is an ingest that must not write a SARIF document: the
// open alerts stay as they are.
type InconclusiveError struct {
	Reason string
}

func (e *InconclusiveError) Error() string {
	return "inconclusive: " + e.Reason
}

func inconclusive(reason string) error { return &InconclusiveError{Reason: reason} }

// PathError is a finding whose location is outside the scanned tree. The
// ingest fails: a finding is never dropped in silence. The message names
// only the finding's position and the class of the problem, never the path
// itself (the log is public).
type PathError struct {
	Index  int
	Reason string
}

func (e *PathError) Error() string {
	return fmt.Sprintf("finding #%d: path refused (%s)", e.Index+1, e.Reason)
}
