package prosefiles

import "testing"

func TestAUR609IsProsePath(t *testing.T) {
	for p, want := range map[string]bool{
		"a/b.txt": true, "run.LOG": true, "README.md": true, "docs/x.rst": true,
		"guide.markdown": true, "manual.adoc": true,
		"main.go": false, "app.py": false, "index.html": false, "config.yml": false,
		"Makefile": false, "notes.md.go": false, "": false,
	} {
		if got := IsProsePath(p); got != want {
			t.Fatalf("IsProsePath(%q) = %v, want %v", p, got, want)
		}
	}
}
