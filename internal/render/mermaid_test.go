package render

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestMermaidHeaderEdgesAndFiles(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{
		{
			Path: "internal/render/summary.go",
			Hunks: []types.DiffHunk{
				{Lines: []string{`+import "github.com/Mpaape/AurumCode/internal/render/mermaid"`}},
			},
		},
		{Path: "internal/render/mermaid.go"},
	}}

	out, err := Mermaid(diff)
	if err != nil {
		t.Fatalf("Mermaid returned an error: %v", err)
	}
	if !strings.HasPrefix(out, "flowchart TD\n") {
		t.Fatalf("expected a valid flowchart header, got:\n%s", out)
	}
	for _, f := range []string{"internal/render/summary.go", "internal/render/mermaid.go"} {
		if !strings.Contains(out, f) {
			t.Fatalf("diagram must contain changed file %q:\n%s", f, out)
		}
	}
	if !strings.Contains(out, "-->") {
		t.Fatalf("expected an inferred dependency edge, got:\n%s", out)
	}
}

func TestMermaidFallbackNoEdges(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{
		{Path: "a.go"},
		{Path: "b.go"},
	}}

	out, err := Mermaid(diff)
	if err != nil {
		t.Fatalf("Mermaid returned an error: %v", err)
	}
	if !strings.HasPrefix(out, "flowchart TD\n") {
		t.Fatalf("expected a valid flowchart header, got:\n%s", out)
	}
	for _, f := range []string{"a.go", "b.go"} {
		if !strings.Contains(out, f) {
			t.Fatalf("fallback diagram must contain changed file %q:\n%s", f, out)
		}
	}
	if strings.Contains(out, "-->") {
		t.Fatalf("fallback diagram should have no edges, got:\n%s", out)
	}
}

func TestMermaidEmptyErrors(t *testing.T) {
	if _, err := Mermaid(nil); err == nil {
		t.Error("expected an error for a nil diff")
	}
	if _, err := Mermaid(&types.Diff{}); err == nil {
		t.Error("expected an error for an empty diff")
	}
}

func TestMermaidDeterministic(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{Path: "a.go"}, {Path: "b.go"}}}
	a, err := Mermaid(diff)
	if err != nil {
		t.Fatalf("Mermaid returned an error: %v", err)
	}
	b, err := Mermaid(diff)
	if err != nil {
		t.Fatalf("Mermaid returned an error: %v", err)
	}
	if a != b {
		t.Error("Mermaid output must be deterministic across calls")
	}
}
