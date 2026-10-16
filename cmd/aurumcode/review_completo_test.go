package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// #178: --completo is HEAD's whole tree as added, with no base and no orphan
// commit written to the repository.
func TestComputeFullDiffIsTheWholeTreeAsAdded(t *testing.T) {
	dir := coverageFixture(t, "")
	before := countObjects(t, dir)
	diff, _, err := computeFullDiff(dir)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, f := range diff.Files {
		paths[f.Path] = true
		for _, h := range f.Hunks {
			if h.OldLines != 0 {
				t.Fatalf("%s: a full diff removes nothing, got hunk %+v", f.Path, h)
			}
			for _, l := range h.Lines {
				if !strings.HasPrefix(l, "+") {
					t.Fatalf("%s: every line is added, got %q", f.Path, l)
				}
			}
		}
	}
	for _, want := range []string{"app.go", "tests/change_test.go"} {
		if !paths[want] {
			t.Fatalf("full diff lacks %s: %v", want, paths)
		}
	}
	if after := countObjects(t, dir); after != before {
		t.Fatalf("the full diff wrote %d object(s) into the repository", after-before)
	}
	if r := localFullScanRange(dir); !r.Full || r.Head == "" || r.Base != "" || r.Empty() {
		t.Fatalf("full scan range: %+v", r)
	}
}

func countObjects(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	filepath.Walk(filepath.Join(dir, ".git", "objects"), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// The run itself: every file reviewed, SARIF in its own category.
func TestReviewCompletoWritesTheInitialScanCategory(t *testing.T) {
	coverageFixture(t, "")
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","issues":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	sarif := filepath.Join(t.TempDir(), "inicial.sarif")

	var out, errOut strings.Builder
	code := runReview([]string{"--completo", "--sarif", sarif}, &out, &errOut, redaction.NewFilter())
	if code != 0 && code != 3 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	raw, err := os.ReadFile(sarif)
	if err != nil {
		t.Fatalf("SARIF not written: %v (stderr %s)", err, errOut.String())
	}
	var doc struct {
		Runs []struct {
			AutomationDetails struct {
				ID string `json:"id"`
			} `json:"automationDetails"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Runs) != 1 {
		t.Fatalf("SARIF: %v %s", err, raw)
	}
	if got := doc.Runs[0].AutomationDetails.ID; got != initialScanCategory+"/" {
		t.Fatalf("category %q, want %q", got, initialScanCategory+"/")
	}
}

func TestReviewCompletoTakesNeitherBaseNorPR(t *testing.T) {
	for _, args := range [][]string{{"--completo", "--base", "HEAD~1"}, {"--full", "--pr", "7", "--repo", "a/b", "--publicar"}} {
		var out, errOut strings.Builder
		if code := runReview(args, &out, &errOut, redaction.NewFilter()); code != 2 || !strings.Contains(errOut.String(), "--completo") {
			t.Fatalf("%v: exit=%d stderr=%s", args, code, errOut.String())
		}
	}
}

func TestFullRangeIsNotEmptyWithoutBase(t *testing.T) {
	if (scanner.Range{Head: "abc", Full: true}).Empty() {
		t.Fatal("a full range has no base by definition")
	}
	if !(scanner.Range{Head: "abc"}).Empty() {
		t.Fatal("a range without base and not full is empty")
	}
}
