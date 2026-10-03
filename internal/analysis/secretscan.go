package analysis

import (
	"math"
	"regexp"
	"strings"
)

// secretRule is one compiled rule of the secret catalog, evaluated with the
// gitleaks semantics: keywords pre-filter the line case-insensitively, the
// regex locates candidates, the secret is the configured capture group (or
// the first non-empty group, or the whole match), the Shannon entropy of
// the secret must exceed the rule's minimum, and the rule's allowlists plus
// the catalog's global allowlist can still discard the candidate.
type secretRule struct {
	id          string
	description string
	origin      ruleOrigin
	re          *regexp.Regexp
	path        *regexp.Regexp
	secretGroup int
	entropy     float64
	keywords    []string
	allowlists  []*allowlist
	outsideStr  bool
}

// secretCandidate is one regex hit under evaluation. It is never exposed:
// the secret value stays inside the evaluator and no Finding carries it.
type secretCandidate struct {
	path   string
	line   string
	match  string
	secret string
}

// firstMatch returns the first rule that reports a secret on line, in
// catalog order (local rules before base ones, so the long-standing keyword
// assignment keeps its fixed message), so a line yields at most one
// hardcoded-secret finding. The global allowlist, including its file-path
// exclusions, belongs to the public base and applies only to base rules.
// inRaw is the raw-string state entering the line.
func (c *secretCatalog) firstMatch(path, line string, inRaw bool) (*secretRule, bool) {
	baseExcluded := c.global.allowsPath(path)
	lower := strings.ToLower(line)
	for _, r := range c.rules {
		if baseExcluded && r.origin == originBase {
			continue
		}
		if r.matches(c.global, path, line, lower, inRaw) {
			return r, true
		}
	}
	return nil, false
}

func (r *secretRule) matches(global *allowlist, path, line, lower string, inRaw bool) bool {
	if r.path != nil && !r.path.MatchString(path) {
		return false
	}
	if !r.hasKeyword(lower) {
		return false
	}
	for _, loc := range r.re.FindAllStringSubmatchIndex(line, -1) {
		if r.outsideStr && isInsideStringLiteral(line, loc[0], inRaw) {
			continue
		}
		cand := secretCandidate{path: path, line: line, match: line[loc[0]:loc[1]], secret: r.secretOf(line, loc)}
		if r.entropy != 0 && shannonEntropy(cand.secret) <= r.entropy {
			continue
		}
		if r.allowed(cand) || (r.origin == originBase && global.allows(cand)) {
			continue
		}
		return true
	}
	return false
}

// hasKeyword applies the keyword pre-filter: a rule without keywords always
// passes, otherwise one keyword must occur in the lower-cased line.
func (r *secretRule) hasKeyword(lower string) bool {
	if len(r.keywords) == 0 {
		return true
	}
	for _, k := range r.keywords {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

// secretOf extracts the secret from one submatch location: the configured
// group when set, else the first non-empty group, else the whole match.
func (r *secretRule) secretOf(line string, loc []int) string {
	group := func(i int) string {
		if 2*i+1 >= len(loc) || loc[2*i] < 0 {
			return ""
		}
		return line[loc[2*i]:loc[2*i+1]]
	}
	if r.secretGroup > 0 {
		return group(r.secretGroup)
	}
	for i := 1; 2*i < len(loc); i++ {
		if s := group(i); s != "" {
			return s
		}
	}
	return group(0)
}

func (r *secretRule) allowed(c secretCandidate) bool {
	for _, a := range r.allowlists {
		if a.allows(c) {
			return true
		}
	}
	return false
}

// allows reports whether the allowlist discards the candidate. Under the
// default OR condition any declared criterion is enough; under AND every
// declared criterion (paths, regexes, stopwords) must hold.
func (a *allowlist) allows(c secretCandidate) bool {
	checks := make([]bool, 0, 3)
	if len(a.paths) > 0 {
		checks = append(checks, anyMatch(a.paths, c.path))
	}
	if len(a.regexes) > 0 || len(a.literals) > 0 {
		target := a.targetOf(c)
		checks = append(checks, anyMatch(a.regexes, target) || a.literals[sha256Of([]byte(target))])
	}
	if len(a.stopwords) > 0 {
		checks = append(checks, containsStopword(a.stopwords, c.secret))
	}
	if len(checks) == 0 {
		return false
	}
	for _, ok := range checks {
		if ok && !a.matchAll {
			return true
		}
		if !ok && a.matchAll {
			return false
		}
	}
	return a.matchAll
}

// allowsPath reports whether a path-only global allowlist entry excludes the
// whole file before any rule runs (lock files, vendored trees, binaries).
func (a *allowlist) allowsPath(path string) bool {
	return anyMatch(a.paths, path)
}

func (a *allowlist) targetOf(c secretCandidate) string {
	switch a.target {
	case targetMatch:
		return c.match
	case targetLine:
		return c.line
	default:
		return c.secret
	}
}

func anyMatch(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func containsStopword(stopwords []string, secret string) bool {
	lower := strings.ToLower(secret)
	for _, w := range stopwords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// shannonEntropy is the Shannon entropy, in bits per character, of s.
func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := make(map[rune]int)
	for _, ch := range s {
		counts[ch]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}
