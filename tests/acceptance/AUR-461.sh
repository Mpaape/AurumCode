#!/usr/bin/env bash
#
# Acceptance program for card AUR-461, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The review prompt the model actually receives names EVERY rule id of
#   the embedded catalog. The check is deliberately independent of the Go
#   test: the expected ids are grepped out of
#   internal/review/rules/*.yml by this shell, and the actual prompt is
#   printed by a generated program that calls the real
#   prompt.PromptBuilder.BuildPrompt. Neither side can agree with itself
#   by construction.
#
#   Before this card, templates/review.md named one rule id, inside its
#   output example, and that example taught `quality/naming`, which the
#   catalog does not define. The 2026-08-14 gateway measurement: six
#   findings returned, five discarded for invented ids, one reached the
#   user -- with a real command injection among the losses, reported as
#   security/shell-injection where the catalog says
#   security/command-injection.
#
#   AC-002 (budget) is asserted here too: the assembled prompt must fit
#   its token budget with the full catalog, and an oversized catalog must
#   FAIL rather than render a shortened list -- a model citing a rule that
#   exists but was cut would have its finding discarded, which is the same
#   defect by another road.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001 mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-461'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR461|IntegrationAUR461|E2EAUR461|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

required_inputs=(
  go.mod
  go.sum
  internal/analyzer
  internal/llm
  internal/prompt
  internal/prompt/rulecatalog.go
  internal/prompt/templates/review.md
  internal/review
  internal/review/rules
  internal/security
  internal/git
  pkg/types
  tests/unit/AUR-461.go
  tests/integration/AUR-461.go
  tests/e2e/AUR-461.sh
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a461.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

# REGRAS INEGOCIAVEIS: bounded memory, offline module resolution, -p=1.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

run_go() { local dir="$1"; shift; ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" ); }

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" internal/analyzer internal/prompt internal/review internal/security internal/llm internal/git
  copy "$root" pkg/types
  chmod -R u+w -- "$root"
}

# write_prompt_dumper generates the observer: it prints the SYSTEM half of
# a real assembled review prompt, and exercises AC-002's two budget paths
# on its own exit code.
write_prompt_dumper() {
  local root="$1"
  mkdir -p "$root/cmd/aur461dump"
  cat >"$root/cmd/aur461dump/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func main() {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  "svc.py",
		Lang:  "python",
		Hunks: []types.DiffHunk{{Lines: []string{`+os.system("ls " + user_input)`}}},
	}}}
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, err := prompt.NewPromptBuilder().BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens: 8000, SchemaKind: "review", Role: "reviewer", ReserveReply: 1000,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "assembly failed with the shipped catalog: %v\n", err)
		os.Exit(2)
	}

	if len(os.Args) > 1 && os.Args[1] == "--budget" {
		// AC-002: an oversized catalog must be an ERROR, never a shorter
		// list. Both the validator and the assembly seam must refuse it.
		oversized := append([]string(nil), prompt.DefaultRuleCatalog...)
		for i := 0; i < 400; i++ {
			oversized = append(oversized, fmt.Sprintf("quality/synthetic-overflow-%04d-%s", i, strings.Repeat("x", 24)))
		}
		sort.Strings(oversized)
		if err := prompt.ValidateRuleCatalog(oversized, prompt.NewHeuristicEstimator()); err == nil {
			fmt.Fprintln(os.Stderr, "an over-budget catalog was accepted")
			os.Exit(3)
		}
		if err := prompt.NewPromptBuilder().SetRuleCatalog(oversized); err == nil {
			fmt.Fprintln(os.Stderr, "assembly accepted an over-budget catalog")
			os.Exit(3)
		}
		rendered := prompt.RenderRuleCatalog(oversized)
		for _, id := range oversized {
			if !strings.Contains(rendered, id) {
				fmt.Fprintf(os.Stderr, "the renderer dropped %s: truncation must be impossible\n", id)
				os.Exit(3)
			}
		}
		fmt.Println("budget-ok")
		return
	}

	fmt.Print(parts.System)
}
EOF
}

expected_ids() {
  grep -h -oE '^[[:space:]]*-[[:space:]]*id:[[:space:]]*[a-z]+/[a-z0-9-]+' "$repo_root"/internal/review/rules/*.yml \
    | sed -E 's/.*id:[[:space:]]*//' | sort -u
}

