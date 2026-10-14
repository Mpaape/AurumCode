package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// The closed vocabulary of scheduled.plugins[].output.format.
const (
	PluginFormatSARIF        = "sarif"
	PluginFormatVVAHFindings = "vvah-findings"
	// DefaultPluginTimeoutMinutes is a plugin's run time when the entry does
	// not declare one. It is a default, never a ceiling: any positive value
	// is accepted.
	DefaultPluginTimeoutMinutes = 60
	// PluginJobSlackMinutes is added to a plugin's timeout for the job
	// around it (image build, ingest, upload), so the plugin's own timeout
	// is the one that fires first and the log can say so.
	PluginJobSlackMinutes = 30
)

var (
	// pluginNamePattern keeps a plugin name usable verbatim in a file name,
	// a SARIF ruleId and a code scanning category.
	pluginNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	// pluginEnvPattern is a variable NAME; a value never lives in the config.
	pluginEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// pluginDigestPattern pins an image by its content digest.
	pluginDigestPattern = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
)

// ScheduledConfig is the `scheduled` section (AUR-622): what the scheduled
// run of the reusable workflow executes beyond the dependency scan. nil
// means "not declared" and nothing changes.
type ScheduledConfig struct {
	Plugins []PluginConfig `yaml:"plugins"`
}

// PluginConfig is one external plugin: its own container image, never code
// compiled into the binary. The image is pinned by digest or built from a
// Dockerfile read from the trusted default branch; env lists only variable
// NAMES whose values come from the caller's secrets.
type PluginConfig struct {
	Name           string             `yaml:"name"`
	Image          string             `yaml:"image"`
	Dockerfile     string             `yaml:"dockerfile"`
	Args           []string           `yaml:"args"`
	Env            []string           `yaml:"env"`
	Output         PluginOutputConfig `yaml:"output"`
	TimeoutMinutes int                `yaml:"timeout_minutes"`
	// Required, in a central policy, keeps the repository from removing or
	// replacing the entry of the same name.
	Required bool `yaml:"required"`
}

// PluginOutputConfig is where the plugin leaves its result and in which
// format: path is relative to the scanned tree.
type PluginOutputConfig struct {
	Format string `yaml:"format"`
	Path   string `yaml:"path"`
}

// Declared reports whether the section was written (nil-safe).
func (s *ScheduledConfig) Declared() bool { return s != nil }

// EffectivePlugins is the declared plugins (nil-safe).
func (s *ScheduledConfig) EffectivePlugins() []PluginConfig {
	if s == nil {
		return nil
	}
	return s.Plugins
}

// EffectiveTimeoutMinutes applies the default.
func (p PluginConfig) EffectiveTimeoutMinutes() int {
	if p.TimeoutMinutes <= 0 {
		return DefaultPluginTimeoutMinutes
	}
	return p.TimeoutMinutes
}

// JobTimeoutMinutes is the timeout of the job that runs the plugin.
func (p PluginConfig) JobTimeoutMinutes() int {
	return p.EffectiveTimeoutMinutes() + PluginJobSlackMinutes
}

func (p PluginConfig) label() string {
	return fmt.Sprintf("scheduled.plugins[%s]", p.Name)
}

