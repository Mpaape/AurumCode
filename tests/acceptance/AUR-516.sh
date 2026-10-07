#!/usr/bin/env bash
#
# Acceptance program for card AUR-516: the review's CI status tells what the
# CI context observed apart from what the model inferred. See
# docs/specs/AUR-516.md.
#
# SELECTORS
#   all      every selector below
#   AC-001   a real --pr review with no CI context and a model that says CI
#            is green publishes no CI state: the item says "not verified"
#   AC-002   a failed check with name and link and no log shows the observed
#            state and link, an unknown cause, the model's cause only as a
#            hypothesis, no fix, and how to diagnose
#   AC-003   with a sanitized log excerpt the model's evidence quotes, the
#            fix is published citing the observation, on lines apart from
#            the model's inference; evidence that does not quote it is AC-002
#   MUT-001  taking the model's own status as the observed state (compiles)
#            turns AC-001 red
#
# EXIT CODES: 0 pass, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-516'
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

for input in go.mod go.sum cmd internal pkg docs/specs/AUR-516.md docs/review-quality.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a516.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod

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

# go_test_v runs the named tests of pkg in root verbosely and requires each
# to PASS.
go_test_v() {
  local root="$1" pkg="$2" pattern="$3" label="$4" name
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$pkg") >"$run_dir/gotest.out" 2>&1 || { cat "$run_dir/gotest.out" >&2; fail "$label"; }
  for name in ${pattern//|/ }; do
    grep -q "^--- PASS: $name " "$run_dir/gotest.out" || fail "not-run:$name"
  done
}

run_scenario() {
  local id="$1" cmd_tests="$2" unit_tests="$3"
  stage "$run_dir/$id"
  go_test_v "$run_dir/$id" ./cmd/aurumcode "$cmd_tests" "$id-review"
  go_test_v "$run_dir/$id" ./internal/review/cistatus "$unit_tests" "$id-cistatus"
}

run_mut001() {
  local root="$run_dir/mut001" target anchor
  stage "$root"
  target="$root/internal/review/cistatus/verify.go"
  anchor='item.Basis, item.Observed, item.Link, item.Grounded = BasisModel, "", "", false'
  grep -Fq "$anchor" "$target" || infra mut001-anchor
  sed -i 's/item.Basis, item.Observed, item.Link, item.Grounded = BasisModel, "", "", false/item.Basis, item.Observed, item.Link, item.Grounded = BasisCI, item.Status, "", false/' "$target"
  grep -Fq 'item.Grounded = BasisCI, item.Status, "", false' "$target" || infra mut001-not-applied
  set +e
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -run 'TestAUR516AC001ModelSaysCIGreenWithoutContext' ./cmd/aurumcode) >"$run_dir/mut.out" 2>&1
  local rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail mutant-survived
  if grep -Eq 'build failed|undefined:|syntax error|cannot use' "$run_dir/mut.out"; then
    cat "$run_dir/mut.out" >&2
    fail mutant-build-failure
  fi
  grep -Fq 'AUR-516 model state published as CI state' "$run_dir/mut.out" || { cat "$run_dir/mut.out" >&2; fail mutant-other-cause; }
  printf '%s/MUT-001/red (o estado do modelo virou estado de CI)\n' "$card"
}

ac001() { run_scenario AC-001 'TestAUR516AC001ModelSaysCIGreenWithoutContext' 'TestVerifyNeverTakesTheModelsState'; }
ac002() { run_scenario AC-002 'TestAUR516AC002FailedCheckWithoutLogsGivesDiagnosis' 'TestVerifyObservedStateAndLink'; }
ac003() { run_scenario AC-003 'TestAUR516AC003EvidenceQuotedKeepsInferenceApart' 'TestVerifyGroundedOnlyWhenEvidenceQuotesTheExcerpt|TestVerifyOneWordIsNotAQuote'; }

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
