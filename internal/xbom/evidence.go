package xbom

import (
	"path/filepath"
	"strings"
)

// lineCitesToken is AUR-552's evidence decision (AC-002): a cited line must
// contain the component's evidence token. It is one return statement on
// purpose so the skeptical mutation MUT-001 can replace it and prove the
// tests catch a BOM that accepts components without evidence.
func lineCitesToken(line, token string, model bool) bool {
	return token != "" && strings.Contains(line, token) && (!model || containsAtBoundary(line, token)) // AUR-552 AC-002: evidence check
}

// safeRepoPath resolves a repo-relative location inside root, refusing
// absolute paths, "..", and symlinks that leave the repository.
func safeRepoPath(root, loc string) (string, bool) {
	if loc == "" || filepath.IsAbs(loc) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(loc))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rootR, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", false
	}
	full, err := filepath.EvalSymlinks(filepath.Join(rootR, clean))
	if err != nil {
		return "", false
	}
	if full != rootR && !strings.HasPrefix(full, rootR+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

// VerifyResult counts what verification dropped.
type VerifyResult struct {
	DroppedComponents  int
	DroppedOccurrences int
}

// verifyEvidence reopens every cited occurrence and keeps only those whose
// line contains the token; a component left with no occurrence is dropped.
func verifyEvidence(root string, comps []*Component) ([]*Component, VerifyResult) {
	var res VerifyResult
	cache := map[string][]string{}
	var kept []*Component
	for _, c := range comps {
		var occ []Occurrence
		for _, o := range c.Occurrences {
			lines, ok := cache[o.Location]
			if !ok {
				if p, good := safeRepoPath(root, o.Location); good {
					lines, _ = readLines(p)
				}
				cache[o.Location] = lines
			}
			if o.Line >= 1 && o.Line <= len(lines) && lineCitesToken(lines[o.Line-1], o.Token, o.Model) {
				occ = append(occ, o)
			} else {
				res.DroppedOccurrences++
			}
		}
		if len(occ) == 0 {
			res.DroppedComponents++
			continue
		}
		c.Occurrences = occ
		kept = append(kept, c)
	}
	return kept, res
}

func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// containsAtBoundary reports whether name occurs in line with no word
// character directly before or after it (so "AS" is not found in "ASSERT").
func containsAtBoundary(line, name string) bool {
	if name == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(line[from:], name)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(name)
		beforeOK := i == 0 || !isWordByte(line[i-1]) || !isWordByte(name[0])
		afterOK := end == len(line) || !isWordByte(line[end]) || !isWordByte(name[len(name)-1])
		if beforeOK && afterOK {
			return true
		}
		from = i + 1
	}
}
