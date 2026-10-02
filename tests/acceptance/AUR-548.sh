#!/usr/bin/env bash
# AUR-548 acceptance: Semgrep, a multi-language SAST pass, runs over the
# whole reviewed tree through quality_gates.sast and feeds its own,
# independent gate decision (applySASTGate, cmd/aurumcode/aur548.go),
# folded into the same gateDecision AUR-519's evaluateGate already
# produces. See docs/specs/AUR-548.md for the full account. Modeled on
# tests/acceptance/AUR-537.sh.
#
# Selectors:
#   all             run every behavior test below, then apply the MUT-001
#                   mutation and confirm it turns AC-003's own tests red
#   AC-001          a Semgrep ERROR-severity finding fails the gate
#                   (exitFindings), naming the check_id and line in the
#                   published output -- including when a central policy
#                   enables SAST and the repository tries to disable it
#   AC-002          a finding below fail_on_severity is produced but does
#                   not fail the gate
#   AC-003          Semgrep absent, erroring, or returning invalid JSON
#                   (or JSON with no "results" key) all become
#                   inconclusive under gate.inconclusive: block
#                   (exitQualityNotReviewed) -- contrasted against a real
#                   clean scan, which still passes
#   AC-004          rule packs come from quality_gates.sast.rule_packs, or
#                   the RFC's own documented defaults when absent; with no
#                   quality_gates.sast section at all, Semgrep is never
#                   invoked
#   AC-005          a model reply that claims to have removed or
#                   downgraded the Semgrep finding has no effect on the
#                   gate
#   AC-003-MUT-001  treat a Semgrep execution error as zero findings
#                   (restore the exact defect this card guards against);
#                   AC-003's own tests must go RED
# A build failure during the mutation run is infrastructure, never a
# silently-passing mutation. Unknown selectors exit 64; infrastructure
# failures exit 79; behavioral failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-548'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-003-MUT-001) ;;
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
for source in \
  cmd/aurumcode/main.go \
  cmd/aurumcode/aur548.go \
  cmd/aurumcode/policygate.go \
  cmd/aurumcode/aur521.go \
  internal/analysis/semgrep.go \
  internal/config/sast.go \
  internal/config/central.go; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done
for behavior in \
  cmd/aurumcode/aur548_test.go \
  cmd/aurumcode/aur519_e2e_test.go \
  cmd/aurumcode/aur476_test.go; do
  [[ -f "$repo_root/$behavior" ]] || infra "missing-behavior-test:$behavior"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a548.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp"
seed_root() {
  rm -rf "$run_dir/root"
  mkdir -p "$run_dir/root"
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$run_dir/root/$source"
  done
  chmod -R u+w -- "$run_dir/root"
}
seed_root

# No real Semgrep, no network: every test in aur548_test.go drives a fake
# "semgrep" executable placed on PATH by the test itself (t.Setenv("PATH",
# ...)), exactly like aur476_test.go's own fake "jq"/"aurumcode" binaries.
# This script adds nothing to PATH itself; it only runs `go test`.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# AC-003-MUT-001: restore the exact defect this card guards against --
# treating a Semgrep execution failure (the runner itself erroring, with
# no parseable JSON left to fall back on) as a clean, zero-finding scan.
# Anchored on internal/analysis/semgrep.go's own wrap of a genuine runner
# error, unique in the file.
apply_mutation_zero_findings() {
  local target="$run_dir/root/internal/analysis/semgrep.go"
  local anchor='return nil, fmt.Errorf("semgrep: execution failed: %w", runErr)'
  local replacement='return []Finding{}, nil'
  local count anchor_line
  count="$(grep -Fc "$anchor" "$target")" || infra mutation-anchor-missing
  [[ "$count" == "1" ]] || infra mutation-anchor-missing
  anchor_line="$(grep -Fn "$anchor" "$target" | head -1 | cut -d: -f1)"
  [[ -n "$anchor_line" ]] || infra mutation-anchor-missing
  sed -i "${anchor_line}s/.*/\t\t\t${replacement}/" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  grep -Fq "$replacement" "$target" || infra mutation-not-applied
  return 0
}

run_go_test() {
  local pattern="$1" log="$2"
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v ./cmd/aurumcode/... -run "$pattern") >"$log" 2>&1
  local status=$?
  set -e
  cat "$log" >&2
  return $status
}

check_mutation_red() {
  local log="$1"
  grep -Eq -- '^--- FAIL: TestAUR548' "$log" || fail 'mutation-survived'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
}

ac001_pattern='^(TestAUR548SeverityBreachFailsGate|TestAUR548PolicyWinsOverRepoDisable)$'
ac002_pattern='^(TestAUR548BelowThresholdDoesNotFailGate)$'
ac003_pattern='^(TestAUR548AbsentSemgrepIsInconclusiveNeverClean|TestAUR548ExecutionFailureIsInconclusive|TestAUR548CleanScanPasses)$'
ac004_pattern='^(TestAUR548NoConfigNeverInvokesSemgrep|TestAUR548RulePacksFromConfig|TestAUR548DefaultRulePacks)$'
ac005_pattern='^(TestAUR548ModelCannotRemoveOrDowngradeFinding)$'

case "$selector" in
  AC-001)
    log="$run_dir/test.log"
    run_go_test "$ac001_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in SeverityBreachFailsGate PolicyWinsOverRepoDisable; do
      grep -q "^--- PASS: TestAUR548$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002)
    log="$run_dir/test.log"
    run_go_test "$ac002_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR548BelowThresholdDoesNotFailGate " "$log" || fail "missing-pass:BelowThresholdDoesNotFailGate"
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003)
    log="$run_dir/test.log"
    run_go_test "$ac003_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in AbsentSemgrepIsInconclusiveNeverClean ExecutionFailureIsInconclusive CleanScanPasses; do
      grep -q "^--- PASS: TestAUR548$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-004)
    log="$run_dir/test.log"
    run_go_test "$ac004_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in NoConfigNeverInvokesSemgrep RulePacksFromConfig DefaultRulePacks; do
      grep -q "^--- PASS: TestAUR548$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-005)
    log="$run_dir/test.log"
    run_go_test "$ac005_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR548ModelCannotRemoveOrDowngradeFinding " "$log" || fail "missing-pass:ModelCannotRemoveOrDowngradeFinding"
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003-MUT-001)
    log="$run_dir/mutation.log"
    apply_mutation_zero_findings
    run_go_test "$ac003_pattern" "$log" || true
    check_mutation_red "$log"
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    log="$run_dir/test.log"
    run_go_test '^TestAUR548' "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in \
      SeverityBreachFailsGate PolicyWinsOverRepoDisable \
      BelowThresholdDoesNotFailGate \
      AbsentSemgrepIsInconclusiveNeverClean ExecutionFailureIsInconclusive CleanScanPasses \
      NoConfigNeverInvokesSemgrep RulePacksFromConfig DefaultRulePacks \
      ModelCannotRemoveOrDowngradeFinding; do
      grep -q "^--- PASS: TestAUR548$name " "$log" || fail "missing-pass:$name"
    done

    mutlog="$run_dir/mutation.log"
    apply_mutation_zero_findings
    run_go_test "$ac003_pattern" "$mutlog" || true
    check_mutation_red "$mutlog"

    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
