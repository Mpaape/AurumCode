package render

import "testing"

// TestAUR521FindingFingerprintStableAcrossRuns covers AC-002's stability
// requirement and is the behavior MUT-001 attacks: the exact same finding
// identity must hash to the exact same fingerprint every time it is
// computed, never only "most of the time".
func TestAUR521FindingFingerprintStableAcrossRuns(t *testing.T) {
	id := FindingIdentity{
		RuleID:  "security#no-hardcoded-secrets",
		Path:    "app.go",
		Line:    3,
		Context: `dbPassword := "hunter2-super-secret"`,
	}
	first := FindingFingerprint(id)
	second := FindingFingerprint(id)
	if first == "" {
		t.Fatal("fingerprint must not be empty")
	}
	if first != second {
		t.Fatalf("fingerprint not stable across two calls with the same finding: %q != %q", first, second)
	}
}

// TestAUR521FindingFingerprintChangesWithContent covers AC-002's other half:
// a real content change (the line itself changed) must produce a different
// fingerprint, so a fingerprint can never be mistaken for a content hash
// that happens to always collide.
func TestAUR521FindingFingerprintChangesWithContent(t *testing.T) {
	base := FindingIdentity{RuleID: "r1", Path: "app.go", Line: 3, Context: "a := 1"}
	changedLine := FindingIdentity{RuleID: "r1", Path: "app.go", Line: 3, Context: "a := 2"}
	changedPath := FindingIdentity{RuleID: "r1", Path: "other.go", Line: 3, Context: "a := 1"}
	changedRule := FindingIdentity{RuleID: "r2", Path: "app.go", Line: 3, Context: "a := 1"}
	changedLineNo := FindingIdentity{RuleID: "r1", Path: "app.go", Line: 4, Context: "a := 1"}

	baseFP := FindingFingerprint(base)
	for name, other := range map[string]FindingIdentity{
		"line content": changedLine,
		"path":         changedPath,
		"rule id":      changedRule,
		"line number":  changedLineNo,
	} {
		if FindingFingerprint(other) == baseFP {
			t.Fatalf("fingerprint did not change when %s changed", name)
		}
	}
}

// TestAUR521FindingFingerprintNormalizesWhitespaceAndPath covers the
// normalization this identity deliberately applies: re-indented code and an
// equivalent path spelling must not be treated as a different finding.
func TestAUR521FindingFingerprintNormalizesWhitespaceAndPath(t *testing.T) {
	a := FindingIdentity{RuleID: "r1", Path: "./pkg/app.go", Line: 3, Context: "  a   :=  1  "}
	b := FindingIdentity{RuleID: "r1", Path: "pkg/app.go", Line: 3, Context: "a := 1"}
	if FindingFingerprint(a) != FindingFingerprint(b) {
		t.Fatal("equivalent path/whitespace spellings must fingerprint identically")
	}
}
