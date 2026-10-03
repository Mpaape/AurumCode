#!/usr/bin/env bash
# AUR-570 acceptance: the Dependency-Track gate reads project metrics only
# after the server's policy evaluation settled (bom/token processing:false
# is not enough). The client refreshes the metrics when the key allows it,
# rereads metrics/project/{id}/current until lastOccurrence is later than the
# upload AND two consecutive readings coincide, and otherwise is
# inconclusive (dtrack_metrics_unsettled), never approved. See
# docs/specs/AUR-570.md.
#
# Selectors:
#   all       AC-001..AC-004, then the skeptical mutation
#   AC-001    metrics that change after processing:false: the gate breaches on
#             the settled value (client and gate level), never approves
#   AC-002    metrics that never settle: dtrack_metrics_unsettled; block fails,
#             warn does not approve
#   AC-003    an already settled server: same outcome as before (AUR-550 tests)
#   AC-004    the tutorial, re-recorded against the real Dependency-Track and
#             without dt_assenta: `run.sh --check` passes
#   AC-001-MUT-001  read the metrics at the first response again; AC-001 must
#                   go RED
# Unknown selectors exit 64; infrastructure failures 79; behavior failures 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-570'
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

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
for source in internal/dtrack/client.go internal/dtrack/settle.go internal/gate/dtrack.go \
  internal/dtrack/settle_test.go cmd/aurumcode/aur570_test.go \
  demo/tutoriais/sbom-dependency-track/run.sh docs/tutorials/sbom-dependency-track.md; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a570.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/root"
for source in go.mod go.sum cmd internal pkg; do
  cp -R "$repo_root/$source" "$run_dir/root/$source"
done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# gotest PKG PATTERN LOG: runs on the seeded copy; returns go's status.
gotest() {
  local status
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v "$1" -run "$2") >"$3" 2>&1
  status=$?
  set -e
  cat "$3" >&2
  return $status
}

need_pass() { grep -q "^--- PASS: $2 " "$1" || fail "missing-pass:$2"; }

ac001() {
  local l="$run_dir/ac001.log" t
  gotest ./internal/dtrack '^TestSettleReadsAfterPolicyEvaluationLands$' "$l" || fail 'dtrack-ac001'
  need_pass "$l" TestSettleReadsAfterPolicyEvaluationLands
  gotest ./cmd/aurumcode '^TestAUR570GateReadsSettledViolation$' "$l" || fail 'gate-ac001'
  need_pass "$l" TestAUR570GateReadsSettledViolation
}
ac002() {
  local l="$run_dir/ac002.log"
  gotest ./internal/dtrack '^(TestSettleNeverStableIsInconclusiveUnsettled|TestSettleStaleForeverIsInconclusiveUnsettled)$' "$l" || fail 'dtrack-ac002'
  need_pass "$l" TestSettleNeverStableIsInconclusiveUnsettled
  need_pass "$l" TestSettleStaleForeverIsInconclusiveUnsettled
  gotest ./cmd/aurumcode '^TestAUR570UnsettledIsInconclusivePerMode$' "$l" || fail 'gate-ac002'
  need_pass "$l" TestAUR570UnsettledIsInconclusivePerMode
}
ac003() {
  local l="$run_dir/ac003.log"
  gotest ./internal/dtrack '^(TestSettleAlreadySettledGradesAsBefore|TestSettleUnchangedSBOMSettlesThroughProjectAnalysis|TestSettleRefreshForbiddenIsToleratedAndRecorded|TestRunApprovesWithinThresholds|TestRunBreachesOverThresholds|TestRunTimesOutWithoutApproving|TestRunIncompleteMetricsIsInconclusiveNeverZero)$' "$l" || fail 'dtrack-ac003'
  gotest ./cmd/aurumcode '^TestAUR550' "$l" || fail 'aur550-regression'
}
ac004() {
  local d="$repo_root/demo/tutoriais/sbom-dependency-track" doc="$repo_root/docs/tutorials/sbom-dependency-track.md" f
  for f in "$d/run.sh" "$d/expected" "$doc"; do
    ! grep -rq 'dt_assenta\|assentamento:' "$f" || fail "ac004/contorno-ainda-presente:${f##*/}"
  done
  grep -q 'dtrack_metrics_unsettled' "$doc" || fail 'ac004/texto-sem-motivo'
  local l="$run_dir/check.log"
  bash "$d/run.sh" --check >"$l" 2>&1 || { cat "$l" >&2; fail 'ac004/check-falhou'; }
  grep -q '^CHECK OK$' "$l" || fail 'ac004/check-sem-ok'
}

# MUT-001: the one line that makes Run read the metrics through
# SettledMetrics is replaced by a plain first-response read.
mut001() {
  local target="$run_dir/root/internal/dtrack/client.go"
  local anchor='metrics, err := client.SettledMetrics('
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra mutation-anchor
  local line; line="$(grep -Fn "$anchor" "$target" | cut -d: -f1)"
  sed -i "${line}s/.*/\tmetrics, err := client.ProjectMetrics(ctx, project); _ = uploadedAt/" "$target"
  grep -Fq 'client.ProjectMetrics(ctx, project); _ = uploadedAt' "$target" || infra mutation-not-applied
  local l="$run_dir/mut.log"
  gotest ./internal/dtrack '^TestSettleReadsAfterPolicyEvaluationLands$' "$l" || true
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$l"; then fail 'mutation-build-failure'; fi
  grep -Eq -- '^--- FAIL: TestSettleReadsAfterPolicyEvaluationLands' "$l" || fail 'mutation-survived'
  gotest ./cmd/aurumcode '^TestAUR570GateReadsSettledViolation$' "$l" || true
  grep -Eq -- '^--- FAIL: TestAUR570GateReadsSettledViolation' "$l" || fail 'mutation-survived-gate'
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  AC-001-MUT-001) mut001; printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"; exit 0 ;;
  all) ac001; ac002; ac003; ac004; mut001 ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
