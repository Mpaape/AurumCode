package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAUR558ArchitectureDocCitesEveryInternalPackage is AC-003: every
// package directory of internal/ that holds Go code is cited by
// docs/architecture.md, and each extension point is described.
func TestAUR558ArchitectureDocCitesEveryInternalPackage(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(doc)
	entries, err := os.ReadDir(filepath.Join("..", "..", "internal"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		if !e.IsDir() || !hasGoFiles(filepath.Join("..", "..", "internal", e.Name())) {
			continue
		}
		checked++
		if !strings.Contains(text, "`internal/"+e.Name()+"`") {
			t.Errorf("docs/architecture.md does not cite internal/%s", e.Name())
		}
	}
	if checked < 20 {
		t.Fatalf("only %d internal packages found; the walk no longer sees internal/", checked)
	}
	for _, point := range []string{"A gate contributor", "A scanner", "A configuration section", "A BOM type", "A grammar"} {
		if !strings.Contains(text, "**"+point+".**") {
			t.Errorf("docs/architecture.md lacks the extension point %q", point)
		}
	}
}

func hasGoFiles(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") {
			found = true
		}
		return nil
	})
	return found
}