# assert_prompt_lists_catalog renders the prompt from $1 and requires every
# YAML id to appear in it. $2 is the failure label.
assert_prompt_lists_catalog() {
  local root="$1" label="$2" out="$root/prompt.txt"
  if ! run_go "$root" run ./cmd/aur461dump >"$out" 2>"$root/dump.err"; then
    cat "$root/dump.err" >&2
    if [[ "$label" == 'MUT-001' ]]; then return 1; fi
    infra dump_failed
  fi
  [[ -s "$out" ]] || infra empty_prompt
  grep -Fq '<no value>' "$out" && { printf 'unfilled template key in the rendered prompt\n' >&2; return 1; }

  local id missing=0
  while read -r id; do
    [[ -n "$id" ]] || continue
    grep -Fq "$id" "$out" || { printf 'prompt never names %s\n' "$id" >&2; missing=$((missing + 1)); }
  done < <(expected_ids)
  ((missing == 0)) || return 1
  return 0
}

nominal_case() {
  local root="$run_dir/root-nominal"
  stage_source "$root"
  write_prompt_dumper "$root"

  local count; count="$(expected_ids | wc -l)"
  ((count > 0)) || infra no_expected_ids

  assert_prompt_lists_catalog "$root" 'AC-001' || fail behavior-missing

  # The prompt must not offer an id the gate would reject: the ids it
  # names, backtick-quoted, are exactly the catalog's.
  local offered; offered="$(grep -oE '`[a-z]+/[a-z0-9-]+`' "$root/prompt.txt" | tr -d '`' | sort -u)"
  [[ "$offered" == "$(expected_ids)" ]] || { printf 'offered ids differ from the catalog\n' >&2; fail behavior-missing; }

  # The id the 2026-08-14 measurement lost the command injection under is
  # NOT offered; the catalog's own id is.
  grep -Fq 'security/command-injection' "$root/prompt.txt" || fail behavior-missing
  grep -Fq 'security/shell-injection' "$root/prompt.txt" && fail invented-id-offered
  grep -Fq 'quality/naming`' "$root/prompt.txt" && fail stale-example-id

  # AC-002: budget holds with the full catalog, and overflow fails high.
  run_go "$root" run ./cmd/aur461dump --budget >"$root/budget.out" 2>"$root/budget.err" \
    || { cat "$root/budget.err" >&2; fail AC-002; }
  grep -Fq 'budget-ok' "$root/budget.out" || fail AC-002
}

# MUT-001: remove the catalog injection from the template and require the
# nominal proof to go RED. A check that passes with the defect restored
# proves nothing.
mutation_case() {
  local root="$run_dir/root-mutant"
  stage_source "$root"
  write_prompt_dumper "$root"

  local target="$root/internal/prompt/templates/review.md"
  grep -Fq '{{.RuleCatalog}}' "$target" || infra mutation_anchor_missing
  local tmp="$root/review.mutated"
  sed 's/{{\.RuleCatalog}}//' "$target" >"$tmp" || infra mutation_rewrite
  mv "$tmp" "$target"
  grep -Fq '{{.RuleCatalog}}' "$target" && infra mutation_not_applied

  if assert_prompt_lists_catalog "$root" 'MUT-001'; then
    fail MUT-001
  fi
}

# bridge_run executes one of the card's Go programs. tests/unit/*.go and
# tests/integration/*.go are plain .go files, so `go test` alone would
# report "no test files" and pass vacuously; the generated bridge is this
# repo's convention for making the file actually execute, and the "ok"
# line is checked so a zero-test run cannot read as green.
bridge_run() {
  local pkg="$1" fn="$2" bridge="$3" label="$4"
  local root="$run_dir/root-$pkg"
  stage_source "$root"
  copy "$root" "tests/$pkg/AUR-461.go"
  chmod -R u+w -- "$root/tests"
  cat >"$root/tests/$pkg/aur461_bridge_test.go" <<EOF
package $pkg

import "testing"

func $bridge(t *testing.T) { $fn(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s "./tests/$pkg" -run "^$bridge\$" -count=1 2>&1)"
  rc=$?
  set -e
  ((rc == 0)) || { printf '%s\n' "$out" >&2; fail "selector:$label:exit:$rc"; }
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "selector:$label:zero-tests"
}

unit_case() { bridge_run unit TestAUR461 TestAUR461UnitBridge TestAUR461; }
integration_case() { bridge_run integration IntegrationAUR461 TestAUR461IntegrationBridge IntegrationAUR461; }

e2e_case() {
  [[ -f "$repo_root/tests/e2e/AUR-461.sh" ]] || infra "missing-input:tests/e2e/AUR-461.sh"
  set +e
  bash "$repo_root/tests/e2e/AUR-461.sh" E2EAUR461
  local rc=$?
  set -e
  ((rc == 0)) || exit "$rc"
}

# Each declared selector runs its OWN program with its OWN assertions. An
# unknown or unimplemented selector exits 64 -- absence reads as absence --
# rather than falling through to AC-001 and harvesting someone else's
# green.
case "$selector" in
  AC-001-MUT-001) mutation_case ;;
  TestAUR461) unit_case ;;
  IntegrationAUR461) integration_case ;;
  E2EAUR461) e2e_case ;;
  AC-001) nominal_case ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac
exit 0
