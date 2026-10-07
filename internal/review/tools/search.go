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

// SearchToolName is the literal text search over the reviewed revision.
const SearchToolName = "search_text"

// searchMaxHits bounds one search; maxHitLineBytes bounds each line shown.
const (
	searchMaxHits   = 50
	maxHitLineBytes = 240
)

type searchArgs struct {
	Query      string `json:"query" desc:"texto literal a procurar (sem expressão regular)"`
	PathPrefix string `json:"path_prefix,omitempty" desc:"restringe a busca a caminhos com este prefixo"`
}

// SearchTool finds a literal text in every readable file of the revision.
type SearchTool struct {
	rev    *Revision
	redact func(string) string
}

// NewSearchTool searches rev; redact is the AUR-009 filter.
func NewSearchTool(rev *Revision, redact func(string) string) *SearchTool {
	return &SearchTool{rev: rev, redact: redact}
}

// Spec implements deliberation.Tool.
func (t *SearchTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        SearchToolName,
		Description: "Procura um texto literal nos arquivos da revisão revisada e devolve arquivo:linha de cada ocorrência (até 50).",
		Parameters:  llm.SchemaOf(searchArgs{}),
	}
}

// Run implements deliberation.Tool.
func (t *SearchTool) Run(ctx context.Context, raw json.RawMessage) (deliberation.Result, error) {
	var args searchArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return deliberation.Result{}, err
	}
	if strings.TrimSpace(args.Query) == "" || strings.Contains(args.Query, "\n") {
		return deliberation.Result{}, fmt.Errorf("query deve ser um texto de uma linha, não vazio")
	}
	var b strings.Builder
	hits, skipped := 0, 0
	for _, p := range t.rev.Paths() {
		if ctx.Err() != nil {
			return deliberation.Result{}, ctx.Err()
		}
		if !strings.HasPrefix(p, args.PathPrefix) {
			continue
		}
		data, err := t.rev.Read(p)
		if err != nil || grammar.LooksBinary(data) {
			skipped++
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, args.Query) {
				continue
			}
			if hits == searchMaxHits {
				fmt.Fprintf(&b, "[busca truncada em %d ocorrências]\n", searchMaxHits)
				return t.finish(b.String(), hits, skipped)
			}
			hits++
			fmt.Fprintf(&b, "%s:%d: %s\n", p, i+1, clip(strings.TrimSpace(line)))
		}
	}
	return t.finish(b.String(), hits, skipped)
}

func (t *SearchTool) finish(body string, hits, skipped int) (deliberation.Result, error) {
	head := fmt.Sprintf("%d ocorrência(s)", hits)
	if skipped > 0 {
		head += fmt.Sprintf("; %d arquivo(s) omitido(s) (binário, divergente da revisão ou ilegível)", skipped)
	}
	return charged(t.rev.Budget(), t.redact, head+"\n"+body, head)
}

// clip bounds a line shown in a hit on a rune boundary.
func clip(line string) string {
	if len(line) <= maxHitLineBytes {
		return line
	}
	cut := maxHitLineBytes
	for cut > 0 && (line[cut]&0xC0) == 0x80 {
		cut--
	}
	return line[:cut] + "…"
}
