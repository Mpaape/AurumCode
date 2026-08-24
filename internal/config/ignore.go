package config

import (
	"regexp"
	"strings"
	"sync"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// FilterIgnoredPaths drops files matching any of cfg.Ignore's glob
// patterns from diff, before either review pass (the LLM quality pass or
// the deterministic --seguranca pass) ever sees them. Zero-config (cfg
// nil, or cfg.Ignore empty) returns diff completely UNCHANGED -- the same
// *types.Diff pointer, not a copy -- so a caller comparing before/after by
// identity, not merely by value, still sees no difference at all.
func FilterIgnoredPaths(diff *types.Diff, cfg *Config) *types.Diff {
	if diff == nil || cfg == nil || len(cfg.Ignore) == 0 {
		return diff
	}
	out := &types.Diff{Files: make([]types.DiffFile, 0, len(diff.Files))}
	for _, f := range diff.Files {
		if matchesAny(cfg.Ignore, f.Path) {
			continue
		}
		out.Files = append(out.Files, f)
	}
	return out
}

func matchesAny(patterns []string, path string) bool {
	for _, pat := range patterns {
		if globMatch(pat, path) {
			return true
		}
	}
	return false
}

var globCacheMu sync.Mutex
var globCache = map[string]*regexp.Regexp{}

// globMatch reports whether path matches pattern, a Copilot/gitignore-
// style glob: "*" matches within one path segment, "**" matches any
// number of segments (including zero), "?" matches one character. This is
// the same matcher applyTo (provider_files.go) uses for path-scoped
// instructions, so an ignore pattern and an applyTo pattern are written
// identically.
func globMatch(pattern, path string) bool {
	globCacheMu.Lock()
	re, ok := globCache[pattern]
	if !ok {
		re = globToRegexp(pattern)
		globCache[pattern] = re
	}
	globCacheMu.Unlock()
	return re.MatchString(path)
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
