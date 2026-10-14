package grammar

import (
	"slices"
	"strings"
	"testing"
)

// Issue #179: a file past the old 1 MiB parse ceiling is parsed whole, so a
// symbol declared after the first MiB is still part of its structure.
func TestFilePastTheOldParseCeilingIsParsedWhole(t *testing.T) {
	var b strings.Builder
	b.WriteString("package big\n\n")
	for b.Len() <= 1<<20 {
		b.WriteString("// " + strings.Repeat("x", 1000) + "\n")
	}
	b.WriteString("func DeclaredPastTheFirstMiB() {}\n")
	s := Default().Analyze("big.go", []byte(b.String()))
	if !s.HasStructure || !slices.Contains(s.Symbols, "DeclaredPastTheFirstMiB") {
		t.Fatalf("structure of a %d-byte file: has=%v reason=%q symbols=%v", b.Len(), s.HasStructure, s.Reason, s.Symbols)
	}
}
