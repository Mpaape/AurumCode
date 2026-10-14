package verify

// Issue #179: with no max_calls declared every finding is verified, and the
// excerpt shows every window of every file of the directory, past the old
// 8-call, 40-file and 24 KiB ceilings.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestEveryFindingIsVerifiedWithoutACallCeiling(t *testing.T) {
	rev, issue := scenario(t)
	var issues []types.ReviewIssue
	for n := 1; n <= 12; n++ {
		i := issue
		i.Line = n
		issues = append(issues, i)
	}
	c := &caller{replies: []string{fixture(t, "verifier-refuted.json")}}
	res := newVerifier(rev, c, 0).Verify(context.Background(), issues, blocksAll)
	if res.Calls != len(issues) {
		t.Fatalf("calls = %d, want every one of the %d findings verified", res.Calls, len(issues))
	}
	for _, r := range res.Records {
		if r.Outcome == OutcomeCallLimit {
			t.Fatalf("a finding hit a call ceiling nobody declared: %+v", r)
		}
	}
}

func TestExcerptShowsEveryFileAndWindowPastTheOldCeilings(t *testing.T) {
	filler := strings.Repeat("// "+strings.Repeat("f", 200)+"\n", 40)
	rev := revision{"d/a.go": "package d\n\nfunc use() { Target() }\n"}
	for n := 0; n < 45; n++ {
		rev[fmt.Sprintf("d/m%02d.go", n)] = "package d\n\nfunc Helper" + fmt.Sprint(n) + "() {}\n" + filler + "// Target\n"
	}
	long := "// Target Alpha Beta " + strings.Repeat("z", 400) + "\n"
	rev["d/z.go"] = "package d\n\nfunc Target() {}\nfunc Alpha() {}\nfunc Beta() {}\n" + strings.Repeat(long, 200)
	issue := types.ReviewIssue{File: "d/a.go", Line: 3, Message: "Target, Alpha e Beta sem verificação"}
	shown, files, err := excerpts(rev, goDecls, issue)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["d/z.go"]; !ok {
		t.Fatalf("the declaring file, past the old 40 siblings, was not shown: %d files", len(files))
	}
	size := 0
	for _, e := range shown {
		size += len(e.Text)
	}
	if size <= 24<<10 {
		t.Fatalf("the excerpt was cut at %d bytes, want every window past the old 24 KiB", size)
	}
}
