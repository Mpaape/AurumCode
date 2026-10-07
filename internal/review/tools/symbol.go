package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/llm"
)

// SymbolToolName is the definitions-and-uses lookup of a symbol.
const SymbolToolName = "find_symbol"

// symbolMaxHits bounds the occurrences one lookup returns.
const symbolMaxHits = 60

type symbolArgs struct {
	Name string `json:"name" desc:"nome do símbolo (função, método, tipo, constante)"`
}

// SymbolTool lists where a symbol is defined and used in the reviewed
// revision, in any language the grammar provider knows: the files that
// define it are those whose grammar structure declares the name; each
// occurrence is a whole-word match outside a comment line (the grammar
// decides what is a comment). A file without a grammar is still searched
// as text and says so. Nothing here knows a language.
type SymbolTool struct {
	rev     *Revision
	grammar grammar.Provider
	redact  func(string) string
}

// NewSymbolTool looks symbols up in rev through provider.
func NewSymbolTool(rev *Revision, provider grammar.Provider, redact func(string) string) *SymbolTool {
	return &SymbolTool{rev: rev, grammar: provider, redact: redact}
}

// Spec implements deliberation.Tool.
func (t *SymbolTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        SymbolToolName,
		Description: "Lista onde um símbolo é definido (pela gramática tree-sitter do arquivo) e onde é usado (arquivo:linha, fora de comentário) na revisão revisada, em qualquer linguagem.",
		Parameters:  llm.SchemaOf(symbolArgs{}),
	}
}

// Run implements deliberation.Tool.
func (t *SymbolTool) Run(ctx context.Context, raw json.RawMessage) (deliberation.Result, error) {
	var args symbolArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return deliberation.Result{}, err
	}
	if args.Name == "" || len(args.Name) > 128 || strings.ContainsFunc(args.Name, unicode.IsSpace) {
		return deliberation.Result{}, fmt.Errorf("name deve ser um identificador sem espaços, até 128 bytes")
	}
	var defined []string
	var b strings.Builder
	hits := 0
	for _, p := range t.rev.Paths() {
		if ctx.Err() != nil {
			return deliberation.Result{}, ctx.Err()
		}
		data, err := t.rev.Read(p)
		if err != nil || grammar.LooksBinary(data) || !strings.Contains(string(data), args.Name) {
			continue
		}
		st := t.grammar.Analyze(p, data)
		if containsString(st.Symbols, args.Name) {
			defined = append(defined, p)
		}
		lang := t.grammar.Detect(p, data)
		for i, line := range strings.Split(string(data), "\n") {
			if !hasWord(line, args.Name) || t.isComment(lang, line) {
				continue
			}
			if hits == symbolMaxHits {
				fmt.Fprintf(&b, "[ocorrências truncadas em %d]\n", symbolMaxHits)
				return t.finish(args.Name, defined, b.String(), hits)
			}
			hits++
			note := ""
			if !st.HasStructure {
				note = " (sem gramática: texto)"
			}
			fmt.Fprintf(&b, "%s:%d%s: %s\n", p, i+1, note, clip(strings.TrimSpace(line)))
		}
	}
	return t.finish(args.Name, defined, b.String(), hits)
}

func (t *SymbolTool) finish(name string, defined []string, body string, hits int) (deliberation.Result, error) {
	where := "nenhum arquivo cuja gramática o declare"
	if len(defined) > 0 {
		where = strings.Join(defined, ", ")
	}
	head := fmt.Sprintf("símbolo %s: definido em %s; %d ocorrência(s) fora de comentário", name, where, hits)
	return charged(t.rev.Budget(), t.redact, head+"\n"+body, fmt.Sprintf("%d definição(ões), %d ocorrência(s)", len(defined), hits))
}

// isComment asks the file's grammar; without one no line is a comment.
func (t *SymbolTool) isComment(lang, line string) bool {
	if lang == "" || strings.TrimSpace(line) == "" {
		return false
	}
	comment, ok := t.grammar.IsComment(lang, line)
	return ok && comment
}

// hasWord reports name in line with no identifier rune touching either end.
func hasWord(line, name string) bool {
	for from := 0; ; {
		i := strings.Index(line[from:], name)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(name)
		before, _ := utf8.DecodeLastRuneInString(line[:start])
		after, _ := utf8.DecodeRuneInString(line[end:])
		if (start == 0 || !isIdentRune(before)) && (end == len(line) || !isIdentRune(after)) {
			return true
		}
		from = start + 1
	}
}

func isIdentRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
