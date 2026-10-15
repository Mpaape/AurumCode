package context

import (
	"reflect"
	"testing"
)

// The context carries what can reach the change, not every file that
// happens to contain the same word: prose and plans, files of another
// language, packages that do not import the change, and names other files
// also define stay out. A real symbol keeps every reference.
func TestReferencesAreOnlyTheFilesThatCanReachTheChange(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"lib/lib.go":        "package lib\n\nfunc Changed() {}\n\nfunc (T) String() string { return \"\" }\n\ntype T struct{}\n",
		"lib/same.go":       "package lib\n\nfunc a() { Changed() }\n",
		"app/user.go":       "package app\n\nimport \"example.com/m/lib\"\n\nfunc b() { lib.Changed() }\n",
		"other/stranger.go": "package other\n\nimport \"fmt\"\n\nfunc Changed() { fmt.Println() }\n",
		"other/strings.go":  "package other\n\nimport \"fmt\"\n\nfunc (x X) String() string { return fmt.Sprint(Changed) }\n\ntype X struct{}\n",
		"docs/plan.md":      "# Plan\n\nCall Changed() and String() everywhere.\n",
		"notes.txt":         "Changed\n",
		"tools/run.sh":      "#!/bin/sh\nChanged\n",
	})
	pack, err := NewResolver().Resolve(root, []string{"lib/lib.go"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"app/user.go", "lib/same.go"}; !reflect.DeepEqual(pack.Dependents, want) {
		t.Fatalf("dependents = %v, want %v", pack.Dependents, want)
	}
	for _, ref := range pack.References {
		if ref.Symbol == "String" {
			t.Fatalf("a name another file also defines yielded a reference: %+v", ref)
		}
	}
}

// What a shell script runs is a use, not a definition, and punctuation or
// an expanded word is never a symbol.
func TestShellCommandsAreNotSymbols(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"lib/tut.sh":  "#!/bin/bash\nexpect_rc() { :; }\nfn=git\n\"$fn\" status\ngit log\n: noop\n",
		"lib/case.sh": "#!/bin/bash\nexpect_rc 0 git status\n",
	})
	pack, err := NewResolver().Resolve(root, []string{"lib/tut.sh"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range pack.Symbols {
		if sym == "git" || sym == ":" || sym == "\"$fn\"" || !isIdentifier(sym) {
			t.Fatalf("symbols = %v: %q is not a definition", pack.Symbols, sym)
		}
	}
	if want := []string{"lib/case.sh"}; !reflect.DeepEqual(pack.Dependents, want) {
		t.Fatalf("dependents = %v, want %v (the shell function is still referenced)", pack.Dependents, want)
	}
}
