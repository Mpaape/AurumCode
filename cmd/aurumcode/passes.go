// Shared review passes for local changes and pull requests.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/memory"
	"github.com/Mpaape/AurumCode/internal/render"
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
	if language == "pt-BR" || language == "pt" {
		return "Changelog indisponível: os metadados de commit desta revisão não puderam ser lidos; a versão sugerida e a entrada de changelog foram omitidas."
	}
	return "Changelog unavailable: this review could not read the commit metadata, so the suggested version and changelog entry were omitted."
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
	if language == "pt-BR" || language == "pt" {
		return fmt.Sprintf("Revisão de qualidade inconclusiva: a resposta do modelo não passou no parser (%s); o modelo não foi considerado e os achados determinísticos (análise estática e segurança) foram publicados.", kind)
	}
	return fmt.Sprintf("Quality review inconclusive: the model's response did not pass the parser (%s); it was not considered, and the deterministic findings (static analysis and security) were still published.", kind)
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
		result.Issues = append(result.Issues, types.ReviewIssue{
			File:     f.Path,
			Line:     f.Line,
			Side:     f.Side,
			Severity: f.Severity,
			RuleID:   f.RuleID,
			Message:  fmt.Sprintf("%s (rule %s)", f.Message, f.RuleID),
		})
	}
}

// resolveCodebaseContext is the codebase-context pass: bounded dependency
// context for the changed paths, resolved from the checkout so the model sees
// what else the change can affect. It is an enhancement, never a gate: any
// failure degrades to empty context and the review continues on the diff
// alone.
func resolveCodebaseContext(diff *types.Diff) string {
	pack, err := codebaseContextPack(diffPaths(diff))
	if err != nil || pack == nil {
		return ""
	}
	data, err := json.Marshal(pack)
	if err != nil {
		return ""
	}
	return string(data)
}

// renderPass is the render pass: it renders the deterministic, localized
// summary (render.Summary) and the Mermaid flow diagram (render.Mermaid) from
// the already-redacted result and the diff. Both paths share it: the PR path
// wraps the two strings into its published comment body, the --base path
// prints them to stdout. Both values are "" when there is nothing to render.
func renderPass(result *types.ReviewResult, diff *types.Diff, language string) (tldr string, diagram string) {
	tldr = render.Summary(result, language)
	if d, err := render.Mermaid(diff); err == nil {
		diagram = d
	}
	return tldr, diagram
}

// renderLocalReport renders the --base path's stdout report from the shared
// render pass: the summary block, then a fenced ```mermaid block when a
// diagram was produced. It is deterministic and derives only from result and
// diff, so the same input always prints the same bytes.
func renderLocalReport(result *types.ReviewResult, diff *types.Diff, language string) string {
	tldr, diagram := renderPass(result, diff, language)
	var b strings.Builder
	if strings.TrimSpace(tldr) != "" {
		b.WriteString(tldr)
		b.WriteString("\n")
	}
	if strings.TrimSpace(diagram) != "" {
		b.WriteString("\n```mermaid\n")
		b.WriteString(diagram)
		b.WriteString("\n```\n")
	}
	return b.String()
}

// openReviewMemory is the memory pass's open-and-load half, shared by both
// paths. mode is review.memory ("off", "ephemeral", "local"); owner/repo are
// empty for the local --base path, where the directory falls back to the
// checkout's origin remote or path hash (memorydir.go, AUR-489). The returned
// store reads and writes that directory; the returned notes are its current
// contents (as untrusted observations), and text is their JSON encoding for
// ReviewContext.MemoryNotes. On any error the store degrades to the off store
// and the reason is reported on stderr -- memory is observation, never
// instruction, so it can never fail a review.
func openReviewMemory(mode, owner, repo string, stderr io.Writer, filter *redaction.Filter) (memory.Store, []memory.Note, string) {
	store, err := newRepoMemory(mode, owner, repo)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: review memory unavailable: %s; continuing without it\n", filter.Redact(err.Error()))
		store, _ = memory.New("off", "")
	}
	notes, loadErr := store.Load()
	if loadErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: loading review memory: %s; continuing without prior observations\n", filter.Redact(loadErr.Error()))
		notes = nil
	}
	text := ""
	if len(notes) > 0 {
		if data, merr := json.Marshal(notes); merr == nil {
			text = string(data)
		}
	}
	return store, notes, text
}

// persistReviewMemory is the memory pass's save half, shared by both paths. It
// writes this round's findings as notes for the next run, merged with the
// existing notes and deduplicated (notesFromIssues). Memory is observation,
// never instruction; a save failure is reported and never affects the verdict.
// Disabled ("" or "off") memory is a no-op, matching the PR path's published
// behavior of never saving when memory is off.
func persistReviewMemory(store memory.Store, mode string, existing []memory.Note, issues []types.ReviewIssue, stderr io.Writer, filter *redaction.Filter) {
	if strings.TrimSpace(mode) == "" || strings.TrimSpace(mode) == "off" {
		return
	}
	if err := store.Save(notesFromIssues(existing, issues)); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: saving review memory: %v\n", filter.Redact(err.Error()))
	}
}
