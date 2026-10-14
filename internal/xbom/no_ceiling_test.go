package xbom

import (
	"fmt"
	"strings"
	"testing"
)

// Issue #179: a Dockerfile past the old 1 MiB file bound, with a reference
// on a line past the old 4000-byte line bound, still yields its candidate.
func TestLargeFileAndLongLineStillYieldCandidates(t *testing.T) {
	root := t.TempDir()
	filler := strings.Repeat("# "+strings.Repeat("f", 1000)+"\n", 1100)
	image := "registry.example/" + strings.Repeat("l", 5000)
	write(t, root, "Dockerfile", filler+"FROM "+image+":1.22 AS build\n")
	cands, err := Extract(root, load(t, "build", root, ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if strings.Contains(c.Line, image+":1.22") {
			return
		}
	}
	t.Fatalf("no candidate from a %d-byte Dockerfile: %d candidates", len(filler), len(cands))
}

// Past the old 300 candidates and 300 bytes per line, every candidate
// reaches the model's prompt with its whole line.
func TestPromptCarriesEveryCandidateWhole(t *testing.T) {
	long := strings.Repeat("c", 400)
	var cands []*Candidate
	for n := 0; n < 310; n++ {
		cands = append(cands, &Candidate{ID: fmt.Sprintf("c%03d", n), Entry: "e", Line: long,
			Component: &Component{Type: "library", Name: "n", Occurrences: []Occurrence{{Location: "f", Line: n + 1, Token: "n"}}}})
	}
	prompt, err := buildPrompt("p", cands, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"c309"`) || strings.Count(prompt, long) != 310 {
		t.Fatalf("prompt carries %d whole lines, last candidate=%v", strings.Count(prompt, long), strings.Contains(prompt, `"c309"`))
	}
}
