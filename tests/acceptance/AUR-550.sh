#!/usr/bin/env bash
# AUR-550 acceptance: a SBOM (an already-generated CycloneDX fixture --
# this card never generates one, AUR-549's own job) is submitted to a
# configured OWASP Dependency-Track v5 server (POST /api/v1/bom,
# multipart "project"+"bom", header X-Api-Key), the upload's processing
# token is polled (GET /api/v1/bom/token/{token}) until the server itself
# reports processing:false or the configured timeout elapses, and the
# project's current metrics (GET /api/v1/metrics/project/{project}/current)
# are compared against quality_gates.ssor_dtrack.thresholds. A breach
# fails the gate with the metric numbers published; an unreachable
# server, an HTTP error or a polling timeout make the gate inconclusive
# per policy, never approved. See docs/specs/AUR-550.md for the full
# account.
#
# Selectors:
#   all       run every behavior test below, then apply the skeptical
#             mutation and confirm it turns AC-003 red
#   AC-001    metrics over threshold fail the gate; the metric numbers
#             reach stderr/limitations and the audit record's blocking
#             findings
#   AC-002    metrics within every threshold approve this part of the
#             gate (exit 0)
#   AC-003    polling timeout, HTTP error (401) and an unreachable server
#             all read as inconclusive per policy, never approved
#   AC-004    the API key canary: the fake server echoes the key back in
#             its own error body, the server genuinely received it (the
#             test is not vacuous), yet it is absent from stdout, stderr,
#             the audit record and SARIF alike
#   AC-005    the env var NAMES the client reads come only from
#             api_key_secret/project_id_secret (a custom pair of names
#             works identically); a run with no ssor_dtrack section makes
#             zero HTTP calls at all -- nothing is hardcoded
#   AC-003-MUT-001  treat a polling timeout as approved (the exact defect
#                   this card refuses to allow); AC-003's own timeout test
#                   must go RED
# A build failure during the mutation run is infrastructure, never a
# silently-passing mutation. Unknown selectors exit 64; infrastructure
# failures exit 79; behavioral failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-550'
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
  cmd/aurumcode/aur550.go \
  cmd/aurumcode/policygate.go \
  cmd/aurumcode/pr.go \
  cmd/aurumcode/main.go \
  internal/dtrack/client.go \
  internal/config/dtrack.go \
  internal/config/central.go; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done
for behavior in cmd/aurumcode/aur550_test.go internal/dtrack/client_test.go; do
  [[ -f "$repo_root/$behavior" ]] || infra "missing-behavior-test:$behavior"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a550.XXXXXX")" || infra mktemp
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

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# AC-003-MUT-001: internal/dtrack.Client.PollUntilProcessed has exactly
# one place a timeout is ever declared (unique in the file on purpose --
# see client.go's own comment at that line). Replacing it with
# `return nil` treats an unfinished scan as processed, falling through to
# "approved" whenever the server also happens to report clean metrics --
# the exact defect this card exists to refuse.
apply_mutation_timeout_approved() {
  local target="$run_dir/root/internal/dtrack/client.go"
  local anchor='return fmt.Errorf("%w", ErrTimeout)'
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  local count
  count="$(grep -Fc "$anchor" "$target")"
  [[ "$count" == "1" ]] || infra mutation-anchor-not-unique
  local line
  line="$(grep -Fn "$anchor" "$target" | head -1 | cut -d: -f1)"
  [[ -n "$line" ]] || infra mutation-anchor-missing
  # Whole-line replacement by line number, not a pattern substitution:
  # the anchor's own parentheses/quotes are plain text here, never a
  # regex this tool has to escape correctly.
  sed -i "${line}s/.*/\t\t\treturn nil/" "$target"
  sed -n "${line}p" "$target" | grep -Fq "$anchor" && infra mutation-not-applied
  sed -n "${line}p" "$target" | grep -Fq 'return nil' || infra mutation-not-applied
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
  grep -Eq -- '^--- FAIL: TestAUR550PollingTimeoutIsInconclusive' "$log" || fail 'mutation-survived'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
}

ac001_pattern='^TestAUR550BreachFailsGateWithNumbers$'
ac002_pattern='^TestAUR550WithinLimitsApproves$'
ac003_pattern='^(TestAUR550PollingTimeoutIsInconclusive|TestAUR550HTTPErrorIsInconclusive|TestAUR550UnreachableServerIsInconclusive)$'
ac004_pattern='^TestAUR550APIKeyNeverLeaks$'
ac005_pattern='^(TestAUR550SecretNamesAreConfigurable|TestAUR550DisabledByDefaultNoOp)$'

case "$selector" in
  AC-001)
    log="$run_dir/test.log"
    run_go_test "$ac001_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR550BreachFailsGateWithNumbers " "$log" || fail 'missing-pass:BreachFailsGateWithNumbers'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002)
    log="$run_dir/test.log"
    run_go_test "$ac002_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR550WithinLimitsApproves " "$log" || fail 'missing-pass:WithinLimitsApproves'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003)
    log="$run_dir/test.log"
    run_go_test "$ac003_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in PollingTimeoutIsInconclusive HTTPErrorIsInconclusive UnreachableServerIsInconclusive; do
      grep -q "^--- PASS: TestAUR550$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-004)
    log="$run_dir/test.log"
    run_go_test "$ac004_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR550APIKeyNeverLeaks " "$log" || fail 'missing-pass:APIKeyNeverLeaks'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-005)
    log="$run_dir/test.log"
    run_go_test "$ac005_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in SecretNamesAreConfigurable DisabledByDefaultNoOp; do
      grep -q "^--- PASS: TestAUR550$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003-MUT-001)
    log="$run_dir/mutation.log"
    apply_mutation_timeout_approved
    run_go_test "$ac003_pattern" "$log" || true
    check_mutation_red "$log"
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    log="$run_dir/test.log"
    run_go_test '^TestAUR550' "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in \
      BreachFailsGateWithNumbers WithinLimitsApproves \
      PollingTimeoutIsInconclusive HTTPErrorIsInconclusive UnreachableServerIsInconclusive \
      APIKeyNeverLeaks SecretNamesAreConfigurable DisabledByDefaultNoOp; do
      grep -q "^--- PASS: TestAUR550$name " "$log" || fail "missing-pass:$name"
    done

    mutlog="$run_dir/mutation.log"
    apply_mutation_timeout_approved
    run_go_test "$ac003_pattern" "$mutlog" || true
    check_mutation_red "$mutlog"

    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
