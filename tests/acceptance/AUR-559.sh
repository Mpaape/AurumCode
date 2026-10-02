#!/usr/bin/env bash
# AUR-559 acceptance: prompt and skills carry no language name in production
# Go; comment lines come from the grammar, aliases from a data catalog.
#
# Selectors:
#   all              AC-001..AC-004, then the mutation
#   AC-001           no language-name string literal in internal/prompt/*.go or
#                    internal/context/skills/*.go outside _test.go (go/ast)
#   AC-002           comment line recognised by the grammar in Go, Python, Ruby,
#                    Java, Terraform and Dockerfile; the same text as code is not
#   AC-003           no grammar: no comment filter and a notice in the context
#   AC-004           alias resolves through the data catalog; unknown alias is
#                    declared; policy override and strict validation
#   AC-003-MUT-001   defaulting to `//` without a notice turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-559'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-003-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
for f in internal/grammar/aur559_test.go internal/prompt/aur559_test.go internal/context/skills/aur559_test.go \
         internal/grammar/catalog/aliases.yml internal/prompt/commentfilter.go; do
  [[ -f "$repo_root/$f" ]] || infra "missing-source:$f"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a559.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/root"
for source in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$source" "$run_dir/root/$source"; done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

run_go_test() { # pkg pattern log
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 400s -v "$1" -run "$2") >"$3" 2>&1
  local status=$?
  set -e
  cat "$3" >&2
  return $status
}
need_pass() { grep -q "^--- PASS: $2 " "$1" || fail "missing-pass:$2"; }

# The language names AC-001 forbids as string literals. This list is DATA of
# the acceptance, the only place a language name may be spelled out.
forbidden_names='go golang python py shell bash sh zsh ruby rb java javascript js typescript ts tsx rust kotlin c cpp csharp c_sharp terraform hcl dockerfile yaml yml json markdown md html sql php swift scala perl lua'

run_ac001() {
  local dir="$run_dir/root/zz_ac001"
  mkdir -p "$dir"
  cat >"$dir/main.go" <<'GO'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	forbidden := map[string]bool{}
	for _, n := range strings.Fields(os.Args[1]) {
		forbidden[n] = true
	}
	bad := 0
	for _, dir := range os.Args[2:] {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		if len(files) == 0 {
			fmt.Println("no go files in", dir)
			os.Exit(2)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			tree, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				fmt.Println(err)
				os.Exit(2)
			}
			ast.Inspect(tree, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				v, _ := strconv.Unquote(lit.Value)
				if forbidden[strings.ToLower(strings.TrimSpace(v))] {
					fmt.Printf("%s: language-name literal %s\n", fset.Position(lit.Pos()), lit.Value)
					bad++
				}
				return true
			})
		}
	}
	if bad > 0 {
		os.Exit(1)
	}
}
GO
  set +e
  (cd "$run_dir/root" && go run ./zz_ac001 "$forbidden_names" internal/prompt internal/context/skills) >"$run_dir/ac001.log" 2>&1
  local status=$?
  set -e
  cat "$run_dir/ac001.log" >&2
  [[ $status -eq 0 ]] || fail 'language-literal-found'
}
run_ac002() {
  local log="$run_dir/ac002.log"
  run_go_test ./internal/grammar/ '^(TestAUR559IsCommentByGrammar)$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR559IsCommentByGrammar
  run_go_test ./internal/prompt/ '^(TestAUR559CommentOnlyChangeIsNotSubstantive)$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR559CommentOnlyChangeIsNotSubstantive
}
run_ac003() {
  local log="$run_dir/ac003.log"
  run_go_test ./internal/prompt/ '^TestAUR559NoGrammarNoFilterAndNotice$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR559NoGrammarNoFilterAndNotice
}
run_ac004() {
  local log="$run_dir/ac004.log"
  run_go_test ./internal/context/skills/ '^TestAUR559AliasResolvesAndUnknownIsDeclared$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR559AliasResolvesAndUnknownIsDeclared
  run_go_test ./internal/grammar/ '^TestAUR559(Aliases|Policy|AliasToMissing)' "$log" || fail 'go-test-failed'
  for n in TestAUR559AliasesEmbeddedAreValid TestAUR559PolicyAliasesReplaceSection TestAUR559AliasToMissingGrammarIsLoadError; do
    need_pass "$log" "$n"
  done
}
run_mutation() {
  local target="$run_dir/root/internal/prompt/commentfilter.go"
  local anchor1='return ok && isComment'
  local anchor2='if len(names) == 0 {'
  [[ "$(grep -Fc "$anchor1" "$target")" == "1" && "$(grep -Fc "$anchor2" "$target")" == "1" ]] || infra mutation-anchor
  sed -i 's|return ok && isComment|if !ok { return strings.HasPrefix(line, "//") }; return isComment // MUT-001|' "$target"
  sed -i 's|if len(names) == 0 {|if true { // MUT-001|' "$target"
  grep -Fq 'MUT-001' "$target" || infra mutation-not-applied
  local log="$run_dir/mutation.log"
  run_go_test ./internal/prompt/ '^TestAUR559NoGrammarNoFilterAndNotice$' "$log" || true
  grep -Eq 'build failed|undefined:|syntax error|declared and not used' "$log" && fail 'mutation-build-failure-not-behavioral'
  grep -q '^--- FAIL: TestAUR559NoGrammarNoFilterAndNotice' "$log" || fail 'mutation-survived'
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  AC-003-MUT-001) run_mutation ;;
  all) run_ac001; run_ac002; run_ac003; run_ac004; run_mutation ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
