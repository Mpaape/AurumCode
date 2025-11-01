package review

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Rule represents a code review rule
type Rule struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Severity    string   `yaml:"severity"`
	Category    string   `yaml:"category"`
	Tags        []string `yaml:"tags"`
}

// RulesFile represents a YAML file containing rules
type RulesFile struct {
	Rules []Rule `yaml:"rules"`
}

// RulesLoader loads review rules from YAML files
type RulesLoader struct {
	rulesDir string
	rules    map[string]Rule // indexed by ID
}

// NewRulesLoader creates a new rules loader
func NewRulesLoader(rulesDir string) *RulesLoader {
	return &RulesLoader{
		rulesDir: rulesDir,
		rules:    make(map[string]Rule),
	}
}

// Load loads all rules from the rules directory
func (l *RulesLoader) Load() error {
	// Read all YAML files in rules directory
	files, err := filepath.Glob(filepath.Join(l.rulesDir, "*.yml"))
	if err != nil {
		return fmt.Errorf("failed to list rules files: %w", err)
	}

	yamlFiles, err := filepath.Glob(filepath.Join(l.rulesDir, "*.yaml"))
	if err == nil {
		files = append(files, yamlFiles...)
	}

	// Load each file
	for _, file := range files {
		if err := l.loadFile(file); err != nil {
			return fmt.Errorf("failed to load %s: %w", file, err)
		}
	}

	return nil
}

// loadFile loads rules from a single YAML file
func (l *RulesLoader) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var rulesFile RulesFile
	if err := yaml.Unmarshal(data, &rulesFile); err != nil {
		return err
	}

	// Index rules by ID
	for _, rule := range rulesFile.Rules {
		l.rules[rule.ID] = rule
	}

	return nil
}

// Get retrieves a rule by ID
func (l *RulesLoader) Get(id string) (Rule, bool) {
	rule, ok := l.rules[id]
	return rule, ok
}

// GetAll returns all loaded rules
func (l *RulesLoader) GetAll() []Rule {
	rules := make([]Rule, 0, len(l.rules))
	for _, rule := range l.rules {
		rules = append(rules, rule)
	}
	return rules
}

// GetByCategory returns all rules in a category
func (l *RulesLoader) GetByCategory(category string) []Rule {
	var rules []Rule
	for _, rule := range l.rules {
		if rule.Category == category {
			rules = append(rules, rule)
		}
	}
	return rules
}
