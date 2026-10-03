#!/usr/bin/env bash
# AUR-577 acceptance: scanners are registered engines. An engine registered
# only in a test reaches the gate line, the audit and the SARIF under its own
# origin with no switch on an engine name in internal/gate or cmd (go/ast);
# an unknown engine is refused by Parse and quality_gates.sast stays the
# semgrep alias; a central policy's required scanner survives a repository's
# disable; a missing binary or an incomplete report is inconclusive; the
# audit and the SARIF come from the same gate findings.
#
# Selectors:
#   all        AC-001..AC-005, then MUT-001 and MUT-002
#   AC-001     fake engine in every sink by origin; no engine switch (go/ast)
#   AC-002     unknown engine refused; quality_gates.sast alias; sast demo
#   AC-003     policy required beats the repository's enabled: false
#   AC-004     missing binary / incomplete report are inconclusive
#   AC-005     ssor_dtrack in the audit and the SARIF from one gate finding
#   MUT-001    the runner ignoring `engine` (always semgrep) turns AC-001 RED
#   MUT-002    the repository overriding a required policy entry turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-577'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg cmd/aurumcode/aur577_test.go cmd/aurumcode/scanner_pass.go internal/config/scanners_policy.go internal/scanner/registry.go demo/tutoriais/sast/run.sh; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a577.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# stage copies the whole module (never enumerated packages) to a fresh root.
stage() {
  local root="$1" source
  mkdir -p "$root"
  for source in go.mod go.sum cmd internal pkg docs demo; do
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  for source in tests/fixtures tests/e2e; do
    if [[ -d "$repo_root/$source" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/$source" "$root/$source"; fi
  done
  chmod -R u+w -- "$root"
}

# go_test root log pattern pkgs... runs the named tests; rc is go's.
go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
}

# require_pass log names... fails unless every named test passed.
require_pass() {
  local log="$1" name; shift
  for name in "$@"; do
    grep -Eq -- "^--- PASS: ${name} " "$log" || { cat "$log" >&2; fail "missing-pass:$name"; }
  done
}

# replace_once file anchor replacement: a literal, unique edit.
replace_once() {
  local file="$1" anchor="$2" replacement="$3" count
  count="$(grep -Fc -- "$anchor" "$file")" || infra "anchor-missing:${file##*/}"
  [[ "$count" == 1 ]] || infra "anchor-not-unique:${file##*/}"
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { a = ENVIRON["ANCHOR"]; r = ENVIRON["REPL"] }
    { i = index($0, a); if (i > 0) { print substr($0, 1, i - 1) r substr($0, i + length(a)) } else { print } }
  ' "$file" >"$file.mut" && mv "$file.mut" "$file"
  grep -Fq -- "$replacement" "$file" || infra "mutation-not-applied:${file##*/}"
}

readonly ac001_tests=(TestAUR577FakeEngineReachesEverySinkByOrigin TestAUR577NoEngineSwitchInGateOrCmd)
readonly ac002_tests=(TestAUR577UnknownEngineRefusedAndSastIsAlias TestAUR577UnknownEngineStopsTheReview TestAUR548SeverityBreachFailsGate TestAUR548CleanScanPasses)
readonly ac003_tests=(TestAUR577PolicyRequiredScannerSurvivesRepositoryDisable TestAUR548PolicyWinsOverRepoDisable)
readonly ac004_tests=(TestAUR577MissingBinaryAndIncompleteAreInconclusive TestAUR548AbsentSemgrepIsInconclusiveNeverClean)
readonly ac005_tests=(TestAUR577AuditAndSARIFShareTheGateFindings)
readonly pkgs=(./cmd/aurumcode/)

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

run_ac() {
  local name="$1"; shift
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}" || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" "$@"
  printf '%s/%s/pass\n' "$card" "$name"
}

# expect_red root log tests...: the mutated copy must compile and turn at
# least one named test RED by its assertion, never by a build error.
expect_red() {
  local root="$1" log="$2"; shift 2
  if go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,4p' >&2
}

run_ac002() {
  run_ac AC-002 "${ac002_tests[@]}"
  (cd "$repo_root" && bash demo/tutoriais/sast/run.sh --check >"$run_dir/sast-check.log" 2>&1) || { cat "$run_dir/sast-check.log" >&2; fail sast-demo-check-failed; }
  printf '%s/AC-002/sast-demo/pass\n' "$card"
}

run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/cmd/aurumcode/scanner_pass.go" \
    'out := s.deps.scanners.Scan(s.ctx, engine, scanner.Request{' \
    'only, _ := scanner.Lookup("semgrep") /* MUT-001: engine ignored */; out := s.deps.scanners.Scan(s.ctx, only, scanner.Request{'
  expect_red "$root" "$run_dir/mut1.log" "${ac001_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR577FakeEngineReachesEverySinkByOrigin' "$run_dir/mut1.log" || fail mut001-wrong-test
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/internal/config/scanners_policy.go" \
    'if declared && p.Required {' \
    'if declared && p.Required && false { // MUT-002: the repository overrides required'
  expect_red "$root" "$run_dir/mut2.log" "${ac003_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR577PolicyRequiredScannerSurvivesRepositoryDisable' "$run_dir/mut2.log" || fail mut002-wrong-test
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "${ac001_tests[@]}" ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  AC-004) run_ac AC-004 "${ac004_tests[@]}" ;;
  AC-005) run_ac AC-005 "${ac005_tests[@]}" ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "${ac001_tests[@]}"
    run_ac002
    run_ac AC-003 "${ac003_tests[@]}"
    run_ac AC-004 "${ac004_tests[@]}"
    run_ac AC-005 "${ac005_tests[@]}"
    run_mut001
    run_mut002
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
