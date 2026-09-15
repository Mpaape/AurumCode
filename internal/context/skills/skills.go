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
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &Set{}, nil
		}
		return nil, fmt.Errorf("skills: reading %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	set := &Set{}
	for _, name := range names {
		full := filepath.Join(dir, name, DocName)
		data, err := os.ReadFile(full)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("skills: reading %s: %w", full, err)
		}
		sk, err := parseSkill(name, full, string(data))
		if err != nil {
			return nil, err
		}
		set.Skills = append(set.Skills, sk)
	}
	return set, nil
}

// Select returns the skills that apply to any of changed, sorted by name then
// directory, with duplicates collapsed. A skill with no selector is never
// selected.
func (s *Set) Select(changed []string) []Skill {
	if s == nil || len(s.Skills) == 0 {
		return nil
	}
	normalized := normalizeChanged(changed)
	type entry struct {
		skill Skill
		key   string
	}
	seen := map[string]entry{}
	for _, sk := range s.Skills {
		for _, cp := range normalized {
			if sk.matches(cp) {
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
	return out
}

// matches reports whether the skill applies to one changed path. A missing
// selector is OFF; when both criteria are declared, both must match.
func (s Skill) matches(cp string) bool {
	sel := s.Selector
	if !sel.Declared() {
		return false
	}
	if len(sel.Languages) > 0 && !matchesLanguage(sel.Languages, cp) {
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

// EstimateTokens approximates tokens at ~4 characters per token, never zero
// for non-empty text (the same heuristic internal/prompt and internal/llm
// use).
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	if n := len(text) / 4; n > 0 {
		return n
	}
	return 1
}

// Provider is the skills ContextProvider. It loads root/.aurumcode/skills,
// selects by the review's changed paths and assembles the selected text. With
// no skills directory, or nothing selected, it contributes nothing -- the
// zero-config signal.
type Provider struct {
	Dir    string
	Budget Budget
}

// NewProvider roots a Provider at root/.aurumcode/skills.
func NewProvider(root string) *Provider {
	return &Provider{Dir: filepath.Join(root, filepath.FromSlash(DefaultDirName))}
}

// Name identifies the provider in the rendered prompt and diagnostics.
func (p *Provider) Name() string { return "skills (.aurumcode/skills/*/SKILL.md)" }

// Provide implements config.ContextProvider. A budget overflow is a loud error
// naming what did not fit; it is never a silent truncation.
func (p *Provider) Provide(_ context.Context, changedPaths []string) (string, error) {
	set, err := LoadDir(p.Dir)
	if err != nil {
		return "", err
	}
	selected := set.Select(changedPaths)
	if len(selected) == 0 {
		return "", nil
	}
	res, err := Assemble(selected, p.Budget)
	if err != nil {
		return "", err
	}
	return res.Text, nil
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
	parseFrontMatter(fm, &sk)
	if strings.TrimSpace(sk.Name) == "" {
		sk.Name = dir
	}
	return sk, nil
}

// parseFrontMatter reads the flat key/value subset skills use: scalars and
// inline ("[a, b]") or block ("- a") lists for languages and paths.
func parseFrontMatter(fm string, sk *Skill) {
	lastList := ""
	var languages, paths []string
	for _, raw := range strings.Split(fm, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			item := yamlScalar(strings.TrimSpace(line[2:]))
			switch lastList {
			case "languages":
				languages = append(languages, item)
			case "paths":
				paths = append(paths, item)
			}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		value := strings.TrimSpace(v)
		lastList = ""
		switch key {
		case "name":
			sk.Name = yamlScalar(value)
		case "version":
			sk.Version = yamlScalar(value)
		case "languages":
			lastList = "languages"
			languages = append(languages, yamlList(value)...)
		case "paths":
			lastList = "paths"
			paths = append(paths, yamlList(value)...)
		}
	}
	sk.Selector = Selector{Languages: languages, Paths: paths}
}

// yamlScalar trims surrounding quotes and whitespace.
func yamlScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// yamlList parses an inline list ("[a, b]") or a single scalar into items.
func yamlList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "[") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		item := yamlScalar(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
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

// languageOf maps a file extension to the canonical language name a selector
// may use. The raw extension is also accepted by matching (see matches), so a
// selector may say "ts" as well as "typescript".
func languageOf(p string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(p), "."))
	switch ext {
	case "go":
		return "go"
	case "py", "pyw", "pyi":
		return "python"
	case "js", "jsx", "mjs", "cjs":
		return "javascript"
	case "ts", "tsx":
		return "typescript"
	case "rb":
		return "ruby"
	case "rs":
		return "rust"
	case "java":
		return "java"
	case "kt", "kts":
		return "kotlin"
	case "c", "h":
		return "c"
	case "cc", "cpp", "cxx", "hpp", "hxx", "hh":
		return "cpp"
	case "cs":
		return "csharp"
	case "sh", "bash":
		return "shell"
	case "yml", "yaml":
		return "yaml"
	case "json":
		return "json"
	case "md", "markdown":
		return "markdown"
	case "tf":
		return "terraform"
	default:
		return ext
	}
}

// matchesLanguage accepts either the canonical language name or the raw
// extension, so "go" and "golang" both work through the common spellings.
func matchesLanguage(list []string, p string) bool {
	lang := languageOf(p)
	if containsFold(list, lang) {
		return true
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(p), "."))
	return containsFold(list, ext)
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
