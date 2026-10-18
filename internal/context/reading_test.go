package context

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

// countingGrammar is the runtime's grammar, counting each file it parses.
type countingGrammar struct {
	grammar.Provider
	mu     sync.Mutex
	parsed map[string]int
}

func (g *countingGrammar) Analyze(name string, content []byte) grammar.Structure {
	g.mu.Lock()
	g.parsed[name]++
	g.mu.Unlock()
	return g.Provider.Analyze(name, content)
}

// #198: a review in batches resolves the context of each batch's files. A
// Reading resolver reads and parses each repository file once whatever the
// number of resolutions, and each answers what a fresh resolver answers.
func TestReadingScansTheRepositoryOnce(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a/alpha.go":   "package a\n\nfunc Alpha() {}\n",
		"b/beta.go":    "package b\n\nfunc Beta() {}\n",
		"c/gamma.go":   "package c\n\nfunc Gamma() {}\n",
		"use/use.go":   "package use\n\nimport (\n\t\"example.com/m/a\"\n\t\"example.com/m/b\"\n\t\"example.com/m/c\"\n)\n\nvar _ = a.Alpha\nvar _ = b.Beta\nvar _ = c.Gamma\n",
		"use/other.go": "package use\n\nfunc Other() {}\n",
		"README.md":    "Alpha Beta Gamma\n",
	})
	counter := &countingGrammar{Provider: grammar.Default(), parsed: map[string]int{}}
	shared := Reading().WithGrammar(counter)
	batches := [][]string{{"a/alpha.go"}, {"b/beta.go"}, {"c/gamma.go"}, {"a/alpha.go", "b/beta.go"}}
	for _, changed := range batches {
		got, err := shared.Resolve(root, changed)
		if err != nil {
			t.Fatal(err)
		}
		want, err := NewResolver().Resolve(root, changed)
		if err != nil {
			t.Fatal(err)
		}
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Fatalf("changed %v: the shared reading answered\n%s\nwant\n%s", changed, gotJSON, wantJSON)
		}
		if len(got.References) == 0 {
			t.Fatalf("changed %v: no reference found, the fixture proves nothing", changed)
		}
	}
	if len(counter.parsed) != 6 {
		t.Fatalf("parsed %v: every file of the tree must have been read", counter.parsed)
	}
	for name, n := range counter.parsed {
		if n != 1 {
			t.Fatalf("%s parsed %d times over %d resolutions, want once", name, n, len(batches))
		}
	}
	if walks := len(shared.reading.walks); walks != 1 {
		t.Fatalf("the root was walked %d times", walks)
	}
}

// Without Reading every resolution reads the repository again: the
// counter above measures the sharing, not the fixture.
func TestFreshResolverReadsAgain(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a/alpha.go": "package a\n\nfunc Alpha() {}\n", "use.go": "package m\n\nvar _ = Alpha\n"})
	counter := &countingGrammar{Provider: grammar.Default(), parsed: map[string]int{}}
	fresh := NewResolver().WithGrammar(counter)
	for i := 0; i < 2; i++ {
		if _, err := fresh.Resolve(root, []string{"a/alpha.go"}); err != nil {
			t.Fatal(err)
		}
	}
	if counter.parsed["use.go"] < 2 {
		t.Fatalf("parsed %v", counter.parsed)
	}
}
