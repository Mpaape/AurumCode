#!/usr/bin/env bash
# AUR-579 acceptance: the model weighs the deterministic evidence. The
# security pass, the embedded analysis and SAST run before the model; their
# findings reach the prompt's evidence section; the model's per-evidence
# assessment travels beside the engine's origin to the report, the audit and
# the SARIF; the gate keeps counting policy evidence whatever the model
# says (a dispute becomes a proposed exception); without a central policy,
# gate.triage: model lets a dispute demote; the cache key carries the
# evidence digest.
#
# Selectors:
#   all        AC-001..AC-005, then MUT-001, MUT-002 and the AC-004 mutation
#   AC-001     the prompt sent carries the findings of the three passes
#   AC-002     origin beside assessment in report, audit and SARIF; an
#              assessment of evidence never offered is discarded loudly
#   AC-003     policy floor + proposed exception; repository gate.triage
#   AC-004     the answer exists only because the evidence section reached
#              the prompt; removing the section (prompt/evidence.go) turns it
#              RED; the gate tutorial's recorded expected carries the
#              model's assessment
#   AC-005     the context/verdict key moves with the evidence
#   MUT-001    the model back before the passes (session.Order) turns AC-001 RED
#   MUT-002    a dispute allowed to demote under a central policy turns
#              AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-579'
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
for input in go.mod go.sum cmd internal pkg internal/review/session/session.go cmd/aurumcode/aur579_test.go cmd/aurumcode/review_gate.go internal/prompt/evidence.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a579.XXXXXX")" || infra mktemp
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

readonly ac001_tests=(TestAUR579PromptCarriesEveryPassBeforeTheModel TestEvidenceRunsBeforeTheModel TestRunFollowsTheOneOrder)
readonly ac002_tests=(TestAUR579AssessmentBesideOriginInEverySink TestWeighAssessmentsKeepsOnlyOfferedEvidence TestWeighAssessmentsWithoutEvidenceKeepsNothing TestModelAssessmentParsedAndOriginIgnored TestAssessmentOfEvidenceOmittedByTheCeilingIsDiscarded)
readonly ac003_tests=(TestAUR579PolicyFloorAndRepositoryTriage TestGateTriageParse TestTriageMatchesTheDisputeByOrigin)
readonly ac004_tests=(TestAUR579EvidenceSectionDecidesTheAnswer TestEvidenceSlotRendersRedactsAndDeclaresOmissions)
readonly ac005_tests=(TestAUR579ContextKeyIncludesEvidence TestRequestCacheKeyCoversEvidence)
readonly pkgs=(./cmd/aurumcode/ ./internal/review/ ./internal/review/session/ ./internal/config/ ./internal/gate/)

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

run_ac004() {
  run_ac ac004 "${ac004_tests[@]}"
  local expected="$repo_root/demo/tutoriais/gate/expected/modelo-pondera.txt"
  [[ -f "$expected" ]] || fail tutorial-expected-missing
  grep -Fq 'avaliacao do modelo: disputed' "$expected" || fail tutorial-without-assessment
  grep -Fq 'Excecoes propostas pelo modelo' "$expected" || fail tutorial-without-proposed-exception
  local root="$run_dir/root-mut-ac004"
  stage "$root"
  replace_once "$root/internal/prompt/evidence.go" \
    'text, admitted := renderBudgetedSlot(slotDeterministicEvidence, items, maxTokens, est)' \
    'text, admitted := "", []int(nil) // AC-004 mutation: the evidence section never reaches the prompt'
  expect_red "$root" "$run_dir/mut-ac004.log" TestAUR579EvidenceSectionDecidesTheAnswer
  printf '%s/AC-004/MUT/rejected\n' "$card"
}

run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/internal/review/session/session.go" \
    'var Order = []Phase{PhaseResolve, PhaseEvidence, PhaseModel, PhaseGate, PhasePublish}' \
    'var Order = []Phase{PhaseResolve, PhaseModel, PhaseEvidence, PhaseGate, PhasePublish}'
  expect_red "$root" "$run_dir/mut1.log" "${ac001_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR579PromptCarriesEveryPassBeforeTheModel' "$run_dir/mut1.log" || fail mut001-wrong-test
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/cmd/aurumcode/review_gate.go" \
    'if s.centralCfg != nil || s.cfg == nil || !s.cfg.Gate.Declared() {' \
    'if false && s.centralCfg != nil || s.cfg == nil || !s.cfg.Gate.Declared() { // MUT-002: a dispute demotes policy evidence'
  expect_red "$root" "$run_dir/mut2.log" "${ac003_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR579PolicyFloorAndRepositoryTriage' "$run_dir/mut2.log" || fail mut002-wrong-test
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "${ac001_tests[@]}" ;;
  AC-002) run_ac AC-002 "${ac002_tests[@]}" ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  AC-004) run_ac004 ;;
  AC-005) run_ac AC-005 "${ac005_tests[@]}" ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "${ac001_tests[@]}"
    run_ac AC-002 "${ac002_tests[@]}"
    run_ac AC-003 "${ac003_tests[@]}"
    run_ac004
    run_ac AC-005 "${ac005_tests[@]}"
    run_mut001
    run_mut002
    printf '%s/all/pass\n' "$card"
    ;;
esac
