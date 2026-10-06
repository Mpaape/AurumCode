package review

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// AUR-519: a central policy (and, when a repository opts in, the
// repository's own skills) checks for what a security review wants in a
// Markdown skill, not in a rule written in Go. This file turns every
// top-level `## ` section of a skill's Markdown content into one citable
// Rule, exactly as if it had come from the embedded YAML catalog
// (rules.go): id `<skill-file-base>#<slug-of-heading>`, Title the heading
// text, Description the section body. A skill is read at every run -- never
// compiled in -- so a new section is citable on the very next review with
// no code change (AC-007).
//
// # Severity, Origin
//
// Severity defaults to "warning" (the engine's own vocabulary, the one
// Rule.Severity already uses -- see rules.go) and can be overridden by the
// section's own first line reading exactly "severity: error|warning|info".
// That is a narrower grammar than the policy's own gate.fail_on aliases
// (internal/config.NormalizeGateSeverity accepts "critical"/"high"/etc.):
// a skill section is free-form Markdown a human wrote for a human, and this
// one optional line is the only directive this engine ever reads out of it.
//
// Origin records where the skill came from ("policy" or "repo") so
// cmd/aurumcode's gate evaluation can apply AC-005: only policy-origin
// rules (or a repository's own explicit gate opt-in) ever fail the check,
// while every section -- either origin -- is always citable by a finding.
const dynamicRuleDescriptionLimit = 400

// skillHeadingPattern matches one Markdown "## " heading line. Headings
// deeper than "## " (###, ####...) are deliberately not sections of their
// own: the card's Mecanismo names "cada secao `##`", and subsections stay
// part of the enclosing section's body/Description.
var skillHeadingPattern = regexp.MustCompile(`(?m)^##[ \t]+(.+?)[ \t]*$`)

// ParseSkillSections returns one Rule per "## " heading of content. An empty
// or heading-less skill returns nil, never an error: a skill with no
// sections simply contributes no dynamic rule, exactly like today.
func ParseSkillSections(skillFile, content, origin string) []Rule {
	base := skillRuleBase(skillFile)
	if base == "" {
		return nil
	}
	matches := skillHeadingPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}
	rules := make([]Rule, 0, len(matches))
	for i, m := range matches {
		title := strings.TrimSpace(content[m[2]:m[3]])
		if title == "" {
			continue
		}
		slug := slugifyHeading(title)
		if slug == "" {
			continue
		}
		bodyStart := m[1]
		bodyEnd := len(content)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		severity, description := extractSkillSectionSeverity(strings.TrimSpace(content[bodyStart:bodyEnd]))
		if len(description) > dynamicRuleDescriptionLimit {
			description = strings.TrimSpace(description[:dynamicRuleDescriptionLimit]) + "..."
		}
		rules = append(rules, Rule{
			ID:          fmt.Sprintf("%s#%s", base, slug),
			Title:       title,
			Description: description,
			Severity:    severity,
			Origin:      origin,
		})
	}
	return rules
}

// skillDocStem is the file name, without extension, of the document inside a
// skill directory (.aurumcode/skills/<name>/SKILL.md).
const skillDocStem = "SKILL"

// skillRuleBase is the stable prefix of a skill file's rule ids: the file
// name without extension, or, for a skill directory's SKILL.md, the
// directory's name. A skill directory therefore yields the same ids whether
// it reaches the review through the directory or listed by path, and two
// skill directories never share a prefix.
func skillRuleBase(skillFile string) string {
	clean := filepath.ToSlash(filepath.Clean(skillFile))
	base := strings.TrimSuffix(path.Base(clean), path.Ext(clean))
	if base == skillDocStem {
		if dir := path.Base(path.Dir(clean)); dir != "." && dir != "/" {
			return dir
		}
	}
	return base
}

// extractSkillSectionSeverity reads body's own first non-blank line as an
// optional "severity: <error|warning|info>" directive. An absent or
// unrecognized value defaults to "warning" and leaves the whole body as the
// Description; a recognized value is consumed and excluded from it.
func extractSkillSectionSeverity(body string) (severity, description string) {
	first, rest, _ := strings.Cut(body, "\n")
	label, value, ok := strings.Cut(strings.TrimSpace(first), ":")
	if ok && strings.EqualFold(strings.TrimSpace(label), "severity") {
		switch v := strings.ToLower(strings.TrimSpace(value)); v {
		case "error", "warning", "info":
			return v, strings.TrimSpace(rest)
		}
	}
	return "warning", body
}

// slugifyHeading lowercases title and replaces every run of non-alphanumeric
// characters with a single '-', trimming any leading/trailing '-'. It is the
// one normalization the card's id scheme names ("slug da secao"): stable
// across re-reading the same heading and insensitive to incidental
// Markdown punctuation (":", "?", multiple spaces).
func slugifyHeading(title string) string {
	var b strings.Builder
	dash := true // true suppresses a leading '-'
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// resolveRule looks id up against the embedded catalog first, then against
// extra (AUR-519's per-run dynamic skill-section rules). The embedded
// catalog always wins on a collision: a skill section can never shadow a
// built-in rule id to relax or hide it.
func resolveRule(rules *RulesLoader, extra map[string]Rule, id string) (Rule, bool) {
	if rule, ok := rules.Get(id); ok {
		return rule, true
	}
	if extra == nil {
		return Rule{}, false
	}
	rule, ok := extra[id]
	return rule, ok
}
