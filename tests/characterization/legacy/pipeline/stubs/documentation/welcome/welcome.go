package welcome

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/llm"
)

var events []string

func ResetTrace() { events = nil }

func Trace() []string { return append([]string(nil), events...) }

type Generator struct{}

type GenerateOptions struct {
	ReadmePath string
	OutputPath string
	ProjectDir string
	Title      string
	NavOrder   int
}

func NewGenerator(_ *llm.Orchestrator) *Generator { return &Generator{} }

func (*Generator) Generate(_ context.Context, options GenerateOptions) (string, error) {
	events = append(events, "generate")
	if err := os.MkdirAll(filepath.Dir(options.OutputPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(options.OutputPath, []byte("# "+options.Title+"\n"), 0o600); err != nil {
		return "", err
	}
	return options.OutputPath, nil
}