// Validate refuses a plugin without a pinned image or a Dockerfile, an env
// entry carrying a value, an output path outside the tree and a repeated
// name. It is the reading of the configuration that fails, before any
// container runs.
func (s *ScheduledConfig) Validate() error {
	if s == nil {
		return nil
	}
	seen := map[string]bool{}
	for i, p := range s.Plugins {
		if !pluginNamePattern.MatchString(p.Name) {
			return fmt.Errorf("scheduled.plugins[%d].name: %q is not a valid plugin name (lowercase letters, digits, '-' and '_')", i, p.Name)
		}
		if seen[p.Name] {
			return fmt.Errorf("%s: plugin name declared twice", p.label())
		}
		seen[p.Name] = true
		if err := p.validate(); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePolicy refuses, in a central policy, a plugin built from a
// Dockerfile: the job builds it from the scanned repository's default
// branch, so the repository could replace the policy's plugin by one that
// reports nothing and closes the open alerts. A policy plugin is an image
// pinned by digest.
func (s *ScheduledConfig) ValidatePolicy() error {
	for _, p := range s.EffectivePlugins() {
		if strings.TrimSpace(p.Dockerfile) != "" {
			return fmt.Errorf("%s.dockerfile: a central policy plugin must be an image pinned by digest (@sha256:); a Dockerfile would be read from the scanned repository", p.label())
		}
	}
	return nil
}

func (p PluginConfig) validate() error {
	image, dockerfile := strings.TrimSpace(p.Image), strings.TrimSpace(p.Dockerfile)
	switch {
	case image == "" && dockerfile == "":
		return fmt.Errorf("%s: declare image pinned by digest (@sha256:) or dockerfile", p.label())
	case image != "" && dockerfile != "":
		return fmt.Errorf("%s: declare image or dockerfile, not both", p.label())
	case image != "" && !pluginDigestPattern.MatchString(image):
		return fmt.Errorf("%s.image: %q is not pinned by digest (@sha256:<64 hex>)", p.label(), image)
	case dockerfile != "" && !localPath(dockerfile):
		return fmt.Errorf("%s.dockerfile: %q must be a relative path inside the repository", p.label(), dockerfile)
	}
	for _, name := range p.Env {
		if !pluginEnvPattern.MatchString(name) {
			return fmt.Errorf("%s.env: %q is not a variable name; list only names, the values come from the caller's secrets", p.label(), name)
		}
	}
	switch p.Output.Format {
	case PluginFormatSARIF, PluginFormatVVAHFindings:
	default:
		return fmt.Errorf("%s.output.format: %q is not %s or %s", p.label(), p.Output.Format, PluginFormatSARIF, PluginFormatVVAHFindings)
	}
	if !localPath(p.Output.Path) {
		return fmt.Errorf("%s.output.path: %q must be a relative path inside the scanned tree", p.label(), p.Output.Path)
	}
	if p.TimeoutMinutes < 0 {
		return fmt.Errorf("%s.timeout_minutes: %d is negative", p.label(), p.TimeoutMinutes)
	}
	return nil
}

// localPath is a non-empty relative path that stays inside its root: no
// absolute path, no "..", no backslash form.
func localPath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || strings.Contains(p, `\`) {
		return false
	}
	return filepath.IsLocal(filepath.FromSlash(p))
}

// governScheduled resolves the plugins of a policy and a repository name by
// name, as mergeScanners does for engines: a required policy entry wins and
// the repository's entry of the same name is dropped with a warning; a
// policy entry that is not required yields to the repository's; an entry
// only one side declares is kept.
func governScheduled(effective, repo, central *Config) []ProviderWarning {
	if central.Scheduled == nil {
		return nil
	}
	policy := map[string]PluginConfig{}
	for _, p := range central.Scheduled.Plugins {
		policy[p.Name] = p
	}
	var warnings []ProviderWarning
	merged := &ScheduledConfig{}
	repoWins := map[string]bool{}
	for _, r := range repo.Scheduled.EffectivePlugins() {
		p, declared := policy[r.Name]
		if declared && p.Required {
			warnings = append(warnings, ignoredWarning(r.label()+" do config do repositório foi ignorado: a política central decide sozinha"))
			continue
		}
		if declared {
			warnings = append(warnings, ignoredWarning(r.label()+" da política central não é obrigatório (required: false): vale a entrada do repositório"))
		}
		repoWins[r.Name] = true
	}
	for _, p := range central.Scheduled.Plugins {
		if !repoWins[p.Name] {
			merged.Plugins = append(merged.Plugins, p)
		}
	}
	for _, r := range repo.Scheduled.EffectivePlugins() {
		if repoWins[r.Name] {
			merged.Plugins = append(merged.Plugins, r)
		}
	}
	effective.Scheduled = merged
	return warnings
}
