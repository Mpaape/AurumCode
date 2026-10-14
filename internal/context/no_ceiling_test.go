package context

import (
	"fmt"
	"strings"
	"testing"
)

// Issue #179: the default resolver passes every old ceiling -- 2000 files,
// 256 KiB per file, 500 references, 4096 symbols, 240 bytes per excerpt
// line -- and still records every dependent, declaring no truncation.
func TestDefaultResolverHasNoCeiling(t *testing.T) {
	root := t.TempDir()
	var symbols strings.Builder
	symbols.WriteString("package big\n\nfunc F() {}\n")
	for n := 0; n < 4100; n++ {
		fmt.Fprintf(&symbols, "func G%d() {}\n", n)
	}
	files := map[string]string{"big/big.go": symbols.String()}
	for n := 0; n < 2100; n++ {
		files[fmt.Sprintf("use/u%04d.txt", n)] = "calls F() here\n"
	}
	long := "calls F() " + strings.Repeat("y", 300)
	files["use/large.txt"] = strings.Repeat("filler line\n", (300<<10)/12) + long + "\n"
	writeTree(t, root, files)

	pack, err := NewResolver().Resolve(root, []string{"big/big.go"})
	if err != nil {
		t.Fatal(err)
	}
	if note := strings.Join(pack.Dropped, "\n"); strings.Contains(note, "truncated") || strings.Contains(note, "skipped") {
		t.Fatalf("a ceiling nobody declared dropped context: %v", pack.Dropped)
	}
	if len(pack.Dependents) != 2101 {
		t.Fatalf("dependents = %d, want all 2101", len(pack.Dependents))
	}
	found := false
	for _, s := range pack.Snippets {
		if s.File == "use/large.txt" && strings.Contains(s.Text, long) {
			found = true
		}
	}
	if !found {
		t.Fatal("the reference past 256 KiB was not excerpted whole")
	}
}
