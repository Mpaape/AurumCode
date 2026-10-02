#!/usr/bin/env bash
# AUR-519 acceptance.
#
# STATUS (see docs/specs/AUR-519.md for the full account): this run proves
# the foundation this card built -- dynamic skill-section rules
# (internal/review.ParseSkillSections/enforceRuleCitations), the policy's
# gate.fail_on/inconclusive config and its precedence over a repository's own
# (internal/config.GateConfig/ApplyCentralPolicy), the forge-safe degraded-
# parse flag (internal/prompt.IsDegradedParse), and the pure gate decision
# core (cmd/aurumcode.evaluateGate) -- all at the unit level. It does NOT yet
# prove the end-to-end CLI behavior AC-001, AC-003, AC-004 and AC-006 ask for
# (a real `aurumcode review --base/--pr` run that fails its exit code/commit
# status and names the skill+section in the published summary): evaluateGate
# is not yet called from runReview/runPRReview. Those selectors, and the
# MUT-001 mutation that depends on that wiring, report infrastructure exit 79
# naming exactly what is missing, never a false pass.
#
# Selectors:
#   all                 run every behavior test implemented so far
#   AC-001              not wired to the CLI yet (infra 79)
#   AC-002              a finding citing an unknown skill/section is
#                       discarded and counted as unlinked
#   AC-003              not wired to the CLI yet (infra 79)
#   AC-004              not wired to the CLI yet (infra 79)
#   AC-005              a repo-origin skill finding never fails a policy's
#                       gate, and ApplyCentralPolicy's own gate precedence
#   AC-006              not wired to the CLI yet (infra 79)
#   AC-007              adding a skill section makes it citable with no
#                       code change
#   AC-008              a model reply that is not valid JSON is flagged as a
#                       degraded parse, and the flag cannot be forged by a
#                       model supplying its own "metadata.parse_mode"
#   AC-003-MUT-001      not wired to the CLI yet (infra 79)
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-519'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|AC-007|AC-008|AC-003-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/internal/review/skillrules.go" ]] || infra missing-source
[[ -f "$repo_root/internal/config/gate.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/policygate.go" ]] || infra missing-source

# AC-001, AC-003, AC-004 and AC-006 need evaluateGate wired into
# runReview/runPRReview's own exit code, commit status and published
# summary -- a real `aurumcode review` run, not a unit call. That wiring
# does not exist yet (see this script's own header); report it honestly
# instead of asserting a selector this build cannot yet exhibit.
case "$selector" in
  AC-001|AC-003|AC-004|AC-006|AC-003-MUT-001)
    infra 'gate-not-wired-to-runReview-or-runPRReview'
    ;;
esac

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a519.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/root" "$run_dir/cache" "$run_dir/gotmp"
for source in go.mod go.sum cmd internal pkg; do
  cp -R "$repo_root/$source" "$run_dir/root/$source"
done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

test_pattern=''
pkgs='./internal/review/... ./internal/config/... ./internal/prompt/... ./cmd/aurumcode/...'
case "$selector" in
  all)    test_pattern='^TestAUR519' ;;
  AC-002) test_pattern='^TestAUR519EnforceRuleCitationsDynamic$' ;;
  AC-005) test_pattern='^(TestAUR519EvaluateGateRepoOriginNeverFails|TestAUR519ApplyCentralPolicyGate)$' ;;
  AC-007) test_pattern='^TestAUR519ParseSkillSections$' ;;
  AC-008) test_pattern='^(TestAUR519DegradedParseDetection|TestAUR519EvaluateGateInconclusiveBlockAndWarn)$' ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR519' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in ParseSkillSections EnforceRuleCitationsDynamic GateConfigThreshold GateConfigInconclusive ApplyCentralPolicyGate DegradedParseDetection EvaluateGateNoGateDeclared EvaluateGateSeverityBreach EvaluateGateRepoOriginNeverFails EvaluateGateInconclusiveBlockAndWarn MergedRuleCatalogIDs; do
    grep -q "^--- PASS: TestAUR519$name" "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
