#!/usr/bin/env bash
#
# Acceptance program for card AUR-467.
#
# WHAT THIS PROVES
#
#   The review prompt assembled by internal/prompt.PromptBuilder.BuildPrompt
#   -- the exact function internal/review/reviewer.go calls for the real
#   gateway -- never carries a documentation file's diff content into the
#   code-rule-catalog section (AC-001), still carries every code file's
#   content at a budget with room (AC-002), and declares, by name and count,
#   any code file a tight budget could not fit (AC-003).
#
#   Checked at prompt assembly, not through a canned-fixture LLM response:
#   the 2026-08-14 defect is entirely in what gets SENT to the model, so a
#   stub that returns fixed JSON regardless of the prompt cannot demonstrate
#   it either way. This program builds a tiny generated Go program
#   ("aur467dump") that calls the real PromptBuilder.BuildPrompt against a
#   synthetic diff shaped like the measured commit, and greps its output --
#   no git, no python3, nothing beyond bash and the Go toolchain, so this
#   runs unmodified in the sealed bootstrap-readonly-v1 image.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-467'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR467|IntegrationAUR467|E2EAUR467|AC-001-MUT-001|AC-003-MUT-002) ;;
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
  internal/prompt
  internal/prompt/coverage.go
  internal/prompt/filetype.go
  pkg/types
  tests/unit/AUR-467.go
  tests/integration/AUR-467.go
  tests/e2e/AUR-467.sh
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a467.XXXXXX")" || infra mktemp
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
  copy "$root" internal/analyzer internal/prompt
  copy "$root" pkg/types
  chmod -R u+w -- "$root"
}

# write_dumper generates the observer: it prints the USER half of a real
# assembled review prompt (which carries both the reviewed content and
# AC-003's coverage declaration) followed by a `META key=value` line per
# PromptParts.Meta entry, for a diff mixing one prose file (AGENTS.md,
# shaped like the 2026-08-14 measurement) with code files.
write_dumper() {
  local root="$1"
  mkdir -p "$root/cmd/aur467dump"
  cat >"$root/cmd/aur467dump/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func codeFile(path string, lines ...string) types.DiffFile {
	return types.DiffFile{Path: path, Hunks: []types.DiffHunk{{Lines: lines}}}
}

func main() {
	mode := "nominal"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	var diff *types.Diff
	maxTokens, reserve := 8000, 1000

	switch mode {
	case "nominal":
		files := []types.DiffFile{
			codeFile("AGENTS.md", "+PROSE_MARKER_115", "+'o que NAO fazer' contem instrucoes redundantes"),
		}
		for i := 0; i < 15; i++ {
			files = append(files, codeFile(fmt.Sprintf("src/mod%02d.mjs", i),
				fmt.Sprintf("+export const CODE_MARKER_%02d = true;", i)))
		}
		diff = &types.Diff{Files: files}
	case "tight":
		var files []types.DiffFile
		files = append(files, codeFile("README.md", "+prose"))
		for i := 0; i < 8; i++ {
			files = append(files, codeFile(fmt.Sprintf("src/big%02d.mjs", i), "+"+repeat("x", 400)))
		}
		diff = &types.Diff{Files: files}
		maxTokens, reserve = 1700, 40
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", mode)
		os.Exit(2)
	}

	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, err := prompt.NewPromptBuilder().BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens: maxTokens, SchemaKind: "review", Role: "reviewer", ReserveReply: reserve,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "assembly failed: %v\n", err)
		os.Exit(3)
	}

	fmt.Print(parts.User)
	keys := make([]string, 0, len(parts.Meta))
	for k := range parts.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("META %s=%s\n", k, parts.Meta[k])
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
EOF
}

# assert_nominal renders mode "nominal" from $1 and checks AC-001 + AC-002 +
# the prose-exclusion declaration (AC-003's other half). $2 labels failure.
assert_nominal() {
  local root="$1" label="$2" out="$root/nominal.out"
  if ! run_go "$root" run ./cmd/aur467dump nominal >"$out" 2>"$root/nominal.err"; then
    cat "$root/nominal.err" >&2
    if [[ "$label" == 'MUT-001' ]]; then return 1; fi
    infra dump_failed
  fi
  [[ -s "$out" ]] || infra empty_output

  # AC-001: the prose file's content never reaches the reviewed section.
  if grep -Fq 'PROSE_MARKER_115' "$out"; then return 1; fi
  # AC-001/Non-goal #3: the exclusion is declared, not silent.
  grep -Fq 'AGENTS.md' "$out" || return 1
  # AC-002: all 15 reconstructed code files still reach the review.
  local i missing=0
  for i in $(seq -w 0 14); do
    grep -Fq "CODE_MARKER_$i" "$out" || missing=$((missing + 1))
  done
  ((missing == 0)) || return 1
  grep -Fq 'META prose_files_excluded=1' "$out" || return 1
  grep -Fq 'META code_files_total=15' "$out" || return 1
  grep -Fq 'META code_files_complete=15' "$out" || return 1
  return 0
}

