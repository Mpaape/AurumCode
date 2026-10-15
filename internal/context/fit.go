package context

import (
	"encoding/json"
	"fmt"
	"sort"
)

// A pack is context, not the code under review: when the model's window is
// known and the pack would not leave room for the change itself, the pack
// is reduced to fit, and the reduction is declared -- in the pack, for the
// model, and in the Reduction the caller reports. Nothing here is a fixed
// ceiling: the size comes from the window the provider said it has.

// Reduction counts what a reduced pack kept of each part. Discarded is set
// when not even the smallest pack fits, or the text was not a pack, and the
// context was left out whole.
type Reduction struct {
	Snippets, SnippetsKept     int
	References, ReferencesKept int
	Dependents, DependentsKept int
	Symbols, SymbolsKept       int
	Discarded                  bool
}

// Reduced reports whether anything was left out.
func (r Reduction) Reduced() bool {
	return r.Discarded || r.SnippetsKept < r.Snippets || r.ReferencesKept < r.References ||
		r.DependentsKept < r.Dependents || r.SymbolsKept < r.Symbols
}

// Fit returns packJSON reduced to at most maxBytes, and what was kept. A
// pack that fits is returned unchanged. Otherwise snippets go first, then
// references, then dependents, then symbols; within snippets and references
// the ones of the least specific symbol (the one with the most hits) go
// first, so what stays is what points at the change most precisely. The
// kept items keep their original order. A text that is not a pack, or a
// pack that does not fit even emptied, is discarded.
func Fit(packJSON string, maxBytes int) (string, Reduction) {
	if len(packJSON) <= maxBytes {
		return packJSON, Reduction{}
	}
	var pack Pack
	if err := json.Unmarshal([]byte(packJSON), &pack); err != nil {
		return "", Reduction{Discarded: true}
	}
	red := Reduction{
		Snippets: len(pack.Snippets), References: len(pack.References),
		Dependents: len(pack.Dependents), Symbols: len(pack.Symbols),
	}
	hits := symbolHits(pack)
	snippets := rankSnippets(pack.Snippets, hits)
	references := rankReferences(pack.References, hits)
	keep := counts{snippets: len(snippets), references: len(references), dependents: len(pack.Dependents), symbols: len(pack.Symbols)}
	render := func(k counts) string {
		trial := pack
		trial.Snippets = keptSnippets(snippets, k.snippets)
		trial.References = keptReferences(references, k.references)
		trial.Dependents = pack.Dependents[:k.dependents]
		trial.Symbols = pack.Symbols[:k.symbols]
		trial.Reduced = k.declare(red)
		data, err := json.Marshal(trial)
		if err != nil {
			return ""
		}
		return string(data)
	}
	fits := func(k counts) bool {
		text := render(k)
		return text != "" && len(text) <= maxBytes
	}
	for _, part := range []*int{&keep.snippets, &keep.references, &keep.dependents, &keep.symbols} {
		whole := *part
		*part = 0
		if !fits(keep) {
			continue
		}
		*part = largestFitting(whole, func(n int) bool {
			saved := *part
			*part = n
			ok := fits(keep)
			*part = saved
			return ok
		})
		text := render(keep)
		red.SnippetsKept, red.ReferencesKept, red.DependentsKept, red.SymbolsKept = keep.snippets, keep.references, keep.dependents, keep.symbols
		return text, red
	}
	return "", Reduction{Snippets: red.Snippets, References: red.References, Dependents: red.Dependents, Symbols: red.Symbols, Discarded: true}
}

// counts is how many of each part a trial pack keeps.
type counts struct{ snippets, references, dependents, symbols int }

// declare is the pack's own note of the reduction, for the model.
func (k counts) declare(whole Reduction) string {
	return fmt.Sprintf("reduced to fit the model's context window: kept %d of %d snippets, %d of %d references, %d of %d dependents, %d of %d symbols; the rest of the repository was not shown",
		k.snippets, whole.Snippets, k.references, whole.References, k.dependents, whole.Dependents, k.symbols, whole.Symbols)
}

// largestFitting is the largest n in [0, whole] for which fits holds,
// given that it holds for 0 and that fitting is monotonic in n.
func largestFitting(whole int, fits func(int) bool) int {
	lo, hi := 0, whole
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// symbolHits counts the references and snippets of each symbol.
func symbolHits(pack Pack) map[string]int {
	hits := map[string]int{}
	for _, r := range pack.References {
		hits[r.Symbol]++
	}
	for _, s := range pack.Snippets {
		hits[s.Symbol]++
	}
	return hits
}

type rankedSnippet struct {
	index int
	item  Snippet
}

type rankedReference struct {
	index int
	item  Reference
}

// rankSnippets orders snippets most specific symbol first, ties in their
// original order.
func rankSnippets(items []Snippet, hits map[string]int) []rankedSnippet {
	out := make([]rankedSnippet, len(items))
	for i, s := range items {
		out[i] = rankedSnippet{index: i, item: s}
	}
	sort.SliceStable(out, func(a, b int) bool { return hits[out[a].item.Symbol] < hits[out[b].item.Symbol] })
	return out
}

// rankReferences is rankSnippets for references.
func rankReferences(items []Reference, hits map[string]int) []rankedReference {
	out := make([]rankedReference, len(items))
	for i, r := range items {
		out[i] = rankedReference{index: i, item: r}
	}
	sort.SliceStable(out, func(a, b int) bool { return hits[out[a].item.Symbol] < hits[out[b].item.Symbol] })
	return out
}

// keptSnippets is the first n ranked snippets, back in their original order.
func keptSnippets(ranked []rankedSnippet, n int) []Snippet {
	kept := append([]rankedSnippet(nil), ranked[:n]...)
	sort.Slice(kept, func(a, b int) bool { return kept[a].index < kept[b].index })
	out := make([]Snippet, 0, n)
	for _, k := range kept {
		out = append(out, k.item)
	}
	return out
}

// keptReferences is keptSnippets for references.
func keptReferences(ranked []rankedReference, n int) []Reference {
	kept := append([]rankedReference(nil), ranked[:n]...)
	sort.Slice(kept, func(a, b int) bool { return kept[a].index < kept[b].index })
	out := make([]Reference, 0, n)
	for _, k := range kept {
		out = append(out, k.item)
	}
	return out
}
