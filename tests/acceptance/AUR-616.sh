#!/usr/bin/env bash
# AUR-616 acceptance: analysts (review.profiles, --perfis) deliberate with
# the same tools as the single reviewer; the rounds that ask for a tool and
# the deliberation cost are summed over the analysts against the review's
# own ceiling; the terminal names the real reason when nothing is offered
# (provider without tool calling, deliberation off in the configuration,
# the review's tool rounds already spent); the audit records each
# analyst's rounds; without analysts nothing changes.
#
# Selectors:
#   all        AC-001..AC-004, then MUT-001 and MUT-002
#   AC-001     each analyst's pass is offered the tools and its findings keep
#              the analyst's label; the profile decorator keeps tool calling
#   AC-002     tool rounds and cost tokens are summed over the analysts
#   AC-003     the message names the provider, the configuration key or the
#              spent budget, and the catalog has the lines in both locales
#   AC-004     without analysts the single reviewer deliberates as before
#   MUT-001    offering no tool when analysts are selected turns AC-001 RED
#   MUT-002    giving each analyst its own limits (no shared budget) turns
#              AC-002 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-616'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg cmd/aurumcode/aur616_test.go cmd/aurumcode/profiles_deliberation.go cmd/aurumcode/review_base_quality.go internal/deliberation/budget.go internal/deliberation/ledger.go internal/i18n/catalog.yml; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a616.XXXXXX")" || infra mktemp
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
  for source in go.mod go.sum cmd internal pkg; do
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  if [[ -d "$repo_root/tests/fixtures" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/tests/fixtures" "$root/tests/fixtures"; fi
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

readonly ac001_tests=(TestAUR616AnalystsDeliberateWithTools TestAUR526ProfileDecoratorForwardsToolCalling TestAUR616MergeAttributesCallsToAnalysts)
readonly ac002_tests=(TestAUR616ToolRoundsAreSummedOverTheAnalysts TestAUR616CostTokensAreSummedOverTheAnalysts TestAUR616SessionsShareTheBudgetRounds TestAUR616AskingAfterTheBudgetIsSpentIsARoundsLimit TestAUR616SessionsShareTheCostCeiling)
readonly ac003_tests=(TestAUR616MessagesNameTheRealReason TestAUR616ToolRoundsAreSummedOverTheAnalysts TestAUR616DeliberationKeysExistInBothLocales TestEveryKeyExistsInEveryLocale)
readonly ac004_tests=(TestAUR616WithoutAnalystsNothingChanges TestAUR580ModelAsksForTheScannerOnALargeDiff TestAUR580ModelDoesNotAskOnASmallDiff TestAUR580MaxRoundsIsALimitErrorWithoutAnswer TestAUR580MaxCostTokensIsALimitError TestAUR580DeferredScannerRunsWhenToolsCannotBeOffered)
readonly pkgs=(./cmd/aurumcode/ ./internal/deliberation/ ./internal/i18n/ ./internal/review/)

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

# MUT-001: with analysts selected no tool is offered, as before the card.
run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/cmd/aurumcode/review_base_quality.go" \
    'b.prepareDeliberation(orchestrator, reviewer, len(analystsOf(b.profileRes)))' \
    'if !b.profilesApplied { b.prepareDeliberation(orchestrator, reviewer, 0) } // MUT-001'
  expect_red "$root" "$run_dir/mut1.log" "${ac001_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR616AnalystsDeliberateWithTools' "$run_dir/mut1.log" || fail mut001-wrong-test
  printf '%s/MUT-001/rejected\n' "$card"
}

# MUT-002: each analyst gets its own limits instead of the review's budget.
run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/cmd/aurumcode/profiles_deliberation.go" \
    'reviewer.SetDeliberation(a.plan.forCaller(orchestrator, a.plan.budget))' \
    'reviewer.SetDeliberation(a.plan.forCaller(orchestrator, nil)) // MUT-002'
  expect_red "$root" "$run_dir/mut2.log" "${ac002_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR616ToolRoundsAreSummedOverTheAnalysts' "$run_dir/mut2.log" || fail mut002-rounds-not-summed
  grep -Eq -- '^--- FAIL: TestAUR616CostTokensAreSummedOverTheAnalysts' "$run_dir/mut2.log" || fail mut002-cost-not-summed
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "${ac001_tests[@]}" ;;
  AC-002) run_ac AC-002 "${ac002_tests[@]}" ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  AC-004) run_ac AC-004 "${ac004_tests[@]}" ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "${ac001_tests[@]}"
    run_ac AC-002 "${ac002_tests[@]}"
    run_ac AC-003 "${ac003_tests[@]}"
    run_ac AC-004 "${ac004_tests[@]}"
    run_mut001
    run_mut002
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
