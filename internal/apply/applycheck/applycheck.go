// Package applycheck applies a unified diff to in-memory files with the
// strictness of plain `git apply` (no --unidiff-zero): hunk counts must match
// their "@@" header, context and removed lines must equal the file, and a hunk
// with no leading (or trailing) context is only accepted at the very start
// (or end) of the file. It exists so the acceptance proof for `aurumcode fix`
// also runs in sealed environments that have no git binary.
package applycheck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeader = regexp.MustCompile(`^@@ -(\d+),(\d+) \+(\d+),(\d+) @@$`)

// Apply applies patch to files (path to content) and returns the new tree.
// A file whose "+++" side is /dev/null is removed from the result.
func Apply(files map[string]string, patch string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range files {
		out[k] = v
	}
	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "--- ") || i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
			return nil, fmt.Errorf("line %d: expected a --- / +++ file header, got %q", i+1, lines[i])
		}
		from, to := strings.TrimPrefix(lines[i], "--- "), strings.TrimPrefix(lines[i+1], "+++ ")
		j := i + 2
		for j < len(lines) && !strings.HasPrefix(lines[j], "--- ") {
			j++
		}
		name, content, err := applyFile(out, from, to, lines[i+2:j])
		if err != nil {
			return nil, err
		}
		if to == "/dev/null" {
			delete(out, name)
		} else {
			out[name] = content
		}
		i = j
	}
	return out, nil
}

func applyFile(files map[string]string, from, to string, body []string) (string, string, error) {
	var name, orig string
	switch {
	case from == "/dev/null":
		name = strings.TrimPrefix(to, "b/")
		if _, ok := files[name]; ok {
			return "", "", fmt.Errorf("%s: already exists", name)
		}
	case strings.HasPrefix(from, "a/"):
		name = strings.TrimPrefix(from, "a/")
		var ok bool
		if orig, ok = files[name]; !ok {
			return "", "", fmt.Errorf("%s: no such file", name)
		}
		if to != "/dev/null" && to != "b/"+name {
			return "", "", fmt.Errorf("%s: header names %s on the +++ side", name, to)
		}
	default:
		return "", "", fmt.Errorf("bad header path %q", from)
	}
	src := splitFile(orig)
	var res []string
	pos := 0 // lines of src already consumed
	for i := 0; i < len(body); {
		m := hunkHeader.FindStringSubmatch(body[i])
		if m == nil {
			return "", "", fmt.Errorf("%s: expected a hunk header, got %q", name, body[i])
		}
		oldStart, oldN, newN := atoi(m[1]), atoi(m[2]), atoi(m[4])
		j := i + 1
		for j < len(body) && !strings.HasPrefix(body[j], "@@ ") {
			j++
		}
		var err error
		res, pos, err = applyHunk(name, src, res, pos, oldStart, oldN, newN, body[i+1:j])
		if err != nil {
			return "", "", err
		}
		i = j
	}
	res = append(res, src[pos:]...)
	if len(res) == 0 {
		return name, "", nil
	}
	return name, strings.Join(res, "\n") + "\n", nil
}

func applyHunk(name string, src, res []string, pos, oldStart, oldN, newN int, h []string) ([]string, int, error) {
	if oldN == 0 {
		oldStart++ // header of a hunk with no old lines names the line before
	}
	if oldStart-1 < pos {
		return nil, 0, fmt.Errorf("%s: hunk at %d overlaps the previous one", name, oldStart)
	}
	res = append(res, src[pos:oldStart-1]...)
	cur := oldStart - 1
	gotOld, gotNew, lead, trail, changed := 0, 0, 0, 0, false
	for _, l := range h {
		if l == "" || l[0] == '\\' {
			continue
		}
		text := l[1:]
		switch l[0] {
		case ' ', '-':
			if cur >= len(src) || src[cur] != text {
				return nil, 0, fmt.Errorf("%s:%d: patch does not apply (file differs from %q)", name, cur+1, l)
			}
			cur++
			gotOld++
			if l[0] == ' ' {
				gotNew++
				res = append(res, text)
				if !changed {
					lead++
				}
				trail++
			} else {
				changed, trail = true, 0
			}
		case '+':
			res = append(res, text)
			gotNew++
			changed, trail = true, 0
		default:
			return nil, 0, fmt.Errorf("%s: bad hunk line %q", name, l)
		}
	}
	if gotOld != oldN || gotNew != newN {
		return nil, 0, fmt.Errorf("%s: hunk counts -%d +%d do not match the header -%d +%d", name, gotOld, gotNew, oldN, newN)
	}
	if lead == 0 && oldStart > 1 && oldN > 0 {
		return nil, 0, fmt.Errorf("%s:%d: hunk without leading context (needs --unidiff-zero)", name, oldStart)
	}
	if trail == 0 && cur < len(src) && oldN > 0 {
		return nil, 0, fmt.Errorf("%s:%d: hunk without trailing context (needs --unidiff-zero)", name, cur)
	}
	return res, cur, nil
}

func splitFile(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
