// Package rules holds the embedded review rule catalog (*.yml in this
// directory) and exposes its rule ids, so every consumer -- the rule gate
// in internal/review and the closed rule_id list in the review prompt --
// derives from the same files instead of keeping a hand-written mirror.
package rules

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed *.yml
var catalogFS embed.FS

// catalogFile is the part of a rules file this package reads: the ids.
type catalogFile struct {
	Rules []struct {
		ID string `yaml:"id"`
	} `yaml:"rules"`
}

// IDs returns every rule id of the embedded catalog, sorted. An empty
// catalog, an empty id or a duplicate id is an error: the prompt must never
// teach the model a list the gate would read differently.
func IDs() ([]string, error) {
	files, err := fs.Glob(catalogFS, "*.yml")
	if err != nil {
		return nil, fmt.Errorf("listing the embedded rule catalog: %w", err)
	}
	seen := map[string]bool{}
	var ids []string
	for _, name := range files {
		raw, err := catalogFS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		var file catalogFile
		if err := yaml.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		for _, rule := range file.Rules {
			if rule.ID == "" || seen[rule.ID] {
				return nil, fmt.Errorf("%s: empty or duplicate rule id %q", name, rule.ID)
			}
			seen[rule.ID] = true
			ids = append(ids, rule.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("the embedded rule catalog declares no rules")
	}
	sort.Strings(ids)
	return ids, nil
}
