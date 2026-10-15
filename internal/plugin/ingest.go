package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Options is one ingest: the plugin's name and format, the file it left,
// the scanned tree its paths are relative to, its exit code and where the
// SARIF goes. HarnessSARIF and HarnessErrors are the vvah-findings
// coverage evidence; empty takes the newest *_report.sarif and
// *_errors.jsonl next to Input.
type Options struct {
	Name          string
	Format        string
	Input         string
	Repo          string
	SARIF         string
	PluginExit    int
	ToolVersion   string
	HarnessSARIF  string
	HarnessErrors string
	Filter        *redaction.Filter
}

// UsageError is a malformed invocation (exit 2), never a plugin result.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// Ingest reads a plugin's output and writes the SARIF document only when
// the run is conclusive and every finding's path is inside the tree.
// Otherwise it returns an *InconclusiveError or a *PathError and writes
// nothing.
func Ingest(o Options) (Counts, error) {
	if !namePattern.MatchString(o.Name) {
		return Counts{}, &UsageError{Msg: fmt.Sprintf("invalid plugin name %q", o.Name)}
	}
	if o.SARIF == "" || o.Input == "" || o.Repo == "" {
		return Counts{}, &UsageError{Msg: "--input, --repo and --sarif are required"}
	}
	parse, err := adapterFor(o.Format)
	if err != nil {
		return Counts{}, err
	}
	if o.PluginExit != 0 {
		return Counts{}, inconclusive(ReasonPluginExit)
	}
	data, err := os.ReadFile(o.Input)
	if err != nil {
		return Counts{}, inconclusive(ReasonOutputMissing)
	}
	if o.Format == config.PluginFormatVVAHFindings {
		if err := vvahCoverage(o); err != nil {
			return Counts{}, err
		}
	}
	parsed, err := parse(data)
	if err != nil {
		return Counts{}, err
	}
	excluded := outputDir(o.Repo, o.Input)
	for i := range parsed.Findings {
		clean, reason := checkPath(o.Repo, excluded, parsed.Findings[i].Path)
		if reason != "" {
			return Counts{}, &PathError{Index: i, Reason: reason}
		}
		parsed.Findings[i].Path = clean
	}
	doc, err := BuildSARIF(o.Name, o.ToolVersion, parsed.Findings, o.Filter)
	if err != nil {
		return Counts{}, err
	}
	if err := writeAtomic(o.SARIF, doc); err != nil {
		return Counts{}, err
	}
	parsed.Counts.Accepted = len(parsed.Findings)
	return parsed.Counts, nil
}

func adapterFor(format string) (func([]byte) (Parsed, error), error) {
	switch format {
	case config.PluginFormatSARIF:
		return parseSARIF, nil
	case config.PluginFormatVVAHFindings:
		return parseVVAH, nil
	default:
		return nil, &UsageError{Msg: fmt.Sprintf("unknown format %q (sarif or vvah-findings)", format)}
	}
}

// vvahCoverage checks the harness's SARIF and errors.jsonl.
func vvahCoverage(o Options) error {
	dir := filepath.Dir(o.Input)
	sarifPath := o.HarnessSARIF
	if sarifPath == "" {
		sarifPath = newestMatching(dir, "*_report.sarif")
	}
	if err := harnessSARIFCoverage(sarifPath); err != nil {
		return err
	}
	errorsPath := o.HarnessErrors
	if errorsPath == "" {
		errorsPath = newestMatching(dir, "*_errors.jsonl")
	}
	return harnessErrors(errorsPath)
}
