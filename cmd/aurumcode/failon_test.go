package main

import (
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestParseFailOnLevel pins the --fail-on vocabulary: the engine's own
// severity names (error, warning, info), the CI-conventional aliases
// (high, medium, low), case-insensitivity, and rejection of anything else.
func TestParseFailOnLevel(t *testing.T) {
	cases := []struct {
		level    string
		wantRank int
		wantName string
	}{
		{"high", rankError, "error"},
		{"error", rankError, "error"},
		{"HIGH", rankError, "error"},
		{"Error", rankError, "error"},
		{"medium", rankWarning, "warning"},
		{"warning", rankWarning, "warning"},
		{"low", rankInfo, "info"},
		{"info", rankInfo, "info"},
	}
	for _, tc := range cases {
		rank, name, err := parseFailOnLevel(tc.level)
		if err != nil {
			t.Errorf("parseFailOnLevel(%q): unexpected error %v", tc.level, err)
			continue
		}
		if rank != tc.wantRank || name != tc.wantName {
			t.Errorf("parseFailOnLevel(%q) = (%d, %q), want (%d, %q)",
				tc.level, rank, name, tc.wantRank, tc.wantName)
		}
	}

	for _, bad := range []string{"", "critical", "none", "3", "err or"} {
		if _, _, err := parseFailOnLevel(bad); err == nil {
			t.Errorf("parseFailOnLevel(%q): expected an error, got none", bad)
		}
	}
}

// TestSeverityRank pins the ordering the gate's >= comparison relies on,
// including the fail-closed default for a severity the parser should never
// let through.
func TestSeverityRank(t *testing.T) {
	if !(severityRank("info") < severityRank("warning") && severityRank("warning") < severityRank("error")) {
		t.Fatalf("severity ranks are not ordered info < warning < error: info=%d warning=%d error=%d",
			severityRank("info"), severityRank("warning"), severityRank("error"))
	}
	if severityRank("ERROR") != rankError || severityRank("Warning") != rankWarning || severityRank("INFO") != rankInfo {
		t.Fatal("severityRank must be case-insensitive")
	}
	if severityRank("unheard-of") != rankError {
		t.Fatalf("an unknown severity must rank as error (fail closed), got %d", severityRank("unheard-of"))
	}
}

// TestCountAtOrAbove pins the "at the chosen severity or above" semantics
// the card promises, on the exact issue type the engine emits.
func TestCountAtOrAbove(t *testing.T) {
	issues := []types.ReviewIssue{
		{File: "a.go", Line: 1, Severity: "info", Message: "note"},
		{File: "b.go", Line: 2, Severity: "warning", Message: "smell"},
		{File: "c.go", Line: 3, Severity: "error", Message: "bug"},
		{File: "d.go", Line: 4, Severity: "error", Message: "bug too"},
	}
	cases := []struct {
		threshold int
		want      int
	}{
		{rankError, 2},   // only the two errors
		{rankWarning, 3}, // warning + both errors
		{rankInfo, 4},    // everything
	}
	for _, tc := range cases {
		if got := countAtOrAbove(issues, tc.threshold); got != tc.want {
			t.Errorf("countAtOrAbove(threshold=%d) = %d, want %d", tc.threshold, got, tc.want)
		}
	}
	if got := countAtOrAbove(nil, rankInfo); got != 0 {
		t.Errorf("countAtOrAbove(no issues) = %d, want 0", got)
	}
}
