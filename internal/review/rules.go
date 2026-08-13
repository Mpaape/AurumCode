package review

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// This file is the c12d7ab internal/review/rules.go, restored by AUR-434
// with the one defect that card names corrected: the original loader
// globbed a rules directory relative to the caller's cwd
// (.aurumcode/rules), so running the binary anywhere else found zero rules
// and silently reviewed with none. The catalog is now embedded in the
// binary itself (rules/*.yml below, restored from c12d7ab's
// .aurumcode/rules) and an empty or malformed catalog is a loud error,
// never a silent zero-rule pass-through. The Rule shape and the
// Get/GetAll/GetByCategory surface are unchanged from the restored
// original.

//go:embed rules/*.yml
var embeddedRules embed.FS

// Rule represents a code review rule
type Rule struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Severity    string   `yaml:"severity"`
	Category    string   `yaml:"category"`
	Tags        []string `yaml:"tags"`
	// Pattern, when non-empty, is the rule's matcher: a Go regular
	// expression the AUR-435 security pass applies to each ADDED line of
	// the reviewed diff (see securitypass.go). This is the piece the
	// c12d7ab restoration measured as missing -- the historical rules were
	// metadata without a matcher. A rule without a Pattern stays
	// metadata-only: it can be cited by a model finding (AUR-434) but the
	// security pass never matches it.
	Pattern string `yaml:"pattern"`
	// Standard, when non-empty, names the rule of the enforced project
	// security standard (standards/security-review/rules.md) that scopes
	// this catalog rule, e.g. SCR-001. Security-pass findings cite it as
	// "standards/security-review <id>".
	Standard string `yaml:"standard"`
}

// RulesFile represents a YAML file containing rules
type RulesFile struct {
	Rules []Rule `yaml:"rules"`
}

// RulesLoader loads review rules from the embedded catalog
type RulesLoader struct {
	rules    map[string]Rule           // indexed by ID
	patterns map[string]*regexp.Regexp // compiled matchers, indexed by rule ID
}

// NewRulesLoader creates a new rules loader over the embedded catalog.
func NewRulesLoader() *RulesLoader {
	return &RulesLoader{
		rules:    make(map[string]Rule),
		patterns: make(map[string]*regexp.Regexp),
	}
}

// Load loads all rules from the embedded catalog. It fails loudly: no
// rule files, zero rules, a rule without an id, or a duplicated id is an
// error, because a silently empty catalog would make the reviewer discard
// every finding (see reviewer.go) with no signal to the user.
func (l *RulesLoader) Load() error {
	return l.loadFrom(embeddedRules)
}

// loadFrom is Load over an explicit fs.FS so tests can prove the loud
// failure modes without rebuilding the embedded catalog.
func (l *RulesLoader) loadFrom(fsys fs.FS) error {
	files, err := fs.Glob(fsys, "rules/*.yml")
	if err != nil {
		return fmt.Errorf("failed to list embedded rules files: %w", err)
	}
	yamlFiles, err := fs.Glob(fsys, "rules/*.yaml")
	if err == nil {
		files = append(files, yamlFiles...)
	}
	if len(files) == 0 {
		return fmt.Errorf("no embedded review rules files: the binary was built without a rules catalog")
	}
	sort.Strings(files)

	for _, file := range files {
		if err := l.loadFile(fsys, file); err != nil {
			return fmt.Errorf("failed to load %s: %w", file, err)
		}
	}

	if len(l.rules) == 0 {
		return fmt.Errorf("embedded review rules catalog is empty: %d file(s) declared zero rules", len(files))
	}
	return nil
}

// loadFile loads rules from a single YAML file
func (l *RulesLoader) loadFile(fsys fs.FS, path string) error {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return err
	}

	var rulesFile RulesFile
	if err := yaml.Unmarshal(data, &rulesFile); err != nil {
		return err
	}

	// Index rules by ID. An empty id would make every finding with a
	// missing rule_id resolve against it, silently defeating the rule
	// gate, and a duplicate id would make the winning definition depend
	// on file order; both are rejected loudly.
	for _, rule := range rulesFile.Rules {
		if rule.ID == "" {
			return fmt.Errorf("rule with empty id (title %q)", rule.Title)
		}
		if _, dup := l.rules[rule.ID]; dup {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		// A rule's matcher is compiled at load, so a broken pattern is a
		// loud catalog error at startup -- never a silently unmatchable
		// rule discovered only when a vulnerability slips through.
		if rule.Pattern != "" {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return fmt.Errorf("rule %s has an invalid pattern: %w", rule.ID, err)
			}
			l.patterns[rule.ID] = re
		}
		l.rules[rule.ID] = rule
	}

	return nil
}

// Get retrieves a rule by ID
func (l *RulesLoader) Get(id string) (Rule, bool) {
	rule, ok := l.rules[id]
	return rule, ok
}

// PatternFor returns the compiled matcher of the rule with the given id,
// when that rule declared one. Rules without a pattern -- every quality
// rule, and any security rule that has no line-level shape -- return
// (nil, false) and are never matched by the security pass.
func (l *RulesLoader) PatternFor(id string) (*regexp.Regexp, bool) {
	re, ok := l.patterns[id]
	return re, ok
}

// GetAll returns all loaded rules, sorted by ID for determinism.
func (l *RulesLoader) GetAll() []Rule {
	rules := make([]Rule, 0, len(l.rules))
	for _, rule := range l.rules {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules
}

// GetByCategory returns all rules in a category, sorted by ID for
// determinism.
func (l *RulesLoader) GetByCategory(category string) []Rule {
	var rules []Rule
	for _, rule := range l.rules {
		if rule.Category == category {
			rules = append(rules, rule)
		}
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules
}
