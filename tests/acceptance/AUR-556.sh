#!/usr/bin/env bash
# AUR-556 acceptance: the embedded analysis catalog counts toward the policy
# gate, gate.sources restricts the origins, and without a gate nothing changes.
#
# Selectors:
#   all              AC-001..AC-004, then the mutation
#   AC-001           analysis finding + silent model + fail_on [error] fails the
#                    gate; origin analysis in stderr, audit, SARIF and --pr body
#   AC-002           sources [skills] does not fail; repo sources cannot change
#                    the policy's list; unknown source is a load error
#   AC-003           no gate declared: same diff, exit 0, no policy-gate trace
#   AC-004           corpus report: approved-with-defect zero for detected
#                    cases, recall not reduced, harness proofs, report current
#   AC-001-MUT-001   counting only skills again turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-556'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg tests/benchmark; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
for f in internal/gate/sources.go cmd/aurumcode/aur556_test.go internal/config/gate.go tests/benchmark/aur556_test.go; do
  [[ -f "$repo_root/$f" ]] || infra "missing-source:$f"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a556.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/root/tests"
for source in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$source" "$run_dir/root/$source"; done
cp -R "$repo_root/tests/benchmark" "$run_dir/root/tests/benchmark"
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

run_go_test() { # pkg pattern log
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 400s -v "$1" -run "$2") >"$3" 2>&1
  local status=$?
  set -e
  cat "$3" >&2
  return $status
}
need_pass() { grep -q "^--- PASS: $2 " "$1" || fail "missing-pass:$2"; }

ac001_pattern='^(TestAUR556AnalysisFindingFailsPolicyGate|TestAUR556PRPathCountsAnalysisFinding)$'

run_ac001() {
  local log="$run_dir/ac001.log"
  run_go_test ./cmd/aurumcode/ "$ac001_pattern" "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR556AnalysisFindingFailsPolicyGate
  need_pass "$log" TestAUR556PRPathCountsAnalysisFinding
}
run_ac002() {
  local log="$run_dir/ac002.log"
  run_go_test ./cmd/aurumcode/ '^(TestAUR556SourcesRestrictAndPolicyGoverns|TestAUR556GateSourcesValidation)$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR556SourcesRestrictAndPolicyGoverns
  need_pass "$log" TestAUR556GateSourcesValidation
}
run_ac003() {
  local log="$run_dir/ac003.log"
  run_go_test ./cmd/aurumcode/ '^TestAUR556NoGateUnchanged$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR556NoGateUnchanged
}
run_ac004() {
  local log="$run_dir/ac004.log"
  run_go_test ./tests/benchmark/ '^(TestAUR556|TestAUR523)' "$log" || fail 'go-test-failed'
  for n in TestAUR556VersionedReportApprovesNoDetectedDefect TestAUR556ApprovedRequiresExitZero \
           TestAUR556CaseWithoutManifestRefused TestAUR556PolicyByteChangesHeaderDigest \
           TestAUR523ReportPerLanguage; do need_pass "$log" "$n"; done
}
run_mutation() {
  local target="$run_dir/root/internal/gate/sources.go"
  local anchor='if !gate.Declared() || !gate.SourceEnabled(config.GateSourceAnalysis) {'
  [[ "$(grep -Fc "$anchor" "$target")" == "1" ]] || infra mutation-anchor
  sed -i "s/if !gate.Declared() || !gate.SourceEnabled(config.GateSourceAnalysis) {/if true { \/\/ MUT-001: only skills count/" "$target"
  grep -Fq 'MUT-001: only skills count' "$target" || infra mutation-not-applied
  local log="$run_dir/mutation.log"
  run_go_test ./cmd/aurumcode/ "$ac001_pattern" "$log" || true
  grep -Eq 'build failed|undefined:|syntax error|declared and not used' "$log" && fail 'mutation-build-failure-not-behavioral'
  grep -q '^--- FAIL: TestAUR556AnalysisFindingFailsPolicyGate' "$log" || fail 'mutation-survived'
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  AC-001-MUT-001) run_mutation ;;
  all) run_ac001; run_ac002; run_ac003; run_ac004; run_mutation ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
