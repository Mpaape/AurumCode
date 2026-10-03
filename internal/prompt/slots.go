package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// The review prompt is one template, templates/review.md. Its main body is
// the system message; its named {{define}} blocks are the slots the user
// message is assembled from. Every section title the model sees lives in
// that file -- Go code decides WHICH slots render and WITH WHAT data, never
// what a section is called -- so a reader auditing the prompt reads one
// file, and a new section cannot appear without appearing there.
const (
	slotUserHeader            = "user_header"
	slotPRHistory             = "pr_history"
	slotCodebaseContext       = "codebase_context"
	slotReviewMemory          = "review_memory"
	slotDeterministicEvidence = "deterministic_evidence"
	slotEvidenceItem          = "evidence_item"
	slotAvailableTools        = "available_tools"
	slotToolItem              = "tool_item"
	slotCoverage              = "coverage"
	slotRepositoryContext     = "repository_context"
	slotRepositoryContextUser = "repository_context_slot"
)

const reviewTemplateName = "review.md"

// reviewSlots is the parsed review template the slot renderers execute.
// It is parsed once from the embedded file; a template that does not parse
// is a build defect, reported loudly at the first render.
var reviewSlots, reviewSlotsErr = parseReviewTemplate()

func parseReviewTemplate() (*template.Template, error) {
	content, err := templateFS.ReadFile("templates/" + reviewTemplateName)
	if err != nil {
		return nil, err
	}
	return template.New(reviewTemplateName).Parse(string(content))
}

// renderSlot executes one named slot of the review template. The slots
// take only engine-built data of fixed types, so an execution error is a
// programming defect in this package and panics instead of returning a
// prompt with a silently missing section.
func renderSlot(name string, data any) string {
	if reviewSlotsErr != nil {
		panic(fmt.Sprintf("review prompt template unavailable: %v", reviewSlotsErr))
	}
	var buf bytes.Buffer
	if err := reviewSlots.ExecuteTemplate(&buf, name, data); err != nil {
		panic(fmt.Sprintf("rendering review prompt slot %q: %v", name, err))
	}
	return buf.String()
}

// renderOptionalSlot renders name around value, or nothing when value is
// empty: an absent input produces no section at all, so a review without
// that input stays byte-identical to one that predates the slot.
func renderOptionalSlot(name, value string) string {
	if isBlank(value) {
		return ""
	}
	return renderSlot(name, value)
}

// userHeaderData feeds the user_header slot: the change summary, the CI
// context echo and the budgeted code-change segments.
type userHeaderData struct {
	TotalFiles   int
	LinesAdded   int
	LinesDeleted int
	CIContext    string
	Segments     []string
}

// RenderRepositoryContext renders the repository context block (the
// untrusted, informational material configured context providers supply)
// through the review template's repository_context slot. sources names
// each contributing provider in order; contributions is the already
// redacted, assembled text.
func RenderRepositoryContext(sources []string, contributions string) string {
	return renderSlot(slotRepositoryContext, struct {
		Sources       []string
		Contributions string
	}{Sources: sources, Contributions: contributions})
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }
