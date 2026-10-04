#!/usr/bin/env bash
# AUR-492 acceptance: real linters complement the review with a verifiable
# origin. go vet is the registered engine `govet` (category lint, origin
# govet): a real go vet diagnostic on a line the reviewed range added is a
# finding with file, line and the analyzer's rule; the corrected change is
# clean; a failed run, a missing binary or an untrusted report is
# inconclusive, never green; the engine runs only when declared; and no
# engine's child process (semgrep, gitleaks, govet) receives a secret of the
# reviewing process.
#
# Selectors:
#   all        AC-001..AC-004, then MUT-001..MUT-004
#   AC-001     real go vet: finding on the added line; corrected change clean
#   AC-002     missing go, failed vet, bad report, no range: inconclusive; recorded demo
#   AC-003     explicit child environment; no secret reaches any engine
#   AC-004     govet runs only when declared: the recorded sast cases without the
#              entry never mention it; config.Parse test where internal/config's
#              closure is materialized (the sealed image holds only the card's
#              paths, without internal/dtrack and the other config imports)
#   MUT-001    a failed go vet read as findings turns AC-002 RED
#   MUT-002    keeping findings outside the diff turns AC-001 RED
#   MUT-003    the engine left out of the binary (ignored) turns AC-003 RED
#   MUT-004    the child inheriting the process environment turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-492'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002|MUT-003|MUT-004) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum internal pkg internal/scanner/govet/engine.go internal/scanner/executor.go internal/scanner/environment.go internal/analysis/vet.go internal/scanner/engines/engines.go demo/tutoriais/sast/run.sh; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a492.XXXXXX")" || infra mktemp
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

readonly ac001_tests=(TestRealVetFindingOnAddedLine TestRealVetCorrectedChangeIsClean TestRealVetIgnoresUntouchedLines TestVetReadsJSONReport TestVetCleanReportHasNoFinding TestParseAddedLines TestDiffIsRelativeToTheRoot)
readonly ac002_tests=(TestMissingGoIsUnavailable TestRealVetFailureIsInconclusive TestNoRangeIsInconclusive TestRefusesOptions TestVetFailureNeverYieldsFindings TestVetKeepsMissingBinary TestVetRefusesUntrustedReport TestParseAddedLinesRefusesBadHeader TestParseAddedLinesRefusesQuotedPath TestRootWithoutModuleIsInconclusive)
readonly ac003_tests=(TestNoEngineChildInheritsSecrets TestChildEnvironmentIsExplicit TestGitSafeDirectoriesDropsOtherKeys TestIsolatedCommandUsesOnlyGivenEnvironment TestIsolatedCommandBoundsOutput TestGoChildHasCgoDisabled)
readonly ac004_tests=(TestGovetRunsOnlyWhenDeclared)
readonly pkgs=(./internal/scanner/ ./internal/scanner/govet/ ./internal/analysis/)
readonly config_pkgs=(./internal/scanner/engines/)

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

demo="$repo_root/demo/tutoriais/sast"

# expect_recorded case: every line of expected/<case>.txt is in out/<case>.log,
# literally (the image identity of out/ is checked by run.sh --check in the
# regression, where the whole tree exists).
expect_recorded() {
  local c="$1" line
  [[ -f "$demo/out/$c.log" && -f "$demo/expected/$c.txt" ]] || fail "demo-missing:$c"
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == '#'* ]] && continue
    grep -Fq -- "$line" "$demo/out/$c.log" || fail "demo-line-missing:$c:$line"
  done <"$demo/expected/$c.txt"
}

run_ac001() {
  run_ac AC-001 "${ac001_tests[@]}"
  expect_recorded govet-achado
  grep -Fq 'calc/calc.go:9: [warning] fmt.Printf format %d has arg s of wrong type string (rule go-vet/printf)' "$demo/out/govet-achado.log" || fail demo-finding-missing
  if grep -Fq 'legado/legado.go' "$demo/out/govet-achado.log"; then fail demo-finding-outside-diff; fi
  printf '%s/AC-001/demo/pass\n' "$card"
}

