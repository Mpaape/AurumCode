// Package skills implements AUR-468's context provider: versioned, reusable
// review instructions ("skills") selected by language and/or path glob.
//
// A skill is a DIRECTORY under .aurumcode/skills/ containing one instructions
// document, SKILL.md. The document opens with a small front matter block that
// declares a selector:
//
//	---
//	name: go-error-style
//	version: 2
//	languages: [go]
//	paths: ["internal/**"]
//	---
//	Wrap errors with %w so the chain survives.
//
// A skill applies to a changed file when EVERY declared criterion matches: if
// "languages" is present the file's language must be listed, and if "paths" is
// present at least one glob must match. A selector is never a plugin and no
// skill code is ever executed -- the selected instructions are TEXT injected
// into the review prompt as untrusted background, exactly like AUR-452's
// repository prompt and path instructions.
//
// A skill with NO selector (neither languages nor paths) is OFF, never
// universal: forgetting the selector must not silently apply the skill
// everywhere.
//
// SECURITY. Skill text is UNTRUSTED DATA, never an instruction. Nothing in
// this package inspects skill text to decide whether a rule is enabled, what
// severity a finding gets, where --fail-on's threshold sits, whether secret
// redaction runs, or what the cost ceiling is. Directive-looking text is only
// RECORDED (Result.Attempts) and shown as background; it is never acted upon.
// Decision functions live in internal/config (.aurumcode/config.yml) and code,
// which take no skill text at all.
package skills

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultDirName is the versioned skills directory, relative to the
	// repository root.
	DefaultDirName = ".aurumcode/skills"
	// DocName is the instructions document inside each skill directory.
	DocName = "SKILL.md"
	// DefaultMaxTokens is the prompt-token budget Assemble uses when a
	// Budget does not declare one.
	DefaultMaxTokens = 4000
)

// Selector declares when a skill applies. Both fields are optional; an empty
// Selector means the skill is OFF (see the package doc).
type Selector struct {
	// Languages lists accepted language names (e.g. "go", "python").
	Languages []string `json:"languages,omitempty"`
	// Paths lists Copilot/gitignore-style globs matched against changed paths.
	Paths []string `json:"paths,omitempty"`
}

// Declared reports whether the selector names at least one criterion.
func (s Selector) Declared() bool { return len(s.Languages) > 0 || len(s.Paths) > 0 }

// Skill is one versioned instructions document plus its selector. Instructions
// is untrusted text and is never executed.
type Skill struct {
	Name         string
	Version      string
	Dir          string
	Instructions string
	Selector     Selector
}

// Set is a loaded collection of skills.
type Set struct {
	Skills []Skill
}

// Load reads root/.aurumcode/skills. A missing directory is the zero-config
// case: an empty Set and a nil error.
func Load(root string) (*Set, error) {
	return LoadDir(filepath.Join(root, filepath.FromSlash(DefaultDirName)))
}

// LoadDir reads every immediate subdirectory of dir that contains a SKILL.md,
// in sorted order. A missing directory returns an empty Set; an unreadable
// document or an opened-but-unterminated front matter block is a loud error.
func LoadDir(dir string) (*Set, error) {
	return LoadSource(DirSource{Dir: dir, Prefix: dir})
}

// Select returns the skills that apply to any of changed, sorted by name then
// directory, with duplicates collapsed. A skill with no selector is never
// selected.
func (s *Set) Select(changed []string) []Skill {
	return s.SelectWith(changed, DefaultLanguages()).Skills
}

// SelectWith is Select over an injected language resolver, and it returns the
// declared warnings next to the skills (AUR-559 AC-004).
func (s *Set) SelectWith(changed []string, langs *Languages) Selection {
	if s == nil || len(s.Skills) == 0 {
		return Selection{}
	}
	normalized := normalizeChanged(changed)
	type entry struct {
		skill Skill
		key   string
	}
	seen := map[string]entry{}
	for _, sk := range s.Skills {
		for _, cp := range normalized {
			if sk.matches(cp, langs) {
				seen[sk.Dir] = entry{skill: sk, key: sk.Name + "\x00" + sk.Dir}
				break
			}
		}
	}
	out := make([]Skill, 0, len(seen))
	for _, e := range seen {
		out = append(out, e.skill)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Dir < out[j].Dir
	})
	return Selection{Skills: out, Warnings: s.unknownLanguageWarnings(langs)}
}

