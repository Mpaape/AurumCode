// Shared review passes for local changes and pull requests.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
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

// reviewCoverageBreakdown is the deterministic per-reason coverage figure the
// coverage pass renders. Each count is a distinct cause a file did not reach
// the model; keeping them separate is the point -- a reader must be able to
// tell "the repository config hides this path" from "the token budget dropped
// it" (AUR-476 AC-004: a configured omission is never presented as proof that
// the file's content -- e.g. tests -- does not exist).
type reviewCoverageBreakdown struct {
	// Total is the code-file figure the prompt builder emitted
	// (PromptParts.Meta["code_files_total"]). It is empty/zero when no prompt
	// was assembled (no provider, --seguranca-only), in which case the counts
	// below stand on their own.
	Total    int
	Complete int
	Partial  int
	Budget   int
	Ignored  int
	Filtered int
	// IgnoredPaths and FilteredPaths are the concrete paths cmd/aurumcode
	// owns the knowledge of: config-hidden and binary/oversized files. They
	// are named in the notice so AC-001's "caminhos nao revisados" holds.
	// Budget omissions are named by the prompt builder inside the model
	// input; only their count reaches cmd through result.Metadata.
	IgnoredPaths  []string
	FilteredPaths []string
}

// covered counts every file that reached the model in full or in part. It is
// derived, never stored, so it cannot disagree with the parts.
func (c reviewCoverageBreakdown) covered() int { return c.Complete + c.Partial }

// uncovered counts every file the review did not fully cover, for any reason.
func (c reviewCoverageBreakdown) uncovered() int {
	return c.Budget + c.Ignored + c.Filtered
}

// partial reports whether any file was left out of the review in whole or in
// part. AC-002: this is the single predicate every sink uses, so a complete
// review can never grow a coverage notice on one path and not the other.
func (c reviewCoverageBreakdown) partial() bool {
	return c.Partial > 0 || c.uncovered() > 0
}

// mergeReviewCoverage combines a model-produced promptMeta with the
// deterministic facts cmd/aurumcode owns: which files the repository config
// ignored (removed before the model ever saw the diff) and which were filtered
// as binary/oversized (analyzer.DiffNotice). A file can be counted at most
// once; the ignored and filtered causes are checked first because they are the
// more specific explanations. When no prompt was assembled (promptMeta nil,
// e.g. the deterministic-only path) the prompt counts are simply absent and
// the configured/filtered counts remain.
func mergeReviewCoverage(promptMeta map[string]string, notices []analyzer.DiffNotice, rawFileCount int, ignoredPaths []string) reviewCoverageBreakdown {
	var c reviewCoverageBreakdown
	c.Total = atoiOrZero(promptMeta["code_files_total"])
	c.Complete = atoiOrZero(promptMeta["code_files_complete"])
	c.Partial = atoiOrZero(promptMeta["code_files_partial"])
	c.Budget = atoiOrZero(promptMeta["code_files_omitted"])
	c.IgnoredPaths = dedupePaths(ignoredPaths)
	c.Ignored = len(c.IgnoredPaths)
	filtered := make([]string, 0, len(notices))
	seen := make(map[string]struct{}, len(notices))
	for _, n := range notices {
		if n.Path == "" {
			continue
		}
		if _, ok := seen[n.Path]; ok {
			continue
		}
		seen[n.Path] = struct{}{}
		filtered = append(filtered, n.Path)
	}
	c.FilteredPaths = filtered
	c.Filtered = len(filtered)
	// The code-file total must account for files the prompt builder never saw:
	// an ignored or filtered file is absent from the diff the builder measured,
	// so its own total undercounts the review's true denominator. Reconcile to
	// the raw diff file count when that is larger, so "covered + uncovered"
	// never silently drops a file that neither the prompt nor the config
	// counted.
	if raw := rawFileCount; raw > c.Total {
		c.Total = raw
	}
	return c
}

