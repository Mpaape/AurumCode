package apply

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestBuildPlanSafeSuggestions(t *testing.T) {
	tests := []struct {
		name string
		s    types.ReviewSuggestion
		want FileEdit
	}{
		{
			name: "single line replacement",
			s: types.ReviewSuggestion{
				File: "internal/app.go", StartLine: 10,
				CurrentCode:  "return err\n",
				ProposedCode: "return fmt.Errorf(\"wrap: %w\", err)\n",
			},
			want: FileEdit{
				Path:      "internal/app.go",
				Removals:  []Line{{Number: 10, Text: "return err"}},
				Additions: []Line{{Number: 10, Text: "return fmt.Errorf(\"wrap: %w\", err)"}},
				Hunk: "@@ -10,1 +10,1 @@\n" +
					"-return err\n" +
					"+return fmt.Errorf(\"wrap: %w\", err)\n",
			},
		},
		{
			name: "multiline replacement keeps context",
			s: types.ReviewSuggestion{
				File: "internal/app.go", StartLine: 10, EndLine: 12,
				CurrentCode:  "a\nb\nc\n",
				ProposedCode: "a\nB\nc\n",
			},
			want: FileEdit{
				Path:      "internal/app.go",
				Removals:  []Line{{Number: 11, Text: "b"}},
				Additions: []Line{{Number: 11, Text: "B"}},
				Hunk: "@@ -10,3 +10,3 @@\n" +
					" a\n-b\n+B\n c\n",
			},
		},
		{
			name: "pure deletion",
			s: types.ReviewSuggestion{
				File: "internal/app.go", StartLine: 10,
				CurrentCode:  "dead\ncode\n",
				ProposedCode: "",
			},
			want: FileEdit{
				Path:     "internal/app.go",
				Removals: []Line{{Number: 10, Text: "dead"}, {Number: 11, Text: "code"}},
				Hunk: "@@ -10,2 +10,0 @@\n" +
					"-dead\n-code\n",
			},
		},
		{
			name: "single line anchor via Line",
			s: types.ReviewSuggestion{
				File: "x.go", Line: 7,
				CurrentCode:  "old\n",
				ProposedCode: "new\n",
			},
			want: FileEdit{
				Path:      "x.go",
				Removals:  []Line{{Number: 7, Text: "old"}},
				Additions: []Line{{Number: 7, Text: "new"}},
				Hunk:      "@@ -7,1 +7,1 @@\n-old\n+new\n",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildPlan([]types.ReviewSuggestion{tc.s})
			if err != nil {
				t.Fatalf("BuildPlan error: %v", err)
			}
			if len(plan.Files) != 1 {
				t.Fatalf("files = %d, want 1", len(plan.Files))
			}
			if got := plan.Files[0]; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("file edit:\n got %#v\nwant %#v", got, tc.want)
			}
		})
	}
}

func TestBuildPlanSkipsUnsafeSuggestions(t *testing.T) {
	valid := types.ReviewSuggestion{
		File: "internal/app.go", StartLine: 10,
		CurrentCode: "return err\n", ProposedCode: "return nil\n",
	}
	tests := []struct {
		name string
		s    types.ReviewSuggestion
	}{
		{"empty current code", types.ReviewSuggestion{File: "a.go", StartLine: 1, CurrentCode: "", ProposedCode: "x\n"}},
		{"whitespace-only current code", types.ReviewSuggestion{File: "a.go", StartLine: 1, CurrentCode: "  \n", ProposedCode: "x\n"}},
		{"proposed equals current", types.ReviewSuggestion{File: "a.go", StartLine: 1, CurrentCode: "x\n", ProposedCode: "x\n"}},
		{"equal up to trailing newline", types.ReviewSuggestion{File: "a.go", StartLine: 1, CurrentCode: "x\n", ProposedCode: "x"}},
		{"equal up to CRLF", types.ReviewSuggestion{File: "a.go", StartLine: 1, CurrentCode: "x\r\ny", ProposedCode: "x\ny"}},
		{"empty file", types.ReviewSuggestion{File: "", StartLine: 1, CurrentCode: "x\n", ProposedCode: "y\n"}},
		{"absolute file", types.ReviewSuggestion{File: "/etc/passwd", StartLine: 1, CurrentCode: "x\n", ProposedCode: "y\n"}},
		{"parent traversal", types.ReviewSuggestion{File: "../secret.go", StartLine: 1, CurrentCode: "x\n", ProposedCode: "y\n"}},
		{"nested parent traversal", types.ReviewSuggestion{File: "a/../../b.go", StartLine: 1, CurrentCode: "x\n", ProposedCode: "y\n"}},
		{"windows absolute", types.ReviewSuggestion{File: `C:\foo\bar.go`, StartLine: 1, CurrentCode: "x\n", ProposedCode: "y\n"}},
		{"no line anchor", types.ReviewSuggestion{File: "a.go", CurrentCode: "x\n", ProposedCode: "y\n"}},
		{"start line after end line", types.ReviewSuggestion{File: "a.go", StartLine: 5, EndLine: 3, CurrentCode: "x\n", ProposedCode: "y\n"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildPlan([]types.ReviewSuggestion{tc.s, valid})
			if err != nil {
				t.Fatalf("BuildPlan error: %v", err)
			}
			if len(plan.Files) != 1 {
				t.Fatalf("files = %d, want 1 (unsafe suggestion must be skipped, valid one kept)", len(plan.Files))
			}
			if plan.Files[0].Path != "internal/app.go" {
				t.Fatalf("kept path = %q, want internal/app.go", plan.Files[0].Path)
			}
		})
	}
}

