package grammar

// This file is the one adapter to the tree-sitter runtime
// (github.com/odvcencio/gotreesitter, pure Go, no cgo). Replacing the runtime
// means replacing this file with another Provider; no consumer changes.

import (
	"bytes"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Runtime is the Provider backed by the tree-sitter grammar runtime.
type Runtime struct{}

// Languages enumerates, at run time, every grammar the linked runtime offers.
// The list is data from the runtime, never a literal in AurumCode.
func (Runtime) Languages() []string {
	all := grammars.AllLanguages()
	names := make([]string, 0, len(all))
	for _, e := range all {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names
}

// Detect resolves the grammar for a file from its name and, failing that, its
// first line (shebang), both through the runtime's own detection. It returns
// "" when the runtime has no grammar for the file.
func (Runtime) Detect(name string, content []byte) string {
	if e := grammars.DetectLanguage(name); e != nil {
		return e.Name
	}
	first := content
	if i := bytes.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	if len(first) > 256 {
		first = first[:256]
	}
	if bytes.HasPrefix(first, []byte("#!")) {
		if e := grammars.DetectLanguageByShebang(string(first)); e != nil {
			return e.Name
		}
	}
	return ""
}

// Analyze extracts structure from content using the grammar the runtime
// selects for name. It never panics and never fails: any problem becomes a
// Structure with HasStructure false and a Reason.
func (r Runtime) Analyze(name string, content []byte) (s Structure) {
	s.Language = NoStructure
	defer func() {
		if r := recover(); r != nil {
			s = Structure{Language: s.Language, Reason: "grammar runtime failed on this file"}
		}
	}()
	langName := r.Detect(name, content)
	if langName == "" {
		s.Reason = "no grammar for this file in the runtime"
		return s
	}
	entry := grammars.DetectLanguageByName(langName)
	if entry == nil || entry.Language == nil {
		s.Reason = "no grammar for this file in the runtime"
		return s
	}
	s.Language = entry.Name
	if len(content) > MaxParseBytes {
		content = content[:MaxParseBytes]
	}
	lang := entry.Language()
	if lang == nil {
		s.Reason = "grammar failed to load"
		return s
	}
	tree, err := gotreesitter.NewParser(lang).Parse(content)
	if err != nil || tree == nil {
		s.Reason = "grammar could not parse the file"
		return s
	}
	s.HasStructure = true
	if q := grammars.ResolveTagsQuery(*entry); strings.TrimSpace(q) != "" {
		if o, oerr := gotreesitter.NewOutliner(lang, q); oerr == nil {
			syms, _ := o.OutlineTree(tree)
			s.Symbols = flatten(syms, nil)
		}
	}
	s.Symbols = unionSorted(s.Symbols, declaredNames(tree.RootNode(), lang, content))
	for _, imp := range gotreesitter.ExtractImports(tree) {
		if imp.Path != "" {
			s.Imports = append(s.Imports, imp.Path)
		}
	}
	return s
}

func flatten(syms []gotreesitter.OutlineSymbol, out []string) []string {
	for _, sym := range syms {
		if sym.Name != "" {
			out = append(out, sym.Name)
		}
		out = flatten(sym.Children, out)
	}
	return out
}

// excludedDeclarationParts name tree-sitter node-type fragments that denote
// something other than a reusable declaration. They are grammar vocabulary
// shared by every grammar (blocks, parameters, call arguments, struct fields),
// not language names.
var excludedDeclarationParts = []string{"block", "param", "argument", "field", "call", "body_statement"}

// declaredNames walks the tree and collects the text of every `name` field of
// a declaration-shaped node outside executable blocks. The tags query of a
// grammar may not cover every declaration kind (types, constants); the `name`
// field is the grammar-neutral convention for the declared identifier.
func declaredNames(root *gotreesitter.Node, lang *gotreesitter.Language, src []byte) []string {
	var out []string
	var walk func(n *gotreesitter.Node, depth int)
	walk = func(n *gotreesitter.Node, depth int) {
		if n == nil || depth > 64 {
			return
		}
		typ := n.Type(lang)
		for _, part := range excludedDeclarationParts {
			if strings.Contains(typ, part) {
				return
			}
		}
		if name := n.ChildByFieldName("name", lang); name != nil && n.IsNamed() {
			if t := strings.TrimSpace(name.Text(src)); t != "" && !strings.ContainsAny(t, " \t\r\n") && len(t) <= 128 {
				out = append(out, t)
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i), depth+1)
		}
	}
	walk(root, 0)
	return out
}

func unionSorted(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	var out []string
	for _, list := range [][]string{a, b} {
		for _, v := range list {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
