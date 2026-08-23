// Package unit holds AUR-463's Unit-layer proof: the native, tool-free
// documentation extractor for JavaScript
// (internal/documentation/extractors/javascript's NewNativeExtractor) reads
// real JSDoc comments out of real ESM source text, renders real Markdown
// pages from them, starts no subprocess to do it, and never synthesizes
// prose for a symbol that carries no JSDoc.
//
// This file is not named "_test.go" on purpose, mirroring AUR-427's
// tests/unit/AUR-427.go: tests/acceptance/AUR-463.sh stages a private
// writable copy of the module and writes a tiny bridge "_test.go" file that
// calls TestAUR463, so the assertions below run inside the sandboxed
// acceptance instead of being swept into an unrelated top-level
// `go test ./...`.
package unit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	javascript "github.com/Mpaape/AurumCode/internal/documentation/extractors/javascript"
)

const jsFixtureAUR463 = `/**
 * Adds two numbers together.
 */
export function add(a, b) {
  return a + b;
}

export function noDoc(x) {
  return x;
}

/**
 * A greeter class.
 */
export class Greeter {
  /**
   * Creates a greeter.
   */
  constructor(name) {
    this.name = name;
  }

  greet() {
    return ` + "`hello ${this.name}`" + `;
  }
}

/**
 * Multiplies two numbers.
 */
export const multiply = (a, b) => a * b;

/**
 * Default export: the module's entry point.
 */
export default function main() {
  return 0;
}
`

// TestAUR463 is the Unit-layer proof. It is called by a bridge _test.go file
// tests/acceptance/AUR-463.sh writes into a staged, writable copy of the
// module (see that script for why: a bare top-level `go test ./...` must not
// pick this file up, since it is not named "_test.go").
func TestAUR463(t *testing.T) {
	t.Run("Validate never depends on an external tool", func(t *testing.T) {
		n := javascript.NewNativeExtractor()
		if err := n.Validate(context.Background()); err != nil {
			t.Fatalf("NativeExtractor.Validate must always succeed, got: %v", err)
		}
		if n.Language() != extractors.LanguageJavaScript {
			t.Fatalf("Language() = %q, want %q", n.Language(), extractors.LanguageJavaScript)
		}
	})

	t.Run("documented and undocumented exports, JSDoc attachment, no synthesized prose", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "src")
		outDir := filepath.Join(dir, "out")
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "sample.mjs"), []byte(jsFixtureAUR463), 0644); err != nil {
			t.Fatal(err)
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
		if len(res.Files) != 1 {
			t.Fatalf("Files = %v, want exactly one generated page", res.Files)
		}

		data, err := os.ReadFile(res.Files[0])
		if err != nil {
			t.Fatal(err)
		}
		page := string(data)

		// export function with JSDoc: name, signature, and prose all present.
		if !strings.Contains(page, "add(a, b)") || !strings.Contains(page, "Adds two numbers together.") {
			t.Errorf("documented function missing signature or JSDoc prose:\n%s", page)
		}

		// export function WITHOUT JSDoc: signature present, and the REGRA QUE
		// NAO PODE CAIR - no synthesized prose anywhere near it. Since this
		// parser never invents text, the only content between this symbol's
		// heading and its code fence must be the fence itself.
		idx := strings.Index(page, "noDoc(x)")
		if idx == -1 {
			t.Fatalf("undocumented function noDoc missing its signature entirely:\n%s", page)
		}
		headingEnd := strings.Index(page[idx:], "\n\n")
		if headingEnd == -1 {
			t.Fatalf("could not locate heading boundary for noDoc:\n%s", page)
		}
		afterHeading := page[idx+headingEnd+2:]
		if !strings.HasPrefix(strings.TrimLeft(afterHeading, "\n"), "```javascript") {
			t.Errorf("undocumented symbol noDoc must be followed directly by its code fence "+
				"(signature, no prose); got:\n%.120s", afterHeading)
		}

		// export class + its method, each with its own JSDoc.
		if !strings.Contains(page, "Greeter") || !strings.Contains(page, "A greeter class.") {
			t.Errorf("class Greeter missing signature or JSDoc:\n%s", page)
		}
		if !strings.Contains(page, "constructor(name)") || !strings.Contains(page, "Creates a greeter.") {
			t.Errorf("method constructor missing signature or JSDoc:\n%s", page)
		}
		// greet() carries no JSDoc: same no-prose rule as noDoc above.
		gidx := strings.Index(page, "greet()")
		if gidx == -1 {
			t.Fatalf("method greet missing entirely:\n%s", page)
		}

		// export const assigned to an arrow function.
		if !strings.Contains(page, "multiply") || !strings.Contains(page, "Multiplies two numbers.") {
			t.Errorf("const multiply missing signature or JSDoc:\n%s", page)
		}

		// export default function.
		if !strings.Contains(page, "main()") || !strings.Contains(page, "the module's entry point") {
			t.Errorf("default export main missing signature or JSDoc:\n%s", page)
		}
	})

	t.Run("a file with no exported symbol produces no document", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "src")
		outDir := filepath.Join(dir, "out")
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			t.Fatal(err)
		}
		empty := "// internal helper, nothing exported\nfunction helper() { return 1; }\nconst x = helper();\n"
		if err := os.WriteFile(filepath.Join(srcDir, "internal.js"), []byte(empty), 0644); err != nil {
			t.Fatal(err)
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
		if len(res.Files) != 0 {
			t.Fatalf("Files = %v, want zero: a file with no exported symbol must not "+
				"produce an empty document", res.Files)
		}
	})
}
