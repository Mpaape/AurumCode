package xbom

import (
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
