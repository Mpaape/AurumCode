package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func fitPack() Pack {
	pack := Pack{Symbols: []string{"Rare", "Common"}, Dependents: []string{"a.go", "b.go"}}
	for i := 0; i < 200; i++ {
		file := fmt.Sprintf("f%03d.go", i)
		pack.References = append(pack.References, Reference{File: file, Symbol: "Common", Line: 1})
		pack.Snippets = append(pack.Snippets, Snippet{File: file, Line: 1, Symbol: "Common", Kind: "use", Text: strings.Repeat("c", 200)})
	}
	pack.References = append(pack.References, Reference{File: "z.go", Symbol: "Rare", Line: 9})
	pack.Snippets = append(pack.Snippets, Snippet{File: "z.go", Line: 9, Symbol: "Rare", Kind: "use", Text: "Rare()"})
	return pack
}

func TestFitLeavesAPackThatFitsUntouched(t *testing.T) {
	data, _ := json.Marshal(fitPack())
	got, red := Fit(string(data), len(data))
	if got != string(data) || red.Reduced() {
		t.Fatalf("a pack that fits was changed: %+v", red)
	}
}

// Snippets go first, the least specific symbol's first, and the reduction is
// declared in the pack itself.
func TestFitDropsTheLeastSpecificSnippetsFirstAndDeclaresIt(t *testing.T) {
	data, _ := json.Marshal(fitPack())
	got, red := Fit(string(data), len(data)/3)
	if len(got) > len(data)/3 || !red.Reduced() || red.Discarded {
		t.Fatalf("Fit = %d bytes, %+v", len(got), red)
	}
	var pack Pack
	if err := json.Unmarshal([]byte(got), &pack); err != nil {
		t.Fatalf("the reduced pack is not JSON: %v", err)
	}
	if red.ReferencesKept != red.References || red.SymbolsKept != red.Symbols || red.SnippetsKept == 0 || red.SnippetsKept >= red.Snippets {
		t.Fatalf("reduction %+v: only snippets had to go", red)
	}
	if !strings.Contains(got, `"Rare()"`) {
		t.Fatal("the snippet of the most specific symbol was dropped")
	}
	if !strings.Contains(pack.Reduced, fmt.Sprintf("kept %d of %d snippets", red.SnippetsKept, red.Snippets)) {
		t.Fatalf("the pack does not declare its reduction: %q", pack.Reduced)
	}
}

func TestFitDiscardsWhatIsNotAPackOrDoesNotFitEmptied(t *testing.T) {
	if got, red := Fit(strings.Repeat("not json ", 100), 10); got != "" || !red.Discarded {
		t.Fatalf("Fit(not a pack) = %q, %+v", got, red)
	}
	data, _ := json.Marshal(fitPack())
	if got, red := Fit(string(data), 10); got != "" || !red.Discarded || red.Snippets != 201 {
		t.Fatalf("Fit(10 bytes) = %q, %+v", got, red)
	}
}
