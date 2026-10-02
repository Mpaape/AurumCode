package skills

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

// Selection is the outcome of selecting skills for a change: the skills that
// apply and the warnings the selection must declare. An unknown language name
// in a selector is never ignored in silence (AUR-559 AC-004).
type Selection struct {
	Skills   []Skill
	Warnings []string
}

// Languages resolves the language names a selector uses. It holds no language
// table: the file's language comes from a grammar.Provider and the accepted
// spellings from the data catalog of grammar.Aliases.
type Languages struct {
	provider grammar.Provider
	aliases  *grammar.Aliases
}

// NewLanguages builds a resolver over injected collaborators.
func NewLanguages(p grammar.Provider, a *grammar.Aliases) *Languages {
	return &Languages{provider: p, aliases: a}
}

// DefaultLanguages is the resolver over the default runtime and the embedded
// alias catalog. The embedded catalog is validated by the grammar tests, so a
// load error here is a build defect; the resolver then knows only canonical
// names (aliases fail loud as unknown, never silently).
func DefaultLanguages() *Languages {
	p := grammar.Default()
	a, err := grammar.LoadAliases(p, "")
	if err != nil {
		a, _ = grammar.LoadAliases(emptyProvider{p}, "")
	}
	return NewLanguages(p, a)
}

// emptyProvider reports no languages, which makes the alias catalog empty of
// aliases and canonical names; it exists only for the unreachable fallback.
type emptyProvider struct{ grammar.Provider }

func (emptyProvider) Languages() []string { return nil }

// Of returns the canonical grammar name of a changed path, or "" when the
// provider has none.
func (l *Languages) Of(p string) string { return l.provider.Detect(p, nil) }

// Matches reports whether any entry of list names the language of p, as a
// canonical name or as a cataloged alias. An unknown entry matches nothing
// (and is reported by Set.SelectWith).
func (l *Languages) Matches(list []string, p string) bool {
	lang := l.Of(p)
	if lang == "" {
		return false
	}
	for _, item := range list {
		if canon, ok := l.aliases.Resolve(item); ok && canon == lang {
			return true
		}
	}
	return false
}

// unknownLanguageWarnings declares every selector language the catalog and the
// runtime do not know, once per skill and name.
func (s *Set) unknownLanguageWarnings(langs *Languages) []string {
	var out []string
	for _, sk := range s.Skills {
		for _, item := range sk.Selector.Languages {
			if _, ok := langs.aliases.Resolve(item); !ok {
				out = append(out, fmt.Sprintf("skill %q (%s): unknown language %q in selector; it matches nothing (add an alias to %s or use a grammar name)",
					sk.Name, filepath.ToSlash(sk.Dir), strings.TrimSpace(item), grammar.PolicyAliasesPath))
			}
		}
	}
	sort.Strings(out)
	return out
}