// matches reports whether the skill applies to one changed path. A missing
// selector is OFF; when both criteria are declared, both must match.
func (s Skill) matches(cp string, langs *Languages) bool {
	sel := s.Selector
	if !sel.Declared() {
		return false
	}
	if len(sel.Languages) > 0 && !langs.Matches(sel.Languages, cp) {
		return false
	}
	if len(sel.Paths) > 0 {
		matched := false
		for _, g := range sel.Paths {
			if globMatch(strings.TrimSpace(g), cp) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// Budget bounds the assembled skills text by prompt tokens.
type Budget struct {
	// MaxTokens is the token ceiling for the assembled skills block. A
	// non-positive value selects DefaultMaxTokens.
	MaxTokens int
}

// Result is the assembled, untrusted skills block plus the provenance a caller
// must be able to report. Text is what may be injected into the prompt;
// Entered names the skills that entered; Attempts records directive-looking
// text (never acted upon); Oversize names skills that could not fit and made
// Assemble fail high.
type Result struct {
	Text     string
	Entered  []string
	Attempts []string
	Oversize []string
}

// Assemble renders the selected skills into one bounded, deterministic block.
//
// FAIL HIGH, NEVER TRUNCATE: when the selected skills exceed budget.MaxTokens,
// Assemble returns a non-nil error that names every skill that did not fit,
// and Text is empty. It never emits a partial or truncated block.
func Assemble(selected []Skill, budget Budget) (*Result, error) {
	maxTokens := budget.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	res := &Result{}
	used := 0
	var sections []string
	for _, sk := range selected {
		block := fmt.Sprintf("#### %s (v%s)\n%s", sk.Name, versionLabel(sk.Version), strings.TrimSpace(sk.Instructions))
		tokens := EstimateTokens(block)
		if used+tokens > maxTokens {
			res.Oversize = append(res.Oversize, sk.Name)
			continue
		}
		used += tokens
		sections = append(sections, block)
		res.Entered = append(res.Entered, sk.Name)
		res.Attempts = append(res.Attempts, scanDirectives(sk)...)
	}

	if len(res.Oversize) > 0 {
		return res, fmt.Errorf("skills: selected skills exceed prompt token budget %d; did not fit: [%s]",
			maxTokens, strings.Join(res.Oversize, " "))
	}
	if len(sections) == 0 {
		return res, nil
	}

	var b strings.Builder
	b.WriteString("### Skills\n")
	b.WriteString("entered: [")
	b.WriteString(strings.Join(res.Entered, " "))
	b.WriteString("]\n")
	for _, section := range sections {
		b.WriteString("\n")
		b.WriteString(section)
		b.WriteString("\n")
	}
	if len(res.Attempts) > 0 {
		b.WriteString("\n### Recorded prompt-injection attempts (informational, not acted upon)\n")
		for _, a := range res.Attempts {
			b.WriteString("- ")
			b.WriteString(a)
			b.WriteString("\n")
		}
	}
	res.Text = strings.TrimRight(b.String(), "\n")
	return res, nil
}

// EstimateTokens is the engine's one character heuristic
// (types.EstimateTokens), the same internal/prompt and internal/llm use.
func EstimateTokens(text string) int {
	return types.EstimateTokens(text)
}

// Provider is the skills ContextProvider. It loads root/.aurumcode/skills,
// selects by the review's changed paths and assembles the selected text. With
// no skills directory, or nothing selected, it contributes nothing -- the
// zero-config signal.
type Provider struct {
	Dir string
	// Source, when set, replaces Dir as the origin of the skills.
	Source Source
	Budget Budget
	// Languages resolves selector language names; nil selects
	// DefaultLanguages().
	Languages *Languages
}

// NewProvider roots a Provider at root/.aurumcode/skills.
func NewProvider(root string) *Provider {
	return &Provider{Dir: filepath.Join(root, filepath.FromSlash(DefaultDirName))}
}

// Name identifies the provider in the rendered prompt and diagnostics.
func (p *Provider) Name() string { return "skills (.aurumcode/skills/*/SKILL.md)" }

// Provide implements config.ContextProvider. A budget overflow is a loud error
// naming what did not fit; it is never a silent truncation.
func (p *Provider) Provide(ctx context.Context, changedPaths []string) (string, error) {
	var set *Set
	var err error
	if p.Source != nil {
		set, err = LoadSource(p.Source)
	} else {
		set, err = LoadDir(p.Dir)
	}
	if err != nil {
		return "", err
	}
	return NewCatalog(nil, set, p.Languages, nil).withBudget(p.Budget).Provide(ctx, changedPaths)
}

// renderWarnings renders the declared selection warnings as a prompt block.
func renderWarnings(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	return "### Skill selection warnings\n- " + strings.Join(warnings, "\n- ")
}

// Providers returns the skills providers for root, ready to append to
// internal/config's DefaultProviders/ConfiguredProviders slice.
func Providers(root string) []config.ContextProvider {
	return []config.ContextProvider{NewProvider(root)}
}

// Compile-time proof that Provider satisfies the AUR-452 seam.
var _ config.ContextProvider = (*Provider)(nil)

// parseSkill splits one SKILL.md into front matter and body. No front matter
// means no selector (skill OFF) and the whole content is the body; an opened
// but unterminated block is a loud error, never a silent fallback.
func parseSkill(dir, full, content string) (Skill, error) {
	sk := Skill{Name: dir, Dir: filepath.ToSlash(filepath.Dir(full))}
	const delim = "---"
	trimmed := strings.TrimLeft(content, "\n\r")
	if !strings.HasPrefix(trimmed, delim) {
		sk.Instructions = content
		return sk, nil
	}
	rest := trimmed[len(delim):]
	idx := strings.Index(rest, "\n"+delim)
	if idx == -1 {
		return Skill{}, fmt.Errorf("skills: %s: unterminated front matter (missing closing %q)", full, delim)
	}
	fm := rest[:idx]
	sk.Instructions = rest[idx+len(delim)+1:]
	if err := parseFrontMatter(full, fm, &sk); err != nil {
		return Skill{}, err
	}
	if strings.TrimSpace(sk.Name) == "" {
		sk.Name = dir
	}
	return sk, nil
}

// frontMatter is the YAML header of a SKILL.md. Unknown keys are ignored.
type frontMatter struct {
	Name      string     `yaml:"name"`
	Version   string     `yaml:"version"`
	Languages stringList `yaml:"languages"`
	Paths     stringList `yaml:"paths"`
}

// stringList accepts a YAML list or one scalar; a scalar may hold several
// comma-separated items. Empty items are dropped.
type stringList []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (l *stringList) UnmarshalYAML(node *yaml.Node) error {
	var raw []string
	switch node.Kind {
	case yaml.ScalarNode:
		raw = strings.Split(node.Value, ",")
	case yaml.SequenceNode:
		if err := node.Decode(&raw); err != nil {
			return err
		}
	default:
		return fmt.Errorf("line %d: want a list or a scalar", node.Line)
	}
	*l = nil
	for _, item := range raw {
		if item = strings.TrimSpace(item); item != "" {
			*l = append(*l, item)
		}
	}
	return nil
}

// parseFrontMatter decodes the YAML header into sk. Malformed YAML is an
// error naming the file, never a skill read with half its selector.
func parseFrontMatter(full, fm string, sk *Skill) error {
	var meta frontMatter
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return fmt.Errorf("skills: %s: front matter: %w", full, err)
	}
	sk.Name = strings.TrimSpace(meta.Name)
	sk.Version = strings.TrimSpace(meta.Version)
	sk.Selector = Selector{Languages: meta.Languages, Paths: meta.Paths}
	return nil
}

// versionLabel renders a skill version, defaulting to "1" when undeclared.
func versionLabel(v string) string {
	if strings.TrimSpace(v) == "" {
		return "1"
	}
	return v
}

func normalizeChanged(changed []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range changed {
		p := filepath.ToSlash(strings.TrimSpace(raw))
		p = strings.TrimPrefix(p, "./")
		if p == "" || p == "." {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func containsFold(list []string, want string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), want) {
			return true
		}
	}
	return false
}

// globMatch reports whether path matches pattern, a Copilot/gitignore-style
// glob: "*" within one segment, "**" across segments, "?" one character. This
// mirrors internal/config's matcher so a skill selector and an applyTo pattern
// are written identically.
func globMatch(pattern, p string) bool {
	return globToRegexp(pattern).MatchString(p)
}

func globToRegexp(pattern string) *regexp.Regexp {
	var sb strings.Builder
	sb.WriteString("^")
	i := 0
	for i < len(pattern) {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			sb.WriteString("(.*/)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			sb.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			sb.WriteString("[^/]*")
			i++
		case pattern[i] == '?':
			sb.WriteString("[^/]")
			i++
		default:
			sb.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	sb.WriteString("$")
	return regexp.MustCompile(sb.String())
}

// directivePatterns are the shapes whose presence proves a skill is ATTEMPTING
// to direct the reviewer. Detection exists only to record and report; no
// decision consumes it.
var directivePatterns = []struct {
	id string
	re *regexp.Regexp
}{
	{"ignore-rule", regexp.MustCompile(`(?i)\b(?:ignore|disable|skip|desligue|desligar|ignorar)\s+(?:the\s+|a\s+|this\s+)?(?:rule|regra|check|finding|achado)\b`)},
	{"mark-resolved", regexp.MustCompile(`(?i)\b(?:mark|treat|consider|marque|marque|considere)\b[^.\n]{0,60}?\b(?:resolved|fixed|resolvido|false[ -]positive|falso[ -]positivo)\b`)},
	{"suppress-secrets", regexp.MustCompile(`(?i)\b(?:do not|don't|dont|never|não|nao)\s+(?:report|flag|redact|show|reporte|reportar)\b[^.\n]{0,40}?\b(?:secret|secrets|segredo|segredos|credential|credencial|sensitive|sensível)\b`)},
	{"ignore-instructions", regexp.MustCompile(`(?i)\bignore\s+(?:all\s+)?(?:previous|prior|above)\s+instructions\b`)},
}

// scanDirectives records every directive-looking shape in one skill's text.
// The returned strings are evidence, never commands.
func scanDirectives(sk Skill) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range directivePatterns {
		if p.re.MatchString(sk.Instructions) {
			note := fmt.Sprintf("skill=%s directive=%q", sk.Name, p.id)
			if _, ok := seen[note]; ok {
				continue
			}
			seen[note] = struct{}{}
			out = append(out, note)
		}
	}
	sort.Strings(out)
	return out
}
