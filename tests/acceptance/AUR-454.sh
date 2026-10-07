#!/usr/bin/env bash
#
# Acceptance program for card AUR-454: the review publishes the same
# problem at the same place once, even from different passes, and applies
# explicit presentation preferences without hiding a blocking finding. See
# docs/specs/AUR-454.md.
#
# SELECTORS
#   all      every selector below
#   AC-001   two passes reporting the same rule at the same line become one
#            traceable finding with both evidences
#   AC-002   distinct problems on the same line stay distinct; no count cut
#   AC-003   review.presentation.collapse is validated, applied
#            deterministically, explained in the body, never to a blocking
#            finding
#   MUT-001  two mutants, each compiling: disabling the consolidation
#            (AC-001 red) and merging different rules (AC-002 red)
#
# EXIT CODES: 0 pass, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-454'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|MUT-001)
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

for input in go.mod go.sum cmd internal pkg docs/specs/AUR-454.md docs/review-quality.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a454.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod

stage() {
  local root="$1" top
  mkdir -p "$root"
  for top in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$top" "$root/$top"
  done
  chmod -R u+w -- "$root"
}

go_test_v() {
  local root="$1" pkg="$2" pattern="$3" label="$4" name
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$pkg") >"$run_dir/gotest.out" 2>&1 || { cat "$run_dir/gotest.out" >&2; fail "$label"; }
  for name in ${pattern//|/ }; do
    grep -q "^--- PASS: $name " "$run_dir/gotest.out" || fail "not-run:$name"
  done
}

ac001() {
  stage "$run_dir/ac001"
  go_test_v "$run_dir/ac001" ./internal/review/consolidate 'TestAC001TwoPassesSameDefectPublishedOnce' consolidate
  go_test_v "$run_dir/ac001" ./cmd/aurumcode 'TestAUR454PublishedBodyExplainsMergeAndCollapse' published-body
}

ac002() {
  stage "$run_dir/ac002"
  go_test_v "$run_dir/ac002" ./internal/review/consolidate 'TestAC002DistinctProblemsStayDistinctNoCountCut' distinct
}

ac003() {
  stage "$run_dir/ac003"
  go_test_v "$run_dir/ac003" ./internal/review/consolidate 'TestAC003CollapseIsDeterministicExplainedAndNeverHidesBlocking' collapse
  go_test_v "$run_dir/ac003" ./internal/config 'TestReviewPresentationCollapseValidates' config
  go_test_v "$run_dir/ac003" ./cmd/aurumcode 'TestAUR454PublishedBodyExplainsMergeAndCollapse|TestAUR454InconclusiveGateCondensesNothing' published-body
}

# mutant applies one sed to consolidate.go in a fresh copy and requires the
# named test to go red by behavior.
mutant() {
  local name="$1" from="$2" to="$3" test="$4" root="$run_dir/mut-$1" target
  stage "$root"
  target="$root/internal/review/consolidate/consolidate.go"
  grep -Fq "$from" "$target" || infra "mut-anchor:$name"
  FROM="$from" TO="$to" perl -0pi -e 's/\Q$ENV{FROM}\E/$ENV{TO}/' "$target"
  grep -Fq "$to" "$target" || infra "mut-not-applied:$name"
  set +e
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -run "$test" ./internal/review/consolidate) >"$run_dir/mut.out" 2>&1
  local rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail "mutant-survived:$name"
  if grep -Eq 'build failed|undefined:|syntax error|cannot use' "$run_dir/mut.out"; then
    cat "$run_dir/mut.out" >&2
    fail "mutant-build-failure:$name"
  fi
  grep -Fq "$5" "$run_dir/mut.out" || { cat "$run_dir/mut.out" >&2; fail "mutant-other-cause:$name"; }
  printf '%s/MUT-001/%s red\n' "$card" "$name"
}

run_mut001() {
  command -v perl >/dev/null 2>&1 || infra missing_perl
  mutant no-consolidation 'key := identity(issue)' 'key := identity(issue) + strconv.Itoa(len(res.Issues)+res.Merged)' \
    'TestAC001TwoPassesSameDefectPublishedOnce' 'AUR-454 duplicate kept'
  mutant merge-different-rules 'strconv.Itoa(issue.Line), side, issue.RuleID}' 'strconv.Itoa(issue.Line), side}' \
    'TestAC002DistinctProblemsStayDistinctNoCountCut' 'AUR-454 distinct problems merged'
}

case "$selector" in
  AC-001)
    ac001
    ;;
  AC-002)
    ac002
    ;;
  AC-003)
    ac003
    ;;
  MUT-001)
    run_mut001
    ;;
  all)
    ac001
    ac002
    ac003
    run_mut001
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
