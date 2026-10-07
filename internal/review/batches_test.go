package review

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// files makes one diff file per path.
func files(paths ...string) []types.DiffFile {
	out := make([]types.DiffFile, 0, len(paths))
	for _, p := range paths {
		out = append(out, types.DiffFile{Path: p})
	}
	return out
}

// A directory that fits stays whole; one that does not fills the current
// batch file by file; a file too large alone is a batch of its own.
func TestPackUnitsKeepsDirectoriesAndFillsBatches(t *testing.T) {
	// Every file weighs 1 except "big", which never fits; a batch holds 3.
	measure := func(fs []types.DiffFile) (bool, error) {
		for _, f := range fs {
			if strings.HasSuffix(f.Path, "big") {
				return false, nil
			}
		}
		return len(fs) <= 3, nil
	}
	units := groupByDirectory(files("a/1", "b/1", "b/2", "b/3", "c/1", "d/big", "e/1"))
	got, err := packUnits(units, measure)
	if err != nil {
		t.Fatal(err)
	}
	var paths [][]string
	for _, b := range got {
		paths = append(paths, filePaths(b))
	}
	want := [][]string{{"a/1", "b/1", "b/2"}, {"b/3", "c/1"}, {"d/big"}, {"e/1"}}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("batches = %v, want %v", paths, want)
	}
}

// Counts add up, warnings are joined, a set flag stays set, and the files
// a ceiling left out are counted and named.
func TestMergeResultsConsolidatesBatches(t *testing.T) {
	a := &types.ReviewResult{Summary: "um", Issues: []types.ReviewIssue{{File: "a.go"}}, Metadata: map[string]string{"code_files_total": "2", "code_files_complete": "2", "discard_warning": "x", "quality_degraded": "false"}}
	b := &types.ReviewResult{Summary: "dois", Issues: []types.ReviewIssue{{File: "b.go"}}, Metadata: map[string]string{"code_files_total": "1", "code_files_complete": "1", "discard_warning": "y", "quality_degraded": "true"}}
	merged := mergeResults([]*types.ReviewResult{a, b})
	declareOutside(merged, []string{"c.go"})
	m := merged.Metadata
	if m["code_files_total"] != "4" || m["code_files_complete"] != "3" || m["code_files_omitted"] != "1" || m[MetaOmittedPaths] != "c.go" {
		t.Fatalf("counts not consolidated: %v", m)
	}
	if m["discard_warning"] != "x; y" || m["quality_degraded"] != "true" || len(merged.Issues) != 2 || merged.Summary != "um\n\ndois" {
		t.Fatalf("result not consolidated: %+v", merged)
	}
}

// A file without a patch never decides the split and never takes a batch of
// its own: it rides in the first batch.
func TestFilesWithoutPatchRideInTheFirstBatch(t *testing.T) {
	withHunk := func(p string) types.DiffFile {
		return types.DiffFile{Path: p, Hunks: []types.DiffHunk{{Lines: []string{"+x"}}}}
	}
	all := []types.DiffFile{withHunk("a/1"), {Path: "b/logo.png"}, withHunk("c/1")}
	withPatch, withoutPatch := splitByPatch(all)
	if len(withPatch) != 2 || len(withoutPatch) != 1 || withoutPatch[0].Path != "b/logo.png" {
		t.Fatalf("split = %v / %v", filePaths(withPatch), filePaths(withoutPatch))
	}
}
