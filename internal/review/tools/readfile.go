package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/llm"
)

// ReadFileToolName is the tool that reads one file of the reviewed revision.
const ReadFileToolName = "read_file"

// readFileMaxLines bounds the lines one call returns; the model pages with
// start_line.
const readFileMaxLines = 200

type readFileArgs struct {
	Path      string `json:"path" desc:"caminho relativo de um arquivo da revisão revisada"`
	StartLine int    `json:"start_line,omitempty" desc:"primeira linha (1 quando omitida)"`
	EndLine   int    `json:"end_line,omitempty" desc:"última linha (no máximo 200 linhas por chamada)"`
}

// ReadFileTool returns numbered lines of one file of the reviewed revision.
type ReadFileTool struct {
	rev    *Revision
	redact func(string) string
}

// NewReadFileTool reads from rev; redact is the AUR-009 filter.
func NewReadFileTool(rev *Revision, redact func(string) string) *ReadFileTool {
	return &ReadFileTool{rev: rev, redact: redact}
}

// Spec implements deliberation.Tool.
func (t *ReadFileTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        ReadFileToolName,
		Description: "Lê linhas numeradas de um arquivo da revisão revisada (qualquer linguagem). Recusa caminho fora do repositório, link simbólico, arquivo ignorado pela política e arquivo de segredo.",
		Parameters:  llm.SchemaOf(readFileArgs{}),
	}
}

// Run implements deliberation.Tool.
func (t *ReadFileTool) Run(_ context.Context, raw json.RawMessage) (deliberation.Result, error) {
	var args readFileArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return deliberation.Result{}, err
	}
	data, err := t.rev.Read(args.Path)
	if err != nil {
		return deliberation.Result{}, err
	}
	if grammar.LooksBinary(data) {
		return deliberation.Result{}, fmt.Errorf("%q é binário", args.Path)
	}
	lines := strings.Split(string(data), "\n")
	first, last, err := lineWindow(args.StartLine, args.EndLine, len(lines))
	if err != nil {
		return deliberation.Result{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (linhas %d-%d de %d)\n", args.Path, first, last, len(lines))
	for i := first; i <= last; i++ {
		fmt.Fprintf(&b, "%d: %s\n", i, lines[i-1])
	}
	return charged(t.rev.Budget(), t.redact, b.String(), fmt.Sprintf("%s:%d-%d", args.Path, first, last))
}

// lineWindow resolves the requested 1-based window within total lines.
func lineWindow(start, end, total int) (int, int, error) {
	if start <= 0 {
		start = 1
	}
	if start > total {
		return 0, 0, fmt.Errorf("start_line %d além do fim do arquivo (%d linhas)", start, total)
	}
	if end <= 0 || end > total {
		end = total
	}
	if end < start {
		return 0, 0, fmt.Errorf("end_line %d antes de start_line %d", end, start)
	}
	if end-start+1 > readFileMaxLines {
		end = start + readFileMaxLines - 1
	}
	return start, end, nil
}
