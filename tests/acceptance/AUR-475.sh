#!/usr/bin/env bash
#
# Acceptance program for card AUR-475.
#
# WHAT THIS PROVES
#
#   TrimToFit (internal/prompt/budgeting.go) no longer stops the whole
#   review at the first segment that doesn't fit the token budget. A single
#   oversized file at the front of the priority order used to consume the
#   budget and `break`, discarding every file behind it whole -- not
#   truncated, discarded. This program proves, against the real
#   PromptBuilder.BuildPrompt boundary: the files behind the oversized one
#   that DO fit are still reviewed (AC-001), nothing that reaches the
#   review is ever partial content -- whole or entirely absent, never cut
#   mid-file (AC-002), and a file whose hunks are only partly covered
#   classifies "partial" while a wholly-skipped file classifies "omitted",
#   never confused with each other (AC-003).
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

readonly card='AUR-475'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR475|IntegrationAUR475|E2EAUR475|AC-001-MUT-001|AC-002-MUT-002) ;;
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
  internal/prompt/budgeting.go
  internal/prompt/coverage.go
  pkg/types
  tests/unit/AUR-475.go
  tests/integration/AUR-475.go
  tests/e2e/AUR-475.sh
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a475.XXXXXX")" || infra mktemp
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
# assembled review prompt, followed by a `META key=value` line per
# PromptParts.Meta entry, for diffs shaped to exercise the starvation
# mechanic this card fixes.
write_dumper() {
  local root="$1"
  mkdir -p "$root/cmd/aur475dump"
  cat >"$root/cmd/aur475dump/main.go" <<'EOF'
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

func codeFile(path string, lines ...string) types.DiffFile {
	return types.DiffFile{Path: path, Hunks: []types.DiffHunk{{Lines: lines}}}
}

func main() {
	mode := "nominal"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	var diff *types.Diff
	maxTokens, reserve := 6000, 500

	switch mode {
	case "nominal":
		// One oversized code file sorts first ("src/aaa_big.go"); five
		// small code files sort after it. Before this card, the oversized
		// file consumed the budget and `break` discarded every file
		// behind it -- none of the five would appear.
		files := []types.DiffFile{codeFile("src/aaa_big.go", "+"+strings.Repeat("z", 20000))}
		for i := 0; i < 5; i++ {
			files = append(files, codeFile(fmt.Sprintf("src/small%02d.go", i),
				fmt.Sprintf("+const SMALL_MARKER_%02d = true", i)))
		}
		diff = &types.Diff{Files: files}
	case "hunklevel":
		// One file whose first hunk is oversized and second hunk is
		// small (must classify "partial", not "omitted"), plus a second
		// file whose single hunk is wholly oversized (must classify
		// "omitted", not "partial").
		diff = &types.Diff{Files: []types.DiffFile{
			{
				Path: "src/two.go",
				Hunks: []types.DiffHunk{
					{Lines: []string{"+" + strings.Repeat("h", 20000)}},
					{Lines: []string{"+const HUNK1_SURVIVES = true"}},
				},
			},
			codeFile("src/wholly_skipped.go", "+"+strings.Repeat("w", 20000)),
		}}
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
EOF
}

# assert_nominal renders mode "nominal" and checks AC-001 (small files
# behind the oversized one are still reviewed, the oversized one is named)
# and AC-002 (no truncation marker anywhere).
assert_nominal() {
  local root="$1" label="$2" out="$root/nominal.out"
  if ! run_go "$root" run ./cmd/aur475dump nominal >"$out" 2>"$root/nominal.err"; then
    cat "$root/nominal.err" >&2
    if [[ "$label" == 'MUT-001' ]]; then return 1; fi
    infra dump_failed
  fi
  [[ -s "$out" ]] || infra empty_output

  # AC-001: every small file behind the oversized one is still reviewed.
  local i marker missing=0
  for i in 0 1 2 3 4; do
    marker="$(printf 'SMALL_MARKER_%02d' "$i")"
    grep -Fq "$marker" "$out" || missing=$((missing + 1))
  done
  ((missing == 0)) || return 1
  # AC-001/Non-goal: the oversized file that did not fit is named, not silent.
  grep -Fq 'src/aaa_big.go' "$out" || return 1
  grep -Fq 'Code files NOT reviewed by this review (token budget): 1' "$out" || return 1
  grep -Fq 'META code_files_complete=5' "$out" || return 1
  grep -Fq 'META code_files_omitted=1' "$out" || return 1
  # AC-002: nothing in the output was truncated.
  if grep -Fq '(truncated)' "$out"; then return 1; fi
  return 0
}

# assert_hunklevel renders mode "hunklevel" and checks AC-003: a file with
# a surviving small hunk behind a skipped big one classifies "partial", a
# wholly-skipped single-hunk file classifies "omitted" -- never confused.
assert_hunklevel() {
  local root="$1" label="$2" out="$root/hunklevel.out"
  if ! run_go "$root" run ./cmd/aur475dump hunklevel >"$out" 2>"$root/hunklevel.err"; then
    cat "$root/hunklevel.err" >&2
    infra dump_failed
  fi
  [[ -s "$out" ]] || infra empty_output

  grep -Fq 'HUNK1_SURVIVES' "$out" || return 1
  grep -Fq 'src/two.go (1/2 hunks)' "$out" || return 1
  grep -Fq 'src/wholly_skipped.go (0/1 hunks)' "$out" || return 1
  grep -Fq 'META code_files_partial=1' "$out" || return 1
  grep -Fq 'META code_files_omitted=1' "$out" || return 1
  if grep -Fq '(truncated)' "$out"; then return 1; fi
  return 0
}

nominal_case() {
  local root="$run_dir/root-nominal"
  stage_source "$root"
  write_dumper "$root"
  assert_nominal "$root" 'AC-001' || fail behavior-missing
  assert_hunklevel "$root" 'AC-003' || fail behavior-missing
}

# MUT-001: revert the fix's `continue` back to `break` (the card's exact
# skeptical mutation). The nominal proof must go RED: the oversized file
# starves every file behind it again.
mutation_case_001() {
  local root="$run_dir/root-mut1"
  stage_source "$root"
  write_dumper "$root"

  local target="$root/internal/prompt/budgeting.go"
  local anchor='continue // AUR-475: skip whole segment, never truncate, never stop'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-001/anchor-not-unique'
  local tmp="$root/budgeting.mutated"
  sed "s/${anchor//\//\\/}/break \/\/ MUT-001: reverted to break/" "$target" >"$tmp" || infra MUT-001-rewrite
  mv "$tmp" "$target"
  grep -Fq 'MUT-001: reverted to break' "$target" || infra MUT-001-not-applied

  if assert_nominal "$root" 'MUT-001'; then
    fail MUT-001
  fi
}

# MUT-002: re-introduce truncation (the card's exact skeptical mutation) --
# a segment that overflows the remaining budget is truncated and included
# instead of skipped whole. AC-002's proof (no `(truncated)` marker ever
# appears) must go RED.
mutation_case_002() {
  local root="$run_dir/root-mut2"
  stage_source "$root"
  write_dumper "$root"

  local target="$root/internal/prompt/budgeting.go"
  local anchor='continue // AUR-475: skip whole segment, never truncate, never stop'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra 'MUT-002/anchor-not-unique'
  local replacement='result = append(result, b.truncateSegment(segment, available-currentTokens)); currentTokens = available; continue // MUT-002: truncation re-introduced'
  local tmp="$root/budgeting.mutated"
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
  grep -Fq 'MUT-002: truncation re-introduced' "$target" || infra MUT-002-not-applied

  if assert_nominal "$root" 'MUT-002'; then
    fail MUT-002
  fi
}

bridge_run() {
  local pkg="$1" fn="$2" bridge="$3" label="$4"
  local root="$run_dir/root-$pkg"
  stage_source "$root"
  copy "$root" "tests/$pkg/AUR-475.go"
  chmod -R u+w -- "$root/tests"
  cat >"$root/tests/$pkg/aur475_bridge_test.go" <<EOF
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

unit_case() { bridge_run unit TestAUR475 TestAUR475UnitBridge TestAUR475; }
integration_case() { bridge_run integration IntegrationAUR475 TestAUR475IntegrationBridge IntegrationAUR475; }

e2e_case() {
  [[ -f "$repo_root/tests/e2e/AUR-475.sh" ]] || infra "missing-input:tests/e2e/AUR-475.sh"
  set +e
  bash "$repo_root/tests/e2e/AUR-475.sh" E2EAUR475
  local rc=$?
  set -e
  ((rc == 0)) || exit "$rc"
}

# Each declared selector runs its OWN program with its OWN assertions. An
# unknown or unimplemented selector exits 64 rather than falling through to
# AC-001 and harvesting someone else's green.
case "$selector" in
  AC-001-MUT-001) mutation_case_001 ;;
  AC-002-MUT-002) mutation_case_002 ;;
  TestAUR475) unit_case ;;
  IntegrationAUR475) integration_case ;;
  E2EAUR475) e2e_case ;;
  AC-001) nominal_case ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac
exit 0
