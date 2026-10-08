package verify

import "strings"

// normalize removes what a copy may legitimately change: carriage returns
// and the spaces at the end of each line. Nothing else is forgiven.
func normalize(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// quoteFound reports whether quote, which must have some non-space text,
// occurs literally in one of files.
func quoteFound(quote string, files map[string]string) bool {
	q := strings.Trim(normalize(quote), "\n")
	if strings.TrimSpace(q) == "" {
		return false
	}
	for _, content := range files {
		if strings.Contains(normalize(content), q) {
			return true
		}
	}
	return false
}
