package tools

import (
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// RepositoryCost is the declared cost of a repository tool.
const RepositoryCost = "leitura local da revisão revisada, menos de 1s; resultado de até ~2000 tokens, somado ao teto max_read_bytes da revisão"

// RepositoryOffers are the read-only repository tools (AUR-526): read a
// file, search text, find a symbol's definitions and uses, and the diff of
// another changed file. They share rev's byte budget; redact is the
// AUR-009 filter every result passes through.
func RepositoryOffers(rev *Revision, diff *types.Diff, provider grammar.Provider, redact func(string) string) []Offer {
	return []Offer{
		{Tool: NewReadFileTool(rev, redact), Cost: RepositoryCost},
		{Tool: NewSearchTool(rev, redact), Cost: RepositoryCost},
		{Tool: NewSymbolTool(rev, provider, redact), Cost: RepositoryCost},
		{Tool: NewDiffFileTool(diff, rev.Budget(), redact), Cost: RepositoryCost},
	}
}
