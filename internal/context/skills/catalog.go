package skills

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
)

// Catalog is the skills the review resolves for one run: the central
// policy's skills layered over the repository's. It is one ContextProvider,
// so the model sees the selected skills and the selection warnings in the
// same context block, and the caller can put the same warnings in the review
// text (Warnings).
//
// A policy skill and a repository skill with the SAME selector (same
// languages after alias resolution, same paths) are the same convention
// written twice: the policy's wins and the repository's is dropped with a
// declared warning, so a repository cannot shadow what the organization
// imposes.
type Catalog struct {
	set      *Set
	langs    *Languages
	budget   Budget
	dropped  []string
	loadWarn []string
}

// NewCatalog layers repo under policy. Either set may be nil. loadWarnings
// are declared notices from loading (for example an unreadable repository
// skill) that the caller wants shown next to the selection warnings. A nil
// langs selects DefaultLanguages().
func NewCatalog(policy, repo *Set, langs *Languages, loadWarnings []string) *Catalog {
	if langs == nil {
		langs = DefaultLanguages()
	}
	c := &Catalog{langs: langs, loadWarn: append([]string(nil), loadWarnings...)}
	c.set, c.dropped = layer(policy, repo, langs)
	return c
}

// layer merges the two sets, dropping every repository skill whose selector a
// policy skill already declares.
func layer(policy, repo *Set, langs *Languages) (*Set, []string) {
	out := &Set{}
	taken := map[string]Skill{}
	if policy != nil {
		for _, sk := range policy.Skills {
			out.Skills = append(out.Skills, sk)
			if sk.Selector.Declared() {
				taken[selectorKey(sk.Selector, langs)] = sk
			}
		}
	}
	var dropped []string
	if repo != nil {
		for _, sk := range repo.Skills {
			if winner, ok := taken[selectorKey(sk.Selector, langs)]; ok && sk.Selector.Declared() {
				dropped = append(dropped, fmt.Sprintf("policy skill %q (%s) overrides repository skill %q (%s): same selector, the repository skill is not sent to the model",
					winner.Name, winner.Dir, sk.Name, sk.Dir))
				continue
			}
			out.Skills = append(out.Skills, sk)
		}
	}
	sort.Strings(dropped)
	return out, dropped
}

// selectorKey is the canonical identity of a selector: resolved language
// names and path globs, each sorted and deduplicated.
func selectorKey(sel Selector, langs *Languages) string {
	seen := map[string]struct{}{}
	for _, item := range sel.Languages {
		name := strings.ToLower(strings.TrimSpace(item))
		if canon, ok := langs.aliases.Resolve(item); ok {
			name = canon
		}
		seen["l:"+name] = struct{}{}
	}
	for _, g := range sel.Paths {
		seen["p:"+strings.TrimSpace(g)] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x00")
}

// withBudget sets the prompt-token budget of the assembled block.
func (c *Catalog) withBudget(b Budget) *Catalog {
	c.budget = b
	return c
}

// Select returns the skills that apply to changed and every warning the
// selection declares: load notices, overridden repository skills and unknown
// selector languages.
func (c *Catalog) Select(changed []string) Selection {
	sel := c.set.SelectWith(changed, c.langs)
	warnings := append([]string(nil), c.loadWarn...)
	warnings = append(warnings, c.dropped...)
	warnings = append(warnings, sel.Warnings...)
	return Selection{Skills: sel.Skills, Warnings: warnings}
}

// Name identifies the catalog in the rendered prompt and diagnostics.
func (c *Catalog) Name() string { return "skills (.aurumcode/skills/*/SKILL.md)" }

// Provide implements config.ContextProvider. A budget overflow is a loud
// error naming what did not fit; it is never a silent truncation.
func (c *Catalog) Provide(_ context.Context, changed []string) (string, error) {
	sel := c.Select(changed)
	warnings := renderWarnings(sel.Warnings)
	if len(sel.Skills) == 0 {
		return warnings, nil
	}
	res, err := Assemble(sel.Skills, c.budget)
	if err != nil {
		return "", err
	}
	if warnings == "" {
		return res.Text, nil
	}
	return warnings + "\n\n" + res.Text, nil
}

var _ config.ContextProvider = (*Catalog)(nil)
