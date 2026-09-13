// Shared review passes for local changes and pull requests.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/memory"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

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