func TestBuildPlanAllUnsafeYieldsEmptyPlan(t *testing.T) {
	plan, err := BuildPlan([]types.ReviewSuggestion{
		{File: "", StartLine: 1, CurrentCode: "x\n", ProposedCode: "y\n"},
		{File: "a.go", StartLine: 1, CurrentCode: "x\n", ProposedCode: "x\n"},
	})
	if err != nil {
		t.Fatalf("BuildPlan error: %v", err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("files = %d, want 0", len(plan.Files))
	}
	patch, err := BuildPatch([]types.ReviewSuggestion{{File: "a.go", StartLine: 1, CurrentCode: "x\n", ProposedCode: "x\n"}})
	if err != nil {
		t.Fatalf("BuildPatch error: %v", err)
	}
	if patch != "" {
		t.Fatalf("patch = %q, want empty", patch)
	}
}

func TestBuildPatchFormatsStandardUnifiedDiff(t *testing.T) {
	suggestions := []types.ReviewSuggestion{
		{File: "internal/app.go", StartLine: 10, CurrentCode: "a\nb\n", ProposedCode: "a\nB\n"},
		{File: "other.go", StartLine: 3, CurrentCode: "old\n", ProposedCode: "new\n"},
	}
	patch, err := BuildPatch(suggestions)
	if err != nil {
		t.Fatalf("BuildPatch error: %v", err)
	}
	want := "--- a/internal/app.go\n" +
		"+++ b/internal/app.go\n" +
		"@@ -10,2 +10,2 @@\n" +
		" a\n-b\n+B\n" +
		"--- a/other.go\n" +
		"+++ b/other.go\n" +
		"@@ -3,1 +3,1 @@\n" +
		"-old\n+new\n"
	if patch != want {
		t.Fatalf("patch:\n%s\nwant:\n%s", patch, want)
	}
}

func TestBuildPatchMergesHunksPerFileInLineOrder(t *testing.T) {
	suggestions := []types.ReviewSuggestion{
		{File: "a.go", StartLine: 20, CurrentCode: "z\n", ProposedCode: "Z\n"},
		{File: "a.go", StartLine: 5, CurrentCode: "a\n", ProposedCode: "A\n"},
	}
	patch, err := BuildPatch(suggestions)
	if err != nil {
		t.Fatalf("BuildPatch error: %v", err)
	}
	want := "--- a/a.go\n" +
		"+++ b/a.go\n" +
		"@@ -5,1 +5,1 @@\n" +
		"-a\n+A\n" +
		"@@ -20,1 +20,1 @@\n" +
		"-z\n+Z\n"
	if patch != want {
		t.Fatalf("patch:\n%s\nwant:\n%s", patch, want)
	}
}

func TestBuildPatchDeterministic(t *testing.T) {
	suggestions := []types.ReviewSuggestion{
		{File: "b.go", StartLine: 2, CurrentCode: "1\n", ProposedCode: "2\n"},
		{File: "a.go", StartLine: 4, CurrentCode: "x\n", ProposedCode: "y\n"},
	}
	first, err := BuildPatch(suggestions)
	if err != nil {
		t.Fatalf("BuildPatch error: %v", err)
	}
	for i := 0; i < 5; i++ {
		got, err := BuildPatch(suggestions)
		if err != nil {
			t.Fatalf("BuildPatch error: %v", err)
		}
		if got != first {
			t.Fatalf("BuildPatch not deterministic on run %d", i)
		}
	}
	if !strings.HasPrefix(first, "--- a/a.go\n") {
		t.Fatalf("expected files sorted by path, got:\n%s", first)
	}
}
