package prompt

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// fakeProvider is a deterministic grammar.Provider: it knows only the
// extensions in langs and treats a line as a comment when it starts with the
// prefix registered for its language.
type fakeProvider struct {
	langs    map[string]string // file extension -> grammar name
	prefixes map[string]string // grammar name -> comment prefix
}

func (f fakeProvider) Languages() []string {
	var out []string
	for _, l := range f.langs {
		out = append(out, l)
	}
	return out
}
func (f fakeProvider) Detect(name string, _ []byte) string {
	for ext, l := range f.langs {
		if strings.HasSuffix(name, ext) {
			return l
		}
	}
	return ""
}
func (f fakeProvider) Analyze(string, []byte) grammar.Structure { return grammar.Structure{} }
func (f fakeProvider) IsComment(lang, line string) (bool, bool) {
	p, ok := f.prefixes[lang]
	if !ok {
		return false, false
	}
	return strings.HasPrefix(line, p), true
}

func diffOf(path string, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{Path: path, Hunks: []types.DiffHunk{{Lines: lines}}}}}
}

// AUR-559 AC-002 (prompt side): the real runtime decides, per language.
func TestAUR559CommentOnlyChangeIsNotSubstantive(t *testing.T) {
	rt := grammar.Default()
	for _, f := range []string{"a.go", "a.py", "a.rb", "A.java", "main.tf", "Dockerfile"} {
		if HasSubstantiveCodeChangeWith(diffOf(f, "+# note", "+// note"), rt) &&
			HasSubstantiveCodeChangeWith(diffOf(f, "+total"), rt) == false {
			t.Errorf("%s: sanity", f)
		}
	}
	if HasSubstantiveCodeChangeWith(diffOf("a.go", "+// only a comment"), rt) {
		t.Error("go comment-only change must not be substantive")
	}
	if !HasSubstantiveCodeChangeWith(diffOf("a.go", "+total := 1"), rt) {
		t.Error("go code change must be substantive")
	}
}

// AUR-559 AC-003: no grammar -> no filter and a declared notice.
func TestAUR559NoGrammarNoFilterAndNotice(t *testing.T) {
	p := fakeProvider{langs: map[string]string{".aa": "aalang"}, prefixes: map[string]string{"aalang": "%%"}}
	d := diffOf("x.zzz", "+// looks like a comment", "+# also")
	if !HasSubstantiveCodeChangeWith(d, p) {
		t.Error("without a grammar nothing is filtered: the lines count as changes")
	}
	scope := ChangeScopeWith(d, p)
	if !strings.Contains(scope, "no grammar is available for x.zzz") {
		t.Errorf("notice missing from scope: %q", scope)
	}
	// With a grammar: filtered and no notice.
	d2 := diffOf("x.aa", "+%% a comment")
	if HasSubstantiveCodeChangeWith(d2, p) {
		t.Error("grammar says comment: not substantive")
	}
	if strings.Contains(ChangeScopeWith(d2, p), "Notice") {
		t.Error("no notice when a grammar exists")
	}
}