// dedupePaths returns paths in first-seen order with duplicates removed.
func dedupePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// maxCoveragePaths caps how many concrete paths each reason lists before
// switching to an explicit count, mirroring internal/prompt's
// maxOmittedBullets: the notice stays a bounded size no matter how large the
// diff is.
const maxCoveragePaths = 20

// writeCoveragePaths appends up to maxCoveragePaths of paths as sub-bullets,
// then a "and N more" line. Paths are repository paths from the diff/config,
// never model output, so they carry no untrusted bytes.
func writeCoveragePaths(b *strings.Builder, paths []string) {
	listed := 0
	for _, p := range paths {
		if listed >= maxCoveragePaths {
			fmt.Fprintf(b, "  - ... and %d more\n", len(paths)-listed)
			break
		}
		fmt.Fprintf(b, "  - %s\n", p)
		listed++
	}
}

// atoiOrZero parses a decimal metadata string, treating an absent or malformed
// value as zero. Metadata is engine-produced trusted text, never model output;
// a malformed value is still never allowed to crash a review.
func atoiOrZero(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// coverageNotice renders the deterministic, localized coverage declaration the
// --base terminal and the published PR body both carry, or "" when the review
// was complete (every file fully or partially covered, nothing ignored or
// filtered). Rendered from the breakdown alone, it is never derived from the
// model's summary, so a model that answers "all files reviewed" cannot erase
// it (AC-003). The paths named are repository paths from the diff/config, not
// model output, so they carry no untrusted bytes.
func coverageNotice(copy reviewCopy, c reviewCoverageBreakdown) string {
	if !c.partial() {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n", copy.coverageHeading, fmt.Sprintf(copy.coverageSummary, c.covered(), c.Total, c.uncovered()))
	if c.Partial > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coveragePartial, c.Partial))
	}
	if c.Budget > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coverageBudget, c.Budget))
	}
	if c.Ignored > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coverageIgnored, c.Ignored))
		writeCoveragePaths(&b, c.IgnoredPaths)
	}
	if c.Filtered > 0 {
		fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(copy.coverageFiltered, c.Filtered))
		writeCoveragePaths(&b, c.FilteredPaths)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ignoredDiffPaths returns the paths of diff that the repository's explicit
