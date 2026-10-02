#!/usr/bin/env bash
# AUR-524 v2 acceptance: a concluded gate verdict is reused MONOTONICALLY
# -- a stored entry can only ADD findings to a later run's own, never
# replace or suppress them (AURUMCODE_CACHE_DIR is untrusted input in CI,
# CR-TRUST-001). See cmd/aurumcode/aur524.go and docs/specs/AUR-524.md.
#
# Selectors:
#   all        run every behavior test below, then apply the MUT-001
#              mutation and confirm AC-002's own tests turn RED
#   AC-001     a provider that FAILS then APPROVES under the identical
#              reviewed content/policy/context/model must still FAIL its
#              second run (proven for --base and --pr); a MISS that
#              stores a clean result never suppresses a later, fresh
#              breach
#   AC-002     changing a repo skill, the model, the prompt-version
#              digest, or a central policy's own digest invalidates reuse
#   AC-003     an inconclusive (provider-failure) run never reuses a prior
#              verdict and never becomes a stored one either
#   AC-004     without AURUMCODE_CACHE_DIR set, a run with a gate declared
#              says plainly that verdict reuse is unavailable
#   AC-005     a forged, empty cache entry (simulating an untrusted cache
#              scope) never produces an approval
#   AC-006     a reused entry is re-evaluated against the CURRENT run's
#              rule config, not the config active when it was written
#   AC-002-MUT-001  drop the policy digest term from gateVerdictCacheKey's
#              own combination (the exact defect MUT-001 names); AC-002's
#              own central-policy test must go RED
# A build failure during the mutation run is infrastructure, never a
# silently-passing mutation. Unknown selectors exit 64; infrastructure
# failures exit 79; behavioral failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-524'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|AC-002-MUT-001) ;;
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
  cmd/aurumcode/aur524.go \
  cmd/aurumcode/main.go \
  cmd/aurumcode/pr.go \
  cmd/aurumcode/review_cache.go \
  cmd/aurumcode/policygate.go \
  internal/review/cache/cache.go \
  internal/render/audit.go \
  internal/config/rules.go \
  internal/prompt/builder.go; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done
[[ -f "$repo_root/cmd/aurumcode/aur524_test.go" ]] || infra "missing-behavior-test:cmd/aurumcode/aur524_test.go"

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a524.XXXXXX")" || infra mktemp
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

# AC-002-MUT-001: drop the policy digest term from gateVerdictCacheKey's
# own combination -- the exact defect this card's gate checklist forbids
# ("never cross policies"). Anchored on the composite-literal VALUES line
# in aur524.go (`{inner, in.PolicyDigest, in.PromptVersionDigest, ...}`);
# replacing `in.PolicyDigest` with `""` keeps the struct's field count
# intact (still compiles) while making the key's policy term constant,
# i.e. absent in all but name.
apply_mutation_no_policy_digest() {
  local target="$run_dir/root/cmd/aurumcode/aur524.go"
  local anchor='{inner, in.PolicyDigest, in.PromptVersionDigest'
  local mutated='{inner, "", in.PromptVersionDigest'
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  MUT_OLD="$anchor" MUT_NEW="$mutated" awk '
    { idx = index($0, ENVIRON["MUT_OLD"])
      if (idx > 0) { $0 = substr($0, 1, idx - 1) ENVIRON["MUT_NEW"] substr($0, idx + length(ENVIRON["MUT_OLD"])) }
      print
    }' "$target" > "$target.tmp" && mv "$target.tmp" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  grep -Fq "$mutated" "$target" || infra mutation-not-applied
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
  grep -Eq -- '^--- FAIL: TestAUR524' "$log" || fail 'mutation-survived'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
}

ac001_pattern='^(TestAUR524AC001StickyFailAcrossRuns|TestAUR524AC001PRStickyFailInsideHunk|TestAUR524AC001CleanThenBreachStillFails)$'
ac002_pattern='^(TestAUR524AC002SkillChangeInvalidatesReuse|TestAUR524AC002ModelChangeInvalidatesReuse|TestAUR524AC002PromptVersionChangeInvalidatesReuse|TestAUR524AC002CentralPolicyChangeInvalidatesReuse)$'
ac003_pattern='^(TestAUR524AC003InconclusiveNeverStoredOrReused)$'
ac004_pattern='^(TestAUR524AC004NoCacheDirDeclaresUnavailable)$'
ac005_pattern='^(TestAUR524AC005ForgedEmptyEntryNeverApproves)$'
ac006_pattern='^(TestAUR524AC006RuleConfigReappliedOnReuse)$'

case "$selector" in
  AC-001)
    log="$run_dir/test.log"
    run_go_test "$ac001_pattern" "$log"; status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in StickyFailAcrossRuns PRStickyFailInsideHunk CleanThenBreachStillFails; do
      grep -q "^--- PASS: TestAUR524AC001$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002)
    log="$run_dir/test.log"
    run_go_test "$ac002_pattern" "$log"; status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in SkillChangeInvalidatesReuse ModelChangeInvalidatesReuse PromptVersionChangeInvalidatesReuse CentralPolicyChangeInvalidatesReuse; do
      grep -q "^--- PASS: TestAUR524AC002$name " "$log" || fail "missing-pass:$name"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003)
    log="$run_dir/test.log"
    run_go_test "$ac003_pattern" "$log"; status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q '^--- PASS: TestAUR524AC003InconclusiveNeverStoredOrReused ' "$log" || fail missing-pass:InconclusiveNeverStoredOrReused
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-004)
    log="$run_dir/test.log"
    run_go_test "$ac004_pattern" "$log"; status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q '^--- PASS: TestAUR524AC004NoCacheDirDeclaresUnavailable ' "$log" || fail missing-pass:NoCacheDirDeclaresUnavailable
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-005)
    log="$run_dir/test.log"
    run_go_test "$ac005_pattern" "$log"; status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q '^--- PASS: TestAUR524AC005ForgedEmptyEntryNeverApproves ' "$log" || fail missing-pass:ForgedEmptyEntryNeverApproves
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-006)
    log="$run_dir/test.log"
    run_go_test "$ac006_pattern" "$log"; status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q '^--- PASS: TestAUR524AC006RuleConfigReappliedOnReuse ' "$log" || fail missing-pass:RuleConfigReappliedOnReuse
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002-MUT-001)
    log="$run_dir/mutation.log"
    apply_mutation_no_policy_digest
    run_go_test "$ac002_pattern" "$log" || true
    check_mutation_red "$log"
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    log="$run_dir/test.log"
    run_go_test '^TestAUR524' "$log"; status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in \
      AC001StickyFailAcrossRuns AC001PRStickyFailInsideHunk AC001CleanThenBreachStillFails \
      AC002SkillChangeInvalidatesReuse AC002ModelChangeInvalidatesReuse \
      AC002PromptVersionChangeInvalidatesReuse AC002CentralPolicyChangeInvalidatesReuse \
      AC003InconclusiveNeverStoredOrReused AC004NoCacheDirDeclaresUnavailable \
      AC005ForgedEmptyEntryNeverApproves AC006RuleConfigReappliedOnReuse; do
      grep -q "^--- PASS: TestAUR524$name " "$log" || fail "missing-pass:$name"
    done

    mutlog="$run_dir/mutation.log"
    apply_mutation_no_policy_digest
    run_go_test "$ac002_pattern" "$mutlog" || true
    check_mutation_red "$mutlog"

    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
