package gitleaks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

const (
	// gitBinary verifies the range before gitleaks sees it.
	gitBinary = "git"
	// ignoreFile is the fingerprint list gitleaks always reads from the
	// scanned root (measured on the pinned image: no flag disables it).
	ignoreFile = ".gitleaksignore"
	// reportFile and configFile live in a private temporary directory.
	reportFile = "report.json"
	configFile = "pinned.toml"
	// pinnedConfig selects gitleaks' own bundled rule base. Passed with
	// --config it outranks GITLEAKS_CONFIG, GITLEAKS_CONFIG_TOML and a
	// .gitleaks.toml at the scanned root (gitleaks' documented precedence),
	// so the rule base is always the pinned one.
	pinnedConfig = "[extend]\nuseDefault = true\n"
)

// The names of the range ends in an error.
const (
	endBase = "base"
	endHead = "head"
)

var (
	// ErrNoRange: the caller gave no commit range. Scanning the final tree
	// instead would miss a secret added and removed inside the range.
	ErrNoRange = errors.New("gitleaks: no reviewed commit range")
	// ErrBadRange: a range end is not a full commit id, is absent from the
	// repository, or the repository is shallow (the range would be cut).
	ErrBadRange = errors.New("gitleaks: reviewed commit range is not scannable")
	// ErrVersionMismatch: the binary is not the pinned version, so its
	// bundled rule base is not the pinned one either.
	ErrVersionMismatch = errors.New("gitleaks: binary is not the pinned version")
	// ErrLoggedError: gitleaks logged an error and still exited 0 with an
	// empty report (measured: an unknown revision in --log-opts does this).
	ErrLoggedError = errors.New("gitleaks: the scan logged an error")

	commitID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	// loggedError matches gitleaks' own error and fatal log levels. With
	// --no-color the pinned binary logs them plain (measured); ansiColor is
	// stripped first so a colored log still matches.
	loggedError = regexp.MustCompile(`(?m)\b(?:ERR|FTL)\b`)
	ansiColor   = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

// binaryVersion runs `gitleaks version` and refuses any version but the
// pinned one. A missing binary keeps exec.ErrNotFound for the executor.
func binaryVersion(ctx context.Context, run scanner.Command, root string) (string, error) {
	stdout, _, err := run(ctx, root, binary, "version")
	if err != nil {
		return "", fmt.Errorf("gitleaks: version: %w", err)
	}
	version := strings.TrimSpace(stdout)
	if version != PinnedVersion {
		return "", fmt.Errorf("%w: got %q, pinned %s", ErrVersionMismatch, version, PinnedVersion)
	}
	return version, nil
}

// verifyRange checks both ends are full commit ids present in a complete
// (not shallow) repository. gitleaks itself reports zero leaks, exit 0, on
// an unknown revision, so this check is what keeps a bad range from
// reading as a clean scan.
func verifyRange(ctx context.Context, run scanner.Command, root string, r scanner.Range) error {
	if r.Empty() {
		return ErrNoRange
	}
	ends := [][2]string{{endBase, r.Base}, {endHead, r.Head}}
	for _, end := range ends {
		if !commitID.MatchString(end[1]) {
			return fmt.Errorf("%w: %s %q is not a full commit id", ErrBadRange, end[0], end[1])
		}
	}
	stdout, stderr, err := run(ctx, root, gitBinary, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return fmt.Errorf("%w: git rev-parse --is-shallow-repository: %v%s", ErrBadRange, err, quotedStderr(stderr))
	}
	if strings.TrimSpace(stdout) != "false" {
		return fmt.Errorf("%w: shallow repository (the checkout needs the full history of the range)", ErrBadRange)
	}
	for _, end := range ends {
		if _, _, err := run(ctx, root, gitBinary, "cat-file", "-e", end[1]+"^{commit}"); err != nil {
			return fmt.Errorf("%w: %s commit %s not found in the checkout", ErrBadRange, end[0], end[1])
		}
	}
	return nil
}

// scanRange runs gitleaks over the range with an explicit JSON report in a
// private temporary directory and decodes it. Under policy, inline
// `gitleaks:allow` is ignored and the fingerprint-list path points at the
// empty temporary directory (the root's own .gitleaksignore is still read
// by gitleaks; see ignoreFileFinding).
func scanRange(ctx context.Context, run scanner.Command, root string, r scanner.Range, policy bool, version string) ([]scanner.Finding, error) {
	dir, err := os.MkdirTemp("", "aurumcode-gitleaks-")
	if err != nil {
		return nil, fmt.Errorf("gitleaks: temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)
	config := filepath.Join(dir, configFile)
	if err := os.WriteFile(config, []byte(pinnedConfig), 0o600); err != nil {
		return nil, fmt.Errorf("gitleaks: pinned config: %w", err)
	}
	report := filepath.Join(dir, reportFile)
	_, stderr, runErr := run(ctx, root, binary, scanArgs(r, report, config, dir, policy)...)
	if runErr != nil {
		return nil, fmt.Errorf("gitleaks: execution failed: %w%s", runErr, quotedStderr(stderr))
	}
	if plain := ansiColor.ReplaceAllString(stderr, " "); loggedError.MatchString(plain) {
		return nil, fmt.Errorf("%w%s", ErrLoggedError, quotedStderr(loggedErrorLine(plain)))
	}
	raw, err := os.ReadFile(report)
	if err != nil {
		return nil, fmt.Errorf("gitleaks: report not written: %w", scanner.ErrInvalidOutput)
	}
	return decodeReport(raw, version)
}

// scanArgs is the gitleaks command line. Every output choice is explicit:
// the pinned binary has no default report format and exits 1 on leaks
// unless told otherwise.
func scanArgs(r scanner.Range, report, config, ignoreDir string, policy bool) []string {
	args := []string{
		"git",
		"--log-opts=" + r.Base + ".." + r.Head,
		"--report-format", "json",
		"--report-path", report,
		"--exit-code", "0",
		"--no-banner",
		"--no-color",
		"--redact",
		"--config", config,
	}
	if policy {
		args = append(args, "--ignore-gitleaks-allow", "--gitleaks-ignore-path", ignoreDir)
	}
	return append(args, ".")
}

// ignoreFileFinding: gitleaks always honors a .gitleaksignore at the
// scanned root and no flag of the pinned version disables it (measured).
// Under a central policy the author must not be able to drop a finding by
// listing its fingerprint, so the file's presence is itself a blocking
// finding the policy owner has to clear. Any entry with that name counts,
// a symlink or a directory included.
func ignoreFileFinding(root string) (scanner.Finding, bool) {
	if _, err := os.Lstat(filepath.Join(root, ignoreFile)); err != nil {
		return scanner.Finding{}, false
	}
	return scanner.Finding{
		Path:     ignoreFile,
		Line:     1,
		Side:     sideRight,
		RuleID:   RuleIgnoreFilePresent,
		Severity: severity,
		Message:  "a .gitleaksignore can hide gitleaks findings and no gitleaks flag disables it; under a central policy its presence is a finding",
	}, true
}

// quotedStderr is the last non-empty line of a command's error output as
// an error suffix ("" when there is none): the line where git and gitleaks
// state why they failed. The line is kept whole and on a line of its own,
// so the redaction that runs before any summary (scanner.Summarize) still
// sees a credential or an auth header exactly as the tool printed it; a
// cut here could split a secret below the redaction patterns.
func quotedStderr(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return ""
	}
	return ":\n" + last
}

// loggedErrorLine is the first line of stderr at gitleaks' error or fatal
// level: the one that says what went wrong, not the summary after it.
func loggedErrorLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if loggedError.MatchString(line) {
			return line
		}
	}
	return ""
}