// `ignore` config will drop, in diff order and without duplicates. It is the
// pre-filter snapshot FilterIgnoredPaths (internal/config) needs and AUR-476's
// coverage notice consumes: after the filter runs those paths are gone, so the
// reason they are absent from the model input can only be recovered before it.
// It is deliberately built on the public config.FilterIgnoredPaths behavior
// -- filtering a single-file probe outstanding -- rather than re-implementing
// the glob semantics, so "ignored" means exactly what the filter already
// means. A nil diff or a zero-config cfg returns nil, matching
// FilterIgnoredPaths' own no-op contract.
func ignoredDiffPaths(diff *types.Diff, cfg *config.Config) []string {
	if diff == nil || cfg == nil || len(cfg.Ignore) == 0 {
		return nil
	}
	var out []string
	for _, f := range diff.Files {
		probe := config.FilterIgnoredPaths(&types.Diff{Files: []types.DiffFile{f}}, cfg)
		if len(probe.Files) == 0 {
			out = append(out, f.Path)
		}
	}
	return out
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
// the already-redacted result and the diff. The --base path prints them to
// stdout; the PR path publishes a single code-review document instead.
func renderPass(result *types.ReviewResult, diff *types.Diff, language string) (tldr string, diagram string) {
	tldr = render.Summary(result, language)
	if d, err := render.Mermaid(diff); err == nil {
		diagram = d
	}
	return tldr, diagram
}

// applicableSuggestionRange reports the 1-based start/end of a suggestion whose
// replacement can actually be applied, reusing the same coordinate rules the
// PR publication path already enforces (suggestionRange + isInlineEligible):
// every line of the range must be a line this diff added, so the replacement
// has an exact, reviewable boundary. A suggestion without a location, or one
// whose location falls outside the added lines, is not applicable and returns
// ok=false -- it is advice, not a one-click fix.
//
// It shares one definition with nativeSuggestionComment and
// filterSuggestionsToChangedLines rather than reimplementing the check, so the
// terminal view and the PR view can never disagree about what is eligible. The
// one extra bound -- end-start > 1000 -- mirrors filterSuggestionsToChangedLines
// so a pathological range is rejected before the line walk, never after.
func applicableSuggestionRange(diff *types.Diff, suggestion types.ReviewSuggestion) (start, end int, ok bool) {
	if strings.TrimSpace(suggestion.ProposedCode) == "" || strings.TrimSpace(suggestion.File) == "" {
		return 0, 0, false
	}
	start, end = suggestionRange(suggestion)
	if start <= 0 || end < start || end-start > 1000 {
		return 0, 0, false
	}
	for line := start; ; line++ {
		if !isInlineEligible(diff, types.ReviewIssue{File: suggestion.File, Line: line}) {
			return 0, 0, false
		}
		if line == end {
			break
		}
	}
	return start, end, true
}

// suggestionLocationLabel renders a suggestion's location in the same
// `<file>:<line>` / `<file>:<start>-<end>` shape the PR summary already uses,
// so the terminal and the PR describe a suggestion identically.
func suggestionLocationLabel(suggestion types.ReviewSuggestion, start, end int) string {
	if start == end {
		return fmt.Sprintf("%s:%d", suggestion.File, start)
	}
	return fmt.Sprintf("%s:%d-%d", suggestion.File, start, end)
}

// renderSuggestions renders the review's suggestions for the local --base
// terminal report. Every suggestion the model returned is shown, exactly once,
// with its title, description, location and proposed replacement, so a user who
// never opens the PR still receives the complete suggestion. A suggestion whose
// replacement is eligible for a one-click change is marked as such (and names
// its exact range); one that is not -- no location, a location outside the
// added lines, or no proposed code -- is shown with an explicit limitation and
// never presented as an applicable substitution, matching the PR path's
// fail-closed classification (filterSuggestionsToChangedLines /
// nativeSuggestionComment). Suggestions without a title and without a
// description carry nothing to render and are skipped, exactly as the PR
// summary skips them. The returned string is empty when there is nothing to
// show, so the zero-suggestion report is byte-identical to the published
// behavior.
func renderSuggestions(result *types.ReviewResult, diff *types.Diff, language string) string {
	if result == nil || len(result.Suggestions) == 0 {
		return ""
	}
	copy := reviewCopyFor(language)
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", copy.suggestions)
	wrote := false
	for _, suggestion := range result.Suggestions {
		title := strings.TrimSpace(suggestion.Title)
		description := strings.TrimSpace(suggestion.Description)
		if title == "" && description == "" {
			continue
		}
		wrote = true
		fmt.Fprintf(&b, "- **%s**", title)
		if description != "" {
			fmt.Fprintf(&b, " — %s", description)
		}
		start, end, applicable := applicableSuggestionRange(diff, suggestion)
		if applicable {
			fmt.Fprintf(&b, " %s", fmt.Sprintf(copy.suggestionApplicable, suggestionLocationLabel(suggestion, start, end)))
		} else {
			fmt.Fprintf(&b, " %s", copy.suggestionNotApplicable)
		}
		b.WriteByte('\n')
		if proposed := strings.TrimSpace(suggestion.ProposedCode); proposed != "" {
			fmt.Fprintf(&b, "  - **%s:**\n\n    ```\n%s\n    ```\n", copy.proposedImplementation, proposed)
		}
		writeSummaryField(&b, copy.rationale, suggestion.Rationale)
		writeSummaryField(&b, copy.verify, suggestion.Verification)
	}
	if !wrote {
		return ""
	}
	return b.String()
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
