// Package grammar is AurumCode's only source of per-language structure. It
// holds no language table, no extension list and no regex per language: the
// set of languages, file-name and shebang detection, and symbol and import
// extraction all come from a Provider, whose default is the generic,
// community-maintained tree-sitter grammars of a pure-Go runtime. A language
// the provider does not know is reported as having no structural context, and
// the reviewing model reads its text directly.
package grammar

// NoStructure is the language name reported when the provider has no grammar
// for a file.
const NoStructure = "unknown"

// MaxParseBytes bounds how much of one file is handed to a parser.
const MaxParseBytes = 1 << 20

// Provider is the small seam consumers depend on. Consumers receive a Provider
// by injection and never import a runtime.
type Provider interface {
	// Languages enumerates, at run time, the grammars the provider offers.
	Languages() []string
	// Detect names the grammar for a file from its name and, failing that, its
	// content (shebang); "" when there is none.
	Detect(name string, content []byte) string
	// Analyze extracts structure. It never fails: a problem becomes a
	// Structure with HasStructure false and a Reason.
	Analyze(name string, content []byte) Structure
}

// Default returns the provider backed by the linked tree-sitter runtime.
func Default() Provider { return Runtime{} }

// Structure is the structural context extracted from one file.
type Structure struct {
	// Language is the runtime grammar name, or NoStructure.
	Language string
	// HasStructure reports whether the file parsed with a grammar. When false
	// the review proceeds on the text alone and must say so.
	HasStructure bool
	// Symbols are the definition names found by the grammar's tags query.
	Symbols []string
	// Imports are the dependency paths found by the runtime's extractor.
	Imports []string
	// Reason explains HasStructure == false.
	Reason string
}
