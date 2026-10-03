#!/usr/bin/env bash
# AUR-537 acceptance: on `--pr`, a provider/transport failure during the
# actual model call (every configured provider failed,
# llm.ErrAllProvidersFailed; or --limite refused the call before any
# provider was reached, llm.ErrBudgetExceeded) now routes through the
# AUR-519 policy gate as the inconclusive reason "provider_failure" instead
# of returning exit 1 before the gate is ever reached, AND the legacy
# aurumcode/review check status (AUR-439) must never read "success" for that
# run, in block or warn alike. See docs/specs/AUR-537.md for the full
# account.
#
# Selectors:
#   all             run every behavior test below, then also apply both
#                   mutations (transport and --limite) and confirm each
#                   turns its own tests red
#   AC-001          gate.inconclusive: block publishes aurumcode/policy-gate
#                   AND aurumcode/review as failure naming provider_failure,
#                   exits the "quality not reviewed" code, and writes an
#                   audit record/SARIF document whose gate/decision reads
#                   fail/inconclusive with executionSuccessful=false -- for
#                   both the transport failure and the --limite refusal
#   AC-002          gate.inconclusive: warn publishes aurumcode/policy-gate
#                   as success carrying a visible inconclusive alert --
#                   never the "aprovado" word -- while aurumcode/review
#                   still reads failure naming provider_failure; exits 0
#                   (with --exigir-qualidade the stricter check exits with the
#                   review-not-completed code instead) -- for both the
#                   transport failure and the --limite refusal
#   AC-003          with no `gate:` key declared anywhere, the failure stays
#                   byte-identical to the command's behavior before this
#                   card: exit 1, no status published, no audit/SARIF file
#                   written even when both paths are given -- for both the
#                   transport failure and the --limite refusal
#   AC-001-MUT-001  restore the unconditional early return on a provider
#                   TRANSPORT failure (the exact defect this card fixes);
#                   AC-001's own tests must go RED
#   AC-001-MUT-002  restore the same unconditional early return on the
#                   --limite/ErrBudgetExceeded branch specifically; AC-001's
#                   own budget tests must go RED
# A build failure during any mutation run is infrastructure, never a
# silently-passing mutation. Unknown selectors exit 64; infrastructure
# failures exit 79; behavioral failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-537'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-001-MUT-001|AC-001-MUT-002) ;;
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
  cmd/aurumcode/pr.go \
  cmd/aurumcode/provider_failure.go \
  cmd/aurumcode/policygate.go \
  cmd/aurumcode/compliance_artifacts.go; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done
for behavior in cmd/aurumcode/aur537_test.go cmd/aurumcode/aur519_e2e_test.go; do
  [[ -f "$repo_root/$behavior" ]] || infra "missing-behavior-test:$behavior"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a537.XXXXXX")" || infra mktemp
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

# AC-001-MUT-001: restore the exact defect this card fixes -- returning
# before the policy gate on a provider TRANSPORT failure, even when a gate
# is declared. Anchored on `rc = 1` (unique in review_pr_generate.go: the generic
# llm.ErrAllProvidersFailed branch with no --modelo) and, two lines below
# it, the `if !gateDeclared {` this card added; replacing that one
# occurrence with `if true {` makes the function return unconditionally
# again, exactly as it did before AUR-537.
apply_mutation_transport() {
  local target="$run_dir/root/cmd/aurumcode/review_pr_generate.go"
  local anchor='rc = 1'
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  local anchor_line
  anchor_line="$(grep -Fn "$anchor" "$target" | head -1 | cut -d: -f1)"
  [[ -n "$anchor_line" ]] || infra mutation-anchor-missing
  local gate_line=$((anchor_line + 2))
  sed -n "${gate_line}p" "$target" | grep -Fq 'if !gateDeclared {' || infra mutation-anchor-missing
  sed -i "${gate_line}s/if !gateDeclared {/if true {/" "$target"
  sed -n "${gate_line}p" "$target" | grep -Fq 'if !gateDeclared {' && infra mutation-not-applied
  sed -n "${gate_line}p" "$target" | grep -Fq 'if true {' || infra mutation-not-applied
  return 0
}

