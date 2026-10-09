// Shared review passes for local changes and pull requests.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// changelogSection is the advisory release section the review derives from the
// reviewed commit messages through the AUR-498 engine.
type changelogSection struct {
	Version string
	Bump    string
	Entry   string
}

// changelogUnavailableNotice is the declared limitation emitted when the
// review cannot read the commit metadata a changelog section needs. It is
// data, not an instruction, and it never fails the review.
func changelogUnavailableNotice(language string) string {
	return i18n.Text(language, "notice.changelog_unavailable")
}

// modelInvalidOutputNotice is AUR-505's declared limitation for a model
// answer the parser could not validate. The provider call itself may have
// succeeded and consumed budget, but the quality half of the review is
// inconclusive, and the published review must say so instead of crashing
// with empty output. kind comes from internal/prompt's own ParseErrorKind
// enum, a trusted constant, so this text carries no model-authored bytes
// and needs no redaction; the review's model-derived fields are redacted
// by internal/review independently. It is a limitation, never a finding.
func modelInvalidOutputNotice(language, kind string) string {
	return i18n.Format(language, "notice.model_invalid_output", kind)
}

// buildChangelogSection runs the AUR-498 engine over the reviewed commit
// messages and renders the review's release section through internal/render.
// Commit text is UNTRUSTED: every field is passed through the same redaction
// sink pr_history.go uses before the engine escapes it for Markdown, so a
// canary secret in a subject never reaches the report. The text is never
// executed and never authorizes a tag, release or publication.
//
// A nil commit list, or a base version that does not parse, returns a zero
// section and a declared limitation instead of crashing the review.
func buildChangelogSection(baseVersion string, commits []changelog.Commit, filter *redaction.Filter) (changelogSection, string) {
	if filter == nil {
		filter = redaction.NewFilter()
	}
	if len(commits) == 0 {
		return changelogSection{}, changelogUnavailableNotice("")
	}
	redacted := make([]changelog.Commit, 0, len(commits))
	for _, c := range commits {
		redacted = append(redacted, changelog.Commit{
			Subject: filter.Redact(c.Subject),
			Body:    filter.Redact(c.Body),
			Hash:    filter.Redact(c.Hash),
		})
	}
	base, err := changelog.ParseVersion(strings.TrimSpace(baseVersion))
	if err != nil {
		if strings.TrimSpace(baseVersion) == "" {
			base = changelog.Version{}
		} else {
			return changelogSection{}, fmt.Sprintf("version base %q is not [v]major.minor.patch", strings.TrimSpace(baseVersion))
		}
	}
	next, bump := changelog.NextVersion(base, redacted)
	return changelogSection{
		Version: next.String(),
		Bump:    bump.String(),
		Entry:   changelog.Render(next, redacted),
	}, ""
}

// writeChangelogOutput writes the Action output in GitHub Actions'
// GITHUB_OUTPUT key=value / heredoc format so scripts/action-entrypoint.sh can
// append it verbatim. The text is already redacted, so no untrusted commit
// content is written raw. An empty path is a no-op (direct CLI use).
func writeChangelogOutput(path string, section changelogSection) error {
	if strings.TrimSpace(path) == "" || section.Version == "" {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "version=%s\n", section.Version)
	fmt.Fprintf(&b, "changelog_bump=%s\n", section.Bump)
	b.WriteString("changelog<<" + changelogOutputDelimiter + "\n")
	b.WriteString(strings.TrimRight(section.Entry, "\n"))
	b.WriteString("\n" + changelogOutputDelimiter + "\n")
	return os.WriteFile(path, []byte(b.String()), 0600)
}

// changelogOutputDelimiter is a fixed, collision-resistant heredoc marker for
// the Action output file.
const changelogOutputDelimiter = "AURUMCODE_CHANGELOG_EOF"

// appendChangelogSection appends the rendered release section to a review
// body, preserving a single blank-line separation. An empty section is a
// no-op, so the body is byte-identical when the feature is off.
func appendChangelogSection(body, section string) string {
	if strings.TrimSpace(section) == "" {
		return body
	}
	return strings.TrimRight(body, "\n") + "\n\n" + strings.TrimLeft(section, "\n")
}

// mergeStaticAnalysis is the deterministic static-analysis pass: findings from
// the embedded regex catalog (analysis/hardcoded-secret and friends) merge
// into result.Issues so patterns that need no model are always reported, not
// only when --seguranca is given. The Finding→ReviewIssue conversion is the
// exact one runPRReview used to inline, message and citation format included.
func mergeStaticAnalysis(diff *types.Diff, result *types.ReviewResult) {
	if result == nil {
		return
	}
	for _, f := range analysis.NewRunner().Analyze(diff) {
		result.Issues = append(result.Issues, f.ToIssue(""))
	}
}

// coverage pass (AUR-476): the review must state, on its own, which files
// the review did not cover and why -- never relying on the model to mention
// the limitation. The prompt builder (internal/prompt, AUR-467/AUR-475/
// AUR-477) already renders a "## Review Coverage" declaration inside the
// model prompt and mirrors its counts in PromptParts.Meta, but that text is
// only seen by the model; cmd/aurumcode owns both user-facing sinks (the
// --base terminal report and the published PR body) and previously never
// surfaced it. This pass closes that gap. It is deterministic and derives
// only from the diff and configuration, so the notice is identical on every
// run and is immune to a model response that claims complete coverage.

// coverageNotice renders the deterministic, localized coverage declaration the
// --base terminal and the published PR body both carry, or "" when the review
// was complete (every file fully or partially covered, nothing ignored or
// filtered). Rendered from the breakdown alone, it is never derived from the
// model's summary, so a model that answers "all files reviewed" cannot erase
// it (AC-003). The paths named are repository paths from the diff/config, not
// model output, so they carry no untrusted bytes.
func coverageNotice(copy reviewCopy, c reviewCoverageBreakdown) string {
	if !c.partial() && !c.declaredIgnored() && len(c.NoStructure) == 0 {
		return ""
	}
	var b strings.Builder
	if c.partial() {
		fmt.Fprintf(&b, "%s — %s\n", copy.coverageHeading, fmt.Sprintf(copy.coverageSummary, c.covered(), c.Total, c.uncovered()))
	} else {
		// Complete coverage of the review's scope: declare only what was
		// out of it (ignored, binary) and what had no grammar.
		fmt.Fprintf(&b, "%s\n", copy.coverageHeading)
	}
	if c.Partial > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coveragePartial, c.Partial))
	}
	if c.Budget > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coverageBudget, c.Budget))
		writeCoveragePaths(&b, c.BudgetPaths, copy.coverageMore)
	}
	if c.Ignored > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coverageIgnored, c.Ignored))
		writeCoveragePaths(&b, c.IgnoredPaths, copy.coverageMore)
	}
	if c.Filtered > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coverageFiltered, c.Filtered))
		writeCoveragePaths(&b, c.FilteredPaths, copy.coverageMore)
	}
	if len(c.NoStructure) > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coverageNoStructure, len(c.NoStructure)))
		writeCoveragePaths(&b, c.NoStructure, copy.coverageMore)
	}
	return strings.TrimRight(b.String(), "\n")
}
