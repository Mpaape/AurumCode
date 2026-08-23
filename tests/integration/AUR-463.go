// Package integration holds AUR-463's Integration-layer proof: extension
// parity (AC-002). A directory holding .js, .jsx, .mjs and .cjs files each
// with a real exported, documented symbol produces one page per file, and a
// file with no exported symbol produces no empty page, all from a single
// NativeExtractor.Extract call over the whole directory (not one file at a
// time, the way tests/unit/AUR-463.go exercises it).
//
// Not named "_test.go" on purpose; see tests/unit/AUR-463.go's package doc
// for why. tests/acceptance/AUR-463.sh writes a bridge _test.go file that
// calls IntegrationAUR463.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	javascript "github.com/Mpaape/AurumCode/internal/documentation/extractors/javascript"
)

// aur463ExtensionFixtures maps each parity extension to source that carries
// exactly one recognized, documented export, plus one file (no extension
// requirement, .js is enough) that carries none.
var aur463ExtensionFixtures = map[string]string{
	"a.js": `/**
 * A documented JS function.
 */
export function jsFn(x) {
  return x;
}
`,
	"b.jsx": `/**
 * A documented JSX-adjacent function.
 */
export function jsxFn(x) {
  return x;
}
`,
	"c.mjs": `/**
 * A documented ESM function.
 */
export function mjsFn(x) {
  return x;
}
`,
	"d.cjs": `/**
 * A documented CommonJS-flavored function.
 */
export function cjsFn(x) {
  return x;
}
`,
	"e_empty.js": `// nothing exported
function helper() { return 1; }
`,
}

// IntegrationAUR463 is the Integration-layer proof.
func IntegrationAUR463(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	for name, content := range aur463ExtensionFixtures {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	n := javascript.NewNativeExtractor()
	res, err := n.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageJavaScript,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("Extract reported errors: %v", res.Errors)
	}

	// Exactly the four extensions with real exports produced a page; the
	// empty file did not add a fifth.
	if len(res.Files) != 4 {
		t.Fatalf("DocsGenerated files = %v (%d), want exactly 4 (one per "+
			".js/.jsx/.mjs/.cjs file with an export, zero for the empty one)",
			res.Files, len(res.Files))
	}

	wantSymbols := []string{"jsFn(x)", "jsxFn(x)", "mjsFn(x)", "cjsFn(x)"}
	var allContent strings.Builder
	for _, f := range res.Files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		allContent.Write(data)
		allContent.WriteByte('\n')
	}
	combined := allContent.String()

	sort.Strings(wantSymbols)
	for _, sym := range wantSymbols {
		if !strings.Contains(combined, sym) {
			t.Errorf("generated pages missing symbol %q for one of the four parity "+
				"extensions; combined output:\n%s", sym, combined)
		}
	}

	if strings.Contains(combined, "helper") {
		t.Errorf("the file with no exported symbol leaked its internal, non-exported "+
			"helper into a generated page:\n%s", combined)
	}
}
