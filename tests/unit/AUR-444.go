// Package unit holds AUR-444's Unit-layer proof.
//
// THE DEFECT THIS PROVES FIXED
//
//	scripts/action-entrypoint.sh invoked "$AURUMCODE_CLI" review with flags
//	(--provider, --model, --api-key, --github-token, --post-comments) that
//	cmd/aurumcode's review command never registered. The binary would refuse
//	the parse: the Action publicised a code review it could never run.
//
// HOW THIS IS PROVED, AND WHY NOT BY grep-ING A HAND LIST
//
//	The card explicitly forbids a hand-written list of "the real flags":
//	that is exactly the kind of claim this card exists to correct (the
//	card's own "Achado medido" names a --check flag that a `go build` +
//	`review --help` against this repository's actual tip does NOT show --
//	see docs/specs/AUR-444.md). So the real, registered flag set is derived
//	here by parsing cmd/aurumcode's OWN source with go/parser: it walks the
//	function that calls flag.NewFlagSet("review", ...) and collects every
//	flag name registered on that flag.FlagSet (.String/.Bool/.Int/...).
//
//	go/parser only needs syntax, not type-checked imports, so this works
//	even when cmd/aurumcode's own internal/... dependencies are not
//	materialized on disk -- which is exactly the situation inside the sealed
//	`bootstrap-readonly-v1` acceptance sandbox: AUR-444's read_paths lists
//	cmd/aurumcode but not internal/analyzer, internal/llm, internal/review,
//	internal/security, internal/git, internal/prompt or pkg/types, so a
//	literal `go build ./cmd/aurumcode` cannot succeed there (confirmed by
//	staging only the declared read_paths and running the build: it fails on
//	"finding module for package .../internal/analyzer" et al., offline,
//	because GOPROXY is off and those packages simply are not on disk). This
//	card cannot repair read_paths -- .board/cards is a forbidden_path -- so
//	this Unit layer is deliberately import-free of cmd/aurumcode's own
//	package, proving the property from source text alone. tests/integration
//	and tests/e2e add build-based proof whenever that closure IS present
//	(a normal working tree, unlike the sealed sandbox).
//
// This file is not named "_test.go" on purpose, mirroring every sibling card
// in this office: tests/acceptance/AUR-444.sh stages a private writable copy
// and writes a tiny bridge "_test.go" file that calls TestAUR428, so this
// assertion runs inside the sandboxed acceptance instead of being swept into
// an unrelated top-level `go test ./...`.
//
// SELECTOR NAME: the card's own TDD proof section names this function
// TestAUR428 -- inherited verbatim from the AUR-428 card template it was
// copied from (a numbering mismatch the acceptance program also notes). It
// is kept exactly as the card declares so a reviewer running the literal
// selector the card names gets a real, executing test.
package unit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// aur444RepoRoot resolves the repository root: the acceptance script exports
// AURUMCODE_ROOT pointing at its staged copy; a developer running the bridge
// manually gets a walk-up from the working directory to the nearest go.mod.
func aur444RepoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-444/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-444/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

// aur444FlagRegistrationMethods are the flag.FlagSet methods that register a
// named flag. This is Go's own standard-library API surface, not
// project-specific knowledge: no cmd/aurumcode FLAG NAME is ever hand-listed
// anywhere in this file, only the fixed method names the flag package
// itself publishes.
var aur444FlagRegistrationMethods = map[string]bool{
	"String": true, "Bool": true, "Int": true, "Int64": true,
	"Uint": true, "Uint64": true, "Float64": true, "Duration": true, "Var": true,
}

// aur444RegisteredFlags parses every non-test .go file directly under dir
// and returns the set of flag names registered on the flag.FlagSet built by
// flag.NewFlagSet(subcommand, ...) inside whichever function creates it.
func aur444RegisteredFlags(t *testing.T, dir, subcommand string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("AUR-444/AC-001/infrastructure: reading %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	flags := map[string]bool{}
	found := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatalf("AUR-444/AC-001/infrastructure: parsing %s: %v", path, perr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			varName, hasSet := aur444FindFlagSetVar(fn.Body, subcommand)
			if !hasSet {
				continue
			}
			found = true
			for _, flagName := range aur444CollectFlagNames(fn.Body, varName) {
				flags[flagName] = true
			}
		}
	}
	if !found {
		t.Fatalf("AUR-444/AC-001/infrastructure: no flag.NewFlagSet(%q, ...) found under %s", subcommand, dir)
	}
	return flags
}

// aur444FindFlagSetVar looks for `<ident> := flag.NewFlagSet("<subcommand>", ...)`
// anywhere in body and returns the identifier the flag.FlagSet was assigned
// to.
func aur444FindFlagSetVar(body *ast.BlockStmt, subcommand string) (string, bool) {
	var varName string
	var found bool
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) == 0 || len(assign.Rhs) == 0 {
			return true
		}
		call, isCall := assign.Rhs[0].(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "NewFlagSet" {
			return true
		}
		pkgIdent, isIdent := sel.X.(*ast.Ident)
		if !isIdent || pkgIdent.Name != "flag" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, isLit := call.Args[0].(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return true
		}
		value, uerr := strconv.Unquote(lit.Value)
		if uerr != nil || value != subcommand {
			return true
		}
		lhsIdent, isIdent2 := assign.Lhs[0].(*ast.Ident)
		if !isIdent2 {
			return true
		}
		varName = lhsIdent.Name
		found = true
		return false
	})
	return varName, found
}