# AC-001-MUT-002: the same defect, this time on the --limite/
# ErrBudgetExceeded branch. Anchored on `rc := reportBudgetExceeded(stderr,
# limiteUSD, err)` (unique in review_pr_generate.go) and, one line below it, its own
# `if !gateDeclared {`.
apply_mutation_budget() {
  local target="$run_dir/root/cmd/aurumcode/review_pr_generate.go"
  local anchor='rc := reportBudgetExceeded(stderr, limiteUSD, err)'
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  local anchor_line
  anchor_line="$(grep -Fn "$anchor" "$target" | head -1 | cut -d: -f1)"
  [[ -n "$anchor_line" ]] || infra mutation-anchor-missing
  local gate_line=$((anchor_line + 1))
  sed -n "${gate_line}p" "$target" | grep -Fq 'if !gateDeclared {' || infra mutation-anchor-missing
  sed -i "${gate_line}s/if !gateDeclared {/if true {/" "$target"
  sed -n "${gate_line}p" "$target" | grep -Fq 'if !gateDeclared {' && infra mutation-not-applied
  sed -n "${gate_line}p" "$target" | grep -Fq 'if true {' || infra mutation-not-applied
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
  grep -Eq -- '^--- FAIL: TestAUR537' "$log" || fail 'mutation-survived'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
}

ac001_pattern='^(TestAUR537ProviderFailureBlocksGate|TestAUR537MutationReturnsBeforeGate|TestAUR537BudgetExceededBlocksGate|TestAUR537BudgetMutationReturnsBeforeGate)$'
ac002_pattern='^(TestAUR537ProviderFailureWarnsGate|TestAUR537ProviderFailureExigirQualidadeStillInconclusive|TestAUR537BudgetExceededWarnsGate)$'
ac003_pattern='^(TestAUR537NoGateProviderFailureUnchanged|TestAUR537BudgetExceededNoGateUnchanged)$'

case "$selector" in
  AC-001)
    log="$run_dir/test.log"
    run_go_test "$ac001_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in ProviderFailureBlocksGate MutationReturnsBeforeGate BudgetExceededBlocksGate BudgetMutationReturnsBeforeGate; do
      grep -q "^--- PASS: TestAUR537$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002)
    log="$run_dir/test.log"
    run_go_test "$ac002_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in ProviderFailureWarnsGate ProviderFailureExigirQualidadeStillInconclusive BudgetExceededWarnsGate; do
      grep -q "^--- PASS: TestAUR537$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003)
    log="$run_dir/test.log"
    run_go_test "$ac003_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in NoGateProviderFailureUnchanged BudgetExceededNoGateUnchanged; do
      grep -q "^--- PASS: TestAUR537$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-001-MUT-001)
    log="$run_dir/mutation.log"
    apply_mutation_transport
    run_go_test "$ac001_pattern" "$log" || true
    check_mutation_red "$log"
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  AC-001-MUT-002)
    log="$run_dir/mutation.log"
    apply_mutation_budget
    run_go_test "$ac001_pattern" "$log" || true
    check_mutation_red "$log"
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    log="$run_dir/test.log"
    run_go_test '^TestAUR537' "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in \
      ProviderFailureBlocksGate ProviderFailureWarnsGate \
      ProviderFailureExigirQualidadeStillInconclusive \
      NoGateProviderFailureUnchanged MutationReturnsBeforeGate \
      BudgetExceededBlocksGate BudgetExceededWarnsGate \
      BudgetExceededNoGateUnchanged BudgetMutationReturnsBeforeGate; do
      grep -q "^--- PASS: TestAUR537$name " "$log" || fail "missing-pass:$name"
    done

    translog="$run_dir/mutation-transport.log"
    apply_mutation_transport
    run_go_test "$ac001_pattern" "$translog" || true
    check_mutation_red "$translog"

    # Each mutation is checked in isolation: re-seed the pristine tree
    # before the second one, so the budget mutation's own RED is never
    # confounded by the transport mutation already applied above.
    seed_root
    budgetlog="$run_dir/mutation-budget.log"
    apply_mutation_budget
    run_go_test "$ac001_pattern" "$budgetlog" || true
    check_mutation_red "$budgetlog"

    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
