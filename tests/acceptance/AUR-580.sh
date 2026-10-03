#!/usr/bin/env bash
# AUR-580 acceptance: deliberation with tools. The model asks for an
# optional scanner when the diff is large and not when it is small; the
# scan's findings count in the gate with their origin; exceeding max_rounds
# or max_cost_tokens is inconclusive with nothing published; every tool
# round is reserved and committed and never falls back to a provider
# without tool calling; invalid arguments are refused before running; a
# required scanner is never offered; the transcript (redacted arguments,
# duration) is in the audit; the deliberacao tutorial's --check passes.
#
# Selectors:
#   all        AC-001..AC-005, then MUT-001 and MUT-002
#   AC-001     large diff asks for the scanner, small diff does not
#   AC-002     max_rounds / max_cost_tokens / tool timeout are inconclusive through
#              the gate (audit with the limit, SARIF, --pr policy-gate failure)
#   AC-003     reserve and commit per round; no fallback without ToolCaller
#   AC-004     invalid arguments refused; required scanner never optional
#   AC-005     transcript in the audit; tutorial --check
#   MUT-001    publishing the last reply over max_rounds turns AC-002 RED
#   MUT-002    skipping the reservation of a tool round turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-580'
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
for input in go.mod go.sum cmd internal pkg cmd/aurumcode/aur580_test.go internal/deliberation/session.go internal/llm/orchestrator_tools.go demo/tutoriais/deliberacao/run.sh docs/tutorials/deliberacao.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a580.XXXXXX")" || infra mktemp
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

readonly ac001_tests=(TestAUR580ModelAsksForTheScannerOnALargeDiff TestAUR580ModelDoesNotAskOnASmallDiff TestAUR580ToolResultGoesBackAndAnswerEnds)
readonly ac002_tests=(TestAUR580MaxRoundsIsALimitErrorWithoutAnswer TestAUR580MaxCostTokensIsALimitError TestAUR580PerToolTimeoutIsALimitError TestAUR580RoundsExceededIsInconclusiveAndUnpublished TestAUR580CostExceededIsInconclusive TestAUR580PullRequestRoundsExceededFailsThePolicyGate)
readonly ac003_tests=(TestAUR580EveryToolRoundIsReservedAndCommitted TestAUR580RoundOverTheCeilingIsRefusedBeforeTheCall TestAUR580NoFallbackToAProviderWithoutTools TestAUR580ToolRoundSendsTheJSONSchema TestAUR580SchemaOfStruct)
readonly ac004_tests=(TestAUR580InvalidArgumentsRefusedBeforeRun TestAUR580InvalidArgumentsRefusedAndRedactedInTheAudit TestAUR580RequiredScannerIsNeverOptional TestAUR580ManifestNamesEachToolWithItsCost TestAUR580InconclusiveScanIsAToolError TestAUR580ContextAndSkillToolsAnswerOnlyKnownNames TestAUR580MissingBinaryIsInconclusive TestAUR580DeferredScannerRunsWhenToolsCannotBeOffered TestAUR580DeliberationParsesWithDefaultsAndRefusesBadValues TestAUR580PolicyDeliberationDecidesAlone TestAUR580LimitsMustBePositive)
readonly ac005_tests=(TestAUR580TranscriptRedactsArguments TestAUR580InvalidArgumentsRefusedAndRedactedInTheAudit TestAUR580ModelAsksForTheScannerOnALargeDiff)
readonly pkgs=(./cmd/aurumcode/ ./internal/deliberation/ ./internal/llm/... ./internal/review/tools/ ./internal/config/)

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

run_ac005() {
  run_ac AC-005 "${ac005_tests[@]}"
  (cd "$repo_root" && bash demo/tutoriais/deliberacao/run.sh --check >"$run_dir/tut-check.log" 2>&1) || { cat "$run_dir/tut-check.log" >&2; fail tutorial-check-failed; }
  printf '%s/AC-005/tutorial/pass\n' "$card"
}

# MUT-001: over max_rounds, the last reply is returned as the answer.
run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/internal/deliberation/session.go" \
    'return s.stop(out, &LimitError{Limit: LimitMaxRounds' \
    'return Outcome{Answer: last.Response, Transcript: out.Transcript}, nil /* MUT-001 */; _ = (&LimitError{Limit: LimitMaxRounds'
  expect_red "$root" "$run_dir/mut1.log" "${ac002_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR580MaxRoundsIsALimitErrorWithoutAnswer' "$run_dir/mut1.log" || fail mut001-wrong-test
  grep -Eq -- '^--- FAIL: TestAUR580RoundsExceededIsInconclusiveAndUnpublished' "$run_dir/mut1.log" || fail mut001-not-published
  printf '%s/MUT-001/rejected\n' "$card"
}

# MUT-002: a tool round is sent without a reservation.
run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/internal/llm/orchestrator_tools.go" \
    'if o.tracker == nil {' \
    'if true || o.tracker == nil { // MUT-002: no reservation for a tool round'
  expect_red "$root" "$run_dir/mut2.log" "${ac003_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR580EveryToolRoundIsReservedAndCommitted' "$run_dir/mut2.log" || fail mut002-wrong-test
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "${ac001_tests[@]}" ;;
  AC-002) run_ac AC-002 "${ac002_tests[@]}" ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  AC-004) run_ac AC-004 "${ac004_tests[@]}" ;;
  AC-005) run_ac005 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "${ac001_tests[@]}"
    run_ac AC-002 "${ac002_tests[@]}"
    run_ac AC-003 "${ac003_tests[@]}"
    run_ac AC-004 "${ac004_tests[@]}"
    run_ac005
    run_mut001
    run_mut002
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
