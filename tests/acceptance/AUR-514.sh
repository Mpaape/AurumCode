#!/usr/bin/env bash
#
# Acceptance program for card AUR-514: merging the findings of two or more
# reviewer profiles keeps evidence, impact, verification, side and the
# suggested fix all the way to the public output. See docs/specs/AUR-514.md.
#
# SELECTORS
#   all      every selector below
#   AC-001   two real profile passes merge a finding with every field: the
#            fields survive, the collapsed duplicate names both profiles,
#            a later duplicate fills what the kept copy lacked, LEFT and
#            RIGHT on the same line stay two findings
#   AC-002   the merged finding shows the same fields in the terminal
#            report, the review document and the inline comment, labeled
#            in pt-BR and in en
#   MUT-001  dropping Evidence, Impact or Side in the profile conversion
#            (three separate mutants, each compiling) turns AC-001 red
#
# EXIT CODES: 0 pass, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-514'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|MUT-001)
    ;;
  *)
    printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2
    exit 64
    ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg docs/specs/AUR-514.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a514.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod

readonly ac001_tests='TestAUR514AC001ProfileMergeKeepsEveryField|TestAUR514AC001DuplicateKeepsTheLaterEvidence|TestAUR514AC001SidesAreDistinctFindings'
readonly ac002_tests='TestAUR514AC002TerminalAndPRShowTheSameFields'

# stage copies the module into a fresh root so a mutation never touches the
# checkout.
stage() {
  local root="$1" top
  mkdir -p "$root"
  for top in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$top" "$root/$top"
  done
  chmod -R u+w -- "$root"
}

# go_test runs the named cmd/aurumcode tests in root; rc in gt_rc, output
# in $run_dir/gotest.out.
go_test() {
  local root="$1" pattern="$2"
  set +e
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -run "$pattern" ./cmd/aurumcode) >"$run_dir/gotest.out" 2>&1
  gt_rc=$?
  set -e
}

# go_test_v runs the named tests verbosely and requires each to PASS.
go_test_v() {
  local root="$1" pattern="$2" label="$3" name
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" ./cmd/aurumcode) >"$run_dir/gotest.out" 2>&1 || { cat "$run_dir/gotest.out" >&2; fail "$label"; }
  for name in ${pattern//|/ }; do
    grep -q "^--- PASS: $name " "$run_dir/gotest.out" || fail "not-run:$name"
  done
}

run_ac001() {
  stage "$run_dir/ac001"
  go_test_v "$run_dir/ac001" "$ac001_tests" merge-tests
}

run_ac002() {
  stage "$run_dir/ac002"
  go_test_v "$run_dir/ac002" "$ac002_tests" output-tests
}

# mutate_and_expect_red drops one field from the conversion in a fresh
# copy and requires AC-001 to go red by behavior, not by a build failure.
mutate_and_expect_red() {
  local field="$1" root="$run_dir/mut-$1" target
  stage "$root"
  target="$root/cmd/aurumcode/profile_findings.go"
  grep -Eq "^[[:space:]]+${field}:[[:space:]]+issue\.${field},\$" "$target" || infra "mut-anchor:$field"
  sed -i -E "/^[[:space:]]+${field}:[[:space:]]+issue\.${field},\$/d" "$target"
  if grep -Eq "^[[:space:]]+${field}:[[:space:]]+issue\.${field},\$" "$target"; then infra "mut-not-applied:$field"; fi
  go_test "$root" "$ac001_tests"
  [[ "$gt_rc" -ne 0 ]] || fail "mutant-survived:$field"
  if grep -Eq 'build failed|undefined:|syntax error|cannot use' "$run_dir/gotest.out"; then
    cat "$run_dir/gotest.out" >&2
    fail "mutant-build-failure:$field"
  fi
  grep -Fq 'AUR-514 field lost' "$run_dir/gotest.out" || { cat "$run_dir/gotest.out" >&2; fail "mutant-other-cause:$field"; }
  printf '%s/MUT-001/%s red\n' "$card" "$field"
}

run_mut001() {
  local field
  for field in Evidence Impact Side; do
    mutate_and_expect_red "$field"
  done
}

case "$selector" in
  AC-001)
    run_ac001
    ;;
  AC-002)
    run_ac002
    ;;
  MUT-001)
    run_mut001
    ;;
  all)
    run_ac001
    run_ac002
    run_mut001
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
