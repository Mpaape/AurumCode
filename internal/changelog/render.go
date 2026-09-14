package changelog

import "strings"

// Bounds. Every rendered entry is capped so hostile or accidental input cannot
// grow the output without limit.
const (
	// MaxCommits is the number of commits considered by ClassifyCommits,
	// NextVersion and Render. Extra commits are ignored.
	MaxCommits = 256
	// MaxSubjectLen is the maximum number of runes taken from a subject or
	// hash before escaping.
	MaxSubjectLen = 500
	// MaxOutputLen is the hard byte ceiling of a rendered entry.
	MaxOutputLen = 1 << 16
)

// truncationMarker is appended once when input or output was clipped. Its
// length is reserved so the final result never exceeds MaxOutputLen.
const truncationMarker = "\n<!-- changelog entry truncated: output bounded -->\n"

// escapeText neutralizes untrusted commit text for Markdown output. HTML
// significant characters become entities and Markdown significant characters
// are backslash-escaped, so a subject can never inject markup or a script tag.
func escapeText(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"`", "\\`",
		"*", "\\*",
		"_", "\\_",
		"[", "\\[",
		"]", "\\]",
	)
	return replacer.Replace(s)
}

// clip bounds s to at most max runes, appending "..." when it truncates.
func clip(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

// bullet renders one commit as an escaped, bounded list item body.
func bullet(cl Classification) string {
	text := escapeText(clip(cl.Subject, MaxSubjectLen))
	if cl.Hash != "" {
		text += " (" + escapeText(clip(cl.Hash, 64)) + ")"
	}
	return text
}

// Render produces the deterministic Markdown entry for version at the top of
// the reviewed range. The section set and order are fixed: Added, Changed,
// Fixed, Removed. Conventional feats land in Added, perf and refactor plus
// every residual (non-conventional or unrecognized-type) subject land in
// Changed, and fixes land in Fixed. Removed is reserved and currently has no
// mapped type; its heading is still emitted so the structure never moves.
// Commits that do not affect version or sections (docs, chore, test, build,
// ci, style) are omitted from the body.
//
// Inside a section, bullets keep input order, so the same []Commit always
// yields byte-identical output.
func Render(version Version, commits []Commit) string {
	classes, truncated := classifyBounded(commits)

	var added, changed, fixed, removed []string
	for _, cl := range classes {
		switch {
		case !cl.Conventional:
			changed = append(changed, bullet(cl))
		case cl.Kind == KindFeat:
			added = append(added, bullet(cl))
		case cl.Kind == KindPerf || cl.Kind == KindRefactor:
			changed = append(changed, bullet(cl))
		case cl.Kind == KindFix:
			fixed = append(fixed, bullet(cl))
		}
	}

	var b strings.Builder
	maxContent := MaxOutputLen - len(truncationMarker)
	if maxContent < 0 {
		maxContent = 0
	}
	write := func(s string) bool {
		if b.Len()+len(s) > maxContent {
			truncated = true
			return false
		}
		b.WriteString(s)
		return true
	}
	writeSection := func(title string, items []string) {
		if !write("\n### " + title + "\n") {
			return
		}
		for _, item := range items {
			if !write("\n- " + item + "\n") {
				return
			}
		}
	}

	write("## " + version.String() + "\n")
	writeSection("Added", added)
	writeSection("Changed", changed)
	writeSection("Fixed", fixed)
	writeSection("Removed", removed)

	if truncated {
		b.WriteString(truncationMarker)
	}
	return b.String()
}
