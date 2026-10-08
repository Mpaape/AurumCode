package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// DiffFileToolName returns the diff of one changed file.
const DiffFileToolName = "changed_file_diff"

type diffFileArgs struct {
	Path string `json:"path" desc:"caminho de um arquivo alterado nesta revisão"`
}

// DiffFileTool returns the reviewed diff of another changed file, for a
// review whose prompt carries only part of the diff (a batch).
type DiffFileTool struct {
	files  map[string]types.DiffFile
	budget *ByteBudget
	redact func(string) string
}

// NewDiffFileTool offers the files of diff; budget is shared with the
// other repository tools.
func NewDiffFileTool(diff *types.Diff, budget *ByteBudget, redact func(string) string) *DiffFileTool {
	files := map[string]types.DiffFile{}
	if diff != nil {
		for _, f := range diff.Files {
			files[f.Path] = f
		}
	}
	return &DiffFileTool{files: files, budget: budget, redact: redact}
}

// Spec implements deliberation.Tool.
func (t *DiffFileTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        DiffFileToolName,
		Description: "Devolve o diff revisado de um arquivo alterado nesta revisão.",
		Parameters:  llm.SchemaOf(diffFileArgs{}),
	}
}

// Run implements deliberation.Tool.
func (t *DiffFileTool) Run(_ context.Context, raw json.RawMessage) (deliberation.Result, error) {
	var args diffFileArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return deliberation.Result{}, err
	}
	f, ok := t.files[args.Path]
	if !ok {
		return deliberation.Result{}, fmt.Errorf("%q não é um arquivo alterado nesta revisão", args.Path)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "diff de %s\n", f.Path)
	for _, h := range f.Hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, line := range h.Lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return charged(t.budget, t.redact, b.String(), fmt.Sprintf("%s: %d hunk(s)", f.Path, len(f.Hunks)))
}
