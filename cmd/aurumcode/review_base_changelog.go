// The changelog check of the local review: when the base declares
// changelog_check in suggest or required, `review --base` runs the same
// check `aurumcode changelog --base` runs (evaluateChangelog, over the same
// diff from the common ancestor and the same effective configuration, the
// central policy included), so the local gate refuses what the
// "Changelog obrigatório" workflow would refuse. The author is a person, as
// in the command without --autor. The suggested entry comes from the commit
// subjects, as in the --pr body: the gate spends no extra model call. The
// --pr path never runs it: there the changelog workflow decides on its own.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// runChangelogCheck evaluates the entry and leaves the verdict for the gate
// (b.changelogCheck) and for the report (b.changelogReport). Mode off, or a
// full review without a base, changes nothing.
func (b *baseReview) runChangelogCheck() {
	b.changelogCheck, b.changelogReport = nil, ""
	if b.f.fullReview() || b.rawDiff == nil {
		return
	}
	// The policy warnings were already printed when the config loaded.
	ev, err := evaluateChangelog(b.cwd, b.policyDir, b.rawDiff, b.notices, false, io.Discard)
	if err != nil {
		b.changelogUndetermined(err)
		return
	}
	if ev.req == nil {
		return
	}
	line := b.redactText(changelogVerdictLine(ev.mode, ev.verdict))
	suggestion := ""
	if !ev.verdict.OK && ev.verdict.Reason != changelog.ReasonIndeterminate {
		suggestion = b.changelogSuggestedEntry(*ev.req, ev.mode)
	}
	b.changelogReport = changelogReportSection(b.reviewLanguage, line, suggestion)
	if ev.verdict.OK || ev.mode != config.ChangelogRequired {
		// A refusal reaches stderr through the gate's own line.
		fmt.Fprintf(b.stderr, "aurumcode review: %s\n", line)
		return
	}
	issue := b.changelogFinding(*ev.req, ev.verdict, line, suggestion != "")
	b.changelogCheck = &changelogGateCheck{
		Finding: &issue,
		Blocks:  true,
		Breach:  ev.verdict.Reason != changelog.ReasonIndeterminate,
		Line:    line,
	}
}

// changelogUndetermined is a check that could not be established: the
// base's mode is then unknown, and the mode of the proposed change (b.cfg)
// never decides it, so the gate closes, as the command exits 1.
func (b *baseReview) changelogUndetermined(err error) {
	line := b.redactText(fmt.Sprintf("changelog: indeterminado: %v", err))
	b.changelogReport = changelogReportSection(b.reviewLanguage, line, "")
	issue := types.ReviewIssue{
		File: config.DefaultConfigPath, Line: 1, Severity: "error",
		RuleID: "changelog/" + changelog.ReasonIndeterminate, Message: line, Origin: gateOriginChangelog,
		Verification: b.changelogVerification(),
	}
	b.changelogCheck = &changelogGateCheck{Finding: &issue, Blocks: true, Line: line}
}

// changelogFinding is the refused entry as a finding of the changelog file,
// at the line of its section when the change touches it.
func (b *baseReview) changelogFinding(req changelog.Requirement, verdict changelog.Verdict, line string, suggested bool) types.ReviewIssue {
	issue := types.ReviewIssue{
		File: req.File, Line: changelogSectionLine(req, b.rawDiff, b.notices), Severity: "error",
		RuleID: "changelog/" + verdict.Reason, Message: line, Origin: gateOriginChangelog,
		Verification: b.changelogVerification(),
	}
	if suggested {
		issue.Suggestion = fmt.Sprintf("cole na seção %s de %s a entrada sugerida neste parecer e ajuste-a ao que a PR muda", req.Section, req.File)
	}
	return issue
}

// changelogVerification is how to repeat the check alone.
func (b *baseReview) changelogVerification() string {
	return b.redactText("aurumcode changelog --base " + b.f.base)
}

// changelogSuggestedEntry is the entry suggested from the commit subjects
// of base..HEAD, redacted, as the --pr body shows it; "" when there is none.
func (b *baseReview) changelogSuggestedEntry(req changelog.Requirement, mode config.ChangelogMode) string {
	commits, err := gitChangelogCommits(b.cwd, b.f.base, "HEAD")
	if err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: commits indisponíveis para a entrada de changelog sugerida: %v\n", err)
		return ""
	}
	s := req.SuggestFromCommits(commits)
	if s.Empty() {
		return ""
	}
	return changelogSuggestionMarkdown(req, mode, s.Source, redactSuggestion(b.filter, s.Block()), b.reviewLanguage)
}

// changelogReportSection is the report's section: the verdict line and, for
// a missing or weak entry, the suggested one.
func changelogReportSection(language, line, suggestion string) string {
	var s strings.Builder
	s.WriteString(i18n.Text(language, "changelog.check.heading") + "\n\n" + line + "\n")
	if suggestion != "" {
		s.WriteString("\n" + suggestion)
	}
	// A blank line keeps the findings that follow apart from the section.
	s.WriteString("\n")
	return s.String()
}

// changelogSectionLine is the line of the entry's section in the changed
// file, 1 when the change does not touch it or has no such section.
func changelogSectionLine(req changelog.Requirement, diff *types.Diff, notices []analyzer.DiffNotice) int {
	side := fileSides(req.File, diff, notices)
	if !side.found || side.unusable {
		return 1
	}
	if n := req.SectionLine(side.new); n > 0 {
		return n
	}
	return 1
}
