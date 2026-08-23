package prompt

import (
	"fmt"
	"sort"
	"strings"
)

// This file gives the review prompt the one thing it never had: the list
// of rule ids the AUR-434 gate accepts.
//
// THE DEFECT (measured 2026-08-14 against the real gateway, with a diff
// carrying three planted defects):
//
//	aurumcode review: 5 finding(s) discarded: 5 citing an unknown rule_id
//	(quality/documentation, quality/naming, quality/readability,
//	 security/shell-injection)
//
// Six findings came back from the model and one reached the user. The
// command injection was FOUND and LOST: the model called it
// security/shell-injection, the catalog calls it
// security/command-injection. templates/review.md showed exactly one
// rule_id, inside its output example, and never said which ids exist --
// so the model invented plausible ones and the gate discarded them. This
// card does not loosen that gate (an unknown rule_id stays discarded and
// announced, AUR-434/AUR-448) and does not fuzzy-match a model's spelling
// onto a catalog id, which would turn a visible model error into a wrong
// citation presented as true. It removes the cause: the model now picks
// from a closed list it was given.
//
// WHY THIS IS A MIRROR AND NOT AN IMPORT
//
// The catalog itself lives in internal/review/rules/*.yml behind
// internal/review.RulesLoader, and internal/review already imports this
// package (reviewer.go builds a PromptBuilder), so `prompt` importing
// `review` is an import cycle. Injection by the caller would mean editing
// internal/review, which this card does not own. So the ids are mirrored
// here as compile-time data, and tests/unit/AUR-461.go asserts SET
// EQUALITY between this slice and review.NewRulesLoader().GetAll(): a rule
// added, renamed or removed in the YAML without the matching edit here
// fails that test, which is exactly what AC-001 asks for ("uma regra nova
// sem entrada no prompt quebre o build"). A dynamic load could not fail
// that way -- it would agree with itself by construction.
//
// SetRuleCatalog below is the seam for the day internal/review can hand
// its live loader down (a later card owning that file), without another
// change to this package's callers.

// DefaultRuleCatalog is the mirror of the embedded review catalog:
// every id of internal/review/rules/{security,quality,performance}.yml,
// sorted, exactly as RulesLoader.Get indexes them. Keep it sorted and keep
// it complete -- tests/unit/AUR-461.go proves both.
var DefaultRuleCatalog = []string{
	"performance/excessive-allocation",
	"performance/inefficient-algorithm",
	"performance/inefficient-loop",
	"performance/memory-leak",
	"performance/n-plus-one",
	"quality/dead-code",
	"quality/duplicate-code",
	"quality/high-complexity",
	"quality/long-function",
	"quality/magic-numbers",
	"quality/missing-error-handling",
	"quality/poor-naming",
	"quality/unused-variable",
	"security/command-injection",
	"security/hardcoded-secret",
	"security/insecure-random",
	"security/missing-auth",
	"security/path-traversal",
	"security/sql-injection",
	"security/weak-crypto",
	"security/xss",
}

// MaxRuleCatalogTokens is the ceiling AC-002 puts on the rendered catalog
// section. The current 21-rule catalog renders at roughly a quarter of it,
// so there is room to grow; growing PAST it is a loud build/assembly
// failure and never a silent truncation. That distinction is the whole
// point: a truncated list would make the model cite a rule that exists but
// was cut, and the gate would discard it -- this card's defect back by
// another road.
const MaxRuleCatalogTokens = 512

// ValidateRuleCatalog reports whether the rendered form of ids fits
// MaxRuleCatalogTokens under est, and rejects an empty or unsorted
// catalog. An empty catalog would render a section telling the model to
// choose from nothing.
func ValidateRuleCatalog(ids []string, est TokenEstimator) error {
	if len(ids) == 0 {
		return fmt.Errorf("review rule catalog is empty: the prompt would ask the model to choose a rule_id from an empty list")
	}
	if !sort.StringsAreSorted(ids) {
		return fmt.Errorf("review rule catalog is not sorted: the rendered prompt must be byte-stable across runs")
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("review rule catalog contains an empty id")
		}
	}
	if est == nil {
		est = NewHeuristicEstimator()
	}
	if got := est.Estimate(RenderRuleCatalog(ids)); got > MaxRuleCatalogTokens {
		return fmt.Errorf(
			"review rule catalog section needs %d tokens, over the %d-token budget for %d rules: refusing to truncate the list, because a model citing a rule that exists but was cut would have its finding discarded",
			got, MaxRuleCatalogTokens, len(ids))
	}
	return nil
}

// RenderRuleCatalog renders the closed list injected into the review
// prompt. It deliberately carries ids only -- no titles: the ids are
// self-descriptive (security/command-injection is what the model called
// "shell-injection"), and mirroring titles too would double the surface
// that can drift from the YAML.
//
// The rendered block must not open a ```json fence: AUR-459's
// TemplateTeachesOneSchema requires the template to show exactly one JSON
// example, and a second one here would teach a second schema.
func RenderRuleCatalog(ids []string) string {
	var sb strings.Builder
	sb.WriteString("The following list is the COMPLETE set of `rule_id` values this project\n")
	sb.WriteString("accepts. Copy the id of the finding's rule verbatim from this list.\n")
	sb.WriteString("An id outside this list -- including a plausible synonym or a different\n")
	sb.WriteString("suffix -- is discarded and the finding never reaches the user, so a\n")
	sb.WriteString("real problem reported under an invented id is a problem you did not\n")
	sb.WriteString("report. If no id fits exactly, pick the closest one on the list; never\n")
	sb.WriteString("invent, abbreviate or pluralize one.\n\n")
	for _, id := range ids {
		sb.WriteString("- `")
		sb.WriteString(id)
		sb.WriteString("`\n")
	}
	return sb.String()
}

// SetRuleCatalog replaces the mirrored catalog this builder renders. It is
// the injection seam for a future caller that can pass internal/review's
// live loader ids down (see the import-cycle note at the top of this
// file); production today uses DefaultRuleCatalog. It validates eagerly so
// an over-budget or empty injected catalog is reported at the seam that
// caused it rather than at some later prompt assembly.
func (b *PromptBuilder) SetRuleCatalog(ids []string) error {
	catalog := append([]string(nil), ids...)
	sort.Strings(catalog)
	if err := ValidateRuleCatalog(catalog, b.estimator); err != nil {
		return err
	}
	b.ruleCatalog = catalog
	return nil
}

// RuleCatalog returns the ids this builder renders into the review prompt.
func (b *PromptBuilder) RuleCatalog() []string {
	return append([]string(nil), b.ruleCatalog...)
}

// ruleCatalogSection renders this builder's catalog, or fails loudly. It
// never returns a partial list: AC-002's requirement is that assembly
// fails high rather than truncating.
func (b *PromptBuilder) ruleCatalogSection() (string, error) {
	if err := ValidateRuleCatalog(b.ruleCatalog, b.estimator); err != nil {
		return "", err
	}
	return RenderRuleCatalog(b.ruleCatalog), nil
}