# assert_tight renders mode "tight" and checks AC-003: a budget too small
# for every code file declares how many, and which, were left out.
assert_tight() {
  local root="$1" label="$2" out="$root/tight.out"
  if ! run_go "$root" run ./cmd/aur467dump tight >"$out" 2>"$root/tight.err"; then
    cat "$root/tight.err" >&2
    if [[ "$label" == 'MUT-002' ]]; then return 1; fi
    infra dump_failed
  fi
  [[ -s "$out" ]] || infra empty_output

  local omitted; omitted="$(grep -oE 'META code_files_omitted=[0-9]+' "$out" | grep -oE '[0-9]+$')" || true
  [[ -n "$omitted" ]] || return 1
  ((omitted > 0)) || infra tight_budget_did_not_omit_anything
  grep -Fq "Code files NOT reviewed by this review (token budget): $omitted" "$out" || return 1
  return 0
}

nominal_case() {
  local root="$run_dir/root-nominal"
  stage_source "$root"
  write_dumper "$root"
  assert_nominal "$root" 'AC-001' || fail behavior-missing
  assert_tight "$root" 'AC-003' || fail behavior-missing
}

# MUT-001: revert the prose exclusion (feed prose segments back into the
# code-reviewed pool) by neutralizing the IsProse guard in builder.go. The
# nominal proof must go RED.
mutation_case_001() {
  local root="$run_dir/root-mut1"
  stage_source "$root"
  write_dumper "$root"

  local target="$root/internal/prompt/builder.go"
  local anchor='if !s.IsProse {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-001/anchor-not-unique'
  local tmp="$root/builder.mutated"
  sed "s/${anchor//\//\\/}/if true { \/\/ MUT-001: prose guard suppressed/" "$target" >"$tmp" || infra MUT-001-rewrite
  mv "$tmp" "$target"
  grep -Fq 'MUT-001: prose guard suppressed' "$target" || infra MUT-001-not-applied

  if assert_nominal "$root" 'MUT-001'; then
    fail MUT-001
  fi
}

# MUT-002: suppress the coverage declaration's own text so the omitted
# count is computed but never printed. AC-003's proof must go RED.
mutation_case_002() {
  local root="$run_dir/root-mut2"
  stage_source "$root"
  write_dumper "$root"

  local target="$root/internal/prompt/coverage.go"
  local anchor='sb.WriteString(fmt.Sprintf("- Code files NOT reviewed by this review (token budget): %d\n", len(omitted)))'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-002/anchor-not-unique'
  local replacement='// MUT-002: coverage line suppressed'
  local tmp="$root/coverage.mutated"
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    {
      idx = index($0, anchor)
      if (idx > 0) {
        print substr($0, 1, idx - 1) repl
      } else {
        print $0
      }
    }
  ' "$target" >"$tmp" && mv "$tmp" "$target"
  grep -Fq 'MUT-002: coverage line suppressed' "$target" || infra MUT-002-not-applied
  [[ "$(grep -Fc "$anchor" "$target")" == 0 ]] || infra MUT-002-not-applied

  if assert_tight "$root" 'MUT-002'; then
    fail MUT-002
  fi
}

bridge_run() {
  local pkg="$1" fn="$2" bridge="$3" label="$4"
  local root="$run_dir/root-$pkg"
  stage_source "$root"
  copy "$root" "tests/$pkg/AUR-467.go"
  chmod -R u+w -- "$root/tests"
  cat >"$root/tests/$pkg/aur467_bridge_test.go" <<EOF
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

unit_case() { bridge_run unit TestAUR467 TestAUR467UnitBridge TestAUR467; }
integration_case() { bridge_run integration IntegrationAUR467 TestAUR467IntegrationBridge IntegrationAUR467; }

e2e_case() {
  [[ -f "$repo_root/tests/e2e/AUR-467.sh" ]] || infra "missing-input:tests/e2e/AUR-467.sh"
  set +e
  bash "$repo_root/tests/e2e/AUR-467.sh" E2EAUR467
  local rc=$?
  set -e
  ((rc == 0)) || exit "$rc"
}

# Each declared selector runs its OWN program with its OWN assertions. An
# unknown or unimplemented selector exits 64 rather than falling through to
# AC-001 and harvesting someone else's green.
case "$selector" in
  AC-001-MUT-001) mutation_case_001 ;;
  AC-003-MUT-002) mutation_case_002 ;;
  TestAUR467) unit_case ;;
  IntegrationAUR467) integration_case ;;
  E2EAUR467) e2e_case ;;
  AC-001) nominal_case ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac
exit 0