run_ac002() {
  run_ac AC-002 "${ac002_tests[@]}"
  expect_recorded govet-sem-go
  grep -Fq 'inconclusivo (lint_unavailable)' "$demo/out/govet-sem-go.log" || fail demo-not-inconclusive
  printf '%s/AC-002/demo/pass\n' "$card"
}

run_ac003() {
  run_ac AC-003 "${ac003_tests[@]}"
  expect_recorded govet-achado
  grep -Fxq 'LLM_API_KEY no ambiente do go: ausente' "$demo/out/govet-achado.log" || fail demo-secret-reached-go
  grep -Fxq 'GITHUB_TOKEN no ambiente do go: ausente' "$demo/out/govet-achado.log" || fail demo-token-reached-go
  printf '%s/AC-003/demo/pass\n' "$card"
}

run_ac004() {
  local c root="$run_dir/root-AC-004" log="$run_dir/AC-004.log"
  for c in regra-local registry-sem-rede nosemgrep-e-semgrepignore origem-sast semgrep-falha; do
    expect_recorded "$c"
    if grep -Eqi 'govet|lint_' "$demo/out/$c.log"; then fail "undeclared-govet-ran:$c"; fi
  done
  printf '%s/AC-004/demo/pass\n' "$card"
  stage "$root"
  if ( cd "$root" && go list -deps ./internal/config/ >/dev/null 2>&1 ); then
    go_test "$root" "$log" "$(pattern_of "${ac004_tests[@]}")" "${config_pkgs[@]}" || { cat "$log" >&2; fail go-test-failed; }
    require_pass "$log" "${ac004_tests[@]}"
    printf '%s/AC-004/config/pass\n' "$card"
  else
    printf '%s/AC-004/config/not-materialized\n' "$card"
  fi
}

# mutate name file anchor replacement tests...: one mutated copy must turn
# the first named test RED.
mutate() {
  local name="$1" file="$2" anchor="$3" replacement="$4"; shift 4
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  stage "$root"
  replace_once "$root/$file" "$anchor" "$replacement"
  expect_red "$root" "$log" "$@"
  grep -Eq -- "^--- FAIL: $1( |$)" "$log" || { cat "$log" >&2; fail "$name-wrong-test"; }
  printf '%s/%s/rejected\n' "$card" "$name"
}

run_mut001() {
  mutate MUT-001 internal/analysis/vet.go 'if runErr != nil {' \
    'if runErr != nil && stdout == "" { // MUT-001: a failed vet with output reads as findings' \
    TestRealVetFailureIsInconclusive TestVetFailureNeverYieldsFindings
}

run_mut002() {
  mutate MUT-002 internal/scanner/govet/diff.go 'if s[f.Path][f.Line] {' \
    'if true || s[f.Path][f.Line] { // MUT-002: findings outside the diff kept' \
    TestRealVetIgnoresUntouchedLines
}

run_mut003() {
  mutate MUT-003 internal/scanner/engines/engines.go '_ "github.com/Mpaape/AurumCode/internal/scanner/govet"' \
    '/* MUT-003: govet left out of the binary */' \
    TestNoEngineChildInheritsSecrets
}

run_mut004() {
  mutate MUT-004 internal/scanner/executor.go 'cmd.Env = env' \
    'cmd.Env = nil /* MUT-004: the child inherits the process environment */' \
    TestNoEngineChildInheritsSecrets TestIsolatedCommandUsesOnlyGivenEnvironment
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  MUT-003) run_mut003 ;;
  MUT-004) run_mut004 ;;
  all)
    run_ac001
    run_ac002
    run_ac003
    run_ac004
    run_mut001
    run_mut002
    run_mut003
    run_mut004
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
