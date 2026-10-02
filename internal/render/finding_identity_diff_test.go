package render

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR521DiffLineAtSkipsNoNewlineMarker is B4: a hunk line that is not
// a real +/-/context line at all -- git's own "\ No newline at end of
// file" marker -- must never advance either side's line counter. Before
// the fix, diffLineAt's `default:` branch treated it exactly like a ' '
// context line (advancing both counters), which silently shifted every
// line AFTER the marker by one and made this function return the WRONG
// line's content (or nothing at all) for a finding anchored past it.
func TestAUR521DiffLineAtSkipsNoNewlineMarker(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: "app.go",
		Hunks: []types.DiffHunk{{
			OldStart: 1, NewStart: 1,
			Lines: []string{
				"+foo",
				`\ No newline at end of file`,
				"+bar",
			},
		}},
	}}}

	if got := diffLineAt(diff, "app.go", 1, "RIGHT"); got != "foo" {
		t.Fatalf("line 1 = %q, want %q", got, "foo")
	}
	// This is the line the bug corrupts: without the fix, the marker line
	// shifts "bar" to appear as line 3 instead of line 2.
	if got := diffLineAt(diff, "app.go", 2, "RIGHT"); got != "bar" {
		t.Fatalf("line 2 = %q, want %q (the no-newline marker must not shift line numbers)", got, "bar")
	}
	if got := diffLineAt(diff, "app.go", 3, "RIGHT"); got != "" {
		t.Fatalf("line 3 = %q, want \"\" (there is no third added line)", got)
	}
}

// TestAUR521FindingIdentityForUsesDiffLine covers the decisive check from
// the earlier review round at the unit level: FindingIdentityFor's Context
// comes from the diff, and is empty (never some other text) when the diff
// does not contain the requested line at all -- this is M1's target
// mutation (FindingIdentityFor ignoring the diff and returning an empty
// FindingIdentity unconditionally): an identity that never varies with its
// own inputs is not an identity at all, and two genuinely different
// findings would collide onto the same fingerprint.
func TestAUR521FindingIdentityForUsesDiffLine(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: "app.go",
		Hunks: []types.DiffHunk{{
			OldStart: 1, NewStart: 1,
			Lines: []string{"+line one", "+line two"},
		}},
	}}}
	id1 := FindingIdentityFor(diff, types.ReviewIssue{RuleID: "r", File: "app.go", Line: 1}, nil)
	id2 := FindingIdentityFor(diff, types.ReviewIssue{RuleID: "r", File: "app.go", Line: 2}, nil)
	if id1.Context != "line one" {
		t.Fatalf("line 1 context = %q, want %q", id1.Context, "line one")
	}
	if id2.Context != "line two" {
		t.Fatalf("line 2 context = %q, want %q", id2.Context, "line two")
	}
	if id1.Context == id2.Context {
		t.Fatal("two different diff lines must not produce the same context")
	}
	notFound := FindingIdentityFor(diff, types.ReviewIssue{RuleID: "r", File: "other.go", Line: 1}, nil)
	if notFound.Context != "" {
		t.Fatalf("a file not present in the diff must yield empty context, got %q", notFound.Context)
	}
}

// TestAUR521FindingIdentityForRedactsBeforeHashing is M2: the Context
// FindingIdentityFor returns must already be redacted -- hashing the RAW
// diff line (skipping or reordering the filter.Redact call) would make the
// SAME finding's fingerprint change whenever the secret it flags is
// rotated between two runs, which defeats AC-002's own stability
// guarantee for exactly the scenario it matters most (a tracked secret
// literally present in the reviewed code). Two runs below review the SAME
// rule/path/line but a DIFFERENT, freshly "rotated" secret value in the
// diff; each run's own filter only knows ITS OWN current secret (exactly
// like AURUM_SECRET_CANARY changing between CI runs) -- so the two
// contexts are redacted to the identical marker and must fingerprint
// identically.
func TestAUR521FindingIdentityForRedactsBeforeHashing(t *testing.T) {
	buildDiff := func(secret string) *types.Diff {
		return &types.Diff{Files: []types.DiffFile{{
			Path: "app.go",
			Hunks: []types.DiffHunk{{
				OldStart: 1, NewStart: 1,
				Lines: []string{`+token := "` + secret + `"`},
			}},
		}}}
	}
	issue := types.ReviewIssue{RuleID: "security/hardcoded-secret", File: "app.go", Line: 1}

	run1Filter := redaction.NewFilter("SECRET-ROTATION-OLD")
	run1 := FindingIdentityFor(buildDiff("SECRET-ROTATION-OLD"), issue, run1Filter)

	run2Filter := redaction.NewFilter("SECRET-ROTATION-NEW")
	run2 := FindingIdentityFor(buildDiff("SECRET-ROTATION-NEW"), issue, run2Filter)

	if run1.Context == `token := "SECRET-ROTATION-OLD"` {
		t.Fatalf("Context was never redacted: %q", run1.Context)
	}
	if run1.Context != run2.Context {
		t.Fatalf("redacted context differs across a rotated secret: %q vs %q", run1.Context, run2.Context)
	}
	fp1 := FindingFingerprint(run1)
	fp2 := FindingFingerprint(run2)
	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("fingerprint changed when only a tracked secret's VALUE rotated: %q vs %q", fp1, fp2)
	}
}
