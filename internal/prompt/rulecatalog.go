package prompt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/review/rules"
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
// WHERE THE IDS COME FROM
//
// The catalog lives in internal/review/rules/*.yml. The ids are read from
// those same embedded files through the internal/review/rules package (a
// leaf package, so there is no import cycle with internal/review, which
// imports this one): a rule added, renamed or removed in the YAML reaches
// the prompt with no second edit, and tests/unit/AUR-461.go still asserts
// set equality with review.NewRulesLoader().GetAll().
//
// SetRuleCatalog below is the seam for a caller that extends the list
// (AUR-519's dynamic skill-section ids).

// DefaultRuleCatalog is every id of the embedded review catalog, sorted,
// exactly as RulesLoader.Get indexes them.
var DefaultRuleCatalog = mustCatalogIDs()

// mustCatalogIDs loads the embedded catalog ids at initialization. The
// files are compiled into the binary, so a failure is a build defect that
// must stop the process rather than send a prompt with no closed list.
func mustCatalogIDs() []string {
	ids, err := rules.IDs()
	if err != nil {
		panic(fmt.Sprintf("review rule catalog unavailable: %v", err))
	}
	return ids
}

// MaxRuleCatalogTokens is the default ceiling AC-002 puts on the rendered
// catalog section, read from templates/limits.yml (rule_catalog_max_tokens).
// Growing PAST it is a loud assembly failure and never a silent
// truncation. That distinction is the whole point: a truncated list would
// make the model cite a rule that exists but was cut, and the gate would
// discard it. A builder may carry a different ceiling (SetSlotLimits).
var MaxRuleCatalogTokens = defaultSlotLimits.RuleCatalogMaxTokens

// ValidateRuleCatalog reports whether the rendered form of ids fits
// MaxRuleCatalogTokens under est, and rejects an empty or unsorted
// catalog. An empty catalog would render a section telling the model to
// choose from nothing.
func ValidateRuleCatalog(ids []string, est TokenEstimator) error {
	return validateRuleCatalogWithin(ids, est, MaxRuleCatalogTokens)
}

func validateRuleCatalogWithin(ids []string, est TokenEstimator, maxTokens int) error {
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
	if got := est.Estimate(RenderRuleCatalog(ids)); got > maxTokens {
		return fmt.Errorf(
			"review rule catalog section needs %d tokens, over the %d-token budget for %d rules: refusing to truncate the list, because a model citing a rule that exists but was cut would have its finding discarded",
			got, maxTokens, len(ids))
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

// SetRuleCatalog replaces the catalog this builder renders (the default
// is DefaultRuleCatalog, read from the embedded rule files). It validates
// eagerly so an over-budget or empty injected catalog is reported at the
// seam that caused it rather than at some later prompt assembly.
func (b *PromptBuilder) SetRuleCatalog(ids []string) error {
	catalog := append([]string(nil), ids...)
	sort.Strings(catalog)
	if err := validateRuleCatalogWithin(catalog, b.estimator, b.limits.RuleCatalogMaxTokens); err != nil {
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
	if err := validateRuleCatalogWithin(b.ruleCatalog, b.estimator, b.limits.RuleCatalogMaxTokens); err != nil {
		return "", err
	}
	return RenderRuleCatalog(b.ruleCatalog), nil
}

// SetSlotLimits replaces this builder's slot ceilings (the prompt-wide
// default, the rule catalog, evidence and tools sections). Every ceiling
// must be positive: a zero would silently disable the section's bound.
func (b *PromptBuilder) SetSlotLimits(limits SlotLimits) error {
	if limits.PromptMaxTokens <= 0 || limits.RuleCatalogMaxTokens <= 0 || limits.EvidenceMaxTokens <= 0 || limits.ToolsMaxTokens <= 0 {
		return fmt.Errorf("every slot ceiling must be a positive token count: %+v", limits)
	}
	b.limits = limits
	return nil
}

// SlotLimits returns this builder's slot ceilings.
func (b *PromptBuilder) SlotLimits() SlotLimits { return b.limits }
