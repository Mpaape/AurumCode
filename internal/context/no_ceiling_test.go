package context

import (
	"fmt"
	"strings"
	"testing"
)

// noTruncation fails when the pack declares a ceiling nobody configured.
func noTruncation(t *testing.T, pack *Pack) {
	t.Helper()
	if note := strings.Join(pack.Dropped, "\n"); strings.Contains(note, "truncated") || strings.Contains(note, "skipped") {
		t.Fatalf("a ceiling nobody declared dropped context: %v", pack.Dropped)
	}
}

// Issue #179: the default resolver passes the old 2000-file, 256 KiB,
// 500-reference and 240-byte excerpt-line ceilings and still records every
// dependent, declaring no truncation.
func TestDefaultResolverHasNoFileOrReferenceCeiling(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"big/big.go": "package big\n\nfunc F() {}\n"}
	for n := 0; n < 2100; n++ {
		files[fmt.Sprintf("use/u%04d.go", n)] = "package use\n\nfunc u() { F() }\n"
	}
	long := "func g() { F() } // " + strings.Repeat("y", 300)
	files["use/large.go"] = "package use\n\n" + strings.Repeat("// filler line\n", (300<<10)/15) + long + "\n"
	writeTree(t, root, files)

	pack, err := NewResolver().Resolve(root, []string{"big/big.go"})
	if err != nil {
		t.Fatal(err)
	}
	noTruncation(t, pack)
	if len(pack.Dependents) != 2101 {
		t.Fatalf("dependents = %d, want all 2101", len(pack.Dependents))
	}
	found := false
	for _, s := range pack.Snippets {
		if s.File == "use/large.go" && strings.Contains(s.Text, long) {
			found = true
		}
	}
	if !found {
		t.Fatal("the reference past 256 KiB was not excerpted whole")
	}
}

// Past the old 4096-symbol ceiling the symbol reference scan still runs.
func TestDefaultResolverScansPastTheOldSymbolCeiling(t *testing.T) {
	root := t.TempDir()
	var symbols strings.Builder
	symbols.WriteString("package big\n\n")
	for n := 0; n < 4100; n++ {
		fmt.Fprintf(&symbols, "func G%d() {}\n", n)
	}
	writeTree(t, root, map[string]string{"big/big.go": symbols.String(), "use/u.go": "package use\n\nfunc u() { G4099() }\n"})

	pack, err := NewResolver().Resolve(root, []string{"big/big.go"})
	if err != nil {
		t.Fatal(err)
	}
	noTruncation(t, pack)
	if len(pack.Dependents) != 1 || pack.Dependents[0] != "use/u.go" {
		t.Fatalf("dependents = %v, want use/u.go", pack.Dependents)
	}
}
