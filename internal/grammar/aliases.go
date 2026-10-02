package grammar

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed catalog/aliases.yml
var embeddedAliases []byte

// PolicyAliasesPath is where a central policy overrides the alias catalog,
// relative to the policy checkout root.
const PolicyAliasesPath = ".aurumcode/grammar/aliases.yml"

type aliasFile struct {
	Version int               `yaml:"version"`
	Aliases map[string]string `yaml:"aliases"`
}

// Aliases resolves a language name used by a consumer (a selector spelling) to
// the runtime's canonical grammar name. It is data: the embedded
// catalog/aliases.yml, optionally replaced section by section by the policy.
type Aliases struct {
	canonical map[string]string // lower-case canonical name -> canonical name
	aliases   map[string]string // lower-case alias -> canonical name
	// Source is "embedded" or "policy".
	Source string
}

// LoadAliases builds the catalog over the languages p offers. policyRoot is
// the central policy checkout ("" for none): when it holds
// .aurumcode/grammar/aliases.yml, its `aliases` section replaces the embedded
// one (precedence by section). Parsing is strict: an unknown key, or an alias
// whose target is not a grammar of the runtime, is a load error.
func LoadAliases(p Provider, policyRoot string) (*Aliases, error) {
	data, source := embeddedAliases, "embedded"
	if policyRoot != "" {
		raw, err := os.ReadFile(filepath.Join(policyRoot, filepath.FromSlash(PolicyAliasesPath)))
		switch {
		case err == nil:
			data, source = raw, "policy"
		case !os.IsNotExist(err):
			return nil, fmt.Errorf("grammar aliases: %w", err)
		}
	}
	var f aliasFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("grammar aliases (%s): %w", source, err)
	}
	a := &Aliases{canonical: map[string]string{}, aliases: map[string]string{}, Source: source}
	for _, name := range p.Languages() {
		a.canonical[strings.ToLower(name)] = name
	}
	var bad []string
	for alias, target := range f.Aliases {
		canon, ok := a.canonical[strings.ToLower(strings.TrimSpace(target))]
		if !ok || strings.TrimSpace(alias) == "" {
			bad = append(bad, alias+" -> "+target)
			continue
		}
		a.aliases[strings.ToLower(strings.TrimSpace(alias))] = canon
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return nil, fmt.Errorf("grammar aliases (%s): target is not a grammar of the runtime: %s", source, strings.Join(bad, ", "))
	}
	return a, nil
}

// Resolve returns the canonical grammar name for name: a canonical name
// itself, or an alias. ok is false for a name the catalog does not know.
func (a *Aliases) Resolve(name string) (canonical string, ok bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if c, found := a.canonical[key]; found {
		return c, true
	}
	c, found := a.aliases[key]
	return c, found
}
