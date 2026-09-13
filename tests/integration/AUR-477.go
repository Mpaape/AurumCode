// AUR-477 integration selector. DISTINCT ASSERTION: the coverage
// declaration's COMPOSITION on a large diff -- the exact omitted count, the
// bounded per-file list, and the explicit "... and N more" line. The unit
// selector covers error/nil and the budget; this one covers what the text
// actually says, because a bounded list that silently dropped the count would
// be the very defect this card exists to kill.
package integration

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func IntegrationAUR477(t *testing.T) {
	builder := prompt.NewPromptBuilder()

	files := make([]types.DiffFile, 0, 200)
	for i := 0; i < 200; i++ {
		files = append(files, types.DiffFile{
			Path:  fmt.Sprintf("src/features/very/deeply/nested/module%d.go", i),
			Hunks: []types.DiffHunk{{Lines: []string{fmt.Sprintf("+var x%d = 1", i)}}},
		})
	}
	diff := &types.Diff{Files: files}
	metrics := &analyzer.DiffMetrics{TotalFiles: 200}

	parts, err := builder.BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens: 1500, SchemaKind: "summary", Role: "reviewer",
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}

	if !strings.Contains(parts.User, "## Review Coverage") {
		t.Fatal("coverage declaration missing")
	}

	omittedCount, err := strconv.Atoi(parts.Meta["code_files_omitted"])
	if err != nil {
		t.Fatalf("code_files_omitted is not an int: %q", parts.Meta["code_files_omitted"])
	}
	if omittedCount < 1 {
		t.Fatalf("expected some omitted files on a 200-file diff at MaxTokens=1500, got %d", omittedCount)
	}

	wantCount := fmt.Sprintf("Code files NOT reviewed by this review (token budget): %d", omittedCount)
	if !strings.Contains(parts.User, wantCount) {
		t.Fatalf("declaration missing the exact omitted count: %q", wantCount)
	}

	// The per-file omitted list is bounded; when more than the bound are
	// omitted it names them by count, never silently.
	if omittedCount > 20 && !strings.Contains(parts.User, "more code files not reviewed") {
		t.Fatal("bounded omitted list missing the explicit '... and N more' line")
	}
}
