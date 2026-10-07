package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/llm"
)

// ContextToolName is the codebase-context tool.
const ContextToolName = "codebase_context"

// contextArgs names the changed file whose context the model asks for.
type contextArgs struct {
	Path string `json:"path" desc:"caminho de um arquivo alterado no diff"`
}

// ContextTool resolves the bounded, deterministic codebase context
// (symbols, references, dependents) of one changed file. It only answers
// for a path of the diff: it is never a general file reader.
type ContextTool struct {
	root    string
	changed map[string]bool
	resolve func(root string, changed []string) (*codebasectx.Pack, error)
}

// NewContextTool offers the context of the changed files under root.
func NewContextTool(root string, changed []string) *ContextTool {
	set := make(map[string]bool, len(changed))
	for _, p := range changed {
		set[p] = true
	}
	return &ContextTool{root: root, changed: set, resolve: codebasectx.NewResolver().Resolve}
}

// Spec implements deliberation.Tool.
func (t *ContextTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        ContextToolName,
		Description: "Devolve o contexto do código de um arquivo alterado: símbolos definidos, referências e arquivos que dependem dele.",
		Parameters:  llm.SchemaOf(contextArgs{}),
	}
}

// Run implements deliberation.Tool.
func (t *ContextTool) Run(_ context.Context, raw json.RawMessage) (deliberation.Result, error) {
	var args contextArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return deliberation.Result{}, err
	}
	if !t.changed[args.Path] {
		return deliberation.Result{}, fmt.Errorf("%q não é um arquivo alterado neste diff", args.Path)
	}
	pack, err := t.resolve(t.root, []string{args.Path})
	if err != nil {
		return deliberation.Result{}, err
	}
	body, err := json.Marshal(pack)
	if err != nil {
		return deliberation.Result{}, err
	}
	sum := sha256.Sum256(body)
	return deliberation.Result{
		Content: string(body),
		Summary: fmt.Sprintf("%d símbolo(s), %d dependente(s)", len(pack.Symbols), len(pack.Dependents)),
		Digest:  hex.EncodeToString(sum[:]),
	}, nil
}

// WithExclude keeps the paths exclude reports (the policy's ignore globs
// and secret files) out of the context the tool returns (AUR-470).
func (t *ContextTool) WithExclude(exclude func(string) bool) *ContextTool {
	t.resolve = codebasectx.NewResolver().WithExclude(exclude).Resolve
	return t
}
