package analyzer

import "github.com/Mpaape/AurumCode/pkg/types"

// unifiedContext is the number of unchanged lines kept around each change,
// the window git and the forges render by default.
const unifiedContext = 3

// editAt is one edit with the 0-based old and new line positions before it.
type editAt struct {
	edit     diffEdit
	old, new int
}

// BuildWindowedDiffFile builds the diff of one file the way a unified diff
// shows it: hunks of changed lines with unifiedContext unchanged lines
// around them, nearby changes merged into one hunk. Line numbers follow git:
// a side with no lines in a hunk starts at the line before it.
func BuildWindowedDiffFile(path, oldContent, newContent string) types.DiffFile {
	edits := myersDiff(splitLines(oldContent), splitLines(newContent))
	positions := make([]editAt, 0, len(edits))
	o, n := 0, 0
	for _, e := range edits {
		positions = append(positions, editAt{edit: e, old: o, new: n})
		switch e.kind {
		case diffEqual:
			o++
			n++
		case diffDelete:
			o++
		case diffInsert:
			n++
		}
	}
	file := types.DiffFile{Path: path}
	for i := 0; i < len(positions); {
		for i < len(positions) && positions[i].edit.kind == diffEqual {
			i++
		}
		if i == len(positions) {
			break
		}
		end := hunkEnd(positions, i)
		start := i - unifiedContext
		if start < 0 {
			start = 0
		}
		file.Hunks = append(file.Hunks, windowHunk(positions[start:end]))
		i = end
	}
	return file
}

// hunkEnd returns the end (exclusive) of the hunk whose first change is at
// first: past the last change that is followed by no more than twice the
// context of unchanged lines before the next one, plus the trailing context.
func hunkEnd(positions []editAt, first int) int {
	last, equalRun := first, 0
	for j := first; j < len(positions); j++ {
		if positions[j].edit.kind != diffEqual {
			last, equalRun = j, 0
			continue
		}
		equalRun++
		if equalRun > 2*unifiedContext {
			break
		}
	}
	end := last + unifiedContext + 1
	if end > len(positions) {
		end = len(positions)
	}
	return end
}

// windowHunk renders a run of edits as one hunk.
func windowHunk(run []editAt) types.DiffHunk {
	h := types.DiffHunk{OldStart: run[0].old + 1, NewStart: run[0].new + 1}
	for _, p := range run {
		switch p.edit.kind {
		case diffEqual:
			h.OldLines++
			h.NewLines++
			h.Lines = append(h.Lines, " "+p.edit.text)
		case diffDelete:
			h.OldLines++
			h.Lines = append(h.Lines, "-"+p.edit.text)
		case diffInsert:
			h.NewLines++
			h.Lines = append(h.Lines, "+"+p.edit.text)
		}
	}
	if h.OldLines == 0 {
		h.OldStart--
	}
	if h.NewLines == 0 {
		h.NewStart--
	}
	return h
}
