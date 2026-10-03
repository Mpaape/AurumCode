package apply

import (
	"fmt"
	"strings"
)

// hunkLine is one rendered line of a hunk body.
type hunkLine struct {
	kind byte // ' ', '-', '+', or '\\' for the no-newline marker
	text string
}

// renderFile turns one file's sorted changes into hunks. Changes whose
// contexts touch or overlap are merged into one hunk, as `diff -u` does.
func renderFile(g *fileGroup) ([]FileEdit, error) {
	var edits []FileEdit
	delta := 0
	for i := 0; i < len(g.changes); {
		j := i + 1
		for ; j < len(g.changes); j++ {
			prev := g.changes[j-1]
			if g.changes[j].start-(prev.start+prev.oldCount()) > 2*contextLines {
				break
			}
		}
		fe, grew, err := renderGroup(g, g.changes[i:j], delta)
		if err != nil {
			return nil, err
		}
		edits = append(edits, fe)
		delta += grew
		i = j
	}
	return edits, nil
}

// renderGroup renders the merged changes as one hunk with real context. delta
// is the net number of lines earlier hunks of the file added; the second
// return value is how many lines this hunk adds.
func renderGroup(g *fileGroup, cs []change, delta int) (FileEdit, int, error) {
	lines := g.text.lines
	first := cs[0].start
	lo := first - contextLines
	if lo < 1 {
		lo = 1
	}
	fe := FileEdit{Path: g.path}
	var body []hunkLine
	for n := lo; n < first; n++ {
		body = append(body, hunkLine{' ', lines[n-1]})
	}
	next := first // next old line not yet emitted
	grew := 0
	for _, c := range cs {
		if c.start < next {
			return fe, 0, fmt.Errorf("%s:%d: suggestions overlap", g.path, c.start)
		}
		if c.start-1 > len(lines) {
			return fe, 0, fmt.Errorf("%s:%d: line does not exist in the working tree (file has %d lines)", g.path, c.start, len(lines))
		}
		for ; next < c.start; next++ {
			body = append(body, hunkLine{' ', lines[next-1]})
		}
		for _, op := range c.ops {
			switch op.kind {
			case ' ':
				body = append(body, hunkLine{' ', lines[next-1]})
				next++
			case '-':
				fe.Removals = append(fe.Removals, Line{Number: next, Text: op.text})
				body = append(body, hunkLine{'-', lines[next-1]})
				next++
				grew--
			case '+':
				fe.Additions = append(fe.Additions, Line{Number: next + grew + delta, Text: op.text})
				body = append(body, hunkLine{'+', op.text})
				grew++
			}
		}
	}
	for n := 0; n < contextLines && next <= len(lines); n++ {
		body = append(body, hunkLine{' ', lines[next-1]})
		next++
	}
	body, err := markNoNewline(g, body, next-1 == len(lines))
	if err != nil {
		return fe, 0, err
	}
	fe.Hunk = hunkHeader(body, lo, lo+delta) + formatBody(body)
	return fe, grew, nil
}

// markNoNewline adds git's "\ No newline at end of file" marker when the hunk
// reaches a final line that has no trailing newline.
func markNoNewline(g *fileGroup, body []hunkLine, reachesEOF bool) ([]hunkLine, error) {
	if g.text.lastEndNL || !reachesEOF {
		return body, nil
	}
	lastOld := -1
	for i, l := range body {
		if l.kind != '+' {
			lastOld = i
		}
	}
	if lastOld < 0 {
		return body, nil
	}
	if lastOld < len(body)-1 && body[lastOld].kind == ' ' {
		return nil, fmt.Errorf("%s: cannot append after a last line without a trailing newline", g.path)
	}
	out := append([]hunkLine{}, body[:lastOld+1]...)
	out = append(out, hunkLine{'\\', " No newline at end of file"})
	out = append(out, body[lastOld+1:]...)
	if lastOld < len(body)-1 {
		out = append(out, hunkLine{'\\', " No newline at end of file"})
	}
	return out, nil
}

// hunkHeader builds "@@ -l,n +l,n @@" from the body's old and new line counts.
func hunkHeader(body []hunkLine, oldStart, newStart int) string {
	oldN, newN := 0, 0
	for _, l := range body {
		switch l.kind {
		case ' ':
			oldN++
			newN++
		case '-':
			oldN++
		case '+':
			newN++
		}
	}
	if oldN == 0 {
		oldStart--
	}
	if newN == 0 {
		newStart--
	}
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldN, newStart, newN)
}

func formatBody(body []hunkLine) string {
	var sb strings.Builder
	for _, l := range body {
		sb.WriteByte(l.kind)
		sb.WriteString(l.text)
		sb.WriteByte('\n')
	}
	return sb.String()
}
