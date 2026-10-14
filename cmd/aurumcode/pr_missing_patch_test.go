package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// A --pr file the API diff carried without any line is reviewed with the
// hunks the checkout yields, small or 2 MiB; a pure rename leaves the diff
// as declared (ignored, never reviewed); without a checkout, or when the
// recomputation fails, the file stays line-less and is not reviewed.
func TestRecoverMissingPatches(t *testing.T) {
	wide := strings.Repeat("w", 2<<20)
	hunk := func(lines ...string) []types.DiffHunk {
		return []types.DiffHunk{{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: lines}}
	}
	recomputed := map[string]types.DiffFile{
		"small.txt": {Path: "small.txt", Hunks: hunk("-one", "+ONE")},
		"wide.txt":  {Path: "wide.txt", Hunks: hunk("-"+wide, "+"+wide+"x")},
	}
	var asked []string
	source := func(paths []string) ([]types.DiffFile, []analyzer.DiffNotice, error) {
		asked = paths
		var files []types.DiffFile
		var notices []analyzer.DiffNotice
		for _, p := range paths {
			if f, ok := recomputed[p]; ok {
				files = append(files, f)
			} else if p == "renamed.txt" {
				notices = append(notices, analyzer.DiffNotice{Path: p, Reason: analyzer.NoticeReasonNoContentChange, Message: "no content change: " + p})
			}
		}
		return files, notices, nil
	}
	diff := &types.Diff{Files: []types.DiffFile{
		{Path: "ok.go", Hunks: hunk("+x")},
		{Path: "small.txt", Lang: "text"},
		{Path: "wide.txt"},
		{Path: "renamed.txt"},
		{Path: "lost.txt"},
	}}

	out, declared := recoverMissingPatches(diff, source)
	if strings.Join(asked, ",") != "small.txt,wide.txt,renamed.txt,lost.txt" {
		t.Errorf("only the line-less files are recomputed, asked %v", asked)
	}
	var paths []string
	for _, f := range out.Files {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, ",") != "ok.go,small.txt,wide.txt,lost.txt" {
		t.Fatalf("the rename leaves the diff, the rest stays in order, got %v", paths)
	}
	if len(out.Files[1].Hunks) == 0 || out.Files[1].Lang != "text" || len(out.Files[2].Hunks) == 0 {
		t.Fatalf("small.txt and the 2 MiB wide.txt must come back with hunks: %+v", out.Files[1:3])
	}
	notices := uninspectedPRNotices(out, "")
	if len(notices) != 1 || notices[0].Path != "lost.txt" || notices[0].Reason != analyzer.NoticeReasonNoPatch {
		t.Fatalf("only the unrecovered file stays unreviewed, got %+v", notices)
	}
	c := mergeReviewCoverage(nil, append(declared, notices...), len(diff.Files), nil)
	if strings.Join(c.IgnoredPaths, ",") != "renamed.txt (no content change)" || strings.Join(c.FilteredPaths, ",") != "lost.txt (no patch)" {
		t.Errorf("ignored %v filtered %v", c.IgnoredPaths, c.FilteredPaths)
	}

	// No checkout (no source) or a failed recomputation: nothing changes and
	// every line-less file is declared not reviewed.
	for name, src := range map[string]patchSource{
		"no checkout": nil,
		"failure":     func([]string) ([]types.DiffFile, []analyzer.DiffNotice, error) { return nil, nil, errors.New("boom") },
	} {
		same, none := recoverMissingPatches(diff, src)
		if same != diff || none != nil {
			t.Errorf("%s: the diff must be left as it was", name)
		}
		if n := uninspectedPRNotices(same, ""); len(n) != 4 {
			t.Errorf("%s: all four line-less files stay unreviewed, got %+v", name, n)
		}
	}
}