// aur444CollectFlagNames returns every flag name registered on <varName>.
func aur444CollectFlagNames(body *ast.BlockStmt, varName string) []string {
	var names []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || !aur444FlagRegistrationMethods[sel.Sel.Name] {
			return true
		}
		recv, isIdent := sel.X.(*ast.Ident)
		if !isIdent || recv.Name != varName {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, isLit := call.Args[0].(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return true
		}
		value, uerr := strconv.Unquote(lit.Value)
		if uerr != nil {
			return true
		}
		names = append(names, value)
		return true
	})
	return names
}

// aur444InvocationRE finds an entrypoint line invoking $AURUMCODE_CLI (in
// either the `"$AURUMCODE_CLI"` or `${AURUMCODE_CLI}` shell spelling) with a
// subcommand name immediately after it.
var aur444InvocationRE = regexp.MustCompile(`\$(?:\{AURUMCODE_CLI\}|AURUMCODE_CLI)"?\s+(review|docs)\b`)

// aur444FlagTokenRE finds every long-flag token (--name) in a shell command.
var aur444FlagTokenRE = regexp.MustCompile(`--([A-Za-z][A-Za-z0-9-]*)`)

// aur444EntrypointFlagUses scans path (scripts/action-entrypoint.sh) and
// returns, for every subcommand it invokes $AURUMCODE_CLI with, the set of
// long-flag tokens (without the leading --) that invocation's shell command
// -- including every backslash-continued line -- actually passes. A
// commented-out line is never mistaken for a live invocation.
func aur444EntrypointFlagUses(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-444/AC-001/infrastructure: reading %s: %v", path, err)
	}
	lines := strings.Split(string(raw), "\n")
	uses := map[string]map[string]bool{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		m := aur444InvocationRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		subcommand := m[1]
		block := line
		for strings.HasSuffix(block, `\`) && i+1 < len(lines) {
			i++
			block = strings.TrimSuffix(block, `\`) + "\n" + lines[i]
		}
		set := uses[subcommand]
		if set == nil {
			set = map[string]bool{}
			uses[subcommand] = set
		}
		for _, fm := range aur444FlagTokenRE.FindAllStringSubmatch(block, -1) {
			set[fm[1]] = true
		}
	}
	return uses
}

func aur444SortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestAUR428 proves criterion (a) of AUR-444's acceptance: every flag token
// the entrypoint sends to `"$AURUMCODE_CLI" review` is a flag cmd/aurumcode
// actually registers, dynamically derived from cmd/aurumcode's own source --
// never a hand-written list -- and at least --base (the one required real
// flag) is among them.
func TestAUR428(t *testing.T) {
	root := aur444RepoRoot(t)
	cliDir := filepath.Join(root, "cmd", "aurumcode")
	entrypoint := filepath.Join(root, "scripts", "action-entrypoint.sh")

	realFlags := aur444RegisteredFlags(t, cliDir, "review")
	if len(realFlags) == 0 {
		t.Fatalf("AUR-444/AC-001/infrastructure: zero flags parsed from cmd/aurumcode's review flag.NewFlagSet")
	}

	uses := aur444EntrypointFlagUses(t, entrypoint)
	reviewUses, hasReview := uses["review"]
	if !hasReview || len(reviewUses) == 0 {
		t.Fatalf("AUR-444/AC-001/behavior-missing: scripts/action-entrypoint.sh sends no review invocation with any flag token (real flags: %s)",
			strings.Join(aur444SortedKeys(realFlags), ", "))
	}
	if !reviewUses["base"] {
		t.Fatalf("AUR-444/AC-001/behavior-missing: the review invocation never sends --base, the one required real flag (sent: %s)",
			strings.Join(aur444SortedKeys(reviewUses), ", "))
	}

	var unknown []string
	for flagName := range reviewUses {
		if !realFlags[flagName] {
			unknown = append(unknown, flagName)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		t.Fatalf("AUR-444/AC-001/behavior-missing: entrypoint sends flag(s) cmd/aurumcode does not register: --%s (registered: %s)",
			strings.Join(unknown, ", --"), strings.Join(aur444SortedKeys(realFlags), ", "))
	}

	// docs is not invoked by the entrypoint today (run_documentation calls
	// the regenerate-docs binary directly, never "$AURUMCODE_CLI" docs), so
	// there is nothing to confront for it; asserting that absence here pins
	// the scope so a future docs invocation cannot slip past this proof
	// unnoticed.
	if _, hasDocs := uses["docs"]; hasDocs {
		docsFlags := aur444RegisteredFlags(t, cliDir, "docs")
		for flagName := range uses["docs"] {
			if !docsFlags[flagName] {
				t.Fatalf("AUR-444/AC-001/behavior-missing: entrypoint sends flag(s) cmd/aurumcode docs does not register: --%s", flagName)
			}
		}
	}
}
