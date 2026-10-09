package rounds

import (
	"strings"
	"testing"
)

const fpA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const fpB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestMarkerRoundTrip(t *testing.T) {
	marker := Marker(fpA, "quality/poor-naming")
	got := Published([]Comment{{Body: "texto\n\n" + marker, Path: "a.go", Line: 3}})
	if len(got) != 1 || got[0].Fingerprint != fpA || got[0].RuleID != "quality/poor-naming" || got[0].Path != "a.go" || got[0].Line != 3 {
		t.Fatalf("round trip = %+v", got)
	}
	for _, bad := range [][2]string{{"short", "r"}, {fpA, ""}, {fpA, "a b"}, {fpA, "x-->"}} {
		if m := Marker(bad[0], bad[1]); m != "" {
			t.Fatalf("Marker(%q, %q) = %q, want none", bad[0], bad[1], m)
		}
	}
}

func TestPublishedIgnoresReplies(t *testing.T) {
	got := Published([]Comment{{Body: "> " + Marker(fpA, "r"), Reply: true}})
	if len(got) != 0 {
		t.Fatalf("a quoted marker in a reply counted: %+v", got)
	}
}

func TestPlanRoundCountsRepeatsAndResolved(t *testing.T) {
	previous := []Previous{{Fingerprint: fpA, RuleID: "r1"}, {Fingerprint: fpB, RuleID: "r2"}}
	plan := PlanRound([]string{fpA, fpA, ""}, nil, previous)
	if plan.Post[0] || !plan.Post[1] || !plan.Post[2] || plan.Repeated != 1 {
		t.Fatalf("plan = %+v: one earlier comment covers one occurrence only", plan)
	}
	if len(plan.Resolved) != 1 || plan.Resolved[0].Fingerprint != fpB {
		t.Fatalf("resolved = %+v", plan.Resolved)
	}
	if !strings.HasPrefix(Marker(fpB, "r2"), "<!-- aurumcode:finding ") {
		t.Fatal("marker shape changed")
	}
}

// TestPlanRoundCondensedIsNeverResolved: a finding still reported, only
// without its own comment, is not listed as resolved.
func TestPlanRoundCondensedIsNeverResolved(t *testing.T) {
	plan := PlanRound(nil, []string{fpB}, []Previous{{Fingerprint: fpB, RuleID: "r2"}})
	if len(plan.Resolved) != 0 {
		t.Fatalf("a condensed finding was called resolved: %+v", plan.Resolved)
	}
}

// A comment told resolved keeps its marker but is not read again, so the
// resolution is named once; Resolved builds that body.
func TestResolvedCommentsAreNotReadAgain(t *testing.T) {
	body := Resolved("**Resolvido:** nao reencontrado.", "**[warning] x**\n\n"+Marker(fpA, "r1"))
	if !strings.HasPrefix(body, "**Resolvido:**") || !strings.HasSuffix(body, ResolvedMarker) || !strings.Contains(body, Marker(fpA, "r1")) {
		t.Fatalf("resolved body = %q", body)
	}
	got := Published([]Comment{{ID: 5, Kind: "inline", Body: body}, {ID: 6, Kind: "comment", Body: Marker(fpB, "r2"), Path: "a.go", Line: 3}})
	if len(got) != 1 || got[0].Fingerprint != fpB || got[0].CommentID != 6 || got[0].CommentKind != "comment" {
		t.Fatalf("published = %+v: the resolved comment must be skipped and the other carry its id", got)
	}
}
