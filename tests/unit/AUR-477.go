// AUR-477 unit selector. DISTINCT ASSERTION: the coverage-declaration
// reservation tracks what is actually declared, not the total file count, so
// a large diff still assembles a valid prompt (>=1 file reviewed) within the
// budget. It asserts on the prompt assembly only (error/nil, the segment and
// coverage counts, and Meta["estimated_tokens"] <= MaxTokens). No claim about
// the CLI's exit code (integration) or the assembled prompt text (covered by
// the acceptance/e2e selectors).
package unit

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur477Diff builds n code files, each with one tiny hunk, on long paths so a
// per-file "omitted" bullet is expensive while the code itself is cheap -- the
// exact shape where the AUR-467 worst-case reservation refused a large diff.
func aur477Diff(n int) *types.Diff {
	files := make([]types.DiffFile, 0, n)
	for i := 0; i < n; i++ {
		files = append(files, types.DiffFile{
			Path:  fmt.Sprintf("src/features/very/deeply/nested/module%d.go", i),
			Hunks: []types.DiffHunk{{Lines: []string{fmt.Sprintf("+var x%d = 1", i)}}},
		})
	}
	return &types.Diff{Files: files}
}

func aur477Estimated(t *testing.T, parts prompt.PromptParts) int {
	t.Helper()
	s := parts.Meta["estimated_tokens"]
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("estimated_tokens is not an int: %q", s)
	}
	return n
}

func TestAUR477(t *testing.T) {
	builder := prompt.NewPromptBuilder()

	t.Run("AC-001-nominal", func(t *testing.T) {
		// 200 code files, MaxTokens=1500. The old worst-case reservation
		// (one bullet per file) exceeded this budget and refused; the fix
		// bounds the omitted list and reserves what is actually declared, so
		// at least one file is still reviewed.
		diff := aur477Diff(200)
		metrics := &analyzer.DiffMetrics{TotalFiles: 200}
		parts, err := builder.BuildPrompt(diff, metrics, prompt.BuildOptions{
			MaxTokens: 1500, SchemaKind: "summary", Role: "reviewer",
		})
		if err != nil {
			t.Fatalf("BuildPrompt refused a large diff it should review: %v", err)
		}
		if used, _ := strconv.Atoi(parts.Meta["segments_used"]); used < 1 {
			t.Fatalf("expected at least one reviewed file, got segments_used=%d", used)
		}
		if got := aur477Estimated(t, parts); got > 1500 {
			t.Fatalf("assembled prompt exceeds the budget: estimated=%d > 1500", got)
		}
		if !strings.Contains(parts.User, "Code files NOT reviewed") {
			t.Fatal("coverage declaration missing the omitted count")
		}
	})

	t.Run("AC-002-teto", func(t *testing.T) {
		// The assembled prompt never exceeds MaxTokens across a spread of
		// budgets, measured on the final text (Meta is the whole System+User).
		for _, maxTokens := range []int{800, 1500, 2500, 4000} {
			diff := aur477Diff(200)
			metrics := &analyzer.DiffMetrics{TotalFiles: 200}
			parts, err := builder.BuildPrompt(diff, metrics, prompt.BuildOptions{
				MaxTokens: maxTokens, SchemaKind: "summary", Role: "reviewer",
			})
			if err != nil {
				t.Fatalf("MaxTokens=%d: unexpected refusal: %v", maxTokens, err)
			}
			if got := aur477Estimated(t, parts); got > maxTokens {
				t.Fatalf("MaxTokens=%d: assembled prompt exceeds budget: estimated=%d", maxTokens, got)
			}
		}
	})

	t.Run("AC-003-recusa", func(t *testing.T) {
		// When not even one code hunk fits after the minimal reservation, the
		// assembly refuses loudly rather than shipping a prompt with no diff.
		diff := aur477Diff(200)
		metrics := &analyzer.DiffMetrics{TotalFiles: 200}
		_, err := builder.BuildPrompt(diff, metrics, prompt.BuildOptions{
			MaxTokens: 50, SchemaKind: "summary", Role: "reviewer",
		})
		if err == nil {
			t.Fatal("expected refusal when no code hunk fits")
		}
		if !strings.Contains(err.Error(), "no room") && !strings.Contains(err.Error(), "no diff") {
			t.Fatalf("refusal should name the reason, got: %v", err)
		}
	})

	t.Run("AC-002-prose-only", func(t *testing.T) {
		// A diff with only documentation files has an unbounded prose list in
		// the declaration. A capped budget must refuse, never exceed MaxTokens
		// silently (the prose list is not trimmed by the code budget).
		files := make([]types.DiffFile, 0, 200)
		for i := 0; i < 200; i++ {
			files = append(files, types.DiffFile{
				Path:  fmt.Sprintf("docs/very/deeply/nested/manual-%d.md", i),
				Hunks: []types.DiffHunk{{Lines: []string{fmt.Sprintf("+documentation line %d", i)}}},
			})
		}
		diff := &types.Diff{Files: files}
		metrics := &analyzer.DiffMetrics{TotalFiles: 200}
		_, err := builder.BuildPrompt(diff, metrics, prompt.BuildOptions{
			MaxTokens: 1500, SchemaKind: "summary", Role: "reviewer",
		})
		if err == nil {
			t.Fatal("expected refusal when a prose-only diff exceeds the budget")
		}
	})
}
